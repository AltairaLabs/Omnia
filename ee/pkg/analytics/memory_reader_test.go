/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package analytics

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	pgmigrate "github.com/altairalabs/omnia/internal/memory/postgres"
)

var memTestConnStr string

func TestMain(m *testing.M) {
	flag.Parse()

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
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	memTestConnStr, err = container.ConnectionString(ctx, "sslmode=disable")
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

// freshMemDB creates an isolated database, runs all migrations, and returns a *pgxpool.Pool.
func freshMemDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbName := fmt.Sprintf("test_%d", time.Now().UnixNano())

	db, err := sql.Open("pgx", memTestConnStr)
	if err != nil {
		t.Fatalf("open admin db: %v", err)
	}
	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close admin db: %v", err)
	}

	connStr := memReplaceDBName(memTestConnStr, dbName)

	logger := zap.New(zap.UseDevMode(true))
	mg, err := pgmigrate.NewMigrator(connStr, logger)
	if err != nil {
		t.Fatalf("create migrator: %v", err)
	}
	if err := mg.Up(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	if err := mg.Close(); err != nil {
		t.Fatalf("close migrator: %v", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		mainDB, dbErr := sql.Open("pgx", memTestConnStr)
		if dbErr == nil {
			_, _ = mainDB.Exec(fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", dbName))
			_ = mainDB.Close()
		}
	})

	return pool
}

// memReplaceDBName replaces the database name in a connection string.
func memReplaceDBName(connStr, newDB string) string {
	qIdx := len(connStr)
	for i, c := range connStr {
		if c == '?' {
			qIdx = i
			break
		}
	}
	slashIdx := 0
	for i := qIdx - 1; i >= 0; i-- {
		if connStr[i] == '/' {
			slashIdx = i
			break
		}
	}
	return connStr[:slashIdx+1] + newDB + connStr[qIdx:]
}

// insertTestEntity inserts a memory entity and returns its UUID.
func insertTestEntity(t *testing.T, pool *pgxpool.Pool, workspaceID, name, kind string, createdAt time.Time) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO memory_entities (workspace_id, name, kind, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)
		RETURNING id`,
		workspaceID, name, kind, createdAt,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert test entity: %v", err)
	}
	return id
}

// insertTestObservation inserts a memory observation and returns its UUID.
func insertTestObservation(t *testing.T, pool *pgxpool.Pool, entityID, content string, createdAt time.Time) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO memory_observations (entity_id, content, created_at, observed_at)
		VALUES ($1, $2, $3, $3)
		RETURNING id`,
		entityID, content, createdAt,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert test observation: %v", err)
	}
	return id
}

func TestMemorySourceReader_ReadMemoryEntities_HappyPath(t *testing.T) {
	pool := freshMemDB(t)
	reader := NewMemorySourceReader(pool)

	workspaceID := "00000000-0000-0000-0000-000000000001"
	base := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Millisecond)

	id1 := insertTestEntity(t, pool, workspaceID, "Alice", "person", base.Add(1*time.Second))
	id2 := insertTestEntity(t, pool, workspaceID, "Bob", "person", base.Add(2*time.Second))

	rows, err := NewMemorySourceReader(pool).ReadMemoryEntities(context.Background(), base, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	ids := map[string]bool{rows[0].ID: true, rows[1].ID: true}
	if !ids[id1] || !ids[id2] {
		t.Errorf("expected IDs %s and %s, got %v", id1, id2, ids)
	}
	_ = reader
}

func TestMemorySourceReader_ReadMemoryEntities_WatermarkFiltering(t *testing.T) {
	pool := freshMemDB(t)
	reader := NewMemorySourceReader(pool)

	workspaceID := "00000000-0000-0000-0000-000000000002"
	base := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Millisecond)

	// Insert entity before watermark — should not appear.
	insertTestEntity(t, pool, workspaceID, "Old", "person", base.Add(-1*time.Second))
	// Insert entity after watermark — should appear.
	id := insertTestEntity(t, pool, workspaceID, "New", "person", base.Add(1*time.Second))

	rows, err := reader.ReadMemoryEntities(context.Background(), base, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].ID != id {
		t.Errorf("expected ID %s, got %s", id, rows[0].ID)
	}
	if rows[0].WorkspaceID != workspaceID {
		t.Errorf("expected WorkspaceID %s, got %s", workspaceID, rows[0].WorkspaceID)
	}
	if rows[0].Name != "New" {
		t.Errorf("expected Name 'New', got %s", rows[0].Name)
	}
	if rows[0].Kind != "person" {
		t.Errorf("expected Kind 'person', got %s", rows[0].Kind)
	}
}

func TestMemorySourceReader_ReadMemoryEntities_ForgottenExcluded(t *testing.T) {
	pool := freshMemDB(t)
	reader := NewMemorySourceReader(pool)

	workspaceID := "00000000-0000-0000-0000-000000000003"
	base := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Millisecond)
	after := base.Add(-1 * time.Second)

	// Insert a forgotten entity — should not appear.
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO memory_entities (workspace_id, name, kind, forgotten, created_at, updated_at)
		VALUES ($1, 'Forgotten', 'person', true, $2, $2)`,
		workspaceID, base,
	)
	if err != nil {
		t.Fatalf("insert forgotten entity: %v", err)
	}
	// Insert a live entity — should appear.
	id := insertTestEntity(t, pool, workspaceID, "Live", "person", base.Add(1*time.Second))

	rows, err := reader.ReadMemoryEntities(context.Background(), after, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (forgotten excluded), got %d", len(rows))
	}
	if rows[0].ID != id {
		t.Errorf("expected non-forgotten entity ID %s, got %s", id, rows[0].ID)
	}
}

func TestMemorySourceReader_ReadMemoryEntities_Empty(t *testing.T) {
	pool := freshMemDB(t)
	reader := NewMemorySourceReader(pool)

	rows, err := reader.ReadMemoryEntities(context.Background(), time.Now(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestMemorySourceReader_ReadMemoryEntities_Limit(t *testing.T) {
	pool := freshMemDB(t)
	reader := NewMemorySourceReader(pool)

	workspaceID := "00000000-0000-0000-0000-000000000004"
	base := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Millisecond)
	after := base.Add(-1 * time.Second)

	for i := 0; i < 5; i++ {
		insertTestEntity(t, pool, workspaceID, fmt.Sprintf("Entity%d", i), "person", base.Add(time.Duration(i)*time.Second))
	}

	rows, err := reader.ReadMemoryEntities(context.Background(), after, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("expected 3 rows (limit), got %d", len(rows))
	}
}

func TestMemorySourceReader_ReadMemoryObservations_HappyPath(t *testing.T) {
	pool := freshMemDB(t)
	reader := NewMemorySourceReader(pool)

	workspaceID := "00000000-0000-0000-0000-000000000005"
	base := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Millisecond)
	after := base.Add(-1 * time.Second)

	entityID := insertTestEntity(t, pool, workspaceID, "Charlie", "person", base)
	obsID := insertTestObservation(t, pool, entityID, "likes coffee", base.Add(1*time.Second))

	rows, err := reader.ReadMemoryObservations(context.Background(), after, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].ID != obsID {
		t.Errorf("expected ID %s, got %s", obsID, rows[0].ID)
	}
	if rows[0].EntityID != entityID {
		t.Errorf("expected EntityID %s, got %s", entityID, rows[0].EntityID)
	}
	if rows[0].Content != "likes coffee" {
		t.Errorf("expected Content 'likes coffee', got %s", rows[0].Content)
	}
}

func TestMemorySourceReader_ReadMemoryObservations_WatermarkFiltering(t *testing.T) {
	pool := freshMemDB(t)
	reader := NewMemorySourceReader(pool)

	workspaceID := "00000000-0000-0000-0000-000000000006"
	base := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Millisecond)

	entityID := insertTestEntity(t, pool, workspaceID, "Dana", "person", base.Add(-5*time.Second))

	// Insert observation before watermark.
	insertTestObservation(t, pool, entityID, "old fact", base.Add(-1*time.Second))
	// Insert observation after watermark.
	newID := insertTestObservation(t, pool, entityID, "new fact", base.Add(1*time.Second))

	rows, err := reader.ReadMemoryObservations(context.Background(), base, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].ID != newID {
		t.Errorf("expected ID %s, got %s", newID, rows[0].ID)
	}
}

func TestMemorySourceReader_ReadMemoryObservations_Empty(t *testing.T) {
	pool := freshMemDB(t)
	reader := NewMemorySourceReader(pool)

	rows, err := reader.ReadMemoryObservations(context.Background(), time.Now(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestMemorySourceReader_ReadMemoryObservations_NullableSessionID(t *testing.T) {
	pool := freshMemDB(t)
	reader := NewMemorySourceReader(pool)

	workspaceID := "00000000-0000-0000-0000-000000000007"
	base := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Millisecond)
	after := base.Add(-1 * time.Second)

	entityID := insertTestEntity(t, pool, workspaceID, "Eve", "person", base)
	// Insert observation with NULL session_id (default).
	insertTestObservation(t, pool, entityID, "no session", base.Add(1*time.Second))

	rows, err := reader.ReadMemoryObservations(context.Background(), after, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	// session_id is NULL in DB — should scan as empty string.
	if rows[0].SessionID != "" {
		t.Errorf("expected empty SessionID for null, got %q", rows[0].SessionID)
	}
}

func TestMemorySourceReader_ReadMemoryObservations_Limit(t *testing.T) {
	pool := freshMemDB(t)
	reader := NewMemorySourceReader(pool)

	workspaceID := "00000000-0000-0000-0000-000000000008"
	base := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Millisecond)
	after := base.Add(-1 * time.Second)

	entityID := insertTestEntity(t, pool, workspaceID, "Frank", "person", base.Add(-1*time.Second))
	for i := 0; i < 5; i++ {
		insertTestObservation(t, pool, entityID, fmt.Sprintf("fact %d", i), base.Add(time.Duration(i)*time.Second))
	}

	rows, err := reader.ReadMemoryObservations(context.Background(), after, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows (limit), got %d", len(rows))
	}
}

func TestMemorySourceReader_ReadMemoryEntities_NullableFields(t *testing.T) {
	pool := freshMemDB(t)
	reader := NewMemorySourceReader(pool)

	// Insert entity with no virtual_user_id and no agent_id (NULLs).
	workspaceID := "00000000-0000-0000-0000-000000000009"
	base := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Millisecond)
	after := base.Add(-1 * time.Second)

	id := insertTestEntity(t, pool, workspaceID, "Grace", "concept", base.Add(1*time.Second))

	rows, err := reader.ReadMemoryEntities(context.Background(), after, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].ID != id {
		t.Errorf("expected ID %s, got %s", id, rows[0].ID)
	}
	// virtual_user_id and agent_id are NULL — should be empty string.
	if rows[0].VirtualUserID != "" {
		t.Errorf("expected empty VirtualUserID, got %q", rows[0].VirtualUserID)
	}
	if rows[0].AgentID != "" {
		t.Errorf("expected empty AgentID, got %q", rows[0].AgentID)
	}
}

func TestMemoryEntityRow_Fields(t *testing.T) {
	now := time.Now()
	row := MemoryEntityRow{
		ID:            "e1",
		WorkspaceID:   "ws1",
		VirtualUserID: "u1",
		AgentID:       "a1",
		Name:          "Alice",
		Kind:          "person",
		SourceType:    "conversation_extraction",
		TrustModel:    "inferred",
		Purpose:       "support_continuity",
		Forgotten:     false,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if row.ID != "e1" {
		t.Errorf("expected ID 'e1', got %s", row.ID)
	}
	if row.Forgotten {
		t.Error("expected Forgotten false")
	}
}

func TestMemoryObservationRow_Fields(t *testing.T) {
	now := time.Now()
	row := MemoryObservationRow{
		ID:          "o1",
		EntityID:    "e1",
		Content:     "likes coffee",
		Confidence:  0.9,
		SourceType:  "conversation_extraction",
		SessionID:   "sess-1",
		ObservedAt:  now,
		CreatedAt:   now,
		AccessCount: 3,
	}
	if row.ID != "o1" {
		t.Errorf("expected ID 'o1', got %s", row.ID)
	}
	if row.Confidence != 0.9 {
		t.Errorf("expected Confidence 0.9, got %f", row.Confidence)
	}
	if row.AccessCount != 3 {
		t.Errorf("expected AccessCount 3, got %d", row.AccessCount)
	}
}

func TestMemorySourceReader_ReadMemoryEntities_WithNonNullFields(t *testing.T) {
	pool := freshMemDB(t)
	reader := NewMemorySourceReader(pool)

	workspaceID := "00000000-0000-0000-0000-00000000000a"
	agentID := "00000000-0000-0000-0000-00000000000b"
	virtualUserID := "user-abc"
	base := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Millisecond)
	after := base.Add(-1 * time.Second)

	ctx := context.Background()
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO memory_entities (workspace_id, virtual_user_id, agent_id, name, kind, created_at, updated_at)
		VALUES ($1, $2, $3, 'Henry', 'person', $4, $4)
		RETURNING id`,
		workspaceID, virtualUserID, agentID, base,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert entity with non-null fields: %v", err)
	}

	rows, err := reader.ReadMemoryEntities(context.Background(), after, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].VirtualUserID != virtualUserID {
		t.Errorf("expected VirtualUserID %q, got %q", virtualUserID, rows[0].VirtualUserID)
	}
	if rows[0].AgentID != agentID {
		t.Errorf("expected AgentID %q, got %q", agentID, rows[0].AgentID)
	}
}

func TestMemorySourceReader_ReadMemoryObservations_WithSessionID(t *testing.T) {
	pool := freshMemDB(t)
	reader := NewMemorySourceReader(pool)

	workspaceID := "00000000-0000-0000-0000-00000000000c"
	sessionID := "00000000-0000-0000-0000-00000000000d"
	base := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Millisecond)
	after := base.Add(-1 * time.Second)

	entityID := insertTestEntity(t, pool, workspaceID, "Iris", "person", base.Add(-1*time.Second))

	ctx := context.Background()
	var obsID string
	err := pool.QueryRow(ctx, `
		INSERT INTO memory_observations (entity_id, content, session_id, created_at, observed_at)
		VALUES ($1, 'had coffee', $2, $3, $3)
		RETURNING id`,
		entityID, sessionID, base,
	).Scan(&obsID)
	if err != nil {
		t.Fatalf("insert observation with session_id: %v", err)
	}

	rows, err := reader.ReadMemoryObservations(context.Background(), after, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].SessionID != sessionID {
		t.Errorf("expected SessionID %q, got %q", sessionID, rows[0].SessionID)
	}
}

func TestNewMemorySourceReader(t *testing.T) {
	pool := freshMemDB(t)
	r := NewMemorySourceReader(pool)
	if r == nil {
		t.Fatal("expected non-nil reader")
	}
	if r.pool != pool {
		t.Error("expected pool to be set")
	}
}

func TestMemorySourceReader_ContextCancellation_Entities(t *testing.T) {
	pool := freshMemDB(t)
	reader := NewMemorySourceReader(pool)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := reader.ReadMemoryEntities(ctx, time.Time{}, 10)
	if err == nil {
		t.Error("expected error with cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		// pgx wraps the error, so just check it's non-nil
		t.Logf("got expected error (non-nil): %v", err)
	}
}

func TestMemorySourceReader_ContextCancellation_Observations(t *testing.T) {
	pool := freshMemDB(t)
	reader := NewMemorySourceReader(pool)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := reader.ReadMemoryObservations(ctx, time.Time{}, 10)
	if err == nil {
		t.Error("expected error with cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Logf("got expected error (non-nil): %v", err)
	}
}

// --- mock queryExecutor and rows for error-path tests -----------------------

var errScan = errors.New("scan error")
var errRowsErr = errors.New("rows iteration error")

// mockRows implements pgx.Rows with configurable behavior.
type mockRows struct {
	hasRow  bool // return true on first Next call
	scanErr error
	rowsErr error
	closed  bool
}

func (m *mockRows) Close() { m.closed = true }

func (m *mockRows) Err() error { return m.rowsErr }

func (m *mockRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

func (m *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (m *mockRows) Next() bool {
	if m.hasRow {
		m.hasRow = false
		return true
	}
	return false
}

func (m *mockRows) Scan(_ ...any) error { return m.scanErr }

func (m *mockRows) Values() ([]any, error) { return nil, nil }

func (m *mockRows) RawValues() [][]byte { return nil }

func (m *mockRows) Conn() *pgx.Conn { return nil }

// mockQueryExecutor returns a preset pgx.Rows or error.
type mockQueryExecutor struct {
	rows pgx.Rows
	err  error
}

func (m *mockQueryExecutor) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.rows, nil
}

func TestMemorySourceReader_ReadEntities_ScanError(t *testing.T) {
	exec := &mockQueryExecutor{rows: &mockRows{hasRow: true, scanErr: errScan}}
	reader := &MemorySourceReader{exec: exec}

	_, err := reader.ReadMemoryEntities(context.Background(), time.Time{}, 10)
	if err == nil {
		t.Fatal("expected scan error")
	}
	if !errors.Is(err, errScan) {
		t.Errorf("expected errScan wrapped, got: %v", err)
	}
}

func TestMemorySourceReader_ReadEntities_RowsErrError(t *testing.T) {
	exec := &mockQueryExecutor{rows: &mockRows{hasRow: false, rowsErr: errRowsErr}}
	reader := &MemorySourceReader{exec: exec}

	_, err := reader.ReadMemoryEntities(context.Background(), time.Time{}, 10)
	if err == nil {
		t.Fatal("expected rows.Err() error")
	}
	if !errors.Is(err, errRowsErr) {
		t.Errorf("expected errRowsErr wrapped, got: %v", err)
	}
}

func TestMemorySourceReader_ReadObservations_ScanError(t *testing.T) {
	exec := &mockQueryExecutor{rows: &mockRows{hasRow: true, scanErr: errScan}}
	reader := &MemorySourceReader{exec: exec}

	_, err := reader.ReadMemoryObservations(context.Background(), time.Time{}, 10)
	if err == nil {
		t.Fatal("expected scan error")
	}
	if !errors.Is(err, errScan) {
		t.Errorf("expected errScan wrapped, got: %v", err)
	}
}

func TestMemorySourceReader_ReadObservations_RowsErrError(t *testing.T) {
	exec := &mockQueryExecutor{rows: &mockRows{hasRow: false, rowsErr: errRowsErr}}
	reader := &MemorySourceReader{exec: exec}

	_, err := reader.ReadMemoryObservations(context.Background(), time.Time{}, 10)
	if err == nil {
		t.Fatal("expected rows.Err() error")
	}
	if !errors.Is(err, errRowsErr) {
		t.Errorf("expected errRowsErr wrapped, got: %v", err)
	}
}

// --- end-to-end integration tests -------------------------------------------

// TestMemorySourceReader_EndToEnd verifies that entities and observations are
// read together correctly in a single end-to-end pass — something no individual
// method test covers.
func TestMemorySourceReader_EndToEnd(t *testing.T) {
	pool := freshMemDB(t)
	reader := NewMemorySourceReader(pool)

	workspaceID := "00000000-0000-0000-0000-0000000000e1"
	base := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Millisecond)
	watermark := base.Add(-1 * time.Second)

	// Insert two entities with observations.
	entityID1 := insertTestEntity(t, pool, workspaceID, "Alice", "person", base.Add(1*time.Second))
	entityID2 := insertTestEntity(t, pool, workspaceID, "Bob", "person", base.Add(2*time.Second))
	obsID1 := insertTestObservation(t, pool, entityID1, "likes tea", base.Add(3*time.Second))
	obsID2 := insertTestObservation(t, pool, entityID2, "likes coffee", base.Add(4*time.Second))

	// Read entities.
	entities, err := reader.ReadMemoryEntities(context.Background(), watermark, 100)
	if err != nil {
		t.Fatalf("ReadMemoryEntities: %v", err)
	}
	if len(entities) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(entities))
	}
	entityIDs := map[string]bool{entities[0].ID: true, entities[1].ID: true}
	if !entityIDs[entityID1] || !entityIDs[entityID2] {
		t.Errorf("expected entities %s and %s, got %v", entityID1, entityID2, entityIDs)
	}

	// Read observations.
	observations, err := reader.ReadMemoryObservations(context.Background(), watermark, 100)
	if err != nil {
		t.Fatalf("ReadMemoryObservations: %v", err)
	}
	if len(observations) != 2 {
		t.Fatalf("expected 2 observations, got %d", len(observations))
	}
	obsIDs := map[string]bool{observations[0].ID: true, observations[1].ID: true}
	if !obsIDs[obsID1] || !obsIDs[obsID2] {
		t.Errorf("expected observations %s and %s, got %v", obsID1, obsID2, obsIDs)
	}

	// Verify cross-references: each observation links to a known entity.
	for _, obs := range observations {
		if !entityIDs[obs.EntityID] {
			t.Errorf("observation %s has unexpected EntityID %s", obs.ID, obs.EntityID)
		}
	}

	// Verify that a watermark after all data returns nothing.
	futureWatermark := base.Add(1 * time.Hour)
	emptyEntities, err := reader.ReadMemoryEntities(context.Background(), futureWatermark, 100)
	if err != nil {
		t.Fatalf("ReadMemoryEntities (future watermark): %v", err)
	}
	if len(emptyEntities) != 0 {
		t.Errorf("expected 0 entities after all data, got %d", len(emptyEntities))
	}
	emptyObs, err := reader.ReadMemoryObservations(context.Background(), futureWatermark, 100)
	if err != nil {
		t.Fatalf("ReadMemoryObservations (future watermark): %v", err)
	}
	if len(emptyObs) != 0 {
		t.Errorf("expected 0 observations after all data, got %d", len(emptyObs))
	}
}

// TestMemorySourceReader_WatermarkProgression verifies that watermark-based
// reads correctly page through data across multiple checkpoints — the pattern
// that analytics sync actually uses.
func TestMemorySourceReader_WatermarkProgression(t *testing.T) {
	pool := freshMemDB(t)
	reader := NewMemorySourceReader(pool)

	workspaceID := "00000000-0000-0000-0000-0000000000e2"
	base := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Millisecond)

	// T1, T2, T3: three time points with one entity each.
	t1 := base
	t2 := base.Add(10 * time.Second)
	t3 := base.Add(20 * time.Second)

	insertTestEntity(t, pool, workspaceID, "EntityAt_T1", "person", t1.Add(1*time.Second))
	insertTestEntity(t, pool, workspaceID, "EntityAt_T2", "person", t2.Add(1*time.Second))
	insertTestEntity(t, pool, workspaceID, "EntityAt_T3", "person", t3.Add(1*time.Second))

	// Checkpoint before T1: all 3 entities returned.
	rows, err := reader.ReadMemoryEntities(context.Background(), t1.Add(-1*time.Second), 100)
	if err != nil {
		t.Fatalf("watermark before T1: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("before T1: expected 3, got %d", len(rows))
	}

	// Checkpoint at T2: only the T3 entity is after T2+1s.
	rows, err = reader.ReadMemoryEntities(context.Background(), t2.Add(1*time.Second), 100)
	if err != nil {
		t.Fatalf("watermark at T2+1: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("after T2+1: expected 1, got %d", len(rows))
	}
	if rows[0].Name != "EntityAt_T3" {
		t.Errorf("expected EntityAt_T3, got %s", rows[0].Name)
	}

	// Checkpoint after T3: no entities remain.
	rows, err = reader.ReadMemoryEntities(context.Background(), t3.Add(2*time.Second), 100)
	if err != nil {
		t.Fatalf("watermark after T3: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("after T3+2s: expected 0, got %d", len(rows))
	}
}
