/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/sourcesync"
)

// SkillSource condition types.
const (
	SkillSourceConditionSourceAvailable = "SourceAvailable"
	SkillSourceConditionContentValid    = "ContentValid"
)

// SkillSourceReconciler reconciles SkillSource objects.
type SkillSourceReconciler struct {
	client.Client
	Scheme   *k8sruntime.Scheme
	Recorder record.EventRecorder

	// WorkspaceContentPath is the base path for workspace content volumes,
	// mirroring the ee arena controllers. Layout:
	//   {WorkspaceContentPath}/{workspace}/{namespace}/{targetPath}/
	WorkspaceContentPath string

	// MaxVersionsPerSource bounds how many content-addressable snapshots
	// FilesystemSyncer retains. Default 10.
	MaxVersionsPerSource int

	// StorageManager optionally provisions the workspace PVC before writes.
	// When nil, the syncer assumes the PVC is already mounted.
	StorageManager sourcesync.StorageManager
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=skillsources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=skillsources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=skillsources/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile implements the SkillSource reconcile loop.
func (r *SkillSourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling SkillSource", "name", req.Name, "namespace", req.Namespace)

	src := &corev1alpha1.SkillSource{}
	if err := r.Get(ctx, req.NamespacedName, src); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if src.Spec.Suspend {
		return ctrl.Result{}, nil
	}

	interval, err := time.ParseDuration(src.Spec.Interval)
	if err != nil {
		return r.errorStatus(ctx, src, "InvalidInterval", err)
	}

	outcome, fetchErr := r.fetchAndSync(ctx, src)
	if fetchErr != nil {
		return r.errorStatus(ctx, src, fetchErr.reason, fetchErr.cause)
	}
	defer func() { _ = os.RemoveAll(outcome.artifact.Path) }()

	resolved, parseErrs := ResolveSkills(
		filepath.Join(r.WorkspaceContentPath, outcome.workspaceName, src.Namespace, outcome.contentPath),
		src.Spec.Filter)
	dupes := findDuplicateNames(resolved)

	r.applySuccessStatus(src, outcome, resolved, parseErrs, dupes, interval)

	if err := r.Status().Update(ctx, src); err != nil {
		log.Error(err, "status update failed")
		return ctrl.Result{}, err
	}
	if r.Recorder != nil {
		r.Recorder.Event(src, "Normal", "Synced",
			fmt.Sprintf("synced %d skills at revision %s", len(resolved), outcome.revision))
	}
	return ctrl.Result{RequeueAfter: interval}, nil
}

// reconcileFailure carries the (reason, cause) pair to errorStatus.
type reconcileFailure struct {
	reason string
	cause  error
}

// syncOutcome bundles everything fetchAndSync produces on success so callers
// don't have to thread six return values around.
type syncOutcome struct {
	artifact      *sourcesync.Artifact
	revision      string
	contentPath   string
	version       string
	workspaceName string
}

// fetchAndSync runs the full fetcher → syncer pipeline. Returns a syncOutcome
// on success or a reconcileFailure describing which stage failed.
func (r *SkillSourceReconciler) fetchAndSync(ctx context.Context, src *corev1alpha1.SkillSource) (*syncOutcome, *reconcileFailure) {
	opts := sourcesync.DefaultOptions()
	if src.Spec.Timeout != "" {
		if to, err := time.ParseDuration(src.Spec.Timeout); err == nil {
			opts.Timeout = to
		}
	}
	fetcher, err := r.fetcherFor(ctx, src, opts)
	if err != nil {
		return nil, &reconcileFailure{reason: "FetcherBuild", cause: err}
	}
	revision, err := fetcher.LatestRevision(ctx)
	if err != nil {
		return nil, &reconcileFailure{reason: "LatestRevision", cause: err}
	}
	artifact, err := fetcher.Fetch(ctx, revision)
	if err != nil {
		return nil, &reconcileFailure{reason: "Fetch", cause: err}
	}

	targetPath := src.Spec.TargetPath
	if targetPath == "" {
		targetPath = filepath.Join("skills", src.Name)
	}
	syncer := &sourcesync.FilesystemSyncer{
		WorkspaceContentPath: r.WorkspaceContentPath,
		MaxVersionsPerSource: r.MaxVersionsPerSource,
	}
	if r.StorageManager != nil {
		syncer.StorageManager = r.StorageManager
	}
	workspaceName := GetWorkspaceForNamespace(ctx, r.Client, src.Namespace)
	contentPath, version, err := syncer.SyncToFilesystem(ctx, sourcesync.SyncParams{
		WorkspaceName: workspaceName,
		Namespace:     src.Namespace,
		TargetPath:    targetPath,
		Artifact:      artifact,
	})
	if err != nil {
		_ = os.RemoveAll(artifact.Path)
		return nil, &reconcileFailure{reason: "Sync", cause: err}
	}
	return &syncOutcome{
		artifact:      artifact,
		revision:      revision,
		contentPath:   contentPath,
		version:       version,
		workspaceName: workspaceName,
	}, nil
}

func findDuplicateNames(resolved []ResolvedSkill) []string {
	seen := map[string]struct{}{}
	var dupes []string
	for _, sk := range resolved {
		if _, ok := seen[sk.Name]; ok {
			dupes = append(dupes, sk.Name)
		}
		seen[sk.Name] = struct{}{}
	}
	return dupes
}

func (r *SkillSourceReconciler) applySuccessStatus(
	src *corev1alpha1.SkillSource,
	outcome *syncOutcome,
	resolved []ResolvedSkill,
	parseErrs []error,
	dupes []string,
	interval time.Duration,
) {
	src.Status.ObservedGeneration = src.Generation
	now := metav1.Time{Time: time.Now()}
	next := metav1.Time{Time: time.Now().Add(interval)}
	src.Status.LastFetchTime = &now
	src.Status.NextFetchTime = &next
	src.Status.Artifact = &corev1alpha1.Artifact{
		Revision:       outcome.revision,
		ContentPath:    outcome.contentPath,
		Version:        outcome.version,
		Checksum:       outcome.artifact.Checksum,
		Size:           outcome.artifact.Size,
		LastUpdateTime: metav1.Time{Time: outcome.artifact.LastModified},
	}
	src.Status.SkillCount = int32(len(resolved))
	meta.SetStatusCondition(&src.Status.Conditions, metav1.Condition{
		Type:               SkillSourceConditionSourceAvailable,
		Status:             metav1.ConditionTrue,
		Reason:             "FetchSucceeded",
		Message:            fmt.Sprintf("revision %s", outcome.revision),
		ObservedGeneration: src.Generation,
	})

	contentValid := len(parseErrs) == 0 && len(dupes) == 0
	condStatus := metav1.ConditionTrue
	condReason := "ContentValid"
	condMsg := fmt.Sprintf("%d skills validated", len(resolved))
	if !contentValid {
		condStatus = metav1.ConditionFalse
		condReason = "InvalidContent"
		condMsg = fmt.Sprintf("%d parse errors; duplicate names: %v", len(parseErrs), dupes)
	}
	meta.SetStatusCondition(&src.Status.Conditions, metav1.Condition{
		Type:               SkillSourceConditionContentValid,
		Status:             condStatus,
		Reason:             condReason,
		Message:            condMsg,
		ObservedGeneration: src.Generation,
	})
	src.Status.Phase = corev1alpha1.SkillSourcePhaseReady
}

func (r *SkillSourceReconciler) fetcherFor(ctx context.Context, src *corev1alpha1.SkillSource, opts sourcesync.Options) (sourcesync.Fetcher, error) {
	switch src.Spec.Type {
	case corev1alpha1.SkillSourceTypeGit:
		return r.gitFetcher(ctx, src, opts)
	case corev1alpha1.SkillSourceTypeOCI:
		return r.ociFetcher(ctx, src, opts)
	case corev1alpha1.SkillSourceTypeConfigMap:
		return r.configMapFetcher(src, opts)
	}
	return nil, fmt.Errorf("unknown source type %q", src.Spec.Type)
}

func (r *SkillSourceReconciler) gitFetcher(ctx context.Context, src *corev1alpha1.SkillSource, opts sourcesync.Options) (sourcesync.Fetcher, error) {
	if src.Spec.Git == nil {
		return nil, fmt.Errorf("git source missing spec.git")
	}
	cfg := sourcesync.GitFetcherConfig{
		URL:     src.Spec.Git.URL,
		Path:    src.Spec.Git.Path,
		Options: opts,
	}
	if src.Spec.Git.Ref != nil {
		cfg.Ref = sourcesync.GitRef{
			Branch: src.Spec.Git.Ref.Branch,
			Tag:    src.Spec.Git.Ref.Tag,
			Commit: src.Spec.Git.Ref.Commit,
		}
	}
	if src.Spec.Git.SecretRef != nil {
		creds, err := sourcesync.LoadGitCredentials(ctx, r.Client, src.Namespace, src.Spec.Git.SecretRef.Name)
		if err != nil {
			return nil, fmt.Errorf("load git credentials: %w", err)
		}
		cfg.Credentials = creds
	}
	return sourcesync.NewGitFetcher(cfg), nil
}

func (r *SkillSourceReconciler) ociFetcher(ctx context.Context, src *corev1alpha1.SkillSource, opts sourcesync.Options) (sourcesync.Fetcher, error) {
	if src.Spec.OCI == nil {
		return nil, fmt.Errorf("oci source missing spec.oci")
	}
	cfg := sourcesync.OCIFetcherConfig{
		URL:      src.Spec.OCI.URL,
		Insecure: src.Spec.OCI.Insecure,
		Options:  opts,
	}
	if src.Spec.OCI.SecretRef != nil {
		creds, err := sourcesync.LoadOCICredentials(ctx, r.Client, src.Namespace, src.Spec.OCI.SecretRef.Name)
		if err != nil {
			return nil, fmt.Errorf("load oci credentials: %w", err)
		}
		cfg.Credentials = creds
	}
	return sourcesync.NewOCIFetcher(cfg), nil
}

func (r *SkillSourceReconciler) configMapFetcher(src *corev1alpha1.SkillSource, opts sourcesync.Options) (sourcesync.Fetcher, error) {
	if src.Spec.ConfigMap == nil {
		return nil, fmt.Errorf("configmap source missing spec.configMap")
	}
	cfg := sourcesync.ConfigMapFetcherConfig{
		Name:      src.Spec.ConfigMap.Name,
		Namespace: src.Namespace,
		Options:   opts,
	}
	return sourcesync.NewConfigMapFetcher(cfg, r.Client), nil
}

func (r *SkillSourceReconciler) errorStatus(ctx context.Context, src *corev1alpha1.SkillSource, reason string, cause error) (ctrl.Result, error) {
	src.Status.Phase = corev1alpha1.SkillSourcePhaseError
	meta.SetStatusCondition(&src.Status.Conditions, metav1.Condition{
		Type:               SkillSourceConditionSourceAvailable,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            cause.Error(),
		ObservedGeneration: src.Generation,
	})
	if err := r.Status().Update(ctx, src); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// SetupWithManager registers the reconciler with a controller-runtime manager.
func (r *SkillSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.SkillSource{}).
		Named("skillsource").
		Complete(r)
}
