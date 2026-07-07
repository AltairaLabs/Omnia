/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

const sec3UserA = "user-a"

// SEC-3: a single-tier read with an empty user_id must NOT return another
// user's private rows — empty user_id anchors to institutional/agent
// (virtual_user_id IS NULL), not "no constraint".
func TestSEC3_EmptyUserReadAnchorsToInstitutional(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	wsOnly := map[string]string{ScopeWorkspaceID: testWorkspace1}

	// Only user A's private row exists → an empty-user read must see nothing.
	insertRawMemory(t, store, sec3UserA, "", hybridKindFact, "user A private", 1.0)
	got, err := store.List(ctx, wsOnly, ListOptions{Limit: 50})
	require.NoError(t, err)
	require.Empty(t, got, "empty-user read must not return another user's rows (SEC-3)")

	// Institutional rows ARE still returned.
	insertRawMemory(t, store, "", "", hybridKindFact, "institutional fact", 1.0)
	got, err = store.List(ctx, wsOnly, ListOptions{Limit: 50})
	require.NoError(t, err)
	require.Len(t, got, 1, "institutional rows must still be returned to an empty-user read")
}

// SEC-3: an empty user_id must not be able to open or forget another user's
// memory by UUID; the row stays intact and reachable by its owner.
func TestSEC3_EmptyUserCannotOpenOrDeleteUserRow(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	ownerScope := map[string]string{ScopeWorkspaceID: testWorkspace1, ScopeVirtualUserID: sec3UserA}
	wsOnly := map[string]string{ScopeWorkspaceID: testWorkspace1}

	mem := &Memory{Type: hybridKindFact, Content: "user A private", Confidence: 1.0, Scope: ownerScope}
	require.NoError(t, store.Save(ctx, mem))
	require.NotEmpty(t, mem.ID)

	_, err := store.GetMemory(ctx, wsOnly, mem.ID)
	require.ErrorIs(t, err, ErrNotFound, "empty-user open must not reach a user row")

	err = store.Delete(ctx, wsOnly, mem.ID)
	require.ErrorIs(t, err, ErrNotFound, "empty-user delete must not forget a user row")

	// The owner can still open it — it was untouched.
	got, err := store.GetMemory(ctx, ownerScope, mem.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
}
