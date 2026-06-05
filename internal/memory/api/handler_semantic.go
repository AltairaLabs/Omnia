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
)

// SemanticRetrieveRequest is the body for POST /api/v1/memories/retrieve/semantic.
type SemanticRetrieveRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Query       string `json:"query"`
	DenyCEL     string `json:"deny_cel,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}

// handleSemanticRetrieve runs hybrid retrieval with the access deny-filter.
func (h *Handler) handleSemanticRetrieve(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)
	var req SemanticRetrieveRequest
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
	if req.Limit < 1 {
		req.Limit = defaultListLimit
	}
	if req.Limit > maxListLimit {
		req.Limit = maxListLimit
	}
	mems, err := h.service.RetrieveSemantic(r.Context(), req.WorkspaceID, req.Query, req.DenyCEL, req.Limit)
	if err != nil {
		h.log.Error(err, "RetrieveSemantic failed", "workspace", req.WorkspaceID)
		writeError(w, err)
		return
	}
	writeJSON(w, MemoryListResponse{
		Memories: wrapMemoriesWithTier(mems),
		Total:    len(mems),
	})
}
