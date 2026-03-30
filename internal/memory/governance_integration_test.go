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
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGovernance_PurposeFiltering(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// Save 3 memories — all default to 'support_continuity'.
	mem1 := &Memory{Type: "fact", Content: "likes Go", Confidence: 0.9, Scope: scope}
	mem2 := &Memory{Type: "fact", Content: "prefers dark mode", Confidence: 0.8, Scope: scope}
	mem3 := &Memory{Type: "preference", Content: "uses vim editor", Confidence: 0.7, Scope: scope}
	require.NoError(t, store.Save(ctx, mem1))
	require.NoError(t, store.Save(ctx, mem2))
	require.NoError(t, store.Save(ctx, mem3))

	// Set different purposes via direct SQL UPDATE.
	_, err := store.Pool().Exec(ctx,
		"UPDATE memory_entities SET purpose = 'personalization' WHERE id = $1", mem2.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx,
		"UPDATE memory_entities SET purpose = 'analytics' WHERE id = $1", mem3.ID)
	require.NoError(t, err)

	// Purpose filtering was removed when migrating to PromptKit types
	// (RetrieveOptions no longer has a Purpose field). Without purpose
	// filtering all memories are returned.
	results, err := store.Retrieve(ctx, scope, "", RetrieveOptions{})
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestGovernance_RetentionExpiry(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// Save a memory with expires_at in the past.
	pastTime := time.Now().Add(-1 * time.Second)
	expired := &Memory{
		Type:       "fact",
		Content:    "this should be expired",
		Confidence: 0.9,
		Scope:      scope,
		ExpiresAt:  &pastTime,
	}
	require.NoError(t, store.Save(ctx, expired))

	// Save a memory with expires_at in the future.
	futureTime := time.Now().Add(1 * time.Hour)
	alive := &Memory{
		Type:       "fact",
		Content:    "this should survive",
		Confidence: 0.9,
		Scope:      scope,
		ExpiresAt:  &futureTime,
	}
	require.NoError(t, store.Save(ctx, alive))

	// Verify both exist before expiry.
	results, err := store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	require.Len(t, results, 2)

	// Run expiry.
	count, err := store.ExpireMemories(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Verify expired memory is gone.
	results, err = store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, alive.ID, results[0].ID)
}

func TestGovernance_CacheInvalidation(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// Set up miniredis-backed CachedStore.
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cached := NewCachedStore(store, rdb, time.Minute, logr.Discard())

	// Save a memory via CachedStore (populates store, bumps version).
	mem := &Memory{
		Type:       "fact",
		Content:    "cached memory",
		Confidence: 0.9,
		Scope:      scope,
	}
	require.NoError(t, cached.Save(ctx, mem))

	// First List — cache miss, fetches from store, populates cache.
	results, err := cached.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1)

	// Verify cache has keys (version key + data key).
	keys := mr.Keys()
	assert.NotEmpty(t, keys, "cache should have keys after first List")

	// Second List — cache hit, same result.
	results, err = cached.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1, "cache hit should return same result")

	// Delete via CachedStore (bumps version, invalidating old cache keys).
	require.NoError(t, cached.Delete(ctx, scope, mem.ID))

	// Third List — cache miss because version changed, returns empty.
	results, err = cached.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, results, "after delete, cache-invalidated list should return empty")
}

func TestGovernance_ExportAll(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	types := []string{"fact", "preference", "goal", "skill", "context"}

	// Save 5 memories with different types.
	mems := make([]*Memory, 5)
	for i := range 5 {
		mem := &Memory{
			Type:       types[i],
			Content:    fmt.Sprintf("export memory %d", i),
			Confidence: 0.9,
			Scope:      scope,
		}
		require.NoError(t, store.Save(ctx, mem))
		mems[i] = mem
	}

	// Set different purposes on some memories via direct SQL.
	_, err := store.Pool().Exec(ctx,
		"UPDATE memory_entities SET purpose = 'personalization' WHERE id = $1", mems[1].ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx,
		"UPDATE memory_entities SET purpose = 'analytics' WHERE id = $1", mems[3].ID)
	require.NoError(t, err)

	// ExportAll — should return all 5 regardless of purpose.
	results, err := store.ExportAll(ctx, scope)
	require.NoError(t, err)
	assert.Len(t, results, 5, "ExportAll should return all 5 memories")

	// Soft-delete one memory.
	require.NoError(t, store.Delete(ctx, scope, mems[2].ID))

	// ExportAll again — should return only 4 (soft-deleted excluded).
	results, err = store.ExportAll(ctx, scope)
	require.NoError(t, err)
	assert.Len(t, results, 4, "ExportAll should exclude soft-deleted memories")

	// Verify the deleted memory is not in the results.
	for _, r := range results {
		assert.NotEqual(t, mems[2].ID, r.ID, "deleted memory should not appear in export")
	}
}
