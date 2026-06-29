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

// handleListMemories returns a paginated list of memories.
func (h *Handler) handleListMemories(w http.ResponseWriter, r *http.Request) {
	scope, err := parseWorkspaceScope(r)
	if err != nil {
		writeError(w, err)
		return
	}

	// "visible to me" mode: institutional + agent tiers plus the user's own,
	// excluding other users' private memories. Opt-in so the default list
	// stays strictly user-scoped (#1254).
	if r.URL.Query().Get("include_shared") == "true" {
		scope[memory.ScopeIncludeShared] = "true"
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
		Memories: wrapMemoriesWithTier(memories),
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
		// Recall queries routinely carry personal content ("what is my home
		// address") — log length, never the verbatim query (SEC-11).
		h.log.Error(err, "SearchMemories failed", "workspace", scope[memory.ScopeWorkspaceID], "queryLen", len(query))
		writeError(w, err)
		return
	}

	wrapped := wrapMemoriesWithPreview(memories, h.service.InlineThresholdBytes(r.Context()))
	related := h.service.RelatedForMemories(r.Context(), scope, memories)
	for _, mw := range wrapped {
		if mw.Memory != nil {
			if rels, ok := related[mw.ID]; ok {
				mw.Related = rels
			}
		}
	}

	writeJSON(w, MemoryListResponse{
		Memories: wrapped,
		Total:    len(wrapped),
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

	applySaveRequestMetadata(mem, &req)

	// Validate the resolved consent category against the registered set.
	// The predicate is wired only in the enterprise path; nil skips the check
	// (OSS default). We read from metadata rather than req.Category so both
	// the top-level field and an explicit metadata["consent_category"] value
	// are covered after applySaveRequestMetadata has merged them.
	if cat, ok := mem.Metadata[memory.MetaKeyConsentCategory].(string); ok && cat != "" {
		if h.categoryRegistered != nil && !h.categoryRegistered(cat) {
			writeError(w, ErrUnknownConsentCategory)
			return
		}
	}

	res, err := h.service.SaveMemoryWithResult(r.Context(), mem)
	if err != nil {
		h.log.Error(err, "SaveMemory failed")
		writeError(w, err)
		return
	}

	h.log.V(1).Info("memory saved",
		"memoryID", mem.ID,
		"type", mem.Type,
		"action", res.Action)
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(SaveMemoryResponse{
		Memory:                   *newMemoryWithTier(mem),
		Action:                   res.Action,
		SupersededObservationIDs: res.SupersededObservationIDs,
		SupersedeReason:          res.SupersedeReason,
		PotentialDuplicates:      res.PotentialDuplicates,
	})
}

// applySaveRequestMetadata propagates the top-level dedup / display fields
// (About, Title, Summary, Category) into mem.Metadata so the store's helpers
// pick them up at write time. The store keys (MetaKeyAboutKind,
// MetaKeyAboutKey, MetaKeyTitle, MetaKeySummary, MetaKeyConsentCategory) are
// the canonical carrier — these top-level fields are an ergonomic alias for
// callers that don't want to assemble a metadata map by hand. An explicit
// metadata value set by the caller wins (Title/Summary/Category don't
// overwrite). In OSS req.Category is always empty so that branch is a no-op.
func applySaveRequestMetadata(mem *memory.Memory, req *SaveMemoryRequest) {
	if req.About != nil && req.About.Kind != "" && req.About.Key != "" {
		ensureMetadata(mem)
		mem.Metadata[memory.MetaKeyAboutKind] = req.About.Kind
		mem.Metadata[memory.MetaKeyAboutKey] = req.About.Key
	}
	if req.Title != "" {
		setMetadataIfAbsent(mem, memory.MetaKeyTitle, req.Title)
	}
	if req.Summary != "" {
		setMetadataIfAbsent(mem, memory.MetaKeySummary, req.Summary)
	}
	if req.Category != "" {
		setMetadataIfAbsent(mem, memory.MetaKeyConsentCategory, req.Category)
	}
}

// ensureMetadata lazily initialises mem.Metadata so callers can write keys
// without repeating the nil check.
func ensureMetadata(mem *memory.Memory) {
	if mem.Metadata == nil {
		mem.Metadata = map[string]any{}
	}
}

// setMetadataIfAbsent writes value under key only when the caller has not
// already set it, preserving an explicit caller-supplied value.
func setMetadataIfAbsent(mem *memory.Memory, key, value string) {
	ensureMetadata(mem)
	if _, set := mem.Metadata[key]; !set {
		mem.Metadata[key] = value
	}
}

// handleOpenMemory returns the full content of a single memory by ID.
// Used by memory__open when the agent decides it needs the body of
// a large memory that recall returned only summarised. Mirrors the
// recall scope filter (workspace + user + agent).
func (h *Handler) handleOpenMemory(w http.ResponseWriter, r *http.Request) {
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

	mem, err := h.service.OpenMemory(r.Context(), scope, id)
	if err != nil {
		if errors.Is(err, memory.ErrNotFound) {
			http.Error(w, `{"error":"memory not found"}`, http.StatusNotFound)
			return
		}
		h.log.Error(err, "OpenMemory failed", "memoryID", id)
		writeError(w, err)
		return
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(MemoryResponse{Memory: mem})
}

// handleUpdateMemory atomically supersedes an entity's prior active
// observation and inserts a new one with the request body's content.
// Returns SaveMemoryResponse with action=auto_superseded so the
// agent can phrase its reply ("Updated X from Y to Z").
func (h *Handler) handleUpdateMemory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, ErrMissingMemoryID)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)

	var req UpdateMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
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

	res, err := h.service.UpdateMemory(r.Context(), id, mem)
	if err != nil {
		if errors.Is(err, memory.ErrNotFound) {
			http.Error(w, `{"error":"memory not found"}`, http.StatusNotFound)
			return
		}
		h.log.Error(err, "UpdateMemory failed", "memoryID", id)
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
