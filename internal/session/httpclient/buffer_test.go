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

package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/sony/gobreaker/v2"

	"github.com/altairalabs/omnia/internal/session"
)

// --- writeBuffer unit tests ---

func TestWriteBuffer_EnqueueDequeue(t *testing.T) {
	buf := newWriteBuffer(3)

	buf.enqueue(bufferedRequest{method: "POST", path: "/a"})
	buf.enqueue(bufferedRequest{method: "POST", path: "/b"})

	if buf.len() != 2 {
		t.Fatalf("expected len 2, got %d", buf.len())
	}

	item, ok := buf.dequeue()
	if !ok || item.path != "/a" {
		t.Fatalf("expected /a, got %s (ok=%v)", item.path, ok)
	}

	item, ok = buf.dequeue()
	if !ok || item.path != "/b" {
		t.Fatalf("expected /b, got %s (ok=%v)", item.path, ok)
	}

	_, ok = buf.dequeue()
	if ok {
		t.Fatal("expected empty buffer")
	}
}

func TestWriteBuffer_Overflow(t *testing.T) {
	buf := newWriteBuffer(2)

	buf.enqueue(bufferedRequest{path: "/a"})
	buf.enqueue(bufferedRequest{path: "/b"})
	dropped := buf.enqueue(bufferedRequest{path: "/c"})

	if !dropped {
		t.Fatal("expected dropped=true on overflow")
	}
	if buf.dropped.Load() != 1 {
		t.Fatalf("expected 1 dropped, got %d", buf.dropped.Load())
	}

	// Oldest (/a) should have been dropped.
	item, _ := buf.dequeue()
	if item.path != "/b" {
		t.Fatalf("expected /b after overflow, got %s", item.path)
	}
	item, _ = buf.dequeue()
	if item.path != "/c" {
		t.Fatalf("expected /c after overflow, got %s", item.path)
	}
}

func TestWriteBuffer_Peek(t *testing.T) {
	buf := newWriteBuffer(3)

	_, ok := buf.peek()
	if ok {
		t.Fatal("expected empty peek")
	}

	buf.enqueue(bufferedRequest{path: "/a"})
	item, ok := buf.peek()
	if !ok || item.path != "/a" {
		t.Fatalf("expected /a, got %s (ok=%v)", item.path, ok)
	}

	// peek should not remove the item.
	if buf.len() != 1 {
		t.Fatalf("expected len 1 after peek, got %d", buf.len())
	}
}

func TestWriteBuffer_WrapAround(t *testing.T) {
	buf := newWriteBuffer(3)

	// Fill and drain to advance head.
	buf.enqueue(bufferedRequest{path: "/a"})
	buf.enqueue(bufferedRequest{path: "/b"})
	buf.dequeue()
	buf.dequeue()

	// Now head is at index 2. Add 3 items to test wrap-around.
	buf.enqueue(bufferedRequest{path: "/c"})
	buf.enqueue(bufferedRequest{path: "/d"})
	buf.enqueue(bufferedRequest{path: "/e"})

	item, _ := buf.dequeue()
	if item.path != "/c" {
		t.Fatalf("expected /c, got %s", item.path)
	}
	item, _ = buf.dequeue()
	if item.path != "/d" {
		t.Fatalf("expected /d, got %s", item.path)
	}
	item, _ = buf.dequeue()
	if item.path != "/e" {
		t.Fatalf("expected /e, got %s", item.path)
	}
}

// --- Store buffer integration tests ---

// newBufferedTestStore creates a store with fast flush interval and a circuit
// breaker that trips quickly, suitable for buffer integration tests.
func newBufferedTestStore(t *testing.T, baseURL string) *Store {
	t.Helper()
	s := NewStore(baseURL, logr.Discard(),
		WithBufferFlushInterval(100*time.Millisecond),
		WithBufferMaxAge(2*time.Second))
	// Replace CB with one that trips fast for testing.
	s.cb = gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{
		Name:        "test-buffer",
		MaxRequests: 2,
		Interval:    0,
		Timeout:     100 * time.Millisecond,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.Requests >= 3 &&
				float64(counts.TotalFailures)/float64(counts.Requests) >= 0.6
		},
		OnStateChange: func(_ string, _, to gobreaker.State) {
			s.notifyFlush(to)
		},
	})
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestBufferedWrite_ServiceDown(t *testing.T) {
	// Server always returns 502 (retryable).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	store := newBufferedTestStore(t, srv.URL)

	// Trip the circuit breaker so subsequent writes fail immediately.
	for range 3 {
		_, _ = store.GetSession(context.Background(), "x")
	}

	// Write while CB is open — should be buffered, not return an error.
	err := store.AppendMessage(context.Background(), "s1", session.Message{
		ID: "m1", Role: session.RoleUser, Content: "hello",
	})
	if err != nil {
		t.Fatalf("expected nil (buffered), got: %v", err)
	}
	if store.Buffered() != 1 {
		t.Fatalf("expected 1 buffered, got %d", store.Buffered())
	}
}

func TestBufferedWrite_FlushOnRecovery(t *testing.T) {
	var serverUp atomic.Bool
	var flushed atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !serverUp.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		flushed.Add(1)
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	store := newBufferedTestStore(t, srv.URL)

	// Trip CB.
	for range 3 {
		_, _ = store.GetSession(context.Background(), "x")
	}

	// Buffer some writes while service is "down".
	for i := range 3 {
		_ = store.AppendMessage(context.Background(), "s1", session.Message{
			ID: fmt.Sprintf("m%d", i), Role: session.RoleUser, Content: "hi",
		})
	}
	if store.Buffered() != 3 {
		t.Fatalf("expected 3 buffered, got %d", store.Buffered())
	}

	// Bring server back up and wait for CB to transition to half-open + flush.
	serverUp.Store(true)
	time.Sleep(500 * time.Millisecond)

	if store.Buffered() != 0 {
		t.Fatalf("expected 0 buffered after flush, got %d", store.Buffered())
	}
	if flushed.Load() != 3 {
		t.Fatalf("expected 3 flushed, got %d", flushed.Load())
	}
}

func TestBufferedWrite_StatsAndTTL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	store := newBufferedTestStore(t, srv.URL)

	// Trip CB.
	for range 3 {
		_, _ = store.GetSession(context.Background(), "x")
	}

	// UpdateSessionStats should buffer.
	err := store.UpdateSessionStats(context.Background(), "s1", session.SessionStatsUpdate{
		AddMessages: 1,
	})
	if err != nil {
		t.Fatalf("UpdateSessionStats: expected nil (buffered), got: %v", err)
	}

	// RefreshTTL should buffer.
	err = store.RefreshTTL(context.Background(), "s1", time.Hour)
	if err != nil {
		t.Fatalf("RefreshTTL: expected nil (buffered), got: %v", err)
	}

	if store.Buffered() != 2 {
		t.Fatalf("expected 2 buffered, got %d", store.Buffered())
	}
}

func TestBufferedWrite_ExpiredItemsDropped(t *testing.T) {
	// Use a working server — we're testing expiry, not flush failure.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard(),
		WithBufferFlushInterval(50*time.Millisecond),
		WithBufferMaxAge(100*time.Millisecond)) // very short max age
	t.Cleanup(func() { _ = store.Close() })

	// Manually enqueue an already-expired item.
	body, _ := json.Marshal(&session.Message{ID: "m1", Role: session.RoleUser, Content: "hi"})
	store.buf.enqueue(bufferedRequest{
		method: http.MethodPost,
		path:   "/api/v1/sessions/s1/messages",
		body:   body,
		queued: time.Now().Add(-time.Minute), // expired 1 minute ago
	})
	if store.Buffered() != 1 {
		t.Fatalf("expected 1 buffered, got %d", store.Buffered())
	}

	// Wait for flush to process the expired item.
	time.Sleep(200 * time.Millisecond)

	if store.Buffered() != 0 {
		t.Fatalf("expected 0 buffered after expiry, got %d", store.Buffered())
	}
}

func TestBufferedWrite_Disabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard(), WithBufferCapacity(0))
	t.Cleanup(func() { _ = store.Close() })

	// With buffer disabled, errors should be returned directly.
	err := store.AppendMessage(context.Background(), "s1", session.Message{
		ID: "m1", Role: session.RoleUser, Content: "hi",
	})
	if err == nil {
		t.Fatal("expected error with buffer disabled")
	}
	if store.Buffered() != 0 {
		t.Fatalf("expected 0 buffered (disabled), got %d", store.Buffered())
	}
	if store.Dropped() != 0 {
		t.Fatalf("expected 0 dropped (disabled), got %d", store.Dropped())
	}
}

func TestBufferedWrite_CreateSessionNotBuffered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	store := newBufferedTestStore(t, srv.URL)

	// Trip CB.
	for range 3 {
		_, _ = store.GetSession(context.Background(), "x")
	}

	// CreateSession should NOT buffer — it must return an error.
	_, err := store.CreateSession(context.Background(), session.CreateSessionOptions{
		AgentName: "a", Namespace: "ns",
	})
	if err == nil {
		t.Fatal("expected error for CreateSession (not buffered)")
	}
	if store.Buffered() != 0 {
		t.Fatalf("expected 0 buffered after CreateSession failure, got %d", store.Buffered())
	}
}

func TestBufferedWrite_CloseFlushesRemaining(t *testing.T) {
	var flushed atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flushed.Add(1)
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard(),
		WithBufferFlushInterval(10*time.Second)) // long interval — won't fire during test
	// Don't register cleanup — we'll close manually.

	// Manually enqueue items (simulating buffered writes).
	body, _ := json.Marshal(&session.Message{ID: "m1", Role: session.RoleUser, Content: "hi"})
	store.buf.enqueue(bufferedRequest{
		method: http.MethodPost,
		path:   "/api/v1/sessions/s1/messages",
		body:   body,
		queued: time.Now(),
	})
	store.buf.enqueue(bufferedRequest{
		method: http.MethodPost,
		path:   "/api/v1/sessions/s2/messages",
		body:   body,
		queued: time.Now(),
	})

	// Close triggers a final drain.
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if flushed.Load() != 2 {
		t.Fatalf("expected 2 flushed on close, got %d", flushed.Load())
	}
}
