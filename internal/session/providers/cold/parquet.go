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

package cold

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/parquet-go/parquet-go"

	"github.com/altairalabs/omnia/internal/session"
)

// jsonNull is the JSON representation of a null value.
const jsonNull = "null"

// sessionRow is the Parquet row schema for archived sessions.
type sessionRow struct {
	ID                 string  `parquet:"id"`
	AgentName          string  `parquet:"agent_name"`
	Namespace          string  `parquet:"namespace"`
	WorkspaceName      string  `parquet:"workspace_name,optional"`
	Status             string  `parquet:"status"`
	CreatedAt          int64   `parquet:"created_at"`
	UpdatedAt          int64   `parquet:"updated_at"`
	ExpiresAt          int64   `parquet:"expires_at"`
	EndedAt            int64   `parquet:"ended_at"`
	MessageCount       int32   `parquet:"message_count"`
	ToolCallCount      int32   `parquet:"tool_call_count"`
	TotalInputTokens   int64   `parquet:"total_input_tokens"`
	TotalOutputTokens  int64   `parquet:"total_output_tokens"`
	EstimatedCostUSD   float64 `parquet:"estimated_cost_usd"`
	Tags               string  `parquet:"tags"`
	State              string  `parquet:"state"`
	LastMessagePreview string  `parquet:"last_message_preview,optional"`
	MessagesJSON       string  `parquet:"messages_json"`
}

// sessionToRow converts a Session to a Parquet row.
func sessionToRow(s *session.Session) sessionRow {
	tags, _ := json.Marshal(s.Tags)
	state, _ := json.Marshal(s.State)
	messages, _ := json.Marshal(s.Messages)

	var expiresAt, endedAt int64
	if !s.ExpiresAt.IsZero() {
		expiresAt = s.ExpiresAt.UnixNano()
	}
	if !s.EndedAt.IsZero() {
		endedAt = s.EndedAt.UnixNano()
	}

	return sessionRow{
		ID:                 s.ID,
		AgentName:          s.AgentName,
		Namespace:          s.Namespace,
		WorkspaceName:      s.WorkspaceName,
		Status:             string(s.Status),
		CreatedAt:          s.CreatedAt.UnixNano(),
		UpdatedAt:          s.UpdatedAt.UnixNano(),
		ExpiresAt:          expiresAt,
		EndedAt:            endedAt,
		MessageCount:       s.MessageCount,
		ToolCallCount:      s.ToolCallCount,
		TotalInputTokens:   s.TotalInputTokens,
		TotalOutputTokens:  s.TotalOutputTokens,
		EstimatedCostUSD:   s.EstimatedCostUSD,
		Tags:               string(tags),
		State:              string(state),
		LastMessagePreview: s.LastMessagePreview,
		MessagesJSON:       string(messages),
	}
}

// rowToSession converts a Parquet row back to a Session.
func rowToSession(r sessionRow) (*session.Session, error) {
	s := &session.Session{
		ID:                 r.ID,
		AgentName:          r.AgentName,
		Namespace:          r.Namespace,
		WorkspaceName:      r.WorkspaceName,
		Status:             session.SessionStatus(r.Status),
		CreatedAt:          time.Unix(0, r.CreatedAt).UTC(),
		UpdatedAt:          time.Unix(0, r.UpdatedAt).UTC(),
		MessageCount:       r.MessageCount,
		ToolCallCount:      r.ToolCallCount,
		TotalInputTokens:   r.TotalInputTokens,
		TotalOutputTokens:  r.TotalOutputTokens,
		EstimatedCostUSD:   r.EstimatedCostUSD,
		LastMessagePreview: r.LastMessagePreview,
	}

	if r.ExpiresAt != 0 {
		s.ExpiresAt = time.Unix(0, r.ExpiresAt).UTC()
	}
	if r.EndedAt != 0 {
		s.EndedAt = time.Unix(0, r.EndedAt).UTC()
	}

	if r.Tags != "" && r.Tags != jsonNull {
		if err := json.Unmarshal([]byte(r.Tags), &s.Tags); err != nil {
			return nil, fmt.Errorf("unmarshal tags: %w", err)
		}
	}
	if r.State != "" && r.State != jsonNull {
		if err := json.Unmarshal([]byte(r.State), &s.State); err != nil {
			return nil, fmt.Errorf("unmarshal state: %w", err)
		}
	}
	if r.MessagesJSON != "" && r.MessagesJSON != jsonNull {
		if err := json.Unmarshal([]byte(r.MessagesJSON), &s.Messages); err != nil {
			return nil, fmt.Errorf("unmarshal messages: %w", err)
		}
	}

	return s, nil
}

// writeParquetBytes serializes rows into Parquet format with Snappy compression.
func writeParquetBytes(rows []sessionRow) ([]byte, error) {
	var buf bytes.Buffer

	w := parquet.NewGenericWriter[sessionRow](&buf, parquet.Compression(&parquet.Snappy))

	if _, err := w.Write(rows); err != nil {
		return nil, fmt.Errorf("parquet write rows: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("parquet close: %w", err)
	}

	return buf.Bytes(), nil
}

// readParquetBytes deserializes Parquet data back into rows.
func readParquetBytes(data []byte) ([]sessionRow, error) {
	f, err := parquet.OpenFile(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("parquet open: %w", err)
	}

	r := parquet.NewGenericReader[sessionRow](f)
	rows := make([]sessionRow, r.NumRows())
	n, err := r.Read(rows)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parquet read: %w", err)
	}
	_ = r.Close()

	return rows[:n], nil
}
