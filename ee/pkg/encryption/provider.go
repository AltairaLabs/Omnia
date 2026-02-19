/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors for encryption operations.
var (
	// ErrProviderNotImplemented indicates the requested KMS provider is not yet available.
	ErrProviderNotImplemented = errors.New("KMS provider not implemented")
	// ErrKeyNotFound indicates the encryption key was not found in the KMS.
	ErrKeyNotFound = errors.New("encryption key not found")
	// ErrEncryptionFailed indicates encryption failed.
	ErrEncryptionFailed = errors.New("encryption failed")
	// ErrDecryptionFailed indicates decryption failed.
	ErrDecryptionFailed = errors.New("decryption failed")
	// ErrRotationFailed indicates key rotation failed.
	ErrRotationFailed = errors.New("key rotation failed")
)

// EncryptOutput holds the result of an encryption operation.
type EncryptOutput struct {
	// Ciphertext is the encrypted data (envelope-encrypted for KMS providers).
	Ciphertext []byte
	// KeyID is the identifier of the key used for encryption.
	KeyID string
	// KeyVersion is the version of the key used.
	KeyVersion string
	// Algorithm is the encryption algorithm used.
	Algorithm string
}

// KeyMetadata contains information about an encryption key.
type KeyMetadata struct {
	// KeyID is the identifier of the key.
	KeyID string
	// KeyVersion is the current version of the key.
	KeyVersion string
	// Algorithm is the key's algorithm.
	Algorithm string
	// CreatedAt is when the key was created.
	CreatedAt time.Time
	// ExpiresAt is when the key expires (zero means no expiry).
	ExpiresAt time.Time
	// Enabled indicates whether the key is active.
	Enabled bool
}

// KeyRotationResult holds the result of a key rotation operation.
type KeyRotationResult struct {
	// PreviousKeyVersion is the version of the key before rotation.
	PreviousKeyVersion string
	// NewKeyVersion is the version of the key after rotation.
	NewKeyVersion string
	// RotatedAt is when the rotation occurred.
	RotatedAt time.Time
}

// Provider defines the interface for KMS encryption providers.
type Provider interface {
	// Encrypt encrypts plaintext using the configured KMS key.
	Encrypt(ctx context.Context, plaintext []byte) (*EncryptOutput, error)
	// Decrypt decrypts ciphertext that was encrypted by this provider.
	Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
	// GetKeyMetadata returns metadata about the configured encryption key.
	GetKeyMetadata(ctx context.Context) (*KeyMetadata, error)
	// RotateKey triggers key rotation in the KMS provider, returning the new key version.
	RotateKey(ctx context.Context) (*KeyRotationResult, error)
	// Close releases any resources held by the provider.
	Close() error
}
