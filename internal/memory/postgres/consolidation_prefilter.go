/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"fmt"
	"time"

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

// RunStaleObservations executes BuildStaleObservationsQuery and decodes
// content + mutability + source_type + observed_at onto each entry, then
// groups in Go by (user, agent, kind, name) and applies MaxPerBucket +
// MinGroupSize caps. Packs need the content text to summarize — empty
// content (the v1 bug) leaves packs with nothing to reason about.
func (r *PreFilterRunner) RunStaleObservations(ctx context.Context, opts consolidation.PreFilterOptions) ([]consolidation.Bucket, error) {
	q, args := consolidation.BuildStaleObservationsQuery(opts)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("stale obs query: %w", err)
	}
	defer rows.Close()

	type bucketKey struct{ user, agent, kind, name string }
	groups := map[bucketKey][]consolidation.BucketEntry{}
	for rows.Next() {
		var (
			obsID                       string
			workspaceID                 string
			userID, agentID, kind, name *string
			content                     string
			observedAt                  time.Time
			mutability, sourceType      string
		)
		if err := rows.Scan(
			&obsID, &workspaceID, &userID, &agentID, &kind, &name,
			&content, &observedAt, &mutability, &sourceType,
		); err != nil {
			return nil, fmt.Errorf("stale obs scan: %w", err)
		}
		key := bucketKey{strOrEmpty(userID), strOrEmpty(agentID), strOrEmpty(kind), strOrEmpty(name)}
		groups[key] = append(groups[key], consolidation.BucketEntry{
			ID:      obsID,
			Content: content,
			Scope: consolidation.Scope{
				WorkspaceID: workspaceID,
				AgentID:     strOrEmpty(agentID),
				UserID:      strOrEmpty(userID),
			},
			Mutability: mutability,
			SourceType: sourceType,
			ObservedAt: observedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]consolidation.Bucket, 0, len(groups))
	for k, entries := range groups {
		if opts.MinGroupSize > 0 && len(entries) < opts.MinGroupSize {
			continue
		}
		if opts.MaxPerBucket > 0 && len(entries) > opts.MaxPerBucket {
			entries = entries[:opts.MaxPerBucket]
		}
		out = append(out, consolidation.Bucket{
			Key:     fmt.Sprintf("kind=%s;name=%s", k.kind, k.name),
			Entries: entries,
			Stats:   map[string]any{"count": len(entries)},
		})
	}
	return out, nil
}

// RunCrossScopeCandidates executes BuildCrossScopeCandidatesQuery and
// decodes content + mutability + source_type + observed_at onto each
// entry, then groups in Go by (kind, name) and applies MaxPerBucket.
// Packs handling cross-scope rescope need observation content to
// reason about prevalence (and to avoid promoting PII-containing
// observations); the v1 adapter dropped all of that.
func (r *PreFilterRunner) RunCrossScopeCandidates(ctx context.Context, opts consolidation.PreFilterOptions) ([]consolidation.Bucket, error) {
	q, args := consolidation.BuildCrossScopeCandidatesQuery(opts)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("cross-scope query: %w", err)
	}
	defer rows.Close()

	type bucketKey struct{ kind, name string }
	type bucketData struct {
		entries       []consolidation.BucketEntry
		distinctUsers map[string]struct{}
	}
	groups := map[bucketKey]*bucketData{}
	for rows.Next() {
		var (
			obsID                       string
			workspaceID                 string
			userID, agentID, kind, name *string
			content                     string
			observedAt                  time.Time
			mutability, sourceType      string
		)
		if err := rows.Scan(
			&obsID, &workspaceID, &userID, &agentID, &kind, &name,
			&content, &observedAt, &mutability, &sourceType,
		); err != nil {
			return nil, fmt.Errorf("cross-scope scan: %w", err)
		}
		key := bucketKey{strOrEmpty(kind), strOrEmpty(name)}
		bd, ok := groups[key]
		if !ok {
			bd = &bucketData{distinctUsers: map[string]struct{}{}}
			groups[key] = bd
		}
		bd.entries = append(bd.entries, consolidation.BucketEntry{
			ID:      obsID,
			Content: content,
			Scope: consolidation.Scope{
				WorkspaceID: workspaceID,
				AgentID:     strOrEmpty(agentID),
				UserID:      strOrEmpty(userID),
			},
			Mutability: mutability,
			SourceType: sourceType,
			ObservedAt: observedAt,
		})
		if u := strOrEmpty(userID); u != "" {
			bd.distinctUsers[u] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]consolidation.Bucket, 0, len(groups))
	for k, bd := range groups {
		entries := bd.entries
		totalObs := len(entries)
		if opts.MaxPerBucket > 0 && len(entries) > opts.MaxPerBucket {
			entries = entries[:opts.MaxPerBucket]
		}
		out = append(out, consolidation.Bucket{
			Key:     fmt.Sprintf("kind=%s;name=%s", k.kind, k.name),
			Entries: entries,
			Stats: map[string]any{
				"distinctUsers": len(bd.distinctUsers),
				"totalObs":      totalObs,
			},
		})
	}
	return out, nil
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
