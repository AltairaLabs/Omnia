/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/altairalabs/omnia/internal/httputil"
)

// Errors for the provider-calls aggregate + discover endpoints.
var (
	errProviderCallsBadGroupBy = errors.New(
		"groupBy must be a comma-separated list of: provider, model, agent, time:hour, time:day")
	errProviderCallsBadMetric = errors.New(
		"metric must be one of: count, sum_cost_usd, sum_input_tokens, sum_output_tokens, sum_cached_tokens, sum_tokens, avg_duration_ms, p95_duration_ms")
)

// ProviderCallsAggregateResponse is the JSON body for
// /api/v1/provider-calls/aggregate.
type ProviderCallsAggregateResponse struct {
	Rows []*ProviderCallAggregateRow `json:"rows"`
}

// handleAggregateProviderCalls runs a namespace-scoped GROUP BY over
// provider_calls. GET /api/v1/provider-calls/aggregate?namespace=X&groupBy=Y&metric=Z
func (h *Handler) handleAggregateProviderCalls(w http.ResponseWriter, r *http.Request) {
	if h.providerCallsService == nil {
		writeProviderCallsError(w, ErrMissingProviderCallsStore)
		return
	}

	opts, err := parseProviderCallsAggregateOpts(r)
	if err != nil {
		writeProviderCallsAggregateError(w, err)
		return
	}

	rows, err := h.providerCallsService.AggregateProviderCalls(r.Context(), opts)
	if err != nil {
		writeProviderCallsError(w, err)
		return
	}
	if rows == nil {
		rows = []*ProviderCallAggregateRow{}
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	_ = json.NewEncoder(w).Encode(ProviderCallsAggregateResponse{Rows: rows})
}

// handleDiscoverProviderCalls returns the distinct (provider, model) values
// for the namespace's provider_calls rows.
// GET /api/v1/provider-calls/discover?namespace=X
func (h *Handler) handleDiscoverProviderCalls(w http.ResponseWriter, r *http.Request) {
	if h.providerCallsService == nil {
		writeProviderCallsError(w, ErrMissingProviderCallsStore)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		writeAggregateError(w, errAggregateMissingNamespace)
		return
	}

	res, err := h.providerCallsService.ProviderCallsDiscovery(r.Context(), namespace)
	if err != nil {
		writeProviderCallsError(w, err)
		return
	}
	if res == nil {
		res = &ProviderCallDiscoveryResult{}
	}
	if res.Providers == nil {
		res.Providers = []string{}
	}
	if res.Models == nil {
		res.Models = []string{}
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	_ = json.NewEncoder(w).Encode(res)
}

// parseProviderCallsAggregateOpts extracts ProviderCallAggregateOpts from
// the request query. Returns one of the err* sentinels above for 400s.
func parseProviderCallsAggregateOpts(r *http.Request) (ProviderCallAggregateOpts, error) {
	q := r.URL.Query()

	namespace := q.Get("namespace")
	if namespace == "" {
		return ProviderCallAggregateOpts{}, errAggregateMissingNamespace
	}

	groupBy, err := parseProviderCallsGroupByList(q.Get("groupBy"))
	if err != nil {
		return ProviderCallAggregateOpts{}, err
	}

	metric, err := parseProviderCallsMetric(q.Get("metric"))
	if err != nil {
		return ProviderCallAggregateOpts{}, err
	}

	opts := ProviderCallAggregateOpts{
		Namespace: namespace,
		AgentName: q.Get("agentName"),
		Provider:  q.Get("provider"),
		Model:     q.Get("model"),
		GroupBy:   groupBy,
		Metric:    metric,
		Limit:     clampProviderCallsAggregateLimit(parseIntQueryParam(q.Get("limit"), DefaultProviderCallAggregateLimit)),
	}

	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return ProviderCallAggregateOpts{}, errAggregateBadFrom
		}
		opts.From = t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return ProviderCallAggregateOpts{}, errAggregateBadTo
		}
		opts.To = t
	}

	return opts, nil
}

// validateProviderCallsGroupBy returns the typed dimension or an error.
func validateProviderCallsGroupBy(v string) (ProviderCallAggregateGroupBy, error) {
	switch ProviderCallAggregateGroupBy(v) {
	case ProviderCallAggregateGroupByProvider,
		ProviderCallAggregateGroupByModel,
		ProviderCallAggregateGroupByAgent,
		ProviderCallAggregateGroupByTimeHour,
		ProviderCallAggregateGroupByTimeDay:
		return ProviderCallAggregateGroupBy(v), nil
	default:
		return "", errProviderCallsBadGroupBy
	}
}

// parseProviderCallsGroupByList parses a comma-separated groupBy into an
// ordered list of validated dimensions. Each dimension produces one segment
// of the composite aggregate key. An empty or all-blank value is rejected.
func parseProviderCallsGroupByList(v string) ([]ProviderCallAggregateGroupBy, error) {
	parts := strings.Split(v, ",")
	out := make([]ProviderCallAggregateGroupBy, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		dim, err := validateProviderCallsGroupBy(p)
		if err != nil {
			return nil, err
		}
		out = append(out, dim)
	}
	if len(out) == 0 {
		return nil, errProviderCallsBadGroupBy
	}
	return out, nil
}

func parseProviderCallsMetric(v string) (ProviderCallAggregateMetric, error) {
	switch ProviderCallAggregateMetric(v) {
	case ProviderCallAggregateMetricCount,
		ProviderCallAggregateMetricSumCostUSD,
		ProviderCallAggregateMetricSumInputTokens,
		ProviderCallAggregateMetricSumOutputTokens,
		ProviderCallAggregateMetricSumCachedTokens,
		ProviderCallAggregateMetricSumTokens,
		ProviderCallAggregateMetricAvgDurationMs,
		ProviderCallAggregateMetricP95DurationMs:
		return ProviderCallAggregateMetric(v), nil
	default:
		return "", errProviderCallsBadMetric
	}
}

func clampProviderCallsAggregateLimit(n int) int {
	if n < 1 {
		return DefaultProviderCallAggregateLimit
	}
	if n > MaxProviderCallAggregateLimit {
		return MaxProviderCallAggregateLimit
	}
	return n
}

// writeProviderCallsAggregateError emits 400 for the err* sentinels above;
// other errors fall through to writeError.
func writeProviderCallsAggregateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errProviderCallsBadGroupBy),
		errors.Is(err, errProviderCallsBadMetric),
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

// writeProviderCallsError maps store-not-configured errors to 503; everything
// else falls through to writeError.
func writeProviderCallsError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrMissingProviderCallsStore) {
		w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "provider calls store not configured"})
		return
	}
	writeError(w, err)
}
