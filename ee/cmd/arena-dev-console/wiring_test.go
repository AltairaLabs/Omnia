/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
)

// TestBuildFacadeMux_RoutesRegistered asserts the dev console's three
// documented HTTP routes are registered on the mux returned by
// buildFacadeMux. Each route is the contract between the dev console and
// the dashboard's reload/test workflow — if a Handle/HandleFunc call is
// removed from main, the dashboard silently 404s.
//
// The wsServer arg is a no-op stub: registration is what we assert, not
// behaviour. The PromptKitHandler is nil; handlers tolerate nil and return
// 503 / empty results — anything other than 404 proves the route exists.
func TestBuildFacadeMux_RoutesRegistered(t *testing.T) {
	wsStub := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	})
	mux := buildFacadeMux(wsStub, nil, logr.Discard(), nil, true)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"websocket endpoint", http.MethodGet, "/ws"},
		{"providers endpoint", http.MethodGet, "/api/providers"},
		{"reload endpoint", http.MethodPost, "/api/reload?path=ignored"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			if rr.Code == http.StatusNotFound {
				t.Errorf("%s %s should be registered, got 404; body=%q",
					tc.method, tc.path, rr.Body.String())
			}
		})
	}
}

// TestHandleListProviders_NilHandler verifies the GET handler degrades
// gracefully when the PromptKitHandler hasn't been initialised. The dev
// console can boot without a config file; the providers list endpoint
// must still respond (with an empty list) rather than crash.
func TestHandleListProviders_NilHandler(t *testing.T) {
	h := handleListProviders(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 when handler nil, got %d", rr.Code)
	}
	if rr.Body.String() != "[]" {
		t.Errorf("expected empty list body, got %q", rr.Body.String())
	}
}

// TestHandleListProviders_MethodNotAllowed verifies non-GET methods are
// rejected. Locks the contract that this is a read-only endpoint.
func TestHandleListProviders_MethodNotAllowed(t *testing.T) {
	h := handleListProviders(nil)
	req := httptest.NewRequest(http.MethodPost, "/api/providers", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST, got %d", rr.Code)
	}
}

// TestHandleReload_NilHandler verifies the POST handler responds 503
// (not crash) when the PromptKitHandler hasn't been initialised.
func TestHandleReload_NilHandler(t *testing.T) {
	h := handleReload(nil, logr.Discard())
	req := httptest.NewRequest(http.MethodPost, "/api/reload?path=cfg.yaml", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when handler nil, got %d", rr.Code)
	}
}

// TestHandleReload_MethodNotAllowed verifies non-POST methods are rejected.
func TestHandleReload_MethodNotAllowed(t *testing.T) {
	h := handleReload(nil, logr.Discard())
	req := httptest.NewRequest(http.MethodGet, "/api/reload?path=cfg.yaml", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET, got %d", rr.Code)
	}
}

// TestHealthzHandler verifies the early-boot health endpoint returns 200
// with a plain "ok" body. The startHealthServer goroutine launches before
// service discovery, so liveness probes pass during the retry loop.
func TestHealthzHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	healthzHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %q", rr.Body.String())
	}
}
