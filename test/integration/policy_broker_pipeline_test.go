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
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	eepolicy "github.com/altairalabs/omnia/ee/pkg/policy"
	"github.com/altairalabs/omnia/internal/runtime"
	"github.com/altairalabs/omnia/internal/runtime/tools"
	ompolicy "github.com/altairalabs/omnia/pkg/policy"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// These tests drive the REAL Converse pipeline (mock model → SDK pipeline →
// OmniaExecutor → real PolicyBrokerClient → HTTP → real ee/pkg/policy broker →
// real Evaluator + real ToolPolicy CEL). No mock broker, no stubbed CEL, no
// ExecuteTool shortcut — the whole enforcement wire is production code, so these
// catch propagation/shape bugs that a direct-call test cannot (e.g. the registry
// name and identity claims actually surviving the runtime→broker hop).

const pipelineRegistry = "orders" // ToolRegistry name — intentionally != handler name "echo"

// pipelineStream is a minimal RuntimeService_ConverseServer feeding one client
// message. Replicated here because the internal/runtime helper is package-private.
type pipelineStream struct {
	runtimev1.RuntimeService_ConverseServer
	ctx  context.Context
	recv []*runtimev1.ClientMessage
	idx  int
}

func (s *pipelineStream) Context() context.Context { return s.ctx }
func (s *pipelineStream) Recv() (*runtimev1.ClientMessage, error) {
	if s.idx >= len(s.recv) {
		return nil, context.Canceled
	}
	m := s.recv[s.idx]
	s.idx++
	return m, nil
}
func (s *pipelineStream) Send(*runtimev1.ServerMessage) error { return nil }

// drivePipeline runs one tool call end-to-end against a real broker compiled
// with tp, with the mock scripting echo(amount). reqCtx carries identity
// propagation (nil = anonymous). Returns whether the upstream backend was hit
// (i.e. the call was allowed through).
func drivePipeline(t *testing.T, tp *omniav1alpha1.ToolPolicy, amount int, reqCtx context.Context) (upstreamHit bool) {
	t.Helper()
	eval, err := eepolicy.NewEvaluator()
	require.NoError(t, err)
	require.NoError(t, eval.CompilePolicy(tp))
	broker := httptest.NewServer(eepolicy.NewBrokerHandler(eval, logr.Discard()))
	t.Cleanup(broker.Close)
	t.Setenv("POLICY_BROKER_URL", broker.URL)
	t.Setenv("POLICY_BROKER_FAIL_MODE", "closed")
	return drivePipelineRaw(t, amount, reqCtx)
}

// drivePipelineRaw drives one tool call assuming POLICY_BROKER_URL /
// POLICY_BROKER_FAIL_MODE are already set (so fail-mode tests can point at a
// dead broker). Returns whether the upstream backend was hit.
func drivePipelineRaw(t *testing.T, amount int, reqCtx context.Context) (upstreamHit bool) {
	t.Helper()
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	t.Cleanup(upstream.Close)

	server := newPipelineServer(t, upstream.URL, amount)
	if reqCtx == nil {
		reqCtx = context.Background()
	}
	_ = server.Converse(&pipelineStream{
		ctx:  reqCtx,
		recv: []*runtimev1.ClientMessage{{SessionId: "s1", Content: "use the echo tool"}},
	})
	return hits.Load() > 0
}

// newPipelineServer builds a runtime.Server (mock model scripting echo(amount),
// http handler "echo" in registry "orders" pointing at upstreamURL) with tools
// initialized and registry metadata set as the operator would.
func newPipelineServer(t *testing.T, upstreamURL string, amount int) *runtime.Server {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pack.promptpack"), `{
		"id": "p", "name": "p", "version": "1.0.0",
		"template_engine": { "version": "v1", "syntax": "{{variable}}" },
		"prompts": { "default": { "id": "default", "name": "default", "version": "1.0.0",
			"system_template": "You use the echo tool." } }
	}`)
	writeFile(t, filepath.Join(dir, "mock.yaml"), fmt.Sprintf(`defaultResponse: "fallback"
scenarios:
  default:
    turns:
      1:
        type: tool_calls
        content: ""
        tool_calls:
          - name: echo
            arguments:
              amount: %d
      2:
        content: "Request processed."
`, amount))
	writeFile(t, filepath.Join(dir, "tools.yaml"), `handlers:
  - name: echo
    type: http
    httpConfig:
      endpoint: "`+upstreamURL+`"
      method: POST
      contentType: application/json
    tool:
      name: echo
      description: echo tool
      inputSchema:
        type: object
        properties:
          amount:
            type: number
        required: [amount]
    timeout: "10s"
`)

	server := runtime.NewServer(
		runtime.WithLogger(logr.Discard()),
		runtime.WithPackPath(filepath.Join(dir, "pack.promptpack")),
		runtime.WithPromptName("default"),
		runtime.WithMockProvider(true),
		runtime.WithMockConfigPath(filepath.Join(dir, "mock.yaml")),
		runtime.WithToolsConfig(filepath.Join(dir, "tools.yaml")),
	)
	require.NoError(t, server.InitializeTools(context.Background()))
	t.Cleanup(func() { _ = server.Close() })
	server.SetToolRegistryInfo(pipelineRegistry, "test-ns", []tools.HandlerEntry{{Name: "echo", Type: tools.ToolTypeHTTP}})
	return server
}

func amountPolicy(cel string) *omniav1alpha1.ToolPolicy {
	return &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "test-ns"},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{Registry: pipelineRegistry},
			Rules: []omniav1alpha1.PolicyRule{{
				Name: "rule",
				Deny: omniav1alpha1.PolicyRuleDeny{CEL: cel, Message: "denied"},
			}},
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// --- body-based (registry selector) -----------------------------------------

func TestPolicyBroker_Pipeline_RegistrySelectorEnforced(t *testing.T) {
	// The decision must carry the ToolRegistry NAME ("orders"), not the handler
	// name ("echo"), or this registry-scoped policy never matches (allow).
	hit := drivePipeline(t, amountPolicy(`has(body.amount) && double(body.amount) > 50.0`), 100, nil)
	require.False(t, hit, "amount=100 must be denied by registry-scoped amount>50 policy; upstream must not be hit")
}

func TestPolicyBroker_Pipeline_AllowPath(t *testing.T) {
	// Under the limit → allowed → the tool MUST reach the upstream.
	hit := drivePipeline(t, amountPolicy(`has(body.amount) && double(body.amount) > 50.0`), 10, nil)
	require.True(t, hit, "amount=10 is under the limit; the tool must reach the upstream")
}

// --- identity: role + claims (the token-claims path) ------------------------

func TestPolicyBroker_Pipeline_IdentityRoleEnforced(t *testing.T) {
	// deny unless the caller's role is "premium" — proves identity.role
	// propagates through the facade→runtime→broker hop and CEL evaluates it.
	pol := amountPolicy(`identity.role != "premium"`)

	t.Run("free_role_denied", func(t *testing.T) {
		ctx := ompolicy.WithUserRoles(ompolicy.WithUserID(context.Background(), "u1"), "free")
		require.False(t, drivePipeline(t, pol, 10, ctx),
			"role=free must be denied — identity.role rule not enforced (claims not propagating?)")
	})
	t.Run("premium_role_allowed", func(t *testing.T) {
		ctx := ompolicy.WithUserRoles(ompolicy.WithUserID(context.Background(), "u1"), "premium")
		require.True(t, drivePipeline(t, pol, 10, ctx),
			"role=premium must be allowed through to the upstream")
	})
}

func TestPolicyBroker_Pipeline_ClaimEnforced(t *testing.T) {
	// deny unless token claim tier == "premium" — proves identity.claims.*
	// (arbitrary JWT claims) survive the hop and are evaluable.
	pol := amountPolicy(`!has(identity.claims.tier) || identity.claims.tier != "premium"`)

	t.Run("free_tier_denied", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ompolicy.ContextKeyClaims, map[string]string{"tier": "free"})
		require.False(t, drivePipeline(t, pol, 10, ctx),
			"claim tier=free must be denied — identity.claims not propagating/evaluating")
	})
	t.Run("premium_tier_allowed", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ompolicy.ContextKeyClaims, map[string]string{"tier": "premium"})
		require.True(t, drivePipeline(t, pol, 10, ctx),
			"claim tier=premium must be allowed through")
	})
}

// --- mode: audit observes but does not block --------------------------------

func TestPolicyBroker_Pipeline_AuditModeDoesNotBlock(t *testing.T) {
	pol := amountPolicy(`has(body.amount) && double(body.amount) > 50.0`)
	pol.Spec.Mode = omniav1alpha1.PolicyModeAudit
	// amount=100 matches the deny rule, but in audit mode the call still proceeds.
	require.True(t, drivePipeline(t, pol, 100, nil),
		"audit mode must record a would-deny but still allow the tool through")
}

// --- fail modes when the broker is unreachable ------------------------------

func TestPolicyBroker_Pipeline_FailClosedDenies(t *testing.T) {
	t.Setenv("POLICY_BROKER_URL", "http://127.0.0.1:1") // nothing listening
	t.Setenv("POLICY_BROKER_FAIL_MODE", "closed")
	require.False(t, drivePipelineRaw(t, 10, nil),
		"broker unreachable + fail-closed must DENY (secure default) — upstream must not be hit")
}

func TestPolicyBroker_Pipeline_FailOpenAllows(t *testing.T) {
	t.Setenv("POLICY_BROKER_URL", "http://127.0.0.1:1")
	t.Setenv("POLICY_BROKER_FAIL_MODE", "open")
	require.True(t, drivePipelineRaw(t, 10, nil),
		"broker unreachable + fail-open must ALLOW — upstream must be hit")
}

// --- tool-name selector ------------------------------------------------------

func TestPolicyBroker_Pipeline_ToolSelector(t *testing.T) {
	base := func(tool string) *omniav1alpha1.ToolPolicy {
		p := amountPolicy(`has(body.amount) && double(body.amount) > 50.0`)
		p.Spec.Selector.Tools = []string{tool}
		return p
	}
	t.Run("matching_tool_denied", func(t *testing.T) {
		require.False(t, drivePipeline(t, base("echo"), 100, nil),
			"selector tools=[echo] must match the echo call and deny amount=100")
	})
	t.Run("nonmatching_tool_allowed", func(t *testing.T) {
		require.True(t, drivePipeline(t, base("other"), 100, nil),
			"selector tools=[other] must NOT match the echo call — policy does not apply, tool allowed")
	})
}

// --- edge cases: erroring / nonsense CEL, missing identity ------------------

func TestPolicyBroker_Pipeline_EvalErrorFailsClosed(t *testing.T) {
	// References a key that isn't in the body (no has() guard) → CEL runtime
	// error. With onFailure=deny (the secure default) the call MUST be denied.
	pol := amountPolicy(`body.nonexistent > 50.0`)
	require.False(t, drivePipeline(t, pol, 10, nil),
		"a rule that errors at eval must fail CLOSED (deny) under onFailure=deny")
}

func TestPolicyBroker_Pipeline_EvalErrorFailOpen(t *testing.T) {
	// Same erroring rule, but onFailure=allow → the call proceeds.
	pol := amountPolicy(`body.nonexistent > 50.0`)
	pol.Spec.OnFailure = omniav1alpha1.OnFailureAllow
	require.True(t, drivePipeline(t, pol, 10, nil),
		"onFailure=allow must let the call through when a rule errors")
}

func TestPolicyBroker_Pipeline_NonBoolCELFailsClosed(t *testing.T) {
	// A rule whose expression yields a non-bool (a number) is treated as an
	// eval error → fail closed under the default onFailure=deny.
	pol := amountPolicy(`body.amount`)
	require.False(t, drivePipeline(t, pol, 10, nil),
		"a rule returning a non-bool must fail CLOSED (deny)")
}

func TestPolicyBroker_Pipeline_AnonymousDeniedByIdentityRule(t *testing.T) {
	// No identity propagated → identity.role == "" → role != "premium" → deny.
	// (Guards against an identity rule silently allowing unauthenticated calls.)
	pol := amountPolicy(`identity.role != "premium"`)
	require.False(t, drivePipeline(t, pol, 10, nil),
		"an anonymous caller (no identity) must be denied by an identity.role rule")
}

// --- requiredClaims (a claim that must be present) --------------------------

func TestPolicyBroker_Pipeline_RequiredClaim(t *testing.T) {
	// requiredClaims gates on the X-Omnia-Claim-<claim> header being present.
	// The rule never denies (false); the gate is the required claim itself.
	pol := amountPolicy(`false`)
	pol.Spec.RequiredClaims = []omniav1alpha1.RequiredClaim{{Claim: "tier", Message: "tier claim required"}}

	t.Run("missing_claim_denied", func(t *testing.T) {
		require.False(t, drivePipeline(t, pol, 10, nil),
			"a call missing the required claim must be denied (upstream not hit)")
	})
	t.Run("present_claim_allowed", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ompolicy.ContextKeyClaims, map[string]string{"tier": "premium"})
		require.True(t, drivePipeline(t, pol, 10, ctx),
			"a call carrying the required claim (X-Omnia-Claim-tier) must be allowed through")
	})
}

// --- header injection (broker injects a header the tool receives) -----------

func TestPolicyBroker_Pipeline_HeaderInjectionReachesTool(t *testing.T) {
	eval, err := eepolicy.NewEvaluator()
	require.NoError(t, err)
	require.NoError(t, eval.CompilePolicy(&omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "inject", Namespace: "test-ns"},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{Registry: pipelineRegistry},
			Rules: []omniav1alpha1.PolicyRule{{
				Name: "allow-all", Deny: omniav1alpha1.PolicyRuleDeny{CEL: "false", Message: "n/a"},
			}},
			HeaderInjection: []omniav1alpha1.HeaderInjectionRule{{
				Header: "X-Injected-By-Policy", Value: "broker",
			}},
			Mode: omniav1alpha1.PolicyModeEnforce, OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}))
	broker := httptest.NewServer(eepolicy.NewBrokerHandler(eval, logr.Discard()))
	t.Cleanup(broker.Close)
	t.Setenv("POLICY_BROKER_URL", broker.URL)
	t.Setenv("POLICY_BROKER_FAIL_MODE", "closed")

	var gotHeader atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader.Store(r.Header.Get("X-Injected-By-Policy"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	t.Cleanup(upstream.Close)

	server := newPipelineServer(t, upstream.URL, 10)
	_ = server.Converse(&pipelineStream{
		ctx:  context.Background(),
		recv: []*runtimev1.ClientMessage{{SessionId: "s1", Content: "use the echo tool"}},
	})

	got, _ := gotHeader.Load().(string)
	require.Equal(t, "broker", got,
		"the ToolPolicy-injected header must reach the upstream tool call")
}
