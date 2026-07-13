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
)

// maybeTriggerVersionRollout implements the version-triggered canary (#1838):
// when spec.rollout.trigger is set, and a newer PromptPack version than the
// agent's current active version appears on the watched channel, it sets
// that version as spec.rollout.candidate so the existing rollout engine
// canaries it on the next reconcile. It never disturbs an in-flight rollout
// or promotion, and never canaries a first deploy (no stable pod yet).
func (r *AgentRuntimeReconciler) maybeTriggerVersionRollout(ctx context.Context, ar *omniav1alpha1.AgentRuntime) (bool, error) {
	if ar.Spec.Rollout == nil || ar.Spec.Rollout.Trigger == nil {
		return false, nil
	}
	if isRolloutActive(ar) || (ar.Status.Rollout != nil && ar.Status.Rollout.Promoting) {
		return false, nil // don't disturb an in-flight rollout
	}
	if ar.Status.ActiveVersion == nil || *ar.Status.ActiveVersion == "" {
		return false, nil // first deploy: let stable come up, no canary
	}
	latest, err := r.latestPackForChannel(ctx, ar.Namespace, ar.Spec.PromptPackRef.Name, ar.Spec.Rollout.Trigger.PromptPackChannel)
	if err != nil {
		if errors.Is(err, errNoMatchingPromptPack) {
			return false, nil // nothing published on the channel yet
		}
		return false, err
	}
	if !versionsNewer(latest.Spec.Version, *ar.Status.ActiveVersion) {
		return false, nil // not newer (equal or older)
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
// parsed with parsePackVersion (shared with the resolver so "v"-prefixed
// forms compare consistently); an unparseable value on either side returns
// false rather than risking a spurious canary on malformed version strings.
func versionsNewer(a, b string) bool {
	av, err := parsePackVersion(a)
	if err != nil {
		return false
	}
	bv, err := parsePackVersion(b)
	if err != nil {
		return false
	}
	return av.GreaterThan(bv)
}
