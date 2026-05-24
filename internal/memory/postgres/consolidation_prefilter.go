/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/altairalabs/omnia/internal/memory/consolidation"
)

// PreFilterRunner runs the SQL from consolidation/prefilter.go against
// Postgres and decodes result rows into consolidation.Bucket values.
type PreFilterRunner struct {
	pool *pgxpool.Pool
}

// NewPreFilterRunner constructs a PreFilterRunner.
func NewPreFilterRunner(pool *pgxpool.Pool) *PreFilterRunner {
	return &PreFilterRunner{pool: pool}
}

// RunStaleObservations executes BuildStaleObservationsQuery and decodes.
func (r *PreFilterRunner) RunStaleObservations(ctx context.Context, opts consolidation.PreFilterOptions) ([]consolidation.Bucket, error) {
	q, args := consolidation.BuildStaleObservationsQuery(opts)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("stale obs query: %w", err)
	}
	defer rows.Close()
	var out []consolidation.Bucket
	for rows.Next() {
		var (
			workspaceID                 string
			userID, agentID, kind, name *string
			n                           int
			obsIDs                      []string
		)
		if err := rows.Scan(&workspaceID, &userID, &agentID, &kind, &name, &n, &obsIDs); err != nil {
			return nil, fmt.Errorf("stale obs scan: %w", err)
		}
		entries := make([]consolidation.BucketEntry, 0, len(obsIDs))
		for _, id := range obsIDs {
			entries = append(entries, consolidation.BucketEntry{
				ID: id,
				Scope: consolidation.Scope{
					WorkspaceID: workspaceID,
					AgentID:     strOrEmpty(agentID),
					UserID:      strOrEmpty(userID),
				},
				Mutability: consolidation.MutabilityMutable,
			})
		}
		out = append(out, consolidation.Bucket{
			Key:     fmt.Sprintf("kind=%s;name=%s", strOrEmpty(kind), strOrEmpty(name)),
			Entries: entries,
			Stats:   map[string]any{"count": n},
		})
	}
	return out, rows.Err()
}

// RunCrossScopeCandidates executes BuildCrossScopeCandidatesQuery.
func (r *PreFilterRunner) RunCrossScopeCandidates(ctx context.Context, opts consolidation.PreFilterOptions) ([]consolidation.Bucket, error) {
	q, args := consolidation.BuildCrossScopeCandidatesQuery(opts)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("cross-scope query: %w", err)
	}
	defer rows.Close()
	var out []consolidation.Bucket
	for rows.Next() {
		var (
			workspaceID, kind, name string
			distinctUsers, totalObs int
			obsIDs                  []string
		)
		if err := rows.Scan(&workspaceID, &kind, &name, &distinctUsers, &totalObs, &obsIDs); err != nil {
			return nil, fmt.Errorf("cross-scope scan: %w", err)
		}
		entries := make([]consolidation.BucketEntry, 0, len(obsIDs))
		for _, id := range obsIDs {
			entries = append(entries, consolidation.BucketEntry{
				ID:         id,
				Scope:      consolidation.Scope{WorkspaceID: workspaceID},
				Mutability: consolidation.MutabilityMutable,
			})
		}
		out = append(out, consolidation.Bucket{
			Key:     fmt.Sprintf("kind=%s;name=%s", kind, name),
			Entries: entries,
			Stats: map[string]any{
				"distinctUsers": distinctUsers,
				"totalObs":      totalObs,
			},
		})
	}
	return out, rows.Err()
}

// RunEntityDuplicateCandidates executes BuildEntityDuplicateCandidatesQuery.
func (r *PreFilterRunner) RunEntityDuplicateCandidates(ctx context.Context, opts consolidation.PreFilterOptions) ([]consolidation.Bucket, error) {
	q, args := consolidation.BuildEntityDuplicateCandidatesQuery(opts)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("entity-dupe query: %w", err)
	}
	defer rows.Close()
	var out []consolidation.Bucket
	for rows.Next() {
		var (
			canonicalID, dupeID                 string
			canonicalName, dupeName, kind, wsID string
			similarity                          float64
		)
		if err := rows.Scan(&canonicalID, &dupeID, &canonicalName, &dupeName, &kind, &wsID, &similarity); err != nil {
			return nil, fmt.Errorf("entity-dupe scan: %w", err)
		}
		out = append(out, consolidation.Bucket{
			Key: fmt.Sprintf("dupe:%s:%s", canonicalID, dupeID),
			Entries: []consolidation.BucketEntry{
				{ID: canonicalID, Content: canonicalName, Scope: consolidation.Scope{WorkspaceID: wsID}, Mutability: consolidation.MutabilityMutable},
				{ID: dupeID, Content: dupeName, Scope: consolidation.Scope{WorkspaceID: wsID}, Mutability: consolidation.MutabilityMutable},
			},
			Stats: map[string]any{"similarity": similarity, "kind": kind},
		})
	}
	return out, rows.Err()
}

func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
