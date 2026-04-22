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

package main

import (
	"context"
	"crypto/rsa"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/altairalabs/omnia/internal/facade/auth"
)

// OIDCJWKSSecretSuffix is appended to `agent-<name>` to form the
// conventional per-agent Secret name that caches the customer IdP's
// JWKS. The AgentRuntime controller maintains the Secret in PR 2d-2
// (not in this PR — operators can populate it manually for now).
const OIDCJWKSSecretSuffix = "-oidc-jwks"

// OIDCJWKSDataKey is the data key inside the Secret holding the raw
// JWKS JSON blob (verbatim from the issuer's jwks_uri). Stable contract
// shared with the reconciler.
const OIDCJWKSDataKey = "jwks.json"

// SecretBackedJWKSStore implements auth.KeySupplier by reading the
// agent's JWKS Secret at startup and refreshing periodically. Rotation
// on the IdP side propagates within the refresh interval.
//
// Missing kid at lookup time does NOT trigger an on-demand refresh in
// MVP — we return (nil, false) and let the validator reject the token.
// PR 2d-2 can add facade-side annotation bumps to nudge the controller
// for the "new kid appeared mid-session" case.
type SecretBackedJWKSStore struct {
	client     client.Client
	namespace  string
	secretName string
	refresh    time.Duration
	log        logr.Logger
	now        func() time.Time

	mu          sync.RWMutex
	set         *auth.KeySet
	lastRefresh time.Time

	stopCh   chan struct{}
	stopOnce sync.Once
}

// SecretBackedJWKSStoreOption tunes the store.
type SecretBackedJWKSStoreOption func(*SecretBackedJWKSStore)

// WithJWKSRefreshInterval overrides the default refresh cadence.
// Defaults to 5 minutes — tight enough to catch manual key rotation
// without hammering the k8s API.
func WithJWKSRefreshInterval(d time.Duration) SecretBackedJWKSStoreOption {
	return func(s *SecretBackedJWKSStore) { s.refresh = d }
}

// WithJWKSClock injects a clock for tests.
func WithJWKSClock(now func() time.Time) SecretBackedJWKSStoreOption {
	return func(s *SecretBackedJWKSStore) { s.now = now }
}

// NewSecretBackedJWKSStore loads the initial JWKS synchronously and
// returns a store that refreshes in the background. Initial-load
// errors are fatal — the facade refuses to admit OIDC tokens against
// a missing JWKS rather than silently 401ing every request.
func NewSecretBackedJWKSStore(
	ctx context.Context,
	c client.Client,
	namespace, secretName string,
	log logr.Logger,
	opts ...SecretBackedJWKSStoreOption,
) (*SecretBackedJWKSStore, error) {
	s := &SecretBackedJWKSStore{
		client:     c,
		namespace:  namespace,
		secretName: secretName,
		refresh:    5 * time.Minute,
		log:        log.WithName("oidc-jwks-store"),
		now:        time.Now,
		stopCh:     make(chan struct{}),
	}
	for _, opt := range opts {
		opt(s)
	}
	if err := s.loadOnce(ctx); err != nil {
		return nil, fmt.Errorf("initial JWKS load: %w", err)
	}
	go s.refreshLoop()
	return s, nil
}

// Lookup implements auth.KeySupplier by delegating to the cached
// KeySet. Safe for concurrent reads; a concurrent refresh swaps the
// whole set under the write lock.
func (s *SecretBackedJWKSStore) Lookup(kid string) (*rsa.PublicKey, bool) {
	s.mu.RLock()
	set := s.set
	s.mu.RUnlock()
	if set == nil {
		return nil, false
	}
	return set.Lookup(kid)
}

// Stop ends the background refresh loop. Idempotent.
func (s *SecretBackedJWKSStore) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

// loadOnce reads the JWKS Secret, parses the blob into an auth.KeySet,
// and swaps it in atomically.
func (s *SecretBackedJWKSStore) loadOnce(ctx context.Context) error {
	secret := &corev1.Secret{}
	err := s.client.Get(ctx, types.NamespacedName{
		Namespace: s.namespace,
		Name:      s.secretName,
	}, secret)
	if err != nil {
		return fmt.Errorf("get JWKS secret %s/%s: %w", s.namespace, s.secretName, err)
	}
	raw, ok := secret.Data[OIDCJWKSDataKey]
	if !ok || len(raw) == 0 {
		return fmt.Errorf("JWKS secret %s/%s missing %q data key",
			s.namespace, s.secretName, OIDCJWKSDataKey)
	}
	set, err := auth.NewKeySetFromJSON(raw)
	if err != nil {
		return fmt.Errorf("parse JWKS blob: %w", err)
	}

	s.mu.Lock()
	s.set = set
	s.lastRefresh = s.now()
	s.mu.Unlock()
	s.log.V(1).Info("JWKS store refreshed")
	return nil
}

// refreshLoop runs until Stop. Errors log and keep the previous
// snapshot — better to serve slightly-stale keys than universally
// reject while the API server blips.
func (s *SecretBackedJWKSStore) refreshLoop() {
	ticker := time.NewTicker(s.refresh)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := s.loadOnce(ctx); err != nil {
				s.log.Error(err, "JWKS refresh failed; keeping previous snapshot")
			}
			cancel()
		}
	}
}

// OIDCJWKSSecretNameFor derives the conventional per-agent JWKS Secret
// name. Kept here (not in controller/constants.go) so cmd/agent
// doesn't import controller internals; the reconciler PR will mirror
// the same helper on its side.
func OIDCJWKSSecretNameFor(agentName string) string {
	return fmt.Sprintf("agent-%s%s", agentName, OIDCJWKSSecretSuffix)
}
