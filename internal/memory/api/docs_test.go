/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package api

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
	h := &Handler{}
	h.registerDocsRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, w.Body.String(), "Omnia Memory API")
	assert.Contains(t, w.Body.String(), "scalar")
}

// openapiDoc is the minimal shape we parse to verify route coverage.
type openapiDoc struct {
	Servers []struct {
		URL string `yaml:"url"`
	} `yaml:"servers"`
	Paths map[string]map[string]any `yaml:"paths"`
}

// documentedRoutes returns the set of "METHOD /absolute/path" operations the
// spec declares, resolving each path against its effective server base.
func documentedRoutes(t *testing.T, doc openapiDoc) map[string]bool {
	t.Helper()
	base := "/"
	if len(doc.Servers) > 0 {
		base = doc.Servers[0].URL
	}
	routes := make(map[string]bool)
	for path, item := range doc.Paths {
		effBase := base
		// A path-item-level servers override (e.g. the admin route) resets base.
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

// TestOpenAPISpecCoversAllRoutes is a contract test: every API route
// registered in RegisterRoutes must be documented in openapi.yaml. This
// guards against the spec-coverage drift filed as #1337.
func TestOpenAPISpecCoversAllRoutes(t *testing.T) {
	var doc openapiDoc
	require.NoError(t, yaml.Unmarshal(openapiSpec, &doc), "openapi.yaml must be valid YAML")

	documented := documentedRoutes(t, doc)

	// The canonical contract — keep in sync with Handler.RegisterRoutes.
	// /healthz, /docs, and /openapi.yaml are infra, not data-plane API.
	want := []string{
		"GET /api/v1/memories",
		"GET /api/v1/memories/search",
		"GET /api/v1/memories/export",
		"POST /api/v1/memories",
		"GET /api/v1/memories/aggregate",
		"GET /api/v1/memories/{id}",
		"PATCH /api/v1/memories/{id}",
		"POST /api/v1/memories/supersede",
		"GET /api/v1/memories/conflicts",
		"POST /api/v1/relations",
		"DELETE /api/v1/memories/{id}",
		"DELETE /api/v1/memories/batch",
		"DELETE /api/v1/memories",
		"POST /api/v1/memories/retrieve",
		"POST /api/v1/memories/retrieve/semantic",
		"POST /api/v1/institutional/memories",
		"GET /api/v1/institutional/memories",
		"DELETE /api/v1/institutional/memories/{id}",
		"POST /api/v1/institutional/ingest",
		"GET /api/v1/ingest/summary-candidates",
		"POST /api/v1/ingest/summaries",
		"POST /api/v1/agent-memories",
		"GET /api/v1/agent-memories",
		"DELETE /api/v1/agent-memories/{id}",
		"GET /api/v1/compaction/candidates",
		"POST /api/v1/compaction/summaries",
		"POST /admin/embedding-dimension-change",
	}
	for _, route := range want {
		assert.True(t, documented[route], "route %q is registered but missing from openapi.yaml", route)
	}
}

func TestOpenAPISpecEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	h := &Handler{}
	h.registerDocsRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/yaml", w.Header().Get("Content-Type"))
	assert.True(t, strings.Contains(w.Body.String(), "openapi:"))
	assert.True(t, strings.Contains(w.Body.String(), "Omnia Memory API"))
}
