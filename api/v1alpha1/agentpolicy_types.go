/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentPolicySelector selects which agents this policy applies to.
type AgentPolicySelector struct {
	// agents is a list of AgentRuntime names this policy applies to.
	// If empty, the policy applies to all agents in the namespace.
	// +optional
	Agents []string `json:"agents,omitempty"`
}

// ClaimMappingEntry maps a single JWT claim to an outbound header.
type ClaimMappingEntry struct {
	// claim is the JWT claim name to extract (e.g., "team", "region", "customer_id").
	// Supports dot-notation for nested claims (e.g., "org.team").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Claim string `json:"claim"`

	// header is the HTTP header name to propagate the claim value as.
	// Must start with "X-Omnia-Claim-" prefix for security.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^X-Omnia-Claim-[A-Za-z0-9-]+$`
	Header string `json:"header"`
}

// ClaimMapping configures JWT claim extraction and header forwarding.
type ClaimMapping struct {
	// forwardClaims is the list of JWT claims to extract and forward as headers.
	// +optional
	// +listType=map
	// +listMapKey=claim
	ForwardClaims []ClaimMappingEntry `json:"forwardClaims,omitempty"`
}

// AgentPolicyMode defines how the agent policy is applied.
// +kubebuilder:validation:Enum=enforce;permissive
type AgentPolicyMode string

const (
	// AgentPolicyModeEnforce applies the policy with full enforcement.
	AgentPolicyModeEnforce AgentPolicyMode = "enforce"
	// AgentPolicyModePermissive logs policy decisions without blocking traffic.
	AgentPolicyModePermissive AgentPolicyMode = "permissive"
)

// AgentPolicySpec defines the desired state of AgentPolicy.
type AgentPolicySpec struct {
	// selector determines which agents this policy applies to.
	// +optional
	Selector *AgentPolicySelector `json:"selector,omitempty"`

	// claimMapping configures JWT claim extraction and header forwarding.
	// +optional
	ClaimMapping *ClaimMapping `json:"claimMapping,omitempty"`

	// mode is the enforcement mode: "enforce" (default) or "permissive".
	// In permissive mode, policy decisions are logged but traffic is not blocked.
	// +kubebuilder:default="enforce"
	// +optional
	Mode AgentPolicyMode `json:"mode,omitempty"`
}

// AgentPolicyPhase represents the current phase of the AgentPolicy.
// +kubebuilder:validation:Enum=Active;Error
type AgentPolicyPhase string

const (
	// AgentPolicyPhaseActive indicates the policy is active and applied.
	AgentPolicyPhaseActive AgentPolicyPhase = "Active"
	// AgentPolicyPhaseError indicates the policy has a configuration error.
	AgentPolicyPhaseError AgentPolicyPhase = "Error"
)

// AgentPolicyStatus defines the observed state of AgentPolicy.
type AgentPolicyStatus struct {
	// phase represents the current lifecycle phase of the AgentPolicy.
	// +optional
	Phase AgentPolicyPhase `json:"phase,omitempty"`

	// matchedAgents is the count of agents matched by the selector.
	// +optional
	MatchedAgents int32 `json:"matchedAgents,omitempty"`

	// conditions represent the current state of the AgentPolicy resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Matched",type=integer,JSONPath=`.status.matchedAgents`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AgentPolicy defines claim-to-header mapping rules for agent communication.
// It configures how JWT claims are extracted and propagated as headers through
// the facade -> runtime -> tool adapter pipeline.
type AgentPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AgentPolicy
	// +required
	Spec AgentPolicySpec `json:"spec"`

	// status defines the observed state of AgentPolicy
	// +optional
	Status AgentPolicyStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AgentPolicyList contains a list of AgentPolicy.
type AgentPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AgentPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentPolicy{}, &AgentPolicyList{})
}
