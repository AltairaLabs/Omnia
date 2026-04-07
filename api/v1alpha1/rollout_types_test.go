/*
Copyright 2025.

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

package v1alpha1

import (
	"testing"
)

func int32Ptr(v int32) *int32    { return &v }
func stringPtr(v string) *string { return &v }

func TestRolloutStepSetWeight(t *testing.T) {
	step := RolloutStep{
		SetWeight: int32Ptr(20),
	}

	if step.SetWeight == nil {
		t.Fatal("SetWeight should not be nil")
	}
	if *step.SetWeight != 20 {
		t.Errorf("SetWeight = %d, want 20", *step.SetWeight)
	}
	if step.Pause != nil {
		t.Error("Pause should be nil when SetWeight is set")
	}
	if step.Analysis != nil {
		t.Error("Analysis should be nil when SetWeight is set")
	}
}

func TestRolloutStepPauseWithDuration(t *testing.T) {
	step := RolloutStep{
		Pause: &RolloutPause{
			Duration: stringPtr("10m"),
		},
	}

	if step.Pause == nil {
		t.Fatal("Pause should not be nil")
	}
	if step.Pause.Duration == nil {
		t.Fatal("Pause.Duration should not be nil")
	}
	if *step.Pause.Duration != "10m" {
		t.Errorf("Pause.Duration = %q, want %q", *step.Pause.Duration, "10m")
	}
}

func TestRolloutStepPauseIndefinite(t *testing.T) {
	step := RolloutStep{
		Pause: &RolloutPause{},
	}

	if step.Pause == nil {
		t.Fatal("Pause should not be nil")
	}
	if step.Pause.Duration != nil {
		t.Errorf("Pause.Duration should be nil for indefinite pause, got %q", *step.Pause.Duration)
	}
}

func TestRolloutStepAnalysis(t *testing.T) {
	step := RolloutStep{
		Analysis: &RolloutAnalysisStep{
			TemplateName: "error-rate-check",
			Args: []AnalysisArg{
				{Name: "threshold", Value: "0.01"},
			},
		},
	}

	if step.Analysis == nil {
		t.Fatal("Analysis should not be nil")
	}
	if step.Analysis.TemplateName != "error-rate-check" {
		t.Errorf("TemplateName = %q, want %q", step.Analysis.TemplateName, "error-rate-check")
	}
	if len(step.Analysis.Args) != 1 {
		t.Fatalf("len(Args) = %d, want 1", len(step.Analysis.Args))
	}
	if step.Analysis.Args[0].Name != "threshold" {
		t.Errorf("Args[0].Name = %q, want %q", step.Analysis.Args[0].Name, "threshold")
	}
	if step.Analysis.Args[0].Value != "0.01" {
		t.Errorf("Args[0].Value = %q, want %q", step.Analysis.Args[0].Value, "0.01")
	}
}

func TestCandidateOverridesEmpty(t *testing.T) {
	c := CandidateOverrides{}

	if c.PromptPackVersion != nil {
		t.Error("PromptPackVersion should be nil for empty CandidateOverrides")
	}
	if c.ProviderRefs != nil {
		t.Error("ProviderRefs should be nil for empty CandidateOverrides")
	}
	if c.ToolRegistryRef != nil {
		t.Error("ToolRegistryRef should be nil for empty CandidateOverrides")
	}
}

func TestCandidateOverridesWithPromptPackVersion(t *testing.T) {
	c := CandidateOverrides{
		PromptPackVersion: stringPtr("v2"),
	}

	if c.PromptPackVersion == nil {
		t.Fatal("PromptPackVersion should not be nil")
	}
	if *c.PromptPackVersion != "v2" {
		t.Errorf("PromptPackVersion = %q, want %q", *c.PromptPackVersion, "v2")
	}
}

func TestRollbackModeEnumValues(t *testing.T) {
	tests := []struct {
		name     string
		mode     RollbackMode
		expected string
	}{
		{"automatic", RollbackModeAutomatic, "automatic"},
		{"manual", RollbackModeManual, "manual"},
		{"disabled", RollbackModeDisabled, "disabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.mode) != tt.expected {
				t.Errorf("RollbackMode = %q, want %q", tt.mode, tt.expected)
			}
		})
	}
}

func TestIstioTrafficRoutingConfig(t *testing.T) {
	cfg := TrafficRoutingConfig{
		Istio: &IstioTrafficRouting{
			VirtualService: IstioVirtualServiceRef{
				Name:   "my-vs",
				Routes: []string{"primary"},
			},
			DestinationRule: IstioDestinationRuleRef{
				Name:            "my-dr",
				StableSubset:    "stable",
				CandidateSubset: "canary",
			},
		},
	}

	if cfg.Istio == nil {
		t.Fatal("Istio should not be nil")
	}
	if cfg.Istio.VirtualService.Name != "my-vs" {
		t.Errorf("VirtualService.Name = %q, want %q", cfg.Istio.VirtualService.Name, "my-vs")
	}
	if len(cfg.Istio.VirtualService.Routes) != 1 || cfg.Istio.VirtualService.Routes[0] != "primary" {
		t.Errorf("VirtualService.Routes = %v, want [primary]", cfg.Istio.VirtualService.Routes)
	}
	if cfg.Istio.DestinationRule.StableSubset != "stable" {
		t.Errorf("DestinationRule.StableSubset = %q, want %q", cfg.Istio.DestinationRule.StableSubset, "stable")
	}
	if cfg.Istio.DestinationRule.CandidateSubset != "canary" {
		t.Errorf("DestinationRule.CandidateSubset = %q, want %q", cfg.Istio.DestinationRule.CandidateSubset, "canary")
	}
}

func TestRolloutConfigIdleState(t *testing.T) {
	// When Candidate is nil, no rollout is active.
	cfg := RolloutConfig{
		Steps: []RolloutStep{
			{SetWeight: int32Ptr(20)},
			{Pause: &RolloutPause{Duration: stringPtr("5m")}},
			{SetWeight: int32Ptr(100)},
		},
	}

	if cfg.Candidate != nil {
		t.Error("Candidate should be nil for idle state")
	}
	if len(cfg.Steps) != 3 {
		t.Errorf("len(Steps) = %d, want 3", len(cfg.Steps))
	}
}

func TestRolloutConfigFullSpec(t *testing.T) {
	cfg := RolloutConfig{
		Candidate: &CandidateOverrides{
			PromptPackVersion: stringPtr("v2"),
		},
		Steps: []RolloutStep{
			{SetWeight: int32Ptr(10)},
			{Pause: &RolloutPause{Duration: stringPtr("10m")}},
			{Analysis: &RolloutAnalysisStep{TemplateName: "latency-check"}},
			{SetWeight: int32Ptr(100)},
		},
		StickySession: &StickySessionConfig{HashOn: "x-user-id"},
		Rollback: &RollbackConfig{
			Mode:     RollbackModeAutomatic,
			Cooldown: stringPtr("5m"),
		},
		TrafficRouting: &TrafficRoutingConfig{
			Istio: &IstioTrafficRouting{
				VirtualService: IstioVirtualServiceRef{
					Name:   "agent-vs",
					Routes: []string{"http"},
				},
				DestinationRule: IstioDestinationRuleRef{
					Name:            "agent-dr",
					StableSubset:    "stable",
					CandidateSubset: "canary",
				},
			},
		},
	}

	if cfg.Candidate == nil {
		t.Fatal("Candidate should not be nil")
	}
	if *cfg.Candidate.PromptPackVersion != "v2" {
		t.Errorf("Candidate.PromptPackVersion = %q, want %q", *cfg.Candidate.PromptPackVersion, "v2")
	}
	if len(cfg.Steps) != 4 {
		t.Errorf("len(Steps) = %d, want 4", len(cfg.Steps))
	}
	if cfg.StickySession == nil || cfg.StickySession.HashOn != "x-user-id" {
		t.Errorf("StickySession.HashOn = %q, want %q", cfg.StickySession.HashOn, "x-user-id")
	}
	if cfg.Rollback == nil || cfg.Rollback.Mode != RollbackModeAutomatic {
		t.Errorf("Rollback.Mode = %q, want %q", cfg.Rollback.Mode, RollbackModeAutomatic)
	}
	if cfg.TrafficRouting == nil || cfg.TrafficRouting.Istio == nil {
		t.Error("TrafficRouting.Istio should not be nil")
	}
}

func TestAgentRuntimeSpecRolloutField(t *testing.T) {
	// Verify the Rollout field exists on AgentRuntimeSpec and is optional (pointer).
	spec := AgentRuntimeSpec{}
	if spec.Rollout != nil {
		t.Error("Rollout should be nil by default")
	}

	spec.Rollout = &RolloutConfig{
		Candidate: &CandidateOverrides{
			PromptPackVersion: stringPtr("v2"),
		},
		Steps: []RolloutStep{{SetWeight: int32Ptr(100)}},
	}
	if spec.Rollout == nil {
		t.Error("Rollout should not be nil after assignment")
	}
}

func TestAgentRuntimeStatusRolloutField(t *testing.T) {
	// Verify the Rollout field exists on AgentRuntimeStatus and is optional (pointer).
	status := AgentRuntimeStatus{}
	if status.Rollout != nil {
		t.Error("Rollout should be nil by default")
	}

	status.Rollout = &RolloutStatus{
		Active:           true,
		CurrentStep:      int32Ptr(1),
		CurrentWeight:    int32Ptr(20),
		StableVersion:    "v1",
		CandidateVersion: "v2",
	}
	if status.Rollout == nil {
		t.Fatal("Rollout should not be nil after assignment")
	}
	if !status.Rollout.Active {
		t.Error("Rollout.Active should be true")
	}
	if *status.Rollout.CurrentWeight != 20 {
		t.Errorf("Rollout.CurrentWeight = %d, want 20", *status.Rollout.CurrentWeight)
	}
}
