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
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"

	sdktools "github.com/AltairaLabs/PromptKit/sdk/tools"

	pktools "github.com/AltairaLabs/PromptKit/runtime/tools"
)

// HTTPAdapterConfig contains configuration for the HTTP adapter.
type HTTPAdapterConfig struct {
	// Name is the adapter's unique name.
	Name string

	// Endpoint is the HTTP endpoint URL.
	Endpoint string

	// Method is the HTTP method (GET, POST, PUT, DELETE). Defaults to POST.
	Method string

	// Headers are custom HTTP headers to include in requests.
	Headers map[string]string

	// ContentType is the Content-Type header. Defaults to application/json.
	ContentType string

	// AuthType is the authentication type: "bearer", "basic", or empty for none.
	AuthType string

	// AuthToken is the authentication token or credentials.
	// For bearer: the token value
	// For basic: "username:password"
	AuthToken string

	// Timeout is the request timeout.
	Timeout time.Duration

	// ToolName is the tool name exposed by this handler.
	// If empty, the adapter name is used.
	ToolName string

	// ToolDescription is the tool description (shown to LLM).
	// If empty, a default description is generated.
	ToolDescription string

	// ToolInputSchema is the JSON Schema for the tool's input.
	// If empty, a generic schema allowing any object is used.
	ToolInputSchema map[string]any
}

// HTTPAdapter implements ToolAdapter by delegating HTTP execution to the
// PromptKit SDK's HTTPExecutor. This ensures a single code path for HTTP
// tool calls with consistent logging, OTel trace propagation, and error
// handling across both the runtime and the SDK.
type HTTPAdapter struct {
	config     HTTPAdapterConfig
	log        logr.Logger
	executor   *sdktools.HTTPExecutor
	descriptor *pktools.ToolDescriptor
	tools      map[string]ToolInfo
	client     *http.Client // kept for health checks
	mu         sync.RWMutex
	connected  bool
}

// NewHTTPAdapter creates a new HTTP adapter.
func NewHTTPAdapter(config HTTPAdapterConfig, log logr.Logger) *HTTPAdapter {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.Method == "" {
		config.Method = http.MethodPost
	}
	if config.ContentType == "" {
		config.ContentType = "application/json"
	}
	return &HTTPAdapter{
		config: config,
		log:    log.WithValues("adapter", config.Name, "endpoint", config.Endpoint),
		tools:  make(map[string]ToolInfo),
	}
}

// Name returns the adapter's name.
func (a *HTTPAdapter) Name() string {
	return a.config.Name
}

// Connect initializes the SDK executor and registers tool metadata.
func (a *HTTPAdapter) Connect(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Build headers including auth
	headers := make(map[string]string, len(a.config.Headers))
	for k, v := range a.config.Headers {
		headers[k] = v
	}
	if err := mergeAuthHeaders(headers, a.config.AuthType, a.config.AuthToken); err != nil {
		return fmt.Errorf("invalid auth config: %w", err)
	}

	// Create the SDK HTTP executor with a client matching our timeout
	client := &http.Client{Timeout: a.config.Timeout}
	a.executor = sdktools.NewHTTPExecutorWithClient(client)
	a.client = client

	// Build the PromptKit ToolDescriptor that the executor expects
	a.descriptor = &pktools.ToolDescriptor{
		Name: a.resolveToolName(),
		HTTPConfig: &pktools.HTTPConfig{
			URL:     a.config.Endpoint,
			Method:  a.config.Method,
			Headers: headers,
		},
	}

	// Register tool info for ListTools
	toolName := a.resolveToolName()
	toolDescription := a.config.ToolDescription
	if toolDescription == "" {
		toolDescription = fmt.Sprintf("HTTP tool at %s", a.config.Endpoint)
	}
	inputSchema := a.config.ToolInputSchema
	if inputSchema == nil {
		inputSchema = map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		}
	}
	a.tools[toolName] = ToolInfo{
		Name:        toolName,
		Description: toolDescription,
		InputSchema: inputSchema,
	}

	a.connected = true
	a.log.Info("HTTP adapter connected", "method", a.config.Method)
	return nil
}

// ListTools returns available tools from this adapter.
func (a *HTTPAdapter) ListTools(_ context.Context) ([]ToolInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	tools := make([]ToolInfo, 0, len(a.tools))
	for _, tool := range a.tools {
		tools = append(tools, tool)
	}
	return tools, nil
}

// Call delegates to the PromptKit SDK HTTPExecutor for consistent logging,
// OTel trace injection, and error handling.
func (a *HTTPAdapter) Call(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	a.mu.RLock()
	executor := a.executor
	connected := a.connected
	a.mu.RUnlock()

	if !connected || executor == nil {
		return nil, fmt.Errorf("adapter not connected")
	}

	// Set Omnia policy propagation headers on the descriptor before calling.
	// The SDK executor handles OTel trace injection and structured logging.
	desc := a.descriptorWithPolicyHeaders(ctx, name, args)

	// For GET/DELETE, encode args as URL query parameters and send an empty
	// body. The SDK executor always sends args as a JSON body, which doesn't
	// work for APIs that expect query parameters.
	var argsJSON []byte
	if methodUsesQueryParams(a.config.Method) && len(args) > 0 {
		desc.HTTPConfig.URL = appendQueryParams(desc.HTTPConfig.URL, args)
		argsJSON = []byte("{}")
	} else {
		var err error
		argsJSON, err = json.Marshal(args)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal arguments: %w", err)
		}
	}

	result, err := executor.ExecuteWithContext(ctx, desc, argsJSON)
	if err != nil {
		// The SDK returns errors for non-2xx responses; convert to ToolResult
		// so the LLM sees the error message rather than a hard failure.
		return &ToolResult{
			Content: err.Error(),
			IsError: true,
		}, nil
	}

	// Parse the JSON result back to a Go value
	var content any
	if json.Unmarshal(result, &content) != nil {
		content = string(result)
	}

	return &ToolResult{
		Content: content,
		IsError: false,
	}, nil
}

// methodUsesQueryParams returns true for HTTP methods where tool arguments
// should be sent as URL query parameters rather than a JSON request body.
func methodUsesQueryParams(method string) bool {
	m := strings.ToUpper(method)
	return m == http.MethodGet || m == http.MethodDelete
}

// appendQueryParams encodes args as URL query parameters appended to baseURL.
// String values are used directly; all other types are JSON-encoded.
func appendQueryParams(baseURL string, args map[string]any) string {
	params := url.Values{}
	for key, val := range args {
		switch v := val.(type) {
		case string:
			params.Set(key, v)
		case float64:
			params.Set(key, fmt.Sprintf("%g", v))
		case bool:
			params.Set(key, fmt.Sprintf("%t", v))
		default:
			// JSON-encode complex values (arrays, objects)
			b, err := json.Marshal(v)
			if err == nil {
				params.Set(key, string(b))
			}
		}
	}

	sep := "?"
	if strings.Contains(baseURL, "?") {
		sep = "&"
	}
	return baseURL + sep + params.Encode()
}

// HealthCheck probes the HTTP endpoint with a short-timeout request.
func (a *HTTPAdapter) HealthCheck(ctx context.Context) error {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	endpoint := a.config.Endpoint
	a.mu.RUnlock()

	if !connected || client == nil {
		return fmt.Errorf("adapter not connected")
	}

	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	method := http.MethodHead
	if strings.ToUpper(a.config.Method) == http.MethodGet {
		method = http.MethodGet
	}

	req, err := http.NewRequestWithContext(probeCtx, method, endpoint, nil)
	if err != nil {
		return fmt.Errorf("health check request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 500 {
		return fmt.Errorf("health check: HTTP %d", resp.StatusCode)
	}
	return nil
}

// Close closes the adapter.
func (a *HTTPAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.connected {
		a.log.Info("closing HTTP adapter")
		a.executor = nil
		a.client = nil
		a.connected = false
	}

	a.tools = make(map[string]ToolInfo)
	return nil
}

// resolveToolName returns the effective tool name.
func (a *HTTPAdapter) resolveToolName() string {
	if a.config.ToolName != "" {
		return a.config.ToolName
	}
	return a.config.Name
}

// descriptorWithPolicyHeaders returns a copy of the descriptor with Omnia
// policy headers merged into the HTTPConfig headers.
func (a *HTTPAdapter) descriptorWithPolicyHeaders(ctx context.Context, toolName string, args map[string]any) *pktools.ToolDescriptor {
	// Start with the base descriptor's headers
	merged := make(map[string]string, len(a.descriptor.HTTPConfig.Headers))
	for k, v := range a.descriptor.HTTPConfig.Headers {
		merged[k] = v
	}

	// Add Omnia policy, tool, and parameter headers
	for k, v := range policyHeaders(ctx, toolName, a.config.Name, args) {
		merged[k] = v
	}

	return &pktools.ToolDescriptor{
		Name: a.descriptor.Name,
		HTTPConfig: &pktools.HTTPConfig{
			URL:     a.descriptor.HTTPConfig.URL,
			Method:  a.descriptor.HTTPConfig.Method,
			Headers: merged,
		},
	}
}

// mergeAuthHeaders adds authentication headers to the map based on auth type.
func mergeAuthHeaders(headers map[string]string, authType, authToken string) error {
	switch strings.ToLower(authType) {
	case "bearer":
		if authToken == "" {
			return fmt.Errorf("bearer auth requires a token")
		}
		headers["Authorization"] = "Bearer " + authToken
	case "basic":
		if authToken == "" {
			return fmt.Errorf("basic auth requires credentials")
		}
		parts := strings.SplitN(authToken, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("basic auth token must be 'username:password'")
		}
		// Use net/http's SetBasicAuth equivalent via header
		req := &http.Request{Header: http.Header{}}
		req.SetBasicAuth(parts[0], parts[1])
		headers["Authorization"] = req.Header.Get("Authorization")
	case "":
		// No authentication
	default:
		return fmt.Errorf("unsupported auth type: %s", authType)
	}
	return nil
}

// policyHeaders returns Omnia policy, tool, and parameter headers as a map.
func policyHeaders(ctx context.Context, toolName, registryName string, args map[string]any) map[string]string {
	// Build a temporary request to collect headers using existing helpers
	req := &http.Request{Header: http.Header{}}
	SetAllOutboundHeaders(ctx, req, toolName, registryName, args)
	result := make(map[string]string, len(req.Header))
	for k, v := range req.Header {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}
