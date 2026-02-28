/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func setupProxyTest(t *testing.T, rules []RuleInput) (*ProxyHandler, *httptest.Server) {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("upstream-ok"))
	}))

	upstreamURL, _ := url.Parse(upstream.URL)
	e := newTestEvaluator(t)
	log := zap.New(zap.UseDevMode(true))

	if len(rules) > 0 {
		err := e.SetPolicy("test-policy", "default", "tools", []string{"refund"}, rules, "enforce", "deny")
		if err != nil {
			t.Fatalf("SetPolicy error: %v", err)
		}
	}

	handler := NewProxyHandler(e, upstreamURL, log, true)
	return handler, upstream
}

func TestProxyHandler_NoToolHeaders_PassThrough(t *testing.T) {
	handler, upstream := setupProxyTest(t, nil)
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestProxyHandler_Allowed(t *testing.T) {
	handler, upstream := setupProxyTest(t, []RuleInput{
		{Name: "r1", CEL: "int(headers['X-Omnia-Param-Amount']) > 1000", Message: "too much"},
	})
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/call", nil)
	req.Header.Set(HeaderToolRegistry, "tools")
	req.Header.Set(HeaderToolName, "refund")
	req.Header.Set("X-Omnia-Param-Amount", "500")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestProxyHandler_Denied(t *testing.T) {
	handler, upstream := setupProxyTest(t, []RuleInput{
		{Name: "amount-limit", CEL: "int(headers['X-Omnia-Param-Amount']) > 1000", Message: "Amount exceeds $1000"},
	})
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/call", nil)
	req.Header.Set(HeaderToolRegistry, "tools")
	req.Header.Set(HeaderToolName, "refund")
	req.Header.Set("X-Omnia-Param-Amount", "2000")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}

	var resp DenyResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error != "policy_denied" {
		t.Errorf("unexpected error field: %q", resp.Error)
	}
	if resp.PolicyName != "test-policy" {
		t.Errorf("unexpected policy name: %q", resp.PolicyName)
	}
	if resp.RuleName != "amount-limit" {
		t.Errorf("unexpected rule name: %q", resp.RuleName)
	}
	if resp.Message != "Amount exceeds $1000" {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

func TestProxyHandler_ToolNotMatched_PassThrough(t *testing.T) {
	handler, upstream := setupProxyTest(t, []RuleInput{
		{Name: "r1", CEL: "true", Message: "always deny"},
	})
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/call", nil)
	req.Header.Set(HeaderToolRegistry, "tools")
	req.Header.Set(HeaderToolName, "transfer") // policy only covers "refund"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (tool not matched), got %d", rr.Code)
	}
}

func TestProxyHandler_WithBody(t *testing.T) {
	e := newTestEvaluator(t)
	_ = e.SetPolicy("body-policy", "default", "tools", []string{"transfer"}, []RuleInput{
		{Name: "body-check", CEL: "body['amount'] == 9999.0", Message: "suspicious amount"},
	}, "enforce", "deny")

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	log := zap.New(zap.UseDevMode(true))
	handler := NewProxyHandler(e, upstreamURL, log, false)

	body := `{"amount": 9999, "currency": "USD"}`
	req := httptest.NewRequest(http.MethodPost, "/call", strings.NewReader(body))
	req.Header.Set(HeaderToolRegistry, "tools")
	req.Header.Set(HeaderToolName, "transfer")
	req.Header.Set(HeaderContentType, ContentTypeJSON)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestProxyHandler_BodyPreservedForUpstream(t *testing.T) {
	var upstreamBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		upstreamBody = string(data)
	}))
	defer upstream.Close()

	e := newTestEvaluator(t)
	_ = e.SetPolicy("p", "ns", "tools", []string{"tool1"}, []RuleInput{
		{Name: "r", CEL: "false", Message: "never denies"},
	}, "enforce", "deny")

	upstreamURL, _ := url.Parse(upstream.URL)
	log := zap.New(zap.UseDevMode(true))
	handler := NewProxyHandler(e, upstreamURL, log, false)

	body := `{"key":"value"}`
	req := httptest.NewRequest(http.MethodPost, "/call", strings.NewReader(body))
	req.Header.Set(HeaderToolRegistry, "tools")
	req.Header.Set(HeaderToolName, "tool1")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if upstreamBody != body {
		t.Errorf("upstream body = %q, want %q", upstreamBody, body)
	}
}

func TestFlattenHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("X-Single", "one")
	h.Add("X-Multi", "first")
	h.Add("X-Multi", "second")

	flat := flattenHeaders(h)
	if flat["X-Single"] != "one" {
		t.Errorf("expected 'one', got %q", flat["X-Single"])
	}
	if flat["X-Multi"] != "first" {
		t.Errorf("expected 'first' (first value), got %q", flat["X-Multi"])
	}
}

func TestReadBody_NilBody(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	result := readBody(req, log)
	if result != nil {
		t.Error("expected nil for nil body")
	}
}

func TestReadBody_EmptyBody(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	result := readBody(req, log)
	if result != nil {
		t.Error("expected nil for empty body")
	}
}

func TestReadBody_InvalidJSON(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not json"))
	result := readBody(req, log)
	if result != nil {
		t.Error("expected nil for invalid JSON")
	}
}

func TestFindDenied_NoDenial(t *testing.T) {
	results := []EvalResult{
		{Denied: false},
		{Denied: false},
	}
	denied, _ := findDenied(results)
	if denied {
		t.Error("expected no denial")
	}
}

func TestFindDenied_WithDenial(t *testing.T) {
	results := []EvalResult{
		{Denied: false},
		{Denied: true, PolicyName: "p2", DenyMessage: "blocked"},
	}
	denied, result := findDenied(results)
	if !denied {
		t.Fatal("expected denial")
	}
	if result.PolicyName != "p2" {
		t.Errorf("unexpected policy name: %q", result.PolicyName)
	}
}

func TestFindDeniedRule(t *testing.T) {
	result := &EvalResult{
		Decisions: []Decision{
			{RuleName: "r1", Denied: false},
			{RuleName: "r2", Denied: true},
		},
	}
	name := findDeniedRule(result)
	if name != "r2" {
		t.Errorf("expected 'r2', got %q", name)
	}
}

func TestFindDeniedRule_NoDenied(t *testing.T) {
	result := &EvalResult{
		Decisions: []Decision{
			{RuleName: "r1", Denied: false},
		},
	}
	name := findDeniedRule(result)
	if name != "" {
		t.Errorf("expected empty, got %q", name)
	}
}

func TestHealthHandler(t *testing.T) {
	handler := HealthHandler()
	rr := httptest.NewRecorder()
	handler(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "ok" {
		t.Errorf("expected 'ok', got %q", rr.Body.String())
	}
}

func TestProxyHandler_MissingRegistryHeader_PassThrough(t *testing.T) {
	handler, upstream := setupProxyTest(t, []RuleInput{
		{Name: "r1", CEL: "true", Message: "always deny"},
	})
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/call", nil)
	req.Header.Set(HeaderToolName, "refund")
	// Missing HeaderToolRegistry
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (pass through), got %d", rr.Code)
	}
}

func TestProxyHandler_MissingToolNameHeader_PassThrough(t *testing.T) {
	handler, upstream := setupProxyTest(t, []RuleInput{
		{Name: "r1", CEL: "true", Message: "always deny"},
	})
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/call", nil)
	req.Header.Set(HeaderToolRegistry, "tools")
	// Missing HeaderToolName
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (pass through), got %d", rr.Code)
	}
}

func TestProxyHandler_AuditMode(t *testing.T) {
	handler, upstream := setupProxyTest(t, []RuleInput{
		{Name: "r1", CEL: "true", Message: "audit only"},
	})
	defer upstream.Close()

	// With audit logging enabled (set in setupProxyTest)
	req := httptest.NewRequest(http.MethodPost, "/call", nil)
	req.Header.Set(HeaderToolRegistry, "tools")
	req.Header.Set(HeaderToolName, "refund")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	// Still denied because the evaluator mode is enforce (proxy doesn't check mode)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}
