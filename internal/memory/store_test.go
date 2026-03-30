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

package memory

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	pgmigrate "github.com/altairalabs/omnia/internal/session/postgres"
)

var testConnStr string

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

// freshDB creates an isolated database, runs all migrations, and returns a pgxpool.Pool.
func freshDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbName := fmt.Sprintf("test_%d", time.Now().UnixNano())

	db, err := sql.Open("pgx", testConnStr)
	require.NoError(t, err)
	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	connStr := replaceDBName(testConnStr, dbName)

	// Run all migrations (including 000025 memory tables).
	logger := zap.New(zap.UseDevMode(true))
	mg, err := pgmigrate.NewMigrator(connStr, logger)
	require.NoError(t, err)
	require.NoError(t, mg.Up())
	require.NoError(t, mg.Close())

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Close()
		mainDB, err := sql.Open("pgx", testConnStr)
		if err == nil {
			_, _ = mainDB.Exec(fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", dbName))
			_ = mainDB.Close()
		}
	})

	return pool
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

func newStore(t *testing.T) *PostgresMemoryStore {
	t.Helper()
	pool := freshDB(t)
	return NewPostgresMemoryStore(pool)
}

// Test UUID constants for workspace isolation tests.
const (
	testWorkspace1 = "a0000000-0000-0000-0000-000000000001"
	testWorkspace2 = "a0000000-0000-0000-0000-000000000002"
)

func testScope(workspaceID string) map[string]string {
	return map[string]string{
		ScopeWorkspaceID: workspaceID,
		ScopeUserID:      "user-1",
	}
}

func TestPostgresMemoryStore_Save(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()

	mem := &Memory{
		Type:       "preference",
		Content:    "prefers dark mode",
		Confidence: 0.9,
		Scope:      testScope(testWorkspace1),
		Metadata:   map[string]any{"source": "chat"},
	}

	err := store.Save(ctx, mem)
	require.NoError(t, err)
	assert.NotEmpty(t, mem.ID, "ID should be populated after save")
	assert.False(t, mem.CreatedAt.IsZero(), "CreatedAt should be populated")
}

func TestPostgresMemoryStore_Save_MissingWorkspace(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()

	mem := &Memory{
		Type:    "fact",
		Content: "test",
		Scope:   map[string]string{},
	}

	err := store.Save(ctx, mem)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

func TestPostgresMemoryStore_SaveUpsert(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// First save — creates entity + observation.
	mem := &Memory{
		Type:       "preference",
		Content:    "likes Go",
		Confidence: 0.8,
		Scope:      scope,
	}
	require.NoError(t, store.Save(ctx, mem))
	originalID := mem.ID

	// Second save — same ID, appends new observation.
	mem.Content = "likes Go and Rust"
	mem.Confidence = 0.95
	require.NoError(t, store.Save(ctx, mem))

	assert.Equal(t, originalID, mem.ID, "ID should remain the same on upsert")

	// Retrieve should return the latest observation content.
	results, err := store.Retrieve(ctx, scope, "Rust", RetrieveOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "likes Go and Rust", results[0].Content)
	assert.InDelta(t, 0.95, results[0].Confidence, 0.001)
}

func TestPostgresMemoryStore_Retrieve(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	memories := []*Memory{
		{Type: "preference", Content: "prefers dark mode", Confidence: 0.9, Scope: scope},
		{Type: "fact", Content: "works at Acme Corp", Confidence: 0.85, Scope: scope},
		{Type: "preference", Content: "uses vim editor", Confidence: 0.7, Scope: scope},
	}
	for _, m := range memories {
		require.NoError(t, store.Save(ctx, m))
	}

	// Retrieve with substring query.
	results, err := store.Retrieve(ctx, scope, "dark", RetrieveOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "prefers dark mode", results[0].Content)

	// Retrieve with type filter.
	results, err = store.Retrieve(ctx, scope, "", RetrieveOptions{Types: []string{"preference"}})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Retrieve with confidence filter.
	results, err = store.Retrieve(ctx, scope, "", RetrieveOptions{MinConfidence: 0.8})
	require.NoError(t, err)
	assert.Len(t, results, 2, "should return memories with confidence >= 0.8")

	// Retrieve with limit.
	results, err = store.Retrieve(ctx, scope, "", RetrieveOptions{Limit: 1})
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestPostgresMemoryStore_Retrieve_MissingWorkspace(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()

	_, err := store.Retrieve(ctx, map[string]string{}, "query", RetrieveOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

func TestPostgresMemoryStore_List(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	for i := range 3 {
		mem := &Memory{
			Type:       "fact",
			Content:    fmt.Sprintf("fact number %d", i),
			Confidence: 0.9,
			Scope:      scope,
		}
		require.NoError(t, store.Save(ctx, mem))
	}

	results, err := store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Pagination.
	results, err = store.List(ctx, scope, ListOptions{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	results, err = store.List(ctx, scope, ListOptions{Limit: 2, Offset: 2})
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestPostgresMemoryStore_List_MissingWorkspace(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()

	_, err := store.List(ctx, map[string]string{}, ListOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

func TestPostgresMemoryStore_Delete(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	mem := &Memory{
		Type:       "preference",
		Content:    "to be deleted",
		Confidence: 0.9,
		Scope:      scope,
	}
	require.NoError(t, store.Save(ctx, mem))

	// Delete (soft).
	err := store.Delete(ctx, scope, mem.ID)
	require.NoError(t, err)

	// Should not appear in list (forgotten = true).
	results, err := store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestPostgresMemoryStore_Delete_NotFound(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()

	err := store.Delete(ctx, testScope(testWorkspace1), "00000000-0000-0000-0000-000000000000")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPostgresMemoryStore_Delete_MissingWorkspace(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()

	err := store.Delete(ctx, map[string]string{}, "some-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

func TestPostgresMemoryStore_DeleteAll(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	for i := range 5 {
		mem := &Memory{
			Type:       "fact",
			Content:    fmt.Sprintf("fact %d", i),
			Confidence: 0.9,
			Scope:      scope,
		}
		require.NoError(t, store.Save(ctx, mem))
	}

	// Verify they exist.
	results, err := store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	assert.Len(t, results, 5)

	// Delete all.
	err = store.DeleteAll(ctx, scope)
	require.NoError(t, err)

	// Verify empty.
	results, err = store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestPostgresMemoryStore_DeleteAll_MissingWorkspace(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()

	err := store.DeleteAll(ctx, map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

func TestPostgresMemoryStore_WorkspaceIsolation(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()

	scope1 := testScope(testWorkspace1)
	scope2 := testScope(testWorkspace2)

	// Save in workspace 1.
	for i := range 3 {
		mem := &Memory{
			Type:       "fact",
			Content:    fmt.Sprintf("ws1 fact %d", i),
			Confidence: 0.9,
			Scope:      scope1,
		}
		require.NoError(t, store.Save(ctx, mem))
	}

	// Query workspace 2 — should be empty.
	results, err := store.List(ctx, scope2, ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, results, "workspace 2 should have no memories")

	// Retrieve from workspace 2 — should be empty.
	results, err = store.Retrieve(ctx, scope2, "ws1", RetrieveOptions{})
	require.NoError(t, err)
	assert.Empty(t, results, "workspace 2 retrieve should return nothing")

	// Workspace 1 should still have 3.
	results, err = store.List(ctx, scope1, ListOptions{})
	require.NoError(t, err)
	assert.Len(t, results, 3, "workspace 1 should have 3 memories")
}

func TestPostgresMemoryStore_Save_WithSessionAndTurnRange(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	mem := &Memory{
		Type:       "fact",
		Content:    "discussed Kubernetes",
		Confidence: 0.85,
		Scope:      scope,
		SessionID:  "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
		TurnRange:  [2]int{1, 5},
	}
	require.NoError(t, store.Save(ctx, mem))

	results, err := store.Retrieve(ctx, scope, "Kubernetes", RetrieveOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", results[0].SessionID)
	assert.Equal(t, [2]int{1, 5}, results[0].TurnRange)
}

func TestPostgresMemoryStore_UpdateEmbedding(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	mem := &Memory{
		Type:       "fact",
		Content:    "likes neural networks",
		Confidence: 0.9,
		Scope:      scope,
	}
	require.NoError(t, store.Save(ctx, mem))

	// Build a 1536-dim embedding.
	embedding := make([]float32, 1536)
	for i := range embedding {
		embedding[i] = float32(i) * 0.001
	}

	err := store.UpdateEmbedding(ctx, mem.ID, embedding)
	require.NoError(t, err)

	// Verify via direct SQL that the embedding is non-null.
	var hasEmbedding bool
	err = store.Pool().QueryRow(ctx, `
		SELECT embedding IS NOT NULL
		FROM memory_observations
		WHERE entity_id = $1
		ORDER BY observed_at DESC
		LIMIT 1`, mem.ID).Scan(&hasEmbedding)
	require.NoError(t, err)
	assert.True(t, hasEmbedding, "embedding should be non-null after update")
}

func TestPostgresMemoryStore_UpdateEmbedding_NotFound(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()

	embedding := make([]float32, 1536)
	err := store.UpdateEmbedding(ctx, "00000000-0000-0000-0000-000000000000", embedding)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no observation found")
}

func TestPostgresMemoryStore_Save_NilMetadata(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()

	mem := &Memory{
		Type:       "fact",
		Content:    "no metadata",
		Confidence: 1.0,
		Scope:      testScope(testWorkspace1),
	}
	require.NoError(t, store.Save(ctx, mem))
	assert.NotEmpty(t, mem.ID)
}

func TestPostgresMemoryStore_Retrieve_PurposeFilter(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// Save two memories — both get the DB default purpose 'support_continuity'.
	mem1 := &Memory{
		Type:       "fact",
		Content:    "likes Go",
		Confidence: 0.9,
		Scope:      scope,
	}
	mem2 := &Memory{
		Type:       "fact",
		Content:    "likes Rust",
		Confidence: 0.9,
		Scope:      scope,
	}
	require.NoError(t, store.Save(ctx, mem1))
	require.NoError(t, store.Save(ctx, mem2))

	// Set mem2's purpose to 'personalization' via direct SQL.
	_, err := store.Pool().Exec(ctx,
		"UPDATE memory_entities SET purpose = 'personalization' WHERE id = $1",
		mem2.ID,
	)
	require.NoError(t, err)

	// Retrieve with purpose = 'personalization' — should return only mem2.
	// Purpose filtering was removed when migrating to PromptKit types
	// (RetrieveOptions no longer has a Purpose field). Without purpose
	// filtering all memories are returned regardless of their DB purpose.
	results, err := store.Retrieve(ctx, scope, "", RetrieveOptions{})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestPostgresMemoryStore_List_PurposeFilter(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// Save two memories — both get the DB default purpose 'support_continuity'.
	mem1 := &Memory{
		Type:       "fact",
		Content:    "prefers dark mode",
		Confidence: 0.9,
		Scope:      scope,
	}
	mem2 := &Memory{
		Type:       "fact",
		Content:    "uses vim editor",
		Confidence: 0.9,
		Scope:      scope,
	}
	require.NoError(t, store.Save(ctx, mem1))
	require.NoError(t, store.Save(ctx, mem2))

	// Set mem2's purpose to 'personalization' via direct SQL.
	_, err := store.Pool().Exec(ctx,
		"UPDATE memory_entities SET purpose = 'personalization' WHERE id = $1",
		mem2.ID,
	)
	require.NoError(t, err)

	// Purpose filtering was removed when migrating to PromptKit types
	// (ListOptions no longer has a Purpose field). Without purpose
	// filtering all memories are returned regardless of their DB purpose.
	results, err := store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestPostgresMemoryStore_ExpireMemories(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	pastTime := time.Now().Add(-1 * time.Hour)
	mem := &Memory{
		Type:       "fact",
		Content:    "expires in the past",
		Confidence: 0.9,
		Scope:      scope,
		ExpiresAt:  &pastTime,
	}
	require.NoError(t, store.Save(ctx, mem))

	// Verify it exists before expiry.
	results, err := store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1)

	expired, err := store.ExpireMemories(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), expired)

	// Verify it is gone after expiry.
	results, err = store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestPostgresMemoryStore_ExpireMemories_NoExpired(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	futureTime := time.Now().Add(1 * time.Hour)
	mem := &Memory{
		Type:       "fact",
		Content:    "expires in the future",
		Confidence: 0.9,
		Scope:      scope,
		ExpiresAt:  &futureTime,
	}
	require.NoError(t, store.Save(ctx, mem))

	expired, err := store.ExpireMemories(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), expired)

	// Verify it still exists.
	results, err := store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestPostgresMemoryStore_ExportAll(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	for i := range 5 {
		mem := &Memory{
			Type:       "fact",
			Content:    fmt.Sprintf("export fact %d", i),
			Confidence: 0.9,
			Scope:      scope,
		}
		require.NoError(t, store.Save(ctx, mem))
	}

	results, err := store.ExportAll(ctx, scope)
	require.NoError(t, err)
	assert.Len(t, results, 5, "ExportAll should return all 5 memories")
}

func TestPostgresMemoryStore_ExportAll_MissingWorkspace(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()

	_, err := store.ExportAll(ctx, map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

func TestPostgresMemoryStore_ExportAll_ExcludesForgotten(t *testing.T) {

	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// Save 3 memories.
	mems := make([]*Memory, 3)
	for i := range 3 {
		mem := &Memory{
			Type:       "fact",
			Content:    fmt.Sprintf("exportable %d", i),
			Confidence: 0.9,
			Scope:      scope,
		}
		require.NoError(t, store.Save(ctx, mem))
		mems[i] = mem
	}

	// Soft-delete one.
	require.NoError(t, store.Delete(ctx, scope, mems[0].ID))

	// ExportAll should only return the 2 non-forgotten ones.
	results, err := store.ExportAll(ctx, scope)
	require.NoError(t, err)
	assert.Len(t, results, 2, "ExportAll should exclude soft-deleted memories")
}
