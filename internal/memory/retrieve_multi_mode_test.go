/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"testing"
)

func TestRetrieveMultiTier_MergesGraphTraversal(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "e0000000-0000-0000-0000-000000000001"

	seed := &Memory{Type: "person", Content: "Alice-graph", Scope: map[string]string{ScopeWorkspaceID: ws}}
	related := &Memory{Type: "company", Content: "Acme-graph", Scope: map[string]string{ScopeWorkspaceID: ws}}
	seedInstitutional(t, store, seed)
	seedInstitutional(t, store, related)
	mustInsertRelation(t, store, ws, seed.ID, related.ID, "works_at")

	// Use a query that matches seed by ILIKE but NOT related ("Alice-graph").
	// Without graph traversal, Acme-graph wouldn't appear; with it, it should.
	res, err := store.RetrieveMultiTier(ctx, MultiTierRequest{
		WorkspaceID:   ws,
		Query:         "Alice-graph",
		SeedEntityIDs: []string{seed.ID},
		MaxGraphHops:  1,
		Limit:         20,
	})
	if err != nil {
		t.Fatalf("RetrieveMultiTier: %v", err)
	}

	ids := map[string]bool{}
	for _, m := range res.Memories {
		ids[m.ID] = true
	}
	if !ids[related.ID] {
		t.Errorf("expected graph-traversed Acme-graph in result, got %v", ids)
	}
}

func TestRetrieveMultiTier_MergesStructuredLookup(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "f0000000-0000-0000-0000-000000000001"

	other := &Memory{Type: "fact", Content: "unrelated stuff", Scope: map[string]string{ScopeWorkspaceID: ws}}
	policy := &Memory{Type: "policy", Content: "API uses snake_case", Scope: map[string]string{ScopeWorkspaceID: ws}}
	seedInstitutional(t, store, other)
	seedInstitutional(t, store, policy)

	// Query string won't match policy content. Structured lookup pulls it in.
	res, err := store.RetrieveMultiTier(ctx, MultiTierRequest{
		WorkspaceID: ws,
		Query:       "nonexistent-query-string",
		StructuredLookups: []StructuredLookup{
			{WorkspaceID: ws, Kinds: []string{"policy"}},
		},
		Limit: 20,
	})
	if err != nil {
		t.Fatalf("RetrieveMultiTier: %v", err)
	}

	ids := map[string]bool{}
	for _, m := range res.Memories {
		ids[m.ID] = true
	}
	if !ids[policy.ID] {
		t.Errorf("expected policy from structured lookup in result, got %v", ids)
	}
}

func TestRetrieveMultiTier_DedupesAcrossSources(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "10000001-0000-0000-0000-000000000001"

	mem := &Memory{Type: "policy", Content: "dedupe me", Scope: map[string]string{ScopeWorkspaceID: ws}}
	seedInstitutional(t, store, mem)

	// The ILIKE query + the structured lookup should both match this row.
	// Expect exactly one copy in the result.
	res, err := store.RetrieveMultiTier(ctx, MultiTierRequest{
		WorkspaceID: ws,
		Query:       "dedupe",
		StructuredLookups: []StructuredLookup{
			{WorkspaceID: ws, Kinds: []string{"policy"}},
		},
		Limit: 20,
	})
	if err != nil {
		t.Fatalf("RetrieveMultiTier: %v", err)
	}

	count := 0
	for _, m := range res.Memories {
		if m.ID == mem.ID {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 copy of dedupe row, got %d", count)
	}
}

func TestClassifyTierFromScope(t *testing.T) {
	cases := []struct {
		name  string
		scope map[string]string
		want  Tier
	}{
		{"institutional", map[string]string{ScopeWorkspaceID: "w"}, TierInstitutional},
		{"agent", map[string]string{ScopeWorkspaceID: "w", ScopeAgentID: "a"}, TierAgent},
		{"user", map[string]string{ScopeWorkspaceID: "w", ScopeVirtualUserID: "u"}, TierUser},
		{"user-for-agent", map[string]string{ScopeWorkspaceID: "w", ScopeVirtualUserID: "u", ScopeAgentID: "a"}, TierUserForAgent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyTierFromScope(c.scope); got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}
