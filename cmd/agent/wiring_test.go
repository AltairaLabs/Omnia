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

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/altairalabs/omnia/internal/agent"
	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/internal/media"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/sessiontest"
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

// mustBuildWS builds the WebSocket server + mux via the real
// buildWebSocketServer and fails the test on error. The auth-setup error path
// no longer os.Exits (#1208); it returns an error, which these wiring tests
// surface here. tracingProvider is always nil in this package's tests.
func mustBuildWS(
	t *testing.T, cfg *agent.Config, store session.Store,
	handler facade.MessageHandler, metrics *agent.Metrics, ms media.Storage,
) (*facade.Server, *http.ServeMux) {
	t.Helper()
	servers, err := buildWebSocketServer(cfg, logr.Discard(), store, handler, metrics, nil, nil, ms)
	if err != nil {
		t.Fatalf("buildWebSocketServer: %v", err)
	}
	return servers.external, servers.externalMux
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
	// The pseudonymization contract is the subject here, not auth. Flip
	// the dev escape hatch so the strict default doesn't 401 the test's
	// unauthenticated WS dial (B2 wiring).
	t.Setenv(envFacadeAllowUnauthenticated, "true")

	store := sessiontest.NewStore()
	t.Cleanup(func() { _ = store.Close() })

	handler := &captureHandler{name: "wiring-test"}

	cfg := &agent.Config{
		AgentName:     "test-agent",
		Namespace:     "test-ns",
		WorkspaceName: "test-ws",
	}
	metrics := agent.NewMetrics(cfg.AgentName, cfg.Namespace)

	wsServer, mux := mustBuildWS(t, cfg, store, handler, metrics, nil)
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

// TestBuildWebSocketServer_StrictDefaultRejectsUnauthenticatedUpgrade
// proves B2 is fixed: when the env escape hatch is unset and the auth
// chain ends up empty (no externalAuth configured AND no mgmt-plane
// pubkey file available — the pod-startup boot-race configuration),
// the facade 401s the upgrade instead of admitting it. This is the
// residual C-3 bypass that PR 3 left open in cmd/agent.
func TestBuildWebSocketServer_StrictDefaultRejectsUnauthenticatedUpgrade(t *testing.T) {
	freshPromRegistry(t)
	// Default (unset) means strict rejection. Explicit clear to avoid
	// pollution from prior tests in the same package.
	t.Setenv(envFacadeAllowUnauthenticated, "")

	store := sessiontest.NewStore()
	t.Cleanup(func() { _ = store.Close() })

	handler := &captureHandler{name: "strict"}
	cfg := &agent.Config{
		AgentName:     "strict-agent",
		Namespace:     "test-ns",
		WorkspaceName: "test-ws",
	}
	metrics := agent.NewMetrics(cfg.AgentName, cfg.Namespace)

	_, mux := mustBuildWS(t, cfg, store, handler, metrics, nil)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?agent=strict-agent"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected dial error under strict default")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		var got int
		if resp != nil {
			got = resp.StatusCode
		}
		t.Errorf("status = %d, want 401 (strict default must reject empty-chain upgrade)", got)
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

	store := sessiontest.NewStore()
	t.Cleanup(func() { _ = store.Close() })

	cfg := &agent.Config{AgentName: probeAgentName, Namespace: "ns"}
	metrics := agent.NewMetrics(cfg.AgentName, cfg.Namespace)
	handler := &captureHandler{name: probeAgentName}

	// With nil media storage: facade reports none wired.
	nilServer, _ := mustBuildWS(t, cfg, store, handler, metrics, nil)
	if nilServer.HasMediaStorage() {
		t.Error("facade reports media storage wired when nil was passed")
	}

	// Fresh prom registry again because NewMetrics registers collectors.
	freshPromRegistry(t)
	metrics2 := agent.NewMetrics(cfg.AgentName, cfg.Namespace)

	// With non-nil media storage: facade reports it wired.
	withStorage, _ := mustBuildWS(t, cfg, store, handler, metrics2, stubMediaStorage{})
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

	store := sessiontest.NewStore()
	t.Cleanup(func() { _ = store.Close() })

	cfg := &agent.Config{AgentName: probeAgentName, Namespace: "ns"}
	metrics := agent.NewMetrics(cfg.AgentName, cfg.Namespace)
	handler := &captureHandler{name: probeAgentName}

	_, mux := mustBuildWS(t, cfg, store, handler, metrics, nil)

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

// TestBuildWebSocketServer_InternalTwinEnabled verifies that when an internal
// management-plane port is configured, buildWebSocketServer builds the internal
// twin server and mounts /ws on its mux.
func TestBuildWebSocketServer_InternalTwinEnabled(t *testing.T) {
	freshPromRegistry(t)
	store := sessiontest.NewStore()
	t.Cleanup(func() { _ = store.Close() })

	cfg := &agent.Config{AgentName: probeAgentName, Namespace: "ns", InternalFacadePort: agent.DefaultInternalFacadePort}
	metrics := agent.NewMetrics(cfg.AgentName, cfg.Namespace)
	handler := &captureHandler{name: probeAgentName}

	servers, err := buildWebSocketServer(cfg, logr.Discard(), store, handler, metrics, nil, nil, nil)
	if err != nil {
		t.Fatalf("buildWebSocketServer: %v", err)
	}
	if servers.internal == nil || servers.internalMux == nil {
		t.Fatal("internal twin server/mux is nil; want it built when InternalFacadePort is set")
	}
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rr := httptest.NewRecorder()
	servers.internalMux.ServeHTTP(rr, req)
	if rr.Code == http.StatusNotFound {
		t.Error("/ws route not registered on the internal management-plane mux")
	}
}

// TestBuildWebSocketServer_InternalTwinDisabled verifies that without an internal
// port the twin is not built (the mgmt plane is reached via the external chain
// in Milestone A, or disabled entirely).
func TestBuildWebSocketServer_InternalTwinDisabled(t *testing.T) {
	freshPromRegistry(t)
	store := sessiontest.NewStore()
	t.Cleanup(func() { _ = store.Close() })

	cfg := &agent.Config{AgentName: probeAgentName, Namespace: "ns"} // InternalFacadePort: 0
	metrics := agent.NewMetrics(cfg.AgentName, cfg.Namespace)
	handler := &captureHandler{name: probeAgentName}

	servers, err := buildWebSocketServer(cfg, logr.Discard(), store, handler, metrics, nil, nil, nil)
	if err != nil {
		t.Fatalf("buildWebSocketServer: %v", err)
	}
	if servers.internal != nil || servers.internalMux != nil {
		t.Error("internal twin server should be nil when InternalFacadePort is 0")
	}
}
