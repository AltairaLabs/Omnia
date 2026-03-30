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
