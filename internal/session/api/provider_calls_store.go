/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"
	"time"
)

// ProviderCallAggregateGroupBy enumerates valid groupBy values for
// AggregateProviderCalls. Time buckets sort ASC (chronological); categorical
// groups sort DESC by value.
type ProviderCallAggregateGroupBy string

const (
	// ProviderCallAggregateGroupByProvider groups by the provider column.
	ProviderCallAggregateGroupByProvider ProviderCallAggregateGroupBy = "provider"
	// ProviderCallAggregateGroupByModel groups by the model column.
	ProviderCallAggregateGroupByModel ProviderCallAggregateGroupBy = "model"
	// ProviderCallAggregateGroupByAgent groups by sessions.agent_name (joined).
	ProviderCallAggregateGroupByAgent ProviderCallAggregateGroupBy = "agent"
	// ProviderCallAggregateGroupByTimeHour buckets created_at by hour (UTC),
	// formatted as RFC3339 hour-precision strings.
	ProviderCallAggregateGroupByTimeHour ProviderCallAggregateGroupBy = "time:hour"
	// ProviderCallAggregateGroupByTimeDay buckets created_at by day (UTC),
	// formatted as YYYY-MM-DD strings.
	ProviderCallAggregateGroupByTimeDay ProviderCallAggregateGroupBy = "time:day"
)

// ProviderCallAggregateMetric enumerates the metrics over provider_calls.
type ProviderCallAggregateMetric string

const (
	ProviderCallAggregateMetricCount           ProviderCallAggregateMetric = "count"
	ProviderCallAggregateMetricSumCostUSD      ProviderCallAggregateMetric = "sum_cost_usd"
	ProviderCallAggregateMetricSumInputTokens  ProviderCallAggregateMetric = "sum_input_tokens"
	ProviderCallAggregateMetricSumOutputTokens ProviderCallAggregateMetric = "sum_output_tokens"
	ProviderCallAggregateMetricSumCachedTokens ProviderCallAggregateMetric = "sum_cached_tokens"
	ProviderCallAggregateMetricSumTokens       ProviderCallAggregateMetric = "sum_tokens"
	ProviderCallAggregateMetricAvgDurationMs   ProviderCallAggregateMetric = "avg_duration_ms"
	ProviderCallAggregateMetricP95DurationMs   ProviderCallAggregateMetric = "p95_duration_ms"
)

// Limits for AggregateProviderCalls.
const (
	DefaultProviderCallAggregateLimit = 500
	MaxProviderCallAggregateLimit     = 5000
)

// ProviderCallAggregateRow is one returned row from AggregateProviderCalls.
type ProviderCallAggregateRow struct {
	Key   string  `json:"key"`
	Value float64 `json:"value"`
	Count int64   `json:"count"`
}

// ProviderCallAggregateOpts configures the AggregateProviderCalls query.
// Namespace is the scoping field (matched against sessions.namespace via
// an INNER JOIN). GroupBy and Metric are required; all other fields are
// optional filters.
type ProviderCallAggregateOpts struct {
	Namespace string // required (sessions.namespace)
	AgentName string // optional (sessions.agent_name)
	Provider  string // optional (provider_calls.provider)
	Model     string // optional (provider_calls.model)
	From      time.Time
	To        time.Time
	GroupBy   []ProviderCallAggregateGroupBy // required, one or more dimensions (composite key)
	Metric    ProviderCallAggregateMetric    // required
	Limit     int
}

// ProviderCallDiscoveryResult is the namespace-scoped discovery payload —
// the distinct provider + model values seen in this namespace's
// provider_calls rows. Slices are non-nil so callers can JSON-serialise
// without a null check.
type ProviderCallDiscoveryResult struct {
	Providers []string `json:"providers"`
	Models    []string `json:"models"`
}

// ProviderCallsStore defines the persistence interface for cross-cutting
// reads over the provider_calls table. Powers dashboard cost/usage views
// without going through Prometheus. See CLAUDE.md → Observability
// Boundaries and the design proposal at
// docs/local-backlog/implemented/2026-04-17-observability-split-design.md.
type ProviderCallsStore interface {
	// AggregateProviderCalls runs a namespace-scoped GROUP BY over
	// provider_calls JOINed to sessions for the namespace/agent filter.
	AggregateProviderCalls(ctx context.Context, opts ProviderCallAggregateOpts) ([]*ProviderCallAggregateRow, error)

	// ProviderCallsDiscovery returns the distinct (provider, model) values
	// that appear in this namespace's provider_calls rows. Replaces
	// Prometheus label-discovery for provider/model dropdowns.
	ProviderCallsDiscovery(ctx context.Context, namespace string) (*ProviderCallDiscoveryResult, error)
}
