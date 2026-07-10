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
	"context"
	"crypto/subtle"
	"errors"
	"net/http"

	"github.com/altairalabs/omnia/pkg/policy"
)

// EndUserHeader is the inbound HTTP header callers MAY set to identify the
// end-user on whose behalf they're acting. Honoured by the sharedToken /
// clientKeys validators only when their `trustEndUserHeader` flag is true on
// the AgentRuntime CRD; the default is to ignore it (Identity.EndUser
// equals Identity.Subject in that case). Documented in
// docs/local-backlog/2026-04-21-agent-facade-auth-design.md.
const EndUserHeader = "X-End-User-Id"

// DefaultSharedTokenSubject is the Subject claim emitted on Identity when
// no override is configured. Single-bearer tokens have no built-in caller
// identity — the placeholder here flags that fact to ToolPolicy and
// audit consumers that compare on `identity.subject`.
const DefaultSharedTokenSubject = "shared-token-holder"

// SharedTokenValidator implements Validator for the simplest data-plane
// auth pattern: a single bearer token shared across every caller of an
// agent. The token is loaded once at facade startup from the Secret
// referenced by spec.externalAuth.sharedToken.secretRef and constant-time
// compared against the bearer payload on each request.
type SharedTokenValidator struct {
	tokenHash          []byte
	subject            string
	trustEndUserHeader bool
}

// SharedTokenOption tunes a SharedTokenValidator. All optional.
type SharedTokenOption func(*SharedTokenValidator)

// WithSharedTokenSubject overrides the Identity.Subject value the
// validator emits on admit. Defaults to DefaultSharedTokenSubject.
func WithSharedTokenSubject(sub string) SharedTokenOption {
	return func(v *SharedTokenValidator) { v.subject = sub }
}

// WithSharedTokenTrustEndUserHeader makes the validator honour
// X-End-User-Id from the inbound request when populating Identity.EndUser.
// Off by default — when off, EndUser == Subject. Turning this on is a
// trust statement: the calling app can spoof arbitrary end-users with a
// valid token, so ToolPolicy rules gating on identity.endUser must be
// paired with an app-level trust assessment (per the design doc).
func WithSharedTokenTrustEndUserHeader(trust bool) SharedTokenOption {
	return func(v *SharedTokenValidator) { v.trustEndUserHeader = trust }
}

// NewSharedTokenValidator constructs a validator that admits requests
// presenting the supplied bearer value. Returns an error if the token
// is empty — empty-string credentials are silently always-pass with
// constant-time compare semantics, which would be a catastrophic
// misconfig. We refuse to construct rather than ship that.
func NewSharedTokenValidator(token string, opts ...SharedTokenOption) (*SharedTokenValidator, error) {
	if token == "" {
		return nil, errors.New("auth: sharedToken value is empty — refusing to construct an always-admit validator")
	}
	v := &SharedTokenValidator{
		// Materialised as a byte slice so we can pass directly to
		// subtle.ConstantTimeCompare without per-request allocation.
		tokenHash: []byte(token),
		subject:   DefaultSharedTokenSubject,
	}
	for _, opt := range opts {
		opt(v)
	}
	return v, nil
}

// Validate implements Validator. It returns ErrNoCredential when no Bearer
// header is present, or when a Bearer header is present but the token does not
// match (the bearer is not our shared token — fall through so a later
// validator can handle it).
func (v *SharedTokenValidator) Validate(_ context.Context, r *http.Request) (*policy.AuthenticatedIdentity, error) {
	tokenString, err := extractBearer(r)
	if err != nil {
		return nil, err
	}
	// Length-mismatch short-circuit can leak length in microbenchmarks.
	// subtle.ConstantTimeCompare already returns 0 when lengths differ
	// without revealing which one is longer — let it handle the check.
	if subtle.ConstantTimeCompare([]byte(tokenString), v.tokenHash) != 1 {
		// Not our shared token. As an opaque-bearer validator we can't tell a
		// wrong token from a credential of another style, so fall through and
		// let a later validator (client keys, or the mgmt-plane on its own
		// listener) handle it rather than short-circuiting the chain. See #1620.
		return nil, ErrNoCredential
	}

	endUser := v.subject
	if v.trustEndUserHeader {
		if h := r.Header.Get(EndUserHeader); h != "" {
			endUser = h
		}
	}

	// Shared-token callers carry no per-caller role — they are gated on
	// identity.origin (OriginSharedToken), not on a structured role.
	return &policy.AuthenticatedIdentity{
		Origin:  policy.OriginSharedToken,
		Subject: v.subject,
		EndUser: endUser,
	}, nil
}
