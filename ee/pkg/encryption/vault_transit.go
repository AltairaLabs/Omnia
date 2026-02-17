/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	defaultMountPath       = "transit"
	vaultHTTPClientTimeout = 30 * time.Second
	vaultTokenHeader       = "X-Vault-Token"
	vaultAlgorithm         = "AES-256-GCM+VAULT-TRANSIT"
)

// vaultTransitClient abstracts the Vault Transit operations for testability.
type vaultTransitClient interface {
	GenerateDataKey(ctx context.Context, keyName string) (*vaultDataKeyResponse, error)
	DecryptDEK(ctx context.Context, keyName string, ciphertext string) ([]byte, error)
	ReadKey(ctx context.Context, keyName string) (*vaultKeyInfo, error)
}

// vaultDataKeyResponse holds the response from the Vault datakey endpoint.
type vaultDataKeyResponse struct {
	Plaintext  []byte // decoded from base64
	Ciphertext string // vault:v1:... (opaque, stored in envelope)
}

// vaultKeyInfo holds metadata about a Vault Transit key.
type vaultKeyInfo struct {
	Name            string
	Type            string
	LatestVersion   int
	MinDecryptVer   int
	DeletionAllowed bool
	CreatedAt       time.Time
}

// vaultHTTPClient is the real HTTP implementation of vaultTransitClient.
type vaultHTTPClient struct {
	httpClient *http.Client
	addr       string
	token      string
	mountPath  string
}

// vaultAPIResponse is the generic Vault API JSON response wrapper.
type vaultAPIResponse struct {
	Data json.RawMessage `json:"data"`
}

// vaultDataKeyData is the data field from the datakey endpoint.
type vaultDataKeyData struct {
	Plaintext  string `json:"plaintext"`
	Ciphertext string `json:"ciphertext"`
}

// vaultDecryptData is the data field from the decrypt endpoint.
type vaultDecryptData struct {
	Plaintext string `json:"plaintext"`
}

// vaultKeyData is the data field from the keys endpoint.
type vaultKeyData struct {
	Name                 string                       `json:"name"`
	Type                 string                       `json:"type"`
	LatestVersion        int                          `json:"latest_version"`
	MinDecryptionVersion int                          `json:"min_decryption_version"`
	DeletionAllowed      bool                         `json:"deletion_allowed"`
	Keys                 map[string]vaultKeyVersionTS `json:"keys"`
}

// vaultKeyVersionTS captures the creation time from key version metadata.
type vaultKeyVersionTS struct {
	CreationTime string `json:"creation_time"`
}

func (c *vaultHTTPClient) GenerateDataKey(ctx context.Context, keyName string) (*vaultDataKeyResponse, error) {
	url := fmt.Sprintf("%s/v1/%s/datakey/plaintext/%s", c.addr, c.mountPath, keyName)
	body := []byte(`{"bits":256}`)

	respBody, err := c.doRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("vault datakey request failed: %w", err)
	}

	var apiResp vaultAPIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("vault datakey: invalid response JSON: %w", err)
	}

	var data vaultDataKeyData
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return nil, fmt.Errorf("vault datakey: invalid data JSON: %w", err)
	}

	plaintext, err := base64.StdEncoding.DecodeString(data.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("vault datakey: invalid base64 plaintext: %w", err)
	}

	return &vaultDataKeyResponse{
		Plaintext:  plaintext,
		Ciphertext: data.Ciphertext,
	}, nil
}

func (c *vaultHTTPClient) DecryptDEK(ctx context.Context, keyName string, ciphertext string) ([]byte, error) {
	url := fmt.Sprintf("%s/v1/%s/decrypt/%s", c.addr, c.mountPath, keyName)

	reqBody, err := json.Marshal(map[string]string{"ciphertext": ciphertext})
	if err != nil {
		return nil, fmt.Errorf("vault decrypt: failed to marshal request: %w", err)
	}

	respBody, err := c.doRequest(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("vault decrypt request failed: %w", err)
	}

	var apiResp vaultAPIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("vault decrypt: invalid response JSON: %w", err)
	}

	var data vaultDecryptData
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return nil, fmt.Errorf("vault decrypt: invalid data JSON: %w", err)
	}

	plaintext, err := base64.StdEncoding.DecodeString(data.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("vault decrypt: invalid base64 plaintext: %w", err)
	}

	return plaintext, nil
}

func (c *vaultHTTPClient) ReadKey(ctx context.Context, keyName string) (*vaultKeyInfo, error) {
	url := fmt.Sprintf("%s/v1/%s/keys/%s", c.addr, c.mountPath, keyName)

	respBody, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("vault read key request failed: %w", err)
	}

	var apiResp vaultAPIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("vault read key: invalid response JSON: %w", err)
	}

	var data vaultKeyData
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return nil, fmt.Errorf("vault read key: invalid data JSON: %w", err)
	}

	info := &vaultKeyInfo{
		Name:            data.Name,
		Type:            data.Type,
		LatestVersion:   data.LatestVersion,
		MinDecryptVer:   data.MinDecryptionVersion,
		DeletionAllowed: data.DeletionAllowed,
	}

	// Parse creation time from version "1" if available.
	if v1, ok := data.Keys["1"]; ok && v1.CreationTime != "" {
		if t, err := time.Parse(time.RFC3339Nano, v1.CreationTime); err == nil {
			info.CreatedAt = t
		}
	}

	return info, nil
}

func (c *vaultHTTPClient) doRequest(ctx context.Context, method, url string, body []byte) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set(vaultTokenHeader, c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("vault returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// vaultProvider implements the Provider interface using HashiCorp Vault Transit.
type vaultProvider struct {
	client vaultTransitClient
	keyID  string
}

func newVaultProvider(cfg ProviderConfig) (*vaultProvider, error) {
	if cfg.VaultURL == "" {
		return nil, fmt.Errorf("vault: vault URL is required")
	}
	if cfg.KeyID == "" {
		return nil, fmt.Errorf("vault: key ID is required")
	}

	token := cfg.Credentials["token"]
	if token == "" {
		return nil, fmt.Errorf("vault: token credential is required")
	}

	mountPath := cfg.Credentials["mount-path"]
	if mountPath == "" {
		mountPath = defaultMountPath
	}

	client := &vaultHTTPClient{
		httpClient: &http.Client{Timeout: vaultHTTPClientTimeout},
		addr:       cfg.VaultURL,
		token:      token,
		mountPath:  mountPath,
	}

	return &vaultProvider{
		client: client,
		keyID:  cfg.KeyID,
	}, nil
}

func (p *vaultProvider) Encrypt(ctx context.Context, plaintext []byte) (*EncryptOutput, error) {
	// Generate a data encryption key via Vault Transit.
	resp, err := p.client.GenerateDataKey(ctx, p.keyID)
	if err != nil {
		return nil, fmt.Errorf("%w: Vault GenerateDataKey failed: %v", ErrEncryptionFailed, err)
	}

	// Encrypt locally with AES-256-GCM.
	nonce, ciphertext, err := aesGCMEncrypt(resp.Plaintext, plaintext)
	if err != nil {
		return nil, err
	}

	// Store the Vault ciphertext string as the wrapped DEK.
	wrappedDEK := []byte(resp.Ciphertext)

	// Package into envelope.
	envBytes, err := sealEnvelope(wrappedDEK, nonce, ciphertext, "")
	if err != nil {
		return nil, err
	}

	return &EncryptOutput{
		Ciphertext: envBytes,
		KeyID:      p.keyID,
		Algorithm:  vaultAlgorithm,
	}, nil
}

func (p *vaultProvider) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	env, err := parseAndValidateEnvelope(ciphertext)
	if err != nil {
		return nil, err
	}

	// Unwrap the DEK using Vault Transit.
	dek, err := p.client.DecryptDEK(ctx, p.keyID, string(env.WrappedDEK))
	if err != nil {
		return nil, fmt.Errorf("%w: Vault Decrypt failed: %v", ErrDecryptionFailed, err)
	}

	return aesGCMDecrypt(dek, env.Nonce, env.Ciphertext)
}

func (p *vaultProvider) GetKeyMetadata(ctx context.Context) (*KeyMetadata, error) {
	info, err := p.client.ReadKey(ctx, p.keyID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyNotFound, err)
	}

	return &KeyMetadata{
		KeyID:      p.keyID,
		KeyVersion: strconv.Itoa(info.LatestVersion),
		Algorithm:  info.Type,
		CreatedAt:  info.CreatedAt,
		Enabled:    true,
	}, nil
}

func (p *vaultProvider) Close() error {
	return nil
}

// newVaultProviderWithClient creates a provider with an injected client for testing.
func newVaultProviderWithClient(client vaultTransitClient, keyID string) *vaultProvider {
	return &vaultProvider{
		client: client,
		keyID:  keyID,
	}
}
