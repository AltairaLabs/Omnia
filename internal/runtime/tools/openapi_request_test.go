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

package tools

import (
	"net/http"
	"os"
	"testing"

	"github.com/go-logr/logr"
)

// newTestAdapter builds an OpenAPIAdapter with the given config for
// unit-testing setAuth directly, without going through Connect/fetchSpec.
func newTestAdapter(cfg OpenAPIAdapterConfig) *OpenAPIAdapter {
	return NewOpenAPIAdapter(cfg, logr.Discard())
}

func TestSetAuth_Bearer_MissingToken(t *testing.T) {
	a := newTestAdapter(OpenAPIAdapterConfig{AuthType: "bearer"})
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err := a.setAuth(req); err == nil {
		t.Fatal("expected error for bearer auth with no token")
	}
}

func TestSetAuth_Basic_Success(t *testing.T) {
	a := newTestAdapter(OpenAPIAdapterConfig{AuthType: "basic", AuthToken: "user:pass"})
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err := a.setAuth(req); err != nil {
		t.Fatalf("setAuth: %v", err)
	}
	user, pass, ok := req.BasicAuth()
	if !ok || user != "user" || pass != "pass" {
		t.Fatalf("BasicAuth() = (%q, %q, %v), want (user, pass, true)", user, pass, ok)
	}
}

func TestSetAuth_Basic_MissingToken(t *testing.T) {
	a := newTestAdapter(OpenAPIAdapterConfig{AuthType: "basic"})
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err := a.setAuth(req); err == nil {
		t.Fatal("expected error for basic auth with no credentials")
	}
}

func TestSetAuth_Basic_Malformed(t *testing.T) {
	a := newTestAdapter(OpenAPIAdapterConfig{AuthType: "basic", AuthToken: "no-colon"})
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err := a.setAuth(req); err == nil {
		t.Fatal("expected error for malformed basic auth token")
	}
}

func TestSetAuth_UnsupportedType(t *testing.T) {
	a := newTestAdapter(OpenAPIAdapterConfig{AuthType: "hmac"})
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err := a.setAuth(req); err == nil {
		t.Fatal("expected error for unsupported auth type")
	}
}

func TestSetAuth_None(t *testing.T) {
	a := newTestAdapter(OpenAPIAdapterConfig{})
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err := a.setAuth(req); err != nil {
		t.Fatalf("setAuth with no auth type should not error: %v", err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("no Authorization header should be set when AuthType is empty")
	}
}

// TestSetAuth_FileToken_Reread proves the OpenAPI auth path re-reads the token
// file each call, so a rotated projected serviceAccount token is used rather
// than a value cached at startup (#1797).
func TestSetAuth_FileToken_Reread(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/token"
	if err := os.WriteFile(path, []byte("tok-A"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := newTestAdapter(OpenAPIAdapterConfig{AuthType: "bearer", AuthTokenPath: path})

	req, _ := http.NewRequest(http.MethodGet, "http://x", nil)
	if err := a.setAuth(req); err != nil {
		t.Fatalf("setAuth: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer tok-A" {
		t.Fatalf("first: got %q, want Bearer tok-A", got)
	}
	if err := os.WriteFile(path, []byte("tok-B"), 0o600); err != nil {
		t.Fatal(err)
	}
	req2, _ := http.NewRequest(http.MethodGet, "http://x", nil)
	if err := a.setAuth(req2); err != nil {
		t.Fatalf("setAuth: %v", err)
	}
	if got := req2.Header.Get("Authorization"); got != "Bearer tok-B" {
		t.Fatalf("second: got %q, want fresh Bearer tok-B", got)
	}
}

func TestSetAuth_FileToken_MissingFileErrors(t *testing.T) {
	a := newTestAdapter(OpenAPIAdapterConfig{AuthType: "bearer", AuthTokenPath: "/nonexistent/token"})
	req, _ := http.NewRequest(http.MethodGet, "http://x", nil)
	if err := a.setAuth(req); err == nil {
		t.Fatal("expected error when the token file is unreadable")
	}
}
