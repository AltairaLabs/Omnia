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
	const (
		facade  = "system:serviceaccount:ws-ns:foo-facade"
		wrongNS = "system:serviceaccount:other-ns:foo-facade"
	)
	tests := []struct {
		name        string
		reviewer    TokenReviewer
		subjects    []string
		namespaces  []string
		exempt      []string
		authHeader  string
		path        string
		wantCode    int
		wantCalled  bool
		wantSubject string // asserted only when non-empty
	}{
		{
			name:        "allowlisted subject -> next, subject in context",
			reviewer:    fakeReviewer{authenticated: true, username: mwSubject},
			subjects:    []string{mwSubject},
			authHeader:  "Bearer good",
			wantCode:    http.StatusOK,
			wantCalled:  true,
			wantSubject: mwSubject,
		},
		{
			name:       "missing token -> 401",
			reviewer:   fakeReviewer{},
			subjects:   []string{mwSubject},
			wantCode:   http.StatusUnauthorized,
			wantCalled: false,
		},
		{
			name:       "reviewer error -> 401",
			reviewer:   fakeReviewer{err: errors.New("boom")},
			subjects:   []string{mwSubject},
			authHeader: "Bearer x",
			wantCode:   http.StatusUnauthorized,
			wantCalled: false,
		},
		{
			name:       "unauthenticated -> 401",
			reviewer:   fakeReviewer{authenticated: false},
			subjects:   []string{mwSubject},
			authHeader: "Bearer x",
			wantCode:   http.StatusUnauthorized,
			wantCalled: false,
		},
		{
			name:       "authenticated but wrong subject -> 403",
			reviewer:   fakeReviewer{authenticated: true, username: "system:serviceaccount:other:thing"},
			subjects:   []string{mwSubject},
			authHeader: "Bearer x",
			wantCode:   http.StatusForbidden,
			wantCalled: false,
		},
		{
			// A facade-shaped SA in the trusted workspace namespace is accepted
			// even though it is not in allowedSubjects.
			name:        "namespace-allowed subject not in subjects -> next",
			reviewer:    fakeReviewer{authenticated: true, username: facade},
			namespaces:  []string{"ws-ns"},
			authHeader:  "Bearer x",
			wantCode:    http.StatusOK,
			wantCalled:  true,
			wantSubject: facade,
		},
		{
			name:       "subject in non-allowed namespace and not in subjects -> 403",
			reviewer:   fakeReviewer{authenticated: true, username: wrongNS},
			namespaces: []string{"ws-ns"},
			authHeader: "Bearer x",
			wantCode:   http.StatusForbidden,
			wantCalled: false,
		},
		{
			// Not a valid 4-part SA subject; ParseServiceAccount must reject it so
			// the namespace allow never matches on a substring.
			name:       "malformed subject with allowed-looking ns substring -> 403",
			reviewer:   fakeReviewer{authenticated: true, username: "system:serviceaccount:ws-ns"},
			namespaces: []string{"ws-ns"},
			authHeader: "Bearer x",
			wantCode:   http.StatusForbidden,
			wantCalled: false,
		},
		{
			name:       "exempt path with no token -> passes",
			reviewer:   fakeReviewer{},
			subjects:   []string{mwSubject},
			exempt:     []string{"/healthz"},
			path:       "/healthz",
			wantCode:   http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "nil reviewer -> passes through",
			reviewer:   nil,
			wantCode:   http.StatusOK,
			wantCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &recordingHandler{}
			mw := RequireServiceAccount(tt.reviewer, tt.subjects, tt.namespaces, tt.exempt...)
			rec := serve(mw, h, tt.authHeader, tt.path)
			if rec.Code != tt.wantCode {
				t.Fatalf("got %d, want %d", rec.Code, tt.wantCode)
			}
			if h.called != tt.wantCalled {
				t.Fatalf("called = %v, want %v", h.called, tt.wantCalled)
			}
			if tt.wantSubject != "" && h.subject != tt.wantSubject {
				t.Fatalf("subject in context = %q, want %q", h.subject, tt.wantSubject)
			}
		})
	}
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
