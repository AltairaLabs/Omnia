/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestDerivePhaseFromConditions(t *testing.T) {
	tests := []struct {
		name       string
		conditions []metav1.Condition
		want       omniav1alpha1.ProviderPhase
	}{
		{
			name:       "no conditions defaults to Ready",
			conditions: nil,
			want:       omniav1alpha1.ProviderPhaseReady,
		},
		{
			name: "CredentialValid=True keeps Ready",
			conditions: []metav1.Condition{
				{Type: ProviderConditionTypeCredentialValid, Status: metav1.ConditionTrue, Reason: "CredentialAccepted"},
			},
			want: omniav1alpha1.ProviderPhaseReady,
		},
		{
			name: "CredentialValid=Unknown keeps Ready (transient probe failure)",
			conditions: []metav1.Condition{
				{Type: ProviderConditionTypeCredentialValid, Status: metav1.ConditionUnknown, Reason: "CredentialValidationError"},
			},
			want: omniav1alpha1.ProviderPhaseReady,
		},
		{
			name: "CredentialValid=False forces Error",
			conditions: []metav1.Condition{
				{Type: ProviderConditionTypeCredentialValid, Status: metav1.ConditionFalse, Reason: "CredentialRejected"},
			},
			want: omniav1alpha1.ProviderPhaseError,
		},
		{
			name: "CredentialConfigured=False (placeholder) forces Error",
			conditions: []metav1.Condition{
				{Type: ProviderConditionTypeCredentialConfigured, Status: metav1.ConditionFalse, Reason: "PlaceholderCredential"},
			},
			want: omniav1alpha1.ProviderPhaseError,
		},
		{
			name: "CredentialValid=False wins over CredentialConfigured=True",
			conditions: []metav1.Condition{
				{Type: ProviderConditionTypeCredentialConfigured, Status: metav1.ConditionTrue, Reason: "SecretFound"},
				{Type: ProviderConditionTypeCredentialValid, Status: metav1.ConditionFalse, Reason: "CredentialRejected"},
			},
			want: omniav1alpha1.ProviderPhaseError,
		},
		{
			name: "ModelValid=False forces Error",
			conditions: []metav1.Condition{
				{Type: ProviderConditionTypeModelValid, Status: metav1.ConditionFalse, Reason: "ModelMissing"},
			},
			want: omniav1alpha1.ProviderPhaseError,
		},
		{
			name: "ModelValid=False forces Error even with credentials green",
			conditions: []metav1.Condition{
				{Type: ProviderConditionTypeCredentialConfigured, Status: metav1.ConditionTrue},
				{Type: ProviderConditionTypeCredentialValid, Status: metav1.ConditionTrue},
				{Type: ProviderConditionTypeModelValid, Status: metav1.ConditionFalse, Reason: "ModelMissing"},
			},
			want: omniav1alpha1.ProviderPhaseError,
		},
		{
			name: "ModelValid=True keeps Ready",
			conditions: []metav1.Condition{
				{Type: ProviderConditionTypeModelValid, Status: metav1.ConditionTrue, Reason: "ModelSet"},
			},
			want: omniav1alpha1.ProviderPhaseReady,
		},
		{
			name: "all green stays Ready",
			conditions: []metav1.Condition{
				{Type: ProviderConditionTypeSecretFound, Status: metav1.ConditionTrue},
				{Type: ProviderConditionTypeCredentialConfigured, Status: metav1.ConditionTrue},
				{Type: ProviderConditionTypeCredentialValid, Status: metav1.ConditionTrue},
				{Type: ProviderConditionTypeModelValid, Status: metav1.ConditionTrue},
			},
			want: omniav1alpha1.ProviderPhaseReady,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &omniav1alpha1.Provider{
				Status: omniav1alpha1.ProviderStatus{Conditions: tt.conditions},
			}
			got := derivePhaseFromConditions(provider)
			if got != tt.want {
				t.Errorf("derivePhaseFromConditions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetModelValidCondition(t *testing.T) {
	tests := []struct {
		name          string
		providerType  omniav1alpha1.ProviderType
		model         string
		wantStatus    metav1.ConditionStatus
		wantReason    string
		wantMsgSubstr string // substring expected in the condition message (suggestion hint)
	}{
		{
			name:          "claude with empty model is ModelValid=False with suggestion",
			providerType:  omniav1alpha1.ProviderTypeClaude,
			model:         "",
			wantStatus:    metav1.ConditionFalse,
			wantReason:    "ModelMissing",
			wantMsgSubstr: "claude-sonnet-4-20250514",
		},
		{
			name:          "openai with empty model suggests an openai model",
			providerType:  omniav1alpha1.ProviderTypeOpenAI,
			model:         "",
			wantStatus:    metav1.ConditionFalse,
			wantReason:    "ModelMissing",
			wantMsgSubstr: "gpt-4o",
		},
		{
			name:         "claude with a model is ModelValid=True",
			providerType: omniav1alpha1.ProviderTypeClaude,
			model:        "claude-sonnet-4-20250514",
			wantStatus:   metav1.ConditionTrue,
			wantReason:   "ModelSet",
		},
		{
			name:         "mock with empty model is exempt (ModelValid=True)",
			providerType: omniav1alpha1.ProviderTypeMock,
			model:        "",
			wantStatus:   metav1.ConditionTrue,
			wantReason:   "ModelNotRequired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{Type: tt.providerType, Model: tt.model},
			}
			r := &ProviderReconciler{}
			r.setModelValidCondition(provider)

			cond := meta.FindStatusCondition(provider.Status.Conditions, ProviderConditionTypeModelValid)
			if cond == nil {
				t.Fatalf("ModelValid condition not set")
			}
			if cond.Status != tt.wantStatus {
				t.Errorf("ModelValid status = %v, want %v", cond.Status, tt.wantStatus)
			}
			if cond.Reason != tt.wantReason {
				t.Errorf("ModelValid reason = %q, want %q", cond.Reason, tt.wantReason)
			}
			if tt.wantMsgSubstr != "" && !strings.Contains(cond.Message, tt.wantMsgSubstr) {
				t.Errorf("ModelValid message = %q, want it to contain %q", cond.Message, tt.wantMsgSubstr)
			}
		})
	}
}

func TestModelSuggestionFallback(t *testing.T) {
	// An unmapped type still yields a non-empty, type-specific hint.
	got := modelSuggestion(omniav1alpha1.ProviderType("someunknownvendor"))
	if !strings.Contains(got, "someunknownvendor") {
		t.Errorf("modelSuggestion fallback = %q, want it to name the type", got)
	}
}
