/*
Copyright 2026.

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

package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testResourceURL = "https://fn.svc/mcp"

func TestAuthDiscovery_ReturnsResourceMetadata(t *testing.T) {
	h := NewAuthDiscoveryHandler(AuthDiscoveryConfig{
		Resource:         testResourceURL,
		DocumentationURL: "https://omnia.altairalabs.ai/docs/functions/mcp",
	})

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("Code: got %d want 200", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type: %q", rr.Header().Get("Content-Type"))
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("body unmarshal: %v", err)
	}
	if body["resource"] != testResourceURL {
		t.Errorf("resource: %v", body["resource"])
	}
	methods, _ := body["bearer_methods_supported"].([]any)
	if len(methods) != 1 || methods[0] != "header" {
		t.Errorf("bearer_methods_supported: %v", body["bearer_methods_supported"])
	}
	servers, ok := body["authorization_servers"].([]any)
	if !ok {
		t.Fatalf("authorization_servers missing or wrong type")
	}
	if len(servers) != 0 {
		t.Errorf("authorization_servers: got %v want empty (v1 has no Omnia-side issuer)", servers)
	}
	if body["resource_documentation"] != "https://omnia.altairalabs.ai/docs/functions/mcp" {
		t.Errorf("resource_documentation: %v", body["resource_documentation"])
	}
}

func TestAuthDiscovery_OmitsDocumentationWhenAbsent(t *testing.T) {
	h := NewAuthDiscoveryHandler(AuthDiscoveryConfig{Resource: testResourceURL})

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil))

	var body map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if _, present := body["resource_documentation"]; present {
		t.Errorf("resource_documentation should be omitted when empty; body=%s", rr.Body.String())
	}
}

func TestAuthDiscovery_RejectsNonGET(t *testing.T) {
	h := NewAuthDiscoveryHandler(AuthDiscoveryConfig{Resource: testResourceURL})

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/.well-known/oauth-protected-resource", nil))

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Code: got %d want 405", rr.Code)
	}
	if rr.Header().Get("Allow") != http.MethodGet {
		t.Errorf("Allow: got %q want GET", rr.Header().Get("Allow"))
	}
}
