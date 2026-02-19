/*
Copyright 2026 Altaira Labs.

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
	"testing"
	"time"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockScanRow implements the scanner interface for testing scanEncryptedMessage.
type mockScanRow struct {
	values []any
	err    error
}

func (m *mockScanRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	for i, d := range dest {
		if i >= len(m.values) {
			break
		}
		switch ptr := d.(type) {
		case *string:
			if v, ok := m.values[i].(string); ok {
				*ptr = v
			}
		case *session.MessageRole:
			if v, ok := m.values[i].(string); ok {
				*ptr = session.MessageRole(v)
			}
		case *time.Time:
			if v, ok := m.values[i].(time.Time); ok {
				*ptr = v
			}
		case **int32:
			if v, ok := m.values[i].(*int32); ok {
				*ptr = v
			}
		case **string:
			if v, ok := m.values[i].(*string); ok {
				*ptr = v
			}
		case *[]byte:
			if v, ok := m.values[i].([]byte); ok {
				*ptr = v
			}
		case *int32:
			if v, ok := m.values[i].(int32); ok {
				*ptr = v
			}
		}
	}
	return nil
}

func TestScanEncryptedMessage(t *testing.T) {
	metadata := map[string]string{
		"_encryption": `{"keyID":"test-key","keyVersion":"v1","algorithm":"AES-256-GCM","fields":["content"]}`,
	}
	metaBytes, err := json.Marshal(metadata)
	require.NoError(t, err)

	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	inputTokens := int32(100)
	outputTokens := int32(200)
	toolCallID := "tc-1"

	row := &mockScanRow{
		values: []any{
			"msg-1",
			"sess-1",
			"user",
			"encrypted-content",
			ts,
			&inputTokens,
			&outputTokens,
			&toolCallID,
			metaBytes,
			int32(1),
		},
	}

	msg, sessionID, err := scanEncryptedMessage(row)
	require.NoError(t, err)
	assert.Equal(t, "msg-1", msg.ID)
	assert.Equal(t, "sess-1", sessionID)
	assert.Equal(t, session.RoleUser, msg.Role)
	assert.Equal(t, "encrypted-content", msg.Content)
	assert.Equal(t, ts, msg.Timestamp)
	assert.Equal(t, int32(100), msg.InputTokens)
	assert.Equal(t, int32(200), msg.OutputTokens)
	assert.Equal(t, "tc-1", msg.ToolCallID)
	assert.Equal(t, int32(1), msg.SequenceNum)
	assert.Contains(t, msg.Metadata, "_encryption")
}

func TestScanEncryptedMessage_NullOptionals(t *testing.T) {
	metadata := map[string]string{
		"_encryption": `{"keyID":"test-key"}`,
	}
	metaBytes, _ := json.Marshal(metadata)
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	row := &mockScanRow{
		values: []any{
			"msg-2",
			"sess-2",
			"assistant",
			"content",
			ts,
			(*int32)(nil),
			(*int32)(nil),
			(*string)(nil),
			metaBytes,
			int32(0),
		},
	}

	msg, sessionID, err := scanEncryptedMessage(row)
	require.NoError(t, err)
	assert.Equal(t, "msg-2", msg.ID)
	assert.Equal(t, "sess-2", sessionID)
	assert.Equal(t, int32(0), msg.InputTokens)
	assert.Equal(t, int32(0), msg.OutputTokens)
	assert.Empty(t, msg.ToolCallID)
	assert.Equal(t, session.RoleAssistant, msg.Role)
	_ = sessionID
}

func TestScanEncryptedMessage_ScanError(t *testing.T) {
	row := &mockScanRow{err: assert.AnError}

	_, _, err := scanEncryptedMessage(row)
	require.Error(t, err)
}
