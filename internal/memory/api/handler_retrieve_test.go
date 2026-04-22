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

func newRetrieveTestHandler(t *testing.T, store memory.Store) *Handler {
	t.Helper()
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	return NewHandler(svc, logr.Discard())
}

func TestHandleRetrieveMultiTier_HappyPath(t *testing.T) {
	store := &multiTierStoreStub{
		mtResult: &memory.MultiTierResult{
			Memories: []*memory.MultiTierMemory{
				{Memory: &memory.Memory{ID: "m-1", Content: "inst"}, Tier: memory.TierInstitutional, Score: 0.9},
				{Memory: &memory.Memory{ID: "m-2", Content: "user"}, Tier: memory.TierUser, Score: 0.7},
			},
			Total: 2,
		},
	}
	h := newRetrieveTestHandler(t, store)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"workspace_id":"ws-1","user_id":"u-1","agent_id":"a-1","query":"dark","limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories/retrieve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out RetrieveMultiTierResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	assert.Equal(t, 2, out.Total)
	require.Len(t, out.Memories, 2)
	assert.Equal(t, memory.TierInstitutional, out.Memories[0].Tier)

	store.mu.Lock()
	require.Len(t, store.mtCalls, 1)
	call := store.mtCalls[0]
	store.mu.Unlock()
	assert.Equal(t, "ws-1", call.WorkspaceID)
	assert.Equal(t, "u-1", call.UserID)
	assert.Equal(t, "a-1", call.AgentID)
	assert.Equal(t, "dark", call.Query)
	assert.Equal(t, 5, call.Limit)
}

func TestHandleRetrieveMultiTier_RejectsMissingWorkspace(t *testing.T) {
	store := &multiTierStoreStub{}
	h := newRetrieveTestHandler(t, store)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories/retrieve", strings.NewReader(`{"user_id":"u-1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	store.mu.Lock()
	assert.Empty(t, store.mtCalls)
	store.mu.Unlock()
}

func TestHandleRetrieveMultiTier_RejectsBadJSON(t *testing.T) {
	store := &multiTierStoreStub{}
	h := newRetrieveTestHandler(t, store)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories/retrieve", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleRetrieveMultiTier_CapsLimit(t *testing.T) {
	store := &multiTierStoreStub{
		mtResult: &memory.MultiTierResult{Memories: []*memory.MultiTierMemory{}, Total: 0},
	}
	h := newRetrieveTestHandler(t, store)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"workspace_id":"ws","limit":9999}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories/retrieve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	store.mu.Lock()
	defer store.mu.Unlock()
	require.Len(t, store.mtCalls, 1)
	assert.Equal(t, maxRetrieveLimit, store.mtCalls[0].Limit)
}

func TestHandleRetrieveMultiTier_ReturnsServiceError(t *testing.T) {
	store := &multiTierStoreStub{mtErr: assertErr{msg: "db exploded"}}
	h := newRetrieveTestHandler(t, store)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"workspace_id":"ws-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories/retrieve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	// writeError maps unknown errors to 500.
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleRetrieveMultiTier_BodyTooLarge(t *testing.T) {
	store := &multiTierStoreStub{}
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	h := NewHandler(svc, logr.Discard())
	h.maxBodySize = 16 // force a too-large error on any non-trivial body.

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"workspace_id":"ws-with-a-reasonably-long-id-that-exceeds-the-limit"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories/retrieve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

// assertErr is a local error type for service-error tests. Separate from the
// standard errors sentinel so writeError takes the default 500 branch.
type assertErr struct{ msg string }

func (a assertErr) Error() string { return a.msg }

func TestHandleRetrieveMultiTier_DefaultLimitApplied(t *testing.T) {
	store := &multiTierStoreStub{
		mtResult: &memory.MultiTierResult{Memories: []*memory.MultiTierMemory{}, Total: 0},
	}
	h := newRetrieveTestHandler(t, store)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"workspace_id":"ws"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memories/retrieve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	store.mu.Lock()
	defer store.mu.Unlock()
	require.Len(t, store.mtCalls, 1)
	assert.Equal(t, defaultRetrieveLimit, store.mtCalls[0].Limit)
}
