/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package tooltest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const dashboardSubject = "system:serviceaccount:omnia-system:omnia-dashboard"

// stubReviewer is a test TokenReviewer.
type stubReviewer struct {
	authenticated bool
	username      string
	err           error
}

func (s stubReviewer) ReviewToken(_ context.Context, _ string) (bool, string, error) {
	return s.authenticated, s.username, s.err
}

func newAuthTestServer(reviewer TokenReviewer) *Server {
	return NewServer(":0", nil, zap.New(zap.UseDevMode(true)), reviewer, []string{dashboardSubject})
}

func doGuarded(t *testing.T, srv *Server, authHeader string) int {
	t.Helper()
	handler := srv.requireAuth(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec.Code
}

func TestRequireAuth(t *testing.T) {
	t.Run("missing token -> 401", func(t *testing.T) {
		code := doGuarded(t, newAuthTestServer(stubReviewer{}), "")
		if code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", code)
		}
	})

	t.Run("unauthenticated token -> 401", func(t *testing.T) {
		code := doGuarded(t, newAuthTestServer(stubReviewer{authenticated: false}), "Bearer bad")
		if code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", code)
		}
	})

	t.Run("token review error -> 401", func(t *testing.T) {
		code := doGuarded(t, newAuthTestServer(stubReviewer{err: errors.New("boom")}), "Bearer x")
		if code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", code)
		}
	})

	t.Run("authenticated but wrong subject -> 403", func(t *testing.T) {
		srv := newAuthTestServer(stubReviewer{authenticated: true, username: "system:serviceaccount:other:thing"})
		code := doGuarded(t, srv, "Bearer x")
		if code != http.StatusForbidden {
			t.Fatalf("got %d, want 403", code)
		}
	})

	t.Run("authenticated allowed subject -> 200", func(t *testing.T) {
		srv := newAuthTestServer(stubReviewer{authenticated: true, username: dashboardSubject})
		code := doGuarded(t, srv, "Bearer good")
		if code != http.StatusOK {
			t.Fatalf("got %d, want 200", code)
		}
	})

	t.Run("nil reviewer -> auth disabled, passes through", func(t *testing.T) {
		srv := NewServer(":0", nil, zap.New(zap.UseDevMode(true)), nil, nil)
		code := doGuarded(t, srv, "")
		if code != http.StatusOK {
			t.Fatalf("got %d, want 200 (auth disabled)", code)
		}
	})
}

func TestNewK8sTokenReviewer(t *testing.T) {
	r, err := NewK8sTokenReviewer(&rest.Config{Host: "https://example.invalid"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil reviewer")
	}
}

func TestBearerToken(t *testing.T) {
	tests := map[string]string{
		"Bearer abc":   "abc",
		"Bearer  abc ": "abc",
		"bearer abc":   "", // case-sensitive scheme
		"abc":          "",
		"":             "",
	}
	for header, want := range tests {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		if got := bearerToken(req); got != want {
			t.Fatalf("bearerToken(%q) = %q, want %q", header, got, want)
		}
	}
}
