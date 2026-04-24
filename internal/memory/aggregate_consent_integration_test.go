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

	"github.com/stretchr/testify/require"
)

// TestAggregateConsentJoin_FiltersUsersByGrant seeds memory_entities
// with mixed user/tier rows plus user_privacy_preferences exercising
// every branch of the helper's decision table, then runs a sample
// COUNT(*) via the helper and verifies the expected row count.
//
// This is the contract test #1004 relies on: institutional rows always
// count, user rows count only when the user has granted
// analytics:aggregate.
func TestAggregateConsentJoin_FiltersUsersByGrant(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	workspaceID := "b0000000-0000-0000-0000-000000001007"

	// Seed user-tier memories via the public Save API (which requires
	// user_id scope). Institutional rows go in via direct SQL because
	// Save() rejects nil user_id.
	for _, userID := range []string{"user-granted", "user-not-granted", "user-no-prefs", "user-opted-out"} {
		mem := &Memory{
			Type:    "fact",
			Content: "sample",
			Scope:   map[string]string{ScopeWorkspaceID: workspaceID, ScopeUserID: userID},
		}
		require.NoError(t, store.Save(ctx, mem))
	}

	pool := store.Pool()
	// Insert one institutional row directly (virtual_user_id NULL,
	// agent_id NULL) — Save() can't produce these without user_id.
	_, err := pool.Exec(ctx, `
		INSERT INTO memory_entities (workspace_id, virtual_user_id, agent_id, name, kind, metadata)
		VALUES ($1, NULL, NULL, $2, $3, '{}'::jsonb)`,
		workspaceID, "institutional-fact", "fact",
	)
	require.NoError(t, err)

	// Seed preferences via direct pool access — the privacy store API
	// lives in EE; we don't want to import it from core tests.
	_, err = pool.Exec(ctx, `
		INSERT INTO user_privacy_preferences (user_id, consent_grants)
		VALUES ($1, $2), ($3, $4)
		ON CONFLICT (user_id) DO UPDATE SET consent_grants = EXCLUDED.consent_grants`,
		"user-granted", []string{"analytics:aggregate", "memory:preferences"},
		"user-not-granted", []string{"memory:preferences"},
	)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO user_privacy_preferences (user_id, opt_out_all)
		VALUES ($1, TRUE)
		ON CONFLICT (user_id) DO UPDATE SET opt_out_all = EXCLUDED.opt_out_all`,
		"user-opted-out",
	)
	require.NoError(t, err)

	// Run the sample aggregate query via the helper.
	join, where := AggregateConsentJoin("e")
	var count int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM memory_entities e `+join+
		` WHERE e.workspace_id = $1 AND e.forgotten = false AND `+where,
		workspaceID).Scan(&count)
	require.NoError(t, err)

	// Expected: user-granted (✓) + institutional (✓) = 2.
	// Excluded: user-not-granted, user-no-prefs, user-opted-out.
	if count != 2 {
		t.Errorf("count = %d, want 2 (one granted user + one institutional row); SQL: SELECT COUNT(*) FROM memory_entities e %s WHERE e.workspace_id = $1 AND e.forgotten = false AND %s", count, join, where)
	}
}
