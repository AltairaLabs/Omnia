/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface check.
var _ MemoryPopulator = (*LLMConversationPopulator)(nil)

// MockLLMProvider is a test double for LLMProvider.
type MockLLMProvider struct {
	Response string
	Err      error
}

func (m *MockLLMProvider) Complete(_ context.Context, _, _ string) (string, error) {
	return m.Response, m.Err
}

func TestLLMConversationPopulator_Populate(t *testing.T) {
	llmResponse := `{
		"entities": [
			{"name": "dark mode", "kind": "preference", "metadata": {"theme": "dark"}},
			{"name": "Go programming", "kind": "topic", "metadata": {}}
		],
		"observations": [
			{"entity_name": "dark mode", "content": "User prefers dark mode for coding", "confidence": 0.9},
			{"entity_name": "Go programming", "content": "User is working on a Go project", "confidence": 0.7}
		],
		"relations": [
			{"source": "dark mode", "target": "Go programming", "type": "relates_to"}
		]
	}`

	mock := &MockLLMProvider{Response: llmResponse}
	pop := NewLLMConversationPopulator(mock, logr.Discard())

	source := PopulationSource{
		Scope: map[string]string{"session_id": "sess-123"},
		Messages: []SimpleMessage{
			{Role: "user", Content: "I prefer dark mode when coding in Go"},
			{Role: "assistant", Content: "I'll remember that preference."},
		},
	}

	result, err := pop.Populate(context.Background(), source)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Len(t, result.Entities, 2)
	assert.Equal(t, "dark mode", result.Entities[0].Name)
	assert.Equal(t, "preference", result.Entities[0].Kind)
	assert.Equal(t, "dark", result.Entities[0].Metadata["theme"])
	assert.Equal(t, "Go programming", result.Entities[1].Name)
	assert.Equal(t, "topic", result.Entities[1].Kind)

	require.Len(t, result.Observations, 2)
	assert.Equal(t, "dark mode", result.Observations[0].EntityName)
	assert.Equal(t, "User prefers dark mode for coding", result.Observations[0].Content)
	assert.InDelta(t, 0.9, float64(result.Observations[0].Confidence), 0.01)
	assert.Equal(t, "sess-123", result.Observations[0].SessionID)
	assert.InDelta(t, 0.7, float64(result.Observations[1].Confidence), 0.01)

	require.Len(t, result.Relations, 1)
	assert.Equal(t, "dark mode", result.Relations[0].SourceName)
	assert.Equal(t, "Go programming", result.Relations[0].TargetName)
	assert.Equal(t, "relates_to", result.Relations[0].RelationType)
	assert.Equal(t, float32(1.0), result.Relations[0].Weight)
}

func TestLLMConversationPopulator_MalformedJSON(t *testing.T) {
	mock := &MockLLMProvider{Response: "not json at all"}
	pop := NewLLMConversationPopulator(mock, logr.Discard())

	source := PopulationSource{
		Messages: []SimpleMessage{
			{Role: "user", Content: "Hello there"},
			{Role: "assistant", Content: "Hi!"},
		},
	}

	result, err := pop.Populate(context.Background(), source)
	require.NoError(t, err)
	require.NotNil(t, result)
	// Fallback produces at least one entity from the rule-based populator.
	assert.NotEmpty(t, result.Entities)
}

func TestLLMConversationPopulator_LLMError(t *testing.T) {
	mock := &MockLLMProvider{Err: errors.New("provider unavailable")}
	pop := NewLLMConversationPopulator(mock, logr.Discard())

	source := PopulationSource{
		Messages: []SimpleMessage{
			{Role: "user", Content: "Tell me about Go"},
			{Role: "assistant", Content: "Go is great."},
		},
	}

	result, err := pop.Populate(context.Background(), source)
	require.NoError(t, err)
	require.NotNil(t, result)
	// Fallback produces at least one entity.
	assert.NotEmpty(t, result.Entities)
}

func TestLLMConversationPopulator_EmptyEntities(t *testing.T) {
	llmResponse := `{"entities":[],"observations":[],"relations":[]}`

	mock := &MockLLMProvider{Response: llmResponse}
	pop := NewLLMConversationPopulator(mock, logr.Discard())

	source := PopulationSource{
		Messages: []SimpleMessage{
			{Role: "user", Content: "Just saying hi"},
		},
	}

	result, err := pop.Populate(context.Background(), source)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Entities)
	assert.Empty(t, result.Observations)
	assert.Empty(t, result.Relations)
}

func TestLLMConversationPopulator_EmptyMessages(t *testing.T) {
	mock := &MockLLMProvider{Response: "should not be called"}
	pop := NewLLMConversationPopulator(mock, logr.Discard())

	source := PopulationSource{
		Messages: []SimpleMessage{},
	}

	result, err := pop.Populate(context.Background(), source)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Entities)
}

func TestLLMConversationPopulator_SourceType(t *testing.T) {
	pop := NewLLMConversationPopulator(nil, logr.Discard())
	assert.Equal(t, "conversation_extraction", pop.SourceType())
}

func TestLLMConversationPopulator_TrustModel(t *testing.T) {
	pop := NewLLMConversationPopulator(nil, logr.Discard())
	assert.Equal(t, "inferred", pop.TrustModel())
}
