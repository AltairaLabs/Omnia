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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPartitionStrategyConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant PartitionStrategy
		expected string
	}{
		{
			name:     "Week partition strategy",
			constant: PartitionStrategyWeek,
			expected: "week",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("PartitionStrategy constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestSessionRetentionPolicyPhaseConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant SessionRetentionPolicyPhase
		expected string
	}{
		{
			name:     "Active phase",
			constant: SessionRetentionPolicyPhaseActive,
			expected: "Active",
		},
		{
			name:     "Error phase",
			constant: SessionRetentionPolicyPhaseError,
			expected: "Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("SessionRetentionPolicyPhase constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestSessionRetentionPolicyCreation(t *testing.T) {
	enabled := true
	maxSessions := int32(500)
	maxMessages := int32(200)

	policy := &SessionRetentionPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
		Spec: SessionRetentionPolicySpec{
			Default: RetentionTierConfig{
				HotCache: &HotCacheConfig{
					Enabled:               &enabled,
					TTLAfterInactive:      "24h",
					MaxSessions:           &maxSessions,
					MaxMessagesPerSession: &maxMessages,
				},
				WarmStore: &WarmStoreConfig{
					RetentionDays: 7,
					PartitionBy:   PartitionStrategyWeek,
				},
			},
		},
	}

	if policy.Name != "default" {
		t.Errorf("SessionRetentionPolicy.Name = %v, want %v", policy.Name, "default")
	}

	if policy.Spec.Default.HotCache == nil {
		t.Fatal("HotCache should not be nil")
	}

	if *policy.Spec.Default.HotCache.Enabled != true {
		t.Errorf("HotCache.Enabled = %v, want true", *policy.Spec.Default.HotCache.Enabled)
	}

	if policy.Spec.Default.HotCache.TTLAfterInactive != "24h" {
		t.Errorf("HotCache.TTLAfterInactive = %v, want 24h", policy.Spec.Default.HotCache.TTLAfterInactive)
	}

	if *policy.Spec.Default.HotCache.MaxSessions != 500 {
		t.Errorf("HotCache.MaxSessions = %v, want 500", *policy.Spec.Default.HotCache.MaxSessions)
	}

	if *policy.Spec.Default.HotCache.MaxMessagesPerSession != 200 {
		t.Errorf("HotCache.MaxMessagesPerSession = %v, want 200", *policy.Spec.Default.HotCache.MaxMessagesPerSession)
	}

	if policy.Spec.Default.WarmStore.RetentionDays != 7 {
		t.Errorf("WarmStore.RetentionDays = %v, want 7", policy.Spec.Default.WarmStore.RetentionDays)
	}

	if policy.Spec.Default.WarmStore.PartitionBy != PartitionStrategyWeek {
		t.Errorf("WarmStore.PartitionBy = %v, want week", policy.Spec.Default.WarmStore.PartitionBy)
	}
}

func TestSessionRetentionPolicyWithColdArchive(t *testing.T) {
	retentionDays := int32(365)
	policy := &SessionRetentionPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "with-cold",
		},
		Spec: SessionRetentionPolicySpec{
			Default: RetentionTierConfig{
				ColdArchive: &ColdArchiveConfig{
					Enabled:            true,
					RetentionDays:      &retentionDays,
					CompactionSchedule: "0 2 * * *",
				},
			},
		},
	}

	if !policy.Spec.Default.ColdArchive.Enabled {
		t.Error("ColdArchive.Enabled should be true")
	}

	if *policy.Spec.Default.ColdArchive.RetentionDays != 365 {
		t.Errorf("ColdArchive.RetentionDays = %v, want 365", *policy.Spec.Default.ColdArchive.RetentionDays)
	}

	if policy.Spec.Default.ColdArchive.CompactionSchedule != "0 2 * * *" {
		t.Errorf("ColdArchive.CompactionSchedule = %v, want '0 2 * * *'", policy.Spec.Default.ColdArchive.CompactionSchedule)
	}
}

func TestSessionRetentionPolicyWithPerWorkspaceOverrides(t *testing.T) {
	coldDays := int32(730)
	policy := &SessionRetentionPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "with-overrides",
		},
		Spec: SessionRetentionPolicySpec{
			Default: RetentionTierConfig{
				WarmStore: &WarmStoreConfig{
					RetentionDays: 7,
					PartitionBy:   PartitionStrategyWeek,
				},
			},
			PerWorkspace: map[string]WorkspaceRetentionOverride{
				"production-ws": {
					WarmStore: &WarmStoreConfig{
						RetentionDays: 30,
					},
					ColdArchive: &ColdArchiveConfig{
						Enabled:       true,
						RetentionDays: &coldDays,
					},
				},
			},
		},
	}

	if len(policy.Spec.PerWorkspace) != 1 {
		t.Fatalf("PerWorkspace length = %d, want 1", len(policy.Spec.PerWorkspace))
	}

	override, exists := policy.Spec.PerWorkspace["production-ws"]
	if !exists {
		t.Fatal("PerWorkspace should contain 'production-ws' key")
	}

	if override.WarmStore.RetentionDays != 30 {
		t.Errorf("Override.WarmStore.RetentionDays = %v, want 30", override.WarmStore.RetentionDays)
	}

	if *override.ColdArchive.RetentionDays != 730 {
		t.Errorf("Override.ColdArchive.RetentionDays = %v, want 730", *override.ColdArchive.RetentionDays)
	}
}

func TestSessionRetentionPolicyStatus(t *testing.T) {
	now := metav1.Now()
	policy := &SessionRetentionPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-policy",
		},
		Status: SessionRetentionPolicyStatus{
			Phase:              SessionRetentionPolicyPhaseActive,
			ObservedGeneration: 3,
			WorkspaceCount:     2,
			Conditions: []metav1.Condition{
				{
					Type:               "PolicyValid",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "Valid",
					Message:            "Policy spec is valid",
				},
			},
		},
	}

	if policy.Status.Phase != SessionRetentionPolicyPhaseActive {
		t.Errorf("Status.Phase = %v, want Active", policy.Status.Phase)
	}

	if policy.Status.ObservedGeneration != 3 {
		t.Errorf("Status.ObservedGeneration = %v, want 3", policy.Status.ObservedGeneration)
	}

	if policy.Status.WorkspaceCount != 2 {
		t.Errorf("Status.WorkspaceCount = %v, want 2", policy.Status.WorkspaceCount)
	}

	if len(policy.Status.Conditions) != 1 {
		t.Fatalf("len(Status.Conditions) = %v, want 1", len(policy.Status.Conditions))
	}

	if policy.Status.Conditions[0].Type != "PolicyValid" {
		t.Errorf("Condition type = %v, want PolicyValid", policy.Status.Conditions[0].Type)
	}
}

const testModifiedRetentionName = "modified"

func TestSessionRetentionPolicyDeepCopy(t *testing.T) {
	enabled := true
	maxSessions := int32(500)
	retentionDays := int32(365)

	original := &SessionRetentionPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "original",
		},
		Spec: SessionRetentionPolicySpec{
			Default: RetentionTierConfig{
				HotCache: &HotCacheConfig{
					Enabled:          &enabled,
					TTLAfterInactive: "24h",
					MaxSessions:      &maxSessions,
				},
				WarmStore: &WarmStoreConfig{
					RetentionDays: 7,
					PartitionBy:   PartitionStrategyWeek,
				},
				ColdArchive: &ColdArchiveConfig{
					Enabled:       true,
					RetentionDays: &retentionDays,
				},
			},
			PerWorkspace: map[string]WorkspaceRetentionOverride{
				"ws1": {
					WarmStore: &WarmStoreConfig{
						RetentionDays: 30,
					},
				},
			},
		},
		Status: SessionRetentionPolicyStatus{
			Phase:              SessionRetentionPolicyPhaseActive,
			ObservedGeneration: 1,
			WorkspaceCount:     1,
			Conditions: []metav1.Condition{
				{
					Type:   "PolicyValid",
					Status: metav1.ConditionTrue,
				},
			},
		},
	}

	copied := original.DeepCopy()

	if copied == original {
		t.Error("DeepCopy should return a new object, not the same pointer")
	}

	if copied.Name != original.Name {
		t.Errorf("DeepCopy().Name = %v, want %v", copied.Name, original.Name)
	}

	if copied.Status.Phase != original.Status.Phase {
		t.Errorf("DeepCopy().Status.Phase = %v, want %v", copied.Status.Phase, original.Status.Phase)
	}

	// Verify nested pointer fields are also deep copied
	if copied.Spec.Default.HotCache == original.Spec.Default.HotCache {
		t.Error("DeepCopy should create new HotCache pointer")
	}

	if copied.Spec.Default.ColdArchive.RetentionDays == original.Spec.Default.ColdArchive.RetentionDays {
		t.Error("DeepCopy should create new RetentionDays pointer")
	}

	// Modify the copy and verify original is unchanged
	copied.Name = testModifiedRetentionName
	if original.Name == testModifiedRetentionName {
		t.Error("Modifying copy should not affect original")
	}
}

func TestSessionRetentionPolicyListDeepCopy(t *testing.T) {
	original := &SessionRetentionPolicyList{
		Items: []SessionRetentionPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "policy1",
				},
				Spec: SessionRetentionPolicySpec{
					Default: RetentionTierConfig{
						WarmStore: &WarmStoreConfig{
							RetentionDays: 7,
						},
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

	copied.Items[0].Name = "modified"
	if original.Items[0].Name == "modified" {
		t.Error("Modifying copy should not affect original")
	}
}

func TestSessionRetentionPolicyTypeRegistration(t *testing.T) {
	policy := &SessionRetentionPolicy{}
	policyList := &SessionRetentionPolicyList{}

	// These should not panic if types are registered correctly
	_ = policy.DeepCopyObject()
	_ = policyList.DeepCopyObject()
}
