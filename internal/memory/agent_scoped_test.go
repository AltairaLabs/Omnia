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

const (
	testAgent1 = "b0000000-0000-0000-0000-000000000011"
	testAgent2 = "b0000000-0000-0000-0000-000000000012"
)

// TestSaveAgentScoped_RequiresWorkspace short-circuits on the guard without
// touching the pool — asserts the workspace validation fires before any DB
// access.
func TestSaveAgentScoped_RequiresWorkspace(t *testing.T) {
	s := &PostgresMemoryStore{}
	err := s.SaveAgentScoped(context.Background(), &Memory{Scope: map[string]string{}})
	if err == nil || err.Error() != errWorkspaceRequired {
		t.Fatalf("want %q, got %v", errWorkspaceRequired, err)
	}
}

// TestSaveAgentScoped_RequiresAgent asserts the agent_id guard fires after
// workspace.
func TestSaveAgentScoped_RequiresAgent(t *testing.T) {
	s := &PostgresMemoryStore{}
	err := s.SaveAgentScoped(context.Background(), &Memory{
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1},
	})
	if err == nil || err.Error() != errAgentIDRequired {
		t.Fatalf("want %q, got %v", errAgentIDRequired, err)
	}
}

func TestSaveAgentScoped_WritesRow(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	mem := &Memory{
		Type:       "policy",
		Content:    "always cite sources",
		Confidence: 1.0,
		Scope: map[string]string{
			ScopeWorkspaceID: testWorkspace1,
			ScopeAgentID:     testAgent1,
		},
	}
	if err := store.SaveAgentScoped(ctx, mem); err != nil {
		t.Fatalf("SaveAgentScoped: %v", err)
	}
	if mem.ID == "" {
		t.Fatalf("ID not populated")
	}
	// Provenance must be forced to operator_curated.
	if got, _ := mem.Metadata[pkmemory.MetaKeyProvenance].(string); got != string(pkmemory.ProvenanceOperatorCurated) {
		t.Errorf("provenance=%q, want %q", got, pkmemory.ProvenanceOperatorCurated)
	}
	// Scope must be sanitized — no user_id key, agent_id preserved.
	if _, ok := mem.Scope[ScopeUserID]; ok {
		t.Errorf("scope still contains user_id after sanitization: %+v", mem.Scope)
	}
	if mem.Scope[ScopeAgentID] != testAgent1 {
		t.Errorf("agent_id lost: %+v", mem.Scope)
	}

	got, err := store.ListAgentScoped(ctx, testWorkspace1, testAgent1, ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListAgentScoped: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("expected at least 1 agent-scoped memory, got 0")
	}
}

// TestSaveAgentScoped_StripsUserIDFromScope guards against a malicious
// caller passing a user_id in the scope map — SaveAgentScoped must drop it
// before the insert path runs so the row is truly agent-tier, not user-tier.
func TestSaveAgentScoped_StripsUserIDFromScope(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	user := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	mem := &Memory{
		Type: "policy", Content: "strip me", Confidence: 1.0,
		Scope: map[string]string{
			ScopeWorkspaceID: testWorkspace1,
			ScopeAgentID:     testAgent1,
			ScopeUserID:      user, // <- should be dropped
		},
	}
	if err := store.SaveAgentScoped(ctx, mem); err != nil {
		t.Fatalf("SaveAgentScoped: %v", err)
	}

	// Listing as the user should NOT surface the agent-scoped row.
	userRes, err := store.Retrieve(ctx,
		map[string]string{ScopeWorkspaceID: testWorkspace1, ScopeUserID: user},
		"strip me", RetrieveOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	for _, m := range userRes {
		if m.Content == "strip me" {
			t.Errorf("agent-scoped row leaked into user scope: %+v", m)
		}
	}
}

func TestListAgentScoped_FiltersByAgent(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	// Agent-1 row.
	must(t, store.SaveAgentScoped(ctx, &Memory{
		Type: "policy", Content: "a1-policy", Confidence: 1.0,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1, ScopeAgentID: testAgent1},
	}))
	// Agent-2 row.
	must(t, store.SaveAgentScoped(ctx, &Memory{
		Type: "policy", Content: "a2-policy", Confidence: 1.0,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1, ScopeAgentID: testAgent2},
	}))
	// Institutional row (should not appear in either agent list).
	must(t, store.SaveInstitutional(ctx, &Memory{
		Type: "policy", Content: "inst", Confidence: 1.0,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1},
	}))

	got, err := store.ListAgentScoped(ctx, testWorkspace1, testAgent1, ListOptions{Limit: 20})
	if err != nil {
		t.Fatalf("ListAgentScoped agent1: %v", err)
	}
	if len(got) != 1 || got[0].Content != "a1-policy" {
		t.Fatalf("expected exactly a1-policy, got %+v", got)
	}

	got2, err := store.ListAgentScoped(ctx, testWorkspace1, testAgent2, ListOptions{Limit: 20})
	if err != nil {
		t.Fatalf("ListAgentScoped agent2: %v", err)
	}
	if len(got2) != 1 || got2[0].Content != "a2-policy" {
		t.Fatalf("expected exactly a2-policy, got %+v", got2)
	}
}

func TestListAgentScoped_RequiresWorkspaceAndAgent(t *testing.T) {
	s := &PostgresMemoryStore{}
	if _, err := s.ListAgentScoped(context.Background(), "", testAgent1, ListOptions{}); err == nil {
		t.Error("expected workspace guard error")
	}
	if _, err := s.ListAgentScoped(context.Background(), testWorkspace1, "", ListOptions{}); err == nil {
		t.Error("expected agent guard error")
	}
}

func TestDeleteAgentScoped_RequiresWorkspaceAndAgent(t *testing.T) {
	s := &PostgresMemoryStore{}
	if err := s.DeleteAgentScoped(context.Background(), "", testAgent1, "id"); err == nil {
		t.Error("expected workspace guard error")
	}
	if err := s.DeleteAgentScoped(context.Background(), testWorkspace1, "", "id"); err == nil {
		t.Error("expected agent guard error")
	}
}

func TestDeleteAgentScoped_SoftDeletesOnlyAgentScoped(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	user := "dddddddd-dddd-dddd-dddd-dddddddddddd"

	agentMem := &Memory{
		Type: "policy", Content: "agent-policy", Confidence: 1.0,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1, ScopeAgentID: testAgent1},
	}
	must(t, store.SaveAgentScoped(ctx, agentMem))

	userMem := &Memory{
		Type: "pref", Content: "user-pref", Confidence: 0.8,
		Scope: map[string]string{ScopeWorkspaceID: testWorkspace1, ScopeUserID: user},
	}
	must(t, store.Save(ctx, userMem))

	// Deleting the user row through the agent-scoped path must be refused.
	err := store.DeleteAgentScoped(ctx, testWorkspace1, testAgent1, userMem.ID)
	if !errors.Is(err, ErrNotAgentScoped) {
		t.Errorf("want ErrNotAgentScoped, got %v", err)
	}

	// Deleting an agent-scoped row through a different agent must also be refused.
	err = store.DeleteAgentScoped(ctx, testWorkspace1, testAgent2, agentMem.ID)
	if !errors.Is(err, ErrNotAgentScoped) {
		t.Errorf("want ErrNotAgentScoped (wrong agent), got %v", err)
	}

	// Happy path: correct agent, correct row.
	if err := store.DeleteAgentScoped(ctx, testWorkspace1, testAgent1, agentMem.ID); err != nil {
		t.Fatalf("DeleteAgentScoped happy path: %v", err)
	}

	// Verify the row is gone from the list.
	got, err := store.ListAgentScoped(ctx, testWorkspace1, testAgent1, ListOptions{Limit: 20})
	if err != nil {
		t.Fatalf("ListAgentScoped after delete: %v", err)
	}
	for _, m := range got {
		if m.ID == agentMem.ID {
			t.Errorf("deleted row still listed: %+v", m)
		}
	}
}
