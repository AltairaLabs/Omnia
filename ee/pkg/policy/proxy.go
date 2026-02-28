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
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// Proxy response and content type constants.
const (
	contentTypeJSON   = "application/json"
	headerContentType = "Content-Type"
)

// DenialResponse is the JSON response returned when a request is denied.
type DenialResponse struct {
	Error   string `json:"error"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// ProxyHandler is an HTTP handler that evaluates ToolPolicy rules before
// forwarding requests to the upstream tool service.
type ProxyHandler struct {
	evaluator *Evaluator
	upstream  *httputil.ReverseProxy
	logger    *slog.Logger
}

// NewProxyHandler creates a new policy proxy HTTP handler.
func NewProxyHandler(evaluator *Evaluator, upstreamURL *url.URL, logger *slog.Logger) *ProxyHandler {
	proxy := httputil.NewSingleHostReverseProxy(upstreamURL)
	return &ProxyHandler{
		evaluator: evaluator,
		upstream:  proxy,
		logger:    logger,
	}
}

// ServeHTTP evaluates policies and either denies the request or forwards it upstream.
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	headers := extractHeaders(r)
	body := parseBody(r, h.logger)

	decision := h.evaluator.Evaluate(headers, body)

	h.logDecision(r, decision)

	if !decision.Allowed {
		writeDenialResponse(w, decision)
		return
	}

	h.upstream.ServeHTTP(w, r)
}

// extractHeaders converts HTTP request headers into a flat string map.
// For headers with multiple values, only the first value is used.
func extractHeaders(r *http.Request) map[string]string {
	headers := make(map[string]string, len(r.Header))
	for key, values := range r.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	return headers
}

// parseBody attempts to parse the request body as JSON.
// Returns nil if the body cannot be parsed.
func parseBody(r *http.Request, logger *slog.Logger) map[string]interface{} {
	if r.Body == nil {
		return nil
	}

	defer func() { _ = r.Body.Close() }()

	data, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Debug("failed to read request body", "error", err.Error())
		return nil
	}

	var body map[string]interface{}
	if err := json.Unmarshal(data, &body); err != nil {
		logger.Debug("failed to parse request body as JSON", "error", err.Error())
		return nil
	}
	return body
}

// writeDenialResponse writes a 403 Forbidden response with denial details.
func writeDenialResponse(w http.ResponseWriter, decision Decision) {
	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(http.StatusForbidden)

	resp := DenialResponse{
		Error:   "policy_denied",
		Rule:    decision.DeniedBy,
		Message: decision.Message,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// logDecision logs the policy decision for audit purposes.
func (h *ProxyHandler) logDecision(r *http.Request, decision Decision) {
	if decision.Allowed && decision.DeniedBy == "" {
		return
	}

	fields := []any{
		"path", r.URL.Path,
		"method", r.Method,
		"allowed", decision.Allowed,
	}

	if decision.DeniedBy != "" {
		fields = append(fields, "deniedBy", decision.DeniedBy, "message", decision.Message)
	}

	if decision.Error != nil {
		fields = append(fields, "error", decision.Error.Error())
	}

	h.logger.Info("policy decision", fields...)
}

// HealthHandler returns a simple health check handler for the proxy.
func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}
