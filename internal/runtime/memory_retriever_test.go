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

// fakeStore is a minimal pkmemory.Store implementation for retriever tests.
type fakeStore struct {
	listMemories     []*pkmemory.Memory
	listErr          error
	listCalls        atomic.Int32
	retrieveMemories []*pkmemory.Memory
	retrieveErr      error
	retrieveCalls    atomic.Int32
	lastQuery        string
}

func (f *fakeStore) Save(_ context.Context, _ *pkmemory.Memory) error { return nil }

func (f *fakeStore) Retrieve(
	_ context.Context, _ map[string]string, query string, _ pkmemory.RetrieveOptions,
) ([]*pkmemory.Memory, error) {
	f.retrieveCalls.Add(1)
	f.lastQuery = query
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
	return map[string]string{"workspace_id": "ws", "user_id": "u"}
}

func TestCompositeRetriever_NoUserIDReturnsNil(t *testing.T) {
	r := NewCompositeRetriever(&fakeStore{}, logr.Discard())
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
	r := NewCompositeRetriever(store, logr.Discard())

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
	r := NewCompositeRetriever(store, logr.Discard())

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
	r := NewCompositeRetriever(store, logr.Discard())

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
	r := NewCompositeRetriever(store, logr.Discard())

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
	r := NewCompositeRetriever(store, logr.Discard())

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
	r := NewCompositeRetriever(store, logr.Discard())

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
	r := NewCompositeRetriever(store, logr.Discard())

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
	r := NewCompositeRetriever(store, logr.Discard())

	if _, err := r.RetrieveContext(context.Background(), map[string]string{"workspace_id": "ws", "user_id": "alice"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := r.RetrieveContext(context.Background(), map[string]string{"workspace_id": "ws", "user_id": "bob"}, nil); err != nil {
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
	r := NewCompositeRetriever(store, logr.Discard())

	got, err := r.RetrieveContext(context.Background(), defaultScope(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "p1" {
		t.Errorf("expected only profile-category memory, got %v", got)
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
