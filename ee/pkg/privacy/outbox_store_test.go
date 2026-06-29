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

// outboxTestPool spins up a throwaway Postgres, creates the required schema
// (user_privacy_preferences + consent_revocation_outbox), and returns a pool.
func outboxTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
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

	// Minimal schema: exactly what the privacy-api 000001 + 000002 migrations create.
	_, err = pool.Exec(ctx, `
		CREATE TABLE user_privacy_preferences (
		    user_id            TEXT        PRIMARY KEY,
		    opt_out_all        BOOLEAN     DEFAULT FALSE,
		    opt_out_workspaces TEXT[]      DEFAULT '{}',
		    opt_out_agents     TEXT[]      DEFAULT '{}',
		    consent_grants     TEXT[]      DEFAULT '{}',
		    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
		    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		CREATE TABLE consent_revocation_outbox (
		    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
		    user_id      TEXT        NOT NULL,
		    category     TEXT        NOT NULL,
		    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
		    delivered_at TIMESTAMPTZ
		)`)
	require.NoError(t, err)

	return pool
}

// seedConsentGrant inserts a preferences row granting the given category.
func seedConsentGrant(t *testing.T, pool *pgxpool.Pool, userID string, category ConsentCategory) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO user_privacy_preferences (user_id, consent_grants)
		 VALUES ($1, ARRAY[$2]::TEXT[])
		 ON CONFLICT (user_id) DO UPDATE
		    SET consent_grants = array_append(user_privacy_preferences.consent_grants, $2)`,
		userID, string(category))
	require.NoError(t, err)
}

// outboxRowCount returns the total number of rows in consent_revocation_outbox.
func outboxRowCount(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var n int64
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM consent_revocation_outbox`).Scan(&n)
	require.NoError(t, err)
	return n
}

func TestOutboxStore_RemoveConsentGrantWithOutbox_RealPostgres(t *testing.T) {
	pool := outboxTestPool(t)
	ctx := context.Background()
	store := NewPreferencesStore(pool)

	const (
		knownUser = "user-outbox-1"
		category  = ConsentMemoryHealth
	)

	// Seed a prefs row with the category granted.
	seedConsentGrant(t, pool, knownUser, category)

	// 1. First revocation: should succeed and return a non-empty outbox id.
	outboxID, err := store.RemoveConsentGrantWithOutbox(ctx, knownUser, category)
	require.NoError(t, err)
	assert.NotEmpty(t, outboxID, "expected non-empty outbox id on first revocation")

	// consent_grants should no longer contain the category.
	grants, err := store.GetConsentGrants(ctx, knownUser)
	require.NoError(t, err)
	assert.NotContains(t, grants, category, "category should be removed from consent_grants")

	// Exactly one undelivered outbox row should exist.
	assert.Equal(t, int64(1), outboxRowCount(t, pool), "expected exactly one outbox row")

	var deliveredAt *time.Time
	err = pool.QueryRow(ctx,
		`SELECT delivered_at FROM consent_revocation_outbox WHERE id = $1`, outboxID,
	).Scan(&deliveredAt)
	require.NoError(t, err)
	assert.Nil(t, deliveredAt, "delivered_at should be NULL for a new outbox row")

	// 2. Second call on already-revoked category: no-op, returns ("", nil).
	id2, err := store.RemoveConsentGrantWithOutbox(ctx, knownUser, category)
	require.NoError(t, err)
	assert.Empty(t, id2, "expected empty id for a no-op revocation")
	assert.Equal(t, int64(1), outboxRowCount(t, pool), "no new outbox row should be created on no-op")

	// 3. Unknown user: no-op, returns ("", nil).
	id3, err := store.RemoveConsentGrantWithOutbox(ctx, "no-such-user", category)
	require.NoError(t, err)
	assert.Empty(t, id3, "expected empty id for unknown user")
	assert.Equal(t, int64(1), outboxRowCount(t, pool), "no outbox row should be created for unknown user")
}

func TestOutboxStore_MarkOutboxDelivered_RealPostgres(t *testing.T) {
	pool := outboxTestPool(t)
	ctx := context.Background()
	store := NewPreferencesStore(pool)

	const (
		user     = "user-outbox-2"
		category = ConsentMemoryHealth
	)
	seedConsentGrant(t, pool, user, category)

	outboxID, err := store.RemoveConsentGrantWithOutbox(ctx, user, category)
	require.NoError(t, err)
	require.NotEmpty(t, outboxID)

	// 4. MarkOutboxDelivered: delivered_at should now be set.
	require.NoError(t, store.MarkOutboxDelivered(ctx, outboxID))

	var deliveredAt *time.Time
	err = pool.QueryRow(ctx,
		`SELECT delivered_at FROM consent_revocation_outbox WHERE id = $1`, outboxID,
	).Scan(&deliveredAt)
	require.NoError(t, err)
	assert.NotNil(t, deliveredAt, "delivered_at should be set after MarkOutboxDelivered")
}

func TestOutboxStore_ListUndeliveredOutbox_RealPostgres(t *testing.T) {
	pool := outboxTestPool(t)
	ctx := context.Background()
	store := NewPreferencesStore(pool)

	const (
		user1    = "user-outbox-3a"
		user2    = "user-outbox-3b"
		category = ConsentMemoryHealth
	)
	seedConsentGrant(t, pool, user1, category)
	seedConsentGrant(t, pool, user2, category)

	id1, err := store.RemoveConsentGrantWithOutbox(ctx, user1, category)
	require.NoError(t, err)
	require.NotEmpty(t, id1)
	id2, err := store.RemoveConsentGrantWithOutbox(ctx, user2, category)
	require.NoError(t, err)
	require.NotEmpty(t, id2)

	// Mark id1 as delivered.
	require.NoError(t, store.MarkOutboxDelivered(ctx, id1))

	// 5. ListUndeliveredOutbox should return only the undelivered row.
	entries, err := store.ListUndeliveredOutbox(ctx, time.Hour, 10)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only one undelivered row expected")
	assert.Equal(t, id2, entries[0].ID)
	assert.Equal(t, user2, entries[0].UserID)
	assert.Equal(t, category, entries[0].Category)
}

func TestOutboxStore_PruneDeliveredOutbox_RealPostgres(t *testing.T) {
	pool := outboxTestPool(t)
	ctx := context.Background()
	store := NewPreferencesStore(pool)

	const (
		user1    = "user-outbox-4a"
		user2    = "user-outbox-4b"
		category = ConsentMemoryHealth
	)
	seedConsentGrant(t, pool, user1, category)
	seedConsentGrant(t, pool, user2, category)

	id1, err := store.RemoveConsentGrantWithOutbox(ctx, user1, category)
	require.NoError(t, err)
	require.NotEmpty(t, id1)
	id2, err := store.RemoveConsentGrantWithOutbox(ctx, user2, category)
	require.NoError(t, err)
	require.NotEmpty(t, id2)

	// Mark id1 delivered.
	require.NoError(t, store.MarkOutboxDelivered(ctx, id1))

	// 6. PruneDeliveredOutbox(0) — any delivered row qualifies (delivered < now()-0s).
	// Move delivered_at far enough into the past so it's older than 0s.
	_, err = pool.Exec(ctx,
		`UPDATE consent_revocation_outbox SET delivered_at = now() - interval '1 second' WHERE id = $1`, id1)
	require.NoError(t, err)

	deleted, err := store.PruneDeliveredOutbox(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted, "expected exactly one delivered row pruned")

	// Undelivered row must still exist.
	assert.Equal(t, int64(1), outboxRowCount(t, pool), "undelivered row must survive prune")
}

// TestOutboxStore_RemoveConsentGrantWithOutbox_RollbackOnInsertFailure_RealPostgres
// verifies that when the INSERT into consent_revocation_outbox fails (here forced
// by dropping the table before the call), the UPDATE to consent_grants is also
// rolled back — the category is still present in the prefs row after the error.
func TestOutboxStore_RemoveConsentGrantWithOutbox_RollbackOnInsertFailure_RealPostgres(t *testing.T) {
	pool := outboxTestPool(t)
	ctx := context.Background()
	store := NewPreferencesStore(pool)

	const (
		rollbackUser     = "user-outbox-rollback"
		rollbackCategory = ConsentMemoryHealth
	)

	// Seed a prefs row with the category granted.
	seedConsentGrant(t, pool, rollbackUser, rollbackCategory)

	// Verify the category is present before the test runs.
	grantsBefore, err := store.GetConsentGrants(ctx, rollbackUser)
	require.NoError(t, err)
	require.Contains(t, grantsBefore, rollbackCategory, "setup: category must be granted before the call")

	// Force the outbox INSERT to fail by dropping the table.
	// This container is isolated to this test, so the DROP does not affect siblings.
	_, err = pool.Exec(ctx, `DROP TABLE consent_revocation_outbox`)
	require.NoError(t, err)

	// The call must return an error (INSERT fails → tx rolls back).
	outboxID, callErr := store.RemoveConsentGrantWithOutbox(ctx, rollbackUser, rollbackCategory)
	require.Error(t, callErr, "expected an error when the outbox INSERT fails")
	assert.Empty(t, outboxID, "expected empty outbox id on failure")

	// Critical assertion: the UPDATE must have been rolled back.
	// consent_grants must still contain the category.
	grantsAfter, err := store.GetConsentGrants(ctx, rollbackUser)
	require.NoError(t, err)
	assert.Contains(t, grantsAfter, rollbackCategory,
		"consent_grants must still contain the category — UPDATE must have rolled back")
}

func TestOutboxStore_CountStuckOutbox_RealPostgres(t *testing.T) {
	pool := outboxTestPool(t)
	ctx := context.Background()
	store := NewPreferencesStore(pool)

	const (
		user     = "user-outbox-5"
		category = ConsentMemoryHealth
	)
	seedConsentGrant(t, pool, user, category)

	outboxID, err := store.RemoveConsentGrantWithOutbox(ctx, user, category)
	require.NoError(t, err)
	require.NotEmpty(t, outboxID)

	// 7. CountStuckOutbox(0): move created_at just into the past so it qualifies.
	_, err = pool.Exec(ctx,
		`UPDATE consent_revocation_outbox SET created_at = now() - interval '1 second' WHERE id = $1`, outboxID)
	require.NoError(t, err)

	count, err := store.CountStuckOutbox(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count, "expected one stuck undelivered row")
}
