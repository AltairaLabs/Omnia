/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package fleet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/facade"
)

// testServer creates a WebSocket test server that runs the given handler function.
func testServer(t *testing.T, handler func(*websocket.Conn)) string {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer func() { _ = conn.Close() }()
		handler(conn)
	}))
	t.Cleanup(srv.Close)

	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func writeServerMsg(t *testing.T, conn *websocket.Conn, msg facade.ServerMessage) {
	t.Helper()
	data, err := json.Marshal(msg)
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, data))
}

func readClientMsg(t *testing.T, conn *websocket.Conn) facade.ClientMessage {
	t.Helper()
	_, data, err := conn.ReadMessage()
	require.NoError(t, err)
	var msg facade.ClientMessage
	require.NoError(t, json.Unmarshal(data, &msg))
	return msg
}

func TestRunConversation_SingleTurn(t *testing.T) {
	wsURL := testServer(t, func(conn *websocket.Conn) {
		// Send connected
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeConnected,
			SessionID: "sess-123",
			Timestamp: time.Now(),
		})

		// Read user message
		msg := readClientMsg(t, conn)
		assert.Equal(t, facade.MessageTypeMessage, msg.Type)
		assert.Equal(t, "Hello agent", msg.Content)
		assert.Equal(t, "sess-123", msg.SessionID)

		// Send response chunks + done
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeChunk,
			SessionID: "sess-123",
			Content:   "Hi there! ",
			Timestamp: time.Now(),
		})
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeDone,
			SessionID: "sess-123",
			Content:   "How can I help?",
			Timestamp: time.Now(),
		})
	})

	client := NewClient(wsURL)
	result, err := client.RunConversation(context.Background(), []ScenarioTurn{
		{Role: "user", Content: "Hello agent"},
	})

	require.NoError(t, err)
	assert.Equal(t, "sess-123", result.SessionID)
	assert.Empty(t, result.Error)
	require.Len(t, result.Messages, 2) // user + assistant
	assert.Equal(t, "user", result.Messages[0].Role)
	assert.Equal(t, "Hello agent", result.Messages[0].Content)
	assert.Equal(t, "assistant", result.Messages[1].Role)
	assert.Equal(t, "Hi there! How can I help?", result.Messages[1].Content)
	assert.True(t, result.Duration > 0)
}

func TestRunConversation_MultiTurn(t *testing.T) {
	wsURL := testServer(t, func(conn *websocket.Conn) {
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeConnected,
			SessionID: "sess-multi",
			Timestamp: time.Now(),
		})

		// Turn 1
		readClientMsg(t, conn)
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeDone,
			SessionID: "sess-multi",
			Content:   "Response 1",
			Timestamp: time.Now(),
		})

		// Turn 2
		readClientMsg(t, conn)
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeDone,
			SessionID: "sess-multi",
			Content:   "Response 2",
			Timestamp: time.Now(),
		})
	})

	client := NewClient(wsURL)
	result, err := client.RunConversation(context.Background(), []ScenarioTurn{
		{Role: "user", Content: "Turn 1"},
		{Role: "user", Content: "Turn 2"},
	})

	require.NoError(t, err)
	require.Len(t, result.Messages, 4) // 2 user + 2 assistant
	assert.Equal(t, "Turn 1", result.Messages[0].Content)
	assert.Equal(t, "Response 1", result.Messages[1].Content)
	assert.Equal(t, "Turn 2", result.Messages[2].Content)
	assert.Equal(t, "Response 2", result.Messages[3].Content)
}

func TestRunConversation_WithToolCalls(t *testing.T) {
	wsURL := testServer(t, func(conn *websocket.Conn) {
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeConnected,
			SessionID: "sess-tools",
			Timestamp: time.Now(),
		})

		readClientMsg(t, conn)

		// Send tool call
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeToolCall,
			SessionID: "sess-tools",
			ToolCall: &facade.ToolCallInfo{
				ID:        "tc-1",
				Name:      "search",
				Arguments: map[string]interface{}{"query": "test"},
			},
			Timestamp: time.Now(),
		})

		// Send tool result
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeToolResult,
			SessionID: "sess-tools",
			ToolResult: &facade.ToolResultInfo{
				ID:     "tc-1",
				Result: "search results",
			},
			Timestamp: time.Now(),
		})

		// Send final response
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeDone,
			SessionID: "sess-tools",
			Content:   "Here are the results",
			Timestamp: time.Now(),
		})
	})

	client := NewClient(wsURL)
	result, err := client.RunConversation(context.Background(), []ScenarioTurn{
		{Role: "user", Content: "Search for something"},
	})

	require.NoError(t, err)
	require.Len(t, result.Messages, 4) // user + tool_call + tool_result + assistant
	assert.Equal(t, "tool_call", result.Messages[1].Role)
	assert.Equal(t, "search", result.Messages[1].ToolCall.Name)
	assert.Equal(t, "tool_result", result.Messages[2].Role)
	assert.Equal(t, "tc-1", result.Messages[2].ToolResult.ID)
	assert.Equal(t, "assistant", result.Messages[3].Role)
}

func TestRunConversation_AgentError(t *testing.T) {
	wsURL := testServer(t, func(conn *websocket.Conn) {
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeConnected,
			SessionID: "sess-err",
			Timestamp: time.Now(),
		})

		readClientMsg(t, conn)

		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeError,
			SessionID: "sess-err",
			Error: &facade.ErrorInfo{
				Code:    "INTERNAL_ERROR",
				Message: "something went wrong",
			},
			Timestamp: time.Now(),
		})
	})

	client := NewClient(wsURL)
	result, err := client.RunConversation(context.Background(), []ScenarioTurn{
		{Role: "user", Content: "Hello"},
	})

	require.NoError(t, err)
	assert.Contains(t, result.Error, "something went wrong")
}

func TestRunConversation_ConnectionFailure(t *testing.T) {
	client := NewClient("ws://localhost:1") // nothing listening
	_, err := client.RunConversation(context.Background(), []ScenarioTurn{
		{Role: "user", Content: "Hello"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect")
}

func TestRunConversation_ContextCancellation(t *testing.T) {
	wsURL := testServer(t, func(conn *websocket.Conn) {
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeConnected,
			SessionID: "sess-ctx",
			Timestamp: time.Now(),
		})

		readClientMsg(t, conn)

		// Don't send a response — let the context expire
		time.Sleep(2 * time.Second)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	client := NewClient(wsURL)
	result, err := client.RunConversation(ctx, []ScenarioTurn{
		{Role: "user", Content: "Hello"},
	})

	// The context should be cancelled, and the error should be recorded
	require.NoError(t, err)
	assert.NotEmpty(t, result.Error)
}

func TestRunConversation_InvalidConnectedMessage(t *testing.T) {
	wsURL := testServer(t, func(conn *websocket.Conn) {
		// Send a non-connected message first
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeChunk,
			SessionID: "sess-bad",
			Content:   "unexpected",
			Timestamp: time.Now(),
		})
	})

	client := NewClient(wsURL)
	_, err := client.RunConversation(context.Background(), []ScenarioTurn{
		{Role: "user", Content: "Hello"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected connected message")
}
