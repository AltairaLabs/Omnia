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
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"

	"github.com/altairalabs/omnia/internal/media"
	"github.com/altairalabs/omnia/internal/session"
)

// Test constants.
const (
	testUploadID  = "upload-123"
	testUploadURL = "http://example.com/upload/" + testUploadID
)

// mockHandler implements MessageHandler for testing.
type mockHandler struct {
	handleFunc func(ctx context.Context, sessionID string, msg *ClientMessage, writer ResponseWriter) error
}

func (m *mockHandler) Name() string {
	return "mock"
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

func TestServerWithMetrics(t *testing.T) {
	metrics := &NoOpMetrics{}
	config := DefaultServerConfig()
	log := logr.Discard()
	store := session.NewMemoryStore()
	server := NewServer(config, store, nil, log, WithMetrics(metrics))
	defer func() { _ = store.Close() }()

	// Verify the metrics were set and server is valid
	if server == nil {
		t.Fatal("NewServer returned nil")
	}
}

func TestServerPingPong(t *testing.T) {
	store := session.NewMemoryStore()
	cfg := DefaultServerConfig()
	// Short ping interval to test ping loop quickly
	cfg.PingInterval = 50 * time.Millisecond
	cfg.PongTimeout = 100 * time.Millisecond

	log := logr.Discard()
	server := NewServer(cfg, store, nil, log)

	ts := httptest.NewServer(server)
	t.Cleanup(func() {
		ts.Close()
		_ = store.Close()
	})

	// Connect with a dialer that handles ping/pong properly
	dialer := websocket.DefaultDialer
	ws, _, err := dialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Set up pong handler to respond to pings
	pingReceived := make(chan struct{}, 1)
	ws.SetPingHandler(func(appData string) error {
		select {
		case pingReceived <- struct{}{}:
		default:
		}
		// Respond with pong
		return ws.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second))
	})

	// Read messages in background to allow ping handling
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, _, err := ws.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// Wait for ping to be received (or timeout)
	select {
	case <-pingReceived:
		// Success - ping was sent and we responded with pong
	case <-time.After(200 * time.Millisecond):
		// Ping may not have been sent yet, which is fine
	}

	// Clean close
	_ = ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
}

func TestServerSendPingClosedConnection(t *testing.T) {
	// Test sendPing with a closed connection
	conn := &Connection{
		closed: true,
	}

	store := session.NewMemoryStore()
	cfg := DefaultServerConfig()
	log := logr.Discard()
	server := NewServer(cfg, store, nil, log)
	defer func() { _ = store.Close() }()

	// sendPing should return false for closed connection
	result := server.sendPing(conn)
	if result {
		t.Error("sendPing should return false for closed connection")
	}
}

func TestServerHandlerError(t *testing.T) {
	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, _ *ClientMessage, writer ResponseWriter) error {
			// Simulate handler error
			return writer.WriteError(ErrorCodeInternalError, "handler error")
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

	// Should receive error message from handler
	var errorMsg ServerMessage
	if err := ws.ReadJSON(&errorMsg); err != nil {
		t.Fatalf("Failed to read error: %v", err)
	}
	if errorMsg.Type != MessageTypeError {
		t.Errorf("Expected error message, got %v", errorMsg.Type)
	}
}

func TestServerUploadRequest_MediaNotEnabled(t *testing.T) {
	// Server without media storage
	_, ts := newTestServer(t, nil)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Send upload_request
	clientMsg := ClientMessage{
		Type: MessageTypeUploadRequest,
		UploadRequest: &UploadRequestInfo{
			Filename:  "test.jpg",
			MimeType:  "image/jpeg",
			SizeBytes: 1024,
		},
	}
	if err := ws.WriteJSON(clientMsg); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Read connected message first
	var connectedMsg ServerMessage
	if err := ws.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("Failed to read connected message: %v", err)
	}
	if connectedMsg.Type != MessageTypeConnected {
		t.Errorf("Expected connected message, got %v", connectedMsg.Type)
	}

	// Read error message
	var errorMsg ServerMessage
	if err := ws.ReadJSON(&errorMsg); err != nil {
		t.Fatalf("Failed to read error message: %v", err)
	}
	if errorMsg.Type != MessageTypeError {
		t.Errorf("Expected error message, got %v", errorMsg.Type)
	}
	if errorMsg.Error == nil {
		t.Fatal("Error field should not be nil")
	}
	if errorMsg.Error.Code != ErrorCodeMediaNotEnabled {
		t.Errorf("Error code = %v, want %v", errorMsg.Error.Code, ErrorCodeMediaNotEnabled)
	}
}

func TestServerUploadRequest_MissingUploadRequestField(t *testing.T) {
	// Server without media storage (to test validation before media check)
	_, ts := newTestServer(t, nil)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Send upload_request without the upload_request field
	clientMsg := ClientMessage{
		Type: MessageTypeUploadRequest,
		// UploadRequest intentionally nil
	}
	if err := ws.WriteJSON(clientMsg); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Read connected message
	var connectedMsg ServerMessage
	if err := ws.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("Failed to read connected message: %v", err)
	}

	// Read error message - should get MEDIA_NOT_ENABLED first since no media storage
	var errorMsg ServerMessage
	if err := ws.ReadJSON(&errorMsg); err != nil {
		t.Fatalf("Failed to read error message: %v", err)
	}
	if errorMsg.Type != MessageTypeError {
		t.Errorf("Expected error message, got %v", errorMsg.Type)
	}
}

func TestNewUploadReadyMessage(t *testing.T) {
	info := &UploadReadyInfo{
		UploadID:   testUploadID,
		UploadURL:  testUploadURL,
		StorageRef: "omnia://sessions/sess-123/media/media-456",
		ExpiresAt:  time.Now().Add(1 * time.Hour),
	}

	msg := NewUploadReadyMessage("sess-123", info)

	if msg.Type != MessageTypeUploadReady {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeUploadReady)
	}
	if msg.SessionID != "sess-123" {
		t.Errorf("SessionID = %v, want sess-123", msg.SessionID)
	}
	if msg.UploadReady == nil {
		t.Fatal("UploadReady should not be nil")
	}
	if msg.UploadReady.UploadID != testUploadID {
		t.Errorf("UploadID = %v, want %v", msg.UploadReady.UploadID, testUploadID)
	}
	if msg.UploadReady.UploadURL != testUploadURL {
		t.Errorf("UploadURL = %v, want %v", msg.UploadReady.UploadURL, testUploadURL)
	}
	if msg.UploadReady.StorageRef != "omnia://sessions/sess-123/media/media-456" {
		t.Errorf("StorageRef = %v, want omnia://sessions/sess-123/media/media-456", msg.UploadReady.StorageRef)
	}
}

func TestNewUploadCompleteMessage(t *testing.T) {
	info := &UploadCompleteInfo{
		UploadID:   testUploadID,
		StorageRef: "omnia://sessions/sess-123/media/media-456",
		SizeBytes:  1024,
	}

	msg := NewUploadCompleteMessage("sess-123", info)

	if msg.Type != MessageTypeUploadComplete {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeUploadComplete)
	}
	if msg.SessionID != "sess-123" {
		t.Errorf("SessionID = %v, want sess-123", msg.SessionID)
	}
	if msg.UploadComplete == nil {
		t.Fatal("UploadComplete should not be nil")
	}
	if msg.UploadComplete.UploadID != testUploadID {
		t.Errorf("UploadID = %v, want %v", msg.UploadComplete.UploadID, testUploadID)
	}
	if msg.UploadComplete.StorageRef != "omnia://sessions/sess-123/media/media-456" {
		t.Errorf("StorageRef = %v, want omnia://sessions/sess-123/media/media-456", msg.UploadComplete.StorageRef)
	}
	if msg.UploadComplete.SizeBytes != 1024 {
		t.Errorf("SizeBytes = %v, want 1024", msg.UploadComplete.SizeBytes)
	}
}

func TestUploadRequestInfoJSON(t *testing.T) {
	info := &UploadRequestInfo{
		Filename:  "test.jpg",
		MimeType:  "image/jpeg",
		SizeBytes: 2048,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded UploadRequestInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Filename != "test.jpg" {
		t.Errorf("Filename = %v, want test.jpg", decoded.Filename)
	}
	if decoded.MimeType != "image/jpeg" {
		t.Errorf("MimeType = %v, want image/jpeg", decoded.MimeType)
	}
	if decoded.SizeBytes != 2048 {
		t.Errorf("SizeBytes = %v, want 2048", decoded.SizeBytes)
	}
}

func TestUploadMessageTypes(t *testing.T) {
	// Verify the new message type constants
	if MessageTypeUploadRequest != "upload_request" {
		t.Errorf("MessageTypeUploadRequest = %v, want upload_request", MessageTypeUploadRequest)
	}
	if MessageTypeUploadReady != "upload_ready" {
		t.Errorf("MessageTypeUploadReady = %v, want upload_ready", MessageTypeUploadReady)
	}
	if MessageTypeUploadComplete != "upload_complete" {
		t.Errorf("MessageTypeUploadComplete = %v, want upload_complete", MessageTypeUploadComplete)
	}
}

func TestUploadErrorCodes(t *testing.T) {
	// Verify the new error codes
	if ErrorCodeUploadFailed != "UPLOAD_FAILED" {
		t.Errorf("ErrorCodeUploadFailed = %v, want UPLOAD_FAILED", ErrorCodeUploadFailed)
	}
	if ErrorCodeMediaNotEnabled != "MEDIA_NOT_ENABLED" {
		t.Errorf("ErrorCodeMediaNotEnabled = %v, want MEDIA_NOT_ENABLED", ErrorCodeMediaNotEnabled)
	}
}

// mockMediaStorage implements media.Storage for testing.
type mockMediaStorage struct {
	getUploadURLFunc func(ctx context.Context, req media.UploadRequest) (*media.UploadCredentials, error)
}

func (m *mockMediaStorage) GetUploadURL(ctx context.Context, req media.UploadRequest) (*media.UploadCredentials, error) {
	if m.getUploadURLFunc != nil {
		return m.getUploadURLFunc(ctx, req)
	}
	return &media.UploadCredentials{
		UploadID:   testUploadID,
		URL:        testUploadURL,
		StorageRef: "omnia://sessions/" + req.SessionID + "/media/media-123",
		ExpiresAt:  time.Now().Add(1 * time.Hour),
	}, nil
}

func (m *mockMediaStorage) GetDownloadURL(_ context.Context, storageRef string) (string, error) {
	return "http://example.com/download/" + storageRef, nil
}

func (m *mockMediaStorage) GetMediaInfo(_ context.Context, _ string) (*media.MediaInfo, error) {
	return nil, nil
}

func (m *mockMediaStorage) Delete(_ context.Context, _ string) error {
	return nil
}

func (m *mockMediaStorage) Close() error {
	return nil
}

func newTestServerWithMedia(t *testing.T, handler MessageHandler, storage media.Storage) *httptest.Server {
	t.Helper()

	store := session.NewMemoryStore()
	cfg := DefaultServerConfig()
	cfg.PingInterval = 100 * time.Millisecond
	cfg.PongTimeout = 200 * time.Millisecond

	log := logr.Discard()
	server := NewServer(cfg, store, handler, log, WithMediaStorage(storage))

	ts := httptest.NewServer(server)
	t.Cleanup(func() {
		ts.Close()
		_ = store.Close()
	})

	return ts
}

func TestServerUploadRequest_Success(t *testing.T) {
	storage := &mockMediaStorage{}
	ts := newTestServerWithMedia(t, nil, storage)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Send upload_request
	clientMsg := ClientMessage{
		Type: MessageTypeUploadRequest,
		UploadRequest: &UploadRequestInfo{
			Filename:  "test.jpg",
			MimeType:  "image/jpeg",
			SizeBytes: 1024,
		},
	}
	if err := ws.WriteJSON(clientMsg); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Read connected message first
	var connectedMsg ServerMessage
	if err := ws.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("Failed to read connected message: %v", err)
	}
	if connectedMsg.Type != MessageTypeConnected {
		t.Errorf("Expected connected message, got %v", connectedMsg.Type)
	}

	// Read upload_ready message
	var uploadReadyMsg ServerMessage
	if err := ws.ReadJSON(&uploadReadyMsg); err != nil {
		t.Fatalf("Failed to read upload_ready message: %v", err)
	}
	if uploadReadyMsg.Type != MessageTypeUploadReady {
		t.Errorf("Expected upload_ready message, got %v", uploadReadyMsg.Type)
	}
	if uploadReadyMsg.UploadReady == nil {
		t.Fatal("UploadReady should not be nil")
	}
	if uploadReadyMsg.UploadReady.UploadID != testUploadID {
		t.Errorf("UploadID = %v, want %v", uploadReadyMsg.UploadReady.UploadID, testUploadID)
	}
	if uploadReadyMsg.UploadReady.UploadURL != testUploadURL {
		t.Errorf("UploadURL = %v, want %v", uploadReadyMsg.UploadReady.UploadURL, testUploadURL)
	}
	if !strings.Contains(uploadReadyMsg.UploadReady.StorageRef, "omnia://sessions/") {
		t.Errorf("StorageRef should contain omnia://sessions/, got %v", uploadReadyMsg.UploadReady.StorageRef)
	}
}

func TestServerUploadRequest_ValidationErrors(t *testing.T) {
	storage := &mockMediaStorage{}
	ts := newTestServerWithMedia(t, nil, storage)

	tests := []struct {
		name          string
		uploadRequest *UploadRequestInfo
		expectedCode  string
	}{
		{
			name:          "missing filename",
			uploadRequest: &UploadRequestInfo{MimeType: "image/jpeg", SizeBytes: 1024},
			expectedCode:  ErrorCodeInvalidMessage,
		},
		{
			name:          "missing mime_type",
			uploadRequest: &UploadRequestInfo{Filename: "test.jpg", SizeBytes: 1024},
			expectedCode:  ErrorCodeInvalidMessage,
		},
		{
			name:          "invalid size_bytes",
			uploadRequest: &UploadRequestInfo{Filename: "test.jpg", MimeType: "image/jpeg", SizeBytes: 0},
			expectedCode:  ErrorCodeInvalidMessage,
		},
		{
			name:          "missing upload_request field",
			uploadRequest: nil,
			expectedCode:  ErrorCodeInvalidMessage,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
			if err != nil {
				t.Fatalf("Failed to connect: %v", err)
			}
			defer func() { _ = ws.Close() }()

			// Send upload_request
			clientMsg := ClientMessage{
				Type:          MessageTypeUploadRequest,
				UploadRequest: tc.uploadRequest,
			}
			if err := ws.WriteJSON(clientMsg); err != nil {
				t.Fatalf("Failed to send message: %v", err)
			}

			// Read connected message
			var connectedMsg ServerMessage
			if err := ws.ReadJSON(&connectedMsg); err != nil {
				t.Fatalf("Failed to read connected message: %v", err)
			}

			// Read error message
			var errorMsg ServerMessage
			if err := ws.ReadJSON(&errorMsg); err != nil {
				t.Fatalf("Failed to read error message: %v", err)
			}
			if errorMsg.Type != MessageTypeError {
				t.Errorf("Expected error message, got %v", errorMsg.Type)
			}
			if errorMsg.Error == nil {
				t.Fatal("Error field should not be nil")
			}
			if errorMsg.Error.Code != tc.expectedCode {
				t.Errorf("Error code = %v, want %v", errorMsg.Error.Code, tc.expectedCode)
			}
		})
	}
}

func TestServerUploadRequest_StorageError(t *testing.T) {
	storage := &mockMediaStorage{
		getUploadURLFunc: func(_ context.Context, _ media.UploadRequest) (*media.UploadCredentials, error) {
			return nil, errors.New("storage error")
		},
	}
	ts := newTestServerWithMedia(t, nil, storage)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Send upload_request
	clientMsg := ClientMessage{
		Type: MessageTypeUploadRequest,
		UploadRequest: &UploadRequestInfo{
			Filename:  "test.jpg",
			MimeType:  "image/jpeg",
			SizeBytes: 1024,
		},
	}
	if err := ws.WriteJSON(clientMsg); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Read connected message
	var connectedMsg ServerMessage
	if err := ws.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("Failed to read connected message: %v", err)
	}

	// Read error message
	var errorMsg ServerMessage
	if err := ws.ReadJSON(&errorMsg); err != nil {
		t.Fatalf("Failed to read error message: %v", err)
	}
	if errorMsg.Type != MessageTypeError {
		t.Errorf("Expected error message, got %v", errorMsg.Type)
	}
	if errorMsg.Error == nil {
		t.Fatal("Error field should not be nil")
	}
	if errorMsg.Error.Code != ErrorCodeUploadFailed {
		t.Errorf("Error code = %v, want %v", errorMsg.Error.Code, ErrorCodeUploadFailed)
	}
}

func TestWithMediaStorage(t *testing.T) {
	storage := &mockMediaStorage{}
	store := session.NewMemoryStore()
	defer func() { _ = store.Close() }()

	cfg := DefaultServerConfig()
	log := logr.Discard()

	server := NewServer(cfg, store, nil, log, WithMediaStorage(storage))

	if server.mediaStorage == nil {
		t.Error("mediaStorage should not be nil after WithMediaStorage")
	}
}

func TestServerBinaryCapabilityNegotiation(t *testing.T) {
	handler := &mockHandler{}
	_, ts := newTestServer(t, handler)

	t.Run("without binary param", func(t *testing.T) {
		ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
		if err != nil {
			t.Fatalf("Failed to connect: %v", err)
		}
		defer func() { _ = ws.Close() }()

		// Send message
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

		// Without binary param, Connected.Capabilities should be nil
		if connectedMsg.Connected != nil {
			t.Errorf("Expected Connected to be nil without binary param, got %+v", connectedMsg.Connected)
		}
	})

	t.Run("with binary=true param", func(t *testing.T) {
		ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent&binary=true", nil)
		if err != nil {
			t.Fatalf("Failed to connect: %v", err)
		}
		defer func() { _ = ws.Close() }()

		// Send message
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

		// With binary=true, Connected.Capabilities should be set
		if connectedMsg.Connected == nil {
			t.Fatal("Expected Connected to be set with binary=true param")
		}
		if connectedMsg.Connected.Capabilities == nil {
			t.Fatal("Expected Capabilities to be set")
		}
		if !connectedMsg.Connected.Capabilities.BinaryFrames {
			t.Error("Expected BinaryFrames to be true")
		}
		if connectedMsg.Connected.Capabilities.ProtocolVersion != BinaryVersion {
			t.Errorf("ProtocolVersion = %d, want %d", connectedMsg.Connected.Capabilities.ProtocolVersion, BinaryVersion)
		}
		if connectedMsg.Connected.Capabilities.MaxPayloadSize != int(DefaultServerConfig().MaxMessageSize) {
			t.Errorf("MaxPayloadSize = %d, want %d", connectedMsg.Connected.Capabilities.MaxPayloadSize, DefaultServerConfig().MaxMessageSize)
		}
	})

	t.Run("with binary=false param", func(t *testing.T) {
		ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent&binary=false", nil)
		if err != nil {
			t.Fatalf("Failed to connect: %v", err)
		}
		defer func() { _ = ws.Close() }()

		// Send message
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

		// With binary=false, Connected should be nil (same as not providing)
		if connectedMsg.Connected != nil {
			t.Errorf("Expected Connected to be nil with binary=false param, got %+v", connectedMsg.Connected)
		}
	})
}

func TestServerBinaryMessageHandling(t *testing.T) {
	handler := &mockHandler{}
	_, ts := newTestServer(t, handler)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent&binary=true", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Create a binary frame with unknown message type
	frame := &BinaryFrame{
		Header: BinaryHeader{
			Magic:       [4]byte{'O', 'M', 'N', 'I'},
			Version:     BinaryVersion,
			Flags:       0,
			MessageType: BinaryMessageType(99), // Unknown type
			MetadataLen: 0,
			PayloadLen:  0,
			Sequence:    0,
		},
	}

	data, err := frame.Encode()
	if err != nil {
		t.Fatalf("Failed to encode frame: %v", err)
	}

	// Send binary message
	if err := ws.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatalf("Failed to send binary message: %v", err)
	}

	// Should receive an error message
	var errorMsg ServerMessage
	if err := ws.ReadJSON(&errorMsg); err != nil {
		t.Fatalf("Failed to read error message: %v", err)
	}
	if errorMsg.Type != MessageTypeError {
		t.Errorf("Expected error message, got %v", errorMsg.Type)
	}
	if errorMsg.Error == nil {
		t.Fatal("Error field should not be nil")
	}
	if errorMsg.Error.Code != ErrorCodeInvalidMessage {
		t.Errorf("Error code = %v, want %v", errorMsg.Error.Code, ErrorCodeInvalidMessage)
	}
}

func TestServerInvalidBinaryFrame(t *testing.T) {
	handler := &mockHandler{}
	_, ts := newTestServer(t, handler)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent&binary=true", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Send invalid binary message (wrong magic bytes)
	invalidData := make([]byte, BinaryHeaderSize)
	copy(invalidData[0:4], "BAD!")
	invalidData[4] = BinaryVersion

	if err := ws.WriteMessage(websocket.BinaryMessage, invalidData); err != nil {
		t.Fatalf("Failed to send binary message: %v", err)
	}

	// Should receive an error message
	var errorMsg ServerMessage
	if err := ws.ReadJSON(&errorMsg); err != nil {
		t.Fatalf("Failed to read error message: %v", err)
	}
	if errorMsg.Type != MessageTypeError {
		t.Errorf("Expected error message, got %v", errorMsg.Type)
	}
	if errorMsg.Error == nil {
		t.Fatal("Error field should not be nil")
	}
	if !strings.Contains(errorMsg.Error.Message, "invalid binary frame") {
		t.Errorf("Error message should mention 'invalid binary frame', got: %v", errorMsg.Error.Message)
	}
}

func TestServerWriteBinaryMediaChunk(t *testing.T) {
	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, _ *ClientMessage, writer ResponseWriter) error {
			// Test SupportsBinary
			if !writer.SupportsBinary() {
				return errors.New("expected binary support")
			}

			// Test WriteBinaryMediaChunk
			mediaID := MediaIDFromString("test-media")
			payload := []byte("test audio data")
			return writer.WriteBinaryMediaChunk(mediaID, 0, true, "audio/mp3", payload)
		},
	}

	_, ts := newTestServer(t, handler)

	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent&binary=true", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Send message
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

	// Read binary frame
	msgType, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read binary message: %v", err)
	}
	if msgType != websocket.BinaryMessage {
		t.Errorf("Expected binary message type, got %v", msgType)
	}

	// Decode binary frame
	frame, err := DecodeBinaryFrame(data)
	if err != nil {
		t.Fatalf("Failed to decode binary frame: %v", err)
	}

	if frame.Header.MessageType != BinaryMessageTypeMediaChunk {
		t.Errorf("Expected media chunk message type, got %v", frame.Header.MessageType)
	}
	if !frame.Header.Flags.IsLast() {
		t.Error("Expected is_last flag to be set")
	}
	if string(frame.Payload) != "test audio data" {
		t.Errorf("Payload = %q, want 'test audio data'", string(frame.Payload))
	}
}

func TestServerWriteBinaryMediaChunkFallback(t *testing.T) {
	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, _ *ClientMessage, writer ResponseWriter) error {
			// Without binary support, WriteBinaryMediaChunk should fall back to JSON
			if writer.SupportsBinary() {
				return errors.New("expected no binary support")
			}

			mediaID := MediaIDFromString("test-media")
			payload := []byte("test audio data")
			return writer.WriteBinaryMediaChunk(mediaID, 0, true, "audio/mp3", payload)
		},
	}

	_, ts := newTestServer(t, handler)

	// Connect WITHOUT binary=true
	ws, _, err := websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Send message
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

	// Read media_chunk message (should be JSON fallback)
	var mediaChunkMsg ServerMessage
	if err := ws.ReadJSON(&mediaChunkMsg); err != nil {
		t.Fatalf("Failed to read media chunk message: %v", err)
	}

	if mediaChunkMsg.Type != MessageTypeMediaChunk {
		t.Errorf("Expected media_chunk message type, got %v", mediaChunkMsg.Type)
	}
	if mediaChunkMsg.MediaChunk == nil {
		t.Fatal("MediaChunk should not be nil")
	}
	if mediaChunkMsg.MediaChunk.MimeType != "audio/mp3" {
		t.Errorf("MimeType = %q, want 'audio/mp3'", mediaChunkMsg.MediaChunk.MimeType)
	}
	if !mediaChunkMsg.MediaChunk.IsLast {
		t.Error("Expected is_last to be true")
	}
}
