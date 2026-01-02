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

// ToolType defines the type of tool endpoint
// +kubebuilder:validation:Enum=http;grpc;mcp
type ToolType string

const (
	// ToolTypeHTTP indicates the tool is accessed via HTTP/REST
	ToolTypeHTTP ToolType = "http"
	// ToolTypeGRPC indicates the tool is accessed via gRPC
	ToolTypeGRPC ToolType = "grpc"
	// ToolTypeMCP indicates the tool is accessed via Model Context Protocol
	ToolTypeMCP ToolType = "mcp"
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

// MCPConfig contains MCP-specific tool configuration
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

// ToolEndpoint defines how to connect to the tool
type ToolEndpoint struct {
	// url specifies the direct URL endpoint for the tool.
	// Mutually exclusive with selector.
	// +optional
	URL *string `json:"url,omitempty"`

	// selector specifies label selectors for discovering tool endpoints
	// from Kubernetes Services. Mutually exclusive with url.
	// +optional
	Selector *ToolSelector `json:"selector,omitempty"`
}

// ToolSelector defines how to discover tool endpoints via Kubernetes Services
type ToolSelector struct {
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

// ToolSchema defines the JSON schema for tool parameters
type ToolSchema struct {
	// input defines the JSON schema for tool input parameters.
	// Should be a valid JSON Schema object represented as a string.
	// +optional
	Input *string `json:"input,omitempty"`

	// output defines the JSON schema for tool output.
	// Should be a valid JSON Schema object represented as a string.
	// +optional
	Output *string `json:"output,omitempty"`
}

// ToolDefinition defines a single tool that can be invoked by agents
type ToolDefinition struct {
	// name is the unique identifier for this tool within the registry.
	// Must be a valid DNS subdomain name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`

	// description provides a human-readable description of what the tool does.
	// This is used by agents to understand when to use the tool.
	// +optional
	Description *string `json:"description,omitempty"`

	// type specifies the protocol used to communicate with the tool.
	// +kubebuilder:validation:Required
	Type ToolType `json:"type"`

	// endpoint specifies how to connect to the tool.
	// Either url or selector must be specified.
	// +kubebuilder:validation:Required
	Endpoint ToolEndpoint `json:"endpoint"`

	// schema defines the input/output schema for the tool.
	// +optional
	Schema *ToolSchema `json:"schema,omitempty"`

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

	// mcpConfig contains MCP-specific configuration (for type=mcp).
	// Required when type is "mcp".
	// +optional
	MCPConfig *MCPConfig `json:"mcpConfig,omitempty"`
}

// ToolRegistrySpec defines the desired state of ToolRegistry
type ToolRegistrySpec struct {
	// tools defines the list of tools available in this registry.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Tools []ToolDefinition `json:"tools"`
}

// DiscoveredTool represents a tool that has been successfully discovered
type DiscoveredTool struct {
	// name is the tool name from the spec
	Name string `json:"name"`

	// endpoint is the resolved endpoint URL
	Endpoint string `json:"endpoint"`

	// status indicates whether the tool is available
	// +kubebuilder:validation:Enum=Available;Unavailable;Unknown
	Status string `json:"status"`

	// lastChecked is the timestamp of the last availability check
	// +optional
	LastChecked *metav1.Time `json:"lastChecked,omitempty"`
}

// ToolRegistryStatus defines the observed state of ToolRegistry.
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
// It defines a collection of tools that can be invoked by agents.
type ToolRegistry struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
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
