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

// Package facade is an importable SDK for building a custom Omnia agent
// facade in Go without reimplementing the platform's cross-cutting
// plumbing.
//
// A facade is the network surface in front of an agent runtime. Whatever
// protocol it speaks (WebSocket, A2A, REST, MCP, or something bespoke), a
// conformant facade must:
//
//   - Authenticate inbound requests with the platform's validator chain
//     (sharedToken / apiKeys / oidc / edgeTrust) on its external listener,
//     and — when it exposes a management-plane twin — accept only
//     dashboard-minted JWTs on that internal listener.
//   - Translate the admitted identity into the platform's propagation
//     fields so downstream gRPC / HTTP calls carry X-Omnia-* headers that
//     ToolPolicy CEL and the runtime rely on.
//   - Record conversation to session-api over HTTP.
//
// This package is pure convenience sugar on top of those contracts. It
// composes the reusable building blocks and adds no policy of its own —
// there is deliberately no license or enforcement logic here (that lives
// at the operator's admission webhook, never in the SDK). The building
// blocks remain independently importable:
//
//   - github.com/altairalabs/omnia/pkg/facade/auth — the validator chain,
//     concrete validators, the management-plane JWKS validator, and the
//     net/http auth middleware.
//   - github.com/altairalabs/omnia/pkg/session/httpclient — the session-api
//     recording client (a session.Store backed by HTTP).
//   - github.com/altairalabs/omnia/pkg/policy — the AuthenticatedIdentity
//     type, propagation-field context helpers, and ToOutboundHeaders /
//     ToGRPCMetadata emission.
//
// Minimal facade:
//
//	chain := facade.Chain{sharedToken, apiKeys} // from pkg/facade/auth
//	rec := facade.NewSessionRecorder("http://session-api:8080", log)
//	scope := facade.IdentityScope{AgentName: "my-agent", Namespace: "ns", Workspace: "team"}
//
//	handler := facade.Authenticate(chain, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	    id := facade.IdentityFromContext(r.Context())      // admitted identity
//	    ctx := facade.PropagateIdentity(r.Context(), id, scope)
//	    md := facade.OutboundMetadata(ctx)                 // X-Omnia-* → runtime gRPC metadata
//	    _ = md
//	    // ... dial the runtime, record via rec ...
//	}), facade.WithLogger(log))
package facade

import (
	"context"
	"net/http"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/pkg/facade/auth"
	"github.com/altairalabs/omnia/pkg/policy"
	"github.com/altairalabs/omnia/pkg/session/httpclient"
)

// Re-exported types so a custom facade needs a single import for the common
// surface. The underlying packages remain importable for advanced use.
type (
	// Validator validates one credential style. See pkg/facade/auth.
	Validator = auth.Validator
	// Chain is an ordered set of Validators; the first to admit wins.
	Chain = auth.Chain
	// Identity is the caller identity attached to an admitted request.
	Identity = policy.AuthenticatedIdentity
	// SessionRecorder records conversation to session-api over HTTP.
	SessionRecorder = httpclient.Store
	// SessionRecorderOption tunes a SessionRecorder.
	SessionRecorderOption = httpclient.StoreOption
	// MiddlewareOption tunes the auth middleware (logger, reject handler,
	// unauthenticated fallback).
	MiddlewareOption = auth.MiddlewareOption
	// MgmtPlaneOption tunes the management-plane validator.
	MgmtPlaneOption = auth.MgmtPlaneOption
)

// Re-exported middleware options so callers do not need a second import
// just to pass a logger.
var (
	// WithLogger binds a logr.Logger for auth-rejection telemetry.
	WithLogger = auth.WithMiddlewareLogger
	// WithOnReject overrides the default 401 response body.
	WithOnReject = auth.WithMiddlewareOnReject
	// WithAllowUnauthenticated controls the empty-chain fallback. Defaults
	// to true; set false to fail closed on an empty chain.
	WithAllowUnauthenticated = auth.WithMiddlewareAllowUnauthenticated
)

// NewMgmtPlaneValidator builds the management-plane twin validator backed
// by the dashboard's JWKS endpoint. Wire the returned Validator as the sole
// entry of the twin listener's Chain so it fails closed — the twin admits
// only dashboard-minted JWTs.
func NewMgmtPlaneValidator(jwksURL string, opts ...MgmtPlaneOption) (Validator, error) {
	return auth.NewMgmtPlaneValidator(jwksURL, opts...)
}

// NewSessionRecorder builds a session-api recording client. baseURL is the
// session-api service URL. It is safe to share one recorder across all
// connections.
func NewSessionRecorder(baseURL string, log logr.Logger, opts ...SessionRecorderOption) *SessionRecorder {
	return httpclient.NewStore(baseURL, log, opts...)
}

// Authenticate wraps next with the platform auth middleware. On admit it
// attaches the AuthenticatedIdentity to the request context (retrievable
// with IdentityFromContext) and calls next; otherwise it responds 401. Use
// it for both the external listener (pass the data-plane Chain) and the
// management-plane twin (pass a Chain holding only the mgmt-plane Validator).
func Authenticate(chain Chain, next http.Handler, opts ...MiddlewareOption) http.Handler {
	return auth.Middleware(chain, next, opts...)
}

// IdentityFromContext returns the AuthenticatedIdentity that Authenticate
// admitted, or nil when the request was not authenticated.
func IdentityFromContext(ctx context.Context) *Identity {
	return policy.IdentityFromContext(ctx)
}

// IdentityScope carries the per-connection propagation inputs a facade owns
// independently of the admitted identity — the agent's coordinates plus the
// resolved end-user fields. It mirrors what the built-in facade forwards to
// the runtime.
type IdentityScope struct {
	AgentName     string // the AgentRuntime name
	Namespace     string // the agent's namespace
	Workspace     string // the agent's deployed workspace (identity workspace wins when set)
	RequestID     string // per-request correlation id
	UserID        string // resolved (pseudonymized) end-user id
	UserRoles     string // end-user roles
	UserEmail     string // end-user email
	Authorization string // inbound bearer, kept in-process (never emitted to tools)
}

// PropagateIdentity stores the admitted identity's propagation fields on ctx
// so OutboundMetadata can emit them to the runtime and downstream tools. It
// reproduces the built-in facade's mapping: origin comes from the admitting
// validator, workspace prefers the token's own scope and falls back to the
// agent's deployed workspace, and the validator's claim map rides along for
// ToolPolicy requiredClaims. A nil id yields the scope fields alone.
func PropagateIdentity(ctx context.Context, id *Identity, scope IdentityScope) context.Context {
	origin := ""
	workspace := scope.Workspace
	var claims map[string]string
	if id != nil {
		origin = id.Origin
		if id.Workspace != "" {
			workspace = id.Workspace
		}
		if len(id.Claims) > 0 {
			claims = id.Claims
		}
	}
	return policy.WithPropagationFields(ctx, &policy.PropagationFields{
		AgentName:     scope.AgentName,
		Namespace:     scope.Namespace,
		RequestID:     scope.RequestID,
		UserID:        scope.UserID,
		UserRoles:     scope.UserRoles,
		UserEmail:     scope.UserEmail,
		Authorization: scope.Authorization,
		Origin:        origin,
		Workspace:     workspace,
		Claims:        claims,
		Identity:      id,
	})
}

// OutboundMetadata converts the propagation fields on ctx into the flat
// X-Omnia-* header/metadata map the runtime and HTTP tool adapters consume.
// Emit it as gRPC metadata on the call to the runtime. It is an alias for
// policy.ToGRPCMetadata (identical to policy.ToOutboundHeaders — see #1772
// scope note: the emission helper is referenced, not moved).
func OutboundMetadata(ctx context.Context) map[string]string {
	return policy.ToGRPCMetadata(ctx)
}
