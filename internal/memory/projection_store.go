/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/pgvector/pgvector-go"

	"github.com/altairalabs/omnia/ee/pkg/memory/projection"
)

// Compile-time check: the concrete store satisfies the Galaxy capability.
var _ ProjectionStore = (*PostgresMemoryStore)(nil)

// LoadProjectionInputs returns one row per entity in scope, carrying the
// most-recent active observation's content/embedding plus the entity metadata
// the Memory Galaxy projection needs.
func (s *PostgresMemoryStore) LoadProjectionInputs(ctx context.Context, scope map[string]string) ([]ProjectionInput, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (e.id)
		    e.id, e.virtual_user_id, e.agent_id, e.consent_category, e.title,
		    e.kind, e.expires_at,
		    o.content, o.confidence, o.observed_at, o.embedding
		FROM memory_entities e
		JOIN memory_observations o ON o.entity_id = e.id
		    AND o.superseded_by IS NULL
		    AND (o.valid_until IS NULL OR o.valid_until > now())
		WHERE e.workspace_id = $1
		    AND ($2::text IS NULL OR e.virtual_user_id = $2)
		    AND ($3::uuid IS NULL OR e.agent_id = $3)
		    AND e.forgotten = false
		ORDER BY e.id, o.observed_at DESC`,
		scope[ScopeWorkspaceID], scopeOrNil(scope, ScopeUserID), scopeOrNil(scope, ScopeAgentID),
	)
	if err != nil {
		return nil, fmt.Errorf("memory: load projection inputs: %w", err)
	}
	defer rows.Close()

	var out []ProjectionInput
	for rows.Next() {
		in, err := scanProjectionInput(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

// rowScanner is the subset of pgx.Rows used for scanning a single row.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanProjectionInput(rows rowScanner) (ProjectionInput, error) {
	var (
		in       ProjectionInput
		userID   *string
		agentID  *string
		category *string
		title    *string
		emb      *pgvector.Vector
	)
	if err := rows.Scan(&in.EntityID, &userID, &agentID, &category, &title,
		&in.Kind, &in.ExpiresAt,
		&in.Content, &in.Confidence, &in.ObservedAt, &emb); err != nil {
		return ProjectionInput{}, fmt.Errorf("memory: projection scan: %w", err)
	}
	if userID != nil {
		in.User = *userID
	}
	if category != nil {
		in.Category = *category
	}
	if title != nil {
		in.Title = *title
	}
	if emb != nil {
		in.Embedding = emb.Slice()
	}
	in.Tier = tierFromColumns(userID, agentID)
	return in, nil
}

// tierFromColumns derives the tier from the raw nullable scope columns.
func tierFromColumns(userID, agentID *string) string {
	switch {
	case userID != nil && agentID != nil:
		return string(TierUserForAgent)
	case userID != nil:
		return string(TierUser)
	case agentID != nil:
		return string(TierAgent)
	default:
		return string(TierInstitutional)
	}
}

// ProjectionFingerprint returns count + max(observed_at) for the scope, used to
// invalidate a stored layout. Returns "" when the scope has no memories.
func (s *PostgresMemoryStore) ProjectionFingerprint(ctx context.Context, scope map[string]string) (string, error) {
	var count int
	var maxObs *time.Time
	var embedded int
	// Mirror LoadProjectionInputs exactly — one row per entity carrying its
	// most-recent active observation — so the embedded fraction matches the
	// projector's basis decision. count(*) FILTER (...) counts entities whose
	// latest active observation has an embedding.
	err := s.pool.QueryRow(ctx, `
		WITH latest AS (
		    SELECT DISTINCT ON (e.id)
		        e.id, o.observed_at, (o.embedding IS NOT NULL) AS has_emb
		    FROM memory_entities e
		    JOIN memory_observations o ON o.entity_id = e.id
		        AND o.superseded_by IS NULL
		        AND (o.valid_until IS NULL OR o.valid_until > now())
		    WHERE e.workspace_id = $1
		        AND ($2::text IS NULL OR e.virtual_user_id = $2)
		        AND ($3::uuid IS NULL OR e.agent_id = $3)
		        AND e.forgotten = false
		    ORDER BY e.id, o.observed_at DESC
		)
		SELECT count(*), max(observed_at), count(*) FILTER (WHERE has_emb)
		FROM latest`,
		scope[ScopeWorkspaceID], scopeOrNil(scope, ScopeUserID), scopeOrNil(scope, ScopeAgentID),
	).Scan(&count, &maxObs, &embedded)
	if err != nil {
		return "", fmt.Errorf("memory: projection fingerprint: %w", err)
	}
	if count == 0 || maxObs == nil {
		return "", nil
	}
	// Third component is the dense-eligibility bit. count + observed_at never
	// change when embeddings backfill onto existing observations, so without
	// this the galaxy stays stuck on its cached lexical layout forever. The bit
	// flips once coverage crosses the projector's dense threshold, triggering
	// exactly one lexical→dense re-render (not one per backfilled batch).
	denseEligible := 0
	if float64(embedded)/float64(count) >= projection.DefaultDenseThreshold {
		denseEligible = 1
	}
	return fmt.Sprintf("%d:%d:%d", count, maxObs.UTC().UnixNano(), denseEligible), nil
}

// LoadProjection returns the stored layout + metadata for scopeKey, or
// (nil, nil) when none is stored.
func (s *PostgresMemoryStore) LoadProjection(ctx context.Context, scopeKey string) (*StoredProjection, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT entity_id, x, y, fingerprint, model, basis, computed_at
		FROM memory_projections WHERE scope_key = $1`, scopeKey)
	if err != nil {
		return nil, fmt.Errorf("memory: load projection: %w", err)
	}
	defer rows.Close()
	sp := &StoredProjection{Layout: map[string][2]float64{}}
	for rows.Next() {
		var id string
		var x, y float64
		if err := rows.Scan(&id, &x, &y, &sp.Fingerprint, &sp.Model, &sp.Basis, &sp.ComputedAt); err != nil {
			return nil, err
		}
		sp.Layout[id] = [2]float64{x, y}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(sp.Layout) == 0 {
		return nil, nil
	}
	return sp, nil
}

// SaveProjection replaces the stored layout for scopeKey in one transaction.
func (s *PostgresMemoryStore) SaveProjection(ctx context.Context, scopeKey, workspaceID, fingerprint, model, basis string, points []ProjectionPoint) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM memory_projections WHERE scope_key = $1`, scopeKey); err != nil {
		return fmt.Errorf("memory: clear projection: %w", err)
	}
	for _, p := range points {
		if _, err := tx.Exec(ctx, `
			INSERT INTO memory_projections
			    (scope_key, workspace_id, entity_id, x, y, model, basis, fingerprint)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			scopeKey, workspaceID, p.EntityID, p.X, p.Y, model, basis, fingerprint); err != nil {
			return fmt.Errorf("memory: insert projection: %w", err)
		}
	}
	return tx.Commit(ctx)
}
