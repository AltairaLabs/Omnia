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

// ConsolidationWriter implements consolidation.Store using pgxpool.
type ConsolidationWriter struct {
	pool *pgxpool.Pool
}

// NewConsolidationWriter constructs a ConsolidationWriter around the
// provided pgxpool.
func NewConsolidationWriter(pool *pgxpool.Pool) *ConsolidationWriter {
	return &ConsolidationWriter{pool: pool}
}

// SaveSummary writes a new observation row marked as a summary,
// attached to the entity_id of the first source observation in FromIDs,
// with promoted_from_ids populated.
//
// CreateSummaryAction.FromIDs MUST be non-empty (validator gate enforces);
// the new summary attaches to the same memory_entity as the first source
// observation, preserving the (workspace, user, agent, kind, name) scope.
func (s *ConsolidationWriter) SaveSummary(ctx context.Context, w consolidation.SummaryWrite) (string, error) {
	if len(w.FromIDs) == 0 {
		return "", fmt.Errorf("SaveSummary: FromIDs required (validator should have rejected)")
	}
	const q = `
INSERT INTO memory_observations (
    entity_id, content, source_type, mutability,
    promoted_from_ids, promoted_by_pack, promoted_at
)
SELECT entity_id, $1, 'ai_summary', 'mutable',
       $2::uuid[], $3, $4
FROM memory_observations
WHERE id = $5
LIMIT 1
RETURNING id;
`
	var id string
	if err := s.pool.QueryRow(ctx, q,
		w.Content, w.FromIDs, w.PromotedByPack, w.PromotedAt, w.FromIDs[0],
	).Scan(&id); err != nil {
		return "", fmt.Errorf("SaveSummary: %w", err)
	}
	return id, nil
}

// Supersede sets superseded_by on target rows.
func (s *ConsolidationWriter) Supersede(ctx context.Context, w consolidation.SupersedeWrite) error {
	const q = `
UPDATE memory_observations
SET superseded_by = $1, promoted_by_pack = $2, promoted_at = $3
WHERE id = ANY($4::uuid[]);
`
	_, err := s.pool.Exec(ctx, q, w.WithID, w.PromotedByPack, w.PromotedAt, w.TargetIDs)
	return err
}

// Rescope updates the scope keys on target observation rows by
// modifying their parent entity's virtual_user_id / agent_id.
func (s *ConsolidationWriter) Rescope(ctx context.Context, w consolidation.RescopeWrite) error {
	const q = `
UPDATE memory_entities
SET virtual_user_id = NULLIF($1, ''),
    agent_id = NULLIF($2, '')::uuid,
    promoted_by_pack = $3,
    promoted_at = $4
WHERE id IN (
    SELECT entity_id FROM memory_observations WHERE id = ANY($5::uuid[])
);
`
	_, err := s.pool.Exec(ctx, q,
		w.NewScope.UserID, w.NewScope.AgentID, w.PromotedByPack, w.PromotedAt, w.TargetIDs)
	return err
}

// Invalidate sets valid_until on target rows.
func (s *ConsolidationWriter) Invalidate(ctx context.Context, w consolidation.InvalidateWrite) error {
	const q = `
UPDATE memory_observations
SET valid_until = $1, promoted_by_pack = $2, promoted_at = $3
WHERE id = ANY($4::uuid[]);
`
	_, err := s.pool.Exec(ctx, q, w.ValidUntil, w.PromotedByPack, w.PromotedAt, w.TargetIDs)
	return err
}

// MergeEntities reparents observations from merge_ids onto canonical_id
// and marks the merged entities forgotten.
func (s *ConsolidationWriter) MergeEntities(ctx context.Context, w consolidation.MergeWrite) error {
	const reparent = `
UPDATE memory_observations
SET entity_id = $1
WHERE entity_id = ANY($2::uuid[]);
`
	const flag = `
UPDATE memory_entities
SET forgotten = true,
    promoted_by_pack = $1,
    promoted_at = $2,
    promoted_from_ids = ARRAY[$3]::uuid[]
WHERE id = ANY($4::uuid[]);
`
	if _, err := s.pool.Exec(ctx, reparent, w.CanonicalID, w.MergeIDs); err != nil {
		return fmt.Errorf("MergeEntities reparent: %w", err)
	}
	if _, err := s.pool.Exec(ctx, flag, w.PromotedByPack, w.PromotedAt, w.CanonicalID, w.MergeIDs); err != nil {
		return fmt.Errorf("MergeEntities flag: %w", err)
	}
	return nil
}

// Discard marks target observations forgotten via valid_until=now.
func (s *ConsolidationWriter) Discard(ctx context.Context, w consolidation.DiscardWrite) error {
	const q = `
UPDATE memory_observations
SET valid_until = NOW(),
    promoted_by_pack = $1,
    promoted_at = $2
WHERE id = ANY($3::uuid[]);
`
	_, err := s.pool.Exec(ctx, q, w.PromotedByPack, w.PromotedAt, w.TargetIDs)
	return err
}

// Rescore updates confidence on a single observation. Importance is
// not yet a column — placeholder for future expansion.
func (s *ConsolidationWriter) Rescore(ctx context.Context, w consolidation.RescoreWrite) error {
	if w.Confidence <= 0 {
		return nil
	}
	const q = `
UPDATE memory_observations
SET confidence = $1
WHERE id = $2;
`
	_, err := s.pool.Exec(ctx, q, w.Confidence, w.TargetID)
	return err
}
