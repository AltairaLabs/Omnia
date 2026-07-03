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

	"github.com/altairalabs/omnia/internal/memory"
	memoryapi "github.com/altairalabs/omnia/internal/memory/api"
	"github.com/altairalabs/omnia/internal/memory/ingestion"
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

// newAuthWiringMux builds a buildAPIMux handler with the minimal wiring
// needed for auth middleware assertions: a no-op store, no embedding
// service/pool, and default (non-enterprise) config. Only the
// reviewer/allowlist parameters vary between tests.
func newAuthWiringMux(t *testing.T, reviewer interface {
	ReviewToken(context.Context, string) (bool, string, error)
}, allowedSubjects, allowedNamespaces []string) http.Handler {
	t.Helper()
	freshPromRegistry(t)
	handler, cleanup := buildAPIMux(
		context.Background(),
		fakeMemoryStore{},
		nil, // embedding service is optional
		memoryapi.MemoryServiceConfig{},
		nil, // event publisher is optional
		false,
		nil, // pool is only used by enterprise privacy middleware
		nil, // policy loader is optional — identity ranker without it
		nil, // auditLogger optional; non-enterprise tests don't exercise it
		logr.Discard(),
		memoryapi.IngestOptions{Fallback: ingestion.Config{
			Strategy: ingestion.StrategyChunk, ChunkSize: 200, ChunkOverlap: 40,
		}},
		"", "", // workspace, serviceGroup — empty in unit tests
		nil, // consentPruner — not needed in auth wiring tests
		reviewer, allowedSubjects, allowedNamespaces,
	)
	t.Cleanup(cleanup)
	return handler
}

// TestBuildAPIMux_AuthMiddlewareWired verifies that when buildAPIMux is given a
// non-nil reviewer and an allowlist, the ServiceAccount auth middleware is
// actually wired into the API handler chain: unauthenticated requests are
// rejected with 401, allowlisted subjects reach the handler, and /healthz
// stays exempt.
//
// This is a wiring test: it exercises the real buildAPIMux assembly with a
// fake reviewer, catching the failure mode where the middleware exists but is
// never installed in the chain.
func TestBuildAPIMux_AuthMiddlewareWired(t *testing.T) {
	reviewer := fakeReviewer{authenticated: true, username: allowedSubject}
	handler := newAuthWiringMux(t, reviewer, []string{allowedSubject}, nil)

	t.Run("no Authorization header is 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/memories?workspace=ws-1&userId=alice", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 with no token, got %d body=%q", rr.Code, rr.Body.String())
		}
	})

	t.Run("allowlisted subject reaches handler", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/memories?workspace=ws-1&userId=alice", nil).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer good-token")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code == http.StatusUnauthorized || rr.Code == http.StatusForbidden {
			t.Errorf("allowlisted subject should pass auth; got %d body=%q", rr.Code, rr.Body.String())
		}
		if rr.Code == http.StatusNotFound {
			t.Errorf("memories route not reached (404) — auth or wiring broke the chain")
		}
	})

	t.Run("healthz exempt with no token is 200", func(t *testing.T) {
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
// rejected with 403 (not 401).
func TestBuildAPIMux_NonAllowlistedSubjectIs403(t *testing.T) {
	otherReviewer := fakeReviewer{authenticated: true, username: "system:serviceaccount:ns:other"}
	handler := newAuthWiringMux(t, otherReviewer, []string{allowedSubject}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories?workspace=ws-1&userId=alice", nil)
	req.Header.Set("Authorization", "Bearer good-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("non-allowlisted subject should be 403, got %d body=%q", rr.Code, rr.Body.String())
	}
}

// TestBuildAPIMux_NamespaceAllowedFacadeReachesHandler verifies that a
// per-AgentRuntime facade SA (system:serviceaccount:<ws-ns>:<name>-facade)
// that is NOT in the exact-subject allowlist still passes auth when its
// namespace is in allowed-namespaces. This is the case that would otherwise
// 403 facades and silently stop memory writes.
func TestBuildAPIMux_NamespaceAllowedFacadeReachesHandler(t *testing.T) {
	facade := fakeReviewer{authenticated: true, username: "system:serviceaccount:ws-ns:foo-facade"}
	handler := newAuthWiringMux(t, facade, []string{allowedSubject}, []string{"ws-ns"})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories?workspace=ws-1&userId=alice", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer good-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code == http.StatusUnauthorized || rr.Code == http.StatusForbidden {
		t.Errorf("facade SA in allowed namespace should pass auth; got %d body=%q", rr.Code, rr.Body.String())
	}
	if rr.Code == http.StatusNotFound {
		t.Errorf("memories route not reached (404) — auth or wiring broke the chain")
	}
}

// TestBuildAPIMux_NilReviewer_NoAuth verifies that when the reviewer is nil
// (auth disabled — the default), buildAPIMux does not gate requests: the
// chain passes through to the handler with no Authorization header. This is
// the behavior every existing deployment must keep seeing until an operator
// opts in.
func TestBuildAPIMux_NilReviewer_NoAuth(t *testing.T) {
	handler := newAuthWiringMux(t, nil, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories?workspace=ws-1&userId=alice", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code == http.StatusUnauthorized || rr.Code == http.StatusForbidden {
		t.Errorf("nil reviewer must not gate requests; got %d", rr.Code)
	}
}

// TestBuildServiceAuth_DisabledReturnsNilReviewer proves that the default
// (auth-enabled=false) config returns a nil reviewer and empty allowlists —
// zero behavior change for every existing deployment.
func TestBuildServiceAuth_DisabledReturnsNilReviewer(t *testing.T) {
	f := &flags{}
	reviewer, subjects, namespaces, err := buildServiceAuth(f, logr.Discard())
	if err != nil {
		t.Fatalf("expected no error when auth disabled, got %v", err)
	}
	if reviewer != nil {
		t.Errorf("expected nil reviewer when auth disabled, got %v", reviewer)
	}
	if subjects != nil || namespaces != nil {
		t.Errorf("expected nil allowlists when auth disabled, got subjects=%v namespaces=%v", subjects, namespaces)
	}
}

// TestBuildServiceAuth_EnabledEmptyAllowlistsErrors proves the fail-closed
// startup guard: enabling auth without any allowlist would reject every
// caller, so buildServiceAuth refuses to start rather than silently locking
// out the whole API.
func TestBuildServiceAuth_EnabledEmptyAllowlistsErrors(t *testing.T) {
	f := &flags{authEnabled: true}
	_, _, _, err := buildServiceAuth(f, logr.Discard())
	if err == nil {
		t.Fatal("expected error when auth-enabled with empty allowlists")
	}
}

// TestSplitAndTrim mirrors session-api's coverage of the shared parsing
// helper backing the auth allowlist flags.
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

// Compile-time assertion that memory.Store is satisfied by fakeMemoryStore
// (defined in wiring_test.go) — guards against this file drifting if the
// store interface changes shape.
var _ memory.Store = fakeMemoryStore{}
