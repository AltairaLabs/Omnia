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

// seedPrivacyPrefs writes a preferences row with the given grants
// array so the JOIN against user_privacy_preferences resolves.
func seedPrivacyPrefs(t *testing.T, store *PostgresMemoryStore, userID string, grants []string) {
	t.Helper()
	_, err := store.pool.Exec(context.Background(),
		`INSERT INTO user_privacy_preferences (user_id, consent_grants)
		 VALUES ($1, $2)
		 ON CONFLICT (user_id) DO UPDATE SET consent_grants = EXCLUDED.consent_grants`,
		userID, grants)
	require.NoError(t, err)
}

// saveUserMemWithCategory saves a user-tier memory tagged with the
// given consent category. The helper wraps the common scope +
// metadata dance so tests stay readable.
func saveUserMemWithCategory(t *testing.T, store *PostgresMemoryStore, userID, category string) string {
	t.Helper()
	mem := &Memory{
		Type: "fact", Content: "user memory", Confidence: 0.9,
		Scope: map[string]string{
			ScopeWorkspaceID: testWorkspace1,
			ScopeUserID:      userID,
		},
		Metadata: map[string]any{MetaKeyConsentCategory: category},
	}
	require.NoError(t, store.Save(context.Background(), mem))
	return mem.ID
}

func TestSoftDeleteRevokedConsent_FlipsRowsMissingFromGrants(t *testing.T) {
	store := newStore(t)
	userID := "user-phase4-a"

	// User grants "memory:context" but not "memory:health".
	seedPrivacyPrefs(t, store, userID, []string{"memory:context"})
	healthID := saveUserMemWithCategory(t, store, userID, "memory:health")
	contextID := saveUserMemWithCategory(t, store, userID, "memory:context")

	n, err := store.SoftDeleteRevokedConsent(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	assert.True(t, mustFetchEntityForgotten(t, store, healthID))
	assert.False(t, mustFetchEntityForgotten(t, store, contextID))
}

func TestSoftDeleteRevokedConsent_IgnoresRowsWithoutCategory(t *testing.T) {
	// A row with NULL consent_category is not part of the cascade —
	// it's either institutional (no category) or untagged legacy data.
	store := newStore(t)
	userID := "user-phase4-b"
	seedPrivacyPrefs(t, store, userID, []string{})

	untagged := &Memory{
		Type: "fact", Content: "no category", Confidence: 0.9,
		Scope: map[string]string{
			ScopeWorkspaceID: testWorkspace1,
			ScopeUserID:      userID,
		},
	}
	require.NoError(t, store.Save(context.Background(), untagged))

	n, err := store.SoftDeleteRevokedConsent(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.False(t, mustFetchEntityForgotten(t, store, untagged.ID))
}

func TestSoftDeleteRevokedConsent_SkipsInstitutionalRows(t *testing.T) {
	// Institutional rows have virtual_user_id IS NULL and therefore
	// don't join against user_privacy_preferences — they should never
	// be touched by the consent cascade even if their category is
	// present on them (which it shouldn't be, but defensively).
	store := newStore(t)
	userID := "user-phase4-c"
	seedPrivacyPrefs(t, store, userID, []string{}) // no grants

	inst := &Memory{
		Type: "policy", Content: "inst policy", Confidence: 1.0,
		Scope:    map[string]string{ScopeWorkspaceID: testWorkspace1},
		Metadata: map[string]any{MetaKeyConsentCategory: "memory:health"},
	}
	require.NoError(t, store.SaveInstitutional(context.Background(), inst))

	n, err := store.SoftDeleteRevokedConsent(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.False(t, mustFetchEntityForgotten(t, store, inst.ID))
}

func TestSoftDeleteRevokedConsent_BatchSizeZeroIsNoOp(t *testing.T) {
	store := newStore(t)
	n, err := store.SoftDeleteRevokedConsent(context.Background(), 0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

func TestHardDeleteRevokedConsent_RemovesRowsMissingFromGrants(t *testing.T) {
	store := newStore(t)
	userID := "user-phase4-d"
	seedPrivacyPrefs(t, store, userID, []string{}) // all revoked
	id := saveUserMemWithCategory(t, store, userID, "memory:location")

	n, err := store.HardDeleteRevokedConsent(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	assert.False(t, mustFetchEntityExists(t, store, id))
}

func TestHardDeleteRevokedConsent_BatchSizeZeroIsNoOp(t *testing.T) {
	store := newStore(t)
	n, err := store.HardDeleteRevokedConsent(context.Background(), 0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

func TestHardDeleteForgottenByConsentOlderThan_UsesForgottenAt(t *testing.T) {
	// Set forgotten_at to 30 days ago; a 7-day grace window should
	// hard-delete the row. updated_at is not consulted — the test
	// leaves it at now() to guard against regressions.
	store := newStore(t)
	userID := "user-phase4-e"
	seedPrivacyPrefs(t, store, userID, []string{})
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
	require.NoError(t, store.SaveInstitutional(context.Background(), mem))
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
			ScopeWorkspaceID: testWorkspace1,
			ScopeUserID:      "user-no-cat",
		},
	}
	require.NoError(t, store.Save(context.Background(), mem))

	var got *string
	err := store.pool.QueryRow(context.Background(),
		"SELECT consent_category FROM memory_entities WHERE id = $1", mem.ID).Scan(&got)
	require.NoError(t, err)
	assert.Nil(t, got)
}
