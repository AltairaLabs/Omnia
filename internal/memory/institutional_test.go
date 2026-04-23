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
	"errors"
	"testing"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
)

// TestSaveInstitutional_RequiresWorkspace is a pure unit test: the workspace
// guard must short-circuit before the nil pool is dereferenced. Using a zero-
// value store exercises the guard without a database.
func TestSaveInstitutional_RequiresWorkspace(t *testing.T) {
	s := &PostgresMemoryStore{}
	err := s.SaveInstitutional(context.Background(), &Memory{Scope: map[string]string{}})
	if err == nil || err.Error() != errWorkspaceRequired {
		t.Fatalf("want %q, got %v", errWorkspaceRequired, err)
	}
}

func TestSaveInstitutional_WritesRow(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	mem := &Memory{
		Type:       "policy",
		Content:    "API uses snake_case",
		Confidence: 1.0,
		Scope:      map[string]string{ScopeWorkspaceID: testWorkspace1},
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
	if _, ok := mem.Scope[ScopeUserID]; ok {
		t.Errorf("scope still contains user_id after sanitization: %+v", mem.Scope)
	}
	if _, ok := mem.Scope[ScopeAgentID]; ok {
		t.Errorf("scope still contains agent_id after sanitization: %+v", mem.Scope)
	}

	got, err := store.ListInstitutional(ctx, testWorkspace1, ListOptions{Limit: 10})
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
	store := newStore(t)
	ctx := context.Background()

	mem := &Memory{
		Type:       "policy",
		Content:    "spoof attempt",
		Confidence: 1.0,
		Scope:      map[string]string{ScopeWorkspaceID: testWorkspace1},
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
	s := &PostgresMemoryStore{}
	_, err := s.ListInstitutional(context.Background(), "", ListOptions{})
	if err == nil || err.Error() != errWorkspaceRequired {
		t.Fatalf("want %q, got %v", errWorkspaceRequired, err)
	}
}

func TestListInstitutional_ExcludesUserAndAgentRows(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	user := "33333333-3333-3333-3333-333333333333"
	agent := "44444444-4444-4444-4444-444444444444"

	// Institutional (should appear).
	must(t, store.SaveInstitutional(ctx, &Memory{
		Type: "policy", Content: "inst", Confidence: 1.0,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1},
	}))
	// User-scoped (should NOT appear).
	must(t, store.Save(ctx, &Memory{
		Type: "pref", Content: "user", Confidence: 0.8,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1, ScopeUserID: user},
	}))
	// Agent-only via raw insert (Save enforces user_id).
	insertRawMemory(t, store, "", agent, "fact", "agent", 0.9)

	got, err := store.ListInstitutional(ctx, testWorkspace1, ListOptions{Limit: 20})
	if err != nil {
		t.Fatalf("ListInstitutional: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 institutional row, got %d: %+v", len(got), got)
	}
	for _, m := range got {
		if m.Scope[ScopeUserID] != "" || m.Scope[ScopeAgentID] != "" {
			t.Errorf("leaked non-institutional row: %+v", m)
		}
	}
}

func TestDeleteInstitutional_RequiresWorkspace(t *testing.T) {
	s := &PostgresMemoryStore{}
	err := s.DeleteInstitutional(context.Background(), "", "some-id")
	if err == nil || err.Error() != errWorkspaceRequired {
		t.Fatalf("want %q, got %v", errWorkspaceRequired, err)
	}
}

func TestDeleteInstitutional_SoftDeletesOnlyInstitutional(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	user := "66666666-6666-6666-6666-666666666666"

	inst := &Memory{
		Type: "policy", Content: "inst", Confidence: 1.0,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1},
	}
	must(t, store.SaveInstitutional(ctx, inst))

	userMem := &Memory{
		Type: "pref", Content: "user", Confidence: 0.8,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1, ScopeUserID: user},
	}
	must(t, store.Save(ctx, userMem))

	// Delete institutional -> succeeds.
	if err := store.DeleteInstitutional(ctx, testWorkspace1, inst.ID); err != nil {
		t.Fatalf("DeleteInstitutional (inst): %v", err)
	}

	// The institutional row must no longer appear in the list.
	after, err := store.ListInstitutional(ctx, testWorkspace1, ListOptions{Limit: 10})
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
	err = store.DeleteInstitutional(ctx, testWorkspace1, userMem.ID)
	if err == nil {
		t.Fatal("expected DeleteInstitutional to refuse user-scoped row")
	}
	if !errors.Is(err, ErrNotInstitutional) {
		t.Errorf("err=%v, want ErrNotInstitutional (errors.Is)", err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
