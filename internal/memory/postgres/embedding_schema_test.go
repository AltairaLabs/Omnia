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

package postgres

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// migratedPool returns a pgx pool against a fresh database that has the
// collapsed migration applied (so the embedding columns are absent at start).
func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	_, connStr := freshDB(t)
	mg, err := NewMigrator(connStr, zap.New(zap.UseDevMode(true)))
	require.NoError(t, err)
	require.NoError(t, mg.Up())
	require.NoError(t, mg.Close())

	pool, err := pgxpool.New(context.Background(), connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func colDim(t *testing.T, pool *pgxpool.Pool, table string) (int, bool) {
	t.Helper()
	dim, present, err := currentEmbeddingDim(context.Background(), pool, table)
	require.NoError(t, err)
	return dim, present
}

func hasHNSWIndex(t *testing.T, pool *pgxpool.Pool, table string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT EXISTS(SELECT 1 FROM pg_indexes
		WHERE schemaname = 'public' AND tablename = $1 AND indexdef ILIKE '%hnsw%')`,
		table).Scan(&exists))
	return exists
}

// seedObservationEmbedding inserts an entity + observation carrying a non-NULL
// embedding, so the destructive path has data to guard. Callers seed after
// EnsureEmbeddingSchema(768), so the vector is 768-dim to match the column.
func seedObservationEmbedding(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	const seededDim = 768
	ctx := context.Background()
	var entityID string
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO memory_entities (workspace_id, name, kind)
		VALUES (gen_random_uuid(), 'e', 'k') RETURNING id`).Scan(&entityID))
	_, err := pool.Exec(ctx, `
		INSERT INTO memory_observations (entity_id, content, embedding)
		VALUES ($1, 'hello', $2)`, entityID, pgvector.NewVector(make([]float32, seededDim)))
	require.NoError(t, err)
}

func insertConsent(t *testing.T, pool *pgxpool.Pool, targetDim int) {
	t.Helper()
	require.NoError(t, InsertDimensionChangeConsent(context.Background(), pool, targetDim, "test"))
}

func consentRows(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM memory_embedding_dim_change_consent`).Scan(&n))
	return n
}

func TestEnsureEmbeddingSchema_AbsentColumnsCreated(t *testing.T) {
	pool := migratedPool(t)

	require.NoError(t, EnsureEmbeddingSchema(context.Background(), pool, 768, logr.Discard()))

	for _, table := range []string{tableObservations, tableEntities} {
		dim, present := colDim(t, pool, table)
		assert.True(t, present, "%s.embedding should exist", table)
		assert.Equal(t, 768, dim, "%s.embedding dimension", table)
	}
	assert.True(t, hasHNSWIndex(t, pool, tableObservations), "observations should have HNSW index")
	assert.False(t, hasHNSWIndex(t, pool, tableEntities), "entities should not be indexed")
}

func TestEnsureEmbeddingSchema_NoOpWhenMatching(t *testing.T) {
	pool := migratedPool(t)
	ctx := context.Background()

	require.NoError(t, EnsureEmbeddingSchema(ctx, pool, 768, logr.Discard()))
	// Second run with the same dimension must be a clean no-op.
	require.NoError(t, EnsureEmbeddingSchema(ctx, pool, 768, logr.Discard()))

	dim, present := colDim(t, pool, tableObservations)
	assert.True(t, present)
	assert.Equal(t, 768, dim)
}

func TestEnsureEmbeddingSchema_EmptyReshapeUngated(t *testing.T) {
	pool := migratedPool(t)
	ctx := context.Background()

	require.NoError(t, EnsureEmbeddingSchema(ctx, pool, 768, logr.Discard()))
	// No embeddings present -> reshape to 1024 needs no consent.
	require.NoError(t, EnsureEmbeddingSchema(ctx, pool, 1024, logr.Discard()))

	dim, _ := colDim(t, pool, tableObservations)
	assert.Equal(t, 1024, dim)
}

func TestEnsureEmbeddingSchema_DestructiveWithoutMarkerFails(t *testing.T) {
	pool := migratedPool(t)
	ctx := context.Background()

	require.NoError(t, EnsureEmbeddingSchema(ctx, pool, 768, logr.Discard()))
	seedObservationEmbedding(t, pool)

	err := EnsureEmbeddingSchema(ctx, pool, 1024, logr.Discard())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires one-shot consent")

	dim, _ := colDim(t, pool, tableObservations)
	assert.Equal(t, 768, dim, "column must be unchanged on refusal")
}

func TestEnsureEmbeddingSchema_DestructiveWithMarkerReshapes(t *testing.T) {
	pool := migratedPool(t)
	ctx := context.Background()

	require.NoError(t, EnsureEmbeddingSchema(ctx, pool, 768, logr.Discard()))
	seedObservationEmbedding(t, pool)
	insertConsent(t, pool, 1024)

	require.NoError(t, EnsureEmbeddingSchema(ctx, pool, 1024, logr.Discard()))

	dim, _ := colDim(t, pool, tableObservations)
	assert.Equal(t, 1024, dim)
	assert.True(t, hasHNSWIndex(t, pool, tableObservations), "index rebuilt after reshape")

	// Embeddings discarded by the reshape.
	var withEmbedding int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM memory_observations WHERE embedding IS NOT NULL`).Scan(&withEmbedding))
	assert.Equal(t, 0, withEmbedding)

	// Marker consumed.
	assert.Equal(t, 0, consentRows(t, pool))
}

func TestEnsureEmbeddingSchema_MismatchedMarkerFails(t *testing.T) {
	pool := migratedPool(t)
	ctx := context.Background()

	require.NoError(t, EnsureEmbeddingSchema(ctx, pool, 768, logr.Discard()))
	seedObservationEmbedding(t, pool)
	insertConsent(t, pool, 512) // authorises a different target

	err := EnsureEmbeddingSchema(ctx, pool, 1024, logr.Discard())
	require.Error(t, err)

	dim, _ := colDim(t, pool, tableObservations)
	assert.Equal(t, 768, dim, "column unchanged on mismatch")
	assert.Equal(t, 1, consentRows(t, pool), "mismatched marker must not be consumed")
}

func TestEnsureEmbeddingSchema_StaleMarkerCleared(t *testing.T) {
	pool := migratedPool(t)
	ctx := context.Background()

	require.NoError(t, EnsureEmbeddingSchema(ctx, pool, 768, logr.Discard()))
	// A dangling marker for the dimension we're already at.
	insertConsent(t, pool, 768)

	require.NoError(t, EnsureEmbeddingSchema(ctx, pool, 768, logr.Discard()))
	assert.Equal(t, 0, consentRows(t, pool), "stale marker should be cleared on no-op reconcile")
}

func TestEnsureEmbeddingSchema_RejectsInvalidDimension(t *testing.T) {
	pool := migratedPool(t)
	require.Error(t, EnsureEmbeddingSchema(context.Background(), pool, 0, logr.Discard()))
}

func TestEnsureEmbeddingSchema_RejectsDimensionAboveIndexCap(t *testing.T) {
	pool := migratedPool(t)
	// pgvector HNSW indexes cap at MaxIndexableEmbeddingDim; a larger dimension
	// is rejected up front (clear error, no DDL) rather than crash-looping the
	// pod on a CREATE INDEX failure.
	err := EnsureEmbeddingSchema(context.Background(), pool, MaxIndexableEmbeddingDim+1, logr.Discard())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maximum indexable dimension")

	_, present := colDim(t, pool, tableObservations)
	assert.False(t, present, "no DDL should run when the dimension is rejected")
}

func TestEnsureEmbeddingSchema_ClearsStaleCrossDimMarker(t *testing.T) {
	pool := migratedPool(t)
	ctx := context.Background()

	require.NoError(t, EnsureEmbeddingSchema(ctx, pool, 768, logr.Discard()))
	seedObservationEmbedding(t, pool)
	// Operator records consent for 1024 but then never actually switches the
	// provider (changed their mind / reverted).
	insertConsent(t, pool, 1024)

	// A normal restart still at 768 must NOT leave the 1024 marker standing —
	// otherwise it would silently authorise a later, unrelated swap to 1024.
	require.NoError(t, EnsureEmbeddingSchema(ctx, pool, 768, logr.Discard()))
	assert.Equal(t, 0, consentRows(t, pool), "a stale marker for a different dimension must be cleared")
}

func TestInsertDimensionChangeConsent_RejectsInvalidDimension(t *testing.T) {
	pool := migratedPool(t)
	require.Error(t, InsertDimensionChangeConsent(context.Background(), pool, 0, "test"))
}

func TestCurrentEmbeddingDim_UnconstrainedVector(t *testing.T) {
	pool := migratedPool(t)
	_, err := pool.Exec(context.Background(), `ALTER TABLE memory_entities ADD COLUMN embedding vector`)
	require.NoError(t, err)

	dim, present, err := currentEmbeddingDim(context.Background(), pool, tableEntities)
	require.NoError(t, err)
	assert.True(t, present, "unconstrained vector column is present")
	assert.Equal(t, 0, dim, "unconstrained vector reports dim 0")
}

// TestEmbeddingSchema_ErrorPaths drives the query/exec failure branches with a
// cancelled context so the error-wrapping code is exercised.
func TestEmbeddingSchema_ErrorPaths(t *testing.T) {
	pool := migratedPool(t)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := currentEmbeddingDim(canceled, pool, tableObservations)
	assert.Error(t, err)

	_, err = hasEmbeddings(canceled, pool, embeddingTables[0])
	assert.Error(t, err)

	assert.Error(t, ensureConsentTable(canceled, pool))

	_, _, err = readConsent(canceled, pool)
	assert.Error(t, err)

	assert.Error(t, consumeConsent(canceled, pool))
	assert.Error(t, clearStaleConsent(canceled, pool))
	assert.Error(t, InsertDimensionChangeConsent(canceled, pool, 768, "test"))
	assert.Error(t, EnsureEmbeddingSchema(canceled, pool, 768, logr.Discard()))

	_, err = needsDestructiveReshape(canceled, pool, 768)
	assert.Error(t, err)
}
