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

import "testing"

func TestAPIKeyEnvVarName(t *testing.T) {
	tests := []struct {
		providerType string
		want         string
	}{
		{"claude", "ANTHROPIC_API_KEY"},
		{"openai", "OPENAI_API_KEY"},
		{"gemini", "GEMINI_API_KEY"},
		{"ollama", ""},
		{"mock", ""},
		{"bedrock", ""},
		{"vertex", ""},
		{"azure-ai", ""},
		{"unknown", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.providerType, func(t *testing.T) {
			got := APIKeyEnvVarName(tt.providerType)
			if got != tt.want {
				t.Errorf("APIKeyEnvVarName(%q) = %q, want %q", tt.providerType, got, tt.want)
			}
		})
	}
}
