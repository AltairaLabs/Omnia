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
	"context"
	"encoding/json"
	"fmt"

	"github.com/altairalabs/omnia/ee/pkg/encryption"
	"github.com/altairalabs/omnia/internal/session"
)

// Compile-time interface check.
var _ encryption.ReEncryptionStore = (*Provider)(nil)

// GetEncryptedMessageBatch returns messages encrypted with the given keyID
// that do not have the specified key version. Supports cursor-based pagination.
func (p *Provider) GetEncryptedMessageBatch(
	ctx context.Context, keyID, notKeyVersion string, batchSize int, afterMessageID string,
) ([]*encryption.EncryptedMessage, error) {
	query := `SELECT m.id, m.session_id, m.role, m.content, m.timestamp,
		m.input_tokens, m.output_tokens, m.tool_call_id, m.metadata, m.sequence_num
		FROM messages m
		WHERE m.metadata ? '_encryption'
			AND m.metadata->'_encryption'->>'keyID' = $1
			AND (m.metadata->'_encryption'->>'keyVersion' IS NULL
				OR m.metadata->'_encryption'->>'keyVersion' != $2)
			AND ($3 = '' OR m.id::text > $3)
		ORDER BY m.id
		LIMIT $4`

	rows, err := p.pool.Query(ctx, query, keyID, notKeyVersion, afterMessageID, batchSize)
	if err != nil {
		return nil, fmt.Errorf("querying encrypted messages: %w", err)
	}
	defer rows.Close()

	var results []*encryption.EncryptedMessage
	for rows.Next() {
		msg, sessionID, err := scanEncryptedMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning encrypted message: %w", err)
		}
		results = append(results, &encryption.EncryptedMessage{
			SessionID: sessionID,
			Message:   msg,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating encrypted messages: %w", err)
	}

	return results, nil
}

// UpdateMessageContent updates the content and metadata of an encrypted message.
func (p *Provider) UpdateMessageContent(ctx context.Context, _ string, msg *session.Message) error {
	query := `UPDATE messages SET content = $1, metadata = $2 WHERE id = $3`

	_, err := p.pool.Exec(ctx, query, msg.Content, marshalJSONB(msg.Metadata), msg.ID)
	if err != nil {
		return fmt.Errorf("updating message content: %w", err)
	}

	return nil
}

// scanner is a minimal interface for scanning a row from pgx.
type scanner interface {
	Scan(dest ...any) error
}

// scanEncryptedMessage scans a row into a session.Message and extracts the session_id.
func scanEncryptedMessage(row scanner) (*session.Message, string, error) {
	var msg session.Message
	var sessionID string
	var toolCallID *string
	var inputTokens, outputTokens *int32
	var metadataBytes []byte

	err := row.Scan(
		&msg.ID,
		&sessionID,
		&msg.Role,
		&msg.Content,
		&msg.Timestamp,
		&inputTokens,
		&outputTokens,
		&toolCallID,
		&metadataBytes,
		&msg.SequenceNum,
	)
	if err != nil {
		return nil, "", err
	}

	if inputTokens != nil {
		msg.InputTokens = *inputTokens
	}
	if outputTokens != nil {
		msg.OutputTokens = *outputTokens
	}
	if toolCallID != nil {
		msg.ToolCallID = *toolCallID
	}

	if len(metadataBytes) > 0 {
		if err := json.Unmarshal(metadataBytes, &msg.Metadata); err != nil {
			return nil, "", fmt.Errorf("unmarshaling metadata: %w", err)
		}
	}

	return &msg, sessionID, nil
}
