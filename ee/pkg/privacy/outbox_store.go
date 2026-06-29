/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"fmt"
	"time"
)

// OutboxEntry is a pending consent-revocation delivery record.
type OutboxEntry struct {
	ID       string
	UserID   string
	Category ConsentCategory
}

// RemoveConsentGrantWithOutbox removes a consent grant and records the revocation
// in the outbox atomically (single transaction).
//
// Returns the new outbox row ID on success. Returns ("", nil) if the user has
// no preferences row or the category is not currently granted — a no-op that
// produces no outbox row.
func (s *PreferencesPostgresStore) RemoveConsentGrantWithOutbox(
	ctx context.Context, userID string, category ConsentCategory,
) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`UPDATE user_privacy_preferences
		    SET consent_grants = array_remove(consent_grants, $2), updated_at = now()
		  WHERE user_id = $1 AND $2 = ANY(consent_grants)`,
		userID, string(category))
	if err != nil {
		return "", fmt.Errorf("remove consent grant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// No prefs row, or category not currently granted — nothing to revoke.
		return "", nil
	}

	var id string
	if err := tx.QueryRow(ctx,
		`INSERT INTO consent_revocation_outbox (user_id, category) VALUES ($1, $2) RETURNING id`,
		userID, string(category)).Scan(&id); err != nil {
		return "", fmt.Errorf("insert outbox: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return id, nil
}

// MarkOutboxDelivered marks the outbox row with the given id as delivered.
func (s *PreferencesPostgresStore) MarkOutboxDelivered(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE consent_revocation_outbox SET delivered_at = now() WHERE id = $1`,
		id)
	return err
}

// ListUndeliveredOutbox returns undelivered outbox rows created within maxAge of
// now, ordered oldest-first, up to limit rows.
func (s *PreferencesPostgresStore) ListUndeliveredOutbox(
	ctx context.Context, maxAge time.Duration, limit int,
) ([]OutboxEntry, error) {
	interval := fmt.Sprintf("%d seconds", int(maxAge.Seconds()))
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, category
		   FROM consent_revocation_outbox
		  WHERE delivered_at IS NULL
		    AND created_at > now() - $1::interval
		  ORDER BY created_at
		  LIMIT $2`,
		interval, limit)
	if err != nil {
		return nil, fmt.Errorf("list undelivered outbox: %w", err)
	}
	defer rows.Close()

	var entries []OutboxEntry
	for rows.Next() {
		var e OutboxEntry
		var cat string
		if err := rows.Scan(&e.ID, &e.UserID, &cat); err != nil {
			return nil, fmt.Errorf("scan outbox row: %w", err)
		}
		e.Category = ConsentCategory(cat)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox rows: %w", err)
	}
	return entries, nil
}

// PruneDeliveredOutbox deletes delivered outbox rows older than ttl.
// Returns the number of rows deleted.
func (s *PreferencesPostgresStore) PruneDeliveredOutbox(ctx context.Context, ttl time.Duration) (int64, error) {
	interval := fmt.Sprintf("%d seconds", int(ttl.Seconds()))
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM consent_revocation_outbox
		  WHERE delivered_at IS NOT NULL
		    AND delivered_at < now() - $1::interval`,
		interval)
	if err != nil {
		return 0, fmt.Errorf("prune delivered outbox: %w", err)
	}
	return tag.RowsAffected(), nil
}

// CountStuckOutbox counts undelivered outbox rows older than stuckAge.
func (s *PreferencesPostgresStore) CountStuckOutbox(ctx context.Context, stuckAge time.Duration) (int64, error) {
	interval := fmt.Sprintf("%d seconds", int(stuckAge.Seconds()))
	var count int64
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM consent_revocation_outbox
		  WHERE delivered_at IS NULL
		    AND created_at < now() - $1::interval`,
		interval).Scan(&count); err != nil {
		return 0, fmt.Errorf("count stuck outbox: %w", err)
	}
	return count, nil
}
