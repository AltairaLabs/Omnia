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
	"time"
)

// touchAccessedTimeout caps how long the background UPDATE is allowed to
// run. Retrieval must not be coupled to write-path latency — if the write
// is slow we'd rather drop the signal than stall the caller (who may
// already have returned to the user).
const touchAccessedTimeout = 5 * time.Second

// touchAccessedOnRead fires a detached UPDATE that bumps accessed_at +
// access_count on the non-superseded observations attached to the given
// entity IDs. It's a no-op for the empty slice so retrieval callsites
// don't need a length guard of their own.
//
// The update runs in its own goroutine with a fresh timeout context, so
// the caller's request context (which may already be cancelled by the
// time the summary lands in the response) doesn't kill the write. This
// is the signal the LRU pruning and recency-decay scoring in the
// retention proposal depend on — losing the occasional update under
// load is acceptable, losing all of them is not.
func (s *PostgresMemoryStore) touchAccessedOnRead(entityIDs []string) {
	if len(entityIDs) == 0 {
		return
	}
	ids := dedupeStrings(entityIDs)

	go func(ids []string) {
		ctx, cancel := context.WithTimeout(context.Background(), touchAccessedTimeout)
		defer cancel()

		start := time.Now()
		tag, err := s.pool.Exec(ctx, `
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
		if err != nil {
			// Intentional log omission — retrieval has already returned to
			// the caller and we don't want a noisy error log for every
			// transient DB blip. Callers who care about the metric observe
			// it via omnia_memory_accessed_update_errors_total.
			_ = err
			_ = tag
		}
	}(ids)
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
