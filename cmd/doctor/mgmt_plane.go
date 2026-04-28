/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package main

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/altairalabs/omnia/internal/facade/auth"
)

// MgmtPlaneTokenMinter signs short-lived mgmt-plane JWTs with the
// dashboard's RSA signing key. Doctor's WebSocket dialer uses one of
// these tokens on every Agent / Memory / Sessions check — without it,
// the facade's mgmt-plane validator (added in commit 30a286bf) rejects
// the upgrade with 401 ("auth: no credential present"), which is the
// other half of issue #1040.
//
// The minter's trust model mirrors the dashboard's: whoever can mount
// the signing-keypair Secret can mint admin tokens. Doctor already
// needs admin scope to verify privacy / supersession behaviours, so
// this is consistent with its existing privilege.
//
// Tokens are minted on demand and cached for slightly less than their
// TTL so a single Doctor run reuses one signature instead of paying
// the RSA cost per check (smoke runs hit ~6 WebSocket checks).
type MgmtPlaneTokenMinter struct {
	key      *rsa.PrivateKey
	kid      string
	issuer   string
	audience string
	subject  string
	ttl      time.Duration
	now      func() time.Time

	mu     sync.Mutex
	cached cachedMgmtPlaneToken
}

type cachedMgmtPlaneToken struct {
	token   string
	agent   string
	worksp  string
	expires time.Time
}

// MgmtPlaneTokenMinterOptions configures a MgmtPlaneTokenMinter.
type MgmtPlaneTokenMinterOptions struct {
	// KeyPath points at the PKCS#8 (or PKCS#1) RSA private key PEM file —
	// the same file the dashboard mounts at /etc/omnia/mgmt-plane/tls.key.
	KeyPath string
	// Issuer / Audience override the iss / aud claims; empty values
	// fall back to auth.DefaultMgmtPlaneIssuer /
	// auth.DefaultMgmtPlaneAudience so the facade's validator admits
	// the token without configuration drift.
	Issuer   string
	Audience string
	// Subject is the principal name that surfaces in audit logs and
	// ToolPolicy bindings as identity.subject. Empty falls back to
	// "doctor-smoke-test" so the audit trail clearly attributes
	// Doctor's actions to the smoke runner rather than a real user.
	Subject string
	// TTL sets the token lifetime. Zero falls back to 5 minutes —
	// matches the dashboard's default and the facade's leeway window.
	TTL time.Duration
}

// defaultDoctorSubject is the principal name baked into Doctor-minted
// tokens when the caller doesn't override it. Audit logs use this to
// distinguish Doctor's smoke-test traffic from real user requests.
const defaultDoctorSubject = "doctor-smoke-test"

// defaultMgmtPlaneTTL is how long a minted token stays valid. Matches
// the dashboard's DEFAULT_TTL_SECONDS so behaviour is consistent
// across the two minters.
const defaultMgmtPlaneTTL = 5 * time.Minute

// reuseSafetyMargin is the slack subtracted from a cached token's
// expiry before it's considered "still good to reuse". Without it the
// minter could hand out a token that's about to expire mid-handshake.
const reuseSafetyMargin = 30 * time.Second

// NewMgmtPlaneTokenMinter loads the RSA private key at opts.KeyPath
// and returns a minter ready to sign tokens. The kid is the RFC 7638
// thumbprint of the matching public JWK so the facade's JWKS resolver
// (which derives the same thumbprint server-side) picks the right
// key during rotation.
//
// Returns an error if the file is missing, unreadable, not PEM, or
// doesn't contain an RSA key. Boot-time errors are intentionally
// loud — silently disabling token minting would mean Doctor's WS
// checks fail open, which is exactly the bug we're fixing.
func NewMgmtPlaneTokenMinter(opts MgmtPlaneTokenMinterOptions) (*MgmtPlaneTokenMinter, error) {
	if opts.KeyPath == "" {
		return nil, errors.New("mgmt-plane minter: KeyPath required")
	}
	pemBytes, err := os.ReadFile(opts.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("mgmt-plane minter: read key %q: %w", opts.KeyPath, err)
	}
	key, err := parseRSAPrivateKey(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("mgmt-plane minter: parse key %q: %w", opts.KeyPath, err)
	}
	kid := rsaThumbprint(&key.PublicKey)

	issuer := opts.Issuer
	if issuer == "" {
		issuer = auth.DefaultMgmtPlaneIssuer
	}
	audience := opts.Audience
	if audience == "" {
		audience = auth.DefaultMgmtPlaneAudience
	}
	subject := opts.Subject
	if subject == "" {
		subject = defaultDoctorSubject
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = defaultMgmtPlaneTTL
	}

	return &MgmtPlaneTokenMinter{
		key:      key,
		kid:      kid,
		issuer:   issuer,
		audience: audience,
		subject:  subject,
		ttl:      ttl,
		now:      time.Now,
	}, nil
}

// parseRSAPrivateKey accepts PKCS#1 ("RSA PRIVATE KEY") or PKCS#8
// ("PRIVATE KEY") PEM blocks — Helm's genSelfSigned emits PKCS#8 — and
// returns the underlying *rsa.PrivateKey. Non-RSA keys are rejected
// (the facade only validates RS256).
func parseRSAPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		anyKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := anyKey.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("PKCS#8 key is %T, expected *rsa.PrivateKey", anyKey)
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("unexpected PEM block type %q", block.Type)
	}
}

// rsaThumbprint produces the RFC 7638 thumbprint of an RSA public key
// in the same shape the dashboard's publicJwkFromKey emits. The
// canonical JSON has fields in lexicographic order with no whitespace,
// SHA-256 hashed and base64url encoded. Mismatched canonicalisation
// would produce a different kid, the JWKS resolver wouldn't find a
// key, and the facade would reject the token.
func rsaThumbprint(pub *rsa.PublicKey) string {
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	canonical := fmt.Sprintf(`{"e":"%s","kty":"RSA","n":"%s"}`, e, n)
	sum := sha256.Sum256([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// Token returns a valid mgmt-plane JWT for the supplied agent +
// workspace. A cached token is reused if it's bound to the same
// (agent, workspace) pair and still has at least reuseSafetyMargin
// before expiry. Otherwise a fresh token is signed.
func (m *MgmtPlaneTokenMinter) Token(agent, workspace string) (string, error) {
	if m == nil {
		return "", errors.New("mgmt-plane minter: nil receiver")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	if m.cached.token != "" &&
		m.cached.agent == agent &&
		m.cached.worksp == workspace &&
		now.Add(reuseSafetyMargin).Before(m.cached.expires) {
		return m.cached.token, nil
	}

	expires := now.Add(m.ttl)
	claims := jwt.MapClaims{
		"iss":       m.issuer,
		"sub":       m.subject,
		"aud":       m.audience,
		"exp":       expires.Unix(),
		"nbf":       now.Add(-1 * time.Second).Unix(),
		"iat":       now.Unix(),
		"origin":    "management-plane",
		"agent":     agent,
		"workspace": workspace,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = m.kid
	signed, err := token.SignedString(m.key)
	if err != nil {
		return "", fmt.Errorf("mgmt-plane minter: sign: %w", err)
	}
	m.cached = cachedMgmtPlaneToken{
		token:   signed,
		agent:   agent,
		worksp:  workspace,
		expires: expires,
	}
	return signed, nil
}
