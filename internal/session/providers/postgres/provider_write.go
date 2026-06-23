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
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/altairalabs/omnia/internal/pgutil"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

func (p *Provider) CreateSession(ctx context.Context, s *session.Session) error {
	query := `INSERT INTO sessions (
		id, agent_name, namespace, workspace_name, status,
		created_at, updated_at, expires_at, ended_at,
		message_count, tool_call_count, total_input_tokens, total_output_tokens,
		estimated_cost_usd, tags, state, last_message_preview,
		prompt_pack_name, prompt_pack_version,
		cohort_id, variant, virtual_user_id
	) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22
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
		pgutil.NullString(s.CohortID), pgutil.NullString(s.Variant), s.VirtualUserID,
	)
	if err != nil {
		return fmt.Errorf("postgres: create session: %w", err)
	}
	// RowsAffected() == 0 means the session already exists (WHERE NOT EXISTS).
	// This is intentionally idempotent — retries after network errors are safe.
	return nil
}

func (p *Provider) UpdateSession(ctx context.Context, s *session.Session) error {
	query := `UPDATE sessions SET
		agent_name=$2, namespace=$3, workspace_name=$4, status=$5,
		updated_at=$6, expires_at=$7, ended_at=$8,
		message_count=$9, tool_call_count=$10, total_input_tokens=$11, total_output_tokens=$12,
		estimated_cost_usd=$13, tags=$14, state=$15, last_message_preview=$16,
		prompt_pack_name=$17, prompt_pack_version=$18,
		cohort_id=$19, variant=$20, virtual_user_id=$21
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
		pgutil.NullString(s.CohortID), pgutil.NullString(s.Variant), s.VirtualUserID,
	)
	if err != nil {
		return fmt.Errorf("postgres: update session: %w", err)
	}
	if res.RowsAffected() == 0 {
		return session.ErrSessionNotFound
	}
	return nil
}

func (p *Provider) RefreshTTL(ctx context.Context, sessionID string, expiresAt time.Time) error {
	query := `UPDATE sessions SET expires_at = $2, updated_at = $3 WHERE id = $1`
	res, err := p.pool.Exec(ctx, query, sessionID, expiresAt, time.Now())
	if err != nil {
		return fmt.Errorf("postgres: refresh TTL: %w", err)
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

// DeleteSessionsByScope deletes all sessions matching the scope and returns the
// count. Child rows cascade via the trg_session_cascade_delete trigger (one
// fire per deleted session). Namespace is required so a delete can never span
// all workspaces.
func (p *Provider) DeleteSessionsByScope(ctx context.Context, scope providers.SessionDeleteScope) (int64, error) {
	if scope.Namespace == "" {
		return 0, fmt.Errorf("postgres: delete by scope: namespace is required")
	}
	query := "DELETE FROM sessions WHERE namespace = $1"
	args := []any{scope.Namespace}
	if scope.AgentName != "" {
		args = append(args, scope.AgentName)
		query += fmt.Sprintf(" AND agent_name = $%d", len(args))
	}
	if !scope.Before.IsZero() {
		args = append(args, scope.Before)
		query += fmt.Sprintf(" AND created_at < $%d", len(args))
	}
	res, err := p.pool.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("postgres: delete by scope: %w", err)
	}
	return res.RowsAffected(), nil
}

func (p *Provider) AppendMessage(ctx context.Context, sessionID string, msg *session.Message) error {
	// Default the partition-key timestamp to now() when zero, so a zero-value
	// (0001-01-01) doesn't fall outside every partition and trip
	// "no partition of relation found for row".
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
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

	// Use a CTE to atomically verify the session exists, insert the message,
	// and update message_count in a single round trip.
	query := `WITH sess AS (
		SELECT id FROM sessions WHERE id = $2
	), ins AS (
		INSERT INTO messages (id, session_id, role, content, timestamp, input_tokens, output_tokens, cost_usd, tool_call_id, metadata, sequence_num, has_media, media_types)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		WHERE EXISTS (SELECT 1 FROM sess)
		RETURNING session_id
	)
	UPDATE sessions SET
		message_count = message_count + $14,
		updated_at = $15,
		last_message_preview = CASE WHEN $9 IS NULL OR $9 = '' THEN LEFT($4, 200) ELSE last_message_preview END
	WHERE id = (SELECT session_id FROM ins)`

	res, err := p.pool.Exec(ctx, query,
		msg.ID, sessionID, msg.Role, msg.Content, msg.Timestamp,
		pgutil.NullInt32(msg.InputTokens), pgutil.NullInt32(msg.OutputTokens),
		msg.CostUSD,
		pgutil.NullString(msg.ToolCallID), pgutil.MarshalJSONB(msg.Metadata), msg.SequenceNum,
		msg.HasMedia, mediaTypes,
		messageIncr,
		time.Now(),
	)
	if err != nil {
		return fmt.Errorf("postgres: append message: %w", err)
	}
	if res.RowsAffected() == 0 {
		return session.ErrSessionNotFound
	}
	return nil
}

// UpdateSessionStatus atomically updates session status and ended_at.
// Delegates to UpdateSessionStatusReturning and discards the result metadata.
func (p *Provider) UpdateSessionStatus(ctx context.Context, sessionID string, update session.SessionStatusUpdate) error {
	_, err := p.UpdateSessionStatusReturning(ctx, sessionID, update)
	return err
}

// DecorateSession merges tags and state into an existing session without
// touching counters or lifecycle status. RemoveTags are dropped first, then
// AddTags append only values not already present (idempotent); state is
// shallow-merged via the jsonb || operator.
func (p *Provider) DecorateSession(ctx context.Context, sessionID string, opts session.DecorateSessionOptions) error {
	// `kept` = existing tags minus RemoveTags (empty RemoveTags keeps all, since
	// `t <> ALL('{}')` is vacuously true). Then append AddTags not already in kept,
	// preserving order. State is shallow-merged (right operand wins on collisions).
	query := `UPDATE sessions s SET
		tags = kept.arr || COALESCE(
			(SELECT array_agg(a) FROM unnest($3::text[]) AS a WHERE a <> ALL(kept.arr)),
			'{}'),
		state = COALESCE(s.state, '{}'::jsonb) || $4::jsonb,
		updated_at = $5
	FROM (
		SELECT COALESCE(
			(SELECT array_agg(t) FROM unnest(COALESCE((SELECT tags FROM sessions WHERE id = $1), '{}')) AS t
			 WHERE t <> ALL($2::text[])),
			'{}') AS arr
	) AS kept
	WHERE s.id = $1`

	removeTags := opts.RemoveTags
	if removeTags == nil {
		removeTags = []string{}
	}
	addTags := opts.AddTags
	if addTags == nil {
		addTags = []string{}
	}

	res, err := p.pool.Exec(ctx, query,
		sessionID, removeTags, addTags, pgutil.MarshalJSONB(opts.MergeState), time.Now(),
	)
	if err != nil {
		return fmt.Errorf("postgres: decorate session: %w", err)
	}
	if res.RowsAffected() == 0 {
		return session.ErrSessionNotFound
	}
	return nil
}

// UpdateSessionStatusReturning atomically updates session status and ended_at,
// returning the previous status and session metadata in the same query via a
// CTE + RETURNING clause. This eliminates the need for separate pre-check and
// post-read GetSession queries when detecting status transitions.
func (p *Provider) UpdateSessionStatusReturning(ctx context.Context, sessionID string, update session.SessionStatusUpdate) (*providers.StatusUpdateResult, error) {
	query := `WITH prev AS (
		SELECT id, status, agent_name, namespace, prompt_pack_name, prompt_pack_version
		FROM sessions WHERE id = $1 FOR UPDATE
	)
	UPDATE sessions SET
		status = CASE
			WHEN (SELECT status FROM prev) IN ('completed','error','expired') THEN status
			WHEN $2::text = '' THEN status
			ELSE $2::text END,
		updated_at = $3,
		ended_at = CASE WHEN $4::timestamptz IS NULL THEN ended_at ELSE $4::timestamptz END
	WHERE id = $1
	RETURNING
		(SELECT status FROM prev) AS previous_status,
		agent_name, namespace, prompt_pack_name, prompt_pack_version`

	var prevStatus string
	var agentName, namespace string
	var promptPackName, promptPackVersion *string

	err := p.pool.QueryRow(ctx, query,
		sessionID,
		string(update.SetStatus),
		time.Now(),
		pgutil.NullTime(update.SetEndedAt),
	).Scan(&prevStatus, &agentName, &namespace, &promptPackName, &promptPackVersion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, session.ErrSessionNotFound
		}
		return nil, fmt.Errorf("postgres: update session status: %w", err)
	}

	applied := !session.IsTerminalStatus(session.SessionStatus(prevStatus)) && update.SetStatus != ""
	return &providers.StatusUpdateResult{
		Applied:           applied,
		PreviousStatus:    session.SessionStatus(prevStatus),
		AgentName:         agentName,
		Namespace:         namespace,
		PromptPackName:    pgutil.DerefString(promptPackName),
		PromptPackVersion: pgutil.DerefString(promptPackVersion),
	}, nil
}

func (p *Provider) RecordToolCall(ctx context.Context, sessionID string, tc *session.ToolCall) error {
	// Default the partition-key timestamp to now() when zero, so a zero-value
	// (0001-01-01) doesn't fall outside every partition and trip
	// "no partition of relation found for row".
	if tc.CreatedAt.IsZero() {
		tc.CreatedAt = time.Now().UTC()
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

	query := `WITH sess AS (
		SELECT id FROM sessions WHERE id = $2
	), ins AS (
		INSERT INTO tool_calls (id, session_id, call_id, name, arguments, result, status, duration_ms, error_message, labels, created_at)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		WHERE EXISTS (SELECT 1 FROM sess)
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

	res, err := p.pool.Exec(ctx, query,
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
	if res.RowsAffected() == 0 {
		return session.ErrSessionNotFound
	}
	return nil
}

func (p *Provider) RecordProviderCall(ctx context.Context, sessionID string, pc *session.ProviderCall) error {
	// Default the partition-key timestamp to now() when zero, so a zero-value
	// (0001-01-01) doesn't fall outside every partition and trip
	// "no partition of relation found for row".
	if pc.CreatedAt.IsZero() {
		pc.CreatedAt = time.Now().UTC()
	}

	// Each provider call lifecycle event (completed, failed) is its own row.
	// Only completed agent calls add tokens/cost to session counters.
	// Judge and self-play calls are tracked separately.
	isAgent := pc.Source == "" || pc.Source == sourceAgent
	isCompleted := pc.Status == session.ProviderCallStatusCompleted
	var inputIncr, outputIncr int64
	var costIncr float64
	if isCompleted && isAgent {
		inputIncr = pc.InputTokens
		outputIncr = pc.OutputTokens
		costIncr = pc.CostUSD
	}

	query := `WITH sess AS (
		SELECT id FROM sessions WHERE id = $2
	), ins AS (
		INSERT INTO provider_calls (id, session_id, namespace, agent_name, provider, provider_name, model, status, input_tokens, output_tokens, cached_tokens, cost_usd, duration_ms, finish_reason, tool_call_count, error_message, labels, source, created_at)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
		WHERE EXISTS (SELECT 1 FROM sess)
		RETURNING session_id
	)
	UPDATE sessions SET
		total_input_tokens = total_input_tokens + $20,
		total_output_tokens = total_output_tokens + $21,
		estimated_cost_usd = estimated_cost_usd + $22,
		updated_at = $23
	WHERE id = (SELECT session_id FROM ins)`

	res, err := p.pool.Exec(ctx, query,
		pc.ID, sessionID, pc.Namespace, pc.AgentName,
		pc.Provider, pgutil.NullString(pc.ProviderName), pc.Model,
		string(pc.Status), pc.InputTokens, pc.OutputTokens, pc.CachedTokens,
		pc.CostUSD, pgutil.NullInt64(pc.DurationMs),
		pgutil.NullString(pc.FinishReason), pc.ToolCallCount,
		pgutil.NullString(pc.ErrorMessage), pgutil.MarshalJSONB(pc.Labels),
		pc.Source, pc.CreatedAt,
		inputIncr, outputIncr, costIncr, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("postgres: record provider call: %w", err)
	}
	if res.RowsAffected() == 0 {
		return session.ErrSessionNotFound
	}
	return nil
}

func (p *Provider) RecordRuntimeEvent(ctx context.Context, sessionID string, evt *session.RuntimeEvent) error {
	// Default the partition-key timestamp to now() when zero, so a zero-value
	// (0001-01-01) doesn't fall outside every partition and trip
	// "no partition of relation found for row".
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}

	query := `INSERT INTO runtime_events (id, session_id, event_type, data, duration_ms, error_message, timestamp)
		SELECT $1, $2, $3, $4, $5, $6, $7
		WHERE EXISTS (SELECT 1 FROM sessions WHERE id = $2)`

	dataJSON, _ := json.Marshal(evt.Data)

	res, err := p.pool.Exec(ctx, query,
		evt.ID, sessionID, evt.EventType,
		dataJSON, pgutil.NullInt64(evt.DurationMs),
		pgutil.NullString(evt.ErrorMessage), evt.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("postgres: record runtime event: %w", err)
	}
	if res.RowsAffected() == 0 {
		return session.ErrSessionNotFound
	}
	return nil
}

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

func (p *Provider) DeleteSessionArtifacts(ctx context.Context, sessionID string) error {
	_, err := p.pool.Exec(ctx, "DELETE FROM message_artifacts WHERE session_id=$1", sessionID)
	if err != nil {
		return fmt.Errorf("postgres: delete session artifacts: %w", err)
	}
	return nil
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
