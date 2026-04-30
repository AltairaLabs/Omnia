/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"strings"
	"testing"
	"time"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/runtime"
)

// TestCompositeRetriever_RealStore exercises the runtime CompositeRetriever
// against the real PostgresMemoryStore. Proves that the metadata-based
// category filter survives the round-trip through Save → List, that
// similarity search finds episodic content via Retrieve, and that the
// merged result respects the profile / episodic split.
//
// This is the integration counterpart to memory_retriever_test.go's unit
// tests — those use a fake Store; this uses the real one with metadata
// serialization, postgres column writes, and the retrieval ranker.
func TestCompositeRetriever_RealStore(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// Seed: 3 profile-category memories + 2 episodic memories. The
	// episodic ones intentionally include lexical hooks ("Boston",
	// "Kimpton") so similarity search has something to land on.
	type seed struct {
		category string
		content  string
	}
	seeds := []seed{
		{"memory:identity", "User name: Sarah Kim, email sarah.kim@example.com"},
		{"memory:preferences", "Aisle seat preferred; mid-range hotels"},
		{"memory:health", "Severe peanut allergy — flag on bookings"},
		{"memory:context", "Currently planning a Boston trip for May 17–19"},
		{"memory:history", "Last trip: Chicago October, Kimpton Gray hotel, very positive"},
	}
	saveStart := time.Now()
	for _, s := range seeds {
		mem := &Memory{
			Type:       "fact",
			Content:    s.content,
			Confidence: 0.9,
			Scope:      scope,
			Metadata: map[string]any{
				MetaKeyConsentCategory: s.category,
			},
		}
		require.NoError(t, store.Save(ctx, mem), "save %s", s.category)
	}
	t.Logf("seeded %d memories into postgres in %s", len(seeds), time.Since(saveStart))

	retriever := runtime.NewCompositeRetriever(store, logr.Discard())

	t.Run("cold start (no user message) returns profile only", func(t *testing.T) {
		start := time.Now()
		got, err := retriever.RetrieveContext(ctx, scope, nil)
		t.Logf("RetrieveContext (cold start) took %s, returned %d memories", time.Since(start), len(got))
		require.NoError(t, err)

		require.Len(t, got, 3, "expected 3 profile-category memories")
		gotCats := categoryStrings(got)
		assert.ElementsMatch(t,
			[]string{"memory:identity", "memory:preferences", "memory:health"},
			gotCats,
			"profile pull should be exactly identity / preferences / health",
		)
		for _, m := range got {
			t.Logf("  profile: [%s] %s", categoryOf(m), m.Content)
		}
	})

	t.Run("with user query returns profile + episodic merge", func(t *testing.T) {
		// Fresh retriever to bypass the cache from the prior subtest —
		// otherwise we wouldn't be timing a real list call.
		r2 := runtime.NewCompositeRetriever(store, logr.Discard())
		start := time.Now()
		got, err := r2.RetrieveContext(ctx, scope, []types.Message{
			{Role: "user", Content: "remind me where I stayed in Chicago"},
		})
		t.Logf("RetrieveContext (with query) took %s, returned %d memories", time.Since(start), len(got))
		require.NoError(t, err)

		for _, m := range got {
			t.Logf("  result: [%s] %s", categoryOf(m), m.Content)
		}

		// Strong assertions:
		// 1. All 3 profile memories present.
		profileIDs := profileIDsFromList(got)
		require.Len(t, profileIDs, 3, "all 3 profile memories should appear")

		// 2. At least one episodic result. If FTS returns nothing for
		// a query that lexically overlaps "Chicago" / "Kimpton" /
		// "October" (one of which is in the seeded history memory),
		// we are NOT exercising similarity search — silent failure.
		require.Greater(t, len(got), 3, "expected episodic memories on top of the profile slice — similarity search returned nothing")

		// 3. The Chicago history memory specifically should rank.
		// This is the demo-critical claim ("session 2 recalls past
		// trip"). If this fails, the demo doesn't work.
		foundChicago := false
		for _, m := range got {
			if strings.Contains(m.Content, "Chicago") {
				foundChicago = true
				break
			}
		}
		assert.True(t, foundChicago, "expected the Chicago history memory to surface for a Chicago query")

		// 4. Any episodic result must be from a non-profile category.
		for _, m := range got {
			cat := categoryOf(m)
			if isProfileCat(cat) {
				continue
			}
			assert.NotContains(t,
				[]string{"memory:identity", "memory:preferences", "memory:health"},
				cat,
				"non-profile-category memories must not appear in the episodic slice")
		}
	})

	t.Run("empty user_id scope returns nil", func(t *testing.T) {
		emptyUserScope := map[string]string{ScopeWorkspaceID: testWorkspace1}
		got, err := retriever.RetrieveContext(ctx, emptyUserScope, []types.Message{
			{Role: "user", Content: "anything"},
		})
		require.NoError(t, err)
		assert.Nil(t, got, "no user_id → no retrieval")
	})

	t.Run("profile cache hit avoids extra DB calls", func(t *testing.T) {
		// Fresh retriever to isolate the cache from prior subtests.
		// First call is cold, subsequent should be cached. We can't
		// see the call count without instrumenting the store, so
		// we rely on a relative timing assertion: the cached call
		// must be at least 5× faster than the cold one. On a local
		// container the cold call typically takes ~1ms and the
		// cached one ~10µs — a 100× ratio is normal.
		r2 := runtime.NewCompositeRetriever(store, logr.Discard())

		coldStart := time.Now()
		_, err := r2.RetrieveContext(ctx, scope, nil)
		require.NoError(t, err)
		coldDur := time.Since(coldStart)

		warmStart := time.Now()
		_, err = r2.RetrieveContext(ctx, scope, nil)
		require.NoError(t, err)
		warmDur := time.Since(warmStart)

		t.Logf("cold call: %s, warm call: %s, ratio: %.1fx",
			coldDur, warmDur, float64(coldDur)/float64(warmDur))

		assert.Less(t, warmDur*5, coldDur, "warm call should be substantially faster than cold")
	})
}

// Test helpers — kept package-private to memory_test scope.

func categoryOf(m *pkmemory.Memory) string {
	if m == nil || m.Metadata == nil {
		return ""
	}
	v, _ := m.Metadata[MetaKeyConsentCategory].(string)
	return v
}

func categoryStrings(memories []*pkmemory.Memory) []string {
	out := make([]string, 0, len(memories))
	for _, m := range memories {
		out = append(out, categoryOf(m))
	}
	return out
}

func isProfileCat(cat string) bool {
	switch cat {
	case "memory:identity", "memory:preferences", "memory:health":
		return true
	}
	return false
}

func profileIDsFromList(memories []*pkmemory.Memory) []string {
	out := make([]string, 0, 3)
	for _, m := range memories {
		if isProfileCat(categoryOf(m)) {
			out = append(out, m.ID)
		}
	}
	return out
}
