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
	"encoding/json"
	"fmt"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
)

// azkeysClient abstracts the Azure Key Vault key operations for testability.
type azkeysClient interface {
	WrapKey(
		ctx context.Context, keyName string, keyVersion string,
		parameters azkeys.KeyOperationParameters, options *azkeys.WrapKeyOptions,
	) (azkeys.WrapKeyResponse, error)
	UnwrapKey(
		ctx context.Context, keyName string, keyVersion string,
		parameters azkeys.KeyOperationParameters, options *azkeys.UnwrapKeyOptions,
	) (azkeys.UnwrapKeyResponse, error)
	GetKey(
		ctx context.Context, keyName string, keyVersion string,
		options *azkeys.GetKeyOptions,
	) (azkeys.GetKeyResponse, error)
}

const (
	aesKeySize      = 32 // AES-256
	envelopeVersion = 1
	wrapAlgorithm   = azkeys.EncryptionAlgorithmRSAOAEP256
)

// envelope is the JSON structure stored as ciphertext for envelope encryption.
type envelope struct {
	Version    int    `json:"v"`
	WrappedDEK []byte `json:"wdek"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ct"`
	KeyVersion string `json:"kv,omitempty"`
}

type azureKeyVaultProvider struct {
	client     azkeysClient
	keyName    string
	keyVersion string
	vaultURL   string
}

func newAzureKeyVaultProvider(cfg ProviderConfig) (*azureKeyVaultProvider, error) {
	if cfg.VaultURL == "" {
		return nil, fmt.Errorf("azure-keyvault: vault URL is required")
	}
	if cfg.KeyID == "" {
		return nil, fmt.Errorf("azure-keyvault: key ID is required")
	}

	cred, err := azureCredentialFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("azure-keyvault: credential error: %w", err)
	}

	client, err := azkeys.NewClient(cfg.VaultURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("azure-keyvault: client creation error: %w", err)
	}

	return &azureKeyVaultProvider{
		client:   client,
		keyName:  cfg.KeyID,
		vaultURL: cfg.VaultURL,
	}, nil
}

func azureCredentialFromConfig(cfg ProviderConfig) (azcore.TokenCredential, error) {
	tenantID := cfg.Credentials["tenant-id"]
	clientID := cfg.Credentials["client-id"]
	clientSecret := cfg.Credentials["client-secret"]

	if tenantID != "" && clientID != "" && clientSecret != "" {
		return azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
	}

	// Fallback to DefaultAzureCredential (workload identity, managed identity, etc.)
	return azidentity.NewDefaultAzureCredential(nil)
}

func (p *azureKeyVaultProvider) Encrypt(ctx context.Context, plaintext []byte) (*EncryptOutput, error) {
	// Generate a random AES-256 DEK.
	dek := make([]byte, aesKeySize)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("%w: failed to generate DEK: %v", ErrEncryptionFailed, err)
	}

	// Wrap the DEK using Azure Key Vault.
	algo := wrapAlgorithm
	wrapResp, err := p.client.WrapKey(ctx, p.keyName, p.keyVersion, azkeys.KeyOperationParameters{
		Algorithm: &algo,
		Value:     dek,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: KMS wrap key failed: %v", ErrEncryptionFailed, err)
	}

	// Encrypt locally with AES-256-GCM.
	nonce, ciphertext, err := aesGCMEncrypt(dek, plaintext)
	if err != nil {
		return nil, err
	}

	// Determine key version from response.
	keyVersion := p.keyVersion
	if wrapResp.KID != nil {
		if kid := wrapResp.KID; kid != nil {
			keyVersion = kid.Version()
		}
	}

	// Package into envelope.
	envBytes, err := sealEnvelope(wrapResp.Result, nonce, ciphertext, keyVersion)
	if err != nil {
		return nil, err
	}

	return &EncryptOutput{
		Ciphertext: envBytes,
		KeyID:      p.keyName,
		KeyVersion: keyVersion,
		Algorithm:  "AES-256-GCM+RSA-OAEP-256",
	}, nil
}

func (p *azureKeyVaultProvider) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	env, err := parseAndValidateEnvelope(ciphertext)
	if err != nil {
		return nil, err
	}

	// Unwrap the DEK using Azure Key Vault.
	algo := wrapAlgorithm
	unwrapResp, err := p.client.UnwrapKey(ctx, p.keyName, env.KeyVersion, azkeys.KeyOperationParameters{
		Algorithm: &algo,
		Value:     env.WrappedDEK,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: KMS unwrap key failed: %v", ErrDecryptionFailed, err)
	}

	return aesGCMDecrypt(unwrapResp.Result, env.Nonce, env.Ciphertext)
}

func (p *azureKeyVaultProvider) GetKeyMetadata(ctx context.Context) (*KeyMetadata, error) {
	resp, err := p.client.GetKey(ctx, p.keyName, p.keyVersion, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyNotFound, err)
	}

	meta := &KeyMetadata{
		KeyID:   p.keyName,
		Enabled: true,
	}

	if resp.Key != nil && resp.Key.KID != nil {
		meta.KeyVersion = resp.Key.KID.Version()
	}

	if resp.Attributes != nil {
		if resp.Attributes.Created != nil {
			meta.CreatedAt = *resp.Attributes.Created
		}
		if resp.Attributes.Expires != nil {
			meta.ExpiresAt = *resp.Attributes.Expires
		}
		if resp.Attributes.Enabled != nil {
			meta.Enabled = *resp.Attributes.Enabled
		}
	}

	// Derive algorithm from key type.
	if resp.Key != nil && resp.Key.Kty != nil {
		meta.Algorithm = string(*resp.Key.Kty)
	}

	return meta, nil
}

func (p *azureKeyVaultProvider) Close() error {
	return nil
}

// newAzureKeyVaultProviderWithClient creates a provider with an injected client for testing.
//
//nolint:unparam // keyVersion is always "" in tests but kept for API completeness
func newAzureKeyVaultProviderWithClient(client azkeysClient, keyName, keyVersion string) *azureKeyVaultProvider {
	return &azureKeyVaultProvider{
		client:     client,
		keyName:    keyName,
		keyVersion: keyVersion,
	}
}

// envelopeFromBytes parses an envelope from ciphertext bytes.
func envelopeFromBytes(data []byte) (*envelope, error) {
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	return &env, nil
}
