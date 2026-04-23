/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"fmt"
)

// SoftDeleteRevokedConsent soft-deletes up to batchSize user-tier rows
// whose consent_category is no longer present in the user's
// consent_grants array. Non-user-tier rows (virtual_user_id IS NULL)
// are skipped — consent is user-scoped.
//
// The join against user_privacy_preferences is safe because that
// table lives in the memory-api's own database (see migration
// 000001_initial_schema.up.sql).
func (s *PostgresMemoryStore) SoftDeleteRevokedConsent(
	ctx context.Context, batchSize int,
) (int64, error) {
	if batchSize <= 0 {
		return 0, nil
	}
	q := `
WITH target AS (
  SELECT e.id FROM memory_entities e
  JOIN user_privacy_preferences p ON p.user_id = e.virtual_user_id
  WHERE e.forgotten = false
    AND e.virtual_user_id IS NOT NULL
    AND e.consent_category IS NOT NULL
    AND NOT (e.consent_category = ANY(p.consent_grants))
  ORDER BY e.created_at ASC
  LIMIT $1
  FOR UPDATE SKIP LOCKED
)
UPDATE memory_entities
SET forgotten = true, forgotten_at = now(), updated_at = now()
WHERE id IN (SELECT id FROM target)`

	tag, err := s.pool.Exec(ctx, q, batchSize)
	if err != nil {
		return 0, fmt.Errorf("memory: soft-delete revoked consent: %w", err)
	}
	return tag.RowsAffected(), nil
}

// HardDeleteRevokedConsent removes up to batchSize user-tier rows
// whose consent_category is no longer granted, skipping the soft-
// delete phase. Used when the policy's consentRevocation.action is
// HardDelete — operators explicitly want immediate removal.
func (s *PostgresMemoryStore) HardDeleteRevokedConsent(
	ctx context.Context, batchSize int,
) (int64, error) {
	if batchSize <= 0 {
		return 0, nil
	}
	q := `
WITH target AS (
  SELECT e.id FROM memory_entities e
  JOIN user_privacy_preferences p ON p.user_id = e.virtual_user_id
  WHERE e.virtual_user_id IS NOT NULL
    AND e.consent_category IS NOT NULL
    AND NOT (e.consent_category = ANY(p.consent_grants))
  ORDER BY e.created_at ASC
  LIMIT $1
  FOR UPDATE SKIP LOCKED
)
DELETE FROM memory_entities
WHERE id IN (SELECT id FROM target)`

	tag, err := s.pool.Exec(ctx, q, batchSize)
	if err != nil {
		return 0, fmt.Errorf("memory: hard-delete revoked consent: %w", err)
	}
	return tag.RowsAffected(), nil
}

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
