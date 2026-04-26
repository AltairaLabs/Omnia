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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/altairalabs/omnia/internal/facade/auth"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/identity"
	"github.com/altairalabs/omnia/pkg/policy"
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

func TestBuildSessionTags_Anonymous(t *testing.T) {
	c := &Connection{agentName: "agent-1"}
	tags := buildSessionTags(c)
	if len(tags) != 1 || tags[0] != "source:interactive" {
		t.Errorf("expected [source:interactive], got %v", tags)
	}
}

func TestBuildSessionTags_Authenticated(t *testing.T) {
	c := &Connection{agentName: "agent-1", userID: "alice"}
	tags := buildSessionTags(c)
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
	if tags[0] != "source:interactive" {
		t.Errorf("tags[0] = %q, want source:interactive", tags[0])
	}
	if tags[1] != "user:alice" {
		t.Errorf("tags[1] = %q, want user:alice", tags[1])
	}
}

func TestBuildSessionState_Empty(t *testing.T) {
	c := &Connection{}
	cfg := ServerConfig{}
	state := buildSessionState(c, cfg)
	if len(state) != 0 {
		t.Errorf("expected empty state, got %v", state)
	}
}

func TestBuildSessionState_Full(t *testing.T) {
	c := &Connection{
		userID:    "alice",
		userEmail: "alice@example.com",
		userRoles: "admin,editor",
	}
	cfg := ServerConfig{
		PromptPackName:    "my-pack",
		PromptPackVersion: "v2",
	}
	state := buildSessionState(c, cfg)
	if state["user.id"] != "alice" {
		t.Errorf("user.id = %q, want alice", state["user.id"])
	}
	if state["user.email"] != "alice@example.com" {
		t.Errorf("user.email = %q", state["user.email"])
	}
	if state["user.roles"] != "admin,editor" {
		t.Errorf("user.roles = %q", state["user.roles"])
	}
	if state["promptpack.name"] != "my-pack" {
		t.Errorf("promptpack.name = %q", state["promptpack.name"])
	}
	if state["promptpack.version"] != "v2" {
		t.Errorf("promptpack.version = %q", state["promptpack.version"])
	}
}

func TestBuildSessionState_PartialUser(t *testing.T) {
	c := &Connection{userID: "bob"}
	cfg := ServerConfig{PromptPackName: "pack-1"}
	state := buildSessionState(c, cfg)
	if len(state) != 2 {
		t.Errorf("expected 2 entries, got %d: %v", len(state), state)
	}
	if state["user.id"] != "bob" {
		t.Errorf("user.id = %q", state["user.id"])
	}
	if state["promptpack.name"] != "pack-1" {
		t.Errorf("promptpack.name = %q", state["promptpack.name"])
	}
}

func TestProcessMessage_PropagatesUserIDToPolicyContext(t *testing.T) {
	var capturedCtx context.Context

	handler := &mockHandler{
		handleFunc: func(ctx context.Context, _ string, msg *ClientMessage, writer ResponseWriter) error {
			capturedCtx = ctx
			return writer.WriteDone("ok")
		},
	}

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

	// Dial with an X-User-Id header to simulate an authenticated user.
	headers := http.Header{}
	headers.Set(policy.IstioHeaderUserID, "test-user-raw")
	ws, _, err := websocket.DefaultDialer.Dial(
		strings.Replace(ts.URL, "http://", "ws://", 1)+"?agent=test-agent", headers)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Read connected message
	var connMsg ServerMessage
	if err := ws.ReadJSON(&connMsg); err != nil {
		t.Fatalf("Failed to read connected: %v", err)
	}

	// Send a message
	if err := ws.WriteJSON(ClientMessage{
		Type: MessageTypeMessage, SessionID: connMsg.SessionID, Content: "hi",
	}); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	// Read done
	var doneMsg ServerMessage
	if err := ws.ReadJSON(&doneMsg); err != nil {
		t.Fatalf("Failed to read done: %v", err)
	}

	// Verify policy.UserID is set on the context (pseudonymized).
	got := policy.UserID(capturedCtx)
	want := identity.PseudonymizeID("test-user-raw")
	if got != want {
		t.Errorf("policy.UserID(ctx) = %q, want %q", got, want)
	}
}

func TestCohortHeaders_ExtractedAndStoredOnSession(t *testing.T) {
	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, _ *ClientMessage, writer ResponseWriter) error {
			return writer.WriteDone("ok")
		},
	}

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

	headers := http.Header{}
	headers.Set(policy.HeaderCohortID, "cohort-abc")
	headers.Set(policy.HeaderVariant, "canary")
	ws, _, err := websocket.DefaultDialer.Dial(
		strings.Replace(ts.URL, "http://", "ws://", 1)+"?agent=test-agent", headers)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	var connMsg ServerMessage
	if err := ws.ReadJSON(&connMsg); err != nil {
		t.Fatalf("Failed to read connected: %v", err)
	}

	// Send a message to trigger session persistence
	if err := ws.WriteJSON(ClientMessage{
		Type: MessageTypeMessage, SessionID: connMsg.SessionID, Content: "hi",
	}); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	var doneMsg ServerMessage
	if err := ws.ReadJSON(&doneMsg); err != nil {
		t.Fatalf("Failed to read done: %v", err)
	}

	// Verify session was created with cohort fields
	sess, err := store.GetSession(context.Background(), connMsg.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.CohortID != "cohort-abc" {
		t.Errorf("CohortID = %q, want %q", sess.CohortID, "cohort-abc")
	}
	if sess.Variant != "canary" {
		t.Errorf("Variant = %q, want %q", sess.Variant, "canary")
	}
}

func TestCohortHeaders_EmptyWhenNotSet(t *testing.T) {
	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, _ *ClientMessage, writer ResponseWriter) error {
			return writer.WriteDone("ok")
		},
	}

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

	// Connect without cohort headers
	ws, _, err := websocket.DefaultDialer.Dial(
		strings.Replace(ts.URL, "http://", "ws://", 1)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	var connMsg ServerMessage
	if err := ws.ReadJSON(&connMsg); err != nil {
		t.Fatalf("Failed to read connected: %v", err)
	}

	if err := ws.WriteJSON(ClientMessage{
		Type: MessageTypeMessage, SessionID: connMsg.SessionID, Content: "hi",
	}); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	var doneMsg ServerMessage
	if err := ws.ReadJSON(&doneMsg); err != nil {
		t.Fatalf("Failed to read done: %v", err)
	}

	sess, err := store.GetSession(context.Background(), connMsg.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.CohortID != "" {
		t.Errorf("CohortID = %q, want empty", sess.CohortID)
	}
	if sess.Variant != "" {
		t.Errorf("Variant = %q, want empty", sess.Variant)
	}
}

func TestCohortHeaders_SpanAttributes(t *testing.T) {
	provider, exporter := newTracingTestProvider(t)

	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, _ *ClientMessage, writer ResponseWriter) error {
			return writer.WriteDone("ok")
		},
	}

	ts := newTestServerWithTracing(t, handler, provider)

	headers := http.Header{}
	headers.Set(policy.HeaderCohortID, "cohort-xyz")
	headers.Set(policy.HeaderVariant, "stable")
	ws, _, err := websocket.DefaultDialer.Dial(
		strings.Replace(ts.URL, "http://", "ws://", 1)+"?agent=test-agent", headers)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	var connMsg ServerMessage
	if err := ws.ReadJSON(&connMsg); err != nil {
		t.Fatalf("Failed to read connected: %v", err)
	}

	if err := ws.WriteJSON(ClientMessage{
		Type: MessageTypeMessage, SessionID: connMsg.SessionID, Content: "hi",
	}); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	var doneMsg ServerMessage
	if err := ws.ReadJSON(&doneMsg); err != nil {
		t.Fatalf("Failed to read done: %v", err)
	}

	_ = ws.Close()
	time.Sleep(50 * time.Millisecond)

	spans := exporter.GetSpans()
	msgSpan := findSpanByName(spans, "omnia.facade.message")
	if msgSpan == nil {
		t.Fatal("expected 'omnia.facade.message' span")
	}

	cohortVal, ok := findSpanAttr(*msgSpan, "omnia.cohort.id")
	if !ok {
		t.Fatal("missing omnia.cohort.id attribute")
	}
	if cohortVal.AsString() != "cohort-xyz" {
		t.Errorf("omnia.cohort.id = %q, want %q", cohortVal.AsString(), "cohort-xyz")
	}

	variantVal, ok := findSpanAttr(*msgSpan, "omnia.variant")
	if !ok {
		t.Fatal("missing omnia.variant attribute")
	}
	if variantVal.AsString() != "stable" {
		t.Errorf("omnia.variant = %q, want %q", variantVal.AsString(), "stable")
	}
}

func TestCohortHeaders_SpanOmitsEmptyAttributes(t *testing.T) {
	provider, exporter := newTracingTestProvider(t)

	handler := &mockHandler{
		handleFunc: func(_ context.Context, _ string, _ *ClientMessage, writer ResponseWriter) error {
			return writer.WriteDone("ok")
		},
	}

	ts := newTestServerWithTracing(t, handler, provider)

	// No cohort headers
	ws, _, err := websocket.DefaultDialer.Dial(
		strings.Replace(ts.URL, "http://", "ws://", 1)+"?agent=test-agent", nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	var connMsg ServerMessage
	if err := ws.ReadJSON(&connMsg); err != nil {
		t.Fatalf("Failed to read connected: %v", err)
	}

	if err := ws.WriteJSON(ClientMessage{
		Type: MessageTypeMessage, SessionID: connMsg.SessionID, Content: "hi",
	}); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	var doneMsg ServerMessage
	if err := ws.ReadJSON(&doneMsg); err != nil {
		t.Fatalf("Failed to read done: %v", err)
	}

	_ = ws.Close()
	time.Sleep(50 * time.Millisecond)

	spans := exporter.GetSpans()
	msgSpan := findSpanByName(spans, "omnia.facade.message")
	if msgSpan == nil {
		t.Fatal("expected 'omnia.facade.message' span")
	}

	if _, ok := findSpanAttr(*msgSpan, "omnia.cohort.id"); ok {
		t.Error("omnia.cohort.id should not be set when header is empty")
	}
	if _, ok := findSpanAttr(*msgSpan, "omnia.variant"); ok {
		t.Error("omnia.variant should not be set when header is empty")
	}
}

func TestFacade_FirstMessageCachesSessionConsentGrants(t *testing.T) {
	c := &Connection{}
	msg := ClientMessage{
		Type:                 MessageTypeMessage,
		Content:              "hi",
		SessionConsentGrants: []string{"memory:preferences", "memory:context"},
	}
	captureSessionConsentGrants(c, &msg)

	c.mu.Lock()
	cached := c.sessionConsentGrants
	c.mu.Unlock()
	if len(cached) != 2 || cached[0] != "memory:preferences" || cached[1] != "memory:context" {
		t.Errorf("cached = %v, want [memory:preferences memory:context]", cached)
	}
}

func TestFacade_EmptySessionGrantsDoNotClearCache(t *testing.T) {
	c := &Connection{sessionConsentGrants: []string{"memory:preferences"}}
	msg := ClientMessage{
		Type:                 MessageTypeMessage,
		Content:              "hi",
		SessionConsentGrants: []string{}, // empty must NOT clear
	}
	captureSessionConsentGrants(c, &msg)

	c.mu.Lock()
	cached := c.sessionConsentGrants
	c.mu.Unlock()
	if len(cached) != 1 || cached[0] != "memory:preferences" {
		t.Errorf("cached = %v, want [memory:preferences]", cached)
	}
}

func TestFacade_EffectiveGrants_PerMessageWins(t *testing.T) {
	c := &Connection{sessionConsentGrants: []string{"memory:preferences"}}
	msg := ClientMessage{
		Type:          MessageTypeMessage,
		Content:       "hi",
		ConsentGrants: []string{"memory:identity"},
	}
	effective, layer := effectiveConsentGrants(c, &msg)
	if layer != "per-message" {
		t.Errorf("layer = %q, want \"per-message\"", layer)
	}
	if len(effective) != 1 || effective[0] != "memory:identity" {
		t.Errorf("effective = %v, want [memory:identity]", effective)
	}
}

func TestFacade_EffectiveGrants_SessionUsedWhenNoPerMessage(t *testing.T) {
	c := &Connection{sessionConsentGrants: []string{"memory:preferences"}}
	msg := ClientMessage{Type: MessageTypeMessage, Content: "hi"}
	effective, layer := effectiveConsentGrants(c, &msg)
	if layer != "session" {
		t.Errorf("layer = %q, want \"session\"", layer)
	}
	if len(effective) != 1 || effective[0] != "memory:preferences" {
		t.Errorf("effective = %v, want [memory:preferences]", effective)
	}
}

func TestFacade_EffectiveGrants_PersistentWhenNeitherSet(t *testing.T) {
	c := &Connection{}
	msg := ClientMessage{Type: MessageTypeMessage, Content: "hi"}
	effective, layer := effectiveConsentGrants(c, &msg)
	if layer != "persistent" {
		t.Errorf("layer = %q, want \"persistent\"", layer)
	}
	if effective != nil {
		t.Errorf("effective = %v, want nil", effective)
	}
}

func TestFacade_ResettingSessionGrantsReplaces(t *testing.T) {
	c := &Connection{sessionConsentGrants: []string{"memory:preferences"}}
	msg := ClientMessage{
		Type:                 MessageTypeMessage,
		Content:              "hi",
		SessionConsentGrants: []string{"memory:context"},
	}
	captureSessionConsentGrants(c, &msg)

	c.mu.Lock()
	cached := c.sessionConsentGrants
	c.mu.Unlock()
	if len(cached) != 1 || cached[0] != "memory:context" {
		t.Errorf("cached = %v, want [memory:context]", cached)
	}
}

// TestProcessMessage_MgmtPlaneJWTPrefersDeviceIDForUserScope reproduces a
// bug where the dashboard's "Try this agent" debug WS upgrade caused the
// facade to use the mgmt-plane operator pseudonym (`omnia-admin-<hash>` /
// `omnia-dashboard-proxy`) as the end-user identity for memory/session
// scoping. The dashboard's "My Memories" page queries memories under
// `pseudonymize(deviceId)`, so the two pseudonyms never matched and saved
// memories were invisible to the user.
//
// The mgmt-plane JWT subject identifies the *operator* (used in audit so
// we can tell admins apart). It is NOT the end user. When the request also
// carries a `device_id` query param (always set by the dashboard's WS
// upgrade), the facade must scope to the deviceId so memories saved during
// a debug session show up in the user's memory list.
func TestProcessMessage_MgmtPlaneJWTPrefersDeviceIDForUserScope(t *testing.T) {
	var capturedCtx context.Context

	handler := &mockHandler{
		handleFunc: func(ctx context.Context, _ string, _ *ClientMessage, writer ResponseWriter) error {
			capturedCtx = ctx
			return writer.WriteDone("ok")
		},
	}

	// Mint a real mgmt-plane JWT signed by an RSA key we control, then
	// install the validator so the facade actually parses it.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	pubPath := writeTestPubKeyPEM(t, t.TempDir(), &key.PublicKey)
	validator, err := auth.NewMgmtPlaneValidator(pubPath)
	if err != nil {
		t.Fatalf("NewMgmtPlaneValidator: %v", err)
	}

	const operatorSubject = "omnia-admin-deadbeef00000000"
	const deviceID = "fc725bcc-c5d7-4bef-bef4-a441d29ba7ec"
	now := time.Now()
	tokenString := mintTestMgmtPlaneToken(t, key, operatorSubject, now)

	store := session.NewMemoryStore()
	cfg := DefaultServerConfig()
	cfg.PingInterval = 100 * time.Millisecond
	cfg.PongTimeout = 200 * time.Millisecond

	log := logr.Discard()
	server := NewServer(cfg, store, handler, log, WithMgmtPlaneValidator(validator))

	ts := httptest.NewServer(server)
	t.Cleanup(func() {
		ts.Close()
		_ = store.Close()
	})

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+tokenString)
	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) +
		"?agent=test-agent&device_id=" + deviceID
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = ws.Close() }()

	var connMsg ServerMessage
	if err := ws.ReadJSON(&connMsg); err != nil {
		t.Fatalf("Failed to read connected: %v", err)
	}
	if err := ws.WriteJSON(ClientMessage{
		Type: MessageTypeMessage, SessionID: connMsg.SessionID, Content: "hi",
	}); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}
	var doneMsg ServerMessage
	if err := ws.ReadJSON(&doneMsg); err != nil {
		t.Fatalf("Failed to read done: %v", err)
	}

	got := policy.UserID(capturedCtx)
	wantDevice := identity.PseudonymizeID(deviceID)
	wantOperator := identity.PseudonymizeID(operatorSubject)

	if got == wantOperator {
		t.Fatalf("policy.UserID(ctx) = %q (operator pseudonym); "+
			"facade should scope memories to the device_id, not the mgmt-plane JWT subject", got)
	}
	if got != wantDevice {
		t.Fatalf("policy.UserID(ctx) = %q, want %q (pseudonymize(device_id))", got, wantDevice)
	}
}

// writeTestPubKeyPEM writes an RSA public key as a PKIX PEM file and
// returns the path. The MgmtPlaneValidator accepts both PUBLIC KEY and
// CERTIFICATE blocks; PUBLIC KEY is the simpler test fixture.
func writeTestPubKeyPEM(t *testing.T, dir string, pub *rsa.PublicKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey: %v", err)
	}
	path := filepath.Join(dir, "pub.pem")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create pub.pem: %v", err)
	}
	defer func() { _ = f.Close() }()
	if err := pem.Encode(f, &pem.Block{Type: "PUBLIC KEY", Bytes: der}); err != nil {
		t.Fatalf("encode pub.pem: %v", err)
	}
	return path
}

// mintTestMgmtPlaneToken signs a mgmt-plane JWT matching the validator's
// default issuer/audience so the facade admits it.
func mintTestMgmtPlaneToken(t *testing.T, key *rsa.PrivateKey, subject string, now time.Time) string {
	t.Helper()
	type claims struct {
		jwt.RegisteredClaims
		Origin    string `json:"origin,omitempty"`
		Agent     string `json:"agent,omitempty"`
		Workspace string `json:"workspace,omitempty"`
	}
	c := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.DefaultMgmtPlaneIssuer,
			Subject:   subject,
			Audience:  jwt.ClaimStrings{auth.DefaultMgmtPlaneAudience},
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Origin:    policy.OriginManagementPlane,
		Agent:     "test-agent",
		Workspace: "default",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}
