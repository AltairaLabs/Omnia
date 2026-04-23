/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"testing"
)

func TestLookupStructured_RequiresWorkspace(t *testing.T) {
	s := &PostgresMemoryStore{}
	_, err := s.LookupStructured(context.Background(), StructuredLookup{})
	if err == nil || err.Error() != errWorkspaceRequired {
		t.Fatalf("want %q, got %v", errWorkspaceRequired, err)
	}
}

func TestLookupStructured_FiltersByKindAndPurpose(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "77777777-7777-7777-7777-777777777777"

	// Mixed rows; purpose defaults to support_continuity.
	must(t, store.SaveInstitutional(ctx, &Memory{
		Type: "policy", Content: "snake_case",
		Scope: map[string]string{ScopeWorkspaceID: ws},
	}))
	must(t, store.SaveInstitutional(ctx, &Memory{
		Type: "glossary", Content: "API terms",
		Scope: map[string]string{ScopeWorkspaceID: ws},
	}))

	// Filter to kind=policy — should only get one row.
	res, err := store.LookupStructured(ctx, StructuredLookup{
		WorkspaceID: ws,
		Kinds:       []string{"policy"},
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("LookupStructured: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 policy row, got %d", len(res))
	}
	if res[0].Type != "policy" {
		t.Errorf("wrong kind: %s", res[0].Type)
	}
}

func TestLookupStructured_FiltersByNamePrefix(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "88888888-8888-8888-8888-888888888888"
	must(t, store.SaveInstitutional(ctx, &Memory{
		Type: "doc", Content: "API authentication guide",
		Scope: map[string]string{ScopeWorkspaceID: ws},
	}))
	must(t, store.SaveInstitutional(ctx, &Memory{
		Type: "doc", Content: "Runbook for incident response",
		Scope: map[string]string{ScopeWorkspaceID: ws},
	}))

	res, err := store.LookupStructured(ctx, StructuredLookup{
		WorkspaceID: ws,
		NamePrefix:  "API",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("LookupStructured: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 'API'-prefixed row, got %d", len(res))
	}
}

func TestLookupStructured_MultiKindUsesAny(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "99999999-9999-9999-9999-999999999999"
	must(t, store.SaveInstitutional(ctx, &Memory{Type: "policy", Content: "a", Scope: map[string]string{ScopeWorkspaceID: ws}}))
	must(t, store.SaveInstitutional(ctx, &Memory{Type: "glossary", Content: "b", Scope: map[string]string{ScopeWorkspaceID: ws}}))
	must(t, store.SaveInstitutional(ctx, &Memory{Type: "note", Content: "c", Scope: map[string]string{ScopeWorkspaceID: ws}}))

	res, err := store.LookupStructured(ctx, StructuredLookup{
		WorkspaceID: ws,
		Kinds:       []string{"policy", "glossary"},
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("LookupStructured: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 matching kinds, got %d", len(res))
	}
}

func TestLookupStructured_UserAndAgentFilters(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	user := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	agent := "dddddddd-dddd-dddd-dddd-dddddddddddd"

	// Institutional (workspace only).
	must(t, store.SaveInstitutional(ctx, &Memory{
		Type: "rule", Content: "inst", Scope: map[string]string{ScopeWorkspaceID: ws},
	}))
	// User-scoped.
	must(t, store.Save(ctx, &Memory{
		Type: "rule", Content: "user", Confidence: 1.0,
		Scope: map[string]string{ScopeWorkspaceID: ws, ScopeUserID: user},
	}))
	// User-for-agent.
	must(t, store.Save(ctx, &Memory{
		Type: "rule", Content: "ua", Confidence: 1.0,
		Scope: map[string]string{ScopeWorkspaceID: ws, ScopeUserID: user, ScopeAgentID: agent},
	}))

	// Lookup with UserID set should include institutional + user + user-for-agent
	// rows (OR with NULL semantics).
	res, err := store.LookupStructured(ctx, StructuredLookup{
		WorkspaceID: ws,
		UserID:      user,
		Kinds:       []string{"rule"},
		Limit:       20,
	})
	if err != nil {
		t.Fatalf("LookupStructured: %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("expected 3 rows (inst+user+ua), got %d", len(res))
	}

	// With UserID + AgentID set, result is still all three (NULL OR match).
	res2, err := store.LookupStructured(ctx, StructuredLookup{
		WorkspaceID: ws,
		UserID:      user,
		AgentID:     agent,
		Kinds:       []string{"rule"},
		Limit:       20,
	})
	if err != nil {
		t.Fatalf("LookupStructured 2: %v", err)
	}
	if len(res2) != 3 {
		t.Errorf("expected 3 rows with (user, agent), got %d", len(res2))
	}
}

func TestLookupStructured_FiltersByPurpose(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"

	// Directly set purpose — SaveInstitutional leaves it at default
	// 'support_continuity'. We insert a second row with a distinct purpose
	// via raw SQL to exercise the WHERE e.purpose clause.
	must(t, store.SaveInstitutional(ctx, &Memory{
		Type: "note", Content: "default purpose row",
		Scope: map[string]string{ScopeWorkspaceID: ws},
	}))
	_, err := store.pool.Exec(ctx, `
		WITH e AS (
			INSERT INTO memory_entities (workspace_id, name, kind, purpose, source_type, trust_model)
			VALUES ($1, 'compliance row', 'note', 'compliance', 'operator_curated', 'curated')
			RETURNING id
		)
		INSERT INTO memory_observations (entity_id, content, confidence)
		SELECT id, 'compliance row', 1.0 FROM e`, ws)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := store.LookupStructured(ctx, StructuredLookup{
		WorkspaceID: ws,
		Purpose:     "compliance",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("LookupStructured: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 compliance row, got %d", len(res))
	}
	if res[0].Content != "compliance row" {
		t.Errorf("wrong row returned: %+v", res[0])
	}
}

func TestLookupStructured_EmptyResultsReturnsEmptySlice(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	res, err := store.LookupStructured(ctx, StructuredLookup{
		WorkspaceID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		Kinds:       []string{"nothing"},
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("expected empty result, got %d rows", len(res))
	}
}
