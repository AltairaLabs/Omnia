/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	pgvector "github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const efSearchTargetContent = "the quick brown fox jumps"

// insertHybridMemoryWS seeds an institutional entity+observation with an
// embedding into an arbitrary workspace (insertHybridMemory hardcodes
// testWorkspace1), so a test can spread rows across workspaces in one DB.
func insertHybridMemoryWS(t *testing.T, store *PostgresMemoryStore, workspace, kind, content string, conf float64, emb []float32) {
	t.Helper()
	ctx := context.Background()
	var entityID string
	require.NoError(t, store.pool.QueryRow(ctx,
		`INSERT INTO memory_entities (workspace_id, name, kind, metadata)
		 VALUES ($1, $2, $3, '{}') RETURNING id`,
		workspace, content, kind).Scan(&entityID))
	_, err := store.pool.Exec(ctx,
		`INSERT INTO memory_observations (entity_id, content, confidence, embedding)
		 VALUES ($1, $2, $3, $4)`,
		entityID, content, conf, pgvector.NewVector(emb))
	require.NoError(t, err)
}

// TestWithHNSWEFSearch_AppliesAndIsTransactionLocal proves the wrapper raises
// hnsw.ef_search inside the transaction (resolving pgvector's lazy-GUC nuance:
// the setting takes effect even on a fresh pooled connection) and that the
// change is scoped to that transaction only.
func TestWithHNSWEFSearch_AppliesAndIsTransactionLocal(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	var inside string
	require.NoError(t, store.withHNSWEFSearch(ctx, 321, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SHOW hnsw.ef_search").Scan(&inside)
	}))
	assert.Equal(t, "321", inside, "ef_search must be raised inside the tx")

	var outside string
	require.NoError(t, store.pool.QueryRow(ctx, "SHOW hnsw.ef_search").Scan(&outside))
	assert.NotEqual(t, "321", outside, "SET LOCAL must not leak past the tx")
}

// TestWithHNSWEFSearch_ErrorPaths covers the wrapper's failure branches: a
// callback error is propagated (and the tx rolled back), and a canceled context
// fails at BEGIN before the callback runs.
func TestWithHNSWEFSearch_ErrorPaths(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	sentinel := errors.New("boom")
	require.ErrorIs(t, store.withHNSWEFSearch(ctx, 100, func(pgx.Tx) error { return sentinel }), sentinel)

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	require.Error(t, store.withHNSWEFSearch(canceled, 100, func(pgx.Tx) error { return nil }))
}

// TestEFSearchWrappedQueries_SurfaceQueryErrors drives the query-error branch
// inside each ef_search-wrapped cosine path. A vector whose dimension differs
// from the indexed column makes the `<=>` comparison fail at execution (one row
// must exist for the operator to evaluate), so the error must surface from
// inside the transaction wrapper.
func TestEFSearchWrappedQueries_SurfaceQueryErrors(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := map[string]string{ScopeWorkspaceID: testWorkspace1}
	insertHybridMemoryWS(t, store, testWorkspace1, hybridKindFact, "a row to compare against", 0.9, oneHotFloat(1, 1536))

	wrongDim := []float32{1, 2, 3} // column is 1536-dim; the <=> comparison fails

	_, err := store.FindSimilarObservations(ctx, scope, wrongDim, 5, 0.5)
	require.Error(t, err, "dimension-mismatch query error must surface")

	_, err = store.RetrieveHybrid(ctx, scope, "q", wrongDim, RetrieveOptions{Limit: 5})
	require.Error(t, err)

	_, err = store.RetrieveMultiTierHybrid(ctx, MultiTierRequest{
		WorkspaceID: testWorkspace1, Query: "q", Limit: 5,
	}, wrongDim)
	require.Error(t, err)
}

// TestClampEFSearch bounds the candidate-list size to pgvector's accepted range.
func TestClampEFSearch(t *testing.T) {
	assert.Equal(t, minHNSWEFSearch, clampEFSearch(10), "below floor clamps up")
	assert.Equal(t, 250, clampEFSearch(250), "in-range passes through")
	assert.Equal(t, maxHNSWEFSearch, clampEFSearch(5000), "above ceiling clamps down")
}

// TestRetrieveHybrid_MultiWorkspaceRecallThroughEFSearchWrapper exercises the
// ef_search-wrapped cosine path with many foreign-workspace rows present in the
// same database: workspace 2 holds dozens of observations at cosine distance 0
// to the query, while workspace 1 holds the one semantically-near, lexically
// disjoint target. The wrapped query must surface the workspace-1 target and
// none of workspace 2's rows.
//
// Note: this asserts correctness, not the HNSW candidate-list cap itself — at
// unit-test scale Postgres does not choose the HNSW index for either the
// windowed hybrid arm (its row_number() partition forces a full materialise) or
// the plain ANN query (btree+sort is cheaper for a few rows), so ef_search has
// no observable effect on the result set here. The wrapper's effect is proven
// directly by TestWithHNSWEFSearch_AppliesAndIsTransactionLocal; the value at
// production scale is the clean ANN path (FindSimilarObservations).
func TestRetrieveHybrid_MultiWorkspaceRecallThroughEFSearchWrapper(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	query := oneHotFloat(7, 1536)
	for i := 0; i < 60; i++ {
		insertHybridMemoryWS(t, store, testWorkspace2, hybridKindFact, "unrelated filler row", 0.9, query)
	}
	target := make([]float32, 1536)
	target[7] = 0.7071
	target[8] = 0.7071
	insertHybridMemoryWS(t, store, testWorkspace1, hybridKindFact, efSearchTargetContent, 0.9, target)

	const queryText = "zzqnomatch" // shares no FTS token with any content
	scope := map[string]string{ScopeWorkspaceID: testWorkspace1}

	got, err := store.RetrieveHybrid(ctx, scope, queryText, query, RetrieveOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, got, 1, "must surface the workspace-1 target and isolate workspace-2 rows")
	assert.Equal(t, efSearchTargetContent, got[0].Content)
}
