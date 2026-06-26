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
	"errors"
	"net/http"

	"github.com/altairalabs/omnia/internal/httputil"
	"github.com/altairalabs/omnia/internal/memory"
)

// handleListConflicts returns entities whose active observation
// count is > 1 — a signal that some write path bypassed the
// supersede / dedup machinery. Operators triage these in the
// dashboard.
func (h *Handler) handleListConflicts(w http.ResponseWriter, r *http.Request) {
	scope, err := parseWorkspaceScope(r)
	if err != nil {
		writeError(w, err)
		return
	}
	limit := min(max(parseIntParam(r, "limit", defaultListLimit), 1), maxListLimit)
	conflicts, err := h.service.FindConflicts(r.Context(), scope[memory.ScopeWorkspaceID], limit)
	if err != nil {
		h.log.Error(err, "FindConflicts failed", "workspace", scope[memory.ScopeWorkspaceID])
		writeError(w, err)
		return
	}
	writeJSON(w, ConflictsResponse{Conflicts: conflicts, Total: len(conflicts)})
}

// handleSupersedeMemories collapses N source entities into one
// canonical truth: each source's active observation is marked
// inactive and a single new observation lands under SourceIDs[0].
// Powers memory__supersede so the agent can resolve duplicate-fact
// noise (e.g. three pre-`about` memories about the user's name) in
// a single round trip.
func (h *Handler) handleSupersedeMemories(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)

	var req SupersedeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}
	if len(req.SourceIDs) == 0 {
		http.Error(w, `{"error":"source_ids must contain at least one entity ID"}`, http.StatusBadRequest)
		return
	}
	if req.Content == "" {
		http.Error(w, `{"error":"content is required"}`, http.StatusBadRequest)
		return
	}
	if req.Scope[memory.ScopeUserID] == "" {
		writeError(w, ErrMissingUserID)
		return
	}

	mem := &memory.Memory{
		Type:       req.Type,
		Content:    req.Content,
		Confidence: req.Confidence,
		Scope:      req.Scope,
	}

	res, err := h.service.SupersedeManyMemories(r.Context(), req.SourceIDs, mem)
	if err != nil {
		h.log.Error(err, "SupersedeManyMemories failed",
			"sourceCount", len(req.SourceIDs))
		writeError(w, err)
		return
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(SaveMemoryResponse{
		Memory:                   *newMemoryWithTier(mem),
		Action:                   res.Action,
		SupersededObservationIDs: res.SupersededObservationIDs,
		SupersedeReason:          res.SupersedeReason,
	})
}

// handleLinkMemories inserts a row into memory_relations connecting
// source_id to target_id with the given relation_type. Used by
// memory__link to attach derived facts (preferences, notes) to anchor
// entities (the user identity) so name changes don't strand them.
func (h *Handler) handleLinkMemories(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)

	var req LinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}
	if req.SourceID == "" || req.TargetID == "" || req.RelationType == "" {
		http.Error(w, `{"error":"source_id, target_id, and relation_type are required"}`, http.StatusBadRequest)
		return
	}
	if req.Scope[memory.ScopeWorkspaceID] == "" {
		writeError(w, ErrMissingWorkspace)
		return
	}

	id, err := h.service.LinkMemories(r.Context(), req.Scope,
		req.SourceID, req.TargetID, req.RelationType, req.Weight)
	if err != nil {
		if errors.Is(err, memory.ErrNotFound) {
			http.Error(w, `{"error":"source or target entity not found"}`, http.StatusNotFound)
			return
		}
		h.log.Error(err, "LinkMemories failed",
			"sourceID", req.SourceID, "targetID", req.TargetID)
		writeError(w, err)
		return
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(LinkResponse{ID: id})
}
