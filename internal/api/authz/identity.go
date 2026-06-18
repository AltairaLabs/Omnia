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

// Package authz verifies dashboard-minted identity JWTs and recomputes the
// caller's workspace role from the target Workspace CR. The operator content
// API trusts only the cryptographically-verified identity + groups in the
// token; it never trusts a role claim, recomputing the role server-side via
// pkg/workspaceauth instead.
package authz

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/altairalabs/omnia/internal/facade/auth"
)

// Default issuer / audience for content-API identity tokens. The dashboard
// reuses its mgmt-plane signing key but mints content-API tokens with a
// distinct audience so they cannot be replayed against the facade (which
// requires aud=omnia-facade) and vice versa.
const (
	IssuerDashboard    = "omnia-dashboard"
	AudienceContentAPI = "omnia-operator"
)

// jwtLeeway tolerates small clock drift between the operator and the dashboard
// on exp/nbf/iat checks, matching the facade mgmt-plane validator.
const jwtLeeway = 30 * time.Second

// ErrExpired marks a token rejected solely because it has expired. Callers map
// it to 401 like any other auth failure but it is distinguished so tests (and
// future telemetry) can tell expiry apart from a bad signature or claim.
var ErrExpired = errors.New("authz: token expired")

// ErrInvalidToken marks a token rejected for any non-expiry reason (bad
// signature, wrong issuer/audience, missing/unknown kid, malformed token).
var ErrInvalidToken = errors.New("authz: invalid token")

// IdentityClaims is the dashboard-minted content-API JWT shape. Identity and
// Groups are the authenticated principal; Workspace scopes the token to a
// single workspace; Anonymous marks an unauthenticated principal admitted via
// the workspace's anonymous-access config.
type IdentityClaims struct {
	jwt.RegisteredClaims
	Identity  string   `json:"identity,omitempty"`
	Groups    []string `json:"groups,omitempty"`
	Workspace string   `json:"workspace,omitempty"`
	Anonymous bool     `json:"anonymous,omitempty"`
}

// VerifiedIdentity is the verified principal extracted from an identity token.
type VerifiedIdentity struct {
	// Subject is the JWT `sub` claim (audit pseudonym / identity label).
	Subject string
	// Identity is the email-or-username used for direct-grant matching; empty
	// for anonymous principals.
	Identity string
	// Groups are the principal's IdP groups used for role-binding matching.
	Groups []string
	// Workspace is the single workspace this token is scoped to.
	Workspace string
	// Anonymous reports whether the principal was admitted anonymously.
	Anonymous bool
}

// IdentityVerifier verifies content-API identity tokens minted by the
// dashboard's signing key. Keys are resolved by JWT kid through a KeyResolver —
// production wiring uses auth.JWKSResolver pointed at the dashboard's
// /api/auth/jwks endpoint; tests use auth.StaticKeyResolver.
type IdentityVerifier struct {
	keys     auth.KeyResolver
	issuer   string
	audience string
}

// IdentityOption tunes an IdentityVerifier.
type IdentityOption func(*IdentityVerifier)

// WithIssuer overrides the expected `iss` claim (defaults to IssuerDashboard).
func WithIssuer(iss string) IdentityOption {
	return func(v *IdentityVerifier) { v.issuer = iss }
}

// WithAudience overrides the expected `aud` claim (defaults to AudienceContentAPI).
func WithAudience(aud string) IdentityOption {
	return func(v *IdentityVerifier) { v.audience = aud }
}

// NewIdentityVerifier constructs a verifier delegating public-key lookup to the
// supplied KeyResolver.
func NewIdentityVerifier(keys auth.KeyResolver, opts ...IdentityOption) *IdentityVerifier {
	v := &IdentityVerifier{
		keys:     keys,
		issuer:   IssuerDashboard,
		audience: AudienceContentAPI,
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// NewIdentityVerifierFromJWKS constructs a verifier backed by the dashboard's
// JWKS endpoint. Returns an error if jwksURL is empty.
func NewIdentityVerifierFromJWKS(jwksURL string, opts ...IdentityOption) (*IdentityVerifier, error) {
	if jwksURL == "" {
		return nil, errors.New("authz: JWKS URL required")
	}
	resolver := auth.NewJWKSResolver(jwksURL, auth.WithJWKSMinRefreshInterval(2*time.Second))
	return NewIdentityVerifier(resolver, opts...), nil
}

// Verify parses and validates tokenString, returning the verified principal.
// It returns ErrExpired for expired tokens and ErrInvalidToken (wrapped) for
// every other failure.
func (v *IdentityVerifier) Verify(ctx context.Context, tokenString string) (*VerifiedIdentity, error) {
	if tokenString == "" {
		return nil, fmt.Errorf("%w: empty token", ErrInvalidToken)
	}

	claims := &IdentityClaims{}
	parser := jwt.NewParser(
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(jwtLeeway),
	)
	token, parseErr := parser.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method %q", t.Header["alg"])
		}
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("identity JWT missing kid header")
		}
		return v.keys.Resolve(ctx, kid)
	})
	if parseErr != nil {
		if errors.Is(parseErr, jwt.ErrTokenExpired) {
			return nil, ErrExpired
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, parseErr)
	}
	if token == nil || !token.Valid {
		return nil, ErrInvalidToken
	}

	return &VerifiedIdentity{
		Subject:   claims.Subject,
		Identity:  claims.Identity,
		Groups:    claims.Groups,
		Workspace: claims.Workspace,
		Anonymous: claims.Anonymous,
	}, nil
}
