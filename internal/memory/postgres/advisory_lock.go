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
// on success. The release function unlocks and frees the connection.
//
// Session-level advisory locks are bound to the CONNECTION that took them.
// Acquiring via pool.QueryRow and unlocking via pool.Exec can land on two
// different pooled connections — the unlock then no-ops and the lock is
// stranded on the first connection until that connection is closed, which
// silently wedges every future TryLock for the key. To avoid that we pin a
// single dedicated connection for the lock's whole lifetime: lock on it,
// unlock on the SAME connection, then return it to the pool.
func (s *AdvisoryLockStore) TryLock(ctx context.Context, workspaceID, trigger string) (bool, func(), error) {
	key := fmt.Sprintf("%s:%s", workspaceID, trigger)
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return false, noopRelease, fmt.Errorf("acquire conn for advisory lock: %w", err)
	}
	var ok bool
	if err := conn.QueryRow(ctx,
		`SELECT pg_try_advisory_lock(hashtext($1))`, key,
	).Scan(&ok); err != nil {
		conn.Release()
		return false, noopRelease, err
	}
	if !ok {
		conn.Release() // not held by us — free the connection immediately
		return false, noopRelease, nil
	}
	release := func() {
		// Unlock on the SAME connection that holds the session lock, then
		// return it to the pool. Using a different connection would leave
		// the lock stranded.
		_, _ = conn.Exec(context.Background(),
			`SELECT pg_advisory_unlock(hashtext($1))`, key)
		conn.Release()
	}
	return true, release, nil
}
