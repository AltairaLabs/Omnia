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
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/memory"
)

// compactionStub records calls against FindCompactionCandidates and
// SaveCompactionSummary so the handler tests can assert on forwarded
// arguments without a live Postgres.
type compactionStub struct {
	mockMemoryStore
	mu sync.Mutex

	findCalls  []memory.FindCompactionCandidatesOptions
	findResult []memory.CompactionCandidate
	findErr    error

	saveCalls []memory.CompactionSummary
	saveID    string
	saveErr   error
}

func (c *compactionStub) FindCompactionCandidates(_ context.Context, opts memory.FindCompactionCandidatesOptions) ([]memory.CompactionCandidate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.findCalls = append(c.findCalls, opts)
	if c.findErr != nil {
		return nil, c.findErr
	}
	return c.findResult, nil
}

func (c *compactionStub) SaveCompactionSummary(_ context.Context, s memory.CompactionSummary) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.saveCalls = append(c.saveCalls, s)
	if c.saveErr != nil {
		return "", c.saveErr
	}
	return c.saveID, nil
}

func newCompactionHandler(t *testing.T, store memory.Store) *http.ServeMux {
	t.Helper()
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	h := NewHandler(svc, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

// TestHandleListCompactionCandidates_HappyPath proves the route is wired,
// the default query params are applied, and the candidates survive the
// round-trip into the response JSON.
func TestHandleListCompactionCandidates_HappyPath(t *testing.T) {
	stub := &compactionStub{
		findResult: []memory.CompactionCandidate{
			{
				WorkspaceID:    "ws-1",
				UserID:         "user-1",
				AgentID:        "agent-1",
				ObservationIDs: []string{"obs-1", "obs-2"},
				Entries: []memory.CompactionEntry{
					{EntityID: "e-1", ObservationID: "obs-1", Kind: "fact", Content: "hello", ObservedAt: time.Unix(1_700_000_000, 0).UTC()},
				},
			},
		},
	}
	mux := newCompactionHandler(t, stub)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/compaction/candidates?workspace=ws-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp ListCompactionCandidatesResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Equal(t, 1, resp.Total)
	require.Len(t, resp.Candidates, 1)
	assert.Equal(t, "ws-1", resp.Candidates[0].WorkspaceID)
	assert.Equal(t, "user-1", resp.Candidates[0].UserID)
	assert.Equal(t, "agent-1", resp.Candidates[0].AgentID)
	assert.Equal(t, []string{"obs-1", "obs-2"}, resp.Candidates[0].ObservationIDs)
	require.Len(t, resp.Candidates[0].Entries, 1)
	assert.Equal(t, "hello", resp.Candidates[0].Entries[0].Content)

	require.Len(t, stub.findCalls, 1)
	got := stub.findCalls[0]
	assert.Equal(t, "ws-1", got.WorkspaceID)
	// Default 720h ago.
	assert.WithinDuration(t, time.Now().Add(-720*time.Hour), got.OlderThan, time.Minute)
	assert.Equal(t, defaultCompactionMaxCandidates, got.MaxCandidates)
	assert.Equal(t, defaultCompactionMaxPerBucket, got.MaxPerCandidate)
	assert.Equal(t, defaultCompactionMinGroupSize, got.MinGroupSize)
}

func TestHandleListCompactionCandidates_AppliesQueryParams(t *testing.T) {
	stub := &compactionStub{findResult: []memory.CompactionCandidate{}}
	mux := newCompactionHandler(t, stub)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/compaction/candidates?workspace=ws-1&older_than_hours=168&limit=5&max_per_bucket=25&min_group_size=3",
		nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, stub.findCalls, 1)
	got := stub.findCalls[0]
	assert.WithinDuration(t, time.Now().Add(-168*time.Hour), got.OlderThan, time.Minute)
	assert.Equal(t, 5, got.MaxCandidates)
	assert.Equal(t, 25, got.MaxPerCandidate)
	assert.Equal(t, 3, got.MinGroupSize)
}

func TestHandleListCompactionCandidates_ClampsLimit(t *testing.T) {
	stub := &compactionStub{findResult: []memory.CompactionCandidate{}}
	mux := newCompactionHandler(t, stub)

	// Above cap — clamped to maxCompactionMaxCandidates.
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/compaction/candidates?workspace=ws-1&limit=9999", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, stub.findCalls, 1)
	assert.Equal(t, maxCompactionMaxCandidates, stub.findCalls[0].MaxCandidates)

	// Below floor — coerced to default.
	req = httptest.NewRequest(http.MethodGet,
		"/api/v1/compaction/candidates?workspace=ws-1&limit=0", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, stub.findCalls, 2)
	assert.Equal(t, defaultCompactionMaxCandidates, stub.findCalls[1].MaxCandidates)
}

func TestHandleListCompactionCandidates_RejectsMissingWorkspace(t *testing.T) {
	mux := newCompactionHandler(t, &compactionStub{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/compaction/candidates", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleListCompactionCandidates_StoreError(t *testing.T) {
	stub := &compactionStub{findErr: errors.New("db down")}
	mux := newCompactionHandler(t, stub)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/compaction/candidates?workspace=ws-1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleSaveCompactionSummary_HappyPath(t *testing.T) {
	stub := &compactionStub{saveID: "summary-1"}
	mux := newCompactionHandler(t, stub)

	body := `{
	  "workspace_id": "ws-1",
	  "user_id": "user-1",
	  "agent_id": "agent-1",
	  "kind": "temporal_summary",
	  "content": "User prefers dark mode. Works with Kubernetes.",
	  "confidence": 0.9,
	  "superseded_observation_ids": ["obs-1", "obs-2"]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/compaction/summaries", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var resp SaveCompactionSummaryResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "summary-1", resp.SummaryEntityID)

	require.Len(t, stub.saveCalls, 1)
	got := stub.saveCalls[0]
	assert.Equal(t, "ws-1", got.WorkspaceID)
	assert.Equal(t, "user-1", got.UserID)
	assert.Equal(t, "agent-1", got.AgentID)
	assert.Equal(t, "temporal_summary", got.Kind)
	assert.Equal(t, []string{"obs-1", "obs-2"}, got.SupersededObservations)
}

func TestHandleSaveCompactionSummary_RejectsMissingWorkspace(t *testing.T) {
	mux := newCompactionHandler(t, &compactionStub{})

	body := `{"content":"x","superseded_observation_ids":["obs-1"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/compaction/summaries", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleSaveCompactionSummary_RejectsBadJSON(t *testing.T) {
	mux := newCompactionHandler(t, &compactionStub{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/compaction/summaries", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleSaveCompactionSummary_RaceReturns409(t *testing.T) {
	stub := &compactionStub{saveErr: memory.ErrCompactionRaced}
	mux := newCompactionHandler(t, stub)

	body := `{"workspace_id":"ws-1","content":"x","superseded_observation_ids":["obs-1"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/compaction/summaries", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
	var errResp ErrorResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Contains(t, errResp.Error, "superseded")
}

func TestHandleSaveCompactionSummary_StoreError(t *testing.T) {
	stub := &compactionStub{saveErr: errors.New("boom")}
	mux := newCompactionHandler(t, stub)

	body := `{"workspace_id":"ws-1","content":"x","superseded_observation_ids":["obs-1"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/compaction/summaries", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleSaveCompactionSummary_BodyTooLarge(t *testing.T) {
	stub := &compactionStub{}
	svc := NewMemoryService(stub, nil, MemoryServiceConfig{}, logr.Discard())
	h := NewHandler(svc, logr.Discard())
	h.maxBodySize = 16

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"workspace_id":"a-very-long-workspace-id-that-exceeds-16-bytes"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/compaction/summaries", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}
