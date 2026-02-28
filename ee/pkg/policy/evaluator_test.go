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
