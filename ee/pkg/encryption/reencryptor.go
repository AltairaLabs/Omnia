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
	"time"

	"github.com/altairalabs/omnia/internal/session"
)

// EncryptedMessage pairs a session message with its session ID for re-encryption.
type EncryptedMessage struct {
	SessionID string
	Message   *session.Message
}

// ReEncryptionStore defines the storage operations needed for re-encryption.
type ReEncryptionStore interface {
	// GetEncryptedMessageBatch returns messages encrypted with a specific key
	// that do not match the target key version.
	GetEncryptedMessageBatch(
		ctx context.Context, keyID, notKeyVersion string, batchSize int, afterMessageID string,
	) ([]*EncryptedMessage, error)
	// UpdateMessageContent updates the content and metadata of a message.
	UpdateMessageContent(ctx context.Context, sessionID string, msg *session.Message) error
}

// ReEncryptionConfig configures a re-encryption batch operation.
type ReEncryptionConfig struct {
	// KeyID is the encryption key identifier to filter messages by.
	KeyID string
	// NotKeyVersion is the key version to exclude (messages already on this version are skipped).
	NotKeyVersion string
	// BatchSize is the number of messages to process per batch.
	BatchSize int
	// AfterMessageID is the cursor for pagination (empty string for the first batch).
	AfterMessageID string
}

// ReEncryptionResult holds the outcome of a re-encryption batch.
type ReEncryptionResult struct {
	// MessagesProcessed is the number of messages successfully re-encrypted.
	MessagesProcessed int
	// MessagesSkipped is the number of messages that were skipped.
	MessagesSkipped int
	// Errors is the number of messages that failed re-encryption.
	Errors int
	// Duration is how long the batch took to process.
	Duration time.Duration
}

// MessageReEncryptor re-encrypts messages from an old key version to the current key.
type MessageReEncryptor struct {
	provider Provider
	store    ReEncryptionStore
}

// NewMessageReEncryptor creates a new MessageReEncryptor.
func NewMessageReEncryptor(provider Provider, store ReEncryptionStore) *MessageReEncryptor {
	return &MessageReEncryptor{
		provider: provider,
		store:    store,
	}
}

// ReEncryptBatch processes a batch of messages, re-encrypting them with the current key version.
// Returns the last message ID processed (for cursor-based pagination), whether more messages remain,
// the batch result, and any error.
func (r *MessageReEncryptor) ReEncryptBatch(
	ctx context.Context, cfg ReEncryptionConfig,
) (string, bool, *ReEncryptionResult, error) {
	start := time.Now()
	result := &ReEncryptionResult{}

	messages, err := r.store.GetEncryptedMessageBatch(
		ctx, cfg.KeyID, cfg.NotKeyVersion, cfg.BatchSize, cfg.AfterMessageID,
	)
	if err != nil {
		return "", false, nil, fmt.Errorf("fetching encrypted messages: %w", err)
	}

	if len(messages) == 0 {
		result.Duration = time.Since(start)
		return cfg.AfterMessageID, false, result, nil
	}

	lastID := cfg.AfterMessageID
	for _, encMsg := range messages {
		lastID = encMsg.Message.ID
		if r.reEncryptMessage(ctx, encMsg) != nil {
			result.Errors++
			continue
		}
		result.MessagesProcessed++
	}

	result.Duration = time.Since(start)
	hasMore := len(messages) == cfg.BatchSize
	return lastID, hasMore, result, nil
}

// reEncryptMessage decrypts and re-encrypts a single message.
func (r *MessageReEncryptor) reEncryptMessage(ctx context.Context, encMsg *EncryptedMessage) error {
	msg := encMsg.Message
	meta, err := r.parseEncryptionMeta(msg)
	if err != nil {
		return err
	}

	decrypted, err := r.decryptFields(ctx, msg, meta)
	if err != nil {
		return err
	}

	reEncrypted, err := r.encryptFields(ctx, decrypted, meta)
	if err != nil {
		return err
	}

	return r.store.UpdateMessageContent(ctx, encMsg.SessionID, reEncrypted)
}

// parseEncryptionMeta extracts the encryption metadata from a message.
func (r *MessageReEncryptor) parseEncryptionMeta(msg *session.Message) (*encryptionMetadata, error) {
	metaStr, ok := msg.Metadata[encryptionMetadataKey]
	if !ok {
		return nil, fmt.Errorf("message %s: missing encryption metadata", msg.ID)
	}

	var meta encryptionMetadata
	if err := json.Unmarshal([]byte(metaStr), &meta); err != nil {
		return nil, fmt.Errorf("message %s: invalid encryption metadata: %w", msg.ID, err)
	}

	return &meta, nil
}

// decryptFields decrypts the encrypted fields of a message.
func (r *MessageReEncryptor) decryptFields(
	ctx context.Context, msg *session.Message, meta *encryptionMetadata,
) (*session.Message, error) {
	decrypted := *msg
	decrypted.Metadata = copyMetadata(msg.Metadata)

	fieldSet := toFieldSet(meta.Fields)

	if _, ok := fieldSet["content"]; ok {
		plaintext, err := r.decryptField(ctx, decrypted.Content)
		if err != nil {
			return nil, fmt.Errorf("message %s: decrypting content: %w", msg.ID, err)
		}
		decrypted.Content = string(plaintext)
	}

	for k, v := range decrypted.Metadata {
		if k == encryptionMetadataKey {
			continue
		}
		if _, ok := fieldSet["metadata."+k]; !ok {
			continue
		}
		plaintext, err := r.decryptField(ctx, v)
		if err != nil {
			return nil, fmt.Errorf("message %s: decrypting metadata %q: %w", msg.ID, k, err)
		}
		decrypted.Metadata[k] = string(plaintext)
	}

	return &decrypted, nil
}

// encryptFields re-encrypts the fields and updates the encryption metadata.
func (r *MessageReEncryptor) encryptFields(
	ctx context.Context, msg *session.Message, meta *encryptionMetadata,
) (*session.Message, error) {
	reEncrypted := *msg
	reEncrypted.Metadata = copyMetadata(msg.Metadata)

	fieldSet := toFieldSet(meta.Fields)
	var lastOutput *EncryptOutput

	if _, ok := fieldSet["content"]; ok {
		out, err := r.provider.Encrypt(ctx, []byte(reEncrypted.Content))
		if err != nil {
			return nil, fmt.Errorf("message %s: re-encrypting content: %w", msg.ID, err)
		}
		reEncrypted.Content = base64.StdEncoding.EncodeToString(out.Ciphertext)
		lastOutput = out
	}

	for k, v := range reEncrypted.Metadata {
		if k == encryptionMetadataKey {
			continue
		}
		if _, ok := fieldSet["metadata."+k]; !ok {
			continue
		}
		out, err := r.provider.Encrypt(ctx, []byte(v))
		if err != nil {
			return nil, fmt.Errorf("message %s: re-encrypting metadata %q: %w", msg.ID, k, err)
		}
		reEncrypted.Metadata[k] = base64.StdEncoding.EncodeToString(out.Ciphertext)
		lastOutput = out
	}

	if lastOutput != nil {
		newMeta := encryptionMetadata{
			KeyID:      lastOutput.KeyID,
			KeyVersion: lastOutput.KeyVersion,
			Algorithm:  lastOutput.Algorithm,
			Fields:     meta.Fields,
		}
		metaBytes, err := json.Marshal(newMeta)
		if err != nil {
			return nil, fmt.Errorf("message %s: marshaling encryption metadata: %w", msg.ID, err)
		}
		reEncrypted.Metadata[encryptionMetadataKey] = string(metaBytes)
	}

	return &reEncrypted, nil
}

// decryptField decodes base64 and decrypts a single field value.
func (r *MessageReEncryptor) decryptField(ctx context.Context, encoded string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	return r.provider.Decrypt(ctx, ciphertext)
}

func copyMetadata(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func toFieldSet(fields []string) map[string]struct{} {
	set := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		set[f] = struct{}{}
	}
	return set
}
