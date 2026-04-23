/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMultiTierQuery_RequiresWorkspace(t *testing.T) {
	_, _, err := buildMultiTierQuery(MultiTierRequest{})
	if err == nil {
		t.Fatal("expected error for missing workspace_id")
	}
}

func TestBuildMultiTierQuery_InstitutionalOnly(t *testing.T) {
	sql, args, err := buildMultiTierQuery(MultiTierRequest{
		WorkspaceID: "ws-1",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(sql, "workspace_id=$1") {
		t.Errorf("workspace filter missing: %s", sql)
	}
	// With no user, predicate must anchor to NULL user_id (no bleed-through).
	if !strings.Contains(sql, "virtual_user_id IS NULL") {
		t.Errorf("user NULL anchor missing: %s", sql)
	}
	if !strings.Contains(sql, "agent_id IS NULL") {
		t.Errorf("agent NULL anchor missing: %s", sql)
	}
	if len(args) != 1 || args[0] != "ws-1" {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestBuildMultiTierQuery_UserAndAgent(t *testing.T) {
	sql, args, err := buildMultiTierQuery(MultiTierRequest{
		WorkspaceID: "ws-1",
		UserID:      "u-1",
		AgentID:     "a-1",
		Limit:       25,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(sql, "virtual_user_id IS NULL OR virtual_user_id=$2") {
		t.Errorf("user tier predicate missing: %s", sql)
	}
	if !strings.Contains(sql, "agent_id IS NULL OR agent_id=$3") {
		t.Errorf("agent tier predicate missing: %s", sql)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d: %v", len(args), args)
	}
}

func TestBuildMultiTierQuery_QueryAddsILIKE(t *testing.T) {
	sql, args, err := buildMultiTierQuery(MultiTierRequest{
		WorkspaceID: "ws-1",
		UserID:      "u-1",
		Query:       "dark mode",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(sql, "o.content ILIKE") {
		t.Errorf("ILIKE filter missing: %s", sql)
	}
	if args[len(args)-1] != "%dark mode%" {
		t.Errorf("ILIKE arg missing: %v", args)
	}
}

func TestBuildMultiTierQuery_DefaultCandidateFloor(t *testing.T) {
	sql, _, err := buildMultiTierQuery(MultiTierRequest{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Candidate pool of 200 is the SQL-level floor regardless of client Limit.
	if !strings.Contains(sql, "LIMIT 200") {
		t.Errorf("expected candidate LIMIT 200: %s", sql)
	}
}

func TestClassifyTier(t *testing.T) {
	u := "u"
	a := "a"
	cases := []struct {
		name    string
		userID  *string
		agentID *string
		want    Tier
	}{
		{"institutional", nil, nil, TierInstitutional},
		{"agent", nil, &a, TierAgent},
		{"user", &u, nil, TierUser},
		{"user-for-agent", &u, &a, TierUserForAgent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyTier(c.userID, c.agentID)
			if got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}

func TestRankResults_OrdersByScoreDesc(t *testing.T) {
	now := parseTime("2026-04-22T12:00:00Z")
	hour := time.Hour
	results := []*MultiTierMemory{
		{Memory: &Memory{Confidence: 0.5, AccessedAt: now.Add(-24 * hour)}, Tier: TierUser, AccessCount: 2},
		{Memory: &Memory{Confidence: 0.9, AccessedAt: now.Add(-1 * hour)}, Tier: TierUser, AccessCount: 10},
		{Memory: &Memory{Confidence: 0.7, AccessedAt: now.Add(-30 * 24 * hour)}, Tier: TierInstitutional, AccessCount: 1},
	}
	rankResults(results, now)
	if results[0].Confidence != 0.9 {
		t.Errorf("expected highest-confidence recent result first, got %+v", results[0])
	}
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// multiTierAgentID is a stable UUID used as the agent_id in multi-tier
// integration tests.
const multiTierAgentID = "b0000000-0000-0000-0000-000000000001"

// insertRawMemory writes a single memory_entities + memory_observations pair
// directly via the store's pool, bypassing Save()'s user_id-required
// invariant. It is needed because institutional and agent-only memories
// legitimately have no user_id, but Save() guards against missing user_id
// for the single-tier write path. The workspace is hard-coded to
// testWorkspace1 since every test in this file operates on that scope.
func insertRawMemory(t *testing.T, store *PostgresMemoryStore, user, agent, kind, content string, confidence float64) {
	t.Helper()
	var userArg, agentArg any
	if user != "" {
		userArg = user
	}
	if agent != "" {
		agentArg = agent
	}
	var entityID string
	err := store.pool.QueryRow(context.Background(),
		`INSERT INTO memory_entities (workspace_id, virtual_user_id, agent_id, name, kind, metadata)
		 VALUES ($1, $2, $3, $4, $5, '{}') RETURNING id`,
		testWorkspace1, userArg, agentArg, content, kind,
	).Scan(&entityID)
	require.NoError(t, err)

	_, err = store.pool.Exec(context.Background(),
		`INSERT INTO memory_observations (entity_id, content, confidence) VALUES ($1, $2, $3)`,
		entityID, content, confidence,
	)
	require.NoError(t, err)
}

func TestRetrieveMultiTier_RequiresWorkspace(t *testing.T) {
	store := newStore(t)
	_, err := store.RetrieveMultiTier(context.Background(), MultiTierRequest{})
	if err == nil {
		t.Fatal("expected error for missing workspace_id")
	}
}

func TestRetrieveMultiTier_SpansAllTiers(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	insertRawMemory(t, store, "", "", "preference", "institutional fact", 1.0)
	insertRawMemory(t, store, "", multiTierAgentID, "preference", "agent guideline", 1.0)
	insertRawMemory(t, store, "user-1", "", "preference", "user prefers dark mode", 1.0)
	insertRawMemory(t, store, "user-1", multiTierAgentID, "preference", "user-for-agent note", 1.0)

	result, err := store.RetrieveMultiTier(ctx, MultiTierRequest{
		WorkspaceID: testWorkspace1,
		UserID:      "user-1",
		AgentID:     multiTierAgentID,
		Limit:       10,
		Now:         time.Now(),
	})
	require.NoError(t, err)
	require.Len(t, result.Memories, 4)
	assert.Equal(t, 4, result.Total)

	tiers := map[Tier]*MultiTierMemory{}
	for _, m := range result.Memories {
		tiers[m.Tier] = m
	}
	assert.Contains(t, tiers, TierInstitutional)
	assert.Contains(t, tiers, TierAgent)
	assert.Contains(t, tiers, TierUser)
	assert.Contains(t, tiers, TierUserForAgent)

	// Scope map must include the tier fields that were present on the row.
	uForA := tiers[TierUserForAgent]
	assert.Equal(t, testWorkspace1, uForA.Scope[ScopeWorkspaceID])
	assert.Equal(t, "user-1", uForA.Scope[ScopeUserID])
	assert.Equal(t, multiTierAgentID, uForA.Scope[ScopeAgentID])
}

func TestRetrieveMultiTier_InstitutionalOnlyDoesNotBleed(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	insertRawMemory(t, store, "", "", "preference", "institutional only", 1.0)
	insertRawMemory(t, store, "user-2", "", "preference", "other user's data", 1.0)

	result, err := store.RetrieveMultiTier(ctx, MultiTierRequest{
		WorkspaceID: testWorkspace1,
		Limit:       10,
	})
	require.NoError(t, err)
	require.Len(t, result.Memories, 1)
	assert.Equal(t, TierInstitutional, result.Memories[0].Tier)
	assert.Empty(t, result.Memories[0].Scope[ScopeUserID])
}

func TestRetrieveMultiTier_TruncatesToLimit(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		insertRawMemory(t, store, "", "", "preference", "fact", 1.0)
	}

	result, err := store.RetrieveMultiTier(ctx, MultiTierRequest{
		WorkspaceID: testWorkspace1,
		Limit:       2,
	})
	require.NoError(t, err)
	require.Len(t, result.Memories, 2)
	assert.Equal(t, 2, result.Total)
}

func TestRetrieveMultiTier_DefaultLimitApplies(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	insertRawMemory(t, store, "", "", "preference", "fact", 1.0)

	// Leave Limit=0 so the defaultMultiTierLimit path is exercised.
	result, err := store.RetrieveMultiTier(ctx, MultiTierRequest{
		WorkspaceID: testWorkspace1,
	})
	require.NoError(t, err)
	assert.Len(t, result.Memories, 1)
}

// insertRawMemoryWithPurpose seeds a memory_entities row with an explicit
// purpose so purpose-filtered retrieval tests can exercise non-default
// values without routing through Save()'s user_id-required invariant.
func insertRawMemoryWithPurpose(t *testing.T, store *PostgresMemoryStore, purpose, user, agent, kind, content string, confidence float64) {
	t.Helper()
	var userArg, agentArg any
	if user != "" {
		userArg = user
	}
	if agent != "" {
		agentArg = agent
	}
	var entityID string
	err := store.pool.QueryRow(context.Background(),
		`INSERT INTO memory_entities (workspace_id, virtual_user_id, agent_id, name, kind, metadata, purpose)
		 VALUES ($1, $2, $3, $4, $5, '{}', $6) RETURNING id`,
		testWorkspace1, userArg, agentArg, content, kind, purpose,
	).Scan(&entityID)
	require.NoError(t, err)

	_, err = store.pool.Exec(context.Background(),
		`INSERT INTO memory_observations (entity_id, content, confidence) VALUES ($1, $2, $3)`,
		entityID, content, confidence,
	)
	require.NoError(t, err)
}

func TestRetrieveMultiTier_PurposeFilter(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	insertRawMemoryWithPurpose(t, store, "support_continuity", "user-1", "", "fact", "support row", 1.0)
	insertRawMemoryWithPurpose(t, store, "personalisation", "user-1", "", "fact", "personalisation row", 1.0)
	insertRawMemoryWithPurpose(t, store, "compliance", "user-1", "", "fact", "compliance row", 1.0)

	t.Run("single purpose filters to matching rows", func(t *testing.T) {
		res, err := store.RetrieveMultiTier(ctx, MultiTierRequest{
			WorkspaceID: testWorkspace1,
			UserID:      "user-1",
			Purposes:    []string{"personalisation"},
			Limit:       10,
		})
		require.NoError(t, err)
		require.Len(t, res.Memories, 1)
		assert.Equal(t, "personalisation row", res.Memories[0].Content)
	})

	t.Run("multiple purposes act as OR", func(t *testing.T) {
		res, err := store.RetrieveMultiTier(ctx, MultiTierRequest{
			WorkspaceID: testWorkspace1,
			UserID:      "user-1",
			Purposes:    []string{"personalisation", "compliance"},
			Limit:       10,
		})
		require.NoError(t, err)
		require.Len(t, res.Memories, 2)
		seen := map[string]bool{}
		for _, m := range res.Memories {
			seen[m.Content] = true
		}
		assert.True(t, seen["personalisation row"])
		assert.True(t, seen["compliance row"])
		assert.False(t, seen["support row"])
	})

	t.Run("empty purposes returns everything", func(t *testing.T) {
		res, err := store.RetrieveMultiTier(ctx, MultiTierRequest{
			WorkspaceID: testWorkspace1,
			UserID:      "user-1",
			Limit:       10,
		})
		require.NoError(t, err)
		require.Len(t, res.Memories, 3, "no filter should return all 3 rows")
	})

	t.Run("non-matching purpose returns empty", func(t *testing.T) {
		res, err := store.RetrieveMultiTier(ctx, MultiTierRequest{
			WorkspaceID: testWorkspace1,
			UserID:      "user-1",
			Purposes:    []string{"never_used"},
			Limit:       10,
		})
		require.NoError(t, err)
		assert.Empty(t, res.Memories)
	})
}

func TestRetrieveMultiTier_TypeAndConfidenceFilters(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	insertRawMemory(t, store, "", "", "preference", "high conf", 0.9)
	insertRawMemory(t, store, "", "", "episodic", "low conf", 0.1)
	insertRawMemory(t, store, "", "", "fact", "third kind", 0.95)

	// Multi-type filter + confidence threshold.
	result, err := store.RetrieveMultiTier(ctx, MultiTierRequest{
		WorkspaceID:   testWorkspace1,
		Types:         []string{"preference", "fact"},
		MinConfidence: 0.5,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, result.Memories, 2)
}

func TestComputeScore_FallsBackToCreatedAtWhenAccessedZero(t *testing.T) {
	now := parseTime("2026-04-22T12:00:00Z")
	m := &MultiTierMemory{
		Memory:      &Memory{Confidence: 1.0, CreatedAt: now.Add(-time.Hour)},
		AccessCount: 0,
	}
	score := computeScore(m, now)
	// Zero AccessedAt must not decay recency to ~0 — CreatedAt kicks in.
	if score <= 0.5 {
		t.Errorf("score too low, CreatedAt fallback missed: %v", score)
	}
}

func TestComputeScore_FutureAccessedAtClampsToZeroAge(t *testing.T) {
	now := parseTime("2026-04-22T12:00:00Z")
	m := &MultiTierMemory{
		Memory:      &Memory{Confidence: 1.0, AccessedAt: now.Add(time.Hour)},
		AccessCount: 0,
	}
	score := computeScore(m, now)
	// Clock skew must not push recency above 1.0 (score bounded).
	if score > 1.0 {
		t.Errorf("score above 1.0: %v", score)
	}
}
