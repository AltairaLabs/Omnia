/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
)

// kmsClient abstracts the AWS KMS operations for testability.
type kmsClient interface {
	GenerateDataKey(
		ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options),
	) (*kms.GenerateDataKeyOutput, error)
	Decrypt(
		ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options),
	) (*kms.DecryptOutput, error)
	DescribeKey(
		ctx context.Context, params *kms.DescribeKeyInput, optFns ...func(*kms.Options),
	) (*kms.DescribeKeyOutput, error)
}

type awsKMSProvider struct {
	client kmsClient
	keyID  string
}

func newAWSKMSProvider(cfg ProviderConfig) (*awsKMSProvider, error) {
	if cfg.KeyID == "" {
		return nil, fmt.Errorf("aws-kms: key ID is required")
	}

	region := cfg.Credentials["region"]
	if region == "" {
		return nil, fmt.Errorf("aws-kms: region is required")
	}

	opts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}

	accessKeyID := cfg.Credentials["access-key-id"]
	secretAccessKey := cfg.Credentials["secret-access-key"]
	if accessKeyID != "" && secretAccessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("aws-kms: failed to load AWS config: %w", err)
	}

	client := kms.NewFromConfig(awsCfg)
	return &awsKMSProvider{
		client: client,
		keyID:  cfg.KeyID,
	}, nil
}

func (p *awsKMSProvider) Encrypt(ctx context.Context, plaintext []byte) (*EncryptOutput, error) {
	// Generate a data encryption key via AWS KMS.
	genResp, err := p.client.GenerateDataKey(ctx, &kms.GenerateDataKeyInput{
		KeyId:   aws.String(p.keyID),
		KeySpec: types.DataKeySpecAes256,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: KMS GenerateDataKey failed: %v", ErrEncryptionFailed, err)
	}

	// Encrypt locally with AES-256-GCM.
	nonce, ciphertext, err := aesGCMEncrypt(genResp.Plaintext, plaintext)
	if err != nil {
		return nil, err
	}

	// Package into envelope.
	envBytes, err := sealEnvelope(genResp.CiphertextBlob, nonce, ciphertext, "")
	if err != nil {
		return nil, err
	}

	return &EncryptOutput{
		Ciphertext: envBytes,
		KeyID:      p.keyID,
		Algorithm:  "AES-256-GCM+AES-256-KMS",
	}, nil
}

func (p *awsKMSProvider) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	env, err := parseAndValidateEnvelope(ciphertext)
	if err != nil {
		return nil, err
	}

	// Unwrap the DEK using AWS KMS.
	decryptResp, err := p.client.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob: env.WrappedDEK,
		KeyId:          aws.String(p.keyID),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: KMS Decrypt failed: %v", ErrDecryptionFailed, err)
	}

	return aesGCMDecrypt(decryptResp.Plaintext, env.Nonce, env.Ciphertext)
}

func (p *awsKMSProvider) GetKeyMetadata(ctx context.Context) (*KeyMetadata, error) {
	resp, err := p.client.DescribeKey(ctx, &kms.DescribeKeyInput{
		KeyId: aws.String(p.keyID),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyNotFound, err)
	}

	meta := &KeyMetadata{
		KeyID:   p.keyID,
		Enabled: true,
	}

	if resp.KeyMetadata != nil {
		meta.Enabled = resp.KeyMetadata.Enabled
		if resp.KeyMetadata.CreationDate != nil {
			meta.CreatedAt = *resp.KeyMetadata.CreationDate
		}
		if resp.KeyMetadata.ValidTo != nil {
			meta.ExpiresAt = *resp.KeyMetadata.ValidTo
		}
		meta.Algorithm = string(resp.KeyMetadata.KeySpec)
	}

	return meta, nil
}

func (p *awsKMSProvider) Close() error {
	return nil
}

// newAWSKMSProviderWithClient creates a provider with an injected client for testing.
func newAWSKMSProviderWithClient(client kmsClient, keyID string) *awsKMSProvider {
	return &awsKMSProvider{
		client: client,
		keyID:  keyID,
	}
}
