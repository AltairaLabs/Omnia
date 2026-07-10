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
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/altairalabs/omnia/pkg/policy"
)

// ClientKey is the in-memory representation of a single agent client key.
// The value is a sha256 hash of the raw bearer token; the validator never
// stores the raw value. Built by cmd/agent's KeyStore from the labelled
// Secret data; defined here so the validator can be unit-tested against
// a synthetic []ClientKey slice.
type ClientKey struct {
	// ID is a stable identifier for the key (the Secret name's suffix
	// after `agent-<agent>-clientkey-`). Surfaces as Identity.Subject so
	// audit and ToolPolicy rules can distinguish callers.
	ID string

	// HashHex is the lowercase-hex sha256 of the raw token bytes. The
	// validator compares the hex representation of the incoming token's
	// hash against this with a constant-time compare.
	HashHex string

	// Claims are arbitrary per-key claims (e.g. "role", or any other
	// caller-defined key) surfaced to ToolPolicy as identity.claims.*.
	// When empty, the validator falls back to {"role": defaultRole} —
	// ClientKeysAuth's defaultRole applied at the cmd/agent layer.
	Claims map[string]string

	// ExpiresAt is the absolute time after which the key is no longer
	// valid. Zero value means "no expiry" — the operator opts in by
	// setting an expiresAt field on the Secret.
	ExpiresAt time.Time
}

// KeyStore is the lookup interface ClientKeyValidator depends on.
// cmd/agent supplies a Secret-backed implementation; tests can swap in a
// hand-built map. Lookup by hex sha256 is O(1) — the common case after
// a token presents.
type KeyStore interface {
	// Lookup returns the ClientKey matching the supplied hex sha256, or
	// (zero, false) if none. Implementations are expected to be safe
	// for concurrent use.
	Lookup(hashHex string) (ClientKey, bool)
}

// ClientKeyValidator implements Validator for the per-caller client key
// pattern: each key is a Secret in the agent's namespace; the facade
// hashes the incoming bearer and looks the hash up in a KeyStore. Hash
// comparison is constant-time even though we're comparing hex strings —
// timing attacks against hex compares are practical.
type ClientKeyValidator struct {
	store              KeyStore
	defaultRole        string
	trustEndUserHeader bool
	now                func() time.Time // injectable for tests; defaults to time.Now
}

// ClientKeyOption tunes a ClientKeyValidator.
type ClientKeyOption func(*ClientKeyValidator)

// WithClientKeyDefaultRole sets the role applied to keys whose Secret
// data carries no claims. Mirrors the CRD's ClientKeysAuth.defaultRole
// field.
func WithClientKeyDefaultRole(role string) ClientKeyOption {
	return func(v *ClientKeyValidator) { v.defaultRole = role }
}

// WithClientKeyTrustEndUserHeader honours X-End-User-Id when populating
// Identity.EndUser. Same security trade-off as sharedToken — see the
// design doc.
func WithClientKeyTrustEndUserHeader(trust bool) ClientKeyOption {
	return func(v *ClientKeyValidator) { v.trustEndUserHeader = trust }
}

// WithClientKeyClock injects a clock for expiry tests.
func WithClientKeyClock(now func() time.Time) ClientKeyOption {
	return func(v *ClientKeyValidator) { v.now = now }
}

// NewClientKeyValidator constructs a validator backed by the supplied
// KeyStore.
func NewClientKeyValidator(store KeyStore, opts ...ClientKeyOption) *ClientKeyValidator {
	v := &ClientKeyValidator{
		store:       store,
		defaultRole: policy.RoleViewer,
		now:         time.Now,
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// HashToken returns the lowercase-hex sha256 of a raw token. Exposed so
// cmd/agent's KeyStore implementation can populate the cache from
// Secret data using the same hash function the validator uses on the
// inbound side.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Validate implements Validator. ErrNoCredential when no Bearer header, or
// when the bearer isn't a known key (an unknown opaque bearer isn't a
// client-key-style credential, so the chain falls through); ErrExpired
// when a matching key has lapsed.
func (v *ClientKeyValidator) Validate(_ context.Context, r *http.Request) (*policy.AuthenticatedIdentity, error) {
	tokenString, err := extractBearer(r)
	if err != nil {
		return nil, err
	}

	candidate := HashToken(tokenString)
	key, found := v.store.Lookup(candidate)
	if !found {
		// Not one of our keys. An opaque-bearer validator can't tell a wrong
		// client key from a credential of another style (e.g. a mgmt-plane
		// JWT), so fall through rather than short-circuit the chain —
		// otherwise a later validator (mgmt-plane) never gets a chance. See
		// #1620.
		return nil, ErrNoCredential
	}
	// Hash collisions on sha256 are not a thing in practice; the
	// constant-time compare is defence-in-depth against any future
	// store implementation that does prefix-matching or similar.
	if subtle.ConstantTimeCompare([]byte(candidate), []byte(key.HashHex)) != 1 {
		return nil, ErrNoCredential
	}
	if !key.ExpiresAt.IsZero() && !v.now().Before(key.ExpiresAt) {
		return nil, ErrExpired
	}

	claims := key.Claims
	if len(claims) == 0 {
		claims = map[string]string{"role": v.defaultRole}
	}

	subject := key.ID
	endUser := subject
	if v.trustEndUserHeader {
		if h := r.Header.Get(EndUserHeader); h != "" {
			endUser = h
		}
	}

	return &policy.AuthenticatedIdentity{
		Origin:    policy.OriginClientKey,
		Subject:   subject,
		EndUser:   endUser,
		Claims:    claims,
		ExpiresAt: key.ExpiresAt,
	}, nil
}

// StaticKeyStore is the in-memory KeyStore — what cmd/agent constructs
// after listing the agent's labelled Secrets, and what tests use
// directly. Maps hex sha256 → ClientKey; safe for concurrent reads (no
// per-request mutation), but writers must replace the whole map atomically.
type StaticKeyStore struct {
	keys map[string]ClientKey
}

// NewStaticKeyStore returns a KeyStore backed by the supplied map. The
// caller passes ownership of the map — callers must not mutate it after
// construction (use Replace on a future hot-reload-friendly impl).
func NewStaticKeyStore(keys map[string]ClientKey) *StaticKeyStore {
	if keys == nil {
		keys = map[string]ClientKey{}
	}
	return &StaticKeyStore{keys: keys}
}

// Lookup implements KeyStore.
func (s *StaticKeyStore) Lookup(hashHex string) (ClientKey, bool) {
	k, ok := s.keys[hashHex]
	return k, ok
}
