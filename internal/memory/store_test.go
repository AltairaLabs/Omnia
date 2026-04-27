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

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"

	pgmigrate "github.com/altairalabs/omnia/internal/memory/postgres"
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

	// Run all memory migrations.
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

func TestPostgresMemoryStore_Save_TrustModelFromProvenance(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	cases := []struct {
		name           string
		provenance     string
		wantTrustModel string
		wantSourceType string
	}{
		{"user_requested", "user_requested", "explicit", "user_requested"},
		{"operator_curated", "operator_curated", "curated", "operator_curated"},
		{"agent_extracted", "agent_extracted", "inferred", "conversation_extraction"},
		{"system_generated", "system_generated", "inferred", "system_generated"},
		{"no_provenance_uses_schema_defaults", "", "inferred", "conversation_extraction"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			meta := map[string]any{}
			if tc.provenance != "" {
				meta[pkmemory.MetaKeyProvenance] = tc.provenance
			}
			mem := &Memory{
				Type: "fact", Content: "trust-" + tc.name, Confidence: 1.0,
				Scope: scope, Metadata: meta,
			}
			require.NoError(t, store.Save(ctx, mem))
			require.NotEmpty(t, mem.ID)

			var trustModel, sourceType string
			row := store.Pool().QueryRow(ctx,
				`SELECT trust_model, source_type FROM memory_entities WHERE id = $1`, mem.ID)
			require.NoError(t, row.Scan(&trustModel, &sourceType))
			assert.Equal(t, tc.wantTrustModel, trustModel, "trust_model")
			assert.Equal(t, tc.wantSourceType, sourceType, "source_type")
		})
	}
}

func TestPostgresMemoryStore_Save_PurposeFromMetadata(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	mem := &Memory{
		Type: "fact", Content: "purpose-tagged", Confidence: 1.0, Scope: scope,
		Metadata: map[string]any{MetaKeyPurpose: "personalisation"},
	}
	require.NoError(t, store.Save(ctx, mem))

	var got string
	require.NoError(t, store.Pool().QueryRow(ctx,
		`SELECT purpose FROM memory_entities WHERE id = $1`, mem.ID).Scan(&got))
	assert.Equal(t, "personalisation", got)
}

func TestPostgresMemoryStore_Save_MissingPurposeUsesSchemaDefault(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	mem := &Memory{
		Type: "fact", Content: "no purpose tag", Confidence: 1.0, Scope: scope,
	}
	require.NoError(t, store.Save(ctx, mem))

	var got string
	require.NoError(t, store.Pool().QueryRow(ctx,
		`SELECT purpose FROM memory_entities WHERE id = $1`, mem.ID).Scan(&got))
	assert.Equal(t, "support_continuity", got, "missing purpose must fall through to schema default")
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

// TestPostgresMemoryStore_Retrieve_FTSQueryFindsTokenizedMatches reproduces
// the bug where a recall query like "my name" returned zero hits against a
// memory whose content was "User's name is Slim Shard" — ILIKE substring
// matching ignored stopwords and word boundaries. With Postgres FTS
// (000003_observation_fts) the query tokenises to {name}, matches the
// stored vector, and ranks via ts_rank_cd. This test fails with the old
// ILIKE implementation and passes with FTS.
func TestPostgresMemoryStore_Retrieve_FTSQueryFindsTokenizedMatches(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	require.NoError(t, store.Save(ctx, &Memory{
		Type: "fact", Content: "User's name is Slim Shard", Confidence: 0.9, Scope: scope,
	}))
	require.NoError(t, store.Save(ctx, &Memory{
		Type: "preference", Content: "Slim Shard likes blue", Confidence: 0.9, Scope: scope,
	}))
	require.NoError(t, store.Save(ctx, &Memory{
		Type: "fact", Content: "Works at Acme Corp", Confidence: 0.8, Scope: scope,
	}))

	results, err := store.Retrieve(ctx, scope, "my name", RetrieveOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, results, "FTS should find the 'name'-bearing memory")

	contents := make([]string, 0, len(results))
	for _, m := range results {
		contents = append(contents, m.Content)
	}
	assert.Contains(t, contents, "User's name is Slim Shard")
}

// TestPostgresMemoryStore_Save_StructuredKeyDedup proves that two writes
// with the same About={kind, key} on the same scope land under one
// entity, with the older observation marked superseded. This is the
// fix for the "user changes name and old name memory still remains"
// bug: the agent passes about={kind:"user", key:"name"} on both
// writes, the server detects the conflict via the unique index, and
// atomically supersedes the prior value.
func TestPostgresMemoryStore_Save_StructuredKeyDedup(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	first := &Memory{
		Type: "fact", Content: "User's name is Slim Shard",
		Confidence: 1.0, Scope: scope,
		Metadata: map[string]any{
			MetaKeyAboutKind: "user",
			MetaKeyAboutKey:  "name",
		},
	}
	require.NoError(t, store.Save(ctx, first))
	require.NotEmpty(t, first.ID)

	second := &Memory{
		Type: "fact", Content: "User's name is Phil Collins",
		Confidence: 1.0, Scope: scope,
		Metadata: map[string]any{
			MetaKeyAboutKind: "user",
			MetaKeyAboutKey:  "name",
		},
	}
	require.NoError(t, store.Save(ctx, second))

	// Both observations should live under the SAME entity — second
	// reuses first's entity_id via the unique index conflict path.
	assert.Equal(t, first.ID, second.ID,
		"second write should land under the same entity as first")

	// Recall should return only the latest active observation for that
	// entity (the older one is superseded).
	results, err := store.Retrieve(ctx, scope, "name", RetrieveOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1, "only one active observation per entity")
	assert.Equal(t, "User's name is Phil Collins", results[0].Content)
}

// TestPostgresMemoryStore_FindSimilarObservations_RanksByCosine proves
// the embedding-similarity dedup query returns matches ordered by
// cosine descending, scoped to the (workspace, user) tuple, and only
// over active observations. Without this the service-layer
// dedup-on-write path can't tell whether a free-form remember is a
// near-duplicate of something already stored.
func TestPostgresMemoryStore_FindSimilarObservations_RanksByCosine(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	near := &Memory{Type: "preference", Content: "User likes blue", Confidence: 0.9, Scope: scope}
	require.NoError(t, store.Save(ctx, near))
	require.NoError(t, store.UpdateEmbedding(ctx, near.ID, repeatFloat(0.1, 1536)))

	far := &Memory{Type: "fact", Content: "Works at Acme", Confidence: 0.9, Scope: scope}
	require.NoError(t, store.Save(ctx, far))
	require.NoError(t, store.UpdateEmbedding(ctx, far.ID, repeatFloat(0.9, 1536)))

	// Query embedding nearly identical to `near` → matches it strongly.
	matches, err := store.FindSimilarObservations(ctx, scope, repeatFloat(0.1, 1536), 5, 0.5)
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.Equal(t, "User likes blue", matches[0].Content)
	assert.Greater(t, matches[0].Similarity, 0.99,
		"near-identical embedding should score ~1.0")
}

// TestPostgresMemoryStore_AppendObservationToEntity_AtomicallySupersedes
// proves the embedding-similarity auto-supersede helper attaches the
// new observation to the existing entity AND marks all prior active
// observations inactive in one transaction. End state: one active
// observation under the entity (the new one), one superseded.
func TestPostgresMemoryStore_AppendObservationToEntity_AtomicallySupersedes(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	original := &Memory{Type: "preference", Content: "User likes blue", Confidence: 0.9, Scope: scope}
	require.NoError(t, store.Save(ctx, original))

	updated := &Memory{Type: "preference", Content: "User loves blue", Confidence: 0.9, Scope: scope}
	supersededIDs, err := store.AppendObservationToEntity(ctx, original.ID, updated)
	require.NoError(t, err)
	require.NotEmpty(t, supersededIDs, "prior observation should be marked superseded")
	assert.Equal(t, original.ID, updated.ID, "new observation lives under the existing entity")

	results, err := store.Retrieve(ctx, scope, "blue", RetrieveOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1, "active filter excludes the superseded observation")
	assert.Equal(t, "User loves blue", results[0].Content)
}

// TestPostgresMemoryStore_FindSimilarObservations_HonoursThreshold verifies
// the minSimilarity filter at the SQL level — too-low matches don't
// reach the service layer at all.
func TestPostgresMemoryStore_FindSimilarObservations_HonoursThreshold(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	mem := &Memory{Type: "fact", Content: "Vegetarian", Confidence: 0.9, Scope: scope}
	require.NoError(t, store.Save(ctx, mem))
	require.NoError(t, store.UpdateEmbedding(ctx, mem.ID, oneHotFloat(0, 1536)))

	// Query embedding orthogonal to the stored one (different one-hot
	// position). Cosine similarity = 0 → threshold 0.5 rejects it.
	matches, err := store.FindSimilarObservations(ctx, scope, oneHotFloat(100, 1536), 5, 0.5)
	require.NoError(t, err)
	assert.Empty(t, matches, "orthogonal embeddings should be filtered by threshold")
}

// repeatFloat returns a float32 slice of length n filled with v —
// useful for synthesizing test embeddings without the cost of a real
// embedding call. Vectors filled with the same constant are parallel
// (cosine similarity = 1.0 regardless of magnitude); use oneHotFloat
// when you need orthogonality.
func repeatFloat(v float32, n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = v
	}
	return out
}

// oneHotFloat returns a length-n vector with a 1.0 at position pos
// and zeros elsewhere. Two one-hot vectors at different positions are
// orthogonal (cosine similarity = 0), useful for testing that the
// threshold filter rejects unrelated embeddings.
func oneHotFloat(pos, n int) []float32 {
	out := make([]float32, n)
	if pos >= 0 && pos < n {
		out[pos] = 1.0
	}
	return out
}

// TestPostgresMemoryStore_Save_StructuredKeyDedup_DifferentKeys verifies
// that different About keys under the same scope don't collide — they
// each get their own entity.
func TestPostgresMemoryStore_Save_StructuredKeyDedup_DifferentKeys(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	name := &Memory{
		Type: "fact", Content: "User's name is Phil",
		Confidence: 1.0, Scope: scope,
		Metadata: map[string]any{
			MetaKeyAboutKind: "user",
			MetaKeyAboutKey:  "name",
		},
	}
	require.NoError(t, store.Save(ctx, name))

	loc := &Memory{
		Type: "fact", Content: "User lives in Berlin",
		Confidence: 1.0, Scope: scope,
		Metadata: map[string]any{
			MetaKeyAboutKind: "user",
			MetaKeyAboutKey:  "location",
		},
	}
	require.NoError(t, store.Save(ctx, loc))

	assert.NotEqual(t, name.ID, loc.ID, "different keys → different entities")
}

// TestPostgresMemoryStore_Retrieve_FTSRanksByRelevance verifies that when
// multiple observations match the FTS query, ts_rank_cd surfaces the
// most-relevant one first — the agent's recall tool can then trust the
// ordering and stop at the first few results.
func TestPostgresMemoryStore_Retrieve_FTSRanksByRelevance(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// First memory mentions "name" once in passing; second is entirely
	// about the user's name — the FTS rank should put the second first.
	require.NoError(t, store.Save(ctx, &Memory{
		Type: "context", Content: "Works at Acme Corp under that name", Confidence: 0.8, Scope: scope,
	}))
	require.NoError(t, store.Save(ctx, &Memory{
		Type: "fact", Content: "User's name is Slim Shard", Confidence: 0.9, Scope: scope,
	}))

	results, err := store.Retrieve(ctx, scope, "name", RetrieveOptions{})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 1)
	assert.Equal(t, "User's name is Slim Shard", results[0].Content,
		"strongest 'name' match should rank first")
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

func TestPostgresMemoryStore_BatchDelete(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// Save 5 memories.
	for i := range 5 {
		mem := &Memory{
			Type:       "fact",
			Content:    fmt.Sprintf("batch fact %d", i),
			Confidence: 0.9,
			Scope:      scope,
		}
		require.NoError(t, store.Save(ctx, mem))
	}

	// BatchDelete with limit=3 should delete 3 rows.
	n, err := store.BatchDelete(ctx, scope, 3)
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	// 2 should remain.
	results, err := store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Delete the rest.
	n, err = store.BatchDelete(ctx, scope, 10)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	// All gone.
	results, err = store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestPostgresMemoryStore_BatchDelete_NoRows(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// BatchDelete on empty store returns 0 with no error.
	n, err := store.BatchDelete(ctx, scope, 500)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestPostgresMemoryStore_BatchDelete_MissingWorkspace(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	_, err := store.BatchDelete(ctx, map[string]string{}, 500)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}
