/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/pkg/logging"
)

const cacheKeyPrefix = "prefs:"

// KVCache is the minimal key/value surface the preference cache needs.
// Backed by Redis in production (ee/cmd/privacy-api) and a map in tests.
type KVCache interface {
	Get(ctx context.Context, key string) (val string, found bool, err error)
	Set(ctx context.Context, key, val string, ttl time.Duration) error
	Del(ctx context.Context, key string) error
}

type preferencesReadWriter interface {
	PreferencesStore
	ConsentSource
}

// CachedPreferencesStore decorates a preferences store with a best-effort warm
// cache keyed by pseudonymized user id. Reads populate the cache; opt-out writes
// bust it. The cache is best-effort: any cache error falls through to the inner
// store and never fails a read.
//
// LIMITATION: consent-grant writes happen via ConsentHandler against the raw
// concrete store and do NOT bust this cache. The TTL bounds that staleness;
// consent grants change rarely. See #1642 design §11.
type CachedPreferencesStore struct {
	inner preferencesReadWriter
	kv    KVCache
	ttl   time.Duration
	log   logr.Logger
}

// NewCachedPreferencesStore creates a new CachedPreferencesStore wrapping inner
// with a best-effort KV cache. ttl controls how long cached entries are retained.
func NewCachedPreferencesStore(
	inner preferencesReadWriter, kv KVCache, ttl time.Duration, log logr.Logger,
) *CachedPreferencesStore {
	return &CachedPreferencesStore{inner: inner, kv: kv, ttl: ttl, log: log.WithName("prefs-cache")}
}

// Compile-time interface checks.
var _ PreferencesStore = (*CachedPreferencesStore)(nil)
var _ ConsentSource = (*CachedPreferencesStore)(nil)

// GetPreferences returns cached preferences when available, falling through to
// the inner store on cache miss or error. Not-found results are never cached.
func (c *CachedPreferencesStore) GetPreferences(ctx context.Context, userID string) (*Preferences, error) {
	key := cacheKeyPrefix + userID
	if raw, found, err := c.kv.Get(ctx, key); err == nil && found {
		var p Preferences
		if jsonErr := json.Unmarshal([]byte(raw), &p); jsonErr == nil {
			return &p, nil
		}
		c.log.V(1).Info("cache entry corrupt, falling through", "userHash", logging.HashID(userID))
	} else if err != nil {
		c.log.V(1).Info("cache get failed, falling through", "reason", err.Error())
	}

	p, err := c.inner.GetPreferences(ctx, userID)
	if err != nil {
		return nil, err // includes ErrPreferencesNotFound — intentionally not cached
	}
	if raw, mErr := json.Marshal(p); mErr == nil {
		if sErr := c.kv.Set(ctx, key, string(raw), c.ttl); sErr != nil {
			c.log.V(1).Info("cache set failed", "reason", sErr.Error())
		}
	}
	return p, nil
}

// GetConsentGrants returns the consent grants from the cached preferences.
// Returns an empty slice (not an error) when the user has no preferences row,
// matching the contract of PreferencesPostgresStore.GetConsentGrants.
func (c *CachedPreferencesStore) GetConsentGrants(ctx context.Context, userID string) ([]ConsentCategory, error) {
	p, err := c.GetPreferences(ctx, userID)
	if errors.Is(err, ErrPreferencesNotFound) {
		return []ConsentCategory{}, nil
	}
	if err != nil {
		return nil, err
	}
	return p.ConsentGrants, nil
}

// SetOptOut delegates to inner and busts the cache entry on success.
func (c *CachedPreferencesStore) SetOptOut(ctx context.Context, userID, scope, target string) error {
	if err := c.inner.SetOptOut(ctx, userID, scope, target); err != nil {
		return err
	}
	c.bust(ctx, userID)
	return nil
}

// RemoveOptOut delegates to inner and busts the cache entry on success.
func (c *CachedPreferencesStore) RemoveOptOut(ctx context.Context, userID, scope, target string) error {
	if err := c.inner.RemoveOptOut(ctx, userID, scope, target); err != nil {
		return err
	}
	c.bust(ctx, userID)
	return nil
}

func (c *CachedPreferencesStore) bust(ctx context.Context, userID string) {
	if err := c.kv.Del(ctx, cacheKeyPrefix+userID); err != nil {
		c.log.V(1).Info("cache bust failed", "reason", err.Error())
	}
}
