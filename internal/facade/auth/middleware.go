/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package auth

import (
	"errors"
	"net/http"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/pkg/policy"
)

// MiddlewareOption tunes Middleware. All optional.
type MiddlewareOption func(*middlewareConfig)

type middlewareConfig struct {
	log logr.Logger
}

// WithMiddlewareLogger binds a logr.Logger for rejection telemetry. The
// middleware never logs admits — that's expected traffic — only 401s
// so operators can debug misconfigured validators.
func WithMiddlewareLogger(log logr.Logger) MiddlewareOption {
	return func(c *middlewareConfig) { c.log = log }
}

// Middleware returns an http.Handler wrapper that runs `chain` against
// each request. On admit it attaches the AuthenticatedIdentity to the
// request context via policy.WithIdentity and calls next. On
// ErrNoCredential (or empty chain) it calls next without attaching an
// identity — the PR 1 unauthenticated-upgrade default stays intact
// until PR 3 flips it. On any other chain error it returns 401 and
// short-circuits next.
//
// Unlike the WS server's inline authenticateRequest, this wrapper works
// with any http.Handler — used by the A2A HTTP server which the
// dashboard proxy doesn't front.
func Middleware(chain Chain, next http.Handler, opts ...MiddlewareOption) http.Handler {
	cfg := &middlewareConfig{log: logr.Discard()}
	for _, opt := range opts {
		opt(cfg)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(chain) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		id, err := chain.Run(r.Context(), r)
		switch {
		case err == nil:
			// Admit: attach identity so downstream handlers (and
			// ToolPolicy CEL) can reason about the caller.
			ctx := policy.WithIdentity(r.Context(), id)
			next.ServeHTTP(w, r.WithContext(ctx))
		case errors.Is(err, ErrNoCredential):
			// PR 1 default: no credential → proceed unauthenticated.
			// PR 3 flips this at the caller layer by configuring a
			// strict chain that returns ErrInvalidCredential instead.
			next.ServeHTTP(w, r)
		default:
			// ErrInvalidCredential / ErrExpired / anything else →
			// reject. 401 is the right status per the design doc.
			cfg.log.V(1).Info("auth middleware rejected request",
				"reason", err.Error(),
				"path", r.URL.Path,
				"method", r.Method)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		}
	})
}
