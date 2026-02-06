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

func TestArenaJobValidatorValidateCreate(t *testing.T) {
	tests := []struct {
		name        string
		job         *omniav1alpha1.ArenaJob
		expectError bool
	}{
		{
			name: "valid evaluation job",
			job: &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
				Spec: omniav1alpha1.ArenaJobSpec{
					Type: omniav1alpha1.ArenaJobTypeEvaluation,
				},
			},
			expectError: false,
		},
		{
			name: "valid loadtest job",
			job: &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
				Spec: omniav1alpha1.ArenaJobSpec{
					Type: omniav1alpha1.ArenaJobTypeLoadTest,
				},
			},
			expectError: false,
		},
	}

	validator := &ArenaJobValidator{LicenseValidator: nil}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validator.ValidateCreate(context.Background(), tt.job)
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestArenaJobValidatorValidateUpdate(t *testing.T) {
	validator := &ArenaJobValidator{LicenseValidator: nil}

	oldJob := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
		Spec: omniav1alpha1.ArenaJobSpec{
			Type: omniav1alpha1.ArenaJobTypeEvaluation,
		},
	}
	newJob := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
		Spec: omniav1alpha1.ArenaJobSpec{
			Type: omniav1alpha1.ArenaJobTypeLoadTest,
		},
	}

	_, err := validator.ValidateUpdate(context.Background(), oldJob, newJob)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestArenaJobValidatorValidateDelete(t *testing.T) {
	validator := &ArenaJobValidator{LicenseValidator: nil}

	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
		Spec: omniav1alpha1.ArenaJobSpec{
			Type: omniav1alpha1.ArenaJobTypeEvaluation,
		},
	}

	_, err := validator.ValidateDelete(context.Background(), job)
	if err != nil {
		t.Errorf("unexpected error on delete: %v", err)
	}
}

func TestArenaJobValidateLicenseNoValidator(t *testing.T) {
	validator := &ArenaJobValidator{LicenseValidator: nil}

	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
		Spec: omniav1alpha1.ArenaJobSpec{
			Type: omniav1alpha1.ArenaJobTypeEvaluation,
		},
	}

	warnings, err := validator.validateLicense(context.Background(), job)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestArenaJobValidateLicenseDevMode(t *testing.T) {
	licenseValidator, err := license.NewValidator(nil, license.WithDevMode())
	if err != nil {
		t.Fatalf("failed to create license validator: %v", err)
	}

	validator := &ArenaJobValidator{LicenseValidator: licenseValidator}

	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
		Spec: omniav1alpha1.ArenaJobSpec{
			Type:    omniav1alpha1.ArenaJobTypeLoadTest,
			Workers: &omniav1alpha1.WorkerConfig{Replicas: 5},
			Schedule: &omniav1alpha1.ScheduleConfig{
				Cron: "0 2 * * * * *",
			},
		},
	}

	warnings, err := validator.validateLicense(context.Background(), job)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestArenaJobValidateLicenseDefaultJobType(t *testing.T) {
	licenseValidator, err := license.NewValidator(nil, license.WithDevMode())
	if err != nil {
		t.Fatalf("failed to create license validator: %v", err)
	}

	validator := &ArenaJobValidator{LicenseValidator: licenseValidator}

	// Empty type defaults to "evaluation"
	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
		Spec:       omniav1alpha1.ArenaJobSpec{},
	}

	warnings, err := validator.validateLicense(context.Background(), job)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestArenaJobValidateLicenseOpenCoreRestriction(t *testing.T) {
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

	validator := &ArenaJobValidator{LicenseValidator: licenseValidator}

	// Scheduled jobs should be restricted on open-core
	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: "default"},
		Spec: omniav1alpha1.ArenaJobSpec{
			Type: omniav1alpha1.ArenaJobTypeEvaluation,
			Schedule: &omniav1alpha1.ScheduleConfig{
				Cron: "0 2 * * * * *",
			},
		},
	}

	warnings, err := validator.validateLicense(context.Background(), job)
	if err == nil {
		t.Error("expected error for scheduled job on open-core license")
	}
	if len(warnings) == 0 {
		t.Error("expected warnings with upgrade message")
	}
}
