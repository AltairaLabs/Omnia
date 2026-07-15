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

// PromptPackSourceValidator validates PromptPackSource resources against the license.
type PromptPackSourceValidator struct {
	LicenseValidator *license.Validator
}

// log is for logging in this package.
var promptpacksourcelog = logf.Log.WithName("promptpacksource-webhook")

// SetupPromptPackSourceWebhookWithManager registers the PromptPackSource webhook with the manager.
func SetupPromptPackSourceWebhookWithManager(mgr ctrl.Manager, licenseValidator *license.Validator) error {
	return ctrl.NewWebhookManagedBy(mgr, &omniav1alpha1.PromptPackSource{}).
		WithValidator(&PromptPackSourceValidator{LicenseValidator: licenseValidator}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-omnia-altairalabs-ai-v1alpha1-promptpacksource,mutating=false,failurePolicy=fail,sideEffects=None,groups=omnia.altairalabs.ai,resources=promptpacksources,verbs=create;update,versions=v1alpha1,name=vpromptpacksource.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*omniav1alpha1.PromptPackSource] = &PromptPackSourceValidator{}

// ValidateCreate implements admission.Validator.
func (v *PromptPackSourceValidator) ValidateCreate(ctx context.Context, source *omniav1alpha1.PromptPackSource) (admission.Warnings, error) {
	promptpacksourcelog.Info("validating create", "name", source.Name)
	return v.validateLicense(ctx, source)
}

// ValidateUpdate implements admission.Validator.
func (v *PromptPackSourceValidator) ValidateUpdate(ctx context.Context, _ *omniav1alpha1.PromptPackSource, source *omniav1alpha1.PromptPackSource) (admission.Warnings, error) {
	promptpacksourcelog.Info("validating update", "name", source.Name)
	return v.validateLicense(ctx, source)
}

// ValidateDelete implements admission.Validator.
func (v *PromptPackSourceValidator) ValidateDelete(_ context.Context, _ *omniav1alpha1.PromptPackSource) (admission.Warnings, error) {
	// No license validation needed for delete
	return nil, nil
}

// validateLicense checks if the source type is allowed by the license.
func (v *PromptPackSourceValidator) validateLicense(ctx context.Context, source *omniav1alpha1.PromptPackSource) (admission.Warnings, error) {
	if v.LicenseValidator == nil {
		// No license validator configured, allow all
		return nil, nil
	}

	sourceType := string(source.Spec.Type)
	if err := v.LicenseValidator.ValidatePromptPackSource(ctx, sourceType); err != nil {
		if licErr, ok := err.(*license.ValidationError); ok {
			promptpacksourcelog.Info("license validation failed",
				"name", source.Name,
				"sourceType", sourceType,
				"feature", licErr.Feature,
			)
			return admission.Warnings{licErr.UpgradeMessage()}, fmt.Errorf("%s", licErr.Error())
		}
		return nil, err
	}

	return nil, nil
}
