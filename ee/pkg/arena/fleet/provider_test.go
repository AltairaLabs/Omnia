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

	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/facade"
)

// providerTestServer creates a WebSocket test server and returns a connected Provider.
func providerTestServer(t *testing.T, handler func(*websocket.Conn)) *Provider {
	t.Helper()
	wsURL := testServer(t, handler)
	p := NewProvider("test-fleet", wsURL, nil)
	require.NoError(t, p.Connect(context.Background()))
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestProvider_IDAndModel(t *testing.T) {
	p := NewProvider("my-fleet", "ws://unused", nil)
	assert.Equal(t, "my-fleet", p.ID())
	assert.Equal(t, "fleet", p.Model())
}

func TestProvider_SupportsStreaming(t *testing.T) {
	p := NewProvider("test", "ws://unused", nil)
	assert.True(t, p.SupportsStreaming())
	assert.False(t, p.ShouldIncludeRawOutput())
}

func TestProvider_CalculateCost(t *testing.T) {
	p := NewProvider("test", "ws://unused", nil)
	cost := p.CalculateCost(100, 200, 50)
	assert.Equal(t, types.CostInfo{}, cost)
}

func TestProvider_Connect(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		wsURL := testServer(t, func(conn *websocket.Conn) {
			writeServerMsg(t, conn, facade.ServerMessage{
				Type:      facade.MessageTypeConnected,
				SessionID: "sess-connect",
				Timestamp: time.Now(),
			})
			// Keep connection open until test completes
			time.Sleep(time.Second)
		})

		p := NewProvider("test", wsURL, nil)
		err := p.Connect(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "sess-connect", p.sessionID)
		_ = p.Close()
	})

	t.Run("connection refused", func(t *testing.T) {
		p := NewProvider("test", "ws://localhost:1", nil)
		err := p.Connect(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to connect")
	})

	t.Run("unexpected first message", func(t *testing.T) {
		wsURL := testServer(t, func(conn *websocket.Conn) {
			writeServerMsg(t, conn, facade.ServerMessage{
				Type:      facade.MessageTypeChunk,
				Content:   "not connected",
				Timestamp: time.Now(),
			})
		})

		p := NewProvider("test", wsURL, nil)
		err := p.Connect(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected connected message")
	})
}

func TestProvider_Predict_SingleTurn(t *testing.T) {
	p := providerTestServer(t, func(conn *websocket.Conn) {
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeConnected,
			SessionID: "sess-predict",
			Timestamp: time.Now(),
		})

		msg := readClientMsg(t, conn)
		assert.Equal(t, facade.MessageTypeMessage, msg.Type)
		assert.Equal(t, "Hello agent", msg.Content)

		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeChunk,
			SessionID: "sess-predict",
			Content:   "Hi there! ",
			Timestamp: time.Now(),
		})
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeDone,
			SessionID: "sess-predict",
			Content:   "How can I help?",
			Timestamp: time.Now(),
		})
	})

	resp, err := p.Predict(context.Background(), providers.PredictionRequest{
		Messages: []types.Message{
			{Role: "user", Content: "Hello agent"},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "Hi there! How can I help?", resp.Content)
	assert.True(t, resp.Latency > 0)
}

func TestProvider_Predict_MultiTurn(t *testing.T) {
	p := providerTestServer(t, func(conn *websocket.Conn) {
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

	// Turn 1
	resp1, err := p.Predict(context.Background(), providers.PredictionRequest{
		Messages: []types.Message{
			{Role: "user", Content: "Turn 1"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "Response 1", resp1.Content)

	// Turn 2 — includes history, but only the last user message is sent
	resp2, err := p.Predict(context.Background(), providers.PredictionRequest{
		Messages: []types.Message{
			{Role: "user", Content: "Turn 1"},
			{Role: "assistant", Content: "Response 1"},
			{Role: "user", Content: "Turn 2"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "Response 2", resp2.Content)
}

func TestProvider_Predict_WithToolCalls(t *testing.T) {
	p := providerTestServer(t, func(conn *websocket.Conn) {
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeConnected,
			SessionID: "sess-tools",
			Timestamp: time.Now(),
		})

		readClientMsg(t, conn)

		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeToolCall,
			SessionID: "sess-tools",
			ToolCall: &facade.ToolCallInfo{
				ID:   "tc-1",
				Name: "search",
			},
			Timestamp: time.Now(),
		})
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeToolResult,
			SessionID: "sess-tools",
			ToolResult: &facade.ToolResultInfo{
				ID:     "tc-1",
				Result: "results here",
			},
			Timestamp: time.Now(),
		})
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeDone,
			SessionID: "sess-tools",
			Content:   "Found results",
			Timestamp: time.Now(),
		})
	})

	resp, err := p.Predict(context.Background(), providers.PredictionRequest{
		Messages: []types.Message{
			{Role: "user", Content: "Search for something"},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "Found results", resp.Content)
	require.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, "search", resp.ToolCalls[0].Name)
	assert.Equal(t, "tc-1", resp.ToolCalls[0].ID)
}

func TestProvider_Predict_AgentError(t *testing.T) {
	p := providerTestServer(t, func(conn *websocket.Conn) {
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

	_, err := p.Predict(context.Background(), providers.PredictionRequest{
		Messages: []types.Message{
			{Role: "user", Content: "Hello"},
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "something went wrong")
}

func TestProvider_Predict_NoUserMessage(t *testing.T) {
	p := NewProvider("test", "ws://unused", nil)
	_, err := p.Predict(context.Background(), providers.PredictionRequest{
		Messages: []types.Message{
			{Role: "assistant", Content: "I'm an assistant"},
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no user message")
}

func TestProvider_Predict_ContextCancellation(t *testing.T) {
	p := providerTestServer(t, func(conn *websocket.Conn) {
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeConnected,
			SessionID: "sess-ctx",
			Timestamp: time.Now(),
		})

		readClientMsg(t, conn)
		// Don't send a response — let context expire
		time.Sleep(2 * time.Second)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := p.Predict(ctx, providers.PredictionRequest{
		Messages: []types.Message{
			{Role: "user", Content: "Hello"},
		},
	})

	require.Error(t, err)
}

func TestProvider_PredictStream(t *testing.T) {
	p := providerTestServer(t, func(conn *websocket.Conn) {
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeConnected,
			SessionID: "sess-stream",
			Timestamp: time.Now(),
		})

		readClientMsg(t, conn)

		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeChunk,
			SessionID: "sess-stream",
			Content:   "Hello ",
			Timestamp: time.Now(),
		})
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeDone,
			SessionID: "sess-stream",
			Content:   "world!",
			Timestamp: time.Now(),
		})
	})

	ch, err := p.PredictStream(context.Background(), providers.PredictionRequest{
		Messages: []types.Message{
			{Role: "user", Content: "Hi"},
		},
	})
	require.NoError(t, err)

	chunks := make([]providers.StreamChunk, 0, 1)
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}

	require.Len(t, chunks, 1)
	assert.Equal(t, "Hello world!", chunks[0].Content)
	assert.NotNil(t, chunks[0].FinishReason)
	assert.Equal(t, "stop", *chunks[0].FinishReason)
}

func TestProvider_PredictStream_Error(t *testing.T) {
	p := providerTestServer(t, func(conn *websocket.Conn) {
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeConnected,
			SessionID: "sess-stream-err",
			Timestamp: time.Now(),
		})

		readClientMsg(t, conn)

		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeError,
			SessionID: "sess-stream-err",
			Error: &facade.ErrorInfo{
				Code:    "INTERNAL_ERROR",
				Message: "stream failed",
			},
			Timestamp: time.Now(),
		})
	})

	ch, err := p.PredictStream(context.Background(), providers.PredictionRequest{
		Messages: []types.Message{
			{Role: "user", Content: "Hi"},
		},
	})
	require.NoError(t, err)

	chunks := make([]providers.StreamChunk, 0, 1)
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}

	require.Len(t, chunks, 1)
	assert.NotNil(t, chunks[0].Error)
	assert.Contains(t, chunks[0].Error.Error(), "stream failed")
	assert.NotNil(t, chunks[0].FinishReason)
	assert.Equal(t, "error", *chunks[0].FinishReason)
}

func TestProvider_PredictStream_NoUserMessage(t *testing.T) {
	p := NewProvider("test", "ws://unused", nil)
	_, err := p.PredictStream(context.Background(), providers.PredictionRequest{
		Messages: []types.Message{
			{Role: "system", Content: "System prompt"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no user message")
}

func TestProvider_Close_NilConn(t *testing.T) {
	p := NewProvider("test", "ws://unused", nil)
	assert.NoError(t, p.Close())
}

func TestExtractLastUserMessage(t *testing.T) {
	tests := []struct {
		name     string
		messages []types.Message
		want     string
	}{
		{
			name:     "single user message",
			messages: []types.Message{{Role: "user", Content: "hello"}},
			want:     "hello",
		},
		{
			name: "last user message from history",
			messages: []types.Message{
				{Role: "user", Content: "first"},
				{Role: "assistant", Content: "response"},
				{Role: "user", Content: "second"},
			},
			want: "second",
		},
		{
			name: "no user message",
			messages: []types.Message{
				{Role: "system", Content: "system prompt"},
				{Role: "assistant", Content: "hi"},
			},
			want: "",
		},
		{
			name:     "empty messages",
			messages: nil,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractLastUserMessage(tt.messages)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildPredictionResponse(t *testing.T) {
	t.Run("assistant text only", func(t *testing.T) {
		msgs := []Message{
			{Role: "assistant", Content: "Hello world"},
		}
		resp := buildPredictionResponse(msgs, 100*time.Millisecond)
		assert.Equal(t, "Hello world", resp.Content)
		assert.Equal(t, 100*time.Millisecond, resp.Latency)
		assert.Empty(t, resp.ToolCalls)
	})

	t.Run("with tool calls", func(t *testing.T) {
		msgs := []Message{
			{Role: "tool_call", ToolCall: &facade.ToolCallInfo{ID: "tc-1", Name: "search"}},
			{Role: "tool_result"},
			{Role: "assistant", Content: "Results found"},
		}
		resp := buildPredictionResponse(msgs, 0)
		assert.Equal(t, "Results found", resp.Content)
		require.Len(t, resp.ToolCalls, 1)
		assert.Equal(t, "search", resp.ToolCalls[0].Name)
	})
}

// testServerRaw is a variant for testing the raw WS test server with custom test logic.
func testServerRaw(t *testing.T, handler func(*websocket.Conn)) string {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		handler(conn)
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func TestProvider_Predict_SendsOnlyLastUserMessage(t *testing.T) {
	var receivedContent string
	wsURL := testServerRaw(t, func(conn *websocket.Conn) {
		data, _ := json.Marshal(facade.ServerMessage{
			Type:      facade.MessageTypeConnected,
			SessionID: "sess-last",
			Timestamp: time.Now(),
		})
		_ = conn.WriteMessage(websocket.TextMessage, data)

		_, msgData, _ := conn.ReadMessage()
		var msg facade.ClientMessage
		_ = json.Unmarshal(msgData, &msg)
		receivedContent = msg.Content

		data, _ = json.Marshal(facade.ServerMessage{
			Type:      facade.MessageTypeDone,
			SessionID: "sess-last",
			Content:   "ok",
			Timestamp: time.Now(),
		})
		_ = conn.WriteMessage(websocket.TextMessage, data)
	})

	p := NewProvider("test", wsURL, nil)
	require.NoError(t, p.Connect(context.Background()))
	defer func() { _ = p.Close() }()

	_, err := p.Predict(context.Background(), providers.PredictionRequest{
		Messages: []types.Message{
			{Role: "user", Content: "first message"},
			{Role: "assistant", Content: "response"},
			{Role: "user", Content: "second message"},
		},
	})
	require.NoError(t, err)

	// Verify only the last user message was sent over the wire
	assert.Equal(t, "second message", receivedContent)
}
