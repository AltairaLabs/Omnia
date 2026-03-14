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

// Package tooltest provides a tool testing API that uses PromptKit executors
// to validate tool configurations from ToolRegistry CRDs.
package tooltest

import "encoding/json"

// TestRequest is the request body for testing a tool call.
type TestRequest struct {
	// HandlerName is the handler to test within the ToolRegistry.
	HandlerName string `json:"handlerName"`
	// ToolName is the specific tool to invoke. For HTTP/gRPC handlers with
	// a single tool, this can be omitted (defaults to the handler's tool name).
	ToolName string `json:"toolName,omitempty"`
	// Arguments is the JSON arguments to pass to the tool.
	Arguments json.RawMessage `json:"arguments"`
}

// TestResponse is the response from a tool test invocation.
type TestResponse struct {
	// Success indicates whether the tool call succeeded.
	Success bool `json:"success"`
	// Result contains the tool's JSON response on success.
	Result json.RawMessage `json:"result,omitempty"`
	// Error contains the error message on failure.
	Error string `json:"error,omitempty"`
	// DurationMs is the execution time in milliseconds.
	DurationMs int64 `json:"durationMs"`
	// HandlerType is the handler type that was used (http, mcp, grpc, openapi).
	HandlerType string `json:"handlerType"`
	// Validation contains schema validation results for both request and response.
	Validation *ValidationResult `json:"validation,omitempty"`
}

// ValidationResult holds schema validation outcomes for the request and response.
type ValidationResult struct {
	// Request is the validation result for the input arguments against inputSchema.
	Request *SchemaCheck `json:"request,omitempty"`
	// Response is the validation result for the tool output against outputSchema.
	Response *SchemaCheck `json:"response,omitempty"`
}

// SchemaCheck is the result of validating a value against a JSON Schema.
type SchemaCheck struct {
	// Valid is true if the value conforms to the schema.
	Valid bool `json:"valid"`
	// Errors lists the validation errors (empty when valid).
	Errors []string `json:"errors,omitempty"`
}
