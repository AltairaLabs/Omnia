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
