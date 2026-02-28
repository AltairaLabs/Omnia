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
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func setupProxyTest(t *testing.T) (*Evaluator, *httptest.Server, *ProxyHandler) {
	t.Helper()

	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"forwarded"}`))
	}))

	upstreamURL, _ := url.Parse(upstream.URL)
	handler := NewProxyHandler(eval, upstreamURL, testLogger())

	return eval, upstream, handler
}

func TestProxyHandler_NoPolicies_ForwardsRequest(t *testing.T) {
	_, upstream, handler := setupProxyTest(t)
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/invoke", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["status"] != "forwarded" {
		t.Errorf("body status = %q, want %q", body["status"], "forwarded")
	}
}

func TestProxyHandler_DeniedRequest(t *testing.T) {
	eval, upstream, handler := setupProxyTest(t)
	defer upstream.Close()

	policy := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
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
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/invoke", nil)
	req.Header.Set(HeaderToolName, "blocked_tool")
	req.Header.Set(HeaderToolRegistry, "test-registry")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	var denial DenialResponse
	if err := json.NewDecoder(rec.Body).Decode(&denial); err != nil {
		t.Fatalf("failed to decode denial: %v", err)
	}
	if denial.Error != "policy_denied" {
		t.Errorf("error = %q, want %q", denial.Error, "policy_denied")
	}
	if denial.Rule != "block-all" {
		t.Errorf("rule = %q, want %q", denial.Rule, "block-all")
	}
}

func TestProxyHandler_AllowedRequest(t *testing.T) {
	eval, upstream, handler := setupProxyTest(t)
	defer upstream.Close()

	policy := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
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
	if err := eval.CompilePolicy(policy); err != nil {
		t.Fatalf("CompilePolicy() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/invoke", nil)
	req.Header.Set(HeaderToolName, "allowed_tool")
	req.Header.Set(HeaderToolRegistry, "test-registry")
	req.Header.Set("X-Omnia-Param-Amount", "500")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestProxyHandler_WithJSONBody(t *testing.T) {
	eval, upstream, handler := setupProxyTest(t)
	defer upstream.Close()

	policy := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "body-policy", Namespace: "default"},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "test-registry",
			},
			Rules: []omniav1alpha1.PolicyRule{
				{
					Name: "body-check",
					Deny: omniav1alpha1.PolicyRuleDeny{
						CEL:     "has(body.blocked) && body.blocked == true",
						Message: "blocked field set",
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

	bodyJSON := `{"blocked": true, "data": "test"}`
	req := httptest.NewRequest(http.MethodPost, "/invoke", strings.NewReader(bodyJSON))
	req.Header.Set(HeaderToolRegistry, "test-registry")
	req.Header.Set(HeaderToolName, "some_tool")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestExtractHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Test", "value1")
	req.Header.Set("X-Other", "value2")
	req.Header.Add("X-Multi", "first")
	req.Header.Add("X-Multi", "second")

	headers := extractHeaders(req)

	if headers["X-Test"] != "value1" {
		t.Errorf("X-Test = %q, want %q", headers["X-Test"], "value1")
	}
	if headers["X-Other"] != "value2" {
		t.Errorf("X-Other = %q, want %q", headers["X-Other"], "value2")
	}
	if headers["X-Multi"] != "first" {
		t.Errorf("X-Multi = %q, want %q", headers["X-Multi"], "first")
	}
}

func TestParseBody_ValidJSON(t *testing.T) {
	body := bytes.NewReader([]byte(`{"key": "value", "num": 42}`))
	req := httptest.NewRequest(http.MethodPost, "/", body)

	result := parseBody(req, testLogger())
	if result == nil {
		t.Fatal("parseBody() returned nil for valid JSON")
	}
	if result["key"] != "value" {
		t.Errorf("key = %v, want %q", result["key"], "value")
	}
}

func TestParseBody_InvalidJSON(t *testing.T) {
	body := bytes.NewReader([]byte(`not json`))
	req := httptest.NewRequest(http.MethodPost, "/", body)

	result := parseBody(req, testLogger())
	if result != nil {
		t.Errorf("parseBody() = %v, want nil for invalid JSON", result)
	}
}

func TestParseBody_NilBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Body = nil

	result := parseBody(req, testLogger())
	if result != nil {
		t.Errorf("parseBody() = %v, want nil for nil body", result)
	}
}

func TestHealthHandler(t *testing.T) {
	handler := HealthHandler()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
}

func TestWriteDenialResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	decision := Decision{
		Allowed:  false,
		DeniedBy: "test-rule",
		Message:  "test denial",
	}

	writeDenialResponse(rec, decision)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	contentType := rec.Header().Get(headerContentType)
	if contentType != contentTypeJSON {
		t.Errorf("content-type = %q, want %q", contentType, contentTypeJSON)
	}

	var resp DenialResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Error != "policy_denied" {
		t.Errorf("error = %q, want %q", resp.Error, "policy_denied")
	}
	if resp.Rule != "test-rule" {
		t.Errorf("rule = %q, want %q", resp.Rule, "test-rule")
	}
	if resp.Message != "test denial" {
		t.Errorf("message = %q, want %q", resp.Message, "test denial")
	}
}

func TestProxyHandler_RecordsMetricsOnDeny(t *testing.T) {
	PolicyDecisionsTotal.Reset()
	PolicyDenialsTotal.Reset()
	PolicyEvaluationDuration.Reset()

	eval, upstream, handler := setupProxyTest(t)
	defer upstream.Close()

	policy := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics-test", Namespace: "default"},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "test-registry",
				Tools:    []string{"denied_tool"},
			},
			Rules: []omniav1alpha1.PolicyRule{
				{
					Name: "deny-rule",
					Deny: omniav1alpha1.PolicyRuleDeny{
						CEL:     "true",
						Message: "denied",
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

	req := httptest.NewRequest(http.MethodPost, "/invoke", nil)
	req.Header.Set(HeaderToolName, "denied_tool")
	req.Header.Set(HeaderToolRegistry, "test-registry")
	req.Header.Set(HeaderAgentName, "agent-x")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	// Verify decisions counter was incremented for deny
	denyVal := getCounterValue(PolicyDecisionsTotal, "tool", "metrics-test", "deny-rule", "deny")
	if denyVal < 1 {
		t.Errorf("decisions deny counter = %f, want >= 1", denyVal)
	}

	// Verify denials counter was incremented
	denialsVal := getCounterValue(PolicyDenialsTotal, "metrics-test", "deny-rule", "agent-x", "denied_tool")
	if denialsVal < 1 {
		t.Errorf("denials counter = %f, want >= 1", denialsVal)
	}

	// Verify histogram has at least one observation
	histCount := getHistogramCount(PolicyEvaluationDuration, "tool")
	if histCount < 1 {
		t.Errorf("histogram count = %d, want >= 1", histCount)
	}
}

func TestProxyHandler_RecordsMetricsOnAllow(t *testing.T) {
	PolicyDecisionsTotal.Reset()
	PolicyEvaluationDuration.Reset()

	eval, upstream, handler := setupProxyTest(t)
	defer upstream.Close()

	policy := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-metrics", Namespace: "default"},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "test-registry",
				Tools:    []string{"ok_tool"},
			},
			Rules: []omniav1alpha1.PolicyRule{
				{
					Name: "pass-rule",
					Deny: omniav1alpha1.PolicyRuleDeny{
						CEL:     "false",
						Message: "never",
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

	req := httptest.NewRequest(http.MethodPost, "/invoke", nil)
	req.Header.Set(HeaderToolName, "ok_tool")
	req.Header.Set(HeaderToolRegistry, "test-registry")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	allowVal := getCounterValue(PolicyDecisionsTotal, "tool", "allow-metrics", "", "allow")
	if allowVal < 1 {
		t.Errorf("decisions allow counter = %f, want >= 1", allowVal)
	}
}

func TestProxyHandler_EmitsAuditLog(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	handler := NewProxyHandler(eval, upstreamURL, logger)

	policy := &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "audit-test", Namespace: "default"},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "test-registry",
				Tools:    []string{"my_tool"},
			},
			Rules: []omniav1alpha1.PolicyRule{
				{
					Name: "block-rule",
					Deny: omniav1alpha1.PolicyRuleDeny{
						CEL:     "true",
						Message: "blocked for audit test",
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

	req := httptest.NewRequest(http.MethodPost, "/invoke", nil)
	req.Header.Set(HeaderToolName, "my_tool")
	req.Header.Set(HeaderToolRegistry, "test-registry")
	req.Header.Set(HeaderAgentName, "agent-1")
	req.Header.Set(HeaderUser, "alice")
	req.Header.Set(HeaderSessionID, "sess-99")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	output := buf.String()
	if !strings.Contains(output, "policy.audit") {
		t.Error("audit log not found in output")
	}
	if !strings.Contains(output, "audit-test") {
		t.Error("policy name not found in audit log")
	}
	if !strings.Contains(output, "agent-1") {
		t.Error("agent name not found in audit log")
	}
	if !strings.Contains(output, "alice") {
		t.Error("user not found in audit log")
	}
	if !strings.Contains(output, "sess-99") {
		t.Error("session ID not found in audit log")
	}
}

func TestRecordMetrics_DurationObservation(t *testing.T) {
	PolicyEvaluationDuration.Reset()

	h := &ProxyHandler{logger: testLogger(), audit: NewAuditLogger(testLogger())}
	decision := Decision{Allowed: true, PolicyName: "test-policy"}
	headers := map[string]string{}

	h.recordMetrics(decision, headers, 50*time.Millisecond)

	count := getHistogramCount(PolicyEvaluationDuration, "tool")
	if count < 1 {
		t.Errorf("histogram count = %d, want >= 1", count)
	}
}

func TestEmitAuditLog_DefaultMode(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	h := &ProxyHandler{logger: logger, audit: NewAuditLogger(logger)}
	decision := Decision{Allowed: true}
	headers := map[string]string{}

	h.emitAuditLog(decision, headers)

	if !strings.Contains(buf.String(), "enforce") {
		t.Error("default mode should be 'enforce' when PolicyMode is empty")
	}
}

func TestNewProxyHandler(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	upstreamURL, _ := url.Parse("http://localhost:9090")
	handler := NewProxyHandler(eval, upstreamURL, testLogger())

	if handler.evaluator != eval {
		t.Error("evaluator not set correctly")
	}
	if handler.audit == nil {
		t.Error("audit logger not initialized")
	}
	if handler.upstream == nil {
		t.Error("upstream proxy not initialized")
	}
}
