/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAdvisoryLock_ReleaseFreesLockAcrossPoolConns is the regression test for
// the stranded-lock bug: the lock was acquired on one pooled connection and
// unlocked on another, so release() no-oped and the lock stayed held forever,
// silently wedging every later TryLock for the key (e.g. the projection worker
// never rendering again after one pass).
func TestAdvisoryLock_ReleaseFreesLockAcrossPoolConns(t *testing.T) {
	pool := freshPgxPool(t)
	store := NewAdvisoryLockStore(pool)
	ctx := context.Background()

	ok, release, err := store.TryLock(ctx, "ws-1", "projection")
	require.NoError(t, err)
	require.True(t, ok, "first lock should be acquired")

	// Held — a second attempt for the same key must fail.
	ok2, rel2, err := store.TryLock(ctx, "ws-1", "projection")
	require.NoError(t, err)
	require.False(t, ok2, "held lock must not be re-acquired")
	rel2() // no-op release on a failed attempt is safe

	// Release, then re-acquire MUST succeed. The old implementation unlocked
	// on a different pooled connection, stranding the lock so this failed.
	release()
	ok3, rel3, err := store.TryLock(ctx, "ws-1", "projection")
	require.NoError(t, err)
	require.True(t, ok3, "lock must be re-acquirable after release")
	rel3()

	// And after that release too — proving release is repeatable, not one-shot.
	ok4, rel4, err := store.TryLock(ctx, "ws-1", "projection")
	require.NoError(t, err)
	require.True(t, ok4, "lock must be re-acquirable after the second release")
	rel4()
}

// TestAdvisoryLock_DistinctKeysDoNotContend proves different (workspace,trigger)
// keys lock independently and don't serialise against each other.
func TestAdvisoryLock_DistinctKeysDoNotContend(t *testing.T) {
	pool := freshPgxPool(t)
	store := NewAdvisoryLockStore(pool)
	ctx := context.Background()

	ok1, rel1, err := store.TryLock(ctx, "ws-1", "projection")
	require.NoError(t, err)
	require.True(t, ok1)
	defer rel1()

	ok2, rel2, err := store.TryLock(ctx, "ws-2", "projection")
	require.NoError(t, err)
	require.True(t, ok2, "a different workspace must not contend")
	rel2()

	ok3, rel3, err := store.TryLock(ctx, "ws-1", "consolidation")
	require.NoError(t, err)
	require.True(t, ok3, "a different trigger on the same workspace must not contend")
	rel3()
}
