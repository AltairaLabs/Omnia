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

package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
)

// multiTierPath is the memory-api endpoint for multi-tier retrieval.
const multiTierPath = "/api/v1/memories/retrieve"

// MultiTierRequest is the client-side body for POST /api/v1/memories/retrieve.
// Mirrors memory/api.RetrieveMultiTierRequest — redeclared here to preserve
// dependency direction (httpclient must not import the api package).
//
// Purposes narrows the result set to entities tagged with one of the listed
// purpose values. Omit to return every purpose.
type MultiTierRequest struct {
	WorkspaceID   string   `json:"workspace_id"`
	UserID        string   `json:"user_id,omitempty"`
	AgentID       string   `json:"agent_id,omitempty"`
	Query         string   `json:"query,omitempty"`
	Types         []string `json:"types,omitempty"`
	Purposes      []string `json:"purposes,omitempty"`
	MinConfidence float64  `json:"min_confidence,omitempty"`
	Limit         int      `json:"limit,omitempty"`
}

// MultiTierMemory is a decoded response entry with tier annotation and score.
type MultiTierMemory struct {
	*pkmemory.Memory
	Tier        string  `json:"tier"`
	AccessCount int     `json:"access_count"`
	Score       float64 `json:"score"`
}

// MultiTierResult is the decoded response.
type MultiTierResult struct {
	Memories []*MultiTierMemory `json:"memories"`
	Total    int                `json:"total"`
}

// RetrieveMultiTier calls POST /api/v1/memories/retrieve and decodes the
// tier-annotated response. Callers that need only flat memories should use
// Retrieve, which dispatches to this method when the scope has agent_id.
func (s *Store) RetrieveMultiTier(ctx context.Context, req MultiTierRequest) (*MultiTierResult, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("memory httpclient: retrieve_multi_tier: encode: %w", err)
	}

	resp, err := s.doRequest(ctx, http.MethodPost, multiTierPath, body)
	if err != nil {
		return nil, fmt.Errorf("memory httpclient: retrieve_multi_tier: %w", err)
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, s.readError("retrieve_multi_tier", resp)
	}

	var out MultiTierResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("memory httpclient: retrieve_multi_tier: decode: %w", err)
	}
	if out.Memories == nil {
		out.Memories = []*MultiTierMemory{}
	}
	return &out, nil
}

// retrieveMultiTierToMemories calls RetrieveMultiTier and discards tier
// metadata to satisfy the pkmemory.Store.Retrieve signature.
func (s *Store) retrieveMultiTierToMemories(ctx context.Context, scope map[string]string, query string, opts pkmemory.RetrieveOptions) ([]*pkmemory.Memory, error) {
	res, err := s.RetrieveMultiTier(ctx, MultiTierRequest{
		WorkspaceID:   scope["workspace_id"],
		UserID:        scope["user_id"],
		AgentID:       scope["agent_id"],
		Query:         query,
		Types:         opts.Types,
		MinConfidence: opts.MinConfidence,
		Limit:         opts.Limit,
	})
	if err != nil {
		return nil, err
	}
	mems := make([]*pkmemory.Memory, 0, len(res.Memories))
	for _, m := range res.Memories {
		if m.Memory != nil {
			mems = append(mems, m.Memory)
		}
	}
	return mems, nil
}
