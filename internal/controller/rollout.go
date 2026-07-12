/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// reasonProgressDeadlineExceeded is the Deployment Progressing-condition reason
// Kubernetes sets when a rollout fails to make progress within
// progressDeadlineSeconds (image pull error, crash loop, unschedulable). It is
// the signal for a genuinely failed candidate — distinct from one that is
// merely still starting.
const reasonProgressDeadlineExceeded = "ProgressDeadlineExceeded"

// Rollout lifecycle Event reasons. Emitted as Kubernetes Events so the rollout
// progression is visible as a chronological history (kubectl describe /
// dashboard timeline) — status conditions only hold current state.
const (
	eventReasonRolloutStep    = "RolloutStep"
	eventReasonPromoting      = "RolloutPromoting"
	eventReasonPromoted       = "RolloutPromoted"
	eventReasonRolledBack     = "RolloutRolledBack"
	eventReasonAnalysisPassed = "RolloutAnalysisPassed"
	eventReasonAnalysisFailed = "RolloutAnalysisFailed"
)

// recordRolloutNormal emits a Normal rollout Event (nil-safe for tests without
// a recorder).
func (r *AgentRuntimeReconciler) recordRolloutNormal(ar *omniav1alpha1.AgentRuntime, reason, message string) {
	if r.Recorder != nil {
		r.Recorder.Event(ar, corev1.EventTypeNormal, reason, message)
	}
}

// recordRolloutWarning emits a Warning rollout Event (nil-safe for tests
// without a recorder).
func (r *AgentRuntimeReconciler) recordRolloutWarning(ar *omniav1alpha1.AgentRuntime, reason, message string) {
	if r.Recorder != nil {
		r.Recorder.Event(ar, corev1.EventTypeWarning, reason, message)
	}
}

// reconcileRollout manages the candidate Deployment lifecycle, step progression,
// promotion, and cleanup. Called from the main Reconcile loop after stable
// resources are created.
func (r *AgentRuntimeReconciler) reconcileRollout(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
	providers map[string]*omniav1alpha1.Provider,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// A promotion in progress owns the reconcile until stable is healthy on the
	// new config. Hold here so idle cleanup doesn't delete the still-serving
	// candidate: spec has already advanced, so isRolloutActive is now false.
	if ar.Status.Rollout != nil && ar.Status.Rollout.Promoting {
		return r.reconcileRolloutPromote(ctx, ar)
	}

	if !isRolloutActive(ar) {
		if r.RolloutMetrics != nil {
			r.RolloutMetrics.Active.WithLabelValues(ar.Namespace, ar.Name).Set(0)
		}
		return r.reconcileRolloutIdle(ctx, ar)
	}

	if r.RolloutMetrics != nil {
		r.RolloutMetrics.Active.WithLabelValues(ar.Namespace, ar.Name).Set(1)
	}

	// Ensure the candidate Deployment exists.
	candidateDeploy, err := r.reconcileCandidateDeployment(ctx, ar, promptPack, toolRegistry, providers)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconcile candidate deployment: %w", err)
	}

	// Auto-rollback when candidate pods are unhealthy and mode is automatic.
	if shouldAutoRollback(ar, candidateDeploy) {
		log.Info("rollout auto-rollback triggered",
			"agentRuntime", ar.Name,
			"reason", "pod_unhealthy")
		rollback(ar)
		if err := r.Update(ctx, ar); err != nil {
			return ctrl.Result{}, fmt.Errorf("persist auto-rollback spec: %w", err)
		}
		if err := r.deleteCandidateDeployment(ctx, ar); err != nil {
			return ctrl.Result{}, fmt.Errorf("delete candidate after auto-rollback: %w", err)
		}
		if ar.Spec.Rollout != nil && ar.Spec.Rollout.TrafficRouting != nil {
			if err := r.resetTrafficRoutingForMode(ctx, ar); err != nil {
				log.Error(err, "failed to reset traffic routing on auto-rollback")
			}
			if ar.Spec.Rollout.TrafficRouting.Istio != nil {
				if err := r.patchDestinationRuleConsistentHash(ctx, ar.Namespace,
					ar.Spec.Rollout.TrafficRouting.Istio, ""); err != nil {
					log.Error(err, "failed to remove consistent hash on auto-rollback")
				}
			}
		}
		if r.RolloutMetrics != nil {
			r.RolloutMetrics.Rollbacks.WithLabelValues(ar.Namespace, ar.Name, "pod_unhealthy").Inc()
			r.RolloutMetrics.TrafficWeight.WithLabelValues(ar.Namespace, ar.Name, "stable").Set(100)
			r.RolloutMetrics.TrafficWeight.WithLabelValues(ar.Namespace, ar.Name, "canary").Set(0)
		}
		ar.Status.Rollout = &omniav1alpha1.RolloutStatus{Active: false, Message: "auto-rollback: pod unhealthy"}
		r.recordRolloutWarning(ar, eventReasonRolledBack, "auto-rollback: candidate pods unhealthy (progress deadline exceeded)")
		SetCondition(&ar.Status.Conditions, ar.Generation,
			ConditionTypeRolloutActive, metav1.ConditionFalse,
			"NoActiveRollout", "auto-rollback triggered: pod unhealthy")
		if err := r.Status().Update(ctx, ar); err != nil {
			return ctrl.Result{}, fmt.Errorf("persist auto-rollback status: %w", err)
		}
		return ctrl.Result{}, nil
	}

	result := reconcileRolloutSteps(ar)
	log.V(1).Info("rollout step evaluated",
		"step", result.currentStep,
		"weight", result.desiredWeight,
		"promote", result.promote,
		"paused", result.paused,
		"message", result.message)

	// Apply traffic routing if configured. Skip on a promote step: promotion
	// owns its own routing (it holds traffic on the candidate until stable is
	// healthy), and the promote step carries desiredWeight 0, which would
	// otherwise momentarily slam traffic onto the not-yet-rolled stable.
	if ar.Spec.Rollout.TrafficRouting != nil {
		if !result.paused && !result.analysis && !result.promote {
			if err := r.applyTrafficRouting(ctx, ar, result.desiredWeight); err != nil {
				return ctrl.Result{}, err
			}
		}
		// Sticky session only applies to the external/istio reference form.
		if ar.Spec.Rollout.StickySession != nil && ar.Spec.Rollout.TrafficRouting.Istio != nil {
			if err := r.patchDestinationRuleConsistentHash(ctx, ar.Namespace,
				ar.Spec.Rollout.TrafficRouting.Istio, ar.Spec.Rollout.StickySession.HashOn); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if result.promote {
		if r.RolloutMetrics != nil {
			r.RolloutMetrics.Promotions.WithLabelValues(ar.Namespace, ar.Name).Inc()
		}
		return r.reconcileRolloutPromote(ctx, ar)
	}

	// Execute analysis step if pending.
	if result.analysis && result.analysisName != "" {
		return r.reconcileRolloutAnalysis(ctx, ar, result)
	}

	return r.reconcileRolloutUpdateStatus(ctx, ar, result)
}

// reconcileRolloutIdle cleans up candidate resources and clears rollout status.
func (r *AgentRuntimeReconciler) reconcileRolloutIdle(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Reset traffic routing if configured.
	if ar.Spec.Rollout != nil && ar.Spec.Rollout.TrafficRouting != nil {
		if err := r.resetTrafficRoutingForMode(ctx, ar); err != nil {
			log.Error(err, "failed to reset traffic routing on idle cleanup")
		}
		if ar.Spec.Rollout.TrafficRouting.Istio != nil {
			if err := r.patchDestinationRuleConsistentHash(ctx, ar.Namespace,
				ar.Spec.Rollout.TrafficRouting.Istio, ""); err != nil {
				log.Error(err, "failed to remove consistent hash on idle cleanup")
			}
		}
	}
	if r.RolloutMetrics != nil {
		r.RolloutMetrics.TrafficWeight.WithLabelValues(ar.Namespace, ar.Name, "stable").Set(100)
		r.RolloutMetrics.TrafficWeight.WithLabelValues(ar.Namespace, ar.Name, "canary").Set(0)
	}

	if err := r.deleteCandidateDeployment(ctx, ar); err != nil {
		return ctrl.Result{}, fmt.Errorf("delete candidate deployment: %w", err)
	}

	log.V(1).Info("rollout idle, cleaning up candidate", "agentRuntime", ar.Name)
	ar.Status.Rollout = nil
	SetCondition(&ar.Status.Conditions, ar.Generation,
		ConditionTypeRolloutActive, metav1.ConditionFalse,
		"NoActiveRollout", "no active rollout")

	return ctrl.Result{}, nil
}

// reconcileRolloutUpdateStatus updates the rollout status from the step result
// and advances to the next step for setWeight steps that are not paused.
func (r *AgentRuntimeReconciler) reconcileRolloutUpdateStatus(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
	result rolloutStepResult,
) (ctrl.Result, error) {
	stableVersion := ""
	if ar.Status.ActiveVersion != nil {
		stableVersion = *ar.Status.ActiveVersion
	}
	candidateVersion := resolveRolloutCandidateVersion(ar)

	step := result.currentStep

	// Preserve StepStartedAt across reconciles for the same step. Stamp a new
	// value only when entering a fresh step; otherwise an unrelated reconcile
	// would reset the pause clock and the pause would never elapse.
	prevStepStartedAt, prevStep := previousStepStamp(ar)
	stepStartedAt := stepStartedAtForStep(prevStepStartedAt, prevStep, step)

	// Preserve the previously-set CurrentWeight when this reconcile didn't
	// produce one (e.g. pause/analysis steps): the weight reflects the last
	// setWeight that took effect, not the current step. Pause/analysis must
	// not clobber it back to 0.
	weight := result.desiredWeight
	currentWeight := &weight
	if weight == 0 && ar.Status.Rollout != nil && ar.Status.Rollout.CurrentWeight != nil {
		currentWeight = ar.Status.Rollout.CurrentWeight
	}

	prevTraffic := snapshotTrafficStatus(ar)
	ar.Status.Rollout = &omniav1alpha1.RolloutStatus{
		Active:           true,
		CurrentStep:      &step,
		CurrentWeight:    currentWeight,
		StableVersion:    stableVersion,
		CandidateVersion: candidateVersion,
		StepStartedAt:    stepStartedAt,
		Message:          result.message,
	}
	carryTrafficStatus(ar, prevTraffic)

	SetCondition(&ar.Status.Conditions, ar.Generation,
		ConditionTypeRolloutActive, metav1.ConditionTrue,
		"RolloutInProgress", result.message)

	// For non-paused, non-analysis steps (setWeight, or pause whose duration
	// elapsed), advance to the next step. Stamp StepStartedAt for the new
	// step so the next pause can measure elapsed time correctly.
	if !result.paused && !result.analysis {
		next := step + 1
		ar.Status.Rollout.CurrentStep = &next
		nextStamp := metav1.Now().Format(time.RFC3339)
		ar.Status.Rollout.StepStartedAt = &nextStamp
		if r.RolloutMetrics != nil {
			r.RolloutMetrics.StepTransitions.WithLabelValues(ar.Namespace, ar.Name, "setWeight").Inc()
		}
		r.recordRolloutNormal(ar, eventReasonRolloutStep, result.message)
	}

	if result.requeueAfter > 0 {
		if err := r.Status().Update(ctx, ar); err != nil {
			return ctrl.Result{}, fmt.Errorf("persist rollout status before requeue: %w", err)
		}
		return ctrl.Result{RequeueAfter: result.requeueAfter}, nil
	}
	return ctrl.Result{}, nil
}

// trafficStatusSnapshot captures the two scalar traffic-routing fields from the
// current rollout status so a subsequent rebuild of RolloutStatus can carry them
// forward. setTrafficStatus (called via applyTrafficRouting) writes these onto
// ar.Status.Rollout; the post-apply status rebuild would otherwise drop them.
type trafficStatusSnapshot struct {
	mode     string
	enforced *bool
}

// snapshotTrafficStatus returns the traffic-routing scalars from the existing
// rollout status (zero values when no status exists).
func snapshotTrafficStatus(ar *omniav1alpha1.AgentRuntime) trafficStatusSnapshot {
	if ar.Status.Rollout == nil {
		return trafficStatusSnapshot{}
	}
	return trafficStatusSnapshot{
		mode:     ar.Status.Rollout.TrafficRoutingMode,
		enforced: ar.Status.Rollout.TrafficWeightEnforced,
	}
}

// carryTrafficStatus copies the snapshotted traffic-routing scalars onto the
// (freshly rebuilt) rollout status so applyTrafficRouting's results survive the
// rebuild. No-op when nothing was recorded.
func carryTrafficStatus(ar *omniav1alpha1.AgentRuntime, snap trafficStatusSnapshot) {
	if ar.Status.Rollout == nil {
		return
	}
	if snap.mode != "" {
		ar.Status.Rollout.TrafficRoutingMode = snap.mode
	}
	if snap.enforced != nil {
		ar.Status.Rollout.TrafficWeightEnforced = snap.enforced
	}
}

// previousStepStamp returns the previously-stamped (StepStartedAt, currentStep)
// from status. Both nil/zero when no prior rollout status exists.
func previousStepStamp(ar *omniav1alpha1.AgentRuntime) (*string, int32) {
	if ar.Status.Rollout == nil {
		return nil, 0
	}
	var prevStep int32
	if ar.Status.Rollout.CurrentStep != nil {
		prevStep = *ar.Status.Rollout.CurrentStep
	}
	return ar.Status.Rollout.StepStartedAt, prevStep
}

// stepStartedAtForStep keeps the existing stamp when we're still on the same
// step; produces a fresh RFC3339 stamp when the step changed (or the prior
// stamp was missing).
func stepStartedAtForStep(prevStamp *string, prevStep, currentStep int32) *string {
	if prevStamp != nil && prevStep == currentStep {
		return prevStamp
	}
	now := metav1.Now().Format(time.RFC3339)
	return &now
}

// reconcileRolloutAnalysis runs the analysis step and advances or rolls back
// based on the result.
func (r *AgentRuntimeReconciler) reconcileRolloutAnalysis(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
	result rolloutStepResult,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	step := ar.Spec.Rollout.Steps[result.currentStep]
	analysisOut, err := r.runAnalysis(ctx, ar.Namespace, step.Analysis)
	if err != nil {
		log.Error(err, "analysis execution error", "template", result.analysisName)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	r.recordAnalysisMetrics(ar, result.analysisName, analysisOut.passed)

	if analysisOut.passed {
		return r.handleAnalysisPass(ctx, ar, result)
	}

	log.Info("analysis failed", "template", result.analysisName, "message", analysisOut.message)

	if ar.Spec.Rollout.Rollback != nil && ar.Spec.Rollout.Rollback.Mode == omniav1alpha1.RollbackModeAutomatic {
		return r.handleAnalysisAutoRollback(ctx, ar, analysisOut.message)
	}

	return r.handleAnalysisManualPause(ctx, ar, result.currentStep, analysisOut.message)
}

// recordAnalysisMetrics records analysis run and step transition metrics.
func (r *AgentRuntimeReconciler) recordAnalysisMetrics(ar *omniav1alpha1.AgentRuntime, templateName string, passed bool) {
	if r.RolloutMetrics == nil {
		return
	}
	outcome := "pass"
	if !passed {
		outcome = "fail"
	}
	r.RolloutMetrics.AnalysisRuns.WithLabelValues(ar.Namespace, ar.Name, templateName, outcome).Inc()
	r.RolloutMetrics.StepTransitions.WithLabelValues(ar.Namespace, ar.Name, "analysis").Inc()
}

// handleAnalysisPass advances to the next rollout step after a passing analysis.
func (r *AgentRuntimeReconciler) handleAnalysisPass(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
	result rolloutStepResult,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("analysis passed, advancing step", "template", result.analysisName)

	nextStep := result.currentStep + 1
	prevTraffic := snapshotTrafficStatus(ar)
	ar.Status.Rollout = &omniav1alpha1.RolloutStatus{
		Active:      true,
		CurrentStep: &nextStep,
		Message:     fmt.Sprintf("analysis %s passed", result.analysisName),
	}
	carryTrafficStatus(ar, prevTraffic)
	r.recordRolloutNormal(ar, eventReasonAnalysisPassed, fmt.Sprintf("analysis %s passed", result.analysisName))
	if err := r.Status().Update(ctx, ar); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist analysis pass status: %w", err)
	}
	return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
}

// handleAnalysisAutoRollback triggers automatic rollback after a failed analysis.
func (r *AgentRuntimeReconciler) handleAnalysisAutoRollback(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
	failMessage string,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	rollback(ar)
	if err := r.Update(ctx, ar); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist analysis rollback spec: %w", err)
	}
	if err := r.deleteCandidateDeployment(ctx, ar); err != nil {
		return ctrl.Result{}, fmt.Errorf("delete candidate after analysis rollback: %w", err)
	}
	if ar.Spec.Rollout != nil && ar.Spec.Rollout.TrafficRouting != nil {
		if err := r.resetTrafficRoutingForMode(ctx, ar); err != nil {
			log.Error(err, "failed to reset traffic routing on analysis rollback")
		}
	}
	if r.RolloutMetrics != nil {
		r.RolloutMetrics.Rollbacks.WithLabelValues(ar.Namespace, ar.Name, "analysis_failed").Inc()
		r.RolloutMetrics.TrafficWeight.WithLabelValues(ar.Namespace, ar.Name, "stable").Set(100)
		r.RolloutMetrics.TrafficWeight.WithLabelValues(ar.Namespace, ar.Name, "canary").Set(0)
	}

	ar.Status.Rollout = &omniav1alpha1.RolloutStatus{Active: false, Message: "auto-rollback: " + failMessage}
	r.recordRolloutWarning(ar, eventReasonRolledBack, "auto-rollback: analysis failed: "+failMessage)
	SetCondition(&ar.Status.Conditions, ar.Generation,
		ConditionTypeRolloutActive, metav1.ConditionFalse,
		"NoActiveRollout", "auto-rollback triggered: analysis failed")
	if err := r.Status().Update(ctx, ar); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist analysis rollback status: %w", err)
	}
	return ctrl.Result{}, nil
}

// handleAnalysisManualPause updates status with the failure message and requeues
// for manual intervention when automatic rollback is not configured.
func (r *AgentRuntimeReconciler) handleAnalysisManualPause(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
	currentStep int32,
	failMessage string,
) (ctrl.Result, error) {
	prevTraffic := snapshotTrafficStatus(ar)
	ar.Status.Rollout = &omniav1alpha1.RolloutStatus{
		Active:      true,
		CurrentStep: &currentStep,
		Message:     "analysis failed: " + failMessage,
	}
	carryTrafficStatus(ar, prevTraffic)
	r.recordRolloutWarning(ar, eventReasonAnalysisFailed, "analysis failed (manual intervention required): "+failMessage)
	SetCondition(&ar.Status.Conditions, ar.Generation,
		ConditionTypeRolloutActive, metav1.ConditionTrue,
		"AnalysisFailed", "analysis failed: "+failMessage)
	if err := r.Status().Update(ctx, ar); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist analysis failure status: %w", err)
	}
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// resolveRolloutCandidateVersion returns the candidate prompt pack version for
// status reporting, derived from the candidate's PromptPack override when set
// (its pinned version, or its name when no version is pinned) and otherwise
// falling back to the stable version.
func resolveRolloutCandidateVersion(ar *omniav1alpha1.AgentRuntime) string {
	if ar.Spec.Rollout != nil && ar.Spec.Rollout.Candidate != nil && ar.Spec.Rollout.Candidate.PromptPackRef != nil {
		ref := ar.Spec.Rollout.Candidate.PromptPackRef
		if ref.Version != nil {
			return *ref.Version
		}
		if ref.Track != nil {
			return *ref.Track
		}
		return ref.Name
	}
	if ar.Spec.PromptPackRef.Version != nil {
		return *ar.Spec.PromptPackRef.Version
	}
	if ar.Spec.PromptPackRef.Track != nil {
		return *ar.Spec.PromptPackRef.Track
	}
	return ""
}

// rolloutStepResult describes the outcome of evaluating the current rollout step.
// It is a pure value — reconcileRolloutSteps reads state and returns a decision
// without modifying anything.
type rolloutStepResult struct {
	active        bool
	currentStep   int32
	desiredWeight int32
	paused        bool
	promote       bool
	analysis      bool
	analysisName  string
	message       string
	requeueAfter  time.Duration
}

// isRolloutActive returns true when the candidate overrides differ from the
// current spec. A nil rollout, nil candidate, empty candidate (no overrides),
// or candidate matching the current spec all return false.
func isRolloutActive(ar *omniav1alpha1.AgentRuntime) bool {
	if ar.Spec.Rollout == nil || ar.Spec.Rollout.Candidate == nil {
		return false
	}
	return candidateDiffers(ar)
}

// candidateDiffers compares candidate overrides against the current spec.
func candidateDiffers(ar *omniav1alpha1.AgentRuntime) bool {
	c := ar.Spec.Rollout.Candidate

	if promptPackRefDiffers(c, ar) {
		return true
	}
	if providerRefsDiffer(c, ar) {
		return true
	}
	if toolRegistryRefDiffers(c, ar) {
		return true
	}
	return false
}

// promptPackRefDiffers checks if the candidate overrides the prompt pack to a
// different name, track, or (semver-equivalent) version than stable. A
// candidate that changes only the release Track — e.g. stable to prerelease,
// with no explicit version pin on either side — must still be detected as an
// active rollout (#1837).
func promptPackRefDiffers(c *omniav1alpha1.CandidateOverrides, ar *omniav1alpha1.AgentRuntime) bool {
	if c.PromptPackRef == nil {
		return false
	}
	if c.PromptPackRef.Name != ar.Spec.PromptPackRef.Name {
		return true
	}
	if derefStr(c.PromptPackRef.Track) != derefStr(ar.Spec.PromptPackRef.Track) {
		return true
	}
	return !versionsEqual(derefStr(c.PromptPackRef.Version), derefStr(ar.Spec.PromptPackRef.Version))
}

// derefStr returns the pointed-to string, or "" for a nil pointer.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// versionsEqual reports whether two version strings are semantically equal.
// Both are parsed with parsePackVersion (strict major.minor.patch, tolerating
// an optional leading "v" per the PromptPack CRD's spec.version pattern, and
// ignoring build metadata per semver semantics); if either fails to parse, it
// falls back to raw string equality — defensive, since these values aren't
// guaranteed to be strict semver at this layer (e.g. free-form placeholder
// version strings). Deliberately strict rather than semver.NewVersion's
// lenient/coercing parse: the latter treats a bare "v1" as equivalent to
// "1.0.0", which would silently conflate distinct informal version labels
// used elsewhere in this codebase.
func versionsEqual(a, b string) bool {
	if a == b {
		return true
	}
	av, aErr := parsePackVersion(a)
	bv, bErr := parsePackVersion(b)
	if aErr != nil || bErr != nil {
		return false
	}
	return av.Equal(bv)
}

// providerRefsDiffer checks if the candidate overrides the provider refs.
func providerRefsDiffer(c *omniav1alpha1.CandidateOverrides, ar *omniav1alpha1.AgentRuntime) bool {
	if len(c.ProviderRefs) == 0 {
		return false
	}
	return !namedProviderRefsEqual(c.ProviderRefs, ar.Spec.Providers)
}

// toolRegistryRefDiffers checks if the candidate overrides the tool registry ref.
func toolRegistryRefDiffers(c *omniav1alpha1.CandidateOverrides, ar *omniav1alpha1.AgentRuntime) bool {
	if c.ToolRegistryRef == nil {
		return false
	}
	if ar.Spec.ToolRegistryRef == nil {
		return true
	}
	return c.ToolRegistryRef.Name != ar.Spec.ToolRegistryRef.Name
}

// namedProviderRefsEqual compares two NamedProviderRef slices for equality.
func namedProviderRefsEqual(a, b []omniav1alpha1.NamedProviderRef) bool {
	if len(a) != len(b) {
		return false
	}
	bMap := make(map[string]omniav1alpha1.ProviderRef, len(b))
	for _, ref := range b {
		bMap[ref.Name] = ref.ProviderRef
	}
	for _, ref := range a {
		bRef, ok := bMap[ref.Name]
		if !ok {
			return false
		}
		if ref.ProviderRef.Name != bRef.Name {
			return false
		}
	}
	return true
}

// reconcileRolloutSteps evaluates rollout state and determines the next action.
// Pure function — reads state, returns decision, does not modify anything.
func reconcileRolloutSteps(ar *omniav1alpha1.AgentRuntime) rolloutStepResult {
	if !isRolloutActive(ar) {
		return rolloutStepResult{active: false, message: "idle"}
	}

	steps := ar.Spec.Rollout.Steps
	stepIdx := currentStepIndex(ar)

	// Past last step — promote.
	if int(stepIdx) >= len(steps) {
		return rolloutStepResult{
			active:      true,
			currentStep: stepIdx,
			promote:     true,
			message:     "all steps completed, promoting",
		}
	}

	step := steps[stepIdx]
	return evaluateStep(step, stepIdx, stepStartedAtFor(ar, stepIdx))
}

// stepStartedAtFor returns the timestamp the controller stamped when it
// entered the current step, parsed from RolloutStatus.StepStartedAt. Returns
// nil when not set or the stamp is for a different step (currentStep was
// advanced since the timestamp was written).
func stepStartedAtFor(ar *omniav1alpha1.AgentRuntime, stepIdx int32) *metav1.Time {
	if ar.Status.Rollout == nil || ar.Status.Rollout.StepStartedAt == nil {
		return nil
	}
	if ar.Status.Rollout.CurrentStep == nil || *ar.Status.Rollout.CurrentStep != stepIdx {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *ar.Status.Rollout.StepStartedAt)
	if err != nil {
		return nil
	}
	return &metav1.Time{Time: t}
}

// currentStepIndex returns the current step index from status, defaulting to 0.
func currentStepIndex(ar *omniav1alpha1.AgentRuntime) int32 {
	if ar.Status.Rollout != nil && ar.Status.Rollout.CurrentStep != nil {
		return *ar.Status.Rollout.CurrentStep
	}
	return 0
}

// evaluateStep evaluates a single rollout step and returns the result.
// stepStartedAt is the timestamp at which the controller entered this step;
// pauses use it to know whether the configured duration has elapsed.
func evaluateStep(step omniav1alpha1.RolloutStep, stepIdx int32, stepStartedAt *metav1.Time) rolloutStepResult {
	switch {
	case step.SetWeight != nil:
		return evaluateSetWeight(step, stepIdx)
	case step.Pause != nil:
		return evaluatePause(step, stepIdx, stepStartedAt)
	case step.Analysis != nil:
		return evaluateAnalysis(step, stepIdx)
	default:
		return rolloutStepResult{
			active:      true,
			currentStep: stepIdx,
			message:     fmt.Sprintf("step %d: unknown step type", stepIdx),
		}
	}
}

func evaluateSetWeight(step omniav1alpha1.RolloutStep, stepIdx int32) rolloutStepResult {
	return rolloutStepResult{
		active:        true,
		currentStep:   stepIdx,
		desiredWeight: *step.SetWeight,
		message:       fmt.Sprintf("step %d: setWeight %d", stepIdx, *step.SetWeight),
	}
}

func evaluatePause(step omniav1alpha1.RolloutStep, stepIdx int32, stepStartedAt *metav1.Time) rolloutStepResult {
	if step.Pause.Duration == nil {
		return rolloutStepResult{
			active:      true,
			currentStep: stepIdx,
			paused:      true,
			message:     fmt.Sprintf("step %d: paused indefinitely", stepIdx),
		}
	}
	d, err := time.ParseDuration(*step.Pause.Duration)
	if err != nil {
		return rolloutStepResult{
			active:      true,
			currentStep: stepIdx,
			paused:      true,
			message:     fmt.Sprintf("step %d: invalid pause duration %q", stepIdx, *step.Pause.Duration),
		}
	}
	// First reconcile entering this pause has no timestamp recorded yet; treat
	// as still pausing. The reconciler stamps stepStartedAt + requeues, so the
	// next reconcile after `d` elapses will see it.
	if stepStartedAt == nil || time.Since(stepStartedAt.Time) < d {
		remaining := d
		if stepStartedAt != nil {
			remaining = d - time.Since(stepStartedAt.Time)
			if remaining < time.Second {
				remaining = time.Second
			}
		}
		return rolloutStepResult{
			active:       true,
			currentStep:  stepIdx,
			paused:       true,
			requeueAfter: remaining,
			message:      fmt.Sprintf("step %d: pause %s", stepIdx, d),
		}
	}
	// Pause duration elapsed — let the reconciler advance to the next step.
	return rolloutStepResult{
		active:      true,
		currentStep: stepIdx,
		message:     fmt.Sprintf("step %d: pause %s elapsed", stepIdx, d),
	}
}

func evaluateAnalysis(step omniav1alpha1.RolloutStep, stepIdx int32) rolloutStepResult {
	return rolloutStepResult{
		active:       true,
		currentStep:  stepIdx,
		analysis:     true,
		analysisName: step.Analysis.TemplateName,
		message:      fmt.Sprintf("step %d: analysis %s", stepIdx, step.Analysis.TemplateName),
	}
}

// promote copies candidate overrides into the main spec fields.
// After promotion, candidate matches spec so isRolloutActive returns false.
func promote(ar *omniav1alpha1.AgentRuntime) {
	if ar.Spec.Rollout == nil || ar.Spec.Rollout.Candidate == nil {
		return
	}
	c := ar.Spec.Rollout.Candidate

	if c.PromptPackRef != nil {
		ar.Spec.PromptPackRef = *c.PromptPackRef.DeepCopy()
	}
	if len(c.ProviderRefs) > 0 {
		ar.Spec.Providers = c.ProviderRefs
	}
	if c.ToolRegistryRef != nil {
		ar.Spec.ToolRegistryRef = c.ToolRegistryRef
	}
}

// shouldAutoRollback returns true when the candidate Deployment is unhealthy
// and automatic rollback is configured.
func shouldAutoRollback(ar *omniav1alpha1.AgentRuntime, candidateDeploy *appsv1.Deployment) bool {
	if ar.Spec.Rollout == nil || ar.Spec.Rollout.Rollback == nil {
		return false
	}
	if ar.Spec.Rollout.Rollback.Mode != omniav1alpha1.RollbackModeAutomatic {
		return false
	}
	if candidateDeploy == nil {
		return false
	}
	// Roll back only on a genuine rollout failure, not normal startup. A freshly
	// created candidate sits at ReadyReplicas==0 / UnavailableReplicas>0 for its
	// entire startup window; treating that as "unhealthy" rolled back every
	// candidate within seconds of creation — before it could become ready and
	// before any analysis step ran, making automatic rollback + analysis-gating
	// unusable. Kubernetes signals real failure via the Progressing condition
	// reason ProgressDeadlineExceeded.
	return candidateProgressDeadlineExceeded(candidateDeploy)
}

// candidateProgressDeadlineExceeded reports whether the Deployment has failed to
// roll out within its progress deadline (the kubectl "rollout failed" state).
func candidateProgressDeadlineExceeded(d *appsv1.Deployment) bool {
	for _, c := range d.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing &&
			c.Status == corev1.ConditionFalse &&
			c.Reason == reasonProgressDeadlineExceeded {
			return true
		}
	}
	return false
}

// rollback reverts candidate overrides to match current spec values.
// After rollback, isRolloutActive returns false.
func rollback(ar *omniav1alpha1.AgentRuntime) {
	if ar.Spec.Rollout == nil || ar.Spec.Rollout.Candidate == nil {
		return
	}
	c := ar.Spec.Rollout.Candidate

	// Revert prompt pack ref to spec.
	c.PromptPackRef = ar.Spec.PromptPackRef.DeepCopy()

	// Revert provider refs to spec.
	if len(ar.Spec.Providers) > 0 {
		c.ProviderRefs = make([]omniav1alpha1.NamedProviderRef, len(ar.Spec.Providers))
		copy(c.ProviderRefs, ar.Spec.Providers)
	} else {
		c.ProviderRefs = nil
	}

	// Revert tool registry ref to spec.
	if ar.Spec.ToolRegistryRef != nil {
		ref := *ar.Spec.ToolRegistryRef
		c.ToolRegistryRef = &ref
	} else {
		c.ToolRegistryRef = nil
	}
}
