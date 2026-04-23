/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustFetchEntityForgotten reads the forgotten flag off a memory entity.
// Used to assert soft-delete state post-retention.
func mustFetchEntityForgotten(t *testing.T, store *PostgresMemoryStore, id string) bool {
	t.Helper()
	var forgotten bool
	err := store.pool.QueryRow(context.Background(),
		"SELECT forgotten FROM memory_entities WHERE id = $1", id).Scan(&forgotten)
	require.NoError(t, err)
	return forgotten
}

// mustFetchEntityExists returns whether the entity row is still present
// (hard-delete removes it entirely).
func mustFetchEntityExists(t *testing.T, store *PostgresMemoryStore, id string) bool {
	t.Helper()
	var count int
	err := store.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM memory_entities WHERE id = $1", id).Scan(&count)
	require.NoError(t, err)
	return count > 0
}

func TestSoftDeleteExpiredTTL_InstitutionalTier(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	expired := &Memory{
		Type: "fact", Content: "old", Confidence: 0.9,
		Scope:     map[string]string{ScopeWorkspaceID: testWorkspace1},
		ExpiresAt: &past,
	}
	keep := &Memory{
		Type: "fact", Content: "fresh", Confidence: 0.9,
		Scope:     map[string]string{ScopeWorkspaceID: testWorkspace1},
		ExpiresAt: &future,
	}
	require.NoError(t, store.SaveInstitutional(ctx, expired))
	require.NoError(t, store.SaveInstitutional(ctx, keep))

	n, err := store.SoftDeleteExpiredTTL(ctx, TierInstitutional, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	assert.True(t, mustFetchEntityForgotten(t, store, expired.ID))
	assert.False(t, mustFetchEntityForgotten(t, store, keep.ID))
}

func TestSoftDeleteExpiredTTL_TierIsolation(t *testing.T) {
	// TTL pruning on the user tier must not touch institutional rows.
	store := newStore(t)
	ctx := context.Background()
	past := time.Now().Add(-1 * time.Hour)

	inst := &Memory{
		Type: "fact", Content: "inst", Confidence: 0.9,
		Scope:     map[string]string{ScopeWorkspaceID: testWorkspace1},
		ExpiresAt: &past,
	}
	userMem := &Memory{
		Type: "fact", Content: "user", Confidence: 0.9,
		Scope:     map[string]string{ScopeWorkspaceID: testWorkspace1, ScopeUserID: "user-a"},
		ExpiresAt: &past,
	}
	require.NoError(t, store.SaveInstitutional(ctx, inst))
	require.NoError(t, store.Save(ctx, userMem))

	n, err := store.SoftDeleteExpiredTTL(ctx, TierUser, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	assert.False(t, mustFetchEntityForgotten(t, store, inst.ID))
	assert.True(t, mustFetchEntityForgotten(t, store, userMem.ID))
}

func TestSoftDeleteExpiredTTL_BatchSizeZeroIsNoOp(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	n, err := store.SoftDeleteExpiredTTL(ctx, TierInstitutional, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

func TestSoftDeleteLRU_MarksOldestRowsFirst(t *testing.T) {
	// Save two entities; backdate the "old" one's observation. LRU
	// pruning with a 30-minute staleAfter should catch the backdated
	// row and leave the fresh one alone.
	store := newStore(t)
	ctx := context.Background()

	old := &Memory{
		Type: "fact", Content: "old", Confidence: 0.9,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1},
	}
	fresh := &Memory{
		Type: "fact", Content: "fresh", Confidence: 0.9,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1},
	}
	require.NoError(t, store.SaveInstitutional(ctx, old))
	require.NoError(t, store.SaveInstitutional(ctx, fresh))

	// Backdate the old entity and every observation it owns. created_at
	// and accessed_at both feed into the LRU calculation.
	_, err := store.pool.Exec(ctx,
		"UPDATE memory_entities SET created_at = now() - interval '2 hours' WHERE id = $1",
		old.ID)
	require.NoError(t, err)
	_, err = store.pool.Exec(ctx,
		"UPDATE memory_observations SET observed_at = now() - interval '2 hours', accessed_at = now() - interval '2 hours' WHERE entity_id = $1",
		old.ID)
	require.NoError(t, err)

	n, err := store.SoftDeleteLRU(ctx, TierInstitutional, 30*time.Minute, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	assert.True(t, mustFetchEntityForgotten(t, store, old.ID))
	assert.False(t, mustFetchEntityForgotten(t, store, fresh.ID))
}

func TestSoftDeleteLRU_ZeroStaleIsNoOp(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	n, err := store.SoftDeleteLRU(ctx, TierInstitutional, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

func TestHardDeleteForgottenOlderThan(t *testing.T) {
	// Soft-delete a row, backdate updated_at, then confirm hard-delete
	// actually removes it from the table.
	store := newStore(t)
	ctx := context.Background()

	mem := &Memory{
		Type: "fact", Content: "obsolete", Confidence: 0.9,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1},
	}
	require.NoError(t, store.SaveInstitutional(ctx, mem))

	_, err := store.pool.Exec(ctx,
		"UPDATE memory_entities SET forgotten = true, updated_at = now() - interval '30 days' WHERE id = $1",
		mem.ID)
	require.NoError(t, err)

	n, err := store.HardDeleteForgottenOlderThan(ctx, 7, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	assert.False(t, mustFetchEntityExists(t, store, mem.ID))
}

func TestHardDeleteForgottenOlderThan_RespectsGrace(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	mem := &Memory{
		Type: "fact", Content: "recently forgotten", Confidence: 0.9,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1},
	}
	require.NoError(t, store.SaveInstitutional(ctx, mem))

	// Soft-delete with recent updated_at — grace window of 7 days
	// should keep the row around.
	_, err := store.pool.Exec(ctx,
		"UPDATE memory_entities SET forgotten = true, updated_at = now() WHERE id = $1",
		mem.ID)
	require.NoError(t, err)

	n, err := store.HardDeleteForgottenOlderThan(ctx, 7, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.True(t, mustFetchEntityExists(t, store, mem.ID))
}

func TestHardDeleteForgottenOlderThan_NegativeGraceErrors(t *testing.T) {
	store := newStore(t)
	_, err := store.HardDeleteForgottenOlderThan(context.Background(), -1, 100)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "negative grace days")
}

func TestHardDeleteForgottenOlderThan_BatchSizeZeroIsNoOp(t *testing.T) {
	store := newStore(t)
	n, err := store.HardDeleteForgottenOlderThan(context.Background(), 7, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}
