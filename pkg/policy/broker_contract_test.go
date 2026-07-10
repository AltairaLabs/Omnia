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

import "testing"

func TestIdentityPayloadFromIdentity_Nil(t *testing.T) {
	if got := IdentityPayloadFromIdentity(nil); got != nil {
		t.Fatalf("IdentityPayloadFromIdentity(nil) = %+v, want nil", got)
	}
}

func TestIdentityPayloadFromIdentity_CopiesFields(t *testing.T) {
	id := &AuthenticatedIdentity{
		Origin:    OriginOIDC,
		Subject:   "user-1",
		EndUser:   "user-1",
		Workspace: "ws-1",
		Agent:     "agent-1",
		Claims:    map[string]string{"role": "editor", "team": "eng"},
	}
	got := IdentityPayloadFromIdentity(id)
	if got == nil {
		t.Fatal("IdentityPayloadFromIdentity returned nil for non-nil identity")
	}
	if got.Origin != id.Origin || got.Subject != id.Subject || got.EndUser != id.EndUser ||
		got.Workspace != id.Workspace || got.Agent != id.Agent {
		t.Fatalf("IdentityPayloadFromIdentity = %+v, want fields copied from %+v", got, id)
	}
	if got.Claims["role"] != "editor" || got.Claims["team"] != "eng" {
		t.Fatalf("Claims not copied: %+v", got.Claims)
	}
}

func TestIdentityPayloadFromPropagation_Nil(t *testing.T) {
	if got := IdentityPayloadFromPropagation(nil); got != nil {
		t.Fatalf("IdentityPayloadFromPropagation(nil) = %+v, want nil", got)
	}
}

func TestIdentityPayloadFromPropagation_EmptyFieldsReturnsNil(t *testing.T) {
	// Unauthenticated/dev-mode traffic: no propagated identity signal at
	// all. Must send a nil Identity, not an empty-but-present one.
	got := IdentityPayloadFromPropagation(&PropagationFields{})
	if got != nil {
		t.Fatalf("IdentityPayloadFromPropagation(empty) = %+v, want nil", got)
	}
}

// TestIdentityPayloadFromPropagation_MapsFaithfulFieldsOnly is the
// regression test for the CRITICAL finding: the broker client used to build
// its IdentityPayload from IdentityFromContext(ctx), which is always nil on
// the runtime side (AuthenticatedIdentity never crosses the facade->runtime
// gRPC hop). This asserts the reconstructed payload maps exactly the fields
// that have a faithful propagated source (Subject/EndUser from UserID,
// Claims verbatim — including a "role" claim, Agent from AgentName) and
// leaves Origin/Workspace unset rather than fabricating them.
func TestIdentityPayloadFromPropagation_MapsFaithfulFieldsOnly(t *testing.T) {
	fields := &PropagationFields{
		AgentName: "agent-1",
		Namespace: "some-k8s-namespace", // must NOT leak into Workspace
		UserID:    "user-1",
		Claims:    map[string]string{"role": "editor", "team": "eng"},
	}

	got := IdentityPayloadFromPropagation(fields)
	if got == nil {
		t.Fatal("IdentityPayloadFromPropagation returned nil for non-empty fields")
	}
	if got.Subject != "user-1" {
		t.Errorf("Subject = %q, want %q", got.Subject, "user-1")
	}
	if got.EndUser != "user-1" {
		t.Errorf("EndUser = %q, want %q", got.EndUser, "user-1")
	}
	if got.Agent != "agent-1" {
		t.Errorf("Agent = %q, want %q", got.Agent, "agent-1")
	}
	if got.Claims["role"] != "editor" {
		t.Errorf("Claims[role] = %q, want %q", got.Claims["role"], "editor")
	}
	if got.Claims["team"] != "eng" {
		t.Errorf("Claims[team] = %q, want %q", got.Claims["team"], "eng")
	}
	if got.Origin != "" {
		t.Errorf("Origin = %q, want empty (no faithful propagated source)", got.Origin)
	}
	if got.Workspace != "" {
		t.Errorf("Workspace = %q, want empty (Namespace is a distinct concept)", got.Workspace)
	}
}

// TestIdentityPayloadFromPropagation_RoleInClaims asserts the wire payload
// carries role via Claims["role"], not a dedicated Role field (removed in
// #1775 Task 5 — roles ride in identity.claims.role end-to-end).
func TestIdentityPayloadFromPropagation_RoleInClaims(t *testing.T) {
	p := IdentityPayloadFromPropagation(&PropagationFields{
		UserID: "u1",
		Claims: map[string]string{"role": "editor"},
	})
	if p == nil {
		t.Fatal("nil payload")
	}
	if got, want := p.Claims["role"], "editor"; got != want {
		t.Fatalf("Claims[role] = %q, want %q", got, want)
	}
}

// TestIdentityPayloadFromPropagation_AgentOnlyIsSufficient asserts that
// AgentName alone (no UserID/Claims) is enough to produce a
// non-nil payload — an agent-scoped ToolPolicy rule (identity.agent) should
// still be able to fire even for anonymous/unauthenticated tool calls.
func TestIdentityPayloadFromPropagation_AgentOnlyIsSufficient(t *testing.T) {
	got := IdentityPayloadFromPropagation(&PropagationFields{AgentName: "agent-1"})
	if got == nil {
		t.Fatal("IdentityPayloadFromPropagation returned nil when AgentName was set")
	}
	if got.Agent != "agent-1" {
		t.Errorf("Agent = %q, want %q", got.Agent, "agent-1")
	}
}
