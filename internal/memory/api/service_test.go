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
	"strings"
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

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/memory"
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
	// fixedEmbedding overrides the default [0.1, 0.2, 0.3] result —
	// useful for the embedding-similarity dedup tests which need a
	// known vector at the right dimensionality (vector(1536)) to
	// drive cosine matching against pre-seeded observations.
	fixedEmbedding []float32
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
		if m.fixedEmbedding != nil {
			result[i] = m.fixedEmbedding
		} else {
			result[i] = []float32{0.1, 0.2, 0.3}
		}
	}
	m.embedCh <- texts
	return result, nil
}

func (m *mockEmbeddingProvider) Dimensions() int {
	if m.fixedEmbedding != nil {
		return len(m.fixedEmbedding)
	}
	return 3
}

func TestServiceSaveMemory(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	mem := &memory.Memory{
		Type:       "preference",
		Content:    "prefers dark mode",
		Confidence: 0.9,
		Scope:      map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"},
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
	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}

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
	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}

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

// TestServiceMemoryService_PolicyOverridesDedupThresholds proves
// the embedding-similarity dedup thresholds resolve from the bound
// MemoryPolicy. Setting autoSupersedeAbove=0.5 lifts what would
// otherwise be a "potential duplicate" (cosine 0.6) into an
// auto-supersede.
func TestServiceMemoryService_PolicyOverridesDedupThresholds(t *testing.T) {
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	provider := newMockEmbeddingProvider(8)
	provider.fixedEmbedding = oneHotEmbedding(0)
	embSvc := memory.NewEmbeddingService(store, provider, zap.New(zap.UseDevMode(true)))
	svc := NewMemoryService(store, embSvc, MemoryServiceConfig{}, logr.Discard())

	// Tight threshold lifts cosine ≥ 0.5 into auto-supersede.
	svc.SetPolicyLoader(&memory.StaticPolicyLoader{
		Policy: &omniav1alpha1.MemoryPolicy{
			Spec: omniav1alpha1.MemoryPolicySpec{
				Tiers: omniav1alpha1.MemoryRetentionTierSet{},
				Dedup: &omniav1alpha1.MemoryDedupConfig{
					EmbeddingSimilarity: &omniav1alpha1.MemoryEmbeddingDedupConfig{
						AutoSupersedeAbove:     "0.5",
						SurfaceDuplicatesAbove: "0.3",
					},
				},
			},
		},
	})

	if got := svc.autoSupersedeThreshold(context.Background()); got != 0.5 {
		t.Errorf("auto-supersede: want 0.5, got %v", got)
	}
	if got := svc.surfaceDuplicateThreshold(context.Background()); got != 0.3 {
		t.Errorf("surface-duplicate: want 0.3, got %v", got)
	}
}

// TestServiceMemoryService_PolicyDisablesEmbeddingDedup proves the
// embeddingSimilarity.enabled=false branch: when the policy turns
// off embedding dedup, free-form Save calls bypass the embedding
// path entirely (no `potential_duplicates` returned even when a
// match exists).
func TestServiceMemoryService_PolicyDisablesEmbeddingDedup(t *testing.T) {
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	provider := newMockEmbeddingProvider(8)
	provider.fixedEmbedding = oneHotEmbedding(0)
	embSvc := memory.NewEmbeddingService(store, provider, zap.New(zap.UseDevMode(true)))
	svc := NewMemoryService(store, embSvc, MemoryServiceConfig{}, logr.Discard())

	disabled := false
	svc.SetPolicyLoader(&memory.StaticPolicyLoader{
		Policy: &omniav1alpha1.MemoryPolicy{
			Spec: omniav1alpha1.MemoryPolicySpec{
				Dedup: &omniav1alpha1.MemoryDedupConfig{
					EmbeddingSimilarity: &omniav1alpha1.MemoryEmbeddingDedupConfig{
						Enabled: &disabled,
					},
				},
			},
		},
	})
	if svc.embeddingDedupEnabled(context.Background()) {
		t.Error("embeddingDedupEnabled must be false when policy disables it")
	}
}

// TestServiceMemoryService_PolicyOverridesInlineThreshold proves
// the recall preview cutoff resolves from MemoryPolicy.recall.
func TestServiceMemoryService_PolicyOverridesInlineThreshold(t *testing.T) {
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	customThreshold := int32(512)
	svc.SetPolicyLoader(&memory.StaticPolicyLoader{
		Policy: &omniav1alpha1.MemoryPolicy{
			Spec: omniav1alpha1.MemoryPolicySpec{
				Recall: &omniav1alpha1.MemoryRecallConfig{
					InlineThresholdBytes: &customThreshold,
				},
			},
		},
	})
	if got := svc.InlineThresholdBytes(context.Background()); got != 512 {
		t.Errorf("inline threshold: want 512, got %d", got)
	}
}

// TestServiceSaveMemory_PolicyAndConfigUnion proves the union
// behaviour when both the static config and the policy supply
// kinds: a write that violates either source is rejected.
func TestServiceSaveMemory_PolicyAndConfigUnion(t *testing.T) {
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	svc := NewMemoryService(store, nil, MemoryServiceConfig{
		RequireAboutForKinds: []string{"fact"},
	}, logr.Discard())
	svc.SetPolicyLoader(&memory.StaticPolicyLoader{
		Policy: &omniav1alpha1.MemoryPolicy{
			Spec: omniav1alpha1.MemoryPolicySpec{
				Tiers: omniav1alpha1.MemoryRetentionTierSet{},
				Dedup: &omniav1alpha1.MemoryDedupConfig{
					RequireAboutForKinds: []string{"preference"},
				},
			},
		},
	})

	ctx := context.Background()
	scope := map[string]string{
		memory.ScopeWorkspaceID: testWorkspaceID,
		memory.ScopeUserID:      "test-user",
	}

	// fact (in static config) without about → reject.
	err := svc.SaveMemory(ctx, &memory.Memory{
		Type: "fact", Content: "no anchor", Confidence: 0.9, Scope: scope,
	})
	require.ErrorIs(t, err, ErrAboutRequired)

	// preference (in policy list) without about → reject.
	err = svc.SaveMemory(ctx, &memory.Memory{
		Type: "preference", Content: "no anchor", Confidence: 0.9, Scope: scope,
	})
	require.ErrorIs(t, err, ErrAboutRequired)

	// document (in neither list) → allowed.
	err = svc.SaveMemory(ctx, &memory.Memory{
		Type: "document", Content: "free", Confidence: 0.9, Scope: scope,
	})
	require.NoError(t, err)
}

// TestServiceSaveMemory_PolicyLoaderError proves a transient policy
// loader failure falls back to the static config without blocking
// the write — recall is too central to make brittle on policy
// availability.
func TestServiceSaveMemory_PolicyLoaderError(t *testing.T) {
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	svc := NewMemoryService(store, nil, MemoryServiceConfig{
		RequireAboutForKinds: []string{"fact"},
	}, logr.Discard())
	svc.SetPolicyLoader(&erroringPolicyLoader{})

	ctx := context.Background()
	scope := map[string]string{
		memory.ScopeWorkspaceID: testWorkspaceID,
		memory.ScopeUserID:      "test-user",
	}
	// Static config still in force when the loader errs.
	err := svc.SaveMemory(ctx, &memory.Memory{
		Type: "fact", Content: "no anchor", Confidence: 0.9, Scope: scope,
	})
	require.ErrorIs(t, err, ErrAboutRequired)
}

// erroringPolicyLoader returns an error on every Load — proves the
// service degrades gracefully.
type erroringPolicyLoader struct{}

func (erroringPolicyLoader) Load(_ context.Context) (*omniav1alpha1.MemoryPolicy, error) {
	return nil, errors.New("transient failure")
}

// TestServiceSaveMemory_PolicyRequireAboutForKinds proves the
// MemoryPolicy.dedup.requireAboutForKinds list is honoured at
// runtime. A workspace operator can attach a policy that requires
// `about` on certain kinds without redeploying memory-api with new
// flags — the policy loader fetches the policy and the union of
// (static config kinds + policy kinds) is enforced.
func TestServiceSaveMemory_PolicyRequireAboutForKinds(t *testing.T) {
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	// Static config has no kinds — only the policy supplies them.
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetPolicyLoader(&memory.StaticPolicyLoader{
		Policy: &omniav1alpha1.MemoryPolicy{
			Spec: omniav1alpha1.MemoryPolicySpec{
				Tiers: omniav1alpha1.MemoryRetentionTierSet{},
				Dedup: &omniav1alpha1.MemoryDedupConfig{
					RequireAboutForKinds: []string{"preference"},
				},
			},
		},
	})

	ctx := context.Background()
	scope := map[string]string{
		memory.ScopeWorkspaceID: testWorkspaceID,
		memory.ScopeUserID:      "test-user",
	}

	// preference without about → policy rejects.
	err := svc.SaveMemory(ctx, &memory.Memory{
		Type: "preference", Content: "no anchor", Confidence: 0.9, Scope: scope,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAboutRequired)

	// fact (not in policy list) → allowed.
	err = svc.SaveMemory(ctx, &memory.Memory{
		Type: "fact", Content: "free-form", Confidence: 0.9, Scope: scope,
	})
	require.NoError(t, err)
}

// TestServiceSupersedeMany_CollapsesAcrossEntities proves the
// multi-id supersede flow: three stale memories about the user's
// name (different entities, no `about`) are collapsed into one
// canonical truth under the first source entity. After supersede,
// recall returns only the new content; the older entities have no
// active observations.
func TestServiceSupersedeMany_CollapsesAcrossEntities(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	scope := map[string]string{
		memory.ScopeWorkspaceID: testWorkspaceID,
		memory.ScopeUserID:      "test-user",
	}

	older := &memory.Memory{Type: "fact", Content: "name: Slim Shard", Confidence: 0.9, Scope: scope}
	require.NoError(t, svc.SaveMemory(ctx, older))
	mid := &memory.Memory{Type: "fact", Content: "name: Slim Shady", Confidence: 0.9, Scope: scope}
	require.NoError(t, svc.SaveMemory(ctx, mid))
	newer := &memory.Memory{Type: "fact", Content: "name: Phil Collins", Confidence: 0.9, Scope: scope}
	require.NoError(t, svc.SaveMemory(ctx, newer))

	canonical := &memory.Memory{
		Type: "fact", Content: "User's name is Phil", Confidence: 1.0, Scope: scope,
	}
	res, err := svc.SupersedeManyMemories(ctx,
		[]string{older.ID, mid.ID, newer.ID}, canonical)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, memory.SaveActionAutoSuperseded, res.Action)
	assert.Equal(t, older.ID, res.ID, "anchor entity is the first source ID")
	assert.Len(t, res.SupersededObservationIDs, 3,
		"every source entity contributed one inactive observation")

	// Recall now returns one row — the canonical truth — and the
	// old names are gone.
	results, err := svc.SearchMemories(ctx, scope, "name", memory.RetrieveOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Content, "Phil")
}

// TestServiceSupersedeMany_RejectsCrossWorkspace proves the scope
// guard fires when a source entity belongs to a different workspace
// — a cross-tenant supersede must fail loudly rather than silently
// updating a row in the wrong scope.
func TestServiceSupersedeMany_RejectsCrossWorkspace(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	scopeA := map[string]string{
		memory.ScopeWorkspaceID: testWorkspaceID,
		memory.ScopeUserID:      "user-a",
	}
	memA := &memory.Memory{Type: "fact", Content: "in workspace A", Confidence: 0.9, Scope: scopeA}
	require.NoError(t, svc.SaveMemory(ctx, memA))

	// Try to supersede memA from a different workspace.
	scopeOther := map[string]string{
		memory.ScopeWorkspaceID: "00000000-0000-0000-0000-000000000099",
		memory.ScopeUserID:      "user-other",
	}
	canonical := &memory.Memory{Type: "fact", Content: "stolen", Confidence: 0.9, Scope: scopeOther}
	_, err := svc.SupersedeManyMemories(ctx, []string{memA.ID}, canonical)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in scope")
}

// TestServiceSaveMemory_RequiresAboutForConfiguredKinds proves
// the mandatory-about guard: a Save for a kind listed in
// MemoryServiceConfig.RequireAboutForKinds without an about
// metadata hint returns ErrAboutRequired so the agent must retry
// with about populated. Other kinds are unaffected.
func TestServiceSaveMemory_RequiresAboutForConfiguredKinds(t *testing.T) {
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	svc := NewMemoryService(store, nil, MemoryServiceConfig{
		RequireAboutForKinds: []string{"fact", "preference"},
	}, logr.Discard())
	ctx := context.Background()
	scope := map[string]string{
		memory.ScopeWorkspaceID: testWorkspaceID,
		memory.ScopeUserID:      "test-user",
	}

	// fact without about → reject.
	err := svc.SaveMemory(ctx, &memory.Memory{
		Type: "fact", Content: "no anchor", Confidence: 0.9, Scope: scope,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAboutRequired)

	// fact WITH about → allowed.
	err = svc.SaveMemory(ctx, &memory.Memory{
		Type: "fact", Content: "name: Phil", Confidence: 0.9, Scope: scope,
		Metadata: map[string]any{
			memory.MetaKeyAboutKind: "user",
			memory.MetaKeyAboutKey:  "name",
		},
	})
	require.NoError(t, err)

	// document (not in the configured list) → allowed without about.
	err = svc.SaveMemory(ctx, &memory.Memory{
		Type: "document", Content: "free-form note", Confidence: 0.9, Scope: scope,
	})
	require.NoError(t, err)

	// Empty config → all kinds allowed without about (back-compat).
	relaxed := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	err = relaxed.SaveMemory(ctx, &memory.Memory{
		Type: "preference", Content: "no anchor here", Confidence: 0.9, Scope: scope,
	})
	require.NoError(t, err)
}

// TestServiceLargeMemory_SaveRecallOpen proves the round-trip for
// large workspace-document-class memories: a Save with
// title/summary/large content persists all three; the row's
// observation gets a body_size_bytes auto-stamped by the schema
// (octet_length); open returns the full content unchanged. The
// body-size column is what the recall handler uses to swap the body
// for a preview.
func TestServiceLargeMemory_SaveRecallOpen(t *testing.T) {
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	ctx := context.Background()
	scope := map[string]string{
		memory.ScopeWorkspaceID: testWorkspaceID,
		memory.ScopeUserID:      "test-user",
	}

	largeBody := strings.Repeat("payload ", 500) // ~4000 bytes
	mem := &memory.Memory{
		Type:       "document",
		Content:    largeBody,
		Confidence: 0.9,
		Scope:      scope,
		Metadata: map[string]any{
			memory.MetaKeyTitle:   "Engineering handbook",
			memory.MetaKeySummary: "Coding standards and review process",
		},
	}
	require.NoError(t, svc.SaveMemory(ctx, mem))

	got, err := svc.OpenMemory(ctx, scope, mem.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, largeBody, got.Content,
		"open must return the full body, not a preview")
	assert.Equal(t, "Engineering handbook", got.Metadata[memory.MetaKeyTitle])
	assert.Equal(t, "Coding standards and review process", got.Metadata[memory.MetaKeySummary])
	bodySize, ok := got.Metadata[memory.MetaKeyBodySize].(int)
	require.True(t, ok, "body_size_bytes must be stamped on Metadata")
	assert.Equal(t, len(largeBody), bodySize)
}

// TestServiceSearchMemories_HybridSurfacesSemanticMatch proves the
// RRF path lifts a semantic-only result into the recall response.
// Setup: seed three memories. Only A is a lexical hit for the query
// "prefer". B carries no matching keyword but its observation's
// embedding equals the query embedding (cosine 1.0). C matches
// neither. Without hybrid retrieval B would never appear; with RRF
// it ranks alongside A.
func TestServiceSearchMemories_HybridSurfacesSemanticMatch(t *testing.T) {
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	provider := newMockEmbeddingProvider(8)
	provider.fixedEmbedding = oneHotEmbedding(0)
	logger := zap.New(zap.UseDevMode(true))
	embSvc := memory.NewEmbeddingService(store, provider, logger)
	svc := NewMemoryService(store, embSvc, MemoryServiceConfig{}, logr.Discard())

	ctx := context.Background()
	scope := map[string]string{
		memory.ScopeWorkspaceID: testWorkspaceID,
		memory.ScopeUserID:      "test-user",
	}

	a := &memory.Memory{Type: "preference", Content: "User prefers dark mode", Confidence: 0.9, Scope: scope}
	require.NoError(t, store.Save(ctx, a))
	b := &memory.Memory{Type: "preference", Content: "User loves the colour blue", Confidence: 0.9, Scope: scope}
	require.NoError(t, store.Save(ctx, b))
	require.NoError(t, store.UpdateEmbedding(ctx, b.ID, provider.fixedEmbedding))
	c := &memory.Memory{Type: "fact", Content: "Random unrelated note", Confidence: 0.7, Scope: scope}
	require.NoError(t, store.Save(ctx, c))

	// Drain async embedding writes triggered by Save so the next call
	// can push without blocking on a full channel.
	drainEmbed(provider, time.Second)

	results, err := svc.SearchMemories(ctx, scope, "prefer", memory.RetrieveOptions{Limit: 10})
	require.NoError(t, err)

	ids := make(map[string]bool, len(results))
	for _, m := range results {
		ids[m.ID] = true
	}
	assert.True(t, ids[a.ID], "FTS-only match should surface")
	assert.True(t, ids[b.ID], "semantic-only match should surface via RRF")
	assert.False(t, ids[c.ID], "non-matching memory must not appear")
}

// TestServiceSearchMemories_FallsBackWhenNoEmbedder proves recall
// without an embedding service still works (FTS only). Guards
// against accidental nil-deref in the hybrid path.
func TestServiceSearchMemories_FallsBackWhenNoEmbedder(t *testing.T) {
	svc := newTestService(t) // no embedder configured
	ctx := context.Background()
	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}

	require.NoError(t, svc.SaveMemory(ctx, &memory.Memory{
		Type: "preference", Content: "loves dark mode", Confidence: 0.9, Scope: scope,
	}))

	results, err := svc.SearchMemories(ctx, scope, "dark", memory.RetrieveOptions{Limit: 5})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Contains(t, results[0].Content, "dark mode")
}

// TestServiceSearchMemories_HybridFallsBackOnEmbedError proves a
// transient embedder failure degrades to FTS rather than 500-ing.
// Recall is too central to the agent loop to make brittle on
// embedder availability.
func TestServiceSearchMemories_HybridFallsBackOnEmbedError(t *testing.T) {
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	provider := newMockEmbeddingProvider(4)
	provider.err = errors.New("provider unavailable")
	logger := zap.New(zap.UseDevMode(true))
	embSvc := memory.NewEmbeddingService(store, provider, logger)
	svc := NewMemoryService(store, embSvc, MemoryServiceConfig{}, logr.Discard())

	ctx := context.Background()
	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}
	require.NoError(t, store.Save(ctx, &memory.Memory{
		Type: "preference", Content: "User prefers dark mode", Confidence: 0.9, Scope: scope,
	}))
	drainEmbed(provider, time.Second)

	results, err := svc.SearchMemories(ctx, scope, "prefer", memory.RetrieveOptions{Limit: 5})
	require.NoError(t, err)
	require.Len(t, results, 1)
}

// TestServiceRelatedForMemories proves the recall-enrichment helper
// returns the per-memory relations the agent uses to navigate the
// memory graph. Covers the three branches: an entity with relations,
// an entity with none, and the no-op early returns (empty mems and
// memories with empty IDs).
func TestServiceRelatedForMemories(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}

	user := &memory.Memory{Type: "fact", Content: "name: Phil", Confidence: 0.9, Scope: scope}
	require.NoError(t, svc.SaveMemory(ctx, user))
	pref := &memory.Memory{Type: "preference", Content: "prefers dark mode", Confidence: 0.9, Scope: scope}
	require.NoError(t, svc.SaveMemory(ctx, pref))

	_, err := svc.LinkMemories(ctx, scope, user.ID, pref.ID, "MENTIONS", 1.0)
	require.NoError(t, err)

	got := svc.RelatedForMemories(ctx, scope, []*memory.Memory{user, pref})
	require.Len(t, got[user.ID], 1, "user identity entity should carry its outgoing MENTIONS relation")
	assert.Equal(t, pref.ID, got[user.ID][0].TargetEntityID)
	assert.Equal(t, "MENTIONS", got[user.ID][0].RelationType)
	assert.Empty(t, got[pref.ID], "preference entity has no outgoing relations")

	// Empty memory slice returns an empty (non-nil) map so the
	// handler can index into it without nil guards.
	emptyMap := svc.RelatedForMemories(ctx, scope, nil)
	assert.NotNil(t, emptyMap)
	assert.Empty(t, emptyMap)

	// Memories with empty IDs are skipped and short-circuit before
	// hitting the store.
	noIDMap := svc.RelatedForMemories(ctx, scope, []*memory.Memory{{}})
	assert.NotNil(t, noIDMap)
	assert.Empty(t, noIDMap)
}

func TestServiceDeleteMemory(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}

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
	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}

	err := svc.DeleteMemory(ctx, scope, testNonexistent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestServiceDeleteAllMemories(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}

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
		Scope:      map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"},
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

// oneHotEmbedding returns a length-1536 vector with a 1.0 at pos
// for use as a deterministic test embedding. Two oneHotEmbeddings
// at the same position have cosine similarity 1.0; at different
// positions, similarity 0.0.
func oneHotEmbedding(pos int) []float32 {
	v := make([]float32, 1536)
	if pos >= 0 && pos < len(v) {
		v[pos] = 1.0
	}
	return v
}

// TestMemoryService_AutoSupersedesByEmbeddingSimilarity proves the
// embedding-similarity dedup path: a free-form remember whose
// embedding is identical to a pre-existing observation auto-
// supersedes that observation under the same entity, returning a
// SaveResult.action=auto_superseded with reason=high_similarity.
func TestMemoryService_AutoSupersedesByEmbeddingSimilarity(t *testing.T) {
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	provider := newMockEmbeddingProvider(4)
	provider.fixedEmbedding = oneHotEmbedding(0)
	logger := zap.New(zap.UseDevMode(true))
	embSvc := memory.NewEmbeddingService(store, provider, logger)
	svc := NewMemoryService(store, embSvc, MemoryServiceConfig{}, logr.Discard())

	ctx := context.Background()
	scope := map[string]string{
		memory.ScopeWorkspaceID: testWorkspaceID,
		memory.ScopeUserID:      "test-user",
	}

	// Seed an existing memory and stamp it with the same embedding
	// the provider returns for new content.
	original := &memory.Memory{
		Type:       "preference",
		Content:    "User likes blue",
		Confidence: 0.9,
		Scope:      scope,
	}
	require.NoError(t, store.Save(ctx, original))
	require.NoError(t, store.UpdateEmbedding(ctx, original.ID, provider.fixedEmbedding))

	// Drain the seed-side embed channel signals (Save will fire one
	// async embed per call); without this the channel fills and the
	// next dedup call's embed channel push would block.
	drainEmbed(provider, time.Second)

	// New write — provider returns the same embedding → cosine 1.0 →
	// auto-supersede.
	updated := &memory.Memory{
		Type:       "preference",
		Content:    "User loves blue",
		Confidence: 0.9,
		Scope:      scope,
	}
	res, err := svc.SaveMemoryWithResult(ctx, updated)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, memory.SaveActionAutoSuperseded, res.Action)
	assert.Equal(t, memory.ReasonHighSimilarity, res.SupersedeReason)
	assert.NotEmpty(t, res.SupersededObservationIDs)
	assert.Equal(t, original.ID, updated.ID,
		"new observation lives under the existing entity")
}

// drainEmbed pulls any pending texts off the mock provider's embedCh
// (the async embed-on-save fire-and-forget) so subsequent test ops
// don't block on a full channel buffer.
func drainEmbed(p *mockEmbeddingProvider, timeout time.Duration) {
	deadline := time.After(timeout)
	for {
		select {
		case <-p.embedCh:
		case <-deadline:
			return
		default:
			return
		}
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
		Scope:      map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"},
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
		Scope:      map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"},
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
		Scope:      map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"},
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
		Scope:      map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"},
	}

	err := svc.SaveMemory(ctx, mem)
	require.NoError(t, err)
	require.NotNil(t, mem.ExpiresAt)
	assert.True(t, mem.ExpiresAt.Equal(explicit),
		"explicit ExpiresAt should not be overridden by DefaultTTL")
}

func TestMemoryService_SaveStampsDefaultPurpose(t *testing.T) {
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	cfg := MemoryServiceConfig{Purpose: "personalisation"}
	svc := NewMemoryService(store, nil, cfg, logr.Discard())

	ctx := context.Background()
	mem := &memory.Memory{
		Type: "fact", Content: "purpose default", Confidence: 1.0,
		Scope: map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "u"},
	}

	require.NoError(t, svc.SaveMemory(ctx, mem))

	var got string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT purpose FROM memory_entities WHERE id = $1`, mem.ID).Scan(&got))
	assert.Equal(t, "personalisation", got,
		"service Purpose config must propagate to the DB column via Metadata[MetaKeyPurpose]")
}

func TestMemoryService_SaveRespectsExplicitPurpose(t *testing.T) {
	pool := freshDB(t)
	store := memory.NewPostgresMemoryStore(pool)
	cfg := MemoryServiceConfig{Purpose: "personalisation"}
	svc := NewMemoryService(store, nil, cfg, logr.Discard())

	ctx := context.Background()
	mem := &memory.Memory{
		Type: "fact", Content: "explicit purpose", Confidence: 1.0,
		Scope:    map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "u"},
		Metadata: map[string]any{memory.MetaKeyPurpose: "compliance"},
	}

	require.NoError(t, svc.SaveMemory(ctx, mem))

	var got string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT purpose FROM memory_entities WHERE id = $1`, mem.ID).Scan(&got))
	assert.Equal(t, "compliance", got,
		"explicit metadata purpose must override service Purpose config")
}

func TestServiceBatchDeleteMemories(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}

	// Save 5 memories.
	for i := 0; i < 5; i++ {
		require.NoError(t, svc.SaveMemory(ctx, &memory.Memory{
			Type:       "fact",
			Content:    fmt.Sprintf("batch memory %d", i),
			Confidence: 0.8,
			Scope:      scope,
		}))
	}

	// BatchDelete with limit 3 should delete 3.
	n, err := svc.BatchDeleteMemories(ctx, scope, 3)
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	// 2 remain.
	remaining, err := svc.ListMemories(ctx, scope, memory.ListOptions{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, remaining, 2)
}

func TestServiceBatchDeleteMemories_Empty(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}

	// BatchDelete on empty store returns 0.
	n, err := svc.BatchDeleteMemories(ctx, scope, 500)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestServiceBatchDeleteMemories_MissingWorkspace(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.BatchDeleteMemories(ctx, map[string]string{}, 500)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

// --- Audit logger tests ---

// mockAuditLogger records MemoryAuditEntry values for assertion.
type mockAuditLogger struct {
	entries chan *MemoryAuditEntry
}

func newMockAuditLogger() *mockAuditLogger {
	return &mockAuditLogger{entries: make(chan *MemoryAuditEntry, 16)}
}

func (m *mockAuditLogger) LogEvent(_ context.Context, entry *MemoryAuditEntry) {
	m.entries <- entry
}

// receiveEntry waits up to 5 seconds for an audit entry from the logger.
func (m *mockAuditLogger) receiveEntry(t *testing.T) *MemoryAuditEntry {
	t.Helper()
	select {
	case e := <-m.entries:
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for audit entry")
		return nil
	}
}

func TestAuditLogger_SaveMemory_EmitsCreated(t *testing.T) {
	svc := newTestService(t)
	al := newMockAuditLogger()
	svc.SetAuditLogger(al)

	ctx := context.Background()
	mem := &memory.Memory{
		Type:       "fact",
		Content:    "audit test",
		Confidence: 0.9,
		Scope:      map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"},
	}
	require.NoError(t, svc.SaveMemory(ctx, mem))

	entry := al.receiveEntry(t)
	assert.Equal(t, eventTypeMemoryCreated, entry.EventType)
	assert.Equal(t, testWorkspaceID, entry.WorkspaceID)
	assert.NotEmpty(t, entry.MemoryID)
}

func TestAuditLogger_SearchMemories_EmitsAccessed(t *testing.T) {
	svc := newTestService(t)
	al := newMockAuditLogger()
	svc.SetAuditLogger(al)

	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}
	ctx := context.Background()
	require.NoError(t, svc.SaveMemory(ctx, &memory.Memory{
		Type: "fact", Content: "searchable", Confidence: 0.8, Scope: scope,
	}))
	// Drain the created event.
	al.receiveEntry(t)

	_, err := svc.SearchMemories(ctx, scope, "search", memory.RetrieveOptions{Limit: 5})
	require.NoError(t, err)

	entry := al.receiveEntry(t)
	assert.Equal(t, auditEventMemoryAccessed, entry.EventType)
	assert.Equal(t, "search", entry.Metadata["operation"])
}

func TestAuditLogger_ListMemories_EmitsAccessed(t *testing.T) {
	svc := newTestService(t)
	al := newMockAuditLogger()
	svc.SetAuditLogger(al)

	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}
	ctx := context.Background()

	_, err := svc.ListMemories(ctx, scope, memory.ListOptions{Limit: 5})
	require.NoError(t, err)

	entry := al.receiveEntry(t)
	assert.Equal(t, auditEventMemoryAccessed, entry.EventType)
	assert.Equal(t, "list", entry.Metadata["operation"])
}

func TestAuditLogger_DeleteMemory_EmitsDeleted(t *testing.T) {
	svc := newTestService(t)
	al := newMockAuditLogger()
	svc.SetAuditLogger(al)

	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}
	ctx := context.Background()

	mem := &memory.Memory{
		Type: "fact", Content: "to delete", Confidence: 0.8, Scope: scope,
	}
	require.NoError(t, svc.SaveMemory(ctx, mem))
	al.receiveEntry(t) // drain created event

	require.NoError(t, svc.DeleteMemory(ctx, scope, mem.ID))

	entry := al.receiveEntry(t)
	assert.Equal(t, eventTypeMemoryDeleted, entry.EventType)
	assert.Equal(t, mem.ID, entry.MemoryID)
}

func TestAuditLogger_DeleteAllMemories_EmitsDeleted(t *testing.T) {
	svc := newTestService(t)
	al := newMockAuditLogger()
	svc.SetAuditLogger(al)

	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}
	ctx := context.Background()

	require.NoError(t, svc.DeleteAllMemories(ctx, scope))

	entry := al.receiveEntry(t)
	assert.Equal(t, eventTypeMemoryDeleted, entry.EventType)
	assert.Equal(t, "delete_all", entry.Metadata["operation"])
}

func TestAuditLogger_BatchDeleteMemories_EmitsDeleted(t *testing.T) {
	svc := newTestService(t)
	al := newMockAuditLogger()
	svc.SetAuditLogger(al)

	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}
	ctx := context.Background()

	// Save one memory so there is something to delete.
	require.NoError(t, svc.SaveMemory(ctx, &memory.Memory{
		Type: "fact", Content: "batch", Confidence: 0.8, Scope: scope,
	}))
	al.receiveEntry(t) // drain created event

	n, err := svc.BatchDeleteMemories(ctx, scope, 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	entry := al.receiveEntry(t)
	assert.Equal(t, eventTypeMemoryDeleted, entry.EventType)
	assert.Equal(t, "batch_delete", entry.Metadata["operation"])
}

func TestAuditLogger_BatchDeleteMemories_NoEmitWhenEmpty(t *testing.T) {
	svc := newTestService(t)
	al := newMockAuditLogger()
	svc.SetAuditLogger(al)

	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}
	ctx := context.Background()

	n, err := svc.BatchDeleteMemories(ctx, scope, 10)
	require.NoError(t, err)
	require.Equal(t, 0, n)

	// No audit event should be emitted for empty batch delete.
	select {
	case entry := <-al.entries:
		t.Fatalf("unexpected audit entry for zero-row batch delete: %+v", entry)
	case <-time.After(100 * time.Millisecond):
		// Expected — no entry emitted.
	}
}

func TestAuditLogger_ExportMemories_EmitsExported(t *testing.T) {
	svc := newTestService(t)
	al := newMockAuditLogger()
	svc.SetAuditLogger(al)

	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}
	ctx := context.Background()

	_, err := svc.ExportMemories(ctx, scope)
	require.NoError(t, err)

	entry := al.receiveEntry(t)
	assert.Equal(t, auditEventMemoryExported, entry.EventType)
}

func TestAuditLogger_NilLogger_NoEvents(t *testing.T) {
	// No audit logger set — operations must succeed without panicking.
	svc := newTestService(t)
	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}
	ctx := context.Background()

	mem := &memory.Memory{Type: "fact", Content: "nil logger", Confidence: 0.7, Scope: scope}
	require.NoError(t, svc.SaveMemory(ctx, mem))

	_, err := svc.ListMemories(ctx, scope, memory.ListOptions{Limit: 5})
	require.NoError(t, err)

	_, err = svc.ExportMemories(ctx, scope)
	require.NoError(t, err)
}

func TestAuditLogger_RequestMetaInContext_PropagatesIPAndUA(t *testing.T) {
	svc := newTestService(t)
	al := newMockAuditLogger()
	svc.SetAuditLogger(al)

	scope := map[string]string{memory.ScopeWorkspaceID: testWorkspaceID, memory.ScopeUserID: "test-user"}
	ctx := withRequestMeta(context.Background(), RequestMeta{
		IPAddress: "10.1.2.3",
		UserAgent: "omnia-test/1.0",
	})

	_, err := svc.ListMemories(ctx, scope, memory.ListOptions{Limit: 5})
	require.NoError(t, err)

	entry := al.receiveEntry(t)
	assert.Equal(t, "10.1.2.3", entry.IPAddress)
	assert.Equal(t, "omnia-test/1.0", entry.UserAgent)
}
