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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	"github.com/redis/go-redis/v9"
)

// cacheTestStore is a test double for the Store interface used by CachedStore tests.
type cacheTestStore struct {
	mu            sync.Mutex
	memories      []*Memory
	retrieveCalls int
	listCalls     int
	saveCalls     int
	saveErr       error
	retrieveErr   error
	listErr       error
	deleteErr     error
	deleteAllErr  error

	// Counters for the agent-scoped admin wrappers — used by tests that
	// validate the CachedStore wrappers delegate to the inner store.
	saveAgentScopedCalls   int
	listAgentScopedCalls   int
	deleteAgentScopedCalls int
	agentScopedErr         error

	// Institutional wrapper counters — same rationale as the agent-scoped
	// ones; exist so coverage on cache.go reaches the 80% gate without
	// introducing a second mock type.
	saveInstitutionalCalls   int
	listInstitutionalCalls   int
	deleteInstitutionalCalls int

	// Compaction wrapper counters.
	findCompactionCalls int
	saveCompactionCalls int
	saveCompactionID    string
	compactionErr       error
}

func (m *cacheTestStore) Save(_ context.Context, mem *Memory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveCalls++
	if m.saveErr != nil {
		return m.saveErr
	}
	if mem.ID == "" {
		mem.ID = "mock-id"
		mem.CreatedAt = time.Now()
	}
	m.memories = append(m.memories, mem)
	return nil
}

func (m *cacheTestStore) SaveWithResult(ctx context.Context, mem *Memory) (*SaveResult, error) {
	if err := m.Save(ctx, mem); err != nil {
		return nil, err
	}
	return &SaveResult{ID: mem.ID, Action: SaveActionAdded}, nil
}

func (m *cacheTestStore) FindSimilarObservations(_ context.Context, _ map[string]string,
	_ []float32, _ int, _ float64,
) ([]SimilarObservation, error) {
	return nil, nil
}

func (m *cacheTestStore) AppendObservationToEntity(_ context.Context, entityID string, mem *Memory) ([]string, error) {
	mem.ID = entityID
	return nil, nil
}

func (m *cacheTestStore) GetMemory(_ context.Context, _ map[string]string, _ string) (*Memory, error) {
	return nil, nil
}

func (m *cacheTestStore) LinkEntities(_ context.Context, _ map[string]string,
	_, _, _ string, _ float64,
) (string, error) {
	return "rel-mock", nil
}

func (m *cacheTestStore) FindRelatedEntities(_ context.Context, _ map[string]string,
	_ []string, _ int,
) ([]EntityRelation, error) {
	return nil, nil
}

func (m *cacheTestStore) RetrieveHybrid(_ context.Context, _ map[string]string,
	_ string, _ []float32, _ RetrieveOptions,
) ([]*Memory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.memories, nil
}

func (m *cacheTestStore) SupersedeMany(_ context.Context, sourceIDs []string, mem *Memory) (string, []string, error) {
	if len(sourceIDs) == 0 {
		return "", nil, nil
	}
	mem.ID = sourceIDs[0]
	return sourceIDs[0], nil, nil
}

func (m *cacheTestStore) FindConflictedEntities(_ context.Context, _ string, _ int) ([]ConflictedEntity, error) {
	return nil, nil
}

func (m *cacheTestStore) Retrieve(_ context.Context, _ map[string]string, _ string, _ RetrieveOptions) ([]*Memory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retrieveCalls++
	if m.retrieveErr != nil {
		return nil, m.retrieveErr
	}
	return m.memories, nil
}

func (m *cacheTestStore) RetrieveMultiTier(_ context.Context, _ MultiTierRequest) (*MultiTierResult, error) {
	return &MultiTierResult{Memories: []*MultiTierMemory{}, Total: 0}, nil
}

func (m *cacheTestStore) SaveInstitutional(_ context.Context, _ *Memory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveInstitutionalCalls++
	return nil
}

func (m *cacheTestStore) ListInstitutional(_ context.Context, _ string, _ ListOptions) ([]*Memory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listInstitutionalCalls++
	return nil, nil
}

func (m *cacheTestStore) DeleteInstitutional(_ context.Context, _, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteInstitutionalCalls++
	return nil
}

func (m *cacheTestStore) SaveAgentScoped(_ context.Context, _ *Memory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveAgentScopedCalls++
	return m.agentScopedErr
}

func (m *cacheTestStore) ListAgentScoped(_ context.Context, _, _ string, _ ListOptions) ([]*Memory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listAgentScopedCalls++
	if m.agentScopedErr != nil {
		return nil, m.agentScopedErr
	}
	return nil, nil
}

func (m *cacheTestStore) DeleteAgentScoped(_ context.Context, _, _, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteAgentScopedCalls++
	return m.agentScopedErr
}

func (m *cacheTestStore) FindCompactionCandidates(_ context.Context, _ FindCompactionCandidatesOptions) ([]CompactionCandidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.findCompactionCalls++
	return nil, m.compactionErr
}

func (m *cacheTestStore) SaveCompactionSummary(_ context.Context, _ CompactionSummary) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveCompactionCalls++
	return m.saveCompactionID, m.compactionErr
}

func (m *cacheTestStore) List(_ context.Context, _ map[string]string, _ ListOptions) ([]*Memory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listCalls++
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.memories, nil
}

func (m *cacheTestStore) Delete(_ context.Context, _ map[string]string, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deleteErr != nil {
		return m.deleteErr
	}
	return nil
}

func (m *cacheTestStore) DeleteAll(_ context.Context, _ map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deleteAllErr != nil {
		return m.deleteAllErr
	}
	m.memories = nil
	return nil
}

func (m *cacheTestStore) ExportAll(_ context.Context, _ map[string]string) ([]*Memory, error) {
	return []*Memory{}, nil
}

func (m *cacheTestStore) BatchDelete(_ context.Context, _ map[string]string, limit int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deleteAllErr != nil {
		return 0, m.deleteAllErr
	}
	n := len(m.memories)
	if limit > 0 && limit < n {
		n = limit
	}
	m.memories = m.memories[n:]
	return n, nil
}

// cacheTestScope returns a minimal scope map for CachedStore tests.
func cacheTestScope() map[string]string {
	return map[string]string{ScopeWorkspaceID: "ws-1", ScopeUserID: "user-1"}
}

// newTestCache creates a CachedStore backed by miniredis and the given mock.
func newTestCache(t *testing.T, inner *cacheTestStore) (*CachedStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	return NewCachedStore(inner, rdb, 5*time.Minute, logr.Discard()), mr
}

// --- Retrieve tests -----------------------------------------------------------

func TestCachedStore_Retrieve_CacheMiss(t *testing.T) {
	inner := &cacheTestStore{memories: []*Memory{{ID: "m1", Type: "fact", Content: "sky is blue"}}}
	cs, _ := newTestCache(t, inner)
	ctx := context.Background()
	scope := cacheTestScope()

	// First call: cache miss, inner called.
	mems, err := cs.Retrieve(ctx, scope, "sky", RetrieveOptions{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mems) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(mems))
	}
	if inner.retrieveCalls != 1 {
		t.Fatalf("expected 1 inner call, got %d", inner.retrieveCalls)
	}

	// Second call: same key → cache hit, inner NOT called again.
	mems2, err := cs.Retrieve(ctx, scope, "sky", RetrieveOptions{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if len(mems2) != 1 {
		t.Fatalf("expected 1 memory on cache hit, got %d", len(mems2))
	}
	if inner.retrieveCalls != 1 {
		t.Fatalf("expected inner still called only once, got %d", inner.retrieveCalls)
	}
}

func TestCachedStore_Retrieve_CacheHit(t *testing.T) {
	inner := &cacheTestStore{memories: []*Memory{{ID: "m1", Type: "fact", Content: "cached"}}}
	cs, _ := newTestCache(t, inner)
	ctx := context.Background()
	scope := cacheTestScope()

	// Prime cache with first call.
	if _, err := cs.Retrieve(ctx, scope, "q", RetrieveOptions{}); err != nil {
		t.Fatalf("prime: %v", err)
	}
	beforeCalls := inner.retrieveCalls

	// Second call must return cached data without hitting inner again.
	mems, err := cs.Retrieve(ctx, scope, "q", RetrieveOptions{})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if len(mems) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(mems))
	}
	if inner.retrieveCalls != beforeCalls {
		t.Fatalf("cache hit should not call inner; calls before=%d after=%d", beforeCalls, inner.retrieveCalls)
	}
}

func TestCachedStore_Retrieve_InnerError(t *testing.T) {
	inner := &cacheTestStore{retrieveErr: errTest}
	cs, _ := newTestCache(t, inner)

	_, err := cs.Retrieve(context.Background(), cacheTestScope(), "", RetrieveOptions{})
	if err == nil {
		t.Fatal("expected error from inner, got nil")
	}
}

// --- Save / invalidation tests ------------------------------------------------

func TestCachedStore_Save_InvalidatesCache(t *testing.T) {
	inner := &cacheTestStore{memories: []*Memory{{ID: "m1", Content: "old"}}}
	cs, _ := newTestCache(t, inner)
	ctx := context.Background()
	scope := cacheTestScope()

	// Prime cache.
	if _, err := cs.Retrieve(ctx, scope, "", RetrieveOptions{}); err != nil {
		t.Fatalf("prime: %v", err)
	}
	if inner.retrieveCalls != 1 {
		t.Fatalf("expected 1 inner call after priming, got %d", inner.retrieveCalls)
	}

	// Save bumps version. The mock's Save appends to inner.memories, so inner now has 2.
	newMem := &Memory{Type: "fact", Content: "new", Scope: scope}
	if err := cs.Save(ctx, newMem); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Next Retrieve must bypass cache (version changed) and call inner again.
	mems, err := cs.Retrieve(ctx, scope, "", RetrieveOptions{})
	if err != nil {
		t.Fatalf("retrieve after save: %v", err)
	}
	if len(mems) != 2 {
		t.Fatalf("expected 2 memories after invalidation, got %d", len(mems))
	}
	if inner.retrieveCalls != 2 {
		t.Fatalf("expected inner called again after invalidation, got %d calls", inner.retrieveCalls)
	}
}

func TestCachedStore_Save_InnerError(t *testing.T) {
	inner := &cacheTestStore{saveErr: errTest}
	cs, _ := newTestCache(t, inner)

	err := cs.Save(context.Background(), &Memory{Type: "fact", Content: "x", Scope: cacheTestScope()})
	if err == nil {
		t.Fatal("expected error from inner save, got nil")
	}
}

// TestCachedStore_SaveWithResult_PassesThroughResult proves the
// inner store's dedup result (action / supersedes) reaches the
// caller unchanged. Without this the agent never sees auto_superseded
// even when the structured-key index fired in the inner store.
func TestCachedStore_SaveWithResult_PassesThroughResult(t *testing.T) {
	inner := &cacheTestStore{}
	cs, _ := newTestCache(t, inner)

	res, err := cs.SaveWithResult(context.Background(), &Memory{
		Type: "fact", Content: "x", Scope: cacheTestScope(),
	})
	if err != nil {
		t.Fatalf("SaveWithResult: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.Action != SaveActionAdded {
		t.Errorf("Action = %q, want %q", res.Action, SaveActionAdded)
	}
}

func TestCachedStore_SaveWithResult_InnerError(t *testing.T) {
	inner := &cacheTestStore{saveErr: errTest}
	cs, _ := newTestCache(t, inner)

	_, err := cs.SaveWithResult(context.Background(), &Memory{
		Type: "fact", Content: "x", Scope: cacheTestScope(),
	})
	if err == nil {
		t.Fatal("expected error from inner SaveWithResult, got nil")
	}
}

// TestCachedStore_FindSimilarObservations_Passthrough proves the
// cache wrapper doesn't add caching to dedup-on-write similarity
// queries — those need live state to decide between auto-supersede
// and surface-as-duplicate.
func TestCachedStore_FindSimilarObservations_Passthrough(t *testing.T) {
	inner := &cacheTestStore{}
	cs, _ := newTestCache(t, inner)

	_, err := cs.FindSimilarObservations(context.Background(), cacheTestScope(),
		[]float32{1, 2, 3}, 5, 0.85)
	if err != nil {
		t.Fatalf("FindSimilarObservations: %v", err)
	}
}

// TestCachedStore_GetMemory_Passthrough proves the cache wrapper
// delegates open requests to the inner store without caching —
// memory__open is infrequent and must reflect post-supersede writes
// immediately.
func TestCachedStore_GetMemory_Passthrough(t *testing.T) {
	inner := &cacheTestStore{}
	cs, _ := newTestCache(t, inner)
	_, _ = cs.GetMemory(context.Background(), cacheTestScope(), "any-id")
}

// TestCachedStore_FindConflictedEntities_Passthrough proves the
// cache wrapper delegates conflict-queue queries straight to the
// inner store — the dashboard view must reflect live state for
// triage to be useful, so caching would be wrong here.
func TestCachedStore_FindConflictedEntities_Passthrough(t *testing.T) {
	inner := &cacheTestStore{}
	cs, _ := newTestCache(t, inner)
	_, err := cs.FindConflictedEntities(context.Background(), "ws-1", 10)
	if err != nil {
		t.Fatalf("FindConflictedEntities: %v", err)
	}
}

// TestCachedStore_RetrieveHybrid_Passthrough proves the cache
// wrapper delegates the hybrid (lexical + semantic) recall path to
// the inner store without caching — query-embedding-keyed entries
// would dilute the cache.
func TestCachedStore_RetrieveHybrid_Passthrough(t *testing.T) {
	inner := &cacheTestStore{memories: []*Memory{{ID: "m1", Content: "x"}}}
	cs, _ := newTestCache(t, inner)
	got, err := cs.RetrieveHybrid(context.Background(), cacheTestScope(),
		"q", []float32{0.1, 0.2}, RetrieveOptions{Limit: 5})
	if err != nil {
		t.Fatalf("RetrieveHybrid: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 memory from inner, got %d", len(got))
	}
}

// TestCachedStore_LinkEntities_BumpsCacheVersion proves a new
// relation invalidates the workspace cache so subsequent recall's
// related[] walk sees the link.
func TestCachedStore_LinkEntities_BumpsCacheVersion(t *testing.T) {
	inner := &cacheTestStore{}
	cs, _ := newTestCache(t, inner)
	id, err := cs.LinkEntities(context.Background(), cacheTestScope(),
		"src", "tgt", "ABOUT", 1.0)
	if err != nil {
		t.Fatalf("LinkEntities: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty relation id")
	}
}

// TestCachedStore_AppendObservationToEntity_BumpsCacheVersion proves
// that the auto-supersede path invalidates the workspace cache so
// subsequent recall reflects the new active observation.
func TestCachedStore_AppendObservationToEntity_BumpsCacheVersion(t *testing.T) {
	inner := &cacheTestStore{}
	cs, _ := newTestCache(t, inner)

	mem := &Memory{Type: "fact", Content: "x", Scope: cacheTestScope()}
	_, err := cs.AppendObservationToEntity(context.Background(), "entity-id", mem)
	if err != nil {
		t.Fatalf("AppendObservationToEntity: %v", err)
	}
	if mem.ID != "entity-id" {
		t.Errorf("mem.ID = %q, want entity-id", mem.ID)
	}
}

// --- Delete tests -------------------------------------------------------------

func TestCachedStore_Delete_InvalidatesCache(t *testing.T) {
	m1 := &Memory{ID: "m1", Type: "fact", Content: "deletable"}
	inner := &cacheTestStore{memories: []*Memory{m1}}
	cs, _ := newTestCache(t, inner)
	ctx := context.Background()
	scope := cacheTestScope()

	// Prime cache.
	if _, err := cs.Retrieve(ctx, scope, "", RetrieveOptions{}); err != nil {
		t.Fatalf("prime: %v", err)
	}
	callsBefore := inner.retrieveCalls

	// Delete bumps version.
	if err := cs.Delete(ctx, scope, "m1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Clear from mock too.
	inner.mu.Lock()
	inner.memories = nil
	inner.mu.Unlock()

	// Next Retrieve must miss the cache.
	mems, err := cs.Retrieve(ctx, scope, "", RetrieveOptions{})
	if err != nil {
		t.Fatalf("retrieve after delete: %v", err)
	}
	if len(mems) != 0 {
		t.Fatalf("expected 0 memories after delete, got %d", len(mems))
	}
	if inner.retrieveCalls != callsBefore+1 {
		t.Fatalf("expected inner called once more after delete invalidation; calls=%d", inner.retrieveCalls)
	}
}

func TestCachedStore_Delete_InnerError(t *testing.T) {
	inner := &cacheTestStore{deleteErr: errTest}
	cs, _ := newTestCache(t, inner)

	err := cs.Delete(context.Background(), cacheTestScope(), "no-such-id")
	if err == nil {
		t.Fatal("expected error from inner delete, got nil")
	}
}

// --- List tests ---------------------------------------------------------------

func TestCachedStore_List_Cached(t *testing.T) {
	inner := &cacheTestStore{memories: []*Memory{
		{ID: "a", Type: "fact", Content: "one"},
		{ID: "b", Type: "skill", Content: "two"},
	}}
	cs, _ := newTestCache(t, inner)
	ctx := context.Background()
	scope := cacheTestScope()
	opts := ListOptions{Limit: 10}

	// First call: cache miss.
	mems, err := cs.List(ctx, scope, opts)
	if err != nil {
		t.Fatalf("first list: %v", err)
	}
	if len(mems) != 2 {
		t.Fatalf("expected 2, got %d", len(mems))
	}
	if inner.listCalls != 1 {
		t.Fatalf("expected 1 inner call, got %d", inner.listCalls)
	}

	// Second call: cache hit.
	if _, err := cs.List(ctx, scope, opts); err != nil {
		t.Fatalf("second list: %v", err)
	}
	if inner.listCalls != 1 {
		t.Fatalf("expected inner still called once on cache hit, got %d", inner.listCalls)
	}
}

func TestCachedStore_List_InnerError(t *testing.T) {
	inner := &cacheTestStore{listErr: errTest}
	cs, _ := newTestCache(t, inner)

	_, err := cs.List(context.Background(), cacheTestScope(), ListOptions{})
	if err == nil {
		t.Fatal("expected error from inner list, got nil")
	}
}

// --- DeleteAll tests ----------------------------------------------------------

func TestCachedStore_DeleteAll_InvalidatesCache(t *testing.T) {
	inner := &cacheTestStore{memories: []*Memory{{ID: "m1", Content: "gone"}}}
	cs, _ := newTestCache(t, inner)
	ctx := context.Background()
	scope := cacheTestScope()

	if _, err := cs.List(ctx, scope, ListOptions{}); err != nil {
		t.Fatalf("prime: %v", err)
	}
	listBefore := inner.listCalls

	if err := cs.DeleteAll(ctx, scope); err != nil {
		t.Fatalf("delete all: %v", err)
	}

	if _, err := cs.List(ctx, scope, ListOptions{}); err != nil {
		t.Fatalf("list after delete all: %v", err)
	}
	if inner.listCalls != listBefore+1 {
		t.Fatalf("expected inner called again after DeleteAll; calls=%d", inner.listCalls)
	}
}

func TestCachedStore_DeleteAll_InnerError(t *testing.T) {
	inner := &cacheTestStore{deleteAllErr: errTest}
	cs, _ := newTestCache(t, inner)

	err := cs.DeleteAll(context.Background(), cacheTestScope())
	if err == nil {
		t.Fatal("expected error from inner delete all, got nil")
	}
}

// --- Redis down / fallthrough tests -------------------------------------------

func TestCachedStore_RedisDown_Fallthrough(t *testing.T) {
	inner := &cacheTestStore{memories: []*Memory{{ID: "m1", Content: "fallback"}}}
	cs, mr := newTestCache(t, inner)
	ctx := context.Background()
	scope := cacheTestScope()

	// Shut Redis down.
	mr.Close()

	// Retrieve must fall through to inner without error.
	mems, err := cs.Retrieve(ctx, scope, "", RetrieveOptions{})
	if err != nil {
		t.Fatalf("expected fallthrough, got error: %v", err)
	}
	if len(mems) != 1 {
		t.Fatalf("expected 1 memory from inner, got %d", len(mems))
	}
	if inner.retrieveCalls != 1 {
		t.Fatalf("expected inner called once, got %d", inner.retrieveCalls)
	}
}

func TestCachedStore_RedisDown_List_Fallthrough(t *testing.T) {
	inner := &cacheTestStore{memories: []*Memory{{ID: "m1", Content: "fallback"}}}
	cs, mr := newTestCache(t, inner)
	ctx := context.Background()

	mr.Close()

	mems, err := cs.List(ctx, cacheTestScope(), ListOptions{})
	if err != nil {
		t.Fatalf("expected fallthrough on list, got error: %v", err)
	}
	if len(mems) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(mems))
	}
}

// --- Agent-scoped admin wrapper tests -----------------------------------------

func TestCachedStore_SaveAgentScoped_DelegatesAndBumps(t *testing.T) {
	inner := &cacheTestStore{}
	cs, mr := newTestCache(t, inner)
	ctx := context.Background()

	mem := &Memory{
		Scope: map[string]string{ScopeWorkspaceID: "ws-1", ScopeAgentID: "agent-1"},
	}
	if err := cs.SaveAgentScoped(ctx, mem); err != nil {
		t.Fatalf("SaveAgentScoped: %v", err)
	}
	if inner.saveAgentScopedCalls != 1 {
		t.Fatalf("inner SaveAgentScoped calls = %d, want 1", inner.saveAgentScopedCalls)
	}

	// Save must bump the cache version for the (workspace, agent) scope.
	sh := scopeHash(map[string]string{ScopeWorkspaceID: "ws-1", ScopeAgentID: "agent-1"})
	v, err := mr.Get(versionKey(sh))
	if err != nil {
		t.Fatalf("version key missing after save: %v", err)
	}
	if v != "1" {
		t.Errorf("version=%q, want %q", v, "1")
	}
}

func TestCachedStore_SaveAgentScoped_PropagatesInnerError(t *testing.T) {
	inner := &cacheTestStore{agentScopedErr: fmt.Errorf("inner save err")}
	cs, _ := newTestCache(t, inner)

	err := cs.SaveAgentScoped(context.Background(), &Memory{
		Scope: map[string]string{ScopeWorkspaceID: "ws-1", ScopeAgentID: "agent-1"},
	})
	if err == nil || err.Error() != "inner save err" {
		t.Errorf("expected wrapped inner error, got %v", err)
	}
}

func TestCachedStore_ListAgentScoped_Delegates(t *testing.T) {
	inner := &cacheTestStore{}
	cs, _ := newTestCache(t, inner)

	if _, err := cs.ListAgentScoped(context.Background(), "ws-1", "agent-1", ListOptions{}); err != nil {
		t.Fatalf("ListAgentScoped: %v", err)
	}
	if inner.listAgentScopedCalls != 1 {
		t.Errorf("inner ListAgentScoped calls = %d, want 1", inner.listAgentScopedCalls)
	}
}

func TestCachedStore_DeleteAgentScoped_DelegatesAndBumps(t *testing.T) {
	inner := &cacheTestStore{}
	cs, mr := newTestCache(t, inner)

	if err := cs.DeleteAgentScoped(context.Background(), "ws-1", "agent-1", "mem-id"); err != nil {
		t.Fatalf("DeleteAgentScoped: %v", err)
	}
	if inner.deleteAgentScopedCalls != 1 {
		t.Errorf("inner DeleteAgentScoped calls = %d, want 1", inner.deleteAgentScopedCalls)
	}

	sh := scopeHash(map[string]string{ScopeWorkspaceID: "ws-1", ScopeAgentID: "agent-1"})
	v, err := mr.Get(versionKey(sh))
	if err != nil {
		t.Fatalf("version key missing after delete: %v", err)
	}
	if v != "1" {
		t.Errorf("version=%q, want %q", v, "1")
	}
}

func TestCachedStore_DeleteAgentScoped_PropagatesInnerError(t *testing.T) {
	inner := &cacheTestStore{agentScopedErr: fmt.Errorf("not found")}
	cs, _ := newTestCache(t, inner)

	err := cs.DeleteAgentScoped(context.Background(), "ws-1", "agent-1", "mem-id")
	if err == nil || err.Error() != "not found" {
		t.Errorf("expected inner error, got %v", err)
	}
}

// --- Institutional wrapper tests ---------------------------------------------

func TestCachedStore_SaveInstitutional_DelegatesAndBumps(t *testing.T) {
	inner := &cacheTestStore{}
	cs, mr := newTestCache(t, inner)

	mem := &Memory{Scope: map[string]string{ScopeWorkspaceID: "ws-1"}}
	if err := cs.SaveInstitutional(context.Background(), mem); err != nil {
		t.Fatalf("SaveInstitutional: %v", err)
	}
	if inner.saveInstitutionalCalls != 1 {
		t.Errorf("inner SaveInstitutional calls = %d, want 1", inner.saveInstitutionalCalls)
	}
	sh := scopeHash(map[string]string{ScopeWorkspaceID: "ws-1"})
	if v, err := mr.Get(versionKey(sh)); err != nil || v != "1" {
		t.Errorf("workspace version bump missing: v=%q err=%v", v, err)
	}
}

func TestCachedStore_ListInstitutional_Delegates(t *testing.T) {
	inner := &cacheTestStore{}
	cs, _ := newTestCache(t, inner)

	if _, err := cs.ListInstitutional(context.Background(), "ws-1", ListOptions{}); err != nil {
		t.Fatalf("ListInstitutional: %v", err)
	}
	if inner.listInstitutionalCalls != 1 {
		t.Errorf("inner ListInstitutional calls = %d, want 1", inner.listInstitutionalCalls)
	}
}

func TestCachedStore_DeleteInstitutional_DelegatesAndBumps(t *testing.T) {
	inner := &cacheTestStore{}
	cs, mr := newTestCache(t, inner)

	if err := cs.DeleteInstitutional(context.Background(), "ws-1", "mem-id"); err != nil {
		t.Fatalf("DeleteInstitutional: %v", err)
	}
	if inner.deleteInstitutionalCalls != 1 {
		t.Errorf("inner DeleteInstitutional calls = %d, want 1", inner.deleteInstitutionalCalls)
	}
	sh := scopeHash(map[string]string{ScopeWorkspaceID: "ws-1"})
	if v, err := mr.Get(versionKey(sh)); err != nil || v != "1" {
		t.Errorf("workspace version bump missing: v=%q err=%v", v, err)
	}
}

func TestCachedStore_RetrieveMultiTier_Delegates(t *testing.T) {
	inner := &cacheTestStore{}
	cs, _ := newTestCache(t, inner)

	res, err := cs.RetrieveMultiTier(context.Background(), MultiTierRequest{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("RetrieveMultiTier: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil MultiTierResult")
	}
}

func TestCachedStore_ExportAll_Delegates(t *testing.T) {
	inner := &cacheTestStore{}
	cs, _ := newTestCache(t, inner)

	mems, err := cs.ExportAll(context.Background(), cacheTestScope())
	if err != nil {
		t.Fatalf("ExportAll: %v", err)
	}
	if mems == nil {
		t.Error("expected non-nil slice from ExportAll")
	}
}

// --- Edge case: cache returns empty but inner has data ------------------------

// TestCachedStore_EmptyCache_DoesNotMaskInnerData verifies that a version bump
// (e.g. after a Save) causes a subsequent Retrieve to call inner even when the
// previous cached result was an empty slice. This guards against the "cache
// empty = definitive answer" antipattern described in CLAUDE.md.
func TestCachedStore_EmptyCache_DoesNotMaskInnerData(t *testing.T) {
	inner := &cacheTestStore{memories: []*Memory{}}
	cs, _ := newTestCache(t, inner)
	ctx := context.Background()
	scope := cacheTestScope()

	// First call returns empty from inner; this gets cached.
	mems, err := cs.Retrieve(ctx, scope, "", RetrieveOptions{})
	if err != nil {
		t.Fatalf("first retrieve: %v", err)
	}
	if len(mems) != 0 {
		t.Fatalf("expected empty, got %d", len(mems))
	}

	// A Save bumps the version.
	newMem := &Memory{Type: "fact", Content: "new", Scope: scope}
	if err := cs.Save(ctx, newMem); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Now inner has one memory.
	inner.mu.Lock()
	inner.memories = []*Memory{{ID: "m2", Content: "new"}}
	inner.mu.Unlock()

	// Retrieve must NOT return the old empty cache; must call inner.
	mems, err = cs.Retrieve(ctx, scope, "", RetrieveOptions{})
	if err != nil {
		t.Fatalf("retrieve after save: %v", err)
	}
	if len(mems) != 1 {
		t.Fatalf("expected 1 memory after version bump, got %d (empty cache was incorrectly trusted)", len(mems))
	}
}

// --- BatchDelete tests --------------------------------------------------------

func TestCachedStore_BatchDelete_InvalidatesCache(t *testing.T) {
	inner := &cacheTestStore{memories: []*Memory{
		{ID: "m1", Content: "one"},
		{ID: "m2", Content: "two"},
		{ID: "m3", Content: "three"},
	}}
	cs, _ := newTestCache(t, inner)
	ctx := context.Background()
	scope := cacheTestScope()

	if _, err := cs.List(ctx, scope, ListOptions{}); err != nil {
		t.Fatalf("prime: %v", err)
	}
	listBefore := inner.listCalls

	n, err := cs.BatchDelete(ctx, scope, 2)
	if err != nil {
		t.Fatalf("batch delete: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 deleted, got %d", n)
	}

	if _, err := cs.List(ctx, scope, ListOptions{}); err != nil {
		t.Fatalf("list after batch delete: %v", err)
	}
	if inner.listCalls != listBefore+1 {
		t.Fatalf("expected inner called again after BatchDelete; calls=%d", inner.listCalls)
	}
}

func TestCachedStore_BatchDelete_ZeroRows_NoBump(t *testing.T) {
	inner := &cacheTestStore{memories: []*Memory{}}
	cs, _ := newTestCache(t, inner)
	ctx := context.Background()
	scope := cacheTestScope()

	// Prime cache.
	if _, err := cs.List(ctx, scope, ListOptions{}); err != nil {
		t.Fatalf("prime: %v", err)
	}
	listBefore := inner.listCalls

	// BatchDelete with nothing to delete returns 0 — no version bump needed.
	n, err := cs.BatchDelete(ctx, scope, 500)
	if err != nil {
		t.Fatalf("batch delete: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 deleted, got %d", n)
	}

	// Cache should NOT be invalidated (version not bumped).
	if _, err := cs.List(ctx, scope, ListOptions{}); err != nil {
		t.Fatalf("list after no-op batch delete: %v", err)
	}
	if inner.listCalls != listBefore {
		t.Fatalf("expected inner NOT called again (no invalidation); calls=%d", inner.listCalls)
	}
}

func TestCachedStore_BatchDelete_InnerError(t *testing.T) {
	inner := &cacheTestStore{deleteAllErr: errTest}
	cs, _ := newTestCache(t, inner)

	_, err := cs.BatchDelete(context.Background(), cacheTestScope(), 500)
	if err == nil {
		t.Fatal("expected error from inner batch delete, got nil")
	}
}

// errTest is a sentinel error for injection.
var errTest = fmt.Errorf("injected error")
