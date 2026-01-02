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
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"

	toolsv1 "github.com/altairalabs/omnia/pkg/tools/v1"
)

func TestGRPCAdapter_Name(t *testing.T) {
	config := GRPCAdapterConfig{
		Name:     "test-grpc",
		Endpoint: "localhost:9090",
	}

	adapter := NewGRPCAdapter(config, logr.Discard())
	if adapter.Name() != "test-grpc" {
		t.Errorf("expected name 'test-grpc', got %q", adapter.Name())
	}
}

func TestGRPCAdapter_DefaultTimeout(t *testing.T) {
	config := GRPCAdapterConfig{
		Name:     "test-grpc",
		Endpoint: "localhost:9090",
	}

	adapter := NewGRPCAdapter(config, logr.Discard())
	if adapter.config.Timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", adapter.config.Timeout)
	}
}

func TestGRPCAdapter_CustomTimeout(t *testing.T) {
	config := GRPCAdapterConfig{
		Name:     "test-grpc",
		Endpoint: "localhost:9090",
		Timeout:  10 * time.Second,
	}

	adapter := NewGRPCAdapter(config, logr.Discard())
	if adapter.config.Timeout != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", adapter.config.Timeout)
	}
}

func TestGRPCAdapter_CallNotConnected(t *testing.T) {
	config := GRPCAdapterConfig{
		Name:     "test-grpc",
		Endpoint: "localhost:9090",
	}

	adapter := NewGRPCAdapter(config, logr.Discard())

	_, err := adapter.Call(context.Background(), "some-tool", nil)
	if err == nil {
		t.Fatal("expected error when not connected")
	}
}

func TestGRPCAdapter_ListToolsEmpty(t *testing.T) {
	config := GRPCAdapterConfig{
		Name:     "test-grpc",
		Endpoint: "localhost:9090",
	}

	adapter := NewGRPCAdapter(config, logr.Discard())

	// ListTools should return empty list when not connected
	tools, err := adapter.ListTools(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestGRPCAdapter_Close(t *testing.T) {
	config := GRPCAdapterConfig{
		Name:     "test-grpc",
		Endpoint: "localhost:9090",
	}

	adapter := NewGRPCAdapter(config, logr.Discard())

	// Close should not error even when not connected
	err := adapter.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGRPCAdapterConfig_TLS(t *testing.T) {
	config := GRPCAdapterConfig{
		Name:                  "tls-adapter",
		Endpoint:              "localhost:9090",
		TLS:                   true,
		TLSCertPath:           "/path/to/cert.pem",
		TLSKeyPath:            "/path/to/key.pem",
		TLSCAPath:             "/path/to/ca.pem",
		TLSInsecureSkipVerify: false,
	}

	adapter := NewGRPCAdapter(config, logr.Discard())
	if !adapter.config.TLS {
		t.Error("expected TLS to be enabled")
	}
	if adapter.config.TLSCertPath != "/path/to/cert.pem" {
		t.Errorf("unexpected TLSCertPath: %v", adapter.config.TLSCertPath)
	}
}

// mockToolServer implements the ToolService for testing.
type mockToolServer struct {
	toolsv1.UnimplementedToolServiceServer
	tools       []*toolsv1.ToolInfo
	executeFunc func(req *toolsv1.ToolRequest) (*toolsv1.ToolResponse, error)
}

func (s *mockToolServer) ListTools(ctx context.Context, req *toolsv1.ListToolsRequest) (*toolsv1.ListToolsResponse, error) {
	return &toolsv1.ListToolsResponse{Tools: s.tools}, nil
}

func (s *mockToolServer) Execute(ctx context.Context, req *toolsv1.ToolRequest) (*toolsv1.ToolResponse, error) {
	if s.executeFunc != nil {
		return s.executeFunc(req)
	}
	return &toolsv1.ToolResponse{
		ResultJson: `{"result": "success"}`,
	}, nil
}

func TestGRPCAdapter_WithMockServer(t *testing.T) {
	// Start a mock gRPC server
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	server := grpc.NewServer()
	mockServer := &mockToolServer{
		tools: []*toolsv1.ToolInfo{
			{
				Name:        "echo",
				Description: "Echoes the input",
				InputSchema: `{"type": "object", "properties": {"message": {"type": "string"}}}`,
			},
		},
		executeFunc: func(req *toolsv1.ToolRequest) (*toolsv1.ToolResponse, error) {
			var args map[string]any
			json.Unmarshal([]byte(req.ArgumentsJson), &args)
			msg := ""
			if m, ok := args["message"].(string); ok {
				msg = m
			}
			result := map[string]string{"echo": msg}
			resultJSON, _ := json.Marshal(result)
			return &toolsv1.ToolResponse{
				ResultJson: string(resultJSON),
			}, nil
		},
	}
	toolsv1.RegisterToolServiceServer(server, mockServer)

	go func() {
		_ = server.Serve(lis)
	}()
	defer server.Stop()

	// Create adapter and connect
	config := GRPCAdapterConfig{
		Name:     "test-grpc",
		Endpoint: lis.Addr().String(),
		Timeout:  5 * time.Second,
	}
	adapter := NewGRPCAdapter(config, logr.Discard())

	ctx := context.Background()
	err = adapter.Connect(ctx)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer adapter.Close()

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
	if tools[0].Description != "Echoes the input" {
		t.Errorf("unexpected description: %q", tools[0].Description)
	}
	if tools[0].InputSchema == nil {
		t.Error("expected InputSchema to be parsed")
	}

	// Test Call
	result, err := adapter.Call(ctx, "echo", map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if result.IsError {
		t.Error("result should not be an error")
	}
	if resultMap, ok := result.Content.(map[string]any); ok {
		if resultMap["echo"] != "hello" {
			t.Errorf("expected echo='hello', got %v", resultMap["echo"])
		}
	} else {
		t.Errorf("unexpected result type: %T", result.Content)
	}
}

func TestGRPCAdapter_ExecuteError(t *testing.T) {
	// Start a mock gRPC server that returns errors
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	server := grpc.NewServer()
	mockServer := &mockToolServer{
		tools: []*toolsv1.ToolInfo{
			{Name: "failing-tool"},
		},
		executeFunc: func(req *toolsv1.ToolRequest) (*toolsv1.ToolResponse, error) {
			return &toolsv1.ToolResponse{
				IsError:      true,
				ErrorMessage: "tool execution failed",
			}, nil
		},
	}
	toolsv1.RegisterToolServiceServer(server, mockServer)

	go func() {
		_ = server.Serve(lis)
	}()
	defer server.Stop()

	config := GRPCAdapterConfig{
		Name:     "test-grpc",
		Endpoint: lis.Addr().String(),
		Timeout:  5 * time.Second,
	}
	adapter := NewGRPCAdapter(config, logr.Discard())

	ctx := context.Background()
	err = adapter.Connect(ctx)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer adapter.Close()

	result, err := adapter.Call(ctx, "failing-tool", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if !result.IsError {
		t.Error("expected result to be an error")
	}
	if result.Content != "tool execution failed" {
		t.Errorf("unexpected error message: %v", result.Content)
	}
}
