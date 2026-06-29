/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"fmt"
)

// HardDeleteForgottenByConsentOlderThan clears rows that were soft-
// deleted through the consent cascade once the policy's grace window
// expires. Mirrors HardDeleteForgottenOlderThan but keys on
// forgotten_at so a row flipped by TTL / LRU doesn't get picked up
// here — that's the session of the general hard-delete pass.
//
// The distinction matters because operators tend to give consent-
// driven deletions a shorter grace than regular TTL deletions.
func (s *PostgresMemoryStore) HardDeleteForgottenByConsentOlderThan(
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
    AND forgotten_at IS NOT NULL
    AND forgotten_at < now() - $1::interval
  ORDER BY forgotten_at ASC
  LIMIT $2
  FOR UPDATE SKIP LOCKED
)
DELETE FROM memory_entities
WHERE id IN (SELECT id FROM target)`

	interval := fmt.Sprintf("%d days", graceDays)
	tag, err := s.pool.Exec(ctx, q, interval, batchSize)
	if err != nil {
		return 0, fmt.Errorf("memory: hard-delete forgotten by consent: %w", err)
	}
	return tag.RowsAffected(), nil
}
