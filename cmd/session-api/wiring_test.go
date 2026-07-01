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
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/altairalabs/omnia/internal/session/providers"
)

// freshPromRegistry swaps the default Prometheus registerer for the duration
// of a test. buildAPIMux → NewHTTPMetrics registers collectors against the
// default registry via promauto, so running buildAPIMux more than once in the
// same process panics with "duplicate metrics collector registration". Tests
// that call buildAPIMux must isolate themselves.
func freshPromRegistry(t *testing.T) {
	t.Helper()
	prev := prometheus.DefaultRegisterer
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
	t.Cleanup(func() { prometheus.DefaultRegisterer = prev })
}

// newBogusPool returns a *pgxpool.Pool with MinConns=0 that points at an
// unreachable address. pgxpool does not dial at construction when MinConns=0,
// so wiring tests that only verify route registration do not block on the
// database.
func newBogusPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig("postgres://test:test@127.0.0.1:1/test")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	cfg.MinConns = 0
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestBuildAPIMux_EnterpriseConsentRoutesNotHosted verifies that session-api no
// longer hosts consent routes even when --enterprise is set. Consent ownership
// was moved to privacy-api in #1642 (Slice B). The route must return 404 here
// so that the dashboard knows to talk to privacy-api directly.
//
// This inverts the previous "routes are wired" assertion: an unregistered route
// returns 404, which is exactly the desired post-migration state.
func TestBuildAPIMux_EnterpriseConsentRoutesNotHosted(t *testing.T) {
	freshPromRegistry(t)
	pool := newBogusPool(t)
	registry := providers.NewRegistry()
	f := &flags{
		enterprise:  true,
		apiAddr:     ":0",
		healthAddr:  ":0",
		metricsAddr: ":0",
	}

	handler, _, cleanup := buildAPIMux(pool, registry, f, logr.Discard(), nil, nil, nil)
	defer cleanup()

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/privacy/preferences/alice/consent",
		nil,
	)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("consent GET route must not be registered in session-api (privacy-api owns it); "+
			"got %d body=%q", rr.Code, rr.Body.String())
	}
}

// TestBuildAPIMux_NoEnterprise_ConsentRoutesNotWired is the negative
// counterpart: without --enterprise, the consent routes must not be
// registered.
func TestBuildAPIMux_NoEnterprise_ConsentRoutesNotWired(t *testing.T) {
	freshPromRegistry(t)
	pool := newBogusPool(t)
	registry := providers.NewRegistry()
	f := &flags{
		enterprise:  false,
		apiAddr:     ":0",
		healthAddr:  ":0",
		metricsAddr: ":0",
	}

	handler, _, cleanup := buildAPIMux(pool, registry, f, logr.Discard(), nil, nil, nil)
	defer cleanup()

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/privacy/preferences/alice/consent",
		nil,
	)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("consent route should not be registered without --enterprise; "+
			"got status %d, body=%q", rr.Code, rr.Body.String())
	}
}

// TestBuildAPIMux_PrivacyPolicyRouteWired verifies that GET /api/v1/privacy-policy
// is registered on the real mux via buildAPIMux in enterprise mode. This catches
// the class of bug where the privacy handler exists and is unit-tested but is
// never connected to the mux.
//
// In wiring tests we can't build a real PolicyWatcher (no K8s cluster), so the
// endpoint returns 204 (graceful degradation when no resolver is set). Either
// 200 or 204 proves the route is wired; 404 indicates a registration regression.
func TestBuildAPIMux_PrivacyPolicyRouteWired(t *testing.T) {
	freshPromRegistry(t)
	pool := newBogusPool(t)
	registry := providers.NewRegistry()
	f := &flags{
		enterprise:  true,
		apiAddr:     ":0",
		healthAddr:  ":0",
		metricsAddr: ":0",
	}

	handler, _, cleanup := buildAPIMux(pool, registry, f, logr.Discard(), nil, nil, nil)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy-policy?namespace=default&agent=x", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("GET /api/v1/privacy-policy should be registered on the real mux; "+
			"buildAPIMux returned 404. body=%q", rr.Body.String())
	}
	if rr.Code != http.StatusOK && rr.Code != http.StatusNoContent {
		t.Errorf("expected 200 or 204 from the privacy policy endpoint, got %d body=%q",
			rr.Code, rr.Body.String())
	}
}

// TestBuildAPIMux_EnterpriseAuditRoutesWired verifies the audit query
// endpoint (GET /api/v1/audit/sessions) is registered when --enterprise
// is set. The audit.Handler.RegisterRoutes call in
// registerEnterpriseRoutes is the wiring boundary; if it's ever
// commented out or moved behind another flag, the audit query API
// silently disappears.
func TestBuildAPIMux_EnterpriseAuditRoutesWired(t *testing.T) {
	freshPromRegistry(t)
	pool := newBogusPool(t)
	registry := providers.NewRegistry()
	f := &flags{
		enterprise:  true,
		apiAddr:     ":0",
		healthAddr:  ":0",
		metricsAddr: ":0",
	}

	handler, _, cleanup := buildAPIMux(pool, registry, f, logr.Discard(), nil, nil, nil)
	defer cleanup()

	// Invalid 'to' parameter forces handleQuery to short-circuit with 400
	// before touching the (unreachable) Postgres pool. Proves the route
	// is registered without requiring a working DB.
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/audit/sessions?to=not-a-timestamp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("GET /api/v1/audit/sessions should be registered in enterprise mode; got 404")
	}
}

// TestBuildAPIMux_NonEnterprise_AuditRoutesAbsent is the negative
// counterpart: without --enterprise, the audit query route must NOT be
// registered.
func TestBuildAPIMux_NonEnterprise_AuditRoutesAbsent(t *testing.T) {
	freshPromRegistry(t)
	pool := newBogusPool(t)
	registry := providers.NewRegistry()
	f := &flags{
		enterprise:  false,
		apiAddr:     ":0",
		healthAddr:  ":0",
		metricsAddr: ":0",
	}

	handler, _, cleanup := buildAPIMux(pool, registry, f, logr.Discard(), nil, nil, nil)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/audit/sessions?to=not-a-timestamp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("audit route must not be registered without --enterprise; got %d", rr.Code)
	}
}

// TestBuildAPIMux_EnterpriseEraseRouteWired verifies the session-tier DSAR
// erasure endpoint (POST /api/v1/privacy/sessions/delete-by-user) is registered
// when --enterprise is set. privacy-api calls this route to erase a group's
// sessions; if the SessionEraseHandler.RegisterRoutes call in
// registerEnterpriseRoutes is dropped, DSAR fan-out silently breaks.
func TestBuildAPIMux_EnterpriseEraseRouteWired(t *testing.T) {
	freshPromRegistry(t)
	pool := newBogusPool(t)
	registry := providers.NewRegistry()
	f := &flags{
		enterprise:  true,
		apiAddr:     ":0",
		healthAddr:  ":0",
		metricsAddr: ":0",
	}

	handler, _, cleanup := buildAPIMux(pool, registry, f, logr.Discard(), nil, nil, nil)
	defer cleanup()

	// Empty body → missing virtual_user_id → the eraser fails closed with 400
	// before touching the (unreachable) warm store. Proves the route is
	// registered without requiring a working DB.
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/privacy/sessions/delete-by-user", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("POST /api/v1/privacy/sessions/delete-by-user should be registered in enterprise mode; got 404")
	}
	if rr.Code != http.StatusBadRequest {
		t.Errorf("delete-by-user with empty body should fail closed 400; got %d", rr.Code)
	}
}

// TestBuildAPIMux_NonEnterprise_EraseRouteAbsent is the negative counterpart:
// without --enterprise, the delete-by-user route must NOT be registered.
func TestBuildAPIMux_NonEnterprise_EraseRouteAbsent(t *testing.T) {
	freshPromRegistry(t)
	pool := newBogusPool(t)
	registry := providers.NewRegistry()
	f := &flags{
		enterprise:  false,
		apiAddr:     ":0",
		healthAddr:  ":0",
		metricsAddr: ":0",
	}

	handler, _, cleanup := buildAPIMux(pool, registry, f, logr.Discard(), nil, nil, nil)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/privacy/sessions/delete-by-user", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("delete-by-user route must not be registered without --enterprise; got %d", rr.Code)
	}
}

// TestBuildAPIMux_NonEnterprise_PrivacyPolicyReturns204 verifies that in
// non-enterprise mode the privacy-policy endpoint is registered (route exists)
// but returns 204 because SetPolicyResolver is only called in the enterprise
// path. This ensures non-enterprise deployments degrade gracefully rather than
// accidentally 404 or leak data.
func TestBuildAPIMux_NonEnterprise_PrivacyPolicyReturns204(t *testing.T) {
	freshPromRegistry(t)
	pool := newBogusPool(t)
	registry := providers.NewRegistry()
	f := &flags{
		enterprise:  false,
		apiAddr:     ":0",
		healthAddr:  ":0",
		metricsAddr: ":0",
	}

	handler, _, cleanup := buildAPIMux(pool, registry, f, logr.Discard(), nil, nil, nil)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy-policy?namespace=default&agent=x", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("GET /api/v1/privacy-policy should be registered even in non-enterprise mode; "+
			"got 404. body=%q", rr.Body.String())
	}
	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204 (no resolver in non-enterprise mode), got %d body=%q",
			rr.Code, rr.Body.String())
	}
}

// TestBuildAPIMux_HealthzNotGatedByEnterprise verifies a smoke route exists on
// the main API handler regardless of enterprise mode. The session API exposes
// core session CRUD routes on the main mux; this test targets a stable one.
func TestBuildAPIMux_SessionRoutesWiredRegardlessOfEnterprise(t *testing.T) {
	freshPromRegistry(t)
	pool := newBogusPool(t)
	registry := providers.NewRegistry()
	f := &flags{
		enterprise:  false,
		apiAddr:     ":0",
		healthAddr:  ":0",
		metricsAddr: ":0",
	}

	handler, _, cleanup := buildAPIMux(pool, registry, f, logr.Discard(), nil, nil, nil)
	defer cleanup()

	// GET /api/v1/sessions should be registered (part of the core API handler).
	// It will likely 500 because the pool is unreachable, but it must not 404.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("core session route /api/v1/sessions not registered; buildAPIMux returned 404")
	}
}
