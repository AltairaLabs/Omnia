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

package controller

import (
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const schemaTypeObject = "object"

func TestUnmarshalRawJSON(t *testing.T) {
	tests := []struct {
		name     string
		raw      []byte
		wantNil  bool
		wantType string
	}{
		{
			name:    "nil input",
			raw:     nil,
			wantNil: true,
		},
		{
			name:    "empty input",
			raw:     []byte{},
			wantNil: true,
		},
		{
			name:    "invalid JSON",
			raw:     []byte("not json"),
			wantNil: true,
		},
		{
			name:     "object schema",
			raw:      []byte(`{"type":"object","properties":{"expr":{"type":"string"}}}`),
			wantNil:  false,
			wantType: "map",
		},
		{
			name:     "string value",
			raw:      []byte(`"hello"`),
			wantNil:  false,
			wantType: "string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unmarshalRawJSON(tt.raw)
			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			switch tt.wantType {
			case "map":
				m, ok := result.(map[string]interface{})
				if !ok {
					t.Errorf("expected map[string]interface{}, got %T", result)
				}
				if m["type"] != schemaTypeObject {
					t.Errorf("expected type=object, got %v", m["type"])
				}
			case "string":
				if _, ok := result.(string); !ok {
					t.Errorf("expected string, got %T", result)
				}
			}
		})
	}
}

func TestBuildToolDefinition(t *testing.T) {
	t.Run("nil tool returns nil", func(t *testing.T) {
		if got := buildToolDefinition(nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("converts InputSchema from raw bytes to structured map", func(t *testing.T) {
		tool := &omniav1alpha1.ToolDefinition{
			Name:        "calculator",
			Description: "Evaluate math expressions",
			InputSchema: apiextensionsv1.JSON{
				Raw: []byte(`{"type":"object","properties":{"expr":{"type":"string","description":"Math expression"}},"required":["expr"]}`),
			},
		}

		def := buildToolDefinition(tool)
		if def == nil {
			t.Fatal("expected non-nil definition")
		}

		schema, ok := def.InputSchema.(map[string]interface{})
		if !ok {
			t.Fatalf("InputSchema should be map[string]interface{}, got %T", def.InputSchema)
		}

		if schema["type"] != schemaTypeObject {
			t.Errorf("expected type=object, got %v", schema["type"])
		}

		props, ok := schema["properties"].(map[string]interface{})
		if !ok {
			t.Fatalf("properties should be map, got %T", schema["properties"])
		}

		expr, ok := props["expr"].(map[string]interface{})
		if !ok {
			t.Fatalf("expr should be map, got %T", props["expr"])
		}
		if expr["type"] != "string" {
			t.Errorf("expected expr.type=string, got %v", expr["type"])
		}
	})

	t.Run("converts OutputSchema from raw bytes to structured map", func(t *testing.T) {
		tool := &omniav1alpha1.ToolDefinition{
			Name:        "calculator",
			Description: "Evaluate math expressions",
			InputSchema: apiextensionsv1.JSON{
				Raw: []byte(`{"type":"object"}`),
			},
			OutputSchema: &apiextensionsv1.JSON{
				Raw: []byte(`{"type":"object","properties":{"result":{"type":"number"}}}`),
			},
		}

		def := buildToolDefinition(tool)
		if def == nil {
			t.Fatal("expected non-nil definition")
		}

		schema, ok := def.OutputSchema.(map[string]interface{})
		if !ok {
			t.Fatalf("OutputSchema should be map[string]interface{}, got %T", def.OutputSchema)
		}
		if schema["type"] != schemaTypeObject {
			t.Errorf("expected type=object, got %v", schema["type"])
		}
	})

	t.Run("nil OutputSchema stays nil", func(t *testing.T) {
		tool := &omniav1alpha1.ToolDefinition{
			Name:        "calculator",
			Description: "Evaluate math expressions",
			InputSchema: apiextensionsv1.JSON{
				Raw: []byte(`{"type":"object"}`),
			},
		}

		def := buildToolDefinition(tool)
		if def.OutputSchema != nil {
			t.Errorf("expected nil OutputSchema, got %v", def.OutputSchema)
		}
	})
}

func TestBuildToolsConfig(t *testing.T) {
	t.Run("handler with tool produces structured schema in config", func(t *testing.T) {
		registry := &omniav1alpha1.ToolRegistry{}
		registry.Spec.Handlers = []omniav1alpha1.HandlerDefinition{
			{
				Name: "calc",
				Type: omniav1alpha1.HandlerTypeHTTP,
				HTTPConfig: &omniav1alpha1.HTTPConfig{
					Endpoint: "http://calc:8080/calculate",
				},
				Tool: &omniav1alpha1.ToolDefinition{
					Name:        "calculator",
					Description: "Evaluate math expressions",
					InputSchema: apiextensionsv1.JSON{
						Raw: []byte(`{"type":"object","properties":{"expr":{"type":"string"}},"required":["expr"]}`),
					},
				},
			},
		}
		registry.Status.DiscoveredTools = []omniav1alpha1.DiscoveredTool{
			{
				HandlerName: "calc",
				Status:      omniav1alpha1.ToolStatusAvailable,
				Endpoint:    "http://calc:8080/calculate",
			},
		}

		r := &AgentRuntimeReconciler{}
		config := r.buildToolsConfig(registry)

		if len(config.Handlers) != 1 {
			t.Fatalf("expected 1 handler, got %d", len(config.Handlers))
		}

		handler := config.Handlers[0]
		if handler.Tool == nil {
			t.Fatal("expected tool definition")
		}

		// The key assertion: InputSchema must be a map, not []byte.
		// If it were []byte, YAML marshaling would base64-encode it,
		// and the runtime couldn't extract the schema.
		schema, ok := handler.Tool.InputSchema.(map[string]interface{})
		if !ok {
			t.Fatalf("InputSchema should be map[string]interface{}, got %T", handler.Tool.InputSchema)
		}
		if schema["type"] != schemaTypeObject {
			t.Errorf("expected schema type=object, got %v", schema["type"])
		}
	})
}
