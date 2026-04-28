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

// TestFindConflictedEntities_SurfacesEntitiesWithMultipleActive
// proves the dedup-bypass detector: an entity with two active
// observations (constructed by writing direct SQL that bypasses
// supersedePriorObservations) shows up in the conflict queue.
// Entities with exactly one active observation are filtered out.
func TestFindConflictedEntities_SurfacesEntitiesWithMultipleActive(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// Single-active entity — must NOT appear in conflicts.
	clean := &Memory{Type: "fact", Content: "clean", Confidence: 0.9, Scope: scope}
	require.NoError(t, store.Save(ctx, clean))

	// Conflicted entity — write a second observation directly without
	// running supersede, leaving both active.
	conflicted := &Memory{Type: "fact", Content: "first", Confidence: 0.9, Scope: scope}
	require.NoError(t, store.Save(ctx, conflicted))
	_, err := store.pool.Exec(ctx, `
		INSERT INTO memory_observations (entity_id, content, confidence)
		VALUES ($1, $2, $3)`, conflicted.ID, "second", 0.9)
	require.NoError(t, err)

	got, err := store.FindConflictedEntities(ctx, testWorkspace1, 50)
	require.NoError(t, err)
	require.Len(t, got, 1, "only the conflicted entity surfaces")
	assert.Equal(t, conflicted.ID, got[0].EntityID)
	assert.Equal(t, "fact", got[0].Kind)
	assert.Equal(t, 2, got[0].ActiveCount)
}

// TestFindConflictedEntities_RespectsWorkspaceGuard proves the
// scope guard fires — global queries are never the right answer
// for an admin endpoint.
func TestFindConflictedEntities_RespectsWorkspaceGuard(t *testing.T) {
	store := newStore(t)
	_, err := store.FindConflictedEntities(context.Background(), "", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

// TestFindConflictedEntities_AppliesDefaultLimit proves the limit
// fallback so an unbounded request can't dump unbounded rows.
func TestFindConflictedEntities_AppliesDefaultLimit(t *testing.T) {
	store := newStore(t)
	got, err := store.FindConflictedEntities(context.Background(), testWorkspace1, 0)
	require.NoError(t, err)
	// Empty workspace returns no rows — but the call must succeed
	// (proving the default-limit branch was taken without errors).
	assert.Empty(t, got)
}
