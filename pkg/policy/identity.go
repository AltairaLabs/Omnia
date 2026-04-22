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

package policy

import (
	"time"

	"github.com/altairalabs/omnia/pkg/logging"
)

// Origin strings identify which validator admitted a request. They flow
// through PropagationFields and surface to ToolPolicy CEL as identity.origin.
// Lives here (not in internal/facade/auth) so pkg/policy can refer to the
// identity contract without importing downstream facade code.
const (
	OriginManagementPlane = "management-plane"
	OriginSharedToken     = "shared-token"
	OriginAPIKey          = "api-key"
	OriginOIDC            = "oidc"
	OriginEdgeTrust       = "edge-trust"
)

// Role strings identify the caller's role. Used by ToolPolicy rules.
const (
	RoleAdmin  = "admin"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

// AuthenticatedIdentity is the normalised result produced by a facade
// Validator. It is the single contract runtime / ToolPolicy see regardless
// of which credential style the caller presented (shared token, API key,
// OIDC JWT, edge-injected headers, management-plane JWT).
//
// # PII WARNING
//
// Subject and EndUser hold RAW user identifiers (email addresses, OIDC
// subs, device IDs, etc.). They are PII. Never log them directly — use
// HashedSubject() / HashedEndUser() which emit a stable sha256-prefixed
// tag suitable for audit trails and log correlation. The flat fields on
// PropagationFields (UserID) are already pseudonymised via
// identity.PseudonymizeID; consumers that build their own logs off the
// Identity struct must do the hashing themselves.
type AuthenticatedIdentity struct {
	// Origin names the validator that admitted the request. One of the
	// Origin* constants above.
	Origin string

	// Subject is the stable identifier of the token-holder (the app, key,
	// or user that presented the credential). RAW — PII. Use
	// HashedSubject() before logging.
	Subject string

	// EndUser identifies the human or device on whose behalf the
	// token-holder is acting. Equals Subject for end-user tokens. For
	// service tokens carrying an actor claim, EndUser is the actor.
	// RAW — PII. Use HashedEndUser() before logging.
	EndUser string

	// Workspace is the workspace the request targets (may be empty for
	// validators that do not carry workspace scope).
	Workspace string

	// Agent is the agent the request targets (may be empty for validators
	// that do not carry agent scope).
	Agent string

	// Role is the caller's role. One of RoleAdmin / RoleEditor / RoleViewer.
	Role string

	// Claims holds extra claim values the validator surfaced (OIDC claim
	// map, edge-injected headers mapped into claims, etc.). Consumed by
	// ToolPolicy CEL as identity.claims.<name>.
	Claims map[string]string

	// IssuedAt and ExpiresAt carry the token's validity window when the
	// underlying credential exposes them. Zero values when not applicable.
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// HashedSubject returns a log-safe pseudonym for Subject via
// logging.HashID. Empty Subject returns empty string so callers can
// build structured-log fields without a nil-check. Use this in every
// log line that mentions who the admitted caller is.
func (a *AuthenticatedIdentity) HashedSubject() string {
	if a == nil || a.Subject == "" {
		return ""
	}
	return logging.HashID(a.Subject)
}

// HashedEndUser returns a log-safe pseudonym for EndUser via
// logging.HashID. Same rules as HashedSubject.
func (a *AuthenticatedIdentity) HashedEndUser() string {
	if a == nil || a.EndUser == "" {
		return ""
	}
	return logging.HashID(a.EndUser)
}
