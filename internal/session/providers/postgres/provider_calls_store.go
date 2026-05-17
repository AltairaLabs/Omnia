/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/altairalabs/omnia/internal/pgutil"
	"github.com/altairalabs/omnia/internal/session/api"
)

// Compile-time interface check.
var _ api.ProviderCallsStore = (*ProviderCallsStoreImpl)(nil)

// ProviderCallsStoreImpl implements api.ProviderCallsStore using PostgreSQL.
// All queries INNER JOIN sessions on session_id so the namespace + agent
// filters can be applied to provider_calls rows (which have no namespace
// column of their own).
type ProviderCallsStoreImpl struct {
	pool *pgxpool.Pool
}

// NewProviderCallsStore creates a new ProviderCallsStoreImpl from an existing
// connection pool.
func NewProviderCallsStore(pool *pgxpool.Pool) *ProviderCallsStoreImpl {
	return &ProviderCallsStoreImpl{pool: pool}
}

// Filter fragments. The QueryBuilder substitutes the `$?` placeholder with
// the appropriate positional argument index.
const (
	pcFilterNamespace     = "s.namespace=$?"
	pcFilterAgentName     = "s.agent_name=$?"
	pcFilterProvider      = "pc.provider=$?"
	pcFilterModel         = "pc.model=$?"
	pcFilterCreatedAfter  = "pc.created_at >= $?"
	pcFilterCreatedBefore = "pc.created_at < $?"

	pcOrderByValueDesc = "ORDER BY value DESC"
	pcOrderByKeyAsc    = "ORDER BY 1 ASC"
)

// AggregateProviderCalls runs a namespace-scoped GROUP BY over provider_calls.
// Powers /api/v1/provider-calls/aggregate so product views can read cost,
// token-usage, request-count, and latency trends without going through
// Prometheus. See
// docs/local-backlog/implemented/2026-04-17-observability-split-design.md.
func (s *ProviderCallsStoreImpl) AggregateProviderCalls(
	ctx context.Context, opts api.ProviderCallAggregateOpts,
) ([]*api.ProviderCallAggregateRow, error) {
	if opts.Namespace == "" {
		return nil, fmt.Errorf("postgres: aggregate provider calls: namespace is required")
	}

	keyExpr, orderClause, err := providerCallGroupByFragments(opts.GroupBy)
	if err != nil {
		return nil, err
	}
	valueExpr, err := providerCallMetricExpression(opts.Metric)
	if err != nil {
		return nil, err
	}

	qb := buildProviderCallAggregateFilters(opts)
	query := fmt.Sprintf(`
		SELECT %s AS key, %s AS value, COUNT(*) AS count
		FROM provider_calls pc
		INNER JOIN sessions s ON s.id = pc.session_id
		WHERE 1=1%s
		GROUP BY 1
		%s
		LIMIT %d`,
		keyExpr, valueExpr, qb.Where(), orderClause, clampProviderCallAggregateLimit(opts.Limit))

	rows, err := s.pool.Query(ctx, query, qb.Args()...)
	if err != nil {
		return nil, fmt.Errorf("postgres: aggregate provider calls: %w", err)
	}
	defer rows.Close()
	return collectProviderCallAggregateRows(rows)
}

// buildProviderCallAggregateFilters seeds a QueryBuilder with the required
// namespace filter plus any optional filters.
func buildProviderCallAggregateFilters(opts api.ProviderCallAggregateOpts) *pgutil.QueryBuilder {
	qb := &pgutil.QueryBuilder{}
	qb.Add(pcFilterNamespace, opts.Namespace)
	if opts.AgentName != "" {
		qb.Add(pcFilterAgentName, opts.AgentName)
	}
	if opts.Provider != "" {
		qb.Add(pcFilterProvider, opts.Provider)
	}
	if opts.Model != "" {
		qb.Add(pcFilterModel, opts.Model)
	}
	if !opts.From.IsZero() {
		qb.Add(pcFilterCreatedAfter, opts.From)
	}
	if !opts.To.IsZero() {
		qb.Add(pcFilterCreatedBefore, opts.To)
	}
	return qb
}

// clampProviderCallAggregateLimit applies defaults / ceiling to a
// caller-supplied limit.
func clampProviderCallAggregateLimit(limit int) int {
	if limit <= 0 {
		return api.DefaultProviderCallAggregateLimit
	}
	if limit > api.MaxProviderCallAggregateLimit {
		return api.MaxProviderCallAggregateLimit
	}
	return limit
}

// collectProviderCallAggregateRows scans aggregate rows. SUMs / AVGs over an
// empty group are NULL; emit 0.0 so callers don't have to special-case
// missing values for buckets with no rows.
func collectProviderCallAggregateRows(rows pgx.Rows) ([]*api.ProviderCallAggregateRow, error) {
	out := []*api.ProviderCallAggregateRow{}
	for rows.Next() {
		var r api.ProviderCallAggregateRow
		var v *float64
		if err := rows.Scan(&r.Key, &v, &r.Count); err != nil {
			return nil, fmt.Errorf("postgres: scan provider call aggregate row: %w", err)
		}
		if v != nil {
			r.Value = *v
		}
		out = append(out, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate provider call aggregate rows: %w", err)
	}
	return out, nil
}

// providerCallGroupByFragments returns the SQL key expression and ORDER BY
// clause for a given ProviderCallAggregateGroupBy.
func providerCallGroupByFragments(g api.ProviderCallAggregateGroupBy) (keyExpr, orderClause string, err error) {
	switch g {
	case api.ProviderCallAggregateGroupByProvider:
		return "pc.provider", pcOrderByValueDesc, nil
	case api.ProviderCallAggregateGroupByModel:
		return "pc.model", pcOrderByValueDesc, nil
	case api.ProviderCallAggregateGroupByAgent:
		return "s.agent_name", pcOrderByValueDesc, nil
	case api.ProviderCallAggregateGroupByTimeHour:
		return "to_char(date_trunc('hour', pc.created_at) AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:00:00\"Z\"')",
			pcOrderByKeyAsc, nil
	case api.ProviderCallAggregateGroupByTimeDay:
		return "to_char(date_trunc('day', pc.created_at)::date, 'YYYY-MM-DD')",
			pcOrderByKeyAsc, nil
	default:
		return "", "", fmt.Errorf("postgres: invalid groupBy %q", g)
	}
}

// providerCallMetricExpression returns the SQL value expression for a metric.
// COUNT is cast to float so all metrics scan into the same *float64 type.
// Percentiles use percentile_cont (standard PG, no extension needed).
func providerCallMetricExpression(m api.ProviderCallAggregateMetric) (string, error) {
	switch m {
	case api.ProviderCallAggregateMetricCount:
		return "COUNT(*)::float", nil
	case api.ProviderCallAggregateMetricSumCostUSD:
		return "SUM(pc.cost_usd)", nil
	case api.ProviderCallAggregateMetricSumInputTokens:
		return "SUM(pc.input_tokens)::float", nil
	case api.ProviderCallAggregateMetricSumOutputTokens:
		return "SUM(pc.output_tokens)::float", nil
	case api.ProviderCallAggregateMetricSumCachedTokens:
		return "SUM(pc.cached_tokens)::float", nil
	case api.ProviderCallAggregateMetricSumTokens:
		return "SUM(pc.input_tokens + pc.output_tokens)::float", nil
	case api.ProviderCallAggregateMetricAvgDurationMs:
		return "AVG(pc.duration_ms) FILTER (WHERE pc.duration_ms IS NOT NULL)", nil
	case api.ProviderCallAggregateMetricP95DurationMs:
		return "percentile_cont(0.95) WITHIN GROUP (ORDER BY pc.duration_ms) FILTER (WHERE pc.duration_ms IS NOT NULL)", nil
	default:
		return "", fmt.Errorf("postgres: invalid metric %q", m)
	}
}

// ProviderCallsDiscovery returns the distinct provider + model values seen
// in this namespace's provider_calls rows.
func (s *ProviderCallsStoreImpl) ProviderCallsDiscovery(
	ctx context.Context, namespace string,
) (*api.ProviderCallDiscoveryResult, error) {
	if namespace == "" {
		return nil, fmt.Errorf("postgres: provider calls discovery: namespace is required")
	}

	providers, err := s.distinctProviderCallColumn(ctx, namespace, "provider")
	if err != nil {
		return nil, err
	}
	models, err := s.distinctProviderCallColumn(ctx, namespace, "model")
	if err != nil {
		return nil, err
	}

	return &api.ProviderCallDiscoveryResult{
		Providers: providers,
		Models:    models,
	}, nil
}

// distinctProviderCallColumn reads sorted DISTINCT non-empty values for one
// provider_calls column, scoped via the sessions JOIN. The column name is
// interpolated into the SQL — it MUST be an internal identifier (never user
// input).
func (s *ProviderCallsStoreImpl) distinctProviderCallColumn(
	ctx context.Context, namespace, column string,
) ([]string, error) {
	//nolint:gosec // column comes from a closed set of internal identifiers.
	q := fmt.Sprintf(`
		SELECT DISTINCT pc.%s
		FROM provider_calls pc
		INNER JOIN sessions s ON s.id = pc.session_id
		WHERE s.namespace = $1 AND pc.%s <> ''
		ORDER BY 1`, column, column)
	rows, err := s.pool.Query(ctx, q, namespace)
	if err != nil {
		return nil, fmt.Errorf("postgres: distinct provider_calls.%s: %w", column, err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("postgres: scan distinct provider_calls.%s: %w", column, err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate distinct provider_calls.%s: %w", column, err)
	}
	return out, nil
}
