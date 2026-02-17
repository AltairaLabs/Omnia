/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

import (
	"context"
	"time"

	"cloud.google.com/go/kms/apiv1/kmspb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// mockGCPKMSClient is a test double for the gcpKMSClient interface.
type mockGCPKMSClient struct {
	EncryptFn      func(ctx context.Context, req *kmspb.EncryptRequest) (*kmspb.EncryptResponse, error)
	DecryptFn      func(ctx context.Context, req *kmspb.DecryptRequest) (*kmspb.DecryptResponse, error)
	GetCryptoKeyFn func(ctx context.Context, req *kmspb.GetCryptoKeyRequest) (*kmspb.CryptoKey, error)
}

func (m *mockGCPKMSClient) Encrypt(ctx context.Context, req *kmspb.EncryptRequest) (*kmspb.EncryptResponse, error) {
	return m.EncryptFn(ctx, req)
}

func (m *mockGCPKMSClient) Decrypt(ctx context.Context, req *kmspb.DecryptRequest) (*kmspb.DecryptResponse, error) {
	return m.DecryptFn(ctx, req)
}

func (m *mockGCPKMSClient) GetCryptoKey(ctx context.Context, req *kmspb.GetCryptoKeyRequest) (*kmspb.CryptoKey, error) {
	return m.GetCryptoKeyFn(ctx, req)
}

func (m *mockGCPKMSClient) Close() error {
	return nil
}

// newMockGCPKMSClient creates a mock GCP KMS client that wraps/unwraps DEKs
// by XORing with a fixed key (same pattern as AWS and Azure mocks).
func newMockGCPKMSClient() *mockGCPKMSClient {
	xorKey := []byte("mock-kms-wrapping-key-32bytes!!!")

	return &mockGCPKMSClient{
		EncryptFn: func(_ context.Context, req *kmspb.EncryptRequest) (*kmspb.EncryptResponse, error) {
			wrapped := make([]byte, len(req.Plaintext))
			for i, b := range req.Plaintext {
				wrapped[i] = b ^ xorKey[i%len(xorKey)]
			}
			return &kmspb.EncryptResponse{
				Ciphertext: wrapped,
				Name:       req.Name,
			}, nil
		},
		DecryptFn: func(_ context.Context, req *kmspb.DecryptRequest) (*kmspb.DecryptResponse, error) {
			unwrapped := make([]byte, len(req.Ciphertext))
			for i, b := range req.Ciphertext {
				unwrapped[i] = b ^ xorKey[i%len(xorKey)]
			}
			return &kmspb.DecryptResponse{
				Plaintext: unwrapped,
			}, nil
		},
		GetCryptoKeyFn: func(_ context.Context, req *kmspb.GetCryptoKeyRequest) (*kmspb.CryptoKey, error) {
			created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			return &kmspb.CryptoKey{
				Name:       req.Name,
				CreateTime: timestamppb.New(created),
				Primary: &kmspb.CryptoKeyVersion{
					Algorithm:  kmspb.CryptoKeyVersion_GOOGLE_SYMMETRIC_ENCRYPTION,
					State:      kmspb.CryptoKeyVersion_ENABLED,
					CreateTime: timestamppb.New(created),
				},
				DestroyScheduledDuration: durationpb.New(24 * time.Hour),
			}, nil
		},
	}
}
