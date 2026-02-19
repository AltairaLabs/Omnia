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
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
)

// mockKMSClient is a test double for the kmsClient interface.
type mockKMSClient struct {
	GenerateDataKeyFn func(
		ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options),
	) (*kms.GenerateDataKeyOutput, error)

	DecryptFn func(
		ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options),
	) (*kms.DecryptOutput, error)

	DescribeKeyFn func(
		ctx context.Context, params *kms.DescribeKeyInput, optFns ...func(*kms.Options),
	) (*kms.DescribeKeyOutput, error)

	RotateKeyOnDemandFn func(
		ctx context.Context, params *kms.RotateKeyOnDemandInput, optFns ...func(*kms.Options),
	) (*kms.RotateKeyOnDemandOutput, error)
}

func (m *mockKMSClient) GenerateDataKey(
	ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options),
) (*kms.GenerateDataKeyOutput, error) {
	return m.GenerateDataKeyFn(ctx, params, optFns...)
}

func (m *mockKMSClient) Decrypt(
	ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options),
) (*kms.DecryptOutput, error) {
	return m.DecryptFn(ctx, params, optFns...)
}

func (m *mockKMSClient) DescribeKey(
	ctx context.Context, params *kms.DescribeKeyInput, optFns ...func(*kms.Options),
) (*kms.DescribeKeyOutput, error) {
	return m.DescribeKeyFn(ctx, params, optFns...)
}

func (m *mockKMSClient) RotateKeyOnDemand(
	ctx context.Context, params *kms.RotateKeyOnDemandInput, optFns ...func(*kms.Options),
) (*kms.RotateKeyOnDemandOutput, error) {
	return m.RotateKeyOnDemandFn(ctx, params, optFns...)
}

// newMockKMSClient creates a mock AWS KMS client that generates real DEKs
// and "wraps" them by XORing with a fixed key (same pattern as Azure mock).
func newMockKMSClient() *mockKMSClient {
	xorKey := []byte("mock-kms-wrapping-key-32bytes!!!")

	return &mockKMSClient{
		GenerateDataKeyFn: func(
			_ context.Context, params *kms.GenerateDataKeyInput, _ ...func(*kms.Options),
		) (*kms.GenerateDataKeyOutput, error) {
			// Generate a real AES-256 DEK.
			dek := make([]byte, aesKeySize)
			if _, err := io.ReadFull(rand.Reader, dek); err != nil {
				return nil, err
			}

			// "Wrap" via XOR.
			wrapped := make([]byte, len(dek))
			for i, b := range dek {
				wrapped[i] = b ^ xorKey[i%len(xorKey)]
			}

			return &kms.GenerateDataKeyOutput{
				Plaintext:      dek,
				CiphertextBlob: wrapped,
				KeyId:          params.KeyId,
			}, nil
		},
		DecryptFn: func(
			_ context.Context, params *kms.DecryptInput, _ ...func(*kms.Options),
		) (*kms.DecryptOutput, error) {
			// "Unwrap" via XOR.
			unwrapped := make([]byte, len(params.CiphertextBlob))
			for i, b := range params.CiphertextBlob {
				unwrapped[i] = b ^ xorKey[i%len(xorKey)]
			}

			return &kms.DecryptOutput{
				Plaintext: unwrapped,
				KeyId:     params.KeyId,
			}, nil
		},
		DescribeKeyFn: func(
			_ context.Context, params *kms.DescribeKeyInput, _ ...func(*kms.Options),
		) (*kms.DescribeKeyOutput, error) {
			enabled := true
			created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			return &kms.DescribeKeyOutput{
				KeyMetadata: &types.KeyMetadata{
					KeyId:        params.KeyId,
					Enabled:      enabled,
					KeySpec:      types.KeySpecSymmetricDefault,
					CreationDate: &created,
				},
			}, nil
		},
		RotateKeyOnDemandFn: func(
			_ context.Context, params *kms.RotateKeyOnDemandInput, _ ...func(*kms.Options),
		) (*kms.RotateKeyOnDemandOutput, error) {
			return &kms.RotateKeyOnDemandOutput{
				KeyId: params.KeyId,
			}, nil
		},
	}
}
