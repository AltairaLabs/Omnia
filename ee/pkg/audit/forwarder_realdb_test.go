/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package audit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// localAuditLogSchema mirrors the memory-api / session-api local audit_log
// (INET ip_address, UUID session_id) plus the forwarded_at column added by the
// drain-forwarder migrations. Kept inline so the test is self-contained.
const localAuditLogSchema = `
CREATE TABLE audit_log (
    id           BIGSERIAL    PRIMARY KEY,
    "timestamp"  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    event_type   TEXT         NOT NULL,
    session_id   UUID,
    user_id      TEXT,
    workspace    TEXT,
    agent_name   TEXT,
    namespace    TEXT,
    query        TEXT,
    result_count INTEGER,
    ip_address   INET,
    user_agent   TEXT,
    reason       TEXT,
    metadata     JSONB        DEFAULT '{}',
    forwarded_at TIMESTAMPTZ
)`

func TestForwarder_DrainOnce_RealPostgres(t *testing.T) {
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

	_, err = pool.Exec(ctx, localAuditLogSchema)
	require.NoError(t, err)

	// Seed rows: two with INET/UUID values to exercise the host()/text cast,
	// one already-forwarded row that must NOT be re-sent.
	_, err = pool.Exec(ctx, `
		INSERT INTO audit_log (event_type, session_id, user_id, workspace, ip_address, metadata, forwarded_at) VALUES
		('memory_write_blocked', gen_random_uuid(), 'u1', 'ws-uid', '10.0.0.1', '{"category":"memory:health"}', NULL),
		('pii_redacted',         NULL,              'u2', 'ws-uid', '10.0.0.2', '{}',                            NULL),
		('pii_redacted',         NULL,              'u3', 'ws-uid', NULL,        '{}',                            now())`)
	require.NoError(t, err)

	var captured struct {
		mu     sync.Mutex
		bodies []forwardRequest
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req forwardRequest
		require.NoError(t, json.Unmarshal(body, &req))
		captured.mu.Lock()
		captured.bodies = append(captured.bodies, req)
		captured.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	f := NewForwarder(pool, srv.URL, "memory-api", nil, time.Hour, 50,
		prometheus.NewRegistry(), zap.New(zap.UseDevMode(true)))
	f.drainOnce(ctx)

	// Exactly the two unforwarded rows were shipped; the pre-forwarded row stayed.
	captured.mu.Lock()
	require.Len(t, captured.bodies, 1)
	require.Len(t, captured.bodies[0].Events, 2)
	require.Equal(t, "10.0.0.1", captured.bodies[0].Events[0].IPAddress)
	require.Equal(t, "ws-uid", captured.bodies[0].Events[0].Workspace)
	captured.mu.Unlock()

	// All rows are now marked forwarded → backlog empty.
	var unforwarded int
	require.NoError(t, pool.QueryRow(ctx,
		"SELECT count(*) FROM audit_log WHERE forwarded_at IS NULL").Scan(&unforwarded))
	require.Equal(t, 0, unforwarded)

	// A second drain ships nothing (idempotent at the source).
	f.drainOnce(ctx)
	captured.mu.Lock()
	require.Len(t, captured.bodies, 1, "no further POSTs once drained")
	captured.mu.Unlock()
}
