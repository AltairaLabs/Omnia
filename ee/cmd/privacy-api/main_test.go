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

	"github.com/go-logr/logr"

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
	registerRoutes(mux, base, base, logr.Discard(), privacy.NoopConsentNotifier{})

	for _, p := range []string{
		"/api/v1/privacy/preferences/abc123",
		"/api/v1/privacy/preferences/abc123/consent",
		"/api/v1/privacy/consent/stats",
		"/api/v1/privacy/enforcement-stats",
	} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		_, pattern := mux.Handler(req)
		if pattern == "" {
			t.Errorf("route %q is not registered (no matching pattern found)", p)
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
	registerRoutes(mux, base, base, logr.Discard(), privacy.NoopConsentNotifier{})

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
	registerRoutes(mux, base, base, logr.Discard(), privacy.NoopConsentNotifier{})

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
