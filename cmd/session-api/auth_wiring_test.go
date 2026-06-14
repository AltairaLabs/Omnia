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
	"reflect"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/session/providers"
)

// fakeReviewer is a configurable serviceauth.TokenReviewer for wiring tests
// (no real Kubernetes API). It reviews every token as the configured subject.
type fakeReviewer struct {
	authenticated bool
	username      string
	err           error
}

func (f fakeReviewer) ReviewToken(_ context.Context, _ string) (bool, string, error) {
	return f.authenticated, f.username, f.err
}

const allowedSubject = "system:serviceaccount:ns:allowed"

// TestBuildAPIMux_AuthMiddlewareWired verifies that when buildAPIMux is given a
// non-nil reviewer and an allowlist, the ServiceAccount auth middleware is
// actually wired into the API handler chain: unauthenticated requests are
// rejected with 401, allowlisted subjects reach the handler, non-allowlisted
// subjects get 403, and /healthz stays exempt.
//
// This is a wiring test: it exercises the real buildAPIMux assembly with a fake
// reviewer, catching the failure mode where the middleware exists but is never
// installed in the chain.
func TestBuildAPIMux_AuthMiddlewareWired(t *testing.T) {
	freshPromRegistry(t)
	pool := newBogusPool(t)
	registry := providers.NewRegistry()
	f := &flags{
		enterprise:  false,
		apiAddr:     ":0",
		healthAddr:  ":0",
		metricsAddr: ":0",
	}

	reviewer := fakeReviewer{authenticated: true, username: allowedSubject}
	handler, _, cleanup := buildAPIMux(
		pool, registry, f, logr.Discard(),
		reviewer, []string{allowedSubject}, nil,
	)
	defer cleanup()

	t.Run("no Authorization header is 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 with no token, got %d body=%q", rr.Code, rr.Body.String())
		}
	})

	t.Run("allowlisted subject reaches handler", func(t *testing.T) {
		// Reviewer reviews any token as the allowed subject, so the request
		// passes auth and reaches the session handler. The handler will try the
		// (unreachable) DB and not-404 — anything other than 401/403 proves auth
		// passed and the request reached the wrapped handler.
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer good-token")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code == http.StatusUnauthorized || rr.Code == http.StatusForbidden {
			t.Errorf("allowlisted subject should pass auth; got %d body=%q", rr.Code, rr.Body.String())
		}
		if rr.Code == http.StatusNotFound {
			t.Errorf("session route not reached (404) — auth or wiring broke the chain")
		}
	})

	t.Run("healthz exempt with no token is 200", func(t *testing.T) {
		// /healthz is exempt from auth in the API handler chain. (The probe path
		// also lives on the separate health server; this asserts the API handler
		// does not gate it.)
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("/healthz should be exempt and 200, got %d body=%q", rr.Code, rr.Body.String())
		}
	})
}

// TestBuildAPIMux_NonAllowlistedSubjectIs403 verifies that a caller who
// authenticates successfully but whose subject is not on the allowlist is
// rejected with 403 (not 401). Separate top-level test so it gets its own fresh
// Prometheus registry (buildAPIMux registers collectors on the default
// registerer).
func TestBuildAPIMux_NonAllowlistedSubjectIs403(t *testing.T) {
	freshPromRegistry(t)
	pool := newBogusPool(t)
	registry := providers.NewRegistry()
	f := &flags{apiAddr: ":0", healthAddr: ":0", metricsAddr: ":0"}

	otherReviewer := fakeReviewer{authenticated: true, username: "system:serviceaccount:ns:other"}
	handler, _, cleanup := buildAPIMux(
		pool, registry, f, logr.Discard(),
		otherReviewer, []string{allowedSubject}, nil,
	)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer good-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("non-allowlisted subject should be 403, got %d body=%q", rr.Code, rr.Body.String())
	}
}

// TestBuildAPIMux_NamespaceAllowedFacadeReachesHandler verifies the fix for the
// SEC-1 functional hole: a per-AgentRuntime facade SA
// (system:serviceaccount:<ws-ns>:<name>-facade) that is NOT in the exact-subject
// allowlist still passes auth when its namespace is in allowed-namespaces. This
// is the case that previously 403'd facades and silently stopped session
// recording.
func TestBuildAPIMux_NamespaceAllowedFacadeReachesHandler(t *testing.T) {
	freshPromRegistry(t)
	pool := newBogusPool(t)
	registry := providers.NewRegistry()
	f := &flags{apiAddr: ":0", healthAddr: ":0", metricsAddr: ":0"}

	// Facade subject in the workspace namespace; not in allowedSubjects.
	facade := fakeReviewer{authenticated: true, username: "system:serviceaccount:ws-ns:foo-facade"}
	handler, _, cleanup := buildAPIMux(
		pool, registry, f, logr.Discard(),
		facade, []string{allowedSubject}, []string{"ws-ns"},
	)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer good-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code == http.StatusUnauthorized || rr.Code == http.StatusForbidden {
		t.Errorf("facade SA in allowed namespace should pass auth; got %d body=%q", rr.Code, rr.Body.String())
	}
	if rr.Code == http.StatusNotFound {
		t.Errorf("session route not reached (404) — auth or wiring broke the chain")
	}
}

// TestBuildAPIMux_NilReviewer_NoAuth verifies that when the reviewer is nil
// (auth disabled), buildAPIMux does not gate requests — the chain passes
// through to the handler with no Authorization header.
func TestBuildAPIMux_NilReviewer_NoAuth(t *testing.T) {
	freshPromRegistry(t)
	pool := newBogusPool(t)
	registry := providers.NewRegistry()
	f := &flags{apiAddr: ":0", healthAddr: ":0", metricsAddr: ":0"}

	handler, _, cleanup := buildAPIMux(pool, registry, f, logr.Discard(), nil, nil, nil)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code == http.StatusUnauthorized || rr.Code == http.StatusForbidden {
		t.Errorf("nil reviewer must not gate requests; got %d", rr.Code)
	}
}

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"whitespace only", "  ,  , ", nil},
		{"single", "a", []string{"a"}},
		{"trims spaces", " a , b ,c ", []string{"a", "b", "c"}},
		{"drops empties", "a,,b,", []string{"a", "b"}},
		{
			"real subjects",
			"system:serviceaccount:omnia-system:omnia-facade, system:serviceaccount:omnia-system:omnia-dashboard",
			[]string{
				"system:serviceaccount:omnia-system:omnia-facade",
				"system:serviceaccount:omnia-system:omnia-dashboard",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitAndTrim(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitAndTrim(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}
