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
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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

// --- nullable helpers -------------------------------------------------------

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func nullInt32(v int32) *int32 {
	if v == 0 {
		return nil
	}
	return &v
}

func marshalJSONB(m map[string]string) []byte {
	if m == nil {
		return []byte("{}")
	}
	b, _ := json.Marshal(m)
	return b
}

func unmarshalJSONB(data []byte) map[string]string {
	if len(data) == 0 {
		return nil
	}
	var m map[string]string
	if json.Unmarshal(data, &m) != nil || len(m) == 0 {
		return nil
	}
	return m
}

// --- row scanners -----------------------------------------------------------

// sessionColumns is the SELECT column list for sessions (no trailing comma).
const sessionColumns = `id, agent_name, namespace, workspace_name, status,
	created_at, updated_at, expires_at, ended_at,
	message_count, tool_call_count, total_input_tokens, total_output_tokens,
	estimated_cost_usd, tags, state, last_message_preview`

// populateSession fills nullable fields on a scanned session.
func populateSession(s *session.Session, workspaceName, lastMsgPreview *string, expiresAt, endedAt *time.Time, stateJSON []byte) {
	s.WorkspaceName = stringOrEmpty(workspaceName)
	s.ExpiresAt = timeOrZero(expiresAt)
	s.EndedAt = timeOrZero(endedAt)
	s.State = unmarshalJSONB(stateJSON)
	s.LastMessagePreview = stringOrEmpty(lastMsgPreview)
	if s.Tags == nil {
		s.Tags = []string{}
	}
}

func scanSession(row pgx.Row) (*session.Session, error) {
	var s session.Session
	var workspaceName, lastMsgPreview *string
	var expiresAt, endedAt *time.Time
	var stateJSON []byte

	err := row.Scan(
		&s.ID, &s.AgentName, &s.Namespace, &workspaceName, &s.Status,
		&s.CreatedAt, &s.UpdatedAt, &expiresAt, &endedAt,
		&s.MessageCount, &s.ToolCallCount, &s.TotalInputTokens, &s.TotalOutputTokens,
		&s.EstimatedCostUSD, &s.Tags, &stateJSON, &lastMsgPreview,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, session.ErrSessionNotFound
		}
		return nil, fmt.Errorf("postgres: scan session: %w", err)
	}

	populateSession(&s, workspaceName, lastMsgPreview, expiresAt, endedAt, stateJSON)
	return &s, nil
}

func scanSessionWithCount(row pgx.Row) (*session.Session, int64, error) {
	var s session.Session
	var workspaceName, lastMsgPreview *string
	var expiresAt, endedAt *time.Time
	var stateJSON []byte
	var totalCount int64

	err := row.Scan(
		&s.ID, &s.AgentName, &s.Namespace, &workspaceName, &s.Status,
		&s.CreatedAt, &s.UpdatedAt, &expiresAt, &endedAt,
		&s.MessageCount, &s.ToolCallCount, &s.TotalInputTokens, &s.TotalOutputTokens,
		&s.EstimatedCostUSD, &s.Tags, &stateJSON, &lastMsgPreview,
		&totalCount,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("postgres: scan session: %w", err)
	}

	populateSession(&s, workspaceName, lastMsgPreview, expiresAt, endedAt, stateJSON)
	return &s, totalCount, nil
}

func scanMessage(row pgx.Row) (*session.Message, error) {
	var m session.Message
	var toolCallID *string
	var inputTokens, outputTokens *int32
	var metadataJSON []byte

	err := row.Scan(
		&m.ID, &m.Role, &m.Content, &m.Timestamp,
		&inputTokens, &outputTokens,
		&toolCallID, &metadataJSON, &m.SequenceNum,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: scan message: %w", err)
	}

	m.ToolCallID = stringOrEmpty(toolCallID)
	m.Metadata = unmarshalJSONB(metadataJSON)
	if inputTokens != nil {
		m.InputTokens = *inputTokens
	}
	if outputTokens != nil {
		m.OutputTokens = *outputTokens
	}
	return &m, nil
}

// --- query builder ----------------------------------------------------------

type queryBuilder struct {
	clauses []string
	args    []any
}

func (qb *queryBuilder) add(clause string, arg any) {
	qb.args = append(qb.args, arg)
	qb.clauses = append(qb.clauses, strings.ReplaceAll(clause, "$?", "$"+strconv.Itoa(len(qb.args))))
}

func (qb *queryBuilder) where() string {
	if len(qb.clauses) == 0 {
		return ""
	}
	return " AND " + strings.Join(qb.clauses, " AND ")
}

// appendPagination adds LIMIT and OFFSET clauses to the query when non-zero.
func (qb *queryBuilder) appendPagination(query string, limit, offset int) string {
	if limit > 0 {
		qb.args = append(qb.args, limit)
		query += " LIMIT $" + strconv.Itoa(len(qb.args))
	}
	if offset > 0 {
		qb.args = append(qb.args, offset)
		query += " OFFSET $" + strconv.Itoa(len(qb.args))
	}
	return query
}

// --- helper: begin transaction ----------------------------------------------

func (p *Provider) beginTx(ctx context.Context) (pgx.Tx, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: begin tx: %w", err)
	}
	return tx, nil
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

// --- helper: collect session page -------------------------------------------

func collectSessionPage(rows pgx.Rows, offset int) (*providers.SessionPage, error) {
	defer rows.Close()

	var sessions []*session.Session
	var totalCount int64

	for rows.Next() {
		s, cnt, err := scanSessionWithCount(rows)
		if err != nil {
			return nil, err
		}
		totalCount = cnt
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate sessions: %w", err)
	}
	if sessions == nil {
		sessions = []*session.Session{}
	}

	return &providers.SessionPage{
		Sessions:   sessions,
		TotalCount: totalCount,
		HasMore:    int64(offset)+int64(len(sessions)) < totalCount,
	}, nil
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

// --- helper: delete child rows in transaction -------------------------------

// childTables lists tables with session_id FK in reverse dependency order.
var childTables = []string{"message_artifacts", "tool_calls", "messages"}

func deleteChildRows(ctx context.Context, tx pgx.Tx, sessionIDClause string, args ...any) error {
	for _, table := range childTables {
		if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE "+sessionIDClause, args...); err != nil {
			return fmt.Errorf("postgres: delete %s: %w", table, err)
		}
	}
	return nil
}

// --- WarmStoreProvider implementation ---------------------------------------

func (p *Provider) CreateSession(ctx context.Context, s *session.Session) error {
	query := `INSERT INTO sessions (
		id, agent_name, namespace, workspace_name, status,
		created_at, updated_at, expires_at, ended_at,
		message_count, tool_call_count, total_input_tokens, total_output_tokens,
		estimated_cost_usd, tags, state, last_message_preview
	) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17
	WHERE NOT EXISTS (SELECT 1 FROM sessions WHERE id=$1)`

	tags := s.Tags
	if tags == nil {
		tags = []string{}
	}

	res, err := p.pool.Exec(ctx, query,
		s.ID, s.AgentName, s.Namespace, nullString(s.WorkspaceName), s.Status,
		s.CreatedAt, s.UpdatedAt, nullTime(s.ExpiresAt), nullTime(s.EndedAt),
		s.MessageCount, s.ToolCallCount, s.TotalInputTokens, s.TotalOutputTokens,
		s.EstimatedCostUSD, tags, marshalJSONB(s.State), nullString(s.LastMessagePreview),
	)
	if err != nil {
		return fmt.Errorf("postgres: create session: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("postgres: create session: duplicate session ID %s", s.ID)
	}
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
		estimated_cost_usd=$13, tags=$14, state=$15, last_message_preview=$16
	WHERE id=$1`

	tags := s.Tags
	if tags == nil {
		tags = []string{}
	}

	res, err := p.pool.Exec(ctx, query,
		s.ID, s.AgentName, s.Namespace, nullString(s.WorkspaceName), s.Status,
		s.UpdatedAt, nullTime(s.ExpiresAt), nullTime(s.EndedAt),
		s.MessageCount, s.ToolCallCount, s.TotalInputTokens, s.TotalOutputTokens,
		s.EstimatedCostUSD, tags, marshalJSONB(s.State), nullString(s.LastMessagePreview),
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
	tx, err := p.beginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := deleteChildRows(ctx, tx, "session_id=$1", sessionID); err != nil {
		return err
	}

	res, err := tx.Exec(ctx, "DELETE FROM sessions WHERE id=$1", sessionID)
	if err != nil {
		return fmt.Errorf("postgres: delete session: %w", err)
	}
	if res.RowsAffected() == 0 {
		return session.ErrSessionNotFound
	}

	return tx.Commit(ctx)
}

func (p *Provider) AppendMessage(ctx context.Context, sessionID string, msg *session.Message) error {
	if err := p.sessionExists(ctx, sessionID); err != nil {
		return err
	}

	query := `INSERT INTO messages (id, session_id, role, content, timestamp, input_tokens, output_tokens, tool_call_id, metadata, sequence_num)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err := p.pool.Exec(ctx, query,
		msg.ID, sessionID, msg.Role, msg.Content, msg.Timestamp,
		nullInt32(msg.InputTokens), nullInt32(msg.OutputTokens),
		nullString(msg.ToolCallID), marshalJSONB(msg.Metadata), msg.SequenceNum,
	)
	if err != nil {
		return fmt.Errorf("postgres: append message: %w", err)
	}
	return nil
}

func (p *Provider) GetMessages(ctx context.Context, sessionID string, opts providers.MessageQueryOpts) ([]*session.Message, error) {
	if err := p.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}

	qb := &queryBuilder{}
	qb.add("session_id=$?", sessionID)

	if opts.AfterSeq > 0 {
		qb.add("sequence_num > $?", opts.AfterSeq)
	}
	if opts.BeforeSeq > 0 {
		qb.add("sequence_num < $?", opts.BeforeSeq)
	}
	if len(opts.Roles) > 0 {
		qb.add("role = ANY($?)", opts.Roles)
	}

	sort := "ASC"
	if opts.SortOrder == providers.SortDesc {
		sort = "DESC"
	}

	query := `SELECT id, role, content, timestamp, input_tokens, output_tokens, tool_call_id, metadata, sequence_num
		FROM messages WHERE 1=1` + qb.where() + ` ORDER BY sequence_num ` + sort
	query = qb.appendPagination(query, opts.Limit, opts.Offset)

	rows, err := p.pool.Query(ctx, query, qb.args...)
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
	qb := &queryBuilder{}
	p.applySessionFilters(qb, opts)

	sort := "DESC"
	if opts.SortOrder == providers.SortAsc {
		sort = "ASC"
	}

	query := `SELECT ` + sessionColumns + `, count(*) OVER() FROM sessions WHERE 1=1` + qb.where() +
		` ORDER BY created_at ` + sort
	query = qb.appendPagination(query, opts.Limit, opts.Offset)

	rows, err := p.pool.Query(ctx, query, qb.args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list sessions: %w", err)
	}
	return collectSessionPage(rows, opts.Offset)
}

func (p *Provider) SearchSessions(ctx context.Context, query string, opts providers.SessionListOpts) (*providers.SessionPage, error) {
	qb := &queryBuilder{}

	// CTE to find session IDs matching the FTS query.
	qb.add("search_vector @@ plainto_tsquery('english', $?)", query)
	cteClauses := qb.where()

	// Continue accumulating args for session filters.
	sessionQB := &queryBuilder{args: qb.args}
	p.applySessionFilters(sessionQB, opts)

	sort := "DESC"
	if opts.SortOrder == providers.SortAsc {
		sort = "ASC"
	}

	sql := `WITH matching AS (
		SELECT DISTINCT session_id FROM messages WHERE 1=1` + cteClauses + `
	) SELECT ` + sessionColumns + `, count(*) OVER()
	FROM sessions s JOIN matching ms ON ms.session_id = s.id
	WHERE 1=1` + sessionQB.where() +
		` ORDER BY s.created_at ` + sort
	sql = sessionQB.appendPagination(sql, opts.Limit, opts.Offset)

	rows, err := p.pool.Query(ctx, sql, sessionQB.args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: search sessions: %w", err)
	}
	return collectSessionPage(rows, opts.Offset)
}

func (p *Provider) applySessionFilters(qb *queryBuilder, opts providers.SessionListOpts) {
	if opts.AgentName != "" {
		qb.add("agent_name=$?", opts.AgentName)
	}
	if opts.Namespace != "" {
		qb.add("namespace=$?", opts.Namespace)
	}
	if opts.WorkspaceName != "" {
		qb.add("workspace_name=$?", opts.WorkspaceName)
	}
	if opts.Status != "" {
		qb.add("status=$?", string(opts.Status))
	}
	if len(opts.Tags) > 0 {
		qb.add("tags @> $?", opts.Tags)
	}
	if !opts.CreatedAfter.IsZero() {
		qb.add("created_at >= $?", opts.CreatedAfter)
	}
	if !opts.CreatedBefore.IsZero() {
		qb.add("created_at < $?", opts.CreatedBefore)
	}
}

// --- Partition management ---------------------------------------------------

var partitionTables = []string{"sessions", "messages", "tool_calls", "message_artifacts"}

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

	tx, err := p.beginTx(ctx)
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

	// Drop all 4 table partitions in reverse dependency order.
	for _, table := range []string{"message_artifacts", "tool_calls", "messages", "sessions"} {
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

	tx, err := p.beginTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := deleteChildRows(ctx, tx, "session_id = ANY($1)", sessionIDs); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE id = ANY($1)", sessionIDs); err != nil {
		return fmt.Errorf("postgres: delete sessions batch: %w", err)
	}

	return tx.Commit(ctx)
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
