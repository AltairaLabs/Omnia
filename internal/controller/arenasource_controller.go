/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/arena/fetcher"
	"github.com/altairalabs/omnia/pkg/license"
)

// ArenaSource condition types
const (
	ArenaSourceConditionTypeReady             = "Ready"
	ArenaSourceConditionTypeFetching          = "Fetching"
	ArenaSourceConditionTypeArtifactAvailable = "ArtifactAvailable"
)

// Event reasons for ArenaSource
const (
	EventReasonFetchStarted   = "FetchStarted"
	EventReasonFetchSucceeded = "FetchSucceeded"
	EventReasonFetchFailed    = "FetchFailed"
	EventReasonArtifactStored = "ArtifactStored"
)

// fetchJob represents an in-progress fetch operation
type fetchJob struct {
	startTime time.Time
	cancel    context.CancelFunc
}

// fetchResult represents the result of a completed fetch operation
type fetchResult struct {
	artifact *fetcher.Artifact
	err      error
}

// ArenaSourceReconciler reconciles an ArenaSource object
type ArenaSourceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// ArtifactDir is the directory where artifacts are stored
	ArtifactDir string

	// ArtifactBaseURL is the base URL for serving artifacts
	ArtifactBaseURL string

	// LicenseValidator validates license for source types (defense in depth)
	LicenseValidator *license.Validator

	// inProgress tracks in-progress fetch operations
	inProgress sync.Map // map[types.NamespacedName]*fetchJob

	// results stores completed fetch results
	results sync.Map // map[types.NamespacedName]*fetchResult
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenasources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenasources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenasources/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ArenaSourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling ArenaSource", "name", req.Name, "namespace", req.Namespace)

	// Fetch the ArenaSource instance
	source := &omniav1alpha1.ArenaSource{}
	if err := r.Get(ctx, req.NamespacedName, source); err != nil {
		if apierrors.IsNotFound(err) {
			// Clean up any in-progress fetch
			if job, ok := r.inProgress.LoadAndDelete(req.NamespacedName); ok {
				job.(*fetchJob).cancel()
			}
			r.results.Delete(req.NamespacedName)
			log.Info("ArenaSource resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get ArenaSource")
		return ctrl.Result{}, err
	}

	// Initialize status if needed
	if source.Status.Phase == "" {
		source.Status.Phase = omniav1alpha1.ArenaSourcePhasePending
	}

	// Update observed generation
	source.Status.ObservedGeneration = source.Generation

	// Check if suspended
	if source.Spec.Suspend {
		log.Info("ArenaSource is suspended, skipping reconciliation")
		// Cancel any in-progress fetch
		if job, ok := r.inProgress.LoadAndDelete(req.NamespacedName); ok {
			job.(*fetchJob).cancel()
		}
		r.setCondition(source, ArenaSourceConditionTypeReady, metav1.ConditionFalse,
			"Suspended", "ArenaSource reconciliation is suspended")
		if err := r.Status().Update(ctx, source); err != nil {
			log.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// License check (defense in depth - webhooks are primary enforcement)
	if r.LicenseValidator != nil {
		sourceType := string(source.Spec.Type)
		if err := r.LicenseValidator.ValidateArenaSource(ctx, sourceType); err != nil {
			log.Info("Source type not allowed by license", "type", sourceType, "error", err)
			source.Status.Phase = omniav1alpha1.ArenaSourcePhaseError
			r.setCondition(source, ArenaSourceConditionTypeReady, metav1.ConditionFalse,
				"LicenseViolation", err.Error())
			if r.Recorder != nil {
				r.Recorder.Event(source, corev1.EventTypeWarning, "LicenseViolation",
					fmt.Sprintf("Source type %s requires Enterprise license", sourceType))
			}
			if statusErr := r.Status().Update(ctx, source); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{}, nil // Don't requeue - license must change
		}
	}

	// Parse interval duration
	interval, err := time.ParseDuration(source.Spec.Interval)
	if err != nil {
		log.Error(err, "Invalid interval format")
		r.setCondition(source, ArenaSourceConditionTypeReady, metav1.ConditionFalse,
			"InvalidInterval", fmt.Sprintf("Invalid interval format: %v", err))
		source.Status.Phase = omniav1alpha1.ArenaSourcePhaseError
		if statusErr := r.Status().Update(ctx, source); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{}, err
	}

	// Parse timeout duration
	timeout := 60 * time.Second
	if source.Spec.Timeout != "" {
		timeout, err = time.ParseDuration(source.Spec.Timeout)
		if err != nil {
			log.Error(err, "Invalid timeout format, using default")
			timeout = 60 * time.Second
		}
	}

	// Check if there's a completed result waiting
	if resultVal, ok := r.results.LoadAndDelete(req.NamespacedName); ok {
		result := resultVal.(*fetchResult)
		r.inProgress.Delete(req.NamespacedName)

		if result.err != nil {
			log.Error(result.err, "Fetch completed with error")
			r.handleFetchError(ctx, source, result.err)
			return ctrl.Result{RequeueAfter: interval}, nil
		}

		// Store the artifact
		artifactURL, err := r.storeArtifact(source, result.artifact)
		if err != nil {
			log.Error(err, "Failed to store artifact")
			r.handleFetchError(ctx, source, err)
			// Clean up temp file
			if result.artifact != nil && result.artifact.Path != "" {
				_ = os.Remove(result.artifact.Path)
			}
			return ctrl.Result{RequeueAfter: interval}, nil
		}

		// Clean up temp file
		if result.artifact != nil && result.artifact.Path != "" {
			_ = os.Remove(result.artifact.Path)
		}

		// Update status with artifact info
		source.Status.Artifact = &omniav1alpha1.Artifact{
			Revision:       result.artifact.Revision,
			URL:            artifactURL,
			Checksum:       result.artifact.Checksum,
			Size:           result.artifact.Size,
			LastUpdateTime: metav1.Now(),
		}
		source.Status.Phase = omniav1alpha1.ArenaSourcePhaseReady

		r.setCondition(source, ArenaSourceConditionTypeFetching, metav1.ConditionFalse,
			"FetchComplete", "Successfully fetched artifact")
		r.setCondition(source, ArenaSourceConditionTypeArtifactAvailable, metav1.ConditionTrue,
			"ArtifactAvailable", fmt.Sprintf("Artifact available at revision %s", result.artifact.Revision))
		r.setCondition(source, ArenaSourceConditionTypeReady, metav1.ConditionTrue,
			"Ready", "ArenaSource is ready")

		nextFetch := metav1.NewTime(time.Now().Add(interval))
		source.Status.NextFetchTime = &nextFetch

		if err := r.Status().Update(ctx, source); err != nil {
			log.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}

		if r.Recorder != nil {
			r.Recorder.Event(source, corev1.EventTypeNormal, EventReasonFetchSucceeded,
				fmt.Sprintf("Successfully fetched artifact at revision %s", result.artifact.Revision))
		}

		log.Info("Successfully reconciled ArenaSource", "revision", result.artifact.Revision)
		return ctrl.Result{RequeueAfter: interval}, nil
	}

	// Check if there's already a fetch in progress
	if _, ok := r.inProgress.Load(req.NamespacedName); ok {
		log.V(1).Info("Fetch already in progress, will check again later")
		// Requeue to check for completion
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Check if we need to fetch (either no artifact or interval elapsed)
	needsFetch := source.Status.Artifact == nil
	if !needsFetch && source.Status.NextFetchTime != nil {
		needsFetch = time.Now().After(source.Status.NextFetchTime.Time)
	}
	if !needsFetch && source.Status.Phase == omniav1alpha1.ArenaSourcePhasePending {
		needsFetch = true
	}

	if !needsFetch {
		// Already up to date
		nextCheck := time.Until(source.Status.NextFetchTime.Time)
		if nextCheck < 0 {
			nextCheck = interval
		}
		return ctrl.Result{RequeueAfter: nextCheck}, nil
	}

	// Set phase to fetching
	source.Status.Phase = omniav1alpha1.ArenaSourcePhaseFetching
	r.setCondition(source, ArenaSourceConditionTypeFetching, metav1.ConditionTrue,
		"FetchInProgress", "Fetching artifact from source")
	now := metav1.Now()
	source.Status.LastFetchTime = &now
	if err := r.Status().Update(ctx, source); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	if r.Recorder != nil {
		r.Recorder.Event(source, corev1.EventTypeNormal, EventReasonFetchStarted, "Started fetching artifact")
	}

	// Start async fetch
	fetchCtx, cancel := context.WithTimeout(context.Background(), timeout)
	job := &fetchJob{
		startTime: time.Now(),
		cancel:    cancel,
	}
	r.inProgress.Store(req.NamespacedName, job)

	// Make a copy of source spec for the goroutine
	sourceSpec := source.Spec.DeepCopy()
	sourceNamespace := source.Namespace
	sourceName := source.Name
	currentRevision := ""
	if source.Status.Artifact != nil {
		currentRevision = source.Status.Artifact.Revision
	}

	go r.doFetchAsync(fetchCtx, req.NamespacedName, sourceSpec, sourceNamespace, sourceName, currentRevision, timeout)

	// Requeue to check for completion
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// doFetchAsync performs the fetch operation asynchronously
func (r *ArenaSourceReconciler) doFetchAsync(ctx context.Context, key types.NamespacedName, spec *omniav1alpha1.ArenaSourceSpec, namespace, name, currentRevision string, timeout time.Duration) {
	log := logf.FromContext(ctx).WithValues("name", name, "namespace", namespace)
	defer func() {
		// Ensure we always store a result and clean up
		if _, ok := r.results.Load(key); !ok {
			// If no result stored, store an error
			r.results.Store(key, &fetchResult{err: fmt.Errorf("fetch terminated unexpectedly")})
		}
	}()

	// Create a mock source for createFetcher
	source := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: *spec,
	}

	opts := fetcher.Options{
		Timeout: timeout,
		WorkDir: r.ArtifactDir,
	}

	f, err := r.createFetcherFromSpec(ctx, source, opts)
	if err != nil {
		log.Error(err, "Failed to create fetcher")
		r.results.Store(key, &fetchResult{err: err})
		return
	}

	// Get latest revision
	revision, err := f.LatestRevision(ctx)
	if err != nil {
		log.Error(err, "Failed to get latest revision")
		r.results.Store(key, &fetchResult{err: err})
		return
	}

	// Check if we already have this revision
	if currentRevision == revision {
		log.V(1).Info("Artifact already up to date", "revision", revision)
		// Return a "no change" result
		r.results.Store(key, &fetchResult{
			artifact: &fetcher.Artifact{Revision: revision},
			err:      nil,
		})
		return
	}

	// Fetch the artifact
	artifact, err := f.Fetch(ctx, revision)
	if err != nil {
		log.Error(err, "Failed to fetch artifact")
		r.results.Store(key, &fetchResult{err: err})
		return
	}

	log.Info("Fetch completed successfully", "revision", revision)
	r.results.Store(key, &fetchResult{artifact: artifact})
}

// createFetcherFromSpec creates the appropriate fetcher based on source spec (for async use).
func (r *ArenaSourceReconciler) createFetcherFromSpec(ctx context.Context, source *omniav1alpha1.ArenaSource, opts fetcher.Options) (fetcher.Fetcher, error) {
	switch source.Spec.Type {
	case omniav1alpha1.ArenaSourceTypeGit:
		return r.createGitFetcher(ctx, source, opts)
	case omniav1alpha1.ArenaSourceTypeOCI:
		return r.createOCIFetcher(ctx, source, opts)
	case omniav1alpha1.ArenaSourceTypeConfigMap:
		return r.createConfigMapFetcher(source, opts)
	default:
		return nil, fmt.Errorf("unsupported source type: %s", source.Spec.Type)
	}
}

// createGitFetcher creates a Git fetcher from the source spec.
func (r *ArenaSourceReconciler) createGitFetcher(ctx context.Context, source *omniav1alpha1.ArenaSource, opts fetcher.Options) (fetcher.Fetcher, error) {
	if source.Spec.Git == nil {
		return nil, fmt.Errorf("git configuration is required for git source type")
	}

	config := fetcher.GitFetcherConfig{
		URL:     source.Spec.Git.URL,
		Path:    source.Spec.Git.Path,
		Options: opts,
	}

	// Set Git reference
	if source.Spec.Git.Ref != nil {
		config.Ref = fetcher.GitRef{
			Branch: source.Spec.Git.Ref.Branch,
			Tag:    source.Spec.Git.Ref.Tag,
			Commit: source.Spec.Git.Ref.Commit,
		}
	}

	// Load credentials if specified
	if source.Spec.Git.SecretRef != nil {
		creds, err := r.loadGitCredentials(ctx, source.Namespace, source.Spec.Git.SecretRef.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to load git credentials: %w", err)
		}
		config.Credentials = creds
	}

	return fetcher.NewGitFetcher(config), nil
}

// createOCIFetcher creates an OCI fetcher from the source spec.
func (r *ArenaSourceReconciler) createOCIFetcher(ctx context.Context, source *omniav1alpha1.ArenaSource, opts fetcher.Options) (fetcher.Fetcher, error) {
	if source.Spec.OCI == nil {
		return nil, fmt.Errorf("oci configuration is required for oci source type")
	}

	config := fetcher.OCIFetcherConfig{
		URL:      source.Spec.OCI.URL,
		Insecure: source.Spec.OCI.Insecure,
		Options:  opts,
	}

	// Load credentials if specified
	if source.Spec.OCI.SecretRef != nil {
		creds, err := r.loadOCICredentials(ctx, source.Namespace, source.Spec.OCI.SecretRef.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to load oci credentials: %w", err)
		}
		config.Credentials = creds
	}

	return fetcher.NewOCIFetcher(config), nil
}

// createConfigMapFetcher creates a ConfigMap fetcher from the source spec.
func (r *ArenaSourceReconciler) createConfigMapFetcher(source *omniav1alpha1.ArenaSource, opts fetcher.Options) (fetcher.Fetcher, error) {
	if source.Spec.ConfigMap == nil {
		return nil, fmt.Errorf("configmap configuration is required for configmap source type")
	}

	config := fetcher.ConfigMapFetcherConfig{
		Name:      source.Spec.ConfigMap.Name,
		Namespace: source.Namespace,
		Options:   opts,
	}

	return fetcher.NewConfigMapFetcher(config, r.Client), nil
}

// loadGitCredentials loads Git credentials from a Secret.
func (r *ArenaSourceReconciler) loadGitCredentials(ctx context.Context, namespace, secretName string) (*fetcher.GitCredentials, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
		return nil, err
	}

	creds := &fetcher.GitCredentials{}

	// HTTPS credentials
	if username, ok := secret.Data["username"]; ok {
		creds.Username = string(username)
	}
	if password, ok := secret.Data["password"]; ok {
		creds.Password = string(password)
	}

	// SSH credentials
	if identity, ok := secret.Data["identity"]; ok {
		creds.PrivateKey = identity
	}
	if knownHosts, ok := secret.Data["known_hosts"]; ok {
		creds.KnownHosts = knownHosts
	}

	return creds, nil
}

// loadOCICredentials loads OCI credentials from a Secret.
func (r *ArenaSourceReconciler) loadOCICredentials(ctx context.Context, namespace, secretName string) (*fetcher.OCICredentials, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
		return nil, err
	}

	creds := &fetcher.OCICredentials{}

	// Basic auth credentials
	if username, ok := secret.Data["username"]; ok {
		creds.Username = string(username)
	}
	if password, ok := secret.Data["password"]; ok {
		creds.Password = string(password)
	}

	// Docker config
	if dockerConfig, ok := secret.Data[".dockerconfigjson"]; ok {
		creds.DockerConfig = dockerConfig
	}

	return creds, nil
}

// storeArtifact stores the fetched artifact and returns its URL.
func (r *ArenaSourceReconciler) storeArtifact(source *omniav1alpha1.ArenaSource, artifact *fetcher.Artifact) (string, error) {
	// If artifact has no path (no-change result), return existing URL
	if artifact.Path == "" && source.Status.Artifact != nil {
		return source.Status.Artifact.URL, nil
	}

	// Create artifact directory if needed
	artifactDir := filepath.Join(r.ArtifactDir, source.Namespace, source.Name)
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create artifact directory: %w", err)
	}

	// Generate artifact filename
	filename := fmt.Sprintf("%s.tar.gz", artifact.Checksum[7:19]) // Use part of checksum as filename
	destPath := filepath.Join(artifactDir, filename)

	// Copy artifact to destination
	if err := copyFile(artifact.Path, destPath); err != nil {
		return "", fmt.Errorf("failed to copy artifact: %w", err)
	}

	// Generate URL
	url := fmt.Sprintf("%s/%s/%s/%s", r.ArtifactBaseURL, source.Namespace, source.Name, filename)
	return url, nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = sourceFile.Close() }()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = destFile.Close() }()

	if _, err := destFile.ReadFrom(sourceFile); err != nil {
		return err
	}

	return destFile.Sync()
}

// handleFetchError handles errors during fetch operations.
func (r *ArenaSourceReconciler) handleFetchError(ctx context.Context, source *omniav1alpha1.ArenaSource, err error) {
	log := logf.FromContext(ctx)

	source.Status.Phase = omniav1alpha1.ArenaSourcePhaseError
	r.setCondition(source, ArenaSourceConditionTypeFetching, metav1.ConditionFalse,
		"FetchFailed", err.Error())
	r.setCondition(source, ArenaSourceConditionTypeReady, metav1.ConditionFalse,
		"FetchError", err.Error())

	if r.Recorder != nil {
		r.Recorder.Event(source, corev1.EventTypeWarning, EventReasonFetchFailed, err.Error())
	}

	if statusErr := r.Status().Update(ctx, source); statusErr != nil {
		log.Error(statusErr, "Failed to update status after fetch error")
	}
}

// setCondition sets a condition on the ArenaSource status.
func (r *ArenaSourceReconciler) setCondition(source *omniav1alpha1.ArenaSource, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&source.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: source.Generation,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ArenaSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.ArenaSource{}).
		Named("arenasource").
		Complete(r)
}
