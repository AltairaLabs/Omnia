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

	// Default parameters for this provider.
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"topP,omitempty"`
	MaxTokens   int     `json:"maxTokens,omitempty"`
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
