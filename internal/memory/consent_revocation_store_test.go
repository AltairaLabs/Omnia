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

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

// fakeConsentRevocationSource is a test double for ConsentRevocationSource.
// nonGrantors maps category string → user IDs who have a preferences record
// but do NOT grant that category (matching the INNER JOIN semantics of the
// former user_privacy_preferences query). Users with no record are never
// included.
type fakeConsentRevocationSource struct {
	nonGrantors map[string][]string
}

func (f *fakeConsentRevocationSource) ListConsentUsers(
	_ context.Context,
	category privacy.ConsentCategory,
	granted bool,
) ([]string, error) {
	if granted {
		// Revocation callers always pass granted=false; this path is not used.
		return nil, nil
	}
	return f.nonGrantors[string(category)], nil
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

// TestSoftDeleteRevokedConsent_DeletesNonGrantorRows verifies that rows
// belonging to a non-granting user are soft-deleted while rows belonging to:
//   - a granting user
//   - a user with no preferences record (never in nonGrantors — key invariant)
//   - an institutional row (virtual_user_id IS NULL)
//
// all survive.
func TestSoftDeleteRevokedConsent_DeletesNonGrantorRows(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	grantingUser := "user-grants-health"
	nonGrantingUser := "user-no-health"
	noRecordUser := "user-no-prefs-record" // never appears in nonGrantors

	healthKept := saveUserMemWithCategory(t, store, grantingUser, "memory:health")
	healthDeleted := saveUserMemWithCategory(t, store, nonGrantingUser, "memory:health")
	healthNoRecord := saveUserMemWithCategory(t, store, noRecordUser, "memory:health")

	inst := &Memory{
		Type: projTypePolicy, Content: "institutional", Confidence: 1.0,
		Scope:    map[string]string{ScopeWorkspaceID: testWorkspace1},
		Metadata: map[string]any{MetaKeyConsentCategory: healthCat},
	}
	seedInstitutional(t, store, inst)

	// Only nonGrantingUser lacks health consent; grantingUser and noRecordUser
	// are NOT included (matching the INNER JOIN: only users WITH a prefs row who
	// don't grant the category).
	src := &fakeConsentRevocationSource{
		nonGrantors: map[string][]string{
			"memory:health": {nonGrantingUser},
		},
	}

	n, err := store.SoftDeleteRevokedConsent(ctx, src, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	assert.True(t, mustFetchEntityForgotten(t, store, healthDeleted),
		"non-granting user row must be soft-deleted")
	assert.False(t, mustFetchEntityForgotten(t, store, healthKept),
		"granting user row must survive")
	assert.False(t, mustFetchEntityForgotten(t, store, healthNoRecord),
		"user with no prefs record must survive — key data-safety invariant")
	assert.False(t, mustFetchEntityForgotten(t, store, inst.ID),
		"institutional row must never be touched")
}

// TestSoftDeleteRevokedConsent_MultiCategory verifies that per-category
// iteration correctly targets rows in each category independently.
func TestSoftDeleteRevokedConsent_MultiCategory(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	userA := "user-multi-a"
	userB := "user-multi-b"

	// userA has memory:identity row; userB has memory:location row
	idRow := saveUserMemWithCategory(t, store, userA, "memory:identity")
	locRow := saveUserMemWithCategory(t, store, userB, "memory:location")
	// userA also has memory:location row — should survive because userA is not
	// in the memory:location non-grantors list
	alsoAlive := saveUserMemWithCategory(t, store, userA, "memory:location")

	src := &fakeConsentRevocationSource{
		nonGrantors: map[string][]string{
			identityCat: {userA},
			locationCat: {userB},
		},
	}

	n, err := store.SoftDeleteRevokedConsent(ctx, src, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)

	assert.True(t, mustFetchEntityForgotten(t, store, idRow))
	assert.True(t, mustFetchEntityForgotten(t, store, locRow))
	assert.False(t, mustFetchEntityForgotten(t, store, alsoAlive),
		"userA's location row must survive — userA not in memory:location non-grantors")
}

// TestSoftDeleteRevokedConsent_NilSourceIsNoOp confirms fail-safe: when no
// consent source is configured the pass must not delete anything.
func TestSoftDeleteRevokedConsent_NilSourceIsNoOp(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	id := saveUserMemWithCategory(t, store, "user-nil-src", "memory:health")
	n, err := store.SoftDeleteRevokedConsent(ctx, nil, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.False(t, mustFetchEntityForgotten(t, store, id), "nil source must not delete")
}

// TestSoftDeleteRevokedConsent_BatchSizeZeroIsNoOp confirms the batch-guard.
func TestSoftDeleteRevokedConsent_BatchSizeZeroIsNoOp(t *testing.T) {
	store := newStore(t)
	src := &fakeConsentRevocationSource{
		nonGrantors: map[string][]string{"memory:health": {"user-bz"}},
	}
	n, err := store.SoftDeleteRevokedConsent(context.Background(), src, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

// TestSoftDeleteRevokedConsent_SetsForgottenFields ensures forgotten_at and
// updated_at are stamped when a row is soft-deleted.
func TestSoftDeleteRevokedConsent_SetsForgottenFields(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	userID := "user-forgotten-fields"
	id := saveUserMemWithCategory(t, store, userID, "memory:health")

	src := &fakeConsentRevocationSource{
		nonGrantors: map[string][]string{"memory:health": {userID}},
	}
	n, err := store.SoftDeleteRevokedConsent(ctx, src, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	var forgotten bool
	var forgottenAt *int64 // just check it's non-null
	err = store.pool.QueryRow(ctx,
		"SELECT forgotten, (forgotten_at IS NOT NULL)::int FROM memory_entities WHERE id = $1", id,
	).Scan(&forgotten, &forgottenAt)
	require.NoError(t, err)
	assert.True(t, forgotten)
	assert.NotNil(t, forgottenAt)
}

// TestSoftDeleteRevokedConsent_SkipsRowsWithoutCategory rows with NULL
// consent_category (untagged / institutional) are not targeted.
func TestSoftDeleteRevokedConsent_SkipsRowsWithoutCategory(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	userID := "user-no-cat"
	untagged := &Memory{
		Type: "fact", Content: "no category", Confidence: 0.9,
		Scope: map[string]string{
			ScopeWorkspaceID: testWorkspace1,
			ScopeUserID:      userID,
		},
	}
	require.NoError(t, store.Save(ctx, untagged))

	// Source returns userID as a non-granter for all categories, but the row
	// has no consent_category — consent_category = $1 never matches NULL.
	src := &fakeConsentRevocationSource{
		nonGrantors: map[string][]string{
			"memory:health":   {userID},
			"memory:identity": {userID},
			"memory:location": {userID},
		},
	}
	n, err := store.SoftDeleteRevokedConsent(ctx, src, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.False(t, mustFetchEntityForgotten(t, store, untagged.ID))
}

// --- HardDeleteRevokedConsent ---

// TestHardDeleteRevokedConsent_RemovesNonGrantorRows verifies immediate
// removal for the hard-delete action path.
func TestHardDeleteRevokedConsent_RemovesNonGrantorRows(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	nonGrantingUser := "user-hard-nongranter"
	grantingUser := "user-hard-granter"
	noRecordUser := "user-hard-norecord"

	deletedID := saveUserMemWithCategory(t, store, nonGrantingUser, "memory:location")
	keptGrantID := saveUserMemWithCategory(t, store, grantingUser, "memory:location")
	keptNoRecordID := saveUserMemWithCategory(t, store, noRecordUser, "memory:location")

	src := &fakeConsentRevocationSource{
		nonGrantors: map[string][]string{
			"memory:location": {nonGrantingUser},
		},
	}

	n, err := store.HardDeleteRevokedConsent(ctx, src, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	assert.False(t, mustFetchEntityExists(t, store, deletedID),
		"non-granting user row must be hard-deleted")
	assert.True(t, mustFetchEntityExists(t, store, keptGrantID),
		"granting user row must survive")
	assert.True(t, mustFetchEntityExists(t, store, keptNoRecordID),
		"no-record user row must survive — key data-safety invariant")
}

// TestHardDeleteRevokedConsent_NilSourceIsNoOp confirms fail-safe.
func TestHardDeleteRevokedConsent_NilSourceIsNoOp(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	id := saveUserMemWithCategory(t, store, "user-nil-hard", "memory:health")
	n, err := store.HardDeleteRevokedConsent(ctx, nil, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
	assert.True(t, mustFetchEntityExists(t, store, id), "nil source must not delete")
}

// TestHardDeleteRevokedConsent_BatchSizeZeroIsNoOp confirms the batch-guard.
func TestHardDeleteRevokedConsent_BatchSizeZeroIsNoOp(t *testing.T) {
	store := newStore(t)
	src := &fakeConsentRevocationSource{
		nonGrantors: map[string][]string{"memory:health": {"user-bz-hard"}},
	}
	n, err := store.HardDeleteRevokedConsent(context.Background(), src, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

// --- HardDeleteForgottenByConsentOlderThan (unchanged — no source needed) ---

func TestHardDeleteForgottenByConsentOlderThan_UsesForgottenAt(t *testing.T) {
	// Set forgotten_at to 30 days ago; a 7-day grace window should
	// hard-delete the row. updated_at is not consulted — the test
	// leaves it at now() to guard against regressions.
	store := newStore(t)
	id := saveUserMemWithCategory(t, store, "user-phase4-e", "memory:location")
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
			ScopeWorkspaceID: testWorkspace1,
			ScopeUserID:      "user-no-cat-nil",
		},
	}
	require.NoError(t, store.Save(context.Background(), mem))

	var got *string
	err := store.pool.QueryRow(context.Background(),
		"SELECT consent_category FROM memory_entities WHERE id = $1", mem.ID).Scan(&got)
	require.NoError(t, err)
	assert.Nil(t, got)
}
