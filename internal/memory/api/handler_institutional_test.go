/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/memory"
)

func newInstitutionalHandler(t *testing.T, store memory.Store) *http.ServeMux {
	t.Helper()
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	h := NewHandler(svc, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestHandleSaveInstitutional_HappyPath(t *testing.T) {
	stub := &institutionalStub{saveMemID: "inst-1"}
	mux := newInstitutionalHandler(t, stub)

	body := `{"workspace_id":"ws-1","type":"policy","content":"snake_case rule","confidence":1.0}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/institutional/memories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var resp MemoryResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "inst-1", resp.Memory.ID)
	assert.Equal(t, "ws-1", resp.Memory.Scope[memory.ScopeWorkspaceID])
}

func TestHandleSaveInstitutional_RejectsMissingWorkspace(t *testing.T) {
	mux := newInstitutionalHandler(t, &institutionalStub{})

	body := `{"type":"policy","content":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/institutional/memories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleSaveInstitutional_RejectsBadJSON(t *testing.T) {
	mux := newInstitutionalHandler(t, &institutionalStub{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/institutional/memories", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleSaveInstitutional_BodyTooLarge(t *testing.T) {
	stub := &institutionalStub{}
	svc := NewMemoryService(stub, nil, MemoryServiceConfig{}, logr.Discard())
	h := NewHandler(svc, logr.Discard())
	h.maxBodySize = 16

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"workspace_id":"a-workspace-that-exceeds-16-bytes"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/institutional/memories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestHandleListInstitutional_HappyPath(t *testing.T) {
	stub := &institutionalStub{
		listResult: []*memory.Memory{{ID: "m-1"}, {ID: "m-2"}},
	}
	mux := newInstitutionalHandler(t, stub)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/institutional/memories?workspace=ws-1", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var out ListInstitutionalResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	assert.Equal(t, 2, out.Total)
	assert.Len(t, out.Memories, 2)
}

func TestHandleListInstitutional_RejectsMissingWorkspace(t *testing.T) {
	mux := newInstitutionalHandler(t, &institutionalStub{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/institutional/memories", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleListInstitutional_CapsLimit(t *testing.T) {
	stub := &institutionalStub{listResult: []*memory.Memory{}}
	mux := newInstitutionalHandler(t, stub)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/institutional/memories?workspace=ws-1&limit=99999", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleDeleteInstitutional_HappyPath(t *testing.T) {
	mux := newInstitutionalHandler(t, &institutionalStub{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/institutional/memories/m-1?workspace=ws-1", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleDeleteInstitutional_RejectsMissingWorkspace(t *testing.T) {
	mux := newInstitutionalHandler(t, &institutionalStub{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/institutional/memories/m-1", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleDeleteInstitutional_NonInstitutionalReturns400(t *testing.T) {
	stub := &institutionalStub{deleteErr: memory.ErrNotInstitutional}
	mux := newInstitutionalHandler(t, stub)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/institutional/memories/m-1?workspace=ws-1", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var errResp ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Contains(t, errResp.Error, "not an institutional")
}

func TestHandleListInstitutional_ServiceError(t *testing.T) {
	stub := &institutionalStub{listErr: assertErr{msg: "db down"}}
	mux := newInstitutionalHandler(t, stub)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/institutional/memories?workspace=ws-1", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleListInstitutional_NegativeLimitNormalizes(t *testing.T) {
	stub := &institutionalStub{listResult: []*memory.Memory{}}
	mux := newInstitutionalHandler(t, stub)

	// Negative limit defaults to defaultListLimit via parseIntParam, then
	// goes through the >=1 clamp — exercises the "limit < 1" branch.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/institutional/memories?workspace=ws-1&limit=0", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleDeleteInstitutional_MissingID(t *testing.T) {
	mux := newInstitutionalHandler(t, &institutionalStub{})

	// No trailing ID — go's router makes this path 404. Assert that explicitly.
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/institutional/memories/?workspace=ws-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// 404 because the {id} path-variable requires at least one char.
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleDeleteInstitutional_ServiceError(t *testing.T) {
	stub := &institutionalStub{deleteErr: assertErr{msg: "boom"}}
	mux := newInstitutionalHandler(t, stub)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/institutional/memories/m-1?workspace=ws-1", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandler_RegisterRoutes_IncludesInstitutional(t *testing.T) {
	mux := newInstitutionalHandler(t, &institutionalStub{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/institutional/memories?workspace=ws-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Fatalf("GET /api/v1/institutional/memories not routed (404); routes not registered")
	}
}
