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

	"github.com/altairalabs/omnia/internal/pgutil"
)

// Delete performs a soft delete by setting forgotten = true on the entity.
func (s *PostgresMemoryStore) Delete(ctx context.Context, scope map[string]string, memoryID string) error {
	if scope[ScopeWorkspaceID] == "" {
		return errors.New(errWorkspaceRequired)
	}

	// Scope guard (#1268, SEC-3): a workspace-only check would let any caller in
	// workspace W forget any user's memory in W by its UUID. Anchor the user
	// dimension — a missing user_id means NULL (IS NOT DISTINCT FROM), so an
	// empty user_id can only forget institutional/agent rows, never another
	// user's. The agent dimension stays permissive (a user-scoped DSAR forget
	// must still reach that user's user_for_agent rows). Mirrors GetMemory.
	tag, err := s.pool.Exec(ctx, `
		UPDATE memory_entities SET forgotten = true, updated_at = now()
		WHERE id = $1 AND workspace_id = $2
		  AND virtual_user_id IS NOT DISTINCT FROM $3::text
		  AND ($4::uuid IS NULL OR agent_id = $4)`,
		memoryID,
		scope[ScopeWorkspaceID],
		scopeOrNil(scope, ScopeVirtualUserID),
		scopeOrNil(scope, ScopeAgentID))
	if err != nil {
		return fmt.Errorf("memory: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("memory: entity %s not found: %w", memoryID, ErrNotFound)
	}
	return nil
}

// DeleteAll hard-deletes all entities (and cascading observations/relations) for the scope.
func (s *PostgresMemoryStore) DeleteAll(ctx context.Context, scope map[string]string) error {
	if scope[ScopeWorkspaceID] == "" {
		return errors.New(errWorkspaceRequired)
	}

	sql, qb := buildDeleteAllQuery(scope)

	_, err := s.pool.Exec(ctx, sql, qb.Args()...)
	if err != nil {
		return fmt.Errorf("memory: delete all: %w", err)
	}
	return nil
}

// buildDeleteAllQuery constructs the SQL and arguments for a DeleteAll call.
func buildDeleteAllQuery(scope map[string]string) (string, *pgutil.QueryBuilder) {
	var qb pgutil.QueryBuilder
	qb.Add(colWorkspaceID, scope[ScopeWorkspaceID])

	if uid := scope[ScopeVirtualUserID]; uid != "" {
		qb.Add(colVirtualUserID, uid)
	}

	sql := "DELETE FROM memory_entities WHERE 1=1" + qb.Where()

	return sql, &qb
}

// BatchDelete hard-deletes up to limit entities (and cascading observations/relations) for the scope.
// It returns the count of deleted rows. Use limit=500 in a loop until count=0 for DSAR cascades.
func (s *PostgresMemoryStore) BatchDelete(ctx context.Context, scope map[string]string, limit int) (int, error) {
	if scope[ScopeWorkspaceID] == "" {
		return 0, errors.New(errWorkspaceRequired)
	}

	sql, qb := buildBatchDeleteQuery(scope, limit)

	tag, err := s.pool.Exec(ctx, sql, qb.Args()...)
	if err != nil {
		return 0, fmt.Errorf("memory: batch delete: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// buildBatchDeleteQuery constructs the SQL and arguments for a BatchDelete call.
func buildBatchDeleteQuery(scope map[string]string, limit int) (string, *pgutil.QueryBuilder) {
	var qb pgutil.QueryBuilder
	qb.Add(colWorkspaceID, scope[ScopeWorkspaceID])

	if uid := scope[ScopeVirtualUserID]; uid != "" {
		qb.Add(colVirtualUserID, uid)
	}

	subquery := "SELECT id FROM memory_entities WHERE 1=1" + qb.Where()
	subquery = qb.AppendPagination(subquery, limit, 0)

	sql := "DELETE FROM memory_entities WHERE id IN (" + subquery + ")"

	return sql, &qb
}
