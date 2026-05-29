/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConsolidationRunStore implements consolidation.RunTracker over pgxpool,
// persisting per-(policy, workspace, axis) last-run timestamps in the
// consolidation_runs table (migration 000012). Durable tracking lets the
// worker honour per-axis cron schedules across pod restarts.
type ConsolidationRunStore struct {
	pool *pgxpool.Pool
}

// NewConsolidationRunStore constructs a ConsolidationRunStore around the
// provided pgxpool.
func NewConsolidationRunStore(pool *pgxpool.Pool) *ConsolidationRunStore {
	return &ConsolidationRunStore{pool: pool}
}

// LastRun returns the last-run timestamp for the tuple. The bool is false
// when no row exists yet (first sighting of this policy/workspace/axis).
func (s *ConsolidationRunStore) LastRun(ctx context.Context, policyName, workspaceID, axis string) (time.Time, bool, error) {
	const q = `
SELECT last_ran_at FROM consolidation_runs
WHERE policy_name = $1 AND workspace_id = $2 AND axis = $3;
`
	var t time.Time
	err := s.pool.QueryRow(ctx, q, policyName, workspaceID, axis).Scan(&t)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("LastRun: %w", err)
	}
	return t, true, nil
}

// MarkRun upserts the last-run timestamp for the tuple.
func (s *ConsolidationRunStore) MarkRun(ctx context.Context, policyName, workspaceID, axis string, at time.Time) error {
	const q = `
INSERT INTO consolidation_runs (policy_name, workspace_id, axis, last_ran_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (policy_name, workspace_id, axis)
DO UPDATE SET last_ran_at = EXCLUDED.last_ran_at;
`
	if _, err := s.pool.Exec(ctx, q, policyName, workspaceID, axis, at); err != nil {
		return fmt.Errorf("MarkRun: %w", err)
	}
	return nil
}
