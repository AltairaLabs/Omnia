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

	"github.com/Masterminds/semver/v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// gcOldVersions garbage-collects Superseded PromptPack version-objects for a
// pack beyond the retention limit. Two guards protect an otherwise-eligible
// candidate: (a) it is referenced by an AgentRuntime, or (b) it is younger than
// the minimum retention age. Each GC'd object takes its backing -content
// ConfigMap with it.
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
	highestName := highestVersionObjName(packs.Items)
	cutoff := time.Now().Add(-r.MinRetentionAge)

	return r.deleteCandidates(ctx, candidates, refs, highestName, cutoff)
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

// deleteCandidates removes each candidate that is neither referenced nor younger
// than the min-age cutoff, deleting its backing ConfigMap alongside it.
func (r *PromptPackSourceReconciler) deleteCandidates(ctx context.Context, candidates []corev1alpha1.PromptPack, refs *arRefIndex, highestName string, cutoff time.Time) error {
	for i := range candidates {
		pp := &candidates[i]
		if refs.isReferenced(pp, pp.Name == highestName) {
			continue // an AgentRuntime still points at this version
		}
		if pp.CreationTimestamp.Time.After(cutoff) {
			continue // younger than the min-age guard
		}
		if err := r.deletePackAndContent(ctx, pp); err != nil {
			return err
		}
	}
	return nil
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
	av, aErr := semver.NewVersion(strings.TrimPrefix(a, "v"))
	bv, bErr := semver.NewVersion(strings.TrimPrefix(b, "v"))
	if aErr != nil || bErr != nil {
		return a > b
	}
	return av.GreaterThan(bv)
}

// highestVersionObjName returns the object name of the highest-version pack in
// the list (across all phases), or "" when the list is empty.
func highestVersionObjName(packs []corev1alpha1.PromptPack) string {
	bestName, bestVer := "", ""
	for i := range packs {
		if bestName == "" || packVersionGreater(packs[i].Spec.Version, bestVer) {
			bestName, bestVer = packs[i].Name, packs[i].Spec.Version
		}
	}
	return bestName
}

// arRefIndex indexes the PromptPack references held by every AgentRuntime in a
// namespace, so GC can protect versions that are still in use.
type arRefIndex struct {
	pinned    map[string]map[string]struct{} // packName -> normalized pinned versions
	trackOnly map[string]bool                // packName -> a track-only ref exists
}

// referencedRefs builds the reference index once per GC call by listing every
// AgentRuntime in the namespace and collecting its stable ref and (when a
// rollout is active) its candidate ref.
func (r *PromptPackSourceReconciler) referencedRefs(ctx context.Context, namespace string) (*arRefIndex, error) {
	var ars corev1alpha1.AgentRuntimeList
	if err := r.List(ctx, &ars, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	idx := &arRefIndex{pinned: map[string]map[string]struct{}{}, trackOnly: map[string]bool{}}
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
// version; a track-only ref flags the pack for newest-object protection.
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
	if ref.Track != nil && *ref.Track != "" {
		x.trackOnly[ref.Name] = true
	}
}

// isReferenced reports whether pp is protected from GC by an AgentRuntime ref.
// A version-pinned ref protects the exact version. A track-only ref is treated
// conservatively: it protects only the highest-version object of the pack
// (isHighest), because resolving the precise channel-max here would need the
// full resolver — over-protecting the newest object is the safe choice, and a
// track-only ref never legitimately points at an older Superseded version.
func (x *arRefIndex) isReferenced(pp *corev1alpha1.PromptPack, isHighest bool) bool {
	name := pp.Labels[labelPromptPackName]
	if versions, ok := x.pinned[name]; ok {
		if _, pinned := versions[normalizeVersion(pp.Spec.Version)]; pinned {
			return true
		}
	}
	return isHighest && x.trackOnly[name]
}

// normalizeVersion strips a leading "v" so pinned refs and pack versions compare
// equal regardless of the optional prefix (v1.2.0 == 1.2.0).
func normalizeVersion(s string) string {
	return strings.TrimPrefix(s, "v")
}
