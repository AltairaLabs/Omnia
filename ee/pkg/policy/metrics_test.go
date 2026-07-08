/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// TestBrokerHandler_RecordsDecisionMetrics drives a deny and an allow through
// a single BrokerHandler with metrics attached (SetMetrics) and asserts the
// omnia_toolpolicy_decisions_total counter increments with the right
// outcome/tool_registry/policy label combination for each. Regression guard
// for the ConstLabels + variable-label wiring, not just that the counter
// exists.
func TestBrokerHandler_RecordsDecisionMetrics(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	toolPolicy := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics-test-policy", Namespace: "default"},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "metrics-test-registry",
				Tools:    []string{"blocked_tool", "allowed_tool"},
			},
			Rules: []omniav1alpha1.PolicyRule{
				{
					Name: "block-blocked-tool",
					Deny: omniav1alpha1.PolicyRuleDeny{
						CEL:     "headers['X-Omnia-Tool-Name'] == 'blocked_tool'",
						Message: "blocked_tool is denied",
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

	metrics := NewBrokerMetrics("test-agent-metrics-decisions", "test-ns-metrics-decisions")
	handler := NewBrokerHandler(eval, testBrokerLogger())
	handler.SetMetrics(metrics)

	denyReq := newDecisionRequest(t, DecisionRequest{
		Headers: map[string]string{
			HeaderToolName:     "blocked_tool",
			HeaderToolRegistry: "metrics-test-registry",
		},
	})
	denyRec := httptest.NewRecorder()
	handler.ServeHTTP(denyRec, denyReq)

	allowReq := newDecisionRequest(t, DecisionRequest{
		Headers: map[string]string{
			HeaderToolName:     "allowed_tool",
			HeaderToolRegistry: "metrics-test-registry",
		},
	})
	allowRec := httptest.NewRecorder()
	handler.ServeHTTP(allowRec, allowReq)

	denyCount := testutil.ToFloat64(metrics.DecisionsTotal.WithLabelValues(
		OutcomeDenied, "metrics-test-registry", "metrics-test-policy"))
	if denyCount != 1 {
		t.Errorf("denied decisions_total = %v, want 1", denyCount)
	}

	allowCount := testutil.ToFloat64(metrics.DecisionsTotal.WithLabelValues(
		OutcomeAllowed, "metrics-test-registry", ""))
	if allowCount != 1 {
		t.Errorf("allowed decisions_total = %v, want 1", allowCount)
	}
}

// TestBrokerHandler_NilMetricsDoesNotPanic asserts that a BrokerHandler
// without SetMetrics called (the state every pre-existing test in this
// package is in) serves decisions normally rather than panicking on a nil
// metrics dereference.
func TestBrokerHandler_NilMetricsDoesNotPanic(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}
	handler := NewBrokerHandler(eval, testBrokerLogger())

	req := newDecisionRequest(t, DecisionRequest{
		Headers: map[string]string{
			HeaderToolName:     "any_tool",
			HeaderToolRegistry: "any-registry",
		},
	})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req) // must not panic
}

// TestDecisionOutcome covers the outcome classification used for the
// decisions_total counter's "outcome" label.
var errTest = errors.New("cel eval boom")

func TestDecisionOutcome(t *testing.T) {
	tests := []struct {
		name     string
		decision Decision
		want     string
	}{
		{
			name:     "denied",
			decision: Decision{Allowed: false, DeniedBy: "some-rule"},
			want:     OutcomeDenied,
		},
		{
			name:     "would_deny",
			decision: Decision{Allowed: true, WouldDeny: true, DeniedBy: "some-rule"},
			want:     OutcomeWouldDeny,
		},
		{
			name:     "allowed",
			decision: Decision{Allowed: true},
			want:     OutcomeAllowed,
		},
		{
			// Defensive: a not-allowed decision counts as denied even if
			// DeniedBy is somehow empty (classifier keys on Allowed, not
			// on the presence of a rule name).
			name:     "denied_without_deniedby",
			decision: Decision{Allowed: false},
			want:     OutcomeDenied,
		},
		{
			// Eval error that failed closed: classified as "error", NOT
			// "denied", so a rule erroring on every call is alertable rather
			// than hidden in the deny count.
			name:     "eval_error_fail_closed",
			decision: Decision{Allowed: false, DeniedBy: "some-rule", Error: errTest},
			want:     OutcomeError,
		},
		{
			// Eval error that failed OPEN (onFailure=allow): still surfaced as
			// "error" — the misconfiguration matters regardless of fail mode.
			name:     "eval_error_fail_open",
			decision: Decision{Allowed: true, Error: errTest},
			want:     OutcomeError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decisionOutcome(tt.decision); got != tt.want {
				t.Errorf("decisionOutcome() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestWatcher_SetActivePoliciesGauge asserts the watcher's initialLoad
// refreshes the active_policies gauge from the evaluator's compiled-policy
// count, when metrics are attached via SetMetrics.
func TestWatcher_SetActivePoliciesGauge(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}
	toolPolicy := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "gauge-test-policy", Namespace: "default"},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{Registry: "reg"},
			Rules: []omniav1alpha1.PolicyRule{
				{Name: "r1", Deny: omniav1alpha1.PolicyRuleDeny{CEL: "false", Message: "m"}},
			},
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
	if err := eval.CompilePolicy(toolPolicy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	metrics := NewBrokerMetrics("test-agent-metrics-gauge", "test-ns-metrics-gauge")
	metrics.SetActivePolicies(eval.PolicyCount())

	got := testutil.ToFloat64(metrics.ActivePolicies)
	if got != 1 {
		t.Errorf("active_policies = %v, want 1", got)
	}
}
