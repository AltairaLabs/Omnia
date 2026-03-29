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

package api

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/altairalabs/omnia/internal/memory"
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

// Test UUID constants.
const (
	testWorkspaceID = "a0000000-0000-0000-0000-000000000001"
	testNonexistent = "b0000000-0000-0000-0000-000000000099"
)

func newTestService(t *testing.T) *MemoryService {
	t.Helper()
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	return NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
}

// mockEmbeddingProvider is a test double for memory.EmbeddingProvider.
// It records calls via a channel so tests can synchronize on async embedding.
type mockEmbeddingProvider struct {
	embedCh chan []string // receives the text slice on each Embed call
	err     error         // if non-nil, Embed returns this error
}

func newMockEmbeddingProvider(bufSize int) *mockEmbeddingProvider {
	return &mockEmbeddingProvider{embedCh: make(chan []string, bufSize)}
}

func (m *mockEmbeddingProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if m.err != nil {
		m.embedCh <- texts
		return nil, m.err
	}
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = []float32{0.1, 0.2, 0.3}
	}
	m.embedCh <- texts
	return result, nil
}

func (m *mockEmbeddingProvider) Dimensions() int { return 3 }

func TestServiceSaveMemory(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	mem := &memory.Memory{
		Type:       "preference",
		Content:    "prefers dark mode",
		Confidence: 0.9,
		Scope:      map[string]string{memory.ScopeWorkspaceID: testWorkspaceID},
	}

	err := svc.SaveMemory(ctx, mem)
	require.NoError(t, err)
	assert.NotEmpty(t, mem.ID)
	assert.False(t, mem.CreatedAt.IsZero())
}

func TestServiceSaveMemory_MissingWorkspace(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	mem := &memory.Memory{
		Type:    "preference",
		Content: "test",
		Scope:   map[string]string{},
	}

	err := svc.SaveMemory(ctx, mem)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

func TestServiceListMemories(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID}

	// Save two memories.
	for _, content := range []string{"likes Go", "uses Linux"} {
		err := svc.SaveMemory(ctx, &memory.Memory{
			Type:       "fact",
			Content:    content,
			Confidence: 0.8,
			Scope:      scope,
		})
		require.NoError(t, err)
	}

	memories, err := svc.ListMemories(ctx, scope, memory.ListOptions{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, memories, 2)
}

func TestServiceSearchMemories(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID}

	err := svc.SaveMemory(ctx, &memory.Memory{
		Type:       "preference",
		Content:    "dark mode",
		Confidence: 0.9,
		Scope:      scope,
	})
	require.NoError(t, err)

	err = svc.SaveMemory(ctx, &memory.Memory{
		Type:       "fact",
		Content:    "something else",
		Confidence: 0.7,
		Scope:      scope,
	})
	require.NoError(t, err)

	results, err := svc.SearchMemories(ctx, scope, "dark", memory.RetrieveOptions{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "dark mode", results[0].Content)
}

func TestServiceDeleteMemory(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID}

	mem := &memory.Memory{
		Type:       "fact",
		Content:    "to be forgotten",
		Confidence: 0.8,
		Scope:      scope,
	}
	require.NoError(t, svc.SaveMemory(ctx, mem))

	err := svc.DeleteMemory(ctx, scope, mem.ID)
	require.NoError(t, err)

	// Memory should no longer appear in list.
	memories, err := svc.ListMemories(ctx, scope, memory.ListOptions{Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, memories)
}

func TestServiceDeleteMemory_NotFound(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID}

	err := svc.DeleteMemory(ctx, scope, testNonexistent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestServiceDeleteAllMemories(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID}

	for i := 0; i < 3; i++ {
		require.NoError(t, svc.SaveMemory(ctx, &memory.Memory{
			Type:       "fact",
			Content:    fmt.Sprintf("memory %d", i),
			Confidence: 0.8,
			Scope:      scope,
		}))
	}

	err := svc.DeleteAllMemories(ctx, scope)
	require.NoError(t, err)

	memories, err := svc.ListMemories(ctx, scope, memory.ListOptions{Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, memories)
}

func TestServiceDeleteAllMemories_MissingWorkspace(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	err := svc.DeleteAllMemories(ctx, map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

func TestMemoryService_SaveWithEmbedding(t *testing.T) {
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	provider := newMockEmbeddingProvider(1)
	logger := zap.New(zap.UseDevMode(true))
	embSvc := memory.NewEmbeddingService(store, provider, logger)
	svc := NewMemoryService(store, embSvc, MemoryServiceConfig{}, logr.Discard())

	ctx := context.Background()
	mem := &memory.Memory{
		Type:       "preference",
		Content:    "likes Go",
		Confidence: 0.9,
		Scope:      map[string]string{memory.ScopeWorkspaceID: testWorkspaceID},
	}

	err := svc.SaveMemory(ctx, mem)
	require.NoError(t, err)
	assert.NotEmpty(t, mem.ID)

	// Confirm embedding was attempted asynchronously.
	select {
	case texts := <-provider.embedCh:
		assert.Equal(t, []string{"likes Go"}, texts)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for async embedding call")
	}
}

func TestMemoryService_SaveWithEmbedding_EmbedError(t *testing.T) {
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	provider := newMockEmbeddingProvider(1)
	provider.err = errors.New("provider unavailable")
	logger := zap.New(zap.UseDevMode(true))
	embSvc := memory.NewEmbeddingService(store, provider, logger)
	svc := NewMemoryService(store, embSvc, MemoryServiceConfig{}, logr.Discard())

	ctx := context.Background()
	mem := &memory.Memory{
		Type:       "fact",
		Content:    "something",
		Confidence: 0.8,
		Scope:      map[string]string{memory.ScopeWorkspaceID: testWorkspaceID},
	}

	// Save should succeed even when embedding fails.
	err := svc.SaveMemory(ctx, mem)
	require.NoError(t, err)
	assert.NotEmpty(t, mem.ID)

	// Embedding was attempted (error is logged, not propagated).
	select {
	case <-provider.embedCh:
		// received — embedding was attempted
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for async embedding call")
	}
}

func TestMemoryService_SaveWithoutEmbedding(t *testing.T) {
	// nil embeddingSvc — save works normally, no panic.
	svc := newTestService(t)
	ctx := context.Background()

	mem := &memory.Memory{
		Type:       "preference",
		Content:    "no embedding configured",
		Confidence: 0.7,
		Scope:      map[string]string{memory.ScopeWorkspaceID: testWorkspaceID},
	}

	err := svc.SaveMemory(ctx, mem)
	require.NoError(t, err)
	assert.NotEmpty(t, mem.ID)
}

func TestMemoryService_SaveWithTTL(t *testing.T) {
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	cfg := MemoryServiceConfig{DefaultTTL: 24 * time.Hour}
	svc := NewMemoryService(store, nil, cfg, logr.Discard())

	ctx := context.Background()
	before := time.Now()
	mem := &memory.Memory{
		Type:       "fact",
		Content:    "TTL test",
		Confidence: 0.9,
		Scope:      map[string]string{memory.ScopeWorkspaceID: testWorkspaceID},
	}

	err := svc.SaveMemory(ctx, mem)
	require.NoError(t, err)
	require.NotNil(t, mem.ExpiresAt, "ExpiresAt should be set by DefaultTTL")
	assert.True(t, mem.ExpiresAt.After(before.Add(23*time.Hour)),
		"ExpiresAt should be ~24h from now")
	assert.True(t, mem.ExpiresAt.Before(before.Add(25*time.Hour)),
		"ExpiresAt should be ~24h from now")
}

func TestMemoryService_SaveWithExplicitExpiry(t *testing.T) {
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	cfg := MemoryServiceConfig{DefaultTTL: 24 * time.Hour}
	svc := NewMemoryService(store, nil, cfg, logr.Discard())

	ctx := context.Background()
	explicit := time.Now().Add(7 * 24 * time.Hour).Truncate(time.Second)
	mem := &memory.Memory{
		Type:       "fact",
		Content:    "explicit expiry test",
		Confidence: 0.9,
		ExpiresAt:  &explicit,
		Scope:      map[string]string{memory.ScopeWorkspaceID: testWorkspaceID},
	}

	err := svc.SaveMemory(ctx, mem)
	require.NoError(t, err)
	require.NotNil(t, mem.ExpiresAt)
	assert.True(t, mem.ExpiresAt.Equal(explicit),
		"explicit ExpiresAt should not be overridden by DefaultTTL")
}
