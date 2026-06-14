/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package httpclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/pkg/sessionapi"
)

// captureAuthServer returns an httptest server that records the Authorization
// header of the first request it sees, plus the server itself.
func captureAuthServer(t *testing.T, gotAuth chan<- string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case gotAuth <- r.Header.Get("Authorization"):
		default:
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(sessionapi.SessionResponse{
			Session: testSessionAPI("11111111-1111-1111-1111-111111111111", "agent", "ns"),
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestStore_SendsServiceAccountToken(t *testing.T) {
	const token = "sa-test-token-abc123"
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	require.NoError(t, os.WriteFile(tokenPath, []byte(token+"\n"), 0o600))
	t.Setenv(tokenPathEnv, tokenPath)

	gotAuth := make(chan string, 1)
	srv := captureAuthServer(t, gotAuth)

	store := NewStore(srv.URL, logr.Discard(), WithBufferCapacity(0))
	defer func() { _ = store.Close() }()

	_, err := store.GetSession(t.Context(), "11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)

	assert.Equal(t, "Bearer "+token, <-gotAuth)
}

func TestStore_NoTokenFile_NoAuthHeader(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(tokenPathEnv, filepath.Join(dir, "does-not-exist"))

	gotAuth := make(chan string, 1)
	srv := captureAuthServer(t, gotAuth)

	store := NewStore(srv.URL, logr.Discard(), WithBufferCapacity(0))
	defer func() { _ = store.Close() }()

	_, err := store.GetSession(t.Context(), "11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)

	assert.Empty(t, <-gotAuth)
}
