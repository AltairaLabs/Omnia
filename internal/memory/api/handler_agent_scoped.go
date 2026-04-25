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

// SaveAgentScopedRequest is the JSON body for
// POST /api/v1/agent-memories.
//
// Agent-scoped admin memories are operator-curated rows that live at the
// agent tier — every session against the named agent sees them regardless
// of user. ExpiresAt follows the same semantics as the institutional path:
// optional, not defaulted, past values are rejected.
type SaveAgentScopedRequest struct {
	WorkspaceID string         `json:"workspace_id"`
	AgentID     string         `json:"agent_id"`
	Type        string         `json:"type"`
	Content     string         `json:"content"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Confidence  float64        `json:"confidence"`
}

// ListAgentScopedResponse is the JSON response for
// GET /api/v1/agent-memories.
type ListAgentScopedResponse struct {
	Memories []*MemoryWithTier `json:"memories"`
	Total    int               `json:"total"`
}

// handleSaveAgentScoped handles POST /api/v1/agent-memories.
// Writes are restricted to (workspace, agent) scope (no user_id).
func (h *Handler) handleSaveAgentScoped(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)

	var req SaveAgentScopedRequest
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
	if req.AgentID == "" {
		writeError(w, ErrMissingAgentID)
		return
	}

	mem := &memory.Memory{
		Type:       req.Type,
		Content:    req.Content,
		Metadata:   req.Metadata,
		Confidence: req.Confidence,
		Scope: map[string]string{
			memory.ScopeWorkspaceID: req.WorkspaceID,
			memory.ScopeAgentID:     req.AgentID,
		},
	}
	if err := h.service.SaveAgentScopedMemory(r.Context(), mem); err != nil {
		h.log.Error(err, "SaveAgentScopedMemory failed",
			"workspace", req.WorkspaceID, "agent", req.AgentID)
		writeError(w, err)
		return
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(MemoryResponse{Memory: mem})
}

// handleListAgentScoped handles GET /api/v1/agent-memories?workspace=X&agent=Y.
func (h *Handler) handleListAgentScoped(w http.ResponseWriter, r *http.Request) {
	workspace := truncateParam(r.URL.Query().Get("workspace"))
	if workspace == "" {
		writeError(w, ErrMissingWorkspace)
		return
	}
	agentID := truncateParam(r.URL.Query().Get("agent"))
	if agentID == "" {
		writeError(w, ErrMissingAgentID)
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

	mems, err := h.service.ListAgentScopedMemories(r.Context(), workspace, agentID, opts)
	if err != nil {
		h.log.Error(err, "ListAgentScopedMemories failed",
			"workspace", workspace, "agent", agentID)
		writeError(w, err)
		return
	}
	writeJSON(w, ListAgentScopedResponse{
		Memories: wrapMemoriesWithTier(mems),
		Total:    len(mems),
	})
}

// handleDeleteAgentScoped handles DELETE /api/v1/agent-memories/{id}?workspace=X&agent=Y.
// Returns 400 (not 500) when the target row isn't agent-scoped so callers
// can distinguish operator misuse from infrastructure failures.
func (h *Handler) handleDeleteAgentScoped(w http.ResponseWriter, r *http.Request) {
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
	agentID := truncateParam(r.URL.Query().Get("agent"))
	if agentID == "" {
		writeError(w, ErrMissingAgentID)
		return
	}

	err := h.service.DeleteAgentScopedMemory(r.Context(), workspace, agentID, id)
	if err != nil {
		if errors.Is(err, memory.ErrNotAgentScoped) {
			writeNotAgentScopedError(w)
			return
		}
		h.log.Error(err, "DeleteAgentScopedMemory failed",
			"workspace", workspace, "agent", agentID, "memoryID", id)
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// writeNotAgentScopedError emits a 400 with the sentinel message — kept
// separate from writeError so the sentinel from the memory package doesn't
// leak into the generic error switch.
func writeNotAgentScopedError(w http.ResponseWriter) {
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: memory.ErrNotAgentScoped.Error()})
}
