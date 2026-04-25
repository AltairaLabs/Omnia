/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"net/http"
	"time"

	"github.com/altairalabs/omnia/internal/httputil"
	"github.com/altairalabs/omnia/internal/memory"
)

// httpError is a small wrapper that writeError recognises for status + body.
// Per-handler 400s use this to carry specific messages without adding a
// sentinel error per case.
type httpError struct {
	status int
	msg    string
}

func (e httpError) Error() string { return e.msg }

// Sentinel errors for the aggregate handler.
var (
	errAggregateBadGroupBy = httpError{
		status: http.StatusBadRequest,
		msg:    "groupBy must be one of: category, agent, day, tier",
	}
	errAggregateBadMetric = httpError{
		status: http.StatusBadRequest,
		msg:    "metric must be one of: count, distinct_users",
	}
	errAggregateBadFrom = httpError{
		status: http.StatusBadRequest,
		msg:    "from must be RFC3339 (e.g. 2026-04-01T00:00:00Z)",
	}
	errAggregateBadTo = httpError{
		status: http.StatusBadRequest,
		msg:    "to must be RFC3339 (e.g. 2026-04-01T00:00:00Z)",
	}
)

// handleMemoryAggregate handles GET /api/v1/memories/aggregate.
func (h *Handler) handleMemoryAggregate(w http.ResponseWriter, r *http.Request) {
	opts, err := parseAggregateOptions(r)
	if err != nil {
		writeError(w, err)
		return
	}

	rows, err := h.service.AggregateMemories(r.Context(), opts)
	if err != nil {
		h.log.Error(err, "AggregateMemories failed",
			"workspace", opts.Workspace,
			"groupBy", opts.GroupBy,
			"metric", opts.Metric)
		writeError(w, err)
		return
	}

	if rows == nil {
		rows = []memory.AggregateRow{}
	}
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	writeJSON(w, rows)
}

// parseAggregateOptions extracts AggregateOptions from the request.
// Returns httpError values for client-facing 400s.
func parseAggregateOptions(r *http.Request) (memory.AggregateOptions, error) {
	q := r.URL.Query()

	workspace := truncateParam(q.Get("workspace"))
	if workspace == "" {
		return memory.AggregateOptions{}, ErrMissingWorkspace
	}

	groupBy, err := parseGroupBy(q.Get("groupBy"))
	if err != nil {
		return memory.AggregateOptions{}, err
	}

	metric, err := parseMetric(q.Get("metric"))
	if err != nil {
		return memory.AggregateOptions{}, err
	}

	opts := memory.AggregateOptions{
		Workspace: workspace,
		GroupBy:   groupBy,
		Metric:    metric,
		Limit:     clampAggregateLimit(parseIntParam(r, "limit", memory.DefaultAggregateLimit)),
	}

	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return memory.AggregateOptions{}, errAggregateBadFrom
		}
		opts.From = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return memory.AggregateOptions{}, errAggregateBadTo
		}
		opts.To = &t
	}

	return opts, nil
}

func parseGroupBy(v string) (memory.AggregateGroupBy, error) {
	switch memory.AggregateGroupBy(v) {
	case memory.AggregateGroupByCategory,
		memory.AggregateGroupByAgent,
		memory.AggregateGroupByDay,
		memory.AggregateGroupByTier:
		return memory.AggregateGroupBy(v), nil
	default:
		return "", errAggregateBadGroupBy
	}
}

func parseMetric(v string) (memory.AggregateMetric, error) {
	switch v {
	case "":
		return memory.AggregateMetricCount, nil
	case string(memory.AggregateMetricCount), string(memory.AggregateMetricDistinctUsers):
		return memory.AggregateMetric(v), nil
	default:
		return "", errAggregateBadMetric
	}
}

func clampAggregateLimit(n int) int {
	if n < 1 {
		return 1
	}
	if n > memory.MaxAggregateLimit {
		return memory.MaxAggregateLimit
	}
	return n
}
