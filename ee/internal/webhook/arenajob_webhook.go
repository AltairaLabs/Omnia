/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package webhook

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/license"
)

// ArenaJobValidator validates ArenaJob resources against the license.
type ArenaJobValidator struct {
	LicenseValidator *license.Validator
}

// log is for logging in this package.
var arenajoblog = logf.Log.WithName("arenajob-webhook")

// SetupArenaJobWebhookWithManager registers the ArenaJob webhook with the manager.
func SetupArenaJobWebhookWithManager(mgr ctrl.Manager, licenseValidator *license.Validator) error {
	return ctrl.NewWebhookManagedBy(mgr, &omniav1alpha1.ArenaJob{}).
		WithValidator(&ArenaJobValidator{LicenseValidator: licenseValidator}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-omnia-altairalabs-ai-v1alpha1-arenajob,mutating=false,failurePolicy=fail,sideEffects=None,groups=omnia.altairalabs.ai,resources=arenajobs,verbs=create;update,versions=v1alpha1,name=varenajob.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*omniav1alpha1.ArenaJob] = &ArenaJobValidator{}

// ValidateCreate implements admission.Validator.
func (v *ArenaJobValidator) ValidateCreate(ctx context.Context, job *omniav1alpha1.ArenaJob) (admission.Warnings, error) {
	arenajoblog.Info("validating create", "name", job.Name)
	return v.validateLicense(ctx, job)
}

// ValidateUpdate implements admission.Validator.
func (v *ArenaJobValidator) ValidateUpdate(ctx context.Context, _ *omniav1alpha1.ArenaJob, job *omniav1alpha1.ArenaJob) (admission.Warnings, error) {
	arenajoblog.Info("validating update", "name", job.Name)
	return v.validateLicense(ctx, job)
}

// ValidateDelete implements admission.Validator.
func (v *ArenaJobValidator) ValidateDelete(_ context.Context, _ *omniav1alpha1.ArenaJob) (admission.Warnings, error) {
	// No license validation needed for delete
	return nil, nil
}

// validateLicense checks if the job configuration is allowed by the license.
func (v *ArenaJobValidator) validateLicense(ctx context.Context, job *omniav1alpha1.ArenaJob) (admission.Warnings, error) {
	if v.LicenseValidator == nil {
		// No license validator configured, allow all
		return nil, nil
	}

	// Determine job type
	jobType := string(job.Spec.Type)
	if jobType == "" {
		jobType = "evaluation" // default
	}

	// Determine replica count
	replicas := 1
	if job.Spec.Workers != nil && job.Spec.Workers.Replicas > 0 {
		replicas = int(job.Spec.Workers.Replicas)
	}

	// Determine if scheduling is enabled
	hasSchedule := job.Spec.Schedule != nil && job.Spec.Schedule.Cron != ""

	// Validate against license
	if err := v.LicenseValidator.ValidateArenaJob(ctx, jobType, replicas, hasSchedule); err != nil {
		if licErr, ok := err.(*license.ValidationError); ok {
			arenajoblog.Info("license validation failed",
				"name", job.Name,
				"jobType", jobType,
				"replicas", replicas,
				"hasSchedule", hasSchedule,
				"feature", licErr.Feature,
			)
			return admission.Warnings{licErr.UpgradeMessage()}, fmt.Errorf("%s", licErr.Error())
		}
		return nil, err
	}

	return nil, nil
}
