/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"errors"
	"fmt"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/promptpack/packselect"
)

// resolveTriggerCandidate decides whether a version-triggered rollout should
// start and, if so, returns the PromptPack to canary. It returns ok=false (no
// error) for every "not now" case: no trigger configured, an in-flight rollout
// or promotion, a first deploy (no stable pod yet), nothing newer on the
// channel, or a version that just rolled back (guarded via the annotation so a
// persistently-failing version doesn't loop canary -> rollback -> canary).
func (r *AgentRuntimeReconciler) resolveTriggerCandidate(ctx context.Context, ar *omniav1alpha1.AgentRuntime) (*omniav1alpha1.PromptPack, bool, error) {
	if ar.Spec.Rollout == nil || ar.Spec.Rollout.Trigger == nil {
		return nil, false, nil
	}
	if isRolloutActive(ar) || (ar.Status.Rollout != nil && ar.Status.Rollout.Promoting) {
		return nil, false, nil // don't disturb an in-flight rollout
	}
	if ar.Status.ActiveVersion == nil || *ar.Status.ActiveVersion == "" {
		return nil, false, nil // first deploy: let stable come up, no canary
	}
	latest, err := r.latestPackForChannel(ctx, ar.Namespace, ar.Spec.PromptPackRef.Name, ar.Spec.Rollout.Trigger.PromptPackChannel)
	if err != nil {
		if errors.Is(err, errNoMatchingPromptPack) {
			return nil, false, nil // nothing published on the channel yet
		}
		return nil, false, err
	}
	if !versionsNewer(latest.Spec.Version, *ar.Status.ActiveVersion) {
		return nil, false, nil // not newer (equal or older)
	}
	if lrb := ar.Annotations[lastRolledBackVersionAnnotation]; lrb != "" && !versionsNewer(latest.Spec.Version, lrb) {
		return nil, false, nil // just rolled back; wait for a strictly newer version
	}
	return latest, true, nil
}

// maybeTriggerVersionRollout implements the version-triggered canary (#1838):
// when a newer PromptPack version than the agent's active version appears on the
// watched channel, it sets that version as spec.rollout.candidate so the existing
// rollout engine canaries it on the next reconcile.
func (r *AgentRuntimeReconciler) maybeTriggerVersionRollout(ctx context.Context, ar *omniav1alpha1.AgentRuntime) (bool, error) {
	latest, ok, err := r.resolveTriggerCandidate(ctx, ar)
	if err != nil || !ok {
		return false, err
	}
	candidate := &omniav1alpha1.CandidateOverrides{
		PromptPackRef: &omniav1alpha1.PromptPackRef{Name: ar.Spec.PromptPackRef.Name, Version: &latest.Spec.Version},
	}
	// idempotency: candidate already reflects latest -> no-op
	if ar.Spec.Rollout.Candidate != nil && !promptPackRefDiffers(&omniav1alpha1.CandidateOverrides{PromptPackRef: candidate.PromptPackRef}, ar) {
		return false, nil
	}
	ar.Spec.Rollout.Candidate = candidate
	if err := r.Update(ctx, ar); err != nil {
		return false, err
	}
	r.recordRolloutNormal(ar, eventReasonRolloutTriggered,
		fmt.Sprintf("version-triggered rollout: candidate %s@%s (channel %s)", ar.Spec.PromptPackRef.Name, latest.Spec.Version, ar.Spec.Rollout.Trigger.PromptPackChannel))
	return true, nil
}

// versionsNewer reports whether a is a strictly newer semver than b. Both are
// parsed with packselect.ParseVersion (shared with the resolver so "v"-prefixed
// forms compare consistently); an unparseable value on either side returns
// false rather than risking a spurious canary on malformed version strings.
func versionsNewer(a, b string) bool {
	av, err := packselect.ParseVersion(a)
	if err != nil {
		return false
	}
	bv, err := packselect.ParseVersion(b)
	if err != nil {
		return false
	}
	return av.GreaterThan(bv)
}

// lastRolledBackVersionAnnotation records the PromptPack version of the most
// recently rolled-back candidate. The version-trigger reads it to avoid
// immediately re-canarying a version that just failed. An annotation (not a
// status field) is used so it survives the per-reconcile status.rollout rebuild.
const lastRolledBackVersionAnnotation = "omnia.altairalabs.ai/last-rolled-back-version"

// recordRolledBackVersion stamps the version of the candidate being rolled back
// onto the AgentRuntime. Call it BEFORE rollback() reverts the candidate ref. It
// is a no-op for a candidate without a pinned version (e.g. a manual provider-
// only override), which the version-trigger never produces anyway.
func recordRolledBackVersion(ar *omniav1alpha1.AgentRuntime) {
	if ar.Spec.Rollout == nil || ar.Spec.Rollout.Candidate == nil ||
		ar.Spec.Rollout.Candidate.PromptPackRef == nil ||
		ar.Spec.Rollout.Candidate.PromptPackRef.Version == nil {
		return
	}
	if ar.Annotations == nil {
		ar.Annotations = map[string]string{}
	}
	ar.Annotations[lastRolledBackVersionAnnotation] = *ar.Spec.Rollout.Candidate.PromptPackRef.Version
}
