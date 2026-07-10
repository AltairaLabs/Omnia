/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// testBrokerLogger returns a discarding logr.Logger for broker tests that
// don't assert on log output. Distinct from proxy_test.go's testLogger()
// (*slog.Logger) since BrokerHandler moved to logr while ProxyHandler (the
// dead reverse-proxy shape, retired in P2.4) stays on slog.
func testBrokerLogger() logr.Logger {
	return logr.Discard()
}

// newCapturingLogger returns a logr.Logger backed by a Zap JSON core whose
// output is captured in the returned buffer, for tests that assert on
// structured log fields.
func newCapturingLogger() (logr.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(encoder, zapcore.AddSync(buf), zapcore.DebugLevel)
	return zapr.NewLogger(zap.New(core)), buf
}

// newDecisionRequest builds an httptest.Request carrying a JSON-encoded
// DecisionRequest to POST /v1/decision.
func newDecisionRequest(t *testing.T, req DecisionRequest) *http.Request {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return httptest.NewRequest(http.MethodPost, "/v1/decision", bytes.NewReader(body))
}

// decodeDecisionResponse decodes the recorder body into a DecisionResponse.
func decodeDecisionResponse(t *testing.T, rec *httptest.ResponseRecorder) DecisionResponse {
	t.Helper()
	var resp DecisionResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode DecisionResponse: %v", err)
	}
	return resp
}

func TestBrokerHandler_Deny(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	toolPolicy := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "block-all-policy", Namespace: "default"},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "test-registry",
				Tools:    []string{"blocked_tool"},
			},
			Rules: []omniav1alpha1.PolicyRule{
				{
					Name: "block-all",
					Deny: omniav1alpha1.PolicyRuleDeny{
						CEL:     "true",
						Message: "all requests blocked",
					},
				},
			},
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
	if err := eval.CompilePolicy(toolPolicy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	handler := NewBrokerHandler(eval, testBrokerLogger())

	req := newDecisionRequest(t, DecisionRequest{
		Headers: map[string]string{
			HeaderToolName:     "blocked_tool",
			HeaderToolRegistry: "test-registry",
		},
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	resp := decodeDecisionResponse(t, rec)
	if resp.Allow {
		t.Error("Allow = true, want false")
	}
	if resp.DeniedBy != "block-all" {
		t.Errorf("DeniedBy = %q, want %q", resp.DeniedBy, "block-all")
	}
	if resp.Message != "all requests blocked" {
		t.Errorf("Message = %q, want %q", resp.Message, "all requests blocked")
	}
	if resp.Mode != string(omniav1alpha1.PolicyModeEnforce) {
		t.Errorf("Mode = %q, want %q", resp.Mode, omniav1alpha1.PolicyModeEnforce)
	}
	if resp.WouldDeny {
		t.Error("WouldDeny = true, want false in enforce mode")
	}
}

func TestBrokerHandler_Allow(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	toolPolicy := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "amount-policy", Namespace: "default"},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "test-registry",
				Tools:    []string{"allowed_tool"},
			},
			Rules: []omniav1alpha1.PolicyRule{
				{
					Name: "amount-check",
					Deny: omniav1alpha1.PolicyRuleDeny{
						CEL:     "int(headers['X-Omnia-Param-Amount']) > 1000",
						Message: "amount too high",
					},
				},
			},
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
	if err := eval.CompilePolicy(toolPolicy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	handler := NewBrokerHandler(eval, testBrokerLogger())

	req := newDecisionRequest(t, DecisionRequest{
		Headers: map[string]string{
			HeaderToolName:         "allowed_tool",
			HeaderToolRegistry:     "test-registry",
			"X-Omnia-Param-Amount": "500",
		},
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	resp := decodeDecisionResponse(t, rec)
	if !resp.Allow {
		t.Error("Allow = false, want true")
	}
	if resp.DeniedBy != "" {
		t.Errorf("DeniedBy = %q, want empty", resp.DeniedBy)
	}
}

func TestBrokerHandler_AuditMode(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	toolPolicy := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "audit-policy", Namespace: "default"},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "test-registry",
				Tools:    []string{"audited_tool"},
			},
			Rules: []omniav1alpha1.PolicyRule{
				{
					Name: "would-block",
					Deny: omniav1alpha1.PolicyRuleDeny{
						CEL:     "true",
						Message: "would have been blocked",
					},
				},
			},
			Mode:      omniav1alpha1.PolicyModeAudit,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
	if err := eval.CompilePolicy(toolPolicy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	handler := NewBrokerHandler(eval, testBrokerLogger())

	req := newDecisionRequest(t, DecisionRequest{
		Headers: map[string]string{
			HeaderToolName:     "audited_tool",
			HeaderToolRegistry: "test-registry",
		},
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := decodeDecisionResponse(t, rec)
	if !resp.Allow {
		t.Error("Allow = false, want true in audit mode")
	}
	if !resp.WouldDeny {
		t.Error("WouldDeny = false, want true in audit mode when a rule matches")
	}
	if resp.DeniedBy != "would-block" {
		t.Errorf("DeniedBy = %q, want %q", resp.DeniedBy, "would-block")
	}
	if resp.Mode != string(omniav1alpha1.PolicyModeAudit) {
		t.Errorf("Mode = %q, want %q", resp.Mode, omniav1alpha1.PolicyModeAudit)
	}
}

func TestBrokerHandler_HeaderInjection(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	toolPolicy := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "inject-policy", Namespace: "default"},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "test-registry",
			},
			Rules: []omniav1alpha1.PolicyRule{
				{
					Name: "allow-all",
					Deny: omniav1alpha1.PolicyRuleDeny{
						CEL:     "false",
						Message: "never deny",
					},
				},
			},
			HeaderInjection: []omniav1alpha1.HeaderInjectionRule{
				{Header: "X-Injected-Static", Value: "injected-value"},
				{Header: "X-Injected-Dynamic", CEL: "headers['X-Omnia-Claim-Team']"},
			},
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
	if err := eval.CompilePolicy(toolPolicy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	handler := NewBrokerHandler(eval, testBrokerLogger())

	req := newDecisionRequest(t, DecisionRequest{
		Headers: map[string]string{
			HeaderToolName:       "some_tool",
			HeaderToolRegistry:   "test-registry",
			"X-Omnia-Claim-Team": "engineering",
		},
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := decodeDecisionResponse(t, rec)
	if !resp.Allow {
		t.Fatal("Allow = false, want true")
	}
	if resp.InjectedHeaders["X-Injected-Static"] != "injected-value" {
		t.Errorf("X-Injected-Static = %q, want %q", resp.InjectedHeaders["X-Injected-Static"], "injected-value")
	}
	if resp.InjectedHeaders["X-Injected-Dynamic"] != "engineering" {
		t.Errorf("X-Injected-Dynamic = %q, want %q", resp.InjectedHeaders["X-Injected-Dynamic"], "engineering")
	}
}

func TestBrokerHandler_MalformedJSON(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}
	handler := NewBrokerHandler(eval, testBrokerLogger())

	req := httptest.NewRequest(http.MethodPost, "/v1/decision", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestBrokerHandler_IdentityRoleGate(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	toolPolicy := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "role-gate-policy", Namespace: "default"},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "test-registry",
				Tools:    []string{"admin_tool"},
			},
			Rules: []omniav1alpha1.PolicyRule{
				{
					Name: "role-gate",
					Deny: omniav1alpha1.PolicyRuleDeny{
						CEL:     "identity.claims.role != 'admin'",
						Message: "admin role required",
					},
				},
			},
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
	if err := eval.CompilePolicy(toolPolicy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	handler := NewBrokerHandler(eval, testBrokerLogger())

	headers := map[string]string{
		HeaderToolName:     "admin_tool",
		HeaderToolRegistry: "test-registry",
	}

	t.Run("viewer role denied", func(t *testing.T) {
		req := newDecisionRequest(t, DecisionRequest{
			Headers:  headers,
			Identity: &IdentityPayload{Claims: map[string]string{"role": "viewer"}},
		})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		resp := decodeDecisionResponse(t, rec)
		if resp.Allow {
			t.Error("Allow = true, want false for viewer role")
		}
		if resp.DeniedBy != "role-gate" {
			t.Errorf("DeniedBy = %q, want %q", resp.DeniedBy, "role-gate")
		}
	})

	t.Run("admin role allowed", func(t *testing.T) {
		req := newDecisionRequest(t, DecisionRequest{
			Headers:  headers,
			Identity: &IdentityPayload{Claims: map[string]string{"role": "admin"}},
		})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		resp := decodeDecisionResponse(t, rec)
		if !resp.Allow {
			t.Error("Allow = false, want true for admin role")
		}
	})

	t.Run("no identity attached denied (zero-value role)", func(t *testing.T) {
		req := newDecisionRequest(t, DecisionRequest{Headers: headers})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		resp := decodeDecisionResponse(t, rec)
		if resp.Allow {
			t.Error("Allow = true, want false when no identity is attached (role defaults to empty)")
		}
	})
}

// TestBrokerHandler_IdentityWorkspaceGate is the broker-side end-to-end proof
// for #1769: a ToolPolicy CEL rule keyed on identity.workspace must actually
// evaluate against the propagated workspace. The broker rebuilds an
// AuthenticatedIdentity from DecisionRequest.Identity (the IdentityPayload the
// runtime posts) and exposes it to CEL as identity.workspace. Before #1769 the
// workspace never crossed the facade->runtime hop, so this rule silently
// no-oped (workspace was always "").
func TestBrokerHandler_IdentityWorkspaceGate(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	toolPolicy := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "workspace-gate-policy", Namespace: "default"},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "test-registry",
				Tools:    []string{"scoped_tool"},
			},
			Rules: []omniav1alpha1.PolicyRule{
				{
					Name: "workspace-gate",
					Deny: omniav1alpha1.PolicyRuleDeny{
						CEL:     "identity.workspace != 'acme'",
						Message: "acme workspace required",
					},
				},
			},
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
	if err := eval.CompilePolicy(toolPolicy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	handler := NewBrokerHandler(eval, testBrokerLogger())
	headers := map[string]string{
		HeaderToolName:     "scoped_tool",
		HeaderToolRegistry: "test-registry",
	}

	t.Run("matching workspace allowed", func(t *testing.T) {
		req := newDecisionRequest(t, DecisionRequest{
			Headers:  headers,
			Identity: &IdentityPayload{Origin: "api-key", Workspace: "acme"},
		})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		resp := decodeDecisionResponse(t, rec)
		if !resp.Allow {
			t.Errorf("Allow = false, want true for identity.workspace == 'acme' (deniedBy=%q)", resp.DeniedBy)
		}
	})

	t.Run("wrong workspace denied", func(t *testing.T) {
		req := newDecisionRequest(t, DecisionRequest{
			Headers:  headers,
			Identity: &IdentityPayload{Origin: "api-key", Workspace: "other"},
		})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		resp := decodeDecisionResponse(t, rec)
		if resp.Allow {
			t.Error("Allow = true, want false for identity.workspace == 'other'")
		}
		if resp.DeniedBy != "workspace-gate" {
			t.Errorf("DeniedBy = %q, want %q", resp.DeniedBy, "workspace-gate")
		}
	})

	t.Run("empty workspace denied (regression: pre-#1769 always-empty state)", func(t *testing.T) {
		req := newDecisionRequest(t, DecisionRequest{Headers: headers})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		resp := decodeDecisionResponse(t, rec)
		if resp.Allow {
			t.Error("Allow = true, want false when identity.workspace is empty")
		}
	})
}

func TestBrokerHandler_NoMatchingPolicyAllows(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}
	handler := NewBrokerHandler(eval, testBrokerLogger())

	req := newDecisionRequest(t, DecisionRequest{
		Headers: map[string]string{
			HeaderToolName:     "any_tool",
			HeaderToolRegistry: "unmatched-registry",
		},
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeDecisionResponse(t, rec)
	if !resp.Allow {
		t.Error("Allow = false, want true for no matching policy")
	}
}

// TestBrokerHandler_NoInjectionOnDeny asserts that a denied decision never
// computes or returns injected headers, mirroring the old proxy's
// short-circuit (ProxyHandler.ServeHTTP only calls injectHeaders after
// checking decision.Allowed).
func TestBrokerHandler_NoInjectionOnDeny(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	toolPolicy := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "deny-with-injection-policy", Namespace: "default"},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "test-registry",
				Tools:    []string{"blocked_tool"},
			},
			Rules: []omniav1alpha1.PolicyRule{
				{
					Name: "block-all",
					Deny: omniav1alpha1.PolicyRuleDeny{
						CEL:     "true",
						Message: "all requests blocked",
					},
				},
			},
			HeaderInjection: []omniav1alpha1.HeaderInjectionRule{
				{Header: "X-Should-Not-Appear", Value: "should-not-appear"},
			},
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
	if err := eval.CompilePolicy(toolPolicy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	handler := NewBrokerHandler(eval, testBrokerLogger())

	req := newDecisionRequest(t, DecisionRequest{
		Headers: map[string]string{
			HeaderToolName:     "blocked_tool",
			HeaderToolRegistry: "test-registry",
		},
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := decodeDecisionResponse(t, rec)
	if resp.Allow {
		t.Fatal("Allow = true, want false")
	}
	if len(resp.InjectedHeaders) != 0 {
		t.Errorf("InjectedHeaders = %v, want empty on deny", resp.InjectedHeaders)
	}
}

// TestBrokerHandler_AuditLogIncludesToolIdentity asserts that the broker's
// decision log carries the tool name and registry as explicit fields.
// logDecision alone logs r.URL.Path/r.Method, which for every broker call
// are always "/v1/decision"/"POST" — useless for identifying which tool was
// evaluated in an audit trail.
func TestBrokerHandler_AuditLogIncludesToolIdentity(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	toolPolicy := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "block-all-policy", Namespace: "default"},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "test-registry",
				Tools:    []string{"blocked_tool"},
			},
			Rules: []omniav1alpha1.PolicyRule{
				{
					Name: "block-all",
					Deny: omniav1alpha1.PolicyRuleDeny{
						CEL:     "true",
						Message: "all requests blocked",
					},
				},
			},
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
	if err := eval.CompilePolicy(toolPolicy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	logger, buf := newCapturingLogger()
	handler := NewBrokerHandler(eval, logger)

	req := newDecisionRequest(t, DecisionRequest{
		Headers: map[string]string{
			HeaderToolName:     "blocked_tool",
			HeaderToolRegistry: "test-registry",
		},
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	logOutput := buf.String()
	if !strings.Contains(logOutput, `"toolName":"blocked_tool"`) {
		t.Errorf("log output missing toolName field: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"toolRegistry":"test-registry"`) {
		t.Errorf("log output missing toolRegistry field: %s", logOutput)
	}
}

// TestBrokerHandler_HeaderInjectionErrorDoesNotAffectDecision exercises the
// EvaluateHeaderInjectionWithContext error path (a CEL expression that
// errors at eval time — accessing a missing header key — under
// OnFailure=Deny, which propagates the error rather than swallowing it).
// It asserts the overall decision is unaffected: still 200, Allow from the
// unrelated deny rules, injectedHeaders empty.
func TestBrokerHandler_HeaderInjectionErrorDoesNotAffectDecision(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	toolPolicy := newTestPolicyWithHeaders("cel-error-broker", minimalRules(),
		[]omniav1alpha1.HeaderInjectionRule{
			{Header: "X-Fail", CEL: "int(headers['X-Missing-Key'])"},
		},
	)
	toolPolicy.Spec.OnFailure = omniav1alpha1.OnFailureDeny
	if err := eval.CompilePolicy(toolPolicy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	handler := NewBrokerHandler(eval, testBrokerLogger())

	req := newDecisionRequest(t, DecisionRequest{
		Headers: map[string]string{
			HeaderToolName:     "process_refund",
			HeaderToolRegistry: "customer-tools",
		},
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeDecisionResponse(t, rec)
	if !resp.Allow {
		t.Error("Allow = false, want true — header injection error must not affect the decision")
	}
	if len(resp.InjectedHeaders) != 0 {
		t.Errorf("InjectedHeaders = %v, want empty when header injection eval errors", resp.InjectedHeaders)
	}
}

// TestBrokerHandler_OversizedBodyRejected asserts that a request body over
// the MaxBytesReader cap is rejected as a malformed request (400) rather
// than being read into memory unbounded.
func TestBrokerHandler_OversizedBodyRejected(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}
	handler := NewBrokerHandler(eval, testBrokerLogger())

	oversizedValue := strings.Repeat("a", maxDecisionRequestBytes+1)
	body := `{"headers":{"` + HeaderToolName + `":"` + oversizedValue + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/decision", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestBrokerHandler_RejectsNonPostMethods asserts /v1/decision is
// POST-only: any other method gets 405, not a 400 from a body-less decode
// failure.
func TestBrokerHandler_RejectsNonPostMethods(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}
	handler := NewBrokerHandler(eval, testBrokerLogger())

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/v1/decision", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}
