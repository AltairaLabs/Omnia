/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package serviceauth

import (
	"net/http"
	"strings"

	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/altairalabs/omnia/internal/httputil"
)

// bearerPrefix is the scheme prefix of an "Authorization: Bearer <token>" header.
const bearerPrefix = "Bearer "

// subjectSet builds a lookup set from a slice of allowed subjects, dropping
// empties.
func subjectSet(allowedSubjects []string) map[string]struct{} {
	allow := make(map[string]struct{}, len(allowedSubjects))
	for _, s := range allowedSubjects {
		if s != "" {
			allow[s] = struct{}{}
		}
	}
	return allow
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
func bearerToken(r *http.Request) string {
	if after, ok := strings.CutPrefix(r.Header.Get("Authorization"), bearerPrefix); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

// writeError writes a generic JSON error body with the given status code.
func writeError(w http.ResponseWriter, status int, msg string) {
	_ = httputil.WriteJSON(w, status, map[string]string{"error": msg})
}

// RequireServiceAccount returns middleware that requires a Bearer token whose
// TokenReview subject is in allowedSubjects. Paths in exempt are passed through
// (e.g. "/healthz"). A nil reviewer disables auth (pass-through) — callers
// should log a startup warning in that mode.
//
// Responses are deliberately generic: 401 {"error":"unauthorized"} for any
// authentication failure (no detail on why, to avoid leaking token state) and
// 403 {"error":"forbidden"} when the caller authenticates but is not on the
// allowlist. On success the verified subject is placed in the request context
// via WithSubject.
func RequireServiceAccount(reviewer TokenReviewer, allowedSubjects []string, exempt ...string) func(http.Handler) http.Handler {
	allow := subjectSet(allowedSubjects)
	exemptSet := subjectSet(exempt)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if reviewer == nil {
				next.ServeHTTP(w, r)
				return
			}
			if _, ok := exemptSet[r.URL.Path]; ok {
				next.ServeHTTP(w, r)
				return
			}

			subject, ok := authenticateHTTP(w, r, reviewer)
			if !ok {
				return
			}
			if _, allowed := allow[subject]; !allowed {
				ctrllog.FromContext(r.Context()).Info("serviceauth: subject not allowed", "subject", subject)
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}

			next.ServeHTTP(w, r.WithContext(WithSubject(r.Context(), subject)))
		})
	}
}

// authenticateHTTP validates the bearer token on r. On success it returns the
// verified subject and true. On failure it writes a 401 response and returns
// ("", false). Authentication-failure detail is logged server-side only.
func authenticateHTTP(w http.ResponseWriter, r *http.Request, reviewer TokenReviewer) (string, bool) {
	token := bearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	authenticated, subject, err := reviewer.ReviewToken(r.Context(), token)
	if err != nil {
		ctrllog.FromContext(r.Context()).Error(err, "serviceauth: token review failed")
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	if !authenticated {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	return subject, true
}
