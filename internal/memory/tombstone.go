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

package memory

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// TombstoneGCOptions configures one tombstone-collection pass.
//
// Tombstones are observations whose active flag is off — either
// `superseded_by IS NOT NULL` (a later observation took over) or
// `valid_until <= now()` (the structured-key dedup path expired
// them). The supersede / dedup machinery never deletes them; they
// pile up indefinitely until this worker runs.
//
// The defaults (30 days minimum age, > 20 inactive per chain, keep
// the most recent 5) match the design spec. Operators tighten these
// when storage pressure climbs and loosen them when the audit
// requirement extends further back.
type TombstoneGCOptions struct {
	// WorkspaceID scopes the GC pass. Required — global delete is
	// never the right answer; ops do this per-workspace so a noisy
	// workspace doesn't push the GC budget out of every other one.
	WorkspaceID string
	// MinAge is the floor — observations younger than this are kept
	// regardless of chain length so brief audit windows are
	// preserved even on hot chains. Defaults to 30 days.
	MinAge time.Duration
	// MinInactiveCount is the chain-length threshold below which the
	// pass leaves a chain alone. A chain with only a handful of
	// inactive entries isn't worth the per-row work; this saves the
	// pass from scanning quiet entities. Defaults to 20.
	MinInactiveCount int
	// KeepRecentInactive is the per-chain audit window — the most
	// recent K inactive observations stay even when the chain
	// crosses the GC threshold. Defaults to 5.
	KeepRecentInactive int
}

// Tombstone GC defaults — matched to the stateful-memory design.
const (
	defaultTombstoneMinAge             = 30 * 24 * time.Hour
	defaultTombstoneMinInactiveCount   = 20
	defaultTombstoneKeepRecentInactive = 5
)

// RunTombstoneGC hard-deletes the older inactive observations for
// every entity in WorkspaceID whose tombstone chain is longer than
// the configured threshold, keeping the most recent K per chain
// for audit. Returns the count of deleted rows so the worker can
// emit a useful log line.
//
// The single-statement WITH-ranked / DELETE pattern keeps the lock
// window short — Postgres holds row locks only on the rows being
// deleted, not on the chains being scanned.
func (s *PostgresMemoryStore) RunTombstoneGC(ctx context.Context, opts TombstoneGCOptions) (int64, error) {
	if opts.WorkspaceID == "" {
		return 0, errors.New(errWorkspaceRequired)
	}
	if opts.MinAge <= 0 {
		opts.MinAge = defaultTombstoneMinAge
	}
	if opts.MinInactiveCount <= 0 {
		opts.MinInactiveCount = defaultTombstoneMinInactiveCount
	}
	if opts.KeepRecentInactive <= 0 {
		opts.KeepRecentInactive = defaultTombstoneKeepRecentInactive
	}

	tag, err := s.pool.Exec(ctx, `
		WITH chains AS (
			SELECT o.entity_id, count(*) AS inactive_count
			FROM memory_observations o
			JOIN memory_entities e ON e.id = o.entity_id
			WHERE e.workspace_id = $1
			  AND (o.superseded_by IS NOT NULL
			       OR (o.valid_until IS NOT NULL AND o.valid_until <= now()))
			  AND o.observed_at < now() - $2::interval
			GROUP BY o.entity_id
			HAVING count(*) > $3
		), ranked AS (
			SELECT o.id,
			       row_number() OVER (
			           PARTITION BY o.entity_id
			           ORDER BY o.observed_at DESC
			       ) AS rn
			FROM memory_observations o
			JOIN chains c ON c.entity_id = o.entity_id
			WHERE (o.superseded_by IS NOT NULL
			       OR (o.valid_until IS NOT NULL AND o.valid_until <= now()))
		)
		DELETE FROM memory_observations
		WHERE id IN (SELECT id FROM ranked WHERE rn > $4)`,
		opts.WorkspaceID,
		fmt.Sprintf("%d seconds", int(opts.MinAge.Seconds())),
		opts.MinInactiveCount,
		opts.KeepRecentInactive,
	)
	if err != nil {
		return 0, fmt.Errorf("memory: tombstone gc: %w", err)
	}
	return tag.RowsAffected(), nil
}
