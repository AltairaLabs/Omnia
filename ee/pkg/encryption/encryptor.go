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

	"github.com/altairalabs/omnia/internal/session"
)

const encryptionMetadataKey = "_encryption"

// EncryptionEvent records a single encryption that occurred on a message.
type EncryptionEvent struct {
	// Field is the name of the field that was encrypted (e.g. "content", "metadata.key").
	Field string
	// KeyID is the key used for encryption.
	KeyID string
	// KeyVersion is the version of the key used.
	KeyVersion string
	// Algorithm is the encryption algorithm used.
	Algorithm string
}

// encryptionMetadata is stored in the message's _encryption metadata key.
type encryptionMetadata struct {
	KeyID      string   `json:"keyID"`
	KeyVersion string   `json:"keyVersion"`
	Algorithm  string   `json:"algorithm"`
	Fields     []string `json:"fields"`
}

// Encryptor performs message-level encryption and decryption.
type Encryptor interface {
	// EncryptMessage encrypts a session message, returning a copy with encrypted fields.
	EncryptMessage(ctx context.Context, msg *session.Message) (*session.Message, []EncryptionEvent, error)
	// DecryptMessage decrypts a session message, returning a copy with decrypted fields.
	DecryptMessage(ctx context.Context, msg *session.Message) (*session.Message, error)
}

type encryptor struct {
	provider Provider
}

// NewEncryptor creates a new Encryptor using the given Provider.
func NewEncryptor(provider Provider) Encryptor {
	return &encryptor{provider: provider}
}

func (e *encryptor) EncryptMessage(
	ctx context.Context, msg *session.Message,
) (*session.Message, []EncryptionEvent, error) {
	if msg == nil {
		return nil, nil, nil
	}

	// Shallow copy the message.
	encrypted := *msg

	// Deep copy metadata to avoid mutating the original.
	if msg.Metadata != nil {
		encrypted.Metadata = make(map[string]string, len(msg.Metadata))
		for k, v := range msg.Metadata {
			encrypted.Metadata[k] = v
		}
	}

	maxFields := 1 + len(encrypted.Metadata)
	events := make([]EncryptionEvent, 0, maxFields)
	fields := make([]string, 0, maxFields)
	var lastOutput *EncryptOutput

	// Encrypt content.
	if encrypted.Content != "" {
		out, err := e.provider.Encrypt(ctx, []byte(encrypted.Content))
		if err != nil {
			return nil, nil, fmt.Errorf("encrypting content: %w", err)
		}
		encrypted.Content = base64.StdEncoding.EncodeToString(out.Ciphertext)
		events = append(events, EncryptionEvent{
			Field:      "content",
			KeyID:      out.KeyID,
			KeyVersion: out.KeyVersion,
			Algorithm:  out.Algorithm,
		})
		fields = append(fields, "content")
		lastOutput = out
	}

	// Encrypt metadata values (except the _encryption key itself).
	for k, v := range encrypted.Metadata {
		if k == encryptionMetadataKey || v == "" {
			continue
		}
		out, err := e.provider.Encrypt(ctx, []byte(v))
		if err != nil {
			return nil, nil, fmt.Errorf("encrypting metadata %q: %w", k, err)
		}
		encrypted.Metadata[k] = base64.StdEncoding.EncodeToString(out.Ciphertext)
		events = append(events, EncryptionEvent{
			Field:      "metadata." + k,
			KeyID:      out.KeyID,
			KeyVersion: out.KeyVersion,
			Algorithm:  out.Algorithm,
		})
		fields = append(fields, "metadata."+k)
		lastOutput = out
	}

	// Store encryption metadata if anything was encrypted.
	if lastOutput != nil {
		meta := encryptionMetadata{
			KeyID:      lastOutput.KeyID,
			KeyVersion: lastOutput.KeyVersion,
			Algorithm:  lastOutput.Algorithm,
			Fields:     fields,
		}
		metaBytes, err := json.Marshal(meta)
		if err != nil {
			return nil, nil, fmt.Errorf("marshaling encryption metadata: %w", err)
		}
		if encrypted.Metadata == nil {
			encrypted.Metadata = make(map[string]string)
		}
		encrypted.Metadata[encryptionMetadataKey] = string(metaBytes)
	}

	return &encrypted, events, nil
}

func (e *encryptor) DecryptMessage(ctx context.Context, msg *session.Message) (*session.Message, error) {
	if msg == nil {
		return nil, nil
	}

	// Check if message has encryption metadata.
	metaStr, ok := msg.Metadata[encryptionMetadataKey]
	if !ok {
		// Not encrypted, return a copy.
		decrypted := *msg
		return &decrypted, nil
	}

	var meta encryptionMetadata
	if err := json.Unmarshal([]byte(metaStr), &meta); err != nil {
		return nil, fmt.Errorf("parsing encryption metadata: %w", err)
	}

	// Shallow copy the message.
	decrypted := *msg

	// Deep copy metadata.
	decrypted.Metadata = make(map[string]string, len(msg.Metadata))
	for k, v := range msg.Metadata {
		decrypted.Metadata[k] = v
	}

	// Build a set of encrypted fields for fast lookup.
	fieldSet := make(map[string]struct{}, len(meta.Fields))
	for _, f := range meta.Fields {
		fieldSet[f] = struct{}{}
	}

	// Decrypt content if it was encrypted.
	if _, ok := fieldSet["content"]; ok {
		ciphertext, err := base64.StdEncoding.DecodeString(decrypted.Content)
		if err != nil {
			return nil, fmt.Errorf("decoding content: %w", err)
		}
		plaintext, err := e.provider.Decrypt(ctx, ciphertext)
		if err != nil {
			return nil, fmt.Errorf("decrypting content: %w", err)
		}
		decrypted.Content = string(plaintext)
	}

	// Decrypt metadata values that were encrypted.
	for k, v := range decrypted.Metadata {
		if k == encryptionMetadataKey {
			continue
		}
		fieldKey := "metadata." + k
		if _, ok := fieldSet[fieldKey]; !ok {
			continue
		}
		ciphertext, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return nil, fmt.Errorf("decoding metadata %q: %w", k, err)
		}
		plaintext, err := e.provider.Decrypt(ctx, ciphertext)
		if err != nil {
			return nil, fmt.Errorf("decrypting metadata %q: %w", k, err)
		}
		decrypted.Metadata[k] = string(plaintext)
	}

	// Remove the _encryption metadata key from the decrypted message.
	delete(decrypted.Metadata, encryptionMetadataKey)

	return &decrypted, nil
}
