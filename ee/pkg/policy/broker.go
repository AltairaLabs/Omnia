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
	"net/http"
	"time"

	"github.com/go-logr/logr"

	omniapolicy "github.com/altairalabs/omnia/pkg/policy"
)

// Broker response constants.
const (
	brokerErrMalformedRequest  = "malformed_request"
	brokerErrMethodNotAllowed  = "method_not_allowed"
	logMsgBrokerPolicyDecision = "policy_decision"
	logMsgBrokerToolDecision   = "broker_tool_decision"
	maxDecisionRequestBytes    = 1 << 20 // 1 MiB
)

// DecisionRequest, IdentityPayload, and DecisionResponse are the wire types
// for POST /v1/decision. They live in the shared pkg/policy package (as
// omniapolicy.DecisionRequest etc.) so internal/runtime (core, must not
// import ee/) can build requests and parse responses without depending on
// this enterprise-only package. Aliased here so this file's existing
// references keep working unchanged.
type (
	// DecisionRequest is the JSON request body for POST /v1/decision. The
	// runtime sends the same (headers, body) shape the evaluator already
	// understands, plus a structured Identity so `identity.*` CEL rules and
	// identity-aware header injection work without lossy header-flattening.
	DecisionRequest = omniapolicy.DecisionRequest

	// IdentityPayload carries the caller's AuthenticatedIdentity fields over
	// the wire so the broker can rebuild an omniapolicy.AuthenticatedIdentity
	// and attach it to the evaluation context.
	IdentityPayload = omniapolicy.IdentityPayload

	// DecisionResponse is the JSON response body for POST /v1/decision.
	DecisionResponse = omniapolicy.DecisionResponse
)

// BrokerHandler is an HTTP handler that answers "may this tool call
// proceed, and what headers to inject" over a localhost decision endpoint.
// It replaces the dead reverse-proxy shape (ProxyHandler) with a call the
// runtime makes per tool call, since Istio ambient mode has no waypoint on
// tool egress to transparently intercept.
type BrokerHandler struct {
	evaluator *Evaluator
	logger    logr.Logger

	// metrics is optional (nil-safe) so existing callers/tests that build a
	// BrokerHandler directly via NewBrokerHandler keep compiling without
	// constructing a *Metrics. Set it via SetMetrics.
	metrics *Metrics
}

// NewBrokerHandler creates a new decision-endpoint HTTP handler.
func NewBrokerHandler(evaluator *Evaluator, logger logr.Logger) *BrokerHandler {
	return &BrokerHandler{
		evaluator: evaluator,
		logger:    logger,
	}
}

// SetMetrics attaches Prometheus metrics to the handler. Nil-safe: when never
// called, ServeHTTP skips recording rather than panicking, so unit tests don't
// need to construct a *Metrics.
func (h *BrokerHandler) SetMetrics(metrics *Metrics) {
	h.metrics = metrics
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
		h.logger.V(1).Info("malformed decision request", "error", err.Error())
		writeMalformedRequestResponse(w)
		return
	}

	// Entry log for EVERY well-formed call, before evaluation. logBrokerDecision
	// (below) skips plain allows, so without this a broker that IS being called
	// but only ever plain-allows looks identical to one nothing calls at all —
	// which made "is the runtime reaching the broker?" undiagnosable. With
	// LOG_LEVEL=debug this answers it in one grep.
	h.logger.V(1).Info("decision request received",
		"toolName", req.Headers[HeaderToolName],
		"toolRegistry", req.Headers[HeaderToolRegistry])

	ctx := withIdentityFromPayload(r.Context(), req.Identity)

	start := time.Now()
	decision := h.evaluator.EvaluateWithContext(ctx, req.Headers, req.Body)
	h.recordDecisionMetrics(decision, req.Headers, time.Since(start))
	logBrokerDecision(h.logger, decision, req.Headers)

	// A rule that failed to evaluate (CEL runtime error / non-bool result) is an
	// operator-actionable misconfiguration, not a normal decision — surface it at
	// Error level (and as the `error` metric outcome) so a rule erroring on every
	// call is loud, not silently folded into the deny count.
	if decision.Error != nil {
		h.logger.Error(decision.Error, "ToolPolicy rule evaluation failed",
			"toolName", req.Headers[HeaderToolName],
			"toolRegistry", req.Headers[HeaderToolRegistry],
			"policy", decision.Policy,
			"deniedBy", decision.DeniedBy,
			"mode", string(decision.Mode))
	}

	// A denied call must not compute or return injected headers — header
	// injection only applies to calls that are actually allowed to proceed.
	var injected map[string]string
	if decision.Allowed {
		injected = h.evaluateHeaderInjection(ctx, req)
	}

	writeDecisionResponse(w, decision, injected)
}

// recordDecisionMetrics records the decision outcome and latency when metrics
// are attached (SetMetrics). No-op when h.metrics is nil.
func (h *BrokerHandler) recordDecisionMetrics(decision Decision, headers map[string]string, elapsed time.Duration) {
	if h.metrics == nil {
		return
	}
	h.metrics.RecordDecision(decision, headers[HeaderToolRegistry], elapsed.Seconds())
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

// logBrokerDecision emits the broker's own structured decision-audit log
// lines. It emits two lines: a "policy_decision" line with the decision
// outcome, and a "broker_tool_decision" line carrying the tool identity
// (which lives in the request headers, not on the request path/method —
// every broker call's path/method is always "/v1/decision"/"POST", which
// loses which tool was evaluated). Skips wholly-uninteresting allows (no
// rule matched) to keep audit noise low.
func logBrokerDecision(logger logr.Logger, decision Decision, headers map[string]string) {
	if decision.Allowed && decision.DeniedBy == "" {
		// Plain allow — no rule denied. Kept out of the Info audit stream to
		// avoid noise, but emitted at debug: this is the ONLY signal that a
		// misconfigured selector produced an unexpected allow. `policy` empty
		// here means NO ToolPolicy matched this call at all — cross-check the
		// logged toolRegistry against your policy's `selector.registry`.
		logger.V(1).Info("broker allowed (no rule denied)",
			"toolName", headers[HeaderToolName],
			"toolRegistry", headers[HeaderToolRegistry],
			"matchedPolicy", decision.Policy,
		)
		return
	}

	logger.Info(logMsgBrokerPolicyDecision,
		"allowed", decision.Allowed,
		"deniedBy", decision.DeniedBy,
		"message", decision.Message,
		"mode", string(decision.Mode),
		"policy", decision.Policy,
	)

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
		h.logger.Error(err, "header injection evaluation failed")
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
