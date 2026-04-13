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

// TestBuildAPIMux_EnterpriseConsentRoutesWired verifies the wiring contract
// that enterprise consent routes are registered on the real server's mux when
// --enterprise is set. This catches the class of bug where a handler is built
// and unit-tested but never actually registered.
//
// This is a wiring test, not a functional test: it calls the real buildAPIMux
// (no mocks for the wiring layer) with a bogus pool that never dials. An
// unregistered route returns 404; a registered route handled by the consent
// handler will return some other status (the consent handler may 500 because
// the DB is unreachable, but that still proves wiring).
func TestBuildAPIMux_EnterpriseConsentRoutesWired(t *testing.T) {
	freshPromRegistry(t)
	pool := newBogusPool(t)
	registry := providers.NewRegistry()
	f := &flags{
		enterprise:  true,
		apiAddr:     ":0",
		healthAddr:  ":0",
		metricsAddr: ":0",
	}

	handler, _, cleanup := buildAPIMux(pool, registry, f, logr.Discard())
	defer cleanup()

	// Short deadline so the request can't hang on the unreachable DB.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/privacy/preferences/alice/consent",
		nil,
	).WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusNotFound {
		t.Errorf("enterprise consent GET route not registered on the real mux; "+
			"buildAPIMux returned 404. body=%q", rr.Body.String())
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

	handler, _, cleanup := buildAPIMux(pool, registry, f, logr.Discard())
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

	handler, _, cleanup := buildAPIMux(pool, registry, f, logr.Discard())
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

	handler, _, cleanup := buildAPIMux(pool, registry, f, logr.Discard())
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

	handler, _, cleanup := buildAPIMux(pool, registry, f, logr.Discard())
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
