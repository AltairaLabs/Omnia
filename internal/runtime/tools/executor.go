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

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/tools"
	"github.com/go-logr/logr"
)

// executeTimeout bounds how long a tool execution can take when no caller
// context is available (the non-context Execute path).
const executeTimeout = 30 * time.Second

// ManagerExecutor adapts the Manager to the PromptKit Executor interface.
// This allows the tool manager to be wired into PromptKit conversations.
type ManagerExecutor struct {
	manager *Manager
	log     logr.Logger
}

// NewManagerExecutor creates a new ManagerExecutor that wraps the given Manager.
func NewManagerExecutor(manager *Manager, log logr.Logger) *ManagerExecutor {
	return &ManagerExecutor{
		manager: manager,
		log:     log.WithName("executor"),
	}
}

// Name returns the executor name.
func (e *ManagerExecutor) Name() string {
	return "omnia-tool-manager"
}

// Execute implements the PromptKit Executor interface.
// It routes tool calls through the manager to the appropriate adapter.
func (e *ManagerExecutor) Execute(descriptor *tools.ToolDescriptor, args json.RawMessage) (json.RawMessage, error) {
	// Parse arguments from JSON
	var argsMap map[string]any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &argsMap); err != nil {
			return nil, fmt.Errorf("failed to parse arguments: %w", err)
		}
	}

	e.log.V(1).Info("executing tool via manager",
		"tool", descriptor.Name,
		"args", string(args))

	// Use a bounded context since Execute does not receive one from the caller.
	ctx, cancel := context.WithTimeout(context.Background(), executeTimeout)
	defer cancel()

	// Call the tool through the manager
	result, err := e.manager.Call(ctx, descriptor.Name, argsMap)
	if err != nil {
		return nil, fmt.Errorf("tool execution failed: %w", err)
	}

	// Handle error results
	if result.IsError {
		errMsg := fmt.Sprintf("%v", result.Content)
		e.log.Info("tool returned error", "tool", descriptor.Name, "error", errMsg)
		return nil, fmt.Errorf("tool error: %s", errMsg)
	}

	// Marshal the result back to JSON
	resultJSON, err := json.Marshal(result.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	e.log.V(1).Info("tool execution complete",
		"tool", descriptor.Name,
		"resultLength", len(resultJSON))

	return resultJSON, nil
}

// ExecuteCtx is a context-aware version of Execute.
// This is the preferred method when context propagation is needed.
func (e *ManagerExecutor) ExecuteCtx(ctx context.Context, descriptor *tools.ToolDescriptor, args json.RawMessage) (json.RawMessage, error) {
	// Parse arguments from JSON
	var argsMap map[string]any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &argsMap); err != nil {
			return nil, fmt.Errorf("failed to parse arguments: %w", err)
		}
	}

	e.log.V(1).Info("executing tool via manager (with context)",
		"tool", descriptor.Name,
		"args", string(args))

	// Call the tool through the manager with context
	result, err := e.manager.Call(ctx, descriptor.Name, argsMap)
	if err != nil {
		return nil, fmt.Errorf("tool execution failed: %w", err)
	}

	// Handle error results
	if result.IsError {
		errMsg := fmt.Sprintf("%v", result.Content)
		e.log.Info("tool returned error", "tool", descriptor.Name, "error", errMsg)
		return nil, fmt.Errorf("tool error: %s", errMsg)
	}

	// Marshal the result back to JSON
	resultJSON, err := json.Marshal(result.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	return resultJSON, nil
}

// ListTools returns all tools available through the manager as PromptKit ToolDescriptors.
func (e *ManagerExecutor) ListTools(ctx context.Context) ([]*tools.ToolDescriptor, error) {
	// Connect to discover tools
	if err := e.manager.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect manager: %w", err)
	}

	// Collect adapters under the lock, then release before calling ListTools
	// to avoid holding the read lock during potentially slow network calls.
	type adapterEntry struct {
		name    string
		adapter ToolAdapter
	}

	e.manager.mu.RLock()
	entries := make([]adapterEntry, 0, len(e.manager.adapters))
	for name, adapter := range e.manager.adapters {
		entries = append(entries, adapterEntry{name: name, adapter: adapter})
	}
	e.manager.mu.RUnlock()

	// Call ListTools outside the lock.
	var descriptors []*tools.ToolDescriptor
	for _, entry := range entries {
		adapterTools, err := entry.adapter.ListTools(ctx)
		if err != nil {
			e.log.Error(err, "failed to list tools from adapter", "adapter", entry.name)
			continue
		}

		for _, tool := range adapterTools {
			// Convert InputSchema to json.RawMessage
			var inputSchema json.RawMessage
			if tool.InputSchema != nil {
				schemaBytes, err := json.Marshal(tool.InputSchema)
				if err == nil {
					inputSchema = schemaBytes
				}
			}

			descriptors = append(descriptors, &tools.ToolDescriptor{
				Name:        tool.Name,
				Description: tool.Description,
				InputSchema: inputSchema,
			})
		}
	}

	return descriptors, nil
}
