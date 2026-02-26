/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/fetcher"
	"github.com/altairalabs/omnia/ee/pkg/license"
	"github.com/altairalabs/omnia/ee/pkg/workspace"
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

	// WorkspaceContentPath is the base path for workspace content volumes.
	// Structure: {WorkspaceContentPath}/{workspace}/{namespace}/arena/{source-name}/
	WorkspaceContentPath string

	// MaxVersionsPerSource is the maximum number of versions to retain per source.
	// Older versions are garbage collected when this limit is exceeded.
	// Default is 10 if not set.
	MaxVersionsPerSource int

	// LicenseValidator validates license for source types (defense in depth)
	LicenseValidator *license.Validator

	// StorageManager handles lazy workspace PVC creation.
	// When set, the reconciler will ensure workspace PVC exists before storing artifacts.
	StorageManager *workspace.StorageManager

	// inProgress tracks in-progress fetch operations
	inProgress sync.Map // map[types.NamespacedName]*fetchJob

	// results stores completed fetch results
	results sync.Map // map[types.NamespacedName]*fetchResult
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenasources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenasources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenasources/finalizers,verbs=update
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=workspaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
//nolint:gocognit,gocyclo // Reconcile functions inherently have high complexity due to state machine logic
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
		SetCondition(&source.Status.Conditions, source.Generation, ArenaSourceConditionTypeReady, metav1.ConditionFalse,
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
			SetCondition(&source.Status.Conditions, source.Generation, ArenaSourceConditionTypeReady, metav1.ConditionFalse,
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

		if r.LicenseValidator.IsDevMode() && r.Recorder != nil {
			r.Recorder.Event(source, corev1.EventTypeWarning, "DevModeLicense",
				"Using development license - not licensed for production use")
		}
	}

	// Parse interval duration
	interval, err := time.ParseDuration(source.Spec.Interval)
	if err != nil {
		log.Error(err, "Invalid interval format")
		SetCondition(&source.Status.Conditions, source.Generation, ArenaSourceConditionTypeReady, metav1.ConditionFalse,
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

		// Store the artifact (sync to filesystem)
		contentPath, version, err := r.storeArtifact(ctx, source, result.artifact)
		if err != nil {
			log.Error(err, "Failed to store artifact")
			r.handleFetchError(ctx, source, err)
			// Clean up artifact directory
			if result.artifact != nil && result.artifact.Path != "" {
				_ = os.RemoveAll(result.artifact.Path)
			}
			return ctrl.Result{RequeueAfter: interval}, nil
		}

		// Clean up artifact directory
		if result.artifact != nil && result.artifact.Path != "" {
			_ = os.RemoveAll(result.artifact.Path)
		}

		// Update status with artifact info
		source.Status.Artifact = &omniav1alpha1.Artifact{
			Revision:       result.artifact.Revision,
			URL:            "",          // Legacy: tar.gz URL (empty for filesystem mode)
			ContentPath:    contentPath, // New: filesystem path
			Version:        version,     // New: content-addressable hash
			Checksum:       result.artifact.Checksum,
			Size:           result.artifact.Size,
			LastUpdateTime: metav1.Now(),
		}
		source.Status.Phase = omniav1alpha1.ArenaSourcePhaseReady

		// Update version tracking status
		if version != "" {
			source.Status.LastSyncRevision = result.artifact.Revision
			source.Status.LastVersionCreated = version
			source.Status.HeadVersion = version
			// Count versions
			if r.WorkspaceContentPath != "" {
				workspaceName := GetWorkspaceForNamespace(ctx, r.Client, source.Namespace)
				targetPath := source.Spec.TargetPath
				if targetPath == "" {
					targetPath = fmt.Sprintf("arena/%s", source.Name)
				}
				versionsDir := filepath.Join(r.WorkspaceContentPath, workspaceName, source.Namespace, targetPath, ".arena", "versions")
				if entries, err := os.ReadDir(versionsDir); err == nil {
					source.Status.VersionCount = len(entries)
				}
			}
		}

		SetCondition(&source.Status.Conditions, source.Generation, ArenaSourceConditionTypeFetching, metav1.ConditionFalse,
			"FetchComplete", "Successfully fetched artifact")
		availableMsg := fmt.Sprintf("Artifact available at revision %s", result.artifact.Revision)
		if version != "" {
			availableMsg = fmt.Sprintf("Content synced at revision %s, version %s", result.artifact.Revision, version)
		}
		SetCondition(&source.Status.Conditions, source.Generation, ArenaSourceConditionTypeArtifactAvailable, metav1.ConditionTrue,
			"ArtifactAvailable", availableMsg)
		SetCondition(&source.Status.Conditions, source.Generation, ArenaSourceConditionTypeReady, metav1.ConditionTrue,
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
	SetCondition(&source.Status.Conditions, source.Generation, ArenaSourceConditionTypeFetching, metav1.ConditionTrue,
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

	// Use a temp directory within WorkspaceContentPath for two reasons:
	// 1. The workspace volume is writable, while /tmp may be read-only in restricted security contexts
	// 2. os.Rename is atomic when source and destination are on the same filesystem
	workDir := os.TempDir()
	if r.WorkspaceContentPath != "" {
		workDir = filepath.Join(r.WorkspaceContentPath, ".tmp")
		if err := os.MkdirAll(workDir, 0755); err != nil {
			log.Error(err, "Failed to create workspace temp directory, falling back to system temp")
			workDir = os.TempDir()
		}
	}

	opts := fetcher.Options{
		Timeout: timeout,
		WorkDir: workDir,
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
		creds, err := LoadGitCredentials(ctx, r.Client, source.Namespace, source.Spec.Git.SecretRef.Name)
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
		creds, err := LoadOCICredentials(ctx, r.Client, source.Namespace, source.Spec.OCI.SecretRef.Name)
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

// storeArtifact stores the fetched artifact by syncing to the workspace content filesystem.
// Returns contentPath, version, url (url is always empty for filesystem mode).
func (r *ArenaSourceReconciler) storeArtifact(ctx context.Context, source *omniav1alpha1.ArenaSource, artifact *fetcher.Artifact) (contentPath, version string, err error) {
	// If artifact has no path (no-change result), return existing values
	if artifact.Path == "" && source.Status.Artifact != nil {
		return source.Status.Artifact.ContentPath,
			source.Status.Artifact.Version,
			nil
	}

	// WorkspaceContentPath is required
	if r.WorkspaceContentPath == "" {
		return "", "", fmt.Errorf("WorkspaceContentPath is required for storing artifacts")
	}

	workspaceName := GetWorkspaceForNamespace(ctx, r.Client, source.Namespace)
	targetPath := source.Spec.TargetPath
	if targetPath == "" {
		targetPath = fmt.Sprintf("arena/%s", source.Name)
	}

	syncer := &FilesystemSyncer{
		WorkspaceContentPath: r.WorkspaceContentPath,
		MaxVersionsPerSource: r.MaxVersionsPerSource,
		StorageManager:       r.StorageManager,
	}

	return syncer.SyncToFilesystem(ctx, SyncParams{
		WorkspaceName: workspaceName,
		Namespace:     source.Namespace,
		TargetPath:    targetPath,
		Artifact:      artifact,
	})
}

// handleFetchError handles errors during fetch operations.
func (r *ArenaSourceReconciler) handleFetchError(ctx context.Context, source *omniav1alpha1.ArenaSource, err error) {
	log := logf.FromContext(ctx)

	source.Status.Phase = omniav1alpha1.ArenaSourcePhaseError
	SetCondition(&source.Status.Conditions, source.Generation, ArenaSourceConditionTypeFetching, metav1.ConditionFalse,
		"FetchFailed", err.Error())
	SetCondition(&source.Status.Conditions, source.Generation, ArenaSourceConditionTypeReady, metav1.ConditionFalse,
		"FetchError", err.Error())

	if r.Recorder != nil {
		r.Recorder.Event(source, corev1.EventTypeWarning, EventReasonFetchFailed, err.Error())
	}

	if statusErr := r.Status().Update(ctx, source); statusErr != nil {
		log.Error(statusErr, "Failed to update status after fetch error")
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ArenaSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.ArenaSource{}).
		Named("arenasource").
		Complete(r)
}
