/*
Copyright 2025.

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
	"database/sql"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx driver for database/sql
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var testConnStr string

func TestMain(m *testing.M) {
	flag.Parse()

	if testing.Short() {
		os.Exit(m.Run())
	}

	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("omnia_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	testConnStr, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to terminate container: %v\n", err)
	}

	os.Exit(code)
}

// freshDB creates a new database within the shared container for test isolation.
// Each test gets its own schema by running migrations on the shared Postgres instance.
func freshDB(t *testing.T) (*sql.DB, string) {
	t.Helper()

	// Create a unique database for this test
	dbName := fmt.Sprintf("test_%d", time.Now().UnixNano())

	db, err := sql.Open("pgx", testConnStr)
	require.NoError(t, err)

	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// Build connection string for the new database by replacing the db name
	// testConnStr format: postgres://user:pass@host:port/omnia_test?sslmode=disable
	// We need to replace "omnia_test" with our new dbName
	connStr := replaceDBName(testConnStr, dbName)

	db, err = sql.Open("pgx", connStr)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = db.Close()
		// Drop the test database
		mainDB, err := sql.Open("pgx", testConnStr)
		if err == nil {
			_, _ = mainDB.Exec(fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", dbName))
			_ = mainDB.Close()
		}
	})

	return db, connStr
}

func replaceDBName(connStr, newDB string) string {
	// Parse the connection string and replace the database name
	// Format: postgres://user:pass@host:port/dbname?params
	// Find the last '/' before '?' and replace the db name
	qIdx := len(connStr)
	for i, c := range connStr {
		if c == '?' {
			qIdx = i
			break
		}
	}

	// Find the last '/' before the query string
	slashIdx := 0
	for i := qIdx - 1; i >= 0; i-- {
		if connStr[i] == '/' {
			slashIdx = i
			break
		}
	}

	return connStr[:slashIdx+1] + newDB + connStr[qIdx:]
}

func TestMigrationFS_ContainsMigrations(t *testing.T) {
	entries, err := MigrationFS.ReadDir("migrations")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 18, "should have at least 18 migration files (9 up + 9 down)")

	// Verify expected migration files exist
	expected := []string{
		"000001_create_sessions.up.sql",
		"000001_create_sessions.down.sql",
		"000005_create_partition_management.up.sql",
		"000005_create_partition_management.down.sql",
		"000006_create_audit_log.up.sql",
		"000006_create_audit_log.down.sql",
		"000007_add_audit_log_partitions.up.sql",
		"000007_add_audit_log_partitions.down.sql",
		"000008_tool_call_id_to_text.up.sql",
		"000008_tool_call_id_to_text.down.sql",
		"000009_create_privacy_preferences.up.sql",
		"000009_create_privacy_preferences.down.sql",
	}
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name()] = true
	}
	for _, name := range expected {
		assert.True(t, names[name], "migration %s should be embedded", name)
	}
}

func TestNewMigrator_InvalidConnection(t *testing.T) {
	logger := zap.New(zap.UseDevMode(true))

	_, err := NewMigrator("postgres://invalid:5432/nonexistent?sslmode=disable&connect_timeout=1", logger)
	assert.Error(t, err, "should fail with invalid connection")
}

func TestMigrator_UpDown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	_, connStr := freshDB(t)
	logger := zap.New(zap.UseDevMode(true))

	mg, err := NewMigrator(connStr, logger)
	require.NoError(t, err)
	defer func() { _ = mg.Close() }()

	// Apply all migrations
	err = mg.Up()
	require.NoError(t, err)

	// Verify version
	v, dirty, err := mg.Version()
	require.NoError(t, err)
	assert.Equal(t, uint(12), v)
	assert.False(t, dirty)

	// Idempotent — running Up again should succeed
	err = mg.Up()
	require.NoError(t, err)

	// Roll back all migrations
	err = mg.Down()
	require.NoError(t, err)
}

func TestMigrator_TablesExist(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, connStr := freshDB(t)
	logger := zap.New(zap.UseDevMode(true))

	mg, err := NewMigrator(connStr, logger)
	require.NoError(t, err)
	defer func() { _ = mg.Close() }()

	err = mg.Up()
	require.NoError(t, err)

	// Verify all expected tables exist as partitioned tables
	for _, table := range []string{"sessions", "messages", "tool_calls", "message_artifacts", "audit_log"} {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM pg_class c
				JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE c.relname = $1
				AND n.nspname = 'public'
				AND c.relkind = 'p'
			)`, table).Scan(&exists)
		require.NoError(t, err, "checking table %s", table)
		assert.True(t, exists, "table %s should exist as partitioned", table)
	}
}

func TestMigrator_PartitionsCreated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, connStr := freshDB(t)
	logger := zap.New(zap.UseDevMode(true))

	mg, err := NewMigrator(connStr, logger)
	require.NoError(t, err)
	defer func() { _ = mg.Close() }()

	err = mg.Up()
	require.NoError(t, err)

	// Each table should have partitions (4 weeks back + 2 weeks ahead ≈ 5-7 partitions)
	for _, table := range []string{"sessions", "messages", "tool_calls", "message_artifacts", "audit_log"} {
		var count int
		err := db.QueryRow(`
			SELECT COUNT(*) FROM pg_class c
			JOIN pg_inherits i ON i.inhrelid = c.oid
			JOIN pg_class parent ON parent.oid = i.inhparent
			WHERE parent.relname = $1
			AND c.relispartition`, table).Scan(&count)
		require.NoError(t, err, "counting partitions for %s", table)
		assert.GreaterOrEqual(t, count, 5, "table %s should have at least 5 partitions", table)
	}
}

func TestMigrator_IndexesExist(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, connStr := freshDB(t)
	logger := zap.New(zap.UseDevMode(true))

	mg, err := NewMigrator(connStr, logger)
	require.NoError(t, err)
	defer func() { _ = mg.Close() }()

	err = mg.Up()
	require.NoError(t, err)

	expectedIndexes := []string{
		"idx_sessions_agent_created",
		"idx_sessions_namespace_created",
		"idx_sessions_workspace_created",
		"idx_sessions_status_active",
		"idx_sessions_expires_at",
		"idx_sessions_tags",
		"idx_messages_session_seq",
		"idx_messages_search",
		"idx_messages_tool_call_id",
		"idx_tool_calls_message",
		"idx_tool_calls_session",
		"idx_tool_calls_name",
		"idx_message_artifacts_message",
		"idx_message_artifacts_session",
	}

	for _, idx := range expectedIndexes {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM pg_class
				WHERE relname = $1
				AND relkind = 'I'
			)`, idx).Scan(&exists)
		require.NoError(t, err, "checking index %s", idx)
		assert.True(t, exists, "index %s should exist", idx)
	}
}

func TestMigrator_DataOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, connStr := freshDB(t)
	logger := zap.New(zap.UseDevMode(true))

	mg, err := NewMigrator(connStr, logger)
	require.NoError(t, err)
	defer func() { _ = mg.Close() }()

	err = mg.Up()
	require.NoError(t, err)

	now := time.Now().UTC()

	// Insert a session
	sessionID := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"
	_, err = db.Exec(`
		INSERT INTO sessions (id, agent_name, namespace, status, created_at, updated_at)
		VALUES ($1, 'test-agent', 'default', 'active', $2, $2)`,
		sessionID, now)
	require.NoError(t, err)

	// Insert a message
	messageID := "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"
	_, err = db.Exec(`
		INSERT INTO messages (id, session_id, role, content, timestamp, sequence_num)
		VALUES ($1, $2, 'user', 'Hello, how can you help me with Kubernetes?', $3, 1)`,
		messageID, sessionID, now)
	require.NoError(t, err)

	// Verify tsvector search works
	var found bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM messages
			WHERE search_vector @@ plainto_tsquery('english', 'kubernetes')
		)`).Scan(&found)
	require.NoError(t, err)
	assert.True(t, found, "tsvector search should find 'kubernetes'")

	// Verify search for a word NOT in the content
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM messages
			WHERE search_vector @@ plainto_tsquery('english', 'database')
		)`).Scan(&found)
	require.NoError(t, err)
	assert.False(t, found, "tsvector search should not find 'database'")

	// Insert a tool call
	toolCallID := "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"
	_, err = db.Exec(`
		INSERT INTO tool_calls (id, message_id, session_id, name, arguments, status, created_at)
		VALUES ($1, $2, $3, 'kubectl_get', '{"resource": "pods"}', 'success', $4)`,
		toolCallID, messageID, sessionID, now)
	require.NoError(t, err)

	// Insert a message artifact
	_, err = db.Exec(`
		INSERT INTO message_artifacts (message_id, session_id, artifact_type, mime_type, storage_uri, created_at)
		VALUES ($1, $2, 'document', 'application/json', 's3://bucket/path/output.json', $3)`,
		messageID, sessionID, now)
	require.NoError(t, err)

	// Verify status check constraint
	_, err = db.Exec(`
		INSERT INTO sessions (id, agent_name, namespace, status, created_at, updated_at)
		VALUES (gen_random_uuid(), 'test-agent', 'default', 'invalid_status', $1, $1)`, now)
	assert.Error(t, err, "inserting session with invalid status should fail")

	// Verify role check constraint
	_, err = db.Exec(`
		INSERT INTO messages (id, session_id, role, content, timestamp, sequence_num)
		VALUES (gen_random_uuid(), $1, 'invalid_role', 'test', $2, 2)`,
		sessionID, now)
	assert.Error(t, err, "inserting message with invalid role should fail")

	// Verify audit_log inserts work (this was the production failure:
	// "no partition of relation audit_log found for row")
	_, err = db.Exec(`
		INSERT INTO audit_log (
			timestamp, event_type, session_id, user_id,
			workspace, agent_name, namespace, query,
			result_count, ip_address, user_agent, reason, metadata
		) VALUES ($1, 'session_accessed', $2, 'test-user',
			'default', 'test-agent', 'default', NULL,
			NULL, '127.0.0.1', 'test-ua', NULL, '{}')`,
		now, sessionID)
	require.NoError(t, err, "inserting into audit_log should succeed (partition must exist for current date)")
}

func TestMigrator_PartitionManagement(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, connStr := freshDB(t)
	logger := zap.New(zap.UseDevMode(true))

	mg, err := NewMigrator(connStr, logger)
	require.NoError(t, err)
	defer func() { _ = mg.Close() }()

	err = mg.Up()
	require.NoError(t, err)

	// Call manage_session_partitions with custom retention
	rows, err := db.Query("SELECT * FROM manage_session_partitions(7, 1)")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var tableName string
		var created, dropped int
		err := rows.Scan(&tableName, &created, &dropped)
		require.NoError(t, err)
		t.Logf("table=%s created=%d dropped=%d", tableName, created, dropped)
	}
	require.NoError(t, rows.Err())

	// Verify create_weekly_partitions is idempotent
	var created int
	err = db.QueryRow(fmt.Sprintf(
		"SELECT create_weekly_partitions('sessions', '%s'::DATE, '%s'::DATE)",
		time.Now().AddDate(0, 0, -7).Format("2006-01-02"),
		time.Now().AddDate(0, 0, 7).Format("2006-01-02"),
	)).Scan(&created)
	require.NoError(t, err)
	assert.Equal(t, 0, created, "re-creating existing partitions should create 0 new ones")
}

func TestMigrator_CleanTeardown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, connStr := freshDB(t)
	logger := zap.New(zap.UseDevMode(true))

	mg, err := NewMigrator(connStr, logger)
	require.NoError(t, err)
	defer func() { _ = mg.Close() }()

	// Up then Down
	err = mg.Up()
	require.NoError(t, err)

	err = mg.Down()
	require.NoError(t, err)

	// Verify all tables are gone
	for _, table := range []string{"sessions", "messages", "tool_calls", "message_artifacts", "audit_log"} {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM pg_class c
				JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE c.relname = $1
				AND n.nspname = 'public'
			)`, table).Scan(&exists)
		require.NoError(t, err, "checking table %s after down", table)
		assert.False(t, exists, "table %s should not exist after down migration", table)
	}

	// Verify functions are gone
	for _, fn := range []string{"create_weekly_partitions", "drop_old_partitions", "manage_session_partitions"} {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM pg_proc p
				JOIN pg_namespace n ON n.oid = p.pronamespace
				WHERE p.proname = $1
				AND n.nspname = 'public'
			)`, fn).Scan(&exists)
		require.NoError(t, err, "checking function %s after down", fn)
		assert.False(t, exists, "function %s should not exist after down migration", fn)
	}
}
