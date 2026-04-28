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

// defaultConflictsLimit caps the conflict-surfacing query so a
// runaway dedup regression doesn't dump 100k rows into the
// dashboard. Operators bump this when they really do need to scan
// the long tail.
const defaultConflictsLimit = 100

// FindConflictedEntities returns entities that hold more than one
// active observation simultaneously, ordered by active count
// descending so the worst offenders surface first. See the Store
// interface for the failure mode this signals.
//
// "Active" matches the recall path: superseded_by IS NULL AND
// (valid_until IS NULL OR valid_until > now()).
func (s *PostgresMemoryStore) FindConflictedEntities(
	ctx context.Context, workspaceID string, limit int,
) ([]ConflictedEntity, error) {
	if workspaceID == "" {
		return nil, errors.New(errWorkspaceRequired)
	}
	if limit <= 0 {
		limit = defaultConflictsLimit
	}

	rows, err := s.pool.Query(ctx, `
		SELECT e.id, e.kind,
		       coalesce(e.virtual_user_id, ''),
		       coalesce(e.agent_id::text, ''),
		       count(*) AS active_count
		FROM memory_entities e
		JOIN memory_observations o ON o.entity_id = e.id
		WHERE e.workspace_id = $1
		  AND NOT e.forgotten
		  AND o.superseded_by IS NULL
		  AND (o.valid_until IS NULL OR o.valid_until > now())
		GROUP BY e.id, e.kind, e.virtual_user_id, e.agent_id
		HAVING count(*) > 1
		ORDER BY active_count DESC, e.id
		LIMIT $2`,
		workspaceID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: find conflicts: %w", err)
	}
	defer rows.Close()

	var out []ConflictedEntity
	for rows.Next() {
		var c ConflictedEntity
		if err := rows.Scan(&c.EntityID, &c.Kind, &c.UserID, &c.AgentID, &c.ActiveCount); err != nil {
			return nil, fmt.Errorf("memory: scan conflict: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
