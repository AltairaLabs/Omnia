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

package schema

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func TestSchemaValidator_Validate(t *testing.T) {
	validator := NewSchemaValidator()

	tests := []struct {
		name        string
		data        string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid pack",
			data: `{
				"id": "test-pack",
				"name": "Test Pack",
				"version": "1.0.0",
				"template_engine": {
					"version": "v1",
					"syntax": "{{variable}}"
				},
				"prompts": {
					"default": {
						"id": "default",
						"name": "Default Prompt",
						"version": "1.0.0",
						"system_template": "You are a helpful assistant."
					}
				}
			}`,
			wantErr: false,
		},
		{
			name: "valid pack with $schema field",
			data: `{
				"$schema": "https://promptpack.org/schema/latest/promptpack.schema.json",
				"id": "test-pack",
				"name": "Test Pack",
				"version": "1.0.0",
				"template_engine": {
					"version": "v1",
					"syntax": "{{variable}}"
				},
				"prompts": {
					"default": {
						"id": "default",
						"name": "Default Prompt",
						"version": "1.0.0",
						"system_template": "You are a helpful assistant."
					}
				}
			}`,
			wantErr: false,
		},
		{
			name:        "missing id",
			data:        `{"name": "Test Pack", "version": "1.0.0", "template_engine": {"version": "v1", "syntax": "{{variable}}"}, "prompts": {"default": {"id": "default", "name": "Default", "version": "1.0.0", "system_template": "Test"}}}`,
			wantErr:     true,
			errContains: "id",
		},
		{
			name:        "missing name",
			data:        `{"id": "test", "version": "1.0.0", "template_engine": {"version": "v1", "syntax": "{{variable}}"}, "prompts": {"default": {"id": "default", "name": "Default", "version": "1.0.0", "system_template": "Test"}}}`,
			wantErr:     true,
			errContains: "name",
		},
		{
			name:        "missing version",
			data:        `{"id": "test", "name": "Test Pack", "template_engine": {"version": "v1", "syntax": "{{variable}}"}, "prompts": {"default": {"id": "default", "name": "Default", "version": "1.0.0", "system_template": "Test"}}}`,
			wantErr:     true,
			errContains: "version",
		},
		{
			name:        "missing template_engine",
			data:        `{"id": "test", "name": "Test Pack", "version": "1.0.0", "prompts": {"default": {"id": "default", "name": "Default", "version": "1.0.0", "system_template": "Test"}}}`,
			wantErr:     true,
			errContains: "template_engine",
		},
		{
			name:        "missing prompts",
			data:        `{"id": "test", "name": "Test Pack", "version": "1.0.0", "template_engine": {"version": "v1", "syntax": "{{variable}}"}}`,
			wantErr:     true,
			errContains: "prompts",
		},
		{
			name:        "empty prompts",
			data:        `{"id": "test", "name": "Test Pack", "version": "1.0.0", "template_engine": {"version": "v1", "syntax": "{{variable}}"}, "prompts": {}}`,
			wantErr:     true,
			errContains: "prompts",
		},
		{
			name:        "invalid JSON",
			data:        `{invalid json`,
			wantErr:     true,
			errContains: "schema validation error",
		},
		{
			name:        "invalid id format (uppercase)",
			data:        `{"id": "Test-Pack", "name": "Test Pack", "version": "1.0.0", "template_engine": {"version": "v1", "syntax": "{{variable}}"}, "prompts": {"default": {"id": "default", "name": "Default", "version": "1.0.0", "system_template": "Test"}}}`,
			wantErr:     true,
			errContains: "id",
		},
		{
			name:        "invalid version format",
			data:        `{"id": "test", "name": "Test Pack", "version": "invalid", "template_engine": {"version": "v1", "syntax": "{{variable}}"}, "prompts": {"default": {"id": "default", "name": "Default", "version": "1.0.0", "system_template": "Test"}}}`,
			wantErr:     true,
			errContains: "version",
		},
		{
			name:        "prompt missing system_template",
			data:        `{"id": "test", "name": "Test Pack", "version": "1.0.0", "template_engine": {"version": "v1", "syntax": "{{variable}}"}, "prompts": {"default": {"id": "default", "name": "Default", "version": "1.0.0"}}}`,
			wantErr:     true,
			errContains: "system_template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate([]byte(tt.data))
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Validate() error = %v, want error containing %q", err, tt.errContains)
				}
			}
		})
	}
}

func TestExtractSchemaURL(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "with $schema field",
			data: `{"$schema": "https://promptpack.org/schema/v1/promptpack.schema.json", "id": "test"}`,
			want: "https://promptpack.org/schema/v1/promptpack.schema.json",
		},
		{
			name: "without $schema field",
			data: `{"id": "test"}`,
			want: "",
		},
		{
			name: "invalid JSON",
			data: `{invalid`,
			want: "",
		},
		{
			name: "empty $schema",
			data: `{"$schema": "", "id": "test"}`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSchemaURL([]byte(tt.data))
			if got != tt.want {
				t.Errorf("extractSchemaURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewSchemaValidatorWithOptions(t *testing.T) {
	// Test with custom options
	validator := NewSchemaValidatorWithOptions(logr.Discard(), nil, 30*time.Minute)
	if validator == nil {
		t.Error("NewSchemaValidatorWithOptions() returned nil")
		return
	}
	if validator.cacheDuration != 30*time.Minute {
		t.Errorf("cacheDuration = %v, want %v", validator.cacheDuration, 30*time.Minute)
	}

	// Test with zero duration defaults to 1 hour
	validator2 := NewSchemaValidatorWithOptions(logr.Discard(), nil, 0)
	if validator2.cacheDuration != defaultCacheDuration {
		t.Errorf("cacheDuration = %v, want %v", validator2.cacheDuration, defaultCacheDuration)
	}
}

func TestGetEmbeddedSchemaVersion(t *testing.T) {
	version, err := GetEmbeddedSchemaVersion()
	if err != nil {
		t.Errorf("GetEmbeddedSchemaVersion() error = %v", err)
		return
	}
	if version == "" {
		t.Error("GetEmbeddedSchemaVersion() returned empty version")
	}
	// Should be a valid semver-like string
	if !strings.Contains(version, ".") {
		t.Errorf("GetEmbeddedSchemaVersion() = %v, expected semver format", version)
	}
}

func TestSchemaValidator_UsesEmbeddedSchema(t *testing.T) {
	// Create validator that will fail network fetch (invalid HTTP client)
	validator := NewSchemaValidator()

	// Valid pack should still validate using embedded schema
	validPack := `{
		"id": "test-pack",
		"name": "Test Pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "Default Prompt",
				"version": "1.0.0",
				"system_template": "You are a helpful assistant."
			}
		}
	}`

	err := validator.Validate([]byte(validPack))
	if err != nil {
		t.Errorf("Validate() with embedded schema failed: %v", err)
	}
}

func TestSchemaValidator_NetworkFetch(t *testing.T) {
	// Track how many times the server is called
	var fetchCount atomic.Int32

	// Create a test server that serves the embedded schema
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Serve the embedded schema
		_, _ = w.Write([]byte(embeddedSchema))
	}))
	defer server.Close()

	// Create validator with custom HTTP client pointing to test server
	validator := NewSchemaValidatorWithOptions(logr.Discard(), server.Client(), 1*time.Hour)

	// Override the cache to use our test server URL
	validPack := `{
		"$schema": "` + server.URL + `/schema.json",
		"id": "test-pack",
		"name": "Test Pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "Default Prompt",
				"version": "1.0.0",
				"system_template": "You are a helpful assistant."
			}
		}
	}`

	// First validation should fetch from network
	err := validator.Validate([]byte(validPack))
	if err != nil {
		t.Errorf("First Validate() failed: %v", err)
	}
	if fetchCount.Load() != 1 {
		t.Errorf("Expected 1 network fetch, got %d", fetchCount.Load())
	}

	// Second validation should use cache (no additional fetch)
	err = validator.Validate([]byte(validPack))
	if err != nil {
		t.Errorf("Second Validate() failed: %v", err)
	}
	if fetchCount.Load() != 1 {
		t.Errorf("Expected still 1 network fetch (cached), got %d", fetchCount.Load())
	}
}

func TestSchemaValidator_CacheExpiration(t *testing.T) {
	var fetchCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(embeddedSchema))
	}))
	defer server.Close()

	// Create validator with very short cache duration
	validator := NewSchemaValidatorWithOptions(logr.Discard(), server.Client(), 10*time.Millisecond)

	validPack := `{
		"$schema": "` + server.URL + `/schema.json",
		"id": "test-pack",
		"name": "Test Pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "Default Prompt",
				"version": "1.0.0",
				"system_template": "You are a helpful assistant."
			}
		}
	}`

	// First validation fetches from network
	err := validator.Validate([]byte(validPack))
	if err != nil {
		t.Errorf("First Validate() failed: %v", err)
	}
	if fetchCount.Load() != 1 {
		t.Errorf("Expected 1 fetch, got %d", fetchCount.Load())
	}

	// Wait for cache to expire
	time.Sleep(20 * time.Millisecond)

	// Third validation should fetch again (cache expired)
	err = validator.Validate([]byte(validPack))
	if err != nil {
		t.Errorf("Third Validate() failed: %v", err)
	}
	if fetchCount.Load() != 2 {
		t.Errorf("Expected 2 fetches after cache expiry, got %d", fetchCount.Load())
	}
}

func TestSchemaValidator_NetworkFailureFallback(t *testing.T) {
	var fetchCount atomic.Int32

	// Create a server that always returns errors
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	validator := NewSchemaValidatorWithOptions(logr.Discard(), server.Client(), 1*time.Hour)

	// Pack with $schema pointing to failing server
	validPack := `{
		"$schema": "` + server.URL + `/schema.json",
		"id": "test-pack",
		"name": "Test Pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "Default Prompt",
				"version": "1.0.0",
				"system_template": "You are a helpful assistant."
			}
		}
	}`

	// Should still succeed using embedded fallback
	err := validator.Validate([]byte(validPack))
	if err != nil {
		t.Errorf("Validate() should succeed with fallback, got: %v", err)
	}

	// Verify network was attempted
	if fetchCount.Load() != 1 {
		t.Errorf("Expected 1 network attempt, got %d", fetchCount.Load())
	}
}

func TestSchemaValidator_InvalidJSONFromNetwork(t *testing.T) {
	var fetchCount atomic.Int32

	// Server returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	validator := NewSchemaValidatorWithOptions(logr.Discard(), server.Client(), 1*time.Hour)

	validPack := `{
		"$schema": "` + server.URL + `/schema.json",
		"id": "test-pack",
		"name": "Test Pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "Default Prompt",
				"version": "1.0.0",
				"system_template": "You are a helpful assistant."
			}
		}
	}`

	// Should still succeed using embedded fallback
	err := validator.Validate([]byte(validPack))
	if err != nil {
		t.Errorf("Validate() should succeed with fallback after invalid JSON, got: %v", err)
	}

	if fetchCount.Load() != 1 {
		t.Errorf("Expected 1 network attempt, got %d", fetchCount.Load())
	}
}

func TestSchemaValidator_GetSchemaLoaderSource(t *testing.T) {
	var fetchCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(embeddedSchema))
	}))
	defer server.Close()

	validator := NewSchemaValidatorWithOptions(logr.Discard(), server.Client(), 1*time.Hour)
	schemaURL := server.URL + "/schema.json"

	// First call should return network source
	_, source := validator.getSchemaLoader(schemaURL)
	if source != SchemaSourceNetwork {
		t.Errorf("First call: expected source %v, got %v", SchemaSourceNetwork, source)
	}

	// Second call should return cache source
	_, source = validator.getSchemaLoader(schemaURL)
	if source != SchemaSourceCache {
		t.Errorf("Second call: expected source %v, got %v", SchemaSourceCache, source)
	}

	// Call with failing URL should return embedded source
	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failingServer.Close()

	validator2 := NewSchemaValidatorWithOptions(logr.Discard(), failingServer.Client(), 1*time.Hour)
	_, source = validator2.getSchemaLoader(failingServer.URL + "/schema.json")
	if source != SchemaSourceEmbedded {
		t.Errorf("Failing URL: expected source %v, got %v", SchemaSourceEmbedded, source)
	}
}
