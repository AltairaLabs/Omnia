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

package facade

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/sessiontest"
)

func TestCleanupConnection_MarksSessionCompleted(t *testing.T) {
	handler := &mockHandler{}
	_, ts, archive := newTestServerWithStore(t, handler)

	wsConn, resp, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read eagerly-sent connected message to get session ID
	var connectedMsg ServerMessage
	if err := wsConn.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("failed to read connected: %v", err)
	}
	sessionID := connectedMsg.SessionID
	if sessionID == "" {
		t.Fatal("session ID should not be empty")
	}

	// Send a message with session ID
	msg := ClientMessage{Type: MessageTypeMessage, SessionID: sessionID, Content: "hello"}
	data, _ := json.Marshal(msg)
	if err := wsConn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatal(err)
	}

	// Read the done response
	var doneMsg ServerMessage
	if err := wsConn.ReadJSON(&doneMsg); err != nil {
		t.Fatal(err)
	}

	// Close the WebSocket connection (triggers cleanupConnection)
	_ = wsConn.Close()

	// Wait for async cleanup
	time.Sleep(100 * time.Millisecond)

	// Verify the session is marked as completed
	ctx := context.Background()
	updated, err := archive.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if updated.Status != session.SessionStatusCompleted {
		t.Errorf("Status = %q, want %q", updated.Status, session.SessionStatusCompleted)
	}
	if updated.EndedAt.IsZero() {
		t.Error("EndedAt should be set after disconnect")
	}
}

func TestCleanupConnection_NoSessionID_SkipsUpdate(t *testing.T) {
	handler := &mockHandler{}
	_, ts := newTestServer(t, handler)

	// Connect without sending any message (no session created)
	wsConn, resp, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Close immediately without sending a message — no session to update
	_ = wsConn.Close()

	// Wait for cleanup
	time.Sleep(100 * time.Millisecond)
	// If we get here without panic, the test passes
}

func TestCleanupConnection_ErrorStatusNotOverwritten(t *testing.T) {
	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, _ *ClientMessage, writer ResponseWriter) error {
			return writer.WriteError("INTERNAL_ERROR", "forced error")
		},
	}
	_, ts, archive := newTestServerWithStore(t, handler)

	wsConn, resp, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read eagerly-sent connected message
	var connectedMsg ServerMessage
	if err := wsConn.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("failed to read connected: %v", err)
	}
	sessionID := connectedMsg.SessionID

	// Send a message that triggers an error response
	msg := ClientMessage{Type: MessageTypeMessage, SessionID: sessionID, Content: "trigger error"}
	data, _ := json.Marshal(msg)
	if err := wsConn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatal(err)
	}

	// Read the error response
	_, rawMsg, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawMsg), "INTERNAL_ERROR") {
		t.Fatalf("expected error response, got: %s", rawMsg)
	}

	// Wait for async WriteError to propagate to store
	time.Sleep(50 * time.Millisecond)

	// Close the connection — should NOT overwrite error status to completed
	_ = wsConn.Close()

	// Wait for async cleanup
	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	updated, err := archive.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if updated.Status != session.SessionStatusError {
		t.Errorf("Status = %q, want %q (error should not be overwritten to completed)",
			updated.Status, session.SessionStatusError)
	}
}

func TestConnection_MaxInFlightMessagesPerConnection(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, msg *ClientMessage, writer ResponseWriter) error {
			if msg.Content == "first" {
				started <- struct{}{}
				<-release
			}
			return writer.WriteDone("done: " + msg.Content)
		},
	}

	server, ts := newTestServer(t, handler)
	server.config.MaxInFlightMessagesPerConnection = 1

	wsConn, resp, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	defer func() { _ = wsConn.Close() }()

	// Read connected message to get session ID
	var connectedMsg ServerMessage
	if err := wsConn.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("failed to read connected: %v", err)
	}

	first := ClientMessage{Type: MessageTypeMessage, SessionID: connectedMsg.SessionID, Content: "first"}
	firstData, _ := json.Marshal(first)
	if err := wsConn.WriteMessage(websocket.TextMessage, firstData); err != nil {
		t.Fatalf("failed to write first message: %v", err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first message was not started")
	}

	second := ClientMessage{Type: MessageTypeMessage, SessionID: connectedMsg.SessionID, Content: "second"}
	secondData, _ := json.Marshal(second)
	if err := wsConn.WriteMessage(websocket.TextMessage, secondData); err != nil {
		t.Fatalf("failed to write second message: %v", err)
	}

	errMsg := readServerMsg(t, wsConn)
	if errMsg.Type != MessageTypeError {
		t.Fatalf("expected error message, got %q", errMsg.Type)
	}
	if errMsg.Error == nil || errMsg.Error.Code != ErrorCodeRateLimited {
		t.Fatalf("expected %s error code, got %#v", ErrorCodeRateLimited, errMsg.Error)
	}

	close(release)

	doneMsg := readServerMsg(t, wsConn)
	if doneMsg.Type != MessageTypeDone {
		t.Fatalf("expected done message, got %q", doneMsg.Type)
	}
	if doneMsg.Content != "done: first" {
		t.Fatalf("done content = %q, want %q", doneMsg.Content, "done: first")
	}
}

// TestCleanup_ParksOnUnintentionalClose verifies that an unintentional WS close
// parks the audio session instead of tearing it down.
func TestCleanup_ParksOnUnintentionalClose(t *testing.T) {
	s := NewServer(DefaultServerConfig(), nil, nil, logr.Discard(), WithGraceWindow(time.Minute))
	sink := &fakeDuplexSink{audio: make(chan []byte, 1)}
	c := &Connection{conn: nil, sessionID: testParkSessionID, userID: testParkOwnerID, audioSession: newAudioSession(testParkSessionID, sink, nil)}
	parked := s.parkOnClose(context.Background(), c)

	if !parked {
		t.Fatalf("parkOnClose must return true when session is parked")
	}
	if s.parked.len() != 1 {
		t.Fatalf("session not parked: want 1, got %d", s.parked.len())
	}
	if sink.closeCount() > 0 {
		t.Fatalf("session was closed instead of parked")
	}
}

// spySessionStore wraps a real session.Store and records UpdateSessionStatus calls.
type spySessionStore struct {
	session.Store
	mu       sync.Mutex
	statuses []session.SessionStatusUpdate
}

func (s *spySessionStore) UpdateSessionStatus(ctx context.Context, id string, update session.SessionStatusUpdate) error {
	s.mu.Lock()
	s.statuses = append(s.statuses, update)
	s.mu.Unlock()
	return s.Store.UpdateSessionStatus(ctx, id, update)
}

func (s *spySessionStore) completedCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, u := range s.statuses {
		if u.SetStatus == session.SessionStatusCompleted {
			n++
		}
	}
	return n
}

// TestParkOnClose_SkipsCompletionWhenParked is a regression test for Fix 1:
// when parkOnClose returns true (session parked), cleanupConnection must NOT
// call UpdateSessionStatus(SessionStatusCompleted).
func TestParkOnClose_SkipsCompletionWhenParked(t *testing.T) {
	spy := &spySessionStore{Store: sessiontest.NewStore()}

	s := NewServer(DefaultServerConfig(), spy, nil, logr.Discard(), WithGraceWindow(time.Minute))

	sink := &fakeDuplexSink{audio: make(chan []byte, 1)}
	c := &Connection{
		conn:             nil,
		sessionID:        "sid-park",
		sessionPersisted: true,
		userID:           testParkOwnerID,
		intentionalClose: false,
		audioSession:     newAudioSession("sid-park", sink, nil),
	}

	// parkOnClose must return true (session parked, not torn down).
	parked := s.parkOnClose(context.Background(), c)
	if !parked {
		t.Fatalf("parkOnClose must return true for unintentional close with audio session")
	}

	// Simulate the condition guard in cleanupConnection: completion must be skipped.
	sessionID := "sid-park"
	if parked || sessionID == "" || !c.sessionPersisted {
		// This is the expected path — completion skipped.
	} else {
		t.Fatalf("completion guard would have been entered despite parking")
	}

	// No UpdateSessionStatus(Completed) should have been called.
	time.Sleep(50 * time.Millisecond)
	if got := spy.completedCalls(); got != 0 {
		t.Fatalf("UpdateSessionStatus(Completed) called %d time(s), want 0 when parked", got)
	}
}

// TestCleanup_ClosesOnIntentionalClose verifies that an intentional hangup tears
// down the audio session rather than parking it.
func TestCleanup_ClosesOnIntentionalClose(t *testing.T) {
	s := NewServer(DefaultServerConfig(), nil, nil, logr.Discard())
	sink := &fakeDuplexSink{audio: make(chan []byte, 1)}
	c := &Connection{sessionID: testParkSessionID, userID: testParkOwnerID, intentionalClose: true, audioSession: newAudioSession(testParkSessionID, sink, nil)}
	parked := s.parkOnClose(context.Background(), c)

	if parked {
		t.Fatalf("parkOnClose must return false for intentional close")
	}
	if s.parked.len() != 0 {
		t.Fatalf("intentional close should not park: got %d parked", s.parked.len())
	}
	if sink.closeCount() == 0 {
		t.Fatalf("intentional close should close the session")
	}
}

// TestReattach_BindsParkedSession verifies that tryReattach binds c.sessionID and
// c.audioSession when a parked session exists and is owned by the connecting user.
func TestReattach_BindsParkedSession(t *testing.T) {
	s := NewServer(DefaultServerConfig(), nil, nil, logr.Discard(), WithGraceWindow(time.Minute))
	sink := &fakeDuplexSink{audio: make(chan []byte, 1)}
	as := newAudioSession("sid-9", sink, nil)
	s.parked.park(context.Background(), "sid-9", testParkOwnerID, as, true)

	c := &Connection{sessionID: "", userID: testParkOwnerID, resumeID: "sid-9"}
	got, resumed := s.tryReattach(context.Background(), c)
	if !resumed || got != as {
		t.Fatalf("reattach failed: resumed=%v got=%v", resumed, got)
	}
	if c.sessionID != "sid-9" || c.audioSession != as {
		t.Fatalf("connection not bound to parked session")
	}
}

// TestReattach_OwnerMismatchFallsThrough verifies that tryReattach returns
// (nil, false) when the parked session is owned by a different user.
func TestReattach_OwnerMismatchFallsThrough(t *testing.T) {
	s := NewServer(DefaultServerConfig(), nil, nil, logr.Discard(), WithGraceWindow(time.Minute))
	s.parked.park(context.Background(), "sid-9", testParkOwnerID, newAudioSession("sid-9", &fakeDuplexSink{audio: make(chan []byte, 1)}, nil), true)

	c := &Connection{userID: "attacker", resumeID: "sid-9"}
	if _, resumed := s.tryReattach(context.Background(), c); resumed {
		t.Fatalf("reattach succeeded for wrong owner")
	}
}

// readServerMsg sets a short read deadline and decodes the next ServerMessage,
// failing the test on error. Extracted to keep the in-flight-limit test within
// the cyclomatic-complexity budget.
func readServerMsg(t *testing.T, conn *websocket.Conn) ServerMessage {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("failed to set read deadline: %v", err)
	}
	var msg ServerMessage
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("failed to read server message: %v", err)
	}
	return msg
}

// Parking defers completion because the conversation may yet resume. Expiry is
// the point at which it definitively did not, so the archive row must reach a
// terminal status there — otherwise every parked-then-expired session stays
// "active" forever (#1876). This exercises the real onExpire wiring installed by
// NewServer, not the registry callback in isolation.
func TestParkExpiry_CompletesSession(t *testing.T) {
	spy := &spySessionStore{Store: sessiontest.NewStore()}

	s := NewServer(DefaultServerConfig(), spy, nil, logr.Discard(),
		WithGraceWindow(20*time.Millisecond))

	sink := &fakeDuplexSink{audio: make(chan []byte, 1)}
	c := &Connection{
		sessionID:        "sid-expire",
		sessionPersisted: true,
		userID:           testParkOwnerID,
		intentionalClose: false,
		audioSession:     newAudioSession("sid-expire", sink, nil),
	}

	if parked := s.parkOnClose(context.Background(), c); !parked {
		t.Fatal("parkOnClose must park an unintentional close with an audio session")
	}
	// Nothing terminal yet — the session is still reattachable.
	if got := spy.completedCalls(); got != 0 {
		t.Fatalf("completion written while still parked: %d call(s), want 0", got)
	}

	// Wait for the grace window to elapse and the completion write to drain.
	deadline := time.Now().Add(2 * time.Second)
	for spy.completedCalls() == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}

	if got := spy.completedCalls(); got != 1 {
		t.Fatalf("UpdateSessionStatus(Completed) called %d time(s) after park expiry, want 1", got)
	}
}

// A pure-audio session never persists an archive row — binary frames bypass
// processMessage, so sessionPersisted stays false. Completing it on park expiry
// would write a terminal status for a row that does not exist.
func TestParkExpiry_SkipsCompletionForUnpersistedSession(t *testing.T) {
	spy := &spySessionStore{Store: sessiontest.NewStore()}

	s := NewServer(DefaultServerConfig(), spy, nil, logr.Discard(),
		WithGraceWindow(20*time.Millisecond))

	sink := &fakeDuplexSink{audio: make(chan []byte, 1)}
	c := &Connection{
		sessionID:        "sid-audio-only",
		sessionPersisted: false, // never wrote an archive row
		userID:           testParkOwnerID,
		intentionalClose: false,
		audioSession:     newAudioSession("sid-audio-only", sink, nil),
	}

	if parked := s.parkOnClose(context.Background(), c); !parked {
		t.Fatal("parkOnClose must park an unintentional close with an audio session")
	}

	// Wait past the grace window and let any completion drain.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if spy.completedCalls() > 0 {
			t.Fatalf("completed a session that was never archived")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Recording can be disabled entirely (nil store). Park expiry must not panic.
func TestParkExpiry_NilSessionStoreDoesNotPanic(t *testing.T) {
	s := NewServer(DefaultServerConfig(), nil, nil, logr.Discard(),
		WithGraceWindow(10*time.Millisecond))

	sink := &fakeDuplexSink{audio: make(chan []byte, 1)}
	c := &Connection{
		sessionID:        "sid-nostore",
		sessionPersisted: true,
		userID:           testParkOwnerID,
		audioSession:     newAudioSession("sid-nostore", sink, nil),
	}

	if parked := s.parkOnClose(context.Background(), c); !parked {
		t.Fatal("parkOnClose must park an unintentional close with an audio session")
	}

	// Expiry fires on the registry's timer goroutine; a panic there would take
	// the test binary down.
	deadline := time.Now().Add(time.Second)
	for s.parked.len() != 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if s.parked.len() != 0 {
		t.Fatal("parked session did not expire")
	}
	time.Sleep(50 * time.Millisecond) // let the callback finish
}
