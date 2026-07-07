/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package runtime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/go-logr/logr"
)

// testDenyCEL is a sample deny expression reused across retrieval tests.
const testDenyCEL = `metadata.url.contains("restricted")`

// fakeStore is a minimal pkmemory.Store implementation for retriever tests.
type fakeStore struct {
	listMemories     []*pkmemory.Memory
	listErr          error
	listCalls        atomic.Int32
	retrieveMemories []*pkmemory.Memory
	retrieveErr      error
	retrieveCalls    atomic.Int32
	lastQuery        string
	lastFetchLimit   int
}

func (f *fakeStore) Save(_ context.Context, _ *pkmemory.Memory) error { return nil }

func (f *fakeStore) Retrieve(
	_ context.Context, _ map[string]string, query string, opts pkmemory.RetrieveOptions,
) ([]*pkmemory.Memory, error) {
	f.retrieveCalls.Add(1)
	f.lastQuery = query
	f.lastFetchLimit = opts.Limit
	if f.retrieveErr != nil {
		return nil, f.retrieveErr
	}
	return f.retrieveMemories, nil
}

func (f *fakeStore) List(
	_ context.Context, _ map[string]string, _ pkmemory.ListOptions,
) ([]*pkmemory.Memory, error) {
	f.listCalls.Add(1)
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listMemories, nil
}

func (f *fakeStore) Delete(_ context.Context, _ map[string]string, _ string) error { return nil }
func (f *fakeStore) DeleteAll(_ context.Context, _ map[string]string) error        { return nil }

// fakeSemanticStore implements both pkmemory.Store and SemanticRetriever so
// tests can verify the semantic branch is taken (or skipped) as configured.
type fakeSemanticStore struct {
	fakeStore
	semanticMemories  []*pkmemory.Memory
	semanticErr       error
	semanticCalls     atomic.Int32
	lastSemanticQuery string
	lastDenyCEL       string
	lastWorkspaceID   string
	lastLimit         int
}

func (f *fakeSemanticStore) RetrieveSemantic(
	_ context.Context, workspaceID, query, denyCEL string, limit int,
) ([]*pkmemory.Memory, error) {
	f.semanticCalls.Add(1)
	f.lastWorkspaceID = workspaceID
	f.lastSemanticQuery = query
	f.lastDenyCEL = denyCEL
	f.lastLimit = limit
	if f.semanticErr != nil {
		return nil, f.semanticErr
	}
	return f.semanticMemories, nil
}

func mem(id, category, content string) *pkmemory.Memory {
	return &pkmemory.Memory{
		ID:      id,
		Content: content,
		Metadata: map[string]any{
			metaKeyConsentCategory: category,
		},
	}
}

func userMsg(content string) types.Message { return types.Message{Role: "user", Content: content} }

func defaultScope() map[string]string {
	return map[string]string{"workspace_id": "ws", "virtual_user_id": "u"}
}

// mustRetriever builds a CompositeRetriever and fails the test on a constructor
// error (e.g. an invalid denyCEL). The logger arg is accepted and ignored so
// existing call sites that pass logr.Discard() compile unchanged.
func mustRetriever(t *testing.T, store pkmemory.Store, cfg RetrievalConfig, _ logr.Logger) *CompositeRetriever {
	t.Helper()
	r, err := NewCompositeRetriever(store, cfg, logr.Discard())
	if err != nil {
		t.Fatalf("NewCompositeRetriever: %v", err)
	}
	return r
}

// memWithMeta builds a memory whose metadata carries a url, for denyCEL tests.
func memWithMeta(id, url string) *pkmemory.Memory {
	return &pkmemory.Memory{ID: id, Content: id, Metadata: map[string]any{"url": url}}
}

func TestRetrieveKeyword_DropsDeniedItems(t *testing.T) {
	store := &fakeStore{retrieveMemories: []*pkmemory.Memory{
		memWithMeta("a", "https://ok/doc"),
		memWithMeta("b", "https://restricted/doc"),
		memWithMeta("c", "https://ok/other"),
	}}
	r, err := NewCompositeRetriever(store, RetrievalConfig{
		Strategy: StrategyKeyword,
		DenyCEL:  testDenyCEL,
	}, logr.Discard())
	if err != nil {
		t.Fatalf("ctor: %v", err)
	}
	got, err := r.retrieveEpisodic(context.Background(), defaultScope(), "anything")
	if err != nil {
		t.Fatalf("retrieveEpisodic: %v", err)
	}
	for _, m := range got {
		if m.ID == "b" {
			t.Fatal("denied item 'b' was returned")
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 allowed items, got %d", len(got))
	}
}

func TestRetrieveKeyword_OverFetchesWhenDenyActive(t *testing.T) {
	store := &fakeStore{retrieveMemories: []*pkmemory.Memory{memWithMeta("a", "https://ok/doc")}}
	r, err := NewCompositeRetriever(store, RetrievalConfig{
		Strategy: StrategyKeyword,
		DenyCEL:  testDenyCEL,
		Limit:    5,
	}, logr.Discard())
	if err != nil {
		t.Fatalf("ctor: %v", err)
	}
	if _, err := r.retrieveEpisodic(context.Background(), defaultScope(), "q"); err != nil {
		t.Fatalf("retrieveEpisodic: %v", err)
	}
	if store.lastFetchLimit != 15 { // episodicLimit(5) * 3
		t.Fatalf("expected over-fetch limit 15, got %d", store.lastFetchLimit)
	}
}

// Silent-fallback trap: strategy=semantic but the store has no semantic
// capability → keyword fallback must still enforce denyCEL.
func TestSemanticFallback_StillEnforcesDeny(t *testing.T) {
	store := &fakeStore{retrieveMemories: []*pkmemory.Memory{
		memWithMeta("a", "https://ok/doc"),
		memWithMeta("b", "https://restricted/doc"),
	}}
	r, err := NewCompositeRetriever(store, RetrievalConfig{
		Strategy: StrategySemantic, // but fakeStore is not a SemanticRetriever
		DenyCEL:  testDenyCEL,
	}, logr.Discard())
	if err != nil {
		t.Fatalf("ctor: %v", err)
	}
	got, err := r.retrieveEpisodic(context.Background(), defaultScope(), "q")
	if err != nil {
		t.Fatalf("retrieveEpisodic: %v", err)
	}
	for _, m := range got {
		if m.ID == "b" {
			t.Fatal("semantic-fallback served a denied item")
		}
	}
}

func TestRetrieveComposite_FusesAndDedups(t *testing.T) {
	store := &fakeSemanticStore{
		fakeStore: fakeStore{retrieveMemories: []*pkmemory.Memory{
			memWithMeta("k1", "https://ok/k1"),
			memWithMeta("shared", "https://ok/shared"),
		}},
		semanticMemories: []*pkmemory.Memory{
			memWithMeta("shared", "https://ok/shared"),
			memWithMeta("s1", "https://ok/s1"),
		},
	}
	r, err := NewCompositeRetriever(store, RetrievalConfig{Strategy: StrategyComposite, Limit: 10}, logr.Discard())
	if err != nil {
		t.Fatalf("ctor: %v", err)
	}
	got, err := r.retrieveEpisodic(context.Background(), defaultScope(), "q")
	if err != nil {
		t.Fatalf("retrieveEpisodic: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 fused unique memories, got %d", len(got))
	}
	// "shared" appears rank-0 in both lists → highest fused score → first.
	if got[0].ID != "shared" {
		t.Fatalf("expected 'shared' ranked first, got %q", got[0].ID)
	}
	if store.semanticCalls.Load() != 1 {
		t.Fatalf("expected semantic leg called once, got %d", store.semanticCalls.Load())
	}
}

func TestRetrieveComposite_DegradesToKeywordWhenNoSemantic(t *testing.T) {
	// fakeStore has no semantic capability → composite must run keyword only.
	store := &fakeStore{retrieveMemories: []*pkmemory.Memory{memWithMeta("k1", "https://ok/k1")}}
	r, err := NewCompositeRetriever(store, RetrievalConfig{Strategy: StrategyComposite}, logr.Discard())
	if err != nil {
		t.Fatalf("ctor: %v", err)
	}
	got, err := r.retrieveEpisodic(context.Background(), defaultScope(), "q")
	if err != nil {
		t.Fatalf("retrieveEpisodic: %v", err)
	}
	if len(got) != 1 || got[0].ID != "k1" {
		t.Fatalf("expected keyword-only degrade to [k1], got %v", got)
	}
}

func TestNewCompositeRetriever_InvalidDenyCELErrors(t *testing.T) {
	_, err := NewCompositeRetriever(&fakeStore{}, RetrievalConfig{DenyCEL: "metadata.url.bad("}, logr.Discard())
	if err == nil {
		t.Fatal("expected error for invalid denyCEL, got nil")
	}
}

func TestNewCompositeRetriever_ValidDenyCELSucceeds(t *testing.T) {
	r, err := NewCompositeRetriever(&fakeStore{}, RetrievalConfig{DenyCEL: testDenyCEL}, logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.denyActive {
		t.Fatal("expected denyActive true when denyCEL set")
	}
}

func TestCompositeRetriever_NoUserIDReturnsNil(t *testing.T) {
	r := mustRetriever(t, &fakeStore{}, RetrievalConfig{}, logr.Discard())
	got, err := r.RetrieveContext(context.Background(), map[string]string{"workspace_id": "ws"}, []types.Message{userMsg("hi")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil result for empty user_id, got %d memories", len(got))
	}
}

func TestCompositeRetriever_ProfileOnlyWhenNoQuery(t *testing.T) {
	store := &fakeStore{
		listMemories: []*pkmemory.Memory{
			mem("1", "memory:identity", "name: Sarah"),
			mem("2", "memory:preferences", "aisle seat"),
			mem("3", "memory:health", "peanut allergy"),
			mem("4", "memory:context", "planning Boston trip"),
		},
	}
	r := mustRetriever(t, store, RetrievalConfig{}, logr.Discard())

	// No user message → no episodic query → profile only.
	got, err := r.RetrieveContext(context.Background(), defaultScope(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 profile memories, got %d", len(got))
	}
	if store.retrieveCalls.Load() != 0 {
		t.Errorf("Retrieve should not be called without a query")
	}
}

func TestCompositeRetriever_CompositeMergesProfileAndEpisodic(t *testing.T) {
	profile := []*pkmemory.Memory{
		mem("p1", "memory:identity", "name: Sarah"),
		mem("p2", "memory:preferences", "aisle seat"),
	}
	episodic := []*pkmemory.Memory{
		mem("e1", "memory:history", "stayed at Kimpton Gray"),
		mem("e2", "memory:context", "October Chicago trip"),
	}
	store := &fakeStore{listMemories: profile, retrieveMemories: episodic}
	r := mustRetriever(t, store, RetrievalConfig{}, logr.Discard())

	got, err := r.RetrieveContext(context.Background(), defaultScope(), []types.Message{userMsg("plan philly")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 memories (2 profile + 2 episodic), got %d", len(got))
	}
	if store.lastQuery != "plan OR philly" {
		t.Errorf("expected OR-rewritten query %q, got %q", "plan OR philly", store.lastQuery)
	}
}

func TestCompositeRetriever_DropsEpisodicProfileCategoryDuplicates(t *testing.T) {
	profile := []*pkmemory.Memory{
		mem("p1", "memory:identity", "name: Sarah"),
	}
	// Similarity search returns a profile-category memory; should be dropped.
	episodic := []*pkmemory.Memory{
		mem("p1", "memory:identity", "name: Sarah"), // same id → dedup
		mem("e1", "memory:health", "vegetarian"),    // profile-category → filtered
		mem("e2", "memory:history", "October trip"), // keep
	}
	store := &fakeStore{listMemories: profile, retrieveMemories: episodic}
	r := mustRetriever(t, store, RetrievalConfig{}, logr.Discard())

	got, err := r.RetrieveContext(context.Background(), defaultScope(), []types.Message{userMsg("hi")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 memories (profile + 1 non-dup non-profile-category episodic), got %d", len(got))
	}
	gotIDs := []string{got[0].ID, got[1].ID}
	if gotIDs[0] != "p1" || gotIDs[1] != "e2" {
		t.Errorf("unexpected merge order: %v", gotIDs)
	}
}

func TestCompositeRetriever_EpisodicErrorFallsBackToProfile(t *testing.T) {
	profile := []*pkmemory.Memory{mem("p1", "memory:identity", "Sarah")}
	store := &fakeStore{listMemories: profile, retrieveErr: errors.New("upstream down")}
	r := mustRetriever(t, store, RetrievalConfig{}, logr.Discard())

	got, err := r.RetrieveContext(context.Background(), defaultScope(), []types.Message{userMsg("hi")})
	if err != nil {
		t.Fatalf("RetrieveContext should not propagate episodic errors, got %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 profile memory, got %d", len(got))
	}
}

func TestCompositeRetriever_ListErrorReturnsEmptyProfile(t *testing.T) {
	store := &fakeStore{
		listErr:          errors.New("memory-api down"),
		retrieveMemories: []*pkmemory.Memory{mem("e1", "memory:history", "thing")},
	}
	r := mustRetriever(t, store, RetrievalConfig{}, logr.Discard())

	got, err := r.RetrieveContext(context.Background(), defaultScope(), []types.Message{userMsg("hi")})
	if err != nil {
		t.Fatalf("RetrieveContext should not propagate list errors, got %v", err)
	}
	if len(got) != 1 || got[0].ID != "e1" {
		t.Fatalf("expected episodic-only fallback, got %+v", got)
	}
}

func TestCompositeRetriever_ProfileCachedWithinTTL(t *testing.T) {
	store := &fakeStore{listMemories: []*pkmemory.Memory{mem("p1", "memory:identity", "Sarah")}}
	r := mustRetriever(t, store, RetrievalConfig{}, logr.Discard())

	for i := 0; i < 5; i++ {
		_, err := r.RetrieveContext(context.Background(), defaultScope(), nil)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if calls := store.listCalls.Load(); calls != 1 {
		t.Errorf("expected 1 List call (cached), got %d", calls)
	}
}

func TestCompositeRetriever_ProfileCacheExpires(t *testing.T) {
	store := &fakeStore{listMemories: []*pkmemory.Memory{mem("p1", "memory:identity", "Sarah")}}
	r := mustRetriever(t, store, RetrievalConfig{}, logr.Discard())

	if _, err := r.RetrieveContext(context.Background(), defaultScope(), nil); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Force-expire cache.
	r.mu.Lock()
	for k, e := range r.cache {
		e.expires = time.Now().Add(-time.Second)
		r.cache[k] = e
	}
	r.mu.Unlock()

	if _, err := r.RetrieveContext(context.Background(), defaultScope(), nil); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if calls := store.listCalls.Load(); calls != 2 {
		t.Errorf("expected 2 List calls after TTL, got %d", calls)
	}
}

func TestCompositeRetriever_ProfileCacheKeyedPerUser(t *testing.T) {
	store := &fakeStore{listMemories: []*pkmemory.Memory{mem("p1", "memory:identity", "Sarah")}}
	r := mustRetriever(t, store, RetrievalConfig{}, logr.Discard())

	if _, err := r.RetrieveContext(context.Background(), map[string]string{"workspace_id": "ws", "virtual_user_id": "alice"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := r.RetrieveContext(context.Background(), map[string]string{"workspace_id": "ws", "virtual_user_id": "bob"}, nil); err != nil {
		t.Fatal(err)
	}
	if calls := store.listCalls.Load(); calls != 2 {
		t.Errorf("expected 2 List calls (one per user), got %d", calls)
	}
}

func TestCompositeRetriever_NonProfileMemoriesFromListAreIgnored(t *testing.T) {
	// List returns mixed; only profile categories survive into the
	// always-include slice. Episodic categories are NOT pulled this way
	// — they come from Retrieve.
	store := &fakeStore{
		listMemories: []*pkmemory.Memory{
			mem("p1", "memory:identity", "Sarah"),
			mem("c1", "memory:context", "trip"),
			mem("h1", "memory:history", "old trip"),
			mem("noCat", "", "untagged"),
		},
	}
	r := mustRetriever(t, store, RetrievalConfig{}, logr.Discard())

	got, err := r.RetrieveContext(context.Background(), defaultScope(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "p1" {
		t.Errorf("expected only profile-category memory, got %v", got)
	}
}

func TestCompositeRetriever_SemanticStrategyCallsSemanticRetriever(t *testing.T) {
	semanticResult := []*pkmemory.Memory{mem("s1", "memory:context", "semantic hit")}
	store := &fakeSemanticStore{
		fakeStore:        fakeStore{listMemories: []*pkmemory.Memory{mem("p1", "memory:identity", "Sarah")}},
		semanticMemories: semanticResult,
	}
	cfg := RetrievalConfig{
		Strategy:    StrategySemantic,
		DenyCEL:     testDenyCEL,
		WorkspaceID: "ws-configured",
	}
	r := mustRetriever(t, store, cfg, logr.Discard())

	_, err := r.RetrieveContext(context.Background(), defaultScope(), []types.Message{userMsg("plan a trip")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.semanticCalls.Load() != 1 {
		t.Errorf("expected 1 semantic call, got %d", store.semanticCalls.Load())
	}
	if store.retrieveCalls.Load() != 0 {
		t.Errorf("expected 0 FTS calls, got %d", store.retrieveCalls.Load())
	}
	if store.lastDenyCEL != cfg.DenyCEL {
		t.Errorf("denyCEL: got %q, want %q", store.lastDenyCEL, cfg.DenyCEL)
	}
	if store.lastWorkspaceID != cfg.WorkspaceID {
		t.Errorf("workspaceID: got %q, want %q", store.lastWorkspaceID, cfg.WorkspaceID)
	}
	if store.lastSemanticQuery != "plan a trip" {
		t.Errorf("query: got %q, want %q", store.lastSemanticQuery, "plan a trip")
	}
}

func TestCompositeRetriever_KeywordStrategyUsesFTS(t *testing.T) {
	episodic := []*pkmemory.Memory{mem("e1", "memory:history", "fts hit")}
	store := &fakeSemanticStore{
		fakeStore: fakeStore{
			listMemories:     []*pkmemory.Memory{mem("p1", "memory:identity", "Sarah")},
			retrieveMemories: episodic,
		},
	}
	// strategy="" (keyword default) — must NOT call semantic even though store supports it.
	r := mustRetriever(t, store, RetrievalConfig{Strategy: StrategyKeyword}, logr.Discard())

	_, err := r.RetrieveContext(context.Background(), defaultScope(), []types.Message{userMsg("chicago trip")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.semanticCalls.Load() != 0 {
		t.Errorf("expected 0 semantic calls for keyword strategy, got %d", store.semanticCalls.Load())
	}
	if store.retrieveCalls.Load() != 1 {
		t.Errorf("expected 1 FTS call, got %d", store.retrieveCalls.Load())
	}
}

func TestCompositeRetriever_SemanticStrategyFallsBackWhenStoreUnsupported(t *testing.T) {
	// Plain fakeStore does NOT implement SemanticRetriever → type-assert is false → FTS.
	episodic := []*pkmemory.Memory{mem("e1", "memory:history", "fts hit")}
	store := &fakeStore{
		listMemories:     []*pkmemory.Memory{mem("p1", "memory:identity", "Sarah")},
		retrieveMemories: episodic,
	}
	cfg := RetrievalConfig{
		Strategy:    StrategySemantic,
		DenyCEL:     testDenyCEL,
		WorkspaceID: "ws1",
	}
	r := mustRetriever(t, store, cfg, logr.Discard())

	// Confirm the type-assert failed (semantic is nil).
	if r.semantic != nil {
		t.Fatal("expected semantic to be nil for a store that doesn't implement SemanticRetriever")
	}

	_, err := r.RetrieveContext(context.Background(), defaultScope(), []types.Message{userMsg("fallback test")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.retrieveCalls.Load() != 1 {
		t.Errorf("expected FTS fallback (1 Retrieve call), got %d", store.retrieveCalls.Load())
	}
}

func TestCompositeRetriever_LimitAppliedToSemanticRetrieval(t *testing.T) {
	store := &fakeSemanticStore{
		fakeStore:        fakeStore{listMemories: []*pkmemory.Memory{mem("p1", "memory:identity", "Sarah")}},
		semanticMemories: []*pkmemory.Memory{mem("s1", "memory:context", "hit")},
	}
	cfg := RetrievalConfig{
		Strategy:    StrategySemantic,
		WorkspaceID: "ws1",
		Limit:       5,
	}
	r := mustRetriever(t, store, cfg, logr.Discard())

	if r.episodicLimit != 5 {
		t.Fatalf("episodicLimit: got %d, want 5", r.episodicLimit)
	}

	_, err := r.RetrieveContext(context.Background(), defaultScope(), []types.Message{userMsg("plan a trip")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.lastLimit != 5 {
		t.Errorf("RetrieveSemantic called with limit %d, want 5", store.lastLimit)
	}
}

func TestCompositeRetriever_ZeroLimitFallsBackToDefault(t *testing.T) {
	store := &fakeSemanticStore{
		fakeStore:        fakeStore{listMemories: []*pkmemory.Memory{mem("p1", "memory:identity", "Sarah")}},
		semanticMemories: []*pkmemory.Memory{mem("s1", "memory:context", "hit")},
	}
	// Limit: 0 → defaultEpisodicLimit (10)
	cfg := RetrievalConfig{
		Strategy:    StrategySemantic,
		WorkspaceID: "ws1",
	}
	r := mustRetriever(t, store, cfg, logr.Discard())

	if r.episodicLimit != defaultEpisodicLimit {
		t.Fatalf("episodicLimit: got %d, want %d (defaultEpisodicLimit)", r.episodicLimit, defaultEpisodicLimit)
	}

	_, err := r.RetrieveContext(context.Background(), defaultScope(), []types.Message{userMsg("plan a trip")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.lastLimit != defaultEpisodicLimit {
		t.Errorf("RetrieveSemantic called with limit %d, want %d (defaultEpisodicLimit)", store.lastLimit, defaultEpisodicLimit)
	}
}

// TestBuildConversationOptions_WiresMemoryRetriever exercises the memory-store
// branch of buildConversationOptions — where the CRD-derived strategy/denyCEL/
// limit are threaded into the CompositeRetriever. A Server with no providerType
// takes the nil-provider (auto-detect) path, so it reaches the memory wiring
// without needing a real provider.
func TestBuildConversationOptions_WiresMemoryRetriever(t *testing.T) {
	store := &fakeSemanticStore{fakeStore: fakeStore{}}
	srv := NewServer(
		WithLogger(logr.Discard()),
		WithMemoryStore(store),
		WithWorkspaceUID("ws-1"),
		WithMemoryRetrieval(StrategySemantic, testDenyCEL, 5),
	)

	opts, err := srv.buildConversationOptions(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("buildConversationOptions error: %v", err)
	}
	if len(opts) == 0 {
		t.Fatal("expected conversation options (incl. memory wiring), got none")
	}
}

func TestToFTSOrQuery(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty passes through", in: "", want: ""},
		{name: "single word unchanged", in: "Chicago", want: "Chicago"},
		{name: "multi word joined with OR", in: "remind me about Chicago", want: "remind OR me OR about OR Chicago"},
		{name: "extra whitespace collapsed", in: "  hello   world  ", want: "hello OR world"},
		{name: "single word with surrounding whitespace passes through unchanged", in: "  hello  ", want: "  hello  "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := toFTSOrQuery(tc.in); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestLastUserContent(t *testing.T) {
	cases := []struct {
		name     string
		messages []types.Message
		want     string
	}{
		{name: "empty", messages: nil, want: ""},
		{name: "no user", messages: []types.Message{{Role: "system", Content: "x"}}, want: ""},
		{name: "single user", messages: []types.Message{userMsg("hello")}, want: "hello"},
		{name: "trims whitespace", messages: []types.Message{userMsg("  hi  ")}, want: "hi"},
		{
			name: "picks last user",
			messages: []types.Message{
				userMsg("first"),
				{Role: "assistant", Content: "ok"},
				userMsg("second"),
			},
			want: "second",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := lastUserContent(tc.messages); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}
