/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// DefaultInactivityTimeout is the default time after which a session with no
// activity is considered complete.
const DefaultInactivityTimeout = 5 * time.Minute

// CompletionCallback is called when a session is detected as complete,
// either via explicit completion or inactivity timeout.
type CompletionCallback func(ctx context.Context, sessionID string) error

// CompletionTracker tracks session activity and detects when sessions
// are complete, either via explicit session.completed events or
// inactivity timeout. It ensures onComplete fires at most once per session.
type CompletionTracker struct {
	mu                sync.Mutex
	lastSeen          map[string]time.Time
	completed         map[string]bool
	inactivityTimeout time.Duration
	onComplete        CompletionCallback
	logger            *slog.Logger
	nowFunc           func() time.Time // for testing
}

// NewCompletionTracker creates a tracker with the given inactivity timeout
// and completion callback.
func NewCompletionTracker(
	timeout time.Duration, onComplete CompletionCallback, logger *slog.Logger,
) *CompletionTracker {
	return &CompletionTracker{
		lastSeen:          make(map[string]time.Time),
		completed:         make(map[string]bool),
		inactivityTimeout: timeout,
		onComplete:        onComplete,
		logger:            logger,
		nowFunc:           time.Now,
	}
}

// RecordActivity updates the last-seen time for the given session.
func (t *CompletionTracker) RecordActivity(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.completed[sessionID] {
		return
	}
	t.lastSeen[sessionID] = t.now()
}

// MarkCompleted marks a session as explicitly completed and triggers the
// onComplete callback if it has not already been triggered.
func (t *CompletionTracker) MarkCompleted(ctx context.Context, sessionID string) {
	if t.tryMarkCompleted(sessionID) {
		t.fireCallback(ctx, sessionID)
	}
}

// CheckInactive scans tracked sessions and triggers onComplete for those
// that have exceeded the inactivity timeout.
func (t *CompletionTracker) CheckInactive(ctx context.Context) {
	expired := t.findExpiredSessions()
	for _, sessionID := range expired {
		t.fireCallback(ctx, sessionID)
	}
}

// StartPeriodicCheck runs CheckInactive on a timer until the context is
// cancelled. It blocks until ctx.Done().
func (t *CompletionTracker) StartPeriodicCheck(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.CheckInactive(ctx)
		}
	}
}

// Cleanup removes a session from tracking entirely.
func (t *CompletionTracker) Cleanup(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.lastSeen, sessionID)
	delete(t.completed, sessionID)
}

// TrackedCount returns the number of sessions currently being tracked.
// This is primarily useful for testing.
func (t *CompletionTracker) TrackedCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.lastSeen)
}

// tryMarkCompleted atomically checks and marks a session as completed.
// Returns true if this call performed the transition.
func (t *CompletionTracker) tryMarkCompleted(sessionID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.completed[sessionID] {
		return false
	}
	t.completed[sessionID] = true
	return true
}

// findExpiredSessions returns session IDs that have exceeded the inactivity
// timeout and marks them as completed.
func (t *CompletionTracker) findExpiredSessions() []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	var expired []string

	for sessionID, lastTime := range t.lastSeen {
		if t.completed[sessionID] {
			continue
		}
		if now.Sub(lastTime) >= t.inactivityTimeout {
			t.completed[sessionID] = true
			expired = append(expired, sessionID)
		}
	}

	return expired
}

// fireCallback invokes the onComplete callback and logs any errors.
func (t *CompletionTracker) fireCallback(ctx context.Context, sessionID string) {
	if t.onComplete == nil {
		return
	}
	if err := t.onComplete(ctx, sessionID); err != nil {
		t.logger.Error("completion callback failed",
			"sessionID", sessionID,
			"error", err,
		)
	}
}

// now returns the current time, using nowFunc for testability.
func (t *CompletionTracker) now() time.Time {
	return t.nowFunc()
}
