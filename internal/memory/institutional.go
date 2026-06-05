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

// institutionalTrustModel is the trust_model value stamped on every
// institutional write. Operator-curated rows are trusted data by definition.
const institutionalTrustModel = "curated"

// institutionalSourceType is the source_type value stamped on every
// institutional write. Separate from the conversation_extraction default
// used for agent-extracted memories.
const institutionalSourceType = "operator_curated"

// ErrNotInstitutional is returned when DeleteInstitutional is called with a
// memory ID that belongs to a user- or agent-scoped row. Callers MUST use
// errors.Is against this sentinel so the HTTP handler in the institutional
// admin API can map it to a 400 response rather than a 500.
var ErrNotInstitutional = errors.New("memory: target is not an institutional memory")

// SaveInstitutional persists a workspace-scoped memory with no user_id and no
// agent_id. Provenance is forced to operator_curated and trust_model to
// curated regardless of caller input — callers of this method are operators
// by definition, so we overwrite any spoofed provenance and sanitize the
// scope map before the insert path runs.
//
// When both about_kind and about_key are present in metadata the write is
// idempotent: a second call with the same keys upserts the entity and
// supersedes its prior active observations, so re-seeding on every helm
// upgrade produces one chunk per about-key, not duplicates.
func (s *PostgresMemoryStore) SaveInstitutional(ctx context.Context, mem *Memory) error {
	workspaceID := mem.Scope[ScopeWorkspaceID]
	if workspaceID == "" {
		return errors.New(errWorkspaceRequired)
	}

	// Replace the scope map entirely so user/agent keys cannot leak into the
	// insert path via scopeOrNil().
	mem.Scope = map[string]string{ScopeWorkspaceID: workspaceID}

	if mem.Metadata == nil {
		mem.Metadata = map[string]any{}
	}
	mem.Metadata[pkmemory.MetaKeyProvenance] = string(pkmemory.ProvenanceOperatorCurated)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("memory: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := saveInstitutionalEntity(ctx, tx, mem); err != nil {
		return err
	}
	if err := insertObservation(ctx, tx, mem); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// saveInstitutionalEntity routes to the upsert path when about keys are
// present, or the plain-insert path otherwise. Extracted to keep
// SaveInstitutional below the cognitive-complexity threshold.
func saveInstitutionalEntity(ctx context.Context, tx pgx.Tx, mem *Memory) error {
	if hasAboutKey(mem) {
		conflicted, err := upsertInstitutionalEntityByAboutKey(ctx, tx, mem)
		if err != nil {
			return err
		}
		if conflicted {
			if _, err := supersedePriorObservations(ctx, tx, mem.ID); err != nil {
				return err
			}
		}
		return nil
	}
	return insertInstitutionalEntity(ctx, tx, mem)
}

// upsertInstitutionalEntityByAboutKey atomically returns the existing
// institutional entity for (workspace_id, about_kind, about_key) — creating
// it if absent — with forced institutional provenance columns. The unique
// index is NULLS NOT DISTINCT so the NULL virtual_user_id/agent_id pair still
// triggers ON CONFLICT. Returns conflicted=true when an existing entity was
// reused (the caller will then supersede its prior active observations).
func upsertInstitutionalEntityByAboutKey(ctx context.Context, tx pgx.Tx, mem *Memory) (bool, error) {
	metaJSON, err := marshalMetadata(mem.Metadata)
	if err != nil {
		return false, err
	}
	aboutKind := stringFromMeta(mem.Metadata, MetaKeyAboutKind)
	aboutKey := stringFromMeta(mem.Metadata, MetaKeyAboutKey)
	title := stringFromMeta(mem.Metadata, MetaKeyTitle)

	row := tx.QueryRow(ctx, `
		INSERT INTO memory_entities
		  (workspace_id, virtual_user_id, agent_id, name, kind, metadata,
		   trust_model, source_type, expires_at,
		   about_kind, about_key, title)
		VALUES
		  ($1, NULL, NULL, $2, $3, $4,
		   $5, $6, $7,
		   $8, $9, NULLIF($10, ''))
		ON CONFLICT (workspace_id, virtual_user_id, agent_id,
		             about_kind, about_key)
		WHERE about_kind IS NOT NULL AND NOT forgotten
		DO UPDATE SET updated_at = now(),
		              metadata   = EXCLUDED.metadata,
		              title      = COALESCE(EXCLUDED.title, memory_entities.title)
		RETURNING id, created_at, (xmax <> 0) AS conflicted`,
		mem.Scope[ScopeWorkspaceID],
		mem.Content,
		mem.Type,
		metaJSON,
		institutionalTrustModel,
		institutionalSourceType,
		mem.ExpiresAt,
		aboutKind,
		aboutKey,
		title,
	)
	var conflicted bool
	if err := row.Scan(&mem.ID, &mem.CreatedAt, &conflicted); err != nil {
		return false, err
	}
	return conflicted, nil
}

// insertInstitutionalEntity inserts a memory_entities row with virtual_user_id
// and agent_id both NULL and stamps the curated trust_model / operator_curated
// source_type so downstream retention and PII pipelines know this row was not
// extracted from conversation. Used for saves without about_kind/about_key.
func insertInstitutionalEntity(ctx context.Context, tx pgx.Tx, mem *Memory) error {
	metaJSON, err := marshalMetadata(mem.Metadata)
	if err != nil {
		return err
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO memory_entities
		  (workspace_id, virtual_user_id, agent_id, name, kind, metadata, trust_model, source_type, expires_at)
		VALUES
		  ($1, NULL, NULL, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`,
		mem.Scope[ScopeWorkspaceID],
		mem.Content,
		mem.Type,
		metaJSON,
		institutionalTrustModel,
		institutionalSourceType,
		mem.ExpiresAt,
	)
	return row.Scan(&mem.ID, &mem.CreatedAt)
}

// ListInstitutional returns every institutional memory in a workspace.
// Institutional is defined as virtual_user_id IS NULL AND agent_id IS NULL.
// Results are ordered by most-recent observation and paginated.
func (s *PostgresMemoryStore) ListInstitutional(ctx context.Context, workspaceID string, opts ListOptions) ([]*Memory, error) {
	if workspaceID == "" {
		return nil, errors.New(errWorkspaceRequired)
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultMemoryLimit
	}
	sql := `
		SELECT DISTINCT ON (e.id)
		  e.id, e.kind, e.metadata, e.created_at, e.expires_at, e.title,
		  o.content, o.confidence, o.session_id, o.turn_range, o.observed_at, o.accessed_at,
		  o.summary, o.body_size_bytes
		FROM memory_entities e
		JOIN memory_observations o ON o.entity_id = e.id
		  AND o.superseded_by IS NULL
		  AND (o.valid_until IS NULL OR o.valid_until > now())
		WHERE e.workspace_id = $1
		  AND e.virtual_user_id IS NULL
		  AND e.agent_id IS NULL
		  AND e.forgotten = false
		ORDER BY e.id, o.observed_at DESC
		LIMIT $2 OFFSET $3`
	rows, err := s.pool.Query(ctx, sql, workspaceID, limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("memory: list institutional: %w", err)
	}
	defer rows.Close()
	return scanMemories(rows, map[string]string{ScopeWorkspaceID: workspaceID})
}

// DeleteInstitutional soft-deletes an institutional memory. It first verifies
// that the target row has no user_id and no agent_id; otherwise the admin API
// could be misused to erase user-scoped data through the institutional path.
// Returns ErrNotInstitutional when the row belongs to a user or agent tier.
func (s *PostgresMemoryStore) DeleteInstitutional(ctx context.Context, workspaceID, memoryID string) error {
	if workspaceID == "" {
		return errors.New(errWorkspaceRequired)
	}
	var userID, agentID *string
	row := s.pool.QueryRow(ctx, `
		SELECT virtual_user_id, agent_id
		FROM memory_entities
		WHERE id = $1 AND workspace_id = $2 AND forgotten = false`,
		memoryID, workspaceID,
	)
	if err := row.Scan(&userID, &agentID); err != nil {
		return fmt.Errorf("memory: lookup institutional: %w", err)
	}
	if userID != nil || agentID != nil {
		return ErrNotInstitutional
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE memory_entities SET forgotten = true, updated_at = now()
		WHERE id = $1 AND workspace_id = $2`,
		memoryID, workspaceID)
	if err != nil {
		return fmt.Errorf("memory: delete institutional: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("memory: entity %s not found", memoryID)
	}
	return nil
}
