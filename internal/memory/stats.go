/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package memory

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// AggregateGroupBy enumerates the supported groupBy dimensions.
type AggregateGroupBy string

const (
	AggregateGroupByCategory AggregateGroupBy = "category"
	AggregateGroupByAgent    AggregateGroupBy = "agent"
	AggregateGroupByDay      AggregateGroupBy = "day"
)

// AggregateMetric enumerates the supported metric expressions.
type AggregateMetric string

const (
	AggregateMetricCount         AggregateMetric = "count"
	AggregateMetricDistinctUsers AggregateMetric = "distinct_users"
)

// DefaultAggregateLimit is applied when AggregateOptions.Limit is 0.
const DefaultAggregateLimit = 100

// MaxAggregateLimit clamps absurd LIMITs.
const MaxAggregateLimit = 1000

// AggregateOptions parameterises the Aggregate query.
type AggregateOptions struct {
	Workspace string
	GroupBy   AggregateGroupBy
	Metric    AggregateMetric
	From      *time.Time // inclusive lower bound on created_at; nil = no bound
	To        *time.Time // exclusive upper bound on created_at; nil = no bound
	Limit     int        // 0 → DefaultAggregateLimit; clamped to [1, MaxAggregateLimit]
}

// AggregateRow is one row of an aggregate response: key + the metric the
// caller asked for + the row count for context. value and count diverge
// when Metric == AggregateMetricDistinctUsers.
type AggregateRow struct {
	Key   string `json:"key"`
	Value int64  `json:"value"`
	Count int64  `json:"count"`
}

// Aggregate runs a workspace-scoped GROUP BY over memory_entities,
// composing AggregateConsentJoin so users without analytics:aggregate
// consent are excluded by construction.
func (s *PostgresMemoryStore) Aggregate(ctx context.Context, opts AggregateOptions) ([]AggregateRow, error) {
	if opts.Workspace == "" {
		return nil, errors.New("memory: workspace is required")
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultAggregateLimit
	}
	if limit > MaxAggregateLimit {
		limit = MaxAggregateLimit
	}

	keyExpr, extraWhere, orderClause, err := groupByFragments(opts.GroupBy)
	if err != nil {
		return nil, err
	}
	valueExpr, err := metricExpression(opts.Metric)
	if err != nil {
		return nil, err
	}

	join, consentWhere := AggregateConsentJoin("e")
	sql := fmt.Sprintf(`
		SELECT %s AS key, %s AS value, COUNT(*) AS count
		FROM memory_entities e %s
		WHERE e.workspace_id = $1
		  AND e.forgotten = false
		  AND %s
		  AND ($2::timestamptz IS NULL OR e.created_at >= $2)
		  AND ($3::timestamptz IS NULL OR e.created_at <  $3)%s
		GROUP BY 1
		%s
		LIMIT $4`,
		keyExpr, valueExpr, join, consentWhere, extraWhere, orderClause)

	var fromArg, toArg any
	if opts.From != nil {
		fromArg = *opts.From
	}
	if opts.To != nil {
		toArg = *opts.To
	}

	rows, err := s.pool.Query(ctx, sql, opts.Workspace, fromArg, toArg, limit)
	if err != nil {
		return nil, fmt.Errorf("memory: aggregate query: %w", err)
	}
	defer rows.Close()

	var out []AggregateRow
	for rows.Next() {
		var r AggregateRow
		if err := rows.Scan(&r.Key, &r.Value, &r.Count); err != nil {
			return nil, fmt.Errorf("memory: aggregate scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: aggregate iterate: %w", err)
	}
	return out, nil
}

// groupByFragments returns the SQL key expression, an extra WHERE
// fragment (with leading " AND " when non-empty), and the ORDER BY
// clause for a given GroupBy value.
func groupByFragments(g AggregateGroupBy) (keyExpr, extraWhere, orderClause string, err error) {
	switch g {
	case AggregateGroupByCategory:
		return "COALESCE(e.consent_category, 'unknown')", "", "ORDER BY value DESC", nil
	case AggregateGroupByAgent:
		// Skip institutional rows: agent_id IS NULL clutters the chart.
		return "e.agent_id", " AND e.agent_id IS NOT NULL", "ORDER BY value DESC", nil
	case AggregateGroupByDay:
		return "to_char(date_trunc('day', e.created_at)::date, 'YYYY-MM-DD')",
			"", "ORDER BY 1 ASC", nil
	default:
		return "", "", "", fmt.Errorf("memory: invalid groupBy %q", g)
	}
}

// metricExpression returns the SQL value expression for a given Metric.
func metricExpression(m AggregateMetric) (string, error) {
	switch m {
	case AggregateMetricCount, "":
		return "COUNT(*)", nil
	case AggregateMetricDistinctUsers:
		return "COUNT(DISTINCT e.virtual_user_id) FILTER (WHERE e.virtual_user_id IS NOT NULL)", nil
	default:
		return "", fmt.Errorf("memory: invalid metric %q", m)
	}
}
