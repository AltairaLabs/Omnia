/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func newTestPolicy(name string, rules []omniav1alpha1.PolicyRule) *omniav1alpha1.ToolPolicy {
	return &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "customer-tools",
				Tools:    []string{"process_refund"},
			},
			Rules:     rules,
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
}

func TestNewEvaluator(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}
	if eval == nil {
		t.Fatal("NewEvaluator() returned nil")
	}
	if eval.PolicyCount() != 0 {
		t.Errorf("PolicyCount() = %d, want 0", eval.PolicyCount())
	}
}

func TestCompilePolicy_ValidRules(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("test-policy", []omniav1alpha1.PolicyRule{
		{
			Name: "amount-limit",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     "int(headers['X-Omnia-Param-Amount']) > 1000",
				Message: "Amount exceeds limit",
			},
		},
	})

	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	if eval.PolicyCount() != 1 {
		t.Errorf("PolicyCount() = %d, want 1", eval.PolicyCount())
	}
}

func TestCompilePolicy_InvalidCEL(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("bad-policy", []omniav1alpha1.PolicyRule{
		{
			Name: "bad-rule",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     "this is not valid CEL %%%",
				Message: "Should not compile",
			},
		},
	})

	err = eval.CompilePolicy(policy)
	if err == nil {
		t.Fatal("CompilePolicy() expected error for invalid CEL")
	}
}

func TestRemovePolicy(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("removable", []omniav1alpha1.PolicyRule{
		{
			Name: "r1",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     "true",
				Message: "always deny",
			},
		},
	})

	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}
	if eval.PolicyCount() != 1 {
		t.Errorf("PolicyCount() = %d, want 1", eval.PolicyCount())
	}

	eval.RemovePolicy("default", "removable")
	if eval.PolicyCount() != 0 {
		t.Errorf("PolicyCount() = %d, want 0 after removal", eval.PolicyCount())
	}
}

func TestEvaluate_AllowWhenNoMatchingPolicy(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:     "other_tool",
		HeaderToolRegistry: "other-registry",
	}
	decision := eval.Evaluate(headers, nil)
	if !decision.Allowed {
		t.Errorf("Evaluate() Allowed = false, want true for no matching policy")
	}
}

func TestEvaluate_DenyByAmountRule(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("refund-limit", []omniav1alpha1.PolicyRule{
		{
			Name: "amount-limit",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     "int(headers['X-Omnia-Param-Amount']) > 1000",
				Message: "Refunds over $1000 not allowed",
			},
		},
	})
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:         "process_refund",
		HeaderToolRegistry:     "customer-tools",
		"X-Omnia-Param-Amount": "2000",
	}

	decision := eval.Evaluate(headers, nil)
	if decision.Allowed {
		t.Error("Evaluate() Allowed = true, want false for amount > 1000")
	}
	if decision.DeniedBy != "amount-limit" {
		t.Errorf("DeniedBy = %q, want %q", decision.DeniedBy, "amount-limit")
	}
	if decision.Message != "Refunds over $1000 not allowed" {
		t.Errorf("Message = %q, want %q", decision.Message, "Refunds over $1000 not allowed")
	}
}

func TestEvaluate_AllowByAmountRule(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("refund-limit", []omniav1alpha1.PolicyRule{
		{
			Name: "amount-limit",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     "int(headers['X-Omnia-Param-Amount']) > 1000",
				Message: "Refunds over $1000 not allowed",
			},
		},
	})
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:         "process_refund",
		HeaderToolRegistry:     "customer-tools",
		"X-Omnia-Param-Amount": "500",
	}

	decision := eval.Evaluate(headers, nil)
	if !decision.Allowed {
		t.Errorf("Evaluate() Allowed = false, want true for amount <= 1000")
	}
}

func TestEvaluate_MultipleRulesFirstDenyStops(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("multi-rule", []omniav1alpha1.PolicyRule{
		{
			Name: "rule-a",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     "headers['X-Omnia-Param-Amount'] == '999'",
				Message: "rule A denied",
			},
		},
		{
			Name: "rule-b",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     "true",
				Message: "rule B always denies",
			},
		},
	})
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:         "process_refund",
		HeaderToolRegistry:     "customer-tools",
		"X-Omnia-Param-Amount": "999",
	}

	decision := eval.Evaluate(headers, nil)
	if decision.Allowed {
		t.Error("Evaluate() Allowed = true, want false")
	}
	if decision.DeniedBy != "rule-a" {
		t.Errorf("DeniedBy = %q, want %q (first matching rule)", decision.DeniedBy, "rule-a")
	}
}

func TestEvaluate_RequiredClaimsMissing(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("claims-policy", []omniav1alpha1.PolicyRule{
		{
			Name: "always-allow",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     "false",
				Message: "never deny",
			},
		},
	})
	policy.Spec.RequiredClaims = []omniav1alpha1.RequiredClaim{
		{
			Claim:   "customer_id",
			Message: "Requires customer_id claim",
		},
	}
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:     "process_refund",
		HeaderToolRegistry: "customer-tools",
	}

	decision := eval.Evaluate(headers, nil)
	if decision.Allowed {
		t.Error("Evaluate() Allowed = true, want false for missing claim")
	}
	if decision.DeniedBy != "required-claim:customer_id" {
		t.Errorf("DeniedBy = %q, want %q", decision.DeniedBy, "required-claim:customer_id")
	}
}

func TestEvaluate_RequiredClaimsPresent(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("claims-policy", []omniav1alpha1.PolicyRule{
		{
			Name: "always-allow",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     "false",
				Message: "never deny",
			},
		},
	})
	policy.Spec.RequiredClaims = []omniav1alpha1.RequiredClaim{
		{
			Claim:   "customer_id",
			Message: "Requires customer_id claim",
		},
	}
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:              "process_refund",
		HeaderToolRegistry:          "customer-tools",
		"X-Omnia-Claim-customer_id": "cust-123",
	}

	decision := eval.Evaluate(headers, nil)
	if !decision.Allowed {
		t.Errorf("Evaluate() Allowed = false, want true when claim is present")
	}
}

func TestEvaluate_AuditModeAllowsDenied(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("audit-policy", []omniav1alpha1.PolicyRule{
		{
			Name: "always-deny",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     "true",
				Message: "always deny",
			},
		},
	})
	policy.Spec.Mode = omniav1alpha1.PolicyModeAudit
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:     "process_refund",
		HeaderToolRegistry: "customer-tools",
	}

	decision := eval.Evaluate(headers, nil)
	if !decision.Allowed {
		t.Error("Evaluate() Allowed = false, want true in audit mode")
	}
	if decision.DeniedBy != "always-deny" {
		t.Errorf("DeniedBy = %q, want %q (should report rule even in audit mode)", decision.DeniedBy, "always-deny")
	}
	if !decision.WouldDeny {
		t.Error("WouldDeny = false, want true in audit mode when rule matches")
	}
	if decision.Mode != omniav1alpha1.PolicyModeAudit {
		t.Errorf("Mode = %q, want %q", decision.Mode, omniav1alpha1.PolicyModeAudit)
	}
	if decision.Policy != "audit-policy" {
		t.Errorf("Policy = %q, want %q", decision.Policy, "audit-policy")
	}
}

func TestEvaluate_AuditModeRequiredClaimsMissing(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("audit-claims-policy", []omniav1alpha1.PolicyRule{
		{
			Name: "no-deny",
			Deny: omniav1alpha1.PolicyRuleDeny{CEL: "false", Message: "never"},
		},
	})
	policy.Spec.Mode = omniav1alpha1.PolicyModeAudit
	policy.Spec.RequiredClaims = []omniav1alpha1.RequiredClaim{
		{Claim: "team", Message: "team required"},
	}
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:     "process_refund",
		HeaderToolRegistry: "customer-tools",
	}

	decision := eval.Evaluate(headers, nil)
	if !decision.Allowed {
		t.Error("Evaluate() Allowed = false, want true in audit mode even with missing claim")
	}
	if !decision.WouldDeny {
		t.Error("WouldDeny = false, want true when claim is missing in audit mode")
	}
	if decision.DeniedBy != "required-claim:team" {
		t.Errorf("DeniedBy = %q, want %q", decision.DeniedBy, "required-claim:team")
	}
}

func TestEvaluate_EnforceModeDecisionFields(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("enforce-policy", []omniav1alpha1.PolicyRule{
		{
			Name: "deny-rule",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     "true",
				Message: "always deny",
			},
		},
	})
	policy.Spec.Mode = omniav1alpha1.PolicyModeEnforce
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:     "process_refund",
		HeaderToolRegistry: "customer-tools",
	}

	decision := eval.Evaluate(headers, nil)
	if decision.Allowed {
		t.Error("Evaluate() Allowed = true, want false in enforce mode")
	}
	if decision.WouldDeny {
		t.Error("WouldDeny = true, want false in enforce mode (actual denial, not hypothetical)")
	}
	if decision.Mode != omniav1alpha1.PolicyModeEnforce {
		t.Errorf("Mode = %q, want %q", decision.Mode, omniav1alpha1.PolicyModeEnforce)
	}
	if decision.Policy != "enforce-policy" {
		t.Errorf("Policy = %q, want %q", decision.Policy, "enforce-policy")
	}
}

func TestEvaluate_OnFailureAllow(t *testing.T) {
	// Test the handleEvalError function directly for onFailure=allow
	t.Run("handleEvalError returns allowed", func(t *testing.T) {
		testErr := fmt.Errorf("test evaluation error")
		decision := handleEvalError("test-rule", testErr, omniav1alpha1.OnFailureAllow)
		if !decision.Allowed {
			t.Error("handleEvalError() Allowed = false, want true with onFailure=allow")
		}
		if decision.Error == nil {
			t.Error("handleEvalError() Error = nil, want non-nil")
		}
	})

	// Test that onFailure=allow with a valid but false rule still passes
	t.Run("valid rule with onFailure=allow", func(t *testing.T) {
		eval, err := NewEvaluator()
		if err != nil {
			t.Fatalf("NewEvaluator() error = %v", err)
		}

		policy := newTestPolicy("allow-policy", []omniav1alpha1.PolicyRule{
			{
				Name: "allow-rule",
				Deny: omniav1alpha1.PolicyRuleDeny{
					CEL:     "false",
					Message: "never deny",
				},
			},
		})
		policy.Spec.OnFailure = omniav1alpha1.OnFailureAllow
		if err := eval.CompilePolicy(policy); err != nil {
			t.Fatalf("CompilePolicy() error = %v", err)
		}

		headers := map[string]string{
			HeaderToolName:     "process_refund",
			HeaderToolRegistry: "customer-tools",
		}
		decision := eval.Evaluate(headers, nil)
		if !decision.Allowed {
			t.Error("Evaluate() Allowed = false, want true")
		}
	})
}

func TestEvaluate_OnFailureDeny(t *testing.T) {
	// Test the handleEvalError function directly for onFailure=deny
	testErr := fmt.Errorf("test evaluation error")
	decision := handleEvalError("test-rule", testErr, omniav1alpha1.OnFailureDeny)
	if decision.Allowed {
		t.Error("handleEvalError() Allowed = true, want false with onFailure=deny")
	}
	if decision.Error == nil {
		t.Error("handleEvalError() Error = nil, want non-nil")
	}
	if decision.DeniedBy != "test-rule" {
		t.Errorf("DeniedBy = %q, want %q", decision.DeniedBy, "test-rule")
	}
}

func TestEvaluate_SelectorMatchesAll(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "all-tools",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "customer-tools",
			},
			Rules: []omniav1alpha1.PolicyRule{
				{
					Name: "deny-all",
					Deny: omniav1alpha1.PolicyRuleDeny{
						CEL:     "true",
						Message: "denied by catch-all",
					},
				},
			},
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:     "any_tool_name",
		HeaderToolRegistry: "customer-tools",
	}

	decision := eval.Evaluate(headers, nil)
	if decision.Allowed {
		t.Error("Evaluate() Allowed = true, want false for catch-all selector")
	}
}

func TestEvaluate_SelectorNoMatchDifferentRegistry(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("registry-policy", []omniav1alpha1.PolicyRule{
		{
			Name: "deny-all",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     "true",
				Message: "denied",
			},
		},
	})
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:     "process_refund",
		HeaderToolRegistry: "different-registry",
	}

	decision := eval.Evaluate(headers, nil)
	if !decision.Allowed {
		t.Error("Evaluate() Allowed = false, want true for non-matching registry")
	}
}

func TestEvaluate_CELWithRolesCheck(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("role-policy", []omniav1alpha1.PolicyRule{
		{
			Name: "supervisor-required",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL: "int(headers['X-Omnia-Param-Amount']) > 1000 " +
					"&& !('supervisor' in headers['X-Omnia-User-Roles'].split(','))",
				Message: "Refunds over $1000 require supervisor role",
			},
		},
	})
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	tests := []struct {
		name    string
		amount  string
		roles   string
		allowed bool
	}{
		{"low amount no supervisor", "500", "agent", true},
		{"high amount with supervisor", "2000", "agent,supervisor", true},
		{"high amount without supervisor", "2000", "agent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := map[string]string{
				HeaderToolName:         "process_refund",
				HeaderToolRegistry:     "customer-tools",
				"X-Omnia-Param-Amount": tt.amount,
				"X-Omnia-User-Roles":   tt.roles,
			}
			decision := eval.Evaluate(headers, nil)
			if decision.Allowed != tt.allowed {
				t.Errorf("Evaluate() Allowed = %v, want %v", decision.Allowed, tt.allowed)
			}
		})
	}
}

func TestEvaluate_CELWithBodyAccess(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("body-policy", []omniav1alpha1.PolicyRule{
		{
			Name: "body-check",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     "has(body.restricted) && body.restricted == true",
				Message: "restricted field not allowed",
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

	t.Run("body without restricted field", func(t *testing.T) {
		body := map[string]interface{}{"amount": 100}
		decision := eval.Evaluate(headers, body)
		if !decision.Allowed {
			t.Error("Evaluate() Allowed = false, want true")
		}
	})

	t.Run("body with restricted=true", func(t *testing.T) {
		body := map[string]interface{}{"restricted": true}
		decision := eval.Evaluate(headers, body)
		if decision.Allowed {
			t.Error("Evaluate() Allowed = true, want false")
		}
	})
}

func TestValidateCEL(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"valid simple", "true", false},
		{"valid header access", "headers['X-Test'] == 'value'", false},
		{"valid body access", "has(body.field)", false},
		{"invalid syntax", "this is not valid %%%", true},
		{"valid int conversion", "int(headers['X-Amount']) > 100", false},
		{"valid string split", "'admin' in headers['X-Roles'].split(',')", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := eval.ValidateCEL(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCEL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEvaluate_MultiplePolicies(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy1 := newTestPolicy("policy-1", []omniav1alpha1.PolicyRule{
		{
			Name: "low-limit",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     "int(headers['X-Omnia-Param-Amount']) > 500",
				Message: "Amount over 500",
			},
		},
	})

	policy2 := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy-2",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "customer-tools",
				Tools:    []string{"process_refund"},
			},
			Rules: []omniav1alpha1.PolicyRule{
				{
					Name: "agent-check",
					Deny: omniav1alpha1.PolicyRuleDeny{
						CEL:     "headers['X-Omnia-Agent-Name'] == 'restricted-agent'",
						Message: "Restricted agent",
					},
				},
			},
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}

	if err := eval.CompilePolicy(policy1); err != nil {
		t.Fatalf("CompilePolicy(1) error = %v", err)
	}
	if err := eval.CompilePolicy(policy2); err != nil {
		t.Fatalf("CompilePolicy(2) error = %v", err)
	}

	if eval.PolicyCount() != 2 {
		t.Errorf("PolicyCount() = %d, want 2", eval.PolicyCount())
	}

	headers := map[string]string{
		HeaderToolName:         "process_refund",
		HeaderToolRegistry:     "customer-tools",
		"X-Omnia-Param-Amount": "300",
		"X-Omnia-Agent-Name":   "restricted-agent",
	}

	decision := eval.Evaluate(headers, nil)
	if decision.Allowed {
		t.Error("Evaluate() Allowed = true, want false (second policy should deny)")
	}
}

func TestMatchesSelector(t *testing.T) {
	tests := []struct {
		name     string
		selector omniav1alpha1.ToolPolicySelector
		registry string
		tool     string
		want     bool
	}{
		{
			"exact match",
			omniav1alpha1.ToolPolicySelector{Registry: "reg", Tools: []string{"tool1"}},
			"reg", "tool1", true,
		},
		{
			"wrong registry",
			omniav1alpha1.ToolPolicySelector{Registry: "reg", Tools: []string{"tool1"}},
			"other", "tool1", false,
		},
		{
			"wrong tool",
			omniav1alpha1.ToolPolicySelector{Registry: "reg", Tools: []string{"tool1"}},
			"reg", "tool2", false,
		},
		{
			"empty tools matches all",
			omniav1alpha1.ToolPolicySelector{Registry: "reg"},
			"reg", "anything", true,
		},
		{
			"multiple tools match second",
			omniav1alpha1.ToolPolicySelector{
				Registry: "reg", Tools: []string{"tool1", "tool2"},
			},
			"reg", "tool2", true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesSelector(tt.selector, tt.registry, tt.tool)
			if got != tt.want {
				t.Errorf("matchesSelector() = %v, want %v", got, tt.want)
			}
		})
	}
}

func newTestPolicyWithHeaders(
	name string,
	rules []omniav1alpha1.PolicyRule,
	headers []omniav1alpha1.HeaderInjectionRule,
) *omniav1alpha1.ToolPolicy {
	p := newTestPolicy(name, rules)
	p.Spec.HeaderInjection = headers
	return p
}

func TestCompilePolicy_HeaderInjectionStaticValue(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicyWithHeaders("static-header", minimalRules(),
		[]omniav1alpha1.HeaderInjectionRule{
			{Header: "X-Custom-Header", Value: "static-value"},
		},
	)

	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}
	if eval.PolicyCount() != 1 {
		t.Errorf("PolicyCount() = %d, want 1", eval.PolicyCount())
	}
}

func TestCompilePolicy_HeaderInjectionCELValue(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicyWithHeaders("cel-header", minimalRules(),
		[]omniav1alpha1.HeaderInjectionRule{
			{Header: "X-Derived", CEL: "headers['X-Omnia-Claim-team']"},
		},
	)

	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}
}

func TestCompilePolicy_HeaderInjectionBothValueAndCEL(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicyWithHeaders("both-set", minimalRules(),
		[]omniav1alpha1.HeaderInjectionRule{
			{Header: "X-Bad", Value: "static", CEL: "headers['X-Test']"},
		},
	)

	if err := eval.CompilePolicy(policy); err == nil {
		t.Fatal("CompilePolicy() expected error when both value and cel are set")
	}
}

func TestCompilePolicy_HeaderInjectionNeitherValueNorCEL(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicyWithHeaders("neither-set", minimalRules(),
		[]omniav1alpha1.HeaderInjectionRule{
			{Header: "X-Empty"},
		},
	)

	if err := eval.CompilePolicy(policy); err == nil {
		t.Fatal("CompilePolicy() expected error when neither value nor cel is set")
	}
}

func TestCompilePolicy_HeaderInjectionInvalidCEL(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicyWithHeaders("bad-cel-header", minimalRules(),
		[]omniav1alpha1.HeaderInjectionRule{
			{Header: "X-Bad", CEL: "invalid CEL %%%"},
		},
	)

	if err := eval.CompilePolicy(policy); err == nil {
		t.Fatal("CompilePolicy() expected error for invalid CEL in header injection")
	}
}

func TestEvaluateHeaderInjection_StaticValue(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicyWithHeaders("static-inject", minimalRules(),
		[]omniav1alpha1.HeaderInjectionRule{
			{Header: "X-Api-Key", Value: "secret-key-123"},
		},
	)
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:     "process_refund",
		HeaderToolRegistry: "customer-tools",
	}

	result, err := eval.EvaluateHeaderInjection(headers, nil)
	if err != nil {
		t.Fatalf("EvaluateHeaderInjection() error = %v", err)
	}
	if result["X-Api-Key"] != "secret-key-123" {
		t.Errorf("X-Api-Key = %q, want %q", result["X-Api-Key"], "secret-key-123")
	}
}

func TestEvaluateHeaderInjection_CELValue(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicyWithHeaders("cel-inject", minimalRules(),
		[]omniav1alpha1.HeaderInjectionRule{
			{Header: "X-Team", CEL: "headers['X-Omnia-Claim-team']"},
		},
	)
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:       "process_refund",
		HeaderToolRegistry:   "customer-tools",
		"X-Omnia-Claim-team": "billing",
	}

	result, err := eval.EvaluateHeaderInjection(headers, nil)
	if err != nil {
		t.Fatalf("EvaluateHeaderInjection() error = %v", err)
	}
	if result["X-Team"] != "billing" {
		t.Errorf("X-Team = %q, want %q", result["X-Team"], "billing")
	}
}

func TestEvaluateHeaderInjection_CELWithBody(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicyWithHeaders("body-inject", minimalRules(),
		[]omniav1alpha1.HeaderInjectionRule{
			{Header: "X-Region", CEL: "string(body.region)"},
		},
	)
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:     "process_refund",
		HeaderToolRegistry: "customer-tools",
	}
	body := map[string]interface{}{"region": "us-east-1"}

	result, err := eval.EvaluateHeaderInjection(headers, body)
	if err != nil {
		t.Fatalf("EvaluateHeaderInjection() error = %v", err)
	}
	if result["X-Region"] != "us-east-1" {
		t.Errorf("X-Region = %q, want %q", result["X-Region"], "us-east-1")
	}
}

func TestEvaluateHeaderInjection_MultipleHeaders(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicyWithHeaders("multi-inject", minimalRules(),
		[]omniav1alpha1.HeaderInjectionRule{
			{Header: "X-Static", Value: "fixed"},
			{Header: "X-Dynamic", CEL: "headers['X-Omnia-Claim-team']"},
		},
	)
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:       "process_refund",
		HeaderToolRegistry:   "customer-tools",
		"X-Omnia-Claim-team": "support",
	}

	result, err := eval.EvaluateHeaderInjection(headers, nil)
	if err != nil {
		t.Fatalf("EvaluateHeaderInjection() error = %v", err)
	}
	if result["X-Static"] != "fixed" {
		t.Errorf("X-Static = %q, want %q", result["X-Static"], "fixed")
	}
	if result["X-Dynamic"] != "support" {
		t.Errorf("X-Dynamic = %q, want %q", result["X-Dynamic"], "support")
	}
}

func TestEvaluateHeaderInjection_NoMatchingPolicy(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicyWithHeaders("no-match", minimalRules(),
		[]omniav1alpha1.HeaderInjectionRule{
			{Header: "X-Should-Not-Appear", Value: "nope"},
		},
	)
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:     "process_refund",
		HeaderToolRegistry: "other-registry",
	}

	result, err := eval.EvaluateHeaderInjection(headers, nil)
	if err != nil {
		t.Fatalf("EvaluateHeaderInjection() error = %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}

func TestEvaluateHeaderInjection_CELErrorOnFailureDeny(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	// CEL that will error at runtime (accessing missing key)
	policy := newTestPolicyWithHeaders("cel-error-deny", minimalRules(),
		[]omniav1alpha1.HeaderInjectionRule{
			{Header: "X-Fail", CEL: "int(headers['X-Missing-Key'])"},
		},
	)
	policy.Spec.OnFailure = omniav1alpha1.OnFailureDeny
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:     "process_refund",
		HeaderToolRegistry: "customer-tools",
	}

	_, err = eval.EvaluateHeaderInjection(headers, nil)
	if err == nil {
		t.Fatal("EvaluateHeaderInjection() expected error with onFailure=deny")
	}
}

func TestEvaluateHeaderInjection_CELErrorOnFailureAllow(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicyWithHeaders("cel-error-allow", minimalRules(),
		[]omniav1alpha1.HeaderInjectionRule{
			{Header: "X-Fail", CEL: "int(headers['X-Missing-Key'])"},
			{Header: "X-Static", Value: "should-still-appear"},
		},
	)
	policy.Spec.OnFailure = omniav1alpha1.OnFailureAllow
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	headers := map[string]string{
		HeaderToolName:     "process_refund",
		HeaderToolRegistry: "customer-tools",
	}

	result, err := eval.EvaluateHeaderInjection(headers, nil)
	if err != nil {
		t.Fatalf("EvaluateHeaderInjection() unexpected error with onFailure=allow: %v", err)
	}
	// The failing header should be skipped, but the static one should still be present
	if result["X-Static"] != "should-still-appear" {
		t.Errorf("X-Static = %q, want %q", result["X-Static"], "should-still-appear")
	}
	if _, ok := result["X-Fail"]; ok {
		t.Error("X-Fail should not be present after CEL error with onFailure=allow")
	}
}

func TestValidateHeaderInjectionRule(t *testing.T) {
	tests := []struct {
		name    string
		rule    omniav1alpha1.HeaderInjectionRule
		wantErr bool
	}{
		{"static value only", omniav1alpha1.HeaderInjectionRule{Header: "X-H", Value: "v"}, false},
		{"cel only", omniav1alpha1.HeaderInjectionRule{Header: "X-H", CEL: "'v'"}, false},
		{"both set", omniav1alpha1.HeaderInjectionRule{Header: "X-H", Value: "v", CEL: "'v'"}, true},
		{"neither set", omniav1alpha1.HeaderInjectionRule{Header: "X-H"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHeaderInjectionRule(tt.rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateHeaderInjectionRule() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// minimalRules returns a minimal set of policy rules for testing header injection.
func minimalRules() []omniav1alpha1.PolicyRule {
	return []omniav1alpha1.PolicyRule{
		{
			Name: "allow-all",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     "false",
				Message: "never deny",
			},
		},
	}
}

func TestEvaluate_RequiredClaimsMultiple(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	policy := newTestPolicy("multi-claims", []omniav1alpha1.PolicyRule{
		{
			Name: "no-deny",
			Deny: omniav1alpha1.PolicyRuleDeny{CEL: "false", Message: "never"},
		},
	})
	policy.Spec.RequiredClaims = []omniav1alpha1.RequiredClaim{
		{Claim: "team", Message: "team required"},
		{Claim: "region", Message: "region required"},
	}
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	t.Run("both present", func(t *testing.T) {
		headers := map[string]string{
			HeaderToolName:         "process_refund",
			HeaderToolRegistry:     "customer-tools",
			"X-Omnia-Claim-team":   "billing",
			"X-Omnia-Claim-region": "us-east",
		}
		decision := eval.Evaluate(headers, nil)
		if !decision.Allowed {
			t.Error("want allowed when both claims present")
		}
	})

	t.Run("first missing", func(t *testing.T) {
		headers := map[string]string{
			HeaderToolName:         "process_refund",
			HeaderToolRegistry:     "customer-tools",
			"X-Omnia-Claim-region": "us-east",
		}
		decision := eval.Evaluate(headers, nil)
		if decision.Allowed {
			t.Error("want denied when first claim missing")
		}
	})
}
