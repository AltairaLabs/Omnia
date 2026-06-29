/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/altairalabs/omnia/internal/serviceauth"
)

// captureServer is a test helper that captures all POST requests it receives.
type captureServer struct {
	mu       sync.Mutex
	requests []*http.Request
	bodies   [][]byte
	status   int // HTTP status to return; 0 → 200
}

func newCaptureServer(status int) *captureServer {
	if status == 0 {
		status = http.StatusOK
	}
	return &captureServer{status: status}
}

func (s *captureServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.requests = append(s.requests, r)
		s.bodies = append(s.bodies, body)
		s.mu.Unlock()
		w.WriteHeader(s.status)
	})
}

func (s *captureServer) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func (s *captureServer) bodyAt(i int) consentEventBody {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b consentEventBody
	_ = json.Unmarshal(s.bodies[i], &b)
	return b
}

func (s *captureServer) authHeaderAt(i int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests[i].Header.Get("Authorization")
}

func (s *captureServer) workspaceParamAt(i int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests[i].URL.Query().Get("workspace")
}

func newTestNotifier(urls []string, workspace string) *MemoryAPINotifier {
	return NewMemoryAPINotifier(urls, workspace, nil, zap.New(zap.UseDevMode(true)))
}

// TestConsentNotifier_FanOutToMultipleURLs asserts that NotifyRevocation POSTs
// to every configured memory-api URL with the correct JSON body.
func TestConsentNotifier_FanOutToMultipleURLs(t *testing.T) {
	srv1 := newCaptureServer(http.StatusOK)
	srv2 := newCaptureServer(http.StatusOK)
	ts1 := httptest.NewServer(srv1.handler())
	ts2 := httptest.NewServer(srv2.handler())
	defer ts1.Close()
	defer ts2.Close()

	n := newTestNotifier([]string{ts1.URL, ts2.URL}, "test-workspace")
	err := n.NotifyRevocation(t.Context(), "user-123", ConsentMemoryIdentity)

	require.NoError(t, err)
	assert.Equal(t, 1, srv1.callCount(), "server 1 should receive exactly one POST")
	assert.Equal(t, 1, srv2.callCount(), "server 2 should receive exactly one POST")

	for _, srv := range []*captureServer{srv1, srv2} {
		body := srv.bodyAt(0)
		assert.Equal(t, "user-123", body.UserID)
		assert.Equal(t, string(ConsentMemoryIdentity), body.Category)
	}
}

// TestConsentNotifier_PartialFailure asserts that a 500 from one target does
// not abort the fan-out to the other target, and that nil is returned.
func TestConsentNotifier_PartialFailure(t *testing.T) {
	srvOK := newCaptureServer(http.StatusOK)
	srvFail := newCaptureServer(http.StatusInternalServerError)
	tsOK := httptest.NewServer(srvOK.handler())
	tsFail := httptest.NewServer(srvFail.handler())
	defer tsOK.Close()
	defer tsFail.Close()

	n := newTestNotifier([]string{tsOK.URL, tsFail.URL}, "test-workspace")
	err := n.NotifyRevocation(t.Context(), "user-abc", ConsentMemoryHealth)

	// Best-effort: error is logged per target but nil is returned to caller.
	assert.NoError(t, err, "partial failure must not surface as an error")
	assert.Equal(t, 1, srvOK.callCount(), "healthy server must still be called")
	assert.Equal(t, 1, srvFail.callCount(), "failing server must be called too")
}

// TestConsentNotifier_EmptyURLList asserts that an empty URL list is a no-op.
func TestConsentNotifier_EmptyURLList(t *testing.T) {
	n := newTestNotifier(nil, "test-workspace")
	err := n.NotifyRevocation(t.Context(), "user-xyz", ConsentMemoryLocation)
	assert.NoError(t, err)
}

// TestConsentNotifier_SATokenAttached asserts that the Authorization header is
// set on every request when a TokenSource is provided.
func TestConsentNotifier_SATokenAttached(t *testing.T) {
	srv := newCaptureServer(http.StatusOK)
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	// Use a real TokenSource backed by a temp file so we control the token value.
	tokenFile := t.TempDir() + "/token"
	require.NoError(t, writeFile(tokenFile, "test-sa-token"))
	tokenSrc := serviceauth.NewTokenSource(tokenFile, 0)

	n := NewMemoryAPINotifier([]string{ts.URL}, "test-workspace", tokenSrc, zap.New(zap.UseDevMode(true)))
	require.NoError(t, n.NotifyRevocation(t.Context(), "user-tok", ConsentMemoryPreferences))

	assert.Equal(t, "Bearer test-sa-token", srv.authHeaderAt(0))
}

// TestConsentNotifier_EnvOverride asserts that the MEMORY_API_URLS env var
// overrides the URLs passed to the constructor.
func TestConsentNotifier_EnvOverride(t *testing.T) {
	srvEnv := newCaptureServer(http.StatusOK)
	tsEnv := httptest.NewServer(srvEnv.handler())
	defer tsEnv.Close()

	// The env var target wins; the constructor-supplied URL is ignored.
	t.Setenv("MEMORY_API_URLS", tsEnv.URL)

	srvIgnored := newCaptureServer(http.StatusOK)
	tsIgnored := httptest.NewServer(srvIgnored.handler())
	defer tsIgnored.Close()

	n := newTestNotifier([]string{tsIgnored.URL}, "test-workspace")
	require.NoError(t, n.NotifyRevocation(t.Context(), "user-env", ConsentMemoryContext))

	assert.Equal(t, 1, srvEnv.callCount(), "env-var server must be called")
	assert.Equal(t, 0, srvIgnored.callCount(), "constructor URL must be ignored when env var is set")
}

// TestNoopConsentNotifier_AlwaysNil confirms the no-op notifier never errors.
func TestNoopConsentNotifier_AlwaysNil(t *testing.T) {
	var n ConsentNotifier = NoopConsentNotifier{}
	assert.NoError(t, n.NotifyRevocation(t.Context(), "u", ConsentMemoryIdentity))
}

// TestConsentNotifier_WorkspaceQueryParam asserts that every POST carries a
// non-empty ?workspace= query parameter. The memory-api consent-events endpoint
// calls parseWorkspaceScope which returns 400 ErrMissingWorkspace when it is absent.
func TestConsentNotifier_WorkspaceQueryParam(t *testing.T) {
	srv := newCaptureServer(http.StatusOK)
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	n := newTestNotifier([]string{ts.URL}, "my-workspace")
	require.NoError(t, n.NotifyRevocation(t.Context(), "user-ws", ConsentMemoryIdentity))

	require.Equal(t, 1, srv.callCount(), "server must receive exactly one POST")
	assert.Equal(t, "my-workspace", srv.workspaceParamAt(0),
		"?workspace query param must match the workspace passed to NewMemoryAPINotifier")
}

// TestConsentNotifier_EmptyWorkspaceOmitsParam asserts that when workspace is
// empty the URL is sent without a ?workspace= query parameter (edge case: env
// override with no workspace configured).
func TestConsentNotifier_EmptyWorkspaceOmitsParam(t *testing.T) {
	srv := newCaptureServer(http.StatusOK)
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	n := newTestNotifier([]string{ts.URL}, "")
	require.NoError(t, n.NotifyRevocation(t.Context(), "user-no-ws", ConsentMemoryIdentity))

	require.Equal(t, 1, srv.callCount())
	assert.Equal(t, "", srv.workspaceParamAt(0), "empty workspace must not set ?workspace= param")
}

// writeFile is a small helper to write a string to a file.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0600)
}
