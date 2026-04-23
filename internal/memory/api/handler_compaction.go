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

// Compaction endpoint defaults.
const (
	defaultCompactionOlderThanHours = 720 // 30 days
	defaultCompactionMaxCandidates  = 20
	maxCompactionMaxCandidates      = 200
	defaultCompactionMaxPerBucket   = 50
	maxCompactionMaxPerBucket       = 500
	defaultCompactionMinGroupSize   = 10
)

// CompactionEntryResponse is the wire form of a single observation in a
// compaction candidate. Mirrors memory.CompactionEntry with JSON tags so the
// summarizer agent can parse buckets over HTTP.
type CompactionEntryResponse struct {
	EntityID      string    `json:"entity_id"`
	ObservationID string    `json:"observation_id"`
	Kind          string    `json:"kind"`
	Content       string    `json:"content"`
	ObservedAt    time.Time `json:"observed_at"`
}

// CompactionCandidateResponse is the wire form of one bucket returned to
// the summarizer agent. Mirrors memory.CompactionCandidate with JSON tags.
type CompactionCandidateResponse struct {
	WorkspaceID    string                    `json:"workspace_id"`
	UserID         string                    `json:"user_id,omitempty"`
	AgentID        string                    `json:"agent_id,omitempty"`
	ObservationIDs []string                  `json:"observation_ids"`
	Entries        []CompactionEntryResponse `json:"entries"`
}

// ListCompactionCandidatesResponse is the JSON response for
// GET /api/v1/compaction/candidates.
type ListCompactionCandidatesResponse struct {
	Candidates []CompactionCandidateResponse `json:"candidates"`
	Total      int                           `json:"total"`
}

// SaveCompactionSummaryRequest is the JSON body for
// POST /api/v1/compaction/summaries. Mirrors memory.CompactionSummary.
type SaveCompactionSummaryRequest struct {
	WorkspaceID            string   `json:"workspace_id"`
	UserID                 string   `json:"user_id,omitempty"`
	AgentID                string   `json:"agent_id,omitempty"`
	Kind                   string   `json:"kind,omitempty"`
	Content                string   `json:"content"`
	Confidence             float64  `json:"confidence,omitempty"`
	SupersededObservations []string `json:"superseded_observation_ids"`
}

// SaveCompactionSummaryResponse is the JSON response for
// POST /api/v1/compaction/summaries.
type SaveCompactionSummaryResponse struct {
	SummaryEntityID string `json:"summary_entity_id"`
}

// handleListCompactionCandidates handles
// GET /api/v1/compaction/candidates?workspace=X[&older_than_hours=720][&limit=20][&max_per_bucket=50][&min_group_size=10].
//
// The endpoint is read-only from the caller's perspective — it never
// mutates memory rows. The summarizer agent calls this to discover buckets
// of stale observations it should summarize, then calls
// POST /api/v1/compaction/summaries to persist the result.
func (h *Handler) handleListCompactionCandidates(w http.ResponseWriter, r *http.Request) {
	workspace := truncateParam(r.URL.Query().Get("workspace"))
	if workspace == "" {
		writeError(w, ErrMissingWorkspace)
		return
	}
	opts := parseCompactionCandidateOptions(r, workspace)

	candidates, err := h.service.FindCompactionCandidates(r.Context(), opts)
	if err != nil {
		h.log.Error(err, "FindCompactionCandidates failed", "workspace", workspace)
		writeError(w, err)
		return
	}
	writeJSON(w, ListCompactionCandidatesResponse{
		Candidates: toCompactionCandidateResponses(candidates),
		Total:      len(candidates),
	})
}

// handleSaveCompactionSummary handles POST /api/v1/compaction/summaries.
// Returns 409 Conflict with the ErrCompactionRaced sentinel when another
// writer superseded the target observations first — the summarizer agent
// should treat that as a no-op, not a failure, and move on to the next
// bucket.
func (h *Handler) handleSaveCompactionSummary(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBodySize)

	var req SaveCompactionSummaryRequest
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

	summary := memory.CompactionSummary{
		WorkspaceID:            req.WorkspaceID,
		UserID:                 req.UserID,
		AgentID:                req.AgentID,
		Kind:                   req.Kind,
		Content:                req.Content,
		Confidence:             req.Confidence,
		SupersededObservations: req.SupersededObservations,
	}

	id, err := h.service.SaveCompactionSummary(r.Context(), summary)
	if err != nil {
		if errors.Is(err, memory.ErrCompactionRaced) {
			writeCompactionRacedResponse(w)
			return
		}
		h.log.Error(err, "SaveCompactionSummary failed", "workspace", req.WorkspaceID)
		writeError(w, err)
		return
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(SaveCompactionSummaryResponse{SummaryEntityID: id})
}

// writeCompactionRacedResponse emits 409 Conflict with the sentinel error.
// Agents should treat this as idempotent no-op and move on.
func writeCompactionRacedResponse(w http.ResponseWriter) {
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: memory.ErrCompactionRaced.Error()})
}

// parseCompactionCandidateOptions reads compaction query params and returns
// a validated options struct with caps applied.
func parseCompactionCandidateOptions(r *http.Request, workspace string) memory.FindCompactionCandidatesOptions {
	olderThanHours := parseIntParam(r, "older_than_hours", defaultCompactionOlderThanHours)
	if olderThanHours < 1 {
		olderThanHours = defaultCompactionOlderThanHours
	}
	maxCandidates := clampCompactionParam(r, "limit",
		defaultCompactionMaxCandidates, maxCompactionMaxCandidates)
	maxPerBucket := clampCompactionParam(r, "max_per_bucket",
		defaultCompactionMaxPerBucket, maxCompactionMaxPerBucket)
	minGroupSize := parseIntParam(r, "min_group_size", defaultCompactionMinGroupSize)
	if minGroupSize < 1 {
		minGroupSize = defaultCompactionMinGroupSize
	}

	return memory.FindCompactionCandidatesOptions{
		WorkspaceID:     workspace,
		OlderThan:       time.Now().Add(-time.Duration(olderThanHours) * time.Hour),
		MinGroupSize:    minGroupSize,
		MaxCandidates:   maxCandidates,
		MaxPerCandidate: maxPerBucket,
	}
}

// clampCompactionParam applies the paired default/max contract used by both
// the candidate-count and entries-per-bucket knobs.
func clampCompactionParam(r *http.Request, name string, def, capN int) int {
	v := parseIntParam(r, name, def)
	if v < 1 {
		return def
	}
	if v > capN {
		return capN
	}
	return v
}

// toCompactionCandidateResponses converts store-level candidates into the
// JSON-tagged HTTP shape.
func toCompactionCandidateResponses(in []memory.CompactionCandidate) []CompactionCandidateResponse {
	if in == nil {
		return []CompactionCandidateResponse{}
	}
	out := make([]CompactionCandidateResponse, 0, len(in))
	for _, c := range in {
		out = append(out, CompactionCandidateResponse{
			WorkspaceID:    c.WorkspaceID,
			UserID:         c.UserID,
			AgentID:        c.AgentID,
			ObservationIDs: c.ObservationIDs,
			Entries:        toCompactionEntryResponses(c.Entries),
		})
	}
	return out
}

func toCompactionEntryResponses(in []memory.CompactionEntry) []CompactionEntryResponse {
	if in == nil {
		return []CompactionEntryResponse{}
	}
	out := make([]CompactionEntryResponse, 0, len(in))
	for _, e := range in {
		out = append(out, CompactionEntryResponse{
			EntityID:      e.EntityID,
			ObservationID: e.ObservationID,
			Kind:          e.Kind,
			Content:       e.Content,
			ObservedAt:    e.ObservedAt,
		})
	}
	return out
}
