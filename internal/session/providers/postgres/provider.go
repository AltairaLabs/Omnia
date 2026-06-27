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
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/altairalabs/omnia/internal/session/providers"
)

// Compile-time interface check.
var _ providers.WarmStoreProvider = (*Provider)(nil)

// qbSessionID is the QueryBuilder filter clause for session_id, extracted to
// avoid SonarCloud S1192 (duplicated string literal).
const qbSessionID = "session_id=$?"

// sourceAgent is the provider-call Source value identifying an agent call (as
// opposed to a judge or self-play call). Extracted to avoid goconst.
const sourceAgent = "agent"

// Provider implements providers.WarmStoreProvider using PostgreSQL.
type Provider struct {
	pool     *pgxpool.Pool
	ownsPool bool
}

// New creates a Provider that owns the underlying connection pool. The pool is
// created from cfg and verified with a PING. Close will shut down the pool.
func New(cfg Config) (*Provider, error) {
	if cfg.ConnString == "" {
		return nil, fmt.Errorf("postgres: connection string is required")
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.ConnString)
	if err != nil {
		return nil, fmt.Errorf("postgres: parsing connection string: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolCfg.HealthCheckPeriod = cfg.HealthCheckPeriod
	if cfg.TLS != nil {
		poolCfg.ConnConfig.TLSConfig = cfg.TLS
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: creating pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping failed: %w", err)
	}

	return &Provider{pool: pool, ownsPool: true}, nil
}

// NewFromPool wraps an existing connection pool. Close is a no-op because the
// caller retains ownership of the pool.
func NewFromPool(pool *pgxpool.Pool) *Provider {
	return &Provider{pool: pool, ownsPool: false}
}

// sessionColumns is the SELECT column list for sessions (no trailing comma).
const sessionColumns = `id, agent_name, namespace, workspace_name, status,
	created_at, updated_at, expires_at, ended_at,
	message_count, tool_call_count, total_input_tokens, total_output_tokens,
	estimated_cost_usd, tags, state, last_message_preview,
	prompt_pack_name, prompt_pack_version,
	cohort_id, variant, virtual_user_id`

// nullableSessionFields groups nullable columns scanned from a session row.
type nullableSessionFields struct {
	workspaceName     *string
	lastMsgPreview    *string
	promptPackName    *string
	promptPackVersion *string
	cohortID          *string
	variant           *string
	expiresAt         *time.Time
	endedAt           *time.Time
	stateJSON         []byte
}

// Compile-time interface check for the optional StatusUpdaterWithResult.
var _ providers.StatusUpdaterWithResult = (*Provider)(nil)

var partitionTables = []string{"sessions", "messages", "tool_calls", "provider_calls", "runtime_events", "message_artifacts", "audit_log"}

// partBoundRe matches partition range expressions like:
// FOR VALUES FROM ('2025-01-06 00:00:00+00') TO ('2025-01-13 00:00:00+00')
var partBoundRe = regexp.MustCompile(`FROM \('([^']+)'\) TO \('([^']+)'\)`)

// partitionDateLayouts lists time formats used by pg_get_expr for partition bounds.
var partitionDateLayouts = []string{
	"2006-01-02 15:04:05-07",
	"2006-01-02 15:04:05+00",
}

func (p *Provider) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

func (p *Provider) Close() error {
	if p.ownsPool {
		p.pool.Close()
	}
	return nil
}
