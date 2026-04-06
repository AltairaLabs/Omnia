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

	"github.com/AltairaLabs/PromptKit/runtime/providers/mock"
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
