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

import (
	"context"
	"os"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/credentials"
	"github.com/AltairaLabs/PromptKit/runtime/providers/mock"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultScenarioRepo_SubstitutesEmptyID(t *testing.T) {
	// Create a file-based repo with a "default" scenario containing tool calls.
	configYAML := `
defaultResponse: "fallback text"
scenarios:
  default:
    turns:
      1:
        type: tool_calls
        content: ""
        tool_calls:
          - name: get_location
            arguments:
              accuracy: high
`
	tmpFile := t.TempDir() + "/mock.yaml"
	require.NoError(t, os.WriteFile(tmpFile, []byte(configYAML), 0o644))

	inner, err := mock.NewFileMockRepository(tmpFile)
	require.NoError(t, err)

	wrapped := &defaultScenarioRepo{inner: inner}
	ctx := context.Background()

	// Empty ScenarioID should be substituted with "default" and match the
	// scenario's turn 1 (tool_calls), not the global defaultResponse.
	turn, err := wrapped.GetTurn(ctx, mock.ResponseParams{
		ScenarioID: "",
		TurnNumber: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, "tool_calls", turn.Type)
	assert.Len(t, turn.ToolCalls, 1)
	assert.Equal(t, "get_location", turn.ToolCalls[0].Name)
}

func TestDefaultScenarioRepo_PassesThroughNonEmpty(t *testing.T) {
	configYAML := `
scenarios:
  custom:
    turns:
      1: "custom response"
  default:
    turns:
      1: "default response"
`
	tmpFile := t.TempDir() + "/mock.yaml"
	require.NoError(t, os.WriteFile(tmpFile, []byte(configYAML), 0o644))

	inner, err := mock.NewFileMockRepository(tmpFile)
	require.NoError(t, err)

	wrapped := &defaultScenarioRepo{inner: inner}
	ctx := context.Background()

	// Non-empty ScenarioID should be passed through unchanged.
	turn, err := wrapped.GetTurn(ctx, mock.ResponseParams{
		ScenarioID: "custom",
		TurnNumber: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, "custom response", turn.Content)
}

func TestBuildProviderSpec_StaticProvider(t *testing.T) {
	s := &Server{
		log:          logr.Discard(),
		providerType: "claude",
		model:        "claude-sonnet-4-20250514",
		baseURL:      "https://api.anthropic.com",
	}

	spec := s.buildProviderSpec()
	assert.Equal(t, "claude", spec.Type)
	assert.Equal(t, "claude-sonnet-4-20250514", spec.Model)
	assert.Equal(t, "https://api.anthropic.com", spec.BaseURL)
	assert.Empty(t, spec.Platform, "static provider must not carry platform")
	assert.Nil(t, spec.PlatformConfig)
	assert.Nil(t, spec.Credential)
}

func TestBuildProviderSpec_Headers(t *testing.T) {
	// OpenRouter-style custom attribution headers flow into the spec.
	s := &Server{
		log:          logr.Discard(),
		providerType: "openai",
		baseURL:      "https://openrouter.ai/api/v1",
		headers: map[string]string{
			"HTTP-Referer": "https://example.com",
			"X-Title":      "omnia",
		},
	}

	spec := s.buildProviderSpec()
	assert.Equal(t, "https://example.com", spec.Headers["HTTP-Referer"])
	assert.Equal(t, "omnia", spec.Headers["X-Title"])
}

func TestBuildProviderSpec_BedrockMapsClaudeModel(t *testing.T) {
	// Pick any model known to be in BedrockModelMapping.
	var claudeName string
	for name := range credentials.BedrockModelMapping {
		claudeName = name
		break
	}
	require.NotEmpty(t, claudeName, "BedrockModelMapping is empty")

	s := &Server{
		log:            logr.Discard(),
		providerType:   "claude",
		model:          claudeName,
		platformType:   "bedrock",
		platformRegion: "us-east-1",
		authType:       "workloadIdentity",
	}

	spec := s.buildProviderSpec()
	assert.Equal(t, "bedrock", spec.Platform)
	require.NotNil(t, spec.PlatformConfig)
	assert.Equal(t, "us-east-1", spec.PlatformConfig.Region)
	assert.Equal(t, credentials.BedrockModelMapping[claudeName], spec.Model,
		"claude release name should map to bedrock model id")
}

func TestBuildProviderSpec_VertexPlatform(t *testing.T) {
	s := &Server{
		log:             logr.Discard(),
		providerType:    "gemini",
		model:           "gemini-1.5-pro",
		platformType:    "vertex",
		platformRegion:  "us-central1",
		platformProject: "my-project",
		authType:        "workloadIdentity",
	}

	spec := s.buildProviderSpec()
	assert.Equal(t, "vertex", spec.Platform)
	require.NotNil(t, spec.PlatformConfig)
	assert.Equal(t, "my-project", spec.PlatformConfig.Project)
	assert.Equal(t, "us-central1", spec.PlatformConfig.Region)
	// Model is not auto-mapped for vertex — passed through as-is.
	assert.Equal(t, "gemini-1.5-pro", spec.Model)
}

func TestBuildProviderSpec_AzurePlatform(t *testing.T) {
	s := &Server{
		log:              logr.Discard(),
		providerType:     "openai",
		model:            "gpt-4o",
		platformType:     "azure",
		platformEndpoint: "https://example.openai.azure.com",
		authType:         "workloadIdentity",
	}

	spec := s.buildProviderSpec()
	assert.Equal(t, "azure", spec.Platform)
	require.NotNil(t, spec.PlatformConfig)
	assert.Equal(t, "https://example.openai.azure.com", spec.PlatformConfig.Endpoint)
}

func TestDefaultScenarioRepo_GetResponse(t *testing.T) {
	configYAML := `
scenarios:
  default:
    defaultResponse: "default scenario text"
`
	tmpFile := t.TempDir() + "/mock.yaml"
	require.NoError(t, os.WriteFile(tmpFile, []byte(configYAML), 0o644))

	inner, err := mock.NewFileMockRepository(tmpFile)
	require.NoError(t, err)

	wrapped := &defaultScenarioRepo{inner: inner}
	ctx := context.Background()

	resp, err := wrapped.GetResponse(ctx, mock.ResponseParams{ScenarioID: ""})
	require.NoError(t, err)
	assert.Equal(t, "default scenario text", resp)
}
