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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/license"
)

// ArenaTemplateSourceValidator validates ArenaTemplateSource resources against the license.
type ArenaTemplateSourceValidator struct {
	LicenseValidator *license.Validator
}

// log is for logging in this package.
var arenatemplatesourcelog = logf.Log.WithName("arenatemplatesource-webhook")

// SetupArenaTemplateSourceWebhookWithManager registers the ArenaTemplateSource webhook with the manager.
func SetupArenaTemplateSourceWebhookWithManager(mgr ctrl.Manager, licenseValidator *license.Validator) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&omniav1alpha1.ArenaTemplateSource{}).
		WithValidator(&ArenaTemplateSourceValidator{LicenseValidator: licenseValidator}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-omnia-altairalabs-ai-v1alpha1-arenatemplatesource,mutating=false,failurePolicy=fail,sideEffects=None,groups=omnia.altairalabs.ai,resources=arenatemplatesources,verbs=create;update,versions=v1alpha1,name=varenatemplatesource.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &ArenaTemplateSourceValidator{}

// ValidateCreate implements webhook.CustomValidator.
func (v *ArenaTemplateSourceValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	source, ok := obj.(*omniav1alpha1.ArenaTemplateSource)
	if !ok {
		return nil, fmt.Errorf("expected ArenaTemplateSource but got %T", obj)
	}
	arenatemplatesourcelog.Info("validating create", "name", source.Name)

	// Validate spec
	if errs := v.validateSpec(source); len(errs) > 0 {
		return nil, fmt.Errorf("invalid spec: %v", errs)
	}

	return v.validateLicense(ctx, source)
}

// ValidateUpdate implements webhook.CustomValidator.
func (v *ArenaTemplateSourceValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	source, ok := newObj.(*omniav1alpha1.ArenaTemplateSource)
	if !ok {
		return nil, fmt.Errorf("expected ArenaTemplateSource but got %T", newObj)
	}
	arenatemplatesourcelog.Info("validating update", "name", source.Name)

	// Validate spec
	if errs := v.validateSpec(source); len(errs) > 0 {
		return nil, fmt.Errorf("invalid spec: %v", errs)
	}

	return v.validateLicense(ctx, source)
}

// ValidateDelete implements webhook.CustomValidator.
func (v *ArenaTemplateSourceValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	// No validation needed for delete
	return nil, nil
}

// validateSpec validates the ArenaTemplateSource spec.
func (v *ArenaTemplateSourceValidator) validateSpec(source *omniav1alpha1.ArenaTemplateSource) []string {
	var errs []string

	// Validate source type has corresponding config
	switch source.Spec.Type {
	case omniav1alpha1.ArenaTemplateSourceTypeGit:
		if source.Spec.Git == nil {
			errs = append(errs, "git configuration is required when type is 'git'")
		}
	case omniav1alpha1.ArenaTemplateSourceTypeOCI:
		if source.Spec.OCI == nil {
			errs = append(errs, "oci configuration is required when type is 'oci'")
		}
	case omniav1alpha1.ArenaTemplateSourceTypeConfigMap:
		if source.Spec.ConfigMap == nil {
			errs = append(errs, "configMap configuration is required when type is 'configmap'")
		}
	}

	return errs
}

// validateLicense checks if the source type is allowed by the license.
func (v *ArenaTemplateSourceValidator) validateLicense(ctx context.Context, source *omniav1alpha1.ArenaTemplateSource) (admission.Warnings, error) {
	if v.LicenseValidator == nil {
		// No license validator configured, allow all
		return nil, nil
	}

	sourceType := string(source.Spec.Type)
	if err := v.LicenseValidator.ValidateArenaSource(ctx, sourceType); err != nil {
		if licErr, ok := err.(*license.ValidationError); ok {
			arenatemplatesourcelog.Info("license validation failed",
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
