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

func TestArenaTemplateSourceValidatorValidateCreate(t *testing.T) {
	tests := []struct {
		name        string
		source      *omniav1alpha1.ArenaTemplateSource
		expectError bool
	}{
		{
			name: "valid git source",
			source: &omniav1alpha1.ArenaTemplateSource{
				ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
				Spec: omniav1alpha1.ArenaTemplateSourceSpec{
					Type: omniav1alpha1.ArenaTemplateSourceTypeGit,
					Git: &omniav1alpha1.GitSource{
						URL: "https://github.com/test/repo",
					},
				},
			},
			expectError: false,
		},
		{
			name: "git type without git config",
			source: &omniav1alpha1.ArenaTemplateSource{
				ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
				Spec: omniav1alpha1.ArenaTemplateSourceSpec{
					Type: omniav1alpha1.ArenaTemplateSourceTypeGit,
				},
			},
			expectError: true,
		},
		{
			name: "oci type without oci config",
			source: &omniav1alpha1.ArenaTemplateSource{
				ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
				Spec: omniav1alpha1.ArenaTemplateSourceSpec{
					Type: omniav1alpha1.ArenaTemplateSourceTypeOCI,
				},
			},
			expectError: true,
		},
		{
			name: "configmap type without configmap config",
			source: &omniav1alpha1.ArenaTemplateSource{
				ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
				Spec: omniav1alpha1.ArenaTemplateSourceSpec{
					Type: omniav1alpha1.ArenaTemplateSourceTypeConfigMap,
				},
			},
			expectError: true,
		},
		{
			name: "valid oci source",
			source: &omniav1alpha1.ArenaTemplateSource{
				ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
				Spec: omniav1alpha1.ArenaTemplateSourceSpec{
					Type: omniav1alpha1.ArenaTemplateSourceTypeOCI,
					OCI: &omniav1alpha1.OCISource{
						URL: "oci://registry.example.com/templates:latest",
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid configmap source",
			source: &omniav1alpha1.ArenaTemplateSource{
				ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
				Spec: omniav1alpha1.ArenaTemplateSourceSpec{
					Type: omniav1alpha1.ArenaTemplateSourceTypeConfigMap,
					ConfigMap: &omniav1alpha1.ConfigMapSource{
						Name: "template-config",
					},
				},
			},
			expectError: false,
		},
	}

	validator := &ArenaTemplateSourceValidator{LicenseValidator: nil}

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

func TestArenaTemplateSourceValidatorValidateUpdate(t *testing.T) {
	tests := []struct {
		name        string
		oldSource   *omniav1alpha1.ArenaTemplateSource
		newSource   *omniav1alpha1.ArenaTemplateSource
		expectError bool
	}{
		{
			name: "valid update",
			oldSource: &omniav1alpha1.ArenaTemplateSource{
				ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
				Spec: omniav1alpha1.ArenaTemplateSourceSpec{
					Type: omniav1alpha1.ArenaTemplateSourceTypeGit,
					Git: &omniav1alpha1.GitSource{
						URL: "https://github.com/test/repo",
					},
					SyncInterval: "1h",
				},
			},
			newSource: &omniav1alpha1.ArenaTemplateSource{
				ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
				Spec: omniav1alpha1.ArenaTemplateSourceSpec{
					Type: omniav1alpha1.ArenaTemplateSourceTypeGit,
					Git: &omniav1alpha1.GitSource{
						URL: "https://github.com/test/repo",
					},
					SyncInterval: "2h",
				},
			},
			expectError: false,
		},
		{
			name: "invalid update - missing git config",
			oldSource: &omniav1alpha1.ArenaTemplateSource{
				ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
				Spec: omniav1alpha1.ArenaTemplateSourceSpec{
					Type: omniav1alpha1.ArenaTemplateSourceTypeGit,
					Git: &omniav1alpha1.GitSource{
						URL: "https://github.com/test/repo",
					},
				},
			},
			newSource: &omniav1alpha1.ArenaTemplateSource{
				ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
				Spec: omniav1alpha1.ArenaTemplateSourceSpec{
					Type: omniav1alpha1.ArenaTemplateSourceTypeGit,
					Git:  nil, // Missing git config
				},
			},
			expectError: true,
		},
	}

	validator := &ArenaTemplateSourceValidator{LicenseValidator: nil}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validator.ValidateUpdate(context.Background(), tt.oldSource, tt.newSource)
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestArenaTemplateSourceValidatorValidateDelete(t *testing.T) {
	validator := &ArenaTemplateSourceValidator{LicenseValidator: nil}

	source := &omniav1alpha1.ArenaTemplateSource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
		Spec: omniav1alpha1.ArenaTemplateSourceSpec{
			Type: omniav1alpha1.ArenaTemplateSourceTypeGit,
			Git: &omniav1alpha1.GitSource{
				URL: "https://github.com/test/repo",
			},
		},
	}

	_, err := validator.ValidateDelete(context.Background(), source)
	if err != nil {
		t.Errorf("unexpected error on delete: %v", err)
	}
}

func TestValidateSpec(t *testing.T) {
	validator := &ArenaTemplateSourceValidator{LicenseValidator: nil}

	tests := []struct {
		name       string
		source     *omniav1alpha1.ArenaTemplateSource
		expectErrs int
	}{
		{
			name: "valid git",
			source: &omniav1alpha1.ArenaTemplateSource{
				Spec: omniav1alpha1.ArenaTemplateSourceSpec{
					Type: omniav1alpha1.ArenaTemplateSourceTypeGit,
					Git:  &omniav1alpha1.GitSource{URL: "https://github.com/test/repo"},
				},
			},
			expectErrs: 0,
		},
		{
			name: "missing git config",
			source: &omniav1alpha1.ArenaTemplateSource{
				Spec: omniav1alpha1.ArenaTemplateSourceSpec{
					Type: omniav1alpha1.ArenaTemplateSourceTypeGit,
				},
			},
			expectErrs: 1,
		},
		{
			name: "missing oci config",
			source: &omniav1alpha1.ArenaTemplateSource{
				Spec: omniav1alpha1.ArenaTemplateSourceSpec{
					Type: omniav1alpha1.ArenaTemplateSourceTypeOCI,
				},
			},
			expectErrs: 1,
		},
		{
			name: "missing configmap config",
			source: &omniav1alpha1.ArenaTemplateSource{
				Spec: omniav1alpha1.ArenaTemplateSourceSpec{
					Type: omniav1alpha1.ArenaTemplateSourceTypeConfigMap,
				},
			},
			expectErrs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.validateSpec(tt.source)
			if len(errs) != tt.expectErrs {
				t.Errorf("validateSpec() returned %d errors, want %d: %v", len(errs), tt.expectErrs, errs)
			}
		})
	}
}

func TestValidateLicenseNoValidator(t *testing.T) {
	validator := &ArenaTemplateSourceValidator{LicenseValidator: nil}

	source := &omniav1alpha1.ArenaTemplateSource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
		Spec: omniav1alpha1.ArenaTemplateSourceSpec{
			Type: omniav1alpha1.ArenaTemplateSourceTypeGit,
			Git:  &omniav1alpha1.GitSource{URL: "https://github.com/test/repo"},
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

func TestValidateCreateWrongType(t *testing.T) {
	validator := &ArenaTemplateSourceValidator{LicenseValidator: nil}

	// Pass wrong type to ValidateCreate
	_, err := validator.ValidateCreate(context.Background(), &omniav1alpha1.ArenaSource{})
	if err == nil {
		t.Error("expected error for wrong type")
	}
}

func TestValidateUpdateWrongType(t *testing.T) {
	validator := &ArenaTemplateSourceValidator{LicenseValidator: nil}

	validSource := &omniav1alpha1.ArenaTemplateSource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
		Spec: omniav1alpha1.ArenaTemplateSourceSpec{
			Type: omniav1alpha1.ArenaTemplateSourceTypeGit,
			Git:  &omniav1alpha1.GitSource{URL: "https://github.com/test/repo"},
		},
	}

	// Pass wrong type as newObj to ValidateUpdate
	_, err := validator.ValidateUpdate(context.Background(), validSource, &omniav1alpha1.ArenaSource{})
	if err == nil {
		t.Error("expected error for wrong type")
	}
}

func TestValidateLicenseWithDevModeValidator(t *testing.T) {
	// Create a dev mode validator which allows all features
	licenseValidator, err := license.NewValidator(nil, license.WithDevMode())
	if err != nil {
		t.Fatalf("failed to create license validator: %v", err)
	}

	validator := &ArenaTemplateSourceValidator{LicenseValidator: licenseValidator}

	source := &omniav1alpha1.ArenaTemplateSource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
		Spec: omniav1alpha1.ArenaTemplateSourceSpec{
			Type: omniav1alpha1.ArenaTemplateSourceTypeGit,
			Git:  &omniav1alpha1.GitSource{URL: "https://github.com/test/repo"},
		},
	}

	warnings, err := validator.validateLicense(context.Background(), source)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Dev mode validator should not return any warnings
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestValidateLicenseWithOCISource(t *testing.T) {
	// Create a dev mode validator
	licenseValidator, err := license.NewValidator(nil, license.WithDevMode())
	if err != nil {
		t.Fatalf("failed to create license validator: %v", err)
	}

	validator := &ArenaTemplateSourceValidator{LicenseValidator: licenseValidator}

	source := &omniav1alpha1.ArenaTemplateSource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
		Spec: omniav1alpha1.ArenaTemplateSourceSpec{
			Type: omniav1alpha1.ArenaTemplateSourceTypeOCI,
			OCI:  &omniav1alpha1.OCISource{URL: "oci://registry.example.com/templates:latest"},
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

func TestValidateLicenseWithConfigMapSource(t *testing.T) {
	// Create a dev mode validator
	licenseValidator, err := license.NewValidator(nil, license.WithDevMode())
	if err != nil {
		t.Fatalf("failed to create license validator: %v", err)
	}

	validator := &ArenaTemplateSourceValidator{LicenseValidator: licenseValidator}

	source := &omniav1alpha1.ArenaTemplateSource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
		Spec: omniav1alpha1.ArenaTemplateSourceSpec{
			Type:      omniav1alpha1.ArenaTemplateSourceTypeConfigMap,
			ConfigMap: &omniav1alpha1.ConfigMapSource{Name: "template-configmap"},
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

func TestValidateLicenseOpenCoreRestriction(t *testing.T) {
	// Create an open-core validator (no dev mode, no license JWT)
	// Open-core doesn't allow OCI sources
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

	validator := &ArenaTemplateSourceValidator{LicenseValidator: licenseValidator}

	// OCI source is not allowed on open-core license
	source := &omniav1alpha1.ArenaTemplateSource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-source", Namespace: "default"},
		Spec: omniav1alpha1.ArenaTemplateSourceSpec{
			Type: omniav1alpha1.ArenaTemplateSourceTypeOCI,
			OCI:  &omniav1alpha1.OCISource{URL: "oci://registry.example.com/templates:latest"},
		},
	}

	warnings, err := validator.validateLicense(context.Background(), source)
	// Open-core should return an error for OCI sources
	if err == nil {
		t.Error("expected error for OCI source on open-core license")
	}
	// Should have warnings (upgrade message)
	if len(warnings) == 0 {
		t.Error("expected warnings with upgrade message")
	}
}
