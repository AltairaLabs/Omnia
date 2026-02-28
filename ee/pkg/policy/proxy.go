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
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/go-logr/logr"
)

// HTTP header constants for tool metadata.
const (
	// HeaderToolRegistry identifies the tool registry name.
	HeaderToolRegistry = "X-Omnia-Tool-Registry"
	// HeaderToolName identifies the tool being called.
	HeaderToolName = "X-Omnia-Tool-Name"
	// ContentTypeJSON is the JSON content type.
	ContentTypeJSON = "application/json"
	// HeaderContentType is the Content-Type header.
	HeaderContentType = "Content-Type"
	// maxBodyBytes is the maximum body size for policy evaluation (1MB).
	maxBodyBytes = 1 << 20
)

// DenyResponse is the structured JSON response for denied requests.
type DenyResponse struct {
	Error      string `json:"error"`
	PolicyName string `json:"policyName"`
	RuleName   string `json:"ruleName"`
	Message    string `json:"message"`
}

// ProxyHandler is an HTTP handler that evaluates ToolPolicy rules before
// forwarding requests to an upstream service.
type ProxyHandler struct {
	evaluator *Evaluator
	proxy     *httputil.ReverseProxy
	log       logr.Logger
	auditLog  bool
}

// NewProxyHandler creates a new policy proxy handler.
func NewProxyHandler(evaluator *Evaluator, upstreamURL *url.URL, log logr.Logger, auditLog bool) *ProxyHandler {
	proxy := httputil.NewSingleHostReverseProxy(upstreamURL)
	return &ProxyHandler{
		evaluator: evaluator,
		proxy:     proxy,
		log:       log.WithName("policy-proxy"),
		auditLog:  auditLog,
	}
}

// ServeHTTP evaluates policies and either denies the request or forwards it upstream.
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	registry := r.Header.Get(HeaderToolRegistry)
	tool := r.Header.Get(HeaderToolName)

	// If no tool metadata headers, pass through directly
	if registry == "" || tool == "" {
		h.proxy.ServeHTTP(w, r)
		return
	}

	evalCtx := h.buildEvalContext(r)
	results := h.evaluator.Evaluate(registry, tool, evalCtx)

	if h.auditLog {
		h.logDecisions(registry, tool, results)
	}

	if denied, result := findDenied(results); denied {
		h.writeDenyResponse(w, result)
		return
	}

	h.proxy.ServeHTTP(w, r)
}

// buildEvalContext creates an EvalContext from the HTTP request.
func (h *ProxyHandler) buildEvalContext(r *http.Request) *EvalContext {
	headers := flattenHeaders(r.Header)
	body := readBody(r, h.log)
	return &EvalContext{
		Headers: headers,
		Body:    body,
	}
}

// flattenHeaders converts multi-value HTTP headers to single-value strings.
func flattenHeaders(httpHeaders http.Header) map[string]string {
	headers := make(map[string]string, len(httpHeaders))
	for key, values := range httpHeaders {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	return headers
}

// readBody reads and parses the request body as JSON.
// Returns nil if the body cannot be read or parsed.
func readBody(r *http.Request, log logr.Logger) map[string]interface{} {
	if r.Body == nil {
		return nil
	}

	limitedReader := io.LimitReader(r.Body, maxBodyBytes)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		log.V(1).Info("failed to read request body", "error", err)
		return nil
	}

	// Reset body so upstream handler can read it
	r.Body = io.NopCloser(strings.NewReader(string(data)))

	if len(data) == 0 {
		return nil
	}

	var body map[string]interface{}
	if err := json.Unmarshal(data, &body); err != nil {
		log.V(1).Info("failed to parse body as JSON", "error", err)
		return nil
	}
	return body
}

// findDenied checks if any result was denied and returns the first denial.
func findDenied(results []EvalResult) (bool, *EvalResult) {
	for i := range results {
		if results[i].Denied {
			return true, &results[i]
		}
	}
	return false, nil
}

// findDeniedRule returns the name of the first denied rule in a result.
func findDeniedRule(result *EvalResult) string {
	for _, d := range result.Decisions {
		if d.Denied {
			return d.RuleName
		}
	}
	return ""
}

// writeDenyResponse writes a 403 JSON response for denied requests.
func (h *ProxyHandler) writeDenyResponse(w http.ResponseWriter, result *EvalResult) {
	resp := DenyResponse{
		Error:      "policy_denied",
		PolicyName: result.PolicyName,
		RuleName:   findDeniedRule(result),
		Message:    result.DenyMessage,
	}

	w.Header().Set(HeaderContentType, ContentTypeJSON)
	w.WriteHeader(http.StatusForbidden)

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Error(err, "failed to write deny response")
	}
}

// logDecisions logs the policy evaluation decisions.
func (h *ProxyHandler) logDecisions(registry, tool string, results []EvalResult) {
	for _, r := range results {
		for _, d := range r.Decisions {
			h.log.Info("policy decision",
				"registry", registry,
				"tool", tool,
				"policy", r.PolicyName,
				"rule", d.RuleName,
				"denied", d.Denied,
			)
		}
	}
}

// HealthHandler returns a simple health check handler.
func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}
