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
	"net/http"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/pkg/policy"
)

// MiddlewareOption tunes Middleware. All optional.
type MiddlewareOption func(*middlewareConfig)

type middlewareConfig struct {
	log                  logr.Logger
	allowUnauthenticated bool
}

// WithMiddlewareLogger binds a logr.Logger for rejection telemetry. The
// middleware never logs admits — that's expected traffic — only 401s
// so operators can debug misconfigured validators.
func WithMiddlewareLogger(log logr.Logger) MiddlewareOption {
	return func(c *middlewareConfig) { c.log = log }
}

// WithMiddlewareAllowUnauthenticated controls the empty-chain fallback.
// Defaults to true so dev/test handlers without a chain configured keep
// working. Set false to reject every unauthenticated request including
// the empty-chain case.
//
// When the chain is non-empty, ErrNoCredential from all validators
// always 401s regardless of this flag — that's the PR 3 default-flip
// semantic. Production deployments run a non-empty chain (mgmt-plane
// at minimum) so this flag is a no-op for them.
func WithMiddlewareAllowUnauthenticated(allow bool) MiddlewareOption {
	return func(c *middlewareConfig) { c.allowUnauthenticated = allow }
}

// Middleware returns an http.Handler wrapper that runs `chain` against
// each request. On admit it attaches the AuthenticatedIdentity to the
// request context via policy.WithIdentity and calls next.
//
// PR 3 flipped the ErrNoCredential path: when a non-empty chain is
// configured and no validator admits, the middleware returns 401
// instead of falling through to next. Empty chain (no validators
// configured) still falls through when allowUnauthenticated is true
// (the default) so dev/test handlers work — production always runs a
// non-empty chain with mgmt-plane at minimum.
//
// Unlike the WS server's inline authenticateRequest, this wrapper works
// with any http.Handler — used by the A2A HTTP server which the
// dashboard proxy doesn't front.
func Middleware(chain Chain, next http.Handler, opts ...MiddlewareOption) http.Handler {
	cfg := &middlewareConfig{
		log:                  logr.Discard(),
		allowUnauthenticated: true,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(chain) == 0 {
			if cfg.allowUnauthenticated {
				next.ServeHTTP(w, r)
				return
			}
			reject401(w, r, cfg.log, "empty chain with allowUnauthenticated=false")
			return
		}
		id, err := chain.Run(r.Context(), r)
		if err != nil {
			// ErrNoCredential / ErrInvalidCredential / ErrExpired /
			// anything else → reject. PR 3 flipped the ErrNoCredential
			// branch from "pass through" to 401 to close pen-test C-3.
			reject401(w, r, cfg.log, err.Error())
			return
		}
		// Admit: attach identity so downstream handlers (and
		// ToolPolicy CEL) can reason about the caller.
		ctx := policy.WithIdentity(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func reject401(w http.ResponseWriter, r *http.Request, log logr.Logger, reason string) {
	log.V(1).Info("auth middleware rejected request",
		"reason", reason,
		"path", r.URL.Path,
		"method", r.Method)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}
