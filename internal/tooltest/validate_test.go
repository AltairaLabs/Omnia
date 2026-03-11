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

package tooltest

import (
	"encoding/json"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestValidateAgainstSchemaNilSchema(t *testing.T) {
	result := validateAgainstSchema(nil, json.RawMessage(`{"key":"value"}`))
	if result != nil {
		t.Errorf("expected nil, got %+v", result)
	}
}

func TestValidateAgainstSchemaEmptySchema(t *testing.T) {
	schema := &apiextensionsv1.JSON{Raw: []byte{}}
	result := validateAgainstSchema(schema, json.RawMessage(`{"key":"value"}`))
	if result != nil {
		t.Errorf("expected nil, got %+v", result)
	}
}

func TestValidateAgainstSchemaEmptyValue(t *testing.T) {
	schema := &apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)}
	result := validateAgainstSchema(schema, nil)
	if result != nil {
		t.Errorf("expected nil, got %+v", result)
	}
}

func TestValidateAgainstSchemaValid(t *testing.T) {
	schema := &apiextensionsv1.JSON{Raw: []byte(`{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "integer"}
		},
		"required": ["name"]
	}`)}
	value := json.RawMessage(`{"name":"Alice","age":30}`)

	result := validateAgainstSchema(schema, value)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}
}

func TestValidateAgainstSchemaInvalid(t *testing.T) {
	schema := &apiextensionsv1.JSON{Raw: []byte(`{
		"type": "object",
		"properties": {
			"name": {"type": "string"}
		},
		"required": ["name"]
	}`)}
	value := json.RawMessage(`{"age":30}`)

	result := validateAgainstSchema(schema, value)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Valid {
		t.Error("expected invalid, got valid")
	}
	if len(result.Errors) == 0 {
		t.Error("expected errors, got none")
	}
}

func TestValidateAgainstSchemaTypeMismatch(t *testing.T) {
	schema := &apiextensionsv1.JSON{Raw: []byte(`{"type":"string"}`)}
	value := json.RawMessage(`42`)

	result := validateAgainstSchema(schema, value)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Valid {
		t.Error("expected invalid, got valid")
	}
}

func TestValidateAgainstSchemaInvalidSchemaJSON(t *testing.T) {
	schema := &apiextensionsv1.JSON{Raw: []byte(`{not valid json`)}
	value := json.RawMessage(`{"key":"value"}`)

	result := validateAgainstSchema(schema, value)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Valid {
		t.Error("expected invalid, got valid")
	}
	if len(result.Errors) == 0 {
		t.Error("expected errors, got none")
	}
}

func TestTesterValidateNilOutcome(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	tester := NewTester(nil, log)

	result := tester.validate(nil, json.RawMessage(`{}`))
	if result != nil {
		t.Errorf("expected nil, got %+v", result)
	}
}

func TestTesterValidateNilHandler(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	tester := NewTester(nil, log)

	outcome := &testOutcome{handlerType: "http"}
	result := tester.validate(outcome, json.RawMessage(`{}`))
	if result != nil {
		t.Errorf("expected nil, got %+v", result)
	}
}

func TestTesterValidateNilTool(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	tester := NewTester(nil, log)

	outcome := &testOutcome{
		handlerType: "http",
		handler:     &omniav1alpha1.HandlerDefinition{},
	}
	result := tester.validate(outcome, json.RawMessage(`{}`))
	if result != nil {
		t.Errorf("expected nil, got %+v", result)
	}
}

func TestTesterValidateRequestOnly(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	tester := NewTester(nil, log)

	outcome := &testOutcome{
		handlerType: "http",
		handler: &omniav1alpha1.HandlerDefinition{
			Tool: &omniav1alpha1.ToolDefinition{
				InputSchema: apiextensionsv1.JSON{Raw: []byte(`{
					"type": "object",
					"properties": {"q": {"type": "string"}},
					"required": ["q"]
				}`)},
			},
		},
	}

	// Valid args
	result := tester.validate(outcome, json.RawMessage(`{"q":"hello"}`))
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Request == nil {
		t.Fatal("expected Request check")
	}
	if !result.Request.Valid {
		t.Errorf("expected valid request, got errors: %v", result.Request.Errors)
	}
	if result.Response != nil {
		t.Error("expected nil Response (no result in outcome)")
	}
}

func TestTesterValidateRequestInvalid(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	tester := NewTester(nil, log)

	outcome := &testOutcome{
		handlerType: "http",
		handler: &omniav1alpha1.HandlerDefinition{
			Tool: &omniav1alpha1.ToolDefinition{
				InputSchema: apiextensionsv1.JSON{Raw: []byte(`{
					"type": "object",
					"properties": {"q": {"type": "string"}},
					"required": ["q"]
				}`)},
			},
		},
	}

	// Missing required field
	result := tester.validate(outcome, json.RawMessage(`{}`))
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Request == nil {
		t.Fatal("expected Request check")
	}
	if result.Request.Valid {
		t.Error("expected invalid request")
	}
}

func TestTesterValidateResponseSchema(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	tester := NewTester(nil, log)

	outcome := &testOutcome{
		handlerType: "http",
		result:      json.RawMessage(`{"status":"ok"}`),
		handler: &omniav1alpha1.HandlerDefinition{
			Tool: &omniav1alpha1.ToolDefinition{
				InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
				OutputSchema: &apiextensionsv1.JSON{Raw: []byte(`{
					"type": "object",
					"properties": {"status": {"type": "string"}},
					"required": ["status"]
				}`)},
			},
		},
	}

	result := tester.validate(outcome, json.RawMessage(`{}`))
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Response == nil {
		t.Fatal("expected Response check")
	}
	if !result.Response.Valid {
		t.Errorf("expected valid response, got errors: %v", result.Response.Errors)
	}
}

func TestTesterValidateResponseInvalid(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	tester := NewTester(nil, log)

	outcome := &testOutcome{
		handlerType: "http",
		result:      json.RawMessage(`{"count":42}`),
		handler: &omniav1alpha1.HandlerDefinition{
			Tool: &omniav1alpha1.ToolDefinition{
				InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
				OutputSchema: &apiextensionsv1.JSON{Raw: []byte(`{
					"type": "object",
					"properties": {"status": {"type": "string"}},
					"required": ["status"]
				}`)},
			},
		},
	}

	result := tester.validate(outcome, json.RawMessage(`{}`))
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Response == nil {
		t.Fatal("expected Response check")
	}
	if result.Response.Valid {
		t.Error("expected invalid response")
	}
}

func TestTesterValidateNoSchemas(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	tester := NewTester(nil, log)

	outcome := &testOutcome{
		handlerType: "http",
		result:      json.RawMessage(`{"data":"ok"}`),
		handler: &omniav1alpha1.HandlerDefinition{
			Tool: &omniav1alpha1.ToolDefinition{
				Name: "no-schema-tool",
				// InputSchema has no Raw data, OutputSchema is nil
			},
		},
	}

	result := tester.validate(outcome, json.RawMessage(`{}`))
	if result != nil {
		t.Errorf("expected nil when no schemas defined, got %+v", result)
	}
}
