/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// fakeEmbStore is a programmable EmbeddingMetricsStore.
type fakeEmbStore struct {
	workspaces []string
	wsErr      error
	coverage   map[string][2]int // ws -> {total, embedded}
	covErr     map[string]error
	backlog    map[string]int
	backErr    map[string]error
}

func (f *fakeEmbStore) ListWorkspaceIDs(context.Context) ([]string, error) {
	return f.workspaces, f.wsErr
}

func (f *fakeEmbStore) EmbeddingCoverage(_ context.Context, ws string) (int, int, error) {
	if err := f.covErr[ws]; err != nil {
		return 0, 0, err
	}
	tc := f.coverage[ws]
	return tc[0], tc[1], nil
}

func (f *fakeEmbStore) CountObservationsMissingEmbedding(_ context.Context, ws, _ string) (int, error) {
	if err := f.backErr[ws]; err != nil {
		return 0, err
	}
	return f.backlog[ws], nil
}

func newCollector(store EmbeddingMetricsStore, m *EmbeddingMetrics) *EmbeddingMetricsCollector {
	return NewEmbeddingMetricsCollector(store, m, "model-x", time.Minute, logr.Discard())
}

func TestCollectOnce_SetsCoverageAndBacklog(t *testing.T) {
	store := &fakeEmbStore{
		workspaces: []string{"ws-a", "ws-b"},
		coverage:   map[string][2]int{"ws-a": {10, 7}, "ws-b": {4, 4}},
		backlog:    map[string]int{"ws-a": 3, "ws-b": 0},
	}
	m := NewEmbeddingMetrics()
	newCollector(store, m).collectOnce(context.Background())

	if got := testutil.ToFloat64(m.Coverage.WithLabelValues("ws-a")); got != 0.7 {
		t.Errorf("ws-a coverage = %v, want 0.7", got)
	}
	if got := testutil.ToFloat64(m.Coverage.WithLabelValues("ws-b")); got != 1.0 {
		t.Errorf("ws-b coverage = %v, want 1.0", got)
	}
	if got := testutil.ToFloat64(m.Backlog.WithLabelValues("ws-a")); got != 3 {
		t.Errorf("ws-a backlog = %v, want 3", got)
	}
	if got := testutil.ToFloat64(m.Backlog.WithLabelValues("ws-b")); got != 0 {
		t.Errorf("ws-b backlog = %v, want 0", got)
	}
}

func TestCollectOnce_SkipsCoverageForEmptyWorkspace(t *testing.T) {
	// A workspace with zero live entities (total=0) must not emit a 0/0 NaN
	// coverage series, but its backlog (0) is still reported.
	store := &fakeEmbStore{
		workspaces: []string{"ws-empty"},
		coverage:   map[string][2]int{"ws-empty": {0, 0}},
		backlog:    map[string]int{"ws-empty": 0},
	}
	m := NewEmbeddingMetrics()
	newCollector(store, m).collectOnce(context.Background())

	if n := testutil.CollectAndCount(m.Coverage); n != 0 {
		t.Errorf("coverage series count = %d, want 0 for empty workspace", n)
	}
	if got := testutil.ToFloat64(m.Backlog.WithLabelValues("ws-empty")); got != 0 {
		t.Errorf("backlog = %v, want 0", got)
	}
}

func TestCollectOnce_ResetsStaleSeries(t *testing.T) {
	store := &fakeEmbStore{
		workspaces: []string{"ws-a", "ws-b"},
		coverage:   map[string][2]int{"ws-a": {10, 5}, "ws-b": {2, 2}},
		backlog:    map[string]int{"ws-a": 1, "ws-b": 1},
	}
	m := NewEmbeddingMetrics()
	c := newCollector(store, m)
	c.collectOnce(context.Background())

	// ws-b disappears (deleted). Its series must not linger at the old value.
	store.workspaces = []string{"ws-a"}
	c.collectOnce(context.Background())

	if n := testutil.CollectAndCount(m.Coverage); n != 1 {
		t.Errorf("coverage series count = %d, want 1 after ws-b removed", n)
	}
	if got := testutil.ToFloat64(m.Coverage.WithLabelValues("ws-a")); got != 0.5 {
		t.Errorf("ws-a coverage = %v, want 0.5", got)
	}
}

func TestCollectOnce_ListErrorLeavesGaugesUntouched(t *testing.T) {
	store := &fakeEmbStore{
		workspaces: []string{"ws-a"},
		coverage:   map[string][2]int{"ws-a": {4, 2}},
		backlog:    map[string]int{"ws-a": 5},
	}
	m := NewEmbeddingMetrics()
	c := newCollector(store, m)
	c.collectOnce(context.Background())

	// A transient list failure must NOT wipe the last-known gauges.
	store.wsErr = errors.New("db down")
	c.collectOnce(context.Background())

	if got := testutil.ToFloat64(m.Coverage.WithLabelValues("ws-a")); got != 0.5 {
		t.Errorf("coverage after list error = %v, want preserved 0.5", got)
	}
}

func TestCollectWorkspace_CoverageErrorSkipsBacklog(t *testing.T) {
	store := &fakeEmbStore{
		workspaces: []string{"ws-a"},
		covErr:     map[string]error{"ws-a": errors.New("query failed")},
		backlog:    map[string]int{"ws-a": 9},
	}
	m := NewEmbeddingMetrics()
	newCollector(store, m).collectOnce(context.Background())

	if n := testutil.CollectAndCount(m.Coverage); n != 0 {
		t.Errorf("coverage series = %d, want 0 on query error", n)
	}
	if n := testutil.CollectAndCount(m.Backlog); n != 0 {
		t.Errorf("backlog series = %d, want 0 (skipped after coverage error)", n)
	}
}

func TestCollectWorkspace_BacklogErrorStillSetsCoverage(t *testing.T) {
	store := &fakeEmbStore{
		workspaces: []string{"ws-a"},
		coverage:   map[string][2]int{"ws-a": {4, 1}},
		backErr:    map[string]error{"ws-a": errors.New("query failed")},
	}
	m := NewEmbeddingMetrics()
	newCollector(store, m).collectOnce(context.Background())

	if got := testutil.ToFloat64(m.Coverage.WithLabelValues("ws-a")); got != 0.25 {
		t.Errorf("coverage = %v, want 0.25 despite backlog error", got)
	}
	if n := testutil.CollectAndCount(m.Backlog); n != 0 {
		t.Errorf("backlog series = %d, want 0 on backlog error", n)
	}
}

func TestRun_InitialCollectThenStopsOnCancel(t *testing.T) {
	store := &fakeEmbStore{
		workspaces: []string{"ws-a"},
		coverage:   map[string][2]int{"ws-a": {2, 1}},
		backlog:    map[string]int{"ws-a": 0},
	}
	m := NewEmbeddingMetrics()
	c := NewEmbeddingMetricsCollector(store, m, "model-x", time.Hour, logr.Discard())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()

	// Initial pass runs synchronously before the ticker; cancel and ensure Run returns.
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
	if got := testutil.ToFloat64(m.Coverage.WithLabelValues("ws-a")); got != 0.5 {
		t.Errorf("coverage after initial pass = %v, want 0.5", got)
	}
}
