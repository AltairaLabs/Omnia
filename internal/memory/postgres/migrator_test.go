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
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
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
func freshDB(t *testing.T) (*sql.DB, string) {
	t.Helper()

	dbName := fmt.Sprintf("test_%d", time.Now().UnixNano())

	db, err := sql.Open("pgx", testConnStr)
	require.NoError(t, err)

	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	connStr := replaceDBName(testConnStr, dbName)

	db, err = sql.Open("pgx", connStr)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = db.Close()
		mainDB, err := sql.Open("pgx", testConnStr)
		if err == nil {
			_, _ = mainDB.Exec(fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", dbName))
			_ = mainDB.Close()
		}
	})

	return db, connStr
}

func replaceDBName(connStr, newDB string) string {
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

func TestMigrationFS_ContainsMigrations(t *testing.T) {
	entries, err := migrationsFS.ReadDir("migrations")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 2, "should have at least 2 migration files (1 up + 1 down)")

	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name()] = true
	}
	assert.True(t, names["000001_initial_schema.up.sql"], "up migration should be embedded")
	assert.True(t, names["000001_initial_schema.down.sql"], "down migration should be embedded")
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

	// Verify version matches the number of up migration files
	entries, err := migrationsFS.ReadDir("migrations")
	require.NoError(t, err)
	var upCount uint
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			upCount++
		}
	}
	v, dirty, err := mg.Version()
	require.NoError(t, err)
	assert.Equal(t, upCount, v)
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

	expectedTables := []string{
		"memory_entities",
		"memory_relations",
		"memory_observations",
		"user_privacy_preferences",
	}

	for _, table := range expectedTables {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM pg_class c
				JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE c.relname = $1
				AND n.nspname = 'public'
			)`, table).Scan(&exists)
		require.NoError(t, err, "checking table %s", table)
		assert.True(t, exists, "table %s should exist", table)
	}
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

	err = mg.Up()
	require.NoError(t, err)

	err = mg.Down()
	require.NoError(t, err)

	// Verify all tables are gone after down migration
	for _, table := range []string{"memory_entities", "memory_relations", "memory_observations", "user_privacy_preferences"} {
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
}

func TestMigrator_VersionBeforeUp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	_, connStr := freshDB(t)
	logger := zap.New(zap.UseDevMode(true))

	mg, err := NewMigrator(connStr, logger)
	require.NoError(t, err)
	defer func() { _ = mg.Close() }()

	// Version before any migrations returns 0 and no error.
	v, dirty, err := mg.Version()
	assert.NoError(t, err)
	assert.Equal(t, uint(0), v)
	assert.False(t, dirty)
}

func TestMigrator_UpIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	_, connStr := freshDB(t)
	logger := zap.New(zap.UseDevMode(true))

	mg, err := NewMigrator(connStr, logger)
	require.NoError(t, err)
	defer func() { _ = mg.Close() }()

	// First Up applies migrations.
	require.NoError(t, mg.Up())

	// Second Up is a no-op (ErrNoChange is swallowed).
	require.NoError(t, mg.Up())

	v, dirty, err := mg.Version()
	require.NoError(t, err)
	assert.False(t, dirty)
	assert.Greater(t, v, uint(0))
}

func TestMigrator_DownIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	_, connStr := freshDB(t)
	logger := zap.New(zap.UseDevMode(true))

	mg, err := NewMigrator(connStr, logger)
	require.NoError(t, err)
	defer func() { _ = mg.Close() }()

	// Down on a fresh DB (no migrations) should not error.
	require.NoError(t, mg.Down())
}

func TestMigrator_Close(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	_, connStr := freshDB(t)
	logger := zap.New(zap.UseDevMode(true))

	mg, err := NewMigrator(connStr, logger)
	require.NoError(t, err)

	// Close should return nil on a valid migrator.
	require.NoError(t, mg.Close())
}

func TestMigrator_Up_DirtyStateRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, connStr := freshDB(t)
	logger := zap.New(zap.UseDevMode(true))

	// First: apply and roll back migrations so we have clean schema_migrations table
	// but no tables.
	mg, err := NewMigrator(connStr, logger)
	require.NoError(t, err)
	require.NoError(t, mg.Up())
	require.NoError(t, mg.Down())
	require.NoError(t, mg.Close())

	// Re-open a fresh migrator, mark as dirty at version 1.
	_, err = db.Exec(`
		INSERT INTO schema_migrations (version, dirty) VALUES (1, true)
		ON CONFLICT (version) DO UPDATE SET dirty = true`)
	require.NoError(t, err)

	mg2, err := NewMigrator(connStr, logger)
	require.NoError(t, err)
	defer func() { _ = mg2.Close() }()

	// Verify we're in dirty state.
	v, dirty, err := mg2.Version()
	require.NoError(t, err)
	assert.True(t, dirty, "database should be dirty before recovery")
	assert.Equal(t, uint(1), v)

	// Up should detect the dirty state, recover to -1, and re-apply.
	require.NoError(t, mg2.Up())

	v2, dirty2, err := mg2.Version()
	require.NoError(t, err)
	assert.False(t, dirty2, "database should no longer be dirty after recovery")
	assert.Greater(t, v2, uint(0))
}
