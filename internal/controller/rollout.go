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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

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
		if ar.Spec.Rollout != nil && ar.Spec.Rollout.TrafficRouting != nil && ar.Spec.Rollout.TrafficRouting.Istio != nil {
			if err := r.resetTrafficRouting(ctx, ar.Namespace, ar.Spec.Rollout.TrafficRouting.Istio); err != nil {
				log.Error(err, "failed to reset traffic routing on auto-rollback")
			}
		}
		if r.RolloutMetrics != nil {
			r.RolloutMetrics.Rollbacks.WithLabelValues(ar.Namespace, ar.Name, "pod_unhealthy").Inc()
		}
		ar.Status.Rollout = &omniav1alpha1.RolloutStatus{Active: false, Message: "auto-rollback: pod unhealthy"}
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

	// Apply traffic routing if configured.
	if ar.Spec.Rollout.TrafficRouting != nil && ar.Spec.Rollout.TrafficRouting.Istio != nil {
		if !result.paused && !result.analysis {
			if err := r.patchVirtualServiceWeights(ctx, ar.Namespace,
				ar.Spec.Rollout.TrafficRouting.Istio, result.desiredWeight); err != nil {
				return ctrl.Result{}, err
			}
			if r.RolloutMetrics != nil {
				r.RolloutMetrics.TrafficWeight.WithLabelValues(ar.Namespace, ar.Name, "stable").Set(float64(100 - result.desiredWeight))
				r.RolloutMetrics.TrafficWeight.WithLabelValues(ar.Namespace, ar.Name, "canary").Set(float64(result.desiredWeight))
			}
		}
	}

	if result.promote {
		if r.RolloutMetrics != nil {
			r.RolloutMetrics.Promotions.WithLabelValues(ar.Namespace, ar.Name).Inc()
		}
		return r.reconcileRolloutPromote(ctx, ar)
	}

	return r.reconcileRolloutUpdateStatus(ctx, ar, result)
}

// reconcileRolloutIdle cleans up candidate resources and clears rollout status.
func (r *AgentRuntimeReconciler) reconcileRolloutIdle(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Reset traffic routing if Istio was configured.
	if ar.Spec.Rollout != nil && ar.Spec.Rollout.TrafficRouting != nil && ar.Spec.Rollout.TrafficRouting.Istio != nil {
		if err := r.resetTrafficRouting(ctx, ar.Namespace, ar.Spec.Rollout.TrafficRouting.Istio); err != nil {
			log.Error(err, "failed to reset traffic routing on idle cleanup")
		}
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

// reconcileRolloutPromote copies candidate overrides into the main spec,
// deletes the candidate Deployment, and marks the rollout inactive.
func (r *AgentRuntimeReconciler) reconcileRolloutPromote(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	promote(ar)

	// Reset traffic to 100% stable before removing the candidate.
	if ar.Spec.Rollout != nil && ar.Spec.Rollout.TrafficRouting != nil && ar.Spec.Rollout.TrafficRouting.Istio != nil {
		if err := r.resetTrafficRouting(ctx, ar.Namespace, ar.Spec.Rollout.TrafficRouting.Istio); err != nil {
			log.Error(err, "failed to reset traffic routing on promotion")
		}
	}

	// Persist the spec mutation (promote copies candidate overrides into main spec).
	// This must happen before status update since they are separate API calls.
	if err := r.Update(ctx, ar); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist promotion: %w", err)
	}

	if err := r.deleteCandidateDeployment(ctx, ar); err != nil {
		return ctrl.Result{}, fmt.Errorf("delete candidate after promotion: %w", err)
	}

	ar.Status.Rollout = &omniav1alpha1.RolloutStatus{Active: false, Message: "promoted"}
	SetCondition(&ar.Status.Conditions, ar.Generation,
		ConditionTypeRolloutActive, metav1.ConditionFalse,
		"NoActiveRollout", "rollout promoted successfully")

	if err := r.Status().Update(ctx, ar); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist promotion status: %w", err)
	}

	log.Info("rollout promoted", "agentRuntime", ar.Name)
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
	weight := result.desiredWeight

	ar.Status.Rollout = &omniav1alpha1.RolloutStatus{
		Active:           true,
		CurrentStep:      &step,
		CurrentWeight:    &weight,
		StableVersion:    stableVersion,
		CandidateVersion: candidateVersion,
		Message:          result.message,
	}

	SetCondition(&ar.Status.Conditions, ar.Generation,
		ConditionTypeRolloutActive, metav1.ConditionTrue,
		"RolloutInProgress", result.message)

	// For setWeight steps (not paused, not analysis), advance to next step.
	if !result.paused && !result.analysis {
		next := step + 1
		ar.Status.Rollout.CurrentStep = &next
		if r.RolloutMetrics != nil {
			r.RolloutMetrics.StepTransitions.WithLabelValues(ar.Namespace, ar.Name, "setWeight").Inc()
		}
	}

	if result.requeueAfter > 0 {
		if err := r.Status().Update(ctx, ar); err != nil {
			return ctrl.Result{}, fmt.Errorf("persist rollout status before requeue: %w", err)
		}
		return ctrl.Result{RequeueAfter: result.requeueAfter}, nil
	}
	return ctrl.Result{}, nil
}

// resolveRolloutCandidateVersion returns the candidate prompt pack version,
// falling back to the stable version when no override is set.
func resolveRolloutCandidateVersion(ar *omniav1alpha1.AgentRuntime) string {
	if ar.Spec.Rollout != nil && ar.Spec.Rollout.Candidate != nil && ar.Spec.Rollout.Candidate.PromptPackVersion != nil {
		return *ar.Spec.Rollout.Candidate.PromptPackVersion
	}
	if ar.Spec.PromptPackRef.Version != nil {
		return *ar.Spec.PromptPackRef.Version
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

	if promptPackVersionDiffers(c, ar) {
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

// promptPackVersionDiffers checks if the candidate overrides the prompt pack version.
func promptPackVersionDiffers(c *omniav1alpha1.CandidateOverrides, ar *omniav1alpha1.AgentRuntime) bool {
	if c.PromptPackVersion == nil {
		return false
	}
	specVersion := ""
	if ar.Spec.PromptPackRef.Version != nil {
		specVersion = *ar.Spec.PromptPackRef.Version
	}
	return *c.PromptPackVersion != specVersion
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
	return evaluateStep(step, stepIdx)
}

// currentStepIndex returns the current step index from status, defaulting to 0.
func currentStepIndex(ar *omniav1alpha1.AgentRuntime) int32 {
	if ar.Status.Rollout != nil && ar.Status.Rollout.CurrentStep != nil {
		return *ar.Status.Rollout.CurrentStep
	}
	return 0
}

// evaluateStep evaluates a single rollout step and returns the result.
func evaluateStep(step omniav1alpha1.RolloutStep, stepIdx int32) rolloutStepResult {
	switch {
	case step.SetWeight != nil:
		return evaluateSetWeight(step, stepIdx)
	case step.Pause != nil:
		return evaluatePause(step, stepIdx)
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

func evaluatePause(step omniav1alpha1.RolloutStep, stepIdx int32) rolloutStepResult {
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
	return rolloutStepResult{
		active:       true,
		currentStep:  stepIdx,
		requeueAfter: d,
		message:      fmt.Sprintf("step %d: pause %s", stepIdx, d),
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

	if c.PromptPackVersion != nil {
		ar.Spec.PromptPackRef.Version = c.PromptPackVersion
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
	return candidateDeploy.Status.UnavailableReplicas > 0 && candidateDeploy.Status.ReadyReplicas == 0
}

// rollback reverts candidate overrides to match current spec values.
// After rollback, isRolloutActive returns false.
func rollback(ar *omniav1alpha1.AgentRuntime) {
	if ar.Spec.Rollout == nil || ar.Spec.Rollout.Candidate == nil {
		return
	}
	c := ar.Spec.Rollout.Candidate

	// Revert prompt pack version to spec.
	if ar.Spec.PromptPackRef.Version != nil {
		v := *ar.Spec.PromptPackRef.Version
		c.PromptPackVersion = &v
	} else {
		c.PromptPackVersion = nil
	}

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
