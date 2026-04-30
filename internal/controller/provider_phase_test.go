/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestDerivePhaseFromCredentialConditions(t *testing.T) {
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
			name: "all green stays Ready",
			conditions: []metav1.Condition{
				{Type: ProviderConditionTypeSecretFound, Status: metav1.ConditionTrue},
				{Type: ProviderConditionTypeCredentialConfigured, Status: metav1.ConditionTrue},
				{Type: ProviderConditionTypeCredentialValid, Status: metav1.ConditionTrue},
			},
			want: omniav1alpha1.ProviderPhaseReady,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &omniav1alpha1.Provider{
				Status: omniav1alpha1.ProviderStatus{Conditions: tt.conditions},
			}
			got := derivePhaseFromCredentialConditions(provider)
			if got != tt.want {
				t.Errorf("derivePhaseFromCredentialConditions() = %v, want %v", got, tt.want)
			}
		})
	}
}
