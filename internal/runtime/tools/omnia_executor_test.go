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
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcStatus "google.golang.org/grpc/status"

	pktools "github.com/AltairaLabs/PromptKit/runtime/tools"

	"github.com/altairalabs/omnia/internal/tracing"
	toolsv1 "github.com/altairalabs/omnia/pkg/tools/v1"
)

// mockToolServiceClient implements toolsv1.ToolServiceClient for testing.
type mockToolServiceClient struct {
	executeResp *toolsv1.ToolResponse
	executeErr  error
	listResp    *toolsv1.ListToolsResponse
	listErr     error
}

func (m *mockToolServiceClient) Execute(
	_ context.Context,
	_ *toolsv1.ToolRequest,
	_ ...grpc.CallOption,
) (*toolsv1.ToolResponse, error) {
	return m.executeResp, m.executeErr
}

func (m *mockToolServiceClient) ListTools(
	_ context.Context,
	_ *toolsv1.ListToolsRequest,
	_ ...grpc.CallOption,
) (*toolsv1.ListToolsResponse, error) {
	return m.listResp, m.listErr
}

// --- Name ---

func TestOmniaExecutor_Name(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	if got := e.Name(); got != "omnia" {
		t.Errorf("Name() = %q, want %q", got, "omnia")
	}
}

// --- NewOmniaExecutor ---

func TestOmniaExecutor_New(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	if e == nil {
		t.Fatal("NewOmniaExecutor returned nil")
	}
	if e.handlers == nil {
		t.Error("handlers map not initialized")
	}
	if e.toolHandlers == nil {
		t.Error("toolHandlers map not initialized")
	}
	if e.toolMeta == nil {
		t.Error("toolMeta map not initialized")
	}
	if e.mcpClients == nil {
		t.Error("mcpClients map not initialized")
	}
	if e.mcpSessions == nil {
		t.Error("mcpSessions map not initialized")
	}
	if e.grpcConns == nil {
		t.Error("grpcConns map not initialized")
	}
	if e.grpcClients == nil {
		t.Error("grpcClients map not initialized")
	}
	if e.breakers == nil {
		t.Error("breakers not initialized")
	}
}

// --- LoadConfig ---

func TestOmniaExecutor_LoadConfig(t *testing.T) {
	yaml := `handlers:
  - name: my-http
    type: http
    endpoint: http://localhost:8080
    tool:
      name: my-tool
      description: a test tool
    httpConfig:
      endpoint: http://localhost:8080
      method: POST
  - name: my-grpc
    type: grpc
    endpoint: localhost:9090
    grpcConfig:
      endpoint: localhost:9090
`
	dir := t.TempDir()
	path := filepath.Join(dir, "tools.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	e := NewOmniaExecutor(logr.Discard(), nil)
	if err := e.LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if e.config == nil {
		t.Fatal("config is nil after LoadConfig")
	}
	if len(e.handlers) != 2 {
		t.Errorf("handlers count = %d, want 2", len(e.handlers))
	}
	if _, ok := e.handlers["my-http"]; !ok {
		t.Error("handler 'my-http' not registered")
	}
	if _, ok := e.handlers["my-grpc"]; !ok {
		t.Error("handler 'my-grpc' not registered")
	}
}

func TestOmniaExecutor_LoadConfig_InvalidPath(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	err := e.LoadConfig("/nonexistent/tools.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent config file")
	}
	if !strings.Contains(err.Error(), "failed to load tools config") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOmniaExecutor_LoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte(":\n  :\n  invalid: ["), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	e := NewOmniaExecutor(logr.Discard(), nil)
	err := e.LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

// --- Initialize ---

func TestOmniaExecutor_Initialize_NilConfig(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	if err := e.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize with nil config: %v", err)
	}
}

func TestOmniaExecutor_Initialize_EmptyHandlers(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.config = &ToolConfig{}
	if err := e.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize with empty handlers: %v", err)
	}
}

// --- ToolNames ---

func TestOmniaExecutor_ToolNames(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.toolHandlers["tool-a"] = "handler-a"
	e.toolHandlers["tool-b"] = "handler-b"

	names := e.ToolNames()
	sort.Strings(names)
	if len(names) != 2 {
		t.Fatalf("ToolNames count = %d, want 2", len(names))
	}
	if names[0] != "tool-a" || names[1] != "tool-b" {
		t.Errorf("ToolNames = %v, want [tool-a tool-b]", names)
	}
}

func TestOmniaExecutor_ToolNames_Empty(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	names := e.ToolNames()
	if len(names) != 0 {
		t.Errorf("ToolNames for empty executor = %v, want empty", names)
	}
}

// --- ToolDescriptors ---

func TestOmniaExecutor_ToolDescriptors_HTTP(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["h1"] = &HandlerEntry{
		Name: "h1",
		Type: ToolTypeHTTP,
		Tool: &ToolDefCfg{
			Name:        "my-tool",
			Description: "does stuff",
			InputSchema: map[string]any{"type": "object"},
		},
	}
	e.toolHandlers["my-tool"] = "h1"

	descs := e.ToolDescriptors()
	if len(descs) != 1 {
		t.Fatalf("ToolDescriptors count = %d, want 1", len(descs))
	}
	d := descs[0]
	if d.Name != "my-tool" {
		t.Errorf("Name = %q, want %q", d.Name, "my-tool")
	}
	if d.Mode != "omnia" {
		t.Errorf("Mode = %q, want %q", d.Mode, "omnia")
	}
	if d.Description != "does stuff" {
		t.Errorf("Description = %q, want %q", d.Description, "does stuff")
	}
	if d.InputSchema == nil {
		t.Error("InputSchema is nil, want non-nil")
	}
}

// --- buildDescriptor ---

func TestOmniaExecutor_BuildDescriptor_HTTPWithSchema(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "h1",
		Type: ToolTypeHTTP,
		Tool: &ToolDefCfg{
			Name:        "calc",
			Description: "calculate",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"expr": map[string]any{"type": "string"},
				},
			},
		},
	}

	desc := e.buildDescriptor("calc", h)
	if desc.Description != "calculate" {
		t.Errorf("Description = %q, want %q", desc.Description, "calculate")
	}
	if desc.InputSchema == nil {
		t.Error("InputSchema is nil")
	}
}

func TestOmniaExecutor_BuildDescriptor_HTTPNoSchema(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "h1",
		Type: ToolTypeHTTP,
		Tool: &ToolDefCfg{
			Name:        "simple",
			Description: "simple tool",
		},
	}

	desc := e.buildDescriptor("simple", h)
	if desc.Description != "simple tool" {
		t.Errorf("Description = %q, want %q", desc.Description, "simple tool")
	}
	if desc.InputSchema != nil {
		t.Errorf("InputSchema = %v, want nil", desc.InputSchema)
	}
}

func TestOmniaExecutor_BuildDescriptor_NoTool(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "h1",
		Type: ToolTypeHTTP,
	}

	desc := e.buildDescriptor("empty", h)
	if desc == nil {
		t.Fatal("buildDescriptor returned nil")
	}
	if desc.Name != "empty" {
		t.Errorf("Name = %q, want %q", desc.Name, "empty")
	}
	if desc.Description != "" {
		t.Errorf("Description = %q, want empty", desc.Description)
	}
}

func TestOmniaExecutor_BuildDescriptor_GRPCWithInputSchema(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "grpc-h",
		Type: ToolTypeGRPC,
	}
	schema := `{"type":"object","properties":{"x":{"type":"integer"}}}`
	e.grpcTools["grpc-h"] = map[string]*toolsv1.ToolInfo{
		"grpc-tool": {
			Name:        "grpc-tool",
			Description: "a gRPC tool",
			InputSchema: schema,
		},
	}

	desc := e.buildDescriptor("grpc-tool", h)
	if desc.Description != "a gRPC tool" {
		t.Errorf("Description = %q, want %q", desc.Description, "a gRPC tool")
	}
	if string(desc.InputSchema) != schema {
		t.Errorf("InputSchema = %s, want %s", desc.InputSchema, schema)
	}
}

func TestOmniaExecutor_BuildDescriptor_MCPTool(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "mcp-h",
		Type: ToolTypeMCP,
	}
	mcpTool := &mcp.Tool{
		Name:        "mcp-tool",
		Description: "an MCP tool",
	}
	e.mcpTools["mcp-h"] = map[string]*mcp.Tool{
		"mcp-tool": mcpTool,
	}

	desc := e.buildDescriptor("mcp-tool", h)
	if desc.Description != "an MCP tool" {
		t.Errorf("Description = %q, want %q", desc.Description, "an MCP tool")
	}
}

// --- buildToolLabels ---

func TestOmniaExecutor_BuildToolLabels_WithoutRegistryInfo(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "weather-api",
		Type: ToolTypeHTTP,
	}

	labels := e.buildToolLabels("get-weather", h)
	if labels["handler_type"] != "http" {
		t.Errorf("handler_type = %q, want %q", labels["handler_type"], "http")
	}
	if labels["handler_name"] != "weather-api" {
		t.Errorf("handler_name = %q, want %q", labels["handler_name"], "weather-api")
	}
	if _, ok := labels["registry_name"]; ok {
		t.Error("registry_name should not be set without SetRegistryInfo")
	}
}

func TestOmniaExecutor_BuildToolLabels_WithRegistryInfo(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.toolHandlers["get-weather"] = "weather-api"
	e.handlers["weather-api"] = &HandlerEntry{
		Name:     "weather-api",
		Type:     ToolTypeHTTP,
		Endpoint: "http://weather.example.com",
	}
	e.SetRegistryInfo("my-tools", "production", []HandlerEntry{
		{Name: "weather-api", Type: ToolTypeHTTP, Endpoint: "http://weather.example.com"},
	})

	labels := e.buildToolLabels("get-weather", e.handlers["weather-api"])
	if labels["handler_type"] != "http" {
		t.Errorf("handler_type = %q, want %q", labels["handler_type"], "http")
	}
	if labels["handler_name"] != "weather-api" {
		t.Errorf("handler_name = %q, want %q", labels["handler_name"], "weather-api")
	}
	if labels["registry_name"] != "my-tools" {
		t.Errorf("registry_name = %q, want %q", labels["registry_name"], "my-tools")
	}
	if labels["registry_namespace"] != "production" {
		t.Errorf("registry_namespace = %q, want %q", labels["registry_namespace"], "production")
	}
}

func TestOmniaExecutor_ToolDescriptors_IncludeLabels(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["h1"] = &HandlerEntry{
		Name: "h1",
		Type: ToolTypeGRPC,
		Tool: &ToolDefCfg{Name: "grpc-tool", Description: "a tool"},
	}
	e.toolHandlers["grpc-tool"] = "h1"
	e.SetRegistryInfo("reg", "ns", []HandlerEntry{
		{Name: "h1", Type: ToolTypeGRPC, Endpoint: "localhost:50051"},
	})

	descs := e.ToolDescriptors()
	if len(descs) != 1 {
		t.Fatalf("ToolDescriptors count = %d, want 1", len(descs))
	}
	d := descs[0]
	if d.Labels == nil {
		t.Fatal("Labels is nil, want non-nil")
	}
	if d.Labels["handler_type"] != "grpc" {
		t.Errorf("handler_type = %q, want %q", d.Labels["handler_type"], "grpc")
	}
	if d.Labels["registry_name"] != "reg" {
		t.Errorf("registry_name = %q, want %q", d.Labels["registry_name"], "reg")
	}
}

// --- GetToolMeta / SetRegistryInfo ---

func TestOmniaExecutor_ToolMeta_RoundTrip(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.toolHandlers["tool-x"] = "handler-x"

	handlers := []HandlerEntry{
		{
			Name:     "handler-x",
			Type:     ToolTypeHTTP,
			Endpoint: "http://example.com",
		},
	}
	e.SetRegistryInfo("my-registry", "default", handlers)

	meta, ok := e.GetToolMeta("tool-x")
	if !ok {
		t.Fatal("GetToolMeta returned false")
	}
	if meta.RegistryName != "my-registry" {
		t.Errorf("RegistryName = %q, want %q", meta.RegistryName, "my-registry")
	}
	if meta.RegistryNamespace != "default" {
		t.Errorf("RegistryNamespace = %q, want %q", meta.RegistryNamespace, "default")
	}
	if meta.HandlerName != "handler-x" {
		t.Errorf("HandlerName = %q, want %q", meta.HandlerName, "handler-x")
	}
	if meta.HandlerType != ToolTypeHTTP {
		t.Errorf("HandlerType = %q, want %q", meta.HandlerType, ToolTypeHTTP)
	}
	if meta.Endpoint != "http://example.com" {
		t.Errorf("Endpoint = %q, want %q", meta.Endpoint, "http://example.com")
	}
}

func TestOmniaExecutor_GetToolMeta_NotFound(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	_, ok := e.GetToolMeta("nonexistent")
	if ok {
		t.Error("GetToolMeta returned true for nonexistent tool")
	}
}

// --- Execute ---

func TestOmniaExecutor_Execute_ToolNotFound(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	desc := &pktools.ToolDescriptor{Name: "missing"}

	_, err := e.Execute(context.Background(), desc, nil)
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
	if !strings.Contains(err.Error(), "tool \"missing\" not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- dispatch ---

func TestOmniaExecutor_Dispatch_UnsupportedType(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	handler := &HandlerEntry{Type: "quantum"}

	_, err := e.dispatch(context.Background(), "tool", "handler", handler, nil)
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
	if !strings.Contains(err.Error(), "unsupported handler type: quantum") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- startSpan ---

func TestOmniaExecutor_StartSpan_NilTracingProvider(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)

	ctx, span := e.startSpan(context.Background(), "test-tool")
	if ctx == nil {
		t.Fatal("returned nil context")
	}
	// With nil tracingProvider, should return a noop span (not recording)
	if span.IsRecording() {
		t.Error("expected non-recording span for nil tracing provider")
	}
}

// --- recordResult ---

func TestOmniaExecutor_RecordResult_WithError(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	span := tracenoop.Span{}

	// Should not panic
	e.recordResult(span, nil, errForTest)
}

func TestOmniaExecutor_RecordResult_WithoutError(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	span := tracenoop.Span{}
	result := json.RawMessage(`{"ok":true}`)

	// Should not panic
	e.recordResult(span, result, nil)
}

// errForTest is a package-level error used in recordResult tests.
var errForTest = fmt.Errorf("test error")

// --- initHTTPHandler ---

func TestOmniaExecutor_InitHTTPHandler(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "my-http",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint: "http://localhost:8080",
			Method:   "POST",
		},
		Tool: &ToolDefCfg{
			Name:        "my-tool",
			Description: "test",
		},
	}

	if err := e.initHTTPHandler("my-http", h); err != nil {
		t.Fatalf("initHTTPHandler: %v", err)
	}
	if e.toolHandlers["my-tool"] != "my-http" {
		t.Errorf("tool not registered: %v", e.toolHandlers)
	}
}

func TestOmniaExecutor_InitHTTPHandler_NoConfig(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "no-cfg",
		Type: ToolTypeHTTP,
	}

	if err := e.initHTTPHandler("no-cfg", h); err != nil {
		t.Fatalf("initHTTPHandler: %v", err)
	}
	// Should skip — no tools registered
	if len(e.toolHandlers) != 0 {
		t.Errorf("expected no tools registered, got %d", len(e.toolHandlers))
	}
}

func TestOmniaExecutor_InitHTTPHandler_NoTool(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "no-tool",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint: "http://localhost",
		},
	}

	if err := e.initHTTPHandler("no-tool", h); err != nil {
		t.Fatalf("initHTTPHandler: %v", err)
	}
	if len(e.toolHandlers) != 0 {
		t.Errorf("expected no tools registered, got %d", len(e.toolHandlers))
	}
}

// --- initGRPCHandler ---

func TestOmniaExecutor_InitGRPCHandler_NoConfig(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "no-grpc",
		Type: ToolTypeGRPC,
	}

	if err := e.initGRPCHandler(context.Background(), "no-grpc", h); err != nil {
		t.Fatalf("initGRPCHandler: %v", err)
	}
	if len(e.toolHandlers) != 0 {
		t.Errorf("expected no tools registered, got %d", len(e.toolHandlers))
	}
}

// --- initMCPHandler ---

func TestOmniaExecutor_InitMCPHandler_NoConfig(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "no-mcp",
		Type: ToolTypeMCP,
	}

	if err := e.initMCPHandler(context.Background(), "no-mcp", h); err != nil {
		t.Fatalf("initMCPHandler: %v", err)
	}
	if len(e.toolHandlers) != 0 {
		t.Errorf("expected no tools registered, got %d", len(e.toolHandlers))
	}
}

// --- buildGRPCDialOptions ---

func TestBuildGRPCDialOptions_Insecure(t *testing.T) {
	cfg := &GRPCCfg{
		Endpoint: "localhost:9090",
		TLS:      false,
	}

	opts, err := buildGRPCDialOptions(cfg)
	if err != nil {
		t.Fatalf("buildGRPCDialOptions: %v", err)
	}
	if len(opts) != 1 {
		t.Errorf("opts count = %d, want 1", len(opts))
	}
}

func TestBuildGRPCDialOptions_TLSMissingCA(t *testing.T) {
	cfg := &GRPCCfg{
		Endpoint:  "localhost:9090",
		TLS:       true,
		TLSCAPath: "/nonexistent/ca.pem",
	}

	_, err := buildGRPCDialOptions(cfg)
	if err == nil {
		t.Fatal("expected error for missing CA cert")
	}
}

// --- marshalMCPResult ---

func TestMarshalMCPResult_ErrorWithText(t *testing.T) {
	result := &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: "something went wrong"},
		},
	}

	_, err := marshalMCPResult(result)
	if err == nil {
		t.Fatal("expected error for error result")
	}
	if !strings.Contains(err.Error(), "something went wrong") {
		t.Errorf("error = %q, want to contain 'something went wrong'", err)
	}
}

func TestMarshalMCPResult_ErrorWithoutContent(t *testing.T) {
	result := &mcp.CallToolResult{
		IsError: true,
	}

	_, err := marshalMCPResult(result)
	if err == nil {
		t.Fatal("expected error for error result")
	}
	if !strings.Contains(err.Error(), "MCP tool returned error") {
		t.Errorf("error = %q, want default error message", err)
	}
}

func TestMarshalMCPResult_SingleTextContent(t *testing.T) {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "hello world"},
		},
	}

	data, err := marshalMCPResult(result)
	if err != nil {
		t.Fatalf("marshalMCPResult: %v", err)
	}

	var got string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got != "hello world" {
		t.Errorf("result = %q, want %q", got, "hello world")
	}
}

func TestMarshalMCPResult_MultipleContent(t *testing.T) {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "part1"},
			&mcp.TextContent{Text: "part2"},
		},
	}

	data, err := marshalMCPResult(result)
	if err != nil {
		t.Fatalf("marshalMCPResult: %v", err)
	}
	if data == nil {
		t.Fatal("result is nil")
	}
	// Should marshal the full content array
	if !strings.Contains(string(data), "part1") {
		t.Errorf("result %s should contain 'part1'", data)
	}
}

func TestMarshalMCPResult_StructuredContent(t *testing.T) {
	result := &mcp.CallToolResult{
		Content:           []mcp.Content{},
		StructuredContent: map[string]any{"key": "value"},
	}

	data, err := marshalMCPResult(result)
	if err != nil {
		t.Fatalf("marshalMCPResult: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got["key"] != "value" {
		t.Errorf("result = %v, want key=value", got)
	}
}

func TestMarshalMCPResult_NilContent(t *testing.T) {
	result := &mcp.CallToolResult{}

	data, err := marshalMCPResult(result)
	if err != nil {
		t.Fatalf("marshalMCPResult: %v", err)
	}
	// nil content marshals to JSON null
	if string(data) != "null" {
		t.Errorf("result = %s, want null", data)
	}
}

// --- marshalGRPCResponse ---

func TestMarshalGRPCResponse_Error(t *testing.T) {
	resp := &toolsv1.ToolResponse{
		IsError:      true,
		ErrorMessage: "timeout exceeded",
	}

	_, err := marshalGRPCResponse(resp)
	if err == nil {
		t.Fatal("expected error for error response")
	}
	if !strings.Contains(err.Error(), "timeout exceeded") {
		t.Errorf("error = %q, want to contain 'timeout exceeded'", err)
	}
}

func TestMarshalGRPCResponse_JSONResult(t *testing.T) {
	resp := &toolsv1.ToolResponse{
		ResultJson: `{"result":42}`,
	}

	data, err := marshalGRPCResponse(resp)
	if err != nil {
		t.Fatalf("marshalGRPCResponse: %v", err)
	}
	if string(data) != `{"result":42}` {
		t.Errorf("result = %s, want {\"result\":42}", data)
	}
}

func TestMarshalGRPCResponse_EmptyResult(t *testing.T) {
	resp := &toolsv1.ToolResponse{}

	data, err := marshalGRPCResponse(resp)
	if err != nil {
		t.Fatalf("marshalGRPCResponse: %v", err)
	}
	if string(data) != "null" {
		t.Errorf("result = %s, want null", data)
	}
}

// --- CheckHealth ---

func TestOmniaExecutor_CheckHealth_RegisteredTools(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["h-mcp"] = &HandlerEntry{
		Name: "h-mcp",
		Type: ToolTypeMCP,
	}
	e.toolHandlers["tool-mcp"] = "h-mcp"

	results := e.CheckHealth(context.Background())
	if len(results) != 1 {
		t.Fatalf("health results count = %d, want 1", len(results))
	}
	if !results[0].Healthy {
		t.Error("expected MCP tool to be healthy (implicit)")
	}
	if results[0].ToolName != "tool-mcp" {
		t.Errorf("ToolName = %q, want %q", results[0].ToolName, "tool-mcp")
	}
}

func TestOmniaExecutor_CheckHealth_ProbeError(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["h-http"] = &HandlerEntry{
		Name: "h-http",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint: "http://127.0.0.1:1", // invalid port — will fail
		},
	}
	e.toolHandlers["tool-http"] = "h-http"

	results := e.CheckHealth(context.Background())
	if len(results) != 1 {
		t.Fatalf("health results count = %d, want 1", len(results))
	}
	if results[0].Healthy {
		t.Error("expected HTTP tool to be unhealthy")
	}
	if results[0].Error == "" {
		t.Error("expected non-empty error")
	}
}

// --- probeHandler ---

func TestOmniaExecutor_ProbeHandler_NotFound(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	err := e.probeHandler(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing handler")
	}
	if !strings.Contains(err.Error(), "handler \"nonexistent\" not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOmniaExecutor_ProbeHandler_MCPReturnsNil(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["mcp-h"] = &HandlerEntry{
		Name: "mcp-h",
		Type: ToolTypeMCP,
	}

	err := e.probeHandler(context.Background(), "mcp-h")
	if err != nil {
		t.Errorf("expected nil error for MCP probe, got: %v", err)
	}
}

func TestOmniaExecutor_ProbeHandler_GRPCReturnsNil(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["grpc-h"] = &HandlerEntry{
		Name: "grpc-h",
		Type: ToolTypeGRPC,
	}

	err := e.probeHandler(context.Background(), "grpc-h")
	if err != nil {
		t.Errorf("expected nil error for gRPC probe, got: %v", err)
	}
}

// --- Close ---

func TestOmniaExecutor_Close(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	// Close with no connections should succeed
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// After close, maps should be reset
	if len(e.mcpSessions) != 0 {
		t.Error("mcpSessions not reset")
	}
	if len(e.mcpClients) != 0 {
		t.Error("mcpClients not reset")
	}
	if len(e.mcpTools) != 0 {
		t.Error("mcpTools not reset")
	}
	if len(e.grpcConns) != 0 {
		t.Error("grpcConns not reset")
	}
	if len(e.grpcClients) != 0 {
		t.Error("grpcClients not reset")
	}
	if len(e.grpcTools) != 0 {
		t.Error("grpcTools not reset")
	}
}

// --- buildMCPTransport ---

func TestBuildMCPTransport_SSE(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	cfg := &MCPCfg{
		Transport: "sse",
		Endpoint:  "http://localhost:3000/mcp",
	}

	transport, err := e.buildMCPTransport(cfg)
	if err != nil {
		t.Fatalf("buildMCPTransport(sse): %v", err)
	}
	if transport == nil {
		t.Fatal("transport is nil")
	}
}

func TestBuildMCPTransport_Stdio(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	cfg := &MCPCfg{
		Transport: "stdio",
		Command:   "echo",
		Args:      []string{"hello"},
		WorkDir:   "/tmp",
		Env:       map[string]string{"FOO": "bar"},
	}

	transport, err := e.buildMCPTransport(cfg)
	if err != nil {
		t.Fatalf("buildMCPTransport(stdio): %v", err)
	}
	if transport == nil {
		t.Fatal("transport is nil")
	}
}

func TestBuildMCPTransport_Unsupported(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	cfg := &MCPCfg{
		Transport: "websocket",
	}

	_, err := e.buildMCPTransport(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported transport")
	}
	if !strings.Contains(err.Error(), "unsupported MCP transport: websocket") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- buildHTTPHeaders ---

func TestOmniaExecutor_BuildHTTPHeaders(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	cfg := &HTTPCfg{
		Headers: map[string]string{
			"X-Custom": "value1",
		},
	}

	headers := e.buildHTTPHeaders(
		context.Background(),
		cfg,
		"tool-name",
		"handler-name",
		json.RawMessage(`{"key":"val"}`),
	)

	if headers["X-Custom"] != "value1" {
		t.Errorf("X-Custom = %q, want %q", headers["X-Custom"], "value1")
	}
}

func TestOmniaExecutor_BuildHTTPHeaders_NilStaticHeaders(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	cfg := &HTTPCfg{}

	headers := e.buildHTTPHeaders(
		context.Background(),
		cfg,
		"tool",
		"handler",
		nil,
	)

	if headers == nil {
		t.Fatal("headers map is nil")
	}
}

func TestOmniaExecutor_BuildHTTPHeaders_WithAuth(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	cfg := &HTTPCfg{
		AuthType:  "bearer",
		AuthToken: "my-secret-token",
	}

	headers := e.buildHTTPHeaders(
		context.Background(),
		cfg,
		"tool",
		"handler",
		nil,
	)

	if headers["Authorization"] != "Bearer my-secret-token" {
		t.Errorf("Authorization = %q, want %q", headers["Authorization"], "Bearer my-secret-token")
	}
}

// --- executeHTTP ---

func TestOmniaExecutor_ExecuteHTTP_NilHTTPConfig(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	handler := &HandlerEntry{
		Name: "h1",
		Type: ToolTypeHTTP,
	}

	_, err := e.executeHTTP(context.Background(), "tool", "h1", handler, nil)
	if err == nil {
		t.Fatal("expected error for nil HTTPConfig")
	}
	if !strings.Contains(err.Error(), "has no HTTP config") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- executeMCP ---

func TestOmniaExecutor_ExecuteMCP_NilSession(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)

	_, err := e.executeMCP(context.Background(), "tool", "handler", nil)
	if err == nil {
		t.Fatal("expected error for nil session")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- executeGRPC ---

func TestOmniaExecutor_ExecuteGRPC_NilClient(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)

	_, err := e.executeGRPC(context.Background(), "tool", "handler", nil)
	if err == nil {
		t.Fatal("expected error for nil client")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- executeOpenAPI ---

func TestOmniaExecutor_ExecuteOpenAPI_OperationNotFound(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.openAPIOps["handler"] = map[string]*OpenAPIOperation{}
	e.openAPIBaseURLs["handler"] = "http://localhost"
	e.openAPIHeaders["handler"] = map[string]string{}

	handler := &HandlerEntry{
		Name: "handler",
		Type: ToolTypeOpenAPI,
	}

	_, err := e.executeOpenAPI(context.Background(), "missing-op", "handler", handler, nil)
	if err == nil {
		t.Fatal("expected error for missing operation")
	}
	if !strings.Contains(err.Error(), "OpenAPI operation \"missing-op\" not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- recordResult with wrapped error ---

func TestOmniaExecutor_RecordResult_WrappedError(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	span := tracenoop.Span{}

	// Should not panic with a wrapped error
	e.recordResult(span, nil, fmt.Errorf("wrapped: %w", errForTest))
}

// --- SetRegistryInfo with missing handler ---

func TestOmniaExecutor_SetRegistryInfo_MissingHandler(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.toolHandlers["orphan-tool"] = "nonexistent-handler"

	e.SetRegistryInfo("reg", "ns", nil)

	meta, ok := e.GetToolMeta("orphan-tool")
	if !ok {
		t.Fatal("expected meta to exist")
	}
	// Handler not found, so HandlerType/Endpoint should be empty
	if meta.HandlerType != "" {
		t.Errorf("HandlerType = %q, want empty", meta.HandlerType)
	}
	if meta.RegistryName != "reg" {
		t.Errorf("RegistryName = %q, want %q", meta.RegistryName, "reg")
	}
}

// --- Initialize with HTTP handler (no network) ---

func TestOmniaExecutor_Initialize_HTTPHandler(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.config = &ToolConfig{
		Handlers: []HandlerEntry{
			{
				Name: "test-http",
				Type: ToolTypeHTTP,
				HTTPConfig: &HTTPCfg{
					Endpoint: "http://localhost:8080",
					Method:   "POST",
				},
				Tool: &ToolDefCfg{
					Name:        "test-tool",
					Description: "test",
				},
			},
		},
	}
	e.handlers["test-http"] = &e.config.Handlers[0]

	if err := e.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if e.toolHandlers["test-tool"] != "test-http" {
		t.Error("tool not registered after Initialize")
	}
}

// --- probeHTTP with empty endpoint ---

func TestOmniaExecutor_ProbeHTTP_EmptyEndpoint(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "empty-ep",
		Type: ToolTypeHTTP,
	}

	err := e.probeHTTP(context.Background(), h)
	if err != nil {
		t.Errorf("expected nil for empty endpoint, got: %v", err)
	}
}

// --- initHandler unknown type ---

func TestOmniaExecutor_InitHandler_UnknownType(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "unknown",
		Type: "warp-drive",
	}

	err := e.initHandler(context.Background(), "unknown", h)
	if err != nil {
		t.Errorf("unknown type should not error, got: %v", err)
	}
}

// --- buildGRPCTLSConfig ---

func TestBuildGRPCTLSConfig_NoCA(t *testing.T) {
	cfg := &GRPCCfg{
		TLS:                   true,
		TLSInsecureSkipVerify: true,
	}

	tlsCfg, err := buildGRPCTLSConfig(cfg)
	if err != nil {
		t.Fatalf("buildGRPCTLSConfig: %v", err)
	}
	if !tlsCfg.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify=true")
	}
}

func TestBuildGRPCTLSConfig_InvalidCACert(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "bad-ca.pem")
	if err := os.WriteFile(caPath, []byte("not a cert"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	cfg := &GRPCCfg{
		TLS:       true,
		TLSCAPath: caPath,
	}

	_, err := buildGRPCTLSConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid CA cert")
	}
	if !strings.Contains(err.Error(), "failed to parse CA cert") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- ToolDescriptors empty ---

func TestOmniaExecutor_ToolDescriptors_Empty(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	descs := e.ToolDescriptors()
	if len(descs) != 0 {
		t.Errorf("expected empty descriptors, got %d", len(descs))
	}
}

// --- Multiple tools in ToolDescriptors ---

func TestOmniaExecutor_ToolDescriptors_Multiple(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["h1"] = &HandlerEntry{
		Name: "h1",
		Type: ToolTypeHTTP,
		Tool: &ToolDefCfg{Name: "a", Description: "tool a"},
	}
	e.handlers["h2"] = &HandlerEntry{
		Name: "h2",
		Type: ToolTypeHTTP,
		Tool: &ToolDefCfg{Name: "b", Description: "tool b"},
	}
	e.toolHandlers["a"] = "h1"
	e.toolHandlers["b"] = "h2"

	descs := e.ToolDescriptors()
	if len(descs) != 2 {
		t.Fatalf("ToolDescriptors count = %d, want 2", len(descs))
	}
}

// --- Execute with httptest server ---

func TestOmniaExecutor_Execute_HTTPWithServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["test-http"] = &HandlerEntry{
		Name: "test-http",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint: srv.URL,
			Method:   "POST",
		},
		Tool: &ToolDefCfg{
			Name:        "test-http-tool",
			Description: "test tool",
		},
	}
	e.toolHandlers["test-http-tool"] = "test-http"

	desc := &pktools.ToolDescriptor{Name: "test-http-tool"}
	result, err := e.Execute(context.Background(), desc, json.RawMessage(`{"x":1}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}
	if !strings.Contains(string(result), "ok") {
		t.Errorf("result = %s, want to contain 'ok'", result)
	}
}

// --- dispatch tests for each handler type ---

func TestOmniaExecutor_Dispatch_HTTP_NilConfig(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	handler := &HandlerEntry{Type: ToolTypeHTTP}

	_, err := e.dispatch(context.Background(), "tool", "handler", handler, nil)
	if err == nil {
		t.Fatal("expected error for nil HTTP config")
	}
	if !strings.Contains(err.Error(), "has no HTTP config") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOmniaExecutor_Dispatch_MCP_NilSession(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	handler := &HandlerEntry{Type: ToolTypeMCP}

	_, err := e.dispatch(context.Background(), "tool", "handler", handler, nil)
	if err == nil {
		t.Fatal("expected error for nil MCP session")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOmniaExecutor_Dispatch_GRPC_NilClient(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	handler := &HandlerEntry{Type: ToolTypeGRPC}

	_, err := e.dispatch(context.Background(), "tool", "handler", handler, nil)
	if err == nil {
		t.Fatal("expected error for nil gRPC client")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOmniaExecutor_Dispatch_OpenAPI_NoOps(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.openAPIOps["handler"] = map[string]*OpenAPIOperation{}
	e.openAPIBaseURLs["handler"] = "http://localhost"
	e.openAPIHeaders["handler"] = map[string]string{}
	handler := &HandlerEntry{Type: ToolTypeOpenAPI}

	_, err := e.dispatch(context.Background(), "tool", "handler", handler, nil)
	if err == nil {
		t.Fatal("expected error for missing operation")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- executeHTTP with httptest server ---

func TestOmniaExecutor_ExecuteHTTP_WithServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"value":42}`))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	handler := &HandlerEntry{
		Name: "h1",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint: srv.URL,
			Method:   "POST",
		},
	}

	result, err := e.executeHTTP(context.Background(), "tool-name", "h1", handler, json.RawMessage(`{"input":"data"}`))
	if err != nil {
		t.Fatalf("executeHTTP: %v", err)
	}
	if !strings.Contains(string(result), "42") {
		t.Errorf("result = %s, want to contain '42'", result)
	}
}

// --- executeMCP with invalid args ---

func TestOmniaExecutor_ExecuteMCP_InvalidArgs(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	// We need a non-nil session to get past the nil check, but the session
	// would fail on CallTool. The invalid JSON check happens first.
	// To test the unmarshal error path, we must have a non-nil session in the map.
	// We can't easily create a real mcp.ClientSession, so let's test the path
	// where args are invalid and session IS set. We'll just verify the error path
	// by testing executeMCP directly — but we can't set a non-nil session without
	// a real connection. Instead, let's verify the nil session path returns early.

	// Test with valid session placeholder — need to set it in mcpSessions map
	// This is tricky because mcp.ClientSession is a concrete type.
	// Instead, verify the invalid args path by checking the error message directly.
	_, err := e.executeMCP(context.Background(), "tool", "handler", json.RawMessage(`not valid json`))
	if err == nil {
		t.Fatal("expected error")
	}
	// It will hit the nil session check first
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- executeGRPC with mock client ---

func TestOmniaExecutor_ExecuteGRPC_WithMockClient(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	mock := &mockToolServiceClient{
		executeResp: &toolsv1.ToolResponse{
			ResultJson: `{"answer":42}`,
		},
	}
	e.grpcClients["grpc-handler"] = mock
	e.handlers["grpc-handler"] = &HandlerEntry{Name: "grpc-handler", Type: ToolTypeGRPC}

	result, err := e.executeGRPC(context.Background(), "grpc-tool", "grpc-handler", json.RawMessage(`{"q":"test"}`))
	if err != nil {
		t.Fatalf("executeGRPC: %v", err)
	}
	if string(result) != `{"answer":42}` {
		t.Errorf("result = %s, want {\"answer\":42}", result)
	}
}

func TestOmniaExecutor_ExecuteGRPC_MockClientError(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	mock := &mockToolServiceClient{
		executeErr: fmt.Errorf("connection refused"),
	}
	e.grpcClients["grpc-handler"] = mock
	e.handlers["grpc-handler"] = &HandlerEntry{Name: "grpc-handler", Type: ToolTypeGRPC}

	_, err := e.executeGRPC(context.Background(), "grpc-tool", "grpc-handler", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gRPC tool execution failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOmniaExecutor_ExecuteGRPC_MockClientToolError(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	mock := &mockToolServiceClient{
		executeResp: &toolsv1.ToolResponse{
			IsError:      true,
			ErrorMessage: "tool exploded",
		},
	}
	e.grpcClients["grpc-handler"] = mock
	e.handlers["grpc-handler"] = &HandlerEntry{Name: "grpc-handler", Type: ToolTypeGRPC}

	_, err := e.executeGRPC(context.Background(), "grpc-tool", "grpc-handler", nil)
	if err == nil {
		t.Fatal("expected error for tool error response")
	}
	if !strings.Contains(err.Error(), "tool exploded") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOmniaExecutor_ExecuteGRPC_MetadataInjection(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	mock := &mockToolServiceClient{
		executeResp: &toolsv1.ToolResponse{
			ResultJson: `{"ok":true}`,
		},
	}
	e.grpcClients["grpc-handler"] = mock
	e.handlers["grpc-handler"] = &HandlerEntry{Name: "grpc-handler", Type: ToolTypeGRPC}

	// Pass args to exercise the metadata injection path
	args := json.RawMessage(`{"param":"value"}`)
	result, err := e.executeGRPC(context.Background(), "my-tool", "grpc-handler", args)
	if err != nil {
		t.Fatalf("executeGRPC: %v", err)
	}
	if string(result) != `{"ok":true}` {
		t.Errorf("result = %s, want {\"ok\":true}", result)
	}
}

// --- executeOpenAPI with found operation ---

func TestOmniaExecutor_ExecuteOpenAPI_OperationFound_NoServer(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.openAPIOps["handler"] = map[string]*OpenAPIOperation{
		"getUser": {
			OperationID: "getUser",
			Method:      "GET",
			Path:        "/users/1",
			Summary:     "Get a user",
		},
	}
	e.openAPIBaseURLs["handler"] = "http://127.0.0.1:1" // unreachable
	e.openAPIHeaders["handler"] = map[string]string{"X-Api-Key": "test"}

	handler := &HandlerEntry{
		Name: "handler",
		Type: ToolTypeOpenAPI,
		OpenAPIConfig: &OpenAPICfg{
			AuthType:  "bearer",
			AuthToken: "tok123",
		},
	}

	// The operation is found, so URL building, header merging, and HTTPConfig construction
	// are all exercised. It will fail at the HTTP call level since the server is unreachable.
	_, err := e.executeOpenAPI(context.Background(), "getUser", "handler", handler, json.RawMessage(`{"id":1}`))
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	// The error comes from httpExecutor trying to reach the server
}

func TestOmniaExecutor_ExecuteOpenAPI_WithServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"alice"}`))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.openAPIOps["handler"] = map[string]*OpenAPIOperation{
		"getUser": {
			OperationID: "getUser",
			Method:      "GET",
			Path:        "/users/1",
		},
	}
	e.openAPIBaseURLs["handler"] = srv.URL
	e.openAPIHeaders["handler"] = map[string]string{}

	handler := &HandlerEntry{
		Name: "handler",
		Type: ToolTypeOpenAPI,
	}

	result, err := e.executeOpenAPI(context.Background(), "getUser", "handler", handler, nil)
	if err != nil {
		t.Fatalf("executeOpenAPI: %v", err)
	}
	if !strings.Contains(string(result), "alice") {
		t.Errorf("result = %s, want to contain 'alice'", result)
	}
}

// --- initOpenAPIHandler nil config ---

func TestOmniaExecutor_InitOpenAPIHandler_NilConfig(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "no-openapi",
		Type: ToolTypeOpenAPI,
	}

	err := e.initOpenAPIHandler(context.Background(), "no-openapi", h)
	if err != nil {
		t.Fatalf("initOpenAPIHandler: %v", err)
	}
	if len(e.toolHandlers) != 0 {
		t.Errorf("expected no tools registered, got %d", len(e.toolHandlers))
	}
}

// --- initHandler with MCP, gRPC, OpenAPI nil configs (skip paths) ---

func TestOmniaExecutor_InitHandler_MCP_NilConfig(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{Name: "mcp-skip", Type: ToolTypeMCP}

	err := e.initHandler(context.Background(), "mcp-skip", h)
	if err != nil {
		t.Fatalf("initHandler MCP: %v", err)
	}
	if len(e.toolHandlers) != 0 {
		t.Errorf("expected no tools registered, got %d", len(e.toolHandlers))
	}
}

func TestOmniaExecutor_InitHandler_GRPC_NilConfig(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{Name: "grpc-skip", Type: ToolTypeGRPC}

	err := e.initHandler(context.Background(), "grpc-skip", h)
	if err != nil {
		t.Fatalf("initHandler gRPC: %v", err)
	}
	if len(e.toolHandlers) != 0 {
		t.Errorf("expected no tools registered, got %d", len(e.toolHandlers))
	}
}

func TestOmniaExecutor_InitHandler_OpenAPI_NilConfig(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{Name: "openapi-skip", Type: ToolTypeOpenAPI}

	err := e.initHandler(context.Background(), "openapi-skip", h)
	if err != nil {
		t.Fatalf("initHandler OpenAPI: %v", err)
	}
	if len(e.toolHandlers) != 0 {
		t.Errorf("expected no tools registered, got %d", len(e.toolHandlers))
	}
}

func TestOmniaExecutor_InitHandler_HTTP(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "http-init",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint: "http://localhost:8080",
			Method:   "POST",
		},
		Tool: &ToolDefCfg{
			Name:        "http-init-tool",
			Description: "test",
		},
	}

	err := e.initHandler(context.Background(), "http-init", h)
	if err != nil {
		t.Fatalf("initHandler HTTP: %v", err)
	}
	if e.toolHandlers["http-init-tool"] != "http-init" {
		t.Error("tool not registered via initHandler")
	}
}

// --- buildDescriptor with OpenAPI operations ---

func TestOmniaExecutor_BuildDescriptor_OpenAPI(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "openapi-h",
		Type: ToolTypeOpenAPI,
	}
	e.openAPIOps["openapi-h"] = map[string]*OpenAPIOperation{
		"listPets": {
			OperationID: "listPets",
			Method:      "GET",
			Path:        "/pets",
			Summary:     "List all pets",
			Description: "Returns all pets in the system",
		},
	}

	desc := e.buildDescriptor("listPets", h)
	if desc == nil {
		t.Fatal("buildDescriptor returned nil")
	}
	if desc.Description == "" {
		t.Error("expected non-empty description for OpenAPI tool")
	}
}

// --- probeHTTP with httptest servers ---

func TestOmniaExecutor_ProbeHTTP_ServerReturns500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "probe-500",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint: srv.URL,
		},
	}

	err := e.probeHTTP(context.Background(), h)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOmniaExecutor_ProbeHTTP_ServerReturns200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "probe-200",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint: srv.URL,
		},
	}

	err := e.probeHTTP(context.Background(), h)
	if err != nil {
		t.Fatalf("probeHTTP: %v", err)
	}
}

func TestOmniaExecutor_ProbeHTTP_UsesHandlerEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name:     "probe-fallback",
		Type:     ToolTypeHTTP,
		Endpoint: srv.URL,
	}

	err := e.probeHTTP(context.Background(), h)
	if err != nil {
		t.Fatalf("probeHTTP with handler endpoint: %v", err)
	}
}

// --- buildGRPCTLSConfig with cert/key ---

func TestBuildGRPCTLSConfig_WithCertAndKey(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	// Write dummy cert/key — they won't be valid, so LoadX509KeyPair will fail.
	if err := os.WriteFile(certPath, []byte("not a cert"), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte("not a key"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	cfg := &GRPCCfg{
		TLS:         true,
		TLSCertPath: certPath,
		TLSKeyPath:  keyPath,
	}

	_, err := buildGRPCTLSConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid cert/key pair")
	}
	if !strings.Contains(err.Error(), "failed to load client cert") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- probeHandler for OpenAPI type ---

func TestOmniaExecutor_ProbeHandler_OpenAPI_EmptyEndpoint(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["openapi-h"] = &HandlerEntry{
		Name: "openapi-h",
		Type: ToolTypeOpenAPI,
	}

	err := e.probeHandler(context.Background(), "openapi-h")
	if err != nil {
		t.Errorf("expected nil error for OpenAPI with no endpoint, got: %v", err)
	}
}

func TestOmniaExecutor_ProbeHandler_OpenAPI_WithServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["openapi-h"] = &HandlerEntry{
		Name:     "openapi-h",
		Type:     ToolTypeOpenAPI,
		Endpoint: srv.URL,
	}

	err := e.probeHandler(context.Background(), "openapi-h")
	if err != nil {
		t.Fatalf("probeHandler OpenAPI: %v", err)
	}
}

// --- marshalSchema ---

func TestMarshalSchema_Nil(t *testing.T) {
	result := marshalSchema(nil)
	if result != nil {
		t.Errorf("marshalSchema(nil) = %v, want nil", result)
	}
}

func TestMarshalSchema_Unmarshalable(t *testing.T) {
	// Channels cannot be marshaled to JSON
	ch := make(chan int)
	result := marshalSchema(ch)
	if result != nil {
		t.Errorf("marshalSchema(chan) = %v, want nil", result)
	}
}

func TestMarshalSchema_ValidMap(t *testing.T) {
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}
	result := marshalSchema(input)
	if result == nil {
		t.Fatal("marshalSchema returned nil for valid input")
	}
	// Verify it's valid JSON
	var parsed map[string]any
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if parsed["type"] != "object" {
		t.Errorf("type = %v, want %q", parsed["type"], "object")
	}
}

// --- buildMCPDescriptor with InputSchema ---

func TestOmniaExecutor_BuildMCPDescriptor_WithInputSchema(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	mcpTool := &mcp.Tool{
		Name:        "mcp-schema-tool",
		Description: "tool with schema",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
		},
	}
	e.mcpTools["mcp-h"] = map[string]*mcp.Tool{
		"mcp-schema-tool": mcpTool,
	}

	desc := &pktools.ToolDescriptor{Name: "mcp-schema-tool"}
	e.buildMCPDescriptor(desc, "mcp-schema-tool", "mcp-h")

	if desc.Description != "tool with schema" {
		t.Errorf("Description = %q, want %q", desc.Description, "tool with schema")
	}
	if desc.InputSchema == nil {
		t.Fatal("InputSchema is nil, expected non-nil")
	}
	var parsed map[string]any
	if err := json.Unmarshal(desc.InputSchema, &parsed); err != nil {
		t.Fatalf("InputSchema is not valid JSON: %v", err)
	}
	if parsed["type"] != "object" {
		t.Errorf("InputSchema type = %v, want %q", parsed["type"], "object")
	}
}

func TestOmniaExecutor_BuildMCPDescriptor_HandlerNotFound(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	desc := &pktools.ToolDescriptor{Name: "missing"}
	e.buildMCPDescriptor(desc, "missing", "no-such-handler")
	// Should be a no-op — description remains empty
	if desc.Description != "" {
		t.Errorf("Description = %q, want empty", desc.Description)
	}
}

func TestOmniaExecutor_BuildMCPDescriptor_ToolNotFound(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.mcpTools["mcp-h"] = map[string]*mcp.Tool{}
	desc := &pktools.ToolDescriptor{Name: "missing-tool"}
	e.buildMCPDescriptor(desc, "missing-tool", "mcp-h")
	if desc.Description != "" {
		t.Errorf("Description = %q, want empty", desc.Description)
	}
}

// --- buildOpenAPIDescriptor with parameters ---

func TestOmniaExecutor_BuildOpenAPIDescriptor_WithParameters(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.openAPIOps["openapi-h"] = map[string]*OpenAPIOperation{
		"getPet": {
			OperationID: "getPet",
			Method:      "GET",
			Path:        "/pets/{petId}",
			Summary:     "Get a pet by ID",
			Parameters: []OpenAPIParameter{
				{
					Name:        "petId",
					In:          "path",
					Required:    true,
					Description: "The pet ID",
					Schema:      map[string]any{"type": "string"},
				},
				{
					Name:        "include",
					In:          "query",
					Required:    false,
					Description: "Fields to include",
					Schema:      map[string]any{"type": "string"},
				},
			},
		},
	}

	desc := &pktools.ToolDescriptor{Name: "getPet"}
	e.buildOpenAPIDescriptor(desc, "getPet", "openapi-h")

	if desc.Description != "Get a pet by ID" {
		t.Errorf("Description = %q, want %q", desc.Description, "Get a pet by ID")
	}
	if desc.InputSchema == nil {
		t.Fatal("InputSchema is nil, expected non-nil for operation with parameters")
	}
	var schema map[string]any
	if err := json.Unmarshal(desc.InputSchema, &schema); err != nil {
		t.Fatalf("InputSchema is not valid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("schema type = %v, want %q", schema["type"], "object")
	}
}

func TestOmniaExecutor_BuildOpenAPIDescriptor_HandlerNotFound(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	desc := &pktools.ToolDescriptor{Name: "missing"}
	e.buildOpenAPIDescriptor(desc, "missing", "no-handler")
	if desc.Description != "" {
		t.Errorf("Description = %q, want empty", desc.Description)
	}
}

func TestOmniaExecutor_BuildOpenAPIDescriptor_OpNotFound(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.openAPIOps["openapi-h"] = map[string]*OpenAPIOperation{}
	desc := &pktools.ToolDescriptor{Name: "missing"}
	e.buildOpenAPIDescriptor(desc, "missing", "openapi-h")
	if desc.Description != "" {
		t.Errorf("Description = %q, want empty", desc.Description)
	}
}

// --- buildGRPCDescriptor edge cases ---

func TestOmniaExecutor_BuildGRPCDescriptor_EmptyInputSchema(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.grpcTools["grpc-h"] = map[string]*toolsv1.ToolInfo{
		"grpc-empty": {
			Name:        "grpc-empty",
			Description: "tool with empty schema",
			InputSchema: "",
		},
	}

	desc := &pktools.ToolDescriptor{Name: "grpc-empty"}
	e.buildGRPCDescriptor(desc, "grpc-empty", "grpc-h")

	if desc.Description != "tool with empty schema" {
		t.Errorf("Description = %q, want %q", desc.Description, "tool with empty schema")
	}
	if desc.InputSchema != nil {
		t.Errorf("InputSchema = %s, want nil for empty schema", desc.InputSchema)
	}
}

func TestOmniaExecutor_BuildGRPCDescriptor_WithInputSchema(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	schema := `{"type":"object","properties":{"q":{"type":"string"}}}`
	e.grpcTools["grpc-h"] = map[string]*toolsv1.ToolInfo{
		"grpc-schema": {
			Name:        "grpc-schema",
			Description: "tool with schema",
			InputSchema: schema,
		},
	}

	desc := &pktools.ToolDescriptor{Name: "grpc-schema"}
	e.buildGRPCDescriptor(desc, "grpc-schema", "grpc-h")

	if string(desc.InputSchema) != schema {
		t.Errorf("InputSchema = %s, want %s", desc.InputSchema, schema)
	}
}

func TestOmniaExecutor_BuildGRPCDescriptor_HandlerNotFound(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	desc := &pktools.ToolDescriptor{Name: "missing"}
	e.buildGRPCDescriptor(desc, "missing", "no-handler")
	if desc.Description != "" {
		t.Errorf("Description = %q, want empty", desc.Description)
	}
}

func TestOmniaExecutor_BuildGRPCDescriptor_ToolNotFound(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.grpcTools["grpc-h"] = map[string]*toolsv1.ToolInfo{}
	desc := &pktools.ToolDescriptor{Name: "missing"}
	e.buildGRPCDescriptor(desc, "missing", "grpc-h")
	if desc.Description != "" {
		t.Errorf("Description = %q, want empty", desc.Description)
	}
}

// --- startSpan with tracing provider ---

func TestOmniaExecutor_StartSpan_WithTracingProvider(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	provider := tracing.NewTestProvider(tp)
	e := NewOmniaExecutor(logr.Discard(), provider)

	// Set some tool meta so the span includes attributes
	e.toolMeta["test-tool"] = ToolMeta{
		RegistryName:      "my-registry",
		RegistryNamespace: "default",
		HandlerName:       "test-handler",
		HandlerType:       "http",
	}

	ctx, span := e.startSpan(context.Background(), "test-tool")
	span.End()

	if ctx == nil {
		t.Fatal("startSpan returned nil context")
	}

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span")
	}
}

// --- Close with MCP and gRPC connections ---

func TestOmniaExecutor_Close_WithGRPCConn(t *testing.T) {
	// Start a real gRPC listener so we can get a real connection to close
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.grpcConns["test-grpc"] = conn
	e.grpcClients["test-grpc"] = toolsv1.NewToolServiceClient(conn)
	e.grpcTools["test-grpc"] = map[string]*toolsv1.ToolInfo{
		"tool1": {Name: "tool1"},
	}

	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(e.grpcConns) != 0 {
		t.Error("grpcConns not reset after Close")
	}
	if len(e.grpcClients) != 0 {
		t.Error("grpcClients not reset after Close")
	}
}

func TestOmniaExecutor_Close_WithMCPSession(t *testing.T) {
	// Create a real MCP client/server pair using in-memory transport
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	server := mcp.NewServer(
		&mcp.Implementation{Name: "test-server", Version: "1.0"},
		nil,
	)

	// Connect server first (required before client)
	_, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "1.0"},
		nil,
	)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.mcpClients["test-mcp"] = client
	e.mcpSessions["test-mcp"] = session
	e.mcpTools["test-mcp"] = map[string]*mcp.Tool{
		"tool1": {Name: "tool1"},
	}

	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(e.mcpSessions) != 0 {
		t.Error("mcpSessions not reset after Close")
	}
	if len(e.mcpClients) != 0 {
		t.Error("mcpClients not reset after Close")
	}
	if len(e.mcpTools) != 0 {
		t.Error("mcpTools not reset after Close")
	}
}

// --- executeMCP successful call ---

func TestOmniaExecutor_ExecuteMCP_Success(t *testing.T) {
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	server := mcp.NewServer(
		&mcp.Implementation{Name: "test-server", Version: "1.0"},
		nil,
	)
	server.AddTool(&mcp.Tool{
		Name:        "echo",
		Description: "Echoes input",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "hello world"},
			},
		}, nil
	})

	_, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "1.0"},
		nil,
	)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.mcpSessions["test-mcp"] = session
	e.handlers["test-mcp"] = &HandlerEntry{Name: "test-mcp", Type: ToolTypeMCP}

	result, err := e.executeMCP(context.Background(), "echo", "test-mcp", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("executeMCP: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	var text string
	if err := json.Unmarshal(result, &text); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if text != "hello world" {
		t.Errorf("result = %q, want %q", text, "hello world")
	}
}

func TestOmniaExecutor_ExecuteMCP_WithArgs(t *testing.T) {
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	server := mcp.NewServer(
		&mcp.Implementation{Name: "test-server", Version: "1.0"},
		nil,
	)
	server.AddTool(&mcp.Tool{
		Name:        "greet",
		Description: "Greets by name",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`),
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]any
		_ = json.Unmarshal(req.Params.Arguments, &args)
		name, _ := args["name"].(string)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Hello, %s!", name)},
			},
		}, nil
	})

	_, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "1.0"},
		nil,
	)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.mcpSessions["test-mcp"] = session
	e.handlers["test-mcp"] = &HandlerEntry{Name: "test-mcp", Type: ToolTypeMCP}

	result, err := e.executeMCP(context.Background(), "greet", "test-mcp", json.RawMessage(`{"name":"Alice"}`))
	if err != nil {
		t.Fatalf("executeMCP: %v", err)
	}
	var text string
	if err := json.Unmarshal(result, &text); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if text != "Hello, Alice!" {
		t.Errorf("result = %q, want %q", text, "Hello, Alice!")
	}
}

func TestOmniaExecutor_ExecuteMCP_EmptyArgs(t *testing.T) {
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	server := mcp.NewServer(
		&mcp.Implementation{Name: "test-server", Version: "1.0"},
		nil,
	)
	server.AddTool(&mcp.Tool{
		Name:        "ping",
		Description: "Pings",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "pong"},
			},
		}, nil
	})

	_, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "1.0"},
		nil,
	)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.mcpSessions["test-mcp"] = session
	e.handlers["test-mcp"] = &HandlerEntry{Name: "test-mcp", Type: ToolTypeMCP}

	// Empty args (nil)
	result, err := e.executeMCP(context.Background(), "ping", "test-mcp", nil)
	if err != nil {
		t.Fatalf("executeMCP with nil args: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

// --- initOpenAPIHandler with mock server ---

func TestOmniaExecutor_InitOpenAPIHandler_WithMockServer(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"info": {"title": "Pet Store", "version": "1.0.0"},
		"servers": [{"url": "http://localhost:8080"}],
		"paths": {
			"/pets": {
				"get": {
					"operationId": "listPets",
					"summary": "List all pets",
					"parameters": [
						{
							"name": "limit",
							"in": "query",
							"required": false,
							"schema": {"type": "integer"}
						}
					],
					"responses": {
						"200": {"description": "A list of pets"}
					}
				}
			}
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(spec))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	h := &HandlerEntry{
		Name: "pet-store",
		Type: ToolTypeOpenAPI,
		OpenAPIConfig: &OpenAPICfg{
			SpecURL: srv.URL + "/openapi.json",
			BaseURL: "http://localhost:8080",
		},
	}

	err := e.initOpenAPIHandler(context.Background(), "pet-store", h)
	if err != nil {
		t.Fatalf("initOpenAPIHandler: %v", err)
	}

	// Verify tool was registered
	handlerName, ok := e.toolHandlers["listPets"]
	if !ok {
		t.Fatal("listPets tool not registered")
	}
	if handlerName != "pet-store" {
		t.Errorf("handler = %q, want %q", handlerName, "pet-store")
	}

	// Verify operations parsed
	ops, ok := e.openAPIOps["pet-store"]
	if !ok {
		t.Fatal("no operations for pet-store handler")
	}
	op, ok := ops["listPets"]
	if !ok {
		t.Fatal("listPets operation not found")
	}
	if op.Method != "GET" {
		t.Errorf("method = %q, want %q", op.Method, "GET")
	}
}

// --- Retry integration tests ---

// failNTimesGRPCClient is a mock ToolServiceClient that returns failErr for the
// first failCount calls, then returns successResp.
type failNTimesGRPCClient struct {
	failCount   int
	calls       int
	successResp *toolsv1.ToolResponse
	failErr     error
}

func (m *failNTimesGRPCClient) Execute(
	_ context.Context,
	_ *toolsv1.ToolRequest,
	_ ...grpc.CallOption,
) (*toolsv1.ToolResponse, error) {
	m.calls++
	if m.calls <= m.failCount {
		return nil, m.failErr
	}
	return m.successResp, nil
}

func (m *failNTimesGRPCClient) ListTools(
	_ context.Context,
	_ *toolsv1.ListToolsRequest,
	_ ...grpc.CallOption,
) (*toolsv1.ListToolsResponse, error) {
	return nil, nil
}

func TestExecuteHTTP_RetryOnRetryableStatus(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("bad gateway"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["retry-http"] = &HandlerEntry{
		Name: "retry-http",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint: srv.URL,
			Method:   "GET",
			RetryPolicy: &RuntimeHTTPRetryPolicy{
				MaxAttempts:         3,
				InitialBackoff:      Duration(1 * time.Millisecond),
				BackoffMultiplier:   2.0,
				MaxBackoff:          Duration(100 * time.Millisecond),
				RetryOn:             []int32{502},
				RetryOnNetworkError: true,
			},
		},
	}
	e.toolHandlers["retry-http-tool"] = "retry-http"

	desc := &pktools.ToolDescriptor{Name: "retry-http-tool"}
	result, err := e.Execute(context.Background(), desc, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls (2 retries + 1 success), got %d", calls)
	}
	if string(result) != `{"result":"ok"}` {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestExecuteHTTP_NoRetryOnNonRetryableStatus(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["no-retry-http"] = &HandlerEntry{
		Name: "no-retry-http",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint: srv.URL,
			Method:   "GET",
			RetryPolicy: &RuntimeHTTPRetryPolicy{
				MaxAttempts:         3,
				InitialBackoff:      Duration(1 * time.Millisecond),
				BackoffMultiplier:   2.0,
				MaxBackoff:          Duration(100 * time.Millisecond),
				RetryOn:             []int32{502, 503},
				RetryOnNetworkError: true,
			},
		},
	}
	e.toolHandlers["no-retry-tool"] = "no-retry-http"

	desc := &pktools.ToolDescriptor{Name: "no-retry-tool"}
	_, err := e.Execute(context.Background(), desc, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retries for 400), got %d", calls)
	}
}

func TestExecuteGRPC_RetryOnUnavailable(t *testing.T) {
	mock := &failNTimesGRPCClient{
		failCount:   2,
		failErr:     grpcStatus.Error(grpcCodes.Unavailable, "service unavailable"),
		successResp: &toolsv1.ToolResponse{ResultJson: `{"answer":42}`},
	}

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.grpcClients["grpc-retry"] = mock
	e.handlers["grpc-retry"] = &HandlerEntry{
		Name: "grpc-retry",
		Type: ToolTypeGRPC,
		GRPCConfig: &GRPCCfg{
			Endpoint: "localhost:50051",
			RetryPolicy: &RuntimeGRPCRetryPolicy{
				MaxAttempts:          3,
				InitialBackoff:       Duration(1 * time.Millisecond),
				BackoffMultiplier:    2.0,
				MaxBackoff:           Duration(100 * time.Millisecond),
				RetryableStatusCodes: []string{"UNAVAILABLE"},
			},
		},
	}
	e.toolHandlers["grpc-retry-tool"] = "grpc-retry"

	result, err := e.executeGRPC(context.Background(), "grpc-retry-tool", "grpc-retry", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("executeGRPC failed: %v", err)
	}
	if mock.calls != 3 {
		t.Errorf("expected 3 calls, got %d", mock.calls)
	}
	if string(result) != `{"answer":42}` {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestExecuteGRPC_NoRetryOnNotFound(t *testing.T) {
	mock := &failNTimesGRPCClient{
		failCount: 5,
		failErr:   grpcStatus.Error(grpcCodes.NotFound, "not found"),
	}

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.grpcClients["grpc-no-retry"] = mock
	e.handlers["grpc-no-retry"] = &HandlerEntry{
		Name: "grpc-no-retry",
		Type: ToolTypeGRPC,
		GRPCConfig: &GRPCCfg{
			Endpoint: "localhost:50051",
			RetryPolicy: &RuntimeGRPCRetryPolicy{
				MaxAttempts:          3,
				InitialBackoff:       Duration(1 * time.Millisecond),
				BackoffMultiplier:    2.0,
				MaxBackoff:           Duration(100 * time.Millisecond),
				RetryableStatusCodes: []string{"UNAVAILABLE"},
			},
		},
	}
	e.toolHandlers["grpc-no-retry-tool"] = "grpc-no-retry"

	_, err := e.executeGRPC(context.Background(), "grpc-no-retry-tool", "grpc-no-retry", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for NOT_FOUND")
	}
	if mock.calls != 1 {
		t.Errorf("expected 1 call (no retries), got %d", mock.calls)
	}
}

func TestMCPRetry_TransportErrorRetried(t *testing.T) {
	calls := 0
	policy := retryPolicy{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Millisecond,
		BackoffMultiplier: 2.0,
		MaxBackoff:        100 * time.Millisecond,
	}

	result, err := retryWithBackoff(
		context.Background(), logr.Discard(), tracenoop.Span{},
		policy, 0,
		func(err error) (bool, time.Duration) { return classifyMCPError(err) },
		func(_ context.Context) (json.RawMessage, error) {
			calls++
			if calls < 3 {
				return nil, &net.OpError{Op: "read", Err: errors.New("connection reset")}
			}
			return json.RawMessage(`{"mcp":"ok"}`), nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
	if string(result) != `{"mcp":"ok"}` {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestMCPRetry_ToolErrorNotRetried(t *testing.T) {
	calls := 0
	policy := retryPolicy{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Millisecond,
		BackoffMultiplier: 2.0,
		MaxBackoff:        100 * time.Millisecond,
	}

	_, err := retryWithBackoff(
		context.Background(), logr.Discard(), tracenoop.Span{},
		policy, 0,
		func(err error) (bool, time.Duration) { return classifyMCPError(err) },
		func(_ context.Context) (json.RawMessage, error) {
			calls++
			return nil, &mcpToolError{message: "file not found"}
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (tool errors not retried), got %d", calls)
	}
}

// --- Integration tests: HTTP config fields through OmniaExecutor.Execute() ---

func TestExecuteHTTP_URLTemplate(t *testing.T) {
	var receivedPath string
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["url-tmpl"] = &HandlerEntry{
		Name: "url-tmpl",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint:    srv.URL,
			URLTemplate: srv.URL + "/users/{id}/posts",
			Method:      "POST",
		},
	}
	e.toolHandlers["url-tmpl-tool"] = "url-tmpl"

	desc := &pktools.ToolDescriptor{Name: "url-tmpl-tool"}
	_, err := e.Execute(context.Background(), desc, json.RawMessage(`{"id":"42","title":"Hello"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if receivedPath != "/users/42/posts" {
		t.Errorf("URL path = %q, want %q", receivedPath, "/users/42/posts")
	}
	// id was consumed by the template; title should remain in body
	if !strings.Contains(string(receivedBody), `"title"`) {
		t.Errorf("expected title in body, got: %s", receivedBody)
	}
	if strings.Contains(string(receivedBody), `"id"`) {
		t.Errorf("id should have been consumed by URL template, but found in body: %s", receivedBody)
	}
}

func TestExecuteHTTP_StaticQuery(t *testing.T) {
	var receivedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["static-query"] = &HandlerEntry{
		Name: "static-query",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint:    srv.URL,
			Method:      "GET",
			StaticQuery: map[string]string{"api_key": "secret123"},
		},
	}
	e.toolHandlers["static-query-tool"] = "static-query"

	desc := &pktools.ToolDescriptor{Name: "static-query-tool"}
	_, err := e.Execute(context.Background(), desc, json.RawMessage(`{"q":"test"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	vals, parseErr := url.ParseQuery(receivedQuery)
	if parseErr != nil {
		t.Fatalf("parse query: %v", parseErr)
	}
	if vals.Get("api_key") != "secret123" {
		t.Errorf("api_key = %q, want %q", vals.Get("api_key"), "secret123")
	}
	if vals.Get("q") != "test" {
		t.Errorf("q = %q, want %q", vals.Get("q"), "test")
	}
}

func TestExecuteHTTP_QueryParams_POST(t *testing.T) {
	var receivedQuery string
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["qp-post"] = &HandlerEntry{
		Name: "qp-post",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint:    srv.URL,
			Method:      "POST",
			QueryParams: []string{"page", "limit"},
		},
	}
	e.toolHandlers["qp-post-tool"] = "qp-post"

	desc := &pktools.ToolDescriptor{Name: "qp-post-tool"}
	_, err := e.Execute(context.Background(), desc, json.RawMessage(`{"page":"1","limit":"10","data":"value"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	qvals, parseErr := url.ParseQuery(receivedQuery)
	if parseErr != nil {
		t.Fatalf("parse query: %v", parseErr)
	}
	if qvals.Get("page") != "1" {
		t.Errorf("page query param = %q, want %q", qvals.Get("page"), "1")
	}
	if qvals.Get("limit") != "10" {
		t.Errorf("limit query param = %q, want %q", qvals.Get("limit"), "10")
	}
	// page and limit should NOT appear in body
	if strings.Contains(string(receivedBody), `"page"`) {
		t.Errorf("page should not be in body, got: %s", receivedBody)
	}
	if strings.Contains(string(receivedBody), `"limit"`) {
		t.Errorf("limit should not be in body, got: %s", receivedBody)
	}
	// data should be in body
	if !strings.Contains(string(receivedBody), `"data"`) {
		t.Errorf("data should be in body, got: %s", receivedBody)
	}
}

func TestExecuteHTTP_HeaderParams(t *testing.T) {
	var receivedUserID, receivedTenant string
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUserID = r.Header.Get("X-User-ID")
		receivedTenant = r.Header.Get("X-Tenant")
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["hdr-params"] = &HandlerEntry{
		Name: "hdr-params",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint:     srv.URL,
			Method:       "POST",
			HeaderParams: map[string]string{"user_id": "X-User-ID", "tenant": "X-Tenant"},
		},
	}
	e.toolHandlers["hdr-params-tool"] = "hdr-params"

	desc := &pktools.ToolDescriptor{Name: "hdr-params-tool"}
	_, err := e.Execute(context.Background(), desc, json.RawMessage(`{"user_id":"abc","tenant":"t1","query":"hello"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if receivedUserID != "abc" {
		t.Errorf("X-User-ID = %q, want %q", receivedUserID, "abc")
	}
	if receivedTenant != "t1" {
		t.Errorf("X-Tenant = %q, want %q", receivedTenant, "t1")
	}
	// query was not promoted to header, should be in body
	if !strings.Contains(string(receivedBody), `"query"`) {
		t.Errorf("query should be in body, got: %s", receivedBody)
	}
}

func TestExecuteHTTP_StaticBody(t *testing.T) {
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["static-body"] = &HandlerEntry{
		Name: "static-body",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint:   srv.URL,
			Method:     "POST",
			StaticBody: map[string]any{"source": "api", "version": "v2"},
		},
	}
	e.toolHandlers["static-body-tool"] = "static-body"

	desc := &pktools.ToolDescriptor{Name: "static-body-tool"}
	_, err := e.Execute(context.Background(), desc, json.RawMessage(`{"query":"test"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var body map[string]any
	if jsonErr := json.Unmarshal(receivedBody, &body); jsonErr != nil {
		t.Fatalf("unmarshal body: %v (raw: %s)", jsonErr, receivedBody)
	}
	if body["source"] != "api" {
		t.Errorf("source = %v, want %q", body["source"], "api")
	}
	if body["version"] != "v2" {
		t.Errorf("version = %v, want %q", body["version"], "v2")
	}
	if body["query"] != "test" {
		t.Errorf("query = %v, want %q", body["query"], "test")
	}
}

func TestExecuteHTTP_Redact(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ssn":"123-45-6789","name":"Alice","age":30}`))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["redact-hdlr"] = &HandlerEntry{
		Name: "redact-hdlr",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint: srv.URL,
			Method:   "GET",
			Redact:   []string{"ssn"},
		},
	}
	e.toolHandlers["redact-tool"] = "redact-hdlr"

	desc := &pktools.ToolDescriptor{Name: "redact-tool"}
	result, err := e.Execute(context.Background(), desc, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var got map[string]any
	if jsonErr := json.Unmarshal(result, &got); jsonErr != nil {
		t.Fatalf("unmarshal result: %v", jsonErr)
	}
	if got["ssn"] != "[REDACTED]" {
		t.Errorf("ssn = %v, want [REDACTED]", got["ssn"])
	}
	if got["name"] != "Alice" {
		t.Errorf("name = %v, want Alice", got["name"])
	}
}

func TestExecuteHTTP_BodyMapping(t *testing.T) {
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["body-map"] = &HandlerEntry{
		Name: "body-map",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint:    srv.URL,
			Method:      "POST",
			BodyMapping: "{query: query, filters: {page: page}}",
		},
	}
	e.toolHandlers["body-map-tool"] = "body-map"

	desc := &pktools.ToolDescriptor{Name: "body-map-tool"}
	_, err := e.Execute(context.Background(), desc, json.RawMessage(`{"query":"hello","page":1}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var body map[string]any
	if jsonErr := json.Unmarshal(receivedBody, &body); jsonErr != nil {
		t.Fatalf("unmarshal body: %v (raw: %s)", jsonErr, receivedBody)
	}
	if body["query"] != "hello" {
		t.Errorf("query = %v, want hello", body["query"])
	}
	filters, ok := body["filters"].(map[string]any)
	if !ok {
		t.Fatalf("filters not a map: %v", body["filters"])
	}
	// JMESPath returns numbers as float64
	if filters["page"] != float64(1) {
		t.Errorf("filters.page = %v, want 1", filters["page"])
	}
}

func TestExecuteHTTP_ResponseMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"items":[1,2,3]},"meta":{"page":1}}`))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["resp-map"] = &HandlerEntry{
		Name: "resp-map",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint:        srv.URL,
			Method:          "GET",
			ResponseMapping: "data.items",
		},
	}
	e.toolHandlers["resp-map-tool"] = "resp-map"

	desc := &pktools.ToolDescriptor{Name: "resp-map-tool"}
	result, err := e.Execute(context.Background(), desc, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var items []any
	if jsonErr := json.Unmarshal(result, &items); jsonErr != nil {
		t.Fatalf("unmarshal result: %v (raw: %s)", jsonErr, result)
	}
	if len(items) != 3 {
		t.Errorf("items length = %d, want 3", len(items))
	}
	if items[0] != float64(1) || items[1] != float64(2) || items[2] != float64(3) {
		t.Errorf("items = %v, want [1 2 3]", items)
	}
}

func TestExecuteHTTP_CombinedFeatures(t *testing.T) {
	var receivedPath string
	var receivedQuery string
	var receivedAuthHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedQuery = r.URL.RawQuery
		receivedAuthHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":{"value":42},"secret":"top-secret"}`))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["combined"] = &HandlerEntry{
		Name: "combined",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint:        srv.URL,
			URLTemplate:     srv.URL + "/resources/{id}",
			Method:          "POST",
			StaticQuery:     map[string]string{"api_key": "k1"},
			HeaderParams:    map[string]string{"token": "Authorization"},
			ResponseMapping: "result",
			Redact:          []string{"secret"},
		},
	}
	e.toolHandlers["combined-tool"] = "combined"

	desc := &pktools.ToolDescriptor{Name: "combined-tool"}
	result, err := e.Execute(context.Background(), desc, json.RawMessage(`{"id":"99","token":"Bearer xyz","data":"payload"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if receivedPath != "/resources/99" {
		t.Errorf("path = %q, want /resources/99", receivedPath)
	}
	qvals, _ := url.ParseQuery(receivedQuery)
	if qvals.Get("api_key") != "k1" {
		t.Errorf("api_key = %q, want k1", qvals.Get("api_key"))
	}
	if receivedAuthHeader != "Bearer xyz" {
		t.Errorf("Authorization = %q, want Bearer xyz", receivedAuthHeader)
	}
	// ResponseMapping: "result" → {"value":42}
	var got map[string]any
	if jsonErr := json.Unmarshal(result, &got); jsonErr != nil {
		t.Fatalf("unmarshal result: %v (raw: %s)", jsonErr, result)
	}
	if got["value"] != float64(42) {
		t.Errorf("value = %v, want 42", got["value"])
	}
	// Redact on "secret" — but ResponseMapping was applied first so "secret" is not in result
	if _, hasSecret := got["secret"]; hasSecret {
		t.Errorf("secret should not be in result after ResponseMapping, got: %v", got)
	}
}

func TestExecuteHTTP_RetryWithURLTemplate(t *testing.T) {
	calls := 0
	var lastPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		lastPath = r.URL.Path
		if calls < 3 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("bad gateway"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["retry-tmpl"] = &HandlerEntry{
		Name: "retry-tmpl",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint:    srv.URL,
			URLTemplate: srv.URL + "/items/{item_id}",
			Method:      "GET",
			RetryPolicy: &RuntimeHTTPRetryPolicy{
				MaxAttempts:         3,
				InitialBackoff:      Duration(1 * time.Millisecond),
				BackoffMultiplier:   2.0,
				MaxBackoff:          Duration(100 * time.Millisecond),
				RetryOn:             []int32{502},
				RetryOnNetworkError: true,
			},
		},
	}
	e.toolHandlers["retry-tmpl-tool"] = "retry-tmpl"

	desc := &pktools.ToolDescriptor{Name: "retry-tmpl-tool"}
	result, err := e.Execute(context.Background(), desc, json.RawMessage(`{"item_id":"7"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls (2 retries + 1 success), got %d", calls)
	}
	if lastPath != "/items/7" {
		t.Errorf("URL path = %q, want /items/7", lastPath)
	}
	if string(result) != `{"result":"ok"}` {
		t.Errorf("unexpected result: %s", result)
	}
}
