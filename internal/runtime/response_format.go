/*
Copyright 2026.

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

package runtime

import "github.com/AltairaLabs/PromptKit/runtime/providers"

// Output format values for AgentRuntime spec.outputFormat.
const (
	outputFormatText       = "text"
	outputFormatJSON       = "json"
	outputFormatJSONSchema = "json_schema"
	modeFunction           = "function"
)

// resolveResponseFormat maps a function-mode AgentRuntime's outputFormat into a
// PromptKit ResponseFormat, or nil when no provider-side constraint applies.
// It returns nil for non-function modes and for "text". When outputFormat is
// empty in function mode it defaults to json_schema (constrain by default,
// per #1483). schemaName is used as the provider schema name (OpenAI requires
// one); it is typically the agent name.
func resolveResponseFormat(mode, outputFormat string, outputSchema []byte, schemaName string) *providers.ResponseFormat {
	if mode != modeFunction {
		return nil
	}
	format := outputFormat
	if format == "" {
		format = outputFormatJSONSchema
	}
	switch format {
	case outputFormatJSON:
		return &providers.ResponseFormat{Type: providers.ResponseFormatJSON}
	case outputFormatJSONSchema:
		return &providers.ResponseFormat{
			Type:       providers.ResponseFormatJSONSchema,
			JSONSchema: outputSchema,
			SchemaName: schemaName,
			Strict:     true,
		}
	default: // outputFormatText or anything unexpected
		return nil
	}
}
