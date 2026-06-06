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
	"github.com/altairalabs/omnia/internal/memory/ingestion"
)

// Ingest-summary work-queue endpoint defaults.
const (
	defaultSummaryCandidates = 20
	maxSummaryCandidates     = 200
)

// SummaryCandidateResponse is the wire form of one pending document-summary
// work item handed to the summarizer agent.
type SummaryCandidateResponse struct {
	WorkspaceID string `json:"workspace_id"`
	AboutKey    string `json:"about_key"`
	Title       string `json:"title,omitempty"`
	URL         string `json:"url,omitempty"`
	Site        string `json:"site,omitempty"`
	Text        string `json:"text"`
	Strategy    string `json:"strategy"`
}

// ListSummaryCandidatesResponse is the JSON response for
// GET /api/v1/ingest/summary-candidates.
type ListSummaryCandidatesResponse struct {
	Candidates []SummaryCandidateResponse `json:"candidates"`
	Total      int                        `json:"total"`
}

// SaveDocumentSummaryRequest is the JSON body for POST /api/v1/ingest/summaries.
type SaveDocumentSummaryRequest struct {
	WorkspaceID string `json:"workspace_id"`
	AboutKey    string `json:"about_key"`
	Summary     string `json:"summary"`
}

// SaveDocumentSummaryResponse is the JSON response for the same.
type SaveDocumentSummaryResponse struct {
	StoredItems int `json:"stored_items"`
}

// handleListSummaryCandidates serves
// GET /api/v1/ingest/summary-candidates?limit=20. Read-only; the summarizer
// agent polls this to discover documents awaiting summarization.
func (h *Handler) handleListSummaryCandidates(w http.ResponseWriter, r *http.Request) {
	limit := clampCompactionParam(r, "limit", defaultSummaryCandidates, maxSummaryCandidates)
	items, err := h.service.ListSummaryCandidates(r.Context(), limit)
	if err != nil {
		h.log.Error(err, "ListSummaryCandidates failed")
		writeError(w, err)
		return
	}
	writeJSON(w, ListSummaryCandidatesResponse{
		Candidates: toSummaryCandidateResponses(items),
		Total:      len(items),
	})
}

// handleSaveDocumentSummary serves POST /api/v1/ingest/summaries. The agent
// posts the summary it produced for a pending work item; memory-api stores it
// (chunking first for summaryThenChunk) and completes the item.
func (h *Handler) handleSaveDocumentSummary(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)
	var req SaveDocumentSummaryRequest
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

	stored, err := h.service.SaveDocumentSummary(r.Context(), req.WorkspaceID, req.AboutKey, req.Summary)
	if err != nil {
		if errors.Is(err, ingestion.ErrWorkItemNotFound) {
			writeNotFound(w, err)
			return
		}
		h.log.Error(err, "SaveDocumentSummary failed", "workspace", req.WorkspaceID, "aboutKey", req.AboutKey)
		writeError(w, err)
		return
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(SaveDocumentSummaryResponse{StoredItems: stored})
}

// writeNotFound emits 404 with the error message.
func writeNotFound(w http.ResponseWriter, err error) {
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
}

// toSummaryCandidateResponses maps work items to their JSON-tagged HTTP shape.
func toSummaryCandidateResponses(in []ingestion.WorkItem) []SummaryCandidateResponse {
	out := make([]SummaryCandidateResponse, 0, len(in))
	for _, it := range in {
		out = append(out, SummaryCandidateResponse{
			WorkspaceID: it.WorkspaceID,
			AboutKey:    it.AboutKey,
			Title:       it.Doc.Title,
			URL:         it.Doc.URL,
			Site:        it.Doc.Site,
			Text:        it.Doc.Text,
			Strategy:    it.Strategy,
		})
	}
	return out
}
