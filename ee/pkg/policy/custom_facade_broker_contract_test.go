/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eeapi "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/examples/custom-facade/facade"
	omniapolicy "github.com/altairalabs/omnia/pkg/policy"
)

// customFacadePrincipal is the same rich identity the reference custom facade
// authenticates and emits (examples/custom-facade). The broker contract test
// drives it through the exact production reconstruction path so an assertion
// failure here means the facade's emitted identity would not reach ToolPolicy
// CEL in a real deployment.
func customFacadePrincipal() *facade.Principal {
	return &facade.Principal{
		UserID:    "user-42",
		Roles:     []string{omniapolicy.RoleAdmin},
		Workspace: "acme",
		Origin:    omniapolicy.OriginSharedToken,
		Claims:    map[string]string{"tier": "gold", "team": "finance"},
	}
}

// identityContractPolicy denies when EVERY identity dimension the facade emits
// is present with its expected value. If the broker reconstructs identity
// correctly from the wire payload, this rule fires (deny); if any dimension is
// dropped, it stays allowed — so a single decision proves subject/endUser,
// role, origin, workspace and claims all arrived.
func identityContractPolicy() *eeapi.ToolPolicy {
	const expr = `identity.subject == "user-42" && ` +
		`identity.endUser == "user-42" && ` +
		`identity.role == "admin" && ` +
		`identity.origin == "shared-token" && ` +
		`identity.workspace == "acme" && ` +
		`has(identity.claims.tier) && identity.claims.tier == "gold" && ` +
		`has(identity.claims.team) && identity.claims.team == "finance"`
	return &eeapi.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "identity-contract", Namespace: "default"},
		Spec: eeapi.ToolPolicySpec{
			Selector: eeapi.ToolPolicySelector{
				Registry: "customer-tools",
				Tools:    []string{"process_refund"},
			},
			Rules: []eeapi.PolicyRule{{
				Name: "require-full-identity",
				Deny: eeapi.PolicyRuleDeny{CEL: expr, Message: "identity matched"},
			}},
			Mode:      eeapi.PolicyModeEnforce,
			OnFailure: eeapi.OnFailureDeny,
		},
	}
}

// decideForFields runs a DecisionRequest through the real BrokerHandler,
// building the Identity payload exactly as internal/runtime does on the wire:
// omniapolicy.IdentityPayloadFromPropagation over the flat propagation fields.
func decideForFields(t *testing.T, fields *omniapolicy.PropagationFields) DecisionResponse {
	t.Helper()
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}
	if err := eval.CompilePolicy(identityContractPolicy()); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}
	handler := NewBrokerHandler(eval, testBrokerLogger())

	req := newDecisionRequest(t, DecisionRequest{
		Headers: map[string]string{
			HeaderToolName:     "process_refund",
			HeaderToolRegistry: "customer-tools",
		},
		Identity: omniapolicy.IdentityPayloadFromPropagation(fields),
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	return decodeDecisionResponse(t, rec)
}

// TestBroker_SeesCustomFacadeIdentity proves the policy broker sees the caller
// id, role, origin, workspace and full claim map that the reference custom
// facade emits. The facade's Principal -> PropagationFields mapping feeds the
// same IdentityPayloadFromPropagation reconstruction the runtime uses, so the
// deny (all dimensions matched) confirms the whole identity contract survives
// facade -> runtime -> broker.
func TestBroker_SeesCustomFacadeIdentity(t *testing.T) {
	fields := customFacadePrincipal().PropagationFields("support-bot")

	resp := decideForFields(t, fields)
	if resp.Allow {
		t.Fatal("Allow = true; broker did not see the full facade identity (expected deny)")
	}
	if resp.DeniedBy != "require-full-identity" {
		t.Errorf("DeniedBy = %q, want require-full-identity", resp.DeniedBy)
	}
}

// TestBroker_AnonymousDoesNotMatchIdentity is the negative control: an
// unauthenticated request carries no identity, so IdentityPayloadFromPropagation
// is nil and the identity rule cannot match — the broker must allow, proving
// the deny above was driven by the emitted identity and not a rule that fires
// unconditionally.
func TestBroker_AnonymousDoesNotMatchIdentity(t *testing.T) {
	resp := decideForFields(t, &omniapolicy.PropagationFields{})
	if !resp.Allow {
		t.Errorf("Allow = false; anonymous call should not match the identity rule (DeniedBy=%q)", resp.DeniedBy)
	}
}

// TestBroker_MissingClaimDoesNotMatch confirms per-claim granularity: drop one
// claim the facade would emit and the all-dimensions rule no longer fires, so
// the broker genuinely reads identity.claims.<name> rather than matching on a
// coarser signal.
func TestBroker_MissingClaimDoesNotMatch(t *testing.T) {
	p := customFacadePrincipal()
	delete(p.Claims, "team")
	resp := decideForFields(t, p.PropagationFields("support-bot"))
	if !resp.Allow {
		t.Errorf("Allow = false; a missing claim must break the identity match (DeniedBy=%q)", resp.DeniedBy)
	}
}
