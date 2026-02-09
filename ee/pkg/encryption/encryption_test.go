/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMockWrapUnwrap creates a mock client that wraps/unwraps DEKs by XORing with a fixed key.
func newMockWrapUnwrap() *mockAzkeysClient {
	xorKey := []byte("mock-kms-wrapping-key-32bytes!!!")

	kid := azkeys.ID("https://myvault.vault.azure.net/keys/test-key/abc123")

	return &mockAzkeysClient{
		WrapKeyFn: func(
			ctx context.Context, keyName, keyVersion string,
			params azkeys.KeyOperationParameters, _ *azkeys.WrapKeyOptions,
		) (azkeys.WrapKeyResponse, error) {
			wrapped := make([]byte, len(params.Value))
			for i, b := range params.Value {
				wrapped[i] = b ^ xorKey[i%len(xorKey)]
			}
			return azkeys.WrapKeyResponse{
				KeyOperationResult: azkeys.KeyOperationResult{
					Result: wrapped,
					KID:    &kid,
				},
			}, nil
		},
		UnwrapKeyFn: func(
			ctx context.Context, keyName, keyVersion string,
			params azkeys.KeyOperationParameters, _ *azkeys.UnwrapKeyOptions,
		) (azkeys.UnwrapKeyResponse, error) {
			unwrapped := make([]byte, len(params.Value))
			for i, b := range params.Value {
				unwrapped[i] = b ^ xorKey[i%len(xorKey)]
			}
			return azkeys.UnwrapKeyResponse{
				KeyOperationResult: azkeys.KeyOperationResult{
					Result: unwrapped,
					KID:    &kid,
				},
			}, nil
		},
		GetKeyFn: func(
			ctx context.Context, keyName, keyVersion string,
			_ *azkeys.GetKeyOptions,
		) (azkeys.GetKeyResponse, error) {
			kid := azkeys.ID("https://myvault.vault.azure.net/keys/test-key/abc123")
			kty := azkeys.KeyTypeRSA
			enabled := true
			created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			return azkeys.GetKeyResponse{
				KeyBundle: azkeys.KeyBundle{
					Key: &azkeys.JSONWebKey{
						KID: &kid,
						Kty: &kty,
					},
					Attributes: &azkeys.KeyAttributes{
						Enabled: &enabled,
						Created: &created,
					},
				},
			}, nil
		},
	}
}

// --- Factory Tests ---

func TestNewProvider_AzureKeyVault(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		ProviderType: ProviderAzureKeyVault,
		KeyID:        "test-key",
		VaultURL:     "https://test.vault.azure.net",
	})
	// May succeed or fail depending on environment credentials;
	// either way, must NOT return ErrProviderNotImplemented.
	if err != nil {
		assert.False(t, errors.Is(err, ErrProviderNotImplemented))
	} else {
		assert.NotNil(t, p)
		_ = p.Close()
	}
}

func TestNewProvider_AWSKMS(t *testing.T) {
	_, err := NewProvider(ProviderConfig{ProviderType: ProviderAWSKMS})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrProviderNotImplemented))
}

func TestNewProvider_GCPKMS(t *testing.T) {
	_, err := NewProvider(ProviderConfig{ProviderType: ProviderGCPKMS})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrProviderNotImplemented))
}

func TestNewProvider_Vault(t *testing.T) {
	_, err := NewProvider(ProviderConfig{ProviderType: ProviderVault})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrProviderNotImplemented))
}

func TestNewProvider_Unknown(t *testing.T) {
	_, err := NewProvider(ProviderConfig{ProviderType: "unknown"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown encryption provider type")
}

// --- Azure Provider Tests ---

func TestAzureProvider_EncryptDecryptRoundTrip(t *testing.T) {
	mock := newMockWrapUnwrap()
	provider := newAzureKeyVaultProviderWithClient(mock, "test-key", "")

	ctx := context.Background()
	plaintext := []byte("Hello, World! This is sensitive data.")

	out, err := provider.Encrypt(ctx, plaintext)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "test-key", out.KeyID)
	assert.Equal(t, "AES-256-GCM+RSA-OAEP-256", out.Algorithm)
	assert.NotEmpty(t, out.Ciphertext)

	// Verify envelope structure.
	env, err := envelopeFromBytes(out.Ciphertext)
	require.NoError(t, err)
	assert.Equal(t, 1, env.Version)
	assert.NotEmpty(t, env.WrappedDEK)
	assert.NotEmpty(t, env.Nonce)
	assert.NotEmpty(t, env.Ciphertext)

	// Decrypt and verify.
	decrypted, err := provider.Decrypt(ctx, out.Ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestAzureProvider_EncryptEmptyPlaintext(t *testing.T) {
	mock := newMockWrapUnwrap()
	provider := newAzureKeyVaultProviderWithClient(mock, "test-key", "")

	ctx := context.Background()
	out, err := provider.Encrypt(ctx, []byte{})
	require.NoError(t, err)

	decrypted, err := provider.Decrypt(ctx, out.Ciphertext)
	require.NoError(t, err)
	assert.Empty(t, decrypted)
}

func TestAzureProvider_InvalidConfig_NoVaultURL(t *testing.T) {
	_, err := newAzureKeyVaultProvider(ProviderConfig{
		ProviderType: ProviderAzureKeyVault,
		KeyID:        "test-key",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vault URL is required")
}

func TestAzureProvider_InvalidConfig_NoKeyID(t *testing.T) {
	_, err := newAzureKeyVaultProvider(ProviderConfig{
		ProviderType: ProviderAzureKeyVault,
		VaultURL:     "https://test.vault.azure.net",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key ID is required")
}

func TestAzureProvider_WrapKeyError(t *testing.T) {
	mock := newMockWrapUnwrap()
	mock.WrapKeyFn = func(
		ctx context.Context, keyName, keyVersion string,
		params azkeys.KeyOperationParameters, _ *azkeys.WrapKeyOptions,
	) (azkeys.WrapKeyResponse, error) {
		return azkeys.WrapKeyResponse{}, fmt.Errorf("KMS unavailable")
	}
	provider := newAzureKeyVaultProviderWithClient(mock, "test-key", "")

	_, err := provider.Encrypt(context.Background(), []byte("test"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrEncryptionFailed))
	assert.Contains(t, err.Error(), "KMS wrap key failed")
}

func TestAzureProvider_UnwrapKeyError(t *testing.T) {
	mock := newMockWrapUnwrap()
	provider := newAzureKeyVaultProviderWithClient(mock, "test-key", "")

	out, err := provider.Encrypt(context.Background(), []byte("test"))
	require.NoError(t, err)

	mock.UnwrapKeyFn = func(
		ctx context.Context, keyName, keyVersion string,
		params azkeys.KeyOperationParameters, _ *azkeys.UnwrapKeyOptions,
	) (azkeys.UnwrapKeyResponse, error) {
		return azkeys.UnwrapKeyResponse{}, fmt.Errorf("KMS unavailable")
	}

	_, err = provider.Decrypt(context.Background(), out.Ciphertext)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptionFailed))
	assert.Contains(t, err.Error(), "KMS unwrap key failed")
}

func TestAzureProvider_DecryptInvalidEnvelope(t *testing.T) {
	provider := newAzureKeyVaultProviderWithClient(newMockWrapUnwrap(), "test-key", "")

	_, err := provider.Decrypt(context.Background(), []byte("not json"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptionFailed))
	assert.Contains(t, err.Error(), "invalid envelope")
}

func TestAzureProvider_DecryptWrongVersion(t *testing.T) {
	provider := newAzureKeyVaultProviderWithClient(newMockWrapUnwrap(), "test-key", "")

	_, err := provider.Decrypt(context.Background(), []byte(`{"v":99,"wdek":"","nonce":"","ct":""}`))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptionFailed))
	assert.Contains(t, err.Error(), "unsupported envelope version")
}

func TestAzureProvider_DecryptTamperedCiphertext(t *testing.T) {
	mock := newMockWrapUnwrap()
	provider := newAzureKeyVaultProviderWithClient(mock, "test-key", "")

	out, err := provider.Encrypt(context.Background(), []byte("secret"))
	require.NoError(t, err)

	env, err := envelopeFromBytes(out.Ciphertext)
	require.NoError(t, err)
	env.Ciphertext[0] ^= 0xFF

	tampered, err := json.Marshal(env)
	require.NoError(t, err)

	_, err = provider.Decrypt(context.Background(), tampered)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptionFailed))
}

func TestAzureProvider_GetKeyMetadata(t *testing.T) {
	mock := newMockWrapUnwrap()
	provider := newAzureKeyVaultProviderWithClient(mock, "test-key", "")

	meta, err := provider.GetKeyMetadata(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-key", meta.KeyID)
	assert.Equal(t, "abc123", meta.KeyVersion)
	assert.Equal(t, "RSA", meta.Algorithm)
	assert.True(t, meta.Enabled)
	assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), meta.CreatedAt)
}

func TestAzureProvider_GetKeyMetadataError(t *testing.T) {
	mock := newMockWrapUnwrap()
	mock.GetKeyFn = func(
		ctx context.Context, keyName, keyVersion string,
		_ *azkeys.GetKeyOptions,
	) (azkeys.GetKeyResponse, error) {
		return azkeys.GetKeyResponse{}, fmt.Errorf("key not found")
	}
	provider := newAzureKeyVaultProviderWithClient(mock, "test-key", "")

	_, err := provider.GetKeyMetadata(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrKeyNotFound))
}

// --- Encryptor Tests ---

func TestEncryptor_EncryptDecryptRoundTrip(t *testing.T) {
	mock := newMockWrapUnwrap()
	provider := newAzureKeyVaultProviderWithClient(mock, "test-key", "")
	enc := NewEncryptor(provider)
	ctx := context.Background()

	msg := &session.Message{
		ID:      "msg-1",
		Role:    session.RoleUser,
		Content: "My SSN is 123-45-6789",
		Metadata: map[string]string{
			"source": "web",
			"ip":     "192.168.1.1",
		},
	}

	encrypted, events, err := enc.EncryptMessage(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, encrypted)
	assert.Len(t, events, 3) // content + 2 metadata keys

	assert.NotEqual(t, msg.Content, encrypted.Content)
	assert.NotEqual(t, msg.Metadata["source"], encrypted.Metadata["source"])
	assert.NotEqual(t, msg.Metadata["ip"], encrypted.Metadata["ip"])

	assert.Equal(t, msg.ID, encrypted.ID)
	assert.Equal(t, msg.Role, encrypted.Role)
	assert.Contains(t, encrypted.Metadata, encryptionMetadataKey)

	decrypted, err := enc.DecryptMessage(ctx, encrypted)
	require.NoError(t, err)
	assert.Equal(t, msg.Content, decrypted.Content)
	assert.Equal(t, msg.Metadata["source"], decrypted.Metadata["source"])
	assert.Equal(t, msg.Metadata["ip"], decrypted.Metadata["ip"])
	assert.Equal(t, msg.ID, decrypted.ID)
	assert.Equal(t, msg.Role, decrypted.Role)

	_, hasEncMeta := decrypted.Metadata[encryptionMetadataKey]
	assert.False(t, hasEncMeta)
}

func TestEncryptor_NilMessage(t *testing.T) {
	enc := NewEncryptor(newAzureKeyVaultProviderWithClient(newMockWrapUnwrap(), "k", ""))

	encrypted, events, err := enc.EncryptMessage(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, encrypted)
	assert.Nil(t, events)

	decrypted, err := enc.DecryptMessage(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, decrypted)
}

func TestEncryptor_EmptyContent(t *testing.T) {
	enc := NewEncryptor(newAzureKeyVaultProviderWithClient(newMockWrapUnwrap(), "k", ""))
	ctx := context.Background()

	msg := &session.Message{
		ID:      "msg-2",
		Content: "",
	}

	encrypted, events, err := enc.EncryptMessage(ctx, msg)
	require.NoError(t, err)
	assert.Empty(t, events)
	assert.Equal(t, "", encrypted.Content)
}

func TestEncryptor_OriginalNotMutated(t *testing.T) {
	mock := newMockWrapUnwrap()
	provider := newAzureKeyVaultProviderWithClient(mock, "test-key", "")
	enc := NewEncryptor(provider)
	ctx := context.Background()

	original := &session.Message{
		ID:      "msg-3",
		Content: "sensitive data",
		Metadata: map[string]string{
			"key": "value",
		},
	}

	origContent := original.Content
	origMeta := original.Metadata["key"]

	_, _, err := enc.EncryptMessage(ctx, original)
	require.NoError(t, err)

	assert.Equal(t, origContent, original.Content)
	assert.Equal(t, origMeta, original.Metadata["key"])
	_, hasEncMeta := original.Metadata[encryptionMetadataKey]
	assert.False(t, hasEncMeta)
}

func TestEncryptor_DecryptUnencryptedMessage(t *testing.T) {
	enc := NewEncryptor(newAzureKeyVaultProviderWithClient(newMockWrapUnwrap(), "k", ""))

	msg := &session.Message{
		ID:      "msg-4",
		Content: "plain text",
	}

	decrypted, err := enc.DecryptMessage(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(t, msg.Content, decrypted.Content)
}

func TestEncryptor_NonContentFieldsPreserved(t *testing.T) {
	enc := NewEncryptor(newAzureKeyVaultProviderWithClient(newMockWrapUnwrap(), "k", ""))
	ctx := context.Background()

	ts := time.Now()
	msg := &session.Message{
		ID:           "msg-5",
		Role:         session.RoleAssistant,
		Content:      "response text",
		Timestamp:    ts,
		InputTokens:  100,
		OutputTokens: 200,
		ToolCallID:   "tc-1",
		SequenceNum:  5,
	}

	encrypted, _, err := enc.EncryptMessage(ctx, msg)
	require.NoError(t, err)
	assert.Equal(t, msg.ID, encrypted.ID)
	assert.Equal(t, msg.Role, encrypted.Role)
	assert.Equal(t, msg.Timestamp, encrypted.Timestamp)
	assert.Equal(t, msg.InputTokens, encrypted.InputTokens)
	assert.Equal(t, msg.OutputTokens, encrypted.OutputTokens)
	assert.Equal(t, msg.ToolCallID, encrypted.ToolCallID)
	assert.Equal(t, msg.SequenceNum, encrypted.SequenceNum)
}

// --- Benchmark ---

func BenchmarkEncryptDecryptMessage(b *testing.B) {
	mock := newMockWrapUnwrap()
	provider := newAzureKeyVaultProviderWithClient(mock, "test-key", "")
	enc := NewEncryptor(provider)
	ctx := context.Background()

	msg := &session.Message{
		ID:      "bench-msg",
		Content: "This is a benchmark message with some content to encrypt.",
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		encrypted, _, err := enc.EncryptMessage(ctx, msg)
		if err != nil {
			b.Fatal(err)
		}
		_, err = enc.DecryptMessage(ctx, encrypted)
		if err != nil {
			b.Fatal(err)
		}
	}
}
