/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/memory"
)

// aggregateStub captures the AggregateOptions passed via the request and
// returns canned rows/err — proves the handler parses params correctly
// without needing a real Postgres or full Service plumbing.
type aggregateStub struct {
	gotOpts memory.AggregateOptions
	rows    []memory.AggregateRow
	err     error
}

// newAggregateMux returns a mux that wires the same parser+writer the real
// handler uses. Bypasses Service layer entirely so handler param parsing
// can be tested in isolation from the type-asserted PostgresMemoryStore
// requirement.
func newAggregateMux(stub *aggregateStub) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/memories/aggregate", func(w http.ResponseWriter, r *http.Request) {
		opts, err := parseAggregateOptions(r)
		if err != nil {
			writeError(w, err)
			return
		}
		stub.gotOpts = opts
		if stub.err != nil {
			writeError(w, stub.err)
			return
		}
		rows := stub.rows
		if rows == nil {
			rows = []memory.AggregateRow{}
		}
		writeJSON(w, rows)
	})
	_ = context.Background()
	return mux
}

func TestHandleMemoryAggregate_MissingWorkspace_400(t *testing.T) {
	stub := &aggregateStub{}
	h := newAggregateMux(stub)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?groupBy=category", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleMemoryAggregate_InvalidGroupBy_400(t *testing.T) {
	stub := &aggregateStub{}
	h := newAggregateMux(stub)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?workspace=ws&groupBy=banana", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleMemoryAggregate_InvalidMetric_400(t *testing.T) {
	stub := &aggregateStub{}
	h := newAggregateMux(stub)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?workspace=ws&groupBy=category&metric=banana", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleMemoryAggregate_MissingGroupBy_400(t *testing.T) {
	stub := &aggregateStub{}
	h := newAggregateMux(stub)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?workspace=ws", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleMemoryAggregate_DefaultMetric_Count(t *testing.T) {
	stub := &aggregateStub{rows: []memory.AggregateRow{{Key: "memory:context", Value: 5, Count: 5}}}
	h := newAggregateMux(stub)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?workspace=ws&groupBy=category", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, memory.AggregateMetricCount, stub.gotOpts.Metric)
	assert.Equal(t, memory.AggregateGroupByCategory, stub.gotOpts.GroupBy)
}

func TestHandleMemoryAggregate_LimitClampedAboveMax(t *testing.T) {
	stub := &aggregateStub{}
	h := newAggregateMux(stub)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?workspace=ws&groupBy=category&limit=99999", nil)
	h.ServeHTTP(httptest.NewRecorder(), r)
	assert.Equal(t, memory.MaxAggregateLimit, stub.gotOpts.Limit)
}

func TestHandleMemoryAggregate_LimitDefault_WhenMissing(t *testing.T) {
	// parseIntParam returns the default when missing. Default is
	// DefaultAggregateLimit (100); it passes through the clamp unchanged.
	stub := &aggregateStub{}
	h := newAggregateMux(stub)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?workspace=ws&groupBy=category", nil)
	h.ServeHTTP(httptest.NewRecorder(), r)
	assert.Equal(t, memory.DefaultAggregateLimit, stub.gotOpts.Limit)
}

func TestClampAggregateLimit_Floor(t *testing.T) {
	// Direct test of the clamp helper — parseIntParam doesn't surface
	// negatives, but the helper still defends against a value of 0 / negative
	// if a future caller composes differently.
	assert.Equal(t, 1, clampAggregateLimit(-5))
	assert.Equal(t, 1, clampAggregateLimit(0))
	assert.Equal(t, 50, clampAggregateLimit(50))
	assert.Equal(t, memory.MaxAggregateLimit, clampAggregateLimit(99999))
}

// TestHandleMemoryAggregate_RealHandler_BadRequest exercises the actual
// h.handleMemoryAggregate method through the production route mux. The
// service-layer call returns an error because the mockStore in handler_test.go
// is not a *PostgresMemoryStore, so AggregateMemories returns a typed error
// → handler writes 500. We assert *not 404* (route registered) + *not 200*
// (handler called and propagated the service error).
func TestHandleMemoryAggregate_RealHandler_RouteRegistered(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	mux := setupMux(h)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?workspace=ws&groupBy=category", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code == http.StatusNotFound {
		t.Fatalf("/api/v1/memories/aggregate not registered; got 404")
	}
	// mockStore is not a *PostgresMemoryStore, so the service returns an
	// error and the handler writes 500. The point of this test is purely
	// to prove the handler is reachable end-to-end via the mux.
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 from non-Postgres store, got %d (body=%q)", w.Code, w.Body.String())
	}
}

// TestHandleMemoryAggregate_RealHandler_BadGroupBy verifies that invalid
// params surface as 400 from the real handler (not just the parser shim
// the other tests use).
func TestHandleMemoryAggregate_RealHandler_BadGroupBy(t *testing.T) {
	store := &mockStore{}
	h := newTestHandler(store)
	mux := setupMux(h)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?workspace=ws&groupBy=banana", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleMemoryAggregate_HappyPath(t *testing.T) {
	stub := &aggregateStub{rows: []memory.AggregateRow{
		{Key: "memory:context", Value: 10, Count: 10},
		{Key: "memory:health", Value: 2, Count: 2},
	}}
	h := newAggregateMux(stub)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?workspace=ws&groupBy=category", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)

	var got []memory.AggregateRow
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, stub.rows, got)
}

func TestHandleMemoryAggregate_FromTo_Parsing(t *testing.T) {
	stub := &aggregateStub{}
	h := newAggregateMux(stub)
	r := httptest.NewRequest(http.MethodGet,
		"/api/v1/memories/aggregate?workspace=ws&groupBy=day&from=2026-04-01T00:00:00Z&to=2026-04-24T00:00:00Z", nil)
	h.ServeHTTP(httptest.NewRecorder(), r)
	require.NotNil(t, stub.gotOpts.From)
	require.NotNil(t, stub.gotOpts.To)
	assert.Equal(t, "2026-04-01T00:00:00Z", stub.gotOpts.From.Format("2006-01-02T15:04:05Z"))
	assert.Equal(t, "2026-04-24T00:00:00Z", stub.gotOpts.To.Format("2006-01-02T15:04:05Z"))
}

func TestHandleMemoryAggregate_BadFrom_400(t *testing.T) {
	stub := &aggregateStub{}
	h := newAggregateMux(stub)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?workspace=ws&groupBy=category&from=not-a-date", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleMemoryAggregate_BadTo_400(t *testing.T) {
	stub := &aggregateStub{}
	h := newAggregateMux(stub)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?workspace=ws&groupBy=category&to=not-a-date", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleMemoryAggregate_GroupByTier_Accepted(t *testing.T) {
	stub := &aggregateStub{rows: []memory.AggregateRow{
		{Key: "institutional", Value: 5, Count: 5},
		{Key: "agent", Value: 3, Count: 3},
		{Key: "user", Value: 12, Count: 12},
	}}
	h := newAggregateMux(stub)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?workspace=ws&groupBy=tier", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, memory.AggregateGroupByTier, stub.gotOpts.GroupBy)

	var got []memory.AggregateRow
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, stub.rows, got)
}

func TestHandleMemoryAggregate_InvalidGroupBy_MessageMentionsAllValues(t *testing.T) {
	// Locks the error body so the message stays in sync with the whitelist.
	stub := &aggregateStub{}
	h := newAggregateMux(stub)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?workspace=ws&groupBy=banana", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest, w.Code)
	body := w.Body.String()
	for _, want := range []string{"category", "agent", "day", "tier"} {
		assert.Contains(t, body, want, "error message should list %q", want)
	}
}
