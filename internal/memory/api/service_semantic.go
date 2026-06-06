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

package api

import (
	"context"

	"github.com/altairalabs/omnia/internal/memory"
	"github.com/altairalabs/omnia/internal/memory/access"
)

const (
	// semanticOverFetchFactor over-fetches from the store before the deny-filter
	// runs, so restricted items consuming result slots don't starve the caller's
	// requested limit. Without it, a query that matches mostly restricted chunks
	// could return far fewer than `limit` allowed items even when more exist.
	semanticOverFetchFactor = 4
	// semanticMaxFetch bounds the over-fetch so a large limit can't explode the
	// store query / RRF cost.
	semanticMaxFetch = 200
)

// RetrieveSemantic runs workspace-scoped hybrid retrieval (semantic + lexical,
// via the existing SearchMemories path) and drops any item denied by denyCEL.
// denyCEL == "" disables filtering. A bad denyCEL fails closed (error) so a
// misconfigured policy can't leak restricted content.
//
// It over-fetches (semanticOverFetchFactor × limit, capped) before filtering,
// then truncates the allowed results to `limit` — so the deny-filter dropping
// restricted items doesn't reduce the number of allowed items the caller gets.
func (s *MemoryService) RetrieveSemantic(ctx context.Context, workspaceID, query, denyCEL string, limit int) ([]*memory.Memory, error) {
	filter, err := access.NewDenyFilter(denyCEL)
	if err != nil {
		return nil, err
	}
	scope := map[string]string{memory.ScopeWorkspaceID: workspaceID}
	mems, err := s.SearchMemories(ctx, scope, query, memory.RetrieveOptions{Limit: overFetchLimit(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]*memory.Memory, 0, len(mems))
	for _, m := range mems {
		if m == nil {
			continue
		}
		if !filter.Allowed(m.Metadata) {
			retrievalDeniedTotal.Inc()
			continue
		}
		out = append(out, m)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// overFetchLimit returns how many rows to request from the store before the
// deny-filter, given the caller's desired post-filter limit. Bounded by
// semanticMaxFetch; limit<=0 (unbounded) over-fetches up to the cap.
func overFetchLimit(limit int) int {
	if limit <= 0 {
		return semanticMaxFetch
	}
	fetch := limit * semanticOverFetchFactor
	if fetch > semanticMaxFetch {
		return semanticMaxFetch
	}
	return fetch
}
