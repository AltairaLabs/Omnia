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
	"context"
	"encoding/json"
	"time"
)

// EvalResult represents a single evaluation result stored in the eval_results table.
//
// NOTE: A structurally similar type exists in internal/session/store.go
// (session.EvalResult). That type is used by the facade-side Store interface
// and has more omitempty JSON tags for the write path. This type is used by
// the session-api HTTP layer and EvalStore interface, where required fields
// (AgentName, Namespace, etc.) must always appear in API responses.
type EvalResult struct {
	ID                string          `json:"id"`
	SessionID         string          `json:"sessionId"`
	MessageID         string          `json:"messageId,omitempty"`
	AgentName         string          `json:"agentName"`
	Namespace         string          `json:"namespace"`
	PromptPackName    string          `json:"promptpackName"`
	PromptPackVersion string          `json:"promptpackVersion,omitempty"`
	EvalID            string          `json:"evalId"`
	EvalType          string          `json:"evalType"`
	Trigger           string          `json:"trigger"`
	Passed            bool            `json:"passed"`
	Score             *float64        `json:"score,omitempty"`
	Details           json.RawMessage `json:"details,omitempty"`
	DurationMs        *int            `json:"durationMs,omitempty"`
	JudgeTokens       *int            `json:"judgeTokens,omitempty"`
	JudgeCostUSD      *float64        `json:"judgeCostUsd,omitempty"`
	Source            string          `json:"source"`
	CreatedAt         time.Time       `json:"createdAt"`
}

// EvalResultSummary contains aggregate statistics for a group of eval results.
type EvalResultSummary struct {
	EvalID        string   `json:"evalId"`
	EvalType      string   `json:"evalType"`
	Total         int      `json:"total"`
	Passed        int      `json:"passed"`
	Failed        int      `json:"failed"`
	PassRate      float64  `json:"passRate"`
	AvgScore      *float64 `json:"avgScore,omitempty"`
	AvgDurationMs *float64 `json:"avgDurationMs,omitempty"`
}

// EvalResultListOpts configures queries for listing eval results.
type EvalResultListOpts struct {
	Limit         int
	Offset        int
	AgentName     string
	Namespace     string
	EvalID        string
	EvalType      string
	Passed        *bool
	CreatedAfter  time.Time
	CreatedBefore time.Time
}

// EvalResultSummaryOpts configures queries for eval result summaries.
type EvalResultSummaryOpts struct {
	AgentName     string
	Namespace     string
	EvalType      string
	CreatedAfter  time.Time
	CreatedBefore time.Time
}

// EvalAggregateGroupBy enumerates valid groupBy values for AggregateEvalResults.
// The "time:*" forms bucket created_at via date_trunc.
type EvalAggregateGroupBy string

const (
	// EvalAggregateGroupByEvalID groups by the eval_id column (one row per
	// eval scenario). The shape that replaces today's Prom-per-metric-name fanout.
	EvalAggregateGroupByEvalID EvalAggregateGroupBy = "eval_id"
	// EvalAggregateGroupByEvalType groups by the eval_type column (llm_judge,
	// assertion, etc.).
	EvalAggregateGroupByEvalType EvalAggregateGroupBy = "eval_type"
	// EvalAggregateGroupByAgent groups by the agent_name column.
	EvalAggregateGroupByAgent EvalAggregateGroupBy = "agent"
	// EvalAggregateGroupByTimeHour buckets created_at by hour (UTC),
	// formatted as RFC3339 hour-precision strings.
	EvalAggregateGroupByTimeHour EvalAggregateGroupBy = "time:hour"
	// EvalAggregateGroupByTimeDay buckets created_at by day (UTC),
	// formatted as YYYY-MM-DD strings.
	EvalAggregateGroupByTimeDay EvalAggregateGroupBy = "time:day"
)

// EvalAggregateMetric enumerates valid metric values for AggregateEvalResults.
// Percentiles use Postgres' percentile_cont (continuous interpolation).
type EvalAggregateMetric string

const (
	EvalAggregateMetricCount        EvalAggregateMetric = "count"
	EvalAggregateMetricAvgScore     EvalAggregateMetric = "avg_score"
	EvalAggregateMetricP50Score     EvalAggregateMetric = "p50_score"
	EvalAggregateMetricP95Score     EvalAggregateMetric = "p95_score"
	EvalAggregateMetricAvgLatencyMs EvalAggregateMetric = "avg_latency_ms"
	EvalAggregateMetricP95LatencyMs EvalAggregateMetric = "p95_latency_ms"
)

// EvalAggregateLimit clamps the maximum number of returned rows. The default
// is large enough to fit a day-bucket trend over 90 days (90 keys) without
// pagination, and the max keeps a malformed query from sweeping the table.
const (
	DefaultEvalAggregateLimit = 500
	MaxEvalAggregateLimit     = 5000
)

// EvalAggregateRow is one returned row from AggregateEvalResults. Key is the
// stringified group value (eval_id, time bucket, etc.); Value is the metric;
// Count is the number of source rows that contributed.
type EvalAggregateRow struct {
	Key   string  `json:"key"`
	Value float64 `json:"value"`
	Count int64   `json:"count"`
}

// EvalAggregateOpts configures the AggregateEvalResults query.
// Namespace is the scoping field (mirrors the workspace_id role on memory_entities).
// GroupBy and Metric are required; all other fields are optional filters.
type EvalAggregateOpts struct {
	Namespace      string // required
	AgentName      string // optional
	PromptPackName string // optional
	EvalID         string // optional
	EvalType       string // optional
	From           time.Time
	To             time.Time
	GroupBy        EvalAggregateGroupBy // required
	Metric         EvalAggregateMetric  // required
	Limit          int
}

// EvalStore defines the persistence interface for eval results.
type EvalStore interface {
	// InsertEvalResults persists one or more eval results.
	InsertEvalResults(ctx context.Context, results []*EvalResult) error

	// GetSessionEvalResults retrieves eval results for a specific session.
	GetSessionEvalResults(ctx context.Context, sessionID string) ([]*EvalResult, error)

	// ListEvalResults retrieves eval results matching the given filters.
	ListEvalResults(ctx context.Context, opts EvalResultListOpts) ([]*EvalResult, int64, error)

	// GetEvalResultSummary returns aggregate statistics grouped by eval_id and eval_type.
	GetEvalResultSummary(ctx context.Context, opts EvalResultSummaryOpts) ([]*EvalResultSummary, error)

	// AggregateEvalResults runs a namespace-scoped GROUP BY over eval_results.
	// Used by /api/v1/eval-results/aggregate to power product views without
	// Prometheus. See docs/local-backlog/implemented/2026-04-17-observability-split-design.md
	// for the design rationale.
	AggregateEvalResults(ctx context.Context, opts EvalAggregateOpts) ([]*EvalAggregateRow, error)

	// EvalDiscovery returns the distinct eval/agent/promptpack values that
	// appear in eval_results for the given namespace. Used by the dashboard's
	// eval discovery + filter UI, replacing the Prometheus /api/v1/metadata
	// and label-discovery query paths.
	EvalDiscovery(ctx context.Context, namespace string) (*EvalDiscoveryResult, error)
}

// EvalDescriptor is one (eval_id, eval_type) pair.
type EvalDescriptor struct {
	EvalID   string `json:"evalId"`
	EvalType string `json:"evalType"`
}

// EvalDiscoveryResult is the namespace-scoped discovery payload returned by
// EvalStore.EvalDiscovery. Slices are non-nil so callers can JSON-serialise
// without a null check.
type EvalDiscoveryResult struct {
	Evals       []EvalDescriptor `json:"evals"`
	Agents      []string         `json:"agents"`
	PromptPacks []string         `json:"promptpacks"`
}
