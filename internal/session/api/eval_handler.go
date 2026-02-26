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
	"strings"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/httputil"
)

// EvalResultListResponse is the JSON response for eval result list endpoints.
type EvalResultListResponse struct {
	Results []*EvalResult `json:"results"`
	Total   int64         `json:"total"`
	HasMore bool          `json:"hasMore"`
}

// EvalResultSessionResponse is the JSON response for session eval results.
type EvalResultSessionResponse struct {
	Results []*EvalResult `json:"results"`
}

// EvalResultSummaryResponse is the JSON response for eval result summary.
type EvalResultSummaryResponse struct {
	Summaries []*EvalResultSummary `json:"summaries"`
}

// EvalHandler provides HTTP endpoints for eval result CRUD.
type EvalHandler struct {
	service *EvalService
	log     logr.Logger
}

// NewEvalHandler creates a new eval result handler.
func NewEvalHandler(service *EvalService, log logr.Logger) *EvalHandler {
	return &EvalHandler{
		service: service,
		log:     log.WithName("eval-handler"),
	}
}

// RegisterRoutes registers eval result routes on the given mux.
func (h *EvalHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/eval-results", h.handleCreateEvalResults)
	mux.HandleFunc("GET /api/v1/sessions/{id}/eval-results", h.handleGetSessionEvalResults)
	mux.HandleFunc("POST /api/v1/sessions/{id}/evaluate", h.handleEvaluateSession)
	mux.HandleFunc("GET /api/v1/eval-results/summary", h.handleGetEvalResultSummary)
	mux.HandleFunc("GET /api/v1/eval-results", h.handleListEvalResults)
}

// handleCreateEvalResults handles POST /api/v1/eval-results.
func (h *EvalHandler) handleCreateEvalResults(w http.ResponseWriter, r *http.Request) {
	var results []*EvalResult
	if err := json.NewDecoder(r.Body).Decode(&results); err != nil {
		writeError(w, ErrMissingBody)
		return
	}

	if err := h.service.CreateEvalResults(r.Context(), results); err != nil {
		h.log.Error(err, "CreateEvalResults failed")
		writeEvalError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// handleGetSessionEvalResults handles GET /api/v1/sessions/{id}/eval-results.
func (h *EvalHandler) handleGetSessionEvalResults(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeError(w, ErrMissingSessionID)
		return
	}

	results, err := h.service.GetSessionEvalResults(r.Context(), sessionID)
	if err != nil {
		h.log.Error(err, "GetSessionEvalResults failed", "sessionID", sessionID)
		writeEvalError(w, err)
		return
	}

	writeJSON(w, EvalResultSessionResponse{Results: results})
}

// handleEvaluateSession handles POST /api/v1/sessions/{id}/evaluate.
func (h *EvalHandler) handleEvaluateSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeError(w, ErrMissingSessionID)
		return
	}

	var req EvaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, ErrMissingBody)
		return
	}

	resp, err := h.service.EvaluateSession(r.Context(), sessionID, req.Evals)
	if err != nil {
		h.log.Error(err, "EvaluateSession failed", "sessionID", sessionID)
		writeEvalError(w, err)
		return
	}

	writeJSON(w, resp)
}

// handleListEvalResults handles GET /api/v1/eval-results.
func (h *EvalHandler) handleListEvalResults(w http.ResponseWriter, r *http.Request) {
	opts, err := parseEvalListParams(r)
	if err != nil {
		writeError(w, err)
		return
	}

	results, total, err := h.service.ListEvalResults(r.Context(), opts)
	if err != nil {
		h.log.Error(err, "ListEvalResults failed")
		writeEvalError(w, err)
		return
	}

	hasMore := int64(opts.Offset)+int64(len(results)) < total
	writeJSON(w, EvalResultListResponse{
		Results: results,
		Total:   total,
		HasMore: hasMore,
	})
}

// handleGetEvalResultSummary handles GET /api/v1/eval-results/summary.
func (h *EvalHandler) handleGetEvalResultSummary(w http.ResponseWriter, r *http.Request) {
	opts, err := parseEvalSummaryParams(r)
	if err != nil {
		writeError(w, err)
		return
	}

	summaries, err := h.service.GetEvalResultSummary(r.Context(), opts)
	if err != nil {
		h.log.Error(err, "GetEvalResultSummary failed")
		writeEvalError(w, err)
		return
	}

	writeJSON(w, EvalResultSummaryResponse{Summaries: summaries})
}

// parseEvalListParams extracts query parameters for eval result listing.
func parseEvalListParams(r *http.Request) (EvalResultListOpts, error) {
	q := r.URL.Query()
	opts := EvalResultListOpts{
		Limit:     min(parseIntParam(r, "limit", defaultListLimit), maxListLimit),
		Offset:    parseIntParam(r, "offset", 0),
		AgentName: q.Get("agent_name"),
		Namespace: q.Get("namespace"),
		EvalID:    q.Get("eval_id"),
	}

	if passed := q.Get("passed"); passed != "" {
		val := strings.EqualFold(passed, "true")
		opts.Passed = &val
	}

	if after := q.Get("created_after"); after != "" {
		t, err := parseTimeParam(after)
		if err != nil {
			return opts, err
		}
		opts.CreatedAfter = t
	}

	if before := q.Get("created_before"); before != "" {
		t, err := parseTimeParam(before)
		if err != nil {
			return opts, err
		}
		opts.CreatedBefore = t
	}

	return opts, nil
}

// parseEvalSummaryParams extracts query parameters for eval result summary.
func parseEvalSummaryParams(r *http.Request) (EvalResultSummaryOpts, error) {
	q := r.URL.Query()
	opts := EvalResultSummaryOpts{
		AgentName: q.Get("agent_name"),
		Namespace: q.Get("namespace"),
	}

	if after := q.Get("created_after"); after != "" {
		t, err := parseTimeParam(after)
		if err != nil {
			return opts, err
		}
		opts.CreatedAfter = t
	}

	if before := q.Get("created_before"); before != "" {
		t, err := parseTimeParam(before)
		if err != nil {
			return opts, err
		}
		opts.CreatedBefore = t
	}

	return opts, nil
}

// writeEvalError maps eval-specific errors to HTTP responses.
func writeEvalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrMissingEvalResults),
		errors.Is(err, ErrMissingEvalDefinition):
		w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
	case errors.Is(err, ErrNoMessages):
		w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
	case errors.Is(err, ErrMissingEvalStore):
		w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "eval store not configured"})
	default:
		writeError(w, err)
	}
}
