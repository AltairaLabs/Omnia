/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"testing"
)

func newTestEvaluator(t *testing.T) *Evaluator {
	t.Helper()
	e, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error: %v", err)
	}
	return e
}

func TestNewEvaluator(t *testing.T) {
	e, err := NewEvaluator()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e == nil {
		t.Fatal("evaluator should not be nil")
	}
	if e.PolicyCount() != 0 {
		t.Errorf("expected 0 policies, got %d", e.PolicyCount())
	}
}

func TestCompileRule_Valid(t *testing.T) {
	e := newTestEvaluator(t)
	rule, err := e.CompileRule("test", "headers['X-Amount'] == '100'", "denied")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.Name != "test" {
		t.Errorf("expected name 'test', got %q", rule.Name)
	}
	if rule.Message != "denied" {
		t.Errorf("expected message 'denied', got %q", rule.Message)
	}
}

func TestCompileRule_InvalidSyntax(t *testing.T) {
	e := newTestEvaluator(t)
	_, err := e.CompileRule("bad", "invalid @@@ expression", "msg")
	if err == nil {
		t.Fatal("expected compilation error")
	}
}

func TestCompileRule_NonBoolReturn(t *testing.T) {
	e := newTestEvaluator(t)
	_, err := e.CompileRule("str-return", "headers['X-Name']", "msg")
	if err == nil {
		t.Fatal("expected error for non-bool return type")
	}
}

func TestSetPolicy_Success(t *testing.T) {
	e := newTestEvaluator(t)
	err := e.SetPolicy("p1", "ns1", "registry1", []string{"tool1"}, []RuleInput{
		{Name: "r1", CEL: "headers['X-Amount'] == '100'", Message: "amount is 100"},
	}, "enforce", "deny")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.PolicyCount() != 1 {
		t.Errorf("expected 1 policy, got %d", e.PolicyCount())
	}
}

func TestSetPolicy_CompilationError(t *testing.T) {
	e := newTestEvaluator(t)
	err := e.SetPolicy("p1", "ns1", "registry1", nil, []RuleInput{
		{Name: "bad", CEL: "!@#$%", Message: "msg"},
	}, "enforce", "deny")
	if err == nil {
		t.Fatal("expected compilation error")
	}
	if e.PolicyCount() != 0 {
		t.Errorf("expected 0 policies after error, got %d", e.PolicyCount())
	}
}

func TestRemovePolicy(t *testing.T) {
	e := newTestEvaluator(t)
	_ = e.SetPolicy("p1", "ns1", "reg", nil, []RuleInput{
		{Name: "r1", CEL: "true", Message: "always deny"},
	}, "enforce", "deny")
	if e.PolicyCount() != 1 {
		t.Fatalf("expected 1 policy, got %d", e.PolicyCount())
	}
	e.RemovePolicy("ns1", "p1")
	if e.PolicyCount() != 0 {
		t.Errorf("expected 0 policies after removal, got %d", e.PolicyCount())
	}
}

func TestEvaluate_DenyRule(t *testing.T) {
	e := newTestEvaluator(t)
	_ = e.SetPolicy("limit-policy", "default", "tools", []string{"refund"}, []RuleInput{
		{Name: "amount-limit", CEL: "int(headers['X-Amount']) > 1000", Message: "Amount exceeds $1000"},
	}, "enforce", "deny")

	ctx := &EvalContext{
		Headers: map[string]string{"X-Amount": "2000"},
	}
	results := e.Evaluate("tools", "refund", ctx)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Denied {
		t.Error("expected request to be denied")
	}
	if results[0].DenyMessage != "Amount exceeds $1000" {
		t.Errorf("unexpected message: %q", results[0].DenyMessage)
	}
}

func TestEvaluate_AllowRule(t *testing.T) {
	e := newTestEvaluator(t)
	_ = e.SetPolicy("limit-policy", "default", "tools", []string{"refund"}, []RuleInput{
		{Name: "amount-limit", CEL: "int(headers['X-Amount']) > 1000", Message: "Amount exceeds $1000"},
	}, "enforce", "deny")

	ctx := &EvalContext{
		Headers: map[string]string{"X-Amount": "500"},
	}
	results := e.Evaluate("tools", "refund", ctx)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Denied {
		t.Error("expected request to be allowed")
	}
}

func TestEvaluate_MultipleRules(t *testing.T) {
	e := newTestEvaluator(t)
	_ = e.SetPolicy("multi", "default", "tools", []string{"transfer"}, []RuleInput{
		{Name: "rule1", CEL: "int(headers['X-Amount']) > 500", Message: "Over 500"},
		{Name: "rule2", CEL: "headers['X-Role'] == 'intern'", Message: "Interns not allowed"},
	}, "enforce", "deny")

	ctx := &EvalContext{
		Headers: map[string]string{"X-Amount": "100", "X-Role": "intern"},
	}
	results := e.Evaluate("tools", "transfer", ctx)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if !r.Denied {
		t.Error("expected denial")
	}
	if r.DenyMessage != "Interns not allowed" {
		t.Errorf("unexpected deny message: %q", r.DenyMessage)
	}
	if len(r.Decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(r.Decisions))
	}
	if r.Decisions[0].Denied {
		t.Error("rule1 should not deny (amount 100 <= 500)")
	}
	if !r.Decisions[1].Denied {
		t.Error("rule2 should deny (role is intern)")
	}
}

func TestEvaluate_NoMatchingPolicy(t *testing.T) {
	e := newTestEvaluator(t)
	_ = e.SetPolicy("p", "ns", "other-registry", nil, []RuleInput{
		{Name: "r", CEL: "true", Message: "blocked"},
	}, "enforce", "deny")

	results := e.Evaluate("tools", "any-tool", &EvalContext{})
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-matching registry, got %d", len(results))
	}
}

func TestEvaluate_ToolNotInList(t *testing.T) {
	e := newTestEvaluator(t)
	_ = e.SetPolicy("p", "ns", "tools", []string{"refund", "credit"}, []RuleInput{
		{Name: "r", CEL: "true", Message: "blocked"},
	}, "enforce", "deny")

	results := e.Evaluate("tools", "transfer", &EvalContext{})
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-matching tool, got %d", len(results))
	}
}

func TestEvaluate_EmptyToolsMatchesAll(t *testing.T) {
	e := newTestEvaluator(t)
	_ = e.SetPolicy("p", "ns", "tools", nil, []RuleInput{
		{Name: "r", CEL: "true", Message: "blocked"},
	}, "enforce", "deny")

	results := e.Evaluate("tools", "any-tool", &EvalContext{})
	if len(results) != 1 {
		t.Fatalf("expected 1 result (empty tools matches all), got %d", len(results))
	}
	if !results[0].Denied {
		t.Error("expected denial")
	}
}

func TestEvaluate_NilHeaders(t *testing.T) {
	e := newTestEvaluator(t)
	_ = e.SetPolicy("p", "ns", "tools", nil, []RuleInput{
		{Name: "r", CEL: "size(headers) == 0", Message: "no headers"},
	}, "enforce", "deny")

	results := e.Evaluate("tools", "tool1", &EvalContext{})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Denied {
		t.Error("expected denial for empty headers")
	}
}

func TestEvaluate_BodyVariable(t *testing.T) {
	e := newTestEvaluator(t)
	_ = e.SetPolicy("p", "ns", "tools", nil, []RuleInput{
		{Name: "r", CEL: "body['nested'] == 'value'", Message: "body matched"},
	}, "enforce", "deny")

	ctx := &EvalContext{
		Body: map[string]interface{}{"nested": "value"},
	}
	results := e.Evaluate("tools", "tool1", ctx)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Denied {
		t.Error("expected denial when body matches")
	}
}

func TestEvaluate_RuntimeError_DeniesRequest(t *testing.T) {
	e := newTestEvaluator(t)
	// This will cause a runtime error: missing header key with int() conversion
	_ = e.SetPolicy("p", "ns", "tools", nil, []RuleInput{
		{Name: "r", CEL: "int(headers['nonexistent']) > 100", Message: "limit exceeded"},
	}, "enforce", "deny")

	results := e.Evaluate("tools", "tool1", &EvalContext{})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// Runtime errors should result in denial for safety.
	if !results[0].Denied {
		t.Error("expected denial on runtime error")
	}
}

func TestEvaluate_MultiplePolicies(t *testing.T) {
	e := newTestEvaluator(t)
	_ = e.SetPolicy("p1", "ns", "tools", []string{"refund"}, []RuleInput{
		{Name: "r1", CEL: "false", Message: "never denies"},
	}, "enforce", "deny")
	_ = e.SetPolicy("p2", "ns2", "tools", []string{"refund"}, []RuleInput{
		{Name: "r2", CEL: "true", Message: "always denies"},
	}, "enforce", "deny")

	results := e.Evaluate("tools", "refund", &EvalContext{})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestValidateCEL_Valid(t *testing.T) {
	e := newTestEvaluator(t)
	err := e.ValidateCEL("headers['X-Test'] == 'value'")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateCEL_InvalidSyntax(t *testing.T) {
	e := newTestEvaluator(t)
	err := e.ValidateCEL("!@#invalid")
	if err == nil {
		t.Error("expected validation error for invalid syntax")
	}
}

func TestValidateCEL_NonBoolReturn(t *testing.T) {
	e := newTestEvaluator(t)
	err := e.ValidateCEL("headers['X-Test']")
	if err == nil {
		t.Error("expected validation error for non-bool return")
	}
}

func TestMatchesTool(t *testing.T) {
	tests := []struct {
		name   string
		tools  []string
		tool   string
		expect bool
	}{
		{"empty matches all", nil, "any", true},
		{"exact match", []string{"a", "b"}, "b", true},
		{"no match", []string{"a", "b"}, "c", false},
		{"single match", []string{"x"}, "x", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesTool(tt.tools, tt.tool)
			if got != tt.expect {
				t.Errorf("matchesTool(%v, %q) = %v, want %v", tt.tools, tt.tool, got, tt.expect)
			}
		})
	}
}

func TestPolicyKey(t *testing.T) {
	if got := policyKey("ns", "name"); got != "ns/name" {
		t.Errorf("policyKey = %q, want %q", got, "ns/name")
	}
}

func TestEvaluate_ComplexCEL_RoleSplit(t *testing.T) {
	e := newTestEvaluator(t)
	celExpr := `int(headers['X-Amount']) > 1000 && !('supervisor' in headers['X-Roles'].split(','))`
	err := e.SetPolicy("refund-guard", "prod", "tools", []string{"process_refund"}, []RuleInput{
		{Name: "refund-limit", CEL: celExpr, Message: "Refunds over $1000 require supervisor role"},
	}, "enforce", "deny")
	if err != nil {
		t.Fatalf("SetPolicy error: %v", err)
	}

	// Supervisor with high amount - should be allowed
	results := e.Evaluate("tools", "process_refund", &EvalContext{
		Headers: map[string]string{"X-Amount": "2000", "X-Roles": "agent,supervisor"},
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Denied {
		t.Error("supervisor should be allowed for high amounts")
	}

	// Non-supervisor with high amount - should be denied
	results = e.Evaluate("tools", "process_refund", &EvalContext{
		Headers: map[string]string{"X-Amount": "2000", "X-Roles": "agent,basic"},
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Denied {
		t.Error("non-supervisor should be denied for high amounts")
	}

	// Non-supervisor with low amount - should be allowed
	results = e.Evaluate("tools", "process_refund", &EvalContext{
		Headers: map[string]string{"X-Amount": "500", "X-Roles": "agent,basic"},
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Denied {
		t.Error("low amounts should be allowed regardless of role")
	}
}

func TestEvaluate_SetPolicyOverwrite(t *testing.T) {
	e := newTestEvaluator(t)
	_ = e.SetPolicy("p", "ns", "reg", nil, []RuleInput{
		{Name: "r", CEL: "true", Message: "v1"},
	}, "enforce", "deny")

	// Overwrite with new version
	_ = e.SetPolicy("p", "ns", "reg", nil, []RuleInput{
		{Name: "r", CEL: "false", Message: "v2"},
	}, "enforce", "deny")

	if e.PolicyCount() != 1 {
		t.Errorf("expected 1 policy after overwrite, got %d", e.PolicyCount())
	}

	results := e.Evaluate("reg", "any", &EvalContext{})
	if results[0].Denied {
		t.Error("overwritten policy should not deny (expression is false)")
	}
}

func TestIsTruthy(t *testing.T) {
	if isTruthy(nil) {
		t.Error("nil should not be truthy")
	}
}
