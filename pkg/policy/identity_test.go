/*
Copyright 2026.

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

package policy

import (
	"strings"
	"testing"
)

// These constants are a downstream contract — ToolPolicy rules compare
// against them and session-logging consumers persist them. Pin the
// on-the-wire values so typo-level changes show up as test failures.

func TestOriginConstants(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		OriginManagementPlane: "management-plane",
		OriginSharedToken:     "shared-token",
		OriginAPIKey:          "api-key",
		OriginOIDC:            "oidc",
		OriginEdgeTrust:       "edge-trust",
	}
	for got, expected := range want {
		if got != expected {
			t.Errorf("origin constant = %q, want %q", got, expected)
		}
	}
}

func TestRoleConstants(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		RoleAdmin:  "admin",
		RoleEditor: "editor",
		RoleViewer: "viewer",
	}
	for got, expected := range want {
		if got != expected {
			t.Errorf("role constant = %q, want %q", got, expected)
		}
	}
}

// T3: HashedSubject / HashedEndUser emit log-safe pseudonyms instead of
// raw PII. The helpers are nil-safe and empty-string-safe so every
// logger call site can use them without guards.
func TestAuthenticatedIdentity_HashedSubject_NilSafe(t *testing.T) {
	t.Parallel()
	var id *AuthenticatedIdentity
	if got := id.HashedSubject(); got != "" {
		t.Errorf("nil identity: got %q, want empty", got)
	}
}

func TestAuthenticatedIdentity_HashedSubject_EmptySubject(t *testing.T) {
	t.Parallel()
	id := &AuthenticatedIdentity{Subject: ""}
	if got := id.HashedSubject(); got != "" {
		t.Errorf("empty Subject: got %q, want empty", got)
	}
}

func TestAuthenticatedIdentity_HashedSubject_RedactsRaw(t *testing.T) {
	t.Parallel()
	id := &AuthenticatedIdentity{Subject: "alice@example.com"}
	got := id.HashedSubject()
	if got == "alice@example.com" {
		t.Error("raw subject leaked through HashedSubject")
	}
	if !strings.HasPrefix(got, "[hash:") {
		t.Errorf("HashedSubject = %q, want logging.HashID-style tag", got)
	}
}

func TestAuthenticatedIdentity_HashedSubject_Deterministic(t *testing.T) {
	// Same input → same hash so log-correlation still works across
	// multiple requests from the same caller.
	t.Parallel()
	a := (&AuthenticatedIdentity{Subject: "alice@example.com"}).HashedSubject()
	b := (&AuthenticatedIdentity{Subject: "alice@example.com"}).HashedSubject()
	if a != b {
		t.Errorf("non-deterministic: %q vs %q", a, b)
	}
	c := (&AuthenticatedIdentity{Subject: "bob@example.com"}).HashedSubject()
	if a == c {
		t.Error("distinct subjects must hash distinctly")
	}
}

func TestAuthenticatedIdentity_HashedEndUser_Symmetry(t *testing.T) {
	t.Parallel()
	var nilID *AuthenticatedIdentity
	if got := nilID.HashedEndUser(); got != "" {
		t.Errorf("nil identity: got %q", got)
	}
	if got := (&AuthenticatedIdentity{}).HashedEndUser(); got != "" {
		t.Errorf("empty EndUser: got %q", got)
	}
	id := &AuthenticatedIdentity{EndUser: "carol@example.com"}
	got := id.HashedEndUser()
	if got == "carol@example.com" {
		t.Error("raw end-user leaked through HashedEndUser")
	}
	if !strings.HasPrefix(got, "[hash:") {
		t.Errorf("HashedEndUser = %q, want hash tag", got)
	}
}
