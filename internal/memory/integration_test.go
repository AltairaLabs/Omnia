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

// TestMemoryEndToEnd exercises the Omnia side of the memory stack that
// remains live now that PromptKit's sdk.WithMemory() owns extraction:
// direct store.Save → Retrieve → Delete → DeleteAll.
//
// The RAG-facing OmniaRetriever path was deleted along with the unused
// RetrievalStrategy abstraction; PromptKit's pipeline now drives recall
// directly through the Store interface.
func TestMemoryEndToEnd(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// Persist a pair of memories the way PromptKit's memory pipeline
	// does — Omnia just exposes the Store as a pkmemory.Store target.
	for _, content := range []string{
		"User really enjoys working with Kubernetes",
		"Kubernetes is a powerful orchestration platform",
	} {
		mem := &Memory{Type: "fact", Content: content, Confidence: 0.9, Scope: scope}
		require.NoError(t, store.Save(ctx, mem))
		require.NotEmpty(t, mem.ID, "saved memory should have an ID")
	}

	// --- retrieve via store (tool-facing) ---
	retrieved, err := store.Retrieve(ctx, scope, "Kubernetes", RetrieveOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, retrieved, "store.Retrieve should find the saved memory")

	// --- list all ---
	listed, err := store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	assert.NotEmpty(t, listed, "List should return at least one memory")

	// --- delete one ---
	targetID := listed[0].ID
	err = store.Delete(ctx, scope, targetID)
	require.NoError(t, err)

	// Verify it's gone from retrieval.
	afterDelete, err := store.Retrieve(ctx, scope, "", RetrieveOptions{})
	require.NoError(t, err)
	for _, m := range afterDelete {
		assert.NotEqual(t, targetID, m.ID, "deleted memory should not appear")
	}

	// --- deleteAll (DSAR) ---
	err = store.DeleteAll(ctx, scope)
	require.NoError(t, err)

	empty, err := store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, empty, "List should be empty after DeleteAll")
}

// TestMemoryWorkspaceIsolation verifies that memories saved in one workspace
// are not visible from another workspace.
func TestMemoryWorkspaceIsolation(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	scope1 := testScope(testWorkspace1)
	scope2 := testScope(testWorkspace2)

	// Save a memory in workspace-1.
	mem := &Memory{
		Type:       "fact",
		Content:    "workspace isolation test fact",
		Confidence: 0.9,
		Scope:      scope1,
	}
	require.NoError(t, store.Save(ctx, mem))
	require.NotEmpty(t, mem.ID)

	// Retrieve from workspace-2 — should be empty.
	results, err := store.Retrieve(ctx, scope2, "isolation", RetrieveOptions{})
	require.NoError(t, err)
	assert.Empty(t, results, "workspace-2 should not see workspace-1 memories")

	// List from workspace-2 — should also be empty.
	listed, err := store.List(ctx, scope2, ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, listed, "workspace-2 list should be empty")

	// Retrieve from workspace-1 — should find the memory.
	results, err = store.Retrieve(ctx, scope1, "isolation", RetrieveOptions{})
	require.NoError(t, err)
	assert.NotEmpty(t, results, "workspace-1 should find its own memory")
}
