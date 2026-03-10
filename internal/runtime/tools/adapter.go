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

// Package tools provides tool adapters for the runtime.
package tools

// ToolInfo describes a discovered tool.
type ToolInfo struct {
	// Name is the tool's unique identifier.
	Name string `json:"name"`

	// Description describes what the tool does.
	Description string `json:"description,omitempty"`

	// InputSchema is the JSON schema for tool input parameters.
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

// ToolMeta holds provenance metadata for a tool, linking it back to the
// ToolRegistry CRD and handler that provides it.
type ToolMeta struct {
	RegistryName      string
	RegistryNamespace string
	HandlerName       string
	HandlerType       string // http, grpc, mcp, openapi
	Endpoint          string
}

// ToolHealth reports the health status of a single tool.
type ToolHealth struct {
	// ToolName is the registered tool name.
	ToolName string `json:"toolName"`

	// AdapterName is the adapter that provides this tool.
	AdapterName string `json:"adapterName"`

	// Healthy is true if the backend is reachable.
	Healthy bool `json:"healthy"`

	// Error is set when the health check failed.
	Error string `json:"error,omitempty"`
}

// ToolResult contains the result of a tool invocation.
type ToolResult struct {
	// Content is the result content (can be text, JSON, etc.).
	Content any `json:"content"`

	// IsError indicates if the result represents an error.
	IsError bool `json:"isError,omitempty"`
}
