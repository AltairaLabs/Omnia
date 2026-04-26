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
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/altairalabs/omnia/pkg/policy"
)

// Default issuer / audience values embedded in dashboard-minted tokens.
const (
	DefaultMgmtPlaneIssuer   = "omnia-dashboard"
	DefaultMgmtPlaneAudience = "omnia-facade"
)

// jwtLeeway tolerates small clock drift between the facade and the
// token issuer on exp/nbf/iat checks. Without leeway a facade pod
// running a second ahead of the dashboard 401s freshly-minted tokens —
// common enough in busy clusters that pen-test finding C-3 triage
// surfaced it explicitly. 30s is conservative enough that short
// expiries still work as intended and aggressive enough to catch
// genuinely-expired tokens.
const jwtLeeway = 30 * time.Second

// bearerPrefix is the case-sensitive scheme tag on Authorization: Bearer <token>.
const bearerPrefix = "Bearer "

// MgmtPlaneValidator verifies JWTs minted by the dashboard's signing key
// and presented by an admin against the facade (for example, the "Try this
// agent" debug view). Keys are resolved by JWT kid through a KeyResolver
// — production wiring uses JWKSResolver pointed at the dashboard's
// /api/auth/jwks endpoint, tests use StaticKeyResolver.
type MgmtPlaneValidator struct {
	keys     KeyResolver
	issuer   string
	audience string
}

// MgmtPlaneOption tunes a MgmtPlaneValidator.
type MgmtPlaneOption func(*MgmtPlaneValidator)

// WithMgmtPlaneIssuer overrides the expected `iss` claim. Defaults to
// DefaultMgmtPlaneIssuer.
func WithMgmtPlaneIssuer(iss string) MgmtPlaneOption {
	return func(v *MgmtPlaneValidator) { v.issuer = iss }
}

// WithMgmtPlaneAudience overrides the expected `aud` claim. Defaults to
// DefaultMgmtPlaneAudience.
func WithMgmtPlaneAudience(aud string) MgmtPlaneOption {
	return func(v *MgmtPlaneValidator) { v.audience = aud }
}

// NewMgmtPlaneValidatorWithResolver constructs a validator that delegates
// public-key lookup to the supplied KeyResolver. Useful for tests and
// for callers that want to plug in a non-HTTP key source.
func NewMgmtPlaneValidatorWithResolver(r KeyResolver, opts ...MgmtPlaneOption) *MgmtPlaneValidator {
	v := &MgmtPlaneValidator{
		keys:     r,
		issuer:   DefaultMgmtPlaneIssuer,
		audience: DefaultMgmtPlaneAudience,
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// NewMgmtPlaneValidator constructs a validator backed by a JWKS endpoint.
// jwksURL is typically the dashboard's in-cluster service URL, e.g.
// http://omnia-dashboard.omnia-system.svc.cluster.local:3000/api/auth/jwks.
// Returns an error if jwksURL is empty.
func NewMgmtPlaneValidator(jwksURL string, opts ...MgmtPlaneOption) (*MgmtPlaneValidator, error) {
	if jwksURL == "" {
		return nil, errors.New("mgmt-plane: JWKS URL required")
	}
	return NewMgmtPlaneValidatorWithResolver(NewJWKSResolver(jwksURL), opts...), nil
}

// mgmtPlaneClaims is the dashboard-minted JWT shape.
type mgmtPlaneClaims struct {
	jwt.RegisteredClaims
	Origin    string `json:"origin"`
	Agent     string `json:"agent,omitempty"`
	Workspace string `json:"workspace,omitempty"`
}

// Validate implements Validator. It admits requests carrying a valid
// mgmt-plane JWT and rejects everything else with one of the typed errors.
func (v *MgmtPlaneValidator) Validate(ctx context.Context, r *http.Request) (*policy.AuthenticatedIdentity, error) {
	tokenString, err := extractBearer(r)
	if err != nil {
		return nil, err
	}

	claims := &mgmtPlaneClaims{}
	parser := jwt.NewParser(
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithLeeway(jwtLeeway),
	)
	token, parseErr := parser.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method %q", t.Header["alg"])
		}
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("mgmt-plane JWT missing kid header")
		}
		return v.keys.Resolve(ctx, kid)
	})
	if parseErr != nil {
		if errors.Is(parseErr, jwt.ErrTokenExpired) {
			return nil, ErrExpired
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidCredential, parseErr)
	}
	if token == nil || !token.Valid {
		return nil, ErrInvalidCredential
	}
	if claims.Origin != policy.OriginManagementPlane {
		return nil, fmt.Errorf("%w: origin %q is not management-plane", ErrInvalidCredential, claims.Origin)
	}

	id := &policy.AuthenticatedIdentity{
		Origin:    policy.OriginManagementPlane,
		Subject:   claims.Subject,
		EndUser:   claims.Subject,
		Workspace: claims.Workspace,
		Agent:     claims.Agent,
		// Mgmt-plane tokens are minted only after dashboard auth admits
		// the user; they always carry admin privileges for the agent they
		// target. Per-user role mapping is a future extension.
		Role: policy.RoleAdmin,
	}
	if claims.IssuedAt != nil {
		id.IssuedAt = claims.IssuedAt.Time
	}
	if claims.ExpiresAt != nil {
		id.ExpiresAt = claims.ExpiresAt.Time
	}
	return id, nil
}

// extractBearer returns the token payload from a "Bearer <token>"
// Authorization header, or one of the sentinel errors when the header is
// absent or malformed.
func extractBearer(r *http.Request) (string, error) {
	raw := r.Header.Get("Authorization")
	if raw == "" {
		return "", ErrNoCredential
	}
	if !strings.HasPrefix(raw, bearerPrefix) {
		// Non-Bearer scheme (Basic, Negotiate, etc.) — not ours. Let the
		// chain continue; a later validator may understand this scheme or
		// the no-auth tail may reject.
		return "", ErrNoCredential
	}
	token := strings.TrimSpace(raw[len(bearerPrefix):])
	if token == "" {
		return "", fmt.Errorf("%w: empty bearer token", ErrInvalidCredential)
	}
	return token, nil
}
