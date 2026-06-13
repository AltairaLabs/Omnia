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
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/altairalabs/omnia/internal/session"
)

func TestCleanupConnection_MarksSessionCompleted(t *testing.T) {
	handler := &mockHandler{}
	server, ts := newTestServer(t, handler)

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
	updated, err := server.sessionStore.GetSession(ctx, sessionID)
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
	server, ts := newTestServer(t, handler)

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
	updated, err := server.sessionStore.GetSession(ctx, sessionID)
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
