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

	"github.com/altairalabs/omnia/internal/httputil"
	"github.com/altairalabs/omnia/internal/memory"
)

// handleExportMemories exports all memories for a scope (DSAR data subject access request).
func (h *Handler) handleExportMemories(w http.ResponseWriter, r *http.Request) {
	scope, err := parseWorkspaceScope(r)
	if err != nil {
		writeError(w, err)
		return
	}

	memories, err := h.service.ExportMemories(r.Context(), scope)
	if err != nil {
		h.log.Error(err, "ExportMemories failed", "workspace", scope[memory.ScopeWorkspaceID])
		writeError(w, err)
		return
	}

	h.log.V(1).Info("memories export served", "workspace", scope[memory.ScopeWorkspaceID], "count", len(memories))
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.Header().Set("Content-Disposition", exportFilename)
	_ = json.NewEncoder(w).Encode(MemoryListResponse{
		Memories: wrapMemoriesWithTier(memories),
		Total:    len(memories),
	})
}

// handleDeleteAllMemories deletes all memories for a scope (DSAR).
func (h *Handler) handleDeleteAllMemories(w http.ResponseWriter, r *http.Request) {
	scope, err := parseWorkspaceScope(r)
	if err != nil {
		writeError(w, err)
		return
	}

	if err := h.service.DeleteAllMemories(r.Context(), scope); err != nil {
		h.log.Error(err, "DeleteAllMemories failed", "workspace", scope[memory.ScopeWorkspaceID])
		writeError(w, err)
		return
	}

	h.log.V(1).Info("all memories deleted", "workspace", scope[memory.ScopeWorkspaceID])
	w.WriteHeader(http.StatusOK)
}

// handleBatchDeleteMemories deletes up to limit memories for a scope (paginated DSAR).
// Route: DELETE /api/v1/memories/batch?workspace=X&user_id=Y&limit=N
func (h *Handler) handleBatchDeleteMemories(w http.ResponseWriter, r *http.Request) {
	scope, err := parseWorkspaceScope(r)
	if err != nil {
		writeError(w, err)
		return
	}

	limit := parseIntParam(r, "limit", defaultBatchDeleteLimit)
	if limit > maxBatchDeleteLimit {
		limit = maxBatchDeleteLimit
	}

	n, err := h.service.BatchDeleteMemories(r.Context(), scope, limit)
	if err != nil {
		h.log.Error(err, "BatchDeleteMemories failed", "workspace", scope[memory.ScopeWorkspaceID])
		writeError(w, err)
		return
	}

	h.log.V(1).Info("batch memories deleted", "workspace", scope[memory.ScopeWorkspaceID], "count", n)
	writeJSON(w, BatchDeleteResponse{Deleted: n})
}
