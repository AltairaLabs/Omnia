/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/altairalabs/omnia/internal/agent"
	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/internal/media"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/pkg/identity"
	"github.com/altairalabs/omnia/pkg/policy"
)

// stubMediaStorage is a no-op media.Storage used by wiring tests to assert
// that cmd/agent threads the storage through to the facade. The methods are
// never invoked in the wiring test path — only the facade server's reference
// to the storage is checked.
type stubMediaStorage struct{}

func (stubMediaStorage) GetUploadURL(context.Context, media.UploadRequest) (*media.UploadCredentials, error) {
	return &media.UploadCredentials{}, nil
}
func (stubMediaStorage) GetDownloadURL(context.Context, string) (string, error) { return "", nil }
func (stubMediaStorage) GetMediaInfo(context.Context, string) (*media.MediaInfo, error) {
	return &media.MediaInfo{}, nil
}
func (stubMediaStorage) Delete(context.Context, string) error { return nil }
func (stubMediaStorage) Close() error                         { return nil }

// freshPromRegistry swaps the default Prometheus registerer for the duration
// of a test. agent.NewMetrics and buildWebSocketServer register collectors on
// the default registry via promauto, so running the test more than once or in
// a package with other metrics tests would otherwise panic with "duplicate
// metrics collector registration".
func freshPromRegistry(t *testing.T) {
	t.Helper()
	prev := prometheus.DefaultRegisterer
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
	t.Cleanup(func() { prometheus.DefaultRegisterer = prev })
}

// captureHandler is a facade.MessageHandler stub that records the context
// passed to HandleMessage so the test can assert on policy propagation.
type captureHandler struct {
	mu          sync.Mutex
	capturedCtx context.Context
	name        string
}

func (h *captureHandler) Name() string { return h.name }

func (h *captureHandler) HandleMessage(
	ctx context.Context,
	_ string,
	_ *facade.ClientMessage,
	writer facade.ResponseWriter,
) error {
	h.mu.Lock()
	h.capturedCtx = ctx
	h.mu.Unlock()
	return writer.WriteDone("ok")
}

func (h *captureHandler) ctx() context.Context {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.capturedCtx
}

// TestBuildWebSocketServer_PseudonymizesUserIDHeader verifies the wiring
// contract that the WebSocket facade, when constructed via the real
// buildWebSocketServer that main() uses, pseudonymizes the X-User-Id header
// and propagates the value into the message handler's context as
// policy.UserID.
//
// A regression here would mean either:
//   - the facade no longer reads the Istio x-user-id header, or
//   - user IDs are not pseudonymized before being stored in the context, or
//   - cmd/agent stopped using the wiring from internal/facade.
func TestBuildWebSocketServer_PseudonymizesUserIDHeader(t *testing.T) {
	freshPromRegistry(t)

	store := session.NewMemoryStore()
	t.Cleanup(func() { _ = store.Close() })

	handler := &captureHandler{name: "wiring-test"}

	cfg := &agent.Config{
		AgentName:     "test-agent",
		Namespace:     "test-ns",
		WorkspaceName: "test-ws",
		SessionTTL:    5 * time.Minute,
	}
	metrics := agent.NewMetrics(cfg.AgentName, cfg.Namespace)

	wsServer, mux := buildWebSocketServer(cfg, logr.Discard(), store, handler, metrics, nil, nil)
	_ = wsServer // shut down implicitly when ts is closed

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	headers := http.Header{}
	headers.Set(policy.IstioHeaderUserID, "alice-raw")
	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?agent=test-agent"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer func() { _ = ws.Close() }()

	var connMsg facade.ServerMessage
	if err := ws.ReadJSON(&connMsg); err != nil {
		t.Fatalf("read connected: %v", err)
	}

	if err := ws.WriteJSON(facade.ClientMessage{
		Type:      facade.MessageTypeMessage,
		SessionID: connMsg.SessionID,
		Content:   "hello",
	}); err != nil {
		t.Fatalf("send message: %v", err)
	}

	var doneMsg facade.ServerMessage
	if err := ws.ReadJSON(&doneMsg); err != nil {
		t.Fatalf("read done: %v", err)
	}

	got := policy.UserID(handler.ctx())
	want := identity.PseudonymizeID("alice-raw")
	if got != want {
		t.Errorf("policy.UserID(ctx) = %q, want pseudonymized %q — "+
			"cmd/agent buildWebSocketServer is not wiring the X-User-Id header "+
			"into the handler context via the facade server",
			got, want)
	}
	if got == "alice-raw" {
		t.Errorf("user ID is not pseudonymized — raw X-User-Id value leaked into handler context")
	}
}

// TestBuildWebSocketServer_WiresMediaStorage verifies that cmd/agent threads
// media storage through buildWebSocketServer into the facade server via
// WithMediaStorage. Without this, the facade's mediaStorage field is nil and
// the WebSocket upload_request / upload_ready / upload_complete flow always
// fails with a "media storage not configured" error — even though the REST
// media routes work because they use the storage directly.
//
// This is the #728 item 1 regression guard.
func TestBuildWebSocketServer_WiresMediaStorage(t *testing.T) {
	freshPromRegistry(t)

	store := session.NewMemoryStore()
	t.Cleanup(func() { _ = store.Close() })

	cfg := &agent.Config{AgentName: "probe", Namespace: "ns"}
	metrics := agent.NewMetrics(cfg.AgentName, cfg.Namespace)
	handler := &captureHandler{name: "probe"}

	// With nil media storage: facade reports none wired.
	nilServer, _ := buildWebSocketServer(cfg, logr.Discard(), store, handler, metrics, nil, nil)
	if nilServer.HasMediaStorage() {
		t.Error("facade reports media storage wired when nil was passed")
	}

	// Fresh prom registry again because NewMetrics registers collectors.
	freshPromRegistry(t)
	metrics2 := agent.NewMetrics(cfg.AgentName, cfg.Namespace)

	// With non-nil media storage: facade reports it wired.
	withStorage, _ := buildWebSocketServer(cfg, logr.Discard(), store, handler, metrics2, nil, stubMediaStorage{})
	if !withStorage.HasMediaStorage() {
		t.Error("facade reports media storage not wired; buildWebSocketServer " +
			"is not forwarding the storage via facade.WithMediaStorage — " +
			"WebSocket upload_request will fail with mediaStorage == nil")
	}
}

// TestBuildWebSocketServer_RegistersWebSocketRoutes verifies the real mux
// returned by buildWebSocketServer has /ws and /api/agents/ routes registered.
// /metrics is not asserted here because it is registered unconditionally by
// the same helper.
func TestBuildWebSocketServer_RegistersWebSocketRoutes(t *testing.T) {
	freshPromRegistry(t)

	store := session.NewMemoryStore()
	t.Cleanup(func() { _ = store.Close() })

	cfg := &agent.Config{AgentName: "probe", Namespace: "ns"}
	metrics := agent.NewMetrics(cfg.AgentName, cfg.Namespace)
	handler := &captureHandler{name: "probe"}

	_, mux := buildWebSocketServer(cfg, logr.Discard(), store, handler, metrics, nil, nil)

	// /ws should at minimum not 404 (will 400 on a non-upgrade GET).
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code == http.StatusNotFound {
		t.Errorf("/ws route not registered on buildWebSocketServer mux")
	}

	// /api/agents/ should also be registered.
	req = httptest.NewRequest(http.MethodGet, "/api/agents/something", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code == http.StatusNotFound {
		t.Errorf("/api/agents/ route not registered on buildWebSocketServer mux")
	}
}
