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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

// Sample OpenAPI 3.0 spec for testing
const testOpenAPISpec = `{
	"openapi": "3.0.0",
	"info": {
		"title": "Test API",
		"version": "1.0.0"
	},
	"servers": [
		{"url": "https://api.example.com/v1"}
	],
	"paths": {
		"/users": {
			"get": {
				"operationId": "listUsers",
				"summary": "List all users",
				"parameters": [
					{
						"name": "limit",
						"in": "query",
						"required": false,
						"schema": {"type": "integer"}
					}
				],
				"responses": {
					"200": {"description": "Success"}
				}
			},
			"post": {
				"operationId": "createUser",
				"summary": "Create a new user",
				"requestBody": {
					"required": true,
					"content": {
						"application/json": {
							"schema": {
								"type": "object",
								"properties": {
									"name": {"type": "string"},
									"email": {"type": "string"}
								},
								"required": ["name", "email"]
							}
						}
					}
				},
				"responses": {
					"201": {"description": "Created"}
				}
			}
		},
		"/users/{id}": {
			"get": {
				"operationId": "getUser",
				"summary": "Get a user by ID",
				"parameters": [
					{
						"name": "id",
						"in": "path",
						"required": true,
						"schema": {"type": "string"}
					}
				],
				"responses": {
					"200": {"description": "Success"}
				}
			}
		}
	}
}`

func TestOpenAPIAdapter_Connect(t *testing.T) {
	// Create a test server that serves the OpenAPI spec
	specServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(testOpenAPISpec))
	}))
	defer specServer.Close()

	config := OpenAPIAdapterConfig{
		Name:    "test-api",
		SpecURL: specServer.URL + "/openapi.json",
		Timeout: 5 * time.Second,
	}

	adapter := NewOpenAPIAdapter(config, logr.Discard())

	ctx := context.Background()
	err := adapter.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Verify tools were discovered
	tools, err := adapter.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	if len(tools) != 3 {
		t.Errorf("Expected 3 tools, got %d", len(tools))
	}

	// Check for expected operations
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	expectedOps := []string{"listUsers", "createUser", "getUser"}
	for _, op := range expectedOps {
		if !toolNames[op] {
			t.Errorf("Expected operation %q not found", op)
		}
	}
}

func TestOpenAPIAdapter_ConnectWithBaseURLOverride(t *testing.T) {
	specServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(testOpenAPISpec))
	}))
	defer specServer.Close()

	customBaseURL := "https://custom.example.com/api"
	config := OpenAPIAdapterConfig{
		Name:    "test-api",
		SpecURL: specServer.URL + "/openapi.json",
		BaseURL: customBaseURL,
		Timeout: 5 * time.Second,
	}

	adapter := NewOpenAPIAdapter(config, logr.Discard())
	adapter.Connect(context.Background())

	if adapter.baseURL != customBaseURL {
		t.Errorf("Expected baseURL %q, got %q", customBaseURL, adapter.baseURL)
	}
}

func TestOpenAPIAdapter_OperationFilter(t *testing.T) {
	specServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(testOpenAPISpec))
	}))
	defer specServer.Close()

	config := OpenAPIAdapterConfig{
		Name:            "test-api",
		SpecURL:         specServer.URL + "/openapi.json",
		OperationFilter: []string{"listUsers", "getUser"},
		Timeout:         5 * time.Second,
	}

	adapter := NewOpenAPIAdapter(config, logr.Discard())
	adapter.Connect(context.Background())

	tools, _ := adapter.ListTools(context.Background())

	if len(tools) != 2 {
		t.Errorf("Expected 2 filtered tools, got %d", len(tools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	if !toolNames["listUsers"] || !toolNames["getUser"] {
		t.Error("Expected listUsers and getUser operations")
	}
	if toolNames["createUser"] {
		t.Error("createUser should have been filtered out")
	}
}

func TestOpenAPIAdapter_Call(t *testing.T) {
	// Create a test server for the spec
	specServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(testOpenAPISpec))
	}))
	defer specServer.Close()

	// Create a test API server
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/users" && r.Method == "GET":
			json.NewEncoder(w).Encode([]map[string]string{
				{"id": "1", "name": "Alice"},
				{"id": "2", "name": "Bob"},
			})
		case r.URL.Path == "/users/123" && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]string{
				"id": "123", "name": "Test User",
			})
		case r.URL.Path == "/users" && r.Method == "POST":
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{
				"id": "new", "name": "New User",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer apiServer.Close()

	config := OpenAPIAdapterConfig{
		Name:    "test-api",
		SpecURL: specServer.URL + "/openapi.json",
		BaseURL: apiServer.URL,
		Timeout: 5 * time.Second,
	}

	adapter := NewOpenAPIAdapter(config, logr.Discard())
	ctx := context.Background()
	adapter.Connect(ctx)

	t.Run("GET with query params", func(t *testing.T) {
		result, err := adapter.Call(ctx, "listUsers", map[string]any{"limit": 10})
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}
		if result.IsError {
			t.Errorf("Unexpected error result: %v", result.Content)
		}
	})

	t.Run("GET with path params", func(t *testing.T) {
		result, err := adapter.Call(ctx, "getUser", map[string]any{"id": "123"})
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}
		if result.IsError {
			t.Errorf("Unexpected error result: %v", result.Content)
		}

		// Check the result contains expected data
		if content, ok := result.Content.(map[string]any); ok {
			if content["id"] != "123" {
				t.Errorf("Expected id=123, got %v", content["id"])
			}
		}
	})

	t.Run("POST with body", func(t *testing.T) {
		result, err := adapter.Call(ctx, "createUser", map[string]any{
			"name":  "New User",
			"email": "new@example.com",
		})
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}
		if result.IsError {
			t.Errorf("Unexpected error result: %v", result.Content)
		}
	})

	t.Run("unknown operation", func(t *testing.T) {
		_, err := adapter.Call(ctx, "unknownOp", nil)
		if err == nil {
			t.Error("Expected error for unknown operation")
		}
	})
}

func TestOpenAPIAdapter_InputSchema(t *testing.T) {
	specServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(testOpenAPISpec))
	}))
	defer specServer.Close()

	config := OpenAPIAdapterConfig{
		Name:    "test-api",
		SpecURL: specServer.URL + "/openapi.json",
		Timeout: 5 * time.Second,
	}

	adapter := NewOpenAPIAdapter(config, logr.Discard())
	adapter.Connect(context.Background())

	tools, _ := adapter.ListTools(context.Background())

	// Find the createUser tool
	var createUserTool *ToolInfo
	for i, tool := range tools {
		if tool.Name == "createUser" {
			createUserTool = &tools[i]
			break
		}
	}

	if createUserTool == nil {
		t.Fatal("createUser tool not found")
	}

	// Check that the input schema includes required fields
	schema := createUserTool.InputSchema
	if schema == nil {
		t.Fatal("InputSchema is nil")
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties not found in schema")
	}

	if _, ok := props["name"]; !ok {
		t.Error("name property not found in schema")
	}
	if _, ok := props["email"]; !ok {
		t.Error("email property not found in schema")
	}
}

func TestOpenAPIAdapter_Close(t *testing.T) {
	specServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(testOpenAPISpec))
	}))
	defer specServer.Close()

	config := OpenAPIAdapterConfig{
		Name:    "test-api",
		SpecURL: specServer.URL + "/openapi.json",
		Timeout: 5 * time.Second,
	}

	adapter := NewOpenAPIAdapter(config, logr.Discard())
	adapter.Connect(context.Background())

	err := adapter.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify adapter is no longer connected
	_, err = adapter.Call(context.Background(), "listUsers", nil)
	if err == nil {
		t.Error("Expected error after Close")
	}
}

func TestOpenAPIAdapter_Swagger2(t *testing.T) {
	// Test with Swagger 2.0 format
	swagger2Spec := `{
		"swagger": "2.0",
		"info": {"title": "Test API", "version": "1.0"},
		"host": "api.example.com",
		"basePath": "/v2",
		"schemes": ["https"],
		"paths": {
			"/items": {
				"get": {
					"operationId": "listItems",
					"summary": "List items",
					"responses": {"200": {"description": "OK"}}
				}
			}
		}
	}`

	specServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(swagger2Spec))
	}))
	defer specServer.Close()

	config := OpenAPIAdapterConfig{
		Name:    "swagger-api",
		SpecURL: specServer.URL + "/swagger.json",
		Timeout: 5 * time.Second,
	}

	adapter := NewOpenAPIAdapter(config, logr.Discard())
	err := adapter.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Check base URL was extracted from host + basePath
	expected := "https://api.example.com/v2"
	if adapter.baseURL != expected {
		t.Errorf("Expected baseURL %q, got %q", expected, adapter.baseURL)
	}
}

func TestOpenAPIAdapter_GeneratedOperationID(t *testing.T) {
	// Spec without operationId - should generate one
	specWithoutOpId := `{
		"openapi": "3.0.0",
		"info": {"title": "Test", "version": "1.0"},
		"servers": [{"url": "https://api.example.com"}],
		"paths": {
			"/items/{id}": {
				"get": {
					"summary": "Get item",
					"parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
					"responses": {"200": {"description": "OK"}}
				}
			}
		}
	}`

	specServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(specWithoutOpId))
	}))
	defer specServer.Close()

	config := OpenAPIAdapterConfig{
		Name:    "test-api",
		SpecURL: specServer.URL + "/openapi.json",
		Timeout: 5 * time.Second,
	}

	adapter := NewOpenAPIAdapter(config, logr.Discard())
	adapter.Connect(context.Background())

	tools, _ := adapter.ListTools(context.Background())
	if len(tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(tools))
	}

	// Generated operationId should be like "get_items_id"
	if tools[0].Name != "get_items_id" {
		t.Errorf("Expected generated operationId 'get_items_id', got %q", tools[0].Name)
	}
}

func TestOpenAPIAdapter_Authentication(t *testing.T) {
	var receivedAuth string
	specServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(testOpenAPISpec))
	}))
	defer specServer.Close()

	config := OpenAPIAdapterConfig{
		Name:      "test-api",
		SpecURL:   specServer.URL + "/openapi.json",
		AuthType:  "bearer",
		AuthToken: "test-token",
		Timeout:   5 * time.Second,
	}

	adapter := NewOpenAPIAdapter(config, logr.Discard())
	adapter.Connect(context.Background())

	expected := "Bearer test-token"
	if receivedAuth != expected {
		t.Errorf("Expected Authorization header %q, got %q", expected, receivedAuth)
	}
}
