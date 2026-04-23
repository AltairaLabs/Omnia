/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFindCompactionCandidates_RequiresWorkspace(t *testing.T) {
	s := &PostgresMemoryStore{}
	_, err := s.FindCompactionCandidates(context.Background(), FindCompactionCandidatesOptions{
		OlderThan: time.Now(),
	})
	if err == nil || err.Error() != errWorkspaceRequired {
		t.Fatalf("want %q, got %v", errWorkspaceRequired, err)
	}
}

func TestFindCompactionCandidates_RequiresOlderThan(t *testing.T) {
	s := &PostgresMemoryStore{}
	_, err := s.FindCompactionCandidates(context.Background(), FindCompactionCandidatesOptions{
		WorkspaceID: "ws-1",
	})
	if err == nil {
		t.Fatal("expected error when OlderThan is zero")
	}
}

func TestFindCompactionCandidates_GroupsAndFiltersByAge(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "aa000000-0000-0000-0000-000000000001"
	user := "aa000000-0000-0000-0000-000000000002"

	// Seed 12 recent observations (today) — should be ignored by OlderThan.
	for i := 0; i < 12; i++ {
		must(t, store.Save(ctx, &Memory{
			Type: "note", Content: "recent", Confidence: 1.0,
			Scope: map[string]string{ScopeWorkspaceID: ws, ScopeUserID: user},
		}))
	}
	// Seed 15 old observations via raw insert with a back-dated observed_at.
	mustInsertOldEntities(t, store, ws, user, "", 15, "old content", time.Now().Add(-90*24*time.Hour))

	candidates, err := store.FindCompactionCandidates(ctx, FindCompactionCandidatesOptions{
		WorkspaceID:   ws,
		OlderThan:     time.Now().Add(-30 * 24 * time.Hour),
		MinGroupSize:  10,
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("FindCompactionCandidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(candidates))
	}
	c := candidates[0]
	if c.WorkspaceID != ws || c.UserID != user || c.AgentID != "" {
		t.Errorf("wrong bucket coords: %+v", c)
	}
	if len(c.Entries) != 15 {
		t.Errorf("expected 15 old entries, got %d", len(c.Entries))
	}
}

func TestFindCompactionCandidates_SkipsBelowMinGroupSize(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "aa000000-0000-0000-0000-000000000010"
	user := "aa000000-0000-0000-0000-000000000011"

	mustInsertOldEntities(t, store, ws, user, "", 3, "too few", time.Now().Add(-90*24*time.Hour))

	candidates, err := store.FindCompactionCandidates(ctx, FindCompactionCandidatesOptions{
		WorkspaceID:  ws,
		OlderThan:    time.Now().Add(-30 * 24 * time.Hour),
		MinGroupSize: 10,
	})
	if err != nil {
		t.Fatalf("FindCompactionCandidates: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 buckets (below min), got %d", len(candidates))
	}
}

func TestFindCompactionCandidates_AppliesDefaults(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "aa000000-0000-0000-0000-000000000020"

	// No-op seed — we don't actually need rows; verifying zero-value defaults
	// doesn't blow up and still runs to completion.
	_, err := store.FindCompactionCandidates(ctx, FindCompactionCandidatesOptions{
		WorkspaceID: ws,
		OlderThan:   time.Now(),
	})
	if err != nil {
		t.Errorf("defaults path failed: %v", err)
	}
}

func TestSaveCompactionSummary_SupersedesAndInserts(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "bb000000-0000-0000-0000-000000000001"
	user := "bb000000-0000-0000-0000-000000000002"

	mustInsertOldEntities(t, store, ws, user, "", 3, "will be summarized", time.Now().Add(-90*24*time.Hour))

	candidates, err := store.FindCompactionCandidates(ctx, FindCompactionCandidatesOptions{
		WorkspaceID:  ws,
		OlderThan:    time.Now().Add(-30 * 24 * time.Hour),
		MinGroupSize: 1,
	})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}

	summaryID, err := store.SaveCompactionSummary(ctx, CompactionSummary{
		WorkspaceID:            ws,
		UserID:                 user,
		Content:                "Summary: 3 notes about the project.",
		SupersededObservations: candidates[0].ObservationIDs,
	})
	if err != nil {
		t.Fatalf("SaveCompactionSummary: %v", err)
	}
	if summaryID == "" {
		t.Fatal("summary entity ID empty")
	}

	// The originals should no longer appear in retrieval (observation join
	// filters out superseded rows).
	res, err := store.RetrieveMultiTier(ctx, MultiTierRequest{
		WorkspaceID: ws, UserID: user, Query: "will be summarized", Limit: 50,
	})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	for _, m := range res.Memories {
		if m.Content == "will be summarized" {
			t.Errorf("superseded memory leaked into retrieval: %+v", m)
		}
	}
}

func TestSaveCompactionSummary_ReturnsRaceSentinel(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	ws := "cc000000-0000-0000-0000-000000000001"
	user := "cc000000-0000-0000-0000-000000000002"

	mustInsertOldEntities(t, store, ws, user, "", 2, "race test", time.Now().Add(-90*24*time.Hour))

	candidates, err := store.FindCompactionCandidates(ctx, FindCompactionCandidatesOptions{
		WorkspaceID:  ws,
		OlderThan:    time.Now().Add(-30 * 24 * time.Hour),
		MinGroupSize: 1,
	})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	obsIDs := candidates[0].ObservationIDs

	// First summary wins.
	if _, err := store.SaveCompactionSummary(ctx, CompactionSummary{
		WorkspaceID: ws, UserID: user, Content: "first", SupersededObservations: obsIDs,
	}); err != nil {
		t.Fatalf("first summary: %v", err)
	}

	// Second attempt over the same IDs races — should return ErrCompactionRaced.
	_, err = store.SaveCompactionSummary(ctx, CompactionSummary{
		WorkspaceID: ws, UserID: user, Content: "second", SupersededObservations: obsIDs,
	})
	if !errors.Is(err, ErrCompactionRaced) {
		t.Errorf("expected ErrCompactionRaced, got %v", err)
	}
}

func TestSaveCompactionSummary_Validation(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	cases := []struct {
		name   string
		input  CompactionSummary
		wanted string
	}{
		{"no workspace", CompactionSummary{Content: "x", SupersededObservations: []string{"a"}}, errWorkspaceRequired},
		{"no content", CompactionSummary{WorkspaceID: "ws", SupersededObservations: []string{"a"}}, "content is required"},
		{"no superseded", CompactionSummary{WorkspaceID: "ws", Content: "x"}, "at least one observation"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := store.SaveCompactionSummary(ctx, c.input)
			if err == nil || !contains(err.Error(), c.wanted) {
				t.Errorf("%s: want error containing %q, got %v", c.name, c.wanted, err)
			}
		})
	}
}

// --- helpers --------------------------------------------------------------

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

// mustInsertOldEntities writes `n` entity+observation pairs with a back-dated
// observed_at so the compaction candidate scan picks them up.
func mustInsertOldEntities(t *testing.T, store *PostgresMemoryStore, workspaceID, userID, agentID string, n int, content string, observedAt time.Time) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		var entityID string
		var userArg, agentArg any
		if userID != "" {
			userArg = userID
		}
		if agentID != "" {
			agentArg = agentID
		}
		row := store.pool.QueryRow(ctx, `
			INSERT INTO memory_entities (workspace_id, virtual_user_id, agent_id, name, kind)
			VALUES ($1, $2, $3, $4, 'note')
			RETURNING id`,
			workspaceID, userArg, agentArg, content)
		if err := row.Scan(&entityID); err != nil {
			t.Fatalf("insert entity: %v", err)
		}
		_, err := store.pool.Exec(ctx, `
			INSERT INTO memory_observations (entity_id, content, confidence, observed_at)
			VALUES ($1, $2, 1.0, $3)`, entityID, content, observedAt)
		if err != nil {
			t.Fatalf("insert observation: %v", err)
		}
	}
}
