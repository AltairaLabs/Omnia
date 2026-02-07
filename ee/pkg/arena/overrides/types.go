/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

// Package overrides provides types for ArenaJob provider and tool override configuration.
// The controller creates a ConfigMap containing OverrideConfig which workers read
// to inject provider and tool configurations into the arena engine.
package overrides

// OverrideConfig is the structure written to ConfigMap and read by worker.
// It contains resolved provider and tool configurations from Provider/ToolRegistry CRDs.
type OverrideConfig struct {
	// Providers maps group name (e.g., "default", "judge") to provider configs.
	// Groups correspond to ArenaJob.spec.providerOverrides keys.
	Providers map[string][]ProviderOverride `json:"providers,omitempty"`

	// Bindings maps "{namespace}/{name}" to ProviderOverride for annotation-based
	// credential resolution. The worker uses this registry to match arena provider
	// YAML files (with binding annotations) to their corresponding Provider CRDs.
	// Old workers without binding support ignore this field.
	Bindings map[string]ProviderOverride `json:"bindings,omitempty"`

	// Tools contains tool override configurations from ToolRegistry CRDs.
	Tools []ToolOverride `json:"tools,omitempty"`
}

// ProviderOverride contains provider config resolved from a Provider CRD.
// This structure is designed to map directly to PromptKit's Provider config format.
type ProviderOverride struct {
	// ID is the unique identifier for this provider (typically the Provider CRD name).
	ID string `json:"id"`

	// Type is the provider type (e.g., "claude", "openai", "ollama", "mock").
	Type string `json:"type"`

	// Model is the model identifier (e.g., "claude-sonnet-4-20250514", "gpt-4o").
	Model string `json:"model,omitempty"`

	// BaseURL overrides the provider's default API endpoint.
	// Required for ollama and vllm providers.
	BaseURL string `json:"baseURL,omitempty"`

	// SecretEnvVar is the environment variable name containing the API key.
	// The controller injects secrets as env vars; this tells the worker which var to use.
	// Empty for providers that don't require credentials (ollama, mock).
	SecretEnvVar string `json:"secretEnvVar,omitempty"`

	// CredentialFile is a file path containing the credential (mounted in pod).
	CredentialFile string `json:"credentialFile,omitempty"`

	// AuthMethod is the authentication method (workloadIdentity, accessKey, etc.).
	AuthMethod string `json:"authMethod,omitempty"`

	// RoleARN is the AWS IAM role ARN for IRSA.
	RoleARN string `json:"roleArn,omitempty"`

	// ServiceAccountEmail is the GCP service account email for workload identity.
	ServiceAccountEmail string `json:"serviceAccountEmail,omitempty"`

	// Platform contains hyperscaler platform configuration.
	Platform *PlatformOverride `json:"platform,omitempty"`

	// Default parameters for this provider.
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"topP,omitempty"`
	MaxTokens   int     `json:"maxTokens,omitempty"`
}

// PlatformOverride contains resolved platform configuration from a Provider CRD.
type PlatformOverride struct {
	// Type is the cloud platform type (aws, gcp, azure).
	Type string `json:"type"`

	// Region is the cloud region.
	Region string `json:"region,omitempty"`

	// Project is the GCP project ID.
	Project string `json:"project,omitempty"`

	// Endpoint overrides the default platform API endpoint.
	Endpoint string `json:"endpoint,omitempty"`
}

// ToolOverride contains tool config resolved from a ToolRegistry CRD.
type ToolOverride struct {
	// Name is the tool name (must match tool name in arena config).
	Name string `json:"name"`

	// Description of the tool (optional, used if provided).
	Description string `json:"description,omitempty"`

	// Endpoint is the URL where the tool handler can be reached.
	Endpoint string `json:"endpoint"`

	// HandlerType is the type of handler (http, grpc, mcp, openapi).
	HandlerType string `json:"handlerType,omitempty"`

	// RegistryName is the name of the ToolRegistry CRD this came from.
	RegistryName string `json:"registryName,omitempty"`

	// HandlerName is the name of the handler in the ToolRegistry.
	HandlerName string `json:"handlerName,omitempty"`
}

// ConfigMapKey is the key used in the ConfigMap's data field.
const ConfigMapKey = "overrides.json"
