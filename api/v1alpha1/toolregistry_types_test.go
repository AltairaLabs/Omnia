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
	"encoding/json"
	"testing"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test constants
const (
	testToolRegistryName      = "test-toolregistry"
	testToolRegistryNamespace = "test-namespace"
	testHandlerName           = "my-handler"
	testToolName              = "my-tool"
	testToolEndpoint          = "https://api.example.com/tool"
	testToolDescription       = "A test tool"
)

func TestHandlerTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant HandlerType
		expected string
	}{
		{
			name:     "HTTP handler type",
			constant: HandlerTypeHTTP,
			expected: "http",
		},
		{
			name:     "OpenAPI handler type",
			constant: HandlerTypeOpenAPI,
			expected: "openapi",
		},
		{
			name:     "gRPC handler type",
			constant: HandlerTypeGRPC,
			expected: "grpc",
		},
		{
			name:     "MCP handler type",
			constant: HandlerTypeMCP,
			expected: "mcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("HandlerType constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestValidHandlerTypesContainsAllConstants(t *testing.T) {
	// Every HandlerType constant must be in ValidHandlerTypes.
	// If you add a new HandlerType and this test fails, add it to ValidHandlerTypes.
	allTypes := []HandlerType{
		HandlerTypeHTTP,
		HandlerTypeOpenAPI,
		HandlerTypeGRPC,
		HandlerTypeMCP,
		HandlerTypeClient,
	}
	for _, ht := range allTypes {
		if !ValidHandlerTypes[ht] {
			t.Errorf("HandlerType %q is not in ValidHandlerTypes — add it", ht)
		}
	}
	// ValidHandlerTypes should not contain extra entries.
	if len(ValidHandlerTypes) != len(allTypes) {
		t.Errorf("ValidHandlerTypes has %d entries but there are %d HandlerType constants", len(ValidHandlerTypes), len(allTypes))
	}
}

func TestToolRegistryPhaseConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant ToolRegistryPhase
		expected string
	}{
		{
			name:     "Pending phase",
			constant: ToolRegistryPhasePending,
			expected: "Pending",
		},
		{
			name:     "Ready phase",
			constant: ToolRegistryPhaseReady,
			expected: "Ready",
		},
		{
			name:     "Degraded phase",
			constant: ToolRegistryPhaseDegraded,
			expected: "Degraded",
		},
		{
			name:     "Failed phase",
			constant: ToolRegistryPhaseFailed,
			expected: "Failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("ToolRegistryPhase constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestToolStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "Available status",
			constant: ToolStatusAvailable,
			expected: "Available",
		},
		{
			name:     "Unavailable status",
			constant: ToolStatusUnavailable,
			expected: "Unavailable",
		},
		{
			name:     "Unknown status",
			constant: ToolStatusUnknown,
			expected: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("Tool status constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestMCPTransportConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant MCPTransport
		expected string
	}{
		{
			name:     "SSE transport",
			constant: MCPTransportSSE,
			expected: "sse",
		},
		{
			name:     "Stdio transport",
			constant: MCPTransportStdio,
			expected: "stdio",
		},
		{
			name:     "Streamable HTTP transport",
			constant: MCPTransportStreamableHTTP,
			expected: "streamable-http",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("MCPTransport constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestToolRegistryCreation(t *testing.T) {
	timeout := "60s"
	retries := int32(3)

	registry := &ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testToolRegistryName,
			Namespace: testToolRegistryNamespace,
		},
		Spec: ToolRegistrySpec{
			Handlers: []HandlerDefinition{
				{
					Name: testHandlerName,
					Type: HandlerTypeHTTP,
					HTTPConfig: &HTTPConfig{
						Endpoint: testToolEndpoint,
						Method:   "POST",
					},
					Tool: &ToolDefinition{
						Name:        testToolName,
						Description: testToolDescription,
						InputSchema: apiextensionsv1.JSON{
							Raw: []byte(`{"type":"object","properties":{"input":{"type":"string"}}}`),
						},
					},
					Timeout: &timeout,
					Retries: &retries,
				},
			},
		},
	}

	if registry.Name != testToolRegistryName {
		t.Errorf("ToolRegistry.Name = %v, want %v", registry.Name, testToolRegistryName)
	}

	if registry.Namespace != testToolRegistryNamespace {
		t.Errorf("ToolRegistry.Namespace = %v, want %v", registry.Namespace, testToolRegistryNamespace)
	}

	if len(registry.Spec.Handlers) != 1 {
		t.Fatalf("len(ToolRegistry.Spec.Handlers) = %v, want 1", len(registry.Spec.Handlers))
	}

	handler := registry.Spec.Handlers[0]
	if handler.Name != testHandlerName {
		t.Errorf("Handler.Name = %v, want %v", handler.Name, testHandlerName)
	}

	if handler.Type != HandlerTypeHTTP {
		t.Errorf("Handler.Type = %v, want %v", handler.Type, HandlerTypeHTTP)
	}

	if handler.HTTPConfig == nil {
		t.Fatal("Handler.HTTPConfig is nil")
	}

	if handler.HTTPConfig.Endpoint != testToolEndpoint {
		t.Errorf("Handler.HTTPConfig.Endpoint = %v, want %v", handler.HTTPConfig.Endpoint, testToolEndpoint)
	}

	if handler.Tool == nil {
		t.Fatal("Handler.Tool is nil")
	}

	if handler.Tool.Name != testToolName {
		t.Errorf("Handler.Tool.Name = %v, want %v", handler.Tool.Name, testToolName)
	}

	if handler.Tool.Description != testToolDescription {
		t.Errorf("Handler.Tool.Description = %v, want %v", handler.Tool.Description, testToolDescription)
	}
}

func TestToolRegistryWithMCPHandler(t *testing.T) {
	endpoint := "http://mcp-server:8080"

	registry := &ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testToolRegistryName,
			Namespace: testToolRegistryNamespace,
		},
		Spec: ToolRegistrySpec{
			Handlers: []HandlerDefinition{
				{
					Name: "mcp-handler",
					Type: HandlerTypeMCP,
					MCPConfig: &MCPConfig{
						Transport: MCPTransportSSE,
						Endpoint:  &endpoint,
					},
				},
			},
		},
	}

	handler := registry.Spec.Handlers[0]
	if handler.Type != HandlerTypeMCP {
		t.Errorf("Handler.Type = %v, want %v", handler.Type, HandlerTypeMCP)
	}

	if handler.MCPConfig == nil {
		t.Fatal("Handler.MCPConfig is nil")
	}

	if handler.MCPConfig.Transport != MCPTransportSSE {
		t.Errorf("MCPConfig.Transport = %v, want %v", handler.MCPConfig.Transport, MCPTransportSSE)
	}

	if handler.MCPConfig.Endpoint == nil || *handler.MCPConfig.Endpoint != endpoint {
		t.Errorf("MCPConfig.Endpoint = %v, want %v", handler.MCPConfig.Endpoint, endpoint)
	}
}

func TestMCPToolFilter(t *testing.T) {
	endpoint := "http://mcp-server:8080"

	handler := HandlerDefinition{
		Name: "filtered-mcp",
		Type: HandlerTypeMCP,
		MCPConfig: &MCPConfig{
			Transport: MCPTransportSSE,
			Endpoint:  &endpoint,
			ToolFilter: &MCPToolFilter{
				Allowlist: []string{"read_file", "write_file"},
				Blocklist: []string{"delete_file"},
			},
		},
	}

	if handler.MCPConfig.ToolFilter == nil {
		t.Fatal("MCPConfig.ToolFilter is nil")
	}

	if len(handler.MCPConfig.ToolFilter.Allowlist) != 2 {
		t.Errorf("Allowlist length = %d, want 2", len(handler.MCPConfig.ToolFilter.Allowlist))
	}

	if handler.MCPConfig.ToolFilter.Allowlist[0] != "read_file" {
		t.Errorf("Allowlist[0] = %v, want read_file", handler.MCPConfig.ToolFilter.Allowlist[0])
	}

	if len(handler.MCPConfig.ToolFilter.Blocklist) != 1 {
		t.Errorf("Blocklist length = %d, want 1", len(handler.MCPConfig.ToolFilter.Blocklist))
	}
}

func TestHTTPConfigAdvancedFields(t *testing.T) {
	bodyMapping := "{ query: @.search_query, limit: @.max_results }"
	responseMapping := "results[].{title: title, url: url}"
	urlTemplate := "/users/{{.user_id}}/orders"

	handler := HandlerDefinition{
		Name: "advanced-http",
		Type: HandlerTypeHTTP,
		HTTPConfig: &HTTPConfig{
			Endpoint:     "https://api.example.com",
			Method:       "GET",
			QueryParams:  []string{"search_query", "page"},
			HeaderParams: map[string]string{"X-Customer-ID": "{{.customer_id}}"},
			StaticQuery:  map[string]string{"api_key": "secret123"},
			StaticBody: &apiextensionsv1.JSON{
				Raw: []byte(`{"source":"omnia"}`),
			},
			BodyMapping:     &bodyMapping,
			ResponseMapping: &responseMapping,
			Redact:          []string{"api_key", "password"},
			URLTemplate:     &urlTemplate,
		},
	}

	cfg := handler.HTTPConfig

	if len(cfg.QueryParams) != 2 {
		t.Errorf("QueryParams length = %d, want 2", len(cfg.QueryParams))
	}

	if cfg.HeaderParams["X-Customer-ID"] != "{{.customer_id}}" {
		t.Errorf("HeaderParams[X-Customer-ID] = %v, want {{.customer_id}}", cfg.HeaderParams["X-Customer-ID"])
	}

	if cfg.StaticQuery["api_key"] != "secret123" {
		t.Errorf("StaticQuery[api_key] = %v, want secret123", cfg.StaticQuery["api_key"])
	}

	if cfg.StaticBody == nil {
		t.Fatal("StaticBody is nil")
	}

	if cfg.BodyMapping == nil || *cfg.BodyMapping != bodyMapping {
		t.Errorf("BodyMapping = %v, want %v", cfg.BodyMapping, bodyMapping)
	}

	if cfg.ResponseMapping == nil || *cfg.ResponseMapping != responseMapping {
		t.Errorf("ResponseMapping = %v, want %v", cfg.ResponseMapping, responseMapping)
	}

	if len(cfg.Redact) != 2 {
		t.Errorf("Redact length = %d, want 2", len(cfg.Redact))
	}

	if cfg.URLTemplate == nil || *cfg.URLTemplate != urlTemplate {
		t.Errorf("URLTemplate = %v, want %v", cfg.URLTemplate, urlTemplate)
	}
}

func TestOpenAPIConfigAuthFields(t *testing.T) {
	authType := "bearer"

	handler := HandlerDefinition{
		Name: "authed-openapi",
		Type: HandlerTypeOpenAPI,
		OpenAPIConfig: &OpenAPIConfig{
			SpecURL:  "https://api.example.com/openapi.json",
			Headers:  map[string]string{"X-Custom": "value"},
			AuthType: &authType,
			AuthSecretRef: &SecretKeySelector{
				Name: "api-secret",
				Key:  "token",
			},
		},
	}

	cfg := handler.OpenAPIConfig

	if cfg.Headers["X-Custom"] != "value" {
		t.Errorf("Headers[X-Custom] = %v, want value", cfg.Headers["X-Custom"])
	}

	if cfg.AuthType == nil || *cfg.AuthType != "bearer" {
		t.Errorf("AuthType = %v, want bearer", cfg.AuthType)
	}

	if cfg.AuthSecretRef == nil {
		t.Fatal("AuthSecretRef is nil")
	}

	if cfg.AuthSecretRef.Name != "api-secret" {
		t.Errorf("AuthSecretRef.Name = %v, want api-secret", cfg.AuthSecretRef.Name)
	}
}

func TestToolRegistryWithOpenAPIHandler(t *testing.T) {
	specURL := "http://api-server/openapi.json"
	baseURL := "http://api-server"

	registry := &ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testToolRegistryName,
			Namespace: testToolRegistryNamespace,
		},
		Spec: ToolRegistrySpec{
			Handlers: []HandlerDefinition{
				{
					Name: "openapi-handler",
					Type: HandlerTypeOpenAPI,
					OpenAPIConfig: &OpenAPIConfig{
						SpecURL: specURL,
						BaseURL: &baseURL,
					},
				},
			},
		},
	}

	handler := registry.Spec.Handlers[0]
	if handler.Type != HandlerTypeOpenAPI {
		t.Errorf("Handler.Type = %v, want %v", handler.Type, HandlerTypeOpenAPI)
	}

	if handler.OpenAPIConfig == nil {
		t.Fatal("Handler.OpenAPIConfig is nil")
	}

	if handler.OpenAPIConfig.SpecURL != specURL {
		t.Errorf("OpenAPIConfig.SpecURL = %v, want %v", handler.OpenAPIConfig.SpecURL, specURL)
	}

	if handler.OpenAPIConfig.BaseURL == nil || *handler.OpenAPIConfig.BaseURL != baseURL {
		t.Errorf("OpenAPIConfig.BaseURL = %v, want %v", handler.OpenAPIConfig.BaseURL, baseURL)
	}
}

func TestDiscoveredToolStructure(t *testing.T) {
	now := metav1.Now()
	inputSchema := apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)}

	tool := DiscoveredTool{
		Name:        testToolName,
		HandlerName: testHandlerName,
		Description: testToolDescription,
		InputSchema: &inputSchema,
		Endpoint:    testToolEndpoint,
		Status:      ToolStatusAvailable,
		LastChecked: &now,
	}

	if tool.Name != testToolName {
		t.Errorf("DiscoveredTool.Name = %v, want %v", tool.Name, testToolName)
	}

	if tool.HandlerName != testHandlerName {
		t.Errorf("DiscoveredTool.HandlerName = %v, want %v", tool.HandlerName, testHandlerName)
	}

	if tool.Status != ToolStatusAvailable {
		t.Errorf("DiscoveredTool.Status = %v, want %v", tool.Status, ToolStatusAvailable)
	}
}

func TestToolRegistryStatus(t *testing.T) {
	now := metav1.Now()

	status := ToolRegistryStatus{
		Phase:                ToolRegistryPhaseReady,
		DiscoveredToolsCount: 2,
		DiscoveredTools: []DiscoveredTool{
			{
				Name:        "tool1",
				HandlerName: "handler1",
				Status:      ToolStatusAvailable,
			},
			{
				Name:        "tool2",
				HandlerName: "handler2",
				Status:      ToolStatusAvailable,
			},
		},
		LastDiscoveryTime: &now,
	}

	if status.Phase != ToolRegistryPhaseReady {
		t.Errorf("Status.Phase = %v, want %v", status.Phase, ToolRegistryPhaseReady)
	}

	if status.DiscoveredToolsCount != 2 {
		t.Errorf("Status.DiscoveredToolsCount = %v, want 2", status.DiscoveredToolsCount)
	}

	if len(status.DiscoveredTools) != 2 {
		t.Errorf("len(Status.DiscoveredTools) = %v, want 2", len(status.DiscoveredTools))
	}
}

func TestHTTPRetryPolicy_JSONRoundTrip(t *testing.T) {
	backoffMult := "2.0"
	retryOnNetErr := true
	respectRetryAfter := false

	original := HTTPRetryPolicy{
		MaxAttempts:         3,
		InitialBackoff:      &metav1.Duration{Duration: 100 * time.Millisecond},
		BackoffMultiplier:   &backoffMult,
		MaxBackoff:          &metav1.Duration{Duration: 30 * time.Second},
		RetryOn:             []int32{502, 503, 504},
		RetryOnNetworkError: &retryOnNetErr,
		RespectRetryAfter:   &respectRetryAfter,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded HTTPRetryPolicy
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.MaxAttempts != original.MaxAttempts {
		t.Errorf("MaxAttempts = %v, want %v", decoded.MaxAttempts, original.MaxAttempts)
	}
	if decoded.InitialBackoff == nil || decoded.InitialBackoff.Duration != 100*time.Millisecond {
		t.Errorf("InitialBackoff = %v, want 100ms", decoded.InitialBackoff)
	}
	if decoded.BackoffMultiplier == nil || *decoded.BackoffMultiplier != "2.0" {
		t.Errorf("BackoffMultiplier = %v, want 2.0", decoded.BackoffMultiplier)
	}
	if decoded.MaxBackoff == nil || decoded.MaxBackoff.Duration != 30*time.Second {
		t.Errorf("MaxBackoff = %v, want 30s", decoded.MaxBackoff)
	}
	if len(decoded.RetryOn) != 3 || decoded.RetryOn[0] != 502 {
		t.Errorf("RetryOn = %v, want [502 503 504]", decoded.RetryOn)
	}
	if decoded.RetryOnNetworkError == nil || *decoded.RetryOnNetworkError != true {
		t.Errorf("RetryOnNetworkError = %v, want true", decoded.RetryOnNetworkError)
	}
	if decoded.RespectRetryAfter == nil || *decoded.RespectRetryAfter != false {
		t.Errorf("RespectRetryAfter = %v, want false", decoded.RespectRetryAfter)
	}
}
