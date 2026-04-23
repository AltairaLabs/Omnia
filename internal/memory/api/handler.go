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
	"strconv"
	"strings"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/httputil"
	"github.com/altairalabs/omnia/internal/memory"
)

// Handler constants.
const (
	defaultListLimit  = 20
	maxListLimit      = 100
	maxStringParamLen = 253

	// DefaultMaxBodySize is the maximum allowed request body size (16 MB).
	DefaultMaxBodySize int64 = 16 << 20

	// defaultBatchDeleteLimit is the default number of rows deleted per batch delete request.
	defaultBatchDeleteLimit = 500
	// maxBatchDeleteLimit is the maximum number of rows allowed per batch delete request.
	maxBatchDeleteLimit = 10000
)

// MemoryListResponse is the JSON response for memory list/search endpoints.
type MemoryListResponse struct {
	Memories []*memory.Memory `json:"memories"`
	Total    int              `json:"total"`
}

// MemoryResponse is the JSON response for a single memory creation.
type MemoryResponse struct {
	Memory *memory.Memory `json:"memory"`
}

// ErrorResponse is the JSON response for errors.
type ErrorResponse struct {
	Error string `json:"error"`
}

// BatchDeleteResponse is the JSON response for DELETE /api/v1/memories/batch.
type BatchDeleteResponse struct {
	Deleted int `json:"deleted"`
}

// SaveMemoryRequest is the JSON body for POST /api/v1/memories.
type SaveMemoryRequest struct {
	Type       string            `json:"type"`
	Content    string            `json:"content"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
	Confidence float64           `json:"confidence"`
	Scope      map[string]string `json:"scope"`
	SessionID  string            `json:"session_id,omitempty"`
	TurnRange  [2]int            `json:"turn_range,omitempty"`
	Category   string            `json:"category,omitempty"`
}

// Handler provides HTTP endpoints for the memory API.
type Handler struct {
	service     *MemoryService
	log         logr.Logger
	maxBodySize int64
}

// NewHandler creates a new memory API handler.
func NewHandler(service *MemoryService, log logr.Logger) *Handler {
	return &Handler{
		service:     service,
		log:         log.WithName("memory-handler"),
		maxBodySize: DefaultMaxBodySize,
	}
}

// RegisterRoutes registers all memory-api HTTP routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /api/v1/memories", h.handleListMemories)
	mux.HandleFunc("GET /api/v1/memories/search", h.handleSearchMemories)
	mux.HandleFunc("GET /api/v1/memories/export", h.handleExportMemories)
	mux.HandleFunc("POST /api/v1/memories", h.handleSaveMemory)
	mux.HandleFunc("DELETE /api/v1/memories/{id}", h.handleDeleteMemory)
	mux.HandleFunc("DELETE /api/v1/memories/batch", h.handleBatchDeleteMemories)
	mux.HandleFunc("DELETE /api/v1/memories", h.handleDeleteAllMemories)
	mux.HandleFunc("POST /api/v1/memories/retrieve", h.handleRetrieveMultiTier)

	mux.HandleFunc("POST /api/v1/institutional/memories", h.handleSaveInstitutional)
	mux.HandleFunc("GET /api/v1/institutional/memories", h.handleListInstitutional)
	mux.HandleFunc("DELETE /api/v1/institutional/memories/{id}", h.handleDeleteInstitutional)

	mux.HandleFunc("POST /api/v1/agent-memories", h.handleSaveAgentScoped)
	mux.HandleFunc("GET /api/v1/agent-memories", h.handleListAgentScoped)
	mux.HandleFunc("DELETE /api/v1/agent-memories/{id}", h.handleDeleteAgentScoped)

	mux.HandleFunc("GET /api/v1/compaction/candidates", h.handleListCompactionCandidates)
	mux.HandleFunc("POST /api/v1/compaction/summaries", h.handleSaveCompactionSummary)

	h.registerDocsRoutes(mux)
}

// handleListMemories returns a paginated list of memories.
func (h *Handler) handleListMemories(w http.ResponseWriter, r *http.Request) {
	scope, err := parseWorkspaceScope(r)
	if err != nil {
		writeError(w, err)
		return
	}

	opts := memory.ListOptions{
		Types:  parseTypes(r.URL.Query().Get("type")),
		Limit:  min(max(parseIntParam(r, "limit", defaultListLimit), 1), maxListLimit),
		Offset: parseIntParam(r, "offset", 0),
	}

	memories, err := h.service.ListMemories(r.Context(), scope, opts)
	if err != nil {
		h.log.Error(err, "ListMemories failed", "workspace", scope[memory.ScopeWorkspaceID])
		writeError(w, err)
		return
	}

	writeJSON(w, MemoryListResponse{
		Memories: memories,
		Total:    len(memories),
	})
}

// handleSearchMemories searches memories by query.
func (h *Handler) handleSearchMemories(w http.ResponseWriter, r *http.Request) {
	scope, err := parseWorkspaceScope(r)
	if err != nil {
		writeError(w, err)
		return
	}

	q := r.URL.Query()
	query := q.Get("q")
	if query == "" {
		writeError(w, ErrMissingQuery)
		return
	}

	opts := memory.RetrieveOptions{
		Types:         parseTypes(q.Get("type")),
		Limit:         min(max(parseIntParam(r, "limit", defaultListLimit), 1), maxListLimit),
		MinConfidence: parseMinConfidence(r),
	}

	memories, err := h.service.SearchMemories(r.Context(), scope, query, opts)
	if err != nil {
		h.log.Error(err, "SearchMemories failed", "workspace", scope[memory.ScopeWorkspaceID], "query", query)
		writeError(w, err)
		return
	}

	writeJSON(w, MemoryListResponse{
		Memories: memories,
		Total:    len(memories),
	})
}

// exportFilename is the Content-Disposition filename for DSAR exports.
const exportFilename = `attachment; filename="memories-export.json"`

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
		Memories: memories,
		Total:    len(memories),
	})
}

// handleSaveMemory creates or updates a memory.
func (h *Handler) handleSaveMemory(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)

	var req SaveMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}

	if req.Scope[memory.ScopeUserID] == "" {
		writeError(w, ErrMissingUserID)
		return
	}

	mem := &memory.Memory{
		Type:       req.Type,
		Content:    req.Content,
		Metadata:   req.Metadata,
		Confidence: req.Confidence,
		Scope:      req.Scope,
		SessionID:  req.SessionID,
		TurnRange:  req.TurnRange,
	}

	if err := h.service.SaveMemory(r.Context(), mem); err != nil {
		h.log.Error(err, "SaveMemory failed")
		writeError(w, err)
		return
	}

	h.log.V(1).Info("memory saved", "memoryID", mem.ID, "type", mem.Type)
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(MemoryResponse{Memory: mem})
}

// handleDeleteMemory soft-deletes a single memory.
func (h *Handler) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, ErrMissingMemoryID)
		return
	}

	scope, err := parseWorkspaceScope(r)
	if err != nil {
		writeError(w, err)
		return
	}

	if err := h.service.DeleteMemory(r.Context(), scope, id); err != nil {
		h.log.Error(err, "DeleteMemory failed", "memoryID", id)
		writeError(w, err)
		return
	}

	h.log.V(1).Info("memory deleted", "memoryID", id)
	w.WriteHeader(http.StatusOK)
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

// --- helpers -----------------------------------------------------------------

// parseWorkspaceScope extracts and validates the workspace parameter, then builds the full scope.
func parseWorkspaceScope(r *http.Request) (map[string]string, error) {
	q := r.URL.Query()
	workspace := truncateParam(q.Get("workspace"))
	if workspace == "" {
		return nil, ErrMissingWorkspace
	}
	return buildScope(q), nil
}

// buildScope constructs a scope map from query parameters.
func buildScope(q interface{ Get(string) string }) map[string]string {
	scope := map[string]string{
		memory.ScopeWorkspaceID: truncateParam(q.Get("workspace")),
	}
	if uid := q.Get("user_id"); uid != "" {
		scope[memory.ScopeUserID] = truncateParam(uid)
	}
	if agent := q.Get("agent"); agent != "" {
		scope[memory.ScopeAgentID] = truncateParam(agent)
	}
	return scope
}

// parseTypes splits a comma-separated type parameter into a slice.
func parseTypes(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// parseIntParam returns an integer query parameter or the default value.
func parseIntParam(r *http.Request, name string, defaultVal int) int {
	s := r.URL.Query().Get(name)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}
	return v
}

// parseMinConfidence extracts the min_confidence query parameter.
func parseMinConfidence(r *http.Request) float64 {
	s := r.URL.Query().Get("min_confidence")
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

// truncateParam silently truncates s to maxStringParamLen if it exceeds the limit.
func truncateParam(s string) string {
	if len(s) > maxStringParamLen {
		return s[:maxStringParamLen]
	}
	return s
}

// writeJSON writes a JSON 200 OK response.
func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	_ = json.NewEncoder(w).Encode(data)
}

// writeError maps known errors to HTTP status codes and writes a JSON error response.
func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	msg := "internal server error"

	switch {
	case errors.Is(err, ErrMissingWorkspace):
		status = http.StatusBadRequest
		msg = ErrMissingWorkspace.Error()
	case errors.Is(err, ErrMissingUserID):
		status = http.StatusBadRequest
		msg = ErrMissingUserID.Error()
	case errors.Is(err, ErrMissingQuery):
		status = http.StatusBadRequest
		msg = ErrMissingQuery.Error()
	case errors.Is(err, ErrMissingMemoryID):
		status = http.StatusBadRequest
		msg = ErrMissingMemoryID.Error()
	case errors.Is(err, ErrMissingBody):
		status = http.StatusBadRequest
		msg = ErrMissingBody.Error()
	case errors.Is(err, ErrBodyTooLarge) || isMaxBytesError(err):
		status = http.StatusRequestEntityTooLarge
		msg = ErrBodyTooLarge.Error()
	case errors.Is(err, ErrExpiresAtInPast):
		status = http.StatusBadRequest
		msg = ErrExpiresAtInPast.Error()
	case errors.Is(err, ErrMissingAgentID):
		status = http.StatusBadRequest
		msg = ErrMissingAgentID.Error()
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: msg})
}

// isMaxBytesError checks if the error is an http.MaxBytesError from MaxBytesReader.
func isMaxBytesError(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}
