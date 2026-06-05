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

// RetrieveSemantic runs workspace-scoped hybrid retrieval (semantic + lexical,
// via the existing SearchMemories path) and drops any item denied by denyCEL.
// denyCEL == "" disables filtering. A bad denyCEL fails closed (error) so a
// misconfigured policy can't leak restricted content.
func (s *MemoryService) RetrieveSemantic(ctx context.Context, workspaceID, query, denyCEL string, limit int) ([]*memory.Memory, error) {
	filter, err := access.NewDenyFilter(denyCEL)
	if err != nil {
		return nil, err
	}
	scope := map[string]string{memory.ScopeWorkspaceID: workspaceID}
	opts := memory.RetrieveOptions{Limit: limit}
	mems, err := s.SearchMemories(ctx, scope, query, opts)
	if err != nil {
		return nil, err
	}
	out := make([]*memory.Memory, 0, len(mems))
	for _, m := range mems {
		if m != nil && filter.Allowed(m.Metadata) {
			out = append(out, m)
		}
	}
	return out, nil
}
