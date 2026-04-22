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
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/altairalabs/omnia/pkg/policy"
)

// Default issuer / audience values embedded in dashboard-minted tokens.
const (
	DefaultMgmtPlaneIssuer   = "omnia-dashboard"
	DefaultMgmtPlaneAudience = "omnia-facade"
)

// bearerPrefix is the case-sensitive scheme tag on Authorization: Bearer <token>.
const bearerPrefix = "Bearer "

// MgmtPlaneValidator verifies JWTs minted by the dashboard's signing key
// and presented by an admin against the facade (for example, the "Try this
// agent" debug view). It is one entry in the facade's auth chain.
type MgmtPlaneValidator struct {
	publicKey *rsa.PublicKey
	issuer    string
	audience  string
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

// NewMgmtPlaneValidator constructs a validator that trusts JWTs signed by
// the RSA public key at pubKeyPath. The file may contain either a PKIX
// "PUBLIC KEY" PEM block or an x509 "CERTIFICATE" PEM block — Helm's
// genSelfSigned emits the latter, operators supplying their own key may
// use either.
func NewMgmtPlaneValidator(pubKeyPath string, opts ...MgmtPlaneOption) (*MgmtPlaneValidator, error) {
	data, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read mgmt-plane public key %q: %w", pubKeyPath, err)
	}
	key, err := parseRSAPublicKey(data)
	if err != nil {
		return nil, fmt.Errorf("parse mgmt-plane public key %q: %w", pubKeyPath, err)
	}

	v := &MgmtPlaneValidator{
		publicKey: key,
		issuer:    DefaultMgmtPlaneIssuer,
		audience:  DefaultMgmtPlaneAudience,
	}
	for _, opt := range opts {
		opt(v)
	}
	return v, nil
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
func (v *MgmtPlaneValidator) Validate(_ context.Context, r *http.Request) (*policy.AuthenticatedIdentity, error) {
	tokenString, err := extractBearer(r)
	if err != nil {
		return nil, err
	}

	claims := &mgmtPlaneClaims{}
	parser := jwt.NewParser(
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
	)
	token, parseErr := parser.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method %q", t.Header["alg"])
		}
		return v.publicKey, nil
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

// parseRSAPublicKey accepts either a PKIX "PUBLIC KEY" PEM block or an
// x509 "CERTIFICATE" PEM block and returns the RSA public key. Helm's
// genSelfSigned template produces certificates; operators BYO-ing a raw
// public key may use the other form.
func parseRSAPublicKey(data []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}

	switch block.Type {
	case "CERTIFICATE":
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse certificate: %w", err)
		}
		key, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("certificate public key is %T, want *rsa.PublicKey", cert.PublicKey)
		}
		return key, nil
	case "PUBLIC KEY":
		pub, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKIX public key: %w", err)
		}
		key, ok := pub.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("public key is %T, want *rsa.PublicKey", pub)
		}
		return key, nil
	case "RSA PUBLIC KEY":
		key, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS1 public key: %w", err)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("unsupported PEM block type %q", block.Type)
	}
}
