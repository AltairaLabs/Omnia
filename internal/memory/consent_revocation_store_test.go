/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// saveUserMemWithCategory saves a user-tier memory tagged with the
// given consent category. The helper wraps the common scope +
// metadata dance so tests stay readable.
func saveUserMemWithCategory(t *testing.T, store *PostgresMemoryStore, userID, category string) string {
	t.Helper()
	mem := &Memory{
		Type: "fact", Content: "user memory", Confidence: 0.9,
		Scope: map[string]string{
			ScopeWorkspaceID:   testWorkspace1,
			ScopeVirtualUserID: userID,
		},
		Metadata: map[string]any{MetaKeyConsentCategory: category},
	}
	require.NoError(t, store.Save(context.Background(), mem))
	return mem.ID
}

func TestHardDeleteForgottenByConsentOlderThan_UsesForgottenAt(t *testing.T) {
	// Set forgotten_at to 30 days ago; a 7-day grace window should
	// hard-delete the row. updated_at is not consulted — the test
	// leaves it at now() to guard against regressions.
	store := newStore(t)
	userID := "user-phase4-e"
	id := saveUserMemWithCategory(t, store, userID, "memory:location")
	_, err := store.pool.Exec(context.Background(),
		"UPDATE memory_entities SET forgotten = true, forgotten_at = now() - interval '30 days' WHERE id = $1",
		id)
	require.NoError(t, err)

	n, err := store.HardDeleteForgottenByConsentOlderThan(context.Background(), 7, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	assert.False(t, mustFetchEntityExists(t, store, id))
}

func TestHardDeleteForgottenByConsentOlderThan_SkipsTTLForgottenRows(t *testing.T) {
	// A row flipped by the TTL branch has forgotten=true but
	// forgotten_at=NULL. The consent-grace pass must skip it so the
	// general hard-delete pass handles it on its own cadence.
	store := newStore(t)
	mem := &Memory{
		Type: "fact", Content: "ttl-flipped", Confidence: 0.9,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1},
	}
	seedInstitutional(t, store, mem)
	_, err := store.pool.Exec(context.Background(),
		"UPDATE memory_entities SET forgotten = true, updated_at = now() - interval '30 days' WHERE id = $1",
		mem.ID)
	require.NoError(t, err)

	n, err := store.HardDeleteForgottenByConsentOlderThan(context.Background(), 7, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.True(t, mustFetchEntityExists(t, store, mem.ID))
}

func TestHardDeleteForgottenByConsentOlderThan_NegativeGraceErrors(t *testing.T) {
	store := newStore(t)
	_, err := store.HardDeleteForgottenByConsentOlderThan(context.Background(), -1, 100)
	require.Error(t, err)
}

func TestHardDeleteForgottenByConsentOlderThan_BatchSizeZeroIsNoOp(t *testing.T) {
	store := newStore(t)
	n, err := store.HardDeleteForgottenByConsentOlderThan(context.Background(), 7, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

func TestConsentCategoryPersistsOnWrite(t *testing.T) {
	// Round-trips MetaKeyConsentCategory through the store to confirm
	// insertEntity writes the column (not just the metadata JSON).
	store := newStore(t)
	id := saveUserMemWithCategory(t, store, "user-persist", "memory:health")

	var got *string
	err := store.pool.QueryRow(context.Background(),
		"SELECT consent_category FROM memory_entities WHERE id = $1", id).Scan(&got)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "memory:health", *got)
}

func TestConsentCategoryNilWhenMetadataMissing(t *testing.T) {
	// A write without MetaKeyConsentCategory should leave the column
	// NULL so the row falls under the default policy.
	store := newStore(t)
	mem := &Memory{
		Type: "fact", Content: "no category", Confidence: 0.9,
		Scope: map[string]string{
			ScopeWorkspaceID:   testWorkspace1,
			ScopeVirtualUserID: "user-no-cat",
		},
	}
	require.NoError(t, store.Save(context.Background(), mem))

	var got *string
	err := store.pool.QueryRow(context.Background(),
		"SELECT consent_category FROM memory_entities WHERE id = $1", mem.ID).Scan(&got)
	require.NoError(t, err)
	assert.Nil(t, got)
}
