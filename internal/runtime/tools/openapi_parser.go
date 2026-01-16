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
	"strings"
)

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
		ops := a.parsePathOperations(path, pathObj)
		operations = append(operations, ops...)
	}
	return operations, nil
}

// parsePathOperations extracts operations for a single path.
func (a *OpenAPIAdapter) parsePathOperations(path string, pathObj map[string]any) []*OpenAPIOperation {
	var operations []*OpenAPIOperation
	for _, method := range []string{"get", "post", "put", "patch", "delete"} {
		op := a.parseMethodOperation(method, path, pathObj)
		if op != nil {
			operations = append(operations, op)
		}
	}
	return operations
}

// parseMethodOperation extracts a single operation for a method.
func (a *OpenAPIAdapter) parseMethodOperation(method, path string, pathObj map[string]any) *OpenAPIOperation {
	opObj, ok := pathObj[method].(map[string]any)
	if !ok {
		return nil
	}

	operationID, _ := opObj["operationId"].(string)
	if operationID == "" {
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

	op.Parameters = a.parseParameters(opObj, pathObj)
	op.RequestBody = a.parseRequestBody(opObj)

	return op
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
		params = append(params, a.parseParameterList(source)...)
	}
	return params
}

// parseParameterList extracts parameters from a parameter list in a source object.
func (a *OpenAPIAdapter) parseParameterList(source map[string]any) []OpenAPIParameter {
	paramList, ok := source["parameters"].([]any)
	if !ok {
		return nil
	}
	var params []OpenAPIParameter
	for _, p := range paramList {
		if param := a.parseSingleParameter(p); param != nil {
			params = append(params, *param)
		}
	}
	return params
}

// parseSingleParameter parses a single parameter definition.
func (a *OpenAPIAdapter) parseSingleParameter(p any) *OpenAPIParameter {
	param, ok := p.(map[string]any)
	if !ok {
		return nil
	}
	name, _ := param["name"].(string)
	if name == "" {
		return nil
	}
	op := &OpenAPIParameter{Name: name}
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
	return op
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
