/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

// Package facade is the reusable core of the reference custom facade
// (examples/custom-facade). It authenticates a trivial bring-your-own
// protocol (a static bearer token), then emits the flat x-omnia-* identity
// and claims metadata contract on the outbound runtime gRPC call using the
// public github.com/altairalabs/omnia/pkg/policy helpers.
//
// It deliberately builds DIRECTLY on the identity/claims wire contract and
// speaks the runtime gRPC surface itself. It does NOT import the built-in
// facade's internal auth or session packages, so a third party can copy this
// directory as the starting point for their own facade image (#1771).
package facade

import (
	"context"
	"errors"
	"strings"

	"github.com/altairalabs/omnia/pkg/policy"
)

// ErrUnknownToken is returned by Authenticate when the presented bearer token
// is not recognised. Callers must fail closed (HTTP 401) on this error.
var ErrUnknownToken = errors.New("custom-facade: unknown bearer token")

// Principal is the facade's own normalised identity, produced by
// authenticating the bring-your-own protocol. It is intentionally a small,
// self-contained type — a real custom facade would populate it from whatever
// credential its clients present (session cookie, mTLS SAN, API key, upstream
// OIDC token, ...). The facade maps it onto the platform's flat propagation
// fields in PropagationFields below; that mapping is the whole identity
// contract a custom facade must honour.
type Principal struct {
	// UserID is the stable caller identifier. Surfaces to ToolPolicy CEL as
	// identity.subject / identity.endUser and to downstream services as the
	// x-omnia-user-id header.
	UserID string
	// Roles is the caller's role set. Emitted comma-joined as x-omnia-user-roles
	// and surfaced to ToolPolicy CEL as identity.role.
	Roles []string
	// Workspace is the workspace the caller targets. Emitted as x-omnia-workspace
	// and surfaced to ToolPolicy CEL as identity.workspace.
	Workspace string
	// Origin names the validator style that admitted the request. A custom
	// facade that runs a shared-token protocol reports policy.OriginSharedToken;
	// surfaces to ToolPolicy CEL as identity.origin.
	Origin string
	// Claims is the full claim map. Each entry is emitted as an
	// x-omnia-claim-<name> header and surfaced to ToolPolicy CEL as
	// identity.claims.<name>.
	Claims map[string]string
}

// PropagationFields maps the facade's Principal onto the platform's flat
// x-omnia-* propagation contract. This is the single place a custom facade
// translates its own identity model into what the runtime + policy-broker
// consume. agentName is the agent this facade fronts (from OMNIA_AGENT_NAME).
func (p *Principal) PropagationFields(agentName string) *policy.PropagationFields {
	return &policy.PropagationFields{
		AgentName: agentName,
		UserID:    p.UserID,
		UserRoles: strings.Join(p.Roles, ","),
		Origin:    p.Origin,
		Workspace: p.Workspace,
		Claims:    p.Claims,
	}
}

// OutboundContext returns a context carrying the caller's identity as policy
// propagation fields, ready to be converted to gRPC metadata via
// policy.ToGRPCMetadata. It is the exact context shape the runtime's policy
// interceptor rehydrates on the other side of the hop.
func (p *Principal) OutboundContext(ctx context.Context, agentName string) context.Context {
	return policy.WithPropagationFields(ctx, p.PropagationFields(agentName))
}

// OutboundMetadata returns the flat x-omnia-* metadata map the facade attaches
// to the outbound runtime gRPC call. Keys are lowercase, per the gRPC metadata
// contract, and include the per-claim x-omnia-claim-<name> headers.
func (p *Principal) OutboundMetadata(agentName string) map[string]string {
	return policy.ToGRPCMetadata(p.OutboundContext(context.Background(), agentName))
}

// Authenticator resolves a bring-your-own credential (here, a static bearer
// token) into a Principal. The reference implementation is a fixed in-memory
// token table; a real facade would swap this for its own credential check.
type Authenticator struct {
	tokens map[string]*Principal
}

// NewAuthenticator builds an Authenticator over a token->Principal table. The
// returned authenticator is read-only and safe for concurrent use.
func NewAuthenticator(tokens map[string]*Principal) *Authenticator {
	return &Authenticator{tokens: tokens}
}

// Authenticate resolves a bearer token to its Principal, or ErrUnknownToken.
// A leading "Bearer " / "bearer " prefix is tolerated so callers can pass the
// raw Authorization header value through.
func (a *Authenticator) Authenticate(token string) (*Principal, error) {
	token = strings.TrimSpace(token)
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")
	if p, ok := a.tokens[token]; ok {
		return p, nil
	}
	return nil, ErrUnknownToken
}
