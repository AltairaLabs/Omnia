/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDocsEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	RegisterDocs(mux)

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, w.Body.String(), "Omnia Privacy API")
	assert.Contains(t, w.Body.String(), "scalar")
}

func TestOpenAPISpecEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	RegisterDocs(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/yaml", w.Header().Get("Content-Type"))
	assert.True(t, strings.Contains(w.Body.String(), "openapi:"))
	assert.True(t, strings.Contains(w.Body.String(), "Omnia Privacy API"))
}

// privacyOpenapiDoc is the minimal shape parsed to verify route coverage.
type privacyOpenapiDoc struct {
	Servers []struct {
		URL string `yaml:"url"`
	} `yaml:"servers"`
	Paths map[string]map[string]any `yaml:"paths"`
}

// privacyDocumentedRoutes returns the set of "METHOD /absolute/path" operations declared in the spec.
func privacyDocumentedRoutes(t *testing.T, doc privacyOpenapiDoc) map[string]bool {
	t.Helper()
	base := "/"
	if len(doc.Servers) > 0 {
		base = doc.Servers[0].URL
	}
	routes := make(map[string]bool)
	for path, item := range doc.Paths {
		effBase := base
		if srv, ok := item["servers"].([]any); ok && len(srv) > 0 {
			if m, ok := srv[0].(map[string]any); ok {
				if u, ok := m["url"].(string); ok {
					effBase = u
				}
			}
		}
		abs := strings.TrimSuffix(effBase, "/") + path
		for method := range item {
			switch method {
			case "get", "post", "patch", "delete", "put":
				routes[strings.ToUpper(method)+" "+abs] = true
			}
		}
	}
	return routes
}

// TestOpenAPISpecCoversAllRoutes verifies that every route registered in
// registerRoutes is documented in openapi.yaml. Guards against spec drift.
func TestOpenAPISpecCoversAllRoutes(t *testing.T) {
	var doc privacyOpenapiDoc
	require.NoError(t, yaml.Unmarshal(openapiSpec, &doc), "openapi.yaml must be valid YAML")

	documented := privacyDocumentedRoutes(t, doc)

	// Keep in sync with registerRoutes in ee/cmd/privacy-api/main.go.
	// /healthz, /docs, and /api/v1/openapi.yaml are infra, not data-plane API.
	want := []string{
		"POST /api/v1/privacy/opt-out",
		"DELETE /api/v1/privacy/opt-out",
		"GET /api/v1/privacy/preferences/{userID}",
		"PUT /api/v1/privacy/preferences/{userID}/consent",
		"GET /api/v1/privacy/preferences/{userID}/consent",
		"GET /api/v1/privacy/consent/stats",
		"GET /api/v1/privacy/enforcement-stats",
	}
	for _, route := range want {
		assert.True(t, documented[route], "route %q is registered but missing from openapi.yaml", route)
	}
}
