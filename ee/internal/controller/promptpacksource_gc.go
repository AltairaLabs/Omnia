/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/promptpack/packselect"
)

// supersededConditionType is the PromptPack status condition the core controller
// stamps when a version is superseded; its LastTransitionTime is the supersession
// time the retention min-age guard measures from. Defined locally because the
// core controller's const lives across the module boundary.
const supersededConditionType = "Superseded"

// trackStable is the stable release channel; a stable track excludes
// prereleases. Aliases packselect.TrackStable.
const trackStable = packselect.TrackStable

// gcOldVersions garbage-collects Superseded PromptPack version-objects for a
// pack beyond the retention limit. Two guards protect an otherwise-eligible
// candidate: (a) it is referenced by an AgentRuntime (an exact version pin, or a
// track-only ref pointing at that track's channel-max), or (b) it was superseded
// more recently than the minimum retention age (protecting the post-promote
// rollback window). Each GC'd object takes its backing -content ConfigMap with it.
func (r *PromptPackSourceReconciler) gcOldVersions(ctx context.Context, src *omniav1alpha1.PromptPackSource) error {
	limit := r.historyLimit(src)

	var packs corev1alpha1.PromptPackList
	if err := r.List(ctx, &packs,
		client.InNamespace(src.Namespace),
		client.MatchingLabels{labelPromptPackName: src.Spec.PackName}); err != nil {
		return err
	}

	// Only Superseded versions are GC candidates.
	superseded := filterSuperseded(packs.Items)
	if len(superseded) <= limit {
		return nil
	}
	sortByVersionDesc(superseded)
	candidates := superseded[limit:] // everything beyond the keep window

	refs, err := r.referencedRefs(ctx, src.Namespace)
	if err != nil {
		return err
	}
	// Object names a track-only ref protects, resolved channel-aware (a stable
	// track protects the stable-channel-max, a prerelease track the overall max).
	protected := refs.protectedTrackNames(src.Spec.PackName, packs.Items)
	cutoff := time.Now().Add(-r.MinRetentionAge)

	return r.deleteCandidates(ctx, candidates, refs, protected, cutoff)
}

// historyLimit resolves the retention window: a per-source spec.historyLimit
// override when set, otherwise the reconciler-wide MaxVersionsPerSource default.
func (r *PromptPackSourceReconciler) historyLimit(src *omniav1alpha1.PromptPackSource) int {
	limit := r.MaxVersionsPerSource
	if src.Spec.HistoryLimit != nil {
		limit = int(*src.Spec.HistoryLimit)
	}
	// Defense in depth against a negative slice bound (Minimum=0 also guards at admission).
	if limit < 0 {
		limit = 0
	}
	return limit
}

// deleteCandidates removes each candidate that is neither referenced nor was
// superseded more recently than the min-age cutoff, deleting its backing
// ConfigMap alongside it.
func (r *PromptPackSourceReconciler) deleteCandidates(ctx context.Context, candidates []corev1alpha1.PromptPack, refs *arRefIndex, protected map[string]struct{}, cutoff time.Time) error {
	for i := range candidates {
		pp := &candidates[i]
		if refs.isReferenced(pp, protected) {
			continue // an AgentRuntime still points at this version
		}
		if supersededAt(pp).After(cutoff) {
			continue // superseded more recently than the min-age guard
		}
		if err := r.deletePackAndContent(ctx, pp); err != nil {
			return err
		}
	}
	return nil
}

// supersededAt reports when pp became Superseded: the Superseded condition's
// LastTransitionTime when present, else the object's creation time. Measuring
// from supersession (not creation) protects a long-lived version that was only
// just superseded — the rollback target after a promote.
func supersededAt(pp *corev1alpha1.PromptPack) time.Time {
	if c := meta.FindStatusCondition(pp.Status.Conditions, supersededConditionType); c != nil {
		return c.LastTransitionTime.Time
	}
	return pp.CreationTimestamp.Time
}

// deletePackAndContent deletes a PromptPack version-object and its backing
// -content ConfigMap. A NotFound on either is treated as already-collected.
func (r *PromptPackSourceReconciler) deletePackAndContent(ctx context.Context, pp *corev1alpha1.PromptPack) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: pp.Name + contentSuffix, Namespace: pp.Namespace},
	}
	if err := r.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err := r.Delete(ctx, pp); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// filterSuperseded returns only the PromptPacks whose status phase is Superseded.
func filterSuperseded(packs []corev1alpha1.PromptPack) []corev1alpha1.PromptPack {
	out := make([]corev1alpha1.PromptPack, 0, len(packs))
	for i := range packs {
		if packs[i].Status.Phase == corev1alpha1.PromptPackPhaseSuperseded {
			out = append(out, packs[i])
		}
	}
	return out
}

// sortByVersionDesc orders PromptPacks newest-version first (semver, tolerating a
// leading "v"). Unparseable versions sort deterministically by raw string.
func sortByVersionDesc(packs []corev1alpha1.PromptPack) {
	sort.SliceStable(packs, func(i, j int) bool {
		return packVersionGreater(packs[i].Spec.Version, packs[j].Spec.Version)
	})
}

// packVersionGreater reports whether version a is greater than b under semver,
// falling back to string comparison when either side is unparseable.
func packVersionGreater(a, b string) bool {
	av, aErr := packselect.ParseVersion(a)
	bv, bErr := packselect.ParseVersion(b)
	if aErr != nil || bErr != nil {
		return a > b
	}
	return av.GreaterThan(bv)
}

// arRefIndex indexes the PromptPack references held by every AgentRuntime in a
// namespace, so GC can protect versions that are still in use.
type arRefIndex struct {
	pinned    map[string]map[string]struct{} // packName -> normalized pinned versions
	trackOnly map[string]map[string]struct{} // packName -> set of track-only tracks referenced
}

// referencedRefs builds the reference index once per GC call by listing every
// AgentRuntime in the namespace and collecting its stable ref and (when a
// rollout is active) its candidate ref.
func (r *PromptPackSourceReconciler) referencedRefs(ctx context.Context, namespace string) (*arRefIndex, error) {
	var ars corev1alpha1.AgentRuntimeList
	if err := r.List(ctx, &ars, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	idx := &arRefIndex{pinned: map[string]map[string]struct{}{}, trackOnly: map[string]map[string]struct{}{}}
	for i := range ars.Items {
		ar := &ars.Items[i]
		idx.add(&ar.Spec.PromptPackRef)
		if ar.Spec.Rollout != nil && ar.Spec.Rollout.Candidate != nil {
			idx.add(ar.Spec.Rollout.Candidate.PromptPackRef)
		}
	}
	return idx, nil
}

// add records one PromptPack reference: a version-pinned ref protects that exact
// version; a track ref (explicit, or a bare {name}-only ref that defaults to the
// stable channel — mirroring selectPromptPack) records the track so GC can
// protect that channel's max.
func (x *arRefIndex) add(ref *corev1alpha1.PromptPackRef) {
	if ref == nil || ref.Name == "" {
		return
	}
	if ref.Version != nil && *ref.Version != "" {
		if x.pinned[ref.Name] == nil {
			x.pinned[ref.Name] = map[string]struct{}{}
		}
		x.pinned[ref.Name][normalizeVersion(*ref.Version)] = struct{}{}
		return
	}
	// No version pin: the ref resolves to a channel-max. An explicit track selects
	// that channel; a bare {name}-only ref defaults to stable (see selectPromptPack).
	track := trackStable
	if ref.Track != nil && *ref.Track != "" {
		track = *ref.Track
	}
	if x.trackOnly[ref.Name] == nil {
		x.trackOnly[ref.Name] = map[string]struct{}{}
	}
	x.trackOnly[ref.Name][track] = struct{}{}
}

// protectedTrackNames returns the set of object names protected by track-only
// refs for packName, resolving each referenced track to its channel-max object.
func (x *arRefIndex) protectedTrackNames(packName string, packs []corev1alpha1.PromptPack) map[string]struct{} {
	out := map[string]struct{}{}
	for track := range x.trackOnly[packName] {
		// Resolve each referenced track to its channel-max object so GC protects
		// exactly the object a track-only ref resolves to. A track with no
		// qualifying version (packselect.ChannelMax error) protects nothing.
		if pp, err := packselect.ChannelMax(packs, track); err == nil {
			out[pp.Name] = struct{}{}
		}
	}
	return out
}

// isReferenced reports whether pp is protected from GC by an AgentRuntime ref: an
// exact version pin, or membership in protected (the channel-max object names a
// track-only ref points at, resolved channel-aware by protectedTrackNames).
func (x *arRefIndex) isReferenced(pp *corev1alpha1.PromptPack, protected map[string]struct{}) bool {
	name := pp.Labels[labelPromptPackName]
	if versions, ok := x.pinned[name]; ok {
		if _, pinned := versions[normalizeVersion(pp.Spec.Version)]; pinned {
			return true
		}
	}
	_, ok := protected[pp.Name]
	return ok
}

// normalizeVersion strips a leading "v" so pinned refs and pack versions compare
// equal regardless of the optional prefix (v1.2.0 == 1.2.0).
func normalizeVersion(s string) string {
	return strings.TrimPrefix(s, "v")
}
