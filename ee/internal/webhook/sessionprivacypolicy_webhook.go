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

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// SessionPrivacyPolicyValidator validates SessionPrivacyPolicy resources
// against inheritance rules (child policies can only be stricter than parents).
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
func (v *SessionPrivacyPolicyValidator) ValidateCreate(ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy) (admission.Warnings, error) {
	privacypolicylog.Info("validating create", "name", policy.Name)
	return v.validateInheritance(ctx, policy)
}

// ValidateUpdate implements admission.Validator.
func (v *SessionPrivacyPolicyValidator) ValidateUpdate(ctx context.Context, _ *omniav1alpha1.SessionPrivacyPolicy, policy *omniav1alpha1.SessionPrivacyPolicy) (admission.Warnings, error) {
	privacypolicylog.Info("validating update", "name", policy.Name)
	return v.validateInheritance(ctx, policy)
}

// ValidateDelete implements admission.Validator.
func (v *SessionPrivacyPolicyValidator) ValidateDelete(ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy) (admission.Warnings, error) {
	privacypolicylog.Info("validating delete", "name", policy.Name)

	if policy.Spec.Level != omniav1alpha1.PolicyLevelGlobal {
		return nil, nil
	}

	// Reject deletion of the last global policy
	var list omniav1alpha1.SessionPrivacyPolicyList
	if err := v.Client.List(ctx, &list); err != nil {
		return nil, fmt.Errorf("failed to list policies: %w", err)
	}

	globalCount := 0
	for i := range list.Items {
		if list.Items[i].Spec.Level == omniav1alpha1.PolicyLevelGlobal {
			globalCount++
		}
	}

	if globalCount <= 1 {
		return nil, fmt.Errorf("cannot delete the last global-level SessionPrivacyPolicy")
	}

	return nil, nil
}

// validateInheritance checks that a child policy is not less restrictive than its parent.
func (v *SessionPrivacyPolicyValidator) validateInheritance(ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy) (admission.Warnings, error) {
	if policy.Spec.Level == omniav1alpha1.PolicyLevelGlobal {
		// Global policies have no parent to validate against
		return nil, nil
	}

	parent, err := v.findParentPolicy(ctx, policy)
	if err != nil {
		return nil, fmt.Errorf("failed to find parent policy: %w", err)
	}
	if parent == nil {
		// No parent found — allow with warning; controller will mark Error in status
		return admission.Warnings{"no parent policy found; policy will be in Error phase until a parent exists"}, nil
	}

	// Validate that child is not less restrictive than parent
	if errs := validateStricterThanParent(policy, parent); len(errs) > 0 {
		return nil, fmt.Errorf("policy is less restrictive than parent (%s-level %q): %v", parent.Spec.Level, parent.Name, errs)
	}

	return nil, nil
}

// findParentPolicy locates the applicable parent policy for inheritance validation.
// For workspace-level: finds the global policy.
// For agent-level: finds the workspace policy (if workspaceRef on agent matches), falling back to global.
func (v *SessionPrivacyPolicyValidator) findParentPolicy(ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy) (*omniav1alpha1.SessionPrivacyPolicy, error) {
	var list omniav1alpha1.SessionPrivacyPolicyList
	if err := v.Client.List(ctx, &list); err != nil {
		return nil, err
	}

	switch policy.Spec.Level {
	case omniav1alpha1.PolicyLevelWorkspace:
		// Parent is the global policy
		return findPolicyByLevel(list.Items, omniav1alpha1.PolicyLevelGlobal, ""), nil

	case omniav1alpha1.PolicyLevelAgent:
		// Try to find a workspace-level parent first
		wsName := ""
		if policy.Spec.AgentRef != nil {
			wsName = policy.Spec.AgentRef.Namespace
		}
		if ws := findPolicyByLevel(list.Items, omniav1alpha1.PolicyLevelWorkspace, wsName); ws != nil {
			return ws, nil
		}
		// Fall back to global
		return findPolicyByLevel(list.Items, omniav1alpha1.PolicyLevelGlobal, ""), nil
	}

	return nil, nil
}

// findPolicyByLevel finds the first policy matching the given level.
// For workspace-level, optionally matches by workspace name.
func findPolicyByLevel(policies []omniav1alpha1.SessionPrivacyPolicy, level omniav1alpha1.PolicyLevel, workspaceName string) *omniav1alpha1.SessionPrivacyPolicy {
	for i := range policies {
		p := &policies[i]
		if p.Spec.Level != level {
			continue
		}
		if level == omniav1alpha1.PolicyLevelWorkspace && workspaceName != "" {
			if p.Spec.WorkspaceRef == nil || p.Spec.WorkspaceRef.Name != workspaceName {
				continue
			}
		}
		return p
	}
	return nil
}

// validateStricterThanParent returns a list of violations where the child is less restrictive.
func validateStricterThanParent(child, parent *omniav1alpha1.SessionPrivacyPolicy) []string {
	var errs []string

	// Cannot enable recording if parent disables it
	if !parent.Spec.Recording.Enabled && child.Spec.Recording.Enabled {
		errs = append(errs, "cannot enable recording when parent disables it")
	}

	// Cannot enable richData if parent disables it
	if !parent.Spec.Recording.RichData && child.Spec.Recording.RichData {
		errs = append(errs, "cannot enable recording.richData when parent disables it")
	}

	// Cannot disable PII redaction if parent enables it
	if parent.Spec.Recording.PII != nil && parent.Spec.Recording.PII.Redact {
		if child.Spec.Recording.PII == nil || !child.Spec.Recording.PII.Redact {
			errs = append(errs, "cannot disable recording.pii.redact when parent enables it")
		}
	}

	// Cannot disable userOptOut if parent enables it
	if parent.Spec.UserOptOut != nil && parent.Spec.UserOptOut.Enabled {
		if child.Spec.UserOptOut == nil || !child.Spec.UserOptOut.Enabled {
			errs = append(errs, "cannot disable userOptOut when parent enables it")
		}
	}

	// Retention days cannot exceed parent's days
	if parent.Spec.Retention != nil && child.Spec.Retention != nil {
		errs = append(errs, validateRetentionNotExceeded(child.Spec.Retention, parent.Spec.Retention)...)
	}

	return errs
}

// validateRetentionNotExceeded checks that child retention does not exceed parent retention.
func validateRetentionNotExceeded(child, parent *omniav1alpha1.PrivacyRetentionConfig) []string {
	var errs []string

	if parent.Facade != nil && child.Facade != nil {
		if parent.Facade.WarmDays != nil && child.Facade.WarmDays != nil && *child.Facade.WarmDays > *parent.Facade.WarmDays {
			errs = append(errs, fmt.Sprintf("retention.facade.warmDays (%d) exceeds parent (%d)", *child.Facade.WarmDays, *parent.Facade.WarmDays))
		}
		if parent.Facade.ColdDays != nil && child.Facade.ColdDays != nil && *child.Facade.ColdDays > *parent.Facade.ColdDays {
			errs = append(errs, fmt.Sprintf("retention.facade.coldDays (%d) exceeds parent (%d)", *child.Facade.ColdDays, *parent.Facade.ColdDays))
		}
	}

	if parent.RichData != nil && child.RichData != nil {
		if parent.RichData.WarmDays != nil && child.RichData.WarmDays != nil && *child.RichData.WarmDays > *parent.RichData.WarmDays {
			errs = append(errs, fmt.Sprintf("retention.richData.warmDays (%d) exceeds parent (%d)", *child.RichData.WarmDays, *parent.RichData.WarmDays))
		}
		if parent.RichData.ColdDays != nil && child.RichData.ColdDays != nil && *child.RichData.ColdDays > *parent.RichData.ColdDays {
			errs = append(errs, fmt.Sprintf("retention.richData.coldDays (%d) exceeds parent (%d)", *child.RichData.ColdDays, *parent.RichData.ColdDays))
		}
	}

	return errs
}
