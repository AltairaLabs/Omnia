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
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/altairalabs/omnia/internal/pgutil"
	"github.com/altairalabs/omnia/internal/session"
)

// populateSession fills nullable fields on a scanned session.
func populateSession(s *session.Session, n nullableSessionFields) {
	s.WorkspaceName = pgutil.DerefString(n.workspaceName)
	s.ExpiresAt = pgutil.TimeOrZero(n.expiresAt)
	s.EndedAt = pgutil.TimeOrZero(n.endedAt)
	s.State = pgutil.UnmarshalJSONB(n.stateJSON)
	s.LastMessagePreview = pgutil.DerefString(n.lastMsgPreview)
	s.PromptPackName = pgutil.DerefString(n.promptPackName)
	s.PromptPackVersion = pgutil.DerefString(n.promptPackVersion)
	s.CohortID = pgutil.DerefString(n.cohortID)
	s.Variant = pgutil.DerefString(n.variant)
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
		&n.cohortID, &n.variant, &s.VirtualUserID,
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
	var finishReason, errorMessage, providerName *string
	var labelsJSON []byte

	err := row.Scan(
		&pc.ID, &pc.SessionID, &pc.Namespace, &pc.AgentName,
		&pc.Provider, &providerName, &pc.Model,
		&pc.Status, &pc.InputTokens, &pc.OutputTokens, &pc.CachedTokens,
		&pc.CostUSD, &durationMs, &finishReason, &pc.ToolCallCount,
		&errorMessage, &labelsJSON, &pc.Source, &pc.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: scan provider call: %w", err)
	}

	if durationMs != nil {
		pc.DurationMs = *durationMs
	}
	pc.ProviderName = pgutil.DerefString(providerName)
	pc.FinishReason = pgutil.DerefString(finishReason)
	pc.ErrorMessage = pgutil.DerefString(errorMessage)
	pc.Labels = pgutil.UnmarshalJSONB(labelsJSON)
	return &pc, nil
}

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
