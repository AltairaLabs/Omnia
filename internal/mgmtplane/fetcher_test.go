/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package mgmtplane

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// writeSAToken makes a temp file holding a fake SA token and returns its path.
func writeSAToken(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestNewTokenFetcher_RequiresEndpoint(t *testing.T) {
	if _, err := NewTokenFetcher(FetcherOptions{}); err == nil {
		t.Fatal("expected error for empty Endpoint")
	}
}

func TestNewTokenFetcher_RejectsInsecureHTTPByDefault(t *testing.T) {
	if _, err := NewTokenFetcher(FetcherOptions{Endpoint: "http://dashboard.example/api/auth/service-token"}); err == nil {
		t.Fatal("expected error for insecure http endpoint")
	}
}

func TestNewTokenFetcher_AllowsInsecureHTTPWhenExplicitlyEnabled(t *testing.T) {
	f, err := NewTokenFetcher(FetcherOptions{
		Endpoint:          "http://dashboard.example/api/auth/service-token",
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("expected constructor to allow insecure endpoint when explicitly enabled, got: %v", err)
	}
	if f == nil {
		t.Fatal("expected fetcher")
	}
}

func TestNewTokenFetcher_AllowsClusterInternalHTTP(t *testing.T) {
	// The dashboard is served over http on a ClusterIP Service in every Omnia
	// install, so the constructor must accept the in-cluster http URL WITHOUT
	// AllowInsecureHTTP — otherwise fleet auth is broken on every deployment.
	f, err := NewTokenFetcher(FetcherOptions{
		Endpoint: "http://omnia-dashboard.omnia.svc.cluster.local:3000/api/auth/service-token",
	})
	if err != nil {
		t.Fatalf("expected cluster-internal http endpoint to be allowed without AllowInsecureHTTP, got: %v", err)
	}
	if f == nil {
		t.Fatal("expected fetcher")
	}
}

func TestToken_RoundtripsWithDashboard(t *testing.T) {
	saTokenPath := writeSAToken(t, "fake-sa-token")
	var seenAuth, seenBody string
	var seenAgent, seenWS string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		seenBody = string(body)
		var b struct{ Agent, Workspace string }
		_ = json.Unmarshal(body, &b)
		seenAgent = b.Agent
		seenWS = b.Workspace
		w.Header().Set("Content-Type", "application/json")
		exp := time.Now().Add(5 * time.Minute).Unix()
		_, _ = fmt.Fprintf(w, `{"token":"jwt-from-dashboard","expires_at":%d}`, exp)
	}))
	defer server.Close()

	f, err := NewTokenFetcher(FetcherOptions{
		Endpoint:                server.URL,
		ServiceAccountTokenPath: saTokenPath,
		AllowInsecureHTTP:       true,
	})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	tok, err := f.Token("agent-x", "ws-y")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if tok != "jwt-from-dashboard" {
		t.Errorf("token: got %q", tok)
	}
	if seenAuth != "Bearer fake-sa-token" {
		t.Errorf("Authorization header: got %q", seenAuth)
	}
	if seenAgent != "agent-x" || seenWS != "ws-y" {
		t.Errorf("body: got agent=%q ws=%q (raw=%s)", seenAgent, seenWS, seenBody)
	}
}

func TestToken_TrimsSATokenWhitespace(t *testing.T) {
	saTokenPath := writeSAToken(t, "fake-sa-token\n")
	var seenAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"t","expires_at":99999}`))
	}))
	defer server.Close()
	f, _ := NewTokenFetcher(FetcherOptions{
		Endpoint:                server.URL,
		ServiceAccountTokenPath: saTokenPath,
		AllowInsecureHTTP:       true,
	})
	if _, err := f.Token("a", "w"); err != nil {
		t.Fatalf("token: %v", err)
	}
	if strings.Contains(seenAuth, "\n") {
		t.Errorf("Authorization carried newline: %q", seenAuth)
	}
	if seenAuth != "Bearer fake-sa-token" {
		t.Errorf("Authorization: got %q", seenAuth)
	}
}

func TestToken_CachedWhenSameAgentWorkspace(t *testing.T) {
	saTokenPath := writeSAToken(t, "tok")
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		exp := time.Now().Add(5 * time.Minute).Unix()
		_, _ = fmt.Fprintf(w, `{"token":"t-%d","expires_at":%d}`, calls, exp)
	}))
	defer server.Close()
	f, _ := NewTokenFetcher(FetcherOptions{
		Endpoint:                server.URL,
		ServiceAccountTokenPath: saTokenPath,
		AllowInsecureHTTP:       true,
	})
	t1, _ := f.Token("a", "w")
	t2, _ := f.Token("a", "w")
	if t1 != t2 {
		t.Errorf("expected cached reuse, got %q vs %q", t1, t2)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("expected 1 dashboard call, got %d", calls)
	}
}

func TestToken_FreshWhenAgentChanges(t *testing.T) {
	saTokenPath := writeSAToken(t, "tok")
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		exp := time.Now().Add(5 * time.Minute).Unix()
		_, _ = fmt.Fprintf(w, `{"token":"t-%d","expires_at":%d}`, n, exp)
	}))
	defer server.Close()
	f, _ := NewTokenFetcher(FetcherOptions{
		Endpoint:                server.URL,
		ServiceAccountTokenPath: saTokenPath,
		AllowInsecureHTTP:       true,
	})
	t1, _ := f.Token("a", "w")
	t2, _ := f.Token("b", "w")
	if t1 == t2 {
		t.Fatal("expected fresh token when agent changes")
	}
}

func TestToken_FreshWhenCacheExpired(t *testing.T) {
	saTokenPath := writeSAToken(t, "tok")
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		// 60s TTL — short enough to expire by simulated-now+90s.
		exp := time.Now().Add(60 * time.Second).Unix()
		_, _ = fmt.Fprintf(w, `{"token":"t-%d","expires_at":%d}`, n, exp)
	}))
	defer server.Close()
	f, _ := NewTokenFetcher(FetcherOptions{
		Endpoint:                server.URL,
		ServiceAccountTokenPath: saTokenPath,
		AllowInsecureHTTP:       true,
	})
	now := time.Now()
	f.now = func() time.Time { return now }
	t1, _ := f.Token("a", "w")
	// Advance past expiry minus safety margin.
	f.now = func() time.Time { return now.Add(2 * time.Minute) }
	t2, _ := f.Token("a", "w")
	if t1 == t2 {
		t.Fatal("expected fresh token after cache expiry")
	}
}

func TestToken_SurfacesDashboardError(t *testing.T) {
	saTokenPath := writeSAToken(t, "tok")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"sa not in allowlist"}`))
	}))
	defer server.Close()
	f, _ := NewTokenFetcher(FetcherOptions{
		Endpoint:                server.URL,
		ServiceAccountTokenPath: saTokenPath,
		AllowInsecureHTTP:       true,
	})
	_, err := f.Token("a", "w")
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "sa not in allowlist") {
		t.Errorf("expected error to embed status + body, got: %v", err)
	}
}

func TestToken_RejectsEmptyTokenInResponse(t *testing.T) {
	saTokenPath := writeSAToken(t, "tok")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"","expires_at":1234}`))
	}))
	defer server.Close()
	f, _ := NewTokenFetcher(FetcherOptions{
		Endpoint:                server.URL,
		ServiceAccountTokenPath: saTokenPath,
		AllowInsecureHTTP:       true,
	})
	if _, err := f.Token("a", "w"); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestToken_NilReceiver(t *testing.T) {
	var f *TokenFetcher
	if _, err := f.Token("a", "w"); err == nil {
		t.Fatal("expected error on nil receiver")
	}
}

func TestToken_MissingSATokenFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"t","expires_at":99999}`))
	}))
	defer server.Close()
	f, _ := NewTokenFetcher(FetcherOptions{
		Endpoint:                server.URL,
		ServiceAccountTokenPath: "/tmp/this-path-does-not-exist-for-sure",
		AllowInsecureHTTP:       true,
	})
	if _, err := f.Token("a", "w"); err == nil {
		t.Fatal("expected error when SA token file missing")
	}
}
