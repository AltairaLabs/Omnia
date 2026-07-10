/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package facade

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrNoMgmtToken is returned when a management-plane request carries no bearer
// token. The mgmt twin listener MUST fail closed on it.
var ErrNoMgmtToken = errors.New("custom-facade: no management-plane token")

// KeyResolver looks up an RSA public key by JWT kid. The mgmt verifier uses it
// as the golang-jwt keyfunc source. A JWKSResolver fetches the dashboard's
// signing keys over HTTP; tests can supply an in-memory implementation.
type KeyResolver interface {
	Resolve(ctx context.Context, kid string) (*rsa.PublicKey, error)
}

// MgmtVerifier validates dashboard-minted RS256 management-plane JWTs. It fails
// closed: any missing/malformed/expired/unknown-signer token is rejected. Only
// a token whose RS256 signature verifies against a JWKS key survives.
type MgmtVerifier struct {
	keys KeyResolver
}

// NewMgmtVerifier builds a verifier backed by the given key resolver.
func NewMgmtVerifier(keys KeyResolver) *MgmtVerifier {
	return &MgmtVerifier{keys: keys}
}

// Verify parses and validates a management-plane JWT string, returning its
// registered claims on success. It enforces RS256 (rejecting alg-confusion /
// "none") and standard expiry. Any failure returns a non-nil error — callers
// must treat that as a 401 and never fall open.
func (v *MgmtVerifier) Verify(ctx context.Context, tokenString string) (*jwt.RegisteredClaims, error) {
	tokenString = strings.TrimSpace(tokenString)
	tokenString = strings.TrimPrefix(tokenString, "Bearer ")
	tokenString = strings.TrimPrefix(tokenString, "bearer ")
	if tokenString == "" {
		return nil, ErrNoMgmtToken
	}

	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(tokenString, claims, v.keyfunc(ctx),
		jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		return nil, fmt.Errorf("custom-facade: mgmt token rejected: %w", err)
	}
	return claims, nil
}

// keyfunc resolves the signing key for a parsed token via the kid header.
func (v *MgmtVerifier) keyfunc(ctx context.Context) jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("custom-facade: mgmt token missing kid")
		}
		return v.keys.Resolve(ctx, kid)
	}
}

// Middleware wraps an HTTP handler with fail-closed management-plane auth: it
// rejects (401) any request whose Authorization bearer token does not verify.
func (v *MgmtVerifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := v.Verify(r.Context(), r.Header.Get("Authorization")); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// JWKSResolver fetches RSA public keys from a JWKS endpoint (the dashboard's
// signing-key endpoint, OMNIA_MGMT_PLANE_JWKS_URL) and caches them by kid,
// re-fetching on a cache miss so key rotation propagates without a restart.
type JWKSResolver struct {
	url    string
	client *http.Client

	mu   sync.Mutex
	keys map[string]*rsa.PublicKey
}

// NewJWKSResolver constructs a resolver that fetches keys from url.
func NewJWKSResolver(url string) *JWKSResolver {
	return &JWKSResolver{
		url:    url,
		client: &http.Client{Timeout: 5 * time.Second},
		keys:   map[string]*rsa.PublicKey{},
	}
}

// Resolve returns the public key for kid, fetching the JWKS endpoint on a
// cache miss. An unknown kid after a fresh fetch is an error (fail closed).
func (r *JWKSResolver) Resolve(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	r.mu.Lock()
	if k, ok := r.keys[kid]; ok {
		r.mu.Unlock()
		return k, nil
	}
	r.mu.Unlock()

	if err := r.refresh(ctx); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if k, ok := r.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("custom-facade: unknown JWT kid %q", kid)
}

// refresh fetches and replaces the cached keyset.
func (r *JWKSResolver) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return fmt.Errorf("custom-facade: build jwks request: %w", err)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("custom-facade: fetch jwks %s: %w", r.url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("custom-facade: jwks %s status %d", r.url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("custom-facade: read jwks body: %w", err)
	}
	keys, err := parseJWKS(body)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.keys = keys
	r.mu.Unlock()
	return nil
}

// jwksDoc is the subset of a JWKS document we parse (RSA keys only).
type jwksDoc struct {
	Keys []struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

// parseJWKS decodes a JWKS document into a kid->key map. Non-RSA keys and keys
// without a kid are skipped; a malformed modulus/exponent is an error.
func parseJWKS(body []byte) (map[string]*rsa.PublicKey, error) {
	var doc jwksDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("custom-facade: parse jwks: %w", err)
	}
	out := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		key, err := rsaKeyFromNE(k.N, k.E)
		if err != nil {
			return nil, fmt.Errorf("custom-facade: kid %q: %w", k.Kid, err)
		}
		out[k.Kid] = key
	}
	return out, nil
}

// rsaKeyFromNE reconstructs an RSA public key from base64url modulus/exponent.
func rsaKeyFromNE(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}
	exp := 0
	for _, b := range eBytes {
		exp = exp<<8 | int(b)
	}
	if exp == 0 {
		return nil, errors.New("zero RSA exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: exp}, nil
}
