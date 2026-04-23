/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/memory"
)

// agentScopedStub tracks calls against the agent-scoped admin path so the
// handler tests can assert on forwarded values without standing up Postgres.
type agentScopedStub struct {
	mockMemoryStore
	mu sync.Mutex

	saveCalls []*memory.Memory
	saveErr   error
	saveMemID string

	listCalls  []struct{ ws, agent string }
	listResult []*memory.Memory
	listErr    error

	deleteCalls []struct{ ws, agent, id string }
	deleteErr   error
}

func (a *agentScopedStub) SaveAgentScoped(_ context.Context, mem *memory.Memory) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.saveCalls = append(a.saveCalls, mem)
	if a.saveErr != nil {
		return a.saveErr
	}
	if a.saveMemID != "" {
		mem.ID = a.saveMemID
	}
	return nil
}

func (a *agentScopedStub) ListAgentScoped(_ context.Context, ws, agent string, _ memory.ListOptions) ([]*memory.Memory, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.listCalls = append(a.listCalls, struct{ ws, agent string }{ws, agent})
	if a.listErr != nil {
		return nil, a.listErr
	}
	return a.listResult, nil
}

func (a *agentScopedStub) DeleteAgentScoped(_ context.Context, ws, agent, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.deleteCalls = append(a.deleteCalls, struct{ ws, agent, id string }{ws, agent, id})
	return a.deleteErr
}

func newAgentScopedHandler(t *testing.T, store memory.Store) *http.ServeMux {
	t.Helper()
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	h := NewHandler(svc, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestHandleSaveAgentScoped_HappyPath(t *testing.T) {
	stub := &agentScopedStub{saveMemID: "agent-mem-1"}
	mux := newAgentScopedHandler(t, stub)

	body := `{"workspace_id":"ws-1","agent_id":"agent-1","type":"policy","content":"always cite sources","confidence":1.0}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-memories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var resp MemoryResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "agent-mem-1", resp.Memory.ID)
	assert.Equal(t, "ws-1", resp.Memory.Scope[memory.ScopeWorkspaceID])
	assert.Equal(t, "agent-1", resp.Memory.Scope[memory.ScopeAgentID])
}

func TestHandleSaveAgentScoped_RejectsMissingWorkspace(t *testing.T) {
	mux := newAgentScopedHandler(t, &agentScopedStub{})

	body := `{"agent_id":"agent-1","type":"policy","content":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-memories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleSaveAgentScoped_RejectsMissingAgent(t *testing.T) {
	stub := &agentScopedStub{}
	mux := newAgentScopedHandler(t, stub)

	body := `{"workspace_id":"ws-1","type":"policy","content":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-memories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, stub.saveCalls, "store should NOT be called when agent_id is missing")
}

func TestHandleSaveAgentScoped_RejectsBadJSON(t *testing.T) {
	mux := newAgentScopedHandler(t, &agentScopedStub{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-memories", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleListAgentScoped_HappyPath(t *testing.T) {
	stub := &agentScopedStub{
		listResult: []*memory.Memory{{ID: "m-1"}, {ID: "m-2"}},
	}
	mux := newAgentScopedHandler(t, stub)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/agent-memories?workspace=ws-1&agent=agent-1", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var out ListAgentScopedResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	assert.Equal(t, 2, out.Total)
	assert.Len(t, out.Memories, 2)
	require.Len(t, stub.listCalls, 1)
	assert.Equal(t, "ws-1", stub.listCalls[0].ws)
	assert.Equal(t, "agent-1", stub.listCalls[0].agent)
}

func TestHandleListAgentScoped_RejectsMissingWorkspace(t *testing.T) {
	mux := newAgentScopedHandler(t, &agentScopedStub{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent-memories?agent=agent-1", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleListAgentScoped_RejectsMissingAgent(t *testing.T) {
	mux := newAgentScopedHandler(t, &agentScopedStub{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent-memories?workspace=ws-1", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleListAgentScoped_ServiceError(t *testing.T) {
	stub := &agentScopedStub{listErr: errors.New("db down")}
	mux := newAgentScopedHandler(t, stub)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/agent-memories?workspace=ws-1&agent=agent-1", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleListAgentScoped_ClampsLimit(t *testing.T) {
	stub := &agentScopedStub{listResult: []*memory.Memory{}}
	mux := newAgentScopedHandler(t, stub)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/agent-memories?workspace=ws-1&agent=agent-1&limit=0", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	req = httptest.NewRequest(http.MethodGet,
		"/api/v1/agent-memories?workspace=ws-1&agent=agent-1&limit=99999", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleDeleteAgentScoped_HappyPath(t *testing.T) {
	stub := &agentScopedStub{}
	mux := newAgentScopedHandler(t, stub)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/agent-memories/m-1?workspace=ws-1&agent=agent-1", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, stub.deleteCalls, 1)
	assert.Equal(t, "m-1", stub.deleteCalls[0].id)
	assert.Equal(t, "ws-1", stub.deleteCalls[0].ws)
	assert.Equal(t, "agent-1", stub.deleteCalls[0].agent)
}

func TestHandleDeleteAgentScoped_RejectsMissingWorkspace(t *testing.T) {
	mux := newAgentScopedHandler(t, &agentScopedStub{})

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/agent-memories/m-1?agent=agent-1", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleDeleteAgentScoped_RejectsMissingAgent(t *testing.T) {
	mux := newAgentScopedHandler(t, &agentScopedStub{})

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/agent-memories/m-1?workspace=ws-1", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleDeleteAgentScoped_NotAgentScopedReturns400(t *testing.T) {
	stub := &agentScopedStub{deleteErr: memory.ErrNotAgentScoped}
	mux := newAgentScopedHandler(t, stub)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/agent-memories/m-1?workspace=ws-1&agent=agent-1", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var errResp ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Contains(t, errResp.Error, "not an agent-scoped")
}

func TestHandleDeleteAgentScoped_ServiceError(t *testing.T) {
	stub := &agentScopedStub{deleteErr: errors.New("boom")}
	mux := newAgentScopedHandler(t, stub)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/agent-memories/m-1?workspace=ws-1&agent=agent-1", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleDeleteAgentScoped_MissingIDReturns404(t *testing.T) {
	mux := newAgentScopedHandler(t, &agentScopedStub{})

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/agent-memories/?workspace=ws-1&agent=agent-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
