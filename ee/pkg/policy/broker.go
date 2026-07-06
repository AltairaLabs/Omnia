/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	omniapolicy "github.com/altairalabs/omnia/pkg/policy"
)

// Broker response constants.
const (
	brokerErrMalformedRequest = "malformed_request"
	brokerErrMethodNotAllowed = "method_not_allowed"
	logMsgBrokerToolDecision  = "broker_tool_decision"
	maxDecisionRequestBytes   = 1 << 20 // 1 MiB
)

// DecisionRequest is the JSON request body for POST /v1/decision. The
// runtime sends the same (headers, body) shape the evaluator already
// understands, plus a structured Identity so `identity.*` CEL rules and
// identity-aware header injection work without lossy header-flattening.
type DecisionRequest struct {
	Headers  map[string]string      `json:"headers"`
	Body     map[string]interface{} `json:"body"`
	Identity *IdentityPayload       `json:"identity"`
}

// IdentityPayload carries the caller's AuthenticatedIdentity fields over the
// wire so the broker can rebuild an omniapolicy.AuthenticatedIdentity and
// attach it to the evaluation context.
type IdentityPayload struct {
	Origin    string            `json:"origin"`
	Subject   string            `json:"subject"`
	EndUser   string            `json:"endUser"`
	Workspace string            `json:"workspace"`
	Agent     string            `json:"agent"`
	Role      string            `json:"role"`
	Claims    map[string]string `json:"claims"`
}

// DecisionResponse is the JSON response body for POST /v1/decision.
type DecisionResponse struct {
	Allow           bool              `json:"allow"`
	DeniedBy        string            `json:"deniedBy"`
	Message         string            `json:"message"`
	Mode            string            `json:"mode"`
	WouldDeny       bool              `json:"wouldDeny"`
	InjectedHeaders map[string]string `json:"injectedHeaders"`
}

// BrokerHandler is an HTTP handler that answers "may this tool call
// proceed, and what headers to inject" over a localhost decision endpoint.
// It replaces the dead reverse-proxy shape (ProxyHandler) with a call the
// runtime makes per tool call, since Istio ambient mode has no waypoint on
// tool egress to transparently intercept.
type BrokerHandler struct {
	evaluator *Evaluator
	logger    *slog.Logger
}

// NewBrokerHandler creates a new decision-endpoint HTTP handler.
func NewBrokerHandler(evaluator *Evaluator, logger *slog.Logger) *BrokerHandler {
	return &BrokerHandler{
		evaluator: evaluator,
		logger:    logger,
	}
}

// ServeHTTP decodes a DecisionRequest, evaluates ToolPolicy rules (and
// header injection) against it, and writes back a DecisionResponse.
func (h *BrokerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowedResponse(w)
		return
	}

	req, err := decodeDecisionRequest(w, r)
	if err != nil {
		h.logger.Debug("malformed decision request", "error", err.Error())
		writeMalformedRequestResponse(w)
		return
	}

	ctx := withIdentityFromPayload(r.Context(), req.Identity)

	decision := h.evaluator.EvaluateWithContext(ctx, req.Headers, req.Body)
	logDecision(h.logger, r, decision)
	logBrokerToolDecision(h.logger, decision, req.Headers)

	// A denied call must not compute or return injected headers — header
	// injection only applies to calls that are actually allowed to proceed.
	var injected map[string]string
	if decision.Allowed {
		injected = h.evaluateHeaderInjection(ctx, req)
	}

	writeDecisionResponse(w, decision, injected)
}

// decodeDecisionRequest decodes the JSON request body into a DecisionRequest.
// The body is wrapped in http.MaxBytesReader so an oversized request is
// rejected as a decode error rather than consuming unbounded memory.
func decodeDecisionRequest(w http.ResponseWriter, r *http.Request) (DecisionRequest, error) {
	var req DecisionRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxDecisionRequestBytes)
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}

// logBrokerToolDecision emits a broker-specific structured log line carrying
// the tool identity for this decision. The shared logDecision (proxy.go)
// logs r.URL.Path/r.Method, which for every broker call are always
// "/v1/decision"/"POST" — that loses which tool was evaluated. The tool
// identity travels in the request headers instead, so it's added here as
// explicit fields. Skip wholly-uninteresting allows (no rule matched) to
// match logDecision's own audit-noise gating.
func logBrokerToolDecision(logger *slog.Logger, decision Decision, headers map[string]string) {
	if decision.Allowed && decision.DeniedBy == "" {
		return
	}
	logger.Info(logMsgBrokerToolDecision,
		"toolName", headers[HeaderToolName],
		"toolRegistry", headers[HeaderToolRegistry],
		"allowed", decision.Allowed,
		"deniedBy", decision.DeniedBy,
		"mode", string(decision.Mode),
	)
}

// evaluateHeaderInjection evaluates header injection rules for the request.
// On error, it logs and returns nil so the decision is still returned to
// the caller — header injection is a best-effort addition to a decision,
// not a reason to fail the whole call.
func (h *BrokerHandler) evaluateHeaderInjection(ctx context.Context, req DecisionRequest) map[string]string {
	injected, err := h.evaluator.EvaluateHeaderInjectionWithContext(ctx, req.Headers, req.Body)
	if err != nil {
		h.logger.Error("header injection evaluation failed", "error", err.Error())
		return nil
	}
	return injected
}

// withIdentityFromPayload rebuilds an AuthenticatedIdentity from the wire
// payload and attaches it to ctx. Returns ctx unchanged when payload is nil.
func withIdentityFromPayload(ctx context.Context, payload *IdentityPayload) context.Context {
	if payload == nil {
		return ctx
	}
	identity := &omniapolicy.AuthenticatedIdentity{
		Origin:    payload.Origin,
		Subject:   payload.Subject,
		EndUser:   payload.EndUser,
		Workspace: payload.Workspace,
		Agent:     payload.Agent,
		Role:      payload.Role,
		Claims:    payload.Claims,
	}
	return omniapolicy.WithIdentity(ctx, identity)
}

// writeDecisionResponse writes a 200 JSON DecisionResponse built from the
// evaluated Decision and injected headers.
func writeDecisionResponse(w http.ResponseWriter, decision Decision, injected map[string]string) {
	resp := DecisionResponse{
		Allow:           decision.Allowed,
		DeniedBy:        decision.DeniedBy,
		Message:         decision.Message,
		Mode:            string(decision.Mode),
		WouldDeny:       decision.WouldDeny,
		InjectedHeaders: injected,
	}
	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// writeMalformedRequestResponse writes a 400 response for a request whose
// body could not be decoded as a DecisionRequest.
func writeMalformedRequestResponse(w http.ResponseWriter) {
	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(`{"error":"` + brokerErrMalformedRequest + `"}`))
}

// writeMethodNotAllowedResponse writes a 405 response for any method other
// than POST — /v1/decision is a POST-only decision endpoint.
func writeMethodNotAllowedResponse(w http.ResponseWriter) {
	w.Header().Set(headerContentType, contentTypeJSON)
	w.Header().Set("Allow", http.MethodPost)
	w.WriteHeader(http.StatusMethodNotAllowed)
	_, _ = w.Write([]byte(`{"error":"` + brokerErrMethodNotAllowed + `"}`))
}
