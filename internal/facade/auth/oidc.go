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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/altairalabs/omnia/pkg/policy"
)

// Defaults for OIDC claim names. Mirrors the CRD's OIDCClaimMapping
// defaults so operators who don't override them get sensible behaviour.
const (
	DefaultOIDCSubjectClaim = "sub"
	DefaultOIDCRoleClaim    = "omnia.role"
	DefaultOIDCEndUserClaim = "sub"
)

// JSONWebKey is the RFC 7517 §4 shape the validator consumes. Only
// RSA / RS256 fields are populated in MVP — Ed25519 and ECDSA support
// is a future addition when a customer actually asks for it.
type JSONWebKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use,omitempty"`
	Alg string `json:"alg,omitempty"`
	N   string `json:"n,omitempty"`
	E   string `json:"e,omitempty"`
}

// JWKS is the RFC 7517 §5 envelope.
type JWKS struct {
	Keys []JSONWebKey `json:"keys"`
}

// KeySet is the facade-side in-memory JWKS cache. Maps kid → RSA public
// key for O(1) lookup during JWT verification. Safe for concurrent use.
type KeySet struct {
	mu   sync.RWMutex
	keys map[string]*rsa.PublicKey
}

// NewKeySet builds a KeySet from a parsed JWKS. Non-RSA keys are
// skipped with no error (the validator just won't admit tokens signed
// by them); a JWKS with zero usable keys returns an error so callers
// see the misconfig immediately.
func NewKeySet(jwks *JWKS) (*KeySet, error) {
	if jwks == nil {
		return nil, errors.New("nil JWKS")
	}
	set := &KeySet{keys: map[string]*rsa.PublicKey{}}
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := rsaPublicKeyFromJWK(k)
		if err != nil {
			// Log-worthy but non-fatal: skip the key and carry on.
			continue
		}
		if k.Kid == "" {
			// A JWK without a kid can't be looked up by the verifier.
			// Skip with no error — odd but legal under RFC 7517.
			continue
		}
		set.keys[k.Kid] = pub
	}
	if len(set.keys) == 0 {
		return nil, errors.New("JWKS contains no usable RSA keys")
	}
	return set, nil
}

// NewKeySetFromJSON parses a raw JWKS JSON blob. Convenience wrapper
// for the facade-side Secret reader which stores the raw issuer
// response verbatim.
func NewKeySetFromJSON(raw []byte) (*KeySet, error) {
	var jwks JWKS
	if err := json.Unmarshal(raw, &jwks); err != nil {
		return nil, fmt.Errorf("parse JWKS: %w", err)
	}
	return NewKeySet(&jwks)
}

// Lookup returns the RSA public key for the given kid, or (nil, false).
func (s *KeySet) Lookup(kid string) (*rsa.PublicKey, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k, ok := s.keys[kid]
	return k, ok
}

// Replace atomically swaps the whole key map — used by cmd/agent's
// periodic refresh to pick up rotated keys.
func (s *KeySet) Replace(keys map[string]*rsa.PublicKey) {
	s.mu.Lock()
	s.keys = keys
	s.mu.Unlock()
}

// rsaPublicKeyFromJWK decodes the `n` and `e` fields (base64url-encoded
// big-endian integers per RFC 7518 §6.3) into an rsa.PublicKey.
func rsaPublicKeyFromJWK(k JSONWebKey) (*rsa.PublicKey, error) {
	if k.N == "" || k.E == "" {
		return nil, errors.New("JWK missing n or e")
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}
	n := new(big.Int).SetBytes(nBytes)
	// RFC 7518 §6.3.1.2 — e is an unsigned big-endian integer in as few
	// bytes as possible. Most IdPs ship "AQAB" (= 65537) so e fits in
	// an int easily.
	e := new(big.Int).SetBytes(eBytes)
	if !e.IsInt64() {
		return nil, fmt.Errorf("exponent too large")
	}
	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}

// OIDCValidator verifies JWTs against a JWKS cache. Construction takes
// the issuer / audience the IdP issued, a KeySet supplier, and an
// OIDCClaimMapping (nil → defaults). Keyset lookup is pluggable so
// cmd/agent can swap in a refreshing impl without this file knowing
// about k8s or Secrets.
type OIDCValidator struct {
	issuer   string
	audience string
	keys     KeySupplier
	mapping  OIDCClaimMapping
}

// KeySupplier abstracts the JWKS backing store — in production a
// refreshing Secret-backed cache, in tests a fixed map.
type KeySupplier interface {
	Lookup(kid string) (*rsa.PublicKey, bool)
}

// OIDCClaimMapping is the validator-side mirror of the CRD's
// OIDCClaimMapping. Empty strings fall back to the Default*Claim
// constants so tests don't have to fill every field.
type OIDCClaimMapping struct {
	Subject string
	Role    string
	EndUser string
}

// OIDCOption tunes an OIDCValidator.
type OIDCOption func(*OIDCValidator)

// WithOIDCClaimMapping overrides the default claim names. Empty-string
// fields keep the corresponding default.
func WithOIDCClaimMapping(m OIDCClaimMapping) OIDCOption {
	return func(v *OIDCValidator) {
		if m.Subject != "" {
			v.mapping.Subject = m.Subject
		}
		if m.Role != "" {
			v.mapping.Role = m.Role
		}
		if m.EndUser != "" {
			v.mapping.EndUser = m.EndUser
		}
	}
}

// NewOIDCValidator constructs an OIDC JWT validator.
func NewOIDCValidator(issuer, audience string, keys KeySupplier, opts ...OIDCOption) (*OIDCValidator, error) {
	if issuer == "" {
		return nil, errors.New("oidc: issuer is required")
	}
	if audience == "" {
		return nil, errors.New("oidc: audience is required")
	}
	if keys == nil {
		return nil, errors.New("oidc: KeySupplier is required")
	}
	v := &OIDCValidator{
		issuer:   issuer,
		audience: audience,
		keys:     keys,
		mapping: OIDCClaimMapping{
			Subject: DefaultOIDCSubjectClaim,
			Role:    DefaultOIDCRoleClaim,
			EndUser: DefaultOIDCEndUserClaim,
		},
	}
	for _, opt := range opts {
		opt(v)
	}
	return v, nil
}

// Validate implements Validator. ErrNoCredential when no Bearer header;
// ErrInvalidCredential on signature / issuer / audience / kid failures;
// ErrExpired when the JWT is past its exp.
func (v *OIDCValidator) Validate(_ context.Context, r *http.Request) (*policy.AuthenticatedIdentity, error) {
	tokenString, err := extractBearer(r)
	if err != nil {
		return nil, err
	}

	// Parse the token once with a Keyfunc that pulls the key from the
	// supplier using the kid header. ParseWithClaims handles signature
	// verification + standard time claims (exp/nbf/iat) + iss + aud.
	claims := jwt.MapClaims{}
	parser := jwt.NewParser(
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
	)
	token, parseErr := parser.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method %q", t.Header["alg"])
		}
		kid, ok := t.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, errors.New("oidc: token header missing kid")
		}
		key, ok := v.keys.Lookup(kid)
		if !ok {
			return nil, fmt.Errorf("oidc: no key for kid %q", kid)
		}
		return key, nil
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

	return v.identityFromClaims(claims), nil
}

// identityFromClaims extracts an AuthenticatedIdentity from verified
// JWT claims. Split out of Validate so the hot-path has fewer nested
// branches (SonarCloud's cognitive-complexity gate at 15 prefers this
// shape).
func (v *OIDCValidator) identityFromClaims(claims jwt.MapClaims) *policy.AuthenticatedIdentity {
	id := &policy.AuthenticatedIdentity{Origin: policy.OriginOIDC}
	if sub, ok := stringClaim(claims, v.mapping.Subject); ok {
		id.Subject = sub
	}
	if eu, ok := stringClaim(claims, v.mapping.EndUser); ok {
		id.EndUser = eu
	}
	if id.EndUser == "" {
		// Fall back to Subject so Identity.EndUser is always populated.
		// Design doc semantics: "Falls back to Subject if claim missing."
		id.EndUser = id.Subject
	}
	if role, ok := stringClaim(claims, v.mapping.Role); ok {
		id.Role = role
	}
	// No role claim is fine — ToolPolicy rules can still gate on
	// identity.origin. Role stays empty intentionally.

	id.Claims = extractExtraClaims(claims, v.mapping)

	// Surface the token's validity window so identity consumers that
	// care (e.g., session logging) can trace its lifetime.
	if exp, ok := numericDateClaim(claims, "exp"); ok {
		id.ExpiresAt = exp
	}
	if iat, ok := numericDateClaim(claims, "iat"); ok {
		id.IssuedAt = iat
	}
	return id
}

// extractExtraClaims flattens any string-valued claims not already
// absorbed into Identity.Subject / Role / EndUser so ToolPolicy CEL
// can reference them via `identity.claims.<name>`. Returns nil when
// no extras are present (keeps Identity.Claims nil rather than an
// empty map, which the CEL evaluator handles via `has()`).
func extractExtraClaims(claims jwt.MapClaims, mapping OIDCClaimMapping) map[string]string {
	extra := map[string]string{}
	for k, vv := range claims {
		if k == mapping.Subject || k == mapping.Role || k == mapping.EndUser {
			continue
		}
		if s, ok := vv.(string); ok && s != "" {
			extra[k] = s
		}
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

// stringClaim extracts a string value from jwt.MapClaims. Falls back
// cleanly on missing / non-string entries.
func stringClaim(m jwt.MapClaims, name string) (string, bool) {
	if name == "" {
		return "", false
	}
	v, ok := m[name]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// numericDateClaim extracts a JWT NumericDate (seconds-since-epoch as
// a JSON number) and converts to time.Time. The golang-jwt library
// already enforces exp/nbf internally; we only need this to populate
// Identity.ExpiresAt / IssuedAt for downstream audit.
func numericDateClaim(m jwt.MapClaims, name string) (time.Time, bool) {
	v, present := m[name]
	if !present {
		return time.Time{}, false
	}
	switch typed := v.(type) {
	case float64:
		return time.Unix(int64(typed), 0).UTC(), true
	case int64:
		return time.Unix(typed, 0).UTC(), true
	}
	return time.Time{}, false
}
