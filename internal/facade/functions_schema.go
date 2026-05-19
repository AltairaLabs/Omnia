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

package facade

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// CompileSchema compiles a raw JSON Schema (draft-2020-12) for use by the
// function-mode request/response validators. The schemaBytes payload must
// be the AgentRuntime spec.inputSchema or spec.outputSchema field as
// stored in the CRD; both are opaque JSON objects.
//
// Returns an opaque compiled schema usable with ValidateJSON, or an error
// if the input is not a valid JSON Schema.
func CompileSchema(schemaBytes []byte) (*jsonschema.Schema, error) {
	if len(schemaBytes) == 0 {
		return nil, fmt.Errorf("schema is empty")
	}
	var raw any
	if err := json.Unmarshal(schemaBytes, &raw); err != nil {
		return nil, fmt.Errorf("schema is not valid JSON: %w", err)
	}
	c := jsonschema.NewCompiler()
	// Resource URL is required by the compiler; use a stable opaque name.
	const resource = "spec://omnia/agentruntime/schema.json"
	if err := c.AddResource(resource, raw); err != nil {
		return nil, fmt.Errorf("schema add resource: %w", err)
	}
	compiled, err := c.Compile(resource)
	if err != nil {
		return nil, fmt.Errorf("schema compile: %w", err)
	}
	return compiled, nil
}

// ValidateJSON validates a raw JSON payload against a compiled schema.
// payload must be valid JSON; if it isn't, the JSON error is returned
// directly. Schema validation failures are wrapped with the offending
// path so the caller can surface useful 4xx detail.
func ValidateJSON(schema *jsonschema.Schema, payload []byte) error {
	if schema == nil {
		// No schema configured — treat as accept-anything. The handler
		// upstream gates on FunctionSpec presence, so a nil schema here
		// is a misconfiguration (not a runtime "validate everything").
		return fmt.Errorf("validator not configured")
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return fmt.Errorf("payload is empty")
	}
	var raw any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return fmt.Errorf("payload is not valid JSON: %w", err)
	}
	if err := schema.Validate(raw); err != nil {
		return fmt.Errorf("payload does not satisfy schema: %w", err)
	}
	return nil
}
