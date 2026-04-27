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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunTombstoneGC_TrimsLongChains seeds an entity whose
// supersession chain has more than the threshold inactive
// observations and proves the GC trims it down to the keep window.
// The active observation, the most-recent K inactive ones, and any
// rows on shorter chains are preserved.
func TestRunTombstoneGC_TrimsLongChains(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// Seed a long chain: 12 observations, all but the last inactive.
	mem := &Memory{Type: "fact", Content: "v1", Confidence: 0.9, Scope: scope}
	require.NoError(t, store.Save(ctx, mem))
	for i := 2; i <= 12; i++ {
		next := &Memory{Type: "fact", Content: "vN", Confidence: 0.9, Scope: scope}
		_, err := store.AppendObservationToEntity(ctx, mem.ID, next)
		require.NoError(t, err)
	}

	// Force the inactive rows past the min-age window so the GC
	// will see them as eligible.
	_, err := store.pool.Exec(ctx, `
		UPDATE memory_observations
		SET observed_at = now() - interval '40 days',
		    valid_until = CASE WHEN valid_until IS NOT NULL THEN now() - interval '40 days' ELSE NULL END
		WHERE entity_id = $1 AND valid_until IS NOT NULL`, mem.ID)
	require.NoError(t, err)

	// 11 inactive observations now, threshold > 5, keep 2 → expect
	// 11 - 2 = 9 deleted.
	deleted, err := store.RunTombstoneGC(ctx, TombstoneGCOptions{
		WorkspaceID:        testWorkspace1,
		MinAge:             30 * 24 * time.Hour,
		MinInactiveCount:   5,
		KeepRecentInactive: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(9), deleted)

	// The active observation (no supersede mark, no valid_until in
	// the past) must still be present.
	var activeCount int
	require.NoError(t, store.pool.QueryRow(ctx, `
		SELECT count(*) FROM memory_observations
		WHERE entity_id = $1
		  AND superseded_by IS NULL
		  AND (valid_until IS NULL OR valid_until > now())`, mem.ID).Scan(&activeCount))
	assert.Equal(t, 1, activeCount)
}

// TestRunTombstoneGC_LeavesShortChainsAlone proves the
// MinInactiveCount guard fires: a chain with fewer inactive entries
// than the threshold is not touched even if the entries are old.
func TestRunTombstoneGC_LeavesShortChainsAlone(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	mem := &Memory{Type: "fact", Content: "v1", Confidence: 0.9, Scope: scope}
	require.NoError(t, store.Save(ctx, mem))
	for i := 0; i < 3; i++ {
		_, err := store.AppendObservationToEntity(ctx, mem.ID,
			&Memory{Type: "fact", Content: "vN", Confidence: 0.9, Scope: scope})
		require.NoError(t, err)
	}
	_, err := store.pool.Exec(ctx, `
		UPDATE memory_observations
		SET observed_at = now() - interval '40 days'
		WHERE entity_id = $1`, mem.ID)
	require.NoError(t, err)

	deleted, err := store.RunTombstoneGC(ctx, TombstoneGCOptions{
		WorkspaceID:        testWorkspace1,
		MinAge:             30 * 24 * time.Hour,
		MinInactiveCount:   20, // chain has 3 inactive — below threshold
		KeepRecentInactive: 5,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted, "short chains are left alone")
}

// TestRunTombstoneGC_RespectsMinAge proves recently-superseded rows
// are kept regardless of chain length so brief audit windows
// survive even on hot chains.
func TestRunTombstoneGC_RespectsMinAge(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	mem := &Memory{Type: "fact", Content: "v1", Confidence: 0.9, Scope: scope}
	require.NoError(t, store.Save(ctx, mem))
	for i := 0; i < 25; i++ {
		_, err := store.AppendObservationToEntity(ctx, mem.ID,
			&Memory{Type: "fact", Content: "vN", Confidence: 0.9, Scope: scope})
		require.NoError(t, err)
	}
	// Don't backdate observed_at — every row is fresh.

	deleted, err := store.RunTombstoneGC(ctx, TombstoneGCOptions{
		WorkspaceID:        testWorkspace1,
		MinAge:             30 * 24 * time.Hour,
		MinInactiveCount:   5,
		KeepRecentInactive: 5,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted, "fresh observations are protected by MinAge")
}

// TestRunTombstoneGC_RequiresWorkspace proves the global-delete
// guard. Tombstone GC without a workspace would hard-delete rows
// across every tenant; the guard makes that impossible.
func TestRunTombstoneGC_RequiresWorkspace(t *testing.T) {
	store := newStore(t)
	_, err := store.RunTombstoneGC(context.Background(), TombstoneGCOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

// TestRunTombstoneGC_AppliesDefaults proves zero-valued options
// fall through to the spec defaults (30 days, > 20 inactive, keep
// 5). With zero values the call must not be a no-op when the chain
// crosses the default threshold.
func TestRunTombstoneGC_AppliesDefaults(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	mem := &Memory{Type: "fact", Content: "v1", Confidence: 0.9, Scope: scope}
	require.NoError(t, store.Save(ctx, mem))
	for i := 0; i < 25; i++ {
		_, err := store.AppendObservationToEntity(ctx, mem.ID,
			&Memory{Type: "fact", Content: "vN", Confidence: 0.9, Scope: scope})
		require.NoError(t, err)
	}
	_, err := store.pool.Exec(ctx, `
		UPDATE memory_observations
		SET observed_at = now() - interval '40 days'
		WHERE entity_id = $1`, mem.ID)
	require.NoError(t, err)

	deleted, err := store.RunTombstoneGC(ctx, TombstoneGCOptions{
		WorkspaceID: testWorkspace1, // everything else zero
	})
	require.NoError(t, err)
	// 25 inactive, defaults: > 20 threshold, keep 5 → delete 25-5 = 20.
	assert.Equal(t, int64(20), deleted)
}
