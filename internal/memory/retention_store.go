/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"fmt"
	"time"
)

// SoftDeleteExpiredTTL soft-deletes up to batchSize entities in the
// given tier whose expires_at is in the past and that aren't already
// forgotten. Returns the number of rows flipped.
//
// Uses `FOR UPDATE SKIP LOCKED` so concurrent worker instances in HA
// deployments never block each other.
func (s *PostgresMemoryStore) SoftDeleteExpiredTTL(
	ctx context.Context, tier Tier, batchSize int,
) (int64, error) {
	if batchSize <= 0 {
		return 0, nil
	}
	q := fmt.Sprintf(`
WITH target AS (
  SELECT id FROM memory_entities
  WHERE forgotten = false
    AND expires_at IS NOT NULL
    AND expires_at < now()
    AND %s
  ORDER BY expires_at ASC
  LIMIT $1
  FOR UPDATE SKIP LOCKED
)
UPDATE memory_entities
SET forgotten = true, updated_at = now()
WHERE id IN (SELECT id FROM target)`, tier.sqlPredicate())

	tag, err := s.pool.Exec(ctx, q, batchSize)
	if err != nil {
		return 0, fmt.Errorf("memory: soft-delete ttl (%s): %w", tier, err)
	}
	return tag.RowsAffected(), nil
}

// SoftDeleteLRU soft-deletes up to batchSize entities in the given
// tier whose most recent activity (accessed_at, falling back to
// observed_at then created_at) is older than staleAfter.
//
// staleAfter must be positive; the caller validates non-zero before
// invoking so we can keep the query path cheap.
func (s *PostgresMemoryStore) SoftDeleteLRU(
	ctx context.Context, tier Tier, staleAfter time.Duration, batchSize int,
) (int64, error) {
	if batchSize <= 0 || staleAfter <= 0 {
		return 0, nil
	}
	q := fmt.Sprintf(`
WITH last_seen AS (
  SELECT e.id,
         GREATEST(
           e.created_at,
           COALESCE(MAX(o.accessed_at), MAX(o.observed_at), e.created_at)
         ) AS last_activity
  FROM memory_entities e
  LEFT JOIN memory_observations o
    ON o.entity_id = e.id AND o.superseded_by IS NULL
  WHERE e.forgotten = false
    AND %s
  GROUP BY e.id
),
target AS (
  SELECT id FROM last_seen
  WHERE last_activity < now() - $1::interval
  ORDER BY last_activity ASC
  LIMIT $2
  FOR UPDATE SKIP LOCKED
)
UPDATE memory_entities
SET forgotten = true, updated_at = now()
WHERE id IN (SELECT id FROM target)`, tier.sqlPredicate())

	interval := fmt.Sprintf("%d seconds", int64(staleAfter.Seconds()))
	tag, err := s.pool.Exec(ctx, q, interval, batchSize)
	if err != nil {
		return 0, fmt.Errorf("memory: soft-delete lru (%s): %w", tier, err)
	}
	return tag.RowsAffected(), nil
}

// HardDeleteForgottenOlderThan removes rows flipped to forgotten=true
// more than graceDays days ago, up to batchSize. Observations cascade
// via the ON DELETE CASCADE FK on memory_observations.entity_id.
func (s *PostgresMemoryStore) HardDeleteForgottenOlderThan(
	ctx context.Context, graceDays int32, batchSize int,
) (int64, error) {
	if batchSize <= 0 {
		return 0, nil
	}
	if graceDays < 0 {
		return 0, fmt.Errorf("memory: negative grace days (%d)", graceDays)
	}
	q := `
WITH target AS (
  SELECT id FROM memory_entities
  WHERE forgotten = true
    AND updated_at < now() - $1::interval
  ORDER BY updated_at ASC
  LIMIT $2
  FOR UPDATE SKIP LOCKED
)
DELETE FROM memory_entities
WHERE id IN (SELECT id FROM target)`

	interval := fmt.Sprintf("%d days", graceDays)
	tag, err := s.pool.Exec(ctx, q, interval, batchSize)
	if err != nil {
		return 0, fmt.Errorf("memory: hard-delete forgotten: %w", err)
	}
	return tag.RowsAffected(), nil
}
