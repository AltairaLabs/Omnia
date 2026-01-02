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

func TestHTTPAdapter_ConnectInvalidURL(t *testing.T) {
	config := HTTPAdapterConfig{
		Name:     "test-http",
		Endpoint: "://invalid-url",
	}

	adapter := NewHTTPAdapter(config, logr.Discard())

	err := adapter.Connect(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid URL")
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

		// Check query parameters
		query := r.URL.Query()
		if query.Get("param1") != "value1" {
			t.Errorf("expected param1=value1, got %s", query.Get("param1"))
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
		t.Error("result should not be an error")
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
	if result.Content.(string) != "HTTP 500: Internal Server Error" {
		t.Errorf("unexpected error message: %v", result.Content)
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
	if result.Content != "Hello, World!" {
		t.Errorf("unexpected content: %v", result.Content)
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

	ctx := context.Background()
	err := adapter.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	_, err = adapter.Call(ctx, "test-http", nil)
	if err == nil {
		t.Fatal("expected error for empty bearer token")
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

	ctx := context.Background()
	err := adapter.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	_, err = adapter.Call(ctx, "test-http", nil)
	if err == nil {
		t.Fatal("expected error for invalid basic auth format")
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

	ctx := context.Background()
	err := adapter.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	_, err = adapter.Call(ctx, "test-http", nil)
	if err == nil {
		t.Fatal("expected error for unsupported auth type")
	}
}

func TestHTTPAdapter_DeleteRequest(t *testing.T) {
	// Start a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}

		// Check query parameters
		query := r.URL.Query()
		if query.Get("id") != "123" {
			t.Errorf("expected id=123, got %s", query.Get("id"))
		}

		w.WriteHeader(http.StatusNoContent)
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
