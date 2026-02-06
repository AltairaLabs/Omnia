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

// ProviderCapability defines a capability that a provider supports.
// +kubebuilder:validation:Enum=text;streaming;vision;tools;json;audio;video;documents;duplex
type ProviderCapability string

const (
	// ProviderCapabilityText indicates the provider supports text generation.
	ProviderCapabilityText ProviderCapability = "text"
	// ProviderCapabilityStreaming indicates the provider supports streaming responses.
	ProviderCapabilityStreaming ProviderCapability = "streaming"
	// ProviderCapabilityVision indicates the provider supports image/vision inputs.
	ProviderCapabilityVision ProviderCapability = "vision"
	// ProviderCapabilityTools indicates the provider supports tool/function calling.
	ProviderCapabilityTools ProviderCapability = "tools"
	// ProviderCapabilityJSON indicates the provider supports structured JSON output.
	ProviderCapabilityJSON ProviderCapability = "json"
	// ProviderCapabilityAudio indicates the provider supports audio inputs/outputs.
	ProviderCapabilityAudio ProviderCapability = "audio"
	// ProviderCapabilityVideo indicates the provider supports video inputs.
	ProviderCapabilityVideo ProviderCapability = "video"
	// ProviderCapabilityDocuments indicates the provider supports document inputs.
	ProviderCapabilityDocuments ProviderCapability = "documents"
	// ProviderCapabilityDuplex indicates the provider supports full-duplex communication.
	ProviderCapabilityDuplex ProviderCapability = "duplex"
)

// SecretKeyRef references a key within a Secret.
type SecretKeyRef struct {
	// name is the name of the Secret.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// key is the key within the Secret to use.
	// If not specified, the provider-appropriate key is used:
	// - ANTHROPIC_API_KEY for Claude
	// - OPENAI_API_KEY for OpenAI
	// - GEMINI_API_KEY for Gemini
	// +optional
	Key *string `json:"key,omitempty"`
}

// ProviderSpec defines the desired state of Provider.
type ProviderSpec struct {
	// type specifies the provider type.
	// +kubebuilder:validation:Required
	Type ProviderType `json:"type"`

	// model specifies the model identifier (e.g., "claude-sonnet-4-20250514", "gpt-4o").
	// If not specified, the provider's default model is used.
	// +optional
	Model string `json:"model,omitempty"`

	// baseURL overrides the provider's default API endpoint.
	// Useful for proxies or self-hosted models.
	// +optional
	BaseURL string `json:"baseURL,omitempty"`

	// secretRef references a Secret containing API credentials.
	// Optional for providers that don't require credentials (e.g., mock, ollama).
	// +optional
	SecretRef *SecretKeyRef `json:"secretRef,omitempty"`

	// defaults contains provider tuning parameters.
	// +optional
	Defaults *ProviderDefaults `json:"defaults,omitempty"`

	// pricing configures cost tracking for this provider.
	// If not specified, PromptKit's built-in pricing is used.
	// +optional
	Pricing *ProviderPricing `json:"pricing,omitempty"`

	// validateCredentials enables credential validation on reconciliation.
	// When enabled, the controller attempts to verify credentials with the provider.
	// +kubebuilder:default=false
	// +optional
	ValidateCredentials bool `json:"validateCredentials,omitempty"`

	// capabilities lists what this provider supports.
	// Used for capability-based filtering when binding arena providers.
	// +optional
	Capabilities []ProviderCapability `json:"capabilities,omitempty"`
}

// ProviderPhase represents the current phase of the Provider.
// +kubebuilder:validation:Enum=Ready;Error
type ProviderPhase string

const (
	// ProviderPhaseReady indicates the provider is configured and ready.
	ProviderPhaseReady ProviderPhase = "Ready"
	// ProviderPhaseError indicates the provider has a configuration error.
	ProviderPhaseError ProviderPhase = "Error"
)

// ProviderStatus defines the observed state of Provider.
type ProviderStatus struct {
	// phase represents the current lifecycle phase of the Provider.
	// +optional
	Phase ProviderPhase `json:"phase,omitempty"`

	// conditions represent the current state of the Provider resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// lastValidatedAt is the timestamp of the last successful credential validation.
	// +optional
	LastValidatedAt *metav1.Time `json:"lastValidatedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.model`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Provider is the Schema for the providers API.
// It defines a reusable LLM provider configuration that can be referenced by AgentRuntimes.
type Provider struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Provider
	// +required
	Spec ProviderSpec `json:"spec"`

	// status defines the observed state of Provider
	// +optional
	Status ProviderStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ProviderList contains a list of Provider.
type ProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Provider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Provider{}, &ProviderList{})
}
