/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"testing"

	"github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insertHybridMemory seeds a memory_entities + memory_observations row at the
// tier implied by user/agent (empty => NULL => institutional / non-agent) with
// an explicit embedding, returning the entity ID. Mirrors insertRawMemory but
// sets the pgvector embedding so the cosine ranker has something to match.
func insertHybridMemory(t *testing.T, store *PostgresMemoryStore, user, agent, kind, content string, confidence float64, emb []float32) string {
	t.Helper()
	ctx := context.Background()
	var userArg, agentArg any
	if user != "" {
		userArg = user
	}
	if agent != "" {
		agentArg = agent
	}
	var entityID string
	err := store.pool.QueryRow(ctx,
		`INSERT INTO memory_entities (workspace_id, virtual_user_id, agent_id, name, kind, metadata)
		 VALUES ($1, $2, $3, $4, $5, '{}') RETURNING id`,
		testWorkspace1, userArg, agentArg, content, kind,
	).Scan(&entityID)
	require.NoError(t, err)

	_, err = store.pool.Exec(ctx,
		`INSERT INTO memory_observations (entity_id, content, confidence, embedding)
		 VALUES ($1, $2, $3, $4)`,
		entityID, content, confidence, pgvector.NewVector(emb),
	)
	require.NoError(t, err)
	return entityID
}

// TestRetrieveMultiTierHybrid_SemanticOnlyMatchAcrossTiers proves the core
// fix: an institutional memory worded with no lexical overlap with the query
// still surfaces through the cosine ranker, classified at its tier. This is
// the FTS-only multi-tier path's blind spot that broke the RAG demo.
func TestRetrieveMultiTierHybrid_SemanticOnlyMatchAcrossTiers(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	emb := oneHotFloat(7, 1536)
	// Institutional doc: query shares NO FTS tokens with this content.
	insertHybridMemory(t, store, "", "", "fact",
		"Refunds are processed within five business days.", 0.9, emb)

	res, err := store.RetrieveMultiTierHybrid(ctx, MultiTierRequest{
		WorkspaceID: testWorkspace1,
		AgentID:     multiTierAgentID,
		Query:       "money back timeframe",
		Limit:       10,
	}, emb)
	require.NoError(t, err)
	require.NotEmpty(t, res.Memories, "semantic-only match must surface via cosine ranker")
	assert.Equal(t, TierInstitutional, res.Memories[0].Tier)
}

// TestRetrieveMultiTierHybrid_FallsBackWhenNoEmbedding proves a nil embedding
// short-circuits to the FTS-only multi-tier path rather than erroring — the
// store mirrors RetrieveHybrid's empty-input contract.
func TestRetrieveMultiTierHybrid_FallsBackWhenNoEmbedding(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	insertHybridMemory(t, store, "", "", "fact", "alpha beta gamma", 0.8, oneHotFloat(1, 1536))

	res, err := store.RetrieveMultiTierHybrid(ctx, MultiTierRequest{
		WorkspaceID: testWorkspace1,
		Query:       "alpha",
		Limit:       5,
	}, nil)
	require.NoError(t, err)
	require.Len(t, res.Memories, 1, "FTS fallback must match the lexical token 'alpha'")
}

// TestRetrieveMultiTierHybrid_TierRankerReorders proves the per-tier weight
// bias is applied to the fused score: with cosine-equal matches at two tiers,
// a heavy user weight floats the user-for-agent memory above institutional.
func TestRetrieveMultiTierHybrid_TierRankerReorders(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	emb := oneHotFloat(3, 1536)

	insertHybridMemory(t, store, "", "", "fact", "institutional refund policy", 0.9, emb)
	insertHybridMemory(t, store, "user-1", multiTierAgentID, "fact", "user prefers fast refunds", 0.9, emb)

	ranker := MultiplicativeTierRanker{Weights: map[Tier]float64{
		TierInstitutional: 0.1,
		TierUser:          1.0,
	}}
	res, err := store.RetrieveMultiTierHybrid(ctx, MultiTierRequest{
		WorkspaceID: testWorkspace1,
		UserID:      "user-1",
		AgentID:     multiTierAgentID,
		Query:       "refund speed",
		Limit:       10,
		Ranker:      ranker,
	}, emb)
	require.NoError(t, err)
	require.Len(t, res.Memories, 2, "both tiers should surface via cosine")
	assert.Equal(t, TierUserForAgent, res.Memories[0].Tier,
		"heavy user weight must float the user-for-agent memory above institutional")
}

// TestRetrieveMultiTierHybrid_TypeFilter proves the type filter applies inside
// the shared candidates CTE so non-matching kinds never reach either ranker.
func TestRetrieveMultiTierHybrid_TypeFilter(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	emb := oneHotFloat(5, 1536)

	insertHybridMemory(t, store, "", "", "fact", "the fact row", 0.8, emb)
	insertHybridMemory(t, store, "", "", "preference", "the preference row", 0.8, emb)

	res, err := store.RetrieveMultiTierHybrid(ctx, MultiTierRequest{
		WorkspaceID: testWorkspace1,
		Query:       "row",
		Types:       []string{"fact"},
		Limit:       10,
	}, emb)
	require.NoError(t, err)
	require.NotEmpty(t, res.Memories)
	for _, m := range res.Memories {
		assert.Equal(t, "fact", m.Type, "type filter must exclude non-fact kinds")
	}
}

// TestRetrieveMultiTierHybrid_RequiresWorkspace proves the workspace guard
// fires before any query work — cross-tenant leaks here would be catastrophic.
func TestRetrieveMultiTierHybrid_RequiresWorkspace(t *testing.T) {
	store := newStore(t)
	_, err := store.RetrieveMultiTierHybrid(context.Background(),
		MultiTierRequest{Query: "q"}, oneHotFloat(0, 1536))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

// TestRetrieveMultiTierHybrid_TruncatesToLimit proves the Go-side truncation
// after ranking caps the result at req.Limit even when more candidates fuse.
func TestRetrieveMultiTierHybrid_TruncatesToLimit(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	emb := oneHotFloat(2, 1536)

	for i := 0; i < 4; i++ {
		insertHybridMemory(t, store, "", "", "fact", "shared topic note", 0.8, emb)
	}

	res, err := store.RetrieveMultiTierHybrid(ctx, MultiTierRequest{
		WorkspaceID: testWorkspace1,
		Query:       "topic",
		Limit:       2,
	}, emb)
	require.NoError(t, err)
	require.Len(t, res.Memories, 2)
	assert.Equal(t, 2, res.Total)
}

// TestBuildMultiTierHybridQuery_Structure is a fast, DB-free wiring test: the
// generated SQL must contain both rankers (FTS + cosine), the RRF fusion, the
// tier NULL-anchoring, and pass the query/embedding/fanout as trailing args.
func TestBuildMultiTierHybridQuery_Structure(t *testing.T) {
	t.Run("requires workspace", func(t *testing.T) {
		_, _, err := buildMultiTierHybridQuery(MultiTierRequest{})
		require.Error(t, err)
	})

	sql, args, err := buildMultiTierHybridQuery(MultiTierRequest{
		WorkspaceID: "ws-1",
		UserID:      "u-1",
		AgentID:     "a-1",
		Query:       "dark mode",
		Limit:       10,
	})
	require.NoError(t, err)

	for _, want := range []string{
		"websearch_to_tsquery('english'",              // FTS ranker
		"embedding <=>",                               // cosine ranker
		"FULL OUTER JOIN",                             // RRF fusion
		"e.forgotten = false",                         // not-forgotten guard
		"virtual_user_id IS NULL OR virtual_user_id=", // user tier anchor
		"agent_id IS NULL OR agent_id=",               // agent tier anchor
		"final_score",
	} {
		assert.Contains(t, sql, want, "SQL missing %q", want)
	}

	// Trailing args after the builder's filter args: query, embedding, fanout.
	require.GreaterOrEqual(t, len(args), 4)
	assert.Equal(t, "dark mode", args[len(args)-3], "query is third-from-last arg")
	assert.Equal(t, hybridFanout, args[len(args)-1], "fanout is the last arg")
}
