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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func newRolloutTestAR() *omniav1alpha1.AgentRuntime {
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "customer-support"
	ar.Namespace = "default"
	ar.Spec.PromptPackRef = omniav1alpha1.PromptPackRef{
		Name:    "support-pack",
		Version: ptr.To("v1"),
	}
	ar.Spec.Facade.Type = omniav1alpha1.FacadeTypeWebSocket
	ar.Spec.Providers = []omniav1alpha1.NamedProviderRef{
		{
			Name:        "default",
			ProviderRef: omniav1alpha1.ProviderRef{Name: "claude-provider"},
		},
	}
	ar.Spec.ToolRegistryRef = &omniav1alpha1.ToolRegistryRef{Name: "tools-v1"}
	return ar
}

// --- isRolloutActive tests ---

func TestIsRolloutActive_NilRollout(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = nil
	assert.False(t, isRolloutActive(ar))
}

func TestIsRolloutActive_NilCandidate(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: nil,
		Steps:     []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}
	assert.False(t, isRolloutActive(ar))
}

func TestIsRolloutActive_EmptyCandidate(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{},
		Steps:     []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}
	assert.False(t, isRolloutActive(ar))
}

func TestIsRolloutActive_CandidateMatchesSpec(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v1"),
			ProviderRefs: []omniav1alpha1.NamedProviderRef{
				{
					Name:        "default",
					ProviderRef: omniav1alpha1.ProviderRef{Name: "claude-provider"},
				},
			},
			ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{Name: "tools-v1"},
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}
	assert.False(t, isRolloutActive(ar))
}

func TestIsRolloutActive_CandidateDiffersPromptPackVersion(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}
	assert.True(t, isRolloutActive(ar))
}

func TestIsRolloutActive_CandidateDiffersProviderRefs(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			ProviderRefs: []omniav1alpha1.NamedProviderRef{
				{
					Name:        "default",
					ProviderRef: omniav1alpha1.ProviderRef{Name: "openai-provider"},
				},
			},
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}
	assert.True(t, isRolloutActive(ar))
}

func TestIsRolloutActive_CandidateDiffersToolRegistry(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{Name: "tools-v2"},
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}
	assert.True(t, isRolloutActive(ar))
}

// --- namedProviderRefsEqual tests ---

func TestNamedProviderRefsEqual_Both_Empty(t *testing.T) {
	assert.True(t, namedProviderRefsEqual(nil, nil))
}

func TestNamedProviderRefsEqual_DifferentLength(t *testing.T) {
	a := []omniav1alpha1.NamedProviderRef{
		{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "p1"}},
	}
	assert.False(t, namedProviderRefsEqual(a, nil))
}

func TestNamedProviderRefsEqual_DifferentProviderName(t *testing.T) {
	a := []omniav1alpha1.NamedProviderRef{
		{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "p1"}},
	}
	b := []omniav1alpha1.NamedProviderRef{
		{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "p2"}},
	}
	assert.False(t, namedProviderRefsEqual(a, b))
}

func TestNamedProviderRefsEqual_MissingName(t *testing.T) {
	a := []omniav1alpha1.NamedProviderRef{
		{Name: "judge", ProviderRef: omniav1alpha1.ProviderRef{Name: "p1"}},
	}
	b := []omniav1alpha1.NamedProviderRef{
		{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "p1"}},
	}
	assert.False(t, namedProviderRefsEqual(a, b))
}

// --- reconcileRolloutSteps tests ---

func TestReconcileRollout_Idle(t *testing.T) {
	ar := newRolloutTestAR()
	result := reconcileRolloutSteps(ar)
	assert.False(t, result.active)
	assert.Equal(t, "idle", result.message)
}

func TestReconcileRollout_FirstStep_SetWeight(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{SetWeight: ptr.To[int32](10)},
			{SetWeight: ptr.To[int32](50)},
		},
	}

	result := reconcileRolloutSteps(ar)
	assert.True(t, result.active)
	assert.Equal(t, int32(0), result.currentStep)
	assert.Equal(t, int32(10), result.desiredWeight)
}

func TestReconcileRollout_FinalStep_Promote(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{SetWeight: ptr.To[int32](100)},
		},
	}
	// Step index past the last step
	step := int32(1)
	ar.Status.Rollout = &omniav1alpha1.RolloutStatus{
		Active:      true,
		CurrentStep: &step,
	}

	result := reconcileRolloutSteps(ar)
	assert.True(t, result.active)
	assert.True(t, result.promote)
}

func TestReconcileRollout_Pause_Indefinite(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{Pause: &omniav1alpha1.RolloutPause{}},
		},
	}

	result := reconcileRolloutSteps(ar)
	assert.True(t, result.active)
	assert.True(t, result.paused)
	assert.Equal(t, time.Duration(0), result.requeueAfter)
}

func TestReconcileRollout_Pause_WithDuration(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{Pause: &omniav1alpha1.RolloutPause{Duration: ptr.To("5m")}},
		},
	}

	result := reconcileRolloutSteps(ar)
	assert.True(t, result.active)
	// A pause-with-duration that just started must be paused so the reconciler
	// doesn't auto-advance currentStep before the duration elapses.
	assert.True(t, result.paused)
	assert.Equal(t, 5*time.Minute, result.requeueAfter)
}

func TestReconcileRollout_Analysis(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{Analysis: &omniav1alpha1.RolloutAnalysisStep{TemplateName: "latency-check"}},
		},
	}

	result := reconcileRolloutSteps(ar)
	assert.True(t, result.active)
	assert.True(t, result.analysis)
	assert.Equal(t, "latency-check", result.analysisName)
}

func TestReconcileRollout_SecondStep(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{SetWeight: ptr.To[int32](10)},
			{SetWeight: ptr.To[int32](50)},
			{SetWeight: ptr.To[int32](100)},
		},
	}
	step := int32(1)
	ar.Status.Rollout = &omniav1alpha1.RolloutStatus{
		Active:      true,
		CurrentStep: &step,
	}

	result := reconcileRolloutSteps(ar)
	assert.True(t, result.active)
	assert.Equal(t, int32(1), result.currentStep)
	assert.Equal(t, int32(50), result.desiredWeight)
}

// --- promote tests ---

func TestPromote_CopiesCandidateToSpec(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
			ProviderRefs: []omniav1alpha1.NamedProviderRef{
				{
					Name:        "default",
					ProviderRef: omniav1alpha1.ProviderRef{Name: "openai-provider"},
				},
			},
			ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{Name: "tools-v2"},
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](100)}},
	}

	promote(ar)

	assert.Equal(t, "v2", *ar.Spec.PromptPackRef.Version)
	assert.Equal(t, "openai-provider", ar.Spec.Providers[0].ProviderRef.Name)
	assert.Equal(t, "tools-v2", ar.Spec.ToolRegistryRef.Name)
	// After promotion, candidate matches spec so rollout is idle.
	assert.False(t, isRolloutActive(ar))
}

func TestPromote_NilRollout(t *testing.T) {
	ar := newRolloutTestAR()
	// Should not panic.
	promote(ar)
	assert.Equal(t, "v1", *ar.Spec.PromptPackRef.Version)
}

func TestPromote_PartialOverrides(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v3"),
			// No provider or tool registry overrides.
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](100)}},
	}

	promote(ar)

	assert.Equal(t, "v3", *ar.Spec.PromptPackRef.Version)
	// Provider unchanged.
	assert.Equal(t, "claude-provider", ar.Spec.Providers[0].ProviderRef.Name)
}

// --- rollback tests ---

func TestRollback_RevertsCandidateToSpec(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
			ProviderRefs: []omniav1alpha1.NamedProviderRef{
				{
					Name:        "default",
					ProviderRef: omniav1alpha1.ProviderRef{Name: "openai-provider"},
				},
			},
			ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{Name: "tools-v2"},
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](100)}},
	}

	rollback(ar)

	c := ar.Spec.Rollout.Candidate
	assert.Equal(t, "v1", *c.PromptPackVersion)
	assert.Equal(t, "claude-provider", c.ProviderRefs[0].ProviderRef.Name)
	assert.Equal(t, "tools-v1", c.ToolRegistryRef.Name)
	// After rollback, candidate matches spec so rollout is idle.
	assert.False(t, isRolloutActive(ar))
}

func TestRollback_NilRollout(t *testing.T) {
	ar := newRolloutTestAR()
	// Should not panic.
	rollback(ar)
}

// --- shouldAutoRollback tests ---

func newAutoRollbackAR() *omniav1alpha1.AgentRuntime {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
		Rollback: &omniav1alpha1.RollbackConfig{
			Mode: omniav1alpha1.RollbackModeAutomatic,
		},
	}
	return ar
}

func unhealthyDeploy() *appsv1.Deployment {
	d := &appsv1.Deployment{}
	d.Status.ReadyReplicas = 0
	d.Status.UnavailableReplicas = 1
	return d
}

func TestShouldAutoRollback_HealthyCandidate(t *testing.T) {
	ar := newAutoRollbackAR()
	d := &appsv1.Deployment{}
	d.Status.ReadyReplicas = 1
	d.Status.UnavailableReplicas = 0
	assert.False(t, shouldAutoRollback(ar, d))
}

func TestShouldAutoRollback_UnhealthyCandidate_AutomaticMode(t *testing.T) {
	ar := newAutoRollbackAR()
	assert.True(t, shouldAutoRollback(ar, unhealthyDeploy()))
}

func TestShouldAutoRollback_UnhealthyCandidate_ManualMode(t *testing.T) {
	ar := newAutoRollbackAR()
	ar.Spec.Rollout.Rollback.Mode = omniav1alpha1.RollbackModeManual
	assert.False(t, shouldAutoRollback(ar, unhealthyDeploy()))
}

func TestShouldAutoRollback_UnhealthyCandidate_DisabledMode(t *testing.T) {
	ar := newAutoRollbackAR()
	ar.Spec.Rollout.Rollback.Mode = omniav1alpha1.RollbackModeDisabled
	assert.False(t, shouldAutoRollback(ar, unhealthyDeploy()))
}

func TestShouldAutoRollback_NilRollbackConfig(t *testing.T) {
	ar := newAutoRollbackAR()
	ar.Spec.Rollout.Rollback = nil
	assert.False(t, shouldAutoRollback(ar, unhealthyDeploy()))
}

func TestShouldAutoRollback_NilCandidateDeploy(t *testing.T) {
	ar := newAutoRollbackAR()
	assert.False(t, shouldAutoRollback(ar, nil))
}

func TestRollback_NilSpecVersion(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.PromptPackRef.Version = nil
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](100)}},
	}

	rollback(ar)

	assert.Nil(t, ar.Spec.Rollout.Candidate.PromptPackVersion)
	assert.False(t, isRolloutActive(ar))
}

func TestRollback_NilProviders(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Providers = nil
	ar.Spec.ToolRegistryRef = nil
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
			ProviderRefs: []omniav1alpha1.NamedProviderRef{
				{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "openai"}},
			},
			ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{Name: "tools-v2"},
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](100)}},
	}

	rollback(ar)

	c := ar.Spec.Rollout.Candidate
	assert.Nil(t, c.ProviderRefs)
	assert.Nil(t, c.ToolRegistryRef)
	assert.False(t, isRolloutActive(ar))
}

func TestRollback_NilCandidate(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: nil,
	}
	// Should not panic.
	rollback(ar)
}

// --- resolveRolloutCandidateVersion tests ---

func TestResolveRolloutCandidateVersion_FromCandidate(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v3"),
		},
	}
	assert.Equal(t, "v3", resolveRolloutCandidateVersion(ar))
}

func TestResolveRolloutCandidateVersion_FallbackToSpec(t *testing.T) {
	ar := newRolloutTestAR()
	// No rollout candidate.
	assert.Equal(t, "v1", resolveRolloutCandidateVersion(ar))
}

func TestResolveRolloutCandidateVersion_NoVersionAnywhere(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.PromptPackRef.Version = nil
	assert.Equal(t, "", resolveRolloutCandidateVersion(ar))
}

// --- evaluateStep tests ---

func TestEvaluateStep_UnknownStepType(t *testing.T) {
	step := omniav1alpha1.RolloutStep{} // No SetWeight, Pause, or Analysis
	result := evaluateStep(step, 3, nil)
	assert.True(t, result.active)
	assert.Equal(t, int32(3), result.currentStep)
	assert.Contains(t, result.message, "unknown step type")
}

func TestEvaluatePause_InvalidDuration(t *testing.T) {
	step := omniav1alpha1.RolloutStep{
		Pause: &omniav1alpha1.RolloutPause{Duration: ptr.To("not-a-duration")},
	}
	result := evaluateStep(step, 0, nil)
	assert.True(t, result.active)
	assert.True(t, result.paused)
	assert.Contains(t, result.message, "invalid pause duration")
}

func TestEvaluatePause_DurationStillRunning(t *testing.T) {
	step := omniav1alpha1.RolloutStep{
		Pause: &omniav1alpha1.RolloutPause{Duration: ptr.To("10m")},
	}
	startedAt := metav1.NewTime(time.Now().Add(-5 * time.Minute))
	result := evaluateStep(step, 1, &startedAt)
	assert.True(t, result.paused, "pause within duration should keep paused=true")
	assert.True(t, result.requeueAfter > 0)
	assert.True(t, result.requeueAfter <= 5*time.Minute+time.Second,
		"requeueAfter should be the remaining pause duration, got %v", result.requeueAfter)
}

func TestEvaluatePause_DurationElapsed(t *testing.T) {
	step := omniav1alpha1.RolloutStep{
		Pause: &omniav1alpha1.RolloutPause{Duration: ptr.To("1s")},
	}
	startedAt := metav1.NewTime(time.Now().Add(-time.Minute))
	result := evaluateStep(step, 1, &startedAt)
	assert.False(t, result.paused, "elapsed pause should release for advancement")
	assert.Contains(t, result.message, "elapsed")
}

func TestEvaluatePause_FirstReconcileWithoutStamp(t *testing.T) {
	step := omniav1alpha1.RolloutStep{
		Pause: &omniav1alpha1.RolloutPause{Duration: ptr.To("10m")},
	}
	result := evaluateStep(step, 1, nil)
	assert.True(t, result.paused, "first reconcile entering pause should be paused=true")
	assert.Equal(t, 10*time.Minute, result.requeueAfter)
}

// --- candidateDiffers edge cases ---

func TestCandidateDiffers_ToolRegistryRef_NilSpec(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.ToolRegistryRef = nil
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{Name: "tools-v2"},
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}
	assert.True(t, candidateDiffers(ar))
}

func TestCandidateDiffers_ToolRegistryRef_SameName(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{Name: "tools-v1"},
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}
	assert.False(t, candidateDiffers(ar))
}

func TestPromptPackVersionDiffers_NilSpecVersion(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.PromptPackRef.Version = nil
	c := &omniav1alpha1.CandidateOverrides{
		PromptPackVersion: ptr.To("v2"),
	}
	assert.True(t, promptPackVersionDiffers(c, ar))
}

func TestPromptPackVersionDiffers_NilCandidateVersion(t *testing.T) {
	ar := newRolloutTestAR()
	c := &omniav1alpha1.CandidateOverrides{}
	assert.False(t, promptPackVersionDiffers(c, ar))
}
