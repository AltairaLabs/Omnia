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

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/license"
)

// AgentRuntimeCustomFacadeValidator rejects AgentRuntimes that declare a
// "custom" (bring-your-own-container) facade unless the license permits it.
//
// This lives entirely under ee/ and is served by the license-gated EE
// arena-controller. The core AgentRuntimeReconciler is intentionally NOT
// license-aware — this admission webhook is the only license gate for custom
// facades.
type AgentRuntimeCustomFacadeValidator struct {
	LicenseValidator *license.Validator
}

var agentruntimecustomfacadelog = logf.Log.WithName("agentruntime-customfacade-webhook")

// SetupAgentRuntimeCustomFacadeWebhookWithManager registers the custom-facade
// license webhook with the manager. Mirrors SetupArenaJobWebhookWithManager.
func SetupAgentRuntimeCustomFacadeWebhookWithManager(mgr ctrl.Manager, licenseValidator *license.Validator) error {
	return ctrl.NewWebhookManagedBy(mgr, &corev1alpha1.AgentRuntime{}).
		WithValidator(&AgentRuntimeCustomFacadeValidator{LicenseValidator: licenseValidator}).
		Complete()
}

// The deployed ValidatingWebhookConfiguration is the hand-written Helm template
// (charts/omnia/templates/enterprise/validatingwebhookconfiguration.yaml). That
// template scopes this webhook to AgentRuntimes that actually declare a custom
// facade via an API-server-side CEL matchCondition and pairs it with
// failurePolicy: Fail — so a down arena-controller makes the custom-facade
// feature UNAVAILABLE (matched objects are denied) while leaving every other
// AgentRuntime admission untouched. The marker below serves the kustomize path
// only; keep the two in sync.
//
// +kubebuilder:webhook:path=/validate-ee-omnia-altairalabs-ai-v1alpha1-agentruntime,mutating=false,failurePolicy=fail,sideEffects=None,groups=omnia.altairalabs.ai,resources=agentruntimes,verbs=create;update,versions=v1alpha1,name=vagentruntimecustomfacade.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*corev1alpha1.AgentRuntime] = &AgentRuntimeCustomFacadeValidator{}

// ValidateCreate implements admission.Validator.
func (v *AgentRuntimeCustomFacadeValidator) ValidateCreate(ctx context.Context, ar *corev1alpha1.AgentRuntime) (admission.Warnings, error) {
	agentruntimecustomfacadelog.Info("validating create", "name", ar.Name, "namespace", ar.Namespace)
	return v.validateLicense(ctx, ar)
}

// ValidateUpdate implements admission.Validator.
func (v *AgentRuntimeCustomFacadeValidator) ValidateUpdate(ctx context.Context, _ *corev1alpha1.AgentRuntime, ar *corev1alpha1.AgentRuntime) (admission.Warnings, error) {
	agentruntimecustomfacadelog.Info("validating update", "name", ar.Name, "namespace", ar.Namespace)
	return v.validateLicense(ctx, ar)
}

// ValidateDelete implements admission.Validator. No license check on delete.
func (v *AgentRuntimeCustomFacadeValidator) ValidateDelete(_ context.Context, _ *corev1alpha1.AgentRuntime) (admission.Warnings, error) {
	return nil, nil
}

// hasCustomFacade reports whether any facade declares type "custom".
func hasCustomFacade(ar *corev1alpha1.AgentRuntime) bool {
	for i := range ar.Spec.Facades {
		if ar.Spec.Facades[i].Type == corev1alpha1.FacadeTypeCustom {
			return true
		}
	}
	return false
}

// validateLicense denies the AgentRuntime when it declares a custom facade the
// license does not permit.
func (v *AgentRuntimeCustomFacadeValidator) validateLicense(ctx context.Context, ar *corev1alpha1.AgentRuntime) (admission.Warnings, error) {
	if v.LicenseValidator == nil {
		// No license validator configured, allow all.
		return nil, nil
	}
	if !hasCustomFacade(ar) {
		// No custom facade declared — nothing to gate.
		return nil, nil
	}

	if err := v.LicenseValidator.ValidateCustomFacade(ctx); err != nil {
		if licErr, ok := err.(*license.ValidationError); ok {
			agentruntimecustomfacadelog.Info("license validation failed",
				"name", ar.Name,
				"namespace", ar.Namespace,
				"feature", licErr.Feature,
			)
			return admission.Warnings{licErr.UpgradeMessage()}, fmt.Errorf("%s", licErr.Error())
		}
		return nil, err
	}

	return nil, nil
}
