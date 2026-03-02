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

	// Send a message to trigger session creation
	msg := ClientMessage{Type: MessageTypeMessage, Content: "hello"}
	data, _ := json.Marshal(msg)
	if err := wsConn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatal(err)
	}

	// Read the connected message to get session ID
	var connectedMsg ServerMessage
	if err := wsConn.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("failed to read connected: %v", err)
	}
	sessionID := connectedMsg.SessionID
	if sessionID == "" {
		t.Fatal("session ID should not be empty")
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

	// Send a message that triggers an error response
	msg := ClientMessage{Type: MessageTypeMessage, Content: "trigger error"}
	data, _ := json.Marshal(msg)
	if err := wsConn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatal(err)
	}

	// Read the connected message
	var connectedMsg ServerMessage
	if err := wsConn.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("failed to read connected: %v", err)
	}
	sessionID := connectedMsg.SessionID

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
