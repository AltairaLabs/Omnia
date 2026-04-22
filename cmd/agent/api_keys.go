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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/altairalabs/omnia/internal/facade/auth"
)

// Secret labels + data keys for agent-scoped API keys. Mirrored on the
// dashboard side when the CRUD UI lands; defined here so cmd/agent can
// list-by-label without importing the controller package.
const (
	// LabelCredentialKind tags every agent-scoped credential Secret. Same
	// label is used for sharedToken / oidc-jwks Secrets so a single
	// list-by-label can find them all if needed later; we filter further
	// by LabelCredentialKindValue.
	LabelCredentialKind = "omnia.altairalabs.ai/credential-kind"
	// LabelCredentialKindAgentAPIKey identifies the api-keys flavour.
	LabelCredentialKindAgentAPIKey = "agent-api-key"
	// LabelAgent narrows the list to keys for THIS agent only — many
	// agents may share a namespace, but each pod must only see its own
	// keys.
	LabelAgent = "omnia.altairalabs.ai/agent"

	// Data keys inside the API-key Secret. The hash and scope formats
	// are stable contracts shared between the dashboard CRUD endpoint
	// and the facade KeyStore.
	APIKeyDataKeyHash      = "keyHash"
	APIKeyDataKeyScopes    = "scopes"
	APIKeyDataKeyExpiresAt = "expiresAt"
)

// apiKeySecretSuffix is the prefix the dashboard CRUD endpoint uses when
// minting a new key Secret (`agent-<agent>-apikey-<id>`). The KeyStore
// derives APIKey.ID from the suffix after the second `-apikey-`. Stable
// and parsed by both sides, so the dashboard can list-by-label and link
// the rendered key list back to the Secret.
const apiKeySecretPrefix = "-apikey-"

// apiKeyScopes is the JSON shape stored under data.scopes. Kept tight —
// per-tool scopes land in a future iteration of the design.
type apiKeyScopes struct {
	Role string `json:"role,omitempty"`
}

// SecretBackedKeyStore is the cmd/agent implementation of
// auth.KeyStore. It maintains an atomically-swapped map keyed by
// hex(sha256(rawKey)) → APIKey, refreshed from the labelled Secrets in
// the agent's namespace on a configurable interval.
//
// Refresh strategy: list-by-label (Get by selector), parse each Secret,
// build a fresh map, swap atomically. Cheap enough at the expected per-
// agent fan-out (tens of keys at most) that an informer's incremental
// updates aren't worth the extra plumbing for PR 2c.
type SecretBackedKeyStore struct {
	client       client.Client
	namespace    string
	agentName    string
	refresh      time.Duration
	log          logr.Logger
	now          func() time.Time
	mu           sync.RWMutex
	keys         map[string]auth.APIKey
	stopCh       chan struct{}
	stopOnce     sync.Once
	lastRefresh  time.Time
	refreshError error
}

// SecretBackedKeyStoreOption tunes the store. All optional.
type SecretBackedKeyStoreOption func(*SecretBackedKeyStore)

// WithKeyStoreRefreshInterval sets how often the store re-lists matching
// Secrets. Defaults to 30 seconds — fast enough to feel responsive when
// an admin revokes a key in the dashboard, slow enough to keep API
// pressure low at scale.
func WithKeyStoreRefreshInterval(d time.Duration) SecretBackedKeyStoreOption {
	return func(s *SecretBackedKeyStore) { s.refresh = d }
}

// WithKeyStoreClock injects a clock for tests.
func WithKeyStoreClock(now func() time.Time) SecretBackedKeyStoreOption {
	return func(s *SecretBackedKeyStore) { s.now = now }
}

// NewSecretBackedKeyStore loads the initial key set synchronously, then
// returns a store that periodically re-lists in the background. Returns
// an error on the initial load failure (operator misconfig surfaces at
// pod startup); a periodic refresh failure later only logs and keeps
// the previous snapshot.
func NewSecretBackedKeyStore(
	ctx context.Context,
	c client.Client,
	namespace, agentName string,
	log logr.Logger,
	opts ...SecretBackedKeyStoreOption,
) (*SecretBackedKeyStore, error) {
	s := &SecretBackedKeyStore{
		client:    c,
		namespace: namespace,
		agentName: agentName,
		refresh:   30 * time.Second,
		log:       log.WithName("apikey-store"),
		now:       time.Now,
		stopCh:    make(chan struct{}),
		keys:      map[string]auth.APIKey{},
	}
	for _, opt := range opts {
		opt(s)
	}
	if err := s.loadOnce(ctx); err != nil {
		return nil, fmt.Errorf("initial api-key load: %w", err)
	}
	go s.refreshLoop()
	return s, nil
}

// Lookup implements auth.KeyStore.
func (s *SecretBackedKeyStore) Lookup(hashHex string) (auth.APIKey, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k, ok := s.keys[hashHex]
	return k, ok
}

// Stop ends the background refresh loop. Safe to call more than once.
func (s *SecretBackedKeyStore) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

// loadOnce performs a single list-and-parse cycle, atomically replacing
// the in-memory key set on success. Surface-area inputs for the tests.
func (s *SecretBackedKeyStore) loadOnce(ctx context.Context) error {
	list := &corev1.SecretList{}
	err := s.client.List(ctx, list,
		client.InNamespace(s.namespace),
		client.MatchingLabels{
			LabelCredentialKind: LabelCredentialKindAgentAPIKey,
			LabelAgent:          s.agentName,
		},
	)
	if err != nil {
		return fmt.Errorf("list api-key secrets: %w", err)
	}

	next := make(map[string]auth.APIKey, len(list.Items))
	for i := range list.Items {
		secret := &list.Items[i]
		key, err := parseAPIKeySecret(secret)
		if err != nil {
			s.log.V(1).Info("skipping malformed api-key secret",
				"secret", secret.Name, "reason", err.Error())
			continue
		}
		next[key.HashHex] = key
	}

	s.mu.Lock()
	s.keys = next
	s.lastRefresh = s.now()
	s.refreshError = nil
	s.mu.Unlock()
	s.log.V(1).Info("api-key store refreshed", "count", len(next))
	return nil
}

// refreshLoop runs until Stop. Refresh failures log and keep the
// previous snapshot — better to serve a slightly-stale key set than
// reject every request because the API server hiccupped.
func (s *SecretBackedKeyStore) refreshLoop() {
	ticker := time.NewTicker(s.refresh)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := s.loadOnce(ctx); err != nil {
				s.mu.Lock()
				s.refreshError = err
				s.mu.Unlock()
				s.log.Error(err, "api-key refresh failed; keeping previous snapshot")
			}
			cancel()
		}
	}
}

// parseAPIKeySecret converts a labelled Secret into an APIKey. Returns
// an error (caller skips + logs) when required fields are missing or
// malformed — never panics on bad input.
func parseAPIKeySecret(secret *corev1.Secret) (auth.APIKey, error) {
	if len(secret.Data) == 0 {
		return auth.APIKey{}, fmt.Errorf("empty data")
	}

	hashRaw, ok := secret.Data[APIKeyDataKeyHash]
	if !ok || len(hashRaw) == 0 {
		return auth.APIKey{}, fmt.Errorf("missing %q data key", APIKeyDataKeyHash)
	}
	// The dashboard endpoint stores the binary sha256; we keep the in-
	// memory representation as lowercase hex so APIKeyValidator's
	// constant-time compare aligns with HashToken's output.
	hashHex := hex.EncodeToString(hashRaw)

	id := strings.TrimPrefix(secret.Name, fmt.Sprintf("agent-%s%s",
		secret.Labels[LabelAgent], apiKeySecretPrefix))
	if id == secret.Name {
		// Name didn't match the expected pattern — fall back to the full
		// Secret name as a last resort. ToolPolicy can still distinguish
		// callers by Identity.Subject.
		id = secret.Name
	}

	key := auth.APIKey{
		ID:      id,
		HashHex: hashHex,
	}

	if scopesRaw, ok := secret.Data[APIKeyDataKeyScopes]; ok && len(scopesRaw) > 0 {
		var scopes apiKeyScopes
		if err := json.Unmarshal(scopesRaw, &scopes); err != nil {
			return auth.APIKey{}, fmt.Errorf("parse scopes: %w", err)
		}
		key.Role = scopes.Role
	}

	if expRaw, ok := secret.Data[APIKeyDataKeyExpiresAt]; ok && len(expRaw) > 0 {
		t, err := time.Parse(time.RFC3339, string(expRaw))
		if err != nil {
			return auth.APIKey{}, fmt.Errorf("parse expiresAt: %w", err)
		}
		key.ExpiresAt = t
	}
	return key, nil
}
