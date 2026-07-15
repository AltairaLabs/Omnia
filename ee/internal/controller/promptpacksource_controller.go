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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/license"
	"github.com/altairalabs/omnia/internal/promptpack/packselect"
	"github.com/altairalabs/omnia/internal/sourcesync"
)

// labelPromptPackName is the resolution label carried by every materialized
// PromptPack version-object and its backing ConfigMap. The value is the logical
// pack name (spec.packName). Aliases packselect.Label, the single source of
// truth for the value.
const labelPromptPackName = packselect.Label

// managedByPromptPack marks the backing ConfigMap as owned by the promptpack
// materialization flow (mirrors the dashboard deploy route convention).
const (
	labelPromptPackManagedBy = "omnia.altairalabs.ai/managed-by"
	managedByPromptPack      = "promptpack"
	contentSuffix            = "-content"
	// packJSONKey is the ConfigMap data key (and fetched filename) holding the pack.
	packJSONKey = "pack.json"
)

// PromptPackSource event reasons.
const (
	eventReasonVersionMaterialized = "VersionMaterialized"
	reasonLicenseViolation         = "LicenseViolation"
	reasonMissingVersion           = "MissingVersion"
)

// errMissingVersion is returned when a fetched pack.json has an empty version.
var errMissingVersion = errors.New("pack.json has no version — cannot materialize a version-object")

// PromptPackSourceReconciler reconciles a PromptPackSource object by polling its
// upstream feed (Linear-HEAD) and materializing the current {packName, version}
// as an immutable PromptPack version-object plus a backing ConfigMap.
type PromptPackSourceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// LicenseValidator gates source types (defense in depth; the webhook is the
	// primary enforcement point). Nil disables the check.
	LicenseValidator *license.Validator

	// MaxVersionsPerSource bounds retained version-objects per pack (used by GC).
	MaxVersionsPerSource int

	// MinRetentionAge is the minimum age before a Superseded version is a GC
	// candidate.
	MinRetentionAge time.Duration

	// FetcherFor lets tests inject a fake fetcher. When nil the real git/oci
	// builder is used.
	FetcherFor func(ctx context.Context, src *omniav1alpha1.PromptPackSource) (sourcesync.Fetcher, error)
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=promptpacksources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=promptpacksources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=promptpacksources/finalizers,verbs=update
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=promptpacks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile fetches the current artifact and materializes its version-object.
func (r *PromptPackSourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling PromptPackSource", "name", req.Name, "namespace", req.Namespace)

	src := &omniav1alpha1.PromptPackSource{}
	if err := r.Get(ctx, req.NamespacedName, src); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if src.Status.Phase == "" {
		src.Status.Phase = omniav1alpha1.PromptPackSourcePhasePending
	}
	src.Status.ObservedGeneration = src.Generation

	if src.Spec.Suspend {
		log.V(1).Info("PromptPackSource suspended, skipping", "name", req.Name)
		return ctrl.Result{}, nil
	}

	if r.LicenseValidator != nil {
		if err := r.LicenseValidator.ValidatePromptPackSource(ctx, string(src.Spec.Type)); err != nil {
			// License must change — do not requeue.
			return r.setErrorStatus(ctx, src, reasonLicenseViolation, err, 0)
		}
	}

	interval, err := time.ParseDuration(src.Spec.Interval)
	if err != nil {
		return r.setErrorStatus(ctx, src, "InvalidInterval", err, time.Minute)
	}

	return r.fetchAndMaterialize(ctx, src, interval)
}

// fetchAndMaterialize runs one Linear-HEAD fetch and materializes the artifact.
func (r *PromptPackSourceReconciler) fetchAndMaterialize(ctx context.Context, src *omniav1alpha1.PromptPackSource, interval time.Duration) (ctrl.Result, error) {
	fetcher, err := r.fetcherFor(ctx, src)
	if err != nil {
		return r.setErrorStatus(ctx, src, "FetcherBuild", err, time.Minute)
	}

	fetchCtx, cancel := context.WithTimeout(ctx, parseTimeout(src.Spec.Timeout))
	defer cancel()

	rev, _ := fetcher.LatestRevision(fetchCtx)
	artifact, err := fetcher.Fetch(fetchCtx, rev)
	if err != nil {
		return r.setErrorStatus(ctx, src, "Fetch", err, interval)
	}
	if artifact.Path != "" && !artifact.Preserve {
		defer func() { _ = os.RemoveAll(artifact.Path) }()
	}

	packJSON, err := os.ReadFile(filepath.Join(artifact.Path, packJSONKey))
	if err != nil {
		return r.setErrorStatus(ctx, src, "ReadPackJSON", err, interval)
	}
	version, err := parsePackVersion(packJSON)
	if err != nil {
		return r.setErrorStatus(ctx, src, "InvalidPackJSON", err, interval)
	}
	if version == "" {
		return r.setErrorStatus(ctx, src, reasonMissingVersion, errMissingVersion, interval)
	}

	created, err := r.materialize(ctx, src, version, packJSON)
	if err != nil {
		return r.setErrorStatus(ctx, src, "Materialize", err, interval)
	}

	r.applySuccessStatus(src, version, rev, artifact, created, interval)
	if err := r.Status().Update(ctx, src); err != nil {
		return ctrl.Result{}, err
	}
	if created && r.Recorder != nil {
		r.Recorder.Event(src, corev1.EventTypeNormal, eventReasonVersionMaterialized,
			fmt.Sprintf("materialized version %s", version))
	}

	if err := r.gcOldVersions(ctx, src); err != nil {
		logf.FromContext(ctx).Error(err, "version GC failed")
	}

	return ctrl.Result{RequeueAfter: interval}, nil
}

// materialize creates the backing ConfigMap and the PromptPack version-object.
// Both are idempotent — AlreadyExists is a benign no-op. Returns whether a new
// PromptPack version-object was created.
func (r *PromptPackSourceReconciler) materialize(ctx context.Context, src *omniav1alpha1.PromptPackSource, version string, packJSON []byte) (bool, error) {
	objName := corev1alpha1.PromptPackObjectName(src.Spec.PackName, version)
	cmName := objName + contentSuffix

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: src.Namespace,
			Labels: map[string]string{
				labelPromptPackManagedBy: managedByPromptPack,
				labelPromptPackName:      src.Spec.PackName,
			},
		},
		Data: map[string]string{packJSONKey: string(packJSON)},
	}
	if err := r.Create(ctx, cm); err != nil && !apierrors.IsAlreadyExists(err) {
		return false, err
	}

	pp := &corev1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objName,
			Namespace: src.Namespace,
			Labels:    map[string]string{labelPromptPackName: src.Spec.PackName},
		},
		Spec: corev1alpha1.PromptPackSpec{
			PackName: src.Spec.PackName,
			Version:  version,
			Source: corev1alpha1.PromptPackContentSource{
				Type:         corev1alpha1.PromptPackSourceTypeConfigMap,
				ConfigMapRef: &corev1.LocalObjectReference{Name: cmName},
			},
		},
	}
	if err := r.Create(ctx, pp); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// applySuccessStatus records a successful materialization on the source status.
func (r *PromptPackSourceReconciler) applySuccessStatus(src *omniav1alpha1.PromptPackSource, version, rev string, artifact *sourcesync.Artifact, created bool, interval time.Duration) {
	src.Status.Phase = omniav1alpha1.PromptPackSourcePhaseReady
	src.Status.LastSyncedVersion = version
	now := metav1.Now()
	next := metav1.NewTime(time.Now().Add(interval))
	src.Status.LastFetchTime = &now
	src.Status.NextFetchTime = &next
	src.Status.Artifact = &corev1alpha1.Artifact{
		Revision:       rev,
		Version:        version,
		Checksum:       artifact.Checksum,
		Size:           artifact.Size,
		LastUpdateTime: metav1.Now(),
	}
	if created {
		src.Status.VersionsMaterialized++
	}
	SetCondition(&src.Status.Conditions, src.Generation, omniav1alpha1.PromptPackSourceConditionReady,
		metav1.ConditionTrue, "Ready", fmt.Sprintf("materialized version %s", version))
}

// setErrorStatus records a degraded/error state and requeues after the given
// duration (0 = no requeue). Returns nil error so the loop does not hot-cycle.
func (r *PromptPackSourceReconciler) setErrorStatus(ctx context.Context, src *omniav1alpha1.PromptPackSource, reason string, cause error, requeue time.Duration) (ctrl.Result, error) {
	src.Status.Phase = omniav1alpha1.PromptPackSourcePhaseError
	SetCondition(&src.Status.Conditions, src.Generation, omniav1alpha1.PromptPackSourceConditionReady,
		metav1.ConditionFalse, reason, cause.Error())
	if r.Recorder != nil {
		r.Recorder.Event(src, corev1.EventTypeWarning, reason, cause.Error())
	}
	if err := r.Status().Update(ctx, src); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: requeue}, nil
}

// fetcherFor returns the injected fetcher factory when set, else the real builder.
func (r *PromptPackSourceReconciler) fetcherFor(ctx context.Context, src *omniav1alpha1.PromptPackSource) (sourcesync.Fetcher, error) {
	if r.FetcherFor != nil {
		return r.FetcherFor(ctx, src)
	}
	return r.buildFetcher(ctx, src)
}

// buildFetcher constructs the real git/oci fetcher for the source.
func (r *PromptPackSourceReconciler) buildFetcher(ctx context.Context, src *omniav1alpha1.PromptPackSource) (sourcesync.Fetcher, error) {
	opts := sourcesync.DefaultOptions()
	opts.Timeout = parseTimeout(src.Spec.Timeout)

	switch src.Spec.Type {
	case omniav1alpha1.PromptPackSourceTypeGit:
		return r.gitFetcher(ctx, src, opts)
	case omniav1alpha1.PromptPackSourceTypeOCI:
		return r.ociFetcher(ctx, src, opts)
	}
	return nil, fmt.Errorf("unknown source type %q", src.Spec.Type)
}

func (r *PromptPackSourceReconciler) gitFetcher(ctx context.Context, src *omniav1alpha1.PromptPackSource, opts sourcesync.Options) (sourcesync.Fetcher, error) {
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

func (r *PromptPackSourceReconciler) ociFetcher(ctx context.Context, src *omniav1alpha1.PromptPackSource, opts sourcesync.Options) (sourcesync.Fetcher, error) {
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

// parsePackVersion extracts the version field from pack.json bytes.
func parsePackVersion(data []byte) (string, error) {
	var meta struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", err
	}
	return meta.Version, nil
}

// parseTimeout returns the fetch timeout, defaulting to 60s on empty/invalid.
func parseTimeout(s string) time.Duration {
	const def = 60 * time.Second
	if s == "" {
		return def
	}
	if to, err := time.ParseDuration(s); err == nil {
		return to
	}
	return def
}

// SetupWithManager registers the reconciler with a controller-runtime manager.
func (r *PromptPackSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.PromptPackSource{}).
		Named("promptpacksource").
		Complete(r)
}
