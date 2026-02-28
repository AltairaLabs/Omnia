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

// PolicyMode represents the enforcement mode of a ToolPolicy.
// +kubebuilder:validation:Enum=enforce;audit
type PolicyMode string

const (
	// PolicyModeEnforce blocks requests that match deny rules.
	PolicyModeEnforce PolicyMode = "enforce"
	// PolicyModeAudit logs violations but allows all requests through.
	PolicyModeAudit PolicyMode = "audit"
)

// OnFailureAction represents what to do when policy evaluation fails.
// +kubebuilder:validation:Enum=deny;allow
type OnFailureAction string

const (
	// OnFailureActionDeny blocks the request when evaluation fails.
	OnFailureActionDeny OnFailureAction = "deny"
	// OnFailureActionAllow allows the request when evaluation fails.
	OnFailureActionAllow OnFailureAction = "allow"
)

// ToolPolicyPhase represents the current phase of the policy.
// +kubebuilder:validation:Enum=Active;Error
type ToolPolicyPhase string

const (
	// ToolPolicyPhaseActive indicates all CEL expressions compiled successfully.
	ToolPolicyPhaseActive ToolPolicyPhase = "Active"
	// ToolPolicyPhaseError indicates one or more CEL expressions failed to compile.
	ToolPolicyPhaseError ToolPolicyPhase = "Error"
)

// ToolPolicySelector identifies which tools this policy applies to.
type ToolPolicySelector struct {
	// registry is the name of the ToolRegistry this policy applies to.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Registry string `json:"registry"`

	// tools is the list of tool names within the registry that this policy covers.
	// If empty, the policy applies to all tools in the registry.
	// +optional
	Tools []string `json:"tools,omitempty"`
}

// CELDenyRule defines a CEL expression that, when true, denies the request.
type CELDenyRule struct {
	// cel is the CEL expression to evaluate. When it returns true, the request is denied.
	// Available variables: headers (map<string, string>), body (map<string, dyn>).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	CEL string `json:"cel"`

	// message is the human-readable message returned when this rule denies a request.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Message string `json:"message"`
}

// PolicyRule defines a single policy rule with a deny condition.
type PolicyRule struct {
	// name is a unique identifier for this rule within the policy.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// description is an optional human-readable description of the rule's intent.
	// +optional
	Description string `json:"description,omitempty"`

	// deny defines the CEL expression that triggers request denial.
	// +kubebuilder:validation:Required
	Deny CELDenyRule `json:"deny"`
}

// RequiredClaim defines a claim that must be present in the request context.
type RequiredClaim struct {
	// claim is the name of the header or context claim that must be present.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Claim string `json:"claim"`

	// message is the error message when the claim is missing.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Message string `json:"message"`
}

// ToolPolicyAuditConfig configures audit logging for policy decisions.
type ToolPolicyAuditConfig struct {
	// logDecisions enables logging of all policy evaluation decisions.
	// +optional
	LogDecisions bool `json:"logDecisions,omitempty"`

	// redactFields lists field names whose values should be redacted in audit logs.
	// +optional
	RedactFields []string `json:"redactFields,omitempty"`
}

// ToolPolicySpec defines the desired state of ToolPolicy.
type ToolPolicySpec struct {
	// selector identifies which tools this policy applies to.
	// +kubebuilder:validation:Required
	Selector ToolPolicySelector `json:"selector"`

	// rules is the ordered list of deny rules to evaluate.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Rules []PolicyRule `json:"rules"`

	// requiredClaims lists claims that must be present in the request context.
	// +optional
	RequiredClaims []RequiredClaim `json:"requiredClaims,omitempty"`

	// mode controls whether the policy enforces or only audits.
	// +kubebuilder:default="enforce"
	// +optional
	Mode PolicyMode `json:"mode,omitempty"`

	// onFailure controls what happens when policy evaluation itself fails (e.g., CEL runtime error).
	// +kubebuilder:default="deny"
	// +optional
	OnFailure OnFailureAction `json:"onFailure,omitempty"`

	// audit configures decision logging and field redaction.
	// +optional
	Audit *ToolPolicyAuditConfig `json:"audit,omitempty"`
}

// ToolPolicyStatus defines the observed state of ToolPolicy.
type ToolPolicyStatus struct {
	// phase represents the current lifecycle phase of the policy.
	// +optional
	Phase ToolPolicyPhase `json:"phase,omitempty"`

	// observedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions represent the current state of the ToolPolicy resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ruleCount is the number of compiled rules in the policy.
	// +optional
	RuleCount int32 `json:"ruleCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Registry",type=string,JSONPath=`.spec.selector.registry`
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name="Rules",type=integer,JSONPath=`.status.ruleCount`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ToolPolicy is the Schema for the toolpolicies API.
// It defines CEL-based parameter validation rules for tool invocations.
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
