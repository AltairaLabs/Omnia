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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/memory"
)

// --- Mock store ---

type mockStore struct {
	memories  []*memory.Memory
	saveErr   error
	retErr    error
	listErr   error
	delErr    error
	delAllErr error
	savedMem  *memory.Memory
}

func (m *mockStore) Save(_ context.Context, mem *memory.Memory) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	mem.ID = "mock-id-001"
	m.savedMem = mem
	return nil
}

func (m *mockStore) Retrieve(_ context.Context, _ map[string]string, _ string, _ memory.RetrieveOptions) ([]*memory.Memory, error) {
	if m.retErr != nil {
		return nil, m.retErr
	}
	return m.memories, nil
}

func (m *mockStore) List(_ context.Context, _ map[string]string, _ memory.ListOptions) ([]*memory.Memory, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.memories, nil
}

func (m *mockStore) Delete(_ context.Context, _ map[string]string, _ string) error {
	return m.delErr
}

func (m *mockStore) DeleteAll(_ context.Context, _ map[string]string) error {
	return m.delAllErr
}

func newTestHandler(store memory.Store) *Handler {
	svc := NewMemoryService(store, logr.Discard())
	return NewHandler(svc, logr.Discard())
}

func setupMux(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

// --- List memories tests ---

func TestHandleListMemories_Success(t *testing.T) {
	store := &mockStore{
		memories: []*memory.Memory{
			{ID: "1", Type: "preference", Content: "likes Go"},
			{ID: "2", Type: "fact", Content: "uses Linux"},
		},
	}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp MemoryListResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 2, resp.Total)
	assert.Len(t, resp.Memories, 2)
}

func TestHandleListMemories_MissingWorkspace(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var resp ErrorResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Contains(t, resp.Error, "workspace")
}

func TestHandleListMemories_WithFilters(t *testing.T) {
	store := &mockStore{memories: []*memory.Memory{}}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/memories?workspace=ws1&user_id=u1&agent=a1&type=preference&limit=10&offset=5", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleListMemories_StoreError(t *testing.T) {
	store := &mockStore{listErr: fmt.Errorf("db connection lost")}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// --- Search memories tests ---

func TestHandleSearchMemories_Success(t *testing.T) {
	store := &mockStore{
		memories: []*memory.Memory{
			{ID: "1", Content: "prefers dark mode"},
		},
	}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/memories/search?workspace=ws1&q=dark+mode", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp MemoryListResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 1, resp.Total)
}

func TestHandleSearchMemories_MissingWorkspace(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/search?q=test", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleSearchMemories_MissingQuery(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/search?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var resp ErrorResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Contains(t, resp.Error, "query")
}

func TestHandleSearchMemories_StoreError(t *testing.T) {
	store := &mockStore{retErr: fmt.Errorf("search failed")}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/memories/search?workspace=ws1&q=test", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// --- Save memory tests ---

func TestHandleSaveMemory_Success(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	mux := setupMux(h)

	body := SaveMemoryRequest{
		Type:       "preference",
		Content:    "likes Go",
		Confidence: 0.9,
		Scope:      map[string]string{"workspace_id": "ws1"},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)

	var resp MemoryResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "mock-id-001", resp.Memory.ID)
}

func TestHandleSaveMemory_BadJSON(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories",
		strings.NewReader("not-json"))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleSaveMemory_StoreError(t *testing.T) {
	store := &mockStore{saveErr: fmt.Errorf("insert failed")}
	h := newTestHandler(store)
	mux := setupMux(h)

	body := SaveMemoryRequest{
		Type:    "fact",
		Content: "test",
		Scope:   map[string]string{"workspace_id": "ws1"},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories", bytes.NewReader(b))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestHandleSaveMemory_BodyTooLarge(t *testing.T) {
	store := &mockStore{}
	svc := NewMemoryService(store, logr.Discard())
	h := &Handler{
		service:     svc,
		log:         logr.Discard(),
		maxBodySize: 10, // very small limit
	}
	mux := setupMux(h)

	// Use valid JSON that exceeds the limit to trigger MaxBytesError.
	body := `{"type":"preference","content":"` + strings.Repeat("x", 100) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
}

// --- Delete memory tests ---

func TestHandleDeleteMemory_Success(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/memories/mem-123?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleDeleteMemory_StoreError(t *testing.T) {
	store := &mockStore{delErr: fmt.Errorf("not found")}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/memories/mem-123?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// --- DeleteAll tests ---

func TestHandleDeleteAllMemories_Success(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/memories?workspace=ws1&user_id=u1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandleDeleteAllMemories_MissingWorkspace(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/memories", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleDeleteAllMemories_StoreError(t *testing.T) {
	store := &mockStore{delAllErr: fmt.Errorf("cascade failed")}
	h := newTestHandler(store)
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/memories?workspace=ws1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// --- Healthz test ---

func TestHandleHealthz(t *testing.T) {
	h := newTestHandler(&mockStore{})
	mux := setupMux(h)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "ok", rr.Body.String())
}

// --- Helper tests ---

func TestParseTypes(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"preference", []string{"preference"}},
		{"preference,fact", []string{"preference", "fact"}},
		{" preference , fact ", []string{"preference", "fact"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseTypes(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMinMax(t *testing.T) {
	// Verify the min/max clamping pattern used in handlers.
	assert.Equal(t, 1, min(max(0, 1), 100))
	assert.Equal(t, 50, min(max(50, 1), 100))
	assert.Equal(t, 100, min(max(200, 1), 100))
}

func TestTruncateParam(t *testing.T) {
	assert.Equal(t, "abc", truncateParam("abc"))
	// String longer than maxStringParamLen gets truncated.
	long := strings.Repeat("x", maxStringParamLen+10)
	assert.Len(t, truncateParam(long), maxStringParamLen)
}

func TestBuildScope(t *testing.T) {
	q := fakeQuery(map[string]string{
		"workspace": "ws1",
		"user_id":   "u1",
		"agent":     "a1",
	})
	scope := buildScope(q)
	assert.Equal(t, "ws1", scope[memory.ScopeWorkspaceID])
	assert.Equal(t, "u1", scope[memory.ScopeUserID])
	assert.Equal(t, "a1", scope[memory.ScopeAgentID])
}

func TestBuildScope_MinimalParams(t *testing.T) {
	q := fakeQuery(map[string]string{"workspace": "ws1"})
	scope := buildScope(q)
	assert.Equal(t, "ws1", scope[memory.ScopeWorkspaceID])
	_, hasUser := scope[memory.ScopeUserID]
	assert.False(t, hasUser)
}

func TestParseIntParam_Defaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?limit=abc", nil)
	assert.Equal(t, 20, parseIntParam(req, "limit", 20))

	req = httptest.NewRequest(http.MethodGet, "/?limit=-5", nil)
	assert.Equal(t, 20, parseIntParam(req, "limit", 20))

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	assert.Equal(t, 20, parseIntParam(req, "limit", 20))
}

func TestParseMinConfidence(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?min_confidence=abc", nil)
	assert.Equal(t, 0.0, parseMinConfidence(req))

	req = httptest.NewRequest(http.MethodGet, "/?min_confidence=-1", nil)
	assert.Equal(t, 0.0, parseMinConfidence(req))

	req = httptest.NewRequest(http.MethodGet, "/?min_confidence=0.5", nil)
	assert.Equal(t, 0.5, parseMinConfidence(req))

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	assert.Equal(t, 0.0, parseMinConfidence(req))
}

func TestWriteError_UnknownError(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, fmt.Errorf("something unexpected"))
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// fakeQuery implements the interface used by buildScope.
type fakeQuery map[string]string

func (f fakeQuery) Get(key string) string {
	return f[key]
}
