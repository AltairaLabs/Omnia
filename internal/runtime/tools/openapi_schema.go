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

import "fmt"

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
		properties[param.Name] = a.buildParameterSchema(param)
		if param.Required {
			required = append(required, param.Name)
		}
	}

	// Add request body properties
	bodyProps, bodyReq := a.extractRequestBodySchema(op.RequestBody)
	for name, schema := range bodyProps {
		properties[name] = schema
	}
	required = append(required, bodyReq...)

	result := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		result["required"] = required
	}
	return result
}

// buildParameterSchema builds a schema for a single parameter.
func (a *OpenAPIAdapter) buildParameterSchema(param OpenAPIParameter) map[string]any {
	schema := param.Schema
	if schema == nil {
		schema = map[string]any{"type": "string"}
	}
	if param.Description == "" {
		return schema
	}
	// Add description to schema copy
	schemaCopy := make(map[string]any)
	for k, v := range schema {
		schemaCopy[k] = v
	}
	schemaCopy["description"] = param.Description
	return schemaCopy
}

// extractRequestBodySchema extracts properties and required fields from request body.
func (a *OpenAPIAdapter) extractRequestBodySchema(reqBody *OpenAPIRequestBody) (map[string]any, []string) {
	if reqBody == nil || reqBody.Schema == nil {
		return nil, nil
	}
	var required []string
	bodyProps, _ := reqBody.Schema["properties"].(map[string]any)
	if bodyRequired, ok := reqBody.Schema["required"].([]any); ok {
		for _, r := range bodyRequired {
			if name, ok := r.(string); ok {
				required = append(required, name)
			}
		}
	}
	return bodyProps, required
}
