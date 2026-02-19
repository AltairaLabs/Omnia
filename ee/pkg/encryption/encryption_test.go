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
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cloud.google.com/go/kms/apiv1/kmspb"
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
		RotateKeyFn: func(
			ctx context.Context, keyName string,
			_ *azkeys.RotateKeyOptions,
		) (azkeys.RotateKeyResponse, error) {
			kid := azkeys.ID("https://myvault.vault.azure.net/keys/test-key/def456")
			kty := azkeys.KeyTypeRSA
			enabled := true
			created := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
			return azkeys.RotateKeyResponse{
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
	p, err := NewProvider(ProviderConfig{
		ProviderType: ProviderAWSKMS,
		KeyID:        "arn:aws:kms:us-east-1:123456789012:key/test-key",
		Credentials:  map[string]string{"region": "us-east-1"},
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

func TestNewProvider_GCPKMS(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		ProviderType: ProviderGCPKMS,
		KeyID:        "projects/test/locations/global/keyRings/test/cryptoKeys/test",
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

func TestNewProvider_Vault(t *testing.T) {
	p, err := NewProvider(ProviderConfig{
		ProviderType: ProviderVault,
		KeyID:        "my-transit-key",
		VaultURL:     "https://vault.example.com:8200",
		Credentials:  map[string]string{"token": "s.test-token"},
	})
	// Must NOT return ErrProviderNotImplemented.
	if err != nil {
		assert.False(t, errors.Is(err, ErrProviderNotImplemented))
	} else {
		assert.NotNil(t, p)
		_ = p.Close()
	}
}

func TestNewProvider_Unknown(t *testing.T) {
	_, err := NewProvider(ProviderConfig{ProviderType: "unknown"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown encryption provider type")
}

// assertEncryptDecryptRoundTrip is a shared helper for provider round-trip tests.
func assertEncryptDecryptRoundTrip(t *testing.T, provider Provider, expectedKeyID, expectedAlgo string) {
	t.Helper()

	ctx := context.Background()
	plaintext := []byte("Hello, World! This is sensitive data.")

	out, err := provider.Encrypt(ctx, plaintext)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, expectedKeyID, out.KeyID)
	assert.Equal(t, expectedAlgo, out.Algorithm)
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

// --- Azure Provider Tests ---

func TestAzureProvider_EncryptDecryptRoundTrip(t *testing.T) {
	mock := newMockWrapUnwrap()
	provider := newAzureKeyVaultProviderWithClient(mock, "test-key", "")
	assertEncryptDecryptRoundTrip(t, provider, "test-key", "AES-256-GCM+RSA-OAEP-256")
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

func TestAzureProvider_RotateKey(t *testing.T) {
	mock := newMockWrapUnwrap()
	provider := newAzureKeyVaultProviderWithClient(mock, "test-key", "")

	result, err := provider.RotateKey(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "abc123", result.PreviousKeyVersion)
	assert.Equal(t, "def456", result.NewKeyVersion)
	assert.False(t, result.RotatedAt.IsZero())
}

func TestAzureProvider_RotateKeyError(t *testing.T) {
	mock := newMockWrapUnwrap()
	mock.RotateKeyFn = func(
		_ context.Context, _ string,
		_ *azkeys.RotateKeyOptions,
	) (azkeys.RotateKeyResponse, error) {
		return azkeys.RotateKeyResponse{}, fmt.Errorf("KMS unavailable")
	}
	provider := newAzureKeyVaultProviderWithClient(mock, "test-key", "")

	_, err := provider.RotateKey(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRotationFailed))
	assert.Contains(t, err.Error(), "Azure RotateKey failed")
}

// --- AWS KMS Provider Tests ---

func TestAWSKMSProvider_EncryptDecryptRoundTrip(t *testing.T) {
	mock := newMockKMSClient()
	keyID := "arn:aws:kms:us-east-1:123456789012:key/test-key"
	provider := newAWSKMSProviderWithClient(mock, keyID)
	assertEncryptDecryptRoundTrip(t, provider, keyID, "AES-256-GCM+AES-256-KMS")
}

func TestAWSKMSProvider_EncryptEmptyPlaintext(t *testing.T) {
	mock := newMockKMSClient()
	provider := newAWSKMSProviderWithClient(mock, "test-key")

	ctx := context.Background()
	out, err := provider.Encrypt(ctx, []byte{})
	require.NoError(t, err)

	decrypted, err := provider.Decrypt(ctx, out.Ciphertext)
	require.NoError(t, err)
	assert.Empty(t, decrypted)
}

func TestAWSKMSProvider_InvalidConfig_NoKeyID(t *testing.T) {
	_, err := newAWSKMSProvider(ProviderConfig{
		ProviderType: ProviderAWSKMS,
		Credentials:  map[string]string{"region": "us-east-1"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key ID is required")
}

func TestAWSKMSProvider_InvalidConfig_NoRegion(t *testing.T) {
	_, err := newAWSKMSProvider(ProviderConfig{
		ProviderType: ProviderAWSKMS,
		KeyID:        "test-key",
		Credentials:  map[string]string{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "region is required")
}

func TestAWSKMSProvider_GenerateDataKeyError(t *testing.T) {
	mock := newMockKMSClient()
	mock.GenerateDataKeyFn = func(
		_ context.Context, _ *kms.GenerateDataKeyInput, _ ...func(*kms.Options),
	) (*kms.GenerateDataKeyOutput, error) {
		return nil, fmt.Errorf("KMS unavailable")
	}
	provider := newAWSKMSProviderWithClient(mock, "test-key")

	_, err := provider.Encrypt(context.Background(), []byte("test"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrEncryptionFailed))
	assert.Contains(t, err.Error(), "KMS GenerateDataKey failed")
}

func TestAWSKMSProvider_EncryptInvalidDEKSize(t *testing.T) {
	mock := newMockKMSClient()
	mock.GenerateDataKeyFn = func(
		_ context.Context, params *kms.GenerateDataKeyInput, _ ...func(*kms.Options),
	) (*kms.GenerateDataKeyOutput, error) {
		// Return a DEK with invalid size (not 16, 24, or 32 bytes).
		return &kms.GenerateDataKeyOutput{
			Plaintext:      []byte("bad"),
			CiphertextBlob: []byte("wrapped"),
			KeyId:          params.KeyId,
		}, nil
	}
	provider := newAWSKMSProviderWithClient(mock, "test-key")

	_, err := provider.Encrypt(context.Background(), []byte("test"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrEncryptionFailed))
	assert.Contains(t, err.Error(), "AES cipher creation failed")
}

func TestAWSKMSProvider_DecryptInvalidDEKSize(t *testing.T) {
	mock := newMockKMSClient()
	provider := newAWSKMSProviderWithClient(mock, "test-key")

	// Encrypt normally first.
	out, err := provider.Encrypt(context.Background(), []byte("test"))
	require.NoError(t, err)

	// Override Decrypt to return invalid key size.
	mock.DecryptFn = func(
		_ context.Context, _ *kms.DecryptInput, _ ...func(*kms.Options),
	) (*kms.DecryptOutput, error) {
		return &kms.DecryptOutput{
			Plaintext: []byte("bad"),
		}, nil
	}

	_, err = provider.Decrypt(context.Background(), out.Ciphertext)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptionFailed))
	assert.Contains(t, err.Error(), "AES cipher creation failed")
}

func TestAWSKMSProvider_DecryptError(t *testing.T) {
	mock := newMockKMSClient()
	provider := newAWSKMSProviderWithClient(mock, "test-key")

	out, err := provider.Encrypt(context.Background(), []byte("test"))
	require.NoError(t, err)

	mock.DecryptFn = func(
		_ context.Context, _ *kms.DecryptInput, _ ...func(*kms.Options),
	) (*kms.DecryptOutput, error) {
		return nil, fmt.Errorf("KMS unavailable")
	}

	_, err = provider.Decrypt(context.Background(), out.Ciphertext)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptionFailed))
	assert.Contains(t, err.Error(), "KMS Decrypt failed")
}

func TestAWSKMSProvider_DecryptInvalidEnvelope(t *testing.T) {
	provider := newAWSKMSProviderWithClient(newMockKMSClient(), "test-key")

	_, err := provider.Decrypt(context.Background(), []byte("not json"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptionFailed))
	assert.Contains(t, err.Error(), "invalid envelope")
}

func TestAWSKMSProvider_DecryptWrongVersion(t *testing.T) {
	provider := newAWSKMSProviderWithClient(newMockKMSClient(), "test-key")

	_, err := provider.Decrypt(context.Background(), []byte(`{"v":99,"wdek":"","nonce":"","ct":""}`))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptionFailed))
	assert.Contains(t, err.Error(), "unsupported envelope version")
}

func TestAWSKMSProvider_DecryptTamperedCiphertext(t *testing.T) {
	mock := newMockKMSClient()
	provider := newAWSKMSProviderWithClient(mock, "test-key")

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

func TestAWSKMSProvider_GetKeyMetadata(t *testing.T) {
	mock := newMockKMSClient()
	provider := newAWSKMSProviderWithClient(mock, "arn:aws:kms:us-east-1:123456789012:key/test-key")

	meta, err := provider.GetKeyMetadata(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:kms:us-east-1:123456789012:key/test-key", meta.KeyID)
	assert.Equal(t, string(types.KeySpecSymmetricDefault), meta.Algorithm)
	assert.True(t, meta.Enabled)
	assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), meta.CreatedAt)
}

func TestAWSKMSProvider_GetKeyMetadataError(t *testing.T) {
	mock := newMockKMSClient()
	mock.DescribeKeyFn = func(
		_ context.Context, _ *kms.DescribeKeyInput, _ ...func(*kms.Options),
	) (*kms.DescribeKeyOutput, error) {
		return nil, fmt.Errorf("key not found")
	}
	provider := newAWSKMSProviderWithClient(mock, "test-key")

	_, err := provider.GetKeyMetadata(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrKeyNotFound))
}

func TestAWSKMSProvider_RotateKey(t *testing.T) {
	mock := newMockKMSClient()
	provider := newAWSKMSProviderWithClient(mock, "arn:aws:kms:us-east-1:123456789012:key/test-key")

	result, err := provider.RotateKey(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.RotatedAt.IsZero())
}

func TestAWSKMSProvider_RotateKeyError(t *testing.T) {
	mock := newMockKMSClient()
	mock.RotateKeyOnDemandFn = func(
		_ context.Context, _ *kms.RotateKeyOnDemandInput, _ ...func(*kms.Options),
	) (*kms.RotateKeyOnDemandOutput, error) {
		return nil, fmt.Errorf("KMS unavailable")
	}
	provider := newAWSKMSProviderWithClient(mock, "test-key")

	_, err := provider.RotateKey(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRotationFailed))
	assert.Contains(t, err.Error(), "AWS RotateKeyOnDemand failed")
}

// --- GCP KMS Provider Tests ---

func TestGCPKMSProvider_EncryptDecryptRoundTrip(t *testing.T) {
	mock := newMockGCPKMSClient()
	keyID := "projects/test/locations/global/keyRings/test/cryptoKeys/test"
	provider := newGCPKMSProviderWithClient(mock, keyID)
	assertEncryptDecryptRoundTrip(t, provider, keyID, "AES-256-GCM+GCP-KMS")
}

func TestGCPKMSProvider_EncryptEmptyPlaintext(t *testing.T) {
	mock := newMockGCPKMSClient()
	provider := newGCPKMSProviderWithClient(mock, "projects/test/locations/global/keyRings/test/cryptoKeys/test")

	ctx := context.Background()
	out, err := provider.Encrypt(ctx, []byte{})
	require.NoError(t, err)

	decrypted, err := provider.Decrypt(ctx, out.Ciphertext)
	require.NoError(t, err)
	assert.Empty(t, decrypted)
}

func TestGCPKMSProvider_InvalidConfig_NoKeyID(t *testing.T) {
	_, err := newGCPKMSProvider(ProviderConfig{
		ProviderType: ProviderGCPKMS,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key ID is required")
}

func TestGCPKMSProvider_EncryptKMSError(t *testing.T) {
	mock := newMockGCPKMSClient()
	mock.EncryptFn = func(_ context.Context, _ *kmspb.EncryptRequest) (*kmspb.EncryptResponse, error) {
		return nil, fmt.Errorf("KMS unavailable")
	}
	provider := newGCPKMSProviderWithClient(mock, "test-key")

	_, err := provider.Encrypt(context.Background(), []byte("test"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrEncryptionFailed))
	assert.Contains(t, err.Error(), "KMS Encrypt (wrap DEK) failed")
}

func TestGCPKMSProvider_DecryptKMSError(t *testing.T) {
	mock := newMockGCPKMSClient()
	provider := newGCPKMSProviderWithClient(mock, "test-key")

	out, err := provider.Encrypt(context.Background(), []byte("test"))
	require.NoError(t, err)

	mock.DecryptFn = func(_ context.Context, _ *kmspb.DecryptRequest) (*kmspb.DecryptResponse, error) {
		return nil, fmt.Errorf("KMS unavailable")
	}

	_, err = provider.Decrypt(context.Background(), out.Ciphertext)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptionFailed))
	assert.Contains(t, err.Error(), "KMS Decrypt failed")
}

func TestGCPKMSProvider_DecryptInvalidEnvelope(t *testing.T) {
	provider := newGCPKMSProviderWithClient(newMockGCPKMSClient(), "test-key")

	_, err := provider.Decrypt(context.Background(), []byte("not json"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptionFailed))
	assert.Contains(t, err.Error(), "invalid envelope")
}

func TestGCPKMSProvider_DecryptWrongVersion(t *testing.T) {
	provider := newGCPKMSProviderWithClient(newMockGCPKMSClient(), "test-key")

	_, err := provider.Decrypt(context.Background(), []byte(`{"v":99,"wdek":"","nonce":"","ct":""}`))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptionFailed))
	assert.Contains(t, err.Error(), "unsupported envelope version")
}

func TestGCPKMSProvider_DecryptTamperedCiphertext(t *testing.T) {
	mock := newMockGCPKMSClient()
	provider := newGCPKMSProviderWithClient(mock, "test-key")

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

func TestGCPKMSProvider_DecryptInvalidDEKSize(t *testing.T) {
	mock := newMockGCPKMSClient()
	provider := newGCPKMSProviderWithClient(mock, "test-key")

	out, err := provider.Encrypt(context.Background(), []byte("test"))
	require.NoError(t, err)

	// Override Decrypt to return invalid key size.
	mock.DecryptFn = func(_ context.Context, _ *kmspb.DecryptRequest) (*kmspb.DecryptResponse, error) {
		return &kmspb.DecryptResponse{
			Plaintext: []byte("bad"),
		}, nil
	}

	_, err = provider.Decrypt(context.Background(), out.Ciphertext)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptionFailed))
	assert.Contains(t, err.Error(), "AES cipher creation failed")
}

func TestGCPKMSProvider_GetKeyMetadata(t *testing.T) {
	mock := newMockGCPKMSClient()
	provider := newGCPKMSProviderWithClient(mock, "projects/test/locations/global/keyRings/test/cryptoKeys/test")

	meta, err := provider.GetKeyMetadata(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "projects/test/locations/global/keyRings/test/cryptoKeys/test", meta.KeyID)
	assert.Equal(t, "GOOGLE_SYMMETRIC_ENCRYPTION", meta.Algorithm)
	assert.True(t, meta.Enabled)
	assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), meta.CreatedAt)
}

func TestGCPKMSProvider_GetKeyMetadataDisabledKey(t *testing.T) {
	mock := newMockGCPKMSClient()
	mock.GetCryptoKeyFn = func(_ context.Context, req *kmspb.GetCryptoKeyRequest) (*kmspb.CryptoKey, error) {
		return &kmspb.CryptoKey{
			Name: req.Name,
			Primary: &kmspb.CryptoKeyVersion{
				State: kmspb.CryptoKeyVersion_DESTROYED,
			},
		}, nil
	}
	provider := newGCPKMSProviderWithClient(mock, "test-key")

	meta, err := provider.GetKeyMetadata(context.Background())
	require.NoError(t, err)
	assert.False(t, meta.Enabled)
}

func TestGCPKMSProvider_GetKeyMetadataError(t *testing.T) {
	mock := newMockGCPKMSClient()
	mock.GetCryptoKeyFn = func(_ context.Context, _ *kmspb.GetCryptoKeyRequest) (*kmspb.CryptoKey, error) {
		return nil, fmt.Errorf("key not found")
	}
	provider := newGCPKMSProviderWithClient(mock, "test-key")

	_, err := provider.GetKeyMetadata(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrKeyNotFound))
}

func TestGCPKMSProvider_Close(t *testing.T) {
	mock := newMockGCPKMSClient()
	provider := newGCPKMSProviderWithClient(mock, "test-key")

	err := provider.Close()
	require.NoError(t, err)
}

func TestGCPKMSProvider_RotateKey(t *testing.T) {
	mock := newMockGCPKMSClient()
	keyID := "projects/test/locations/global/keyRings/test/cryptoKeys/test"
	provider := newGCPKMSProviderWithClient(mock, keyID)

	result, err := provider.RotateKey(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Contains(t, result.NewKeyVersion, "cryptoKeyVersions/2")
	assert.False(t, result.RotatedAt.IsZero())
}

func TestGCPKMSProvider_RotateKeyCreateError(t *testing.T) {
	mock := newMockGCPKMSClient()
	mock.CreateCryptoKeyVersionFn = func(
		_ context.Context, _ *kmspb.CreateCryptoKeyVersionRequest,
	) (*kmspb.CryptoKeyVersion, error) {
		return nil, fmt.Errorf("KMS unavailable")
	}
	provider := newGCPKMSProviderWithClient(mock, "test-key")

	_, err := provider.RotateKey(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRotationFailed))
	assert.Contains(t, err.Error(), "GCP CreateCryptoKeyVersion failed")
}

func TestGCPKMSProvider_RotateKeyPromoteError(t *testing.T) {
	mock := newMockGCPKMSClient()
	mock.UpdateCryptoKeyPrimaryVersionFn = func(
		_ context.Context, _ *kmspb.UpdateCryptoKeyPrimaryVersionRequest,
	) (*kmspb.CryptoKey, error) {
		return nil, fmt.Errorf("KMS unavailable")
	}
	provider := newGCPKMSProviderWithClient(mock, "test-key")

	_, err := provider.RotateKey(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRotationFailed))
	assert.Contains(t, err.Error(), "GCP UpdateCryptoKeyPrimaryVersion failed")
}

// --- Vault Transit Provider Tests ---

func TestVaultProvider_EncryptDecryptRoundTrip(t *testing.T) {
	mock := newMockVaultTransitClient()
	provider := newVaultProviderWithClient(mock, "my-transit-key")
	assertEncryptDecryptRoundTrip(t, provider, "my-transit-key", "AES-256-GCM+VAULT-TRANSIT")
}

func TestVaultProvider_EncryptEmptyPlaintext(t *testing.T) {
	mock := newMockVaultTransitClient()
	provider := newVaultProviderWithClient(mock, "my-transit-key")

	ctx := context.Background()
	out, err := provider.Encrypt(ctx, []byte{})
	require.NoError(t, err)

	decrypted, err := provider.Decrypt(ctx, out.Ciphertext)
	require.NoError(t, err)
	assert.Empty(t, decrypted)
}

func TestVaultProvider_InvalidConfig_NoVaultURL(t *testing.T) {
	_, err := newVaultProvider(ProviderConfig{
		ProviderType: ProviderVault,
		KeyID:        "test-key",
		Credentials:  map[string]string{"token": "s.test"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vault URL is required")
}

func TestVaultProvider_InvalidConfig_NoKeyID(t *testing.T) {
	_, err := newVaultProvider(ProviderConfig{
		ProviderType: ProviderVault,
		VaultURL:     "https://vault.example.com:8200",
		Credentials:  map[string]string{"token": "s.test"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key ID is required")
}

func TestVaultProvider_InvalidConfig_NoToken(t *testing.T) {
	_, err := newVaultProvider(ProviderConfig{
		ProviderType: ProviderVault,
		KeyID:        "test-key",
		VaultURL:     "https://vault.example.com:8200",
		Credentials:  map[string]string{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token credential is required")
}

func TestVaultProvider_GenerateDataKeyError(t *testing.T) {
	mock := newMockVaultTransitClient()
	mock.GenerateDataKeyFn = func(_ context.Context, _ string) (*vaultDataKeyResponse, error) {
		return nil, fmt.Errorf("Vault unavailable")
	}
	provider := newVaultProviderWithClient(mock, "test-key")

	_, err := provider.Encrypt(context.Background(), []byte("test"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrEncryptionFailed))
	assert.Contains(t, err.Error(), "Vault GenerateDataKey failed")
}

func TestVaultProvider_DecryptError(t *testing.T) {
	mock := newMockVaultTransitClient()
	provider := newVaultProviderWithClient(mock, "test-key")

	out, err := provider.Encrypt(context.Background(), []byte("test"))
	require.NoError(t, err)

	mock.DecryptDEKFn = func(_ context.Context, _ string, _ string) ([]byte, error) {
		return nil, fmt.Errorf("Vault unavailable")
	}

	_, err = provider.Decrypt(context.Background(), out.Ciphertext)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptionFailed))
	assert.Contains(t, err.Error(), "Vault Decrypt failed")
}

func TestVaultProvider_DecryptInvalidEnvelope(t *testing.T) {
	provider := newVaultProviderWithClient(newMockVaultTransitClient(), "test-key")

	_, err := provider.Decrypt(context.Background(), []byte("not json"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptionFailed))
	assert.Contains(t, err.Error(), "invalid envelope")
}

func TestVaultProvider_DecryptWrongVersion(t *testing.T) {
	provider := newVaultProviderWithClient(newMockVaultTransitClient(), "test-key")

	_, err := provider.Decrypt(context.Background(), []byte(`{"v":99,"wdek":"","nonce":"","ct":""}`))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptionFailed))
	assert.Contains(t, err.Error(), "unsupported envelope version")
}

func TestVaultProvider_DecryptTamperedCiphertext(t *testing.T) {
	mock := newMockVaultTransitClient()
	provider := newVaultProviderWithClient(mock, "test-key")

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

func TestVaultProvider_DecryptInvalidDEKSize(t *testing.T) {
	mock := newMockVaultTransitClient()
	provider := newVaultProviderWithClient(mock, "test-key")

	out, err := provider.Encrypt(context.Background(), []byte("test"))
	require.NoError(t, err)

	// Override DecryptDEK to return invalid key size.
	mock.DecryptDEKFn = func(_ context.Context, _ string, _ string) ([]byte, error) {
		return []byte("bad"), nil
	}

	_, err = provider.Decrypt(context.Background(), out.Ciphertext)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDecryptionFailed))
	assert.Contains(t, err.Error(), "AES cipher creation failed")
}

func TestVaultProvider_GetKeyMetadata(t *testing.T) {
	mock := newMockVaultTransitClient()
	provider := newVaultProviderWithClient(mock, "my-transit-key")

	meta, err := provider.GetKeyMetadata(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "my-transit-key", meta.KeyID)
	assert.Equal(t, "1", meta.KeyVersion)
	assert.Equal(t, "aes256-gcm96", meta.Algorithm)
	assert.True(t, meta.Enabled)
	assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), meta.CreatedAt)
}

func TestVaultProvider_GetKeyMetadataError(t *testing.T) {
	mock := newMockVaultTransitClient()
	mock.ReadKeyFn = func(_ context.Context, _ string) (*vaultKeyInfo, error) {
		return nil, fmt.Errorf("key not found")
	}
	provider := newVaultProviderWithClient(mock, "test-key")

	_, err := provider.GetKeyMetadata(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrKeyNotFound))
}

func TestVaultProvider_Close(t *testing.T) {
	mock := newMockVaultTransitClient()
	provider := newVaultProviderWithClient(mock, "test-key")

	err := provider.Close()
	require.NoError(t, err)
}

func TestVaultProvider_RotateKey(t *testing.T) {
	mock := newMockVaultTransitClient()
	provider := newVaultProviderWithClient(mock, "my-transit-key")

	result, err := provider.RotateKey(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "1", result.PreviousKeyVersion)
	assert.Equal(t, "2", result.NewKeyVersion)
	assert.False(t, result.RotatedAt.IsZero())
}

func TestVaultProvider_RotateKeyError(t *testing.T) {
	mock := newMockVaultTransitClient()
	mock.RotateKeyFn = func(_ context.Context, _ string) (*vaultKeyInfo, error) {
		return nil, fmt.Errorf("Vault unavailable")
	}
	provider := newVaultProviderWithClient(mock, "test-key")

	_, err := provider.RotateKey(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRotationFailed))
	assert.Contains(t, err.Error(), "Vault RotateKey failed")
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
