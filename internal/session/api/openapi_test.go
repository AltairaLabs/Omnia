/*
Copyright 2026 Altaira Labs.

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

package api

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/altairalabs/omnia/internal/session"
)

// specRelPath is the path to the OpenAPI spec relative to this test file.
const specRelPath = "../../../api/session-api/openapi.yaml"

func specFilePath() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), specRelPath)
}

// openAPISpec is a minimal representation of an OpenAPI 3.0 spec for testing.
type openAPISpec struct {
	Paths      map[string]map[string]specOperation `yaml:"paths"`
	Components struct {
		Schemas map[string]specSchema `yaml:"schemas"`
	} `yaml:"components"`
}

type specOperation struct {
	OperationID string `yaml:"operationId"`
}

type specSchema struct {
	Properties map[string]any `yaml:"properties"`
	Enum       []string       `yaml:"enum"`
}

func loadSpec(t *testing.T) openAPISpec {
	t.Helper()
	data, err := os.ReadFile(specFilePath())
	if err != nil {
		t.Fatalf("reading spec: %v", err)
	}
	var spec openAPISpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parsing spec: %v", err)
	}
	return spec
}

// jsonFieldNames returns the set of JSON field names for a struct type,
// following the same rules as encoding/json: uses the json tag name if present,
// the Go field name if no tag, and skips fields tagged with "-".
// For embedded structs, it inlines their fields (matching encoding/json behavior).
func jsonFieldNames(t reflect.Type) map[string]bool {
	fields := make(map[string]bool)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		// Handle embedded structs: inline their fields.
		if f.Anonymous {
			for k := range jsonFieldNames(f.Type) {
				fields[k] = true
			}
			continue
		}

		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := strings.SplitN(tag, ",", 2)[0]
		if name == "" {
			name = f.Name
		}
		fields[name] = true
	}
	return fields
}

// TestOpenAPISchemaMatchesGoTypes verifies that every schema in the OpenAPI spec
// has properties that exactly match the json tags on the corresponding Go struct.
// This catches drift between the spec and the actual types.
func TestOpenAPISchemaMatchesGoTypes(t *testing.T) {
	spec := loadSpec(t)

	// Map of OpenAPI schema name → Go type.
	schemaTypes := map[string]reflect.Type{
		// Core session types (internal/session/store.go)
		"Session":      reflect.TypeOf(session.Session{}),
		"Message":      reflect.TypeOf(session.Message{}),
		"ToolCall":     reflect.TypeOf(session.ToolCall{}),
		"ProviderCall": reflect.TypeOf(session.ProviderCall{}),
		"RuntimeEvent": reflect.TypeOf(session.RuntimeEvent{}),

		// Request/response types (internal/session/api/)
		"CreateSessionRequest":      reflect.TypeOf(CreateSessionRequest{}),
		"RefreshTTLRequest":         reflect.TypeOf(RefreshTTLRequest{}),
		"SessionStatusUpdate":       reflect.TypeOf(session.SessionStatusUpdate{}),
		"SessionResponse":           reflect.TypeOf(SessionResponse{}),
		"SessionListResponse":       reflect.TypeOf(SessionListResponse{}),
		"MessagesResponse":          reflect.TypeOf(MessagesResponse{}),
		"ErrorResponse":             reflect.TypeOf(ErrorResponse{}),
		"EvalResult":                reflect.TypeOf(EvalResult{}),
		"EvalResultListResponse":    reflect.TypeOf(EvalResultListResponse{}),
		"EvalResultSessionResponse": reflect.TypeOf(EvalResultSessionResponse{}),
		"EvaluateAcceptedResponse":  reflect.TypeOf(EvaluateAcceptedResponse{}),
	}

	for name, goType := range schemaTypes {
		t.Run(name, func(t *testing.T) {
			schema, ok := spec.Components.Schemas[name]
			if !ok {
				t.Fatalf("schema %q not found in OpenAPI spec", name)
			}

			goFields := jsonFieldNames(goType)
			specFields := make(map[string]bool, len(schema.Properties))
			for k := range schema.Properties {
				specFields[k] = true
			}

			// Check for fields in Go but not in spec.
			for field := range goFields {
				if !specFields[field] {
					t.Errorf("Go struct has json field %q but OpenAPI schema does not", field)
				}
			}

			// Check for properties in spec but not in Go.
			for prop := range specFields {
				if !goFields[prop] {
					t.Errorf("OpenAPI schema has property %q but Go struct does not", prop)
				}
			}
		})
	}
}

// TestOpenAPIEnumsMatchGoConstants verifies that enum values in the spec
// match the const declarations in Go code.
func TestOpenAPIEnumsMatchGoConstants(t *testing.T) {
	spec := loadSpec(t)

	enumTypes := map[string][]string{
		"SessionStatus": {
			string(session.SessionStatusActive),
			string(session.SessionStatusCompleted),
			string(session.SessionStatusError),
			string(session.SessionStatusExpired),
		},
		"MessageRole": {
			string(session.RoleUser),
			string(session.RoleAssistant),
			string(session.RoleSystem),
		},
		"ToolCallStatus": {
			string(session.ToolCallStatusPending),
			string(session.ToolCallStatusSuccess),
			string(session.ToolCallStatusError),
		},
		"ProviderCallStatus": {
			string(session.ProviderCallStatusPending),
			string(session.ProviderCallStatusCompleted),
			string(session.ProviderCallStatusFailed),
		},
	}

	for name, goValues := range enumTypes {
		t.Run(name, func(t *testing.T) {
			schema, ok := spec.Components.Schemas[name]
			if !ok {
				t.Fatalf("enum schema %q not found in spec", name)
			}

			sort.Strings(goValues)
			specValues := make([]string, len(schema.Enum))
			copy(specValues, schema.Enum)
			sort.Strings(specValues)

			if !reflect.DeepEqual(goValues, specValues) {
				t.Errorf("enum mismatch:\n  Go:   %v\n  Spec: %v", goValues, specValues)
			}
		})
	}
}

// TestOpenAPIRoutesMatchHandler verifies that every route registered by the
// handler has a corresponding path+method in the OpenAPI spec and vice versa.
//
// The handlerRoutes list is the canonical set of routes from RegisterRoutes().
// If you add or remove a route there, update this list — the test will tell you
// which direction is out of sync.
func TestOpenAPIRoutesMatchHandler(t *testing.T) {
	spec := loadSpec(t)

	// These must exactly mirror the patterns in RegisterRoutes().
	// Go 1.22+ mux patterns use "METHOD /path/{param}" syntax which matches
	// OpenAPI's "METHOD /path/{param}" when we build the key below.
	handlerRoutes := []string{
		"GET /healthz",
		"GET /api/v1/sessions",
		"GET /api/v1/sessions/search",
		"GET /api/v1/sessions/{sessionID}",
		"GET /api/v1/sessions/{sessionID}/messages",
		"POST /api/v1/sessions",
		"POST /api/v1/sessions/{sessionID}/messages",
		"PATCH /api/v1/sessions/{sessionID}/status",
		"POST /api/v1/sessions/{sessionID}/ttl",
		"DELETE /api/v1/sessions/{sessionID}",
		"POST /api/v1/sessions/{sessionID}/tool-calls",
		"GET /api/v1/sessions/{sessionID}/tool-calls",
		"POST /api/v1/sessions/{sessionID}/provider-calls",
		"GET /api/v1/sessions/{sessionID}/provider-calls",
		"POST /api/v1/sessions/{sessionID}/events",
		"GET /api/v1/sessions/{sessionID}/events",
		"GET /api/v1/sessions/{sessionID}/eval-results",
		"POST /api/v1/sessions/{sessionID}/evaluate",
		"POST /api/v1/eval-results",
		"GET /api/v1/eval-results",
	}

	// Build spec routes.
	specRoutes := make(map[string]bool)
	for path, methods := range spec.Paths {
		for method := range methods {
			specRoutes[strings.ToUpper(method)+" "+path] = true
		}
	}

	handlerSet := make(map[string]bool, len(handlerRoutes))
	for _, r := range handlerRoutes {
		handlerSet[r] = true
	}

	for route := range handlerSet {
		if !specRoutes[route] {
			t.Errorf("handler registers %q but it is missing from the OpenAPI spec", route)
		}
	}
	for route := range specRoutes {
		if !handlerSet[route] {
			t.Errorf("OpenAPI spec has %q but handler does not register it", route)
		}
	}
}
