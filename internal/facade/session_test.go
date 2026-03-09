/*
Copyright 2026.

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
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/tracing"
)

// newTracingTestProvider creates a Provider backed by an in-memory span exporter.
func newTracingTestProvider(t *testing.T) (*tracing.Provider, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	return tracing.NewTestProvider(tp), exporter
}

// findSpanByName returns the first span with the given name, or nil.
func findSpanByName(spans []tracetest.SpanStub, name string) *tracetest.SpanStub {
	for i := range spans {
		if spans[i].Name == name {
			return &spans[i]
		}
	}
	return nil
}

// findSpanAttr looks up an attribute by key in a span's attribute set.
func findSpanAttr(span tracetest.SpanStub, key string) (attribute.Value, bool) {
	for _, a := range span.Attributes {
		if string(a.Key) == key {
			return a.Value, true
		}
	}
	return attribute.Value{}, false
}

func newTestServerWithTracing(t *testing.T, handler MessageHandler, provider *tracing.Provider) *httptest.Server {
	t.Helper()

	store := session.NewMemoryStore()
	cfg := DefaultServerConfig()
	cfg.PingInterval = 100 * time.Millisecond
	cfg.PongTimeout = 200 * time.Millisecond

	log := logr.Discard()
	server := NewServer(cfg, store, handler, log, WithTracingProvider(provider))

	ts := httptest.NewServer(server)
	t.Cleanup(func() {
		ts.Close()
		_ = store.Close()
	})

	return ts
}

func TestProcessMessage_CreatesMessageSpan(t *testing.T) {
	provider, exporter := newTracingTestProvider(t)

	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, msg *ClientMessage, writer ResponseWriter) error {
			return writer.WriteDone("echo: " + msg.Content)
		},
	}

	ts := newTestServerWithTracing(t, handler, provider)

	// Connect and read eagerly-sent connected
	ws, _, err := websocket.DefaultDialer.Dial(
		strings.Replace(ts.URL, "http://", "ws://", 1)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Read eagerly-sent connected message
	var connectedMsg ServerMessage
	if err := ws.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("Failed to read connected: %v", err)
	}
	if connectedMsg.Type != MessageTypeConnected {
		t.Fatalf("Expected connected, got %v", connectedMsg.Type)
	}
	sessionID := connectedMsg.SessionID

	// Send message with session ID
	if err := ws.WriteJSON(ClientMessage{Type: MessageTypeMessage, SessionID: sessionID, Content: "hello"}); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Read done message
	var doneMsg ServerMessage
	if err := ws.ReadJSON(&doneMsg); err != nil {
		t.Fatalf("Failed to read done: %v", err)
	}
	if doneMsg.Type != MessageTypeDone {
		t.Fatalf("Expected done, got %v", doneMsg.Type)
	}

	// Close the WebSocket to end the facade.session span
	_ = ws.Close()

	// Give the server a moment to finish span cleanup
	time.Sleep(50 * time.Millisecond)

	// Verify spans were recorded
	spans := exporter.GetSpans()

	// Find facade.message span
	msgSpan := findSpanByName(spans, "omnia.facade.message")
	if msgSpan == nil {
		t.Fatal("expected 'facade.message' span to be recorded")
	}

	if msgSpan.SpanKind != trace.SpanKindServer {
		t.Errorf("expected SpanKindServer, got %v", msgSpan.SpanKind)
	}

	// Verify session.id attribute was set
	val, ok := findSpanAttr(*msgSpan, "session.id")
	if !ok {
		t.Fatal("missing attribute 'session.id' on facade.message span")
	}
	spanSessionID := val.AsString()
	if spanSessionID == "" {
		t.Fatal("session.id attribute should not be empty")
	}

	// Verify trace ID is derived from session ID (UUID without dashes)
	expectedTraceID := sessionIDToTraceID(spanSessionID)
	if msgSpan.SpanContext.TraceID() != expectedTraceID {
		t.Errorf("trace ID = %s, want %s (derived from session %s)",
			msgSpan.SpanContext.TraceID(), expectedTraceID, spanSessionID)
	}

	// Verify span has a non-zero duration (it wasn't ended immediately)
	if msgSpan.EndTime.Before(msgSpan.StartTime) || msgSpan.EndTime.Equal(msgSpan.StartTime) {
		t.Error("expected span to have non-zero duration")
	}

	// Verify agent attributes are set on the message span (moved from session span)
	agentVal, ok := findSpanAttr(*msgSpan, "omnia.agent.name")
	if !ok {
		t.Fatal("missing attribute 'omnia.agent.name' on facade.message span")
	}
	if agentVal.AsString() != "test-agent" {
		t.Errorf("omnia.agent.name = %q, want %q", agentVal.AsString(), "test-agent")
	}

	// Verify no facade.session span exists (removed to fix trace visualization)
	sessionSpan := findSpanByName(spans, "facade.session")
	if sessionSpan != nil {
		t.Error("facade.session span should no longer be created")
	}
}

func TestProcessMessage_WithParentTraceContext(t *testing.T) {
	// Register the W3C trace context propagator so the facade extracts traceparent.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
	))

	provider, exporter := newTracingTestProvider(t)

	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, msg *ClientMessage, writer ResponseWriter) error {
			return writer.WriteDone("echo: " + msg.Content)
		},
	}

	ts := newTestServerWithTracing(t, handler, provider)

	// Create a traceparent header with a known trace ID
	parentTraceID := trace.TraceID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	parentSpanID := trace.SpanID{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11}
	traceparent := "00-" + parentTraceID.String() + "-" + parentSpanID.String() + "-01"

	// Connect with traceparent header
	dialer := websocket.Dialer{}
	headers := make(map[string][]string)
	headers["Traceparent"] = []string{traceparent}

	ws, _, err := dialer.Dial(
		strings.Replace(ts.URL, "http://", "ws://", 1)+"?agent=test-agent", headers)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Read eagerly-sent connected message
	var connectedMsg ServerMessage
	if err := ws.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("Failed to read connected: %v", err)
	}
	sessionID := connectedMsg.SessionID

	// Send message with session ID
	if err := ws.WriteJSON(ClientMessage{Type: MessageTypeMessage, SessionID: sessionID, Content: "hello"}); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Read done message
	var doneMsg ServerMessage
	if err := ws.ReadJSON(&doneMsg); err != nil {
		t.Fatalf("Failed to read done: %v", err)
	}

	_ = ws.Close()
	time.Sleep(50 * time.Millisecond)

	spans := exporter.GetSpans()
	msgSpan := findSpanByName(spans, "omnia.facade.message")
	if msgSpan == nil {
		t.Fatal("expected 'omnia.facade.message' span to be recorded")
	}

	// Verify the span uses the session-derived trace ID (not the caller's).
	// This ensures all messages in a session share one trace, so evals and
	// downstream spans are nested under the session in Tempo.
	spanSessionID := ""
	if val, ok := findSpanAttr(*msgSpan, "session.id"); ok {
		spanSessionID = val.AsString()
	}
	if spanSessionID == "" {
		t.Fatal("missing session.id attribute")
	}

	expectedSessionTraceID := sessionIDToTraceID(spanSessionID)
	if msgSpan.SpanContext.TraceID() != expectedSessionTraceID {
		t.Errorf("trace ID = %s, want %s (session-derived)",
			msgSpan.SpanContext.TraceID(), expectedSessionTraceID)
	}

	// Verify the caller's trace context is present as a span link for cross-referencing.
	if len(msgSpan.Links) == 0 {
		t.Fatal("expected span link for caller trace context")
	}
	if msgSpan.Links[0].SpanContext.TraceID() != parentTraceID {
		t.Errorf("link trace ID = %s, want %s (caller trace)",
			msgSpan.Links[0].SpanContext.TraceID(), parentTraceID)
	}

	// Verify link has the caller-trace type attribute
	linkHasType := false
	for _, a := range msgSpan.Links[0].Attributes {
		if string(a.Key) == "link.type" && a.Value.AsString() == "caller-trace" {
			linkHasType = true
		}
	}
	if !linkHasType {
		t.Error("expected link.type=caller-trace attribute on span link")
	}
}

func TestProcessMessage_NoTracingProvider(t *testing.T) {
	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, msg *ClientMessage, writer ResponseWriter) error {
			return writer.WriteDone("echo: " + msg.Content)
		},
	}

	// Create server WITHOUT tracing provider
	_, ts := newTestServer(t, handler)

	ws, _, err := websocket.DefaultDialer.Dial(
		strings.Replace(ts.URL, "http://", "ws://", 1)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Read eagerly-sent connected message
	var connectedMsg ServerMessage
	if err := ws.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("Failed to read connected: %v", err)
	}
	if connectedMsg.Type != MessageTypeConnected {
		t.Fatalf("Expected connected, got %v", connectedMsg.Type)
	}
	sessionID := connectedMsg.SessionID

	// Send message with session ID — should succeed without panic
	if err := ws.WriteJSON(ClientMessage{Type: MessageTypeMessage, SessionID: sessionID, Content: "hello"}); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Read done message — verifies full path works without tracing
	var doneMsg ServerMessage
	if err := ws.ReadJSON(&doneMsg); err != nil {
		t.Fatalf("Failed to read done: %v", err)
	}
	if doneMsg.Type != MessageTypeDone {
		t.Fatalf("Expected done, got %v", doneMsg.Type)
	}
	if doneMsg.Content != "echo: hello" {
		t.Errorf("Content = %q, want %q", doneMsg.Content, "echo: hello")
	}
}
