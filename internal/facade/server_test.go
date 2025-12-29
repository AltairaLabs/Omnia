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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"

	"github.com/altairalabs/omnia/internal/session"
)

// mockHandler implements MessageHandler for testing.
type mockHandler struct {
	handleFunc func(ctx context.Context, sessionID string, msg *ClientMessage, writer ResponseWriter) error
}

func (m *mockHandler) HandleMessage(ctx context.Context, sessionID string, msg *ClientMessage, writer ResponseWriter) error {
	if m.handleFunc != nil {
		return m.handleFunc(ctx, sessionID, msg, writer)
	}
	return writer.WriteDone("echo: " + msg.Content)
}

func newTestServer(t *testing.T, handler MessageHandler) (*Server, *httptest.Server) {
	t.Helper()

	store := session.NewMemoryStore()
	cfg := DefaultServerConfig()
	cfg.PingInterval = 100 * time.Millisecond
	cfg.PongTimeout = 200 * time.Millisecond

	log := logr.Discard()
	server := NewServer(cfg, store, handler, log)

	ts := httptest.NewServer(server)
	t.Cleanup(func() {
		ts.Close()
		_ = store.Close()
	})

	return server, ts
}

func wsURL(httpURL string) string {
	return strings.Replace(httpURL, "http://", "ws://", 1)
}

func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()

	if cfg.ReadBufferSize != 1024 {
		t.Errorf("ReadBufferSize = %v, want 1024", cfg.ReadBufferSize)
	}
	if cfg.WriteBufferSize != 1024 {
		t.Errorf("WriteBufferSize = %v, want 1024", cfg.WriteBufferSize)
	}
	if cfg.MaxMessageSize != 512*1024 {
		t.Errorf("MaxMessageSize = %v, want 524288", cfg.MaxMessageSize)
	}
}

func TestServerRequiresAgentParam(t *testing.T) {
	_, ts := newTestServer(t, nil)

	// Try to connect without agent param
	_, resp, err := websocket.DefaultDialer.Dial(wsURL(ts.URL), nil)
	if err == nil {
		t.Fatal("Expected error when connecting without agent param")
	}
	if resp != nil && resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %v", resp.StatusCode)
	}
}

func TestServerConnection(t *testing.T) {
	server, ts := newTestServer(t, nil)

	// Connect with agent param
	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Wait for connection to be registered (may take a moment on CI)
	var count int
	for i := 0; i < 50; i++ {
		count = server.ConnectionCount()
		if count == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if count != 1 {
		t.Errorf("ConnectionCount = %v, want 1", count)
	}
}

func TestServerConnectionWithNamespace(t *testing.T) {
	_, ts := newTestServer(t, nil)

	// Connect with agent and namespace params
	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent&namespace=test-ns", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()
}

func TestServerMessageHandling(t *testing.T) {
	handler := &mockHandler{
		handleFunc: func(_ context.Context, sessionID string, msg *ClientMessage, writer ResponseWriter) error {
			if err := writer.WriteChunk("chunk1"); err != nil {
				return err
			}
			return writer.WriteDone("done")
		},
	}

	_, ts := newTestServer(t, handler)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Send a message
	clientMsg := ClientMessage{
		Type:    MessageTypeMessage,
		Content: "Hello",
	}
	if err := ws.WriteJSON(clientMsg); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Read connected message
	var connectedMsg ServerMessage
	if err := ws.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("Failed to read connected message: %v", err)
	}
	if connectedMsg.Type != MessageTypeConnected {
		t.Errorf("Expected connected message, got %v", connectedMsg.Type)
	}
	if connectedMsg.SessionID == "" {
		t.Error("Session ID should not be empty")
	}

	// Read chunk message
	var chunkMsg ServerMessage
	if err := ws.ReadJSON(&chunkMsg); err != nil {
		t.Fatalf("Failed to read chunk: %v", err)
	}
	if chunkMsg.Type != MessageTypeChunk {
		t.Errorf("Expected chunk message, got %v", chunkMsg.Type)
	}
	if chunkMsg.Content != "chunk1" {
		t.Errorf("Chunk content = %v, want 'chunk1'", chunkMsg.Content)
	}

	// Read done message
	var doneMsg ServerMessage
	if err := ws.ReadJSON(&doneMsg); err != nil {
		t.Fatalf("Failed to read done: %v", err)
	}
	if doneMsg.Type != MessageTypeDone {
		t.Errorf("Expected done message, got %v", doneMsg.Type)
	}
	if doneMsg.Content != "done" {
		t.Errorf("Done content = %v, want 'done'", doneMsg.Content)
	}
}

func TestServerSessionResumption(t *testing.T) {
	handler := &mockHandler{}
	_, ts := newTestServer(t, handler)

	// First connection
	ws1, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Send message to get session ID
	clientMsg := ClientMessage{
		Type:    MessageTypeMessage,
		Content: "Hello",
	}
	if err := ws1.WriteJSON(clientMsg); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Read connected message to get session ID
	var connectedMsg ServerMessage
	if err := ws1.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("Failed to read connected: %v", err)
	}
	sessionID := connectedMsg.SessionID
	if sessionID == "" {
		t.Fatal("Session ID should not be empty")
	}

	// Drain the done message
	var doneMsg ServerMessage
	if err := ws1.ReadJSON(&doneMsg); err != nil {
		t.Fatalf("Failed to read done: %v", err)
	}

	// Close first connection
	_ = ws1.Close()

	// Second connection with session resumption
	ws2, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws2.Close() }()

	// Send message with existing session ID
	clientMsg2 := ClientMessage{
		Type:      MessageTypeMessage,
		SessionID: sessionID,
		Content:   "Resume",
	}
	if err := ws2.WriteJSON(clientMsg2); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Should not receive connected message for resumed session
	// Should receive done message directly
	var msg ServerMessage
	if err := ws2.ReadJSON(&msg); err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}
	if msg.Type != MessageTypeDone {
		t.Errorf("Expected done message, got %v", msg.Type)
	}
}

func TestServerToolCallHandling(t *testing.T) {
	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, _ *ClientMessage, writer ResponseWriter) error {
			toolCall := &ToolCallInfo{
				ID:   "tool-1",
				Name: "search",
				Arguments: map[string]interface{}{
					"query": "test",
				},
			}
			if err := writer.WriteToolCall(toolCall); err != nil {
				return err
			}

			result := &ToolResultInfo{
				ID:     "tool-1",
				Result: "found results",
			}
			if err := writer.WriteToolResult(result); err != nil {
				return err
			}

			return writer.WriteDone("complete")
		},
	}

	_, ts := newTestServer(t, handler)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Send message
	if err := ws.WriteJSON(ClientMessage{Type: MessageTypeMessage, Content: "test"}); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	// Read connected
	var connectedMsg ServerMessage
	if err := ws.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("Failed to read connected: %v", err)
	}

	// Read tool call
	var toolCallMsg ServerMessage
	if err := ws.ReadJSON(&toolCallMsg); err != nil {
		t.Fatalf("Failed to read tool call: %v", err)
	}
	if toolCallMsg.Type != MessageTypeToolCall {
		t.Errorf("Expected tool_call, got %v", toolCallMsg.Type)
	}
	if toolCallMsg.ToolCall == nil {
		t.Fatal("ToolCall should not be nil")
	}
	if toolCallMsg.ToolCall.Name != "search" {
		t.Errorf("ToolCall.Name = %v, want 'search'", toolCallMsg.ToolCall.Name)
	}

	// Read tool result
	var toolResultMsg ServerMessage
	if err := ws.ReadJSON(&toolResultMsg); err != nil {
		t.Fatalf("Failed to read tool result: %v", err)
	}
	if toolResultMsg.Type != MessageTypeToolResult {
		t.Errorf("Expected tool_result, got %v", toolResultMsg.Type)
	}

	// Read done
	var doneMsg ServerMessage
	if err := ws.ReadJSON(&doneMsg); err != nil {
		t.Fatalf("Failed to read done: %v", err)
	}
	if doneMsg.Type != MessageTypeDone {
		t.Errorf("Expected done, got %v", doneMsg.Type)
	}
}

func TestServerErrorHandling(t *testing.T) {
	_, ts := newTestServer(t, nil)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Send invalid JSON
	if err := ws.WriteMessage(websocket.TextMessage, []byte("not json")); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	// Should receive error message
	var errorMsg ServerMessage
	if err := ws.ReadJSON(&errorMsg); err != nil {
		t.Fatalf("Failed to read error: %v", err)
	}
	if errorMsg.Type != MessageTypeError {
		t.Errorf("Expected error message, got %v", errorMsg.Type)
	}
	if errorMsg.Error == nil {
		t.Fatal("Error should not be nil")
	}
	if errorMsg.Error.Code != ErrorCodeInvalidMessage {
		t.Errorf("Error code = %v, want %v", errorMsg.Error.Code, ErrorCodeInvalidMessage)
	}
}

func TestServerShutdown(t *testing.T) {
	server, ts := newTestServer(t, nil)

	// Connect
	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Wait for connection to be registered (may take a moment on CI)
	var count int
	for i := 0; i < 50; i++ {
		count = server.ConnectionCount()
		if count == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if count != 1 {
		t.Errorf("ConnectionCount = %v, want 1", count)
	}

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}

func TestServerRejectsAfterShutdown(t *testing.T) {
	server, ts := newTestServer(t, nil)

	// Shutdown first
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	// Try to connect - should fail
	_, resp, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err == nil {
		t.Fatal("Expected error when connecting after shutdown")
	}
	if resp != nil && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %v", resp.StatusCode)
	}
}

func TestServerDefaultHandler(t *testing.T) {
	// Create server with nil handler
	_, ts := newTestServer(t, nil)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Send message
	if err := ws.WriteJSON(ClientMessage{Type: MessageTypeMessage, Content: "test"}); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	// Read connected
	var connectedMsg ServerMessage
	if err := ws.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("Failed to read connected: %v", err)
	}

	// Should get default response
	var doneMsg ServerMessage
	if err := ws.ReadJSON(&doneMsg); err != nil {
		t.Fatalf("Failed to read done: %v", err)
	}
	if doneMsg.Type != MessageTypeDone {
		t.Errorf("Expected done, got %v", doneMsg.Type)
	}
	if doneMsg.Content != "Handler not configured" {
		t.Errorf("Content = %v, want 'Handler not configured'", doneMsg.Content)
	}
}

func TestClientMessageUnmarshal(t *testing.T) {
	jsonStr := `{"type":"message","session_id":"sess-1","content":"hello","metadata":{"key":"value"}}`

	var msg ClientMessage
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if msg.Type != MessageTypeMessage {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeMessage)
	}
	if msg.SessionID != "sess-1" {
		t.Errorf("SessionID = %v, want sess-1", msg.SessionID)
	}
	if msg.Content != "hello" {
		t.Errorf("Content = %v, want hello", msg.Content)
	}
	if msg.Metadata["key"] != "value" {
		t.Errorf("Metadata[key] = %v, want value", msg.Metadata["key"])
	}
}
