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
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/altairalabs/omnia/internal/pgutil"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// Compile-time interface check.
var _ providers.WarmStoreProvider = (*Provider)(nil)

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

// --- row scanners -----------------------------------------------------------

// sessionColumns is the SELECT column list for sessions (no trailing comma).
const sessionColumns = `id, agent_name, namespace, workspace_name, status,
	created_at, updated_at, expires_at, ended_at,
	message_count, tool_call_count, total_input_tokens, total_output_tokens,
	estimated_cost_usd, tags, state, last_message_preview,
	prompt_pack_name, prompt_pack_version`

// nullableSessionFields groups nullable columns scanned from a session row.
type nullableSessionFields struct {
	workspaceName     *string
	lastMsgPreview    *string
	promptPackName    *string
	promptPackVersion *string
	expiresAt         *time.Time
	endedAt           *time.Time
	stateJSON         []byte
}

// populateSession fills nullable fields on a scanned session.
func populateSession(s *session.Session, n nullableSessionFields) {
	s.WorkspaceName = pgutil.DerefString(n.workspaceName)
	s.ExpiresAt = pgutil.TimeOrZero(n.expiresAt)
	s.EndedAt = pgutil.TimeOrZero(n.endedAt)
	s.State = pgutil.UnmarshalJSONB(n.stateJSON)
	s.LastMessagePreview = pgutil.DerefString(n.lastMsgPreview)
	s.PromptPackName = pgutil.DerefString(n.promptPackName)
	s.PromptPackVersion = pgutil.DerefString(n.promptPackVersion)
	if s.Tags == nil {
		s.Tags = []string{}
	}
}

func scanSession(row pgx.Row) (*session.Session, error) {
	var s session.Session
	var n nullableSessionFields

	err := row.Scan(
		&s.ID, &s.AgentName, &s.Namespace, &n.workspaceName, &s.Status,
		&s.CreatedAt, &s.UpdatedAt, &n.expiresAt, &n.endedAt,
		&s.MessageCount, &s.ToolCallCount, &s.TotalInputTokens, &s.TotalOutputTokens,
		&s.EstimatedCostUSD, &s.Tags, &n.stateJSON, &n.lastMsgPreview,
		&n.promptPackName, &n.promptPackVersion,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, session.ErrSessionNotFound
		}
		return nil, fmt.Errorf("postgres: scan session: %w", err)
	}

	populateSession(&s, n)
	return &s, nil
}

func scanMessage(row pgx.Row) (*session.Message, error) {
	var m session.Message
	var toolCallID *string
	var inputTokens, outputTokens *int32
	var metadataJSON []byte

	err := row.Scan(
		&m.ID, &m.Role, &m.Content, &m.Timestamp,
		&inputTokens, &outputTokens, &m.CostUSD,
		&toolCallID, &metadataJSON, &m.SequenceNum,
		&m.HasMedia, &m.MediaTypes,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: scan message: %w", err)
	}

	m.ToolCallID = pgutil.DerefString(toolCallID)
	m.Metadata = pgutil.UnmarshalJSONB(metadataJSON)
	if inputTokens != nil {
		m.InputTokens = *inputTokens
	}
	if outputTokens != nil {
		m.OutputTokens = *outputTokens
	}
	if m.MediaTypes == nil {
		m.MediaTypes = []string{}
	}
	return &m, nil
}

func scanToolCall(row pgx.Row) (*session.ToolCall, error) {
	var tc session.ToolCall
	var durationMs *int64
	var errorMessage *string
	var argsJSON, resultJSON, labelsJSON []byte

	err := row.Scan(
		&tc.ID, &tc.SessionID, &tc.CallID, &tc.Name,
		&argsJSON, &resultJSON,
		&tc.Status, &durationMs,
		&errorMessage, &labelsJSON, &tc.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: scan tool call: %w", err)
	}

	if len(argsJSON) > 0 {
		_ = json.Unmarshal(argsJSON, &tc.Arguments)
	}
	if len(resultJSON) > 0 {
		_ = json.Unmarshal(resultJSON, &tc.Result)
	}
	if durationMs != nil {
		tc.DurationMs = *durationMs
	}
	tc.ErrorMessage = pgutil.DerefString(errorMessage)
	tc.Labels = pgutil.UnmarshalJSONB(labelsJSON)
	return &tc, nil
}

func scanProviderCall(row pgx.Row) (*session.ProviderCall, error) {
	var pc session.ProviderCall
	var durationMs *int64
	var finishReason, errorMessage *string
	var labelsJSON []byte

	err := row.Scan(
		&pc.ID, &pc.SessionID, &pc.Provider, &pc.Model,
		&pc.Status, &pc.InputTokens, &pc.OutputTokens, &pc.CachedTokens,
		&pc.CostUSD, &durationMs, &finishReason, &pc.ToolCallCount,
		&errorMessage, &labelsJSON, &pc.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: scan provider call: %w", err)
	}

	if durationMs != nil {
		pc.DurationMs = *durationMs
	}
	pc.FinishReason = pgutil.DerefString(finishReason)
	pc.ErrorMessage = pgutil.DerefString(errorMessage)
	pc.Labels = pgutil.UnmarshalJSONB(labelsJSON)
	return &pc, nil
}

// --- helper: session exists check -------------------------------------------

func (p *Provider) sessionExists(ctx context.Context, sessionID string) error {
	var exists bool
	err := p.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM sessions WHERE id=$1)", sessionID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("postgres: check session: %w", err)
	}
	if !exists {
		return session.ErrSessionNotFound
	}
	return nil
}

// --- helper: collect session list -------------------------------------------

func collectSessions(rows pgx.Rows) ([]*session.Session, error) {
	defer rows.Close()

	var sessions []*session.Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate sessions: %w", err)
	}
	if sessions == nil {
		sessions = []*session.Session{}
	}
	return sessions, nil
}

// --- WarmStoreProvider implementation ---------------------------------------

func (p *Provider) CreateSession(ctx context.Context, s *session.Session) error {
	query := `INSERT INTO sessions (
		id, agent_name, namespace, workspace_name, status,
		created_at, updated_at, expires_at, ended_at,
		message_count, tool_call_count, total_input_tokens, total_output_tokens,
		estimated_cost_usd, tags, state, last_message_preview,
		prompt_pack_name, prompt_pack_version
	) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19
	WHERE NOT EXISTS (SELECT 1 FROM sessions WHERE id=$1)`

	tags := s.Tags
	if tags == nil {
		tags = []string{}
	}

	_, err := p.pool.Exec(ctx, query,
		s.ID, s.AgentName, s.Namespace, pgutil.NullString(s.WorkspaceName), s.Status,
		s.CreatedAt, s.UpdatedAt, pgutil.NullTime(s.ExpiresAt), pgutil.NullTime(s.EndedAt),
		s.MessageCount, s.ToolCallCount, s.TotalInputTokens, s.TotalOutputTokens,
		s.EstimatedCostUSD, tags, pgutil.MarshalJSONB(s.State), pgutil.NullString(s.LastMessagePreview),
		pgutil.NullString(s.PromptPackName), pgutil.NullString(s.PromptPackVersion),
	)
	if err != nil {
		return fmt.Errorf("postgres: create session: %w", err)
	}
	// RowsAffected() == 0 means the session already exists (WHERE NOT EXISTS).
	// This is intentionally idempotent — retries after network errors are safe.
	return nil
}

func (p *Provider) GetSession(ctx context.Context, sessionID string) (*session.Session, error) {
	query := `SELECT ` + sessionColumns + ` FROM sessions WHERE id=$1 LIMIT 1`
	return scanSession(p.pool.QueryRow(ctx, query, sessionID))
}

func (p *Provider) UpdateSession(ctx context.Context, s *session.Session) error {
	query := `UPDATE sessions SET
		agent_name=$2, namespace=$3, workspace_name=$4, status=$5,
		updated_at=$6, expires_at=$7, ended_at=$8,
		message_count=$9, tool_call_count=$10, total_input_tokens=$11, total_output_tokens=$12,
		estimated_cost_usd=$13, tags=$14, state=$15, last_message_preview=$16,
		prompt_pack_name=$17, prompt_pack_version=$18
	WHERE id=$1`

	tags := s.Tags
	if tags == nil {
		tags = []string{}
	}

	res, err := p.pool.Exec(ctx, query,
		s.ID, s.AgentName, s.Namespace, pgutil.NullString(s.WorkspaceName), s.Status,
		s.UpdatedAt, pgutil.NullTime(s.ExpiresAt), pgutil.NullTime(s.EndedAt),
		s.MessageCount, s.ToolCallCount, s.TotalInputTokens, s.TotalOutputTokens,
		s.EstimatedCostUSD, tags, pgutil.MarshalJSONB(s.State), pgutil.NullString(s.LastMessagePreview),
		pgutil.NullString(s.PromptPackName), pgutil.NullString(s.PromptPackVersion),
	)
	if err != nil {
		return fmt.Errorf("postgres: update session: %w", err)
	}
	if res.RowsAffected() == 0 {
		return session.ErrSessionNotFound
	}
	return nil
}

func (p *Provider) DeleteSession(ctx context.Context, sessionID string) error {
	// Child rows (messages, tool_calls, message_artifacts, eval_results) are
	// removed automatically by the trg_session_cascade_delete trigger added in
	// migration 000014.
	res, err := p.pool.Exec(ctx, "DELETE FROM sessions WHERE id=$1", sessionID)
	if err != nil {
		return fmt.Errorf("postgres: delete session: %w", err)
	}
	if res.RowsAffected() == 0 {
		return session.ErrSessionNotFound
	}
	return nil
}

func (p *Provider) AppendMessage(ctx context.Context, sessionID string, msg *session.Message) error {
	if err := p.sessionExists(ctx, sessionID); err != nil {
		return err
	}

	// Auto-increment message_count only. Token/cost counters are derived from
	// provider_calls (via RecordProviderCall) and tool_call_count from tool_calls
	// (via RecordToolCall) to avoid double-counting.
	// Messages with a ToolCallID don't increment message_count.
	hasToolCallID := msg.ToolCallID != ""
	messageIncr := int32(1)
	if hasToolCallID {
		messageIncr = 0
	}

	mediaTypes := msg.MediaTypes
	if mediaTypes == nil {
		mediaTypes = []string{}
	}

	// Use a CTE to atomically insert the message and update message_count
	// in a single statement, preventing races between concurrent AppendMessage calls.
	query := `WITH ins AS (
		INSERT INTO messages (id, session_id, role, content, timestamp, input_tokens, output_tokens, cost_usd, tool_call_id, metadata, sequence_num, has_media, media_types)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING session_id
	)
	UPDATE sessions SET
		message_count = message_count + $14,
		updated_at = $15
	WHERE id = (SELECT session_id FROM ins)`

	_, err := p.pool.Exec(ctx, query,
		msg.ID, sessionID, msg.Role, msg.Content, msg.Timestamp,
		pgutil.NullInt32(msg.InputTokens), pgutil.NullInt32(msg.OutputTokens),
		msg.CostUSD,
		pgutil.NullString(msg.ToolCallID), pgutil.MarshalJSONB(msg.Metadata), msg.SequenceNum,
		msg.HasMedia, mediaTypes,
		messageIncr,
		time.Now(),
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return session.ErrSessionNotFound
		}
		return fmt.Errorf("postgres: append message: %w", err)
	}
	return nil
}

func (p *Provider) GetMessages(ctx context.Context, sessionID string, opts providers.MessageQueryOpts) ([]*session.Message, error) {
	if err := p.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}

	qb := &pgutil.QueryBuilder{}
	qb.Add("session_id=$?", sessionID)

	if opts.AfterSeq > 0 {
		qb.Add("sequence_num > $?", opts.AfterSeq)
	}
	if opts.BeforeSeq > 0 {
		qb.Add("sequence_num < $?", opts.BeforeSeq)
	}
	if len(opts.Roles) > 0 {
		qb.Add("role = ANY($?)", opts.Roles)
	}

	sort := "ASC"
	if opts.SortOrder == providers.SortDesc {
		sort = "DESC"
	}

	query := `SELECT id, role, content, timestamp, input_tokens, output_tokens, cost_usd, tool_call_id, metadata, sequence_num, has_media, media_types
		FROM messages WHERE 1=1` + qb.Where() + ` ORDER BY sequence_num ` + sort
	query = qb.AppendPagination(query, opts.Limit, opts.Offset)

	rows, err := p.pool.Query(ctx, query, qb.Args()...)
	if err != nil {
		return nil, fmt.Errorf("postgres: get messages: %w", err)
	}
	defer rows.Close()

	var msgs []*session.Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate messages: %w", err)
	}
	if msgs == nil {
		msgs = []*session.Message{}
	}
	return msgs, nil
}

func (p *Provider) ListSessions(ctx context.Context, opts providers.SessionListOpts) (*providers.SessionPage, error) {
	qb := &pgutil.QueryBuilder{}
	p.applySessionFilters(qb, opts)

	sort := "DESC"
	if opts.SortOrder == providers.SortAsc {
		sort = "ASC"
	}

	// Data query — no window function, uses index-backed LIMIT/OFFSET.
	dataQuery := `SELECT ` + sessionColumns + ` FROM sessions WHERE 1=1` + qb.Where() +
		` ORDER BY created_at ` + sort
	dataQuery = qb.AppendPagination(dataQuery, opts.Limit, opts.Offset)

	rows, err := p.pool.Query(ctx, dataQuery, qb.Args()...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list sessions: %w", err)
	}
	sessions, err := collectSessions(rows)
	if err != nil {
		return nil, err
	}

	// Separate count query — simple aggregate that can use index-only scans.
	countQB := &pgutil.QueryBuilder{}
	p.applySessionFilters(countQB, opts)
	countQuery := `SELECT count(*) FROM sessions WHERE 1=1` + countQB.Where()
	var totalCount int64
	if err := p.pool.QueryRow(ctx, countQuery, countQB.Args()...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("postgres: count sessions: %w", err)
	}

	return &providers.SessionPage{
		Sessions:   sessions,
		TotalCount: totalCount,
		HasMore:    int64(opts.Offset)+int64(len(sessions)) < totalCount,
	}, nil
}

func (p *Provider) SearchSessions(ctx context.Context, query string, opts providers.SessionListOpts) (*providers.SessionPage, error) {
	qb := &pgutil.QueryBuilder{}

	// CTE to find session IDs matching the FTS query.
	qb.Add("search_vector @@ plainto_tsquery('english', $?)", query)
	cteClauses := qb.Where()

	// Continue accumulating args for session filters.
	sessionQB := &pgutil.QueryBuilder{}
	sessionQB.SetArgs(qb.Args())
	p.applySessionFilters(sessionQB, opts)

	sort := "DESC"
	if opts.SortOrder == providers.SortAsc {
		sort = "ASC"
	}

	// Data query — no window function.
	dataSQL := `WITH matching AS (
		SELECT DISTINCT session_id FROM messages WHERE 1=1` + cteClauses + `
	) SELECT ` + sessionColumns + `
	FROM sessions s JOIN matching ms ON ms.session_id = s.id
	WHERE 1=1` + sessionQB.Where() +
		` ORDER BY s.created_at ` + sort
	dataSQL = sessionQB.AppendPagination(dataSQL, opts.Limit, opts.Offset)

	rows, err := p.pool.Query(ctx, dataSQL, sessionQB.Args()...)
	if err != nil {
		return nil, fmt.Errorf("postgres: search sessions: %w", err)
	}
	sessions, err := collectSessions(rows)
	if err != nil {
		return nil, err
	}

	// Separate count query.
	countQB := &pgutil.QueryBuilder{}
	countQB.Add("search_vector @@ plainto_tsquery('english', $?)", query)
	countCTE := countQB.Where()
	countSessionQB := &pgutil.QueryBuilder{}
	countSessionQB.SetArgs(countQB.Args())
	p.applySessionFilters(countSessionQB, opts)

	countSQL := `WITH matching AS (
		SELECT DISTINCT session_id FROM messages WHERE 1=1` + countCTE + `
	) SELECT count(*)
	FROM sessions s JOIN matching ms ON ms.session_id = s.id
	WHERE 1=1` + countSessionQB.Where()

	var totalCount int64
	if err := p.pool.QueryRow(ctx, countSQL, countSessionQB.Args()...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("postgres: count search sessions: %w", err)
	}

	return &providers.SessionPage{
		Sessions:   sessions,
		TotalCount: totalCount,
		HasMore:    int64(opts.Offset)+int64(len(sessions)) < totalCount,
	}, nil
}

func (p *Provider) applySessionFilters(qb *pgutil.QueryBuilder, opts providers.SessionListOpts) {
	if opts.AgentName != "" {
		qb.Add("agent_name=$?", opts.AgentName)
	}
	if opts.Namespace != "" {
		qb.Add("namespace=$?", opts.Namespace)
	}
	if opts.WorkspaceName != "" {
		qb.Add("workspace_name=$?", opts.WorkspaceName)
	}
	if opts.Status != "" {
		qb.Add("status=$?", string(opts.Status))
	}
	if len(opts.Tags) > 0 {
		qb.Add("tags @> $?", opts.Tags)
	}
	if !opts.CreatedAfter.IsZero() {
		qb.Add("created_at >= $?", opts.CreatedAfter)
	}
	if !opts.CreatedBefore.IsZero() {
		qb.Add("created_at < $?", opts.CreatedBefore)
	}
}

// UpdateSessionStats atomically updates session status and ended_at.
// Counter increments (messages, tool calls, tokens, cost) are auto-derived
// from AppendMessage and should not be set via this method.
func (p *Provider) UpdateSessionStats(ctx context.Context, sessionID string, update session.SessionStatsUpdate) error {
	query := `UPDATE sessions SET
		status = CASE
			WHEN status IN ('completed','error','expired') THEN status
			WHEN $2::text = '' THEN status
			ELSE $2::text END,
		updated_at = $3,
		ended_at = CASE WHEN $4::timestamptz IS NULL THEN ended_at ELSE $4::timestamptz END
	WHERE id = $1`

	res, err := p.pool.Exec(ctx, query,
		sessionID,
		string(update.SetStatus),
		time.Now(),
		pgutil.NullTime(update.SetEndedAt),
	)
	if err != nil {
		return fmt.Errorf("postgres: update session stats: %w", err)
	}
	if res.RowsAffected() == 0 {
		return session.ErrSessionNotFound
	}
	return nil
}

// --- Tool call and provider call management ---------------------------------

func (p *Provider) RecordToolCall(ctx context.Context, sessionID string, tc *session.ToolCall) error {
	if err := p.sessionExists(ctx, sessionID); err != nil {
		return err
	}

	// Each tool call lifecycle event (started, completed, failed, client_request)
	// is recorded as its own row. Rows sharing the same call_id represent the
	// same logical tool invocation. Only "pending" events (the initial start)
	// increment session.tool_call_count.
	isStart := tc.Status == session.ToolCallStatusPending
	toolCallIncr := int32(0)
	if isStart {
		toolCallIncr = 1
	}

	query := `WITH ins AS (
		INSERT INTO tool_calls (id, session_id, call_id, name, arguments, result, status, duration_ms, error_message, labels, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING session_id
	)
	UPDATE sessions SET
		tool_call_count = tool_call_count + $12,
		updated_at = $13
	WHERE id = (SELECT session_id FROM ins)`

	argsJSON, _ := json.Marshal(tc.Arguments)
	var resultJSON []byte
	if tc.Result != nil {
		resultJSON, _ = json.Marshal(tc.Result)
	}

	_, err := p.pool.Exec(ctx, query,
		tc.ID, sessionID, tc.CallID, tc.Name,
		argsJSON, resultJSON,
		string(tc.Status), pgutil.NullInt64(tc.DurationMs),
		pgutil.NullString(tc.ErrorMessage),
		pgutil.MarshalJSONB(tc.Labels), tc.CreatedAt,
		toolCallIncr, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("postgres: record tool call: %w", err)
	}
	return nil
}

func (p *Provider) RecordProviderCall(ctx context.Context, sessionID string, pc *session.ProviderCall) error {
	if err := p.sessionExists(ctx, sessionID); err != nil {
		return err
	}

	// Each provider call lifecycle event (completed, failed) is its own row.
	// Only completed calls add tokens/cost to session counters.
	isCompleted := pc.Status == session.ProviderCallStatusCompleted
	var inputIncr, outputIncr int64
	var costIncr float64
	if isCompleted {
		inputIncr = pc.InputTokens
		outputIncr = pc.OutputTokens
		costIncr = pc.CostUSD
	}

	query := `WITH ins AS (
		INSERT INTO provider_calls (id, session_id, provider, model, status, input_tokens, output_tokens, cached_tokens, cost_usd, duration_ms, finish_reason, tool_call_count, error_message, labels, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING session_id
	)
	UPDATE sessions SET
		total_input_tokens = total_input_tokens + $16,
		total_output_tokens = total_output_tokens + $17,
		estimated_cost_usd = estimated_cost_usd + $18,
		updated_at = $19
	WHERE id = (SELECT session_id FROM ins)`

	_, err := p.pool.Exec(ctx, query,
		pc.ID, sessionID, pc.Provider, pc.Model,
		string(pc.Status), pc.InputTokens, pc.OutputTokens, pc.CachedTokens,
		pc.CostUSD, pgutil.NullInt64(pc.DurationMs),
		pgutil.NullString(pc.FinishReason), pc.ToolCallCount,
		pgutil.NullString(pc.ErrorMessage), pgutil.MarshalJSONB(pc.Labels),
		pc.CreatedAt,
		inputIncr, outputIncr, costIncr, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("postgres: record provider call: %w", err)
	}
	return nil
}

func (p *Provider) GetToolCalls(ctx context.Context, sessionID string) ([]*session.ToolCall, error) {
	if err := p.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}

	query := `SELECT id, session_id, call_id, name, arguments, result, status, duration_ms, error_message, labels, created_at
		FROM tool_calls WHERE session_id=$1 ORDER BY created_at ASC`

	rows, err := p.pool.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("postgres: get tool calls: %w", err)
	}
	defer rows.Close()

	var calls []*session.ToolCall
	for rows.Next() {
		tc, err := scanToolCall(rows)
		if err != nil {
			return nil, err
		}
		calls = append(calls, tc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate tool calls: %w", err)
	}
	if calls == nil {
		calls = []*session.ToolCall{}
	}
	return calls, nil
}

func (p *Provider) GetProviderCalls(ctx context.Context, sessionID string) ([]*session.ProviderCall, error) {
	if err := p.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}

	query := `SELECT id, session_id, provider, model, status, input_tokens, output_tokens, cached_tokens, cost_usd, duration_ms, finish_reason, tool_call_count, error_message, labels, created_at
		FROM provider_calls WHERE session_id=$1 ORDER BY created_at ASC`

	rows, err := p.pool.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("postgres: get provider calls: %w", err)
	}
	defer rows.Close()

	var calls []*session.ProviderCall
	for rows.Next() {
		pc, err := scanProviderCall(rows)
		if err != nil {
			return nil, err
		}
		calls = append(calls, pc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate provider calls: %w", err)
	}
	if calls == nil {
		calls = []*session.ProviderCall{}
	}
	return calls, nil
}

// --- Runtime event management -----------------------------------------------

func scanRuntimeEvent(row pgx.Row) (*session.RuntimeEvent, error) {
	var evt session.RuntimeEvent
	var durationMs *int64
	var errorMessage *string
	var dataJSON []byte

	err := row.Scan(
		&evt.ID, &evt.SessionID, &evt.EventType,
		&dataJSON, &durationMs, &errorMessage, &evt.Timestamp,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: scan runtime event: %w", err)
	}

	if len(dataJSON) > 0 {
		_ = json.Unmarshal(dataJSON, &evt.Data)
	}
	if durationMs != nil {
		evt.DurationMs = *durationMs
	}
	evt.ErrorMessage = pgutil.DerefString(errorMessage)
	return &evt, nil
}

func (p *Provider) RecordRuntimeEvent(ctx context.Context, sessionID string, evt *session.RuntimeEvent) error {
	if err := p.sessionExists(ctx, sessionID); err != nil {
		return err
	}

	query := `INSERT INTO runtime_events (id, session_id, event_type, data, duration_ms, error_message, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	dataJSON, _ := json.Marshal(evt.Data)

	_, err := p.pool.Exec(ctx, query,
		evt.ID, sessionID, evt.EventType,
		dataJSON, pgutil.NullInt64(evt.DurationMs),
		pgutil.NullString(evt.ErrorMessage), evt.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("postgres: record runtime event: %w", err)
	}
	return nil
}

func (p *Provider) GetRuntimeEvents(ctx context.Context, sessionID string) ([]*session.RuntimeEvent, error) {
	if err := p.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}

	query := `SELECT id, session_id, event_type, data, duration_ms, error_message, timestamp
		FROM runtime_events WHERE session_id=$1 ORDER BY timestamp ASC`

	rows, err := p.pool.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("postgres: get runtime events: %w", err)
	}
	defer rows.Close()

	var events []*session.RuntimeEvent
	for rows.Next() {
		evt, err := scanRuntimeEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, evt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate runtime events: %w", err)
	}
	if events == nil {
		events = []*session.RuntimeEvent{}
	}
	return events, nil
}

// --- Artifact management ----------------------------------------------------

func (p *Provider) SaveArtifact(ctx context.Context, artifact *session.Artifact) error {
	query := `INSERT INTO message_artifacts (id, message_id, session_id, artifact_type, mime_type,
		storage_uri, size_bytes, filename, checksum_sha256, metadata, created_at,
		width, height, duration_ms, channels, sample_rate)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`

	_, err := p.pool.Exec(ctx, query,
		artifact.ID, artifact.MessageID, artifact.SessionID,
		artifact.Type, artifact.MIMEType, artifact.StorageURI,
		pgutil.NullInt64(artifact.SizeBytes), pgutil.NullString(artifact.Filename),
		pgutil.NullString(artifact.Checksum), pgutil.MarshalJSONB(artifact.Metadata),
		artifact.CreatedAt,
		pgutil.NullInt32(artifact.Width), pgutil.NullInt32(artifact.Height),
		pgutil.NullInt32(artifact.DurationMs), pgutil.NullInt32(artifact.Channels),
		pgutil.NullInt32(artifact.SampleRate),
	)
	if err != nil {
		return fmt.Errorf("postgres: save artifact: %w", err)
	}
	return nil
}

func (p *Provider) GetArtifacts(ctx context.Context, messageID string) ([]*session.Artifact, error) {
	query := `SELECT id, message_id, session_id, artifact_type, mime_type, storage_uri,
		size_bytes, filename, checksum_sha256, metadata, created_at,
		width, height, duration_ms, channels, sample_rate
		FROM message_artifacts WHERE message_id=$1 ORDER BY created_at ASC`

	rows, err := p.pool.Query(ctx, query, messageID)
	if err != nil {
		return nil, fmt.Errorf("postgres: get artifacts: %w", err)
	}
	return collectArtifacts(rows)
}

func (p *Provider) GetSessionArtifacts(ctx context.Context, sessionID string) ([]*session.Artifact, error) {
	const maxSessionArtifacts = 1000
	query := `SELECT id, message_id, session_id, artifact_type, mime_type, storage_uri,
		size_bytes, filename, checksum_sha256, metadata, created_at,
		width, height, duration_ms, channels, sample_rate
		FROM message_artifacts WHERE session_id=$1 ORDER BY created_at ASC LIMIT $2`

	rows, err := p.pool.Query(ctx, query, sessionID, maxSessionArtifacts)
	if err != nil {
		return nil, fmt.Errorf("postgres: get session artifacts: %w", err)
	}
	return collectArtifacts(rows)
}

func (p *Provider) DeleteSessionArtifacts(ctx context.Context, sessionID string) error {
	_, err := p.pool.Exec(ctx, "DELETE FROM message_artifacts WHERE session_id=$1", sessionID)
	if err != nil {
		return fmt.Errorf("postgres: delete session artifacts: %w", err)
	}
	return nil
}

func scanArtifact(row pgx.Row) (*session.Artifact, error) {
	var a session.Artifact
	var sizeBytes *int64
	var filename, checksum *string
	var metadataJSON []byte
	var width, height, durationMs, channels, sampleRate *int32

	err := row.Scan(
		&a.ID, &a.MessageID, &a.SessionID, &a.Type, &a.MIMEType, &a.StorageURI,
		&sizeBytes, &filename, &checksum, &metadataJSON, &a.CreatedAt,
		&width, &height, &durationMs, &channels, &sampleRate,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, session.ErrArtifactNotFound
		}
		return nil, fmt.Errorf("postgres: scan artifact: %w", err)
	}

	if sizeBytes != nil {
		a.SizeBytes = *sizeBytes
	}
	a.Filename = pgutil.DerefString(filename)
	a.Checksum = pgutil.DerefString(checksum)
	a.Metadata = pgutil.UnmarshalJSONB(metadataJSON)
	if width != nil {
		a.Width = *width
	}
	if height != nil {
		a.Height = *height
	}
	if durationMs != nil {
		a.DurationMs = *durationMs
	}
	if channels != nil {
		a.Channels = *channels
	}
	if sampleRate != nil {
		a.SampleRate = *sampleRate
	}
	return &a, nil
}

func collectArtifacts(rows pgx.Rows) ([]*session.Artifact, error) {
	defer rows.Close()
	var artifacts []*session.Artifact
	for rows.Next() {
		a, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate artifacts: %w", err)
	}
	if artifacts == nil {
		artifacts = []*session.Artifact{}
	}
	return artifacts, nil
}

// --- Partition management ---------------------------------------------------

var partitionTables = []string{"sessions", "messages", "tool_calls", "provider_calls", "runtime_events", "message_artifacts", "audit_log"}

func (p *Provider) CreatePartition(ctx context.Context, date time.Time) error {
	// Align to ISO week boundary (Monday).
	isoYear, isoWeek := date.ISOWeek()
	weekStart := isoWeekStart(isoYear, isoWeek)
	weekEnd := weekStart.AddDate(0, 0, 7)

	var totalCreated int
	for _, table := range partitionTables {
		var created int
		err := p.pool.QueryRow(ctx,
			"SELECT create_weekly_partitions($1, $2::DATE, $3::DATE)",
			table, weekStart, weekEnd,
		).Scan(&created)
		if err != nil {
			return fmt.Errorf("postgres: create partition for %s: %w", table, err)
		}
		totalCreated += created
	}

	if totalCreated == 0 {
		return providers.ErrPartitionExists
	}
	return nil
}

func (p *Provider) DropPartition(ctx context.Context, date time.Time) error {
	isoYear, isoWeek := date.ISOWeek()
	suffix := fmt.Sprintf("w%04d_%02d", isoYear, isoWeek)

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Check that the sessions partition exists.
	var exists bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relname = $1 AND n.nspname = current_schema()
	)`, "sessions_"+suffix).Scan(&exists)
	if err != nil {
		return fmt.Errorf("postgres: check partition: %w", err)
	}
	if !exists {
		return providers.ErrPartitionNotFound
	}

	// Drop all table partitions in reverse dependency order.
	for _, table := range []string{"audit_log", "message_artifacts", "runtime_events", "provider_calls", "tool_calls", "messages", "sessions"} {
		name := pgx.Identifier{table + "_" + suffix}.Sanitize()
		_, err := tx.Exec(ctx, "DROP TABLE IF EXISTS "+name)
		if err != nil {
			return fmt.Errorf("postgres: drop partition %s: %w", name, err)
		}
	}

	return tx.Commit(ctx)
}

// partBoundRe matches partition range expressions like:
// FOR VALUES FROM ('2025-01-06 00:00:00+00') TO ('2025-01-13 00:00:00+00')
var partBoundRe = regexp.MustCompile(`FROM \('([^']+)'\) TO \('([^']+)'\)`)

// partitionDateLayouts lists time formats used by pg_get_expr for partition bounds.
var partitionDateLayouts = []string{
	"2006-01-02 15:04:05-07",
	"2006-01-02 15:04:05+00",
}

// parsePartitionDate tries each known layout to parse a partition bound timestamp.
func parsePartitionDate(s string) (time.Time, bool) {
	for _, layout := range partitionDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func (p *Provider) ListPartitions(ctx context.Context) ([]providers.PartitionInfo, error) {
	query := `SELECT c.relname,
		pg_get_expr(c.relpartbound, c.oid),
		pg_table_size(c.oid),
		pg_stat_get_live_tuples(c.oid)
	FROM pg_class c
	JOIN pg_inherits i ON i.inhrelid = c.oid
	JOIN pg_class parent ON parent.oid = i.inhparent
	JOIN pg_namespace n ON n.oid = parent.relnamespace
	WHERE parent.relname = 'sessions'
	AND n.nspname = current_schema()
	AND c.relispartition
	ORDER BY c.relname`

	rows, err := p.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("postgres: list partitions: %w", err)
	}
	defer rows.Close()

	var infos []providers.PartitionInfo
	for rows.Next() {
		var name, boundExpr string
		var sizeBytes, rowCount int64

		if err := rows.Scan(&name, &boundExpr, &sizeBytes, &rowCount); err != nil {
			return nil, fmt.Errorf("postgres: scan partition: %w", err)
		}

		matches := partBoundRe.FindStringSubmatch(boundExpr)
		if len(matches) != 3 {
			continue
		}

		startDate, ok := parsePartitionDate(matches[1])
		if !ok {
			continue
		}
		endDate, ok := parsePartitionDate(matches[2])
		if !ok {
			continue
		}

		infos = append(infos, providers.PartitionInfo{
			Name:      name,
			StartDate: startDate,
			EndDate:   endDate,
			RowCount:  rowCount,
			SizeBytes: sizeBytes,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate partitions: %w", err)
	}
	if infos == nil {
		infos = []providers.PartitionInfo{}
	}
	return infos, nil
}

// --- Batch operations -------------------------------------------------------

func (p *Provider) GetSessionsOlderThan(ctx context.Context, cutoff time.Time, batchSize int) ([]*session.Session, error) {
	query := `SELECT ` + sessionColumns + ` FROM sessions WHERE updated_at < $1 ORDER BY updated_at ASC LIMIT $2`

	rows, err := p.pool.Query(ctx, query, cutoff, batchSize)
	if err != nil {
		return nil, fmt.Errorf("postgres: get sessions older than: %w", err)
	}
	return collectSessions(rows)
}

func (p *Provider) DeleteSessionsBatch(ctx context.Context, sessionIDs []string) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	// Child rows are removed by the trg_session_cascade_delete trigger.
	_, err := p.pool.Exec(ctx, "DELETE FROM sessions WHERE id = ANY($1)", sessionIDs)
	if err != nil {
		return fmt.Errorf("postgres: delete sessions batch: %w", err)
	}
	return nil
}

// --- Infrastructure ---------------------------------------------------------

func (p *Provider) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

func (p *Provider) Close() error {
	if p.ownsPool {
		p.pool.Close()
	}
	return nil
}

// --- ISO week helper --------------------------------------------------------

// isoWeekStart returns the Monday 00:00 UTC of the given ISO year/week.
func isoWeekStart(isoYear, isoWeek int) time.Time {
	// Jan 4 is always in ISO week 1.
	jan4 := time.Date(isoYear, time.January, 4, 0, 0, 0, 0, time.UTC)
	// Go back to Monday of that week.
	offset := int(time.Monday - jan4.Weekday())
	if jan4.Weekday() == time.Sunday {
		offset = -6
	}
	week1Monday := jan4.AddDate(0, 0, offset)
	return week1Monday.AddDate(0, 0, (isoWeek-1)*7)
}
