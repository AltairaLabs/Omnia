/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestPromptPackInvalidReason(t *testing.T) {
	cases := []struct {
		name    string
		pp      *omniav1alpha1.PromptPack
		wantBad bool
	}{
		{
			name: "schema validation failed condition",
			pp: &omniav1alpha1.PromptPack{Status: omniav1alpha1.PromptPackStatus{
				Conditions: []metav1.Condition{{
					Type: PromptPackConditionTypeSchemaValid, Status: metav1.ConditionFalse,
					Reason: "SchemaValidationFailed", Message: "template_engine is required",
				}},
			}},
			wantBad: true,
		},
		{
			name:    "failed phase",
			pp:      &omniav1alpha1.PromptPack{Status: omniav1alpha1.PromptPackStatus{Phase: omniav1alpha1.PromptPackPhaseFailed}},
			wantBad: true,
		},
		{
			name: "active and schema-valid",
			pp: &omniav1alpha1.PromptPack{Status: omniav1alpha1.PromptPackStatus{
				Phase:      omniav1alpha1.PromptPackPhaseActive,
				Conditions: []metav1.Condition{{Type: PromptPackConditionTypeSchemaValid, Status: metav1.ConditionTrue}},
			}},
			wantBad: false,
		},
		{
			name:    "no status yet (still reconciling) is not gated",
			pp:      &omniav1alpha1.PromptPack{},
			wantBad: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := promptPackInvalidReason(c.pp)
			if c.wantBad && got == "" {
				t.Fatal("expected a non-empty reason")
			}
			if !c.wantBad && got != "" {
				t.Fatalf("expected empty reason, got %q", got)
			}
		})
	}
}

func conditionByType(conds []metav1.Condition, t string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == t {
			return &conds[i]
		}
	}
	return nil
}

func TestReconcileReferences_PromptPackGate(t *testing.T) {
	scheme := newScheme(t)

	newAR := func() *omniav1alpha1.AgentRuntime {
		return &omniav1alpha1.AgentRuntime{
			ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "default"},
			Spec:       omniav1alpha1.AgentRuntimeSpec{PromptPackRef: omniav1alpha1.PromptPackRef{Name: "pack"}},
		}
	}

	t.Run("failed pack gates the agent to Failed", func(t *testing.T) {
		pp := &omniav1alpha1.PromptPack{
			ObjectMeta: metav1.ObjectMeta{Name: "pack", Namespace: "default"},
			Status: omniav1alpha1.PromptPackStatus{
				Phase: omniav1alpha1.PromptPackPhaseFailed,
				Conditions: []metav1.Condition{{
					Type: PromptPackConditionTypeSchemaValid, Status: metav1.ConditionFalse,
					Reason: "SchemaValidationFailed", Message: "template_engine is required",
					LastTransitionTime: metav1.Now(),
				}},
			},
		}
		ar := newAR()
		fc := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&omniav1alpha1.AgentRuntime{}).
			WithObjects(pp, ar).Build()
		r := &AgentRuntimeReconciler{Client: fc, Scheme: scheme}

		gotPack, _, _, result, err := r.reconcileReferences(context.Background(), logf.Log, ar)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotPack != nil {
			t.Fatal("expected nil pack on gate")
		}
		if result.RequeueAfter == 0 {
			t.Fatal("expected a requeue when the pack is invalid")
		}
		if ar.Status.Phase != omniav1alpha1.AgentRuntimePhaseFailed {
			t.Fatalf("phase = %q, want Failed", ar.Status.Phase)
		}
		cond := conditionByType(ar.Status.Conditions, ConditionTypePromptPackReady)
		if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "PromptPackInvalid" {
			t.Fatalf("PromptPackReady = %+v, want False/PromptPackInvalid", cond)
		}
	})

	t.Run("active schema-valid pack passes the gate", func(t *testing.T) {
		pp := &omniav1alpha1.PromptPack{
			ObjectMeta: metav1.ObjectMeta{Name: "pack", Namespace: "default"},
			Status: omniav1alpha1.PromptPackStatus{
				Phase:      omniav1alpha1.PromptPackPhaseActive,
				Conditions: []metav1.Condition{{Type: PromptPackConditionTypeSchemaValid, Status: metav1.ConditionTrue}},
			},
		}
		ar := newAR()
		fc := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&omniav1alpha1.AgentRuntime{}).
			WithObjects(pp, ar).Build()
		r := &AgentRuntimeReconciler{Client: fc, Scheme: scheme}

		gotPack, _, _, result, err := r.reconcileReferences(context.Background(), logf.Log, ar)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotPack == nil {
			t.Fatal("expected the pack to be returned")
		}
		if result.RequeueAfter != 0 {
			t.Fatalf("expected no requeue for a valid pack, got %v", result.RequeueAfter)
		}
		cond := conditionByType(ar.Status.Conditions, ConditionTypePromptPackReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Fatalf("PromptPackReady = %+v, want True", cond)
		}
	})
}
