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

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
	"github.com/jackc/pgx/v5"
)

// agentScopedTrustModel and agentScopedSourceType mirror the institutional
// path — rows written here are operator-curated by definition and must be
// visibly distinct from conversation-extracted observations.
const (
	agentScopedTrustModel = "curated"
	agentScopedSourceType = "operator_curated"
)

// errAgentIDRequired is returned when an agent-scoped admin operation is
// called without an agent_id.
const errAgentIDRequired = "memory: agent_id is required for agent-scoped admin operations"

// ErrNotAgentScoped is returned by DeleteAgentScoped when the target memory
// ID belongs to a row that is not (workspace, agent)-scoped — i.e. it still
// carries a user_id or has no agent_id. Callers MUST use errors.Is against
// this sentinel so the HTTP handler can map it to 400 rather than 500.
var ErrNotAgentScoped = errors.New("memory: target is not an agent-scoped admin memory")

// SaveAgentScoped persists an operator-curated memory tied to a specific
// agent but not owned by any user (virtual_user_id IS NULL, agent_id = X).
// These are agent-level policies, training snippets, or runbooks that should
// be visible to every session against that agent regardless of user.
//
// Like SaveInstitutional, the scope map is sanitized so a caller cannot leak
// a user_id into this path, and provenance is forced to operator_curated.
func (s *PostgresMemoryStore) SaveAgentScoped(ctx context.Context, mem *Memory) error {
	workspaceID := mem.Scope[ScopeWorkspaceID]
	if workspaceID == "" {
		return errors.New(errWorkspaceRequired)
	}
	agentID := mem.Scope[ScopeAgentID]
	if agentID == "" {
		return errors.New(errAgentIDRequired)
	}

	// Replace the scope map so user_id cannot leak into the insert path via
	// scopeOrNil(). Keep only workspace + agent.
	mem.Scope = map[string]string{
		ScopeWorkspaceID: workspaceID,
		ScopeAgentID:     agentID,
	}

	if mem.Metadata == nil {
		mem.Metadata = map[string]any{}
	}
	mem.Metadata[pkmemory.MetaKeyProvenance] = string(pkmemory.ProvenanceOperatorCurated)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("memory: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := insertAgentScopedEntity(ctx, tx, mem); err != nil {
		return err
	}
	if err := insertObservation(ctx, tx, mem); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// insertAgentScopedEntity inserts a memory_entities row with agent_id set and
// virtual_user_id NULL. Trust model and source type are stamped so downstream
// retention and PII pipelines know this row was operator-curated.
func insertAgentScopedEntity(ctx context.Context, tx pgx.Tx, mem *Memory) error {
	metaJSON, err := marshalMetadata(mem.Metadata)
	if err != nil {
		return err
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO memory_entities
		  (workspace_id, virtual_user_id, agent_id, name, kind, metadata, trust_model, source_type, expires_at)
		VALUES
		  ($1, NULL, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at`,
		mem.Scope[ScopeWorkspaceID],
		mem.Scope[ScopeAgentID],
		mem.Content,
		mem.Type,
		metaJSON,
		agentScopedTrustModel,
		agentScopedSourceType,
		mem.ExpiresAt,
	)
	return row.Scan(&mem.ID, &mem.CreatedAt)
}

// ListAgentScoped returns every agent-scoped admin memory for a
// (workspace, agent) pair. Scope is defined as virtual_user_id IS NULL AND
// agent_id = $agentID. Results are ordered by most-recent observation and
// paginated.
func (s *PostgresMemoryStore) ListAgentScoped(ctx context.Context, workspaceID, agentID string, opts ListOptions) ([]*Memory, error) {
	if workspaceID == "" {
		return nil, errors.New(errWorkspaceRequired)
	}
	if agentID == "" {
		return nil, errors.New(errAgentIDRequired)
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultMemoryLimit
	}
	sql := `
		SELECT DISTINCT ON (e.id)
		  e.id, e.kind, e.metadata, e.created_at, e.expires_at,
		  o.content, o.confidence, o.session_id, o.turn_range, o.observed_at, o.accessed_at
		FROM memory_entities e
		JOIN memory_observations o ON o.entity_id = e.id AND o.superseded_by IS NULL
		WHERE e.workspace_id = $1
		  AND e.virtual_user_id IS NULL
		  AND e.agent_id = $2::uuid
		  AND e.forgotten = false
		ORDER BY e.id, o.observed_at DESC
		LIMIT $3 OFFSET $4`
	rows, err := s.pool.Query(ctx, sql, workspaceID, agentID, limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("memory: list agent-scoped: %w", err)
	}
	defer rows.Close()
	return scanMemories(rows, map[string]string{
		ScopeWorkspaceID: workspaceID,
		ScopeAgentID:     agentID,
	})
}

// DeleteAgentScoped soft-deletes an agent-scoped admin memory after verifying
// the target row is indeed (workspace, agent)-scoped. Returns ErrNotAgentScoped
// when the row carries a user_id or belongs to a different agent, preventing
// the admin path from being misused to touch user-scoped data.
func (s *PostgresMemoryStore) DeleteAgentScoped(ctx context.Context, workspaceID, agentID, memoryID string) error {
	if workspaceID == "" {
		return errors.New(errWorkspaceRequired)
	}
	if agentID == "" {
		return errors.New(errAgentIDRequired)
	}
	var userID, rowAgentID *string
	row := s.pool.QueryRow(ctx, `
		SELECT virtual_user_id, agent_id::text
		FROM memory_entities
		WHERE id = $1 AND workspace_id = $2 AND forgotten = false`,
		memoryID, workspaceID,
	)
	if err := row.Scan(&userID, &rowAgentID); err != nil {
		return fmt.Errorf("memory: lookup agent-scoped: %w", err)
	}
	if userID != nil || rowAgentID == nil || *rowAgentID != agentID {
		return ErrNotAgentScoped
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE memory_entities SET forgotten = true, updated_at = now()
		WHERE id = $1 AND workspace_id = $2`,
		memoryID, workspaceID)
	if err != nil {
		return fmt.Errorf("memory: delete agent-scoped: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("memory: entity %s not found", memoryID)
	}
	return nil
}
