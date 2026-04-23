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

// StructuredLookup is a filter-based query that returns every memory matching
// the combination of (workspace, kind, name-prefix, purpose). Unlike vector
// or ILIKE search, structured lookup is exact — used when the retriever
// already knows what slot to fill (e.g. "load the API style guide").
type StructuredLookup struct {
	WorkspaceID string
	UserID      string // empty = only institutional/agent rows
	AgentID     string // empty = only non-agent rows
	Kinds       []string
	NamePrefix  string
	Purpose     string
	Limit       int
}

// LookupStructured returns memories matching the given structured filter.
// Results are ordered by observed_at DESC and capped by Limit (default 15).
func (s *PostgresMemoryStore) LookupStructured(ctx context.Context, q StructuredLookup) ([]*Memory, error) {
	if q.WorkspaceID == "" {
		return nil, errors.New(errWorkspaceRequired)
	}

	limit := q.Limit
	if limit <= 0 {
		limit = defaultMultiTierLimit
	}

	args := []any{q.WorkspaceID}
	where := []string{"e.workspace_id=$1", "e.forgotten = false"}

	where, args = appendOptionalScope(where, args, q.UserID, q.AgentID)
	where, args = appendKindFilter(where, args, q.Kinds)

	if q.NamePrefix != "" {
		args = append(args, q.NamePrefix+"%")
		where = append(where, fmt.Sprintf("e.name ILIKE $%d", len(args)))
	}
	if q.Purpose != "" {
		args = append(args, q.Purpose)
		where = append(where, fmt.Sprintf("e.purpose=$%d", len(args)))
	}

	args = append(args, limit)
	sql := fmt.Sprintf(`
		SELECT DISTINCT ON (e.id)
		  e.id, e.kind, e.metadata, e.created_at, e.expires_at,
		  o.content, o.confidence, o.session_id, o.turn_range, o.observed_at, o.accessed_at
		FROM memory_entities e
		JOIN memory_observations o ON o.entity_id = e.id AND o.superseded_by IS NULL
		WHERE %s
		ORDER BY e.id, o.observed_at DESC
		LIMIT $%d`, joinAnd(where), len(args))

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: structured lookup: %w", err)
	}
	defer rows.Close()

	scopeForScan := map[string]string{ScopeWorkspaceID: q.WorkspaceID}
	if q.UserID != "" {
		scopeForScan[ScopeUserID] = q.UserID
	}
	if q.AgentID != "" {
		scopeForScan[ScopeAgentID] = q.AgentID
	}
	return scanMemories(rows, scopeForScan)
}

// appendOptionalScope adds user/agent filters when the respective scope key
// is set. Both empty means "no scope filter" — useful when the caller wants
// to pull across the whole workspace. If UserID is empty but AgentID is set,
// restrict to agent-tier rows (virtual_user_id IS NULL).
func appendOptionalScope(where []string, args []any, userID, agentID string) ([]string, []any) {
	if userID != "" {
		args = append(args, userID)
		where = append(where, fmt.Sprintf("(e.virtual_user_id IS NULL OR e.virtual_user_id=$%d)", len(args)))
	}
	if agentID != "" {
		args = append(args, agentID)
		where = append(where, fmt.Sprintf("(e.agent_id IS NULL OR e.agent_id=$%d)", len(args)))
	}
	return where, args
}

// appendKindFilter adds a kind = X or kind = ANY($N) filter.
func appendKindFilter(where []string, args []any, kinds []string) ([]string, []any) {
	switch len(kinds) {
	case 0:
		return where, args
	case 1:
		args = append(args, kinds[0])
		return append(where, fmt.Sprintf("e.kind=$%d", len(args))), args
	default:
		args = append(args, kinds)
		return append(where, fmt.Sprintf("e.kind = ANY($%d)", len(args))), args
	}
}
