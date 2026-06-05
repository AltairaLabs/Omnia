/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/memory"
)

func TestHandleSemantic_FiltersAndReturns(t *testing.T) {
	store := &fixedSearchStore{out: []*memory.Memory{
		{ID: "a", Content: "allowed", Metadata: map[string]any{testMetaKeyURL: testURLAllowed}},
		{ID: "b", Content: "secret", Metadata: map[string]any{testMetaKeyURL: "https://sp/restricted/s.docx"}},
	}}
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	h := NewHandler(svc, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"workspace_id":"ws-1","query":"failover","deny_cel":"metadata.url.contains(\"restricted\")","limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories/retrieve/semantic", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"a"`)
	assert.NotContains(t, rec.Body.String(), `"b"`)
}

func TestHandleSemantic_MissingWorkspace(t *testing.T) {
	svc := NewMemoryService(&fixedSearchStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	h := NewHandler(svc, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"query":"test","limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories/retrieve/semantic", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "workspace")
}

func TestHandleSemantic_InvalidCELReturnsError(t *testing.T) {
	svc := NewMemoryService(&fixedSearchStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	h := NewHandler(svc, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"workspace_id":"ws-1","query":"q","deny_cel":"metadata.url.bad(","limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories/retrieve/semantic", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleSemantic_DefaultLimit(t *testing.T) {
	store := &fixedSearchStore{out: []*memory.Memory{
		{ID: "x", Metadata: map[string]any{testMetaKeyURL: "u"}},
	}}
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	h := NewHandler(svc, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// limit omitted → defaults to defaultListLimit
	body := `{"workspace_id":"ws-1","query":"q"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories/retrieve/semantic", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"x"`)
}
