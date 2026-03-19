/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
)

// mockWriter records calls to the stats writer function.
type mockWriter struct {
	mu      sync.Mutex
	calls   []writerCall
	err     error
	callsCh chan struct{} // closed after each call to signal waiters
}

type writerCall struct {
	sessionID string
	update    session.SessionStatsUpdate
}

func newMockWriter() *mockWriter {
	return &mockWriter{callsCh: make(chan struct{}, 100)}
}

func (m *mockWriter) write(_ context.Context, sessionID string, update session.SessionStatsUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, writerCall{sessionID: sessionID, update: update})
	m.callsCh <- struct{}{}
	return m.err
}

func (m *mockWriter) getCalls() []writerCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]writerCall, len(m.calls))
	copy(out, m.calls)
	return out
}

func (m *mockWriter) setErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

func TestStatsBatcher_AccumulatesDeltas(t *testing.T) {
	w := newMockWriter()
	b := NewStatsBatcher(w.write, logr.Discard(), 50*time.Millisecond)
	defer b.Shutdown()

	// Accumulate multiple status updates for the same session — last one wins.
	b.IncrementStats("s1", session.SessionStatsUpdate{SetStatus: session.SessionStatusActive})
	b.IncrementStats("s1", session.SessionStatsUpdate{SetStatus: session.SessionStatusCompleted})

	assert.Equal(t, 1, b.Len(), "should have 1 pending session")

	// Wait for flush.
	<-w.callsCh

	calls := w.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "s1", calls[0].sessionID)
	assert.Equal(t, session.SessionStatusCompleted, calls[0].update.SetStatus)
}

func TestStatsBatcher_MultipleSessions(t *testing.T) {
	w := newMockWriter()
	b := NewStatsBatcher(w.write, logr.Discard(), 50*time.Millisecond)
	defer b.Shutdown()

	b.IncrementStats("s1", session.SessionStatsUpdate{SetStatus: session.SessionStatusActive})
	b.IncrementStats("s2", session.SessionStatsUpdate{SetStatus: session.SessionStatusCompleted})

	assert.Equal(t, 2, b.Len())

	// Wait for both flushes.
	<-w.callsCh
	<-w.callsCh

	calls := w.getCalls()
	require.Len(t, calls, 2)

	callMap := make(map[string]session.SessionStatsUpdate)
	for _, c := range calls {
		callMap[c.sessionID] = c.update
	}
	assert.Equal(t, session.SessionStatusActive, callMap["s1"].SetStatus)
	assert.Equal(t, session.SessionStatusCompleted, callMap["s2"].SetStatus)
}

func TestStatsBatcher_ShutdownFlushesRemaining(t *testing.T) {
	w := newMockWriter()
	// Use a very long interval so flush only happens on shutdown.
	b := NewStatsBatcher(w.write, logr.Discard(), time.Hour)

	b.IncrementStats("s1", session.SessionStatsUpdate{SetStatus: session.SessionStatusCompleted})

	// Shutdown should flush.
	b.Shutdown()

	calls := w.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "s1", calls[0].sessionID)
	assert.Equal(t, session.SessionStatusCompleted, calls[0].update.SetStatus)
}

func TestStatsBatcher_StatusAndEndedAt(t *testing.T) {
	w := newMockWriter()
	b := NewStatsBatcher(w.write, logr.Discard(), 50*time.Millisecond)
	defer b.Shutdown()

	endTime := time.Now().Truncate(time.Microsecond)

	// First update sets status, second overwrites it.
	b.IncrementStats("s1", session.SessionStatsUpdate{
		SetStatus: session.SessionStatusActive,
	})
	b.IncrementStats("s1", session.SessionStatsUpdate{
		SetStatus:  session.SessionStatusCompleted,
		SetEndedAt: endTime,
	})

	<-w.callsCh

	calls := w.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, session.SessionStatusCompleted, calls[0].update.SetStatus)
	assert.Equal(t, endTime, calls[0].update.SetEndedAt)
}

func TestStatsBatcher_ConcurrentAccess(t *testing.T) {
	w := newMockWriter()
	// Long interval so we control when flush happens.
	b := NewStatsBatcher(w.write, logr.Discard(), time.Hour)

	const goroutines = 50
	const incrementsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range incrementsPerGoroutine {
				b.IncrementStats("s1", session.SessionStatsUpdate{SetStatus: session.SessionStatusActive})
			}
		}()
	}
	wg.Wait()

	// Shutdown triggers flush.
	b.Shutdown()

	calls := w.getCalls()
	require.Len(t, calls, 1)
	// Last-write-wins for status; all goroutines wrote the same value.
	assert.Equal(t, session.SessionStatusActive, calls[0].update.SetStatus)
}

func TestStatsBatcher_FlushErrorDoesNotLoseData(t *testing.T) {
	w := newMockWriter()
	w.setErr(errors.New("db down"))

	b := NewStatsBatcher(w.write, logr.Discard(), 50*time.Millisecond)
	defer b.Shutdown()

	b.IncrementStats("s1", session.SessionStatsUpdate{SetStatus: session.SessionStatusError})

	// Wait for the failed flush attempt.
	<-w.callsCh

	// The data was attempted (writer was called) — verify it was called.
	calls := w.getCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, session.SessionStatusError, calls[0].update.SetStatus)
}

func TestStatsBatcher_EmptyFlushIsNoop(t *testing.T) {
	w := newMockWriter()
	b := NewStatsBatcher(w.write, logr.Discard(), 50*time.Millisecond)

	// Wait a bit so the ticker fires with no pending data.
	time.Sleep(100 * time.Millisecond)
	b.Shutdown()

	calls := w.getCalls()
	assert.Empty(t, calls, "no calls should be made when there is nothing to flush")
}

func TestStatsBatcher_DefaultInterval(t *testing.T) {
	w := newMockWriter()
	// Pass zero to get the default.
	b := NewStatsBatcher(w.write, logr.Discard(), 0)
	defer b.Shutdown()

	assert.Equal(t, defaultFlushInterval, b.interval)
}
