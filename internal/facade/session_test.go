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
	"go.opentelemetry.io/otel/attribute"
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

func newTestServerWithTracing(t *testing.T, handler MessageHandler, provider *tracing.Provider) (*Server, *httptest.Server) {
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

	return server, ts
}

func TestProcessMessage_CreatesMessageSpan(t *testing.T) {
	provider, exporter := newTracingTestProvider(t)

	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, msg *ClientMessage, writer ResponseWriter) error {
			return writer.WriteDone("echo: " + msg.Content)
		},
	}

	_, ts := newTestServerWithTracing(t, handler, provider)

	// Connect and send a message
	ws, _, err := websocket.DefaultDialer.Dial(
		strings.Replace(ts.URL, "http://", "ws://", 1)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	if err := ws.WriteJSON(ClientMessage{Type: MessageTypeMessage, Content: "hello"}); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Read connected message
	var connectedMsg ServerMessage
	if err := ws.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("Failed to read connected: %v", err)
	}
	if connectedMsg.Type != MessageTypeConnected {
		t.Fatalf("Expected connected, got %v", connectedMsg.Type)
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
	msgSpan := findSpanByName(spans, "facade.message")
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
	if val.AsString() == "" {
		t.Error("session.id attribute should not be empty")
	}

	// Verify span has a non-zero duration (it wasn't ended immediately)
	if msgSpan.EndTime.Before(msgSpan.StartTime) || msgSpan.EndTime.Equal(msgSpan.StartTime) {
		t.Error("expected span to have non-zero duration")
	}

	// Find facade.session span (ended when WebSocket was closed)
	sessionSpan := findSpanByName(spans, "facade.session")
	if sessionSpan == nil {
		t.Fatal("expected 'facade.session' span to be recorded")
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

	// Send message — should succeed without panic
	if err := ws.WriteJSON(ClientMessage{Type: MessageTypeMessage, Content: "hello"}); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Read connected message
	var connectedMsg ServerMessage
	if err := ws.ReadJSON(&connectedMsg); err != nil {
		t.Fatalf("Failed to read connected: %v", err)
	}
	if connectedMsg.Type != MessageTypeConnected {
		t.Fatalf("Expected connected, got %v", connectedMsg.Type)
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
