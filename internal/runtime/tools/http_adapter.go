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
	"bytes"
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

// HTTPAdapter implements ToolAdapter for HTTP tool endpoints.
type HTTPAdapter struct {
	config    HTTPAdapterConfig
	log       logr.Logger
	client    *http.Client
	tools     map[string]ToolInfo
	mu        sync.RWMutex
	connected bool
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

// Connect initializes the HTTP client and validates the endpoint.
func (a *HTTPAdapter) Connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Validate endpoint URL
	if _, err := url.Parse(a.config.Endpoint); err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}

	// Create HTTP client with timeout
	a.client = &http.Client{
		Timeout: a.config.Timeout,
	}

	// Use tool definition from config if provided, otherwise use defaults
	toolName := a.config.ToolName
	if toolName == "" {
		toolName = a.config.Name
	}

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

	// Register the tool
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
func (a *HTTPAdapter) ListTools(ctx context.Context) ([]ToolInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	tools := make([]ToolInfo, 0, len(a.tools))
	for _, tool := range a.tools {
		tools = append(tools, tool)
	}
	return tools, nil
}

// Call invokes the HTTP endpoint with the given arguments.
func (a *HTTPAdapter) Call(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("adapter not connected")
	}

	a.log.V(1).Info("calling HTTP tool", "name", name, "method", a.config.Method)

	// Build request
	req, err := a.buildRequest(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for HTTP errors
	if resp.StatusCode >= 400 {
		return &ToolResult{
			Content: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)),
			IsError: true,
		}, nil
	}

	// Parse response as JSON if possible
	var content any
	if json.Unmarshal(body, &content) != nil {
		// If not JSON, return as string
		content = string(body)
	}

	return &ToolResult{
		Content: content,
		IsError: false,
	}, nil
}

// buildRequest creates an HTTP request with the configured options.
func (a *HTTPAdapter) buildRequest(ctx context.Context, args map[string]any) (*http.Request, error) {
	var reqBody io.Reader
	endpoint := a.config.Endpoint

	method := strings.ToUpper(a.config.Method)

	// For GET/DELETE, encode args as query parameters
	// For POST/PUT/PATCH, encode args as JSON body
	switch method {
	case http.MethodGet, http.MethodDelete:
		if len(args) > 0 {
			u, err := url.Parse(endpoint)
			if err != nil {
				return nil, err
			}
			q := u.Query()
			for k, v := range args {
				q.Set(k, fmt.Sprintf("%v", v))
			}
			u.RawQuery = q.Encode()
			endpoint = u.String()
		}
	default:
		// POST, PUT, PATCH - encode as JSON body
		if args != nil {
			jsonBody, err := json.Marshal(args)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal arguments: %w", err)
			}
			reqBody = bytes.NewReader(jsonBody)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reqBody)
	if err != nil {
		return nil, err
	}

	// Set Content-Type for requests with body
	if reqBody != nil {
		req.Header.Set("Content-Type", a.config.ContentType)
	}

	// Set custom headers
	for k, v := range a.config.Headers {
		req.Header.Set(k, v)
	}

	// Set authentication
	if err := a.setAuth(req); err != nil {
		return nil, err
	}

	return req, nil
}

// setAuth sets the authentication header on the request.
func (a *HTTPAdapter) setAuth(req *http.Request) error {
	switch strings.ToLower(a.config.AuthType) {
	case "bearer":
		if a.config.AuthToken == "" {
			return fmt.Errorf("bearer auth requires a token")
		}
		req.Header.Set("Authorization", "Bearer "+a.config.AuthToken)
	case "basic":
		if a.config.AuthToken == "" {
			return fmt.Errorf("basic auth requires credentials")
		}
		// AuthToken should be "username:password"
		parts := strings.SplitN(a.config.AuthToken, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("basic auth token must be 'username:password'")
		}
		req.SetBasicAuth(parts[0], parts[1])
	case "":
		// No authentication
	default:
		return fmt.Errorf("unsupported auth type: %s", a.config.AuthType)
	}
	return nil
}

// Close closes the adapter.
func (a *HTTPAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client != nil {
		a.log.Info("closing HTTP adapter")
		// HTTP client doesn't need explicit closing, but we clear state
		a.client = nil
		a.connected = false
	}

	a.tools = make(map[string]ToolInfo)
	return nil
}
