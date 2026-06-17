/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package memory

import (
	"context"
	"errors"
	"fmt"
	"testing"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"

	ossmemory "github.com/altairalabs/omnia/internal/memory"
)

// TestSaveInstitutional_RequiresWorkspace is a pure unit test: the workspace
// guard must short-circuit before the nil pool is dereferenced. Using a zero-
// value store exercises the guard without a database.
func TestSaveInstitutional_RequiresWorkspace(t *testing.T) {
	s := &PostgresInstitutionalStore{}
	err := s.SaveInstitutional(context.Background(), &ossmemory.Memory{Scope: map[string]string{}})
	if err == nil || err.Error() != errWorkspaceRequired {
		t.Fatalf("want %q, got %v", errWorkspaceRequired, err)
	}
}

func TestSaveInstitutional_WritesRow(t *testing.T) {
	store := newInstitutionalStore(t)
	ctx := context.Background()

	mem := &ossmemory.Memory{
		Type:       "policy",
		Content:    "API uses snake_case",
		Confidence: 1.0,
		Scope:      map[string]string{ossmemory.ScopeWorkspaceID: testWorkspace1},
	}
	if err := store.SaveInstitutional(ctx, mem); err != nil {
		t.Fatalf("SaveInstitutional: %v", err)
	}
	if mem.ID == "" {
		t.Fatalf("ID not populated")
	}
	// Provenance must be forced regardless of caller input.
	if got, _ := mem.Metadata[pkmemory.MetaKeyProvenance].(string); got != string(pkmemory.ProvenanceOperatorCurated) {
		t.Errorf("provenance=%q, want %q", got, pkmemory.ProvenanceOperatorCurated)
	}
	// Scope must be sanitized — no user/agent keys should remain.
	if _, ok := mem.Scope[ossmemory.ScopeUserID]; ok {
		t.Errorf("scope still contains user_id after sanitization: %+v", mem.Scope)
	}
	if _, ok := mem.Scope[ossmemory.ScopeAgentID]; ok {
		t.Errorf("scope still contains agent_id after sanitization: %+v", mem.Scope)
	}

	got, err := store.ListInstitutional(ctx, testWorkspace1, ossmemory.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListInstitutional: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("expected at least 1 institutional memory, got 0")
	}
}

// TestSaveInstitutional_OverwritesConflictingProvenance guards the rule that
// any caller-supplied provenance is overwritten with operator_curated — an
// operator API by definition emits operator_curated rows and a client must not
// be able to spoof user_requested, etc.
func TestSaveInstitutional_OverwritesConflictingProvenance(t *testing.T) {
	store := newInstitutionalStore(t)
	ctx := context.Background()

	mem := &ossmemory.Memory{
		Type:       "policy",
		Content:    "spoof attempt",
		Confidence: 1.0,
		Scope:      map[string]string{ossmemory.ScopeWorkspaceID: testWorkspace1},
		Metadata: map[string]any{
			pkmemory.MetaKeyProvenance: string(pkmemory.ProvenanceUserRequested),
		},
	}
	must(t, store.SaveInstitutional(ctx, mem))

	if got, _ := mem.Metadata[pkmemory.MetaKeyProvenance].(string); got != string(pkmemory.ProvenanceOperatorCurated) {
		t.Errorf("provenance=%q, want %q", got, pkmemory.ProvenanceOperatorCurated)
	}
}

func TestListInstitutional_RequiresWorkspace(t *testing.T) {
	s := &PostgresInstitutionalStore{}
	_, err := s.ListInstitutional(context.Background(), "", ossmemory.ListOptions{})
	if err == nil || err.Error() != errWorkspaceRequired {
		t.Fatalf("want %q, got %v", errWorkspaceRequired, err)
	}
}

func TestListInstitutional_ExcludesUserAndAgentRows(t *testing.T) {
	store := newInstitutionalStore(t)
	ctx := context.Background()

	user := "33333333-3333-3333-3333-333333333333"
	agent := "44444444-4444-4444-4444-444444444444"

	// Institutional (should appear).
	must(t, store.SaveInstitutional(ctx, &ossmemory.Memory{
		Type: "policy", Content: "inst", Confidence: 1.0,
		Scope: map[string]string{ossmemory.ScopeWorkspaceID: testWorkspace1},
	}))
	// User-scoped (should NOT appear) — use raw insert to bypass Save's user_id guard.
	insertRawMemory(t, store.pool, user, "", "pref", "user", 0.8)
	// Agent-only via raw insert (Save enforces user_id).
	insertRawMemory(t, store.pool, "", agent, "fact", "agent", 0.9)

	got, err := store.ListInstitutional(ctx, testWorkspace1, ossmemory.ListOptions{Limit: 20})
	if err != nil {
		t.Fatalf("ListInstitutional: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 institutional row, got %d: %+v", len(got), got)
	}
	for _, m := range got {
		if m.Scope[ossmemory.ScopeUserID] != "" || m.Scope[ossmemory.ScopeAgentID] != "" {
			t.Errorf("leaked non-institutional row: %+v", m)
		}
	}
}

func TestDeleteInstitutional_RequiresWorkspace(t *testing.T) {
	s := &PostgresInstitutionalStore{}
	err := s.DeleteInstitutional(context.Background(), "", "some-id")
	if err == nil || err.Error() != errWorkspaceRequired {
		t.Fatalf("want %q, got %v", errWorkspaceRequired, err)
	}
}

func TestDeleteInstitutional_SoftDeletesOnlyInstitutional(t *testing.T) {
	store := newInstitutionalStore(t)
	ctx := context.Background()

	user := "66666666-6666-6666-6666-666666666666"

	inst := &ossmemory.Memory{
		Type: "policy", Content: "inst", Confidence: 1.0,
		Scope: map[string]string{ossmemory.ScopeWorkspaceID: testWorkspace1},
	}
	must(t, store.SaveInstitutional(ctx, inst))

	// Insert a user-scoped row via raw insert so we have a non-institutional ID.
	var userMemID string
	err := store.pool.QueryRow(ctx,
		`INSERT INTO memory_entities (workspace_id, virtual_user_id, agent_id, name, kind, metadata)
		 VALUES ($1, $2, NULL, $3, $4, '{}') RETURNING id`,
		testWorkspace1, user, "user", "pref",
	).Scan(&userMemID)
	if err != nil {
		t.Fatalf("raw insert user mem: %v", err)
	}
	_, err = store.pool.Exec(ctx,
		`INSERT INTO memory_observations (entity_id, content, confidence) VALUES ($1, $2, $3)`,
		userMemID, "user", 0.8,
	)
	if err != nil {
		t.Fatalf("raw insert user obs: %v", err)
	}

	// Delete institutional -> succeeds.
	if err := store.DeleteInstitutional(ctx, testWorkspace1, inst.ID); err != nil {
		t.Fatalf("DeleteInstitutional (inst): %v", err)
	}

	// The institutional row must no longer appear in the list.
	after, err := store.ListInstitutional(ctx, testWorkspace1, ossmemory.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListInstitutional after delete: %v", err)
	}
	for _, m := range after {
		if m.ID == inst.ID {
			t.Errorf("soft-deleted institutional row still listed: %+v", m)
		}
	}

	// Delete user-scoped via institutional endpoint -> must refuse with the
	// exported sentinel so Task 4's handler can map to HTTP 400.
	err = store.DeleteInstitutional(ctx, testWorkspace1, userMemID)
	if err == nil {
		t.Fatal("expected DeleteInstitutional to refuse user-scoped row")
	}
	if !errors.Is(err, ErrNotInstitutional) {
		t.Errorf("err=%v, want ErrNotInstitutional (errors.Is)", err)
	}
}

// aboutKindSharePointDoc is the about_kind used by the ingest path; pinned here
// so the idempotency tests exercise the same structured-key shape.
const aboutKindSharePointDoc = "sharepoint_doc"

// TestSaveInstitutional_AboutKeyIdempotency verifies that re-saving the same
// about_kind+about_key pair produces exactly one active entity (upsert), not
// two, and that the content of the surviving entity reflects the second write.
// This covers the demo re-seed scenario where the seed Job runs on every
// helm upgrade: chunks must supersede rather than duplicate.
func TestSaveInstitutional_AboutKeyIdempotency(t *testing.T) {
	store := newInstitutionalStore(t)
	ctx := context.Background()

	save := func(content string) {
		must(t, store.SaveInstitutional(ctx, &ossmemory.Memory{
			Type:       aboutKindSharePointDoc,
			Content:    content,
			Confidence: 1.0,
			Scope:      map[string]string{ossmemory.ScopeWorkspaceID: testWorkspace1},
			Metadata: map[string]any{
				ossmemory.MetaKeyAboutKind: aboutKindSharePointDoc,
				ossmemory.MetaKeyAboutKey:  "https://sp/x#0",
			},
		}))
	}

	save("first write")
	save("second write") // same about_key — must supersede, not duplicate

	got, err := store.ListInstitutional(ctx, testWorkspace1, ossmemory.ListOptions{Limit: 20})
	if err != nil {
		t.Fatalf("ListInstitutional: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 active entity for about-key, got %d", len(got))
	}
	if got[0].Content != "second write" {
		t.Errorf("content=%q, want %q", got[0].Content, "second write")
	}
}

// TestSaveInstitutional_DifferentAboutKeysTwoEntities guards the complement:
// two saves with distinct about_keys must each produce their own entity.
func TestSaveInstitutional_DifferentAboutKeysTwoEntities(t *testing.T) {
	store := newInstitutionalStore(t)
	ctx := context.Background()

	for i, key := range []string{"https://sp/x#0", "https://sp/x#1"} {
		must(t, store.SaveInstitutional(ctx, &ossmemory.Memory{
			Type:       aboutKindSharePointDoc,
			Content:    fmt.Sprintf("chunk %d", i),
			Confidence: 1.0,
			Scope:      map[string]string{ossmemory.ScopeWorkspaceID: testWorkspace1},
			Metadata: map[string]any{
				ossmemory.MetaKeyAboutKind: aboutKindSharePointDoc,
				ossmemory.MetaKeyAboutKey:  key,
			},
		}))
	}

	got, err := store.ListInstitutional(ctx, testWorkspace1, ossmemory.ListOptions{Limit: 20})
	if err != nil {
		t.Fatalf("ListInstitutional: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected exactly 2 entities for 2 distinct about-keys, got %d", len(got))
	}
}
