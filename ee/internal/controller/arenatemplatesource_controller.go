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
	"strings"
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
		r.setCondition(source, ArenaTemplateSourceConditionTypeReady, metav1.ConditionFalse,
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
			r.setCondition(source, ArenaTemplateSourceConditionTypeReady, metav1.ConditionFalse,
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
		contentPath, version, err := r.storeArtifact(source, result.artifact)
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

		// Update template metadata in status
		source.Status.Templates = r.convertTemplatesToCRD(result.templates)
		source.Status.TemplateCount = len(result.templates)
		source.Status.HeadVersion = version
		source.Status.Phase = omniav1alpha1.ArenaTemplateSourcePhaseReady

		r.setCondition(source, ArenaTemplateSourceConditionTypeFetching, metav1.ConditionFalse,
			"FetchComplete", "Successfully fetched content")
		r.setCondition(source, ArenaTemplateSourceConditionTypeTemplatesScanned, metav1.ConditionTrue,
			"ScanComplete", fmt.Sprintf("Discovered %d templates", len(result.templates)))
		r.setCondition(source, ArenaTemplateSourceConditionTypeArtifactAvailable, metav1.ConditionTrue,
			"ArtifactAvailable", fmt.Sprintf("Content synced at revision %s", result.artifact.Revision))
		r.setCondition(source, ArenaTemplateSourceConditionTypeReady, metav1.ConditionTrue,
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
		nextCheck := time.Until(source.Status.NextFetchTime.Time)
		if nextCheck < 0 {
			nextCheck = syncInterval
		}
		return ctrl.Result{RequeueAfter: nextCheck}, nil
	}

	// Set phase to fetching
	source.Status.Phase = omniav1alpha1.ArenaTemplateSourcePhaseFetching
	r.setCondition(source, ArenaTemplateSourceConditionTypeFetching, metav1.ConditionTrue,
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

	// Start async fetch
	fetchCtx, cancel := context.WithTimeout(context.Background(), timeout)
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
		creds, err := r.loadGitCredentials(ctx, namespace, spec.Git.SecretRef.Name)
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
		creds, err := r.loadOCICredentials(ctx, namespace, spec.OCI.SecretRef.Name)
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

// loadGitCredentials loads Git credentials from a Secret.
func (r *ArenaTemplateSourceReconciler) loadGitCredentials(ctx context.Context, namespace, secretName string) (*fetcher.GitCredentials, error) {
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
func (r *ArenaTemplateSourceReconciler) loadOCICredentials(ctx context.Context, namespace, secretName string) (*fetcher.OCICredentials, error) {
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

// storeArtifact stores the fetched artifact by syncing to the workspace content filesystem.
func (r *ArenaTemplateSourceReconciler) storeArtifact(
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

	return r.syncToFilesystem(source, artifact)
}

// syncToFilesystem copies the artifact directory to the workspace content filesystem.
func (r *ArenaTemplateSourceReconciler) syncToFilesystem(
	source *omniav1alpha1.ArenaTemplateSource,
	artifact *fetcher.Artifact,
) (contentPath, version string, err error) {
	ctx := context.Background()
	log := logf.FromContext(ctx).WithValues(
		"source", source.Name,
		"namespace", source.Namespace,
	)

	// Get workspace name from namespace label
	workspaceName := r.getWorkspaceForNamespace(ctx, source.Namespace)

	// Ensure workspace PVC exists (lazy creation)
	if r.StorageManager != nil {
		if _, pvcErr := r.StorageManager.EnsureWorkspacePVC(ctx, workspaceName); pvcErr != nil {
			log.Error(pvcErr, "failed to ensure workspace PVC exists", "workspace", workspaceName)
			return "", "", fmt.Errorf("failed to ensure workspace PVC: %w", pvcErr)
		}
		log.V(1).Info("workspace PVC ensured", "workspace", workspaceName)
	}

	// Target path for templates: arena/template-sources/{source-name}
	targetPath := fmt.Sprintf("arena/template-sources/%s", source.Name)

	// Workspace content structure: {base}/{workspace}/{namespace}/{targetPath}
	workspacePath := filepath.Join(r.WorkspaceContentPath, workspaceName, source.Namespace, targetPath)

	// Use the checksum from the artifact
	contentHash := strings.TrimPrefix(artifact.Checksum, "sha256:")
	if contentHash == "" || contentHash == artifact.Checksum || len(contentHash) < 12 {
		var err error
		contentHash, err = fetcher.CalculateDirectoryHash(artifact.Path)
		if err != nil {
			return "", "", fmt.Errorf("failed to calculate content hash: %w", err)
		}
	}

	// Short version for display (first 12 chars of SHA256)
	version = contentHash[:12]

	// Check if this version already exists
	versionDir := filepath.Join(workspacePath, ".arena", "versions", version)
	if _, err := os.Stat(versionDir); err == nil {
		log.V(1).Info("Version already exists, skipping sync", "version", version)
		contentPath = filepath.Join(targetPath, ".arena", "versions", version)
		if err := r.updateHEAD(workspacePath, version); err != nil {
			return "", "", fmt.Errorf("failed to update HEAD: %w", err)
		}
		return contentPath, version, nil
	}

	// Create version directory
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create version directory: %w", err)
	}

	// Try os.Rename first (atomic, same filesystem), fallback to copy
	if err := os.Rename(artifact.Path, versionDir); err != nil {
		_ = os.RemoveAll(versionDir)
		if err := os.MkdirAll(versionDir, 0755); err != nil {
			return "", "", fmt.Errorf("failed to create version directory: %w", err)
		}
		if err := copyDirectory(artifact.Path, versionDir); err != nil {
			_ = os.RemoveAll(versionDir)
			return "", "", fmt.Errorf("failed to copy content to version directory: %w", err)
		}
	}

	// Update HEAD pointer atomically
	if err := r.updateHEAD(workspacePath, version); err != nil {
		return "", "", fmt.Errorf("failed to update HEAD: %w", err)
	}

	// Garbage collect old versions
	if err := r.gcOldVersions(workspacePath); err != nil {
		log.Error(err, "Failed to garbage collect old versions")
	}

	log.Info("Successfully synced content to filesystem",
		"version", version,
		"path", versionDir,
	)

	contentPath = filepath.Join(targetPath, ".arena", "versions", version)
	return contentPath, version, nil
}

// getWorkspaceForNamespace looks up the workspace name from a namespace's labels.
func (r *ArenaTemplateSourceReconciler) getWorkspaceForNamespace(ctx context.Context, namespace string) string {
	if r.Client == nil {
		return namespace
	}
	ns := &corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		return namespace
	}
	if wsName, ok := ns.Labels[labelWorkspace]; ok && wsName != "" {
		return wsName
	}
	return namespace
}

// updateHEAD atomically updates the HEAD pointer to the given version.
func (r *ArenaTemplateSourceReconciler) updateHEAD(workspacePath, version string) error {
	arenaDir := filepath.Join(workspacePath, ".arena")
	if err := os.MkdirAll(arenaDir, 0755); err != nil {
		return err
	}

	headPath := filepath.Join(arenaDir, "HEAD")
	tempPath := headPath + ".tmp"

	if err := os.WriteFile(tempPath, []byte(version), 0644); err != nil {
		return err
	}

	return os.Rename(tempPath, headPath)
}

// gcOldVersions removes old versions exceeding MaxVersionsPerSource.
func (r *ArenaTemplateSourceReconciler) gcOldVersions(workspacePath string) error {
	versionsDir := filepath.Join(workspacePath, ".arena", "versions")

	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	maxVersions := r.MaxVersionsPerSource
	if maxVersions <= 0 {
		maxVersions = 10
	}

	if len(entries) <= maxVersions {
		return nil
	}

	// Get version directories with their mod times
	type versionInfo struct {
		name    string
		modTime time.Time
	}
	versions := make([]versionInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		versions = append(versions, versionInfo{
			name:    entry.Name(),
			modTime: info.ModTime(),
		})
	}

	// Sort by mod time (oldest first)
	for i := 0; i < len(versions)-1; i++ {
		for j := i + 1; j < len(versions); j++ {
			if versions[i].modTime.After(versions[j].modTime) {
				versions[i], versions[j] = versions[j], versions[i]
			}
		}
	}

	// Remove oldest versions
	for i := 0; i < len(versions)-maxVersions; i++ {
		versionPath := filepath.Join(versionsDir, versions[i].name)
		if err := os.RemoveAll(versionPath); err != nil {
			return fmt.Errorf("failed to remove old version %s: %w", versions[i].name, err)
		}
	}

	return nil
}

// convertTemplatesToCRD converts internal template types to CRD types.
func (r *ArenaTemplateSourceReconciler) convertTemplatesToCRD(templates []arenaTemplate.Template) []omniav1alpha1.TemplateMetadata {
	result := make([]omniav1alpha1.TemplateMetadata, len(templates))
	for i, t := range templates {
		result[i] = omniav1alpha1.TemplateMetadata{
			Name:        t.Name,
			Version:     t.Version,
			DisplayName: t.DisplayName,
			Description: t.Description,
			Category:    t.Category,
			Tags:        t.Tags,
			Variables:   r.convertVariablesToCRD(t.Variables),
			Files:       r.convertFilesToCRD(t.Files),
			Path:        t.Path,
		}
	}
	return result
}

// convertVariablesToCRD converts internal variable types to CRD types.
func (r *ArenaTemplateSourceReconciler) convertVariablesToCRD(variables []arenaTemplate.Variable) []omniav1alpha1.TemplateVariable {
	result := make([]omniav1alpha1.TemplateVariable, len(variables))
	for i, v := range variables {
		result[i] = omniav1alpha1.TemplateVariable{
			Name:        v.Name,
			Type:        omniav1alpha1.TemplateVariableType(v.Type),
			Description: v.Description,
			Required:    v.Required,
			Default:     v.Default,
			Pattern:     v.Pattern,
			Options:     v.Options,
			Min:         v.Min,
			Max:         v.Max,
		}
	}
	return result
}

// convertFilesToCRD converts internal file spec types to CRD types.
func (r *ArenaTemplateSourceReconciler) convertFilesToCRD(files []arenaTemplate.FileSpec) []omniav1alpha1.TemplateFileSpec {
	result := make([]omniav1alpha1.TemplateFileSpec, len(files))
	for i, f := range files {
		result[i] = omniav1alpha1.TemplateFileSpec{
			Path:   f.Path,
			Render: f.Render,
		}
	}
	return result
}

// handleFetchError handles errors during fetch operations.
func (r *ArenaTemplateSourceReconciler) handleFetchError(ctx context.Context, source *omniav1alpha1.ArenaTemplateSource, err error) {
	log := logf.FromContext(ctx)

	source.Status.Phase = omniav1alpha1.ArenaTemplateSourcePhaseError
	source.Status.Message = err.Error()
	r.setCondition(source, ArenaTemplateSourceConditionTypeFetching, metav1.ConditionFalse,
		"FetchFailed", err.Error())
	r.setCondition(source, ArenaTemplateSourceConditionTypeReady, metav1.ConditionFalse,
		"FetchError", err.Error())

	if r.Recorder != nil {
		r.Recorder.Event(source, corev1.EventTypeWarning, EventReasonTemplateFetchFailed, err.Error())
	}

	if statusErr := r.Status().Update(ctx, source); statusErr != nil {
		log.Error(statusErr, "Failed to update status after fetch error")
	}
}

// setCondition sets a condition on the ArenaTemplateSource status.
func (r *ArenaTemplateSourceReconciler) setCondition(
	source *omniav1alpha1.ArenaTemplateSource,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) {
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
func (r *ArenaTemplateSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.ArenaTemplateSource{}).
		Named("arenatemplatesource").
		Complete(r)
}
