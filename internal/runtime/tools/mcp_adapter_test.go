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
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPAdapter_Name(t *testing.T) {
	config := MCPAdapterConfig{
		Name:      "test-mcp",
		Transport: MCPTransportSSE,
		Endpoint:  "http://localhost:8080/sse",
	}

	adapter := NewMCPAdapter(config, logr.Discard())
	if adapter.Name() != "test-mcp" {
		t.Errorf("expected name 'test-mcp', got %q", adapter.Name())
	}
}

func TestMCPAdapter_ConnectInvalidTransport(t *testing.T) {
	config := MCPAdapterConfig{
		Name:      "test-mcp",
		Transport: "invalid",
	}

	adapter := NewMCPAdapter(config, logr.Discard())
	err := adapter.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid transport")
	}
}

func TestMCPAdapter_CallNotConnected(t *testing.T) {
	config := MCPAdapterConfig{
		Name:      "test-mcp",
		Transport: MCPTransportSSE,
		Endpoint:  "http://localhost:8080/sse",
	}

	adapter := NewMCPAdapter(config, logr.Discard())

	_, err := adapter.Call(context.Background(), "some-tool", nil)
	if err == nil {
		t.Fatal("expected error when not connected")
	}
}

func TestMCPAdapter_ListToolsEmpty(t *testing.T) {
	config := MCPAdapterConfig{
		Name:      "test-mcp",
		Transport: MCPTransportSSE,
	}

	adapter := NewMCPAdapter(config, logr.Discard())

	// ListTools should return empty list when not connected
	tools, err := adapter.ListTools(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestMCPAdapter_Close(t *testing.T) {
	config := MCPAdapterConfig{
		Name:      "test-mcp",
		Transport: MCPTransportSSE,
	}

	adapter := NewMCPAdapter(config, logr.Discard())

	// Close should not error even when not connected
	err := adapter.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMCPAdapterConfig_SSE(t *testing.T) {
	config := MCPAdapterConfig{
		Name:      "sse-adapter",
		Transport: MCPTransportSSE,
		Endpoint:  "http://mcp-server:8080/sse",
	}

	adapter := NewMCPAdapter(config, logr.Discard())
	if adapter.config.Transport != MCPTransportSSE {
		t.Errorf("expected SSE transport, got %v", adapter.config.Transport)
	}
	if adapter.config.Endpoint != "http://mcp-server:8080/sse" {
		t.Errorf("unexpected endpoint: %v", adapter.config.Endpoint)
	}
}

func TestMCPAdapterConfig_Stdio(t *testing.T) {
	config := MCPAdapterConfig{
		Name:      "stdio-adapter",
		Transport: MCPTransportStdio,
		Command:   "/usr/bin/mcp-server",
		Args:      []string{"--verbose"},
		WorkDir:   "/app",
		Env:       map[string]string{"DEBUG": "true"},
	}

	adapter := NewMCPAdapter(config, logr.Discard())
	if adapter.config.Transport != MCPTransportStdio {
		t.Errorf("expected Stdio transport, got %v", adapter.config.Transport)
	}
	if adapter.config.Command != "/usr/bin/mcp-server" {
		t.Errorf("unexpected command: %v", adapter.config.Command)
	}
	if len(adapter.config.Args) != 1 || adapter.config.Args[0] != "--verbose" {
		t.Errorf("unexpected args: %v", adapter.config.Args)
	}
	if adapter.config.WorkDir != "/app" {
		t.Errorf("unexpected workDir: %v", adapter.config.WorkDir)
	}
	if adapter.config.Env["DEBUG"] != "true" {
		t.Errorf("unexpected env: %v", adapter.config.Env)
	}
}

// EchoParams is the input for the echo tool.
type EchoParams struct {
	Message string `json:"message"`
}

// TestMCPAdapter_WithInMemoryServer tests the MCP adapter with an in-memory MCP server.
func TestMCPAdapter_WithInMemoryServer(t *testing.T) {
	ctx := context.Background()

	// Create in-memory transports for client and server
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	// Create a mock MCP server
	server := mcp.NewServer(
		&mcp.Implementation{Name: "test-server", Version: "1.0.0"},
		nil,
	)

	// Register a test tool using the generic AddTool function
	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "Echoes the input",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args EchoParams) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Echo: " + args.Message},
			},
		}, nil, nil
	})

	// Start server in background
	go func() {
		_, _ = server.Connect(ctx, serverTransport, nil)
	}()

	// Create MCP adapter with the client transport
	config := MCPAdapterConfig{
		Name:      "test-mcp",
		Transport: MCPTransportSSE, // Type doesn't matter for in-memory
	}
	adapter := NewMCPAdapter(config, logr.Discard())

	// Override the transport by connecting directly
	adapter.client = mcp.NewClient(
		&mcp.Implementation{Name: "omnia-runtime", Version: "v1.0.0"},
		nil,
	)

	session, err := adapter.client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	adapter.session = session

	// Discover tools
	adapter.tools = make(map[string]*mcp.Tool)
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("failed to list tools: %v", err)
		}
		adapter.tools[tool.Name] = tool
	}

	// Test ListTools
	tools, err := adapter.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "echo" {
		t.Errorf("expected tool name 'echo', got %q", tools[0].Name)
	}

	// Test Call
	result, err := adapter.Call(ctx, "echo", map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if result.IsError {
		t.Error("result should not be an error")
	}
	// Content should be a TextContent
	if textContent, ok := result.Content.(*mcp.TextContent); ok {
		if textContent.Text != "Echo: hello" {
			t.Errorf("expected 'Echo: hello', got %q", textContent.Text)
		}
	}

	// Test Call with unknown tool
	_, err = adapter.Call(ctx, "unknown", nil)
	if err == nil {
		t.Error("expected error for unknown tool")
	}

	// Test Close
	err = adapter.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify tools are cleared
	tools, _ = adapter.ListTools(ctx)
	if len(tools) != 0 {
		t.Errorf("expected 0 tools after close, got %d", len(tools))
	}
}
