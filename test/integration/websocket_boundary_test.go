//go:build integration

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

// Package integration contains integration tests that verify communication
// between facade and runtime containers without requiring a Kubernetes cluster.
package integration

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/internal/session"
)

// wsURL converts an HTTP URL to a WebSocket URL.
func wsURL(httpURL string) string {
	return strings.Replace(httpURL, "http://", "ws://", 1)
}

// newTestFacade creates a facade Server with an in-memory session store.
func newTestFacade(t *testing.T, handler facade.MessageHandler) (*facade.Server, *httptest.Server) {
	t.Helper()

	store := session.NewMemoryStore()
	cfg := facade.DefaultServerConfig()
	cfg.PingInterval = 100 * time.Millisecond
	cfg.PongTimeout = 200 * time.Millisecond

	server := facade.NewServer(cfg, store, handler, logr.Discard())
	ts := httptest.NewServer(server)
	t.Cleanup(func() {
		ts.Close()
		_ = store.Close()
	})

	return server, ts
}

// connectWS opens a WebSocket connection and reads the initial "connected" message.
// Returns the WebSocket connection and the session ID.
func connectWS(t *testing.T, url string) (*websocket.Conn, string) {
	t.Helper()

	ws, _, err := websocket.DefaultDialer.Dial(url+"?agent=test-agent", nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })

	// Read the eager "connected" message
	var msg facade.ServerMessage
	require.NoError(t, ws.ReadJSON(&msg))
	require.Equal(t, facade.MessageTypeConnected, msg.Type)
	require.NotEmpty(t, msg.SessionID)

	return ws, msg.SessionID
}

// sendMessage sends a text message over WebSocket with the session ID.
func sendMessage(t *testing.T, ws *websocket.Conn, sessionID, content string) {
	t.Helper()
	require.NoError(t, ws.WriteJSON(facade.ClientMessage{
		Type:      facade.MessageTypeMessage,
		SessionID: sessionID,
		Content:   content,
	}))
}

// readMessage reads and returns the next ServerMessage from WebSocket.
func readMessage(t *testing.T, ws *websocket.Conn) facade.ServerMessage {
	t.Helper()
	var msg facade.ServerMessage
	require.NoError(t, ws.ReadJSON(&msg))
	return msg
}

// --- Test handlers ---

// echoHandler echoes the user's message back as a done response.
type echoHandler struct{}

func (h *echoHandler) Name() string { return "echo" }
func (h *echoHandler) HandleMessage(_ context.Context, _ string, msg *facade.ClientMessage, w facade.ResponseWriter) error {
	return w.WriteDone("echo: " + msg.Content)
}

// streamHandler sends multiple chunks before a done message.
type streamHandler struct{}

func (h *streamHandler) Name() string { return "stream" }
func (h *streamHandler) HandleMessage(_ context.Context, _ string, _ *facade.ClientMessage, w facade.ResponseWriter) error {
	for _, chunk := range []string{"Hello", " ", "world"} {
		if err := w.WriteChunk(chunk); err != nil {
			return err
		}
	}
	return w.WriteDone("Hello world")
}

// clientToolHandler sends a client-side tool call and waits for a result via the
// toolResultCh channel. This simulates the runtime asking the dashboard to execute
// a tool and waiting for the response.
type clientToolHandler struct {
	toolResultCh chan *facade.ClientToolResultInfo
}

func (h *clientToolHandler) Name() string { return "client-tool" }
func (h *clientToolHandler) HandleMessage(_ context.Context, _ string, _ *facade.ClientMessage, w facade.ResponseWriter) error {
	// Send a client-side tool call to the dashboard
	if err := w.WriteToolCall(&facade.ToolCallInfo{
		ID:             "tool-1",
		Name:           "get_location",
		Arguments:      map[string]interface{}{"query": "current"},
		ConsentMessage: "Allow location access?",
		Categories:     []string{"location"},
	}); err != nil {
		return err
	}

	// Wait for the client to respond (simulates runtime suspension)
	result := <-h.toolResultCh

	if result.Error != "" {
		return w.WriteDone("Tool rejected: " + result.Error)
	}
	return w.WriteDone("Tool result received")
}

// SendToolResult implements ClientToolRouter for the test handler.
func (h *clientToolHandler) SendToolResult(_ string, result *facade.ClientToolResultInfo) bool {
	select {
	case h.toolResultCh <- result:
		return true
	default:
		return false
	}
}

// errorHandler always returns an error.
type errorHandler struct{}

func (h *errorHandler) Name() string { return "error" }
func (h *errorHandler) HandleMessage(_ context.Context, _ string, _ *facade.ClientMessage, w facade.ResponseWriter) error {
	return w.WriteError("TEST_ERROR", "something went wrong")
}

// --- Tests ---

// TestWebSocketProtocol exercises the WebSocket protocol boundary between
// the dashboard and facade using table-driven test cases. Each case verifies
// a different protocol flow over a real WebSocket connection.
func TestWebSocketProtocol(t *testing.T) {
	t.Run("connect and receive session ID", func(t *testing.T) {
		_, ts := newTestFacade(t, &echoHandler{})
		ws, sessionID := connectWS(t, wsURL(ts.URL))
		_ = ws
		assert.NotEmpty(t, sessionID)
	})

	t.Run("echo round-trip", func(t *testing.T) {
		_, ts := newTestFacade(t, &echoHandler{})
		ws, sid := connectWS(t, wsURL(ts.URL))

		sendMessage(t, ws, sid, "hello")

		msg := readMessage(t, ws)
		assert.Equal(t, facade.MessageTypeDone, msg.Type)
		assert.Equal(t, "echo: hello", msg.Content)
	})

	t.Run("streaming chunks then done", func(t *testing.T) {
		_, ts := newTestFacade(t, &streamHandler{})
		ws, sid := connectWS(t, wsURL(ts.URL))

		sendMessage(t, ws, sid, "test")

		// Read three chunks
		var chunks []string
		for i := 0; i < 3; i++ {
			msg := readMessage(t, ws)
			require.Equal(t, facade.MessageTypeChunk, msg.Type)
			chunks = append(chunks, msg.Content)
		}
		assert.Equal(t, []string{"Hello", " ", "world"}, chunks)

		// Read done
		done := readMessage(t, ws)
		assert.Equal(t, facade.MessageTypeDone, done.Type)
		assert.Equal(t, "Hello world", done.Content)
	})

	t.Run("error response", func(t *testing.T) {
		_, ts := newTestFacade(t, &errorHandler{})
		ws, sid := connectWS(t, wsURL(ts.URL))

		sendMessage(t, ws, sid, "fail")

		msg := readMessage(t, ws)
		assert.Equal(t, facade.MessageTypeError, msg.Type)
		require.NotNil(t, msg.Error)
		assert.Equal(t, "TEST_ERROR", msg.Error.Code)
		assert.Equal(t, "something went wrong", msg.Error.Message)
	})

	t.Run("client tool call round-trip", func(t *testing.T) {
		handler := &clientToolHandler{
			toolResultCh: make(chan *facade.ClientToolResultInfo, 1),
		}
		server, ts := newTestFacade(t, handler)
		_ = server // handler itself implements ClientToolRouter

		ws, sid := connectWS(t, wsURL(ts.URL))

		sendMessage(t, ws, sid, "where am I?")

		// Should receive a tool_call message
		toolCallMsg := readMessage(t, ws)
		assert.Equal(t, facade.MessageTypeToolCall, toolCallMsg.Type)
		require.NotNil(t, toolCallMsg.ToolCall)
		assert.Equal(t, "tool-1", toolCallMsg.ToolCall.ID)
		assert.Equal(t, "get_location", toolCallMsg.ToolCall.Name)
		assert.Equal(t, "Allow location access?", toolCallMsg.ToolCall.ConsentMessage)
		assert.Equal(t, []string{"location"}, toolCallMsg.ToolCall.Categories)

		// Send tool result back
		require.NoError(t, ws.WriteJSON(facade.ClientMessage{
			Type:      facade.MessageTypeToolResult,
			SessionID: sid,
			ToolResult: &facade.ClientToolResultInfo{
				CallID: "tool-1",
				Result: map[string]string{"city": "Denver"},
			},
		}))

		// Should receive done after tool result is processed
		doneMsg := readMessage(t, ws)
		assert.Equal(t, facade.MessageTypeDone, doneMsg.Type)
		assert.Equal(t, "Tool result received", doneMsg.Content)
	})

	t.Run("client tool call rejection", func(t *testing.T) {
		handler := &clientToolHandler{
			toolResultCh: make(chan *facade.ClientToolResultInfo, 1),
		}
		server, ts := newTestFacade(t, handler)
		_ = server // handler itself implements ClientToolRouter

		ws, sid := connectWS(t, wsURL(ts.URL))

		sendMessage(t, ws, sid, "where am I?")

		// Read tool call
		toolCallMsg := readMessage(t, ws)
		require.Equal(t, facade.MessageTypeToolCall, toolCallMsg.Type)

		// Reject the tool call
		require.NoError(t, ws.WriteJSON(facade.ClientMessage{
			Type:      facade.MessageTypeToolResult,
			SessionID: sid,
			ToolResult: &facade.ClientToolResultInfo{
				CallID: "tool-1",
				Error:  "User denied location access",
			},
		}))

		// Should receive done with rejection message
		doneMsg := readMessage(t, ws)
		assert.Equal(t, facade.MessageTypeDone, doneMsg.Type)
		assert.Equal(t, "Tool rejected: User denied location access", doneMsg.Content)
	})

	t.Run("session ID is consistent across messages", func(t *testing.T) {
		_, ts := newTestFacade(t, &echoHandler{})
		ws, sessionID := connectWS(t, wsURL(ts.URL))

		sendMessage(t, ws, sessionID, "first")
		msg1 := readMessage(t, ws)
		assert.Equal(t, sessionID, msg1.SessionID)

		sendMessage(t, ws, sessionID, "second")
		msg2 := readMessage(t, ws)
		assert.Equal(t, sessionID, msg2.SessionID)
	})

	t.Run("multimodal message parts", func(t *testing.T) {
		handler := &mockFacadeHandler{
			handleFunc: func(_ context.Context, _ string, msg *facade.ClientMessage, w facade.ResponseWriter) error {
				// Verify the message has parts
				if len(msg.Parts) == 0 {
					return w.WriteDone("no parts received")
				}
				return w.WriteDoneWithParts([]facade.ContentPart{
					{Type: facade.ContentPartTypeText, Text: "I see your image"},
				})
			},
		}
		_, ts := newTestFacade(t, handler)
		ws, sid := connectWS(t, wsURL(ts.URL))

		// Send a message with text and image parts
		require.NoError(t, ws.WriteJSON(facade.ClientMessage{
			Type:      facade.MessageTypeMessage,
			SessionID: sid,
			Parts: []facade.ContentPart{
				{Type: facade.ContentPartTypeText, Text: "What is this?"},
				{Type: facade.ContentPartTypeImage, Media: &facade.MediaContent{
					Data:     "base64data",
					MimeType: "image/png",
				}},
			},
		}))

		msg := readMessage(t, ws)
		assert.Equal(t, facade.MessageTypeDone, msg.Type)
		require.Len(t, msg.Parts, 1)
		assert.Equal(t, "I see your image", msg.Parts[0].Text)
	})

	t.Run("multiple concurrent connections", func(t *testing.T) {
		_, ts := newTestFacade(t, &echoHandler{})

		const numConns = 5
		type result struct {
			sessionID string
			response  string
		}

		results := make(chan result, numConns)

		for i := 0; i < numConns; i++ {
			go func(n int) {
				ws, sid := connectWS(t, wsURL(ts.URL))
				content := strings.Repeat("x", n+1) // unique per connection
				sendMessage(t, ws, sid, content)
				msg := readMessage(t, ws)
				results <- result{sessionID: sid, response: msg.Content}
			}(i)
		}

		sessionIDs := make(map[string]bool)
		for i := 0; i < numConns; i++ {
			select {
			case r := <-results:
				// Each connection should get a unique session ID
				assert.False(t, sessionIDs[r.sessionID], "duplicate session ID: %s", r.sessionID)
				sessionIDs[r.sessionID] = true
				// Each should get its echo back
				assert.True(t, strings.HasPrefix(r.response, "echo: "))
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for concurrent connection result")
			}
		}
		assert.Len(t, sessionIDs, numConns)
	})

	t.Run("requires agent query param", func(t *testing.T) {
		_, ts := newTestFacade(t, &echoHandler{})

		// Connect without agent param — should fail
		_, resp, err := websocket.DefaultDialer.Dial(wsURL(ts.URL), nil)
		require.Error(t, err)
		if resp != nil {
			assert.Equal(t, 400, resp.StatusCode)
		}
	})
}

// mockFacadeHandler is a generic handler with a configurable function.
type mockFacadeHandler struct {
	handleFunc func(ctx context.Context, sessionID string, msg *facade.ClientMessage, writer facade.ResponseWriter) error
}

func (h *mockFacadeHandler) Name() string { return "mock" }
func (h *mockFacadeHandler) HandleMessage(ctx context.Context, sessionID string, msg *facade.ClientMessage, writer facade.ResponseWriter) error {
	if h.handleFunc != nil {
		return h.handleFunc(ctx, sessionID, msg, writer)
	}
	return writer.WriteDone("ok")
}
