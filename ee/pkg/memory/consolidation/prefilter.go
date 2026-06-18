/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package consolidation

import (
	"fmt"
	"time"
)

// PreFilterOptions configures all three pre-filter query builders.
// Not every field is used by every builder.
type PreFilterOptions struct {
	WorkspaceID       string
	OlderThan         time.Time
	MinGroupSize      int
	MinDistinctUsers  int
	SimilarityFloor   float32
	MaxBucketsPerPass int
	MaxPerBucket      int
}

// BuildStaleObservationsQuery returns the SQL + args for the stale-
// observations pre-filter. Selects per-row (no GROUP BY) so the
// adapter can decode content + mutability + observed_at + source_type
// onto each BucketEntry — packs need the content text to summarize.
// Bucketing by (workspace, user, agent, kind, name) + MaxPerBucket
// cap is applied in Go in the adapter. Excludes rows with mutability
// != 'mutable' or source_type = 'regulated' from the candidate
// target set.
func BuildStaleObservationsQuery(o PreFilterOptions) (string, []any) {
	const q = `
SELECT o.id, e.workspace_id, e.virtual_user_id, e.agent_id,
       e.kind, e.name, o.content, o.observed_at,
       o.mutability, o.source_type
FROM memory_observations o
JOIN memory_entities e ON e.id = o.entity_id
WHERE e.workspace_id = $1
  AND o.observed_at < $2
  AND o.mutability = 'mutable'
  AND o.source_type != 'regulated'
  AND o.superseded_by IS NULL
ORDER BY e.workspace_id, e.virtual_user_id, e.agent_id, e.kind, e.name, o.observed_at
LIMIT $3;
`
	// Outer LIMIT bounds the candidate pull: at most MaxBucketsPerPass
	// distinct groups, each with up to MaxPerBucket entries.
	maxPer := o.MaxPerBucket
	if maxPer <= 0 {
		maxPer = 10
	}
	args := []any{
		o.WorkspaceID,
		o.OlderThan,
		o.MaxBucketsPerPass * maxPer,
	}
	return q, args
}

// BuildCrossScopeCandidatesQuery returns SQL + args for cross-scope
// promotion candidates: observations sharing (kind, name) across
// ≥ MinDistinctUsers distinct users within the workspace.
//
// Returns per-row (no SQL GROUP BY in the outer projection) so the
// adapter can surface content + mutability + source_type + observed_at
// onto each BucketEntry. Groups that fail the k-anonymity threshold
// are filtered via an inner aggregate subquery so we don't drag every
// kind/name across the wire. Adapter groups in Go and applies
// MaxPerBucket.
func BuildCrossScopeCandidatesQuery(o PreFilterOptions) (string, []any) {
	const q = `
WITH eligible AS (
    SELECT o.id, e.workspace_id, e.virtual_user_id, e.agent_id, e.kind, e.name,
           o.content, o.observed_at, o.mutability, o.source_type
    FROM memory_observations o
    JOIN memory_entities e ON e.id = o.entity_id
    WHERE e.workspace_id = $1
      AND o.mutability = 'mutable'
      AND o.source_type != 'regulated'
      AND o.superseded_by IS NULL
      AND e.virtual_user_id IS NOT NULL
), qualifying AS (
    SELECT kind, name
    FROM eligible
    GROUP BY kind, name
    HAVING COUNT(DISTINCT virtual_user_id) >= $2
)
SELECT e.id, e.workspace_id, e.virtual_user_id, e.agent_id, e.kind, e.name,
       e.content, e.observed_at, e.mutability, e.source_type
FROM eligible e
JOIN qualifying q ON q.kind = e.kind AND q.name = e.name
ORDER BY e.kind, e.name, e.observed_at DESC
LIMIT $3;
`
	// Outer LIMIT bounds the candidate pull: at most MaxBucketsPerPass
	// distinct (kind, name) groups, each with up to MaxPerBucket entries.
	maxPer := o.MaxPerBucket
	if maxPer <= 0 {
		maxPer = 10
	}
	args := []any{
		o.WorkspaceID,
		o.MinDistinctUsers,
		o.MaxBucketsPerPass * maxPer,
	}
	return q, args
}

// BuildEntityDuplicateCandidatesQuery returns SQL + args for entity
// duplicate-pair candidates: entities with similar (embedding) names
// within a workspace.
func BuildEntityDuplicateCandidatesQuery(o PreFilterOptions) (string, []any) {
	const q = `
SELECT e1.id AS canonical_id, e2.id AS dupe_id,
       e1.name AS canonical_name, e2.name AS dupe_name,
       e1.kind, e1.workspace_id,
       (1.0 - (e1.embedding <=> e2.embedding)) AS similarity
FROM memory_entities e1
JOIN memory_entities e2
  ON e1.workspace_id = e2.workspace_id
 AND e1.kind = e2.kind
 AND e1.id < e2.id
WHERE e1.workspace_id = $1
  AND e1.mutability = 'mutable'
  AND e2.mutability = 'mutable'
  AND e1.embedding IS NOT NULL
  AND e2.embedding IS NOT NULL
  AND (1.0 - (e1.embedding <=> e2.embedding)) >= $2
ORDER BY similarity DESC
LIMIT $3;
`
	args := []any{
		o.WorkspaceID,
		o.SimilarityFloor,
		o.MaxBucketsPerPass,
	}
	return q, args
}

// ValidatePreFilterOptions returns a descriptive error if required
// fields are missing for the given axis. Pure validation; safe to call
// before any DB query.
func ValidatePreFilterOptions(axis PreFilterAxis, o PreFilterOptions) error {
	if o.WorkspaceID == "" {
		return fmt.Errorf("preFilter %s: WorkspaceID required", axis)
	}
	switch axis {
	case AxisStaleObservations:
		if o.OlderThan.IsZero() || o.MinGroupSize <= 0 {
			return fmt.Errorf("preFilter %s: OlderThan + MinGroupSize required", axis)
		}
	case AxisCrossScopeCandidates:
		if o.MinDistinctUsers <= 0 {
			return fmt.Errorf("preFilter %s: MinDistinctUsers required", axis)
		}
	case AxisEntityDuplicateCandidates:
		if o.SimilarityFloor <= 0 {
			return fmt.Errorf("preFilter %s: SimilarityFloor required", axis)
		}
	}
	if o.MaxBucketsPerPass <= 0 {
		return fmt.Errorf("preFilter %s: MaxBucketsPerPass must be > 0", axis)
	}
	return nil
}
