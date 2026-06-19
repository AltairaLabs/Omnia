/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestPreferencesStore_Stats_RealPostgres runs Stats() against a real Postgres
// so the query's SQL is actually parsed and executed. The mock-pool tests can't
// catch SQL errors (the mock ignores the query string entirely), which is how
// the reserved-word bug in #1490 -- "grant" used as an unquoted identifier,
// SQLSTATE 42601 -- reached production. Before the fix this test fails on the
// Stats() call; after it, the aggregates are returned correctly.
func TestPreferencesStore_Stats_RealPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("requires a Docker Postgres; skipped under -short")
	}
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "pgvector/pgvector:pg16",
		tcpostgres.WithDatabase("omnia_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	// Minimal schema covering exactly what Stats() reads.
	_, err = pool.Exec(ctx, `
		CREATE TABLE user_privacy_preferences (
		    user_id        TEXT PRIMARY KEY,
		    opt_out_all    BOOLEAN NOT NULL DEFAULT FALSE,
		    consent_grants TEXT[]  NOT NULL DEFAULT '{}'
		)`)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO user_privacy_preferences (user_id, opt_out_all, consent_grants) VALUES
		    ($1, FALSE, $2),
		    ($3, FALSE, $4),
		    ($5, TRUE,  $6)`,
		"u1", []string{string(ConsentMemoryContext), string(ConsentAnalyticsAggregate)},
		"u2", []string{string(ConsentMemoryContext)},
		"u3", []string{},
	)
	require.NoError(t, err)

	store := NewPreferencesStore(pool)
	stats, err := store.Stats(ctx)
	require.NoError(t, err) // fails with SQLSTATE 42601 before the #1490 fix

	assert.Equal(t, int64(3), stats.TotalUsers)
	assert.Equal(t, int64(1), stats.OptedOutAll)
	assert.Equal(t, map[string]int64{
		string(ConsentMemoryContext):      2,
		string(ConsentAnalyticsAggregate): 1,
	}, stats.GrantsByCategory)
}
