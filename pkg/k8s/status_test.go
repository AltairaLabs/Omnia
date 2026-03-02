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

package k8s

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestPatchAgentRuntimeCondition_SetsCondition(t *testing.T) {
	s := Scheme()
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-agent",
			Namespace:  "default",
			Generation: 3,
		},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(ar).
		WithStatusSubresource(ar).
		Build()

	err := PatchAgentRuntimeCondition(
		context.Background(), c,
		"test-agent", "default",
		ConditionPackContentValid, metav1.ConditionTrue,
		"PackContentValid", "Pack content validated successfully",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the condition was set
	got, err := GetAgentRuntime(context.Background(), c, "test-agent", "default")
	if err != nil {
		t.Fatalf("failed to get AgentRuntime: %v", err)
	}

	if len(got.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(got.Status.Conditions))
	}

	cond := got.Status.Conditions[0]
	if cond.Type != ConditionPackContentValid {
		t.Errorf("expected type %s, got %s", ConditionPackContentValid, cond.Type)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected status True, got %s", cond.Status)
	}
	if cond.Reason != "PackContentValid" {
		t.Errorf("expected reason PackContentValid, got %s", cond.Reason)
	}
	if cond.ObservedGeneration != 3 {
		t.Errorf("expected observedGeneration 3, got %d", cond.ObservedGeneration)
	}
}

func TestPatchAgentRuntimeCondition_UpsertsExistingCondition(t *testing.T) {
	s := Scheme()
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-agent",
			Namespace:  "default",
			Generation: 2,
		},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
		},
		Status: omniav1alpha1.AgentRuntimeStatus{
			Conditions: []metav1.Condition{
				{
					Type:               ConditionPackContentValid,
					Status:             metav1.ConditionFalse,
					Reason:             "ContentIssuesFound",
					Message:            "old issue",
					ObservedGeneration: 1,
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(ar).
		WithStatusSubresource(ar).
		Build()

	err := PatchAgentRuntimeCondition(
		context.Background(), c,
		"test-agent", "default",
		ConditionPackContentValid, metav1.ConditionTrue,
		"PackContentValid", "Pack content validated successfully",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := GetAgentRuntime(context.Background(), c, "test-agent", "default")
	if err != nil {
		t.Fatalf("failed to get AgentRuntime: %v", err)
	}

	// Should still be 1 condition (upserted, not duplicated)
	if len(got.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition (upserted), got %d", len(got.Status.Conditions))
	}

	cond := got.Status.Conditions[0]
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected status True after upsert, got %s", cond.Status)
	}
	if cond.ObservedGeneration != 2 {
		t.Errorf("expected observedGeneration 2, got %d", cond.ObservedGeneration)
	}
}

func TestPatchAgentRuntimeCondition_NotFound(t *testing.T) {
	s := Scheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()

	err := PatchAgentRuntimeCondition(
		context.Background(), c,
		"nonexistent", "default",
		ConditionPackContentValid, metav1.ConditionTrue,
		"PackContentValid", "ok",
	)
	if err == nil {
		t.Fatal("expected error for not found AgentRuntime")
	}
}
