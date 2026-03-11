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

	"github.com/xeipuuv/gojsonschema"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// validateAgainstSchema validates a JSON value against a JSON Schema.
// Returns nil if no schema is provided (nothing to validate against).
func validateAgainstSchema(schema *apiextensionsv1.JSON, value json.RawMessage) *SchemaCheck {
	if schema == nil || len(schema.Raw) == 0 {
		return nil
	}
	if len(value) == 0 {
		return nil
	}

	schemaLoader := gojsonschema.NewBytesLoader(schema.Raw)
	documentLoader := gojsonschema.NewBytesLoader(value)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return &SchemaCheck{
			Valid:  false,
			Errors: []string{"schema validation error: " + err.Error()},
		}
	}

	if result.Valid() {
		return &SchemaCheck{Valid: true}
	}

	errors := make([]string, 0, len(result.Errors()))
	for _, e := range result.Errors() {
		errors = append(errors, e.String())
	}
	return &SchemaCheck{
		Valid:  false,
		Errors: errors,
	}
}
