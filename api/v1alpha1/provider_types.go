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

// CredentialConfig defines how to obtain credentials for this provider.
// Exactly one field must be specified.
// +kubebuilder:validation:XValidation:rule="(has(self.secretRef) ? 1 : 0) + (has(self.envVar) ? 1 : 0) + (has(self.filePath) ? 1 : 0) <= 1",message="at most one credential method may be specified"
type CredentialConfig struct {
	// secretRef references a Kubernetes Secret containing the credential.
	// +optional
	SecretRef *SecretKeyRef `json:"secretRef,omitempty"`

	// envVar specifies an environment variable name containing the credential.
	// The variable must be available in the runtime pod.
	// +optional
	// +kubebuilder:validation:Pattern=`^[A-Za-z_][A-Za-z0-9_]*$`
	EnvVar string `json:"envVar,omitempty"`

	// filePath specifies a path to a file containing the credential.
	// The file must be mounted in the runtime pod.
	// +optional
	// +kubebuilder:validation:Pattern=`^/.*`
	FilePath string `json:"filePath,omitempty"`
}

// PlatformType defines the hyperscaler hosting platform for a base provider.
// Values are hosting-layer names (matching PromptKit's PlatformConfig.Type):
//   - "bedrock" hosts claude on AWS
//   - "vertex" hosts gemini on GCP
//   - "azure"  hosts openai on Azure AI Foundry
//
// +kubebuilder:validation:Enum=bedrock;vertex;azure
type PlatformType string

const (
	// PlatformTypeBedrock hosts the provider on AWS Bedrock.
	PlatformTypeBedrock PlatformType = "bedrock"
	// PlatformTypeVertex hosts the provider on GCP Vertex AI.
	PlatformTypeVertex PlatformType = "vertex"
	// PlatformTypeAzure hosts the provider on Azure AI Foundry.
	PlatformTypeAzure PlatformType = "azure"
)

// PlatformConfig defines hyperscaler-specific configuration.
// +kubebuilder:validation:XValidation:rule="self.type != 'vertex' || size(self.project) > 0",message="project is required when platform.type is vertex"
// +kubebuilder:validation:XValidation:rule="self.type != 'azure' || size(self.endpoint) > 0",message="endpoint is required when platform.type is azure"
type PlatformConfig struct {
	// type is the hyperscaler hosting platform.
	// +kubebuilder:validation:Required
	Type PlatformType `json:"type"`

	// region is the cloud region (e.g., us-east-1, us-central1, eastus).
	// Required for bedrock and vertex; ignored for azure (inferred from endpoint).
	// +optional
	Region string `json:"region,omitempty"`

	// project is the GCP project ID. Required for vertex.
	// +optional
	Project string `json:"project,omitempty"`

	// endpoint overrides the default platform API endpoint.
	// Required for azure (the Azure OpenAI resource URL).
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
}

// AuthMethod defines the authentication method for hyperscaler providers.
// +kubebuilder:validation:Enum=workloadIdentity;accessKey;serviceAccount;servicePrincipal
type AuthMethod string

const (
	// AuthMethodWorkloadIdentity uses Kubernetes-native identity federation (IRSA, GCP WI, Azure AD WI).
	AuthMethodWorkloadIdentity AuthMethod = "workloadIdentity"
	// AuthMethodAccessKey uses static access key credentials (e.g., AWS access key).
	AuthMethodAccessKey AuthMethod = "accessKey"
	// AuthMethodServiceAccount uses GCP service account key credentials.
	AuthMethodServiceAccount AuthMethod = "serviceAccount"
	// AuthMethodServicePrincipal uses Azure service principal credentials.
	AuthMethodServicePrincipal AuthMethod = "servicePrincipal"
)

// AuthConfig defines authentication configuration for hyperscaler platforms.
// +kubebuilder:validation:XValidation:rule="self.type != 'workloadIdentity' || !has(self.credentialsSecretRef)",message="credentialsSecretRef is not used with workloadIdentity auth"
type AuthConfig struct {
	// type is the authentication method.
	// +kubebuilder:validation:Required
	Type AuthMethod `json:"type"`

	// roleArn is the AWS IAM role ARN for IRSA (optional override).
	// Only applicable when platform.type is bedrock.
	// +optional
	RoleArn string `json:"roleArn,omitempty"`

	// serviceAccountEmail is the GCP service account email for workload identity.
	// Only applicable when platform.type is vertex.
	// +optional
	ServiceAccountEmail string `json:"serviceAccountEmail,omitempty"`

	// credentialsSecretRef references a secret containing platform credentials.
	// Required for accessKey, serviceAccount, and servicePrincipal auth types.
	// Not used with workloadIdentity.
	//
	// Expected secret keys per auth type:
	//   accessKey        (bedrock):  AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY
	//   serviceAccount   (vertex):   credentials.json (GCP SA key JSON)
	//   servicePrincipal (azure):    AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_CLIENT_SECRET
	// +optional
	CredentialsSecretRef *SecretKeyRef `json:"credentialsSecretRef,omitempty"`
}

// ProviderSpec defines the desired state of Provider.
// +kubebuilder:validation:XValidation:rule="!has(self.platform) || (self.type in ['claude', 'openai', 'gemini'])",message="platform is only valid for provider types claude, openai, or gemini"
// +kubebuilder:validation:XValidation:rule="has(self.platform) == has(self.auth)",message="spec.platform and spec.auth must be set together"
// +kubebuilder:validation:XValidation:rule="!has(self.platform) || self.platform.type != 'bedrock' || self.auth.type in ['workloadIdentity', 'accessKey']",message="platform.type bedrock requires auth.type of workloadIdentity or accessKey"
// +kubebuilder:validation:XValidation:rule="!has(self.platform) || self.platform.type != 'vertex' || self.auth.type in ['workloadIdentity', 'serviceAccount']",message="platform.type vertex requires auth.type of workloadIdentity or serviceAccount"
// +kubebuilder:validation:XValidation:rule="!has(self.platform) || self.platform.type != 'azure' || self.auth.type in ['workloadIdentity', 'servicePrincipal']",message="platform.type azure requires auth.type of workloadIdentity or servicePrincipal"
// +kubebuilder:validation:XValidation:rule="!(has(self.auth) && self.auth.type != 'workloadIdentity') || has(self.auth.credentialsSecretRef)",message="credentialsSecretRef is required for non-workloadIdentity auth types"
// +kubebuilder:validation:XValidation:rule="!has(self.platform) || self.platform.type != 'vertex' || self.type != 'openai'",message="openai on vertex is not supported: Vertex AI does not host OpenAI as a partner"
// +kubebuilder:validation:XValidation:rule="!has(self.platform) || self.platform.type != 'bedrock' || self.type != 'gemini'",message="gemini on bedrock is not supported: AWS Bedrock does not host Gemini"
// +kubebuilder:validation:XValidation:rule="!has(self.platform) || self.platform.type != 'azure' || self.type != 'gemini'",message="gemini on azure is not supported: Azure AI Foundry does not host Gemini"
type ProviderSpec struct {
	// type specifies the provider wire protocol.
	// +kubebuilder:validation:Required
	Type ProviderType `json:"type"`

	// model specifies the model identifier (e.g., "claude-sonnet-4-20250514", "gpt-4o").
	// If not specified, the provider's default model is used.
	// When platform.type is bedrock, a claude release name is auto-mapped to the
	// corresponding Bedrock model ID by PromptKit.
	// +optional
	Model string `json:"model,omitempty"`

	// baseURL overrides the provider's default API endpoint.
	// Useful for proxies, gateways (OpenRouter), or self-hosted models.
	// +optional
	BaseURL string `json:"baseURL,omitempty"`

	// headers contains custom HTTP headers to include on every provider request.
	// Useful for gateway providers such as OpenRouter that require custom
	// attribution headers, or for tenant routing in shared vLLM deployments.
	// Collisions with built-in provider headers are rejected by PromptKit.
	// +optional
	Headers map[string]string `json:"headers,omitempty"`

	// platform defines hyperscaler hosting configuration.
	// Supported provider × platform pairs (PromptKit v1.4.6+):
	//   claude:  bedrock, vertex, azure
	//   openai:  azure, bedrock
	//   gemini:  vertex
	// Three pairs are rejected at admission because the hyperscaler does
	// not host the model vendor as a partner endpoint:
	//   openai × vertex, gemini × bedrock, gemini × azure.
	// Auth method is constrained by platform, not by provider type (see
	// spec.auth).
	// +optional
	Platform *PlatformConfig `json:"platform,omitempty"`

	// auth defines authentication configuration for hyperscaler platforms.
	// Required when spec.platform is set; forbidden otherwise.
	// +optional
	Auth *AuthConfig `json:"auth,omitempty"`

	// credential defines how to obtain credentials for this provider.
	// Optional for providers that don't require credentials (e.g., mock,
	// ollama, vllm).
	// +optional
	Credential *CredentialConfig `json:"credential,omitempty"`

	// defaults contains provider tuning parameters.
	// +optional
	Defaults *ProviderDefaults `json:"defaults,omitempty"`

	// pricing configures cost tracking for this provider.
	// If not specified, PromptKit's built-in pricing is used.
	// +optional
	Pricing *ProviderPricing `json:"pricing,omitempty"`

	// capabilities lists what this provider supports.
	// Used for capability-based filtering when binding arena providers.
	// +optional
	Capabilities []ProviderCapability `json:"capabilities,omitempty"`
}

// ProviderPhase represents the current phase of the Provider.
// +kubebuilder:validation:Enum=Ready;Error;Unavailable
type ProviderPhase string

const (
	// ProviderPhaseReady indicates the provider is configured and reachable.
	ProviderPhaseReady ProviderPhase = "Ready"
	// ProviderPhaseError indicates the provider has a configuration error.
	ProviderPhaseError ProviderPhase = "Error"
	// ProviderPhaseUnavailable indicates the provider endpoint is not reachable.
	ProviderPhaseUnavailable ProviderPhase = "Unavailable"
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
