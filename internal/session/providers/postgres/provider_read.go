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
	"time"

	"github.com/altairalabs/omnia/internal/pgutil"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

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

func (p *Provider) GetSession(ctx context.Context, sessionID string) (*session.Session, error) {
	query := `SELECT ` + sessionColumns + ` FROM sessions WHERE id=$1 LIMIT 1`
	return scanSession(p.pool.QueryRow(ctx, query, sessionID))
}

func (p *Provider) GetMessages(ctx context.Context, sessionID string, opts providers.MessageQueryOpts) ([]*session.Message, error) {
	if err := p.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}

	qb := &pgutil.QueryBuilder{}
	qb.Add(qbSessionID, sessionID)

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

	// Fetch limit+1 rows to determine HasMore without a separate COUNT(*).
	fetchLimit := opts.Limit
	if fetchLimit > 0 {
		fetchLimit++
	}

	dataQuery := `SELECT ` + sessionColumns + ` FROM sessions WHERE 1=1` + qb.Where() +
		` ORDER BY created_at ` + sort
	dataQuery = qb.AppendPagination(dataQuery, fetchLimit, opts.Offset)

	rows, err := p.pool.Query(ctx, dataQuery, qb.Args()...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list sessions: %w", err)
	}
	sessions, err := collectSessions(rows)
	if err != nil {
		return nil, err
	}

	hasMore := opts.Limit > 0 && len(sessions) > opts.Limit
	if hasMore {
		sessions = sessions[:opts.Limit]
	}

	totalCount := int64(-1)
	if opts.IncludeCount {
		totalCount, err = p.countSessions(ctx, opts)
		if err != nil {
			return nil, err
		}
	}

	return &providers.SessionPage{
		Sessions:   sessions,
		TotalCount: totalCount,
		HasMore:    hasMore,
	}, nil
}

func (p *Provider) SearchSessions(ctx context.Context, query string, opts providers.SessionListOpts) (*providers.SessionPage, error) {
	qb := &pgutil.QueryBuilder{}

	// EXISTS subquery: short-circuits after the first matching message per session.
	qb.Add("EXISTS (SELECT 1 FROM messages m WHERE m.session_id = s.id AND m.search_vector @@ plainto_tsquery('english', $?))", query)
	p.applySessionFilters(qb, opts)

	sort := "DESC"
	if opts.SortOrder == providers.SortAsc {
		sort = "ASC"
	}

	// Fetch limit+1 rows to determine HasMore without a separate COUNT(*).
	fetchLimit := opts.Limit
	if fetchLimit > 0 {
		fetchLimit++
	}

	dataSQL := `SELECT ` + sessionColumns + ` FROM sessions s WHERE 1=1` + qb.Where() +
		` ORDER BY s.created_at ` + sort
	dataSQL = qb.AppendPagination(dataSQL, fetchLimit, opts.Offset)

	rows, err := p.pool.Query(ctx, dataSQL, qb.Args()...)
	if err != nil {
		return nil, fmt.Errorf("postgres: search sessions: %w", err)
	}
	sessions, err := collectSessions(rows)
	if err != nil {
		return nil, err
	}

	hasMore := opts.Limit > 0 && len(sessions) > opts.Limit
	if hasMore {
		sessions = sessions[:opts.Limit]
	}

	totalCount := int64(-1)
	if opts.IncludeCount {
		totalCount, err = p.countSearchSessions(ctx, query, opts)
		if err != nil {
			return nil, err
		}
	}

	return &providers.SessionPage{
		Sessions:   sessions,
		TotalCount: totalCount,
		HasMore:    hasMore,
	}, nil
}

// countSessions runs a separate COUNT(*) query for ListSessions.
func (p *Provider) countSessions(ctx context.Context, opts providers.SessionListOpts) (int64, error) {
	countQB := &pgutil.QueryBuilder{}
	p.applySessionFilters(countQB, opts)
	countQuery := `SELECT count(*) FROM sessions WHERE 1=1` + countQB.Where()
	var total int64
	if err := p.pool.QueryRow(ctx, countQuery, countQB.Args()...).Scan(&total); err != nil {
		return 0, fmt.Errorf("postgres: count sessions: %w", err)
	}
	return total, nil
}

// countSearchSessions runs a separate COUNT(*) query for SearchSessions.
func (p *Provider) countSearchSessions(ctx context.Context, query string, opts providers.SessionListOpts) (int64, error) {
	countQB := &pgutil.QueryBuilder{}
	countQB.Add("EXISTS (SELECT 1 FROM messages m WHERE m.session_id = s.id AND m.search_vector @@ plainto_tsquery('english', $?))", query)
	p.applySessionFilters(countQB, opts)
	countSQL := `SELECT count(*) FROM sessions s WHERE 1=1` + countQB.Where()
	var total int64
	if err := p.pool.QueryRow(ctx, countSQL, countQB.Args()...).Scan(&total); err != nil {
		return 0, fmt.Errorf("postgres: count search sessions: %w", err)
	}
	return total, nil
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
	if opts.VirtualUserID != "" {
		qb.Add("virtual_user_id=$?", opts.VirtualUserID)
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

func (p *Provider) GetToolCalls(ctx context.Context, sessionID string, opts providers.PaginationOpts) ([]*session.ToolCall, error) {
	if err := p.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}

	qb := &pgutil.QueryBuilder{}
	qb.Add(qbSessionID, sessionID)

	query := `SELECT id, session_id, call_id, name, arguments, result, status, duration_ms, error_message, labels, created_at
		FROM tool_calls WHERE 1=1` + qb.Where() + ` ORDER BY created_at ASC`
	query = qb.AppendPagination(query, opts.Limit, opts.Offset)

	rows, err := p.pool.Query(ctx, query, qb.Args()...)
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

func (p *Provider) GetProviderCalls(ctx context.Context, sessionID string, opts providers.PaginationOpts) ([]*session.ProviderCall, error) {
	if err := p.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}

	qb := &pgutil.QueryBuilder{}
	qb.Add(qbSessionID, sessionID)

	query := `SELECT id, session_id, namespace, agent_name, provider, provider_name, model, status, input_tokens, output_tokens, cached_tokens, cost_usd, duration_ms, finish_reason, tool_call_count, error_message, labels, source, created_at
		FROM provider_calls WHERE 1=1` + qb.Where() + ` ORDER BY created_at ASC`
	query = qb.AppendPagination(query, opts.Limit, opts.Offset)

	rows, err := p.pool.Query(ctx, query, qb.Args()...)
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

func (p *Provider) GetRuntimeEvents(ctx context.Context, sessionID string, opts providers.PaginationOpts) ([]*session.RuntimeEvent, error) {
	if err := p.sessionExists(ctx, sessionID); err != nil {
		return nil, err
	}

	qb := &pgutil.QueryBuilder{}
	qb.Add(qbSessionID, sessionID)

	query := `SELECT id, session_id, event_type, data, duration_ms, error_message, timestamp
		FROM runtime_events WHERE 1=1` + qb.Where() + ` ORDER BY timestamp ASC`
	query = qb.AppendPagination(query, opts.Limit, opts.Offset)

	rows, err := p.pool.Query(ctx, query, qb.Args()...)
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

func (p *Provider) GetSessionsOlderThan(ctx context.Context, cutoff time.Time, batchSize int) ([]*session.Session, error) {
	query := `SELECT ` + sessionColumns + ` FROM sessions WHERE updated_at < $1 ORDER BY updated_at ASC LIMIT $2`

	rows, err := p.pool.Query(ctx, query, cutoff, batchSize)
	if err != nil {
		return nil, fmt.Errorf("postgres: get sessions older than: %w", err)
	}
	return collectSessions(rows)
}
