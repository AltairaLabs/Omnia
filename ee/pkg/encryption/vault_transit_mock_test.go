/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"time"
)

// mockVaultTransitClient is a test double for the vaultTransitClient interface.
type mockVaultTransitClient struct {
	GenerateDataKeyFn func(ctx context.Context, keyName string) (*vaultDataKeyResponse, error)
	DecryptDEKFn      func(ctx context.Context, keyName string, ciphertext string) ([]byte, error)
	ReadKeyFn         func(ctx context.Context, keyName string) (*vaultKeyInfo, error)
	RotateKeyFn       func(ctx context.Context, keyName string) (*vaultKeyInfo, error)
}

func (m *mockVaultTransitClient) GenerateDataKey(ctx context.Context, keyName string) (*vaultDataKeyResponse, error) {
	return m.GenerateDataKeyFn(ctx, keyName)
}

func (m *mockVaultTransitClient) DecryptDEK(ctx context.Context, keyName string, ciphertext string) ([]byte, error) {
	return m.DecryptDEKFn(ctx, keyName, ciphertext)
}

func (m *mockVaultTransitClient) ReadKey(ctx context.Context, keyName string) (*vaultKeyInfo, error) {
	return m.ReadKeyFn(ctx, keyName)
}

func (m *mockVaultTransitClient) RotateKey(ctx context.Context, keyName string) (*vaultKeyInfo, error) {
	return m.RotateKeyFn(ctx, keyName)
}

// newMockVaultTransitClient creates a mock Vault Transit client that generates real DEKs
// and "wraps" them by XORing with a fixed key (same pattern as AWS and GCP mocks).
func newMockVaultTransitClient() *mockVaultTransitClient {
	xorKey := []byte("mock-kms-wrapping-key-32bytes!!!")

	return &mockVaultTransitClient{
		GenerateDataKeyFn: func(_ context.Context, _ string) (*vaultDataKeyResponse, error) {
			// Generate a real AES-256 DEK.
			dek := make([]byte, aesKeySize)
			if _, err := io.ReadFull(rand.Reader, dek); err != nil {
				return nil, err
			}

			// "Wrap" via XOR and encode as vault-style ciphertext.
			wrapped := make([]byte, len(dek))
			for i, b := range dek {
				wrapped[i] = b ^ xorKey[i%len(xorKey)]
			}
			vaultCiphertext := fmt.Sprintf("vault:v1:%s", base64.StdEncoding.EncodeToString(wrapped))

			return &vaultDataKeyResponse{
				Plaintext:  dek,
				Ciphertext: vaultCiphertext,
			}, nil
		},
		DecryptDEKFn: func(_ context.Context, _ string, ciphertext string) ([]byte, error) {
			// Strip "vault:v1:" prefix and decode.
			const prefix = "vault:v1:"
			if len(ciphertext) <= len(prefix) {
				return nil, fmt.Errorf("invalid vault ciphertext format")
			}
			encoded := ciphertext[len(prefix):]
			wrapped, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				return nil, fmt.Errorf("invalid base64 in ciphertext: %w", err)
			}

			// "Unwrap" via XOR.
			unwrapped := make([]byte, len(wrapped))
			for i, b := range wrapped {
				unwrapped[i] = b ^ xorKey[i%len(xorKey)]
			}

			return unwrapped, nil
		},
		ReadKeyFn: func(_ context.Context, keyName string) (*vaultKeyInfo, error) {
			return &vaultKeyInfo{
				Name:            keyName,
				Type:            "aes256-gcm96",
				LatestVersion:   1,
				MinDecryptVer:   1,
				DeletionAllowed: false,
				CreatedAt:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			}, nil
		},
		RotateKeyFn: func(_ context.Context, keyName string) (*vaultKeyInfo, error) {
			return &vaultKeyInfo{
				Name:            keyName,
				Type:            "aes256-gcm96",
				LatestVersion:   2,
				MinDecryptVer:   1,
				DeletionAllowed: false,
				CreatedAt:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			}, nil
		},
	}
}
