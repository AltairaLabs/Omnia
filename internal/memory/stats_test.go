/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	aggregateTestWorkspace = "b0000000-0000-0000-0000-000000001004"
	agentAUUID             = "c1000000-0000-0000-0000-000000000001"
	agentBUUID             = "c1000000-0000-0000-0000-000000000002"
)

// seedAggregateFixtures inserts a known set of memories + preferences.
// Returns the workspace ID for query convenience.
func seedAggregateFixtures(t *testing.T, store *PostgresMemoryStore) string {
	t.Helper()
	ctx := context.Background()
	pool := store.Pool()

	// Three users: granted (consents), denied (no consent), opted-out.
	_, err := pool.Exec(ctx, `
		INSERT INTO user_privacy_preferences (user_id, consent_grants)
		VALUES ($1, $2), ($3, $4)
		ON CONFLICT (user_id) DO UPDATE SET consent_grants = EXCLUDED.consent_grants`,
		"agg-user-granted", []string{"analytics:aggregate"},
		"agg-user-denied", []string{"memory:preferences"},
	)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO user_privacy_preferences (user_id, opt_out_all)
		VALUES ($1, TRUE)
		ON CONFLICT (user_id) DO UPDATE SET opt_out_all = EXCLUDED.opt_out_all`,
		"agg-user-opted-out",
	)
	require.NoError(t, err)

	insertMem := func(userID, agentID, category string, when time.Time) {
		var virtualUserID, agent any
		if userID != "" {
			virtualUserID = userID
		}
		if agentID != "" {
			agent = agentID
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO memory_entities
			    (workspace_id, virtual_user_id, agent_id, name, kind, metadata, consent_category, created_at)
			VALUES ($1, $2, $3, $4, $5, '{}'::jsonb, $6, $7)`,
			aggregateTestWorkspace, virtualUserID, agent, "fact", "fact", category, when,
		)
		require.NoError(t, err)
	}

	now := time.Now().UTC()
	day1 := now.Add(-72 * time.Hour)
	day2 := now.Add(-48 * time.Hour)
	day3 := now.Add(-24 * time.Hour)

	// Granted user: 3 memories spread over 3 days, 2 categories, 2 agents.
	insertMem("agg-user-granted", agentAUUID, "memory:context", day1)
	insertMem("agg-user-granted", agentAUUID, "memory:context", day2)
	insertMem("agg-user-granted", agentBUUID, "memory:health", day3)

	// Denied user: 2 memories that MUST be excluded by the consent filter.
	insertMem("agg-user-denied", agentAUUID, "memory:context", day2)
	insertMem("agg-user-denied", agentBUUID, "memory:identity", day3)

	// Opted-out user: 1 memory that MUST also be excluded.
	insertMem("agg-user-opted-out", agentAUUID, "memory:preferences", day3)

	// One institutional row (no user, no agent) — counted for category/day,
	// skipped for agent.
	insertMem("", "", "memory:context", day1)

	return aggregateTestWorkspace
}

func TestAggregate_GroupByCategory_Count(t *testing.T) {
	store := newStore(t)
	workspace := seedAggregateFixtures(t, store)

	rows, err := store.Aggregate(context.Background(), AggregateOptions{
		Workspace: workspace,
		GroupBy:   AggregateGroupByCategory,
		Metric:    AggregateMetricCount,
		Limit:     100,
	})
	require.NoError(t, err)

	got := map[string]int64{}
	for _, r := range rows {
		got[r.Key] = r.Value
	}

	want := map[string]int64{
		"memory:context": 3, // 2 granted + 1 institutional
		"memory:health":  1, // 1 granted
	}
	require.Equal(t, want, got)
}

func TestAggregate_GroupByAgent_SkipsInstitutional(t *testing.T) {
	store := newStore(t)
	workspace := seedAggregateFixtures(t, store)

	rows, err := store.Aggregate(context.Background(), AggregateOptions{
		Workspace: workspace,
		GroupBy:   AggregateGroupByAgent,
		Metric:    AggregateMetricCount,
		Limit:     100,
	})
	require.NoError(t, err)

	got := map[string]int64{}
	for _, r := range rows {
		got[r.Key] = r.Value
	}
	want := map[string]int64{
		agentAUUID: 2, // granted user's 2 rows
		agentBUUID: 1, // granted user's 1 row
	}
	require.Equal(t, want, got)
}

func TestAggregate_GroupByDay_OrderedAscending(t *testing.T) {
	store := newStore(t)
	workspace := seedAggregateFixtures(t, store)

	rows, err := store.Aggregate(context.Background(), AggregateOptions{
		Workspace: workspace,
		GroupBy:   AggregateGroupByDay,
		Metric:    AggregateMetricCount,
		Limit:     100,
	})
	require.NoError(t, err)
	require.NotEmpty(t, rows)

	for i := 1; i < len(rows); i++ {
		require.LessOrEqual(t, rows[i-1].Key, rows[i].Key,
			"days must be ordered ascending; got %s before %s", rows[i-1].Key, rows[i].Key)
	}

	var total int64
	for _, r := range rows {
		total += r.Value
	}
	require.Equal(t, int64(4), total)
}

func TestAggregate_DistinctUsers_DiffersFromCount(t *testing.T) {
	store := newStore(t)
	workspace := seedAggregateFixtures(t, store)

	rows, err := store.Aggregate(context.Background(), AggregateOptions{
		Workspace: workspace,
		GroupBy:   AggregateGroupByCategory,
		Metric:    AggregateMetricDistinctUsers,
		Limit:     100,
	})
	require.NoError(t, err)

	got := map[string]int64{}
	gotCounts := map[string]int64{}
	for _, r := range rows {
		got[r.Key] = r.Value
		gotCounts[r.Key] = r.Count
	}

	require.Equal(t, int64(1), got["memory:context"], "distinct users for context")
	require.Equal(t, int64(3), gotCounts["memory:context"], "row count for context")
	require.Equal(t, int64(1), got["memory:health"], "distinct users for health")
	require.Equal(t, int64(1), gotCounts["memory:health"], "row count for health")
}

func TestAggregate_TimeBounds_FromExcludesEarlier(t *testing.T) {
	store := newStore(t)
	workspace := seedAggregateFixtures(t, store)

	from := time.Now().UTC().Add(-36 * time.Hour)

	rows, err := store.Aggregate(context.Background(), AggregateOptions{
		Workspace: workspace,
		GroupBy:   AggregateGroupByCategory,
		Metric:    AggregateMetricCount,
		From:      &from,
		Limit:     100,
	})
	require.NoError(t, err)

	got := map[string]int64{}
	for _, r := range rows {
		got[r.Key] = r.Value
	}
	require.Equal(t, map[string]int64{"memory:health": 1}, got)
}

func TestAggregate_LimitClamping(t *testing.T) {
	store := newStore(t)
	workspace := seedAggregateFixtures(t, store)

	rows, err := store.Aggregate(context.Background(), AggregateOptions{
		Workspace: workspace,
		GroupBy:   AggregateGroupByCategory,
		Metric:    AggregateMetricCount,
		Limit:     1,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "memory:context", rows[0].Key)
	require.Equal(t, int64(3), rows[0].Value)
}

func TestAggregate_MissingWorkspace_Errors(t *testing.T) {
	store := newStore(t)
	_, err := store.Aggregate(context.Background(), AggregateOptions{
		GroupBy: AggregateGroupByCategory,
		Metric:  AggregateMetricCount,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "workspace is required")
}

func TestAggregate_InvalidGroupBy_Errors(t *testing.T) {
	store := newStore(t)
	_, err := store.Aggregate(context.Background(), AggregateOptions{
		Workspace: aggregateTestWorkspace,
		GroupBy:   "banana",
		Metric:    AggregateMetricCount,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid groupBy")
}

func TestAggregate_InvalidMetric_Errors(t *testing.T) {
	store := newStore(t)
	_, err := store.Aggregate(context.Background(), AggregateOptions{
		Workspace: aggregateTestWorkspace,
		GroupBy:   AggregateGroupByCategory,
		Metric:    "banana",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid metric")
}

func TestAggregate_LimitDefaults_WhenZero(t *testing.T) {
	store := newStore(t)
	workspace := seedAggregateFixtures(t, store)
	rows, err := store.Aggregate(context.Background(), AggregateOptions{
		Workspace: workspace,
		GroupBy:   AggregateGroupByCategory,
		Metric:    AggregateMetricCount,
		// Limit unset → defaults to DefaultAggregateLimit
	})
	require.NoError(t, err)
	require.NotEmpty(t, rows)
}

func TestAggregate_LimitClamped_WhenAboveMax(t *testing.T) {
	store := newStore(t)
	workspace := seedAggregateFixtures(t, store)
	rows, err := store.Aggregate(context.Background(), AggregateOptions{
		Workspace: workspace,
		GroupBy:   AggregateGroupByCategory,
		Metric:    AggregateMetricCount,
		Limit:     99999,
	})
	require.NoError(t, err)
	require.NotEmpty(t, rows)
}

func TestAggregate_DefaultMetric_TreatedAsCount(t *testing.T) {
	store := newStore(t)
	workspace := seedAggregateFixtures(t, store)
	rows, err := store.Aggregate(context.Background(), AggregateOptions{
		Workspace: workspace,
		GroupBy:   AggregateGroupByCategory,
		Metric:    "", // empty → COUNT(*)
		Limit:     100,
	})
	require.NoError(t, err)
	require.NotEmpty(t, rows)
}

func TestAggregate_EmptyWorkspace_ReturnsEmpty(t *testing.T) {
	store := newStore(t)
	rows, err := store.Aggregate(context.Background(), AggregateOptions{
		Workspace: "00000000-0000-0000-0000-deadbeef0000",
		GroupBy:   AggregateGroupByCategory,
		Metric:    AggregateMetricCount,
		Limit:     100,
	})
	require.NoError(t, err)
	require.Empty(t, rows)
}
