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

package controller

import (
	"reflect"
	"strings"
	"testing"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	runtimetools "github.com/altairalabs/omnia/internal/runtime/tools"
)

const schemaTypeObject = "object"

func TestUnmarshalRawJSON(t *testing.T) {
	tests := []struct {
		name     string
		raw      []byte
		wantNil  bool
		wantType string
	}{
		{
			name:    "nil input",
			raw:     nil,
			wantNil: true,
		},
		{
			name:    "empty input",
			raw:     []byte{},
			wantNil: true,
		},
		{
			name:    "invalid JSON",
			raw:     []byte("not json"),
			wantNil: true,
		},
		{
			name:     "object schema",
			raw:      []byte(`{"type":"object","properties":{"expr":{"type":"string"}}}`),
			wantNil:  false,
			wantType: "map",
		},
		{
			name:     "string value",
			raw:      []byte(`"hello"`),
			wantNil:  false,
			wantType: "string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unmarshalRawJSON(tt.raw)
			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			switch tt.wantType {
			case "map":
				m, ok := result.(map[string]interface{})
				if !ok {
					t.Errorf("expected map[string]interface{}, got %T", result)
				}
				if m["type"] != schemaTypeObject {
					t.Errorf("expected type=object, got %v", m["type"])
				}
			case "string":
				if _, ok := result.(string); !ok {
					t.Errorf("expected string, got %T", result)
				}
			}
		})
	}
}

func TestBuildToolDefinition(t *testing.T) {
	t.Run("nil tool returns nil", func(t *testing.T) {
		if got := buildToolDefinition(nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("converts InputSchema from raw bytes to structured map", func(t *testing.T) {
		tool := &omniav1alpha1.ToolDefinition{
			Name:        "calculator",
			Description: "Evaluate math expressions",
			InputSchema: apiextensionsv1.JSON{
				Raw: []byte(`{"type":"object","properties":{"expr":{"type":"string","description":"Math expression"}},"required":["expr"]}`),
			},
		}

		def := buildToolDefinition(tool)
		if def == nil {
			t.Fatal("expected non-nil definition")
		}

		schema, ok := def.InputSchema.(map[string]interface{})
		if !ok {
			t.Fatalf("InputSchema should be map[string]interface{}, got %T", def.InputSchema)
		}

		if schema["type"] != schemaTypeObject {
			t.Errorf("expected type=object, got %v", schema["type"])
		}

		props, ok := schema["properties"].(map[string]interface{})
		if !ok {
			t.Fatalf("properties should be map, got %T", schema["properties"])
		}

		expr, ok := props["expr"].(map[string]interface{})
		if !ok {
			t.Fatalf("expr should be map, got %T", props["expr"])
		}
		if expr["type"] != "string" {
			t.Errorf("expected expr.type=string, got %v", expr["type"])
		}
	})

	t.Run("converts OutputSchema from raw bytes to structured map", func(t *testing.T) {
		tool := &omniav1alpha1.ToolDefinition{
			Name:        "calculator",
			Description: "Evaluate math expressions",
			InputSchema: apiextensionsv1.JSON{
				Raw: []byte(`{"type":"object"}`),
			},
			OutputSchema: &apiextensionsv1.JSON{
				Raw: []byte(`{"type":"object","properties":{"result":{"type":"number"}}}`),
			},
		}

		def := buildToolDefinition(tool)
		if def == nil {
			t.Fatal("expected non-nil definition")
		}

		schema, ok := def.OutputSchema.(map[string]interface{})
		if !ok {
			t.Fatalf("OutputSchema should be map[string]interface{}, got %T", def.OutputSchema)
		}
		if schema["type"] != schemaTypeObject {
			t.Errorf("expected type=object, got %v", schema["type"])
		}
	})

	t.Run("nil OutputSchema stays nil", func(t *testing.T) {
		tool := &omniav1alpha1.ToolDefinition{
			Name:        "calculator",
			Description: "Evaluate math expressions",
			InputSchema: apiextensionsv1.JSON{
				Raw: []byte(`{"type":"object"}`),
			},
		}

		def := buildToolDefinition(tool)
		if def.OutputSchema != nil {
			t.Errorf("expected nil OutputSchema, got %v", def.OutputSchema)
		}
	})
}

func TestBuildToolsConfig(t *testing.T) {
	t.Run("handler with tool produces structured schema in config", func(t *testing.T) {
		registry := &omniav1alpha1.ToolRegistry{}
		registry.Spec.Handlers = []omniav1alpha1.HandlerDefinition{
			{
				Name: "calc",
				Type: omniav1alpha1.HandlerTypeHTTP,
				HTTPConfig: &omniav1alpha1.HTTPConfig{
					Endpoint: "http://calc:8080/calculate",
				},
				Tool: &omniav1alpha1.ToolDefinition{
					Name:        "calculator",
					Description: "Evaluate math expressions",
					InputSchema: apiextensionsv1.JSON{
						Raw: []byte(`{"type":"object","properties":{"expr":{"type":"string"}},"required":["expr"]}`),
					},
				},
			},
		}
		registry.Status.DiscoveredTools = []omniav1alpha1.DiscoveredTool{
			{
				HandlerName: "calc",
				Status:      omniav1alpha1.ToolStatusAvailable,
				Endpoint:    "http://calc:8080/calculate",
			},
		}

		r := &AgentRuntimeReconciler{}
		config, err := r.buildToolsConfig(registry)
		if err != nil {
			t.Fatalf("buildToolsConfig: %v", err)
		}

		if len(config.Handlers) != 1 {
			t.Fatalf("expected 1 handler, got %d", len(config.Handlers))
		}

		handler := config.Handlers[0]
		if handler.Tool == nil {
			t.Fatal("expected tool definition")
		}

		// The key assertion: InputSchema must be a map, not []byte.
		// If it were []byte, YAML marshaling would base64-encode it,
		// and the runtime couldn't extract the schema.
		schema, ok := handler.Tool.InputSchema.(map[string]interface{})
		if !ok {
			t.Fatalf("InputSchema should be map[string]interface{}, got %T", handler.Tool.InputSchema)
		}
		if schema["type"] != schemaTypeObject {
			t.Errorf("expected schema type=object, got %v", schema["type"])
		}
	})
}

func TestBuildHTTPConfigAdvancedFields(t *testing.T) {
	bodyMapping := "{ query: @.q }"
	responseMapping := "results[]"
	urlTemplate := "/users/{{.id}}"

	h := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeHTTP,
		HTTPConfig: &omniav1alpha1.HTTPConfig{
			Endpoint:     "https://api.example.com",
			Method:       "GET",
			QueryParams:  []string{"q", "page"},
			HeaderParams: map[string]string{"X-ID": "{{.id}}"},
			StaticQuery:  map[string]string{"format": "json"},
			StaticBody: &apiextensionsv1.JSON{
				Raw: []byte(`{"source":"omnia"}`),
			},
			BodyMapping:     &bodyMapping,
			ResponseMapping: &responseMapping,
			Redact:          []string{"secret"},
			URLTemplate:     &urlTemplate,
		},
	}

	cfg, err := buildHTTPConfig(h, "https://api.example.com")
	if err != nil {
		t.Fatalf("buildHTTPConfig: %v", err)
	}

	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	if len(cfg.QueryParams) != 2 || cfg.QueryParams[0] != "q" {
		t.Errorf("QueryParams = %v, want [q page]", cfg.QueryParams)
	}

	if cfg.HeaderParams["X-ID"] != "{{.id}}" {
		t.Errorf("HeaderParams[X-ID] = %v, want {{.id}}", cfg.HeaderParams["X-ID"])
	}

	if cfg.StaticQuery["format"] != "json" {
		t.Errorf("StaticQuery[format] = %v, want json", cfg.StaticQuery["format"])
	}

	if cfg.StaticBody == nil {
		t.Fatal("StaticBody is nil")
	}

	body, ok := cfg.StaticBody.(map[string]interface{})
	if !ok {
		t.Fatalf("StaticBody should be map, got %T", cfg.StaticBody)
	}
	if body["source"] != "omnia" {
		t.Errorf("StaticBody.source = %v, want omnia", body["source"])
	}

	if cfg.BodyMapping != bodyMapping {
		t.Errorf("BodyMapping = %v, want %v", cfg.BodyMapping, bodyMapping)
	}

	if cfg.ResponseMapping != responseMapping {
		t.Errorf("ResponseMapping = %v, want %v", cfg.ResponseMapping, responseMapping)
	}

	if len(cfg.Redact) != 1 || cfg.Redact[0] != "secret" {
		t.Errorf("Redact = %v, want [secret]", cfg.Redact)
	}

	if cfg.URLTemplate != urlTemplate {
		t.Errorf("URLTemplate = %v, want %v", cfg.URLTemplate, urlTemplate)
	}
}

func TestBuildHTTPConfigNilOptionalFields(t *testing.T) {
	h := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeHTTP,
		HTTPConfig: &omniav1alpha1.HTTPConfig{
			Endpoint: "https://api.example.com",
			Method:   "POST",
		},
	}

	cfg, err := buildHTTPConfig(h, "https://api.example.com")
	if err != nil {
		t.Fatalf("buildHTTPConfig: %v", err)
	}

	if cfg.BodyMapping != "" {
		t.Errorf("BodyMapping should be empty, got %v", cfg.BodyMapping)
	}
	if cfg.ResponseMapping != "" {
		t.Errorf("ResponseMapping should be empty, got %v", cfg.ResponseMapping)
	}
	if cfg.URLTemplate != "" {
		t.Errorf("URLTemplate should be empty, got %v", cfg.URLTemplate)
	}
	if cfg.StaticBody != nil {
		t.Errorf("StaticBody should be nil, got %v", cfg.StaticBody)
	}
}

func TestBuildMCPConfigWithToolFilter(t *testing.T) {
	endpoint := "http://mcp:8080"

	h := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeMCP,
		MCPConfig: &omniav1alpha1.MCPConfig{
			Transport: omniav1alpha1.MCPTransportSSE,
			Endpoint:  &endpoint,
			ToolFilter: &omniav1alpha1.MCPToolFilter{
				Allowlist: []string{"read_file", "list_dir"},
				Blocklist: []string{"delete_file"},
			},
		},
	}

	cfg, err := buildMCPConfig(h)
	if err != nil {
		t.Fatalf("buildMCPConfig: %v", err)
	}

	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	if cfg.ToolFilter == nil {
		t.Fatal("ToolFilter is nil")
	}

	if len(cfg.ToolFilter.Allowlist) != 2 {
		t.Errorf("Allowlist length = %d, want 2", len(cfg.ToolFilter.Allowlist))
	}

	if cfg.ToolFilter.Allowlist[0] != "read_file" {
		t.Errorf("Allowlist[0] = %v, want read_file", cfg.ToolFilter.Allowlist[0])
	}

	if len(cfg.ToolFilter.Blocklist) != 1 || cfg.ToolFilter.Blocklist[0] != "delete_file" {
		t.Errorf("Blocklist = %v, want [delete_file]", cfg.ToolFilter.Blocklist)
	}
}

func TestBuildMCPConfigWithoutToolFilter(t *testing.T) {
	endpoint := "http://mcp:8080"

	h := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeMCP,
		MCPConfig: &omniav1alpha1.MCPConfig{
			Transport: omniav1alpha1.MCPTransportSSE,
			Endpoint:  &endpoint,
		},
	}

	cfg, err := buildMCPConfig(h)
	if err != nil {
		t.Fatalf("buildMCPConfig: %v", err)
	}
	if cfg.ToolFilter != nil {
		t.Errorf("ToolFilter should be nil, got %v", cfg.ToolFilter)
	}
}

func TestBuildMCPConfigStreamableHTTP(t *testing.T) {
	endpoint := "http://mcp:8080/mcp"

	h := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeMCP,
		MCPConfig: &omniav1alpha1.MCPConfig{
			Transport: omniav1alpha1.MCPTransportStreamableHTTP,
			Endpoint:  &endpoint,
		},
	}

	cfg, err := buildMCPConfig(h)
	if err != nil {
		t.Fatalf("buildMCPConfig: %v", err)
	}
	if cfg.Transport != "streamable-http" {
		t.Errorf("Transport = %v, want streamable-http", cfg.Transport)
	}
}

func TestBuildOpenAPIConfigWithHeaders(t *testing.T) {
	baseURL := "https://api.example.com"

	h := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeOpenAPI,
		OpenAPIConfig: &omniav1alpha1.OpenAPIConfig{
			SpecURL: "https://api.example.com/openapi.json",
			BaseURL: &baseURL,
			Headers: map[string]string{
				"X-API-Key": "key123",
				"Accept":    "application/json",
			},
		},
	}

	cfg, err := buildOpenAPIConfig(h)
	if err != nil {
		t.Fatalf("buildOpenAPIConfig: %v", err)
	}

	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	if len(cfg.Headers) != 2 {
		t.Errorf("Headers length = %d, want 2", len(cfg.Headers))
	}

	if cfg.Headers["X-API-Key"] != "key123" {
		t.Errorf("Headers[X-API-Key] = %v, want key123", cfg.Headers["X-API-Key"])
	}
}

func TestFindEndpoint(t *testing.T) {
	registry := &omniav1alpha1.ToolRegistry{}
	registry.Status.DiscoveredTools = []omniav1alpha1.DiscoveredTool{
		{
			HandlerName: "handler-a",
			Status:      omniav1alpha1.ToolStatusAvailable,
			Endpoint:    "http://a:8080",
		},
		{
			HandlerName: "handler-b",
			Status:      omniav1alpha1.ToolStatusUnavailable,
			Endpoint:    "http://b:8080",
		},
	}

	t.Run("finds available handler endpoint", func(t *testing.T) {
		ep := findEndpoint(registry, "handler-a")
		if ep != "http://a:8080" {
			t.Errorf("endpoint = %v, want http://a:8080", ep)
		}
	})

	t.Run("returns empty for unavailable handler", func(t *testing.T) {
		ep := findEndpoint(registry, "handler-b")
		if ep != "" {
			t.Errorf("endpoint = %v, want empty", ep)
		}
	})

	t.Run("returns empty for unknown handler", func(t *testing.T) {
		ep := findEndpoint(registry, "handler-c")
		if ep != "" {
			t.Errorf("endpoint = %v, want empty", ep)
		}
	})
}

func TestBuildHandlerEntry(t *testing.T) {
	timeout := metav1.Duration{Duration: 10 * time.Second}

	t.Run("HTTP handler", func(t *testing.T) {
		h := &omniav1alpha1.HandlerDefinition{
			Name: "my-http",
			Type: omniav1alpha1.HandlerTypeHTTP,
			HTTPConfig: &omniav1alpha1.HTTPConfig{
				Endpoint: "http://svc:8080",
				Method:   "POST",
			},
			Tool: &omniav1alpha1.ToolDefinition{
				Name:        "my_tool",
				Description: "A tool",
				InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
			},
			Timeout: &timeout,
		}

		entry, err := buildHandlerEntry(h, "http://svc:8080")
		if err != nil {
			t.Fatalf("buildHandlerEntry: %v", err)
		}
		if entry.Name != "my-http" {
			t.Errorf("Name = %v, want my-http", entry.Name)
		}
		if entry.Timeout != "10s" {
			t.Errorf("Timeout = %v, want 10s", entry.Timeout)
		}
		if entry.HTTPConfig == nil {
			t.Error("HTTPConfig is nil")
		}
		if entry.Tool == nil {
			t.Error("Tool is nil")
		}
	})

	t.Run("gRPC handler", func(t *testing.T) {
		h := &omniav1alpha1.HandlerDefinition{
			Name: "my-grpc",
			Type: omniav1alpha1.HandlerTypeGRPC,
			GRPCConfig: &omniav1alpha1.GRPCConfig{
				Endpoint: "grpc-svc:50051",
				TLS:      true,
			},
			Tool: &omniav1alpha1.ToolDefinition{
				Name:        "my_tool",
				Description: "A tool",
				InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
			},
		}

		entry, err := buildHandlerEntry(h, "grpc-svc:50051")
		if err != nil {
			t.Fatalf("buildHandlerEntry: %v", err)
		}
		if entry.GRPCConfig == nil {
			t.Error("GRPCConfig is nil")
		}
		if entry.Tool == nil {
			t.Error("Tool is nil")
		}
	})

	t.Run("MCP handler", func(t *testing.T) {
		endpoint := "http://mcp:8080"
		h := &omniav1alpha1.HandlerDefinition{
			Name: "my-mcp",
			Type: omniav1alpha1.HandlerTypeMCP,
			MCPConfig: &omniav1alpha1.MCPConfig{
				Transport: omniav1alpha1.MCPTransportSSE,
				Endpoint:  &endpoint,
			},
		}

		entry, err := buildHandlerEntry(h, "http://mcp:8080")
		if err != nil {
			t.Fatalf("buildHandlerEntry: %v", err)
		}
		if entry.MCPConfig == nil {
			t.Error("MCPConfig is nil")
		}
		if entry.Tool != nil {
			t.Error("MCP handler should not have Tool")
		}
	})

	t.Run("OpenAPI handler", func(t *testing.T) {
		h := &omniav1alpha1.HandlerDefinition{
			Name: "my-openapi",
			Type: omniav1alpha1.HandlerTypeOpenAPI,
			OpenAPIConfig: &omniav1alpha1.OpenAPIConfig{
				SpecURL: "http://api/openapi.json",
			},
		}

		entry, err := buildHandlerEntry(h, "http://api")
		if err != nil {
			t.Fatalf("buildHandlerEntry: %v", err)
		}
		if entry.OpenAPIConfig == nil {
			t.Error("OpenAPIConfig is nil")
		}
	})
}

func TestBuildHTTPConfigNil(t *testing.T) {
	h := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeHTTP,
	}
	got, err := buildHTTPConfig(h, "ep")
	if err != nil {
		t.Fatalf("buildHTTPConfig: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestBuildGRPCConfigNil(t *testing.T) {
	h := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeGRPC,
	}
	got, err := buildGRPCConfig(h, "ep")
	if err != nil {
		t.Fatalf("buildGRPCConfig: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestBuildMCPConfigNil(t *testing.T) {
	h := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeMCP,
	}
	got, err := buildMCPConfig(h)
	if err != nil {
		t.Fatalf("buildMCPConfig: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestBuildOpenAPIConfigNil(t *testing.T) {
	h := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeOpenAPI,
	}
	got, err := buildOpenAPIConfig(h)
	if err != nil {
		t.Fatalf("buildOpenAPIConfig: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestBuildGRPCConfigAllFields(t *testing.T) {
	certPath := "/certs/tls.crt"
	keyPath := "/certs/tls.key"
	caPath := "/certs/ca.crt"

	h := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeGRPC,
		GRPCConfig: &omniav1alpha1.GRPCConfig{
			Endpoint:              "grpc:50051",
			TLS:                   true,
			TLSCertPath:           &certPath,
			TLSKeyPath:            &keyPath,
			TLSCAPath:             &caPath,
			TLSInsecureSkipVerify: true,
		},
	}

	cfg, err := buildGRPCConfig(h, "grpc:50051")
	if err != nil {
		t.Fatalf("buildGRPCConfig: %v", err)
	}
	if !cfg.TLS {
		t.Error("TLS should be true")
	}
	if cfg.TLSCertPath != certPath {
		t.Errorf("TLSCertPath = %v, want %v", cfg.TLSCertPath, certPath)
	}
	if cfg.TLSKeyPath != keyPath {
		t.Errorf("TLSKeyPath = %v, want %v", cfg.TLSKeyPath, keyPath)
	}
	if cfg.TLSCAPath != caPath {
		t.Errorf("TLSCAPath = %v, want %v", cfg.TLSCAPath, caPath)
	}
	if !cfg.TLSInsecureSkipVerify {
		t.Error("TLSInsecureSkipVerify should be true")
	}
}

func TestBuildToolsConfigSkipsUnavailableHandlers(t *testing.T) {
	registry := &omniav1alpha1.ToolRegistry{}
	registry.Spec.Handlers = []omniav1alpha1.HandlerDefinition{
		{
			Name: "available",
			Type: omniav1alpha1.HandlerTypeHTTP,
			HTTPConfig: &omniav1alpha1.HTTPConfig{
				Endpoint: "http://a:8080",
			},
		},
		{
			Name: "unavailable",
			Type: omniav1alpha1.HandlerTypeHTTP,
			HTTPConfig: &omniav1alpha1.HTTPConfig{
				Endpoint: "http://b:8080",
			},
		},
	}
	registry.Status.DiscoveredTools = []omniav1alpha1.DiscoveredTool{
		{
			HandlerName: "available",
			Status:      omniav1alpha1.ToolStatusAvailable,
			Endpoint:    "http://a:8080",
		},
	}

	r := &AgentRuntimeReconciler{}
	config, err := r.buildToolsConfig(registry)
	if err != nil {
		t.Fatalf("buildToolsConfig: %v", err)
	}

	if len(config.Handlers) != 1 {
		t.Fatalf("expected 1 handler, got %d", len(config.Handlers))
	}
	if config.Handlers[0].Name != "available" {
		t.Errorf("expected handler 'available', got %v", config.Handlers[0].Name)
	}
}

func TestBuildHTTPRetryPolicy(t *testing.T) {
	valid := "2.0"
	lowMult := "0.5"
	badMult := "abc"
	retryOnNetErr := true
	respectRA := false

	tests := []struct {
		name    string
		input   *omniav1alpha1.HTTPRetryPolicy
		want    *runtimetools.RuntimeHTTPRetryPolicy
		wantErr string
	}{
		{
			name:  "nil policy returns nil",
			input: nil,
			want:  nil,
		},
		{
			name: "minimum valid policy gets defaults",
			input: &omniav1alpha1.HTTPRetryPolicy{
				MaxAttempts: 3,
			},
			want: &runtimetools.RuntimeHTTPRetryPolicy{
				MaxAttempts:         3,
				InitialBackoff:      runtimetools.Duration(100 * time.Millisecond),
				BackoffMultiplier:   2.0,
				MaxBackoff:          runtimetools.Duration(30 * time.Second),
				RetryOn:             []int32{408, 429, 500, 502, 503, 504},
				RetryOnNetworkError: true,
				RespectRetryAfter:   true,
			},
		},
		{
			name: "fully populated policy",
			input: &omniav1alpha1.HTTPRetryPolicy{
				MaxAttempts:         5,
				InitialBackoff:      &metav1.Duration{Duration: 200 * time.Millisecond},
				BackoffMultiplier:   &valid,
				MaxBackoff:          &metav1.Duration{Duration: 60 * time.Second},
				RetryOn:             []int32{502, 503},
				RetryOnNetworkError: &retryOnNetErr,
				RespectRetryAfter:   &respectRA,
			},
			want: &runtimetools.RuntimeHTTPRetryPolicy{
				MaxAttempts:         5,
				InitialBackoff:      runtimetools.Duration(200 * time.Millisecond),
				BackoffMultiplier:   2.0,
				MaxBackoff:          runtimetools.Duration(60 * time.Second),
				RetryOn:             []int32{502, 503},
				RetryOnNetworkError: true,
				RespectRetryAfter:   false,
			},
		},
		{
			name: "empty RetryOn slice stays empty (not defaulted)",
			input: &omniav1alpha1.HTTPRetryPolicy{
				MaxAttempts: 2,
				RetryOn:     []int32{},
			},
			want: &runtimetools.RuntimeHTTPRetryPolicy{
				MaxAttempts:         2,
				InitialBackoff:      runtimetools.Duration(100 * time.Millisecond),
				BackoffMultiplier:   2.0,
				MaxBackoff:          runtimetools.Duration(30 * time.Second),
				RetryOn:             []int32{},
				RetryOnNetworkError: true,
				RespectRetryAfter:   true,
			},
		},
		{
			name: "invalid BackoffMultiplier string errors",
			input: &omniav1alpha1.HTTPRetryPolicy{
				MaxAttempts:       3,
				BackoffMultiplier: &badMult,
			},
			wantErr: "invalid backoffMultiplier",
		},
		{
			name: "BackoffMultiplier below 1.0 errors",
			input: &omniav1alpha1.HTTPRetryPolicy{
				MaxAttempts:       3,
				BackoffMultiplier: &lowMult,
			},
			wantErr: "must be >= 1.0",
		},
		{
			name: "MaxBackoff less than InitialBackoff errors",
			input: &omniav1alpha1.HTTPRetryPolicy{
				MaxAttempts:    3,
				InitialBackoff: &metav1.Duration{Duration: 10 * time.Second},
				MaxBackoff:     &metav1.Duration{Duration: 1 * time.Second},
			},
			wantErr: "maxBackoff",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildHTTPRetryPolicy(tc.input)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestBuildGRPCRetryPolicy(t *testing.T) {
	valid := "1.5"
	badMult := "x"

	tests := []struct {
		name    string
		input   *omniav1alpha1.GRPCRetryPolicy
		want    *runtimetools.RuntimeGRPCRetryPolicy
		wantErr string
	}{
		{
			name:  "nil policy returns nil",
			input: nil,
			want:  nil,
		},
		{
			name: "minimum valid policy gets defaults",
			input: &omniav1alpha1.GRPCRetryPolicy{
				MaxAttempts: 3,
			},
			want: &runtimetools.RuntimeGRPCRetryPolicy{
				MaxAttempts:          3,
				InitialBackoff:       runtimetools.Duration(100 * time.Millisecond),
				BackoffMultiplier:    2.0,
				MaxBackoff:           runtimetools.Duration(30 * time.Second),
				RetryableStatusCodes: []string{"UNAVAILABLE", "DEADLINE_EXCEEDED", "RESOURCE_EXHAUSTED"},
			},
		},
		{
			name: "fully populated policy",
			input: &omniav1alpha1.GRPCRetryPolicy{
				MaxAttempts:          5,
				InitialBackoff:       &metav1.Duration{Duration: 50 * time.Millisecond},
				BackoffMultiplier:    &valid,
				MaxBackoff:           &metav1.Duration{Duration: 10 * time.Second},
				RetryableStatusCodes: []string{"UNAVAILABLE"},
			},
			want: &runtimetools.RuntimeGRPCRetryPolicy{
				MaxAttempts:          5,
				InitialBackoff:       runtimetools.Duration(50 * time.Millisecond),
				BackoffMultiplier:    1.5,
				MaxBackoff:           runtimetools.Duration(10 * time.Second),
				RetryableStatusCodes: []string{"UNAVAILABLE"},
			},
		},
		{
			name: "unknown gRPC status code errors",
			input: &omniav1alpha1.GRPCRetryPolicy{
				MaxAttempts:          3,
				RetryableStatusCodes: []string{"UNAVAILABLE", "FOOBAR"},
			},
			wantErr: "unknown gRPC status code \"FOOBAR\"",
		},
		{
			name: "invalid BackoffMultiplier errors",
			input: &omniav1alpha1.GRPCRetryPolicy{
				MaxAttempts:       3,
				BackoffMultiplier: &badMult,
			},
			wantErr: "invalid backoffMultiplier",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildGRPCRetryPolicy(tc.input)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestBuildMCPRetryPolicy(t *testing.T) {
	valid := "2.0"

	tests := []struct {
		name    string
		input   *omniav1alpha1.MCPRetryPolicy
		want    *runtimetools.RuntimeMCPRetryPolicy
		wantErr string
	}{
		{
			name:  "nil policy returns nil",
			input: nil,
			want:  nil,
		},
		{
			name: "minimum valid policy gets defaults",
			input: &omniav1alpha1.MCPRetryPolicy{
				MaxAttempts: 2,
			},
			want: &runtimetools.RuntimeMCPRetryPolicy{
				MaxAttempts:       2,
				InitialBackoff:    runtimetools.Duration(100 * time.Millisecond),
				BackoffMultiplier: 2.0,
				MaxBackoff:        runtimetools.Duration(30 * time.Second),
			},
		},
		{
			name: "fully populated policy",
			input: &omniav1alpha1.MCPRetryPolicy{
				MaxAttempts:       4,
				InitialBackoff:    &metav1.Duration{Duration: 500 * time.Millisecond},
				BackoffMultiplier: &valid,
				MaxBackoff:        &metav1.Duration{Duration: 60 * time.Second},
			},
			want: &runtimetools.RuntimeMCPRetryPolicy{
				MaxAttempts:       4,
				InitialBackoff:    runtimetools.Duration(500 * time.Millisecond),
				BackoffMultiplier: 2.0,
				MaxBackoff:        runtimetools.Duration(60 * time.Second),
			},
		},
		{
			name: "MaxBackoff less than InitialBackoff errors",
			input: &omniav1alpha1.MCPRetryPolicy{
				MaxAttempts:    2,
				InitialBackoff: &metav1.Duration{Duration: 10 * time.Second},
				MaxBackoff:     &metav1.Duration{Duration: 1 * time.Second},
			},
			wantErr: "maxBackoff",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildMCPRetryPolicy(tc.input)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestBuildHandlerEntry_HTTPWithRetryPolicy(t *testing.T) {
	backoffMult := "2.0"
	h := &omniav1alpha1.HandlerDefinition{
		Name: "retried-http",
		Type: omniav1alpha1.HandlerTypeHTTP,
		HTTPConfig: &omniav1alpha1.HTTPConfig{
			Endpoint: "http://svc:8080",
			Method:   "GET",
			RetryPolicy: &omniav1alpha1.HTTPRetryPolicy{
				MaxAttempts:       3,
				BackoffMultiplier: &backoffMult,
			},
		},
		Tool: &omniav1alpha1.ToolDefinition{
			Name:        "my_tool",
			Description: "A tool",
			InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
		},
	}

	entry, err := buildHandlerEntry(h, "http://svc:8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.HTTPConfig == nil || entry.HTTPConfig.RetryPolicy == nil {
		t.Fatal("retry policy is nil")
	}
	if entry.HTTPConfig.RetryPolicy.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", entry.HTTPConfig.RetryPolicy.MaxAttempts)
	}
	if entry.HTTPConfig.RetryPolicy.BackoffMultiplier != 2.0 {
		t.Errorf("BackoffMultiplier = %v, want 2.0", entry.HTTPConfig.RetryPolicy.BackoffMultiplier)
	}
}

func TestBuildHandlerEntry_InvalidRetryPolicyError(t *testing.T) {
	badMult := "not-a-number"
	h := &omniav1alpha1.HandlerDefinition{
		Name: "bad-http",
		Type: omniav1alpha1.HandlerTypeHTTP,
		HTTPConfig: &omniav1alpha1.HTTPConfig{
			Endpoint: "http://svc:8080",
			Method:   "GET",
			RetryPolicy: &omniav1alpha1.HTTPRetryPolicy{
				MaxAttempts:       3,
				BackoffMultiplier: &badMult,
			},
		},
		Tool: &omniav1alpha1.ToolDefinition{
			Name:        "my_tool",
			Description: "A tool",
			InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
		},
	}

	_, err := buildHandlerEntry(h, "http://svc:8080")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "bad-http") {
		t.Errorf("error %v should include handler name", err)
	}
	if !strings.Contains(err.Error(), "invalid backoffMultiplier") {
		t.Errorf("error %v should include builder error", err)
	}
}

func TestBuildHandlerEntry_GRPCWithRetryPolicy(t *testing.T) {
	h := &omniav1alpha1.HandlerDefinition{
		Name: "retried-grpc",
		Type: omniav1alpha1.HandlerTypeGRPC,
		GRPCConfig: &omniav1alpha1.GRPCConfig{
			Endpoint: "grpc:50051",
			RetryPolicy: &omniav1alpha1.GRPCRetryPolicy{
				MaxAttempts:          3,
				RetryableStatusCodes: []string{"UNAVAILABLE"},
			},
		},
		Tool: &omniav1alpha1.ToolDefinition{
			Name:        "my_tool",
			Description: "A tool",
			InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
		},
	}

	entry, err := buildHandlerEntry(h, "grpc:50051")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.GRPCConfig == nil || entry.GRPCConfig.RetryPolicy == nil {
		t.Fatal("retry policy is nil")
	}
	if len(entry.GRPCConfig.RetryPolicy.RetryableStatusCodes) != 1 {
		t.Errorf("RetryableStatusCodes = %v, want [UNAVAILABLE]", entry.GRPCConfig.RetryPolicy.RetryableStatusCodes)
	}
}

func TestBuildHandlerEntry_MCPWithRetryPolicy(t *testing.T) {
	endpoint := "http://mcp:8080"
	h := &omniav1alpha1.HandlerDefinition{
		Name: "retried-mcp",
		Type: omniav1alpha1.HandlerTypeMCP,
		MCPConfig: &omniav1alpha1.MCPConfig{
			Transport: omniav1alpha1.MCPTransportSSE,
			Endpoint:  &endpoint,
			RetryPolicy: &omniav1alpha1.MCPRetryPolicy{
				MaxAttempts: 2,
			},
		},
	}

	entry, err := buildHandlerEntry(h, "http://mcp:8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.MCPConfig == nil || entry.MCPConfig.RetryPolicy == nil {
		t.Fatal("retry policy is nil")
	}
	if entry.MCPConfig.RetryPolicy.MaxAttempts != 2 {
		t.Errorf("MaxAttempts = %d, want 2", entry.MCPConfig.RetryPolicy.MaxAttempts)
	}
}

func TestBuildHandlerEntry_OpenAPIWithRetryPolicy(t *testing.T) {
	h := &omniav1alpha1.HandlerDefinition{
		Name: "retried-openapi",
		Type: omniav1alpha1.HandlerTypeOpenAPI,
		OpenAPIConfig: &omniav1alpha1.OpenAPIConfig{
			SpecURL: "http://api/openapi.json",
			RetryPolicy: &omniav1alpha1.HTTPRetryPolicy{
				MaxAttempts: 4,
			},
		},
	}

	entry, err := buildHandlerEntry(h, "http://api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.OpenAPIConfig == nil || entry.OpenAPIConfig.RetryPolicy == nil {
		t.Fatal("retry policy is nil")
	}
	if entry.OpenAPIConfig.RetryPolicy.MaxAttempts != 4 {
		t.Errorf("MaxAttempts = %d, want 4", entry.OpenAPIConfig.RetryPolicy.MaxAttempts)
	}
}
