/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package controller

import (
	"context"
	"encoding/json"
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
	arenaTemplate "github.com/altairalabs/omnia/ee/pkg/arena/template"
	"github.com/altairalabs/omnia/ee/pkg/license"
	"github.com/altairalabs/omnia/ee/pkg/workspace"
)

// ArenaTemplateSource condition types
const (
	ArenaTemplateSourceConditionTypeReady             = "Ready"
	ArenaTemplateSourceConditionTypeFetching          = "Fetching"
	ArenaTemplateSourceConditionTypeTemplatesScanned  = "TemplatesScanned"
	ArenaTemplateSourceConditionTypeArtifactAvailable = "ArtifactAvailable"
)

// Event reasons for ArenaTemplateSource
const (
	EventReasonTemplateFetchStarted   = "FetchStarted"
	EventReasonTemplateFetchSucceeded = "FetchSucceeded"
	EventReasonTemplateFetchFailed    = "FetchFailed"
	EventReasonTemplateScanSucceeded  = "TemplateScanSucceeded"
	EventReasonTemplateScanFailed     = "TemplateScanFailed"
)

// templateFetchJob represents an in-progress fetch operation for templates
type templateFetchJob struct {
	startTime time.Time
	cancel    context.CancelFunc
}

// templateFetchResult represents the result of a completed fetch operation
type templateFetchResult struct {
	artifact  *fetcher.Artifact
	templates []arenaTemplate.Template
	err       error
}

// ArenaTemplateSourceReconciler reconciles an ArenaTemplateSource object
type ArenaTemplateSourceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// WorkspaceContentPath is the base path for workspace content volumes.
	WorkspaceContentPath string

	// MaxVersionsPerSource is the maximum number of versions to retain per source.
	MaxVersionsPerSource int

	// LicenseValidator validates license for source types (defense in depth)
	LicenseValidator *license.Validator

	// StorageManager handles lazy workspace PVC creation.
	StorageManager *workspace.StorageManager

	// inProgress tracks in-progress fetch operations
	inProgress sync.Map // map[types.NamespacedName]*templateFetchJob

	// results stores completed fetch results
	results sync.Map // map[types.NamespacedName]*templateFetchResult
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenatemplatesources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenatemplatesources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenatemplatesources/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
//nolint:gocognit,gocyclo // Reconcile functions inherently have high complexity due to state machine logic
func (r *ArenaTemplateSourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling ArenaTemplateSource", "name", req.Name, "namespace", req.Namespace)

	// Fetch the ArenaTemplateSource instance
	source := &omniav1alpha1.ArenaTemplateSource{}
	if err := r.Get(ctx, req.NamespacedName, source); err != nil {
		if apierrors.IsNotFound(err) {
			// Clean up any in-progress fetch
			if job, ok := r.inProgress.LoadAndDelete(req.NamespacedName); ok {
				job.(*templateFetchJob).cancel()
			}
			r.results.Delete(req.NamespacedName)
			log.Info("ArenaTemplateSource resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get ArenaTemplateSource")
		return ctrl.Result{}, err
	}

	// Initialize status if needed
	if source.Status.Phase == "" {
		source.Status.Phase = omniav1alpha1.ArenaTemplateSourcePhasePending
	}

	// Update observed generation
	source.Status.ObservedGeneration = source.Generation

	// Check if suspended
	if source.Spec.Suspend {
		log.Info("ArenaTemplateSource is suspended, skipping reconciliation")
		// Cancel any in-progress fetch
		if job, ok := r.inProgress.LoadAndDelete(req.NamespacedName); ok {
			job.(*templateFetchJob).cancel()
		}
		SetCondition(&source.Status.Conditions, source.Generation, ArenaTemplateSourceConditionTypeReady, metav1.ConditionFalse,
			"Suspended", "ArenaTemplateSource reconciliation is suspended")
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
			source.Status.Phase = omniav1alpha1.ArenaTemplateSourcePhaseError
			SetCondition(&source.Status.Conditions, source.Generation, ArenaTemplateSourceConditionTypeReady, metav1.ConditionFalse,
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

	// Parse sync interval
	syncInterval := time.Hour // Default 1h
	if source.Spec.SyncInterval != "" {
		var err error
		syncInterval, err = time.ParseDuration(source.Spec.SyncInterval)
		if err != nil {
			log.Error(err, "Invalid syncInterval format, using default 1h")
			syncInterval = time.Hour
		}
	}

	// Parse timeout duration
	timeout := 60 * time.Second
	if source.Spec.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(source.Spec.Timeout)
		if err != nil {
			log.Error(err, "Invalid timeout format, using default 60s")
			timeout = 60 * time.Second
		}
	}

	// Check if there's a completed result waiting
	if resultVal, ok := r.results.LoadAndDelete(req.NamespacedName); ok {
		result := resultVal.(*templateFetchResult)
		r.inProgress.Delete(req.NamespacedName)

		if result.err != nil {
			log.Error(result.err, "Fetch completed with error")
			r.handleFetchError(ctx, source, result.err)
			return ctrl.Result{RequeueAfter: syncInterval}, nil
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
			return ctrl.Result{RequeueAfter: syncInterval}, nil
		}

		// Clean up artifact directory
		if result.artifact != nil && result.artifact.Path != "" {
			_ = os.RemoveAll(result.artifact.Path)
		}

		// Update status with artifact info
		source.Status.Artifact = &omniav1alpha1.Artifact{
			Revision:       result.artifact.Revision,
			ContentPath:    contentPath,
			Version:        version,
			Checksum:       result.artifact.Checksum,
			Size:           result.artifact.Size,
			LastUpdateTime: metav1.Now(),
		}

		// Write template index file for fast API lookups
		if err := r.writeTemplateIndex(source, contentPath, result.templates); err != nil {
			log.Error(err, "Failed to write template index")
			r.handleFetchError(ctx, source, fmt.Errorf("failed to write template index: %w", err))
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		source.Status.TemplateCount = len(result.templates)
		source.Status.HeadVersion = version
		source.Status.Phase = omniav1alpha1.ArenaTemplateSourcePhaseReady

		SetCondition(&source.Status.Conditions, source.Generation, ArenaTemplateSourceConditionTypeFetching, metav1.ConditionFalse,
			"FetchComplete", "Successfully fetched content")
		SetCondition(&source.Status.Conditions, source.Generation, ArenaTemplateSourceConditionTypeTemplatesScanned, metav1.ConditionTrue,
			"ScanComplete", fmt.Sprintf("Discovered %d templates", len(result.templates)))
		SetCondition(&source.Status.Conditions, source.Generation, ArenaTemplateSourceConditionTypeArtifactAvailable, metav1.ConditionTrue,
			"ArtifactAvailable", fmt.Sprintf("Content synced at revision %s", result.artifact.Revision))
		SetCondition(&source.Status.Conditions, source.Generation, ArenaTemplateSourceConditionTypeReady, metav1.ConditionTrue,
			"Ready", "ArenaTemplateSource is ready")

		nextFetch := metav1.NewTime(time.Now().Add(syncInterval))
		source.Status.NextFetchTime = &nextFetch

		if err := r.Status().Update(ctx, source); err != nil {
			log.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}

		if r.Recorder != nil {
			r.Recorder.Event(source, corev1.EventTypeNormal, EventReasonTemplateFetchSucceeded,
				fmt.Sprintf("Successfully fetched %d templates at revision %s", len(result.templates), result.artifact.Revision))
		}

		log.Info("Successfully reconciled ArenaTemplateSource",
			"revision", result.artifact.Revision,
			"templateCount", len(result.templates))
		return ctrl.Result{RequeueAfter: syncInterval}, nil
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
	if !needsFetch && source.Status.Phase == omniav1alpha1.ArenaTemplateSourcePhasePending {
		needsFetch = true
	}

	if !needsFetch {
		// Already up to date
		nextCheck := syncInterval
		if source.Status.NextFetchTime != nil {
			nextCheck = time.Until(source.Status.NextFetchTime.Time)
			if nextCheck < 0 {
				nextCheck = syncInterval
			}
		}
		return ctrl.Result{RequeueAfter: nextCheck}, nil
	}

	// Set phase to fetching
	source.Status.Phase = omniav1alpha1.ArenaTemplateSourcePhaseFetching
	SetCondition(&source.Status.Conditions, source.Generation, ArenaTemplateSourceConditionTypeFetching, metav1.ConditionTrue,
		"FetchInProgress", "Fetching templates from source")
	now := metav1.Now()
	source.Status.LastFetchTime = &now
	if err := r.Status().Update(ctx, source); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	if r.Recorder != nil {
		r.Recorder.Event(source, corev1.EventTypeNormal, EventReasonTemplateFetchStarted, "Started fetching templates")
	}

	// Start async fetch - use Background context intentionally since fetch outlives the request.
	// The request context (ctx) is unsuitable because this goroutine must continue running
	// after the reconciliation loop returns to process the async template fetch.
	fetchCtx, cancel := context.WithTimeout(context.Background(), timeout) // NOSONAR(S8239)
	job := &templateFetchJob{
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
func (r *ArenaTemplateSourceReconciler) doFetchAsync(
	ctx context.Context,
	key types.NamespacedName,
	spec *omniav1alpha1.ArenaTemplateSourceSpec,
	namespace, name, currentRevision string,
	timeout time.Duration,
) {
	log := logf.FromContext(ctx).WithValues("name", name, "namespace", namespace)
	defer func() {
		// Ensure we always store a result and clean up
		if _, ok := r.results.Load(key); !ok {
			// If no result stored, store an error
			r.results.Store(key, &templateFetchResult{err: fmt.Errorf("fetch terminated unexpectedly")})
		}
	}()

	// Create fetcher options
	opts := fetcher.Options{
		Timeout: timeout,
		WorkDir: os.TempDir(),
	}

	f, err := r.createFetcher(ctx, spec, namespace, opts)
	if err != nil {
		log.Error(err, "Failed to create fetcher")
		r.results.Store(key, &templateFetchResult{err: err})
		return
	}

	// Get latest revision
	revision, err := f.LatestRevision(ctx)
	if err != nil {
		log.Error(err, "Failed to get latest revision")
		r.results.Store(key, &templateFetchResult{err: err})
		return
	}

	// Check if we already have this revision
	if currentRevision == revision {
		log.V(1).Info("Content already up to date", "revision", revision)
		// Return a "no change" result - still scan templates in case definition changed
		r.results.Store(key, &templateFetchResult{
			artifact: &fetcher.Artifact{Revision: revision},
			err:      nil,
		})
		return
	}

	// Fetch the content
	artifact, err := f.Fetch(ctx, revision)
	if err != nil {
		log.Error(err, "Failed to fetch content")
		r.results.Store(key, &templateFetchResult{err: err})
		return
	}

	// Discover templates
	templatesPath := spec.TemplatesPath
	if templatesPath == "" {
		templatesPath = arenaTemplate.DefaultTemplatesPath
	}

	discoverer := arenaTemplate.NewDiscoverer(artifact.Path, templatesPath)
	templates, err := discoverer.Discover()
	if err != nil {
		log.Error(err, "Failed to discover templates")
		// Still store result with artifact but report scan error
		r.results.Store(key, &templateFetchResult{
			artifact:  artifact,
			templates: nil,
			err:       fmt.Errorf("template discovery failed: %w", err),
		})
		return
	}

	log.Info("Fetch completed successfully",
		"revision", revision,
		"templateCount", len(templates))
	r.results.Store(key, &templateFetchResult{
		artifact:  artifact,
		templates: templates,
	})
}

// createFetcher creates the appropriate fetcher based on source spec.
func (r *ArenaTemplateSourceReconciler) createFetcher(
	ctx context.Context,
	spec *omniav1alpha1.ArenaTemplateSourceSpec,
	namespace string,
	opts fetcher.Options,
) (fetcher.Fetcher, error) {
	switch spec.Type {
	case omniav1alpha1.ArenaTemplateSourceTypeGit:
		return r.createGitFetcher(ctx, spec, namespace, opts)
	case omniav1alpha1.ArenaTemplateSourceTypeOCI:
		return r.createOCIFetcher(ctx, spec, namespace, opts)
	case omniav1alpha1.ArenaTemplateSourceTypeConfigMap:
		return r.createConfigMapFetcher(spec, namespace, opts)
	default:
		return nil, fmt.Errorf("unsupported source type: %s", spec.Type)
	}
}

// createGitFetcher creates a Git fetcher from the source spec.
func (r *ArenaTemplateSourceReconciler) createGitFetcher(
	ctx context.Context,
	spec *omniav1alpha1.ArenaTemplateSourceSpec,
	namespace string,
	opts fetcher.Options,
) (fetcher.Fetcher, error) {
	if spec.Git == nil {
		return nil, fmt.Errorf("git configuration is required for git source type")
	}

	config := fetcher.GitFetcherConfig{
		URL:     spec.Git.URL,
		Path:    spec.Git.Path,
		Options: opts,
	}

	// Set Git reference
	if spec.Git.Ref != nil {
		config.Ref = fetcher.GitRef{
			Branch: spec.Git.Ref.Branch,
			Tag:    spec.Git.Ref.Tag,
			Commit: spec.Git.Ref.Commit,
		}
	}

	// Load credentials if specified
	if spec.Git.SecretRef != nil {
		creds, err := LoadGitCredentials(ctx, r.Client, namespace, spec.Git.SecretRef.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to load git credentials: %w", err)
		}
		config.Credentials = creds
	}

	return fetcher.NewGitFetcher(config), nil
}

// createOCIFetcher creates an OCI fetcher from the source spec.
func (r *ArenaTemplateSourceReconciler) createOCIFetcher(
	ctx context.Context,
	spec *omniav1alpha1.ArenaTemplateSourceSpec,
	namespace string,
	opts fetcher.Options,
) (fetcher.Fetcher, error) {
	if spec.OCI == nil {
		return nil, fmt.Errorf("oci configuration is required for oci source type")
	}

	config := fetcher.OCIFetcherConfig{
		URL:      spec.OCI.URL,
		Insecure: spec.OCI.Insecure,
		Options:  opts,
	}

	// Load credentials if specified
	if spec.OCI.SecretRef != nil {
		creds, err := LoadOCICredentials(ctx, r.Client, namespace, spec.OCI.SecretRef.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to load oci credentials: %w", err)
		}
		config.Credentials = creds
	}

	return fetcher.NewOCIFetcher(config), nil
}

// createConfigMapFetcher creates a ConfigMap fetcher from the source spec.
func (r *ArenaTemplateSourceReconciler) createConfigMapFetcher(
	spec *omniav1alpha1.ArenaTemplateSourceSpec,
	namespace string,
	opts fetcher.Options,
) (fetcher.Fetcher, error) {
	if spec.ConfigMap == nil {
		return nil, fmt.Errorf("configmap configuration is required for configmap source type")
	}

	config := fetcher.ConfigMapFetcherConfig{
		Name:      spec.ConfigMap.Name,
		Namespace: namespace,
		Options:   opts,
	}

	return fetcher.NewConfigMapFetcher(config, r.Client), nil
}

// storeArtifact stores the fetched artifact by syncing to the workspace content filesystem.
func (r *ArenaTemplateSourceReconciler) storeArtifact(
	ctx context.Context,
	source *omniav1alpha1.ArenaTemplateSource,
	artifact *fetcher.Artifact,
) (contentPath, version string, err error) {
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
	targetPath := fmt.Sprintf("arena/template-sources/%s", source.Name)

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

// TemplateIndexDir is the directory for template index files.
const TemplateIndexDir = "arena/template-indexes"

// writeTemplateIndex writes the template index to a JSON file for fast API lookups.
// Path: {WorkspaceContentPath}/{workspace}/{namespace}/arena/template-indexes/{source}.json
func (r *ArenaTemplateSourceReconciler) writeTemplateIndex(source *omniav1alpha1.ArenaTemplateSource, _ string, templates []arenaTemplate.Template) error {
	// Get workspace name for the namespace
	workspaceName := GetWorkspaceForNamespace(context.Background(), r.Client, source.Namespace)

	// Build index directory path
	indexDir := filepath.Join(r.WorkspaceContentPath, workspaceName, source.Namespace, TemplateIndexDir)
	if err := os.MkdirAll(indexDir, 0755); err != nil {
		return fmt.Errorf("failed to create template index directory: %w", err)
	}

	// Write to {source}.json
	indexPath := filepath.Join(indexDir, source.Name+".json")

	data, err := json.MarshalIndent(templates, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal template index: %w", err)
	}

	// Write atomically using temp file + rename
	tempPath := indexPath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write template index file: %w", err)
	}
	if err := os.Rename(tempPath, indexPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to rename template index file: %w", err)
	}

	return nil
}

// handleFetchError handles errors during fetch operations.
func (r *ArenaTemplateSourceReconciler) handleFetchError(ctx context.Context, source *omniav1alpha1.ArenaTemplateSource, err error) {
	log := logf.FromContext(ctx)

	source.Status.Phase = omniav1alpha1.ArenaTemplateSourcePhaseError
	source.Status.Message = err.Error()
	SetCondition(&source.Status.Conditions, source.Generation, ArenaTemplateSourceConditionTypeFetching, metav1.ConditionFalse,
		"FetchFailed", err.Error())
	SetCondition(&source.Status.Conditions, source.Generation, ArenaTemplateSourceConditionTypeReady, metav1.ConditionFalse,
		"FetchError", err.Error())

	if r.Recorder != nil {
		r.Recorder.Event(source, corev1.EventTypeWarning, EventReasonTemplateFetchFailed, err.Error())
	}

	if statusErr := r.Status().Update(ctx, source); statusErr != nil {
		log.Error(statusErr, "Failed to update status after fetch error")
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ArenaTemplateSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.ArenaTemplateSource{}).
		Named("arenatemplatesource").
		Complete(r)
}
