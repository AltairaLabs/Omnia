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
	"time"

	"github.com/altairalabs/omnia/internal/httputil"
)

// Errors for the aggregate + discover endpoints. The handler writes 400 with
// the textual message; the store/service wraps with a leading "postgres:" or
// similar prefix that we strip before user-facing exposure.
var (
	errAggregateBadGroupBy = errors.New(
		"groupBy must be one of: eval_id, eval_type, agent, time:hour, time:day")
	errAggregateBadMetric = errors.New(
		"metric must be one of: count, avg_score, p50_score, p95_score, avg_latency_ms, p95_latency_ms")
	errAggregateBadFrom = errors.New(
		"from must be RFC3339 (e.g. 2026-04-01T00:00:00Z)")
	errAggregateBadTo = errors.New(
		"to must be RFC3339 (e.g. 2026-04-01T00:00:00Z)")
	errAggregateMissingNamespace = errors.New("namespace query param is required")
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

	h.limitBody(w, r)
	var results []*EvalResult
	if err := json.NewDecoder(r.Body).Decode(&results); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
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

// handleGetEvalResultSummary returns aggregate statistics for eval results.
// GET /api/v1/sessions/{sessionID}/eval-results/summary
func (h *Handler) handleGetEvalResultSummary(w http.ResponseWriter, r *http.Request) {
	if h.evalService == nil {
		writeEvalError(w, ErrMissingEvalStore)
		return
	}

	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	opts := parseSummaryOpts(r)
	// Scope the summary to this session's agent/namespace by fetching the session first.
	// For now, the route is session-scoped so we pass the filters from query params.
	_ = sessionID // sessionID available for future per-session filtering

	summaries, err := h.evalService.GetEvalResultSummary(r.Context(), opts)
	if err != nil {
		writeEvalError(w, err)
		return
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	_ = json.NewEncoder(w).Encode(EvalResultSummaryResponse{Summaries: summaries})
}

// parseSummaryOpts extracts query parameters for eval result summary.
func parseSummaryOpts(r *http.Request) EvalResultSummaryOpts {
	q := r.URL.Query()
	return EvalResultSummaryOpts{
		AgentName: q.Get("agentName"),
		Namespace: q.Get("namespace"),
		EvalType:  q.Get("evalType"),
	}
}

// EvalAggregateResponse is the JSON body for /api/v1/eval-results/aggregate.
type EvalAggregateResponse struct {
	Rows []*EvalAggregateRow `json:"rows"`
}

// handleAggregateEvalResults runs a namespace-scoped GROUP BY over eval_results.
// GET /api/v1/eval-results/aggregate?namespace=X&groupBy=time:day&metric=avg_score
func (h *Handler) handleAggregateEvalResults(w http.ResponseWriter, r *http.Request) {
	if h.evalService == nil {
		writeEvalError(w, ErrMissingEvalStore)
		return
	}

	opts, err := parseEvalAggregateOpts(r)
	if err != nil {
		writeAggregateError(w, err)
		return
	}

	rows, err := h.evalService.AggregateEvalResults(r.Context(), opts)
	if err != nil {
		writeEvalError(w, err)
		return
	}
	if rows == nil {
		rows = []*EvalAggregateRow{}
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	_ = json.NewEncoder(w).Encode(EvalAggregateResponse{Rows: rows})
}

// handleDiscoverEvals returns the namespace-scoped set of evals + agents +
// promptpacks observed in eval_results. Replaces dashboard Prometheus
// metric-name and label-value discovery.
// GET /api/v1/eval-results/discover?namespace=X
func (h *Handler) handleDiscoverEvals(w http.ResponseWriter, r *http.Request) {
	if h.evalService == nil {
		writeEvalError(w, ErrMissingEvalStore)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		writeAggregateError(w, errAggregateMissingNamespace)
		return
	}

	result, err := h.evalService.EvalDiscovery(r.Context(), namespace)
	if err != nil {
		writeEvalError(w, err)
		return
	}
	if result == nil {
		result = &EvalDiscoveryResult{}
	}
	// Normalise nil slices to empty arrays so JSON renders [] not null —
	// dashboard consumers iterate without a null check.
	if result.Evals == nil {
		result.Evals = []EvalDescriptor{}
	}
	if result.Agents == nil {
		result.Agents = []string{}
	}
	if result.PromptPacks == nil {
		result.PromptPacks = []string{}
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	_ = json.NewEncoder(w).Encode(result)
}

// parseEvalAggregateOpts extracts EvalAggregateOpts from the request query.
// Returns one of the err* sentinels above for client-facing 400s.
func parseEvalAggregateOpts(r *http.Request) (EvalAggregateOpts, error) {
	q := r.URL.Query()

	namespace := q.Get("namespace")
	if namespace == "" {
		return EvalAggregateOpts{}, errAggregateMissingNamespace
	}

	groupBy, err := parseEvalGroupBy(q.Get("groupBy"))
	if err != nil {
		return EvalAggregateOpts{}, err
	}

	metric, err := parseEvalMetric(q.Get("metric"))
	if err != nil {
		return EvalAggregateOpts{}, err
	}

	opts := EvalAggregateOpts{
		Namespace:      namespace,
		AgentName:      q.Get("agentName"),
		PromptPackName: q.Get("promptpackName"),
		EvalID:         q.Get("evalId"),
		EvalType:       q.Get("evalType"),
		GroupBy:        groupBy,
		Metric:         metric,
		Limit:          clampEvalAggregateLimit(parseIntQueryParam(q.Get("limit"), DefaultEvalAggregateLimit)),
	}

	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return EvalAggregateOpts{}, errAggregateBadFrom
		}
		opts.From = t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return EvalAggregateOpts{}, errAggregateBadTo
		}
		opts.To = t
	}

	return opts, nil
}

func parseEvalGroupBy(v string) (EvalAggregateGroupBy, error) {
	switch EvalAggregateGroupBy(v) {
	case EvalAggregateGroupByEvalID,
		EvalAggregateGroupByEvalType,
		EvalAggregateGroupByAgent,
		EvalAggregateGroupByTimeHour,
		EvalAggregateGroupByTimeDay:
		return EvalAggregateGroupBy(v), nil
	default:
		return "", errAggregateBadGroupBy
	}
}

func parseEvalMetric(v string) (EvalAggregateMetric, error) {
	switch EvalAggregateMetric(v) {
	case EvalAggregateMetricCount,
		EvalAggregateMetricAvgScore,
		EvalAggregateMetricP50Score,
		EvalAggregateMetricP95Score,
		EvalAggregateMetricAvgLatencyMs,
		EvalAggregateMetricP95LatencyMs:
		return EvalAggregateMetric(v), nil
	default:
		return "", errAggregateBadMetric
	}
}

func clampEvalAggregateLimit(n int) int {
	if n < 1 {
		return DefaultEvalAggregateLimit
	}
	if n > MaxEvalAggregateLimit {
		return MaxEvalAggregateLimit
	}
	return n
}

func parseIntQueryParam(v string, fallback int) int {
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// writeAggregateError emits a 400 with the textual message for the err*
// sentinels above. Any other error falls through to writeError.
func writeAggregateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAggregateBadGroupBy),
		errors.Is(err, errAggregateBadMetric),
		errors.Is(err, errAggregateBadFrom),
		errors.Is(err, errAggregateBadTo),
		errors.Is(err, errAggregateMissingNamespace):
		w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
	default:
		writeError(w, err)
	}
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
