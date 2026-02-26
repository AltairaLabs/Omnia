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

// --- helpers ----------------------------------------------------------------

func applyEvalFilters(qb *pgutil.QueryBuilder, opts api.EvalResultListOpts) {
	if opts.AgentName != "" {
		qb.Add("agent_name=$?", opts.AgentName)
	}
	if opts.Namespace != "" {
		qb.Add("namespace=$?", opts.Namespace)
	}
	if opts.EvalID != "" {
		qb.Add("eval_id=$?", opts.EvalID)
	}
	if opts.EvalType != "" {
		qb.Add("eval_type=$?", opts.EvalType)
	}
	if opts.Passed != nil {
		qb.Add("passed=$?", *opts.Passed)
	}
	if !opts.CreatedAfter.IsZero() {
		qb.Add("created_at >= $?", opts.CreatedAfter)
	}
	if !opts.CreatedBefore.IsZero() {
		qb.Add("created_at < $?", opts.CreatedBefore)
	}
}

func applySummaryFilters(qb *pgutil.QueryBuilder, opts api.EvalResultSummaryOpts) {
	if opts.AgentName != "" {
		qb.Add("agent_name=$?", opts.AgentName)
	}
	if opts.Namespace != "" {
		qb.Add("namespace=$?", opts.Namespace)
	}
	if opts.EvalType != "" {
		qb.Add("eval_type=$?", opts.EvalType)
	}
	if !opts.CreatedAfter.IsZero() {
		qb.Add("created_at >= $?", opts.CreatedAfter)
	}
	if !opts.CreatedBefore.IsZero() {
		qb.Add("created_at < $?", opts.CreatedBefore)
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
