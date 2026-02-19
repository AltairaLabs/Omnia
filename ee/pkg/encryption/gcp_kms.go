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
	"fmt"
	"io"
	"time"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"google.golang.org/api/option"
)

// gcpKMSClient abstracts the GCP Cloud KMS operations for testability.
type gcpKMSClient interface {
	Encrypt(ctx context.Context, req *kmspb.EncryptRequest) (*kmspb.EncryptResponse, error)
	Decrypt(ctx context.Context, req *kmspb.DecryptRequest) (*kmspb.DecryptResponse, error)
	GetCryptoKey(ctx context.Context, req *kmspb.GetCryptoKeyRequest) (*kmspb.CryptoKey, error)
	CreateCryptoKeyVersion(ctx context.Context, req *kmspb.CreateCryptoKeyVersionRequest) (*kmspb.CryptoKeyVersion, error)
	UpdateCryptoKeyPrimaryVersion(
		ctx context.Context, req *kmspb.UpdateCryptoKeyPrimaryVersionRequest,
	) (*kmspb.CryptoKey, error)
	Close() error
}

// gcpKMSClientWrapper wraps the real KMS client to satisfy the gcpKMSClient interface.
type gcpKMSClientWrapper struct {
	client *kms.KeyManagementClient
}

func (w *gcpKMSClientWrapper) Encrypt(
	ctx context.Context, req *kmspb.EncryptRequest,
) (*kmspb.EncryptResponse, error) {
	return w.client.Encrypt(ctx, req)
}

func (w *gcpKMSClientWrapper) Decrypt(
	ctx context.Context, req *kmspb.DecryptRequest,
) (*kmspb.DecryptResponse, error) {
	return w.client.Decrypt(ctx, req)
}

func (w *gcpKMSClientWrapper) GetCryptoKey(
	ctx context.Context, req *kmspb.GetCryptoKeyRequest,
) (*kmspb.CryptoKey, error) {
	return w.client.GetCryptoKey(ctx, req)
}

func (w *gcpKMSClientWrapper) CreateCryptoKeyVersion(
	ctx context.Context, req *kmspb.CreateCryptoKeyVersionRequest,
) (*kmspb.CryptoKeyVersion, error) {
	return w.client.CreateCryptoKeyVersion(ctx, req)
}

func (w *gcpKMSClientWrapper) UpdateCryptoKeyPrimaryVersion(
	ctx context.Context, req *kmspb.UpdateCryptoKeyPrimaryVersionRequest,
) (*kmspb.CryptoKey, error) {
	return w.client.UpdateCryptoKeyPrimaryVersion(ctx, req)
}

func (w *gcpKMSClientWrapper) Close() error {
	return w.client.Close()
}

type gcpKMSProvider struct {
	client gcpKMSClient
	keyID  string
}

func newGCPKMSProvider(cfg ProviderConfig) (*gcpKMSProvider, error) {
	if cfg.KeyID == "" {
		return nil, fmt.Errorf("gcp-kms: key ID is required")
	}

	var opts []option.ClientOption
	if creds := cfg.Credentials["credentials-json"]; creds != "" {
		//nolint:staticcheck // same pattern as blobstore_gcs.go
		opts = append(opts, option.WithCredentialsJSON([]byte(creds)))
	}

	client, err := kms.NewKeyManagementClient(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("gcp-kms: failed to create client: %w", err)
	}

	return &gcpKMSProvider{
		client: &gcpKMSClientWrapper{client: client},
		keyID:  cfg.KeyID,
	}, nil
}

func (p *gcpKMSProvider) Encrypt(ctx context.Context, plaintext []byte) (*EncryptOutput, error) {
	// Generate a random AES-256 DEK locally.
	dek := make([]byte, aesKeySize)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("%w: failed to generate DEK: %v", ErrEncryptionFailed, err)
	}

	// Wrap the DEK using GCP Cloud KMS.
	wrapResp, err := p.client.Encrypt(ctx, &kmspb.EncryptRequest{
		Name:                        p.keyID,
		Plaintext:                   dek,
		PlaintextCrc32C:             nil,
		AdditionalAuthenticatedData: nil,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: KMS Encrypt (wrap DEK) failed: %v", ErrEncryptionFailed, err)
	}

	// Encrypt locally with AES-256-GCM.
	nonce, ciphertext, err := aesGCMEncrypt(dek, plaintext)
	if err != nil {
		return nil, err
	}

	// Package into envelope.
	envBytes, err := sealEnvelope(wrapResp.Ciphertext, nonce, ciphertext, "")
	if err != nil {
		return nil, err
	}

	return &EncryptOutput{
		Ciphertext: envBytes,
		KeyID:      p.keyID,
		Algorithm:  "AES-256-GCM+GCP-KMS",
	}, nil
}

func (p *gcpKMSProvider) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	env, err := parseAndValidateEnvelope(ciphertext)
	if err != nil {
		return nil, err
	}

	// Unwrap the DEK using GCP Cloud KMS.
	decryptResp, err := p.client.Decrypt(ctx, &kmspb.DecryptRequest{
		Name:       p.keyID,
		Ciphertext: env.WrappedDEK,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: KMS Decrypt failed: %v", ErrDecryptionFailed, err)
	}

	return aesGCMDecrypt(decryptResp.Plaintext, env.Nonce, env.Ciphertext)
}

func (p *gcpKMSProvider) GetKeyMetadata(ctx context.Context) (*KeyMetadata, error) {
	resp, err := p.client.GetCryptoKey(ctx, &kmspb.GetCryptoKeyRequest{
		Name: p.keyID,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyNotFound, err)
	}

	meta := &KeyMetadata{
		KeyID:   p.keyID,
		Enabled: true,
	}

	if resp.Primary != nil {
		meta.Algorithm = resp.Primary.Algorithm.String()
		if resp.Primary.CreateTime != nil {
			meta.CreatedAt = resp.Primary.CreateTime.AsTime()
		}
		if resp.Primary.State == kmspb.CryptoKeyVersion_CRYPTO_KEY_VERSION_STATE_UNSPECIFIED ||
			resp.Primary.State == kmspb.CryptoKeyVersion_DESTROYED ||
			resp.Primary.State == kmspb.CryptoKeyVersion_DESTROY_SCHEDULED {
			meta.Enabled = false
		}
	}

	if resp.DestroyScheduledDuration != nil {
		meta.ExpiresAt = resp.CreateTime.AsTime().Add(resp.DestroyScheduledDuration.AsDuration())
	}

	return meta, nil
}

func (p *gcpKMSProvider) RotateKey(ctx context.Context) (*KeyRotationResult, error) {
	// Get current key version before rotation.
	prevMeta, err := p.GetKeyMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get current key version: %v", ErrRotationFailed, err)
	}

	// Create a new key version.
	newVer, err := p.client.CreateCryptoKeyVersion(ctx, &kmspb.CreateCryptoKeyVersionRequest{
		Parent: p.keyID,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: GCP CreateCryptoKeyVersion failed: %v", ErrRotationFailed, err)
	}

	// Promote the new version to primary.
	_, err = p.client.UpdateCryptoKeyPrimaryVersion(ctx, &kmspb.UpdateCryptoKeyPrimaryVersionRequest{
		Name:               p.keyID,
		CryptoKeyVersionId: newVer.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: GCP UpdateCryptoKeyPrimaryVersion failed: %v", ErrRotationFailed, err)
	}

	return &KeyRotationResult{
		PreviousKeyVersion: prevMeta.KeyVersion,
		NewKeyVersion:      newVer.Name,
		RotatedAt:          time.Now(),
	}, nil
}

func (p *gcpKMSProvider) Close() error {
	return p.client.Close()
}

// newGCPKMSProviderWithClient creates a provider with an injected client for testing.
func newGCPKMSProviderWithClient(client gcpKMSClient, keyID string) *gcpKMSProvider {
	return &gcpKMSProvider{
		client: client,
		keyID:  keyID,
	}
}
