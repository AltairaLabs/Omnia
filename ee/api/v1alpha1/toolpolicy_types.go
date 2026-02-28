/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PolicyMode defines how the policy is applied.
// +kubebuilder:validation:Enum=enforce;audit
type PolicyMode string

const (
	// PolicyModeEnforce blocks requests that violate the policy.
	PolicyModeEnforce PolicyMode = "enforce"
	// PolicyModeAudit logs violations but allows requests through.
	PolicyModeAudit PolicyMode = "audit"
)

// OnFailureAction defines what happens when policy evaluation fails.
// +kubebuilder:validation:Enum=deny;allow
type OnFailureAction string

const (
	// OnFailureDeny denies the request when policy evaluation fails.
	OnFailureDeny OnFailureAction = "deny"
	// OnFailureAllow allows the request when policy evaluation fails.
	OnFailureAllow OnFailureAction = "allow"
)

// ToolPolicySelector defines which tools this policy applies to.
type ToolPolicySelector struct {
	// registry is the name of the ToolRegistry to match.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Registry string `json:"registry"`

	// tools is a list of specific tool names to match.
	// If empty, the policy applies to all tools in the registry.
	// +optional
	Tools []string `json:"tools,omitempty"`
}

// PolicyRuleDeny defines the CEL expression and message for a deny rule.
type PolicyRuleDeny struct {
	// cel is a CEL expression that, when true, denies the request.
	// Available variables: headers (map<string, string>), body (map<string, dyn>).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	CEL string `json:"cel"`

	// message is a human-readable message returned when the rule denies a request.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Message string `json:"message"`
}

// PolicyRule defines a single policy rule evaluated via CEL.
type PolicyRule struct {
	// name is a unique identifier for this rule within the policy.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// description is a human-readable description of the rule's purpose.
	// +optional
	Description string `json:"description,omitempty"`

	// deny defines the CEL expression and message for denying requests.
	// +kubebuilder:validation:Required
	Deny PolicyRuleDeny `json:"deny"`
}

// RequiredClaim defines a claim that must be present in request headers.
type RequiredClaim struct {
	// claim is the name of the claim (maps to X-Omnia-Claim-<Claim> header).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Claim string `json:"claim"`

	// message is the error message returned when the claim is missing.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Message string `json:"message"`
}

// ToolPolicyAuditConfig configures audit logging for the policy.
type ToolPolicyAuditConfig struct {
	// logDecisions enables logging of all policy decisions (allow/deny).
	// +optional
	LogDecisions bool `json:"logDecisions,omitempty"`

	// redactFields lists field names whose values should be redacted in audit logs.
	// +optional
	RedactFields []string `json:"redactFields,omitempty"`
}

// ToolPolicySpec defines the desired state of ToolPolicy.
type ToolPolicySpec struct {
	// selector defines which tools this policy applies to.
	// +kubebuilder:validation:Required
	Selector ToolPolicySelector `json:"selector"`

	// rules defines the CEL-based policy rules to evaluate.
	// Rules are evaluated in order; the first deny stops evaluation.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:Required
	Rules []PolicyRule `json:"rules"`

	// requiredClaims defines claims that must be present in request headers.
	// +optional
	RequiredClaims []RequiredClaim `json:"requiredClaims,omitempty"`

	// mode determines whether the policy enforces or only audits violations.
	// +kubebuilder:default="enforce"
	// +optional
	Mode PolicyMode `json:"mode,omitempty"`

	// onFailure determines what happens when policy evaluation encounters an error.
	// +kubebuilder:default="deny"
	// +optional
	OnFailure OnFailureAction `json:"onFailure,omitempty"`

	// audit configures audit logging for policy decisions.
	// +optional
	Audit *ToolPolicyAuditConfig `json:"audit,omitempty"`
}

// ToolPolicyPhase represents the current phase of the ToolPolicy.
// +kubebuilder:validation:Enum=Active;Error
type ToolPolicyPhase string

const (
	// ToolPolicyPhaseActive indicates the policy is valid and active.
	ToolPolicyPhaseActive ToolPolicyPhase = "Active"
	// ToolPolicyPhaseError indicates the policy has a configuration error.
	ToolPolicyPhaseError ToolPolicyPhase = "Error"
)

// ToolPolicyStatus defines the observed state of ToolPolicy.
type ToolPolicyStatus struct {
	// phase represents the current lifecycle phase of the policy.
	// +optional
	Phase ToolPolicyPhase `json:"phase,omitempty"`

	// conditions represent the current state of the policy.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ruleCount is the number of compiled rules.
	// +optional
	RuleCount int32 `json:"ruleCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Registry",type=string,JSONPath=`.spec.selector.registry`
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Rules",type=integer,JSONPath=`.status.ruleCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ToolPolicy is the Schema for the toolpolicies API.
// It defines CEL-based access control rules for tool invocations.
type ToolPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of ToolPolicy
	// +required
	Spec ToolPolicySpec `json:"spec"`

	// status defines the observed state of ToolPolicy
	// +optional
	Status ToolPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ToolPolicyList contains a list of ToolPolicy.
type ToolPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ToolPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ToolPolicy{}, &ToolPolicyList{})
}
