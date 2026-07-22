/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/agent"
	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/internal/session/sessiontest"
)

// TestReadyz_NotReadyWhenDraining verifies that the /readyz handler returns 503
// while the facade is draining. This is the wiring test for the drain-aware
// readiness probe: without this, Kubernetes keeps sending traffic during
// shutdown even though the facade is rejecting new upgrades.
func TestReadyz_NotReadyWhenDraining(t *testing.T) {
	t.Parallel()

	s := facade.NewServer(facade.DefaultServerConfig(), nil, nil, logr.Discard())
	h := readyzHandler(nil, nil, s)
	// No sessions → Drain returns immediately and marks draining.
	s.Drain(context.Background())

	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	h(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz should be 503 while draining, got %d", w.Code)
	}
}

// TestReadyz_ReadyWhenNotDraining verifies the handler returns 200 for a
// newly constructed server (not yet draining) so we know the drain check
// doesn't break normal readiness.
func TestReadyz_ReadyWhenNotDraining(t *testing.T) {
	t.Parallel()

	store := sessiontest.NewStore()
	t.Cleanup(func() { _ = store.Close() })

	s := facade.NewServer(facade.DefaultServerConfig(), store, nil, logr.Discard())
	h := readyzHandler(store, nil, s)

	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	h(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("readyz should be 200 when not draining, got %d", w.Code)
	}
}

// TestBuildWebSocketServer_PropagatesDrainTimeout verifies that when
// cfg.DrainTimeout is non-zero, buildWebSocketServer sets it on the
// facade server's drain timeout (exposed via DrainTimeoutForShutdown).
// This guards the wiring: without it the server always uses the 30s
// default regardless of what the CRD specifies.
func TestBuildWebSocketServer_PropagatesDrainTimeout(t *testing.T) {
	freshPromRegistry(t)

	store := sessiontest.NewStore()
	t.Cleanup(func() { _ = store.Close() })

	want := 90 * time.Second
	cfg := &agent.Config{
		AgentName:    probeAgentName,
		Namespace:    "ns",
		DrainTimeout: want,
	}
	metrics := agent.NewMetrics(cfg.AgentName, cfg.Namespace)
	handler := &captureHandler{name: probeAgentName}

	servers, err := buildWebSocketServer(cfg, logr.Discard(), store, handler, metrics, nil, nil, nil)
	if err != nil {
		t.Fatalf("buildWebSocketServer: %v", err)
	}
	srv := servers.external

	if got := srv.DrainTimeoutForShutdown(); got != want {
		t.Errorf("DrainTimeoutForShutdown() = %v, want %v — "+
			"buildWebSocketServer is not propagating cfg.DrainTimeout into the facade config",
			got, want)
	}
}

// TestBuildWebSocketServer_DefaultDrainTimeoutWhenCRDUnset verifies that when
// cfg.DrainTimeout is zero (CRD field unset), buildWebSocketServer preserves
// the 30s default from DefaultServerConfig rather than overwriting it with zero.
func TestBuildWebSocketServer_DefaultDrainTimeoutWhenCRDUnset(t *testing.T) {
	freshPromRegistry(t)

	store := sessiontest.NewStore()
	t.Cleanup(func() { _ = store.Close() })

	cfg := &agent.Config{
		AgentName:    "probe-drain-zero",
		Namespace:    "ns",
		DrainTimeout: 0, // unset — should NOT clobber the 30s default
	}
	metrics := agent.NewMetrics(cfg.AgentName, cfg.Namespace)
	handler := &captureHandler{name: "probe-drain-zero"}

	servers, err := buildWebSocketServer(cfg, logr.Discard(), store, handler, metrics, nil, nil, nil)
	if err != nil {
		t.Fatalf("buildWebSocketServer: %v", err)
	}
	srv := servers.external

	const defaultDrainTimeout = 30 * time.Second
	if got := srv.DrainTimeoutForShutdown(); got != defaultDrainTimeout {
		t.Errorf("DrainTimeoutForShutdown() = %v, want %v (30s default) — "+
			"buildWebSocketServer overwrote DefaultServerConfig DrainTimeout with zero",
			got, defaultDrainTimeout)
	}
}
