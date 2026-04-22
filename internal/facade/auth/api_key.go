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

// APIKey is the in-memory representation of a single agent API key. The
// value is a sha256 hash of the raw bearer token; the validator never
// stores the raw value. Built by cmd/agent's KeyStore from the labelled
// Secret data; defined here so the validator can be unit-tested against
// a synthetic []APIKey slice.
type APIKey struct {
	// ID is a stable identifier for the key (the Secret name's suffix
	// after `agent-<agent>-apikey-`). Surfaces as Identity.Subject so
	// audit and ToolPolicy rules can distinguish callers.
	ID string

	// HashHex is the lowercase-hex sha256 of the raw token bytes. The
	// validator compares the hex representation of the incoming token's
	// hash against this with a constant-time compare.
	HashHex string

	// Role is the caller's role for this key. One of policy.RoleAdmin /
	// policy.RoleEditor / policy.RoleViewer. Falls back to APIKeysAuth's
	// defaultRole at the cmd/agent layer if unset on the Secret.
	Role string

	// ExpiresAt is the absolute time after which the key is no longer
	// valid. Zero value means "no expiry" — the operator opts in by
	// setting an expiresAt field on the Secret.
	ExpiresAt time.Time
}

// KeyStore is the lookup interface APIKeyValidator depends on. cmd/agent
// supplies a Secret-backed implementation; tests can swap in a
// hand-built map. Lookup by hex sha256 is O(1) — the common case after
// a token presents.
type KeyStore interface {
	// Lookup returns the APIKey matching the supplied hex sha256, or
	// (zero, false) if none. Implementations are expected to be safe
	// for concurrent use.
	Lookup(hashHex string) (APIKey, bool)
}

// APIKeyValidator implements Validator for the per-caller API key
// pattern: each key is a Secret in the agent's namespace; the facade
// hashes the incoming bearer and looks the hash up in a KeyStore. Hash
// comparison is constant-time even though we're comparing hex strings —
// timing attacks against hex compares are practical.
type APIKeyValidator struct {
	store              KeyStore
	defaultRole        string
	trustEndUserHeader bool
	now                func() time.Time // injectable for tests; defaults to time.Now
}

// APIKeyOption tunes an APIKeyValidator.
type APIKeyOption func(*APIKeyValidator)

// WithAPIKeyDefaultRole sets the role applied to keys whose Secret data
// does not carry one. Mirrors the CRD's APIKeysAuth.defaultRole field.
func WithAPIKeyDefaultRole(role string) APIKeyOption {
	return func(v *APIKeyValidator) { v.defaultRole = role }
}

// WithAPIKeyTrustEndUserHeader honours X-End-User-Id when populating
// Identity.EndUser. Same security trade-off as sharedToken — see the
// design doc.
func WithAPIKeyTrustEndUserHeader(trust bool) APIKeyOption {
	return func(v *APIKeyValidator) { v.trustEndUserHeader = trust }
}

// WithAPIKeyClock injects a clock for expiry tests.
func WithAPIKeyClock(now func() time.Time) APIKeyOption {
	return func(v *APIKeyValidator) { v.now = now }
}

// NewAPIKeyValidator constructs a validator backed by the supplied
// KeyStore.
func NewAPIKeyValidator(store KeyStore, opts ...APIKeyOption) *APIKeyValidator {
	v := &APIKeyValidator{
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

// Validate implements Validator. ErrNoCredential when no Bearer header;
// ErrInvalidCredential when the hash isn't in the store; ErrExpired
// when a matching key has lapsed.
func (v *APIKeyValidator) Validate(_ context.Context, r *http.Request) (*policy.AuthenticatedIdentity, error) {
	tokenString, err := extractBearer(r)
	if err != nil {
		return nil, err
	}

	candidate := HashToken(tokenString)
	key, found := v.store.Lookup(candidate)
	if !found {
		return nil, ErrInvalidCredential
	}
	// Hash collisions on sha256 are not a thing in practice; the
	// constant-time compare is defence-in-depth against any future
	// store implementation that does prefix-matching or similar.
	if subtle.ConstantTimeCompare([]byte(candidate), []byte(key.HashHex)) != 1 {
		return nil, ErrInvalidCredential
	}
	if !key.ExpiresAt.IsZero() && !v.now().Before(key.ExpiresAt) {
		return nil, ErrExpired
	}

	role := key.Role
	if role == "" {
		role = v.defaultRole
	}

	subject := key.ID
	endUser := subject
	if v.trustEndUserHeader {
		if h := r.Header.Get(EndUserHeader); h != "" {
			endUser = h
		}
	}

	return &policy.AuthenticatedIdentity{
		Origin:    policy.OriginAPIKey,
		Subject:   subject,
		EndUser:   endUser,
		Role:      role,
		ExpiresAt: key.ExpiresAt,
	}, nil
}

// StaticKeyStore is the in-memory KeyStore — what cmd/agent constructs
// after listing the agent's labelled Secrets, and what tests use
// directly. Maps hex sha256 → APIKey; safe for concurrent reads (no
// per-request mutation), but writers must replace the whole map atomically.
type StaticKeyStore struct {
	keys map[string]APIKey
}

// NewStaticKeyStore returns a KeyStore backed by the supplied map. The
// caller passes ownership of the map — callers must not mutate it after
// construction (use Replace on a future hot-reload-friendly impl).
func NewStaticKeyStore(keys map[string]APIKey) *StaticKeyStore {
	if keys == nil {
		keys = map[string]APIKey{}
	}
	return &StaticKeyStore{keys: keys}
}

// Lookup implements KeyStore.
func (s *StaticKeyStore) Lookup(hashHex string) (APIKey, bool) {
	k, ok := s.keys[hashHex]
	return k, ok
}
