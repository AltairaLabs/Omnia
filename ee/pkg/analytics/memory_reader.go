/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SQL query constants for memory analytics reads.
const (
	readMemoryEntitiesQuery = `
SELECT id, workspace_id, virtual_user_id, agent_id, name, kind,
       source_type, trust_model, purpose, forgotten, created_at, updated_at
FROM memory_entities
WHERE created_at > $1 AND forgotten = false
ORDER BY created_at
LIMIT $2`

	readMemoryObservationsQuery = `
SELECT id, entity_id, content, confidence, source_type, session_id,
       observed_at, created_at, access_count
FROM memory_observations
WHERE created_at > $1
ORDER BY created_at
LIMIT $2`
)

// MemorySourceReader reads memory entities and observations from Postgres
// for analytics sync. It uses watermark-based cursors on created_at.
type MemorySourceReader struct {
	pool *pgxpool.Pool
}

// NewMemorySourceReader creates a new MemorySourceReader backed by the given pool.
func NewMemorySourceReader(pool *pgxpool.Pool) *MemorySourceReader {
	return &MemorySourceReader{pool: pool}
}

// ReadMemoryEntities returns non-forgotten memory entities created after the
// given watermark, up to limit rows, ordered by created_at ascending.
func (r *MemorySourceReader) ReadMemoryEntities(
	ctx context.Context, after time.Time, limit int,
) ([]MemoryEntityRow, error) {
	rows, err := r.pool.Query(ctx, readMemoryEntitiesQuery, after, limit)
	if err != nil {
		return nil, fmt.Errorf("query memory entities: %w", err)
	}
	defer rows.Close()

	var results []MemoryEntityRow
	for rows.Next() {
		var (
			row           MemoryEntityRow
			virtualUserID *string
			agentID       *string
		)
		if err := rows.Scan(
			&row.ID, &row.WorkspaceID, &virtualUserID, &agentID,
			&row.Name, &row.Kind, &row.SourceType, &row.TrustModel,
			&row.Purpose, &row.Forgotten, &row.CreatedAt, &row.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan memory entity: %w", err)
		}
		if virtualUserID != nil {
			row.VirtualUserID = *virtualUserID
		}
		if agentID != nil {
			row.AgentID = *agentID
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate memory entities: %w", err)
	}
	return results, nil
}

// ReadMemoryObservations returns memory observations created after the given
// watermark, up to limit rows, ordered by created_at ascending.
func (r *MemorySourceReader) ReadMemoryObservations(
	ctx context.Context, after time.Time, limit int,
) ([]MemoryObservationRow, error) {
	rows, err := r.pool.Query(ctx, readMemoryObservationsQuery, after, limit)
	if err != nil {
		return nil, fmt.Errorf("query memory observations: %w", err)
	}
	defer rows.Close()

	var results []MemoryObservationRow
	for rows.Next() {
		var (
			row       MemoryObservationRow
			sessionID *string
		)
		if err := rows.Scan(
			&row.ID, &row.EntityID, &row.Content, &row.Confidence,
			&row.SourceType, &sessionID, &row.ObservedAt,
			&row.CreatedAt, &row.AccessCount,
		); err != nil {
			return nil, fmt.Errorf("scan memory observation: %w", err)
		}
		if sessionID != nil {
			row.SessionID = *sessionID
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate memory observations: %w", err)
	}
	return results, nil
}
