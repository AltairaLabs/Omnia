//go:build integration

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package integration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	eepolicy "github.com/altairalabs/omnia/ee/pkg/policy"
	"github.com/altairalabs/omnia/internal/runtime/tools"
)

// TestPolicyBrokerEndToEnd_RealBrokerDeniesToolCall is the P2.3 proof that
// ToolPolicy enforcement works end-to-end through the new broker.
//
// Path chosen: FULL EXECUTOR. internal/runtime/tools exports
// NewOmniaExecutor + LoadConfigFromEntries + Initialize + ExecuteTool
// specifically so external callers can build a real executor without
// reaching into unexported fields (the same pattern internal/tooltest/tester.go
// uses). That lets this test drive the actual production chokepoint —
// OmniaExecutor.dispatch's enforcePolicy call (omnia_executor.go) — rather
// than calling PolicyBrokerClient.Decide directly, so it proves the full
// wire: dispatch -> PolicyBrokerClient.Decide (policy_broker_client.go) ->
// HTTP POST /v1/decision -> REAL ee/pkg/policy.BrokerHandler (P2.1) -> REAL
// *policy.Evaluator evaluating a REAL ToolPolicy CEL rule compiled from a
// real ToolPolicy CR. No mock broker, no mock CEL evaluation, no stubbed
// dispatch — every hop this test crosses is production code. It also
// proves the behavior that actually matters operationally: a denied call
// never reaches the upstream tool backend, and an allowed call does.
func TestPolicyBrokerEndToEnd_RealBrokerDeniesToolCall(t *testing.T) {
	// --- Real broker: real Evaluator + real ToolPolicy CEL rule ---
	eval, err := eepolicy.NewEvaluator()
	require.NoError(t, err)

	const registryName = "test-registry"
	tp := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "amount-limit", Namespace: "test-ns"},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{Registry: registryName},
			Rules: []omniav1alpha1.PolicyRule{{
				Name: "high-amount",
				Deny: omniav1alpha1.PolicyRuleDeny{
					CEL:     `has(body.amount) && double(body.amount) > 500.0`,
					Message: "Amount exceeds limit",
				},
			}},
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
	require.NoError(t, eval.CompilePolicy(tp))

	brokerLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	brokerSrv := httptest.NewServer(eepolicy.NewBrokerHandler(eval, brokerLogger))
	defer brokerSrv.Close()

	// The runtime's PolicyBrokerClient reads POLICY_BROKER_URL at
	// construction time (NewPolicyBrokerClient), so it must be set before
	// the executor (which builds the client inside NewOmniaExecutor) is
	// constructed below.
	t.Setenv("POLICY_BROKER_URL", brokerSrv.URL)

	// --- Real upstream tool backend, so we can prove it is/isn't hit ---
	toolCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		toolCalled = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer upstream.Close()

	// --- Real runtime executor, real dispatch hook ---
	executor := tools.NewOmniaExecutor(logr.Discard(), nil)
	require.NoError(t, executor.LoadConfigFromEntries([]tools.HandlerEntry{{
		Name: registryName,
		Type: tools.ToolTypeHTTP,
		HTTPConfig: &tools.HTTPCfg{
			Endpoint: upstream.URL,
			Method:   http.MethodPost,
		},
		Tool: &tools.ToolDefCfg{Name: "calculator", Description: "test calculator tool"},
	}}))
	require.NoError(t, executor.Initialize(context.Background()))
	defer func() { _ = executor.Close() }()

	t.Run("deny_amount_over_limit", func(t *testing.T) {
		toolCalled = false
		_, err := executor.ExecuteTool(context.Background(), "calculator", json.RawMessage(`{"amount":600}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "high-amount")
		assert.Contains(t, err.Error(), "Amount exceeds limit")
		assert.False(t, toolCalled, "denied tool call must never reach the upstream backend")
	})

	t.Run("allow_amount_under_limit", func(t *testing.T) {
		toolCalled = false
		result, err := executor.ExecuteTool(context.Background(), "calculator", json.RawMessage(`{"amount":100}`))
		require.NoError(t, err)
		assert.Contains(t, string(result), "ok")
		assert.True(t, toolCalled, "allowed tool call must reach the upstream backend")
	})
}
