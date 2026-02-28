//go:build integration

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

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

func compileTestPolicy(
	t *testing.T,
	eval *policy.Evaluator,
	name, namespace, registry string,
	tools []string,
	rules []omniav1alpha1.PolicyRule,
	mode omniav1alpha1.PolicyMode,
) *omniav1alpha1.ToolPolicy {
	t.Helper()
	tp := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: registry,
				Tools:    tools,
			},
			Rules:     rules,
			Mode:      mode,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
	require.NoError(t, eval.CompilePolicy(tp))
	return tp
}

func setupProxyWithUpstream(
	t *testing.T,
	eval *policy.Evaluator,
	logger *slog.Logger,
) (proxyURL string, upstream *httptest.Server, cleanup func()) {
	t.Helper()
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo back the received headers and body as JSON.
		hdrs := make(map[string]string)
		for k := range r.Header {
			hdrs[k] = r.Header.Get(k)
		}
		var body map[string]interface{}
		if r.Body != nil {
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, &body)
		}
		resp := map[string]interface{}{"headers": hdrs, "body": body}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))

	upstreamURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)

	handler := policy.NewProxyHandler(eval, upstreamURL, logger)
	proxy := httptest.NewServer(handler)

	return proxy.URL, upstream, func() {
		proxy.Close()
		upstream.Close()
	}
}

func makeJSONRequest(t *testing.T, proxyURL string, headers map[string]string, body interface{}) *http.Response {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		reqBody = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, proxyURL, reqBody)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

type upstreamEcho struct {
	Headers map[string]string      `json:"headers"`
	Body    map[string]interface{} `json:"body"`
}

func readUpstreamEcho(t *testing.T, resp *http.Response) upstreamEcho {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	var echo upstreamEcho
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&echo))
	return echo
}

func readDenialResponse(t *testing.T, resp *http.Response) policy.DenialResponse {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	var denial policy.DenialResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&denial))
	return denial
}

func captureLogs(handler func(*slog.Logger)) *bytes.Buffer {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	handler(logger)
	return buf
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

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

func TestPolicyProxyEndToEnd(t *testing.T) {
	eval, err := policy.NewEvaluator()
	require.NoError(t, err)

	compileTestPolicy(t, eval, "amount-limit", testNamespace, testRegistry, nil,
		[]omniav1alpha1.PolicyRule{{
			Name: "high-amount",
			Deny: omniav1alpha1.PolicyRuleDeny{
				CEL:     `has(body.amount) && double(body.amount) > 500.0`,
				Message: "Amount exceeds limit",
			},
		}},
		omniav1alpha1.PolicyModeEnforce,
	)

	proxyURL, _, cleanup := setupProxyWithUpstream(t, eval, discardLogger())
	defer cleanup()

	t.Run("allowed_request", func(t *testing.T) {
		resp := makeJSONRequest(t, proxyURL, policyHeaders(), map[string]interface{}{"amount": 100})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		echo := readUpstreamEcho(t, resp)
		assert.NotNil(t, echo.Headers)
	})

	t.Run("denied_request", func(t *testing.T) {
		resp := makeJSONRequest(t, proxyURL, policyHeaders(), map[string]interface{}{"amount": 600})
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		denial := readDenialResponse(t, resp)
		assert.Equal(t, "policy_denied", denial.Error)
		assert.Equal(t, "high-amount", denial.Rule)
		assert.Equal(t, "Amount exceeds limit", denial.Message)
	})
}

func TestPolicyProxyAuditMode(t *testing.T) {
	var logBuf *bytes.Buffer

	logBuf = captureLogs(func(logger *slog.Logger) {
		eval, err := policy.NewEvaluator()
		require.NoError(t, err)

		compileTestPolicy(t, eval, "audit-policy", testNamespace, testRegistry, nil,
			[]omniav1alpha1.PolicyRule{{
				Name: "block-all",
				Deny: omniav1alpha1.PolicyRuleDeny{
					CEL:     "true",
					Message: "blocked",
				},
			}},
			omniav1alpha1.PolicyModeAudit,
		)

		proxyURL, _, cleanup := setupProxyWithUpstream(t, eval, logger)
		defer cleanup()

		resp := makeJSONRequest(t, proxyURL, policyHeaders(), map[string]interface{}{"data": "test"})
		assert.Equal(t, http.StatusOK, resp.StatusCode, "audit mode should forward request")
		_ = resp.Body.Close()
	})

	// Verify audit log was emitted with wouldDeny
	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "policy_decision")
	assert.Contains(t, logOutput, `"wouldDeny"`)
}

func TestPolicyProxyRequiredClaims(t *testing.T) {
	eval, err := policy.NewEvaluator()
	require.NoError(t, err)

	tp := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "claims-policy", Namespace: testNamespace},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{Registry: testRegistry},
			Rules: []omniav1alpha1.PolicyRule{{
				Name: "noop-rule",
				Deny: omniav1alpha1.PolicyRuleDeny{CEL: "false", Message: "never"},
			}},
			RequiredClaims: []omniav1alpha1.RequiredClaim{{
				Claim:   "Team",
				Message: "Team claim is required",
			}},
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
	require.NoError(t, eval.CompilePolicy(tp))

	proxyURL, _, cleanup := setupProxyWithUpstream(t, eval, discardLogger())
	defer cleanup()

	t.Run("missing_claim_denied", func(t *testing.T) {
		resp := makeJSONRequest(t, proxyURL, policyHeaders(), nil)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		denial := readDenialResponse(t, resp)
		assert.Contains(t, denial.Rule, "Team")
		assert.Equal(t, "Team claim is required", denial.Message)
	})

	t.Run("with_claim_allowed", func(t *testing.T) {
		hdrs := policyHeaders()
		hdrs["X-Omnia-Claim-Team"] = "engineering"
		resp := makeJSONRequest(t, proxyURL, hdrs, nil)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()
	})
}

func TestPolicyProxyHeaderInjection(t *testing.T) {
	eval, err := policy.NewEvaluator()
	require.NoError(t, err)

	tp := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "inject-policy", Namespace: testNamespace},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{Registry: testRegistry},
			Rules: []omniav1alpha1.PolicyRule{{
				Name: "amount-check",
				Deny: omniav1alpha1.PolicyRuleDeny{
					CEL:     `has(body.amount) && double(body.amount) > 1000.0`,
					Message: "too much",
				},
			}},
			HeaderInjection: []omniav1alpha1.HeaderInjectionRule{
				{Header: "X-Static-Header", Value: "static-value"},
				{Header: "X-Dynamic-Header", CEL: `"computed-" + headers["X-Omnia-Tool-Name"]`},
			},
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
	require.NoError(t, eval.CompilePolicy(tp))

	proxyURL, _, cleanup := setupProxyWithUpstream(t, eval, discardLogger())
	defer cleanup()

	t.Run("allowed_request_has_injected_headers", func(t *testing.T) {
		resp := makeJSONRequest(t, proxyURL, policyHeaders(), map[string]interface{}{"amount": 50})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		echo := readUpstreamEcho(t, resp)
		assert.Equal(t, "static-value", echo.Headers["X-Static-Header"])
		assert.Equal(t, "computed-"+testToolName, echo.Headers["X-Dynamic-Header"])
	})

	t.Run("denied_request_no_injection", func(t *testing.T) {
		resp := makeJSONRequest(t, proxyURL, policyHeaders(), map[string]interface{}{"amount": 2000})
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		_ = resp.Body.Close()
		// No upstream call happened so no headers to check — just verify denied.
	})
}

func TestPolicyWatcherEvaluatorIntegration(t *testing.T) {
	eval, err := policy.NewEvaluator()
	require.NoError(t, err)

	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	watcher := policy.NewWatcher(eval, k8sClient, scheme, testNamespace, discardLogger())

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

func TestPolicyProxyMultiplePolicies(t *testing.T) {
	eval, err := policy.NewEvaluator()
	require.NoError(t, err)

	// First policy: allows (rule doesn't match)
	compileTestPolicy(t, eval, "allow-policy", testNamespace, testRegistry, nil,
		[]omniav1alpha1.PolicyRule{{
			Name: "never-deny",
			Deny: omniav1alpha1.PolicyRuleDeny{CEL: "false", Message: "never"},
		}},
		omniav1alpha1.PolicyModeEnforce,
	)

	// Second policy: denies all
	compileTestPolicy(t, eval, "deny-policy", testNamespace, testRegistry, nil,
		[]omniav1alpha1.PolicyRule{{
			Name: "always-deny",
			Deny: omniav1alpha1.PolicyRuleDeny{CEL: "true", Message: "blocked by second policy"},
		}},
		omniav1alpha1.PolicyModeEnforce,
	)

	proxyURL, _, cleanup := setupProxyWithUpstream(t, eval, discardLogger())
	defer cleanup()

	resp := makeJSONRequest(t, proxyURL, policyHeaders(), nil)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	denial := readDenialResponse(t, resp)
	assert.Equal(t, "always-deny", denial.Rule)
	assert.Equal(t, "blocked by second policy", denial.Message)
}

func TestContextPropagationHeaders(t *testing.T) {
	fields := &policyctx.PropagationFields{
		AgentName: "test-agent",
		Namespace: "test-ns",
		SessionID: "sess-123",
		RequestID: "req-456",
		UserID:    "user-789",
		UserRoles: "admin,editor",
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
	assert.Equal(t, "admin,editor", headers[policyctx.HeaderUserRoles])
	assert.Equal(t, "user@example.com", headers[policyctx.HeaderUserEmail])
	assert.Equal(t, "openai", headers[policyctx.HeaderProvider])
	assert.Equal(t, "gpt-4", headers[policyctx.HeaderModel])

	// Verify claim headers
	assert.Equal(t, "platform", headers[policyctx.HeaderClaimPrefix+"Team"])
	assert.Equal(t, "us-east", headers[policyctx.HeaderClaimPrefix+"Region"])

	// Verify round-trip through a real HTTP proxy
	eval, err := policy.NewEvaluator()
	require.NoError(t, err)

	// No policies — everything passes through
	proxyURL, _, cleanup := setupProxyWithUpstream(t, eval, discardLogger())
	defer cleanup()

	resp := makeJSONRequest(t, proxyURL, headers, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	echo := readUpstreamEcho(t, resp)

	// Upstream should receive all X-Omnia-* headers
	assert.Equal(t, "test-agent", echo.Headers["X-Omnia-Agent-Name"])
	assert.Equal(t, "test-ns", echo.Headers["X-Omnia-Namespace"])
	assert.Equal(t, "sess-123", echo.Headers["X-Omnia-Session-Id"])
	assert.Equal(t, "platform", echo.Headers["X-Omnia-Claim-Team"])
	assert.Equal(t, "us-east", echo.Headers["X-Omnia-Claim-Region"])
}
