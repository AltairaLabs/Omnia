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
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// ErrUnknownKid is returned by a KeyResolver when no key is registered
// under the requested kid (after at least one fetch). Callers should
// surface it as ErrInvalidCredential when checking JWT signatures.
var ErrUnknownKid = errors.New("auth: unknown JWT kid")

// KeyResolver looks up the RSA public key associated with a JWT kid.
// JWKSResolver fetches keys over HTTP from a dashboard-style JWKS
// endpoint; StaticKeyResolver wraps an in-memory map for tests and for
// callers that load keys out of band.
type KeyResolver interface {
	Resolve(ctx context.Context, kid string) (*rsa.PublicKey, error)
}

// StaticKeyResolver is a KeyResolver backed by a fixed in-memory key
// map. Useful for tests; production wiring uses JWKSResolver.
type StaticKeyResolver struct {
	Keys map[string]*rsa.PublicKey
}

// Resolve returns the key for kid or ErrUnknownKid.
func (s *StaticKeyResolver) Resolve(_ context.Context, kid string) (*rsa.PublicKey, error) {
	if s == nil || s.Keys == nil {
		return nil, ErrUnknownKid
	}
	if k, ok := s.Keys[kid]; ok {
		return k, nil
	}
	return nil, ErrUnknownKid
}

// JWKSResolver fetches RSA public keys from a JWKS endpoint and caches
// them by kid. On a kid cache miss it re-fetches the keyset (single-
// flight); this is what lets a key rotation propagate to facade pods
// without restarting them and without polling. The cache has no time-
// based expiry on purpose: the JWKS endpoint is on the dashboard's
// in-cluster service, and a cache miss is the only signal we need that
// rotation has happened.
type JWKSResolver struct {
	url    string
	client *http.Client

	mu       sync.Mutex
	keys     map[string]*rsa.PublicKey
	fetching bool
	cond     *sync.Cond
}

// JWKSOption tunes a JWKSResolver.
type JWKSOption func(*JWKSResolver)

// WithJWKSHTTPClient overrides the HTTP client used to fetch the
// keyset. The default is a 5-second-timeout client.
func WithJWKSHTTPClient(c *http.Client) JWKSOption {
	return func(r *JWKSResolver) { r.client = c }
}

// NewJWKSResolver constructs a resolver that fetches keys from url.
// url is typically the dashboard's in-cluster JWKS endpoint, e.g.
// http://omnia-dashboard.omnia-system.svc.cluster.local:3000/api/auth/jwks.
func NewJWKSResolver(url string, opts ...JWKSOption) *JWKSResolver {
	r := &JWKSResolver{
		url:    url,
		client: &http.Client{Timeout: 5 * time.Second},
		keys:   map[string]*rsa.PublicKey{},
	}
	r.cond = sync.NewCond(&r.mu)
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Resolve returns the public key for kid, fetching the JWKS endpoint
// once if the kid is not already cached. Concurrent callers waiting on
// the same fetch are coalesced.
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
	return nil, ErrUnknownKid
}

// refresh fetches the JWKS body and replaces the in-memory cache.
// Single-flight: a second concurrent caller waits for the in-flight
// fetch instead of duplicating it.
func (r *JWKSResolver) refresh(ctx context.Context) error {
	r.mu.Lock()
	for r.fetching {
		r.cond.Wait()
	}
	r.fetching = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.fetching = false
		r.cond.Broadcast()
		r.mu.Unlock()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return fmt.Errorf("jwks: build request: %w", err)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("jwks: fetch %s: %w", r.url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks: %s returned status %d", r.url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("jwks: read body: %w", err)
	}
	keys, err := parseJWKSBody(body)
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.keys = keys
	r.mu.Unlock()
	return nil
}

// jwksDoc is the on-the-wire shape of a JWKS response. We only handle
// the RSA fields here — other key types are silently skipped.
type jwksDoc struct {
	Keys []struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		Alg string `json:"alg,omitempty"`
		Use string `json:"use,omitempty"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

// parseJWKSBody decodes a JWKS document and returns a kid -> key map.
// Non-RSA keys, keys without a kid, and keys whose modulus / exponent
// fail to decode are skipped (the latter raises an error so we don't
// silently drop a malformed key the dashboard intended to ship).
func parseJWKSBody(body []byte) (map[string]*rsa.PublicKey, error) {
	var doc jwksDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("jwks: parse body: %w", err)
	}
	out := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, fmt.Errorf("jwks: decode n for kid %q: %w", k.Kid, err)
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, fmt.Errorf("jwks: decode e for kid %q: %w", k.Kid, err)
		}
		// e is typically a few bytes (0x010001 for the canonical 65537).
		// Pack big-endian into an int.
		exp := 0
		for _, b := range eBytes {
			exp = exp<<8 | int(b)
		}
		if exp == 0 {
			return nil, fmt.Errorf("jwks: zero RSA exponent for kid %q", k.Kid)
		}
		out[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: exp,
		}
	}
	return out, nil
}
