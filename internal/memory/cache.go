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

package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/redis/go-redis/v9"
)

// Compile-time interface check.
var _ Store = (*CachedStore)(nil)

// Cache key constants to avoid duplication (SonarCloud S1192).
const (
	cacheKeyPrefix   = "mem:"
	cacheKeyVersion  = ":version"
	cacheKeyRetrieve = ":retrieve:"
	cacheKeyList     = ":list:"
)

// CachedStore wraps a Store with a Redis cache layer.
// Cache keys are scoped by workspace + user to prevent cross-tenant leakage.
// Invalidation uses a scope-version key: on every write the version is incremented,
// so all previously cached keys (which embed the old version) naturally become stale
// and are never returned. Old entries expire via TTL.
type CachedStore struct {
	inner Store
	redis *redis.Client
	ttl   time.Duration
	log   logr.Logger
}

// NewCachedStore creates a CachedStore that wraps inner with a Redis cache.
func NewCachedStore(inner Store, rdb *redis.Client, ttl time.Duration, log logr.Logger) *CachedStore {
	return &CachedStore{
		inner: inner,
		redis: rdb,
		ttl:   ttl,
		log:   log,
	}
}

// Save delegates to the inner store then invalidates the cache for the scope.
func (c *CachedStore) Save(ctx context.Context, mem *Memory) error {
	if err := c.inner.Save(ctx, mem); err != nil {
		return err
	}
	c.bumpVersion(ctx, mem.Scope)
	return nil
}

// SaveWithResult delegates to the inner store and invalidates the
// cache. The inner store's dedup result is passed through unchanged
// so the agent sees auto_superseded / potential_duplicates info.
func (c *CachedStore) SaveWithResult(ctx context.Context, mem *Memory) (*SaveResult, error) {
	res, err := c.inner.SaveWithResult(ctx, mem)
	if err != nil {
		return nil, err
	}
	c.bumpVersion(ctx, mem.Scope)
	return res, nil
}

// Retrieve returns cached results when available, falling back to the inner store on miss or Redis error.
func (c *CachedStore) Retrieve(ctx context.Context, scope map[string]string, query string, opts RetrieveOptions) ([]*Memory, error) {
	key := c.retrieveKey(ctx, scope, query, opts)
	if key != "" {
		if mems, ok := c.cacheGet(ctx, key); ok {
			return mems, nil
		}
	}

	mems, err := c.inner.Retrieve(ctx, scope, query, opts)
	if err != nil {
		return nil, err
	}

	if key != "" {
		c.cacheSet(ctx, key, mems)
	}
	return mems, nil
}

// RetrieveMultiTier delegates to the inner store without caching. The
// multi-tier result shape carries per-row tier tags and scores that are cheap
// to recompute; adding a cache layer here would complicate invalidation for
// modest savings. Revisit if retrieval becomes hot.
func (c *CachedStore) RetrieveMultiTier(ctx context.Context, req MultiTierRequest) (*MultiTierResult, error) {
	return c.inner.RetrieveMultiTier(ctx, req)
}

// SaveInstitutional delegates to the inner store then invalidates the
// workspace-scoped cache so any cached list/search sees the new row.
func (c *CachedStore) SaveInstitutional(ctx context.Context, mem *Memory) error {
	if err := c.inner.SaveInstitutional(ctx, mem); err != nil {
		return err
	}
	c.bumpVersion(ctx, map[string]string{ScopeWorkspaceID: mem.Scope[ScopeWorkspaceID]})
	return nil
}

// ListInstitutional delegates to the inner store without caching. The admin
// list path is infrequent and needs to reflect writes immediately.
func (c *CachedStore) ListInstitutional(ctx context.Context, workspaceID string, opts ListOptions) ([]*Memory, error) {
	return c.inner.ListInstitutional(ctx, workspaceID, opts)
}

// DeleteInstitutional delegates to the inner store then invalidates the
// workspace-scoped cache.
func (c *CachedStore) DeleteInstitutional(ctx context.Context, workspaceID, memoryID string) error {
	if err := c.inner.DeleteInstitutional(ctx, workspaceID, memoryID); err != nil {
		return err
	}
	c.bumpVersion(ctx, map[string]string{ScopeWorkspaceID: workspaceID})
	return nil
}

// SaveAgentScoped delegates to the inner store then invalidates the
// (workspace, agent) cache.
func (c *CachedStore) SaveAgentScoped(ctx context.Context, mem *Memory) error {
	if err := c.inner.SaveAgentScoped(ctx, mem); err != nil {
		return err
	}
	c.bumpVersion(ctx, map[string]string{
		ScopeWorkspaceID: mem.Scope[ScopeWorkspaceID],
		ScopeAgentID:     mem.Scope[ScopeAgentID],
	})
	return nil
}

// ListAgentScoped delegates to the inner store without caching — the admin
// list path is infrequent and must reflect writes immediately.
func (c *CachedStore) ListAgentScoped(ctx context.Context, workspaceID, agentID string, opts ListOptions) ([]*Memory, error) {
	return c.inner.ListAgentScoped(ctx, workspaceID, agentID, opts)
}

// DeleteAgentScoped delegates to the inner store then invalidates the
// (workspace, agent) cache.
func (c *CachedStore) DeleteAgentScoped(ctx context.Context, workspaceID, agentID, memoryID string) error {
	if err := c.inner.DeleteAgentScoped(ctx, workspaceID, agentID, memoryID); err != nil {
		return err
	}
	c.bumpVersion(ctx, map[string]string{
		ScopeWorkspaceID: workspaceID,
		ScopeAgentID:     agentID,
	})
	return nil
}

// FindCompactionCandidates delegates to the inner store without caching.
// Compaction scans are infrequent and need live data — a cached bucket
// list would defeat the purpose.
func (c *CachedStore) FindCompactionCandidates(ctx context.Context, opts FindCompactionCandidatesOptions) ([]CompactionCandidate, error) {
	return c.inner.FindCompactionCandidates(ctx, opts)
}

// SaveCompactionSummary delegates to the inner store then invalidates the
// (workspace, user, agent) cache so post-compaction retrieval reflects
// the new supersede chain without serving stale rows.
func (c *CachedStore) SaveCompactionSummary(ctx context.Context, summary CompactionSummary) (string, error) {
	id, err := c.inner.SaveCompactionSummary(ctx, summary)
	if err != nil {
		return "", err
	}
	scope := map[string]string{ScopeWorkspaceID: summary.WorkspaceID}
	if summary.UserID != "" {
		scope[ScopeUserID] = summary.UserID
	}
	if summary.AgentID != "" {
		scope[ScopeAgentID] = summary.AgentID
	}
	c.bumpVersion(ctx, scope)
	return id, nil
}

// List returns cached results when available, falling back to the inner store on miss or Redis error.
func (c *CachedStore) List(ctx context.Context, scope map[string]string, opts ListOptions) ([]*Memory, error) {
	key := c.listKey(ctx, scope, opts)
	if key != "" {
		if mems, ok := c.cacheGet(ctx, key); ok {
			return mems, nil
		}
	}

	mems, err := c.inner.List(ctx, scope, opts)
	if err != nil {
		return nil, err
	}

	if key != "" {
		c.cacheSet(ctx, key, mems)
	}
	return mems, nil
}

// Delete delegates to the inner store then invalidates the cache for the scope.
func (c *CachedStore) Delete(ctx context.Context, scope map[string]string, memoryID string) error {
	if err := c.inner.Delete(ctx, scope, memoryID); err != nil {
		return err
	}
	c.bumpVersion(ctx, scope)
	return nil
}

// DeleteAll delegates to the inner store then invalidates the cache for the scope.
func (c *CachedStore) DeleteAll(ctx context.Context, scope map[string]string) error {
	if err := c.inner.DeleteAll(ctx, scope); err != nil {
		return err
	}
	c.bumpVersion(ctx, scope)
	return nil
}

// ExportAll delegates to the inner store. Results are not cached (DSAR export is infrequent).
func (c *CachedStore) ExportAll(ctx context.Context, scope map[string]string) ([]*Memory, error) {
	return c.inner.ExportAll(ctx, scope)
}

// BatchDelete delegates to the inner store then invalidates the cache for the scope.
func (c *CachedStore) BatchDelete(ctx context.Context, scope map[string]string, limit int) (int, error) {
	n, err := c.inner.BatchDelete(ctx, scope, limit)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		c.bumpVersion(ctx, scope)
	}
	return n, nil
}

// --- cache helpers -------------------------------------------------------------

// versionKey returns the Redis key that tracks the invalidation version for a scope.
func versionKey(sh string) string {
	return cacheKeyPrefix + sh + cacheKeyVersion
}

// getVersion fetches the current cache version for a scope hash.
// Returns "0" when no version key exists yet, and "" on Redis error.
func (c *CachedStore) getVersion(ctx context.Context, sh string) string {
	v, err := c.redis.Get(ctx, versionKey(sh)).Result()
	if err == nil {
		return v
	}
	if errors.Is(err, redis.Nil) {
		return "0"
	}
	c.log.V(1).Info("cache version get failed", "scopeHash", sh, "error", err)
	return ""
}

// bumpVersion increments the scope version, invalidating all cached keys for that scope.
func (c *CachedStore) bumpVersion(ctx context.Context, scope map[string]string) {
	sh := scopeHash(scope)
	if err := c.redis.Incr(ctx, versionKey(sh)).Err(); err != nil {
		c.log.V(1).Info("cache version bump failed", "scopeHash", sh, "error", err)
	}
}

// retrieveKey builds a versioned cache key for a Retrieve call.
// Returns "" if the version cannot be fetched (Redis down).
func (c *CachedStore) retrieveKey(ctx context.Context, scope map[string]string, query string, opts RetrieveOptions) string {
	sh := scopeHash(scope)
	v := c.getVersion(ctx, sh)
	if v == "" {
		return ""
	}
	qh := shortHash(fmt.Sprintf("%s|%v|%d|%f", query, opts.Types, opts.Limit, opts.MinConfidence))
	return cacheKeyPrefix + sh + ":v" + v + cacheKeyRetrieve + qh
}

// listKey builds a versioned cache key for a List call.
// Returns "" if the version cannot be fetched (Redis down).
func (c *CachedStore) listKey(ctx context.Context, scope map[string]string, opts ListOptions) string {
	sh := scopeHash(scope)
	v := c.getVersion(ctx, sh)
	if v == "" {
		return ""
	}
	descriptor := fmt.Sprintf("%v:%d:%d", opts.Types, opts.Limit, opts.Offset)
	return cacheKeyPrefix + sh + ":v" + v + cacheKeyList + shortHash(descriptor)
}

// cacheGet fetches a []*Memory slice from Redis. Returns (nil, false) on miss or error.
func (c *CachedStore) cacheGet(ctx context.Context, key string) ([]*Memory, bool) {
	data, err := c.redis.Get(ctx, key).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			c.log.V(1).Info("cache get failed", "key", key, "error", err)
		}
		return nil, false
	}

	var mems []*Memory
	if err := json.Unmarshal(data, &mems); err != nil {
		c.log.V(1).Info("cache unmarshal failed", "key", key, "error", err)
		return nil, false
	}
	return mems, true
}

// cacheSet marshals a []*Memory slice and stores it in Redis with the configured TTL.
func (c *CachedStore) cacheSet(ctx context.Context, key string, mems []*Memory) {
	data, err := json.Marshal(mems)
	if err != nil {
		c.log.V(1).Info("cache marshal failed", "key", key, "error", err)
		return
	}
	if err := c.redis.Set(ctx, key, data, c.ttl).Err(); err != nil {
		c.log.V(1).Info("cache set failed", "key", key, "error", err)
	}
}

// --- key helpers ---------------------------------------------------------------

// scopeHash returns a short deterministic hex hash of the sorted scope map.
func scopeHash(scope map[string]string) string {
	keys := make([]string, 0, len(scope))
	for k, v := range scope {
		keys = append(keys, k+"="+v)
	}
	sort.Strings(keys)
	h := sha256.Sum256([]byte(strings.Join(keys, "&")))
	return hex.EncodeToString(h[:8])
}

// shortHash returns the first 8 bytes of a SHA-256 hash as hex.
func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}
