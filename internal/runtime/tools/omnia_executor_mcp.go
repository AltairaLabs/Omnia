/*
Copyright 2025.

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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"

	pktools "github.com/AltairaLabs/PromptKit/runtime/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// buildMCPDescriptor populates the descriptor from discovered MCP tools.
func (e *OmniaExecutor) buildMCPDescriptor(desc *pktools.ToolDescriptor, toolName, handlerName string) {
	tools, ok := e.mcpTools[handlerName]
	if !ok {
		return
	}
	tool, ok := tools[toolName]
	if !ok {
		return
	}
	desc.Description = tool.Description
	desc.InputSchema = marshalSchema(tool.InputSchema)
}

func (e *OmniaExecutor) initMCPHandler(ctx context.Context, name string, h *HandlerEntry) error {
	if h.MCPConfig == nil {
		e.log.Info("skipping MCP handler without config", "handler", name)
		return nil
	}

	client := mcp.NewClient(
		&mcp.Implementation{Name: "omnia-runtime", Version: "v1.0.0"},
		nil,
	)

	transport, err := e.buildMCPTransport(h.MCPConfig)
	if err != nil {
		return err
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("failed to connect MCP: %w", err)
	}

	e.mcpClients[name] = client
	e.mcpSessions[name] = session
	e.mcpTools[name] = make(map[string]*mcp.Tool)

	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			e.log.Error(err, "failed to list MCP tool", "handler", name)
			continue
		}
		if h.MCPConfig.ToolFilter != nil && !h.MCPConfig.ToolFilter.Includes(tool.Name) {
			e.log.V(1).Info("filtered out MCP tool", "tool", tool.Name, "handler", name)
			continue
		}
		e.mcpTools[name][tool.Name] = tool
		e.toolHandlers[tool.Name] = name
		e.log.V(1).Info("registered MCP tool", "tool", tool.Name, "handler", name)
	}

	return nil
}

func (e *OmniaExecutor) buildMCPTransport(cfg *MCPCfg) (mcp.Transport, error) {
	tr := &injectedHeaderTransport{}
	if cfg.AuthType == authTypeWorkloadIdentity {
		// workloadIdentity tokens expire (~1h) but the MCP transport/session is
		// built once at handler init and reused for the connection's life, so
		// the token cannot be pre-computed here — resolve it per-request in
		// RoundTrip instead (the acquirer caches, so this is cheap).
		tr.acquirer = e.tokenAcquirer
		tr.wifCloud, tr.wifAudience, tr.wifHeader = cfg.AuthCloud, cfg.AuthAudience, cfg.AuthHeader
	} else {
		authHeader, err := authorizationValue(cfg.AuthType, cfg.AuthToken)
		if err != nil {
			return nil, fmt.Errorf("MCP tool auth: %w", err)
		}
		tr.authHeader = authHeader
	}
	switch MCPTransportType(cfg.Transport) {
	case MCPTransportSSE:
		return &mcp.SSEClientTransport{
			Endpoint:   cfg.Endpoint,
			HTTPClient: &http.Client{Transport: tr},
		}, nil
	case MCPTransportStreamableHTTP:
		return &mcp.StreamableClientTransport{
			Endpoint:   cfg.Endpoint,
			HTTPClient: &http.Client{Transport: tr},
		}, nil
	case MCPTransportStdio:
		// Stdio MCP has no header/metadata channel: the subprocess speaks
		// JSON-RPC over stdin/stdout, so there is no outbound HTTP request to
		// merge InjectedHeadersFromContext into. ToolPolicy broker-injected
		// headers cannot reach a stdio MCP handler.
		cmd := exec.Command(cfg.Command, cfg.Args...)
		if cfg.WorkDir != "" {
			cmd.Dir = cfg.WorkDir
		}
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
		return &mcp.CommandTransport{Command: cmd}, nil
	default:
		return nil, fmt.Errorf("unsupported MCP transport: %s", cfg.Transport)
	}
}

// injectedHeaderTransport wraps an http.RoundTripper and merges ToolPolicy
// broker-injected headers (stashed on the request's context via
// WithInjectedHeaders) into every outbound MCP HTTP request. It gives the SSE
// and Streamable-HTTP MCP transports parity with the HTTP/gRPC executors.
//
// Unlike buildHTTPHeaders and executeGRPC's metadata merge, the MCP
// client/session is built once at handler init (initMCPHandler) and reused
// for every subsequent tool call, so there is no per-call header-building
// hook on the SDK's transport types. The go-sdk/mcp package does thread each
// JSON-RPC call's ctx through to http.NewRequestWithContext for every request
// it sends (streamableClientConn.Write, sseClientConn's message POST), so a
// ctx-aware RoundTripper is the per-call injection point available for this
// transport.
type injectedHeaderTransport struct {
	base http.RoundTripper
	// authHeader is the pre-computed static tool-credential Authorization value
	// ("Bearer <token>"/"Basic <...>"), or "" when the handler has no static auth.
	authHeader string
	// WIF fields (set only for auth.type workloadIdentity): the token is
	// acquired per-request in RoundTrip because it expires and the transport
	// outlives it.
	acquirer    TokenAcquirer
	wifCloud    string
	wifAudience string
	wifHeader   string
}

// RoundTrip resolves workloadIdentity auth fresh (if configured), applies the
// tool credential, then merges broker-injected headers last so they win over
// the tool credential (and any transport-set headers) on key collision —
// mirrors the ordering in buildHTTPHeaders and executeGRPC.
func (t *injectedHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	var wifName, wifVal string
	if t.wifAudience != "" {
		n, v, err := resolveWorkloadIdentityHeader(req.Context(), t.acquirer, t.wifCloud, t.wifAudience, t.wifHeader)
		if err != nil {
			return nil, fmt.Errorf("MCP workloadIdentity auth: %w", err)
		}
		wifName, wifVal = n, v
	}

	headers := InjectedHeadersFromContext(req.Context())
	if t.authHeader == "" && wifVal == "" && len(headers) == 0 {
		return base.RoundTrip(req)
	}

	req = req.Clone(req.Context())
	if t.authHeader != "" {
		req.Header.Set("Authorization", t.authHeader)
	}
	if wifVal != "" {
		req.Header.Set(wifName, wifVal)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return base.RoundTrip(req)
}

func (e *OmniaExecutor) executeMCP(
	ctx context.Context,
	toolName, handlerName string,
	args json.RawMessage,
) (json.RawMessage, error) {
	e.mu.RLock()
	session := e.mcpSessions[handlerName]
	handler := e.handlers[handlerName]
	e.mu.RUnlock()

	if session == nil {
		return nil, fmt.Errorf("MCP handler %q not connected", handlerName)
	}

	policy, classify := mcpRetryParams(handler.MCPConfig)

	return retryWithBackoff(ctx, e.log, e.currentSpan(ctx), policy, handler.Timeout.Get(), classify,
		func(attemptCtx context.Context) (json.RawMessage, error) {
			argsMap, err := parseMCPArgs(args)
			if err != nil {
				return nil, err
			}
			return e.callMCPTool(attemptCtx, session, toolName, argsMap)
		},
	)
}

// parseMCPArgs unmarshals the raw tool arguments into a map.
func parseMCPArgs(args json.RawMessage) (map[string]any, error) {
	var argsMap map[string]any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &argsMap); err != nil {
			return nil, fmt.Errorf("failed to parse MCP args: %w", err)
		}
	}
	return argsMap, nil
}

// callMCPTool runs a single MCP tool call through the circuit breaker. MCP
// tool errors are wrapped as clientError so the breaker doesn't trip on them.
func (e *OmniaExecutor) callMCPTool(
	ctx context.Context,
	session *mcp.ClientSession,
	toolName string,
	argsMap map[string]any,
) (json.RawMessage, error) {
	var mcpResult json.RawMessage
	_, cbErr := e.breakers.Execute(toolName, func() ([]byte, error) {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      toolName,
			Arguments: argsMap,
		})
		if err != nil {
			return nil, fmt.Errorf("MCP tool call failed: %w", err)
		}

		// Convert MCP tool errors to mcpToolError so the classifier
		// can distinguish them from transport errors.
		if result.IsError {
			return nil, &clientError{err: &mcpToolError{message: mcpErrorMessage(result)}}
		}

		var marshalErr error
		mcpResult, marshalErr = marshalMCPResult(result)
		return nil, marshalErr
	})
	if cbErr != nil {
		return nil, cbErr
	}
	return mcpResult, nil
}

// mcpErrorMessage extracts a human-readable error message from an MCP result.
func mcpErrorMessage(result *mcp.CallToolResult) string {
	msg := "MCP tool returned error"
	if len(result.Content) > 0 {
		if tc, ok := result.Content[0].(*mcp.TextContent); ok && tc.Text != "" {
			msg = tc.Text
		}
	}
	return msg
}

func marshalMCPResult(result *mcp.CallToolResult) (json.RawMessage, error) {
	if result.IsError {
		errMsg := "MCP tool returned error"
		if len(result.Content) > 0 {
			if tc, ok := result.Content[0].(*mcp.TextContent); ok && tc.Text != "" {
				errMsg = tc.Text
			}
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	var content any
	if len(result.Content) == 1 {
		if tc, ok := result.Content[0].(*mcp.TextContent); ok {
			content = tc.Text
		} else {
			content = result.Content[0]
		}
	} else if result.StructuredContent != nil {
		content = result.StructuredContent
	} else if len(result.Content) > 0 {
		content = result.Content
	}

	return json.Marshal(content)
}
