/*
Copyright 2025-2026.

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
	"testing"

	"github.com/gorilla/websocket"
)

// TestConnResponseWriter_WriteInterrupt verifies that WriteInterrupt sends a
// JSON ServerMessage with type "interrupt" to the WebSocket client.
func TestConnResponseWriter_WriteInterrupt(t *testing.T) {
	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, _ *ClientMessage, writer ResponseWriter) error {
			return writer.WriteInterrupt()
		},
	}

	_, ts := newTestServer(t, handler)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Read eagerly-sent connected message.
	sessionID := readConnected(t, ws)

	// Send a message to trigger the handler.
	if err := ws.WriteJSON(ClientMessage{
		Type:      MessageTypeMessage,
		SessionID: sessionID,
		Content:   "barge-in",
	}); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Read the interrupt message.
	var msg ServerMessage
	if err := ws.ReadJSON(&msg); err != nil {
		t.Fatalf("Failed to read interrupt message: %v", err)
	}

	if msg.Type != MessageTypeInterrupt {
		t.Errorf("msg.Type = %q, want %q", msg.Type, MessageTypeInterrupt)
	}
	if msg.SessionID == "" {
		t.Error("msg.SessionID should not be empty")
	}
	if msg.Timestamp.IsZero() {
		t.Error("msg.Timestamp should not be zero")
	}
}
