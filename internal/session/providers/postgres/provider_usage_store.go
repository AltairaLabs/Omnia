/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/altairalabs/omnia/internal/pgutil"
	"github.com/altairalabs/omnia/internal/session/api"
)

// Compile-time interface check.
var _ api.ProviderUsageStore = (*ProviderUsageStoreImpl)(nil)

// ProviderUsageStoreImpl implements api.ProviderUsageStore using PostgreSQL.
// provider_usage is session-less and workspace-scoped (keyed by namespace).
type ProviderUsageStoreImpl struct {
	pool *pgxpool.Pool
}

// NewProviderUsageStore creates a new ProviderUsageStoreImpl from an existing
// connection pool.
func NewProviderUsageStore(pool *pgxpool.Pool) *ProviderUsageStoreImpl {
	return &ProviderUsageStoreImpl{pool: pool}
}

// RecordProviderUsage inserts the given rows in a single batch. CreatedAt
// defaults to now() and CallCount to 1 when zero; the service layer enforces
// required fields.
func (s *ProviderUsageStoreImpl) RecordProviderUsage(ctx context.Context, rows []*api.ProviderUsage) error {
	if len(rows) == 0 {
		return nil
	}

	now := time.Now().UTC()
	batch := &pgx.Batch{}
	const q = `INSERT INTO provider_usage (
			namespace, workspace_name, provider, provider_name, model, source,
			input_tokens, output_tokens, cached_tokens, cost_usd, call_count, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	for _, r := range rows {
		callCount := r.CallCount
		if callCount == 0 {
			callCount = 1
		}
		createdAt := r.CreatedAt
		if createdAt.IsZero() {
			createdAt = now
		}
		batch.Queue(q,
			r.Namespace, pgutil.NullString(r.WorkspaceName), r.Provider,
			pgutil.NullString(r.ProviderName), r.Model, r.Source,
			r.InputTokens, r.OutputTokens, r.CachedTokens, r.CostUSD,
			callCount, createdAt,
		)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("postgres: record provider usage: %w", err)
		}
	}
	return nil
}
