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
	"io"
	"os"
	"path/filepath"
	"sort"
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

		// Store the artifact (sync to filesystem)
		contentPath, version, artifactURL, err := r.storeArtifact(source, result.artifact)
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
			URL:            artifactURL, // Legacy: tar.gz URL (empty for filesystem mode)
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
				workspaceName := r.getWorkspaceForNamespace(ctx, source.Namespace)
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

		r.setCondition(source, ArenaSourceConditionTypeFetching, metav1.ConditionFalse,
			"FetchComplete", "Successfully fetched artifact")
		availableMsg := fmt.Sprintf("Artifact available at revision %s", result.artifact.Revision)
		if version != "" {
			availableMsg = fmt.Sprintf("Content synced at revision %s, version %s", result.artifact.Revision, version)
		}
		r.setCondition(source, ArenaSourceConditionTypeArtifactAvailable, metav1.ConditionTrue,
			"ArtifactAvailable", availableMsg)
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

// storeArtifact stores the fetched artifact by syncing to the workspace content filesystem.
// Returns contentPath, version, url (url is always empty for filesystem mode).
func (r *ArenaSourceReconciler) storeArtifact(source *omniav1alpha1.ArenaSource, artifact *fetcher.Artifact) (contentPath, version, url string, err error) {
	// If artifact has no path (no-change result), return existing values
	if artifact.Path == "" && source.Status.Artifact != nil {
		return source.Status.Artifact.ContentPath,
			source.Status.Artifact.Version,
			"",
			nil
	}

	// WorkspaceContentPath is required
	if r.WorkspaceContentPath == "" {
		return "", "", "", fmt.Errorf("WorkspaceContentPath is required for storing artifacts")
	}

	return r.syncToFilesystem(source, artifact)
}

// syncToFilesystem copies the artifact directory to the workspace content filesystem
// and creates a content-addressable version.
func (r *ArenaSourceReconciler) syncToFilesystem(source *omniav1alpha1.ArenaSource, artifact *fetcher.Artifact) (contentPath, version, url string, err error) {
	ctx := context.Background()
	log := logf.FromContext(ctx).WithValues(
		"source", source.Name,
		"namespace", source.Namespace,
	)

	// Get workspace name from namespace label (allows future multi-namespace workspaces)
	workspaceName := r.getWorkspaceForNamespace(ctx, source.Namespace)

	// Ensure workspace PVC exists (lazy creation)
	if r.StorageManager != nil {
		if _, pvcErr := r.StorageManager.EnsureWorkspacePVC(ctx, workspaceName); pvcErr != nil {
			log.Error(pvcErr, "failed to ensure workspace PVC exists", "workspace", workspaceName)
			return "", "", "", fmt.Errorf("failed to ensure workspace PVC: %w", pvcErr)
		}
		log.V(1).Info("workspace PVC ensured", "workspace", workspaceName)
	}

	// Determine target path within workspace content
	targetPath := source.Spec.TargetPath
	if targetPath == "" {
		targetPath = fmt.Sprintf("arena/%s", source.Name)
	}

	// Workspace content structure: {base}/{workspace}/{namespace}/{targetPath}
	// This structure supports future multi-namespace workspaces
	workspacePath := filepath.Join(r.WorkspaceContentPath, workspaceName, source.Namespace, targetPath)

	// Use the checksum from the artifact (already calculated by fetcher)
	// Extract the hash part from "sha256:<hash>"
	contentHash := strings.TrimPrefix(artifact.Checksum, "sha256:")
	if contentHash == "" || contentHash == artifact.Checksum || len(contentHash) < 12 {
		// Fallback: calculate hash if checksum format is unexpected or too short
		var err error
		contentHash, err = fetcher.CalculateDirectoryHash(artifact.Path)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to calculate content hash: %w", err)
		}
	}

	// Short version for display (first 12 chars of SHA256)
	version = contentHash[:12]

	// Check if this version already exists
	versionDir := filepath.Join(workspacePath, ".arena", "versions", version)
	if _, err := os.Stat(versionDir); err == nil {
		log.V(1).Info("Version already exists, skipping sync", "version", version)
		// Version already exists, just update HEAD
		contentPath = filepath.Join(targetPath, ".arena", "versions", version)
		if err := r.updateHEAD(workspacePath, version); err != nil {
			return "", "", "", fmt.Errorf("failed to update HEAD: %w", err)
		}
		return contentPath, version, "", nil
	}

	// Create version directory
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return "", "", "", fmt.Errorf("failed to create version directory: %w", err)
	}

	// Try os.Rename first (atomic, same filesystem), fallback to copy
	if err := os.Rename(artifact.Path, versionDir); err != nil {
		// Rename failed (likely cross-filesystem), copy instead
		// First remove the empty versionDir we just created
		_ = os.RemoveAll(versionDir)
		if err := os.MkdirAll(versionDir, 0755); err != nil {
			return "", "", "", fmt.Errorf("failed to create version directory: %w", err)
		}
		if err := copyDirectory(artifact.Path, versionDir); err != nil {
			// Clean up on failure
			_ = os.RemoveAll(versionDir)
			return "", "", "", fmt.Errorf("failed to copy content to version directory: %w", err)
		}
	}

	// Update HEAD pointer atomically
	if err := r.updateHEAD(workspacePath, version); err != nil {
		return "", "", "", fmt.Errorf("failed to update HEAD: %w", err)
	}

	// Garbage collect old versions
	if err := r.gcOldVersions(workspacePath); err != nil {
		// Log but don't fail on GC errors
		log.Error(err, "Failed to garbage collect old versions")
	}

	log.Info("Successfully synced content to filesystem",
		"version", version,
		"path", versionDir,
	)

	contentPath = filepath.Join(targetPath, ".arena", "versions", version)
	return contentPath, version, "", nil
}

// getWorkspaceForNamespace looks up the workspace name from a namespace's labels.
// Returns the namespace name as fallback if workspace label is not found.
func (r *ArenaSourceReconciler) getWorkspaceForNamespace(ctx context.Context, namespace string) string {
	// Handle nil client (e.g., in tests that don't set up the client)
	if r.Client == nil {
		return namespace
	}
	ns := &corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		// Fallback to namespace name if we can't look it up
		return namespace
	}
	if wsName, ok := ns.Labels[labelWorkspace]; ok && wsName != "" {
		return wsName
	}
	// Fallback to namespace name
	return namespace
}

// updateHEAD atomically updates the HEAD pointer to the given version.
func (r *ArenaSourceReconciler) updateHEAD(workspacePath, version string) error {
	arenaDir := filepath.Join(workspacePath, ".arena")
	if err := os.MkdirAll(arenaDir, 0755); err != nil {
		return err
	}

	headPath := filepath.Join(arenaDir, "HEAD")
	tempPath := headPath + ".tmp"

	// Write to temp file first
	if err := os.WriteFile(tempPath, []byte(version), 0644); err != nil {
		return err
	}

	// Atomic rename
	return os.Rename(tempPath, headPath)
}

// gcOldVersions removes old versions exceeding MaxVersionsPerSource.
func (r *ArenaSourceReconciler) gcOldVersions(workspacePath string) error {
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
		maxVersions = 10 // Default
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
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].modTime.Before(versions[j].modTime)
	})

	// Remove oldest versions
	for i := 0; i < len(versions)-maxVersions; i++ {
		versionPath := filepath.Join(versionsDir, versions[i].name)
		if err := os.RemoveAll(versionPath); err != nil {
			return fmt.Errorf("failed to remove old version %s: %w", versions[i].name, err)
		}
	}

	return nil
}

// copyDirectory recursively copies a directory.
func copyDirectory(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, targetPath)
		}

		// Copy file
		return copyFileWithMode(path, targetPath, info.Mode())
	})
}

// copyFileWithMode copies a file preserving its mode.
func copyFileWithMode(src, dst string, mode os.FileMode) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = sourceFile.Close() }()

	destFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		_ = destFile.Close()
		return err
	}

	if err := destFile.Sync(); err != nil {
		_ = destFile.Close()
		return err
	}

	return destFile.Close()
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
