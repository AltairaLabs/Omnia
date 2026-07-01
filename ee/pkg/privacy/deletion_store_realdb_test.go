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
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// deletionRequestsSchema mirrors migrations/000004_deletion_requests.up.sql. Kept
// inline (as the other real-DB tests in this package do) so the store test is
// self-contained; the embedded migration is guarded separately in the migrations
// package's embed test.
const deletionRequestsSchema = `
CREATE TABLE deletion_requests (
    id               TEXT        NOT NULL,
    virtual_user_id  TEXT        NOT NULL,
    reason           TEXT        NOT NULL,
    scope            TEXT        NOT NULL DEFAULT 'all',
    workspace        TEXT,
    date_from        TIMESTAMPTZ,
    date_to          TIMESTAMPTZ,
    status           TEXT        NOT NULL DEFAULT 'pending',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at       TIMESTAMPTZ,
    completed_at     TIMESTAMPTZ,
    sessions_deleted INTEGER     DEFAULT 0,
    errors           JSONB       DEFAULT '[]'::jsonb,
    CONSTRAINT deletion_requests_reason_check CHECK (reason IN ('gdpr_erasure', 'ccpa_delete', 'user_request')),
    CONSTRAINT deletion_requests_scope_check  CHECK (scope IN ('all', 'workspace', 'date_range')),
    CONSTRAINT deletion_requests_status_check CHECK (status IN ('pending', 'in_progress', 'completed', 'failed')),
    PRIMARY KEY (id)
)`

func TestPostgresDeletionStore_RoundTrip_RealPostgres(t *testing.T) {
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

	_, err = pool.Exec(ctx, deletionRequestsSchema)
	require.NoError(t, err)

	store := NewPostgresDeletionStore(pool)

	req := &DeletionRequest{
		ID:            "req-1",
		VirtualUserID: "vu-1",
		Reason:        "gdpr_erasure",
		Scope:         "all",
		Status:        StatusPending,
		CreatedAt:     time.Now().UTC(),
		Errors:        []string{},
	}
	require.NoError(t, store.CreateRequest(ctx, req))

	got, err := store.GetRequest(ctx, "req-1")
	require.NoError(t, err)
	require.Equal(t, "vu-1", got.VirtualUserID)
	require.Equal(t, StatusPending, got.Status)

	// Update to completed with a count + error, then re-read.
	now := time.Now().UTC()
	got.Status = StatusCompleted
	got.CompletedAt = &now
	got.SessionsDeleted = 3
	got.Errors = []string{"group-b memory: boom"}
	require.NoError(t, store.UpdateRequest(ctx, got))

	reread, err := store.GetRequest(ctx, "req-1")
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, reread.Status)
	require.Equal(t, 3, reread.SessionsDeleted)
	require.Equal(t, []string{"group-b memory: boom"}, reread.Errors)

	list, err := store.ListRequestsByUser(ctx, "vu-1")
	require.NoError(t, err)
	require.Len(t, list, 1)

	// Unknown id → ErrRequestNotFound.
	_, err = store.GetRequest(ctx, "nope")
	require.ErrorIs(t, err, ErrRequestNotFound)
}
