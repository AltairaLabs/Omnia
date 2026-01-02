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

import (
	"context"
)

// ToolAdapter is the interface for tool execution backends.
type ToolAdapter interface {
	// Name returns the adapter's name.
	Name() string

	// Connect establishes connection to the tool backend.
	Connect(ctx context.Context) error

	// ListTools returns available tools from this adapter.
	ListTools(ctx context.Context) ([]ToolInfo, error)

	// Call invokes a tool with the given arguments.
	Call(ctx context.Context, name string, args map[string]any) (*ToolResult, error)

	// Close closes the connection.
	Close() error
}

// ToolInfo describes a discovered tool.
type ToolInfo struct {
	// Name is the tool's unique identifier.
	Name string `json:"name"`

	// Description describes what the tool does.
	Description string `json:"description,omitempty"`

	// InputSchema is the JSON schema for tool input parameters.
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

// ToolResult contains the result of a tool invocation.
type ToolResult struct {
	// Content is the result content (can be text, JSON, etc.).
	Content any `json:"content"`

	// IsError indicates if the result represents an error.
	IsError bool `json:"isError,omitempty"`
}
