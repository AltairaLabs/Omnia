/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockReEncryptionStore is a test double for ReEncryptionStore.
type mockReEncryptionStore struct {
	GetEncryptedMessageBatchFn func(
		ctx context.Context, keyID, notKeyVersion string, batchSize int, afterMessageID string,
	) ([]*EncryptedMessage, error)
	UpdateMessageContentFn func(ctx context.Context, sessionID string, msg *session.Message) error
}

func (m *mockReEncryptionStore) GetEncryptedMessageBatch(
	ctx context.Context, keyID, notKeyVersion string, batchSize int, afterMessageID string,
) ([]*EncryptedMessage, error) {
	return m.GetEncryptedMessageBatchFn(ctx, keyID, notKeyVersion, batchSize, afterMessageID)
}

func (m *mockReEncryptionStore) UpdateMessageContent(
	ctx context.Context, sessionID string, msg *session.Message,
) error {
	return m.UpdateMessageContentFn(ctx, sessionID, msg)
}

func TestReEncryptBatch_RoundTrip(t *testing.T) {
	mock := newMockWrapUnwrap()
	provider := newAzureKeyVaultProviderWithClient(mock, "test-key", "")

	encryptor := NewEncryptor(provider)
	ctx := context.Background()

	original := &session.Message{
		ID:      "msg-1",
		Role:    session.RoleUser,
		Content: "sensitive data",
		Metadata: map[string]string{
			"source": "web",
		},
	}

	encrypted, _, err := encryptor.EncryptMessage(ctx, original)
	require.NoError(t, err)

	var updatedMsg *session.Message
	store := &mockReEncryptionStore{
		GetEncryptedMessageBatchFn: func(
			_ context.Context, _, _ string, _ int, _ string,
		) ([]*EncryptedMessage, error) {
			return []*EncryptedMessage{
				{SessionID: "sess-1", Message: encrypted},
			}, nil
		},
		UpdateMessageContentFn: func(_ context.Context, sessionID string, msg *session.Message) error {
			assert.Equal(t, "sess-1", sessionID)
			updatedMsg = msg
			return nil
		},
	}

	reEncryptor := NewMessageReEncryptor(provider, store)
	lastID, hasMore, result, err := reEncryptor.ReEncryptBatch(ctx, ReEncryptionConfig{
		KeyID:         "test-key",
		NotKeyVersion: "new-version",
		BatchSize:     10,
	})

	require.NoError(t, err)
	assert.False(t, hasMore)
	assert.Equal(t, "msg-1", lastID)
	assert.Equal(t, 1, result.MessagesProcessed)
	assert.Equal(t, 0, result.Errors)
	assert.NotNil(t, updatedMsg)

	// Verify the re-encrypted message can be decrypted.
	decrypted, err := encryptor.DecryptMessage(ctx, updatedMsg)
	require.NoError(t, err)
	assert.Equal(t, "sensitive data", decrypted.Content)
	assert.Equal(t, "web", decrypted.Metadata["source"])
}

func TestReEncryptBatch_EmptyBatch(t *testing.T) {
	provider := newAzureKeyVaultProviderWithClient(newMockWrapUnwrap(), "test-key", "")

	store := &mockReEncryptionStore{
		GetEncryptedMessageBatchFn: func(
			_ context.Context, _, _ string, _ int, _ string,
		) ([]*EncryptedMessage, error) {
			return nil, nil
		},
		UpdateMessageContentFn: func(_ context.Context, _ string, _ *session.Message) error {
			t.Fatal("should not be called")
			return nil
		},
	}

	reEncryptor := NewMessageReEncryptor(provider, store)
	lastID, hasMore, result, err := reEncryptor.ReEncryptBatch(context.Background(), ReEncryptionConfig{
		KeyID:     "test-key",
		BatchSize: 10,
	})

	require.NoError(t, err)
	assert.False(t, hasMore)
	assert.Empty(t, lastID)
	assert.Equal(t, 0, result.MessagesProcessed)
}

func TestReEncryptBatch_DecryptError(t *testing.T) {
	mock := newMockWrapUnwrap()
	provider := newAzureKeyVaultProviderWithClient(mock, "test-key", "")

	meta := encryptionMetadata{
		KeyID:     "test-key",
		Algorithm: "AES-256-GCM+RSA-OAEP-256",
		Fields:    []string{"content"},
	}
	metaBytes, _ := json.Marshal(meta)

	badMsg := &session.Message{
		ID:      "msg-bad",
		Content: base64.StdEncoding.EncodeToString([]byte("not-valid-envelope")),
		Metadata: map[string]string{
			encryptionMetadataKey: string(metaBytes),
		},
	}

	store := &mockReEncryptionStore{
		GetEncryptedMessageBatchFn: func(
			_ context.Context, _, _ string, _ int, _ string,
		) ([]*EncryptedMessage, error) {
			return []*EncryptedMessage{
				{SessionID: "sess-1", Message: badMsg},
			}, nil
		},
		UpdateMessageContentFn: func(_ context.Context, _ string, _ *session.Message) error {
			t.Fatal("should not be called for failed messages")
			return nil
		},
	}

	reEncryptor := NewMessageReEncryptor(provider, store)
	_, _, result, err := reEncryptor.ReEncryptBatch(context.Background(), ReEncryptionConfig{
		KeyID:     "test-key",
		BatchSize: 10,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.MessagesProcessed)
	assert.Equal(t, 1, result.Errors)
}

func TestReEncryptBatch_StoreUpdateError(t *testing.T) {
	mock := newMockWrapUnwrap()
	provider := newAzureKeyVaultProviderWithClient(mock, "test-key", "")

	encryptor := NewEncryptor(provider)
	ctx := context.Background()

	original := &session.Message{
		ID:      "msg-1",
		Content: "sensitive",
	}

	encrypted, _, err := encryptor.EncryptMessage(ctx, original)
	require.NoError(t, err)

	store := &mockReEncryptionStore{
		GetEncryptedMessageBatchFn: func(
			_ context.Context, _, _ string, _ int, _ string,
		) ([]*EncryptedMessage, error) {
			return []*EncryptedMessage{
				{SessionID: "sess-1", Message: encrypted},
			}, nil
		},
		UpdateMessageContentFn: func(_ context.Context, _ string, _ *session.Message) error {
			return fmt.Errorf("database error")
		},
	}

	reEncryptor := NewMessageReEncryptor(provider, store)
	_, _, result, err := reEncryptor.ReEncryptBatch(ctx, ReEncryptionConfig{
		KeyID:     "test-key",
		BatchSize: 10,
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.MessagesProcessed)
	assert.Equal(t, 1, result.Errors)
}

func TestReEncryptBatch_HasMore(t *testing.T) {
	provider := newAzureKeyVaultProviderWithClient(newMockWrapUnwrap(), "test-key", "")
	encryptor := NewEncryptor(provider)
	ctx := context.Background()

	batchSize := 2
	messages := make([]*EncryptedMessage, batchSize)
	for i := range batchSize {
		msg := &session.Message{
			ID:      fmt.Sprintf("msg-%d", i),
			Content: "data",
		}
		enc, _, err := encryptor.EncryptMessage(ctx, msg)
		require.NoError(t, err)
		messages[i] = &EncryptedMessage{SessionID: "sess-1", Message: enc}
	}

	store := &mockReEncryptionStore{
		GetEncryptedMessageBatchFn: func(
			_ context.Context, _, _ string, _ int, _ string,
		) ([]*EncryptedMessage, error) {
			return messages, nil
		},
		UpdateMessageContentFn: func(_ context.Context, _ string, _ *session.Message) error {
			return nil
		},
	}

	reEncryptor := NewMessageReEncryptor(provider, store)
	lastID, hasMore, result, err := reEncryptor.ReEncryptBatch(ctx, ReEncryptionConfig{
		KeyID:     "test-key",
		BatchSize: batchSize,
	})

	require.NoError(t, err)
	assert.True(t, hasMore)
	assert.Equal(t, "msg-1", lastID)
	assert.Equal(t, batchSize, result.MessagesProcessed)
}

func TestReEncryptBatch_FetchError(t *testing.T) {
	provider := newAzureKeyVaultProviderWithClient(newMockWrapUnwrap(), "test-key", "")

	store := &mockReEncryptionStore{
		GetEncryptedMessageBatchFn: func(
			_ context.Context, _, _ string, _ int, _ string,
		) ([]*EncryptedMessage, error) {
			return nil, fmt.Errorf("database error")
		},
		UpdateMessageContentFn: func(_ context.Context, _ string, _ *session.Message) error {
			return nil
		},
	}

	reEncryptor := NewMessageReEncryptor(provider, store)
	_, _, _, err := reEncryptor.ReEncryptBatch(context.Background(), ReEncryptionConfig{
		KeyID:     "test-key",
		BatchSize: 10,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching encrypted messages")
}
