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
		Role:      RoleEditor,
		Claims:    map[string]string{"team": "eng"},
	}
	got := IdentityPayloadFromIdentity(id)
	if got == nil {
		t.Fatal("IdentityPayloadFromIdentity returned nil for non-nil identity")
	}
	if got.Origin != id.Origin || got.Subject != id.Subject || got.EndUser != id.EndUser ||
		got.Workspace != id.Workspace || got.Agent != id.Agent || got.Role != id.Role {
		t.Fatalf("IdentityPayloadFromIdentity = %+v, want fields copied from %+v", got, id)
	}
	if got.Claims["team"] != "eng" {
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
// that have a faithful propagated source (Subject/EndUser from UserID, Role
// from UserRoles, Claims verbatim, Agent from AgentName) and leaves
// Origin/Workspace unset rather than fabricating them.
func TestIdentityPayloadFromPropagation_MapsFaithfulFieldsOnly(t *testing.T) {
	fields := &PropagationFields{
		AgentName: "agent-1",
		Namespace: "some-k8s-namespace", // must NOT leak into Workspace
		UserID:    "user-1",
		UserRoles: RoleEditor,
		Claims:    map[string]string{"team": "eng"},
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
	if got.Role != RoleEditor {
		t.Errorf("Role = %q, want %q", got.Role, RoleEditor)
	}
	if got.Agent != "agent-1" {
		t.Errorf("Agent = %q, want %q", got.Agent, "agent-1")
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

// TestIdentityPayloadFromPropagation_AgentOnlyIsSufficient asserts that
// AgentName alone (no UserID/UserRoles/Claims) is enough to produce a
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
