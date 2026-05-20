/*
Copyright 2025.

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
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/altairalabs/omnia/internal/pgutil"
	"github.com/altairalabs/omnia/internal/session/api"
)

// Compile-time interface check.
var _ api.FunctionInvocationsStore = (*FunctionInvocationsStoreImpl)(nil)

// FunctionInvocationsStoreImpl implements api.FunctionInvocationsStore.
// Reads / writes the function_invocations table — a partitioned table
// independent of sessions (Functions are not session-scoped).
type FunctionInvocationsStoreImpl struct {
	pool *pgxpool.Pool
}

// NewFunctionInvocationsStore wires a store onto an existing pool.
func NewFunctionInvocationsStore(pool *pgxpool.Pool) *FunctionInvocationsStoreImpl {
	return &FunctionInvocationsStoreImpl{pool: pool}
}

// Filter fragments used by the QueryBuilder to position arguments.
const (
	fiFilterNamespace     = "namespace=$?"
	fiFilterFunctionName  = "function_name=$?"
	fiFilterCreatedAfter  = "created_at >= $?"
	fiFilterCreatedBefore = "created_at < $?"
)

const insertFunctionInvocation = `
INSERT INTO function_invocations
    (id, namespace, function_name, input_hash, output_json, status,
     duration_ms, cost_usd, trace_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), $10)
`

// CreateFunctionInvocation inserts a single audit row. trace_id is
// stored as NULL when empty (matches the partial index in the
// migration).
func (s *FunctionInvocationsStoreImpl) CreateFunctionInvocation(
	ctx context.Context, inv *api.FunctionInvocation,
) error {
	if inv == nil {
		return errors.New("postgres: create function invocation: invocation is nil")
	}
	if inv.Namespace == "" {
		return errors.New("postgres: create function invocation: namespace is required")
	}
	if inv.FunctionName == "" {
		return errors.New("postgres: create function invocation: function_name is required")
	}
	if inv.Status == "" {
		return errors.New("postgres: create function invocation: status is required")
	}
	_, err := s.pool.Exec(ctx, insertFunctionInvocation,
		inv.ID,
		inv.Namespace,
		inv.FunctionName,
		inv.InputHash,
		inv.OutputJSON,
		inv.Status,
		inv.DurationMs,
		inv.CostUSD,
		inv.TraceID,
		inv.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: create function invocation: %w", err)
	}
	return nil
}

const selectFunctionInvocation = `
SELECT id, namespace, function_name, input_hash, output_json, status,
       duration_ms, cost_usd, COALESCE(trace_id, '') AS trace_id, created_at
FROM function_invocations
WHERE namespace=$1 AND id=$2
LIMIT 1
`

// GetFunctionInvocation returns a single row. Cross-tenant reads (a
// namespace that doesn't match the row's) return ErrNotFound, mirroring
// how the sessions store handles isolation.
func (s *FunctionInvocationsStoreImpl) GetFunctionInvocation(
	ctx context.Context, namespace, id string,
) (*api.FunctionInvocation, error) {
	if namespace == "" {
		return nil, errors.New("postgres: get function invocation: namespace is required")
	}
	if id == "" {
		return nil, errors.New("postgres: get function invocation: id is required")
	}
	row := s.pool.QueryRow(ctx, selectFunctionInvocation, namespace, id)
	inv, err := scanFunctionInvocation(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, api.ErrFunctionInvocationNotFound
		}
		return nil, fmt.Errorf("postgres: get function invocation: %w", err)
	}
	return inv, nil
}

// ListFunctionInvocations returns recent invocations, optionally
// filtered by function_name + time window. Ordered by created_at DESC
// so the most-recent rows surface first (the primary dashboard view).
func (s *FunctionInvocationsStoreImpl) ListFunctionInvocations(
	ctx context.Context, opts api.FunctionInvocationListOpts,
) ([]*api.FunctionInvocation, error) {
	if opts.Namespace == "" {
		return nil, errors.New("postgres: list function invocations: namespace is required")
	}
	qb := buildFunctionInvocationListFilters(opts)
	query := fmt.Sprintf(`
		SELECT id, namespace, function_name, input_hash, output_json, status,
		       duration_ms, cost_usd, COALESCE(trace_id, '') AS trace_id, created_at
		FROM function_invocations
		WHERE 1=1%s
		ORDER BY created_at DESC
		LIMIT %d`, qb.Where(), clampFunctionInvocationListLimit(opts.Limit))

	rows, err := s.pool.Query(ctx, query, qb.Args()...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list function invocations: %w", err)
	}
	defer rows.Close()

	out := []*api.FunctionInvocation{}
	for rows.Next() {
		inv, err := scanFunctionInvocation(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan function invocation: %w", err)
		}
		out = append(out, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate function invocations: %w", err)
	}
	return out, nil
}

// buildFunctionInvocationListFilters seeds a QueryBuilder with the
// required namespace filter plus any optional filters.
func buildFunctionInvocationListFilters(opts api.FunctionInvocationListOpts) *pgutil.QueryBuilder {
	qb := &pgutil.QueryBuilder{}
	qb.Add(fiFilterNamespace, opts.Namespace)
	if opts.FunctionName != "" {
		qb.Add(fiFilterFunctionName, opts.FunctionName)
	}
	if !opts.From.IsZero() {
		qb.Add(fiFilterCreatedAfter, opts.From)
	}
	if !opts.To.IsZero() {
		qb.Add(fiFilterCreatedBefore, opts.To)
	}
	return qb
}

// clampFunctionInvocationListLimit applies defaults / ceiling.
func clampFunctionInvocationListLimit(limit int) int {
	if limit <= 0 {
		return api.DefaultFunctionInvocationListLimit
	}
	if limit > api.MaxFunctionInvocationListLimit {
		return api.MaxFunctionInvocationListLimit
	}
	return limit
}

// rowScanner is the pgx row interface implemented by both Row and Rows.
type functionInvocationRowScanner interface {
	Scan(dest ...any) error
}

// scanFunctionInvocation pulls one row into a FunctionInvocation. Used
// by both Get (pgx.Row) and List (pgx.Rows) — the Scan signature is
// identical so this helper accepts either.
func scanFunctionInvocation(r functionInvocationRowScanner) (*api.FunctionInvocation, error) {
	var inv api.FunctionInvocation
	var output []byte
	if err := r.Scan(
		&inv.ID,
		&inv.Namespace,
		&inv.FunctionName,
		&inv.InputHash,
		&output,
		&inv.Status,
		&inv.DurationMs,
		&inv.CostUSD,
		&inv.TraceID,
		&inv.CreatedAt,
	); err != nil {
		return nil, err
	}
	if len(output) > 0 {
		inv.OutputJSON = output
	}
	return &inv, nil
}
