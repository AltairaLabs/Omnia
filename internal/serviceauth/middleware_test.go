/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package serviceauth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const mwSubject = "system:serviceaccount:omnia-system:omnia-session-api"

// recordingHandler records whether it was called and the subject in its context.
type recordingHandler struct {
	called  bool
	subject string
}

func (h *recordingHandler) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	h.called = true
	h.subject = SubjectFromContext(r.Context())
}

func serve(mw func(http.Handler) http.Handler, next http.Handler, authHeader, path string) *httptest.ResponseRecorder {
	if path == "" {
		path = "/api/v1/x"
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)
	return rec
}

func TestRequireServiceAccount(t *testing.T) {
	t.Run("allowlisted subject -> next, subject in context", func(t *testing.T) {
		h := &recordingHandler{}
		mw := RequireServiceAccount(fakeReviewer{authenticated: true, username: mwSubject}, []string{mwSubject})
		rec := serve(mw, h, "Bearer good", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200", rec.Code)
		}
		if !h.called {
			t.Fatal("next not called")
		}
		if h.subject != mwSubject {
			t.Fatalf("subject in context = %q, want %q", h.subject, mwSubject)
		}
	})

	t.Run("missing token -> 401", func(t *testing.T) {
		h := &recordingHandler{}
		mw := RequireServiceAccount(fakeReviewer{}, []string{mwSubject})
		rec := serve(mw, h, "", "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", rec.Code)
		}
		if h.called {
			t.Fatal("next should not be called")
		}
	})

	t.Run("reviewer error -> 401", func(t *testing.T) {
		h := &recordingHandler{}
		mw := RequireServiceAccount(fakeReviewer{err: errors.New("boom")}, []string{mwSubject})
		rec := serve(mw, h, "Bearer x", "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", rec.Code)
		}
	})

	t.Run("unauthenticated -> 401", func(t *testing.T) {
		h := &recordingHandler{}
		mw := RequireServiceAccount(fakeReviewer{authenticated: false}, []string{mwSubject})
		rec := serve(mw, h, "Bearer x", "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", rec.Code)
		}
	})

	t.Run("authenticated but wrong subject -> 403", func(t *testing.T) {
		h := &recordingHandler{}
		mw := RequireServiceAccount(fakeReviewer{authenticated: true, username: "system:serviceaccount:other:thing"}, []string{mwSubject})
		rec := serve(mw, h, "Bearer x", "")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("got %d, want 403", rec.Code)
		}
		if h.called {
			t.Fatal("next should not be called")
		}
	})

	t.Run("exempt path with no token -> passes", func(t *testing.T) {
		h := &recordingHandler{}
		mw := RequireServiceAccount(fakeReviewer{}, []string{mwSubject}, "/healthz")
		rec := serve(mw, h, "", "/healthz")
		if rec.Code != http.StatusOK || !h.called {
			t.Fatalf("got %d called=%v, want 200 + called", rec.Code, h.called)
		}
	})

	t.Run("nil reviewer -> passes through", func(t *testing.T) {
		h := &recordingHandler{}
		mw := RequireServiceAccount(nil, nil)
		rec := serve(mw, h, "", "")
		if rec.Code != http.StatusOK || !h.called {
			t.Fatalf("got %d called=%v, want 200 + called", rec.Code, h.called)
		}
	})
}

func TestSubjectContextRoundTrip(t *testing.T) {
	ctx := WithSubject(t.Context(), "sub")
	if got := SubjectFromContext(ctx); got != "sub" {
		t.Fatalf("got %q, want sub", got)
	}
	if got := SubjectFromContext(t.Context()); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestBearerToken(t *testing.T) {
	tests := map[string]string{
		"Bearer abc":   "abc",
		"Bearer  abc ": "abc",
		"bearer abc":   "",
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
