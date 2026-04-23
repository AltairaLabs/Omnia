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
	"encoding/json"
	"net/http"

	"github.com/altairalabs/omnia/internal/memory"
)

// Retrieve endpoint limits.
const (
	defaultRetrieveLimit = 15
	maxRetrieveLimit     = 100
)

// RetrieveMultiTierRequest is the JSON body for POST /api/v1/memories/retrieve.
//
// Purposes narrows the result set to memories tagged with one of the listed
// purpose values (e.g. "support_continuity", "personalisation"). Omit to
// return every purpose — the pre-filter default.
type RetrieveMultiTierRequest struct {
	WorkspaceID   string   `json:"workspace_id"`
	UserID        string   `json:"user_id,omitempty"`
	AgentID       string   `json:"agent_id,omitempty"`
	Query         string   `json:"query,omitempty"`
	Types         []string `json:"types,omitempty"`
	Purposes      []string `json:"purposes,omitempty"`
	MinConfidence float64  `json:"min_confidence,omitempty"`
	Limit         int      `json:"limit,omitempty"`
}

// RetrieveMultiTierResponse is the JSON response for the endpoint.
type RetrieveMultiTierResponse struct {
	Memories []*memory.MultiTierMemory `json:"memories"`
	Total    int                       `json:"total"`
}

// handleRetrieveMultiTier handles POST /api/v1/memories/retrieve.
// Body-based so query + types don't inflate URLs and so the scope map
// carrying user_id/agent_id isn't exposed in request logs.
func (h *Handler) handleRetrieveMultiTier(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)

	var req RetrieveMultiTierRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}

	if req.WorkspaceID == "" {
		writeError(w, ErrMissingWorkspace)
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = defaultRetrieveLimit
	}
	if limit > maxRetrieveLimit {
		limit = maxRetrieveLimit
	}

	storeReq := memory.MultiTierRequest{
		WorkspaceID:   req.WorkspaceID,
		UserID:        req.UserID,
		AgentID:       req.AgentID,
		Query:         req.Query,
		Types:         req.Types,
		Purposes:      req.Purposes,
		MinConfidence: req.MinConfidence,
		Limit:         limit,
	}

	result, err := h.service.RetrieveMultiTier(r.Context(), storeReq)
	if err != nil {
		h.log.Error(err, "RetrieveMultiTier failed", "workspace", req.WorkspaceID)
		writeError(w, err)
		return
	}

	writeJSON(w, RetrieveMultiTierResponse{
		Memories: result.Memories,
		Total:    result.Total,
	})
}
