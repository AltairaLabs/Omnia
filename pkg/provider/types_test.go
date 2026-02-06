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

package provider

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		typ      Type
		expected bool
	}{
		{"claude", TypeClaude, true},
		{"openai", TypeOpenAI, true},
		{"gemini", TypeGemini, true},
		{"ollama", TypeOllama, true},
		{"mock", TypeMock, true},
		{"bedrock", TypeBedrock, true},
		{"vertex", TypeVertex, true},
		{"azure-ai", TypeAzureAI, true},
		{"invalid", Type("invalid"), false},
		{"empty", Type(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.typ.IsValid())
		})
	}
}

func TestType_String(t *testing.T) {
	assert.Equal(t, "claude", TypeClaude.String())
	assert.Equal(t, "ollama", TypeOllama.String())
}

func TestType_RequiresCredentials(t *testing.T) {
	tests := []struct {
		name     string
		typ      Type
		expected bool
	}{
		{"claude", TypeClaude, true},
		{"openai", TypeOpenAI, true},
		{"gemini", TypeGemini, true},
		{"ollama", TypeOllama, false},
		{"mock", TypeMock, false},
		{"bedrock", TypeBedrock, false},
		{"vertex", TypeVertex, false},
		{"azure-ai", TypeAzureAI, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.typ.RequiresCredentials())
		})
	}
}

func TestValidTypes_Complete(t *testing.T) {
	// Ensure ValidTypes contains all defined constants
	expected := []Type{TypeClaude, TypeOpenAI, TypeGemini, TypeOllama, TypeMock, TypeBedrock, TypeVertex, TypeAzureAI}
	assert.ElementsMatch(t, expected, ValidTypes)
}

// TestKubebuilderEnumAnnotation verifies that the kubebuilder enum annotation
// for ProviderType in agentruntime_types.go matches the ValidTypes defined here.
// This catches drift when new provider types are added.
func TestKubebuilderEnumAnnotation(t *testing.T) {
	// Find the api/v1alpha1/agentruntime_types.go file
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to get current file path")

	// Navigate from pkg/provider/ to api/v1alpha1/
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(currentFile)))
	typesFile := filepath.Join(repoRoot, "api", "v1alpha1", "agentruntime_types.go")

	// Read the file and find the kubebuilder enum annotation for ProviderType
	content, err := os.ReadFile(typesFile)
	require.NoError(t, err, "failed to read agentruntime_types.go")

	// Look for the pattern: enum annotation followed by "type ProviderType string"
	// This ensures we find the right enum, not FacadeType or other enums
	enumRegex := regexp.MustCompile(`\+kubebuilder:validation:Enum=([a-z;-]+)\s*\n\s*type ProviderType string`)

	matches := enumRegex.FindStringSubmatch(string(content))
	require.Len(t, matches, 2, "kubebuilder enum annotation for ProviderType not found in agentruntime_types.go")

	enumValues := strings.Split(matches[1], ";")

	// Convert ValidTypes to strings for comparison
	validStrings := make([]string, len(ValidTypes))
	for i, vt := range ValidTypes {
		validStrings[i] = string(vt)
	}

	// Sort both for comparison
	sort.Strings(enumValues)
	sort.Strings(validStrings)

	assert.Equal(t, validStrings, enumValues,
		"kubebuilder enum annotation in api/v1alpha1/agentruntime_types.go does not match pkg/provider.ValidTypes. "+
			"Update the annotation to: // +kubebuilder:validation:Enum=%s",
		strings.Join(validStrings, ";"))
}
