/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

// Package schemautil compiles draft-2020-12 JSON Schemas. It is the single
// implementation shared by the function-mode facade (runtime validation)
// and the AgentRuntime admission webhook (apply-time validation), so the
// two can never disagree about whether a schema is valid.
package schemautil

import (
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// CompileSchema compiles a raw JSON Schema (draft-2020-12). schemaBytes is
// the AgentRuntime spec.inputSchema or spec.outputSchema field as stored in
// the CRD; both are opaque JSON objects. Returns an opaque compiled schema,
// or an error if the input is not a valid JSON Schema.
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
