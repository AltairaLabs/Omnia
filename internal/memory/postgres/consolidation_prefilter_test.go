/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/altairalabs/omnia/internal/memory/consolidation"
)

// freshPgxPool creates a fresh database, runs all migrations, and returns
// a pgxpool against it. Cleanup is registered with t.Cleanup.
func freshPgxPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	_, connStr := freshDB(t)
	logger := zap.New(zap.UseDevMode(true))
	mg, err := NewMigrator(connStr, logger)
	require.NoError(t, err)
	require.NoError(t, mg.Up())
	require.NoError(t, mg.Close())

	pool, err := pgxpool.New(context.Background(), connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

// seedEntity inserts a memory_entities row and returns its UUID. virtualUserID
// may be empty.
func seedEntity(t *testing.T, pool *pgxpool.Pool, workspaceID, virtualUserID, kind, name string) string {
	t.Helper()
	var id string
	var vu any
	if virtualUserID != "" {
		vu = virtualUserID
	}
	err := pool.QueryRow(context.Background(), `
		INSERT INTO memory_entities (workspace_id, virtual_user_id, kind, name)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text`,
		workspaceID, vu, kind, name,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

// seedObservation inserts a memory_observations row under entityID with the
// given content, mutability, and observed_at timestamp. Returns the new UUID.
func seedObservation(t *testing.T, pool *pgxpool.Pool, entityID, content, mutability string, observedAt time.Time) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO memory_observations (entity_id, content, mutability, observed_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text`,
		entityID, content, mutability, observedAt,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestRunStaleObservations_DecodesContentAndMutability(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := freshPgxPool(t)
	wsID := "00000000-0000-0000-0000-00000000c001"
	entityID := seedEntity(t, pool, wsID, "user-1", "fact", "favorite_color")
	stale1 := seedObservation(t, pool, entityID, "blue", "mutable", time.Now().Add(-60*24*time.Hour))
	stale2 := seedObservation(t, pool, entityID, "azure", "mutable", time.Now().Add(-50*24*time.Hour))

	r := NewPreFilterRunner(pool)
	buckets, err := r.RunStaleObservations(context.Background(), consolidation.PreFilterOptions{
		WorkspaceID:       wsID,
		OlderThan:         time.Now().Add(-30 * 24 * time.Hour),
		MinGroupSize:      1,
		MaxBucketsPerPass: 10,
		MaxPerBucket:      10,
	})
	require.NoError(t, err)
	require.Len(t, buckets, 1, "want exactly one bucket")

	gotContent := map[string]string{}
	gotMutability := map[string]string{}
	gotSourceType := map[string]string{}
	gotObservedAt := map[string]time.Time{}
	for _, e := range buckets[0].Entries {
		gotContent[e.ID] = e.Content
		gotMutability[e.ID] = e.Mutability
		gotSourceType[e.ID] = e.SourceType
		gotObservedAt[e.ID] = e.ObservedAt
	}
	assert.Equal(t, "blue", gotContent[stale1], "content for stale1 should be decoded")
	assert.Equal(t, "azure", gotContent[stale2], "content for stale2 should be decoded")
	for _, id := range []string{stale1, stale2} {
		assert.Equal(t, "mutable", gotMutability[id], "mutability for %s", id)
		assert.NotEmpty(t, gotSourceType[id], "source_type for %s", id)
		assert.False(t, gotObservedAt[id].IsZero(), "observed_at for %s", id)
	}
}

func TestRunStaleObservations_ExcludesNonMutableAndRegulated(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := freshPgxPool(t)
	wsID := "00000000-0000-0000-0000-00000000c002"
	entityID := seedEntity(t, pool, wsID, "user-1", "fact", "k")
	_ = seedObservation(t, pool, entityID, "mutable-row", "mutable", time.Now().Add(-60*24*time.Hour))
	_ = seedObservation(t, pool, entityID, "immutable-row", "immutable", time.Now().Add(-60*24*time.Hour))

	// Insert a regulated source_type row directly (helper defaults to conversation_extraction).
	var regID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO memory_observations (entity_id, content, mutability, source_type, observed_at)
		VALUES ($1, 'regulated-row', 'mutable', 'regulated', $2)
		RETURNING id::text`,
		entityID, time.Now().Add(-60*24*time.Hour),
	).Scan(&regID)
	require.NoError(t, err)

	r := NewPreFilterRunner(pool)
	buckets, err := r.RunStaleObservations(context.Background(), consolidation.PreFilterOptions{
		WorkspaceID:       wsID,
		OlderThan:         time.Now().Add(-30 * 24 * time.Hour),
		MinGroupSize:      1,
		MaxBucketsPerPass: 10,
		MaxPerBucket:      10,
	})
	require.NoError(t, err)
	// Only the mutable, non-regulated row should surface.
	var got []string
	for _, b := range buckets {
		for _, e := range b.Entries {
			got = append(got, e.Content)
		}
	}
	assert.Equal(t, []string{"mutable-row"}, got)
}

func TestRunStaleObservations_AppliesMaxPerBucket(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := freshPgxPool(t)
	wsID := "00000000-0000-0000-0000-00000000c003"
	entityID := seedEntity(t, pool, wsID, "user-1", "fact", "k")
	for i := 0; i < 7; i++ {
		_ = seedObservation(t, pool, entityID, "obs", "mutable", time.Now().Add(-time.Duration(60-i)*24*time.Hour))
	}

	r := NewPreFilterRunner(pool)
	buckets, err := r.RunStaleObservations(context.Background(), consolidation.PreFilterOptions{
		WorkspaceID:       wsID,
		OlderThan:         time.Now().Add(-30 * 24 * time.Hour),
		MinGroupSize:      1,
		MaxBucketsPerPass: 10,
		MaxPerBucket:      3,
	})
	require.NoError(t, err)
	require.Len(t, buckets, 1)
	assert.LessOrEqual(t, len(buckets[0].Entries), 3, "MaxPerBucket should cap entries")
}
