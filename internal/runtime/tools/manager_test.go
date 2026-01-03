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
	"testing"

	"github.com/go-logr/logr"
)

// mockAdapter is a mock implementation of ToolAdapter for testing.
type mockAdapter struct {
	name        string
	tools       []ToolInfo
	connectErr  error
	callResults map[string]*ToolResult
	callErrs    map[string]error
	connected   bool
	closed      bool
}

func newMockAdapter(name string, tools []ToolInfo) *mockAdapter {
	return &mockAdapter{
		name:        name,
		tools:       tools,
		callResults: make(map[string]*ToolResult),
		callErrs:    make(map[string]error),
	}
}

func (a *mockAdapter) Name() string {
	return a.name
}

func (a *mockAdapter) Connect(ctx context.Context) error {
	if a.connectErr != nil {
		return a.connectErr
	}
	a.connected = true
	return nil
}

func (a *mockAdapter) ListTools(ctx context.Context) ([]ToolInfo, error) {
	return a.tools, nil
}

func (a *mockAdapter) Call(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	if err, ok := a.callErrs[name]; ok {
		return nil, err
	}
	if result, ok := a.callResults[name]; ok {
		return result, nil
	}
	return &ToolResult{Content: "mock result"}, nil
}

func (a *mockAdapter) Close() error {
	a.closed = true
	return nil
}

func TestManager_RegisterAdapter(t *testing.T) {
	m := NewManager(logr.Discard())

	adapter := newMockAdapter("test-adapter", nil)
	err := m.RegisterAdapter(adapter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Registering same adapter again should fail
	err = m.RegisterAdapter(adapter)
	if err == nil {
		t.Fatal("expected error when registering duplicate adapter")
	}
}

func TestManager_Connect(t *testing.T) {
	m := NewManager(logr.Discard())

	tools := []ToolInfo{
		{Name: "tool1", Description: "First tool"},
		{Name: "tool2", Description: "Second tool"},
	}
	adapter := newMockAdapter("test-adapter", tools)
	_ = m.RegisterAdapter(adapter)

	ctx := context.Background()
	err := m.Connect(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !adapter.connected {
		t.Error("adapter should be connected")
	}

	// Check tools were discovered
	discoveredTools := m.ListTools()
	if len(discoveredTools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(discoveredTools))
	}
}

func TestManager_Call(t *testing.T) {
	m := NewManager(logr.Discard())

	tools := []ToolInfo{{Name: "echo"}}
	adapter := newMockAdapter("test-adapter", tools)
	adapter.callResults["echo"] = &ToolResult{Content: "hello world"}
	_ = m.RegisterAdapter(adapter)

	ctx := context.Background()
	_ = m.Connect(ctx)

	result, err := m.Call(ctx, "echo", map[string]any{"msg": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content != "hello world" {
		t.Errorf("expected 'hello world', got %v", result.Content)
	}
}

func TestManager_CallNotFound(t *testing.T) {
	m := NewManager(logr.Discard())

	ctx := context.Background()
	_, err := m.Call(ctx, "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
}

func TestManager_Close(t *testing.T) {
	m := NewManager(logr.Discard())

	adapter := newMockAdapter("test-adapter", nil)
	_ = m.RegisterAdapter(adapter)
	_ = m.Connect(context.Background())

	err := m.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !adapter.closed {
		t.Error("adapter should be closed")
	}
}

func TestManager_LoadFromToolConfig(t *testing.T) {
	m := NewManager(logr.Discard())

	config := &ToolConfig{
		Tools: []ToolEntry{
			{
				Name: "http-tool",
				Type: ToolTypeHTTP,
				HTTPConfig: &HTTPCfg{
					Endpoint: "http://example.com/api",
				},
			},
			{
				Name: "mcp-tool",
				Type: ToolTypeMCP,
				MCPConfig: &MCPCfg{
					Transport: "sse",
					Endpoint:  "http://mcp-server:8080/sse",
				},
			},
		},
	}

	err := m.LoadFromToolConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have registered both the HTTP and MCP adapters
	m.mu.RLock()
	adapterCount := len(m.adapters)
	m.mu.RUnlock()

	if adapterCount != 2 {
		t.Errorf("expected 2 adapters, got %d", adapterCount)
	}
}

func TestManager_LoadFromToolConfig_SkipMissingMCPConfig(t *testing.T) {
	m := NewManager(logr.Discard())

	config := &ToolConfig{
		Tools: []ToolEntry{
			{
				Name: "mcp-tool-no-config",
				Type: ToolTypeMCP,
				// MCPConfig is nil - should be skipped
			},
		},
	}

	err := m.LoadFromToolConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m.mu.RLock()
	adapterCount := len(m.adapters)
	m.mu.RUnlock()

	if adapterCount != 0 {
		t.Errorf("expected 0 adapters (tool should be skipped), got %d", adapterCount)
	}
}

func TestManager_LoadFromHandlers(t *testing.T) {
	m := NewManager(logr.Discard())

	config := &ToolConfig{
		Handlers: []HandlerEntry{
			{
				Name:     "http-handler",
				Type:     ToolTypeHTTP,
				Endpoint: "http://example.com/api",
				Tool: &ToolDefCfg{
					Name:        "create_user",
					Description: "Creates a new user",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":  map[string]any{"type": "string"},
							"email": map[string]any{"type": "string"},
						},
						"required": []string{"name", "email"},
					},
				},
				HTTPConfig: &HTTPCfg{
					Endpoint: "http://example.com/api",
					Method:   "POST",
				},
			},
			{
				Name:     "openapi-handler",
				Type:     ToolTypeOpenAPI,
				Endpoint: "http://api.example.com/openapi.json",
				OpenAPIConfig: &OpenAPICfg{
					SpecURL: "http://api.example.com/openapi.json",
				},
			},
		},
	}

	err := m.LoadFromToolConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have registered both handlers
	m.mu.RLock()
	adapterCount := len(m.adapters)
	m.mu.RUnlock()

	if adapterCount != 2 {
		t.Errorf("expected 2 adapters, got %d", adapterCount)
	}
}

func TestManager_LoadFromHandlers_PrioritizedOverTools(t *testing.T) {
	m := NewManager(logr.Discard())

	// Config with both Handlers and Tools - Handlers should take precedence
	config := &ToolConfig{
		Handlers: []HandlerEntry{
			{
				Name:     "handler-http",
				Type:     ToolTypeHTTP,
				Endpoint: "http://handler.example.com",
				Tool: &ToolDefCfg{
					Name:        "handler_tool",
					Description: "From handler",
					InputSchema: map[string]any{"type": "object"},
				},
				HTTPConfig: &HTTPCfg{
					Endpoint: "http://handler.example.com",
				},
			},
		},
		Tools: []ToolEntry{
			{
				Name: "legacy-tool",
				Type: ToolTypeHTTP,
				HTTPConfig: &HTTPCfg{
					Endpoint: "http://legacy.example.com",
				},
			},
		},
	}

	err := m.LoadFromToolConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only have the handler adapter, not the legacy tool
	m.mu.RLock()
	_, hasHandler := m.adapters["handler-http"]
	_, hasLegacy := m.adapters["legacy-tool"]
	m.mu.RUnlock()

	if !hasHandler {
		t.Error("expected handler-http adapter to be registered")
	}
	if hasLegacy {
		t.Error("legacy tool should not be registered when handlers are present")
	}
}

func TestManager_LoadFromHandlers_SkipHTTPWithoutToolDef(t *testing.T) {
	m := NewManager(logr.Discard())

	config := &ToolConfig{
		Handlers: []HandlerEntry{
			{
				Name:     "http-no-tool",
				Type:     ToolTypeHTTP,
				Endpoint: "http://example.com",
				HTTPConfig: &HTTPCfg{
					Endpoint: "http://example.com",
				},
				// Tool is nil - should be skipped
			},
		},
	}

	err := m.LoadFromToolConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m.mu.RLock()
	adapterCount := len(m.adapters)
	m.mu.RUnlock()

	if adapterCount != 0 {
		t.Errorf("expected 0 adapters (HTTP handler without tool def should be skipped), got %d", adapterCount)
	}
}

func TestManager_LoadFromHandlers_SkipGRPCWithoutToolDef(t *testing.T) {
	m := NewManager(logr.Discard())

	config := &ToolConfig{
		Handlers: []HandlerEntry{
			{
				Name:     "grpc-no-tool",
				Type:     ToolTypeGRPC,
				Endpoint: "localhost:50051",
				GRPCConfig: &GRPCCfg{
					Endpoint: "localhost:50051",
				},
				// Tool is nil - should be skipped
			},
		},
	}

	err := m.LoadFromToolConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m.mu.RLock()
	adapterCount := len(m.adapters)
	m.mu.RUnlock()

	if adapterCount != 0 {
		t.Errorf("expected 0 adapters (gRPC handler without tool def should be skipped), got %d", adapterCount)
	}
}

func TestManager_LoadFromHandlers_MCPDoesNotRequireToolDef(t *testing.T) {
	m := NewManager(logr.Discard())

	config := &ToolConfig{
		Handlers: []HandlerEntry{
			{
				Name:     "mcp-handler",
				Type:     ToolTypeMCP,
				Endpoint: "http://mcp-server/sse",
				MCPConfig: &MCPCfg{
					Transport: "sse",
					Endpoint:  "http://mcp-server/sse",
				},
				// No Tool definition - MCP is self-describing
			},
		},
	}

	err := m.LoadFromToolConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m.mu.RLock()
	adapterCount := len(m.adapters)
	m.mu.RUnlock()

	if adapterCount != 1 {
		t.Errorf("expected 1 adapter (MCP is self-describing), got %d", adapterCount)
	}
}

func TestManager_LoadFromHandlers_OpenAPIDoesNotRequireToolDef(t *testing.T) {
	m := NewManager(logr.Discard())

	config := &ToolConfig{
		Handlers: []HandlerEntry{
			{
				Name:     "openapi-handler",
				Type:     ToolTypeOpenAPI,
				Endpoint: "http://api.example.com/openapi.json",
				OpenAPIConfig: &OpenAPICfg{
					SpecURL: "http://api.example.com/openapi.json",
				},
				// No Tool definition - OpenAPI is self-describing
			},
		},
	}

	err := m.LoadFromToolConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m.mu.RLock()
	adapterCount := len(m.adapters)
	m.mu.RUnlock()

	if adapterCount != 1 {
		t.Errorf("expected 1 adapter (OpenAPI is self-describing), got %d", adapterCount)
	}
}

func TestManager_LoadFromHandlers_Timeout(t *testing.T) {
	m := NewManager(logr.Discard())

	config := &ToolConfig{
		Handlers: []HandlerEntry{
			{
				Name:     "http-with-timeout",
				Type:     ToolTypeHTTP,
				Endpoint: "http://example.com",
				Timeout:  "5s",
				Tool: &ToolDefCfg{
					Name:        "test_tool",
					Description: "Test",
					InputSchema: map[string]any{"type": "object"},
				},
				HTTPConfig: &HTTPCfg{
					Endpoint: "http://example.com",
				},
			},
		},
	}

	err := m.LoadFromToolConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m.mu.RLock()
	adapterCount := len(m.adapters)
	m.mu.RUnlock()

	if adapterCount != 1 {
		t.Errorf("expected 1 adapter, got %d", adapterCount)
	}
}
