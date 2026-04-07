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
	"fmt"
	"time"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

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
