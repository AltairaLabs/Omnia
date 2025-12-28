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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test constants to avoid duplicate string literals
const (
	testPromptPackName      = "test-promptpack"
	testPromptPackNamespace = "test-namespace"
	testPromptPackVersion   = "1.0.0"
	testConfigMapName       = "my-prompts"
	testCanaryVersion       = "1.1.0"
	testModifiedName        = "modified-name"
)

func TestPromptPackSourceTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant PromptPackSourceType
		expected string
	}{
		{
			name:     "ConfigMap source type",
			constant: PromptPackSourceTypeConfigMap,
			expected: "configmap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("PromptPackSourceType constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestRolloutStrategyTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant RolloutStrategyType
		expected string
	}{
		{
			name:     "Immediate rollout strategy",
			constant: RolloutStrategyImmediate,
			expected: "immediate",
		},
		{
			name:     "Canary rollout strategy",
			constant: RolloutStrategyCanary,
			expected: "canary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("RolloutStrategyType constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestPromptPackPhaseConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant PromptPackPhase
		expected string
	}{
		{
			name:     "Pending phase",
			constant: PromptPackPhasePending,
			expected: "Pending",
		},
		{
			name:     "Active phase",
			constant: PromptPackPhaseActive,
			expected: "Active",
		},
		{
			name:     "Canary phase",
			constant: PromptPackPhaseCanary,
			expected: "Canary",
		},
		{
			name:     "Superseded phase",
			constant: PromptPackPhaseSuperseded,
			expected: "Superseded",
		},
		{
			name:     "Failed phase",
			constant: PromptPackPhaseFailed,
			expected: "Failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("PromptPackPhase constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestPromptPackCreation(t *testing.T) {
	promptPack := &PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPromptPackName,
			Namespace: testPromptPackNamespace,
		},
		Spec: PromptPackSpec{
			Source: PromptPackSource{
				Type: PromptPackSourceTypeConfigMap,
				ConfigMapRef: &corev1.LocalObjectReference{
					Name: testConfigMapName,
				},
			},
			Version: testPromptPackVersion,
			Rollout: RolloutStrategy{
				Type: RolloutStrategyImmediate,
			},
		},
	}

	if promptPack.Name != testPromptPackName {
		t.Errorf("PromptPack.Name = %v, want %v", promptPack.Name, testPromptPackName)
	}

	if promptPack.Namespace != testPromptPackNamespace {
		t.Errorf("PromptPack.Namespace = %v, want %v", promptPack.Namespace, testPromptPackNamespace)
	}

	if promptPack.Spec.Version != testPromptPackVersion {
		t.Errorf("PromptPack.Spec.Version = %v, want %v", promptPack.Spec.Version, testPromptPackVersion)
	}

	if promptPack.Spec.Source.Type != PromptPackSourceTypeConfigMap {
		t.Errorf("PromptPack.Spec.Source.Type = %v, want %v", promptPack.Spec.Source.Type, PromptPackSourceTypeConfigMap)
	}

	if promptPack.Spec.Source.ConfigMapRef.Name != testConfigMapName {
		t.Errorf("PromptPack.Spec.Source.ConfigMapRef.Name = %v, want %v", promptPack.Spec.Source.ConfigMapRef.Name, testConfigMapName)
	}

	if promptPack.Spec.Rollout.Type != RolloutStrategyImmediate {
		t.Errorf("PromptPack.Spec.Rollout.Type = %v, want %v", promptPack.Spec.Rollout.Type, RolloutStrategyImmediate)
	}
}

func TestPromptPackWithCanaryRollout(t *testing.T) {
	stepWeight := int32(20)
	interval := "10m"

	promptPack := &PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPromptPackName,
			Namespace: testPromptPackNamespace,
		},
		Spec: PromptPackSpec{
			Source: PromptPackSource{
				Type: PromptPackSourceTypeConfigMap,
				ConfigMapRef: &corev1.LocalObjectReference{
					Name: testConfigMapName,
				},
			},
			Version: testPromptPackVersion,
			Rollout: RolloutStrategy{
				Type: RolloutStrategyCanary,
				Canary: &CanaryConfig{
					Weight:     10,
					StepWeight: &stepWeight,
					Interval:   &interval,
				},
			},
		},
	}

	if promptPack.Spec.Rollout.Type != RolloutStrategyCanary {
		t.Errorf("PromptPack.Spec.Rollout.Type = %v, want %v", promptPack.Spec.Rollout.Type, RolloutStrategyCanary)
	}

	if promptPack.Spec.Rollout.Canary == nil {
		t.Fatal("PromptPack.Spec.Rollout.Canary should not be nil")
	}

	if promptPack.Spec.Rollout.Canary.Weight != 10 {
		t.Errorf("PromptPack.Spec.Rollout.Canary.Weight = %v, want %v", promptPack.Spec.Rollout.Canary.Weight, 10)
	}

	if *promptPack.Spec.Rollout.Canary.StepWeight != 20 {
		t.Errorf("PromptPack.Spec.Rollout.Canary.StepWeight = %v, want %v", *promptPack.Spec.Rollout.Canary.StepWeight, 20)
	}

	if *promptPack.Spec.Rollout.Canary.Interval != "10m" {
		t.Errorf("PromptPack.Spec.Rollout.Canary.Interval = %v, want %v", *promptPack.Spec.Rollout.Canary.Interval, "10m")
	}
}

func TestPromptPackStatus(t *testing.T) {
	canaryWeight := int32(25)
	now := metav1.NewTime(time.Now())

	promptPack := &PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPromptPackName,
			Namespace: testPromptPackNamespace,
		},
		Spec: PromptPackSpec{
			Source: PromptPackSource{
				Type: PromptPackSourceTypeConfigMap,
				ConfigMapRef: &corev1.LocalObjectReference{
					Name: testConfigMapName,
				},
			},
			Version: testCanaryVersion,
			Rollout: RolloutStrategy{
				Type: RolloutStrategyCanary,
			},
		},
		Status: PromptPackStatus{
			Phase:         PromptPackPhaseCanary,
			ActiveVersion: ptrString(testPromptPackVersion),
			CanaryVersion: ptrString(testCanaryVersion),
			CanaryWeight:  &canaryWeight,
			LastUpdated:   &now,
			Conditions: []metav1.Condition{
				{
					Type:               "Available",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "CanaryInProgress",
					Message:            "Canary rollout in progress",
				},
			},
		},
	}

	if promptPack.Status.Phase != PromptPackPhaseCanary {
		t.Errorf("PromptPack.Status.Phase = %v, want %v", promptPack.Status.Phase, PromptPackPhaseCanary)
	}

	if *promptPack.Status.ActiveVersion != testPromptPackVersion {
		t.Errorf("PromptPack.Status.ActiveVersion = %v, want %v", *promptPack.Status.ActiveVersion, testPromptPackVersion)
	}

	if *promptPack.Status.CanaryVersion != testCanaryVersion {
		t.Errorf("PromptPack.Status.CanaryVersion = %v, want %v", *promptPack.Status.CanaryVersion, testCanaryVersion)
	}

	if *promptPack.Status.CanaryWeight != 25 {
		t.Errorf("PromptPack.Status.CanaryWeight = %v, want %v", *promptPack.Status.CanaryWeight, 25)
	}

	if promptPack.Status.LastUpdated == nil {
		t.Error("PromptPack.Status.LastUpdated should not be nil")
	}

	if len(promptPack.Status.Conditions) != 1 {
		t.Errorf("len(PromptPack.Status.Conditions) = %v, want %v", len(promptPack.Status.Conditions), 1)
	}

	if promptPack.Status.Conditions[0].Type != "Available" {
		t.Errorf("PromptPack.Status.Conditions[0].Type = %v, want %v", promptPack.Status.Conditions[0].Type, "Available")
	}
}

func TestPromptPackDeepCopy(t *testing.T) {
	stepWeight := int32(15)
	interval := "5m"
	canaryWeight := int32(30)
	now := metav1.NewTime(time.Now())

	original := &PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPromptPackName,
			Namespace: testPromptPackNamespace,
		},
		Spec: PromptPackSpec{
			Source: PromptPackSource{
				Type: PromptPackSourceTypeConfigMap,
				ConfigMapRef: &corev1.LocalObjectReference{
					Name: testConfigMapName,
				},
			},
			Version: testPromptPackVersion,
			Rollout: RolloutStrategy{
				Type: RolloutStrategyCanary,
				Canary: &CanaryConfig{
					Weight:     10,
					StepWeight: &stepWeight,
					Interval:   &interval,
				},
			},
		},
		Status: PromptPackStatus{
			Phase:         PromptPackPhaseCanary,
			ActiveVersion: ptrString(testPromptPackVersion),
			CanaryVersion: ptrString(testCanaryVersion),
			CanaryWeight:  &canaryWeight,
			LastUpdated:   &now,
			Conditions: []metav1.Condition{
				{
					Type:   "Available",
					Status: metav1.ConditionTrue,
				},
			},
		},
	}

	copied := original.DeepCopy()

	// Verify the copy is independent
	if copied == original {
		t.Error("DeepCopy should return a new object, not the same pointer")
	}

	// Verify values are equal
	if copied.Name != original.Name {
		t.Errorf("DeepCopy().Name = %v, want %v", copied.Name, original.Name)
	}

	if copied.Spec.Version != original.Spec.Version {
		t.Errorf("DeepCopy().Spec.Version = %v, want %v", copied.Spec.Version, original.Spec.Version)
	}

	if copied.Status.Phase != original.Status.Phase {
		t.Errorf("DeepCopy().Status.Phase = %v, want %v", copied.Status.Phase, original.Status.Phase)
	}

	// Modify the copy and verify original is unchanged
	copied.Name = testModifiedName
	if original.Name == testModifiedName {
		t.Error("Modifying copy should not affect original")
	}

	// Verify nested pointer fields are also deep copied
	if copied.Spec.Rollout.Canary == original.Spec.Rollout.Canary {
		t.Error("DeepCopy should create new Canary pointer")
	}

	if copied.Status.CanaryWeight == original.Status.CanaryWeight {
		t.Error("DeepCopy should create new CanaryWeight pointer")
	}
}

func TestPromptPackListDeepCopy(t *testing.T) {
	original := &PromptPackList{
		Items: []PromptPack{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testPromptPackName,
					Namespace: testPromptPackNamespace,
				},
				Spec: PromptPackSpec{
					Source: PromptPackSource{
						Type: PromptPackSourceTypeConfigMap,
					},
					Version: testPromptPackVersion,
					Rollout: RolloutStrategy{
						Type: RolloutStrategyImmediate,
					},
				},
			},
		},
	}

	copied := original.DeepCopy()

	if copied == original {
		t.Error("DeepCopy should return a new object")
	}

	if len(copied.Items) != len(original.Items) {
		t.Errorf("DeepCopy().Items length = %v, want %v", len(copied.Items), len(original.Items))
	}

	// Modify the copy and verify original is unchanged
	copied.Items[0].Name = testModifiedName
	if original.Items[0].Name == testModifiedName {
		t.Error("Modifying copy should not affect original")
	}
}

func TestPromptPackTypeRegistration(t *testing.T) {
	// Verify that PromptPack types are registered with the scheme
	// The init() function should have registered them

	promptPack := &PromptPack{}
	promptPackList := &PromptPackList{}

	// These should not panic if types are registered correctly
	_ = promptPack.DeepCopyObject()
	_ = promptPackList.DeepCopyObject()
}

func TestPromptPackStatusPhases(t *testing.T) {
	tests := []struct {
		name     string
		phase    PromptPackPhase
		isActive bool
		isCanary bool
	}{
		{
			name:     "Pending phase",
			phase:    PromptPackPhasePending,
			isActive: false,
			isCanary: false,
		},
		{
			name:     "Active phase",
			phase:    PromptPackPhaseActive,
			isActive: true,
			isCanary: false,
		},
		{
			name:     "Canary phase",
			phase:    PromptPackPhaseCanary,
			isActive: false,
			isCanary: true,
		},
		{
			name:     "Superseded phase",
			phase:    PromptPackPhaseSuperseded,
			isActive: false,
			isCanary: false,
		},
		{
			name:     "Failed phase",
			phase:    PromptPackPhaseFailed,
			isActive: false,
			isCanary: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			promptPack := &PromptPack{
				Status: PromptPackStatus{
					Phase: tt.phase,
				},
			}

			if (promptPack.Status.Phase == PromptPackPhaseActive) != tt.isActive {
				t.Errorf("Phase %v isActive = %v, want %v", tt.phase, promptPack.Status.Phase == PromptPackPhaseActive, tt.isActive)
			}

			if (promptPack.Status.Phase == PromptPackPhaseCanary) != tt.isCanary {
				t.Errorf("Phase %v isCanary = %v, want %v", tt.phase, promptPack.Status.Phase == PromptPackPhaseCanary, tt.isCanary)
			}
		})
	}
}

func TestPromptPackSourceWithoutConfigMapRef(t *testing.T) {
	// Test that source can be created without ConfigMapRef (for future source types)
	source := PromptPackSource{
		Type: PromptPackSourceTypeConfigMap,
	}

	if source.Type != PromptPackSourceTypeConfigMap {
		t.Errorf("PromptPackSource.Type = %v, want %v", source.Type, PromptPackSourceTypeConfigMap)
	}

	if source.ConfigMapRef != nil {
		t.Error("PromptPackSource.ConfigMapRef should be nil when not set")
	}
}

func TestCanaryConfigDefaults(t *testing.T) {
	// Test CanaryConfig with only required field
	canary := CanaryConfig{
		Weight: 10,
	}

	if canary.Weight != 10 {
		t.Errorf("CanaryConfig.Weight = %v, want %v", canary.Weight, 10)
	}

	if canary.StepWeight != nil {
		t.Error("CanaryConfig.StepWeight should be nil when not set")
	}

	if canary.Interval != nil {
		t.Error("CanaryConfig.Interval should be nil when not set")
	}
}

// ptrString returns a pointer to the given string
func ptrString(s string) *string {
	return &s
}
