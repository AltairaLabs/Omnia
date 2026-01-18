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

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func newTestServerWithGuardrails(t *testing.T, config GuardrailsConfig, objs ...interface{}) *Server {
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))

	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, obj := range objs {
		if o, ok := obj.(runtime.Object); ok {
			builder = builder.WithRuntimeObjects(o)
		}
	}

	fakeClient := builder.Build()
	return NewServer(fakeClient, nil, zap.New(zap.UseDevMode(true)), "", config)
}

func TestParseNamespaceList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single namespace",
			input:    "default",
			expected: []string{"default"},
		},
		{
			name:     "multiple namespaces",
			input:    "default,kube-system,production",
			expected: []string{"default", "kube-system", "production"},
		},
		{
			name:     "namespaces with spaces",
			input:    "default, kube-system , production",
			expected: []string{"default", "kube-system", "production"},
		},
		{
			name:     "namespaces with empty parts",
			input:    "default,,kube-system",
			expected: []string{"default", "kube-system"},
		},
		{
			name:     "wildcard",
			input:    "*",
			expected: []string{"*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseNamespaceList(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNamespaceAllowed(t *testing.T) {
	tests := []struct {
		name      string
		config    GuardrailsConfig
		namespace string
		expected  bool
	}{
		{
			name: "empty config allows all",
			config: GuardrailsConfig{
				Mode: ProxyModeStrict,
			},
			namespace: "any-namespace",
			expected:  true,
		},
		{
			name: "denied namespace",
			config: GuardrailsConfig{
				Mode:             ProxyModeStrict,
				DeniedNamespaces: []string{"kube-system"},
			},
			namespace: "kube-system",
			expected:  false,
		},
		{
			name: "allowed namespace",
			config: GuardrailsConfig{
				Mode:              ProxyModeStrict,
				AllowedNamespaces: []string{"production", "staging"},
			},
			namespace: "production",
			expected:  true,
		},
		{
			name: "namespace not in allowed list",
			config: GuardrailsConfig{
				Mode:              ProxyModeStrict,
				AllowedNamespaces: []string{"production", "staging"},
			},
			namespace: "development",
			expected:  false,
		},
		{
			name: "deny takes precedence over allow",
			config: GuardrailsConfig{
				Mode:              ProxyModeStrict,
				AllowedNamespaces: []string{"kube-system", "production"},
				DeniedNamespaces:  []string{"kube-system"},
			},
			namespace: "kube-system",
			expected:  false,
		},
		{
			name: "wildcard in allowed list",
			config: GuardrailsConfig{
				Mode:              ProxyModeStrict,
				AllowedNamespaces: []string{"*"},
				DeniedNamespaces:  []string{"kube-system"},
			},
			namespace: "any-namespace",
			expected:  true,
		},
		{
			name: "wildcard allowed but specific denied",
			config: GuardrailsConfig{
				Mode:              ProxyModeStrict,
				AllowedNamespaces: []string{"*"},
				DeniedNamespaces:  []string{"kube-system"},
			},
			namespace: "kube-system",
			expected:  false,
		},
		{
			name: "list all namespaces with allowed list",
			config: GuardrailsConfig{
				Mode:              ProxyModeStrict,
				AllowedNamespaces: []string{"production"},
			},
			namespace: "*",
			expected:  true, // Allow list operation, filtering should happen at result level
		},
		{
			name: "list all namespaces with wildcard",
			config: GuardrailsConfig{
				Mode:              ProxyModeStrict,
				AllowedNamespaces: []string{"*"},
			},
			namespace: "*",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newTestServerWithGuardrails(t, tt.config)
			result := server.isNamespaceAllowed(tt.namespace)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGuardrailsMiddlewareCompatMode(t *testing.T) {
	agent := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "kube-system",
		},
	}

	// In compat mode, requests to denied namespaces should still be allowed
	config := GuardrailsConfig{
		Mode:             ProxyModeCompat,
		DeniedNamespaces: []string{"kube-system"},
	}

	server := newTestServerWithGuardrails(t, config, agent)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/agents/kube-system/test-agent", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should succeed in compat mode even though kube-system is in denied list
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestGuardrailsMiddlewareStrictModeDeny(t *testing.T) {
	agent := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "kube-system",
		},
	}

	config := GuardrailsConfig{
		Mode:             ProxyModeStrict,
		DeniedNamespaces: []string{"kube-system"},
	}

	server := newTestServerWithGuardrails(t, config, agent)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/agents/kube-system/test-agent", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should be forbidden in strict mode
	assert.Equal(t, http.StatusForbidden, rec.Code)

	var response map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, "access to namespace denied", response["error"])
}

func TestGuardrailsMiddlewareStrictModeAllow(t *testing.T) {
	agent := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "production",
		},
	}

	config := GuardrailsConfig{
		Mode:              ProxyModeStrict,
		AllowedNamespaces: []string{"production", "staging"},
	}

	server := newTestServerWithGuardrails(t, config, agent)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/agents/production/test-agent", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should succeed for allowed namespace
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestGuardrailsMiddlewareStrictModeNotInAllowList(t *testing.T) {
	config := GuardrailsConfig{
		Mode:              ProxyModeStrict,
		AllowedNamespaces: []string{"production", "staging"},
	}

	server := newTestServerWithGuardrails(t, config)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/agents/development/test-agent", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should be forbidden for namespace not in allowed list
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestParseRequestInfo(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		path         string
		expectedVerb string
		expectedRes  string
		expectedNs   string
		expectedName string
	}{
		{
			name:         "list agents",
			method:       "GET",
			path:         "/api/v1/agents",
			expectedVerb: "list",
			expectedRes:  "agents",
			expectedNs:   "*",
			expectedName: "",
		},
		{
			name:         "get agent",
			method:       "GET",
			path:         "/api/v1/agents/default/my-agent",
			expectedVerb: "get",
			expectedRes:  "agents",
			expectedNs:   "default",
			expectedName: "my-agent",
		},
		{
			name:         "get agent logs",
			method:       "GET",
			path:         "/api/v1/agents/production/my-agent/logs",
			expectedVerb: "get",
			expectedRes:  "agents/logs",
			expectedNs:   "production",
			expectedName: "my-agent",
		},
		{
			name:         "get agent events",
			method:       "GET",
			path:         "/api/v1/agents/production/my-agent/events",
			expectedVerb: "get",
			expectedRes:  "agents/events",
			expectedNs:   "production",
			expectedName: "my-agent",
		},
		{
			name:         "scale agent",
			method:       "PUT",
			path:         "/api/v1/agents/default/my-agent/scale",
			expectedVerb: "update",
			expectedRes:  "agents/scale",
			expectedNs:   "default",
			expectedName: "my-agent",
		},
		{
			name:         "create agent",
			method:       "POST",
			path:         "/api/v1/agents",
			expectedVerb: "create",
			expectedRes:  "agents",
			expectedNs:   "", // namespace is in request body, not path
			expectedName: "",
		},
		{
			name:         "list promptpacks",
			method:       "GET",
			path:         "/api/v1/promptpacks",
			expectedVerb: "list",
			expectedRes:  "promptpacks",
			expectedNs:   "*",
			expectedName: "",
		},
		{
			name:         "get promptpack content",
			method:       "GET",
			path:         "/api/v1/promptpacks/default/my-pack/content",
			expectedVerb: "get",
			expectedRes:  "promptpacks/content",
			expectedNs:   "default",
			expectedName: "my-pack",
		},
		{
			name:         "list toolregistries",
			method:       "GET",
			path:         "/api/v1/toolregistries",
			expectedVerb: "list",
			expectedRes:  "toolregistries",
			expectedNs:   "*",
			expectedName: "",
		},
		{
			name:         "get toolregistry",
			method:       "GET",
			path:         "/api/v1/toolregistries/default/my-registry",
			expectedVerb: "get",
			expectedRes:  "toolregistries",
			expectedNs:   "default",
			expectedName: "my-registry",
		},
		{
			name:         "list providers",
			method:       "GET",
			path:         "/api/v1/providers",
			expectedVerb: "list",
			expectedRes:  "providers",
			expectedNs:   "*",
			expectedName: "",
		},
		{
			name:         "get stats",
			method:       "GET",
			path:         "/api/v1/stats",
			expectedVerb: "list",
			expectedRes:  "stats",
			expectedNs:   "*",
			expectedName: "",
		},
		{
			name:         "list namespaces",
			method:       "GET",
			path:         "/api/v1/namespaces",
			expectedVerb: "list",
			expectedRes:  "namespaces",
			expectedNs:   "*",
			expectedName: "",
		},
	}

	config := GuardrailsConfig{Mode: ProxyModeCompat}
	server := newTestServerWithGuardrails(t, config)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			info := server.parseRequestInfo(req)

			assert.Equal(t, tt.expectedVerb, info.Verb, "unexpected verb")
			assert.Equal(t, tt.expectedRes, info.Resource, "unexpected resource")
			assert.Equal(t, tt.expectedNs, info.Namespace, "unexpected namespace")
			assert.Equal(t, tt.expectedName, info.Name, "unexpected name")
			assert.Equal(t, tt.path, info.Path, "unexpected path")
		})
	}
}

func TestGetUserFromRequest(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected string
	}{
		{
			name:     "no headers",
			headers:  nil,
			expected: "anonymous",
		},
		{
			name:     "X-Forwarded-User header",
			headers:  map[string]string{"X-Forwarded-User": "alice@example.com"},
			expected: "alice@example.com",
		},
		{
			name:     "X-Forwarded-Email header",
			headers:  map[string]string{"X-Forwarded-Email": "bob@example.com"},
			expected: "bob@example.com",
		},
		{
			name:     "X-Remote-User header",
			headers:  map[string]string{"X-Remote-User": "charlie"},
			expected: "charlie",
		},
		{
			name:     "Bearer token",
			headers:  map[string]string{"Authorization": "Bearer eyJhbGc..."},
			expected: "bearer-token",
		},
		{
			name:     "X-Forwarded-User takes precedence",
			headers:  map[string]string{"X-Forwarded-User": "alice", "X-Forwarded-Email": "alice@example.com"},
			expected: "alice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/agents", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			result := getUserFromRequest(req)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGuardrailsCORSPreflightBypass(t *testing.T) {
	// OPTIONS requests should bypass guardrails
	config := GuardrailsConfig{
		Mode:             ProxyModeStrict,
		DeniedNamespaces: []string{"kube-system"},
	}

	server := newTestServerWithGuardrails(t, config)
	handler := server.Handler()

	req := httptest.NewRequest("OPTIONS", "/api/v1/agents/kube-system/test-agent", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// OPTIONS should succeed even for denied namespace
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestGuardrailsListOperationAllowed(t *testing.T) {
	agent1 := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-1",
			Namespace: "production",
		},
	}
	agent2 := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-2",
			Namespace: "staging",
		},
	}

	config := GuardrailsConfig{
		Mode:              ProxyModeStrict,
		AllowedNamespaces: []string{"production"},
	}

	server := newTestServerWithGuardrails(t, config, agent1, agent2)
	handler := server.Handler()

	// List all agents - should be allowed but results would be filtered (if implemented)
	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// List operation is allowed (filtering is done at result level if needed)
	assert.Equal(t, http.StatusOK, rec.Code)
}
