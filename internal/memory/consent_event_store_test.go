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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSoftDeleteUserConsentCategory covers the per-user/category soft-delete
// path and verifies the id-contract: only rows matching the exact
// (workspace, userID, category) triple are touched; other users, other
// categories, and institutional rows (virtual_user_id IS NULL) are untouched.
func TestSoftDeleteUserConsentCategory(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	ctx := context.Background()

	ws := insertWorkspace(t, pool)
	wsOther := insertWorkspace(t, pool)

	userX := "user-x-soft"
	userY := "user-y-soft"
	catC := "memory:health"
	catD := "memory:preferences"

	// Seed rows: X/C (target), X/D (keep — different category),
	// Y/C (keep — different user), institutional with C (keep — no virtual_user_id).
	xCRow1 := insertMemoryEntity(t, pool, ws, ptr(userX), nil, ptr(catC))
	xCRow2 := insertMemoryEntity(t, pool, ws, ptr(userX), nil, ptr(catC))
	xDRow := insertMemoryEntity(t, pool, ws, ptr(userX), nil, ptr(catD))
	yCRow := insertMemoryEntity(t, pool, ws, ptr(userY), nil, ptr(catC))
	instRow := insertMemoryEntity(t, pool, ws, nil, nil, ptr(catC))
	// Row in a different workspace (same user/category) — must not be touched.
	otherWsRow := insertMemoryEntity(t, pool, wsOther, ptr(userX), nil, ptr(catC))

	n, err := store.SoftDeleteUserConsentCategory(ctx, ws, userX, catC)
	require.NoError(t, err)
	assert.EqualValues(t, 2, n, "expected exactly 2 X/C rows soft-deleted")

	// X/C rows must now be forgotten.
	assert.True(t, isForgotten(t, pool, xCRow1), "xCRow1 must be forgotten")
	assert.True(t, isForgotten(t, pool, xCRow2), "xCRow2 must be forgotten")

	// Other rows must be untouched.
	assert.False(t, isForgotten(t, pool, xDRow), "xDRow must NOT be forgotten (different category)")
	assert.False(t, isForgotten(t, pool, yCRow), "yCRow must NOT be forgotten (different user — id-contract)")
	assert.False(t, isForgotten(t, pool, instRow), "instRow must NOT be forgotten (institutional)")
	assert.False(t, isForgotten(t, pool, otherWsRow), "otherWsRow must NOT be forgotten (different workspace)")
}

// TestSoftDeleteUserConsentCategory_Idempotent verifies that calling the
// soft-delete twice doesn't error and only returns >0 on the first call.
func TestSoftDeleteUserConsentCategory_Idempotent(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	ctx := context.Background()

	ws := insertWorkspace(t, pool)
	row := insertMemoryEntity(t, pool, ws, ptr("user-idem"), nil, ptr("memory:health"))

	n1, err := store.SoftDeleteUserConsentCategory(ctx, ws, "user-idem", "memory:health")
	require.NoError(t, err)
	assert.EqualValues(t, 1, n1)

	n2, err := store.SoftDeleteUserConsentCategory(ctx, ws, "user-idem", "memory:health")
	require.NoError(t, err)
	assert.EqualValues(t, 0, n2, "second call should affect 0 rows (already forgotten)")
	assert.True(t, isForgotten(t, pool, row))
}

// TestSoftDeleteUserConsentCategory_UnknownUser verifies that an unknown user
// returns 0, not an error.
func TestSoftDeleteUserConsentCategory_UnknownUser(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	ctx := context.Background()

	ws := insertWorkspace(t, pool)

	n, err := store.SoftDeleteUserConsentCategory(ctx, ws, "no-such-user", "memory:health")
	require.NoError(t, err)
	assert.EqualValues(t, 0, n)
}

// TestSoftDeleteUserConsentCategory_MissingArgs verifies that empty parameters
// return an error immediately without touching the database.
func TestSoftDeleteUserConsentCategory_MissingArgs(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	ctx := context.Background()

	_, err := store.SoftDeleteUserConsentCategory(ctx, "", "user", "cat")
	assert.Error(t, err, "empty workspaceID must error")

	_, err = store.SoftDeleteUserConsentCategory(ctx, "ws", "", "cat")
	assert.Error(t, err, "empty userID must error")

	_, err = store.SoftDeleteUserConsentCategory(ctx, "ws", "user", "")
	assert.Error(t, err, "empty category must error")
}

// TestHardDeleteUserConsentCategory covers the per-user/category hard-delete
// path and verifies the same id-contract as the soft-delete test.
func TestHardDeleteUserConsentCategory(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	ctx := context.Background()

	ws := insertWorkspace(t, pool)
	wsOther := insertWorkspace(t, pool)

	userX := "user-x-hard"
	userY := "user-y-hard"
	catC := "memory:health"
	catD := "memory:preferences"

	xCRow1 := insertMemoryEntity(t, pool, ws, ptr(userX), nil, ptr(catC))
	xCRow2 := insertMemoryEntity(t, pool, ws, ptr(userX), nil, ptr(catC))
	xDRow := insertMemoryEntity(t, pool, ws, ptr(userX), nil, ptr(catD))
	yCRow := insertMemoryEntity(t, pool, ws, ptr(userY), nil, ptr(catC))
	instRow := insertMemoryEntity(t, pool, ws, nil, nil, ptr(catC))
	otherWsRow := insertMemoryEntity(t, pool, wsOther, ptr(userX), nil, ptr(catC))

	n, err := store.HardDeleteUserConsentCategory(ctx, ws, userX, catC)
	require.NoError(t, err)
	assert.EqualValues(t, 2, n, "expected exactly 2 X/C rows hard-deleted")

	// X/C rows must be gone.
	assert.False(t, rowExists(t, pool, xCRow1), "xCRow1 must be deleted")
	assert.False(t, rowExists(t, pool, xCRow2), "xCRow2 must be deleted")

	// Other rows must still exist.
	assert.True(t, rowExists(t, pool, xDRow), "xDRow must still exist (different category)")
	assert.True(t, rowExists(t, pool, yCRow), "yCRow must still exist (different user — id-contract)")
	assert.True(t, rowExists(t, pool, instRow), "instRow must still exist (institutional)")
	assert.True(t, rowExists(t, pool, otherWsRow), "otherWsRow must still exist (different workspace)")
}

// TestHardDeleteUserConsentCategory_UnknownUser returns 0, not an error.
func TestHardDeleteUserConsentCategory_UnknownUser(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	ctx := context.Background()

	ws := insertWorkspace(t, pool)

	n, err := store.HardDeleteUserConsentCategory(ctx, ws, "no-such-user", "memory:health")
	require.NoError(t, err)
	assert.EqualValues(t, 0, n)
}

// TestHardDeleteUserConsentCategory_MissingArgs returns error for empty params.
func TestHardDeleteUserConsentCategory_MissingArgs(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	ctx := context.Background()

	_, err := store.HardDeleteUserConsentCategory(ctx, "", "user", "cat")
	assert.Error(t, err)

	_, err = store.HardDeleteUserConsentCategory(ctx, "ws", "", "cat")
	assert.Error(t, err)

	_, err = store.HardDeleteUserConsentCategory(ctx, "ws", "user", "")
	assert.Error(t, err)
}
