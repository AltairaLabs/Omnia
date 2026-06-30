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

	"github.com/altairalabs/omnia/ee/pkg/audit"
)

// auditLogSchema mirrors migrations/000003_audit_log.up.sql. Kept inline (as the
// other real-DB tests in this package do) so the store test is self-contained.
const auditLogSchema = `
CREATE TABLE audit_log (
    id             BIGSERIAL    PRIMARY KEY,
    source_service TEXT         NOT NULL,
    source_id      BIGINT       NOT NULL,
    "timestamp"    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    event_type     TEXT         NOT NULL,
    session_id     TEXT,
    user_id        TEXT,
    workspace      TEXT,
    agent_name     TEXT,
    namespace      TEXT,
    query          TEXT,
    result_count   INTEGER,
    ip_address     TEXT,
    user_agent     TEXT,
    reason         TEXT,
    metadata       JSONB        NOT NULL DEFAULT '{}',
    UNIQUE (source_service, source_id)
)`

func TestAuditStore_InsertEvents_RealPostgres(t *testing.T) {
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

	_, err = pool.Exec(ctx, auditLogSchema)
	require.NoError(t, err)

	store := NewAuditStore(pool)
	now := time.Now().UTC()
	events := []*audit.Entry{
		{ID: 1, Timestamp: now, EventType: audit.EventMemoryWriteBlocked, Workspace: "ws-uid", UserID: "u1",
			Metadata: map[string]string{"category": "memory:health"}},
		{ID: 2, Timestamp: now, EventType: audit.EventPIIRedacted, Workspace: "ws-uid"},
	}

	// First delivery: both rows new.
	n, err := store.InsertEvents(ctx, "memory-api", events)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	// At-least-once redelivery of the same source rows: idempotent, 0 new.
	n, err = store.InsertEvents(ctx, "memory-api", events)
	require.NoError(t, err)
	require.Equal(t, 0, n)

	// Same source ids from a different service are distinct (composite key).
	n, err = store.InsertEvents(ctx, "session-api", events)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	// Empty batch and nil entries are no-ops, not errors.
	n, err = store.InsertEvents(ctx, "memory-api", nil)
	require.NoError(t, err)
	require.Equal(t, 0, n)
	n, err = store.InsertEvents(ctx, "memory-api", []*audit.Entry{nil})
	require.NoError(t, err)
	require.Equal(t, 0, n)

	// Total rows = 4 (2 per service), and metadata round-trips as JSONB.
	var total int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM audit_log").Scan(&total))
	require.Equal(t, 4, total)

	var category string
	require.NoError(t, pool.QueryRow(ctx,
		"SELECT metadata->>'category' FROM audit_log WHERE source_service='memory-api' AND source_id=1").
		Scan(&category))
	require.Equal(t, "memory:health", category)

	// Missing sourceService is rejected.
	_, err = store.InsertEvents(ctx, "", events)
	require.Error(t, err)
}
