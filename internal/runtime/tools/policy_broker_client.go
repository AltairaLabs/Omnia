/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/pkg/policy"
)

// Environment variables that configure the policy broker client. See
// docs/local-backlog/2026-07-05-toolpolicy-enforcement-phase2-design.md.
const (
	// envPolicyBrokerURL is the base URL of the policy-broker decision
	// endpoint (e.g. "http://localhost:8083"). Empty disables the client.
	envPolicyBrokerURL = "POLICY_BROKER_URL"
	// envPolicyBrokerFailMode selects behavior when the broker is
	// unreachable: "closed" (default, deny) or "open" (allow).
	envPolicyBrokerFailMode = "POLICY_BROKER_FAIL_MODE"
)

const (
	policyBrokerFailModeOpen = "open"

	policyBrokerDecisionPath = "/v1/decision"
	policyBrokerTimeout      = 2 * time.Second

	// policyDeniedByTransport tags a synthetic decision produced because the
	// broker could not be reached, distinguishing it from a real rule denial
	// (DecisionResponse.DeniedBy normally names a ToolPolicy rule).
	policyDeniedByTransport = "policy-broker-unreachable"

	logMsgPolicyBrokerUnreachable = "policy broker unreachable"
)

// errPolicyDenied is wrapped into the error dispatch returns when the
// policy broker denies a tool call, so callers can identify a
// policy-denied abort (as opposed to a transport/handler error) with
// errors.Is.
var errPolicyDenied = errors.New("tool call denied by policy")

// PolicyBrokerClient calls the ToolPolicy decision broker (P2.1,
// ee/pkg/policy/broker.go) for a per-tool-call allow/deny + header-injection
// decision. When POLICY_BROKER_URL is unset, the client is disabled: Decide
// always returns an allow with no injected headers, so deployments that
// don't run a broker (non-enterprise, or enterprise without ToolPolicy) see
// zero behavior change.
type PolicyBrokerClient struct {
	url      string
	failOpen bool
	client   *http.Client
	log      logr.Logger
}

// NewPolicyBrokerClient builds a client from POLICY_BROKER_URL and
// POLICY_BROKER_FAIL_MODE (default fail-closed — an enforcement layer that
// silently no-ops when its decision service is down is the bug this phase
// fixes, so the secure default is deny).
func NewPolicyBrokerClient(log logr.Logger) *PolicyBrokerClient {
	return &PolicyBrokerClient{
		url:      os.Getenv(envPolicyBrokerURL),
		failOpen: os.Getenv(envPolicyBrokerFailMode) == policyBrokerFailModeOpen,
		client:   &http.Client{Timeout: policyBrokerTimeout},
		log:      log.WithName("policy-broker-client"),
	}
}

// Enabled reports whether a broker URL is configured.
func (c *PolicyBrokerClient) Enabled() bool {
	return c.url != ""
}

// Decide asks the broker whether a tool call may proceed. When the client is
// disabled, it returns a synthetic allow with no injected headers. On
// transport failure (timeout, connection refused, non-200 response, bad
// JSON), it logs the failure and returns a synthetic decision per the
// configured fail mode — deny (fail-closed, default) or allow (fail-open).
// The returned error is non-nil only for caller-side mistakes (e.g. failing
// to marshal args); transport failures are already resolved into a
// decision and never surface as an error here.
func (c *PolicyBrokerClient) Decide(
	ctx context.Context,
	toolName, registryName string,
	args json.RawMessage,
) (*policy.DecisionResponse, error) {
	if !c.Enabled() {
		return &policy.DecisionResponse{Allow: true}, nil
	}

	argsMap := decodeArgsMap(args)
	reqBody := policy.DecisionRequest{
		Headers:  buildDecisionHeaders(ctx, toolName, registryName, argsMap),
		Body:     argsMap,
		Identity: policy.IdentityPayloadFromIdentity(policy.IdentityFromContext(ctx)),
	}

	decision, err := c.post(ctx, reqBody)
	if err != nil {
		c.log.Error(err, logMsgPolicyBrokerUnreachable,
			"toolName", toolName,
			"registryName", registryName,
			"failOpen", c.failOpen)
		return c.failureDecision(), nil
	}
	return decision, nil
}

// decodeArgsMap best-effort unmarshals tool arguments into a map for the
// DecisionRequest body and header-promotion. Returns nil (not an error) on
// empty or malformed args — an empty body is a valid decision request.
func decodeArgsMap(args json.RawMessage) map[string]any {
	if len(args) == 0 {
		return nil
	}
	var argsMap map[string]any
	if err := json.Unmarshal(args, &argsMap); err != nil {
		return nil
	}
	return argsMap
}

// buildDecisionHeaders assembles the same outbound policy/tool/param headers
// an HTTP tool call would carry (SetAllOutboundHeaders), canonicalized via a
// scratch *http.Request so keys match the "X-Omnia-Tool-Name" casing the
// broker's CEL evaluator expects. Handler-specific static/auth headers are
// deliberately excluded — the broker only needs identity/tool/param context,
// not downstream credentials.
func buildDecisionHeaders(ctx context.Context, toolName, registryName string, argsMap map[string]any) map[string]string {
	req := &http.Request{Header: http.Header{}}
	SetAllOutboundHeaders(ctx, req, toolName, registryName, argsMap)
	headers := make(map[string]string, len(req.Header))
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	return headers
}

// post sends the decision request and decodes the response. Any error
// (marshal, network, non-200, decode) is returned unwrapped so Decide can
// log it once with context.
func (c *PolicyBrokerClient) post(ctx context.Context, reqBody policy.DecisionRequest) (*policy.DecisionResponse, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal decision request: %w", err)
	}

	postCtx, cancel := context.WithTimeout(ctx, policyBrokerTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(postCtx, http.MethodPost, c.url+policyBrokerDecisionPath, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build decision request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("decision request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("decision endpoint returned status %d", resp.StatusCode)
	}

	var decision policy.DecisionResponse
	if err := json.NewDecoder(resp.Body).Decode(&decision); err != nil {
		return nil, fmt.Errorf("decode decision response: %w", err)
	}
	return &decision, nil
}

// failureDecision synthesizes a decision for a broker transport failure,
// per the configured fail mode.
func (c *PolicyBrokerClient) failureDecision() *policy.DecisionResponse {
	if c.failOpen {
		return &policy.DecisionResponse{Allow: true}
	}
	return &policy.DecisionResponse{
		Allow:    false,
		DeniedBy: policyDeniedByTransport,
		Message:  "policy broker unreachable; failing closed",
	}
}
