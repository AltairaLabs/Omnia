/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

func newTestEvalHandler(store EvalStore) *http.ServeMux {
	svc := NewEvalService(store, logr.Discard())
	h := NewHandler(nil, logr.Discard())
	h.SetEvalService(svc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestHandleGetSessionEvalResults(t *testing.T) {
	score := 0.95
	store := &mockEvalStore{
		getResults: []*EvalResult{
			{ID: "r1", SessionID: "b0fda631-4057-4ba6-844c-3b4a6fe192dc", EvalID: "e1", Passed: true, Score: &score},
		},
	}
	mux := newTestEvalHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/b0fda631-4057-4ba6-844c-3b4a6fe192dc/eval-results", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp EvalResultSessionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Results, 1)
	assert.Equal(t, "e1", resp.Results[0].EvalID)
	assert.True(t, resp.Results[0].Passed)
}

func TestHandleGetSessionEvalResults_NoEvalService(t *testing.T) {
	h := NewHandler(nil, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/b0fda631-4057-4ba6-844c-3b4a6fe192dc/eval-results", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleGetSessionEvalResults_InvalidSessionID(t *testing.T) {
	store := &mockEvalStore{}
	mux := newTestEvalHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/not-a-uuid/eval-results", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleGetSessionEvalResults_StoreError(t *testing.T) {
	store := &mockEvalStore{getErr: fmt.Errorf("db error")}
	mux := newTestEvalHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/b0fda631-4057-4ba6-844c-3b4a6fe192dc/eval-results", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleCreateEvalResults(t *testing.T) {
	store := &mockEvalStore{}
	mux := newTestEvalHandler(store)

	results := []*EvalResult{
		{SessionID: "s1", EvalID: "e1", Passed: true, Source: "eval-worker"},
	}
	body, _ := json.Marshal(results)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/eval-results", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestHandleCreateEvalResults_InvalidJSON(t *testing.T) {
	store := &mockEvalStore{}
	mux := newTestEvalHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/eval-results", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleCreateEvalResults_EmptyResults(t *testing.T) {
	store := &mockEvalStore{}
	mux := newTestEvalHandler(store)

	body, _ := json.Marshal([]*EvalResult{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/eval-results", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleCreateEvalResults_NoEvalService(t *testing.T) {
	h := NewHandler(nil, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal([]*EvalResult{{EvalID: "e1"}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/eval-results", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleListEvalResults(t *testing.T) {
	store := &mockEvalStore{
		listResults: []*EvalResult{
			{ID: "r1", EvalID: "e1", Passed: true},
			{ID: "r2", EvalID: "e2", Passed: false},
		},
		listTotal: 2,
	}
	mux := newTestEvalHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/eval-results?limit=10&agentName=test", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp EvalResultListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Results, 2)
	assert.Equal(t, int64(2), resp.Total)
	assert.False(t, resp.HasMore)
}

func TestHandleListEvalResults_HasMore(t *testing.T) {
	store := &mockEvalStore{
		listResults: []*EvalResult{{ID: "r1"}},
		listTotal:   50,
	}
	mux := newTestEvalHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/eval-results?limit=10", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp EvalResultListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.HasMore)
}

func TestParseEvalListOpts(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/eval-results?limit=50&offset=10&passed=true&agentName=bot&evalType=contains", nil)
	opts := parseEvalListOpts(req)

	assert.Equal(t, 50, opts.Limit)
	assert.Equal(t, 10, opts.Offset)
	require.NotNil(t, opts.Passed)
	assert.True(t, *opts.Passed)
	assert.Equal(t, "bot", opts.AgentName)
	assert.Equal(t, "contains", opts.EvalType)
}

func TestParseEvalListOpts_Defaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/eval-results", nil)
	opts := parseEvalListOpts(req)

	assert.Equal(t, defaultListLimit, opts.Limit)
	assert.Equal(t, 0, opts.Offset)
	assert.Nil(t, opts.Passed)
}

func TestParseEvalListOpts_LimitCapped(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/eval-results?limit=999", nil)
	opts := parseEvalListOpts(req)

	assert.Equal(t, maxListLimit, opts.Limit)
}

func TestHandleEvaluateSession_Success(t *testing.T) {
	pub := &mockEventPublisher{}
	warm := newMockWarmStore()
	warm.sessions["b0fda631-4057-4ba6-844c-3b4a6fe192dc"] = &session.Session{
		ID:        "b0fda631-4057-4ba6-844c-3b4a6fe192dc",
		AgentName: "test-agent",
		Namespace: "test-ns",
	}
	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)

	svc := newServiceWithPublisher(registry, pub)
	h := NewHandler(svc, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/b0fda631-4057-4ba6-844c-3b4a6fe192dc/evaluate", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var resp EvaluateAcceptedResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "b0fda631-4057-4ba6-844c-3b4a6fe192dc", resp.SessionID)
	assert.Equal(t, "evaluation queued", resp.Message)

	// Verify event was published synchronously.
	events := pub.getEvents()
	require.Len(t, events, 1)
	assert.Equal(t, "session.evaluate", events[0].EventType)
	assert.Equal(t, "b0fda631-4057-4ba6-844c-3b4a6fe192dc", events[0].SessionID)
	assert.Equal(t, "test-agent", events[0].AgentName)
	assert.Equal(t, "test-ns", events[0].Namespace)
}

func TestHandleEvaluateSession_InvalidSessionID(t *testing.T) {
	h := NewHandler(nil, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/not-a-uuid/evaluate", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleEvaluateSession_NoService(t *testing.T) {
	h := NewHandler(nil, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/b0fda631-4057-4ba6-844c-3b4a6fe192dc/evaluate", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
