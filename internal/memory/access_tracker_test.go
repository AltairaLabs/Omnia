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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRetrieve_TouchesAccessedAt exercises the read-path accessed_at
// update: a Retrieve against a freshly-saved memory must, within a short
// window, move the observation's accessed_at forward and increment
// access_count. This is the signal LRU pruning and recency-weighted
// scoring rely on.
func TestRetrieve_TouchesAccessedAt(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	mem := &Memory{
		Type: "fact",
		// Plain words so the FTS websearch_to_tsquery match below is
		// straightforward — this test is about the accessed_at touch
		// path, not search semantics.
		Content:    "accessed marker fact",
		Confidence: 0.9,
		Scope:      scope,
	}
	require.NoError(t, store.Save(ctx, mem))
	require.NotEmpty(t, mem.ID)

	// Baseline row: access_count = 0, accessed_at IS NULL.
	var baselineCount int
	var baselineAccessedAt *time.Time
	require.NoError(t, store.pool.QueryRow(ctx, `
		SELECT access_count, accessed_at FROM memory_observations
		WHERE entity_id = $1 AND superseded_by IS NULL
		ORDER BY observed_at DESC LIMIT 1`, mem.ID,
	).Scan(&baselineCount, &baselineAccessedAt))
	assert.Equal(t, 0, baselineCount)
	assert.Nil(t, baselineAccessedAt, "accessed_at should start NULL on fresh inserts")

	// Do a retrieve that will return the row. The touch is async, so we
	// poll briefly for it to land.
	res, err := store.Retrieve(ctx, scope, "marker", RetrieveOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, res, 1)

	require.Eventually(t, func() bool {
		var n int
		var at *time.Time
		err := store.pool.QueryRow(ctx, `
			SELECT access_count, accessed_at FROM memory_observations
			WHERE entity_id = $1 AND superseded_by IS NULL
			ORDER BY observed_at DESC LIMIT 1`, mem.ID,
		).Scan(&n, &at)
		return err == nil && n == 1 && at != nil
	}, 3*time.Second, 50*time.Millisecond,
		"accessed_at should be updated by Retrieve within a few seconds")
}

func TestRetrieveMultiTier_TouchesAccessedAt(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	// Institutional row — clean scope, no user filter needed.
	insertRawMemory(t, store, "", "", "fact", "multi-tier-touch", 0.9)

	result, err := store.RetrieveMultiTier(ctx, MultiTierRequest{
		WorkspaceID: testWorkspace1,
		Limit:       10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Memories)
	entityID := result.Memories[0].ID

	require.Eventually(t, func() bool {
		var n int
		var at *time.Time
		err := store.pool.QueryRow(ctx, `
			SELECT access_count, accessed_at FROM memory_observations
			WHERE entity_id = $1 AND superseded_by IS NULL
			ORDER BY observed_at DESC LIMIT 1`, entityID,
		).Scan(&n, &at)
		return err == nil && n == 1 && at != nil
	}, 3*time.Second, 50*time.Millisecond,
		"RetrieveMultiTier must touch accessed_at on the rows it returns")
}

// TestRetrieve_EmptyResultsIsNoop proves the touch path exits cleanly on
// empty retrievals (no wasted UPDATE, no panic).
func TestRetrieve_EmptyResultsIsNoop(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	// No rows saved; retrieval returns empty.
	res, err := store.Retrieve(ctx, testScope(testWorkspace1), "nothing here", RetrieveOptions{Limit: 5})
	require.NoError(t, err)
	assert.Empty(t, res)
	// Nothing to assert on the DB side — the goal is "no panic, no
	// hang". A fire-and-forget goroutine with an empty slice should
	// return immediately.
	time.Sleep(50 * time.Millisecond)
}

func TestDedupeStrings(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"empty", nil, nil},
		{"single", []string{"a"}, []string{"a"}},
		{"no dupes", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"repeated head", []string{"a", "a", "b"}, []string{"a", "b"}},
		{"repeated tail", []string{"a", "b", "b", "b"}, []string{"a", "b"}},
		{"interleaved", []string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dedupeStrings(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestEntityIDsFromMemories_SkipsNilAndEmpty(t *testing.T) {
	in := []*Memory{
		{ID: "one"},
		nil,
		{ID: ""},
		{ID: "two"},
	}
	got := entityIDsFromMemories(in)
	assert.Equal(t, []string{"one", "two"}, got)
}

func TestEntityIDsFromMultiTier_SkipsNilAndEmpty(t *testing.T) {
	in := []*MultiTierMemory{
		{Memory: &Memory{ID: "one"}},
		nil,
		{Memory: nil},
		{Memory: &Memory{ID: ""}},
		{Memory: &Memory{ID: "two"}},
	}
	got := entityIDsFromMultiTier(in)
	assert.Equal(t, []string{"one", "two"}, got)
}
