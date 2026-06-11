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

	// Embedding columns are reconciler-owned (#1309); the prefilter reads
	// memory_entities.embedding, so materialise the columns before use.
	require.NoError(t, EnsureEmbeddingSchema(context.Background(), pool, 1536, logger))
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

// seedEntityFor is like seedEntity but takes per-user agent_id NULL (the
// cross-scope SQL filters virtual_user_id IS NOT NULL). Returns the
// new entity's UUID.
func seedEntityFor(t *testing.T, pool *pgxpool.Pool, workspaceID, virtualUserID, kind, name string) string {
	return seedEntity(t, pool, workspaceID, virtualUserID, kind, name)
}

func TestRunCrossScopeCandidates_DecodesContentAndCountsDistinctUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := freshPgxPool(t)
	wsID := "00000000-0000-0000-0000-00000000cc01"

	// Three users with the same (kind, name) entity; one observation each.
	// Plus one user with a different (kind, name) — should not appear.
	e1 := seedEntityFor(t, pool, wsID, "u-alpha", "preference", "units")
	e2 := seedEntityFor(t, pool, wsID, "u-beta", "preference", "units")
	e3 := seedEntityFor(t, pool, wsID, "u-gamma", "preference", "units")
	other := seedEntityFor(t, pool, wsID, "u-alpha", "fact", "other")
	now := time.Now()
	o1 := seedObservation(t, pool, e1, "metric", "mutable", now.Add(-1*time.Hour))
	o2 := seedObservation(t, pool, e2, "imperial", "mutable", now.Add(-2*time.Hour))
	o3 := seedObservation(t, pool, e3, "metric", "mutable", now.Add(-3*time.Hour))
	_ = seedObservation(t, pool, other, "should-not-surface", "mutable", now.Add(-1*time.Hour))

	r := NewPreFilterRunner(pool)
	buckets, err := r.RunCrossScopeCandidates(context.Background(), consolidation.PreFilterOptions{
		WorkspaceID:       wsID,
		MinDistinctUsers:  3,
		MaxBucketsPerPass: 10,
		MaxPerBucket:      10,
	})
	require.NoError(t, err)
	require.Len(t, buckets, 1, "want one bucket meeting k-anonymity")

	b := buckets[0]
	assert.Equal(t, "kind=preference;name=units", b.Key)
	assert.Equal(t, 3, b.Stats["distinctUsers"])

	// Content must be decoded — the v1 adapter dropped it.
	gotContent := map[string]string{}
	gotUser := map[string]string{}
	gotMutability := map[string]string{}
	for _, e := range b.Entries {
		gotContent[e.ID] = e.Content
		gotUser[e.ID] = e.Scope.UserID
		gotMutability[e.ID] = e.Mutability
	}
	assert.Equal(t, "metric", gotContent[o1])
	assert.Equal(t, "imperial", gotContent[o2])
	assert.Equal(t, "metric", gotContent[o3])
	assert.Equal(t, "u-alpha", gotUser[o1])
	assert.Equal(t, "u-beta", gotUser[o2])
	assert.Equal(t, "u-gamma", gotUser[o3])
	for _, id := range []string{o1, o2, o3} {
		assert.Equal(t, "mutable", gotMutability[id])
	}
}

func TestRunCrossScopeCandidates_FiltersBelowKAnonymity(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := freshPgxPool(t)
	wsID := "00000000-0000-0000-0000-00000000cc02"

	// Only 2 distinct users for (preference, units) — below k=3 threshold.
	e1 := seedEntityFor(t, pool, wsID, "u-1", "preference", "units")
	e2 := seedEntityFor(t, pool, wsID, "u-2", "preference", "units")
	_ = seedObservation(t, pool, e1, "a", "mutable", time.Now())
	_ = seedObservation(t, pool, e2, "b", "mutable", time.Now())

	r := NewPreFilterRunner(pool)
	buckets, err := r.RunCrossScopeCandidates(context.Background(), consolidation.PreFilterOptions{
		WorkspaceID:       wsID,
		MinDistinctUsers:  3,
		MaxBucketsPerPass: 10,
		MaxPerBucket:      10,
	})
	require.NoError(t, err)
	assert.Empty(t, buckets, "below k-anonymity should produce no buckets")
}

func TestRunCrossScopeCandidates_ExcludesRegulatedAndNonMutable(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := freshPgxPool(t)
	wsID := "00000000-0000-0000-0000-00000000cc03"
	e1 := seedEntityFor(t, pool, wsID, "u-1", "fact", "k")
	e2 := seedEntityFor(t, pool, wsID, "u-2", "fact", "k")
	e3 := seedEntityFor(t, pool, wsID, "u-3", "fact", "k")
	_ = seedObservation(t, pool, e1, "ok", "mutable", time.Now())
	_ = seedObservation(t, pool, e2, "ok-too", "mutable", time.Now())
	// regulated row for u-3 should NOT count toward distinct users
	_, err := pool.Exec(context.Background(),
		`INSERT INTO memory_observations (entity_id, content, mutability, source_type, observed_at)
         VALUES ($1, 'regulated-row', 'mutable', 'regulated', NOW())`, e3)
	require.NoError(t, err)
	// immutable row for u-3 should also not count
	_, err = pool.Exec(context.Background(),
		`INSERT INTO memory_observations (entity_id, content, mutability, observed_at)
         VALUES ($1, 'immutable-row', 'immutable', NOW())`, e3)
	require.NoError(t, err)

	r := NewPreFilterRunner(pool)
	buckets, err := r.RunCrossScopeCandidates(context.Background(), consolidation.PreFilterOptions{
		WorkspaceID:       wsID,
		MinDistinctUsers:  3,
		MaxBucketsPerPass: 10,
		MaxPerBucket:      10,
	})
	require.NoError(t, err)
	assert.Empty(t, buckets, "only 2 mutable+non-regulated users → below k=3")
}
