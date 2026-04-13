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
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// SessionPrivacyPolicyValidator validates SessionPrivacyPolicy resources.
// Create and Update are accepted at the webhook layer (CEL handles structural
// validation in the CRD). Delete is rejected when any consumer still references
// the policy.
type SessionPrivacyPolicyValidator struct {
	Client client.Reader
}

var privacypolicylog = logf.Log.WithName("sessionprivacypolicy-webhook")

// SetupSessionPrivacyPolicyWebhookWithManager registers the SessionPrivacyPolicy webhook with the manager.
func SetupSessionPrivacyPolicyWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &omniav1alpha1.SessionPrivacyPolicy{}).
		WithValidator(&SessionPrivacyPolicyValidator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-omnia-altairalabs-ai-v1alpha1-sessionprivacypolicy,mutating=false,failurePolicy=fail,sideEffects=None,groups=omnia.altairalabs.ai,resources=sessionprivacypolicies,verbs=create;update;delete,versions=v1alpha1,name=vsessionprivacypolicy.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*omniav1alpha1.SessionPrivacyPolicy] = &SessionPrivacyPolicyValidator{}

// ValidateCreate implements admission.Validator.
// Structural validation is handled by CEL rules in the CRD.
func (v *SessionPrivacyPolicyValidator) ValidateCreate(_ context.Context, policy *omniav1alpha1.SessionPrivacyPolicy) (admission.Warnings, error) {
	privacypolicylog.Info("validating create", "name", policy.Name, "namespace", policy.Namespace)
	return nil, nil
}

// ValidateUpdate implements admission.Validator.
// Structural validation is handled by CEL rules in the CRD.
func (v *SessionPrivacyPolicyValidator) ValidateUpdate(_ context.Context, _ *omniav1alpha1.SessionPrivacyPolicy, policy *omniav1alpha1.SessionPrivacyPolicy) (admission.Warnings, error) {
	privacypolicylog.Info("validating update", "name", policy.Name, "namespace", policy.Namespace)
	return nil, nil
}

// ValidateDelete implements admission.Validator.
// Rejects deletion if any Workspace service group or AgentRuntime references this policy.
func (v *SessionPrivacyPolicyValidator) ValidateDelete(ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy) (admission.Warnings, error) {
	privacypolicylog.Info("validating delete", "name", policy.Name, "namespace", policy.Namespace)

	if err := v.checkWorkspaceReferences(ctx, policy); err != nil {
		return nil, err
	}
	if err := v.checkAgentRuntimeReferences(ctx, policy); err != nil {
		return nil, err
	}
	return nil, nil
}

// checkWorkspaceReferences rejects deletion if any Workspace service group references the policy.
func (v *SessionPrivacyPolicyValidator) checkWorkspaceReferences(ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy) error {
	var wsList corev1alpha1.WorkspaceList
	if err := v.Client.List(ctx, &wsList); err != nil {
		return fmt.Errorf("failed to list workspaces: %w", err)
	}

	for i := range wsList.Items {
		ws := &wsList.Items[i]
		if ws.Spec.Namespace.Name != policy.Namespace {
			continue
		}
		for _, sg := range ws.Spec.Services {
			if sg.PrivacyPolicyRef != nil && sg.PrivacyPolicyRef.Name == policy.Name {
				return fmt.Errorf("policy %q is referenced by Workspace %s service group %q and cannot be deleted",
					policy.Name, ws.Name, sg.Name)
			}
		}
	}
	return nil
}

// checkAgentRuntimeReferences rejects deletion if any AgentRuntime in the policy's namespace references it.
func (v *SessionPrivacyPolicyValidator) checkAgentRuntimeReferences(ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy) error {
	var arList corev1alpha1.AgentRuntimeList
	if err := v.Client.List(ctx, &arList, client.InNamespace(policy.Namespace)); err != nil {
		return fmt.Errorf("failed to list agentruntimes: %w", err)
	}

	for i := range arList.Items {
		ar := &arList.Items[i]
		if ar.Spec.PrivacyPolicyRef != nil && ar.Spec.PrivacyPolicyRef.Name == policy.Name {
			return fmt.Errorf("policy %q is referenced by AgentRuntime %s/%s and cannot be deleted",
				policy.Name, ar.Namespace, ar.Name)
		}
	}
	return nil
}
