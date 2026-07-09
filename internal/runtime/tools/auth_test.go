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
	"os"
	"strings"
	"testing"
)

func TestMergeAuthHeaders_Bearer(t *testing.T) {
	headers := make(map[string]string)
	if err := mergeAuthHeaders(headers, "bearer", "my-token"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if headers["Authorization"] != "Bearer my-token" {
		t.Errorf("expected Bearer my-token, got %s", headers["Authorization"])
	}
}

func TestMergeAuthHeaders_BearerNoToken(t *testing.T) {
	headers := make(map[string]string)
	err := mergeAuthHeaders(headers, "bearer", "")
	if err == nil {
		t.Fatal("expected error for empty bearer token")
	}
	if !strings.Contains(err.Error(), "requires a token") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMergeAuthHeaders_Basic(t *testing.T) {
	headers := make(map[string]string)
	if err := mergeAuthHeaders(headers, "basic", "user:pass"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	auth := headers["Authorization"]
	if !strings.HasPrefix(auth, "Basic ") {
		t.Errorf("expected Basic auth header, got %s", auth)
	}
}

func TestMergeAuthHeaders_BasicNoToken(t *testing.T) {
	headers := make(map[string]string)
	err := mergeAuthHeaders(headers, "basic", "")
	if err == nil {
		t.Fatal("expected error for empty basic token")
	}
	if !strings.Contains(err.Error(), "requires credentials") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMergeAuthHeaders_BasicInvalidFormat(t *testing.T) {
	headers := make(map[string]string)
	err := mergeAuthHeaders(headers, "basic", "no-colon")
	if err == nil {
		t.Fatal("expected error for invalid basic format")
	}
	if !strings.Contains(err.Error(), "username:password") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMergeAuthHeaders_Empty(t *testing.T) {
	headers := make(map[string]string)
	if err := mergeAuthHeaders(headers, "", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := headers["Authorization"]; ok {
		t.Error("expected no Authorization header for empty auth type")
	}
}

func TestMergeAuthHeaders_Unsupported(t *testing.T) {
	headers := make(map[string]string)
	err := mergeAuthHeaders(headers, "oauth2", "token")
	if err == nil {
		t.Fatal("expected error for unsupported auth type")
	}
	if !strings.Contains(err.Error(), "unsupported auth type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMergeAuthHeaders_CaseInsensitive(t *testing.T) {
	headers := make(map[string]string)
	if err := mergeAuthHeaders(headers, "BEARER", "tok"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if headers["Authorization"] != "Bearer tok" {
		t.Errorf("expected Bearer tok, got %s", headers["Authorization"])
	}
}

func TestFreshAuthToken_RereadsFileEachCall(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/token"
	if err := os.WriteFile(path, []byte("token-A\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// First read picks up token-A (trailing newline trimmed).
	got, err := freshAuthToken("bearer", "", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "token-A" {
		t.Fatalf("first read: expected token-A, got %q", got)
	}
	// Simulate kubelet rotating the projected token before expiry.
	if err := os.WriteFile(path, []byte("token-B"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = freshAuthToken("bearer", "", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "token-B" {
		t.Fatalf("second read: expected fresh token-B, got %q (stale)", got)
	}
}

func TestFreshAuthToken_NoPathReturnsStaticToken(t *testing.T) {
	got, err := freshAuthToken("bearer", "static", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "static" {
		t.Fatalf("expected static, got %q", got)
	}
}

func TestFreshAuthToken_WorkloadIdentitySkipsFile(t *testing.T) {
	// workloadIdentity resolves its own token elsewhere; even with a path set,
	// freshAuthToken must not read a file.
	got, err := freshAuthToken(authTypeWorkloadIdentity, "", "/nonexistent/token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty passthrough, got %q", got)
	}
}

func TestFreshAuthToken_UnreadablePathErrors(t *testing.T) {
	if _, err := freshAuthToken("bearer", "", "/nonexistent/token"); err == nil {
		t.Fatal("expected error reading a missing token file")
	}
}
