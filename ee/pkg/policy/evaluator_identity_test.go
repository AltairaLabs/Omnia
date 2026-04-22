/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"context"
	"testing"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	omniapolicy "github.com/altairalabs/omnia/pkg/policy"
)

// newIdentityPolicy builds a single-rule ToolPolicy whose deny expression is
// provided by the caller. Used by the identity-root tests below.
func newIdentityPolicy(name, expr string) *omniav1alpha1.ToolPolicy {
	return newTestPolicy(name, []omniav1alpha1.PolicyRule{
		{
			Name: "identity-rule",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     expr,
				Message: "denied by identity rule",
			},
		},
	})
}

// evalIdentityRule compiles an identity-root rule into a fresh evaluator and
// evaluates it against the supplied identity (or no identity if nil).
func evalIdentityRule(
	t *testing.T,
	expr string,
	identity *omniapolicy.AuthenticatedIdentity,
) Decision {
	t.Helper()

	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}
	policy := newIdentityPolicy("identity-policy", expr)
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:     "process_refund",
		HeaderToolRegistry: "customer-tools",
	}
	ctx := context.Background()
	if identity != nil {
		ctx = omniapolicy.WithIdentity(ctx, identity)
	}
	return eval.EvaluateWithContext(ctx, headers, nil)
}

// TestEvaluateWithContext_IdentityOrigin_ManagementPlane asserts the
// `identity.origin == "management-plane"` idiom from the design doc.
func TestEvaluateWithContext_IdentityOrigin_ManagementPlane(t *testing.T) {
	identity := &omniapolicy.AuthenticatedIdentity{
		Origin:  omniapolicy.OriginManagementPlane,
		Subject: "user-1",
	}

	decision := evalIdentityRule(t, `identity.origin == "management-plane"`, identity)
	if decision.Allowed {
		t.Error("rule should have denied when identity.origin == management-plane")
	}
	if decision.DeniedBy != "identity-rule" {
		t.Errorf("DeniedBy = %q, want identity-rule", decision.DeniedBy)
	}
}

// TestEvaluateWithContext_IdentityOrigin_NoIdentity asserts the zero-value
// default: a context without Identity surfaces identity.origin == "", so a
// `identity.origin == "management-plane"` rule evaluates to false (no deny).
func TestEvaluateWithContext_IdentityOrigin_NoIdentity(t *testing.T) {
	decision := evalIdentityRule(t, `identity.origin == "management-plane"`, nil)
	if !decision.Allowed {
		t.Error("rule should be allowed when context carries no identity (zero-value origin)")
	}
}

// TestEvaluateWithContext_IdentityRole_Viewer verifies identity.role is
// usable from CEL.
func TestEvaluateWithContext_IdentityRole_Viewer(t *testing.T) {
	identity := &omniapolicy.AuthenticatedIdentity{
		Origin: omniapolicy.OriginOIDC,
		Role:   omniapolicy.RoleViewer,
	}

	decision := evalIdentityRule(t, `identity.role == "viewer"`, identity)
	if decision.Allowed {
		t.Error("rule should have denied when identity.role == viewer")
	}
}

// TestEvaluateWithContext_IdentitySubject_NotEqualEndUser covers the
// service-token-spoofing-end-user detection pattern called out in the design
// doc: rule flags calls where the token holder and the acted-upon user
// differ.
func TestEvaluateWithContext_IdentitySubject_NotEqualEndUser(t *testing.T) {
	identity := &omniapolicy.AuthenticatedIdentity{
		Origin:  omniapolicy.OriginSharedToken,
		Subject: "svc-x",
		EndUser: "alice",
	}

	decision := evalIdentityRule(t, `identity.subject != identity.endUser`, identity)
	if decision.Allowed {
		t.Error("rule should have denied when subject != endUser")
	}
}

// TestEvaluateWithContext_IdentityClaim_Present verifies that arbitrary
// claims are reachable via identity.claims.<name>.
func TestEvaluateWithContext_IdentityClaim_Present(t *testing.T) {
	identity := &omniapolicy.AuthenticatedIdentity{
		Origin: omniapolicy.OriginOIDC,
		Claims: map[string]string{"groups": "finance"},
	}

	decision := evalIdentityRule(t, `identity.claims.groups == "finance"`, identity)
	if decision.Allowed {
		t.Error("rule should have denied when claims.groups == finance")
	}
}

// TestEvaluateWithContext_IdentityClaim_MissingViaHas matches the idiom used
// elsewhere (`has(body.field)`): rules probe for optional claims with has(),
// and a missing claim is simply absent from the map.
func TestEvaluateWithContext_IdentityClaim_MissingViaHas(t *testing.T) {
	identity := &omniapolicy.AuthenticatedIdentity{
		Origin: omniapolicy.OriginOIDC,
		Claims: map[string]string{"groups": "finance"},
	}

	decision := evalIdentityRule(t, `!has(identity.claims.missing)`, identity)
	if decision.Allowed {
		t.Error("rule should have denied when has(identity.claims.missing) is false")
	}
}

// TestEvaluateWithContext_IdentityWorkspaceAgent covers the remaining flat
// identity fields (workspace, agent) in a single rule.
func TestEvaluateWithContext_IdentityWorkspaceAgent(t *testing.T) {
	identity := &omniapolicy.AuthenticatedIdentity{
		Origin:    omniapolicy.OriginAPIKey,
		Workspace: "ws-finance",
		Agent:     "support-bot",
	}

	expr := `identity.workspace == "ws-finance" && identity.agent == "support-bot"`
	decision := evalIdentityRule(t, expr, identity)
	if decision.Allowed {
		t.Error("rule should have denied when workspace/agent both match")
	}
}

// TestEvaluateWithContext_NoIdentity_ClaimsEmpty asserts that when no
// identity is attached, identity.claims is an empty map and `has(...)` on
// any claim is false.
func TestEvaluateWithContext_NoIdentity_ClaimsEmpty(t *testing.T) {
	decision := evalIdentityRule(t, `has(identity.claims.groups)`, nil)
	if !decision.Allowed {
		t.Error("rule should be allowed when no identity is attached (claims empty)")
	}
}

// TestEvaluateWithContext_NilClaimsMap verifies an identity with nil Claims
// still yields a non-nil CEL map so `has(...)` works without erroring.
func TestEvaluateWithContext_NilClaimsMap(t *testing.T) {
	identity := &omniapolicy.AuthenticatedIdentity{
		Origin: omniapolicy.OriginAPIKey,
		// Claims left nil on purpose.
	}

	decision := evalIdentityRule(t, `has(identity.claims.any)`, identity)
	if !decision.Allowed {
		t.Error("rule should be allowed when Claims is nil (treated as empty map)")
	}
}

// TestEvaluateHeaderInjectionWithContext_Identity covers the header
// injection path — identity should be reachable from CEL-valued header
// injection rules too.
func TestEvaluateHeaderInjectionWithContext_Identity(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("inject-policy", []omniav1alpha1.PolicyRule{
		{
			Name: "never",
			Deny: omniav1alpha1.PolicyRuleDeny{CEL: "false", Message: "never"},
		},
	})
	policy.Spec.HeaderInjection = []omniav1alpha1.HeaderInjectionRule{
		{
			Header: "X-Omnia-Injected-Origin",
			CEL:    "identity.origin",
		},
	}
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	ctx := omniapolicy.WithIdentity(context.Background(), &omniapolicy.AuthenticatedIdentity{
		Origin: omniapolicy.OriginManagementPlane,
	})
	headers := map[string]string{
		HeaderToolName:     "process_refund",
		HeaderToolRegistry: "customer-tools",
	}

	injected, err := eval.EvaluateHeaderInjectionWithContext(ctx, headers, nil)
	if err != nil {
		t.Fatalf("EvaluateHeaderInjectionWithContext() error = %v", err)
	}
	got := injected["X-Omnia-Injected-Origin"]
	if got != omniapolicy.OriginManagementPlane {
		t.Errorf("injected origin = %q, want %q", got, omniapolicy.OriginManagementPlane)
	}
}

// TestEvaluate_LegacyAPI_StillWorks confirms the non-context Evaluate entry
// point keeps working with identity-agnostic rules — a sanity check for
// back-compat with callers that have not been updated to pass context.
func TestEvaluate_LegacyAPI_StillWorks(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("legacy", []omniav1alpha1.PolicyRule{
		{
			Name: "deny-mgmt-plane",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     `identity.origin == "management-plane"`,
				Message: "blocked",
			},
		},
	})
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:     "process_refund",
		HeaderToolRegistry: "customer-tools",
	}

	// Legacy API path → no identity attached → origin defaults to "" → rule
	// does not match → allowed.
	decision := eval.Evaluate(headers, nil)
	if !decision.Allowed {
		t.Errorf("legacy Evaluate() should allow (origin empty); got denied by %q", decision.DeniedBy)
	}
}

// TestEvaluateHeaderInjection_LegacyAPI_StillWorks mirrors the above for the
// header-injection entry point.
func TestEvaluateHeaderInjection_LegacyAPI_StillWorks(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("legacy-inject", []omniav1alpha1.PolicyRule{
		{
			Name: "never",
			Deny: omniav1alpha1.PolicyRuleDeny{CEL: "false", Message: "never"},
		},
	})
	policy.Spec.HeaderInjection = []omniav1alpha1.HeaderInjectionRule{
		{
			Header: "X-Omnia-Injected-Origin",
			CEL:    "identity.origin",
		},
	}
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:     "process_refund",
		HeaderToolRegistry: "customer-tools",
	}

	injected, err := eval.EvaluateHeaderInjection(headers, nil)
	if err != nil {
		t.Fatalf("EvaluateHeaderInjection() error = %v", err)
	}
	if got := injected["X-Omnia-Injected-Origin"]; got != "" {
		t.Errorf("injected origin = %q, want empty string (no identity)", got)
	}
}
