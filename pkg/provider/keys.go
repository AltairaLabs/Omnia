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

package provider

// APIKeyEnvVarName returns the primary environment variable name that the
// PromptKit SDK expects for API key credentials of the given provider type.
// Returns empty string for provider types that don't use API key credentials.
func APIKeyEnvVarName(providerType string) string {
	switch Type(providerType) {
	case TypeClaude:
		return "ANTHROPIC_API_KEY"
	case TypeOpenAI:
		return "OPENAI_API_KEY"
	case TypeGemini:
		return "GEMINI_API_KEY"
	default:
		return ""
	}
}
