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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/pkg/policy"
	toolsv1 "github.com/altairalabs/omnia/pkg/tools/v1"
)

// newHTTPToolServer builds a tool backend that records the last request's
// headers and echoes a fixed JSON body.
func newHTTPToolServer(t *testing.T, captured *http.Header) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*captured = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
}

// newHTTPToolExecutor builds an OmniaExecutor with a single HTTP tool
// pointed at toolSrv, so dispatch is exercised end-to-end through
// ExecuteTool.
func newHTTPToolExecutor(toolSrv *httptest.Server) *OmniaExecutor {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.handlers["test-http"] = &HandlerEntry{
		Name: "test-http",
		Type: ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{
			Endpoint: toolSrv.URL,
			Method:   "POST",
		},
		Tool: &ToolDefCfg{Name: "test-http-tool", Description: "test tool"},
	}
	e.toolHandlers["test-http-tool"] = "test-http"
	return e
}

// TestNewOmniaExecutor_WiresPolicyBrokerClient is the wiring-test companion
// to the stale TODO removed from cmd/runtime/wiring_test.go (#728 item 4):
// it proves every OmniaExecutor built by internal/runtime.Server.InitializeTools
// (server.go, the real runtime construction path) carries a non-nil
// PolicyBrokerClient, and that the client's enabled/disabled state tracks
// POLICY_BROKER_URL exactly as NewPolicyBrokerClient documents. This is a
// pure construction-time assertion (no network call) — the behavioral proof
// that a wired, enabled client actually blocks/allows tool calls is covered
// by the TestDispatch_PolicyBroker* tests below and, against a real broker +
// real CEL evaluation, by test/integration/policy_broker_test.go.
func TestNewOmniaExecutor_WiresPolicyBrokerClient(t *testing.T) {
	t.Run("broker_url_unset_client_present_but_disabled", func(t *testing.T) {
		t.Setenv(envPolicyBrokerURL, "")
		e := NewOmniaExecutor(logr.Discard(), nil)
		require.NotNil(t, e.policyBroker, "NewOmniaExecutor must always wire a PolicyBrokerClient")
		assert.False(t, e.policyBroker.Enabled())
	})

	t.Run("broker_url_set_client_enabled", func(t *testing.T) {
		t.Setenv(envPolicyBrokerURL, "http://example-broker:8083")
		e := NewOmniaExecutor(logr.Discard(), nil)
		require.NotNil(t, e.policyBroker)
		assert.True(t, e.policyBroker.Enabled())
	})
}

func TestDispatch_PolicyBrokerDeny_AbortsCall(t *testing.T) {
	toolCalled := false
	toolSrv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		toolCalled = true
	}))
	defer toolSrv.Close()

	brokerSrv := httptest.NewServer(jsonHandler(t, `{"allow":false,"deniedBy":"deny-all","message":"nope"}`))
	defer brokerSrv.Close()
	t.Setenv(envPolicyBrokerURL, brokerSrv.URL)

	e := newHTTPToolExecutor(toolSrv)

	_, err := e.ExecuteTool(context.Background(), "test-http-tool", json.RawMessage(`{}`))
	require.Error(t, err)
	assert.True(t, errors.Is(err, errPolicyDenied), "expected errPolicyDenied, got %v", err)
	assert.Contains(t, err.Error(), "deny-all")
	assert.False(t, toolCalled, "tool backend must not be called when the broker denies")
}

// roleGatedBrokerHandler simulates a ToolPolicy rule keyed on
// `identity.claims.role` (e.g. `identity.claims.role != "admin"` denies): it
// decodes the DecisionRequest identity payload and denies unless the
// propagated role claim matches requiredRole.
func roleGatedBrokerHandler(requiredRole string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req policy.DecisionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if req.Identity != nil && req.Identity.Claims["role"] == requiredRole {
			_, _ = w.Write([]byte(`{"allow":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"allow":false,"deniedBy":"role-gate","message":"role mismatch"}`))
	}
}

// TestDispatch_PolicyBrokerDeny_IdentityRoleFromPropagation_AbortsCall proves
// the CRITICAL fix: an identity.claims.role-gated ToolPolicy rule actually
// denies when the caller's role claim arrives via the runtime's propagated
// PropagationFields (the real production path — extractPolicyFromMetadata
// rehydrating gRPC metadata), not via policy.WithIdentity (which is
// facade-only and never reaches the runtime). Before the fix, the broker
// client always sent Identity: nil in this scenario, so a role-gated rule
// could never fire.
func TestDispatch_PolicyBrokerDeny_IdentityRoleFromPropagation_AbortsCall(t *testing.T) {
	toolCalled := false
	toolSrv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		toolCalled = true
	}))
	defer toolSrv.Close()

	brokerSrv := httptest.NewServer(roleGatedBrokerHandler(policy.RoleAdmin))
	defer brokerSrv.Close()
	t.Setenv(envPolicyBrokerURL, brokerSrv.URL)

	e := newHTTPToolExecutor(toolSrv)

	// Mirrors what internal/runtime/interceptor.go's extractPolicyFromMetadata
	// does on every inbound gRPC call: rehydrate PropagationFields from
	// metadata, not an in-process AuthenticatedIdentity.
	ctx := policy.WithPropagationFields(context.Background(), &policy.PropagationFields{
		UserID: "user-1",
		Claims: map[string]string{"role": policy.RoleViewer},
	})

	_, err := e.ExecuteTool(ctx, "test-http-tool", json.RawMessage(`{}`))
	require.Error(t, err)
	assert.True(t, errors.Is(err, errPolicyDenied), "expected errPolicyDenied, got %v", err)
	assert.Contains(t, err.Error(), "role-gate")
	assert.False(t, toolCalled, "tool backend must not be called when identity.claims.role gates the call")
}

// TestDispatch_PolicyBrokerAllow_IdentityRoleFromPropagation_ProceedsWithCall
// is the allow-side complement: the same role-gated rule proceeds when the
// propagated role matches, proving the mapping is faithful (not just
// fail-closed by accident).
func TestDispatch_PolicyBrokerAllow_IdentityRoleFromPropagation_ProceedsWithCall(t *testing.T) {
	toolCalled := false
	toolSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		toolCalled = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer toolSrv.Close()

	brokerSrv := httptest.NewServer(roleGatedBrokerHandler(policy.RoleAdmin))
	defer brokerSrv.Close()
	t.Setenv(envPolicyBrokerURL, brokerSrv.URL)

	e := newHTTPToolExecutor(toolSrv)

	ctx := policy.WithPropagationFields(context.Background(), &policy.PropagationFields{
		UserID: "user-1",
		Claims: map[string]string{"role": policy.RoleAdmin},
	})

	_, err := e.ExecuteTool(ctx, "test-http-tool", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.True(t, toolCalled, "tool backend must be called when identity.claims.role matches")
}

func TestDispatch_PolicyBrokerAllowWithInjectedHeaders_ReachesOutboundRequest(t *testing.T) {
	var captured http.Header
	toolSrv := newHTTPToolServer(t, &captured)
	defer toolSrv.Close()

	brokerSrv := httptest.NewServer(jsonHandler(t, `{"allow":true,"injectedHeaders":{"X-Injected-Auth":"secret-token"}}`))
	defer brokerSrv.Close()
	t.Setenv(envPolicyBrokerURL, brokerSrv.URL)

	e := newHTTPToolExecutor(toolSrv)

	result, err := e.ExecuteTool(context.Background(), "test-http-tool", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Contains(t, string(result), "ok")
	assert.Equal(t, "secret-token", captured.Get("X-Injected-Auth"))
}

// newGRPCToolExecutor builds an OmniaExecutor with a single gRPC tool
// backed by mock, so dispatch is exercised end-to-end through ExecuteTool —
// the gRPC mirror of newHTTPToolExecutor.
func newGRPCToolExecutor(mock *mockToolServiceClient) *OmniaExecutor {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.grpcClients["test-grpc"] = mock
	e.handlers["test-grpc"] = &HandlerEntry{Name: "test-grpc", Type: ToolTypeGRPC}
	e.toolHandlers["test-grpc-tool"] = "test-grpc"
	return e
}

// TestDispatch_PolicyBrokerAllowWithInjectedHeaders_ReachesOutboundGRPCMetadata
// is the gRPC mirror of TestDispatch_PolicyBrokerAllowWithInjectedHeaders_ReachesOutboundRequest
// (finding: the HTTP path had metadata-merge coverage via a header-capturing
// httptest server, but omnia_executor_grpc.go's InjectedHeadersFromContext
// merge into outgoing gRPC metadata had none — the old mockToolServiceClient
// discarded ctx entirely). Asserts a broker-injected header both reaches the
// gRPC call and wins over a colliding policy-propagated header.
func TestDispatch_PolicyBrokerAllowWithInjectedHeaders_ReachesOutboundGRPCMetadata(t *testing.T) {
	brokerSrv := httptest.NewServer(jsonHandler(t, `{"allow":true,"injectedHeaders":{"x-omnia-user-id":"injected-user","X-Injected-Auth":"secret-token"}}`))
	defer brokerSrv.Close()
	t.Setenv(envPolicyBrokerURL, brokerSrv.URL)

	mock := &mockToolServiceClient{executeResp: &toolsv1.ToolResponse{ResultJson: `{"result":"ok"}`}}
	e := newGRPCToolExecutor(mock)

	// Seed a policy-propagated user-id header so the injected header has
	// something to collide with.
	ctx := policy.WithPropagationFields(context.Background(), &policy.PropagationFields{
		UserID: "original-user",
	})

	result, err := e.ExecuteTool(ctx, "test-grpc-tool", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Contains(t, string(result), "ok")

	require.NotNil(t, mock.capturedMD)
	assert.Equal(t, []string{"secret-token"}, mock.capturedMD.Get("X-Injected-Auth"),
		"broker-injected header must reach outgoing gRPC metadata")
	assert.Equal(t, []string{"injected-user"}, mock.capturedMD.Get(policy.HeaderUserID),
		"broker-injected header must win over the colliding policy-propagated header")
}

// TestDispatch_GRPCToolAuth_ReachesOutboundMetadata asserts a resolved gRPC tool
// credential is sent as the outgoing "authorization" metadata.
func TestDispatch_GRPCToolAuth_ReachesOutboundMetadata(t *testing.T) {
	mock := &mockToolServiceClient{executeResp: &toolsv1.ToolResponse{ResultJson: `{"result":"ok"}`}}
	e := newGRPCToolExecutor(mock)
	e.handlers["test-grpc"].GRPCConfig = &GRPCCfg{Endpoint: "svc:9000", AuthType: "bearer", AuthToken: "grpc-tok"}

	result, err := e.ExecuteTool(context.Background(), "test-grpc-tool", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Contains(t, string(result), "ok")
	require.NotNil(t, mock.capturedMD)
	assert.Equal(t, []string{"Bearer grpc-tok"}, mock.capturedMD.Get("authorization"),
		"gRPC tool credential must reach the outgoing authorization metadata")
}

const mcpInjectedHeaderToolName = "test-mcp-tool"

// newMCPToolServer builds a real Streamable-HTTP MCP server (single echo
// tool) wrapped in a middleware that records the headers of the tools/call
// JSON-RPC request specifically. Filtering on the JSON-RPC method (rather
// than capturing every request) keeps the assertion deterministic: the
// client also issues initialize / tools/list POSTs and, unless
// DisableStandaloneSSE is set, a background GET for the standalone SSE
// stream, any of which would otherwise race with and overwrite the captured
// headers.
func newMCPToolServer(t *testing.T, captured *http.Header) *httptest.Server {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "test-mcp-server", Version: "v0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: mcpInjectedHeaderToolName},
		func(_ context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
		})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(body))
			if strings.Contains(string(body), `"method":"tools/call"`) {
				*captured = r.Header.Clone()
			}
		}
		handler.ServeHTTP(w, r)
	}))
}

// newMCPToolExecutor builds an OmniaExecutor with a single MCP tool backed by
// mcpSrv, connecting for real (LoadConfigFromEntries + Initialize) so
// dispatch is exercised end-to-end through ExecuteTool — the MCP mirror of
// newHTTPToolExecutor / newGRPCToolExecutor.
func newMCPToolExecutor(t *testing.T, mcpSrv *httptest.Server) *OmniaExecutor {
	t.Helper()
	e := NewOmniaExecutor(logr.Discard(), nil)
	require.NoError(t, e.LoadConfigFromEntries([]HandlerEntry{{
		Name: "test-mcp",
		Type: ToolTypeMCP,
		MCPConfig: &MCPCfg{
			Transport: string(MCPTransportStreamableHTTP),
			Endpoint:  mcpSrv.URL,
		},
	}}))
	require.NoError(t, e.Initialize(context.Background()))
	return e
}

// TestDispatch_PolicyBrokerAllowWithInjectedHeaders_ReachesOutboundMCPRequest
// is the MCP mirror of TestDispatch_PolicyBrokerAllowWithInjectedHeaders_ReachesOutboundRequest
// / _ReachesOutboundGRPCMetadata (finding: the MCP path had no
// InjectedHeadersFromContext merge at all — the SSE/Streamable-HTTP MCP
// transports never saw ToolPolicy broker-injected headers, a pre-existing
// parity gap against HTTP/gRPC/OpenAPI). Asserts a broker-injected header
// reaches the outbound MCP tools/call HTTP request.
func TestDispatch_PolicyBrokerAllowWithInjectedHeaders_ReachesOutboundMCPRequest(t *testing.T) {
	var captured http.Header
	mcpSrv := newMCPToolServer(t, &captured)
	defer mcpSrv.Close()

	brokerSrv := httptest.NewServer(jsonHandler(t, `{"allow":true,"injectedHeaders":{"X-Injected-Auth":"secret-token"}}`))
	defer brokerSrv.Close()
	t.Setenv(envPolicyBrokerURL, brokerSrv.URL)

	e := newMCPToolExecutor(t, mcpSrv)
	defer func() { _ = e.Close() }()

	result, err := e.ExecuteTool(context.Background(), mcpInjectedHeaderToolName, json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Contains(t, string(result), "ok")
	assert.Equal(t, "secret-token", captured.Get("X-Injected-Auth"))
}

// recordingRoundTripper is a minimal http.RoundTripper test double that
// records the last request it saw and returns a canned 200 response.
type recordingRoundTripper struct {
	lastReq *http.Request
}

func (rt *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.lastReq = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Header:     make(http.Header),
	}, nil
}

// TestInjectedHeaderTransport_RoundTrip unit-tests the MCP-specific
// injection point directly: unlike buildHTTPHeaders/executeGRPC, the MCP
// client/session is built once at handler init and reused for every call, so
// injectedHeaderTransport (a ctx-aware http.RoundTripper) is where broker
// headers get merged in instead. Proves both reach and collision precedence
// without depending on real MCP protocol headers (Mcp-Session-Id,
// Mcp-Protocol-Version), which would break the handshake if overridden.
func TestInjectedHeaderTransport_RoundTrip(t *testing.T) {
	t.Run("no injected headers passes the request through unchanged", func(t *testing.T) {
		base := &recordingRoundTripper{}
		rt := &injectedHeaderTransport{base: base}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://mcp.example.invalid", nil)
		require.NoError(t, err)
		req.Header.Set("X-Existing", "orig")

		_, err = rt.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, "orig", base.lastReq.Header.Get("X-Existing"))
	})

	t.Run("injected headers merge in and win on collision", func(t *testing.T) {
		base := &recordingRoundTripper{}
		rt := &injectedHeaderTransport{base: base}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://mcp.example.invalid", nil)
		require.NoError(t, err)
		req.Header.Set("X-Existing", "orig")
		ctx := WithInjectedHeaders(req.Context(), map[string]string{
			"X-Existing":      "broker-wins",
			"X-Injected-Auth": "secret-token",
		})
		req = req.WithContext(ctx)

		_, err = rt.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, "broker-wins", base.lastReq.Header.Get("X-Existing"),
			"broker-injected header must win over the colliding pre-existing header")
		assert.Equal(t, "secret-token", base.lastReq.Header.Get("X-Injected-Auth"))
	})

	t.Run("static tool credential is applied", func(t *testing.T) {
		base := &recordingRoundTripper{}
		rt := &injectedHeaderTransport{base: base, authType: "bearer", authToken: "mcp-tok"}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://mcp.example.invalid", nil)
		require.NoError(t, err)

		_, err = rt.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, "Bearer mcp-tok", base.lastReq.Header.Get("Authorization"))
	})

	t.Run("broker-injected Authorization wins over the tool credential", func(t *testing.T) {
		base := &recordingRoundTripper{}
		rt := &injectedHeaderTransport{base: base, authType: "bearer", authToken: "tool-tok"}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://mcp.example.invalid", nil)
		require.NoError(t, err)
		req = req.WithContext(WithInjectedHeaders(req.Context(), map[string]string{"Authorization": "Bearer broker-tok"}))

		_, err = rt.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, "Bearer broker-tok", base.lastReq.Header.Get("Authorization"),
			"an explicit ToolPolicy enforcement decision must win over the static tool credential")
	})
}

func TestDispatch_PolicyBrokerAuditWouldDeny_ProceedsWithCall(t *testing.T) {
	toolCalled := false
	toolSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		toolCalled = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer toolSrv.Close()

	brokerSrv := httptest.NewServer(jsonHandler(t, `{"allow":false,"wouldDeny":true,"deniedBy":"deny-all","mode":"audit"}`))
	defer brokerSrv.Close()
	t.Setenv(envPolicyBrokerURL, brokerSrv.URL)

	e := newHTTPToolExecutor(toolSrv)

	_, err := e.ExecuteTool(context.Background(), "test-http-tool", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.True(t, toolCalled, "audit-mode wouldDeny must not block the call")
}

func TestDispatch_PolicyBrokerDisabled_NoBehaviorChange(t *testing.T) {
	t.Setenv(envPolicyBrokerURL, "")

	var captured http.Header
	toolSrv := newHTTPToolServer(t, &captured)
	defer toolSrv.Close()

	e := newHTTPToolExecutor(toolSrv)

	result, err := e.ExecuteTool(context.Background(), "test-http-tool", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Contains(t, string(result), "ok")
}

// TestDispatch_RegistryConfiguredButIdentityUnknown_FailsClosed proves the
// #1874 hardening: when a ToolRegistry is configured for the agent but a tool's
// registry identity cannot be resolved (no per-tool ToolMeta), enforcePolicy
// DENIES rather than falling back to the handler name — which would let a
// registry-scoped ToolPolicy silently not match and allow the call. No broker
// is needed: the deny happens before the broker is consulted.
func TestDispatch_RegistryConfiguredButIdentityUnknown_FailsClosed(t *testing.T) {
	t.Setenv(envPolicyBrokerURL, "")

	toolCalled := false
	toolSrv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		toolCalled = true
	}))
	defer toolSrv.Close()

	e := NewOmniaExecutor(logr.Discard(), nil)
	// A ToolRegistry is configured for the agent. Recording it with no handler
	// entries (and before any tool is registered) leaves per-tool ToolMeta
	// unset — the "registry configured but identity unknown" state.
	e.SetRegistryInfo("orders", "agent-ns", nil)
	e.handlers["test-http"] = &HandlerEntry{
		Name:       "test-http",
		Type:       ToolTypeHTTP,
		HTTPConfig: &HTTPCfg{Endpoint: toolSrv.URL, Method: "POST"},
		Tool:       &ToolDefCfg{Name: "test-http-tool", Description: "test tool"},
	}
	e.toolHandlers["test-http-tool"] = "test-http"

	_, err := e.ExecuteTool(context.Background(), "test-http-tool", json.RawMessage(`{}`))
	require.Error(t, err)
	assert.True(t, errors.Is(err, errPolicyDenied), "expected errPolicyDenied, got %v", err)
	assert.False(t, toolCalled, "tool backend must not be called when registry identity is unknown")
}
