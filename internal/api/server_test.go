/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func newTestServer(_ *testing.T, artifactDir string) *Server {
	return NewServer(zap.New(zap.UseDevMode(true)), artifactDir)
}

func TestHealthz(t *testing.T) {
	server := newTestServer(t, "")
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String())
}

func TestArtifactServing(t *testing.T) {
	// Create a temp directory with a test artifact
	tmpDir := t.TempDir()
	testContent := "test artifact content"
	err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(testContent), 0644)
	require.NoError(t, err)

	server := newTestServer(t, tmpDir)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/artifacts/test.txt", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, testContent, rec.Body.String())
}

func TestArtifactServingCORS(t *testing.T) {
	tmpDir := t.TempDir()
	err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("content"), 0644)
	require.NoError(t, err)

	server := newTestServer(t, tmpDir)
	handler := server.Handler()

	// Test OPTIONS request for CORS preflight
	req := httptest.NewRequest("OPTIONS", "/artifacts/test.txt", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, OPTIONS", rec.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type", rec.Header().Get("Access-Control-Allow-Headers"))
}

func TestArtifactServingCORSOnGet(t *testing.T) {
	tmpDir := t.TempDir()
	err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("content"), 0644)
	require.NoError(t, err)

	server := newTestServer(t, tmpDir)
	handler := server.Handler()

	// Test GET request includes CORS headers
	req := httptest.NewRequest("GET", "/artifacts/test.txt", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestArtifactNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	server := newTestServer(t, tmpDir)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/artifacts/nonexistent.txt", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestNoArtifactDir(t *testing.T) {
	// When artifactDir is empty, artifacts endpoint should not be registered
	server := newTestServer(t, "")
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/artifacts/test.txt", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should 404 because the handler isn't registered
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestServerRun(t *testing.T) {
	server := newTestServer(t, "")

	ctx, cancel := context.WithCancel(context.Background())

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx, ":0") // Use port 0 for random available port
	}()

	// Cancel context to trigger shutdown
	cancel()

	// Server should shut down cleanly
	err := <-errCh
	assert.NoError(t, err)
}

func TestNestedArtifactServing(t *testing.T) {
	// Create a temp directory with nested artifacts
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "job-123", "output")
	err := os.MkdirAll(nestedDir, 0755)
	require.NoError(t, err)

	testContent := "nested artifact content"
	err = os.WriteFile(filepath.Join(nestedDir, "result.json"), []byte(testContent), 0644)
	require.NoError(t, err)

	server := newTestServer(t, tmpDir)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/artifacts/job-123/output/result.json", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, testContent, rec.Body.String())
}
