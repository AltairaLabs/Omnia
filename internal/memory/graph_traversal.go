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
	"fmt"
)

// Default depth limits for graph traversal.
const (
	defaultGraphMaxHops = 2
	maxGraphMaxHops     = 5 // hard cap to prevent runaway recursion
)

// GraphTraversal describes a breadth-first walk starting from one or more
// seed entity IDs, following edges in memory_relations up to MaxHops deep.
// RelationTypes narrows the edge types followed; empty slice = all.
type GraphTraversal struct {
	WorkspaceID   string
	SeedIDs       []string
	RelationTypes []string
	MaxHops       int
	Limit         int
}

// TraverseRelations walks the relation graph starting from the seed entities
// and returns the unique entities discovered (excluding the seeds themselves).
// Results are ordered by hop distance ascending, then by observed_at DESC.
// The walk is strictly workspace-scoped — cross-workspace edges are ignored
// at the recursive step.
func (s *PostgresMemoryStore) TraverseRelations(ctx context.Context, g GraphTraversal) ([]*Memory, error) {
	if g.WorkspaceID == "" {
		return nil, errors.New(errWorkspaceRequired)
	}
	if len(g.SeedIDs) == 0 {
		return []*Memory{}, nil
	}

	maxHops := g.MaxHops
	if maxHops <= 0 {
		maxHops = defaultGraphMaxHops
	}
	if maxHops > maxGraphMaxHops {
		maxHops = maxGraphMaxHops
	}
	limit := g.Limit
	if limit <= 0 {
		limit = defaultMultiTierLimit
	}

	// $1 = workspace_id
	// $2 = seed IDs (UUID[])
	// $3 = max_hops
	// $4 = limit
	// $5 = relation_types (TEXT[] or NULL — handled by the NULL-or-match clause)
	args := []any{g.WorkspaceID, g.SeedIDs, maxHops, limit}
	relArg := any(nil)
	if len(g.RelationTypes) > 0 {
		relArg = g.RelationTypes
	}
	args = append(args, relArg)

	const sql = `
WITH RECURSIVE walk AS (
	-- Seed: hop 0
	SELECT id, 0 AS hop
	FROM memory_entities
	WHERE workspace_id = $1
	  AND id = ANY($2)
	  AND forgotten = false

	UNION

	-- Step: follow edges to undiscovered entities, bounded by max_hops.
	SELECT next.id, walk.hop + 1 AS hop
	FROM walk
	JOIN memory_relations r ON r.workspace_id = $1
		AND (r.source_entity_id = walk.id OR r.target_entity_id = walk.id)
		AND ($5::text[] IS NULL OR r.relation_type = ANY($5::text[]))
	JOIN memory_entities next ON next.id = CASE
		WHEN r.source_entity_id = walk.id THEN r.target_entity_id
		ELSE r.source_entity_id
	END
		AND next.workspace_id = $1
		AND next.forgotten = false
	WHERE walk.hop < $3
)
SELECT DISTINCT ON (e.id)
  e.id, e.kind, e.metadata, e.created_at, e.expires_at,
  o.content, o.confidence, o.session_id, o.turn_range, o.observed_at, o.accessed_at
FROM walk w
JOIN memory_entities e ON e.id = w.id
JOIN memory_observations o ON o.entity_id = e.id AND o.superseded_by IS NULL
WHERE w.hop > 0
ORDER BY e.id, o.observed_at DESC
LIMIT $4`

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: graph traversal: %w", err)
	}
	defer rows.Close()

	return scanMemories(rows, map[string]string{ScopeWorkspaceID: g.WorkspaceID})
}
