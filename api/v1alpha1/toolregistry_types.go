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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HandlerType defines the type of tool handler
// +kubebuilder:validation:Enum=http;openapi;grpc;mcp
type HandlerType string

const (
	// HandlerTypeHTTP indicates a plain HTTP endpoint (requires tool definition)
	HandlerTypeHTTP HandlerType = "http"
	// HandlerTypeOpenAPI indicates an OpenAPI-described HTTP service (self-describing)
	HandlerTypeOpenAPI HandlerType = "openapi"
	// HandlerTypeGRPC indicates a gRPC service (requires tool definition)
	HandlerTypeGRPC HandlerType = "grpc"
	// HandlerTypeMCP indicates a Model Context Protocol server (self-describing)
	HandlerTypeMCP HandlerType = "mcp"
)

// MCPTransport defines the transport type for MCP connections
// +kubebuilder:validation:Enum=sse;stdio
type MCPTransport string

const (
	// MCPTransportSSE indicates connection via Server-Sent Events
	MCPTransportSSE MCPTransport = "sse"
	// MCPTransportStdio indicates connection via spawned subprocess stdin/stdout
	MCPTransportStdio MCPTransport = "stdio"
)

// MCPConfig contains MCP-specific handler configuration
type MCPConfig struct {
	// transport specifies the MCP transport type.
	// +kubebuilder:validation:Required
	Transport MCPTransport `json:"transport"`

	// endpoint is the SSE server URL (required for SSE transport).
	// +optional
	Endpoint *string `json:"endpoint,omitempty"`

	// command is the command to run for stdio transport.
	// +optional
	Command *string `json:"command,omitempty"`

	// args are the command arguments for stdio transport.
	// +optional
	Args []string `json:"args,omitempty"`

	// workDir is the working directory for stdio transport.
	// +optional
	WorkDir *string `json:"workDir,omitempty"`

	// env are additional environment variables for stdio transport.
	// +optional
	Env map[string]string `json:"env,omitempty"`
}

// OpenAPIConfig contains OpenAPI-specific handler configuration
type OpenAPIConfig struct {
	// specURL is the URL to the OpenAPI specification (JSON or YAML).
	// +kubebuilder:validation:Required
	SpecURL string `json:"specURL"`

	// baseURL overrides the base URL from the OpenAPI spec.
	// If not specified, uses the first server URL from the spec.
	// +optional
	BaseURL *string `json:"baseURL,omitempty"`

	// operationFilter limits which operations are exposed as tools.
	// If empty, all operations are exposed.
	// +optional
	OperationFilter []string `json:"operationFilter,omitempty"`
}

// GRPCConfig contains gRPC-specific handler configuration
type GRPCConfig struct {
	// endpoint is the gRPC server address (host:port).
	// +kubebuilder:validation:Required
	Endpoint string `json:"endpoint"`

	// tls enables TLS for the connection.
	// +optional
	TLS bool `json:"tls,omitempty"`

	// tlsCertPath is the path to the TLS certificate.
	// +optional
	TLSCertPath *string `json:"tlsCertPath,omitempty"`

	// tlsKeyPath is the path to the TLS key.
	// +optional
	TLSKeyPath *string `json:"tlsKeyPath,omitempty"`

	// tlsCAPath is the path to the CA certificate.
	// +optional
	TLSCAPath *string `json:"tlsCAPath,omitempty"`

	// tlsInsecureSkipVerify skips TLS verification (not recommended for production).
	// +optional
	TLSInsecureSkipVerify bool `json:"tlsInsecureSkipVerify,omitempty"`
}

// HTTPConfig contains HTTP-specific handler configuration
type HTTPConfig struct {
	// endpoint is the HTTP endpoint URL.
	// +kubebuilder:validation:Required
	Endpoint string `json:"endpoint"`

	// method is the HTTP method to use (GET, POST, PUT, DELETE).
	// Defaults to POST.
	// +kubebuilder:default="POST"
	// +optional
	Method string `json:"method,omitempty"`

	// headers are additional HTTP headers to include in requests.
	// +optional
	Headers map[string]string `json:"headers,omitempty"`

	// contentType is the Content-Type header value.
	// Defaults to "application/json".
	// +kubebuilder:default="application/json"
	// +optional
	ContentType string `json:"contentType,omitempty"`

	// authType specifies the authentication type (none, bearer, basic).
	// +optional
	AuthType *string `json:"authType,omitempty"`

	// authSecretRef references a secret containing auth credentials.
	// +optional
	AuthSecretRef *SecretKeySelector `json:"authSecretRef,omitempty"`
}

// SecretKeySelector selects a key from a Secret
type SecretKeySelector struct {
	// name is the name of the secret.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// key is the key in the secret to select.
	// +kubebuilder:validation:Required
	Key string `json:"key"`
}

// ToolDefinition defines a tool's interface for plain HTTP/gRPC handlers
type ToolDefinition struct {
	// name is the tool name that will be exposed to the LLM.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-z][a-z0-9_]*$`
	// +kubebuilder:validation:MaxLength=64
	Name string `json:"name"`

	// description explains what the tool does (shown to LLM).
	// +kubebuilder:validation:Required
	Description string `json:"description"`

	// inputSchema is the JSON Schema for the tool's input parameters.
	// +kubebuilder:validation:Required
	InputSchema apiextensionsv1.JSON `json:"inputSchema"`

	// outputSchema is the JSON Schema for the tool's output.
	// +optional
	OutputSchema *apiextensionsv1.JSON `json:"outputSchema,omitempty"`
}

// ServiceSelector defines how to discover handler endpoints via Kubernetes Services
type ServiceSelector struct {
	// matchLabels specifies labels that must match on the Service.
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty"`

	// namespace specifies the namespace to search for Services.
	// If empty, searches in the same namespace as the ToolRegistry.
	// +optional
	Namespace *string `json:"namespace,omitempty"`

	// port specifies the port name or number on the Service.
	// If empty, uses the first port.
	// +optional
	Port *string `json:"port,omitempty"`
}

// HandlerDefinition defines a tool handler that exposes one or more tools
type HandlerDefinition struct {
	// name is a unique identifier for this handler within the registry.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`

	// type specifies the handler protocol.
	// +kubebuilder:validation:Required
	Type HandlerType `json:"type"`

	// selector discovers the handler endpoint from Kubernetes Services.
	// Mutually exclusive with inline endpoint configuration.
	// +optional
	Selector *ServiceSelector `json:"selector,omitempty"`

	// tool defines the tool interface (required for http and grpc types).
	// Self-describing handlers (mcp, openapi) discover tools automatically.
	// +optional
	Tool *ToolDefinition `json:"tool,omitempty"`

	// httpConfig contains HTTP-specific configuration.
	// Required when type is "http".
	// +optional
	HTTPConfig *HTTPConfig `json:"httpConfig,omitempty"`

	// openAPIConfig contains OpenAPI-specific configuration.
	// Required when type is "openapi".
	// +optional
	OpenAPIConfig *OpenAPIConfig `json:"openAPIConfig,omitempty"`

	// grpcConfig contains gRPC-specific configuration.
	// Required when type is "grpc".
	// +optional
	GRPCConfig *GRPCConfig `json:"grpcConfig,omitempty"`

	// mcpConfig contains MCP-specific configuration.
	// Required when type is "mcp".
	// +optional
	MCPConfig *MCPConfig `json:"mcpConfig,omitempty"`

	// timeout specifies the maximum duration for tool invocation.
	// Defaults to "30s".
	// +kubebuilder:default="30s"
	// +optional
	Timeout *string `json:"timeout,omitempty"`

	// retries specifies the number of retry attempts on failure.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	// +kubebuilder:default=0
	// +optional
	Retries *int32 `json:"retries,omitempty"`
}

// ToolRegistrySpec defines the desired state of ToolRegistry
type ToolRegistrySpec struct {
	// handlers defines the list of tool handlers in this registry.
	// Each handler can expose one or more tools.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Handlers []HandlerDefinition `json:"handlers"`
}

// DiscoveredTool represents a tool discovered from a handler
type DiscoveredTool struct {
	// name is the tool name (used by LLM)
	Name string `json:"name"`

	// handlerName is the handler that provides this tool
	HandlerName string `json:"handlerName"`

	// description is the tool description (for LLM)
	Description string `json:"description"`

	// inputSchema is the JSON Schema for input parameters
	// +optional
	InputSchema *apiextensionsv1.JSON `json:"inputSchema,omitempty"`

	// outputSchema is the JSON Schema for output
	// +optional
	OutputSchema *apiextensionsv1.JSON `json:"outputSchema,omitempty"`

	// endpoint is the resolved endpoint URL/address
	Endpoint string `json:"endpoint"`

	// status indicates whether the tool is available
	// +kubebuilder:validation:Enum=Available;Unavailable;Unknown
	Status string `json:"status"`

	// lastChecked is the timestamp of the last availability check
	// +optional
	LastChecked *metav1.Time `json:"lastChecked,omitempty"`

	// error contains the error message if status is Unavailable
	// +optional
	Error *string `json:"error,omitempty"`
}

// ToolRegistryStatus defines the observed state of ToolRegistry
type ToolRegistryStatus struct {
	// phase represents the current lifecycle phase of the ToolRegistry.
	// +optional
	Phase ToolRegistryPhase `json:"phase,omitempty"`

	// discoveredToolsCount is the number of tools successfully discovered.
	// +optional
	DiscoveredToolsCount int32 `json:"discoveredToolsCount,omitempty"`

	// discoveredTools contains details of each discovered tool.
	// +optional
	DiscoveredTools []DiscoveredTool `json:"discoveredTools,omitempty"`

	// lastDiscoveryTime is the timestamp of the last successful discovery.
	// +optional
	LastDiscoveryTime *metav1.Time `json:"lastDiscoveryTime,omitempty"`

	// conditions represent the current state of the ToolRegistry resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ToolRegistryPhase represents the current phase of the ToolRegistry
// +kubebuilder:validation:Enum=Pending;Ready;Degraded;Failed
type ToolRegistryPhase string

const (
	// ToolRegistryPhasePending indicates the ToolRegistry is being processed
	ToolRegistryPhasePending ToolRegistryPhase = "Pending"
	// ToolRegistryPhaseReady indicates all tools are discovered and available
	ToolRegistryPhaseReady ToolRegistryPhase = "Ready"
	// ToolRegistryPhaseDegraded indicates some tools are unavailable
	ToolRegistryPhaseDegraded ToolRegistryPhase = "Degraded"
	// ToolRegistryPhaseFailed indicates the ToolRegistry failed to initialize
	ToolRegistryPhaseFailed ToolRegistryPhase = "Failed"
)

// Tool status constants
const (
	ToolStatusAvailable   = "Available"
	ToolStatusUnavailable = "Unavailable"
	ToolStatusUnknown     = "Unknown"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Tools",type="integer",JSONPath=".status.discoveredToolsCount",description="Number of discovered tools"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Current phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ToolRegistry is the Schema for the toolregistries API.
// It defines a collection of tool handlers that expose tools to agents.
type ToolRegistry struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of ToolRegistry
	// +required
	Spec ToolRegistrySpec `json:"spec"`

	// status defines the observed state of ToolRegistry
	// +optional
	Status ToolRegistryStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// ToolRegistryList contains a list of ToolRegistry
type ToolRegistryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ToolRegistry `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ToolRegistry{}, &ToolRegistryList{})
}

// Legacy type aliases for backward compatibility during migration
// TODO: Remove these after all references are updated

// ToolType is deprecated, use HandlerType instead
type ToolType = HandlerType

// ToolTypeHTTP is deprecated
const ToolTypeHTTP = HandlerTypeHTTP

// ToolTypeGRPC is deprecated
const ToolTypeGRPC = HandlerTypeGRPC

// ToolTypeMCP is deprecated
const ToolTypeMCP = HandlerTypeMCP
