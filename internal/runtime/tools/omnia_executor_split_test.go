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
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Test-local string constants (goconst: avoid 3+ duplicate literals).
const (
	splitClientHandler  = "client-handler"
	splitMethodPost     = "POST"
	splitMCPDefaultErr  = "MCP tool returned error"
	splitGRPCToolName   = "grpc-tool"
	splitLocalhostEndpt = "http://localhost"
)

// --- LoadConfigFromEntries ---

func TestOmniaExecutor_LoadConfigFromEntries(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	entries := []HandlerEntry{
		{
			Name: splitClientHandler,
			Type: ToolTypeClient,
			Tool: &ToolDefCfg{Name: "client-tool", Description: "a client tool"},
		},
		{
			Name:       "http-handler",
			Type:       ToolTypeHTTP,
			Tool:       &ToolDefCfg{Name: "http-tool"},
			HTTPConfig: &HTTPCfg{Endpoint: splitLocalhostEndpt + ":8080", Method: splitMethodPost},
		},
	}

	if err := e.LoadConfigFromEntries(entries); err != nil {
		t.Fatalf("LoadConfigFromEntries: %v", err)
	}
	if e.config == nil {
		t.Fatal("config is nil after LoadConfigFromEntries")
	}
	if len(e.handlers) != 2 {
		t.Errorf("handlers count = %d, want 2", len(e.handlers))
	}
	if _, ok := e.handlers[splitClientHandler]; !ok {
		t.Error("handler 'client-handler' not registered")
	}
	if _, ok := e.handlers["http-handler"]; !ok {
		t.Error("handler 'http-handler' not registered")
	}
}

// --- ExecuteTool ---

func TestOmniaExecutor_ExecuteTool_NotFound(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	_, err := e.ExecuteTool(context.Background(), "missing", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOmniaExecutor_ExecuteTool_DispatchesToHandler(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	// Register a handler with an unsupported type so dispatch returns a
	// deterministic error without needing a live backend — this still
	// exercises the lookup-and-dispatch path of ExecuteTool.
	e.handlers["h"] = &HandlerEntry{Name: "h", Type: "bogus"}
	e.toolHandlers["t"] = "h"

	_, err := e.ExecuteTool(context.Background(), "t", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected dispatch error for unsupported handler type")
	}
	if !strings.Contains(err.Error(), "unsupported handler type") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- initClientHandler ---

func TestOmniaExecutor_InitClientHandler_RegistersTool(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: splitClientHandler,
		Type: ToolTypeClient,
		Tool: &ToolDefCfg{Name: "browser-tool", Description: "runs in browser"},
	}
	if err := e.initClientHandler(splitClientHandler, h); err != nil {
		t.Fatalf("initClientHandler: %v", err)
	}
	if got := e.toolHandlers["browser-tool"]; got != splitClientHandler {
		t.Errorf("toolHandlers[browser-tool] = %q, want %q", got, splitClientHandler)
	}
}

func TestOmniaExecutor_InitClientHandler_SkipsWithoutTool(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{Name: splitClientHandler, Type: ToolTypeClient}
	if err := e.initClientHandler(splitClientHandler, h); err != nil {
		t.Fatalf("initClientHandler: %v", err)
	}
	if len(e.toolHandlers) != 0 {
		t.Errorf("expected no tools registered, got %d", len(e.toolHandlers))
	}
}

// --- initClientHandler via initHandler dispatch ---

func TestOmniaExecutor_InitHandler_Client(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: splitClientHandler,
		Type: ToolTypeClient,
		Tool: &ToolDefCfg{Name: "dispatch-tool"},
	}
	if err := e.initHandler(context.Background(), splitClientHandler, h); err != nil {
		t.Fatalf("initHandler: %v", err)
	}
	if _, ok := e.toolHandlers["dispatch-tool"]; !ok {
		t.Error("client tool not registered via initHandler")
	}
}

// --- mcpErrorMessage ---

func TestMCPErrorMessage(t *testing.T) {
	tests := []struct {
		name   string
		result *mcp.CallToolResult
		want   string
	}{
		{
			name:   "no content uses default",
			result: &mcp.CallToolResult{},
			want:   splitMCPDefaultErr,
		},
		{
			name: "text content used as message",
			result: &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "boom"}},
			},
			want: "boom",
		},
		{
			name: "empty text falls back to default",
			result: &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: ""}},
			},
			want: splitMCPDefaultErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mcpErrorMessage(tt.result); got != tt.want {
				t.Errorf("mcpErrorMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- initGRPCHandler ---

func TestOmniaExecutor_InitGRPCHandler_WithToolDef(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	// A tool definition in the handler config lets the handler register
	// without a live ListTools RPC. The dial is lazy (no WithBlock), so a
	// bogus endpoint does not fail initialization.
	h := &HandlerEntry{
		Name: "g",
		Type: ToolTypeGRPC,
		Tool: &ToolDefCfg{Name: splitGRPCToolName, Description: "a grpc tool"},
		GRPCConfig: &GRPCCfg{
			Endpoint: "passthrough:///bogus:9090",
		},
	}
	if err := e.initGRPCHandler(context.Background(), "g", h); err != nil {
		t.Fatalf("initGRPCHandler: %v", err)
	}
	if got := e.toolHandlers[splitGRPCToolName]; got != "g" {
		t.Errorf("toolHandlers[grpc-tool] = %q, want %q", got, "g")
	}
	if _, ok := e.grpcTools["g"][splitGRPCToolName]; !ok {
		t.Error("grpc-tool not registered in grpcTools")
	}
	_ = e.Close()
}

// --- initMCPHandler ---

func TestOmniaExecutor_InitMCPHandler_UnsupportedTransport(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name:      "m",
		Type:      ToolTypeMCP,
		MCPConfig: &MCPCfg{Transport: "bogus", Endpoint: splitLocalhostEndpt},
	}
	err := e.initMCPHandler(context.Background(), "m", h)
	if err == nil {
		t.Fatal("expected error for unsupported MCP transport")
	}
	if !strings.Contains(err.Error(), "unsupported MCP transport") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- parseMCPArgs ---

func TestParseMCPArgs(t *testing.T) {
	t.Run("empty returns nil map", func(t *testing.T) {
		m, err := parseMCPArgs(nil)
		if err != nil {
			t.Fatalf("parseMCPArgs: %v", err)
		}
		if m != nil {
			t.Errorf("expected nil map, got %v", m)
		}
	})
	t.Run("valid json parsed", func(t *testing.T) {
		m, err := parseMCPArgs(json.RawMessage(`{"a":1}`))
		if err != nil {
			t.Fatalf("parseMCPArgs: %v", err)
		}
		if m["a"] != float64(1) {
			t.Errorf("expected a=1, got %v", m["a"])
		}
	})
	t.Run("invalid json errors", func(t *testing.T) {
		_, err := parseMCPArgs(json.RawMessage(`{bad`))
		if err == nil {
			t.Fatal("expected error for invalid json")
		}
		if !strings.Contains(err.Error(), "failed to parse MCP args") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
