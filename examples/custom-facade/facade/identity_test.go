/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package facade_test

import (
	"errors"
	"testing"

	"github.com/altairalabs/omnia/examples/custom-facade/facade"
	"github.com/altairalabs/omnia/pkg/policy"
)

func demoPrincipal() *facade.Principal {
	return &facade.Principal{
		UserID:    "user-42",
		Roles:     []string{policy.RoleAdmin, policy.RoleEditor},
		Workspace: "acme",
		Origin:    policy.OriginSharedToken,
		Claims:    map[string]string{"tier": "gold"},
	}
}

func TestAuthenticate_KnownToken(t *testing.T) {
	auth := facade.NewAuthenticator(map[string]*facade.Principal{"tok": demoPrincipal()})
	for _, in := range []string{"tok", "Bearer tok", "bearer tok", "  tok  "} {
		p, err := auth.Authenticate(in)
		if err != nil {
			t.Fatalf("Authenticate(%q) error = %v", in, err)
		}
		if p.UserID != "user-42" {
			t.Errorf("Authenticate(%q) UserID = %q", in, p.UserID)
		}
	}
}

func TestAuthenticate_UnknownToken(t *testing.T) {
	auth := facade.NewAuthenticator(map[string]*facade.Principal{"tok": demoPrincipal()})
	if _, err := auth.Authenticate("nope"); !errors.Is(err, facade.ErrUnknownToken) {
		t.Fatalf("Authenticate(unknown) error = %v, want ErrUnknownToken", err)
	}
}

// TestOutboundMetadata_EmitsFlatContract asserts the emitted metadata carries
// each x-omnia-* field with the canonical lowercase key and correct value,
// including one x-omnia-claim-<name> header per claim.
func TestOutboundMetadata_EmitsFlatContract(t *testing.T) {
	md := demoPrincipal().OutboundMetadata("support-bot")

	cases := map[string]string{
		policy.HeaderUserID:               "user-42",
		policy.HeaderClaimPrefix + "role": "admin,editor",
		policy.HeaderWorkspace:            "acme",
		policy.HeaderOrigin:               policy.OriginSharedToken,
		policy.HeaderAgentName:            "support-bot",
		policy.HeaderClaimPrefix + "tier": "gold",
	}
	for k, want := range cases {
		if got := md[k]; got != want {
			t.Errorf("metadata[%q] = %q, want %q", k, got, want)
		}
	}
}

// TestPropagationFields_JoinsRoles verifies the roles list is comma-joined into
// the identity.claims.role claim (the structured role field was removed in #1775).
func TestPropagationFields_JoinsRoles(t *testing.T) {
	f := demoPrincipal().PropagationFields("support-bot")
	if f.Claims["role"] != "admin,editor" {
		t.Errorf("Claims[role] = %q, want admin,editor", f.Claims["role"])
	}
	if f.AgentName != "support-bot" {
		t.Errorf("AgentName = %q, want support-bot", f.AgentName)
	}
}
