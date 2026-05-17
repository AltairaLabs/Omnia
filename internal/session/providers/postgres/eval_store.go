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

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/altairalabs/omnia/internal/pgutil"
	"github.com/altairalabs/omnia/internal/session/api"
)

// Compile-time interface check.
var _ api.EvalStore = (*EvalStoreImpl)(nil)

// EvalStoreImpl implements api.EvalStore using PostgreSQL.
type EvalStoreImpl struct {
	pool *pgxpool.Pool
}

// NewEvalStore creates a new EvalStoreImpl from an existing connection pool.
func NewEvalStore(pool *pgxpool.Pool) *EvalStoreImpl {
	return &EvalStoreImpl{pool: pool}
}

// evalResultColumns is the SELECT column list for eval_results.
const evalResultColumns = `id, session_id, message_id, agent_name, namespace,
	promptpack_name, promptpack_version, eval_id, eval_type, trigger,
	passed, score, details, duration_ms, judge_tokens, judge_cost_usd,
	source, created_at`

// Reusable QueryBuilder filter fragments. The QueryBuilder substitutes the
// `$?` placeholder with the appropriate positional argument index, so a
// single constant safely serves multiple call sites.
const (
	filterAgentName      = "agent_name=$?"
	filterNamespace      = "namespace=$?"
	filterEvalID         = "eval_id=$?"
	filterEvalType       = "eval_type=$?"
	filterPromptPackName = "promptpack_name=$?"
	filterPassed         = "passed=$?"
	filterCreatedAfter   = "created_at >= $?"
	filterCreatedBefore  = "created_at < $?"

	orderByValueDesc = "ORDER BY value DESC"
)

// InsertEvalResults persists one or more eval results using a batch insert.
func (s *EvalStoreImpl) InsertEvalResults(ctx context.Context, results []*api.EvalResult) error {
	query := `INSERT INTO eval_results (
		session_id, message_id, agent_name, namespace,
		promptpack_name, promptpack_version, eval_id, eval_type, trigger,
		passed, score, details, duration_ms, judge_tokens, judge_cost_usd,
		source
	) VALUES `

	args := make([]any, 0, len(results)*16)
	valueRows := make([]string, 0, len(results))

	for i, r := range results {
		base := i * 16
		placeholders := make([]string, 16)
		for j := range 16 {
			placeholders[j] = "$" + strconv.Itoa(base+j+1)
		}
		valueRows = append(valueRows, "("+strings.Join(placeholders, ",")+")")

		args = append(args,
			r.SessionID, pgutil.NullString(r.MessageID), r.AgentName, r.Namespace,
			r.PromptPackName, pgutil.NullString(r.PromptPackVersion), r.EvalID, r.EvalType, r.Trigger,
			r.Passed, r.Score, nullJSONB(r.Details), r.DurationMs, r.JudgeTokens, r.JudgeCostUSD,
			r.Source,
		)
	}

	query += strings.Join(valueRows, ",")

	_, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("postgres: insert eval results: %w", err)
	}
	return nil
}

// GetSessionEvalResults retrieves all eval results for a given session.
func (s *EvalStoreImpl) GetSessionEvalResults(ctx context.Context, sessionID string) ([]*api.EvalResult, error) {
	query := `SELECT ` + evalResultColumns + ` FROM eval_results WHERE session_id=$1 ORDER BY created_at ASC`

	rows, err := s.pool.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("postgres: get session eval results: %w", err)
	}
	return collectEvalResults(rows)
}

// ListEvalResults retrieves eval results matching the given filters with pagination.
func (s *EvalStoreImpl) ListEvalResults(ctx context.Context, opts api.EvalResultListOpts) ([]*api.EvalResult, int64, error) {
	qb := &pgutil.QueryBuilder{}
	applyEvalFilters(qb, opts)

	countQuery := `SELECT count(*) FROM eval_results WHERE 1=1` + qb.Where()
	var total int64
	if err := s.pool.QueryRow(ctx, countQuery, qb.Args()...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("postgres: count eval results: %w", err)
	}

	query := `SELECT ` + evalResultColumns + ` FROM eval_results WHERE 1=1` + qb.Where() +
		` ORDER BY created_at DESC`
	query = qb.AppendPagination(query, opts.Limit, opts.Offset)

	rows, err := s.pool.Query(ctx, query, qb.Args()...)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: list eval results: %w", err)
	}

	results, err := collectEvalResults(rows)
	if err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

// GetEvalResultSummary returns aggregate statistics grouped by eval_id and eval_type.
func (s *EvalStoreImpl) GetEvalResultSummary(ctx context.Context, opts api.EvalResultSummaryOpts) ([]*api.EvalResultSummary, error) {
	qb := &pgutil.QueryBuilder{}
	applySummaryFilters(qb, opts)

	query := `SELECT eval_id, eval_type,
		count(*) AS total,
		count(*) FILTER (WHERE passed = true) AS passed,
		count(*) FILTER (WHERE passed = false) AS failed,
		CASE WHEN count(*) > 0
			THEN count(*) FILTER (WHERE passed = true)::float / count(*)
			ELSE 0
		END AS pass_rate,
		avg(score) AS avg_score,
		avg(duration_ms) AS avg_duration_ms
	FROM eval_results WHERE 1=1` + qb.Where() +
		` GROUP BY eval_id, eval_type ORDER BY eval_id`

	rows, err := s.pool.Query(ctx, query, qb.Args()...)
	if err != nil {
		return nil, fmt.Errorf("postgres: get eval result summary: %w", err)
	}
	defer rows.Close()

	var summaries []*api.EvalResultSummary
	for rows.Next() {
		var sm api.EvalResultSummary
		if err := rows.Scan(
			&sm.EvalID, &sm.EvalType,
			&sm.Total, &sm.Passed, &sm.Failed,
			&sm.PassRate, &sm.AvgScore, &sm.AvgDurationMs,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan eval summary: %w", err)
		}
		summaries = append(summaries, &sm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate eval summaries: %w", err)
	}
	if summaries == nil {
		summaries = []*api.EvalResultSummary{}
	}
	return summaries, nil
}

// AggregateEvalResults runs a namespace-scoped GROUP BY over eval_results.
// Powers /api/v1/eval-results/aggregate so product views can read trends and
// percentiles without going through Prometheus. See
// docs/local-backlog/implemented/2026-04-17-observability-split-design.md.
func (s *EvalStoreImpl) AggregateEvalResults(ctx context.Context, opts api.EvalAggregateOpts) ([]*api.EvalAggregateRow, error) {
	if opts.Namespace == "" {
		return nil, fmt.Errorf("postgres: aggregate eval results: namespace is required")
	}

	keyExpr, orderClause, err := evalGroupByFragments(opts.GroupBy)
	if err != nil {
		return nil, err
	}
	valueExpr, err := evalMetricExpression(opts.Metric)
	if err != nil {
		return nil, err
	}

	qb := buildEvalAggregateFilters(opts)
	query := fmt.Sprintf(`
		SELECT %s AS key, %s AS value, COUNT(*) AS count
		FROM eval_results
		WHERE 1=1%s
		GROUP BY 1
		%s
		LIMIT %d`,
		keyExpr, valueExpr, qb.Where(), orderClause, clampEvalAggregateLimit(opts.Limit))

	rows, err := s.pool.Query(ctx, query, qb.Args()...)
	if err != nil {
		return nil, fmt.Errorf("postgres: aggregate eval results: %w", err)
	}
	defer rows.Close()
	return collectEvalAggregateRows(rows)
}

// buildEvalAggregateFilters seeds a QueryBuilder with the required namespace
// filter plus any optional filters set on opts. Extracted from
// AggregateEvalResults to keep its cognitive complexity below the Sonar Way
// threshold (≤15).
func buildEvalAggregateFilters(opts api.EvalAggregateOpts) *pgutil.QueryBuilder {
	qb := &pgutil.QueryBuilder{}
	qb.Add(filterNamespace, opts.Namespace)
	if opts.AgentName != "" {
		qb.Add(filterAgentName, opts.AgentName)
	}
	if opts.PromptPackName != "" {
		qb.Add(filterPromptPackName, opts.PromptPackName)
	}
	if opts.EvalID != "" {
		qb.Add(filterEvalID, opts.EvalID)
	}
	if opts.EvalType != "" {
		qb.Add(filterEvalType, opts.EvalType)
	}
	if !opts.From.IsZero() {
		qb.Add(filterCreatedAfter, opts.From)
	}
	if !opts.To.IsZero() {
		qb.Add(filterCreatedBefore, opts.To)
	}
	return qb
}

// clampEvalAggregateLimit applies the package defaults / ceiling to a
// caller-supplied limit.
func clampEvalAggregateLimit(limit int) int {
	if limit <= 0 {
		return api.DefaultEvalAggregateLimit
	}
	if limit > api.MaxEvalAggregateLimit {
		return api.MaxEvalAggregateLimit
	}
	return limit
}

// collectEvalAggregateRows scans an aggregate result set into the API row
// shape. avg/percentile over an empty group is NULL; emit 0.0 so callers
// don't have to special-case missing values for buckets with only NULL scores.
func collectEvalAggregateRows(rows pgx.Rows) ([]*api.EvalAggregateRow, error) {
	out := []*api.EvalAggregateRow{}
	for rows.Next() {
		var r api.EvalAggregateRow
		var v *float64
		if err := rows.Scan(&r.Key, &v, &r.Count); err != nil {
			return nil, fmt.Errorf("postgres: scan eval aggregate row: %w", err)
		}
		if v != nil {
			r.Value = *v
		}
		out = append(out, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate eval aggregate rows: %w", err)
	}
	return out, nil
}

// evalGroupByFragments returns the SQL key expression and ORDER BY clause
// for a given EvalAggregateGroupBy. Time buckets sort ASC (for chronological
// charts); categorical groups sort DESC by value (largest first).
func evalGroupByFragments(g api.EvalAggregateGroupBy) (keyExpr, orderClause string, err error) {
	switch g {
	case api.EvalAggregateGroupByEvalID:
		return "eval_id", orderByValueDesc, nil
	case api.EvalAggregateGroupByEvalType:
		return "eval_type", orderByValueDesc, nil
	case api.EvalAggregateGroupByAgent:
		return "agent_name", orderByValueDesc, nil
	case api.EvalAggregateGroupByTimeHour:
		return "to_char(date_trunc('hour', created_at) AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:00:00\"Z\"')",
			"ORDER BY 1 ASC", nil
	case api.EvalAggregateGroupByTimeDay:
		return "to_char(date_trunc('day', created_at)::date, 'YYYY-MM-DD')",
			"ORDER BY 1 ASC", nil
	default:
		return "", "", fmt.Errorf("postgres: invalid groupBy %q", g)
	}
}

// evalMetricExpression returns the SQL value expression for a given metric.
// Percentiles use percentile_cont (continuous interpolation, standard PG).
// Score metrics filter out NULL scores so percentile calculations don't
// degrade — assertion-type evals have NULL score by design.
func evalMetricExpression(m api.EvalAggregateMetric) (string, error) {
	switch m {
	case api.EvalAggregateMetricCount:
		return "COUNT(*)::float", nil
	case api.EvalAggregateMetricAvgScore:
		return "AVG(score) FILTER (WHERE score IS NOT NULL)", nil
	case api.EvalAggregateMetricP50Score:
		return "percentile_cont(0.5) WITHIN GROUP (ORDER BY score) FILTER (WHERE score IS NOT NULL)", nil
	case api.EvalAggregateMetricP95Score:
		return "percentile_cont(0.95) WITHIN GROUP (ORDER BY score) FILTER (WHERE score IS NOT NULL)", nil
	case api.EvalAggregateMetricAvgLatencyMs:
		return "AVG(duration_ms) FILTER (WHERE duration_ms IS NOT NULL)", nil
	case api.EvalAggregateMetricP95LatencyMs:
		return "percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL)", nil
	default:
		return "", fmt.Errorf("postgres: invalid metric %q", m)
	}
}

// DistinctEvals returns the set of (eval_id, eval_type) pairs that have at
// least one row in eval_results for the given namespace. Replaces Prometheus'
// metric-discovery for dashboard product views.
func (s *EvalStoreImpl) DistinctEvals(ctx context.Context, namespace string) ([]api.EvalDescriptor, error) {
	if namespace == "" {
		return nil, fmt.Errorf("postgres: distinct evals: namespace is required")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT eval_id, eval_type
		FROM eval_results
		WHERE namespace = $1
		ORDER BY eval_id`, namespace)
	if err != nil {
		return nil, fmt.Errorf("postgres: distinct evals: %w", err)
	}
	defer rows.Close()

	out := []api.EvalDescriptor{}
	for rows.Next() {
		var d api.EvalDescriptor
		if err := rows.Scan(&d.EvalID, &d.EvalType); err != nil {
			return nil, fmt.Errorf("postgres: scan distinct eval: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate distinct evals: %w", err)
	}
	return out, nil
}

// --- helpers ----------------------------------------------------------------

func applyEvalFilters(qb *pgutil.QueryBuilder, opts api.EvalResultListOpts) {
	if opts.AgentName != "" {
		qb.Add(filterAgentName, opts.AgentName)
	}
	if opts.Namespace != "" {
		qb.Add(filterNamespace, opts.Namespace)
	}
	if opts.EvalID != "" {
		qb.Add(filterEvalID, opts.EvalID)
	}
	if opts.EvalType != "" {
		qb.Add(filterEvalType, opts.EvalType)
	}
	if opts.Passed != nil {
		qb.Add(filterPassed, *opts.Passed)
	}
	if !opts.CreatedAfter.IsZero() {
		qb.Add(filterCreatedAfter, opts.CreatedAfter)
	}
	if !opts.CreatedBefore.IsZero() {
		qb.Add(filterCreatedBefore, opts.CreatedBefore)
	}
}

func applySummaryFilters(qb *pgutil.QueryBuilder, opts api.EvalResultSummaryOpts) {
	if opts.AgentName != "" {
		qb.Add(filterAgentName, opts.AgentName)
	}
	if opts.Namespace != "" {
		qb.Add(filterNamespace, opts.Namespace)
	}
	if opts.EvalType != "" {
		qb.Add(filterEvalType, opts.EvalType)
	}
	if !opts.CreatedAfter.IsZero() {
		qb.Add(filterCreatedAfter, opts.CreatedAfter)
	}
	if !opts.CreatedBefore.IsZero() {
		qb.Add(filterCreatedBefore, opts.CreatedBefore)
	}
}

func scanEvalResult(row pgx.Row) (*api.EvalResult, error) {
	var r api.EvalResult
	var messageID, promptPackVersion *string
	var details []byte

	err := row.Scan(
		&r.ID, &r.SessionID, &messageID, &r.AgentName, &r.Namespace,
		&r.PromptPackName, &promptPackVersion, &r.EvalID, &r.EvalType, &r.Trigger,
		&r.Passed, &r.Score, &details, &r.DurationMs, &r.JudgeTokens, &r.JudgeCostUSD,
		&r.Source, &r.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: scan eval result: %w", err)
	}

	if messageID != nil {
		r.MessageID = *messageID
	}
	if promptPackVersion != nil {
		r.PromptPackVersion = *promptPackVersion
	}
	if len(details) > 0 {
		r.Details = json.RawMessage(details)
	}

	return &r, nil
}

func collectEvalResults(rows pgx.Rows) ([]*api.EvalResult, error) {
	defer rows.Close()
	var results []*api.EvalResult
	for rows.Next() {
		r, err := scanEvalResult(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate eval results: %w", err)
	}
	if results == nil {
		results = []*api.EvalResult{}
	}
	return results, nil
}

// nullJSONB returns nil for empty/null JSON values.
func nullJSONB(data json.RawMessage) []byte {
	if len(data) == 0 {
		return nil
	}
	return []byte(data)
}
