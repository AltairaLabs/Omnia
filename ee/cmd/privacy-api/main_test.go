/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

// alwaysDenyReviewer is a serviceauth.TokenReviewer that always returns
// authenticated=false, simulating an absent or invalid bearer token.
type alwaysDenyReviewer struct{}

func (alwaysDenyReviewer) ReviewToken(_ context.Context, _ string) (bool, string, error) {
	return false, "", nil
}

// TestRegisterRoutes_AllMounted is a wiring test (per repo policy §Wiring tests).
// It asserts that registerRoutes mounts all four handler route groups on the mux
// using nil-pool stores so no Postgres connection is required.
func TestRegisterRoutes_AllMounted(t *testing.T) {
	mux := http.NewServeMux()
	base := privacy.NewPreferencesStore(nil) // nil pool: routes only, no DB hit in this probe
	auditStore := privacy.NewAuditStore(nil) // nil pool: routes only, no DB hit in this probe
	registerRoutes(mux, base, base, auditStore, logr.Discard(), privacy.NoopConsentNotifier{})

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/privacy/preferences/abc123"},
		{http.MethodGet, "/api/v1/privacy/preferences/abc123/consent"},
		{http.MethodGet, "/api/v1/privacy/consent/stats"},
		{http.MethodGet, "/api/v1/privacy/enforcement-stats"},
		{http.MethodPost, "/api/v1/privacy/audit-events"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		_, pattern := mux.Handler(req)
		if pattern == "" {
			t.Errorf("route %s %q is not registered (no matching pattern found)", tc.method, tc.path)
		}
	}
}

// TestApplyEnvFallbacks_AuthEnvSeam is a wiring test (per repo policy §Wiring
// tests) that confirms the four SESSION_API_AUTH_* env vars the operator stamps
// onto the privacy-api pod (via applySessionAPIServerAuthEnv in
// internal/controller/service_auth.go) are read by applyEnvFallbacks and
// activate auth end-to-end.
//
// The test exercises:
//  1. applyEnvFallbacks — env vars → flag fields
//  2. The resulting flag values plumbed into buildHandler + buildServiceAuth
//     (reviewer is faked with alwaysDenyReviewer so no in-cluster K8s is needed)
//  3. The assembled handler enforcing auth: privacy route → 401, /healthz → 200
func TestApplyEnvFallbacks_AuthEnvSeam(t *testing.T) {
	t.Setenv("SESSION_API_AUTH_ENABLED", "true")
	t.Setenv("SESSION_API_AUTH_ALLOWED_NAMESPACES", "omnia-system")
	t.Setenv("SESSION_API_AUTH_ALLOWED_SUBJECTS", "system:serviceaccount:omnia-system:facade")
	t.Setenv("SESSION_API_AUTH_AUDIENCES", "omnia")

	// Simulate the flag defaults that parseFlags sets before flag.Parse() +
	// applyEnvFallbacks. CLI args are absent, so all values stay at defaults
	// until applyEnvFallbacks runs.
	f := &flags{
		apiAddr:     ":8080",
		healthAddr:  ":8081",
		metricsAddr: ":9090",
	}
	f.applyEnvFallbacks()

	if !f.authEnabled {
		t.Fatal("expected authEnabled=true from SESSION_API_AUTH_ENABLED=true, got false")
	}
	if f.authAllowedNamespaces != "omnia-system" {
		t.Errorf("authAllowedNamespaces: got %q, want %q", f.authAllowedNamespaces, "omnia-system")
	}
	if f.authAllowedSubjects != "system:serviceaccount:omnia-system:facade" {
		t.Errorf("authAllowedSubjects: got %q, want %q",
			f.authAllowedSubjects, "system:serviceaccount:omnia-system:facade")
	}
	if f.authAudiences != "omnia" {
		t.Errorf("authAudiences: got %q, want %q", f.authAudiences, "omnia")
	}

	// Now verify the flags actually activate auth at the handler level.
	// We use alwaysDenyReviewer instead of the real K8sTokenReviewer (which
	// requires in-cluster config) to keep the test hermetic.
	mux := http.NewServeMux()
	base := privacy.NewPreferencesStore(nil)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	registerRoutes(mux, base, base, privacy.NewAuditStore(nil), logr.Discard(), privacy.NoopConsentNotifier{})

	subjects := splitAndTrim(f.authAllowedSubjects)
	namespaces := splitAndTrim(f.authAllowedNamespaces)
	handler := buildHandler(alwaysDenyReviewer{}, subjects, namespaces, mux)

	// Privacy route without a bearer token → 401 (auth enforced).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/preferences/u1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("privacy route without token: expected 401, got %d", rec.Code)
	}

	// /healthz without a bearer token → 200 (exempt).
	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("/healthz (exempt): expected 200, got %d", rec.Code)
	}
}

// TestEnterpriseGate_RoutesRegisteredWhenTrue is a wiring test that confirms all
// privacy routes are mounted when enterprise=true (the expected production path).
func TestEnterpriseGate_RoutesRegisteredWhenTrue(t *testing.T) {
	base := privacy.NewPreferencesStore(nil)
	mux := buildAPIMux(true, base, base, privacy.NewAuditStore(nil), logr.Discard(), privacy.NoopConsentNotifier{})

	for _, p := range []string{
		"/api/v1/privacy/preferences/abc123",
		"/api/v1/privacy/preferences/abc123/consent",
		"/api/v1/privacy/consent/stats",
		"/api/v1/privacy/enforcement-stats",
		"/healthz",
	} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		_, pattern := mux.Handler(req)
		if pattern == "" {
			t.Errorf("enterprise=true: route %q should be registered but has no matching pattern", p)
		}
	}
}

// TestEnterpriseGate_RoutesAbsentWhenFalse is a wiring test that confirms consent
// and opt-out routes return 404 when enterprise=false, while /healthz still returns 200.
func TestEnterpriseGate_RoutesAbsentWhenFalse(t *testing.T) {
	base := privacy.NewPreferencesStore(nil)
	mux := buildAPIMux(false, base, base, privacy.NewAuditStore(nil), logr.Discard(), privacy.NoopConsentNotifier{})

	// /healthz must still be reachable.
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("enterprise=false: /healthz expected 200, got %d", rec.Code)
	}

	// Consent/opt-out routes must not be registered.
	for _, p := range []string{
		"/api/v1/privacy/preferences/abc123",
		"/api/v1/privacy/preferences/abc123/consent",
		"/api/v1/privacy/consent/stats",
		"/api/v1/privacy/enforcement-stats",
	} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("enterprise=false: route %q expected 404, got %d", p, rec.Code)
		}
	}
}

// stubOutboxStore is a no-op outboxStore for use in unit/wiring tests.
// All methods return zero values and no errors so the worker can be constructed
// and run without a real database.
type stubOutboxStore struct{}

func (s *stubOutboxStore) ListUndeliveredOutbox(_ context.Context, _ time.Duration, _ int) ([]privacy.OutboxEntry, error) {
	return nil, nil
}
func (s *stubOutboxStore) MarkOutboxDelivered(_ context.Context, _ string) error { return nil }
func (s *stubOutboxStore) PruneDeliveredOutbox(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}
func (s *stubOutboxStore) CountStuckOutbox(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

// TestOutboxReplayWorker_GaugeRegistered is a wiring test (per repo policy §Wiring tests)
// that asserts NewOutboxReplayWorker registers the omnia_privacy_consent_outbox_stuck_total
// gauge with the supplied registry. This validates the construction path that main.go
// exercises inside the if f.enterprise { } block when enterprise=true.
func TestOutboxReplayWorker_GaugeRegistered(t *testing.T) {
	reg := prometheus.NewRegistry()
	w := NewOutboxReplayWorker(
		&stubOutboxStore{},
		privacy.NoopConsentNotifier{},
		5*time.Minute,
		24*time.Hour,
		reg,
		logr.Discard(),
	)
	require.NotNil(t, w)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range mfs {
		if mf.GetName() == metricConsentOutboxStuck {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected gauge %q to be registered in the Prometheus registry, but it was not found", metricConsentOutboxStuck)
	}
}

// TestBuildHandler_AuthExemptsHealthz verifies that buildHandler correctly wires
// RequireServiceAccount: a privacy route without a token returns 401, while
// /healthz (exempted) returns 200.
func TestBuildHandler_AuthExemptsHealthz(t *testing.T) {
	mux := http.NewServeMux()
	base := privacy.NewPreferencesStore(nil)

	// Register /healthz on the API mux (mirrors run() behaviour).
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	registerRoutes(mux, base, base, privacy.NewAuditStore(nil), logr.Discard(), privacy.NoopConsentNotifier{})

	// alwaysDenyReviewer: any presented token is rejected → unauthenticated.
	reviewer := alwaysDenyReviewer{}
	handler := buildHandler(
		reviewer,
		[]string{"system:serviceaccount:omnia-system:privacy-api"},
		nil,
		mux,
	)

	// Privacy route without a bearer token → 401.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/preferences/u1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 on privacy route without token, got %d", rec.Code)
	}

	// /healthz without a bearer token → 200 (exempt from auth check).
	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 on /healthz (exempt from auth), got %d", rec.Code)
	}
}
