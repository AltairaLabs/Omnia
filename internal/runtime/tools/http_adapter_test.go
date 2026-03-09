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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func TestHTTPAdapter_Name(t *testing.T) {
	config := HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: "http://localhost:8080/api",
	}

	adapter := NewHTTPAdapter(config, logr.Discard())
	if adapter.Name() != "test-http" {
		t.Errorf("expected name 'test-http', got %q", adapter.Name())
	}
}

func TestHTTPAdapter_DefaultValues(t *testing.T) {
	config := HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: "http://localhost:8080/api",
	}

	adapter := NewHTTPAdapter(config, logr.Discard())
	if adapter.config.Timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", adapter.config.Timeout)
	}
	if adapter.config.Method != http.MethodPost {
		t.Errorf("expected default method POST, got %q", adapter.config.Method)
	}
	if adapter.config.ContentType != "application/json" {
		t.Errorf("expected default content-type application/json, got %q", adapter.config.ContentType)
	}
}

func TestHTTPAdapter_CustomValues(t *testing.T) {
	config := HTTPAdapterConfig{
		Name:        "test-http",
		Endpoint:    "http://localhost:8080/api",
		Timeout:     10 * time.Second,
		Method:      http.MethodGet,
		ContentType: "application/xml",
	}

	adapter := NewHTTPAdapter(config, logr.Discard())
	if adapter.config.Timeout != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", adapter.config.Timeout)
	}
	if adapter.config.Method != http.MethodGet {
		t.Errorf("expected method GET, got %q", adapter.config.Method)
	}
	if adapter.config.ContentType != "application/xml" {
		t.Errorf("expected content-type application/xml, got %q", adapter.config.ContentType)
	}
}

func TestHTTPAdapter_CallNotConnected(t *testing.T) {
	config := HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: "http://localhost:8080/api",
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	_, err := adapter.Call(context.Background(), "test-http", nil)
	if err == nil {
		t.Fatal("expected error when not connected")
	}
}

func TestHTTPAdapter_ListToolsEmpty(t *testing.T) {
	config := HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: "http://localhost:8080/api",
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	// ListTools should return empty list when not connected
	tools, err := adapter.ListTools(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestHTTPAdapter_Close(t *testing.T) {
	config := HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: "http://localhost:8080/api",
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	// Close should not error even when not connected
	err := adapter.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPAdapter_Connect(t *testing.T) {
	config := HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: "http://localhost:8080/api",
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	ctx := context.Background()
	err := adapter.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// After connect, should have one tool
	tools, err := adapter.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "test-http" {
		t.Errorf("expected tool name 'test-http', got %q", tools[0].Name)
	}
}

func TestHTTPAdapter_CallInvalidURL(t *testing.T) {
	config := HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: "://invalid-url",
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	ctx := context.Background()
	err := adapter.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// The invalid URL is caught at call time by the SDK executor
	result, err := adapter.Call(ctx, "test-http", nil)
	if err != nil {
		t.Fatalf("Call should not return error (SDK wraps it): %v", err)
	}
	if !result.IsError {
		t.Error("result should be an error for invalid URL")
	}
}

func TestHTTPAdapter_PostRequest(t *testing.T) {
	// Start a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		var args map[string]any
		json.Unmarshal(body, &args)

		response := map[string]any{
			"received": args,
			"status":   "ok",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: server.URL,
		Method:   http.MethodPost,
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	ctx := context.Background()
	err := adapter.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer adapter.Close()

	result, err := adapter.Call(ctx, "test-http", map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if result.IsError {
		t.Error("result should not be an error")
	}

	resultMap, ok := result.Content.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type: %T", result.Content)
	}
	if resultMap["status"] != "ok" {
		t.Errorf("unexpected status: %v", resultMap["status"])
	}
}

func TestHTTPAdapter_GetRequest(t *testing.T) {
	// Start a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		// Args should be sent as query parameters for GET
		if r.URL.Query().Get("param1") != "value1" {
			t.Errorf("expected query param param1=value1, got %s", r.URL.Query().Get("param1"))
		}

		response := map[string]string{"result": "success"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: server.URL,
		Method:   http.MethodGet,
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	ctx := context.Background()
	err := adapter.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer adapter.Close()

	result, err := adapter.Call(ctx, "test-http", map[string]any{"param1": "value1"})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if result.IsError {
		t.Errorf("result should not be an error: %v", result.Content)
	}
}

func TestHTTPAdapter_CustomHeaders(t *testing.T) {
	// Start a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom-Header") != "custom-value" {
			t.Errorf("expected X-Custom-Header=custom-value, got %s", r.Header.Get("X-Custom-Header"))
		}
		if r.Header.Get("X-Another") != "another-value" {
			t.Errorf("expected X-Another=another-value, got %s", r.Header.Get("X-Another"))
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	config := HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: server.URL,
		Headers: map[string]string{
			"X-Custom-Header": "custom-value",
			"X-Another":       "another-value",
		},
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	ctx := context.Background()
	err := adapter.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer adapter.Close()

	_, err = adapter.Call(ctx, "test-http", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
}

func TestHTTPAdapter_BearerAuth(t *testing.T) {
	// Start a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer my-secret-token" {
			t.Errorf("expected Bearer auth, got %s", auth)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"authenticated": true}`))
	}))
	defer server.Close()

	config := HTTPAdapterConfig{
		Name:      "test-http",
		Endpoint:  server.URL,
		AuthType:  "bearer",
		AuthToken: "my-secret-token",
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	ctx := context.Background()
	err := adapter.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer adapter.Close()

	result, err := adapter.Call(ctx, "test-http", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if result.IsError {
		t.Error("result should not be an error")
	}
}

func TestHTTPAdapter_BasicAuth(t *testing.T) {
	// Start a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok {
			t.Error("expected basic auth")
		}
		if user != "myuser" || pass != "mypass" {
			t.Errorf("expected myuser:mypass, got %s:%s", user, pass)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"authenticated": true}`))
	}))
	defer server.Close()

	config := HTTPAdapterConfig{
		Name:      "test-http",
		Endpoint:  server.URL,
		AuthType:  "basic",
		AuthToken: "myuser:mypass",
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	ctx := context.Background()
	err := adapter.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer adapter.Close()

	result, err := adapter.Call(ctx, "test-http", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if result.IsError {
		t.Error("result should not be an error")
	}
}

func TestHTTPAdapter_HTTPError(t *testing.T) {
	// Start a mock HTTP server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	config := HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: server.URL,
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	ctx := context.Background()
	err := adapter.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer adapter.Close()

	result, err := adapter.Call(ctx, "test-http", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if !result.IsError {
		t.Error("result should be an error for HTTP 500")
	}
	errMsg, ok := result.Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", result.Content)
	}
	if !strings.Contains(errMsg, "500") {
		t.Errorf("expected error message to contain '500', got: %s", errMsg)
	}
}

func TestHTTPAdapter_NonJSONResponse(t *testing.T) {
	// Start a mock HTTP server that returns plain text
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Hello, World!"))
	}))
	defer server.Close()

	config := HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: server.URL,
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	ctx := context.Background()
	err := adapter.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer adapter.Close()

	result, err := adapter.Call(ctx, "test-http", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if result.IsError {
		t.Error("result should not be an error")
	}
	// SDK wraps non-JSON responses in {"result": "..."} which unmarshals to a map
	resultMap, ok := result.Content.(map[string]any)
	if !ok {
		t.Fatalf("expected map content for non-JSON response, got %T: %v", result.Content, result.Content)
	}
	if resultMap["result"] != "Hello, World!" {
		t.Errorf("unexpected content: %v", resultMap["result"])
	}
}

func TestHTTPAdapter_InvalidBearerAuth(t *testing.T) {
	config := HTTPAdapterConfig{
		Name:      "test-http",
		Endpoint:  "http://localhost:8080",
		AuthType:  "bearer",
		AuthToken: "", // Empty token
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	// Auth validation now happens at Connect time
	err := adapter.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error for empty bearer token")
	}
	if !strings.Contains(err.Error(), "bearer auth requires a token") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHTTPAdapter_InvalidBasicAuth(t *testing.T) {
	config := HTTPAdapterConfig{
		Name:      "test-http",
		Endpoint:  "http://localhost:8080",
		AuthType:  "basic",
		AuthToken: "no-colon-here", // Invalid format
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	// Auth validation now happens at Connect time
	err := adapter.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid basic auth format")
	}
	if !strings.Contains(err.Error(), "username:password") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHTTPAdapter_UnsupportedAuthType(t *testing.T) {
	config := HTTPAdapterConfig{
		Name:      "test-http",
		Endpoint:  "http://localhost:8080",
		AuthType:  "oauth", // Not supported
		AuthToken: "token",
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	// Auth validation now happens at Connect time
	err := adapter.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error for unsupported auth type")
	}
	if !strings.Contains(err.Error(), "unsupported auth type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHTTPAdapter_HealthCheck_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := NewHTTPAdapter(HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: server.URL,
		Method:   http.MethodGet,
	}, logr.Discard())

	ctx := context.Background()
	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer adapter.Close()

	if err := adapter.HealthCheck(ctx); err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
}

func TestHTTPAdapter_HealthCheck_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	adapter := NewHTTPAdapter(HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: server.URL,
	}, logr.Discard())

	ctx := context.Background()
	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer adapter.Close()

	err := adapter.HealthCheck(ctx)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestHTTPAdapter_HealthCheck_Unreachable(t *testing.T) {
	adapter := NewHTTPAdapter(HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: "http://localhost:1", // nothing listening
	}, logr.Discard())

	ctx := context.Background()
	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer adapter.Close()

	err := adapter.HealthCheck(ctx)
	if err == nil {
		t.Fatal("expected error for unreachable endpoint")
	}
}

func TestHTTPAdapter_HealthCheck_NotConnected(t *testing.T) {
	adapter := NewHTTPAdapter(HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: "http://localhost:8080",
	}, logr.Discard())

	err := adapter.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("expected error when not connected")
	}
}

func TestHTTPAdapter_HealthCheck_UsesGETForGetMethod(t *testing.T) {
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := NewHTTPAdapter(HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: server.URL,
		Method:   http.MethodGet,
	}, logr.Discard())

	ctx := context.Background()
	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer adapter.Close()

	if err := adapter.HealthCheck(ctx); err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
	if receivedMethod != http.MethodGet {
		t.Errorf("expected GET for GET-based adapter, got %s", receivedMethod)
	}
}

func TestHTTPAdapter_HealthCheck_UsesHEADForPostMethod(t *testing.T) {
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := NewHTTPAdapter(HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: server.URL,
		Method:   http.MethodPost,
	}, logr.Discard())

	ctx := context.Background()
	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer adapter.Close()

	if err := adapter.HealthCheck(ctx); err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
	if receivedMethod != http.MethodHead {
		t.Errorf("expected HEAD for POST-based adapter, got %s", receivedMethod)
	}
}

func TestHTTPAdapter_ImplementsHealthChecker(t *testing.T) {
	adapter := NewHTTPAdapter(HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: "http://localhost:8080",
	}, logr.Discard())

	var _ HealthChecker = adapter // compile-time check
}

func TestHTTPAdapter_DeleteRequest(t *testing.T) {
	// Start a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}

		// Args should be sent as query parameters for DELETE
		if r.URL.Query().Get("id") != "123" {
			t.Errorf("expected query param id=123, got %s", r.URL.Query().Get("id"))
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"deleted": true}`))
	}))
	defer server.Close()

	config := HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: server.URL,
		Method:   http.MethodDelete,
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	ctx := context.Background()
	err := adapter.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer adapter.Close()

	result, err := adapter.Call(ctx, "test-http", map[string]any{"id": "123"})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if result.IsError {
		t.Error("result should not be an error")
	}
}

func TestHTTPAdapter_PutRequest(t *testing.T) {
	// Start a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}

		body, _ := io.ReadAll(r.Body)
		var args map[string]any
		json.Unmarshal(body, &args)

		if args["name"] != "updated" {
			t.Errorf("expected name=updated, got %v", args["name"])
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"updated": true}`))
	}))
	defer server.Close()

	config := HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: server.URL,
		Method:   http.MethodPut,
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	ctx := context.Background()
	err := adapter.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer adapter.Close()

	result, err := adapter.Call(ctx, "test-http", map[string]any{"name": "updated"})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if result.IsError {
		t.Error("result should not be an error")
	}
}

func TestAppendQueryParams(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		args    map[string]any
		wantURL string
	}{
		{
			name:    "string value",
			baseURL: "http://example.com/api",
			args:    map[string]any{"expr": "2+2"},
			wantURL: "http://example.com/api?expr=2%2B2",
		},
		{
			name:    "numeric value",
			baseURL: "http://example.com/api",
			args:    map[string]any{"lat": 51.5},
			wantURL: "http://example.com/api?lat=51.5",
		},
		{
			name:    "boolean value",
			baseURL: "http://example.com/api",
			args:    map[string]any{"verbose": true},
			wantURL: "http://example.com/api?verbose=true",
		},
		{
			name:    "existing query params",
			baseURL: "http://example.com/api?key=abc",
			args:    map[string]any{"q": "test"},
			wantURL: "http://example.com/api?key=abc&q=test",
		},
		{
			name:    "array value (JSON-encoded)",
			baseURL: "http://example.com/api",
			args:    map[string]any{"ids": []any{"a", "b"}},
			wantURL: `http://example.com/api?ids=%5B%22a%22%2C%22b%22%5D`,
		},
		{
			name:    "empty args",
			baseURL: "http://example.com/api",
			args:    map[string]any{},
			wantURL: "http://example.com/api?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendQueryParams(tt.baseURL, tt.args)
			if got != tt.wantURL {
				t.Errorf("appendQueryParams() = %q, want %q", got, tt.wantURL)
			}
		})
	}
}

func TestMethodUsesQueryParams(t *testing.T) {
	if !methodUsesQueryParams("GET") {
		t.Error("GET should use query params")
	}
	if !methodUsesQueryParams("get") {
		t.Error("get (lowercase) should use query params")
	}
	if !methodUsesQueryParams("DELETE") {
		t.Error("DELETE should use query params")
	}
	if methodUsesQueryParams("POST") {
		t.Error("POST should not use query params")
	}
	if methodUsesQueryParams("PUT") {
		t.Error("PUT should not use query params")
	}
	if methodUsesQueryParams("PATCH") {
		t.Error("PATCH should not use query params")
	}
}
