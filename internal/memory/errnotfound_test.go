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

const missingEntityID = "00000000-0000-0000-0000-000000000000"

// MAINT-2: store "not found" paths must wrap ErrNotFound so handlers map them
// to 404 instead of 500.
func TestPostgresMemoryStore_Delete_NotFoundWrapsErrNotFound(t *testing.T) {
	store := newStore(t)
	err := store.Delete(context.Background(), testScope(testWorkspace1), missingEntityID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresMemoryStore_SupersedeMany_NotFoundWrapsErrNotFound(t *testing.T) {
	store := newStore(t)
	_, _, err := store.SupersedeMany(context.Background(), []string{missingEntityID},
		&Memory{Type: "fact", Content: "x", Confidence: 1.0, Scope: testScope(testWorkspace1)})
	require.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresMemoryStore_AppendObservationToEntity_NotFoundWrapsErrNotFound(t *testing.T) {
	store := newStore(t)
	_, err := store.AppendObservationToEntity(context.Background(), missingEntityID,
		&Memory{Type: "fact", Content: "x", Confidence: 1.0, Scope: testScope(testWorkspace1)})
	require.ErrorIs(t, err, ErrNotFound)
}

// SEC-7: the DSAR export must return every memory, not just the first page.
func TestPostgresMemoryStore_ExportAll_PaginatesBeyondOnePage(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	const total = 5
	for i := 0; i < total; i++ {
		require.NoError(t, store.Save(ctx, &Memory{
			Type: "fact", Content: "export row", Confidence: 0.9, Scope: scope,
		}))
	}

	// Page size 2 over 5 rows exercises the multi-page loop (2+2+1).
	all, err := store.exportAllPaged(ctx, scope, 2)
	require.NoError(t, err)
	require.Len(t, all, total, "export must return every row across pages, not just the first page")
}
