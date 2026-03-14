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
	"io"
	"net/http"
	"strconv"

	"github.com/altairalabs/omnia/internal/httputil"
)

// handleGetSessionEvalResults returns eval results for a specific session.
// GET /api/v1/sessions/{sessionID}/eval-results
func (h *Handler) handleGetSessionEvalResults(w http.ResponseWriter, r *http.Request) {
	if h.evalService == nil {
		writeEvalError(w, ErrMissingEvalStore)
		return
	}

	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	results, err := h.evalService.GetSessionEvalResults(r.Context(), sessionID)
	if err != nil {
		writeEvalError(w, err)
		return
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	_ = json.NewEncoder(w).Encode(EvalResultSessionResponse{Results: results})
}

// handleCreateEvalResults persists one or more eval results.
// POST /api/v1/eval-results
func (h *Handler) handleCreateEvalResults(w http.ResponseWriter, r *http.Request) {
	if h.evalService == nil {
		writeEvalError(w, ErrMissingEvalStore)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, h.maxBodySize))
	if err != nil {
		w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read request body"})
		return
	}

	var results []*EvalResult
	if err := json.Unmarshal(body, &results); err != nil {
		w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid JSON body"})
		return
	}

	if err := h.evalService.CreateEvalResults(r.Context(), results); err != nil {
		writeEvalError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// handleListEvalResults returns eval results matching query filters.
// GET /api/v1/eval-results
func (h *Handler) handleListEvalResults(w http.ResponseWriter, r *http.Request) {
	if h.evalService == nil {
		writeEvalError(w, ErrMissingEvalStore)
		return
	}

	opts := parseEvalListOpts(r)

	results, total, err := h.evalService.ListEvalResults(r.Context(), opts)
	if err != nil {
		writeEvalError(w, err)
		return
	}

	hasMore := int64(opts.Offset+opts.Limit) < total

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	_ = json.NewEncoder(w).Encode(EvalResultListResponse{
		Results: results,
		Total:   total,
		HasMore: hasMore,
	})
}

// handleEvaluateSession triggers eval execution for a stored session.
// POST /api/v1/sessions/{sessionID}/evaluate
// Returns 202 Accepted. Results are written asynchronously by the eval worker
// and can be retrieved via GET /api/v1/sessions/{sessionID}/eval-results.
func (h *Handler) handleEvaluateSession(w http.ResponseWriter, r *http.Request) {
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	if h.service == nil {
		w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "session service not configured"})
		return
	}

	if err := h.service.PublishEvaluateEvent(r.Context(), sessionID); err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(EvaluateAcceptedResponse{
		SessionID: sessionID,
		Message:   "evaluation queued",
	})
}

// EvaluateAcceptedResponse is the JSON response for accepted eval requests.
type EvaluateAcceptedResponse struct {
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
}

// parseEvalListOpts extracts query parameters for eval result listing.
func parseEvalListOpts(r *http.Request) EvalResultListOpts {
	q := r.URL.Query()
	opts := EvalResultListOpts{
		Limit:     defaultListLimit,
		AgentName: q.Get("agentName"),
		Namespace: q.Get("namespace"),
		EvalID:    q.Get("evalId"),
		EvalType:  q.Get("evalType"),
	}

	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > maxListLimit {
				n = maxListLimit
			}
			opts.Limit = n
		}
	}

	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			opts.Offset = n
		}
	}

	if v := q.Get("passed"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			opts.Passed = &b
		}
	}

	return opts
}
