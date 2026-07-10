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
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/policy"
	policyctx "github.com/altairalabs/omnia/pkg/policy"
)

// --- helpers ---
//
// NOTE: this file used to also cover the reverse-proxy shape (ProxyHandler),
// which was retired in P2.4 — it never worked in production and is replaced
// by the policy-broker (see ee/pkg/policy/broker.go and the P2.3 end-to-end
// proof in test/integration/policy_broker_test.go). Only the watcher +
// header-building coverage below remains relevant.

// --- test constants ---

const (
	testRegistry  = "test-registry"
	testNamespace = "test-ns"
	testToolName  = "execute"
)

func policyHeaders() map[string]string {
	return map[string]string{
		"X-Omnia-Tool-Name":     testToolName,
		"X-Omnia-Tool-Registry": testRegistry,
	}
}

// --- tests ---

func TestPolicyWatcherEvaluatorIntegration(t *testing.T) {
	eval, err := policy.NewEvaluator()
	require.NoError(t, err)

	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	watcher := policy.NewWatcher(eval, k8sClient, scheme, testNamespace, logr.Discard())

	tp := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "watch-policy", Namespace: testNamespace},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{Registry: testRegistry},
			Rules: []omniav1alpha1.PolicyRule{{
				Name: "deny-all",
				Deny: omniav1alpha1.PolicyRuleDeny{CEL: "true", Message: "denied"},
			}},
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}

	// Add
	watcher.HandleEvent(watch.Added, tp)
	assert.Equal(t, 1, eval.PolicyCount())

	// Verify it actually denies
	decision := eval.Evaluate(policyHeaders(), nil)
	assert.False(t, decision.Allowed)

	// Modify — change to a rule that allows
	updatedTP := tp.DeepCopy()
	updatedTP.Spec.Rules[0].Deny.CEL = "false"
	watcher.HandleEvent(watch.Modified, updatedTP)
	assert.Equal(t, 1, eval.PolicyCount())

	decision = eval.Evaluate(policyHeaders(), nil)
	assert.True(t, decision.Allowed)

	// Delete
	watcher.HandleEvent(watch.Deleted, updatedTP)
	assert.Equal(t, 0, eval.PolicyCount())

	// After delete, default allow
	decision = eval.Evaluate(policyHeaders(), nil)
	assert.True(t, decision.Allowed)
}

func TestContextPropagationHeaders(t *testing.T) {
	fields := &policyctx.PropagationFields{
		AgentName: "test-agent",
		Namespace: "test-ns",
		SessionID: "sess-123",
		RequestID: "req-456",
		UserID:    "user-789",
		UserEmail: "user@example.com",
		Provider:  "openai",
		Model:     "gpt-4",
		Claims: map[string]string{
			"Team":   "platform",
			"Region": "us-east",
		},
	}

	ctx := policyctx.WithPropagationFields(context.Background(), fields)
	headers := policyctx.ToOutboundHeaders(ctx)

	// Verify standard headers
	assert.Equal(t, "test-agent", headers[policyctx.HeaderAgentName])
	assert.Equal(t, "test-ns", headers[policyctx.HeaderNamespace])
	assert.Equal(t, "sess-123", headers[policyctx.HeaderSessionID])
	assert.Equal(t, "req-456", headers[policyctx.HeaderRequestID])
	assert.Equal(t, "user-789", headers[policyctx.HeaderUserID])
	assert.Equal(t, "user@example.com", headers[policyctx.HeaderUserEmail])
	assert.Equal(t, "openai", headers[policyctx.HeaderProvider])
	assert.Equal(t, "gpt-4", headers[policyctx.HeaderModel])

	// Verify claim headers
	assert.Equal(t, "platform", headers[policyctx.HeaderClaimPrefix+"Team"])
	assert.Equal(t, "us-east", headers[policyctx.HeaderClaimPrefix+"Region"])
}
