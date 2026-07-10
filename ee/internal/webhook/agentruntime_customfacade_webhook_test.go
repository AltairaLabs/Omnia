/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package webhook

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/license"
)

func customFacadeAR(name string, facadeTypes ...corev1alpha1.FacadeType) *corev1alpha1.AgentRuntime {
	facades := make([]corev1alpha1.FacadeConfig, 0, len(facadeTypes))
	for _, ft := range facadeTypes {
		facades = append(facades, corev1alpha1.FacadeConfig{Type: ft})
	}
	return &corev1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       corev1alpha1.AgentRuntimeSpec{Facades: facades},
	}
}

// openCoreValidator returns a license validator that degrades to open-core
// (real client, no dev mode, no license Secret).
func openCoreValidator(t *testing.T) *license.Validator {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	v, err := license.NewValidator(c)
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	return v
}

func devValidator(t *testing.T) *license.Validator {
	t.Helper()
	v, err := license.NewValidator(nil, license.WithDevMode())
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	return v
}

func TestAgentRuntimeCustomFacadeValidator_DeniesUnlicensedCustomFacade(t *testing.T) {
	v := &AgentRuntimeCustomFacadeValidator{LicenseValidator: openCoreValidator(t)}
	ar := customFacadeAR("byo", corev1alpha1.FacadeTypeCustom)

	warnings, err := v.ValidateCreate(context.Background(), ar)
	if err == nil {
		t.Fatal("expected denial for an unlicensed custom facade, got nil")
	}
	if len(warnings) == 0 {
		t.Error("expected an upgrade-message warning")
	}
}

func TestAgentRuntimeCustomFacadeValidator_AdmitsLicensedCustomFacade(t *testing.T) {
	v := &AgentRuntimeCustomFacadeValidator{LicenseValidator: devValidator(t)}
	ar := customFacadeAR("byo", corev1alpha1.FacadeTypeCustom)

	if _, err := v.ValidateCreate(context.Background(), ar); err != nil {
		t.Errorf("expected dev license to admit custom facade, got %v", err)
	}
}

func TestAgentRuntimeCustomFacadeValidator_AdmitsNonCustomFacade(t *testing.T) {
	// Even on open-core, a non-custom AgentRuntime must be admitted — the gate
	// only fires for custom facades.
	v := &AgentRuntimeCustomFacadeValidator{LicenseValidator: openCoreValidator(t)}
	ar := customFacadeAR("ws", corev1alpha1.FacadeTypeWebSocket, corev1alpha1.FacadeTypeA2A)

	if _, err := v.ValidateCreate(context.Background(), ar); err != nil {
		t.Errorf("expected non-custom AgentRuntime to be admitted, got %v", err)
	}
}

func TestAgentRuntimeCustomFacadeValidator_UpdateGates(t *testing.T) {
	v := &AgentRuntimeCustomFacadeValidator{LicenseValidator: openCoreValidator(t)}
	oldAR := customFacadeAR("byo", corev1alpha1.FacadeTypeWebSocket)
	newAR := customFacadeAR("byo", corev1alpha1.FacadeTypeCustom)

	if _, err := v.ValidateUpdate(context.Background(), oldAR, newAR); err == nil {
		t.Error("expected update adding a custom facade to be denied on open-core")
	}
}

func TestAgentRuntimeCustomFacadeValidator_NilValidatorAllows(t *testing.T) {
	v := &AgentRuntimeCustomFacadeValidator{LicenseValidator: nil}
	ar := customFacadeAR("byo", corev1alpha1.FacadeTypeCustom)

	if _, err := v.ValidateCreate(context.Background(), ar); err != nil {
		t.Errorf("nil validator should allow, got %v", err)
	}
}

func TestAgentRuntimeCustomFacadeValidator_DeleteAllows(t *testing.T) {
	v := &AgentRuntimeCustomFacadeValidator{LicenseValidator: openCoreValidator(t)}
	ar := customFacadeAR("byo", corev1alpha1.FacadeTypeCustom)

	if _, err := v.ValidateDelete(context.Background(), ar); err != nil {
		t.Errorf("delete should never be gated, got %v", err)
	}
}
