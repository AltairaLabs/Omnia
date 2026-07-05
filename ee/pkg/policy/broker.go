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
	req, err := decodeDecisionRequest(r)
	if err != nil {
		h.logger.Debug("malformed decision request", "error", err.Error())
		writeMalformedRequestResponse(w)
		return
	}

	ctx := withIdentityFromPayload(r.Context(), req.Identity)

	decision := h.evaluator.EvaluateWithContext(ctx, req.Headers, req.Body)
	logDecision(h.logger, r, decision)

	injected := h.evaluateHeaderInjection(ctx, req)

	writeDecisionResponse(w, decision, injected)
}

// decodeDecisionRequest decodes the JSON request body into a DecisionRequest.
func decodeDecisionRequest(r *http.Request) (DecisionRequest, error) {
	var req DecisionRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
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
