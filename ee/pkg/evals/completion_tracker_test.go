/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callbackRecorder records calls to the completion callback.
type callbackRecorder struct {
	mu        sync.Mutex
	sessions  []string
	returnErr error
	callCount int
}

func (r *callbackRecorder) callback(_ context.Context, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callCount++
	r.sessions = append(r.sessions, sessionID)
	return r.returnErr
}

func (r *callbackRecorder) getSessions() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]string, len(r.sessions))
	copy(result, r.sessions)
	return result
}

func (r *callbackRecorder) getCallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callCount
}

func TestCompletionTracker_RecordActivity_And_CheckInactive(t *testing.T) {
	rec := &callbackRecorder{}
	tracker := NewCompletionTracker(100*time.Millisecond, rec.callback, testLogger())

	// Use a fixed clock for deterministic testing.
	now := time.Now()
	tracker.nowFunc = func() time.Time { return now }

	tracker.RecordActivity("s1")
	tracker.RecordActivity("s2")

	// Not yet expired.
	tracker.CheckInactive(context.Background())
	assert.Empty(t, rec.getSessions())

	// Advance past inactivity timeout.
	now = now.Add(200 * time.Millisecond)
	tracker.CheckInactive(context.Background())

	sessions := rec.getSessions()
	assert.Len(t, sessions, 2)
	assert.Contains(t, sessions, "s1")
	assert.Contains(t, sessions, "s2")
}

func TestCompletionTracker_MarkCompleted_TriggersImmediately(t *testing.T) {
	rec := &callbackRecorder{}
	tracker := NewCompletionTracker(5*time.Minute, rec.callback, testLogger())

	tracker.RecordActivity("s1")
	tracker.MarkCompleted(context.Background(), "s1")

	sessions := rec.getSessions()
	require.Len(t, sessions, 1)
	assert.Equal(t, "s1", sessions[0])
}

func TestCompletionTracker_NoDuplicateTriggers_ExplicitThenInactive(t *testing.T) {
	rec := &callbackRecorder{}
	tracker := NewCompletionTracker(100*time.Millisecond, rec.callback, testLogger())

	now := time.Now()
	tracker.nowFunc = func() time.Time { return now }

	tracker.RecordActivity("s1")

	// Explicit completion.
	tracker.MarkCompleted(context.Background(), "s1")
	assert.Equal(t, 1, rec.getCallCount())

	// Advance past timeout and check — should NOT fire again.
	now = now.Add(200 * time.Millisecond)
	tracker.CheckInactive(context.Background())
	assert.Equal(t, 1, rec.getCallCount())
}

func TestCompletionTracker_NoDuplicateTriggers_InactiveThenExplicit(t *testing.T) {
	rec := &callbackRecorder{}
	tracker := NewCompletionTracker(100*time.Millisecond, rec.callback, testLogger())

	now := time.Now()
	tracker.nowFunc = func() time.Time { return now }

	tracker.RecordActivity("s1")

	// Expire via inactivity.
	now = now.Add(200 * time.Millisecond)
	tracker.CheckInactive(context.Background())
	assert.Equal(t, 1, rec.getCallCount())

	// Explicit completion — should NOT fire again.
	tracker.MarkCompleted(context.Background(), "s1")
	assert.Equal(t, 1, rec.getCallCount())
}

func TestCompletionTracker_NoDuplicateTriggers_DoubleMarkCompleted(t *testing.T) {
	rec := &callbackRecorder{}
	tracker := NewCompletionTracker(5*time.Minute, rec.callback, testLogger())

	tracker.MarkCompleted(context.Background(), "s1")
	tracker.MarkCompleted(context.Background(), "s1")

	assert.Equal(t, 1, rec.getCallCount())
}

func TestCompletionTracker_RecordActivity_IgnoredAfterCompletion(t *testing.T) {
	rec := &callbackRecorder{}
	tracker := NewCompletionTracker(100*time.Millisecond, rec.callback, testLogger())

	now := time.Now()
	tracker.nowFunc = func() time.Time { return now }

	tracker.RecordActivity("s1")
	tracker.MarkCompleted(context.Background(), "s1")

	// Recording activity after completion should not re-add.
	tracker.RecordActivity("s1")

	now = now.Add(200 * time.Millisecond)
	tracker.CheckInactive(context.Background())

	// Should still be just one callback call from the MarkCompleted.
	assert.Equal(t, 1, rec.getCallCount())
}

func TestCompletionTracker_Cleanup_RemovesSession(t *testing.T) {
	rec := &callbackRecorder{}
	tracker := NewCompletionTracker(100*time.Millisecond, rec.callback, testLogger())

	now := time.Now()
	tracker.nowFunc = func() time.Time { return now }

	tracker.RecordActivity("s1")
	tracker.RecordActivity("s2")
	assert.Equal(t, 2, tracker.TrackedCount())

	tracker.Cleanup("s1")
	assert.Equal(t, 1, tracker.TrackedCount())

	// Advance and check — only s2 should fire.
	now = now.Add(200 * time.Millisecond)
	tracker.CheckInactive(context.Background())

	sessions := rec.getSessions()
	require.Len(t, sessions, 1)
	assert.Equal(t, "s2", sessions[0])
}

func TestCompletionTracker_Cleanup_AfterCompletion(t *testing.T) {
	rec := &callbackRecorder{}
	tracker := NewCompletionTracker(5*time.Minute, rec.callback, testLogger())

	tracker.RecordActivity("s1")
	tracker.MarkCompleted(context.Background(), "s1")
	tracker.Cleanup("s1")

	assert.Equal(t, 0, tracker.TrackedCount())

	// After cleanup, the session can be tracked again.
	tracker.RecordActivity("s1")
	assert.Equal(t, 1, tracker.TrackedCount())
}

func TestCompletionTracker_ThreadSafety(t *testing.T) {
	var callCount atomic.Int64
	callback := func(_ context.Context, _ string) error {
		callCount.Add(1)
		return nil
	}

	tracker := NewCompletionTracker(10*time.Millisecond, callback, testLogger())

	var wg sync.WaitGroup
	ctx := context.Background()

	// Concurrent RecordActivity calls.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.RecordActivity("s1")
		}()
	}

	// Concurrent MarkCompleted calls.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.MarkCompleted(ctx, "s1")
		}()
	}

	// Concurrent CheckInactive calls.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.CheckInactive(ctx)
		}()
	}

	wg.Wait()

	// onComplete should have fired exactly once.
	assert.Equal(t, int64(1), callCount.Load())
}

func TestCompletionTracker_StartPeriodicCheck_CancelsOnContextDone(t *testing.T) {
	rec := &callbackRecorder{}
	tracker := NewCompletionTracker(10*time.Millisecond, rec.callback, testLogger())

	// Record activity at a fixed "past" time.
	pastTime := time.Now()
	tracker.nowFunc = func() time.Time { return pastTime }
	tracker.RecordActivity("s1")

	// Advance clock past the inactivity timeout so CheckInactive will trigger.
	tracker.nowFunc = func() time.Time { return pastTime.Add(time.Hour) }

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		tracker.StartPeriodicCheck(ctx, 10*time.Millisecond)
		close(done)
	}()

	// Wait for at least one tick.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// OK — returned after cancel.
	case <-time.After(2 * time.Second):
		t.Fatal("StartPeriodicCheck did not return after context cancel")
	}

	// Should have fired for s1 at least once.
	assert.GreaterOrEqual(t, rec.getCallCount(), 1)
}

func TestCompletionTracker_NilCallback(t *testing.T) {
	tracker := NewCompletionTracker(100*time.Millisecond, nil, testLogger())

	tracker.RecordActivity("s1")

	// Should not panic with nil callback.
	tracker.MarkCompleted(context.Background(), "s1")

	now := time.Now()
	tracker.nowFunc = func() time.Time { return now.Add(time.Hour) }
	tracker.RecordActivity("s2")
	tracker.CheckInactive(context.Background())
}

func TestCompletionTracker_CallbackError_LogsButContinues(t *testing.T) {
	rec := &callbackRecorder{returnErr: assert.AnError}
	tracker := NewCompletionTracker(100*time.Millisecond, rec.callback, testLogger())

	// Should not panic on error.
	tracker.MarkCompleted(context.Background(), "s1")
	assert.Equal(t, 1, rec.getCallCount())

	// Still counts as completed — no retry.
	tracker.MarkCompleted(context.Background(), "s1")
	assert.Equal(t, 1, rec.getCallCount())
}

func TestCompletionTracker_MultipleSessions_Independent(t *testing.T) {
	rec := &callbackRecorder{}
	tracker := NewCompletionTracker(100*time.Millisecond, rec.callback, testLogger())

	now := time.Now()
	tracker.nowFunc = func() time.Time { return now }

	tracker.RecordActivity("s1")

	// Advance 50ms, add s2.
	now = now.Add(50 * time.Millisecond)
	tracker.RecordActivity("s2")

	// Advance another 60ms — s1 expired (110ms), s2 not yet (60ms).
	now = now.Add(60 * time.Millisecond)
	tracker.CheckInactive(context.Background())

	sessions := rec.getSessions()
	require.Len(t, sessions, 1)
	assert.Equal(t, "s1", sessions[0])

	// Advance another 50ms — s2 now expired (110ms).
	now = now.Add(50 * time.Millisecond)
	tracker.CheckInactive(context.Background())

	sessions = rec.getSessions()
	require.Len(t, sessions, 2)
	assert.Equal(t, "s2", sessions[1])
}

func TestCompletionTracker_MarkCompleted_WithoutRecordActivity(t *testing.T) {
	rec := &callbackRecorder{}
	tracker := NewCompletionTracker(5*time.Minute, rec.callback, testLogger())

	// MarkCompleted without prior RecordActivity should still fire.
	tracker.MarkCompleted(context.Background(), "s1")

	sessions := rec.getSessions()
	require.Len(t, sessions, 1)
	assert.Equal(t, "s1", sessions[0])
}

func TestNewCompletionTracker_DefaultInactivityTimeout(t *testing.T) {
	assert.Equal(t, 5*time.Minute, DefaultInactivityTimeout)
}
