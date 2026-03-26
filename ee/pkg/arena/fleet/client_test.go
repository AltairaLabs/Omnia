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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

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

// testServerWithHeaders creates a WebSocket test server that captures upgrade
// request headers and runs the given handler.
func testServerWithHeaders(t *testing.T, capturedHeaders *http.Header, handler func(*websocket.Conn)) string {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*capturedHeaders = r.Header.Clone()
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

func TestTraceHeaders_InjectsTraceparent(t *testing.T) {
	// Set up propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
	))

	// Create a context with a span so there's trace context to inject
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-op")
	defer span.End()

	headers := traceHeaders(ctx)
	traceparent := headers.Get("Traceparent")
	require.NotEmpty(t, traceparent, "traceparent header should be injected")

	// Verify format: version-traceID-spanID-flags
	parts := strings.Split(traceparent, "-")
	require.Len(t, parts, 4, "traceparent should have 4 parts")
	assert.Equal(t, "00", parts[0], "version should be 00")
	assert.Equal(t, span.SpanContext().TraceID().String(), parts[1])
}

func TestTraceHeaders_EmptyWithoutSpan(t *testing.T) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
	))

	headers := traceHeaders(context.Background())
	assert.Empty(t, headers.Get("Traceparent"), "no traceparent without active span")
}

func TestConnect_InjectsTraceHeaders(t *testing.T) {
	// Set up propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
	))

	// Create a trace context
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-work-item")
	defer span.End()

	var capturedHeaders http.Header
	wsURL := testServerWithHeaders(t, &capturedHeaders, func(conn *websocket.Conn) {
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeConnected,
			SessionID: "sess-trace",
			Timestamp: time.Now(),
		})
		time.Sleep(time.Second)
	})

	p := NewProvider("test-fleet", wsURL, nil)
	err := p.Connect(ctx)
	require.NoError(t, err)
	defer func() { _ = p.Close() }()

	// Verify traceparent was sent in upgrade headers
	traceparent := capturedHeaders.Get("Traceparent")
	require.NotEmpty(t, traceparent, "Connect should inject traceparent header")
	assert.Contains(t, traceparent, span.SpanContext().TraceID().String())
}

func TestCollectTurnResponse_RejectsToolCalls(t *testing.T) {
	wsURL := testServer(t, func(conn *websocket.Conn) {
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeConnected,
			SessionID: "sess-reject",
			Timestamp: time.Now(),
		})

		// Read the user message
		readClientMsg(t, conn)

		// Send a tool_call
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeToolCall,
			SessionID: "sess-reject",
			ToolCall: &facade.ToolCallInfo{
				ID:   "tc-fleet-1",
				Name: "get_location",
			},
			Timestamp: time.Now(),
		})

		// Read the rejection that collectTurnResponse should send
		rejection := readClientMsg(t, conn)
		assert.Equal(t, facade.MessageTypeToolResult, rejection.Type)
		require.NotNil(t, rejection.ToolResult)
		assert.Equal(t, "tc-fleet-1", rejection.ToolResult.CallID)
		assert.Contains(t, rejection.ToolResult.Error, "arena evaluation mode")

		// Send done to complete the turn
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeDone,
			SessionID: "sess-reject",
			Content:   "Tool was rejected",
			Timestamp: time.Now(),
		})
	})

	p := NewProvider("test-fleet", wsURL, nil)
	require.NoError(t, p.Connect(context.Background()))
	defer func() { _ = p.Close() }()

	// Send a message to trigger the tool call flow
	fb := p.fallback
	require.NoError(t, sendMessage(fb.conn, fb.sessionID, "do something"))

	turnResult, err := collectTurnResponse(context.Background(), fb.conn, fb.sessionID, time.Now())
	require.NoError(t, err)

	// Should have the tool_call in the transcript and the assistant done message
	require.Len(t, turnResult.Messages, 2)
	assert.Equal(t, "tool_call", turnResult.Messages[0].Role)
	assert.Equal(t, "assistant", turnResult.Messages[1].Role)
	assert.Equal(t, "Tool was rejected", turnResult.Messages[1].Content)
}

func TestCollectTurnResponse_MeasuresTTFT(t *testing.T) {
	wsURL := testServer(t, func(conn *websocket.Conn) {
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeConnected,
			SessionID: "sess-ttft",
			Timestamp: time.Now(),
		})

		// Read user message
		readClientMsg(t, conn)

		// Add a small delay before sending the first chunk to make TTFT measurable
		time.Sleep(20 * time.Millisecond)

		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeChunk,
			SessionID: "sess-ttft",
			Content:   "Hello ",
			Timestamp: time.Now(),
		})
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeDone,
			SessionID: "sess-ttft",
			Content:   "world!",
			Timestamp: time.Now(),
		})
	})

	p := NewProvider("test-fleet", wsURL, nil)
	require.NoError(t, p.Connect(context.Background()))
	defer func() { _ = p.Close() }()

	fb := p.fallback
	require.NoError(t, sendMessage(fb.conn, fb.sessionID, "hi"))

	turnResult, err := collectTurnResponse(context.Background(), fb.conn, fb.sessionID, time.Now())
	require.NoError(t, err)

	assert.True(t, turnResult.TTFT > 0, "TTFT should be positive")
	assert.True(t, turnResult.TTFT >= 20*time.Millisecond, "TTFT should be at least 20ms (server delay)")
	require.Len(t, turnResult.Messages, 1)
	assert.Equal(t, "assistant", turnResult.Messages[0].Role)
	assert.Equal(t, "Hello world!", turnResult.Messages[0].Content)
}

func TestCollectTurnResponse_TTFTZeroOnError(t *testing.T) {
	wsURL := testServer(t, func(conn *websocket.Conn) {
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeConnected,
			SessionID: "sess-ttft-err",
			Timestamp: time.Now(),
		})

		// Read user message, then close connection to trigger error
		readClientMsg(t, conn)
		_ = conn.Close()
	})

	p := NewProvider("test-fleet", wsURL, nil)
	require.NoError(t, p.Connect(context.Background()))
	defer func() { _ = p.Close() }()

	fb := p.fallback
	require.NoError(t, sendMessage(fb.conn, fb.sessionID, "hi"))

	turnResult, err := collectTurnResponse(context.Background(), fb.conn, fb.sessionID, time.Now())
	require.Error(t, err)
	assert.Equal(t, time.Duration(0), turnResult.TTFT, "TTFT should be zero when no messages received")
}

func TestConnect_WorksWithoutTraceContext(t *testing.T) {
	var capturedHeaders http.Header
	wsURL := testServerWithHeaders(t, &capturedHeaders, func(conn *websocket.Conn) {
		writeServerMsg(t, conn, facade.ServerMessage{
			Type:      facade.MessageTypeConnected,
			SessionID: "sess-no-trace",
			Timestamp: time.Now(),
		})
		time.Sleep(time.Second)
	})

	p := NewProvider("test-fleet", wsURL, nil)
	err := p.Connect(context.Background())
	require.NoError(t, err)
	defer func() { _ = p.Close() }()

	assert.Equal(t, "sess-no-trace", p.SessionID())
}
