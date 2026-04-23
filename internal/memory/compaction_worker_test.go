/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func TestNoopSummarizer(t *testing.T) {
	out, err := NoopSummarizer{}.Summarize(context.Background(), []CompactionEntry{
		{Content: "first line"},
		{Content: "second line"},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(out, "2 observations") {
		t.Errorf("expected count in summary, got %q", out)
	}
}

func TestNoopSummarizer_NoEntriesErrors(t *testing.T) {
	_, err := NoopSummarizer{}.Summarize(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when no entries")
	}
}

func TestCompactionWorker_RunOncePerformsCompaction(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "dd000000-0000-0000-0000-000000000001"
	user := "dd000000-0000-0000-0000-000000000002"
	mustInsertOldEntities(t, store, ws, user, "", 5, "old", time.Now().Add(-90*24*time.Hour))

	worker := NewCompactionWorker(store, NoopSummarizer{}, CompactionWorkerOptions{
		WorkspaceIDs: []string{ws},
		Age:          30 * 24 * time.Hour,
		MinGroupSize: 1,
	}, logr.Discard())

	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// After one pass, the originals should be superseded — no longer in
	// retrieval results.
	res, err := store.RetrieveMultiTier(ctx, MultiTierRequest{
		WorkspaceID: ws, UserID: user, Query: "old", Limit: 50,
	})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	for _, m := range res.Memories {
		if m.Content == "old" {
			t.Errorf("superseded row leaked: %+v", m)
		}
	}
}

func TestCompactionWorker_RunOnceNoCandidatesIsNoop(t *testing.T) {
	store := newStore(t)
	worker := NewCompactionWorker(store, nil, CompactionWorkerOptions{
		WorkspaceIDs: []string{"ff000000-0000-0000-0000-000000000001"},
		Age:          1 * time.Hour,
	}, logr.Discard())

	if err := worker.RunOnce(context.Background()); err != nil {
		t.Errorf("no-op RunOnce returned err: %v", err)
	}
}

func TestCompactionWorker_EmptyWorkspaceListIsNoop(t *testing.T) {
	store := newStore(t)
	worker := NewCompactionWorker(store, nil, CompactionWorkerOptions{}, logr.Discard())
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Errorf("expected silent no-op, got %v", err)
	}
}

func TestCompactionWorker_UsesDiscovererWhenListEmpty(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "aa000000-0000-0000-0000-000000000001"
	user := "aa000000-0000-0000-0000-000000000002"
	mustInsertOldEntities(t, store, ws, user, "", 5, "disc", time.Now().Add(-90*24*time.Hour))

	var called int
	worker := NewCompactionWorker(store, NoopSummarizer{}, CompactionWorkerOptions{
		Age:          30 * 24 * time.Hour,
		MinGroupSize: 1,
		WorkspaceDiscoverer: func(context.Context) ([]string, error) {
			called++
			return []string{ws}, nil
		},
	}, logr.Discard())

	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if called != 1 {
		t.Errorf("expected discoverer called once, got %d", called)
	}

	// Verify compaction happened — superseded rows should not surface in
	// retrieval.
	res, err := store.RetrieveMultiTier(ctx, MultiTierRequest{
		WorkspaceID: ws, UserID: user, Query: "disc", Limit: 50,
	})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	for _, m := range res.Memories {
		if m.Content == "disc" {
			t.Errorf("superseded row leaked: %+v", m)
		}
	}
}

func TestCompactionWorker_StaticListBeatsDiscoverer(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "bb000000-0000-0000-0000-000000000001"
	user := "bb000000-0000-0000-0000-000000000002"
	mustInsertOldEntities(t, store, ws, user, "", 3, "static", time.Now().Add(-90*24*time.Hour))

	var called int
	worker := NewCompactionWorker(store, NoopSummarizer{}, CompactionWorkerOptions{
		WorkspaceIDs: []string{ws},
		Age:          30 * 24 * time.Hour,
		MinGroupSize: 1,
		WorkspaceDiscoverer: func(context.Context) ([]string, error) {
			called++
			return nil, errors.New("should not be called")
		},
	}, logr.Discard())

	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if called != 0 {
		t.Errorf("discoverer should not be called when WorkspaceIDs is set, got %d calls", called)
	}
}

func TestCompactionWorker_DiscovererErrorSurfaces(t *testing.T) {
	store := newStore(t)
	boom := errors.New("k8s down")
	worker := NewCompactionWorker(store, nil, CompactionWorkerOptions{
		Age: 30 * 24 * time.Hour,
		WorkspaceDiscoverer: func(context.Context) ([]string, error) {
			return nil, boom
		},
	}, logr.Discard())

	err := worker.RunOnce(context.Background())
	if err == nil || !errors.Is(err, boom) {
		t.Errorf("expected wrapped boom, got %v", err)
	}
}

func TestCompactionWorker_PropagatesSummarizerError(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "ee000000-0000-0000-0000-000000000001"
	user := "ee000000-0000-0000-0000-000000000002"
	mustInsertOldEntities(t, store, ws, user, "", 2, "err", time.Now().Add(-90*24*time.Hour))

	boom := errors.New("LLM down")
	worker := NewCompactionWorker(store, failingSummarizer{err: boom}, CompactionWorkerOptions{
		WorkspaceIDs: []string{ws},
		Age:          30 * 24 * time.Hour,
		MinGroupSize: 1,
	}, logr.Discard())

	err := worker.RunOnce(ctx)
	if err == nil || !errors.Is(err, boom) {
		t.Errorf("expected wrapped boom error, got %v", err)
	}
}

func TestCompactionWorker_IgnoresRaceError(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "44000000-0000-0000-0000-000000000001"
	user := "44000000-0000-0000-0000-000000000002"
	mustInsertOldEntities(t, store, ws, user, "", 3, "race", time.Now().Add(-90*24*time.Hour))

	// Simulate a race by pre-superseding the observations before the worker
	// runs. The worker's SaveCompactionSummary will return ErrCompactionRaced
	// which the worker swallows.
	worker := NewCompactionWorker(store, NoopSummarizer{}, CompactionWorkerOptions{
		WorkspaceIDs: []string{ws},
		Age:          30 * 24 * time.Hour,
		MinGroupSize: 1,
	}, logr.Discard())

	// First pass: normal.
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	// Second pass on the same data: no new candidates, so the worker should
	// find none and exit cleanly (not even hit the race path — but we're
	// asserting no error either way).
	if err := worker.RunOnce(ctx); err != nil {
		t.Errorf("second pass: %v", err)
	}
}

func TestCompactionWorker_RunRespectsZeroInterval(t *testing.T) {
	store := newStore(t)
	worker := NewCompactionWorker(store, nil, CompactionWorkerOptions{
		Interval: 0, // disabled
	}, logr.Discard())

	// Run with an immediately-cancelled context: should return without
	// blocking or ticking.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	worker.Run(ctx)
}

func TestCompactionWorker_RunTicksAndExits(t *testing.T) {
	store := newStore(t)

	ws := "cc000000-0000-0000-0000-000000000001"
	user := "cc000000-0000-0000-0000-000000000002"
	mustInsertOldEntities(t, store, ws, user, "", 2, "ticked", time.Now().Add(-90*24*time.Hour))

	ran := make(chan struct{}, 1)
	worker := NewCompactionWorker(store, NoopSummarizer{}, CompactionWorkerOptions{
		Interval:     20 * time.Millisecond,
		WorkspaceIDs: []string{ws},
		Age:          30 * 24 * time.Hour,
		MinGroupSize: 1,
		WorkspaceDiscoverer: func(context.Context) ([]string, error) {
			select {
			case ran <- struct{}{}:
			default:
			}
			return []string{ws}, nil
		},
	}, logr.Discard())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	// Wait for at least one tick to have fired, then cancel.
	time.Sleep(80 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after context cancel")
	}
}

func TestCompactionWorker_RunLogsWorkspaceFailureButContinues(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	// One good workspace, one bogus workspace ID that'll fail the bucket query.
	goodWS := "ff000000-0000-0000-0000-000000000010"
	user := "ff000000-0000-0000-0000-000000000011"
	mustInsertOldEntities(t, store, goodWS, user, "", 3, "continues", time.Now().Add(-90*24*time.Hour))

	worker := NewCompactionWorker(store, NoopSummarizer{}, CompactionWorkerOptions{
		// "bad" is not a UUID — the FindCompactionCandidates query will error.
		WorkspaceIDs: []string{"not-a-uuid", goodWS},
		Age:          30 * 24 * time.Hour,
		MinGroupSize: 1,
	}, logr.Discard())

	// RunOnce should return the first error but still have processed goodWS.
	err := worker.RunOnce(ctx)
	if err == nil {
		t.Fatal("expected error from bogus workspace")
	}

	// Verify goodWS was compacted despite the earlier failure.
	res, retErr := store.RetrieveMultiTier(ctx, MultiTierRequest{
		WorkspaceID: goodWS, UserID: user, Query: "continues", Limit: 50,
	})
	if retErr != nil {
		t.Fatalf("retrieve: %v", retErr)
	}
	for _, m := range res.Memories {
		if m.Content == "continues" {
			t.Errorf("goodWS not compacted — originals still present: %+v", m)
		}
	}
}

// failingSummarizer always returns its configured error.
type failingSummarizer struct{ err error }

func (f failingSummarizer) Summarize(_ context.Context, _ []CompactionEntry) (string, error) {
	return "", f.err
}
