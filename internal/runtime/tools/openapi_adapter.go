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

// fetchSpec fetches the OpenAPI specification from the configured URL.
func (a *OpenAPIAdapter) fetchSpec(ctx context.Context) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.config.SpecURL, nil)
	if err != nil {
		return nil, err
	}

	// Add auth headers for spec fetch if configured
	if err := a.setAuth(req); err != nil {
		return nil, err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var spec map[string]any
	if err := json.Unmarshal(body, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse spec as JSON: %w", err)
	}

	return spec, nil
}

// extractBaseURL extracts the base URL from the OpenAPI spec.
func (a *OpenAPIAdapter) extractBaseURL(spec map[string]any) string {
	// OpenAPI 3.x: look for servers[0].url
	if servers, ok := spec["servers"].([]any); ok && len(servers) > 0 {
		if server, ok := servers[0].(map[string]any); ok {
			if url, ok := server["url"].(string); ok {
				return strings.TrimSuffix(url, "/")
			}
		}
	}

	// OpenAPI 2.x (Swagger): look for host + basePath
	host, _ := spec["host"].(string)
	basePath, _ := spec["basePath"].(string)
	if host != "" {
		scheme := "https"
		if schemes, ok := spec["schemes"].([]any); ok && len(schemes) > 0 {
			if s, ok := schemes[0].(string); ok {
				scheme = s
			}
		}
		return fmt.Sprintf("%s://%s%s", scheme, host, basePath)
	}

	return ""
}

// parseOperations extracts operations from the OpenAPI spec.
func (a *OpenAPIAdapter) parseOperations(spec map[string]any) ([]*OpenAPIOperation, error) {
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("no paths found in spec")
	}

	var operations []*OpenAPIOperation

	for path, pathItem := range paths {
		pathObj, ok := pathItem.(map[string]any)
		if !ok {
			continue
		}

		// Check each HTTP method
		for _, method := range []string{"get", "post", "put", "patch", "delete"} {
			opObj, ok := pathObj[method].(map[string]any)
			if !ok {
				continue
			}

			operationID, _ := opObj["operationId"].(string)
			if operationID == "" {
				// Generate operationId from method + path if not provided
				operationID = a.generateOperationID(method, path)
			}

			op := &OpenAPIOperation{
				OperationID: operationID,
				Method:      strings.ToUpper(method),
				Path:        path,
			}

			if summary, ok := opObj["summary"].(string); ok {
				op.Summary = summary
			}
			if desc, ok := opObj["description"].(string); ok {
				op.Description = desc
			}

			// Parse parameters
			op.Parameters = a.parseParameters(opObj, pathObj)

			// Parse request body (OpenAPI 3.x)
			op.RequestBody = a.parseRequestBody(opObj)

			operations = append(operations, op)
		}
	}

	return operations, nil
}

// generateOperationID generates an operationId from method and path.
func (a *OpenAPIAdapter) generateOperationID(method, path string) string {
	// Convert /users/{id}/posts to users_id_posts
	cleaned := strings.ReplaceAll(path, "/", "_")
	cleaned = strings.ReplaceAll(cleaned, "{", "")
	cleaned = strings.ReplaceAll(cleaned, "}", "")
	cleaned = strings.Trim(cleaned, "_")
	return method + "_" + cleaned
}

// parseParameters extracts parameters from an operation.
func (a *OpenAPIAdapter) parseParameters(opObj, pathObj map[string]any) []OpenAPIParameter {
	var params []OpenAPIParameter

	// Collect parameters from path level and operation level
	for _, source := range []map[string]any{pathObj, opObj} {
		if paramList, ok := source["parameters"].([]any); ok {
			for _, p := range paramList {
				param, ok := p.(map[string]any)
				if !ok {
					continue
				}

				op := OpenAPIParameter{
					Name: param["name"].(string),
				}
				if in, ok := param["in"].(string); ok {
					op.In = in
				}
				if req, ok := param["required"].(bool); ok {
					op.Required = req
				}
				if desc, ok := param["description"].(string); ok {
					op.Description = desc
				}
				if schema, ok := param["schema"].(map[string]any); ok {
					op.Schema = schema
				}

				params = append(params, op)
			}
		}
	}

	return params
}

// parseRequestBody extracts the request body schema from an operation.
func (a *OpenAPIAdapter) parseRequestBody(opObj map[string]any) *OpenAPIRequestBody {
	reqBody, ok := opObj["requestBody"].(map[string]any)
	if !ok {
		return nil
	}

	rb := &OpenAPIRequestBody{}
	if req, ok := reqBody["required"].(bool); ok {
		rb.Required = req
	}
	if desc, ok := reqBody["description"].(string); ok {
		rb.Description = desc
	}

	// Extract schema from content.application/json.schema
	if content, ok := reqBody["content"].(map[string]any); ok {
		if jsonContent, ok := content[contentTypeJSON].(map[string]any); ok {
			if schema, ok := jsonContent["schema"].(map[string]any); ok {
				rb.Schema = schema
			}
		}
	}

	return rb
}

// buildDescription builds a tool description from an operation.
func (a *OpenAPIAdapter) buildDescription(op *OpenAPIOperation) string {
	if op.Summary != "" {
		return op.Summary
	}
	if op.Description != "" {
		// Truncate long descriptions
		if len(op.Description) > 200 {
			return op.Description[:197] + "..."
		}
		return op.Description
	}
	return fmt.Sprintf("%s %s", op.Method, op.Path)
}

// buildInputSchema builds a JSON Schema for the tool's input.
func (a *OpenAPIAdapter) buildInputSchema(op *OpenAPIOperation) map[string]any {
	properties := make(map[string]any)
	required := make([]string, 0)

	// Add parameters
	for _, param := range op.Parameters {
		schema := param.Schema
		if schema == nil {
			schema = map[string]any{"type": "string"}
		}

		// Add description to schema
		if param.Description != "" {
			schemaCopy := make(map[string]any)
			for k, v := range schema {
				schemaCopy[k] = v
			}
			schemaCopy["description"] = param.Description
			schema = schemaCopy
		}

		properties[param.Name] = schema
		if param.Required {
			required = append(required, param.Name)
		}
	}

	// Add request body properties
	if op.RequestBody != nil && op.RequestBody.Schema != nil {
		if bodyProps, ok := op.RequestBody.Schema["properties"].(map[string]any); ok {
			for name, schema := range bodyProps {
				properties[name] = schema
			}
		}
		if bodyRequired, ok := op.RequestBody.Schema["required"].([]any); ok {
			for _, r := range bodyRequired {
				if name, ok := r.(string); ok {
					required = append(required, name)
				}
			}
		}
	}

	result := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		result["required"] = required
	}

	return result
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

// buildRequest creates an HTTP request for an operation.
func (a *OpenAPIAdapter) buildRequest(ctx context.Context, op *OpenAPIOperation, args map[string]any) (*http.Request, error) {
	// Build URL with path parameters substituted
	path := op.Path
	queryParams := url.Values{}
	bodyParams := make(map[string]any)
	headerParams := make(map[string]string)

	// Categorize parameters
	for _, param := range op.Parameters {
		value, exists := args[param.Name]
		if !exists {
			continue
		}

		switch param.In {
		case "path":
			path = strings.ReplaceAll(path, "{"+param.Name+"}", fmt.Sprintf("%v", value))
		case "query":
			queryParams.Set(param.Name, fmt.Sprintf("%v", value))
		case "header":
			headerParams[param.Name] = fmt.Sprintf("%v", value)
		}
	}

	// Remaining args go to request body (for POST/PUT/PATCH)
	for name, value := range args {
		isParam := false
		for _, param := range op.Parameters {
			if param.Name == name {
				isParam = true
				break
			}
		}
		if !isParam {
			bodyParams[name] = value
		}
	}

	// Build full URL
	fullURL := a.baseURL + path
	if len(queryParams) > 0 {
		fullURL += "?" + queryParams.Encode()
	}

	// Build request body
	var reqBody io.Reader
	if len(bodyParams) > 0 && (op.Method == "POST" || op.Method == "PUT" || op.Method == "PATCH") {
		jsonBody, err := json.Marshal(bodyParams)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, op.Method, fullURL, reqBody)
	if err != nil {
		return nil, err
	}

	// Set headers
	if reqBody != nil {
		req.Header.Set("Content-Type", contentTypeJSON)
	}
	req.Header.Set("Accept", contentTypeJSON)

	// Custom headers from config
	for k, v := range a.config.Headers {
		req.Header.Set(k, v)
	}

	// Parameter headers
	for k, v := range headerParams {
		req.Header.Set(k, v)
	}

	// Authentication
	if err := a.setAuth(req); err != nil {
		return nil, err
	}

	return req, nil
}

// setAuth sets authentication headers on a request.
func (a *OpenAPIAdapter) setAuth(req *http.Request) error {
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
