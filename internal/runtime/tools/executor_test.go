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
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/tools"
	"github.com/go-logr/logr"
)

func TestManagerExecutor_Name(t *testing.T) {
	manager := NewManager(logr.Discard())
	executor := NewManagerExecutor(manager, logr.Discard())

	if executor.Name() != "omnia-tool-manager" {
		t.Errorf("expected name 'omnia-tool-manager', got %q", executor.Name())
	}
}

func TestManagerExecutor_Execute(t *testing.T) {
	manager := NewManager(logr.Discard())

	// Register a mock adapter
	mockTools := []ToolInfo{
		{Name: "test_tool", Description: "A test tool"},
	}
	adapter := newMockAdapter("mock", mockTools)
	adapter.callResults["test_tool"] = &ToolResult{
		Content: map[string]any{"result": "success"},
		IsError: false,
	}
	_ = manager.RegisterAdapter(adapter)
	_ = manager.Connect(context.Background())

	executor := NewManagerExecutor(manager, logr.Discard())

	// Test successful execution
	descriptor := &tools.ToolDescriptor{Name: "test_tool"}
	args := json.RawMessage(`{"input": "test"}`)

	result, err := executor.Execute(descriptor, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resultMap map[string]any
	if err := json.Unmarshal(result, &resultMap); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if resultMap["result"] != "success" {
		t.Errorf("expected result 'success', got %v", resultMap["result"])
	}
}

func TestManagerExecutor_ExecuteError(t *testing.T) {
	manager := NewManager(logr.Discard())

	// Register a mock adapter that returns an error
	mockTools := []ToolInfo{
		{Name: "error_tool", Description: "A tool that returns errors"},
	}
	adapter := newMockAdapter("mock", mockTools)
	adapter.callResults["error_tool"] = &ToolResult{
		Content: "something went wrong",
		IsError: true,
	}
	_ = manager.RegisterAdapter(adapter)
	_ = manager.Connect(context.Background())

	executor := NewManagerExecutor(manager, logr.Discard())

	descriptor := &tools.ToolDescriptor{Name: "error_tool"}
	args := json.RawMessage(`{}`)

	_, err := executor.Execute(descriptor, args)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	expectedMsg := "tool error: something went wrong"
	if err.Error() != expectedMsg {
		t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
	}
}

func TestManagerExecutor_ExecuteNotFound(t *testing.T) {
	manager := NewManager(logr.Discard())
	executor := NewManagerExecutor(manager, logr.Discard())

	descriptor := &tools.ToolDescriptor{Name: "nonexistent"}
	args := json.RawMessage(`{}`)

	_, err := executor.Execute(descriptor, args)
	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
}

func TestManagerExecutor_ExecuteCtx(t *testing.T) {
	manager := NewManager(logr.Discard())

	// Register a mock adapter
	mockTools := []ToolInfo{
		{Name: "ctx_tool", Description: "A context-aware tool"},
	}
	adapter := newMockAdapter("mock", mockTools)
	adapter.callResults["ctx_tool"] = &ToolResult{
		Content: "context result",
		IsError: false,
	}
	_ = manager.RegisterAdapter(adapter)
	_ = manager.Connect(context.Background())

	executor := NewManagerExecutor(manager, logr.Discard())

	ctx := context.Background()
	descriptor := &tools.ToolDescriptor{Name: "ctx_tool"}
	args := json.RawMessage(`{}`)

	result, err := executor.ExecuteCtx(ctx, descriptor, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resultStr string
	if err := json.Unmarshal(result, &resultStr); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if resultStr != "context result" {
		t.Errorf("expected 'context result', got %q", resultStr)
	}
}

func TestManagerExecutor_ListTools(t *testing.T) {
	manager := NewManager(logr.Discard())

	// Register mock adapters with tools
	tools1 := []ToolInfo{
		{Name: "tool1", Description: "First tool", InputSchema: map[string]any{"type": "object"}},
		{Name: "tool2", Description: "Second tool"},
	}
	tools2 := []ToolInfo{
		{Name: "tool3", Description: "Third tool"},
	}

	adapter1 := newMockAdapter("adapter1", tools1)
	adapter2 := newMockAdapter("adapter2", tools2)

	_ = manager.RegisterAdapter(adapter1)
	_ = manager.RegisterAdapter(adapter2)

	executor := NewManagerExecutor(manager, logr.Discard())

	ctx := context.Background()
	descriptors, err := executor.ListTools(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(descriptors) != 3 {
		t.Errorf("expected 3 tools, got %d", len(descriptors))
	}

	// Check that all expected tools are present
	toolNames := make(map[string]bool)
	for _, d := range descriptors {
		toolNames[d.Name] = true
	}

	for _, name := range []string{"tool1", "tool2", "tool3"} {
		if !toolNames[name] {
			t.Errorf("expected tool %q not found", name)
		}
	}
}

func TestManagerExecutor_ExecuteInvalidJSON(t *testing.T) {
	manager := NewManager(logr.Discard())
	executor := NewManagerExecutor(manager, logr.Discard())

	descriptor := &tools.ToolDescriptor{Name: "test"}
	args := json.RawMessage(`{invalid json}`)

	_, err := executor.Execute(descriptor, args)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestManagerExecutor_ExecuteEmptyArgs(t *testing.T) {
	manager := NewManager(logr.Discard())

	// Register a mock adapter
	mockTools := []ToolInfo{
		{Name: "no_args_tool", Description: "A tool with no args"},
	}
	adapter := newMockAdapter("mock", mockTools)
	adapter.callResults["no_args_tool"] = &ToolResult{
		Content: "ok",
		IsError: false,
	}
	_ = manager.RegisterAdapter(adapter)
	_ = manager.Connect(context.Background())

	executor := NewManagerExecutor(manager, logr.Discard())

	descriptor := &tools.ToolDescriptor{Name: "no_args_tool"}

	// Test with empty args
	result, err := executor.Execute(descriptor, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resultStr string
	if err := json.Unmarshal(result, &resultStr); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if resultStr != "ok" {
		t.Errorf("expected 'ok', got %q", resultStr)
	}
}
