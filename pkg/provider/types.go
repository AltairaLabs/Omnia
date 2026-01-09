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

// Package provider defines shared provider type constants used across Omnia components.
// This is the single source of truth for provider types.
//
// IMPORTANT: When adding new provider types:
//  1. Add the constant here
//  2. Add to ValidTypes slice
//  3. Update the kubebuilder enum annotation in api/v1alpha1/agentruntime_types.go
//  4. Run tests to verify consistency
package provider

// Type defines the LLM provider type.
type Type string

// Provider type constants.
const (
	// TypeAuto uses PromptKit's auto-detection based on available credentials.
	TypeAuto Type = "auto"
	// TypeClaude uses Anthropic's Claude models.
	TypeClaude Type = "claude"
	// TypeOpenAI uses OpenAI's GPT models.
	TypeOpenAI Type = "openai"
	// TypeGemini uses Google's Gemini models.
	TypeGemini Type = "gemini"
	// TypeOllama uses locally-hosted Ollama models.
	// Does not require API credentials. Requires baseURL to be set.
	TypeOllama Type = "ollama"
	// TypeMock uses PromptKit's mock provider for testing.
	// Does not require API credentials. Returns canned responses.
	TypeMock Type = "mock"
)

// ValidTypes contains all valid provider types.
// Used for validation across components.
var ValidTypes = []Type{
	TypeAuto,
	TypeClaude,
	TypeOpenAI,
	TypeGemini,
	TypeOllama,
	TypeMock,
}

// IsValid returns true if the provider type is valid.
func (t Type) IsValid() bool {
	for _, valid := range ValidTypes {
		if t == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the provider type.
func (t Type) String() string {
	return string(t)
}

// RequiresCredentials returns true if the provider type requires API credentials.
func (t Type) RequiresCredentials() bool {
	switch t {
	case TypeOllama, TypeMock:
		return false
	default:
		return true
	}
}
