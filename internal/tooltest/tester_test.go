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

package tooltest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// testScheme builds a scheme with core + omnia CRDs registered.
func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = omniav1alpha1.AddToScheme(s)
	return s
}

func TestApplyAuthHeader(t *testing.T) {
	tests := []struct {
		name      string
		authType  string
		token     string
		wantKey   string
		wantValue string
	}{
		{
			name:      "bearer auth",
			authType:  "bearer",
			token:     "my-token",
			wantKey:   "Authorization",
			wantValue: "Bearer my-token",
		},
		{
			name:      "basic auth",
			authType:  "basic",
			token:     "dXNlcjpwYXNz",
			wantKey:   "Authorization",
			wantValue: "Basic dXNlcjpwYXNz",
		},
		{
			name:      "api-key auth",
			authType:  "api-key",
			token:     "key123",
			wantKey:   "X-API-Key",
			wantValue: "key123",
		},
		{
			name:      "unknown auth type",
			authType:  "custom",
			token:     "val",
			wantKey:   "",
			wantValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := make(map[string]string)
			applyAuthHeader(headers, tt.authType, tt.token)

			if tt.wantKey == "" {
				if len(headers) != 0 {
					t.Errorf("expected no headers, got %v", headers)
				}
				return
			}

			got := headers[tt.wantKey]
			if got != tt.wantValue {
				t.Errorf("headers[%q] = %q, want %q", tt.wantKey, got, tt.wantValue)
			}
		})
	}
}

func TestResolveTimeout(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	tester := NewTester(nil, log)

	tests := []struct {
		name    string
		timeout *string
		want    time.Duration
	}{
		{
			name:    "nil timeout uses max",
			timeout: nil,
			want:    maxTestTimeout,
		},
		{
			name:    "valid short timeout",
			timeout: strPtr("10s"),
			want:    10 * time.Second,
		},
		{
			name:    "timeout exceeding max is capped",
			timeout: strPtr("5m"),
			want:    maxTestTimeout,
		},
		{
			name:    "invalid timeout uses max",
			timeout: strPtr("invalid"),
			want:    maxTestTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &omniav1alpha1.HandlerDefinition{
				Timeout: tt.timeout,
			}
			got := tester.resolveTimeout(handler)
			if got != tt.want {
				t.Errorf("resolveTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindHandler(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	tester := NewTester(nil, log)

	registry := &omniav1alpha1.ToolRegistry{
		Spec: omniav1alpha1.ToolRegistrySpec{
			Handlers: []omniav1alpha1.HandlerDefinition{
				{Name: "handler-a", Type: omniav1alpha1.HandlerTypeHTTP},
				{Name: "handler-b", Type: omniav1alpha1.HandlerTypeMCP},
			},
		},
	}

	t.Run("found", func(t *testing.T) {
		h, err := tester.findHandler(registry, "handler-b")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if h.Name != "handler-b" {
			t.Errorf("handler.Name = %q, want %q", h.Name, "handler-b")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := tester.findHandler(registry, "nonexistent")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestBuildHandlerConfigHTTP(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	tester := NewTester(nil, log)

	bodyMapping := "{ query: @.q }"
	responseMapping := "results[]"
	urlTemplate := "/api/{{.id}}"

	handler := &omniav1alpha1.HandlerDefinition{
		Name: "http-handler",
		Type: omniav1alpha1.HandlerTypeHTTP,
		HTTPConfig: &omniav1alpha1.HTTPConfig{
			Endpoint:        "https://api.example.com",
			Method:          "POST",
			Headers:         map[string]string{"X-Custom": "val"},
			ContentType:     "application/json",
			QueryParams:     []string{"page"},
			HeaderParams:    map[string]string{"X-ID": "{{.id}}"},
			StaticQuery:     map[string]string{"key": "val"},
			BodyMapping:     &bodyMapping,
			ResponseMapping: &responseMapping,
			Redact:          []string{"secret"},
			URLTemplate:     &urlTemplate,
			StaticBody: &apiextensionsv1.JSON{
				Raw: []byte(`{"source":"test"}`),
			},
		},
		Tool: &omniav1alpha1.ToolDefinition{
			Name:        "search",
			Description: "Search things",
			InputSchema: apiextensionsv1.JSON{
				Raw: []byte(`{"type":"object"}`),
			},
		},
		Timeout: strPtr("15s"),
		Retries: int32Ptr(2),
	}

	entry := tester.buildHandlerConfig(handler)

	if entry.Name != "http-handler" {
		t.Errorf("Name = %q, want %q", entry.Name, "http-handler")
	}
	if entry.Type != "http" {
		t.Errorf("Type = %q, want %q", entry.Type, "http")
	}
	if entry.Timeout != "15s" {
		t.Errorf("Timeout = %q, want %q", entry.Timeout, "15s")
	}
	if entry.Retries != 2 {
		t.Errorf("Retries = %d, want 2", entry.Retries)
	}
	if entry.HTTPConfig == nil {
		t.Fatal("HTTPConfig is nil")
	}
	if entry.HTTPConfig.Endpoint != "https://api.example.com" {
		t.Errorf("HTTPConfig.Endpoint = %q, want %q", entry.HTTPConfig.Endpoint, "https://api.example.com")
	}
	if entry.HTTPConfig.BodyMapping != bodyMapping {
		t.Errorf("HTTPConfig.BodyMapping = %q, want %q", entry.HTTPConfig.BodyMapping, bodyMapping)
	}
	if entry.HTTPConfig.ResponseMapping != responseMapping {
		t.Errorf("HTTPConfig.ResponseMapping = %q, want %q", entry.HTTPConfig.ResponseMapping, responseMapping)
	}
	if entry.HTTPConfig.URLTemplate != urlTemplate {
		t.Errorf("HTTPConfig.URLTemplate = %q, want %q", entry.HTTPConfig.URLTemplate, urlTemplate)
	}
	if entry.Tool == nil {
		t.Fatal("Tool is nil")
	}
	if entry.Tool.Name != "search" {
		t.Errorf("Tool.Name = %q, want %q", entry.Tool.Name, "search")
	}
}

func TestBuildHandlerConfigMCP(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	tester := NewTester(nil, log)

	endpoint := "http://mcp:8080"
	command := "node"
	workDir := "/app"

	handler := &omniav1alpha1.HandlerDefinition{
		Name: "mcp-handler",
		Type: omniav1alpha1.HandlerTypeMCP,
		MCPConfig: &omniav1alpha1.MCPConfig{
			Transport: omniav1alpha1.MCPTransportSSE,
			Endpoint:  &endpoint,
			Command:   &command,
			Args:      []string{"server.js"},
			WorkDir:   &workDir,
			Env:       map[string]string{"NODE_ENV": "prod"},
			ToolFilter: &omniav1alpha1.MCPToolFilter{
				Allowlist: []string{"read_file"},
				Blocklist: []string{"delete_file"},
			},
		},
	}

	entry := tester.buildHandlerConfig(handler)

	if entry.MCPConfig == nil {
		t.Fatal("MCPConfig is nil")
	}
	if entry.MCPConfig.Transport != "sse" {
		t.Errorf("Transport = %q, want %q", entry.MCPConfig.Transport, "sse")
	}
	if entry.MCPConfig.Endpoint != endpoint {
		t.Errorf("Endpoint = %q, want %q", entry.MCPConfig.Endpoint, endpoint)
	}
	if entry.MCPConfig.Command != command {
		t.Errorf("Command = %q, want %q", entry.MCPConfig.Command, command)
	}
	if entry.MCPConfig.WorkDir != workDir {
		t.Errorf("WorkDir = %q, want %q", entry.MCPConfig.WorkDir, workDir)
	}
	if entry.MCPConfig.ToolFilter == nil {
		t.Fatal("ToolFilter is nil")
	}
	if len(entry.MCPConfig.ToolFilter.Allowlist) != 1 {
		t.Errorf("Allowlist len = %d, want 1", len(entry.MCPConfig.ToolFilter.Allowlist))
	}
}

func TestBuildHandlerConfigGRPC(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	tester := NewTester(nil, log)

	certPath := "/certs/tls.crt"
	keyPath := "/certs/tls.key"
	caPath := "/certs/ca.crt"

	handler := &omniav1alpha1.HandlerDefinition{
		Name: "grpc-handler",
		Type: omniav1alpha1.HandlerTypeGRPC,
		GRPCConfig: &omniav1alpha1.GRPCConfig{
			Endpoint:              "grpc-server:50051",
			TLS:                   true,
			TLSCertPath:           &certPath,
			TLSKeyPath:            &keyPath,
			TLSCAPath:             &caPath,
			TLSInsecureSkipVerify: true,
		},
		Tool: &omniav1alpha1.ToolDefinition{
			Name:        "grpc-tool",
			Description: "A gRPC tool",
			InputSchema: apiextensionsv1.JSON{
				Raw: []byte(`{"type":"object"}`),
			},
		},
	}

	entry := tester.buildHandlerConfig(handler)

	if entry.GRPCConfig == nil {
		t.Fatal("GRPCConfig is nil")
	}
	if entry.GRPCConfig.Endpoint != "grpc-server:50051" {
		t.Errorf("Endpoint = %q, want %q", entry.GRPCConfig.Endpoint, "grpc-server:50051")
	}
	if !entry.GRPCConfig.TLS {
		t.Error("TLS = false, want true")
	}
	if entry.GRPCConfig.TLSCertPath != certPath {
		t.Errorf("TLSCertPath = %q, want %q", entry.GRPCConfig.TLSCertPath, certPath)
	}
	if entry.Tool == nil {
		t.Fatal("Tool is nil")
	}
}

func TestBuildHandlerConfigOpenAPI(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	tester := NewTester(nil, log)

	baseURL := "https://api.example.com"
	authType := "bearer"

	handler := &omniav1alpha1.HandlerDefinition{
		Name: "openapi-handler",
		Type: omniav1alpha1.HandlerTypeOpenAPI,
		OpenAPIConfig: &omniav1alpha1.OpenAPIConfig{
			SpecURL:         "https://api.example.com/openapi.json",
			BaseURL:         &baseURL,
			OperationFilter: []string{"getUsers", "createUser"},
			Headers:         map[string]string{"X-Custom": "val"},
			AuthType:        &authType,
		},
	}

	entry := tester.buildHandlerConfig(handler)

	if entry.OpenAPIConfig == nil {
		t.Fatal("OpenAPIConfig is nil")
	}
	if entry.OpenAPIConfig.SpecURL != "https://api.example.com/openapi.json" {
		t.Errorf("SpecURL = %q", entry.OpenAPIConfig.SpecURL)
	}
	if entry.OpenAPIConfig.BaseURL != baseURL {
		t.Errorf("BaseURL = %q, want %q", entry.OpenAPIConfig.BaseURL, baseURL)
	}
	if entry.OpenAPIConfig.AuthType != "bearer" {
		t.Errorf("AuthType = %q, want %q", entry.OpenAPIConfig.AuthType, "bearer")
	}
	if len(entry.OpenAPIConfig.OperationFilter) != 2 {
		t.Errorf("OperationFilter len = %d, want 2", len(entry.OpenAPIConfig.OperationFilter))
	}
}

func TestBuildHandlerConfigNilConfigs(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	tester := NewTester(nil, log)

	tests := []struct {
		name    string
		handler *omniav1alpha1.HandlerDefinition
	}{
		{
			name: "HTTP with nil config",
			handler: &omniav1alpha1.HandlerDefinition{
				Name: "h1",
				Type: omniav1alpha1.HandlerTypeHTTP,
			},
		},
		{
			name: "MCP with nil config",
			handler: &omniav1alpha1.HandlerDefinition{
				Name: "h2",
				Type: omniav1alpha1.HandlerTypeMCP,
			},
		},
		{
			name: "gRPC with nil config",
			handler: &omniav1alpha1.HandlerDefinition{
				Name: "h3",
				Type: omniav1alpha1.HandlerTypeGRPC,
			},
		},
		{
			name: "OpenAPI with nil config",
			handler: &omniav1alpha1.HandlerDefinition{
				Name: "h4",
				Type: omniav1alpha1.HandlerTypeOpenAPI,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := tester.buildHandlerConfig(tt.handler)
			if entry.Name != tt.handler.Name {
				t.Errorf("Name = %q, want %q", entry.Name, tt.handler.Name)
			}
		})
	}
}

func TestUnmarshalRaw(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want interface{}
	}{
		{
			name: "nil input",
			raw:  nil,
			want: nil,
		},
		{
			name: "empty input",
			raw:  []byte{},
			want: nil,
		},
		{
			name: "invalid json",
			raw:  []byte(`{invalid`),
			want: nil,
		},
		{
			name: "valid object",
			raw:  []byte(`{"type":"object"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unmarshalRaw(tt.raw)
			if tt.name == "valid object" {
				if got == nil {
					t.Error("unmarshalRaw() = nil, want non-nil")
				}
				return
			}
			if got != nil {
				t.Errorf("unmarshalRaw() = %v, want nil", got)
			}
		})
	}
}

func TestBuildHTTPEntryWithOutputSchema(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	tester := NewTester(nil, log)

	handler := &omniav1alpha1.HandlerDefinition{
		Name: "http-with-output",
		Type: omniav1alpha1.HandlerTypeHTTP,
		HTTPConfig: &omniav1alpha1.HTTPConfig{
			Endpoint: "https://api.example.com",
			Method:   "POST",
		},
		Tool: &omniav1alpha1.ToolDefinition{
			Name:        "tool-with-output",
			Description: "Has output schema",
			InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
			OutputSchema: &apiextensionsv1.JSON{
				Raw: []byte(`{"type":"string"}`),
			},
		},
	}

	entry := tester.buildHandlerConfig(handler)
	if entry.Tool == nil {
		t.Fatal("Tool is nil")
	}
	if entry.Tool.OutputSchema == nil {
		t.Fatal("Tool.OutputSchema is nil")
	}

	output, err := json.Marshal(entry.Tool.OutputSchema)
	if err != nil {
		t.Fatalf("failed to marshal OutputSchema: %v", err)
	}
	if string(output) != `{"type":"string"}` {
		t.Errorf("OutputSchema = %s, want %s", string(output), `{"type":"string"}`)
	}
}

func TestResolveHTTPAuthNilConfig(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	tester := NewTester(c, log)

	handler := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeHTTP,
		// HTTPConfig is nil
	}
	err := tester.resolveHTTPAuth(context.Background(), "default", handler)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestResolveHTTPAuthNilSecretRef(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	tester := NewTester(c, log)

	handler := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeHTTP,
		HTTPConfig: &omniav1alpha1.HTTPConfig{
			Endpoint: "https://example.com",
			// AuthSecretRef is nil
		},
	}
	err := tester.resolveHTTPAuth(context.Background(), "default", handler)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestResolveHTTPAuthSuccess(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"},
		Data:       map[string][]byte{"token": []byte("s3cr3t")},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
	tester := NewTester(c, log)

	handler := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeHTTP,
		HTTPConfig: &omniav1alpha1.HTTPConfig{
			Endpoint: "https://example.com",
			AuthSecretRef: &omniav1alpha1.SecretKeySelector{
				Name: "my-secret",
				Key:  "token",
			},
		},
	}
	err := tester.resolveHTTPAuth(context.Background(), "default", handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handler.HTTPConfig.Headers["Authorization"] != "Bearer s3cr3t" {
		t.Errorf("Authorization = %q, want %q", handler.HTTPConfig.Headers["Authorization"], "Bearer s3cr3t")
	}
}

func TestResolveHTTPAuthCustomAuthType(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"},
		Data:       map[string][]byte{"token": []byte("key123")},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
	tester := NewTester(c, log)

	authType := "api-key"
	handler := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeHTTP,
		HTTPConfig: &omniav1alpha1.HTTPConfig{
			Endpoint: "https://example.com",
			AuthType: &authType,
			AuthSecretRef: &omniav1alpha1.SecretKeySelector{
				Name: "my-secret",
				Key:  "token",
			},
		},
	}
	err := tester.resolveHTTPAuth(context.Background(), "default", handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handler.HTTPConfig.Headers["X-API-Key"] != "key123" {
		t.Errorf("X-API-Key = %q, want %q", handler.HTTPConfig.Headers["X-API-Key"], "key123")
	}
}

func TestResolveHTTPAuthMissingSecret(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	tester := NewTester(c, log)

	handler := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeHTTP,
		HTTPConfig: &omniav1alpha1.HTTPConfig{
			Endpoint: "https://example.com",
			AuthSecretRef: &omniav1alpha1.SecretKeySelector{
				Name: "nonexistent",
				Key:  "token",
			},
		},
	}
	err := tester.resolveHTTPAuth(context.Background(), "default", handler)
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
}

func TestResolveOpenAPINilConfig(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	tester := NewTester(c, log)

	handler := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeOpenAPI,
	}
	err := tester.resolveOpenAPIAuth(context.Background(), "default", handler)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestResolveOpenAPINilSecretRef(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	tester := NewTester(c, log)

	handler := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeOpenAPI,
		OpenAPIConfig: &omniav1alpha1.OpenAPIConfig{
			SpecURL: "https://example.com/spec.json",
		},
	}
	err := tester.resolveOpenAPIAuth(context.Background(), "default", handler)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestResolveOpenAPIAuthSuccess(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "api-secret", Namespace: "ns1"},
		Data:       map[string][]byte{"api-key": []byte("mytoken")},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
	tester := NewTester(c, log)

	handler := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeOpenAPI,
		OpenAPIConfig: &omniav1alpha1.OpenAPIConfig{
			SpecURL: "https://example.com/spec.json",
			AuthSecretRef: &omniav1alpha1.SecretKeySelector{
				Name: "api-secret",
				Key:  "api-key",
			},
		},
	}
	err := tester.resolveOpenAPIAuth(context.Background(), "ns1", handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handler.OpenAPIConfig.Headers["Authorization"] != "Bearer mytoken" {
		t.Errorf("Authorization = %q, want %q", handler.OpenAPIConfig.Headers["Authorization"], "Bearer mytoken")
	}
}

func TestResolveOpenAPIAuthCustomAuthType(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "api-secret", Namespace: "default"},
		Data:       map[string][]byte{"cred": []byte("dXNlcjpwYXNz")},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
	tester := NewTester(c, log)

	authType := "basic"
	handler := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeOpenAPI,
		OpenAPIConfig: &omniav1alpha1.OpenAPIConfig{
			SpecURL:  "https://example.com/spec.json",
			AuthType: &authType,
			AuthSecretRef: &omniav1alpha1.SecretKeySelector{
				Name: "api-secret",
				Key:  "cred",
			},
		},
	}
	err := tester.resolveOpenAPIAuth(context.Background(), "default", handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handler.OpenAPIConfig.Headers["Authorization"] != "Basic dXNlcjpwYXNz" {
		t.Errorf("Authorization = %q, want %q", handler.OpenAPIConfig.Headers["Authorization"], "Basic dXNlcjpwYXNz")
	}
}

func TestResolveOpenAPIAuthMissingSecret(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	tester := NewTester(c, log)

	handler := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeOpenAPI,
		OpenAPIConfig: &omniav1alpha1.OpenAPIConfig{
			SpecURL: "https://example.com/spec.json",
			AuthSecretRef: &omniav1alpha1.SecretKeySelector{
				Name: "nonexistent",
				Key:  "token",
			},
		},
	}
	err := tester.resolveOpenAPIAuth(context.Background(), "default", handler)
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
}

func TestReadSecretKeyMissingKey(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"},
		Data:       map[string][]byte{"other-key": []byte("val")},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
	tester := NewTester(c, log)

	_, err := tester.readSecretKey(context.Background(), "default", "my-secret", "missing-key")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestReadSecretKeySuccess(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"},
		Data:       map[string][]byte{"token": []byte("secret-value")},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
	tester := NewTester(c, log)

	val, err := tester.readSecretKey(context.Background(), "default", "my-secret", "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "secret-value" {
		t.Errorf("got %q, want %q", val, "secret-value")
	}
}

func TestResolveAuthSecretsDefaultType(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	tester := NewTester(c, log)

	// MCP handler type goes through default case, returns nil
	handler := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeMCP,
	}
	err := tester.resolveAuthSecrets(context.Background(), "default", handler)
	if err != nil {
		t.Fatalf("expected nil error for MCP, got: %v", err)
	}
}

func TestResolveAuthSecretsHTTPRoute(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "http-secret", Namespace: "default"},
		Data:       map[string][]byte{"tok": []byte("val")},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
	tester := NewTester(c, log)

	handler := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeHTTP,
		HTTPConfig: &omniav1alpha1.HTTPConfig{
			Endpoint: "https://example.com",
			AuthSecretRef: &omniav1alpha1.SecretKeySelector{
				Name: "http-secret",
				Key:  "tok",
			},
		},
	}
	err := tester.resolveAuthSecrets(context.Background(), "default", handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handler.HTTPConfig.Headers["Authorization"] != "Bearer val" {
		t.Errorf("Authorization = %q, want %q", handler.HTTPConfig.Headers["Authorization"], "Bearer val")
	}
}

func TestResolveAuthSecretsOpenAPIRoute(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "oa-secret", Namespace: "default"},
		Data:       map[string][]byte{"key": []byte("oaval")},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
	tester := NewTester(c, log)

	handler := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeOpenAPI,
		OpenAPIConfig: &omniav1alpha1.OpenAPIConfig{
			SpecURL: "https://example.com/spec",
			AuthSecretRef: &omniav1alpha1.SecretKeySelector{
				Name: "oa-secret",
				Key:  "key",
			},
		},
	}
	err := tester.resolveAuthSecrets(context.Background(), "default", handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handler.OpenAPIConfig.Headers["Authorization"] != "Bearer oaval" {
		t.Errorf("Authorization = %q, want %q", handler.OpenAPIConfig.Headers["Authorization"], "Bearer oaval")
	}
}

func TestExecuteTestRegistryNotFound(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	tester := NewTester(c, log)

	req := &TestRequest{
		HandlerName: "my-handler",
		Arguments:   json.RawMessage(`{}`),
	}
	outcome, err := tester.executeTest(context.Background(), "default", "nonexistent", req)
	if err == nil {
		t.Fatal("expected error for missing ToolRegistry")
	}
	if outcome != nil {
		t.Errorf("expected nil outcome, got %+v", outcome)
	}
}

func TestExecuteTestHandlerNotFound(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	registry := &omniav1alpha1.ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{Name: "my-reg", Namespace: "default"},
		Spec: omniav1alpha1.ToolRegistrySpec{
			Handlers: []omniav1alpha1.HandlerDefinition{
				{Name: "other-handler", Type: omniav1alpha1.HandlerTypeHTTP},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(registry).Build()
	tester := NewTester(c, log)

	req := &TestRequest{
		HandlerName: "my-handler",
		Arguments:   json.RawMessage(`{}`),
	}
	outcome, err := tester.executeTest(context.Background(), "default", "my-reg", req)
	if err == nil {
		t.Fatal("expected error for handler not found")
	}
	if outcome != nil {
		t.Errorf("expected nil outcome, got %+v", outcome)
	}
}

func TestExecuteTestMissingToolName(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	registry := &omniav1alpha1.ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{Name: "my-reg", Namespace: "default"},
		Spec: omniav1alpha1.ToolRegistrySpec{
			Handlers: []omniav1alpha1.HandlerDefinition{
				{
					Name: "my-handler",
					Type: omniav1alpha1.HandlerTypeHTTP,
					// No Tool definition and no ToolName in request
				},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(registry).Build()
	tester := NewTester(c, log)

	req := &TestRequest{
		HandlerName: "my-handler",
		Arguments:   json.RawMessage(`{}`),
	}
	outcome, err := tester.executeTest(context.Background(), "default", "my-reg", req)
	if err == nil {
		t.Fatal("expected error for missing tool name")
	}
	if outcome == nil {
		t.Fatal("expected non-nil outcome even on error")
	}
	if outcome.handlerType != "http" {
		t.Errorf("handlerType = %q, want %q", outcome.handlerType, "http")
	}
}

func TestExecuteTestToolNameFromHandler(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	registry := &omniav1alpha1.ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{Name: "my-reg", Namespace: "default"},
		Spec: omniav1alpha1.ToolRegistrySpec{
			Handlers: []omniav1alpha1.HandlerDefinition{
				{
					Name: "my-handler",
					Type: omniav1alpha1.HandlerTypeHTTP,
					HTTPConfig: &omniav1alpha1.HTTPConfig{
						Endpoint: "https://example.com/api",
						Method:   "POST",
					},
					Tool: &omniav1alpha1.ToolDefinition{
						Name:        "my-tool",
						Description: "A test tool",
						InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
					},
				},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(registry).Build()
	tester := NewTester(c, log)

	// ToolName left empty, should use handler's Tool.Name
	req := &TestRequest{
		HandlerName: "my-handler",
		Arguments:   json.RawMessage(`{}`),
	}
	// This will fail at executor.Initialize because there's no real HTTP server,
	// but it should get past the toolName resolution step
	outcome, err := tester.executeTest(context.Background(), "default", "my-reg", req)
	// We expect an error from the executor (not from missing tool name)
	if outcome == nil {
		t.Fatal("expected non-nil outcome")
	}
	if outcome.handlerType != "http" {
		t.Errorf("handlerType = %q, want %q", outcome.handlerType, "http")
	}
	// The error should NOT be about missing toolName
	if err != nil && err.Error() == "toolName is required for http handlers without an inline tool definition" {
		t.Error("should not get toolName-required error when handler has Tool.Name")
	}
}

func TestExecuteTestAuthSecretResolutionError(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	registry := &omniav1alpha1.ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{Name: "my-reg", Namespace: "default"},
		Spec: omniav1alpha1.ToolRegistrySpec{
			Handlers: []omniav1alpha1.HandlerDefinition{
				{
					Name: "my-handler",
					Type: omniav1alpha1.HandlerTypeHTTP,
					HTTPConfig: &omniav1alpha1.HTTPConfig{
						Endpoint: "https://example.com",
						AuthSecretRef: &omniav1alpha1.SecretKeySelector{
							Name: "missing-secret",
							Key:  "token",
						},
					},
					Tool: &omniav1alpha1.ToolDefinition{
						Name:        "my-tool",
						Description: "Test",
						InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
					},
				},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(registry).Build()
	tester := NewTester(c, log)

	req := &TestRequest{
		HandlerName: "my-handler",
		ToolName:    "my-tool",
		Arguments:   json.RawMessage(`{}`),
	}
	outcome, err := tester.executeTest(context.Background(), "default", "my-reg", req)
	if err == nil {
		t.Fatal("expected error for missing auth secret")
	}
	if outcome == nil {
		t.Fatal("expected non-nil outcome on auth error")
	}
}

func TestTestMethodIntegration(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()

	// Test with a registry that doesn't exist — covers the Test() wrapper's error path
	c := fake.NewClientBuilder().WithScheme(s).Build()
	tester := NewTester(c, log)

	req := &TestRequest{
		HandlerName: "handler-a",
		Arguments:   json.RawMessage(`{}`),
	}
	resp := tester.Test(context.Background(), "default", "nonexistent-reg", req)
	if resp.Success {
		t.Error("expected failure response")
	}
	if resp.Error == "" {
		t.Error("expected non-empty error")
	}
	if resp.DurationMs < 0 {
		t.Error("expected non-negative duration")
	}
}

func TestTestMethodWithOutcome(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()

	// A registry with a handler that has no tool name — will fail with an outcome set
	registry := &omniav1alpha1.ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{Name: "test-reg", Namespace: "default"},
		Spec: omniav1alpha1.ToolRegistrySpec{
			Handlers: []omniav1alpha1.HandlerDefinition{
				{
					Name: "handler-a",
					Type: omniav1alpha1.HandlerTypeMCP,
					// No tool, no MCP config — will fail with "toolName is required"
				},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(registry).Build()
	tester := NewTester(c, log)

	req := &TestRequest{
		HandlerName: "handler-a",
		Arguments:   json.RawMessage(`{}`),
	}
	resp := tester.Test(context.Background(), "default", "test-reg", req)
	if resp.Success {
		t.Error("expected failure response")
	}
	if resp.HandlerType != "mcp" {
		t.Errorf("HandlerType = %q, want %q", resp.HandlerType, "mcp")
	}
	if resp.Error == "" {
		t.Error("expected non-empty error")
	}
}

func TestResolveHTTPAuthWithExistingHeaders(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"},
		Data:       map[string][]byte{"token": []byte("tok123")},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
	tester := NewTester(c, log)

	handler := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeHTTP,
		HTTPConfig: &omniav1alpha1.HTTPConfig{
			Endpoint: "https://example.com",
			Headers:  map[string]string{"X-Custom": "existing"},
			AuthSecretRef: &omniav1alpha1.SecretKeySelector{
				Name: "my-secret",
				Key:  "token",
			},
		},
	}
	err := tester.resolveHTTPAuth(context.Background(), "default", handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should preserve existing headers
	if handler.HTTPConfig.Headers["X-Custom"] != "existing" {
		t.Errorf("X-Custom = %q, want %q", handler.HTTPConfig.Headers["X-Custom"], "existing")
	}
	if handler.HTTPConfig.Headers["Authorization"] != "Bearer tok123" {
		t.Errorf("Authorization = %q, want %q", handler.HTTPConfig.Headers["Authorization"], "Bearer tok123")
	}
}

func TestResolveOpenAPIAuthWithExistingHeaders(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	s := testScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "oa-secret", Namespace: "default"},
		Data:       map[string][]byte{"key": []byte("tok456")},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
	tester := NewTester(c, log)

	handler := &omniav1alpha1.HandlerDefinition{
		Type: omniav1alpha1.HandlerTypeOpenAPI,
		OpenAPIConfig: &omniav1alpha1.OpenAPIConfig{
			SpecURL: "https://example.com/spec",
			Headers: map[string]string{"X-Existing": "val"},
			AuthSecretRef: &omniav1alpha1.SecretKeySelector{
				Name: "oa-secret",
				Key:  "key",
			},
		},
	}
	err := tester.resolveOpenAPIAuth(context.Background(), "default", handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handler.OpenAPIConfig.Headers["X-Existing"] != "val" {
		t.Errorf("X-Existing = %q, want %q", handler.OpenAPIConfig.Headers["X-Existing"], "val")
	}
	if handler.OpenAPIConfig.Headers["Authorization"] != "Bearer tok456" {
		t.Errorf("Authorization = %q, want %q", handler.OpenAPIConfig.Headers["Authorization"], "Bearer tok456")
	}
}

func strPtr(s string) *string {
	return &s
}

func int32Ptr(i int32) *int32 {
	return &i
}
