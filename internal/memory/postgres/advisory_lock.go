/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AdvisoryLockStore implements consolidation.LockStore using Postgres
// session-level advisory locks. Lock is keyed by
// hashtext(<workspace>:<trigger>) so two replicas trying to consolidate
// the same workspace serialise — the loser skips this tick.
type AdvisoryLockStore struct {
	pool *pgxpool.Pool
}

// NewAdvisoryLockStore constructs an AdvisoryLockStore around the
// provided pgxpool.
func NewAdvisoryLockStore(pool *pgxpool.Pool) *AdvisoryLockStore {
	return &AdvisoryLockStore{pool: pool}
}

// noopRelease is the no-op release function returned on TryLock failure
// or "not acquired" paths. Callers always defer release(), so a no-op
// satisfies the contract without holding any lock state.
func noopRelease() {}

// TryLock attempts pg_try_advisory_lock. Returns (true, release, nil)
// on success. The release function unlocks via the same pool.
func (s *AdvisoryLockStore) TryLock(ctx context.Context, workspaceID, trigger string) (bool, func(), error) {
	key := fmt.Sprintf("%s:%s", workspaceID, trigger)
	var ok bool
	if err := s.pool.QueryRow(ctx,
		`SELECT pg_try_advisory_lock(hashtext($1))`, key,
	).Scan(&ok); err != nil {
		return false, noopRelease, err
	}
	if !ok {
		return false, noopRelease, nil
	}
	release := func() {
		_, _ = s.pool.Exec(context.Background(),
			`SELECT pg_advisory_unlock(hashtext($1))`, key)
	}
	return true, release, nil
}
