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
	"time"

	"github.com/altairalabs/omnia/internal/httputil"
	"github.com/altairalabs/omnia/internal/memory"
)

// SaveInstitutionalRequest is the JSON body for POST /api/v1/institutional/memories.
//
// ExpiresAt is optional and is NOT defaulted from MemoryServiceConfig.DefaultTTL —
// institutional memories are operator-curated and permanent by default. Callers
// opt in to expiry only when they intentionally want a rule or policy to
// self-retire (e.g. a time-boxed promotion or regulatory freeze). Values in
// the past are rejected with 400 to prevent accidental insert-then-expire
// races that would hide the write from ListInstitutional.
type SaveInstitutionalRequest struct {
	WorkspaceID string         `json:"workspace_id"`
	Type        string         `json:"type"`
	Content     string         `json:"content"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Confidence  float64        `json:"confidence"`
	ExpiresAt   *time.Time     `json:"expires_at,omitempty"`
}

// ListInstitutionalResponse is the JSON response for GET /api/v1/institutional/memories.
type ListInstitutionalResponse struct {
	Memories []*MemoryWithTier `json:"memories"`
	Total    int               `json:"total"`
}

// handleSaveInstitutional handles POST /api/v1/institutional/memories.
// Writes are restricted to workspace scope (no user_id, no agent_id).
func (h *Handler) handleSaveInstitutional(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)

	var req SaveInstitutionalRequest
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
	if req.ExpiresAt != nil && !req.ExpiresAt.After(time.Now()) {
		writeError(w, ErrExpiresAtInPast)
		return
	}

	mem := &memory.Memory{
		Type:       req.Type,
		Content:    req.Content,
		Metadata:   req.Metadata,
		Confidence: req.Confidence,
		ExpiresAt:  req.ExpiresAt,
		Scope:      map[string]string{memory.ScopeWorkspaceID: req.WorkspaceID},
	}
	if err := h.service.SaveInstitutionalMemory(r.Context(), mem); err != nil {
		h.log.Error(err, "SaveInstitutionalMemory failed", "workspace", req.WorkspaceID)
		writeError(w, err)
		return
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(MemoryResponse{Memory: mem})
}

// handleListInstitutional handles GET /api/v1/institutional/memories?workspace=X.
func (h *Handler) handleListInstitutional(w http.ResponseWriter, r *http.Request) {
	workspace := truncateParam(r.URL.Query().Get("workspace"))
	if workspace == "" {
		writeError(w, ErrMissingWorkspace)
		return
	}
	limit := parseIntParam(r, "limit", defaultListLimit)
	if limit < 1 {
		limit = 1
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	opts := memory.ListOptions{
		Limit:  limit,
		Offset: parseIntParam(r, "offset", 0),
	}

	mems, err := h.service.ListInstitutionalMemories(r.Context(), workspace, opts)
	if err != nil {
		h.log.Error(err, "ListInstitutionalMemories failed", "workspace", workspace)
		writeError(w, err)
		return
	}
	writeJSON(w, ListInstitutionalResponse{
		Memories: wrapMemoriesWithTier(mems),
		Total:    len(mems),
	})
}

// handleDeleteInstitutional handles DELETE /api/v1/institutional/memories/{id}?workspace=X.
// Returns 400 (not 500) when the target row is not institutional so callers
// can distinguish operator misuse from infrastructure failures.
func (h *Handler) handleDeleteInstitutional(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, ErrMissingMemoryID)
		return
	}
	workspace := truncateParam(r.URL.Query().Get("workspace"))
	if workspace == "" {
		writeError(w, ErrMissingWorkspace)
		return
	}

	err := h.service.DeleteInstitutionalMemory(r.Context(), workspace, id)
	if err != nil {
		if errors.Is(err, memory.ErrNotInstitutional) {
			writeNotInstitutionalError(w)
			return
		}
		h.log.Error(err, "DeleteInstitutionalMemory failed", "workspace", workspace, "memoryID", id)
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// writeNotInstitutionalError emits a 400 with the sentinel message. Kept
// separate from writeError so the sentinel from the memory package doesn't
// need to be plumbed into the writeError switch.
func writeNotInstitutionalError(w http.ResponseWriter) {
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: memory.ErrNotInstitutional.Error()})
}
