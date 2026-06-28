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
	"errors"
	"testing"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
	"github.com/stretchr/testify/require"
)

// fakeGrantorSource is a test double for consentGrantorSource.
type fakeGrantorSource struct {
	ids []string
	err error
}

func (f *fakeGrantorSource) ListConsentUsers(
	_ context.Context, _ privacy.ConsentCategory, _ bool,
) ([]string, error) {
	return f.ids, f.err
}

const consentTestWorkspace = "b0000000-0000-0000-0000-000000001007"

// seedConsentEntities inserts memory_entities for the consent-filter tests:
//   - one institutional row (virtual_user_id IS NULL) — always counted
//   - one row for grantingUser                        — counted when in grantor set
//   - one row for nonGrantingUser                     — excluded when absent from grantor set
//
// No user_privacy_preferences rows are inserted — the filter now reads
// from privacy-api (consentGrantorSource), not the local table.
func seedConsentEntities(t *testing.T, store *PostgresMemoryStore) {
	t.Helper()
	ctx := context.Background()
	pool := store.Pool()

	// Institutional row: virtual_user_id NULL.
	_, err := pool.Exec(ctx, `
		INSERT INTO memory_entities (workspace_id, virtual_user_id, agent_id, name, kind, metadata)
		VALUES ($1, NULL, NULL, $2, $3, '{}'::jsonb)`,
		consentTestWorkspace, "institutional-fact", "fact",
	)
	require.NoError(t, err)

	// Granting user's memory (via Save so virtual_user_id is populated correctly).
	for _, userID := range []string{"user-granted", "user-not-granted"} {
		mem := &Memory{
			Type:    "fact",
			Content: "sample",
			Scope:   map[string]string{ScopeWorkspaceID: consentTestWorkspace, ScopeUserID: userID},
		}
		require.NoError(t, store.Save(ctx, mem))
	}
}

// TestAggregate_ConsentFilter_FiltersUsersByGrant verifies the full
// include/exclude contract that the old LEFT JOIN produced:
//   - institutional rows (virtual_user_id IS NULL) are always counted
//   - user rows for IDs in the grantor set are counted
//   - user rows for IDs absent from the grantor set are excluded
func TestAggregate_ConsentFilter_FiltersUsersByGrant(t *testing.T) {
	store := newStore(t)
	seedConsentEntities(t, store)

	store.SetConsentGrantorSource(&fakeGrantorSource{ids: []string{"user-granted"}})

	rows, err := store.Aggregate(context.Background(), AggregateOptions{
		Workspace: consentTestWorkspace,
		GroupBy:   AggregateGroupByCategory,
		Metric:    AggregateMetricCount,
		Limit:     100,
	})
	require.NoError(t, err)

	var total int64
	for _, r := range rows {
		total += r.Value
	}
	// Expected: user-granted (1) + institutional (1) = 2.
	// Excluded: user-not-granted.
	require.Equal(t, int64(2), total,
		"count = %d, want 2 (one granted user + one institutional row)", total)
}

// TestAggregate_ConsentFilter_EmptyGrantorSet verifies that when the grantor
// set is empty, only institutional rows (virtual_user_id IS NULL) are counted.
func TestAggregate_ConsentFilter_EmptyGrantorSet(t *testing.T) {
	store := newStore(t)
	seedConsentEntities(t, store)

	store.SetConsentGrantorSource(&fakeGrantorSource{ids: []string{}})

	rows, err := store.Aggregate(context.Background(), AggregateOptions{
		Workspace: consentTestWorkspace,
		GroupBy:   AggregateGroupByCategory,
		Metric:    AggregateMetricCount,
		Limit:     100,
	})
	require.NoError(t, err)

	var total int64
	for _, r := range rows {
		total += r.Value
	}
	// Only the institutional row (virtual_user_id IS NULL) is counted.
	require.Equal(t, int64(1), total,
		"empty grantor set should yield only institutional rows, got %d", total)
}

// TestAggregate_ConsentFilter_NilSource verifies that when no
// consentGrantorSource is configured, Aggregate uses the conservative
// default: count only institutional rows (virtual_user_id IS NULL).
func TestAggregate_ConsentFilter_NilSource(t *testing.T) {
	store := newStore(t)
	seedConsentEntities(t, store)
	// store.grantorSource is nil by default — no SetConsentGrantorSource call.

	rows, err := store.Aggregate(context.Background(), AggregateOptions{
		Workspace: consentTestWorkspace,
		GroupBy:   AggregateGroupByCategory,
		Metric:    AggregateMetricCount,
		Limit:     100,
	})
	require.NoError(t, err)

	var total int64
	for _, r := range rows {
		total += r.Value
	}
	// Only the institutional row is counted.
	require.Equal(t, int64(1), total,
		"nil grantor source should yield only institutional rows, got %d", total)
}

// TestAggregate_ConsentFilter_SourceError verifies that a grantor-source
// error is propagated as an Aggregate error (fail-closed).
func TestAggregate_ConsentFilter_SourceError(t *testing.T) {
	store := newStore(t)
	seedConsentEntities(t, store)

	store.SetConsentGrantorSource(&fakeGrantorSource{
		err: errors.New("privacy-api unreachable"),
	})

	_, err := store.Aggregate(context.Background(), AggregateOptions{
		Workspace: consentTestWorkspace,
		GroupBy:   AggregateGroupByCategory,
		Metric:    AggregateMetricCount,
		Limit:     100,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "grantor lookup")
}
