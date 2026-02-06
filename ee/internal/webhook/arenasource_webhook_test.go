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

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/license"
)

func TestArenaSourceValidatorValidateCreate(t *testing.T) {
	tests := []struct {
		name        string
		source      *omniav1alpha1.ArenaSource
		expectError bool
	}{
		{
			name: "valid git source",
			source: &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type: omniav1alpha1.ArenaSourceTypeGit,
				},
			},
			expectError: false,
		},
		{
			name: "valid oci source",
			source: &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type: omniav1alpha1.ArenaSourceTypeOCI,
				},
			},
			expectError: false,
		},
		{
			name: "valid configmap source",
			source: &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type: omniav1alpha1.ArenaSourceTypeConfigMap,
				},
			},
			expectError: false,
		},
	}

	validator := &ArenaSourceValidator{LicenseValidator: nil}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validator.ValidateCreate(context.Background(), tt.source)
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestArenaSourceValidatorValidateUpdate(t *testing.T) {
	validator := &ArenaSourceValidator{LicenseValidator: nil}

	oldSource := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
		Spec: omniav1alpha1.ArenaSourceSpec{
			Type: omniav1alpha1.ArenaSourceTypeGit,
		},
	}
	newSource := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
		Spec: omniav1alpha1.ArenaSourceSpec{
			Type: omniav1alpha1.ArenaSourceTypeOCI,
		},
	}

	_, err := validator.ValidateUpdate(context.Background(), oldSource, newSource)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestArenaSourceValidatorValidateDelete(t *testing.T) {
	validator := &ArenaSourceValidator{LicenseValidator: nil}

	source := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
		Spec: omniav1alpha1.ArenaSourceSpec{
			Type: omniav1alpha1.ArenaSourceTypeGit,
		},
	}

	_, err := validator.ValidateDelete(context.Background(), source)
	if err != nil {
		t.Errorf("unexpected error on delete: %v", err)
	}
}

func TestArenaSourceValidateLicenseNoValidator(t *testing.T) {
	validator := &ArenaSourceValidator{LicenseValidator: nil}

	source := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
		Spec: omniav1alpha1.ArenaSourceSpec{
			Type: omniav1alpha1.ArenaSourceTypeGit,
		},
	}

	warnings, err := validator.validateLicense(context.Background(), source)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestArenaSourceValidateLicenseDevMode(t *testing.T) {
	licenseValidator, err := license.NewValidator(nil, license.WithDevMode())
	if err != nil {
		t.Fatalf("failed to create license validator: %v", err)
	}

	validator := &ArenaSourceValidator{LicenseValidator: licenseValidator}

	source := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
		Spec: omniav1alpha1.ArenaSourceSpec{
			Type: omniav1alpha1.ArenaSourceTypeOCI,
		},
	}

	warnings, err := validator.validateLicense(context.Background(), source)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestArenaSourceValidateLicenseOpenCoreRestriction(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	licenseValidator, err := license.NewValidator(fakeClient)
	if err != nil {
		t.Fatalf("failed to create license validator: %v", err)
	}

	validator := &ArenaSourceValidator{LicenseValidator: licenseValidator}

	// OCI source should be restricted on open-core
	source := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
		Spec: omniav1alpha1.ArenaSourceSpec{
			Type: omniav1alpha1.ArenaSourceTypeOCI,
		},
	}

	warnings, err := validator.validateLicense(context.Background(), source)
	if err == nil {
		t.Error("expected error for OCI source on open-core license")
	}
	if len(warnings) == 0 {
		t.Error("expected warnings with upgrade message")
	}
}
