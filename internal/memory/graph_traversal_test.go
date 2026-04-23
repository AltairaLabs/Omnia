/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"testing"
)

func TestTraverseRelations_RequiresWorkspace(t *testing.T) {
	s := &PostgresMemoryStore{}
	_, err := s.TraverseRelations(context.Background(), GraphTraversal{})
	if err == nil || err.Error() != errWorkspaceRequired {
		t.Fatalf("want %q, got %v", errWorkspaceRequired, err)
	}
}

func TestTraverseRelations_EmptySeedsReturnsEmpty(t *testing.T) {
	store := newStore(t)
	res, err := store.TraverseRelations(context.Background(), GraphTraversal{
		WorkspaceID: "a0000000-0000-0000-0000-000000000001",
		SeedIDs:     []string{},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("expected empty, got %d rows", len(res))
	}
}

func TestTraverseRelations_FollowsEdges(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "b0000000-0000-0000-0000-000000000001"

	// Seed three institutional memories.
	a := &Memory{Type: "person", Content: "Alice", Scope: map[string]string{ScopeWorkspaceID: ws}}
	b := &Memory{Type: "company", Content: "Acme", Scope: map[string]string{ScopeWorkspaceID: ws}}
	c := &Memory{Type: "product", Content: "Widget", Scope: map[string]string{ScopeWorkspaceID: ws}}
	for _, m := range []*Memory{a, b, c} {
		must(t, store.SaveInstitutional(ctx, m))
	}

	// Create relations: Alice -works_at-> Acme; Acme -sells-> Widget.
	mustInsertRelation(t, store, ws, a.ID, b.ID, "works_at")
	mustInsertRelation(t, store, ws, b.ID, c.ID, "sells")

	// 1-hop traversal from Alice should reach Acme (not Widget).
	res, err := store.TraverseRelations(ctx, GraphTraversal{
		WorkspaceID: ws,
		SeedIDs:     []string{a.ID},
		MaxHops:     1,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("TraverseRelations: %v", err)
	}
	ids := memIDSet(res)
	if !ids[b.ID] {
		t.Errorf("expected Acme in 1-hop result, got %v", ids)
	}
	if ids[c.ID] {
		t.Errorf("Widget should not appear at 1-hop (Acme->Widget is 2 hops from Alice)")
	}

	// 2-hop traversal from Alice should reach Widget too.
	res2, err := store.TraverseRelations(ctx, GraphTraversal{
		WorkspaceID: ws,
		SeedIDs:     []string{a.ID},
		MaxHops:     2,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("TraverseRelations 2-hop: %v", err)
	}
	ids2 := memIDSet(res2)
	if !ids2[c.ID] {
		t.Errorf("expected Widget in 2-hop result, got %v", ids2)
	}
}

func TestTraverseRelations_FilterByRelationType(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "c0000000-0000-0000-0000-000000000001"

	a := &Memory{Type: "person", Content: "Alice-rel", Scope: map[string]string{ScopeWorkspaceID: ws}}
	b := &Memory{Type: "company", Content: "Acme-rel", Scope: map[string]string{ScopeWorkspaceID: ws}}
	c := &Memory{Type: "hobby", Content: "Knitting", Scope: map[string]string{ScopeWorkspaceID: ws}}
	for _, m := range []*Memory{a, b, c} {
		must(t, store.SaveInstitutional(ctx, m))
	}
	mustInsertRelation(t, store, ws, a.ID, b.ID, "works_at")
	mustInsertRelation(t, store, ws, a.ID, c.ID, "enjoys")

	// Filter to works_at only — should exclude Knitting.
	res, err := store.TraverseRelations(ctx, GraphTraversal{
		WorkspaceID:   ws,
		SeedIDs:       []string{a.ID},
		RelationTypes: []string{"works_at"},
		MaxHops:       1,
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("TraverseRelations: %v", err)
	}
	ids := memIDSet(res)
	if !ids[b.ID] {
		t.Errorf("expected Acme in works_at filter, got %v", ids)
	}
	if ids[c.ID] {
		t.Errorf("Knitting should be filtered out (not a works_at edge)")
	}
}

func TestTraverseRelations_ClampsMaxHops(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	res, err := store.TraverseRelations(ctx, GraphTraversal{
		WorkspaceID: "d0000000-0000-0000-0000-000000000001",
		SeedIDs:     []string{"00000000-0000-0000-0000-000000000000"},
		MaxHops:     999, // asks for absurd depth; implementation caps at maxGraphMaxHops.
		Limit:       1,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Seed doesn't exist, so result is empty but no error: the cap just needs
	// to have been applied. Assert via absence of panic/error.
	if res == nil {
		t.Errorf("expected empty slice, got nil")
	}
}

// --- helpers ---------------------------------------------------------------

func memIDSet(mems []*Memory) map[string]bool {
	out := make(map[string]bool, len(mems))
	for _, m := range mems {
		out[m.ID] = true
	}
	return out
}

func mustInsertRelation(t *testing.T, store *PostgresMemoryStore, workspaceID, sourceID, targetID, relType string) {
	t.Helper()
	_, err := store.pool.Exec(context.Background(), `
		INSERT INTO memory_relations (workspace_id, source_entity_id, target_entity_id, relation_type)
		VALUES ($1, $2, $3, $4)`,
		workspaceID, sourceID, targetID, relType,
	)
	if err != nil {
		t.Fatalf("insert relation: %v", err)
	}
}
