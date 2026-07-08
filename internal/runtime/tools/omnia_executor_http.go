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
	"time"
)

// probeHandler performs a basic health check for a handler.
func (e *OmniaExecutor) probeHandler(ctx context.Context, handlerName string) error {
	h, ok := e.handlers[handlerName]
	if !ok {
		return fmt.Errorf("handler %q not found", handlerName)
	}

	switch h.Type {
	case ToolTypeHTTP, ToolTypeOpenAPI:
		return e.probeHTTP(ctx, h)
	default:
		return nil // MCP/gRPC health is implicit from connection state
	}
}

// probeHTTP performs a lightweight HTTP health probe.
func (e *OmniaExecutor) probeHTTP(ctx context.Context, h *HandlerEntry) error {
	endpoint := h.Endpoint
	if h.HTTPConfig != nil {
		endpoint = h.HTTPConfig.Endpoint
	}
	if endpoint == "" {
		return nil
	}

	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodHead, endpoint, nil)
	if err != nil {
		return fmt.Errorf("health check request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("health check: HTTP %d", resp.StatusCode)
	}
	return nil
}

// initClientHandler registers a client-side tool. No backend connection needed —
// the SDK handles routing to the client via ChunkClientTool.
func (e *OmniaExecutor) initClientHandler(name string, h *HandlerEntry) error {
	if h.Tool == nil {
		e.log.Info("skipping client handler without tool definition", "handler", name)
		return nil
	}
	toolName := h.Tool.Name
	e.toolHandlers[toolName] = name
	e.log.V(1).Info("registered client tool", "tool", toolName, "handler", name)
	return nil
}

func (e *OmniaExecutor) initHTTPHandler(name string, h *HandlerEntry) error {
	if h.HTTPConfig == nil || h.Tool == nil {
		e.log.Info("skipping HTTP handler without config or tool", "handler", name)
		return nil
	}
	toolName := h.Tool.Name
	e.toolHandlers[toolName] = name
	e.log.V(1).Info("registered HTTP tool", "tool", toolName, "handler", name)
	return nil
}

func (e *OmniaExecutor) executeHTTP(
	ctx context.Context,
	toolName, handlerName string,
	handler *HandlerEntry,
	args json.RawMessage,
) (json.RawMessage, error) {
	cfg := handler.HTTPConfig
	if cfg == nil {
		return nil, fmt.Errorf("handler %q has no HTTP config", handlerName)
	}

	return e.executeHTTPCall(ctx, toolName, handlerName, handler.Timeout.Get(), cfg, args)
}

// executeHTTPCall is the shared retry+breaker execution path for HTTP and
// OpenAPI handlers.
func (e *OmniaExecutor) executeHTTPCall(
	ctx context.Context,
	toolName, handlerName string,
	timeout time.Duration,
	cfg *HTTPCfg,
	args json.RawMessage,
) (json.RawMessage, error) {
	headers, err := e.buildHTTPHeaders(ctx, cfg, toolName, handlerName, args)
	if err != nil {
		return nil, err
	}
	policy := httpRetryParams(cfg)

	var lastCallResult httpCallResult
	classify := func(_ error) (bool, time.Duration) {
		if cfg.RetryPolicy == nil {
			return false, 0
		}
		return classifyHTTPResult(lastCallResult, cfg.RetryPolicy)
	}

	return retryWithBackoff(ctx, e.log, e.currentSpan(ctx), policy, timeout, classify,
		func(attemptCtx context.Context) (json.RawMessage, error) {
			return e.executeHTTPWithBreaker(attemptCtx, toolName, cfg, headers, args, &lastCallResult)
		},
	)
}

// executeHTTPWithBreaker runs an HTTP request through the circuit breaker.
// HTTP 4xx errors are wrapped as clientError so they don't trip the breaker.
func (e *OmniaExecutor) executeHTTPWithBreaker(
	ctx context.Context,
	toolName string,
	cfg *HTTPCfg,
	headers map[string]string,
	args json.RawMessage,
	lastCallResult *httpCallResult,
) (json.RawMessage, error) {
	var result json.RawMessage
	_, cbErr := e.breakers.Execute(toolName, func() ([]byte, error) {
		var callResult httpCallResult
		var httpErr error
		result, callResult, httpErr = doHTTPRequest(ctx, &http.Client{}, cfg, headers, args)
		*lastCallResult = callResult
		if httpErr != nil && callResult.StatusCode >= 400 && callResult.StatusCode < 500 {
			return nil, &clientError{err: httpErr}
		}
		return nil, httpErr
	})
	if cbErr != nil {
		return result, cbErr
	}
	return result, nil
}

// buildHTTPHeaders merges static headers, auth headers, and policy headers.
func (e *OmniaExecutor) buildHTTPHeaders(
	ctx context.Context,
	cfg *HTTPCfg,
	toolName, handlerName string,
	args json.RawMessage,
) (map[string]string, error) {
	headers := make(map[string]string)

	// Static headers from config
	for k, v := range cfg.Headers {
		headers[k] = v
	}

	// Auth headers
	if cfg.AuthType == authTypeWorkloadIdentity {
		name, val, err := resolveWorkloadIdentityHeader(ctx, e.tokenAcquirer, cfg.AuthCloud, cfg.AuthAudience, cfg.AuthHeader)
		if err != nil {
			return nil, fmt.Errorf("handler %q auth: %w", handlerName, err)
		}
		headers[name] = val
	} else if err := mergeAuthHeaders(headers, cfg.AuthType, cfg.AuthToken); err != nil {
		return nil, fmt.Errorf("handler %q auth: %w", handlerName, err)
	}

	// Omnia policy headers
	var argsMap map[string]any
	if len(args) > 0 {
		_ = json.Unmarshal(args, &argsMap)
	}
	req := &http.Request{Header: http.Header{}}
	SetAllOutboundHeaders(ctx, req, toolName, handlerName, argsMap)
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	// ToolPolicy broker-injected headers win over static/auth/policy headers
	// on key collision — they're an explicit enforcement decision.
	for k, v := range InjectedHeadersFromContext(ctx) {
		headers[k] = v
	}

	return headers, nil
}
