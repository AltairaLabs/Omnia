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
	"sync"
	"time"
)

// touchAccessedTimeout caps how long a single batch UPDATE is
// allowed to run. Retrieval must not be coupled to write-path
// latency — if the write is slow we'd rather drop the signal than
// stall the next batch.
const touchAccessedTimeout = 5 * time.Second

// touchAccessedFlushInterval is the debounce window. A read-heavy
// workload accumulates entity IDs in memory for this long before
// one UPDATE drains the buffer; the longer the window the fewer
// (but larger) writes hit the DB. 1s is the sweet spot — short
// enough that the LRU signal stays fresh, long enough to coalesce
// the hundreds of recall calls a busy workspace makes per second.
const touchAccessedFlushInterval = time.Second

// touchAccessedBufferCap forces an early flush when the in-memory
// buffer grows past this size. Bounds memory under recall storms
// (a runaway loop returning thousands of distinct entities/sec).
const touchAccessedBufferCap = 5000

// accessTouchBatcher debounces accessed_at updates so a recall-
// heavy workload writes one row per entity per flush window
// instead of one row per recall call.
//
// Without batching: 100 agents × 10 recalls/min × 50 results
// = 50k UPDATEs/min/workspace, every one creating an MVCC dead
// tuple that autovacuum has to eat. With a 1s flush window the
// effective rate drops to ~50 UPDATEs/sec/workspace (one per
// distinct accessed entity per second), and the access_count
// increments accumulate in-memory so we still capture how many
// times each row was touched.
type accessTouchBatcher struct {
	exec    func(ctx context.Context, ids []string, counts []int) error
	mu      sync.Mutex
	pending map[string]int // entityID -> increment in this window
	stopCh  chan struct{}
	flushCh chan struct{} // signal an immediate flush
}

func newAccessTouchBatcher(exec func(ctx context.Context, ids []string, counts []int) error) *accessTouchBatcher {
	b := &accessTouchBatcher{
		exec:    exec,
		pending: make(map[string]int),
		stopCh:  make(chan struct{}),
		flushCh: make(chan struct{}, 1),
	}
	go b.run()
	return b
}

// add coalesces an entity ID into the pending buffer. Bumps the
// access_count increment for the row by 1 (or more, if seen this
// window). Triggers an immediate flush when the buffer crosses
// touchAccessedBufferCap.
func (b *accessTouchBatcher) add(ids []string) {
	if len(ids) == 0 {
		return
	}
	b.mu.Lock()
	for _, id := range ids {
		if id == "" {
			continue
		}
		b.pending[id]++
	}
	overflow := len(b.pending) >= touchAccessedBufferCap
	b.mu.Unlock()
	if overflow {
		// Non-blocking signal — if a flush is already queued we
		// don't need a second one.
		select {
		case b.flushCh <- struct{}{}:
		default:
		}
	}
}

// run is the batcher's flush loop. Exits cleanly on Stop. Each
// flush takes a snapshot of pending under the lock, then runs the
// UPDATE outside the lock so subsequent recalls aren't serialized
// on disk I/O.
func (b *accessTouchBatcher) run() {
	ticker := time.NewTicker(touchAccessedFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-b.stopCh:
			b.flush() // best-effort drain on stop
			return
		case <-ticker.C:
			b.flush()
		case <-b.flushCh:
			b.flush()
		}
	}
}

func (b *accessTouchBatcher) flush() {
	b.mu.Lock()
	if len(b.pending) == 0 {
		b.mu.Unlock()
		return
	}
	ids := make([]string, 0, len(b.pending))
	counts := make([]int, 0, len(b.pending))
	for id, c := range b.pending {
		ids = append(ids, id)
		counts = append(counts, c)
	}
	b.pending = make(map[string]int)
	b.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), touchAccessedTimeout)
	defer cancel()

	start := time.Now()
	err := b.exec(ctx, ids, counts)
	dur := time.Since(start)
	if m := defaultAccessMetrics.Load(); m != nil {
		m.recordAccessUpdate(dur, err)
	}
}

// Stop drains the batcher and exits the goroutine. Safe to call
// multiple times — the close is one-shot via channel-of-struct.
func (b *accessTouchBatcher) Stop() {
	select {
	case <-b.stopCh:
		return
	default:
		close(b.stopCh)
	}
}

// touchAccessedOnRead enqueues entity IDs into the batcher. It's a
// no-op for the empty slice so retrieval callsites don't need a
// length guard of their own. Falls back to the inline UPDATE when
// the batcher hasn't been wired (test stores constructed without
// the batcher).
//
// The signal feeds the LRU pruning and recency-decay scoring in
// the retention worker — losing the occasional update under load
// is acceptable, losing all of them is not.
func (s *PostgresMemoryStore) touchAccessedOnRead(entityIDs []string) {
	if len(entityIDs) == 0 {
		return
	}
	if s.accessTouch != nil {
		s.accessTouch.add(entityIDs)
		return
	}
	// Test path or store constructed without the batcher: keep the
	// old fire-and-forget behaviour so unit tests don't leak goroutines
	// against a long-lived ticker.
	ids := dedupeStrings(entityIDs)
	go func(ids []string) {
		ctx, cancel := context.WithTimeout(context.Background(), touchAccessedTimeout)
		defer cancel()

		start := time.Now()
		_, err := s.pool.Exec(ctx, `
			UPDATE memory_observations
			SET accessed_at = now(), access_count = access_count + 1
			WHERE entity_id = ANY($1::uuid[])
			  AND superseded_by IS NULL
			  AND (valid_until IS NULL OR valid_until > now())`,
			ids,
		)
		dur := time.Since(start)
		if m := defaultAccessMetrics.Load(); m != nil {
			m.recordAccessUpdate(dur, err)
		}
	}(ids)
}

// runBatchedAccessUpdate is the SQL the batcher dispatches every
// flush window. The unnest($1, $2) trick lets one statement bump
// `accessed_at` to now() and increment `access_count` by the
// per-entity count accumulated in-memory — one round trip even
// for 5000 entities.
func (s *PostgresMemoryStore) runBatchedAccessUpdate(ctx context.Context, ids []string, counts []int) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE memory_observations o
		SET accessed_at = now(),
		    access_count = o.access_count + bumps.c
		FROM (
			SELECT unnest($1::uuid[]) AS id, unnest($2::int[]) AS c
		) bumps
		WHERE o.entity_id = bumps.id
		  AND o.superseded_by IS NULL
		  AND (o.valid_until IS NULL OR o.valid_until > now())`,
		ids, counts,
	)
	return err
}

// dedupeStrings returns a sorted-free copy of the input with duplicates
// removed. The UPDATE's WHERE clause already tolerates duplicates in the
// array, but dedupe keeps the query plan simple and saves a bit of pgx
// marshalling work when a retrieval returns the same entity more than
// once (happens rarely, e.g. structured-lookup + graph merge paths).
func dedupeStrings(in []string) []string {
	if len(in) <= 1 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// entityIDsFromMemories extracts the IDs from a retrieval result. Used
// as the accessed_at touch input.
func entityIDsFromMemories(mems []*Memory) []string {
	if len(mems) == 0 {
		return nil
	}
	ids := make([]string, 0, len(mems))
	for _, m := range mems {
		if m != nil && m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

// entityIDsFromMultiTier extracts IDs from a MultiTierMemory slice.
func entityIDsFromMultiTier(mems []*MultiTierMemory) []string {
	if len(mems) == 0 {
		return nil
	}
	ids := make([]string, 0, len(mems))
	for _, m := range mems {
		if m != nil && m.Memory != nil && m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids
}
