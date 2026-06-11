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

package postgres

// consent.go implements the one-shot consent marker that gates a destructive
// embedding-dimension change (#1309). The marker is a single-row table owned
// by the reconciler (created via CREATE TABLE IF NOT EXISTS, not a migration).
// It is conscious (must be created), specific (names the exact target
// dimension), and one-shot (consumed atomically with the reshape) — so consent
// can never be left standing to silently permit a later accidental swap.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ensureConsentTable creates the singleton consent table if it doesn't exist.
func ensureConsentTable(ctx context.Context, db pgExecutor) error {
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS memory_embedding_dim_change_consent (
			id          BOOLEAN     PRIMARY KEY DEFAULT true CHECK (id),
			target_dim  INT         NOT NULL,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			created_by  TEXT
		)`)
	if err != nil {
		return fmt.Errorf("memory: ensure consent table: %w", err)
	}
	return nil
}

// readConsent returns the authorised target dimension and whether a marker
// exists.
func readConsent(ctx context.Context, db pgExecutor) (targetDim int, present bool, err error) {
	err = db.QueryRow(ctx, `SELECT target_dim FROM memory_embedding_dim_change_consent LIMIT 1`).Scan(&targetDim)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("memory: read consent: %w", err)
	}
	return targetDim, true, nil
}

// consumeConsent deletes the marker — called atomically with a reshape.
func consumeConsent(ctx context.Context, db pgExecutor) error {
	if _, err := db.Exec(ctx, `DELETE FROM memory_embedding_dim_change_consent`); err != nil {
		return fmt.Errorf("memory: consume consent: %w", err)
	}
	return nil
}

// clearStaleConsent removes a marker that authorises the dimension the store is
// already at — so a no-op restart can't leave a dangling authorisation around.
func clearStaleConsent(ctx context.Context, db pgExecutor, dim int) error {
	if _, err := db.Exec(ctx, `DELETE FROM memory_embedding_dim_change_consent WHERE target_dim = $1`, dim); err != nil {
		return fmt.Errorf("memory: clear stale consent: %w", err)
	}
	return nil
}

// InsertDimensionChangeConsent records one-shot consent to change the embedding
// dimension to targetDim. Used by the memory-api admin endpoint. createdBy is
// an already-hashed identifier (callers must not pass raw user IDs).
func InsertDimensionChangeConsent(ctx context.Context, pool *pgxpool.Pool, targetDim int, createdBy string) error {
	if targetDim <= 0 {
		return fmt.Errorf("memory: invalid target dimension %d", targetDim)
	}
	if err := ensureConsentTable(ctx, pool); err != nil {
		return err
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO memory_embedding_dim_change_consent (id, target_dim, created_by)
		VALUES (true, $1, $2)
		ON CONFLICT (id) DO UPDATE
			SET target_dim = EXCLUDED.target_dim,
			    created_by = EXCLUDED.created_by,
			    created_at = now()`, targetDim, createdBy)
	if err != nil {
		return fmt.Errorf("memory: insert dimension change consent: %w", err)
	}
	return nil
}
