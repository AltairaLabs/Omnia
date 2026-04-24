/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"fmt"
)

// HardDeleteSupersededObservations removes up to batchSize observations
// whose superseded_by pointer references a summary observation that was
// created more than graceDays ago. The grace window is measured from
// the summary's creation so operators get the same rollback budget
// regardless of when the original observation was written.
//
// Superseded observations are already invisible to retrieval
// (retrieve_multi_tier.go:288 filters on `o.superseded_by IS NULL`)
// so hard-deleting them doesn't change API behaviour — it only
// reclaims storage.
func (s *PostgresMemoryStore) HardDeleteSupersededObservations(
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
  SELECT old.id FROM memory_observations old
  JOIN memory_observations new ON new.id = old.superseded_by
  WHERE old.superseded_by IS NOT NULL
    AND new.created_at < now() - $1::interval
  ORDER BY new.created_at ASC
  LIMIT $2
  FOR UPDATE SKIP LOCKED
)
DELETE FROM memory_observations
WHERE id IN (SELECT id FROM target)`

	interval := fmt.Sprintf("%d days", graceDays)
	tag, err := s.pool.Exec(ctx, q, interval, batchSize)
	if err != nil {
		return 0, fmt.Errorf("memory: hard-delete superseded observations: %w", err)
	}
	return tag.RowsAffected(), nil
}
