/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"fmt"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

// ConsentRevocationSource resolves the set of user IDs whose consent for a
// given category matches the granted flag. *httpclient.Client
// (ee/pkg/privacy/httpclient) satisfies this interface.
//
// A nil source means consent state cannot be verified (e.g. no privacy-api
// configured). The consent-revocation pass MUST skip entirely when the source
// is nil — deleting rows without verified consent state would be unsafe.
type ConsentRevocationSource interface {
	ListConsentUsers(ctx context.Context, category privacy.ConsentCategory, granted bool) ([]string, error)
}

// SoftDeleteRevokedConsent soft-deletes up to batchSize user-tier rows per
// category whose consent_category is no longer granted by the owning user.
//
// For each consent category C in privacy.ValidCategories(), it calls
// src.ListConsentUsers(ctx, C, false) to obtain the set of users who HAVE a
// preferences record but do NOT grant C (matching the semantics of the former
// INNER JOIN against user_privacy_preferences). Rows with
// virtual_user_id = ANY(nonGrantors) and consent_category = C are then
// soft-set to forgotten=true / forgotten_at=now() / updated_at=now().
//
// Non-user-tier rows (virtual_user_id IS NULL) are never touched — consent is
// user-scoped by design.
//
// Nil source: returns (0, nil) immediately — a destructive pass must not run
// when consent state is unverifiable (fail-safe).
//
// Batching: LIMIT $batchSize is applied per category, not across all categories
// in a single pass. This is a benign behavioural change from the former single
// JOIN query — total rows processed per call is up to 7 × batchSize, but
// per-category FOR UPDATE SKIP LOCKED atomicity is preserved.
func (s *PostgresMemoryStore) SoftDeleteRevokedConsent(
	ctx context.Context,
	src ConsentRevocationSource,
	batchSize int,
) (int64, error) {
	if batchSize <= 0 {
		return 0, nil
	}
	if src == nil {
		return 0, nil
	}

	const q = `
WITH target AS (
  SELECT e.id FROM memory_entities e
  WHERE e.forgotten = false
    AND e.virtual_user_id IS NOT NULL
    AND e.consent_category = $1
    AND e.virtual_user_id = ANY($2::text[])
  ORDER BY e.created_at ASC
  LIMIT $3
  FOR UPDATE SKIP LOCKED
)
UPDATE memory_entities
SET forgotten = true, forgotten_at = now(), updated_at = now()
WHERE id IN (SELECT id FROM target)`

	var total int64
	for _, cat := range privacy.ValidCategories() {
		nonGrantors, err := src.ListConsentUsers(ctx, cat, false)
		if err != nil {
			return total, fmt.Errorf("memory: soft-delete revoked consent: list users (%s): %w", cat, err)
		}
		if len(nonGrantors) == 0 {
			continue
		}
		tag, err := s.pool.Exec(ctx, q, string(cat), nonGrantors, batchSize)
		if err != nil {
			return total, fmt.Errorf("memory: soft-delete revoked consent: %s: %w", cat, err)
		}
		total += tag.RowsAffected()
	}
	return total, nil
}

// HardDeleteRevokedConsent removes up to batchSize user-tier rows per
// category whose consent_category is no longer granted by the owning user,
// skipping the soft-delete phase. Used when the policy's
// consentRevocation.action is HardDelete — operators explicitly want
// immediate removal.
//
// Semantics and caveats match SoftDeleteRevokedConsent; see that doc for
// details on src-nil fail-safe behaviour and per-category batching.
func (s *PostgresMemoryStore) HardDeleteRevokedConsent(
	ctx context.Context,
	src ConsentRevocationSource,
	batchSize int,
) (int64, error) {
	if batchSize <= 0 {
		return 0, nil
	}
	if src == nil {
		return 0, nil
	}

	const q = `
WITH target AS (
  SELECT e.id FROM memory_entities e
  WHERE e.virtual_user_id IS NOT NULL
    AND e.consent_category = $1
    AND e.virtual_user_id = ANY($2::text[])
  ORDER BY e.created_at ASC
  LIMIT $3
  FOR UPDATE SKIP LOCKED
)
DELETE FROM memory_entities
WHERE id IN (SELECT id FROM target)`

	var total int64
	for _, cat := range privacy.ValidCategories() {
		nonGrantors, err := src.ListConsentUsers(ctx, cat, false)
		if err != nil {
			return total, fmt.Errorf("memory: hard-delete revoked consent: list users (%s): %w", cat, err)
		}
		if len(nonGrantors) == 0 {
			continue
		}
		tag, err := s.pool.Exec(ctx, q, string(cat), nonGrantors, batchSize)
		if err != nil {
			return total, fmt.Errorf("memory: hard-delete revoked consent: %s: %w", cat, err)
		}
		total += tag.RowsAffected()
	}
	return total, nil
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
