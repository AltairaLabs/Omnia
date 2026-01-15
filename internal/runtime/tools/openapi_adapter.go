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
	"sync"
	"time"

	"github.com/go-logr/logr"
)

const (
	// contentTypeJSON is the MIME type for JSON content.
	contentTypeJSON = "application/json"
)

// OpenAPIAdapterConfig contains configuration for the OpenAPI adapter.
type OpenAPIAdapterConfig struct {
	// Name is the adapter's unique name.
	Name string

	// SpecURL is the URL to fetch the OpenAPI specification from.
	SpecURL string

	// BaseURL overrides the base URL from the spec. If empty, uses first server from spec.
	BaseURL string

	// OperationFilter limits which operations are exposed as tools.
	// If empty, all operations are exposed.
	OperationFilter []string

	// Headers are custom HTTP headers to include in requests.
	Headers map[string]string

	// AuthType is the authentication type: "bearer", "basic", or empty for none.
	AuthType string

	// AuthToken is the authentication token or credentials.
	AuthToken string

	// Timeout is the request timeout.
	Timeout time.Duration
}

// OpenAPIOperation represents a parsed OpenAPI operation.
type OpenAPIOperation struct {
	OperationID string
	Method      string
	Path        string
	Summary     string
	Description string
	Parameters  []OpenAPIParameter
	RequestBody *OpenAPIRequestBody
}

// OpenAPIParameter represents an OpenAPI parameter.
type OpenAPIParameter struct {
	Name        string
	In          string // "path", "query", "header"
	Required    bool
	Description string
	Schema      map[string]any
}

// OpenAPIRequestBody represents an OpenAPI request body.
type OpenAPIRequestBody struct {
	Required    bool
	Description string
	Schema      map[string]any
}

// OpenAPIAdapter implements ToolAdapter for OpenAPI-described HTTP services.
type OpenAPIAdapter struct {
	config     OpenAPIAdapterConfig
	log        logr.Logger
	client     *http.Client
	baseURL    string
	operations map[string]*OpenAPIOperation // operationID -> operation
	tools      map[string]ToolInfo
	mu         sync.RWMutex
	connected  bool
}

// NewOpenAPIAdapter creates a new OpenAPI adapter.
func NewOpenAPIAdapter(config OpenAPIAdapterConfig, log logr.Logger) *OpenAPIAdapter {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	return &OpenAPIAdapter{
		config:     config,
		log:        log.WithValues("adapter", config.Name, "specURL", config.SpecURL),
		operations: make(map[string]*OpenAPIOperation),
		tools:      make(map[string]ToolInfo),
	}
}

// Name returns the adapter's name.
func (a *OpenAPIAdapter) Name() string {
	return a.config.Name
}

// Connect fetches the OpenAPI spec and discovers operations.
func (a *OpenAPIAdapter) Connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Create HTTP client
	a.client = &http.Client{
		Timeout: a.config.Timeout,
	}

	// Fetch the OpenAPI spec
	spec, err := a.fetchSpec(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch OpenAPI spec: %w", err)
	}

	// Determine base URL
	a.baseURL = a.config.BaseURL
	if a.baseURL == "" {
		a.baseURL = a.extractBaseURL(spec)
	}
	if a.baseURL == "" {
		return fmt.Errorf("no base URL configured and none found in spec")
	}

	// Parse operations from spec
	operations, err := a.parseOperations(spec)
	if err != nil {
		return fmt.Errorf("failed to parse operations: %w", err)
	}

	// Apply operation filter if configured
	if len(a.config.OperationFilter) > 0 {
		filterSet := make(map[string]bool)
		for _, op := range a.config.OperationFilter {
			filterSet[op] = true
		}
		filtered := make([]*OpenAPIOperation, 0)
		for _, op := range operations {
			if filterSet[op.OperationID] {
				filtered = append(filtered, op)
			}
		}
		operations = filtered
	}

	// Register operations as tools
	for _, op := range operations {
		a.operations[op.OperationID] = op
		a.tools[op.OperationID] = ToolInfo{
			Name:        op.OperationID,
			Description: a.buildDescription(op),
			InputSchema: a.buildInputSchema(op),
		}
	}

	a.connected = true
	a.log.Info("OpenAPI adapter connected",
		"baseURL", a.baseURL,
		"operations", len(a.operations))
	return nil
}

// ListTools returns available tools from this adapter.
func (a *OpenAPIAdapter) ListTools(ctx context.Context) ([]ToolInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	tools := make([]ToolInfo, 0, len(a.tools))
	for _, tool := range a.tools {
		tools = append(tools, tool)
	}
	return tools, nil
}

// Call invokes an operation with the given arguments.
func (a *OpenAPIAdapter) Call(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	a.mu.RLock()
	op, exists := a.operations[name]
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("adapter not connected")
	}
	if !exists {
		return nil, fmt.Errorf("operation %q not found", name)
	}

	a.log.V(1).Info("calling OpenAPI operation",
		"operation", name,
		"method", op.Method,
		"path", op.Path)

	// Build and execute the request
	req, err := a.buildRequest(ctx, op, args)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return &ToolResult{
			Content: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)),
			IsError: true,
		}, nil
	}

	// Parse response as JSON if possible
	var content any
	if json.Unmarshal(body, &content) != nil {
		content = string(body)
	}

	return &ToolResult{
		Content: content,
		IsError: false,
	}, nil
}

// Close closes the adapter.
func (a *OpenAPIAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client != nil {
		a.log.Info("closing OpenAPI adapter")
		a.client = nil
		a.connected = false
	}

	a.operations = make(map[string]*OpenAPIOperation)
	a.tools = make(map[string]ToolInfo)
	return nil
}
