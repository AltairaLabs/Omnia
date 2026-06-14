/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package serviceauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeToken(t *testing.T, path, val string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(val), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
}

func TestTokenSource_ReadsAndCaches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	writeToken(t, path, "tok-1\n")

	now := time.Unix(0, 0)
	ts := NewTokenSource(path, 30*time.Second)
	ts.now = func() time.Time { return now }

	got, err := ts.Token()
	if err != nil || got != "tok-1" {
		t.Fatalf("got %q err=%v, want tok-1", got, err)
	}

	// Change file but stay within TTL -> cached value returned.
	writeToken(t, path, "tok-2")
	now = now.Add(10 * time.Second)
	if got, _ := ts.Token(); got != "tok-1" {
		t.Fatalf("within TTL got %q, want cached tok-1", got)
	}

	// Advance past TTL -> re-reads new value.
	now = now.Add(30 * time.Second)
	if got, _ := ts.Token(); got != "tok-2" {
		t.Fatalf("after TTL got %q, want tok-2", got)
	}
}

func TestTokenSource_MissingFile(t *testing.T) {
	ts := NewTokenSource(filepath.Join(t.TempDir(), "nope"), time.Second)
	got, err := ts.Token()
	if err != nil || got != "" {
		t.Fatalf("got %q err=%v, want empty/nil", got, err)
	}
}

func TestTokenSource_Defaults(t *testing.T) {
	ts := NewTokenSource("", 0)
	if ts.path != DefaultTokenPath {
		t.Fatalf("path = %q, want default", ts.path)
	}
	if ts.ttl != defaultTTL {
		t.Fatalf("ttl = %v, want default", ts.ttl)
	}
}

func TestTokenSource_Authorize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	writeToken(t, path, "abc")
	ts := NewTokenSource(path, time.Second)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := ts.Authorize(req); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer abc" {
		t.Fatalf("header = %q, want Bearer abc", got)
	}
}

func TestTokenSource_AuthorizeMissingFileNoHeader(t *testing.T) {
	ts := NewTokenSource(filepath.Join(t.TempDir(), "nope"), time.Second)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := ts.Authorize(req); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("header = %q, want empty", got)
	}
}

func TestTokenSource_PerRPCCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	writeToken(t, path, "abc")
	ts := NewTokenSource(path, time.Second)
	creds := ts.PerRPCCredentials()

	if creds.RequireTransportSecurity() {
		t.Fatal("RequireTransportSecurity should be false")
	}
	md, err := creds.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("GetRequestMetadata: %v", err)
	}
	if md["authorization"] != "Bearer abc" {
		t.Fatalf("metadata = %v, want Bearer abc", md)
	}
}

func TestTokenSource_PerRPCCredentialsMissingFile(t *testing.T) {
	ts := NewTokenSource(filepath.Join(t.TempDir(), "nope"), time.Second)
	md, err := ts.PerRPCCredentials().GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("GetRequestMetadata: %v", err)
	}
	if md != nil {
		t.Fatalf("metadata = %v, want nil", md)
	}
}
