/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import "testing"

func TestIsPlaceholderCredential(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		// Real placeholders that ship in config/samples/dev/samples.yaml —
		// these are the actual strings that bit us in issue #1037.
		{"anthropic placeholder", "sk-ant-demo-key-replace-with-real-key", true},
		{"openai placeholder", "sk-demo-key-replace-with-real-key", true},
		{"gemini placeholder", "ya29-demo-key-replace-with-real-key", true},

		// Common variants seen in other Helm-chart-style samples.
		{"REPLACE-ME shouty", "REPLACE-ME", true},
		{"your-api-key-here", "your-api-key-here", true},
		{"changeme", "changeme", true},
		{"changeme suffix", "supersecret-CHANGEME", true},

		// Real-shaped keys that should NOT match. Values below are
		// SYNTHETIC — never paste a real API key into a test case,
		// even one you intend to rotate. If a real key ever starts
		// containing "replace-with-real-key" we have bigger problems;
		// the marker is intentionally distinctive.
		{"anthropic shape", "sk-ant-api03-XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX", false},
		{"openai shape", "sk-proj-XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX", false},
		{"gemini shape", "AIzaSyXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX", false},

		// Edge cases — empty / whitespace are missing-key, not placeholder.
		{"empty", "", false},
		{"whitespace only", "   ", false},
		{"single space", " ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPlaceholderCredential(tt.value); got != tt.want {
				t.Errorf("IsPlaceholderCredential(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
