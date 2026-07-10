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
	"net/http"

	"github.com/altairalabs/omnia/pkg/policy"
)

// EdgeTrust default header mapping. Matches the chart's existing
// authentication.enabled=true setup
// (charts/omnia/templates/gateway/authentication.yaml uses Istio's
// outputClaimToHeaders to emit x-user-id / x-user-roles / x-user-email).
// Operators running a different edge override these.
const (
	DefaultEdgeSubjectHeader = "x-user-id"
	DefaultEdgeRoleHeader    = "x-user-roles"
	DefaultEdgeEndUserHeader = "x-user-id"
	DefaultEdgeEmailHeader   = "x-user-email"
)

// EdgeTrustValidator admits requests whose Identity has been validated
// by an upstream JWT/auth filter (Istio RequestAuthentication, an API
// gateway, etc.). It does NOT re-verify the underlying token — the
// trust boundary is the operator's responsibility (Istio's
// AuthorizationPolicy strips inbound claim-headers from untrusted
// sources on the chart's setup).
//
// Admit semantics: a request is admitted iff the Subject header is
// non-empty. Without a subject we have no stable caller identity to
// emit, so falling through to the next validator is safer than
// guessing. Other fields (role, endUser, email, extra claims) are
// optional and contribute to the Identity when present.
type EdgeTrustValidator struct {
	subjectHeader  string
	roleHeader     string
	endUserHeader  string
	emailHeader    string
	defaultRole    string
	extraClaims    map[string]string // inboundHeader → claimName
	defaultSubject string
}

// EdgeTrustOption tunes an EdgeTrustValidator.
type EdgeTrustOption func(*EdgeTrustValidator)

// WithEdgeTrustSubjectHeader overrides the inbound header read for
// Identity.Subject. Defaults to DefaultEdgeSubjectHeader.
func WithEdgeTrustSubjectHeader(name string) EdgeTrustOption {
	return func(v *EdgeTrustValidator) {
		if name != "" {
			v.subjectHeader = name
		}
	}
}

// WithEdgeTrustRoleHeader overrides the inbound header read for
// Identity.Role. Defaults to DefaultEdgeRoleHeader.
func WithEdgeTrustRoleHeader(name string) EdgeTrustOption {
	return func(v *EdgeTrustValidator) {
		if name != "" {
			v.roleHeader = name
		}
	}
}

// WithEdgeTrustEndUserHeader overrides the inbound header read for
// Identity.EndUser. Defaults to DefaultEdgeEndUserHeader.
func WithEdgeTrustEndUserHeader(name string) EdgeTrustOption {
	return func(v *EdgeTrustValidator) {
		if name != "" {
			v.endUserHeader = name
		}
	}
}

// WithEdgeTrustEmailHeader overrides the inbound header read for
// Identity.Claims["email"]. Defaults to DefaultEdgeEmailHeader.
func WithEdgeTrustEmailHeader(name string) EdgeTrustOption {
	return func(v *EdgeTrustValidator) {
		if name != "" {
			v.emailHeader = name
		}
	}
}

// WithEdgeTrustDefaultRole sets the role applied when the role header
// is absent. Defaults to policy.RoleViewer.
func WithEdgeTrustDefaultRole(role string) EdgeTrustOption {
	return func(v *EdgeTrustValidator) {
		if role != "" {
			v.defaultRole = role
		}
	}
}

// WithEdgeTrustExtraClaims wires additional inbound headers into the
// Identity.Claims map. Keyed by header name (case-insensitive — net/http
// canonicalises), value is the claim name to emit.
//
// Example: {"x-user-groups": "groups"} → identity.claims.groups in
// ToolPolicy CEL. The header name is sent through http.Header.Get which
// is case-insensitive; the claim name lands verbatim.
func WithEdgeTrustExtraClaims(m map[string]string) EdgeTrustOption {
	return func(v *EdgeTrustValidator) {
		if v.extraClaims == nil {
			v.extraClaims = map[string]string{}
		}
		for k, name := range m {
			if k == "" || name == "" {
				continue
			}
			v.extraClaims[k] = name
		}
	}
}

// NewEdgeTrustValidator constructs an edgeTrust validator with the
// default chart-shipped header mapping. Override any field via the
// option helpers above.
func NewEdgeTrustValidator(opts ...EdgeTrustOption) *EdgeTrustValidator {
	v := &EdgeTrustValidator{
		subjectHeader:  DefaultEdgeSubjectHeader,
		roleHeader:     DefaultEdgeRoleHeader,
		endUserHeader:  DefaultEdgeEndUserHeader,
		emailHeader:    DefaultEdgeEmailHeader,
		defaultRole:    policy.RoleViewer,
		extraClaims:    map[string]string{},
		defaultSubject: "",
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// Validate implements Validator. ErrNoCredential when the subject
// header is absent (chain falls through). The validator never returns
// ErrInvalidCredential or ErrExpired — the upstream edge has already
// done the validation, and a missing-but-trusted-edge state is
// indistinguishable from "no credential".
func (v *EdgeTrustValidator) Validate(_ context.Context, r *http.Request) (*policy.AuthenticatedIdentity, error) {
	subject := r.Header.Get(v.subjectHeader)
	if subject == "" {
		return nil, ErrNoCredential
	}

	role := r.Header.Get(v.roleHeader)
	if role == "" {
		role = v.defaultRole
	}

	endUser := r.Header.Get(v.endUserHeader)
	if endUser == "" {
		endUser = subject
	}

	claims := map[string]string{}
	if email := r.Header.Get(v.emailHeader); email != "" {
		claims["email"] = email
	}
	for header, claimName := range v.extraClaims {
		if val := r.Header.Get(header); val != "" {
			claims[claimName] = val
		}
	}

	id := &policy.AuthenticatedIdentity{
		Origin:  policy.OriginEdgeTrust,
		Subject: subject,
		EndUser: endUser,
		Role:    role,
	}
	if len(claims) > 0 {
		id.Claims = claims
	}
	return id, nil
}
