/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestVaultServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *vaultHTTPClient) {
	t.Helper()
	srv := httptest.NewServer(handler)
	client := &vaultHTTPClient{
		httpClient: srv.Client(),
		addr:       srv.URL,
		token:      "s.test-token",
		mountPath:  "transit",
	}
	return srv, client
}

func TestVaultHTTPClient_GenerateDataKey(t *testing.T) {
	plaintext := make([]byte, 32)
	for i := range plaintext {
		plaintext[i] = byte(i)
	}

	srv, client := newTestVaultServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/transit/datakey/plaintext/my-key", r.URL.Path)
		assert.Equal(t, "s.test-token", r.Header.Get("X-Vault-Token"))

		resp := map[string]any{
			"data": map[string]any{
				"plaintext":  base64.StdEncoding.EncodeToString(plaintext),
				"ciphertext": "vault:v1:abc123",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
	defer srv.Close()

	result, err := client.GenerateDataKey(context.Background(), "my-key")
	require.NoError(t, err)
	assert.Equal(t, plaintext, result.Plaintext)
	assert.Equal(t, "vault:v1:abc123", result.Ciphertext)
}

func TestVaultHTTPClient_GenerateDataKey_HTTPError(t *testing.T) {
	srv, client := newTestVaultServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"errors":["permission denied"]}`)) //nolint:errcheck
	})
	defer srv.Close()

	_, err := client.GenerateDataKey(context.Background(), "my-key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 403")
}

func TestVaultHTTPClient_DecryptDEK(t *testing.T) {
	expectedPlaintext := make([]byte, 32)
	for i := range expectedPlaintext {
		expectedPlaintext[i] = byte(i)
	}

	srv, client := newTestVaultServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/transit/decrypt/my-key", r.URL.Path)

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
		assert.Equal(t, "vault:v1:abc123", body["ciphertext"])

		resp := map[string]any{
			"data": map[string]any{
				"plaintext": base64.StdEncoding.EncodeToString(expectedPlaintext),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
	defer srv.Close()

	result, err := client.DecryptDEK(context.Background(), "my-key", "vault:v1:abc123")
	require.NoError(t, err)
	assert.Equal(t, expectedPlaintext, result)
}

func TestVaultHTTPClient_DecryptDEK_HTTPError(t *testing.T) {
	srv, client := newTestVaultServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"errors":["invalid ciphertext"]}`)) //nolint:errcheck
	})
	defer srv.Close()

	_, err := client.DecryptDEK(context.Background(), "my-key", "vault:v1:bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 400")
}

func TestVaultHTTPClient_ReadKey(t *testing.T) {
	srv, client := newTestVaultServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/transit/keys/my-key", r.URL.Path)

		resp := map[string]any{
			"data": map[string]any{
				"name":                   "my-key",
				"type":                   "aes256-gcm96",
				"latest_version":         1,
				"min_decryption_version": 1,
				"deletion_allowed":       false,
				"keys": map[string]any{
					"1": map[string]any{
						"creation_time": "2026-01-01T00:00:00Z",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
	defer srv.Close()

	info, err := client.ReadKey(context.Background(), "my-key")
	require.NoError(t, err)
	assert.Equal(t, "my-key", info.Name)
	assert.Equal(t, "aes256-gcm96", info.Type)
	assert.Equal(t, 1, info.LatestVersion)
	assert.Equal(t, 1, info.MinDecryptVer)
	assert.False(t, info.DeletionAllowed)
	assert.False(t, info.CreatedAt.IsZero())
}

func TestVaultHTTPClient_ReadKey_HTTPError(t *testing.T) {
	srv, client := newTestVaultServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errors":["no key found"]}`)) //nolint:errcheck
	})
	defer srv.Close()

	_, err := client.ReadKey(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestVaultHTTPClient_CustomMountPath(t *testing.T) {
	srv, client := newTestVaultServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/custom-transit/keys/my-key", r.URL.Path)

		resp := map[string]any{
			"data": map[string]any{
				"name":           "my-key",
				"type":           "aes256-gcm96",
				"latest_version": 1,
				"keys":           map[string]any{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
	defer srv.Close()

	client.mountPath = "custom-transit"
	info, err := client.ReadKey(context.Background(), "my-key")
	require.NoError(t, err)
	assert.Equal(t, "my-key", info.Name)
}
