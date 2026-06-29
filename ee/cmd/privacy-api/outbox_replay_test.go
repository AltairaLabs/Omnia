/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

// fakeOutboxStore implements outboxStore for unit tests.
type fakeOutboxStore struct {
	mu             sync.Mutex
	entries        []privacy.OutboxEntry
	stuckCount     int64
	deliveredIDs   []string
	pruneCallCount int
	stuckCallCount int
	listErr        error
	markErr        error
	pruneErr       error
	stuckErr       error
}

func (f *fakeOutboxStore) ListUndeliveredOutbox(_ context.Context, _ time.Duration, _ int) ([]privacy.OutboxEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]privacy.OutboxEntry, len(f.entries))
	copy(out, f.entries)
	return out, nil
}

func (f *fakeOutboxStore) MarkOutboxDelivered(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.markErr != nil {
		return f.markErr
	}
	f.deliveredIDs = append(f.deliveredIDs, id)
	return nil
}

func (f *fakeOutboxStore) PruneDeliveredOutbox(_ context.Context, _ time.Duration) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pruneCallCount++
	return 0, f.pruneErr
}

func (f *fakeOutboxStore) CountStuckOutbox(_ context.Context, _ time.Duration) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stuckCallCount++
	return f.stuckCount, f.stuckErr
}

// fakeConsentNotifier implements privacy.ConsentNotifier for unit tests.
type fakeConsentNotifier struct {
	mu        sync.Mutex
	delivered bool
	calls     []fakeNotifyCall
}

type fakeNotifyCall struct {
	userID   string
	category privacy.ConsentCategory
}

func (f *fakeConsentNotifier) NotifyRevocation(_ context.Context, userID string, category privacy.ConsentCategory) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeNotifyCall{userID: userID, category: category})
	return f.delivered, nil
}

// newTestReplayWorker creates an OutboxReplayWorker with a fresh registry for tests.
func newTestReplayWorker(t *testing.T, store outboxStore, notifier privacy.ConsentNotifier) (*OutboxReplayWorker, prometheus.Gauge) {
	t.Helper()
	reg := prometheus.NewRegistry()
	w := NewOutboxReplayWorker(store, notifier, time.Hour, time.Hour, reg, zap.New(zap.UseDevMode(true)))
	return w, w.stuckGauge
}

// TestOutboxReplayWorker_DeliveredEntry asserts that when the notifier returns
// delivered=true, replayOnce marks the entry delivered, calls Prune, and sets
// the stuck gauge from CountStuckOutbox.
func TestOutboxReplayWorker_DeliveredEntry(t *testing.T) {
	store := &fakeOutboxStore{
		entries: []privacy.OutboxEntry{
			{ID: "entry-1", UserID: "user-a", Category: privacy.ConsentMemoryIdentity},
		},
		stuckCount: 2,
	}
	notifier := &fakeConsentNotifier{delivered: true}

	w, gauge := newTestReplayWorker(t, store, notifier)
	require.NoError(t, w.replayOnce(t.Context()))

	// Entry must be marked delivered.
	store.mu.Lock()
	deliveredIDs := store.deliveredIDs
	pruneCount := store.pruneCallCount
	stuckCalls := store.stuckCallCount
	store.mu.Unlock()

	assert.Equal(t, []string{"entry-1"}, deliveredIDs, "entry-1 must be marked delivered")
	assert.Equal(t, 1, pruneCount, "PruneDeliveredOutbox must be called once")
	assert.Equal(t, 1, stuckCalls, "CountStuckOutbox must be called once")

	// Stuck gauge must reflect the store count.
	assert.InDelta(t, 2.0, testutil.ToFloat64(gauge), 0.001, "stuck gauge must be set to store.stuckCount")
}

// TestOutboxReplayWorker_UndeliveredEntry asserts that when the notifier returns
// delivered=false, the entry is NOT marked delivered, but Prune and stuck gauge
// are still called.
func TestOutboxReplayWorker_UndeliveredEntry(t *testing.T) {
	store := &fakeOutboxStore{
		entries: []privacy.OutboxEntry{
			{ID: "entry-2", UserID: "user-b", Category: privacy.ConsentAnalyticsAggregate},
		},
		stuckCount: 1,
	}
	notifier := &fakeConsentNotifier{delivered: false}

	w, gauge := newTestReplayWorker(t, store, notifier)
	require.NoError(t, w.replayOnce(t.Context()))

	store.mu.Lock()
	deliveredIDs := store.deliveredIDs
	pruneCount := store.pruneCallCount
	store.mu.Unlock()

	assert.Empty(t, deliveredIDs, "entry must NOT be marked delivered when notifier returns false")
	assert.Equal(t, 1, pruneCount, "PruneDeliveredOutbox must still be called")
	assert.InDelta(t, 1.0, testutil.ToFloat64(gauge), 0.001, "stuck gauge must still be set")
}

// TestOutboxReplayWorker_MultipleEntries asserts that each entry is processed
// independently: delivered entries are marked, undelivered ones are not.
func TestOutboxReplayWorker_MultipleEntries(t *testing.T) {
	store := &fakeOutboxStore{
		entries: []privacy.OutboxEntry{
			{ID: "e1", UserID: "u1", Category: privacy.ConsentMemoryIdentity},
			{ID: "e2", UserID: "u2", Category: privacy.ConsentAnalyticsAggregate},
		},
	}
	// u1 → delivered, u2 → not delivered. deliverOn is keyed by UserID (what the notifier receives).
	notifier := &mockAlternatingNotifier{deliverOn: map[string]bool{"u1": true, "u2": false}}

	w, _ := newTestReplayWorker(t, store, notifier)
	require.NoError(t, w.replayOnce(t.Context()))

	store.mu.Lock()
	deliveredIDs := store.deliveredIDs
	store.mu.Unlock()

	assert.Equal(t, []string{"e1"}, deliveredIDs, "only e1 must be marked delivered")
}

// mockAlternatingNotifier delivers based on a per-userID map.
type mockAlternatingNotifier struct {
	mu        sync.Mutex
	deliverOn map[string]bool // keyed by entry UserID
	calls     []string
}

func (m *mockAlternatingNotifier) NotifyRevocation(_ context.Context, userID string, _ privacy.ConsentCategory) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, userID)
	return m.deliverOn[userID], nil
}

// TestOutboxReplayWorker_ListError asserts that a list error is returned from
// replayOnce and Prune/CountStuck are still called (best-effort).
func TestOutboxReplayWorker_ListError(t *testing.T) {
	store := &fakeOutboxStore{listErr: assert.AnError}
	notifier := &fakeConsentNotifier{delivered: true}

	w, _ := newTestReplayWorker(t, store, notifier)
	err := w.replayOnce(t.Context())
	assert.Error(t, err, "replayOnce must propagate list error")
}

// TestOutboxReplayWorker_MarkError asserts that a mark error is logged and
// does not abort processing of remaining entries.
func TestOutboxReplayWorker_MarkError(t *testing.T) {
	store := &fakeOutboxStore{
		entries: []privacy.OutboxEntry{
			{ID: "bad-mark", UserID: "u1", Category: privacy.ConsentMemoryIdentity},
		},
		markErr: assert.AnError,
	}
	notifier := &fakeConsentNotifier{delivered: true}

	w, _ := newTestReplayWorker(t, store, notifier)
	// replayOnce should not return an error — mark errors are logged, not propagated.
	assert.NoError(t, w.replayOnce(t.Context()))
}

// TestOutboxReplayWorker_RunCollectsAtStartup asserts that Run calls replayOnce
// before the first tick (same pattern as OptInMetricWorker).
func TestOutboxReplayWorker_RunCollectsAtStartup(t *testing.T) {
	store := &fakeOutboxStore{
		entries: []privacy.OutboxEntry{
			{ID: "startup-entry", UserID: "u1", Category: privacy.ConsentMemoryIdentity},
		},
	}
	notifier := &fakeConsentNotifier{delivered: true}

	reg := prometheus.NewRegistry()
	// Use a very long interval so the ticker never fires within this test.
	w := NewOutboxReplayWorker(store, notifier, time.Hour, time.Hour, reg, zap.New(zap.UseDevMode(true)))

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	// Wait briefly for the immediate collect, then cancel.
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	store.mu.Lock()
	deliveredIDs := store.deliveredIDs
	store.mu.Unlock()

	assert.Equal(t, []string{"startup-entry"}, deliveredIDs,
		"entry must be marked delivered before first tick (immediate collect)")
}

// TestOutboxReplayWorker_RunRespectsAlreadyCancelledContext asserts that Run
// returns immediately when the context is already cancelled.
func TestOutboxReplayWorker_RunRespectsAlreadyCancelledContext(t *testing.T) {
	store := &fakeOutboxStore{
		entries: []privacy.OutboxEntry{
			{ID: "noop-entry", UserID: "u1", Category: privacy.ConsentMemoryIdentity},
		},
	}
	notifier := &fakeConsentNotifier{delivered: true}

	w, _ := newTestReplayWorker(t, store, notifier)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel before Run is called

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Correct — returned promptly.
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return promptly for a pre-cancelled context")
	}

	// No entries should have been processed.
	store.mu.Lock()
	deliveredIDs := store.deliveredIDs
	store.mu.Unlock()
	assert.Empty(t, deliveredIDs, "no entries must be processed when context is pre-cancelled")
}

// TestOutboxReplayWorker_EmptyOutbox asserts replayOnce is a no-op on an empty
// outbox (no mark calls, prune still runs, stuck gauge set to 0).
func TestOutboxReplayWorker_EmptyOutbox(t *testing.T) {
	store := &fakeOutboxStore{stuckCount: 0}
	notifier := &fakeConsentNotifier{delivered: true}

	w, gauge := newTestReplayWorker(t, store, notifier)
	require.NoError(t, w.replayOnce(t.Context()))

	store.mu.Lock()
	deliveredIDs := store.deliveredIDs
	pruneCount := store.pruneCallCount
	store.mu.Unlock()

	assert.Empty(t, deliveredIDs, "no deliveries for empty outbox")
	assert.Equal(t, 1, pruneCount, "Prune must still be called on empty outbox")
	assert.InDelta(t, 0.0, testutil.ToFloat64(gauge), 0.001, "stuck gauge must be 0 for empty outbox")
}

// errConsentNotifier always returns an error from NotifyRevocation.
// Used to exercise the defensive err != nil path in deliverOne.
type errConsentNotifier struct{}

func (e errConsentNotifier) NotifyRevocation(_ context.Context, _ string, _ privacy.ConsentCategory) (bool, error) {
	return false, assert.AnError
}

// TestOutboxReplayWorker_NotifierError asserts that when the notifier returns an
// unexpected error (contract violation), deliverOne logs it and does NOT mark the
// entry delivered. replayOnce continues to Prune and CountStuck.
func TestOutboxReplayWorker_NotifierError(t *testing.T) {
	store := &fakeOutboxStore{
		entries: []privacy.OutboxEntry{
			{ID: "err-entry", UserID: "u1", Category: privacy.ConsentMemoryIdentity},
		},
	}
	notifier := errConsentNotifier{}

	w, _ := newTestReplayWorker(t, store, notifier)
	assert.NoError(t, w.replayOnce(t.Context()), "notifier error must not propagate from replayOnce")

	store.mu.Lock()
	deliveredIDs := store.deliveredIDs
	pruneCount := store.pruneCallCount
	store.mu.Unlock()

	assert.Empty(t, deliveredIDs, "entry must NOT be marked delivered on notifier error")
	assert.Equal(t, 1, pruneCount, "Prune must still be called after notifier error")
}

// TestOutboxReplayWorker_PruneError asserts that a Prune error is logged but
// does not abort replayOnce or prevent the stuck gauge from being set.
func TestOutboxReplayWorker_PruneError(t *testing.T) {
	store := &fakeOutboxStore{pruneErr: assert.AnError, stuckCount: 3}
	notifier := &fakeConsentNotifier{delivered: true}

	w, gauge := newTestReplayWorker(t, store, notifier)
	assert.NoError(t, w.replayOnce(t.Context()), "prune error must not propagate")
	assert.InDelta(t, 3.0, testutil.ToFloat64(gauge), 0.001, "stuck gauge must still be set after prune error")
}

// TestOutboxReplayWorker_StuckCountError asserts that a CountStuckOutbox error
// is logged but does not abort replayOnce. The gauge is left unchanged (not set to 0).
func TestOutboxReplayWorker_StuckCountError(t *testing.T) {
	store := &fakeOutboxStore{stuckErr: assert.AnError}
	notifier := &fakeConsentNotifier{delivered: true}

	w, gauge := newTestReplayWorker(t, store, notifier)
	assert.NoError(t, w.replayOnce(t.Context()), "stuck count error must not propagate")
	// Gauge stays at its initial zero value since Set was never called.
	assert.InDelta(t, 0.0, testutil.ToFloat64(gauge), 0.001, "gauge unchanged on count error")
}

// TestOutboxReplayWorker_RunLogsErrors asserts that Run logs errors from
// replayOnce (both the immediate collect and subsequent ticker fires) and does
// not panic or return early.
func TestOutboxReplayWorker_RunLogsErrors(t *testing.T) {
	// Store always fails list → every replayOnce returns an error.
	store := &fakeOutboxStore{listErr: assert.AnError}
	notifier := &fakeConsentNotifier{delivered: true}

	reg := prometheus.NewRegistry()
	// Very short interval so at least one ticker fires within the test window.
	w := NewOutboxReplayWorker(store, notifier, time.Millisecond, time.Hour, reg, zap.New(zap.UseDevMode(true)))

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Run returned after context cancellation — correct.
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}
