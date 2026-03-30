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
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func newTestRetriever(store *PostgresMemoryStore) *OmniaRetriever {
	log := zap.New(zap.UseDevMode(true))
	return NewOmniaRetriever(store, &KeywordStrategy{}, 10, log)
}

func TestOmniaRetriever_RetrieveContext(t *testing.T) {
	store := newStore(t)

	scope := testScope(testWorkspace1)

	// Pre-populate with a memory. The last user message is used as the keyword query
	// via ILIKE, so the stored content must contain the user message as a substring.
	const userQuery = "kubernetes"
	mem := &Memory{
		Type:       "fact",
		Content:    "kubernetes is used for container orchestration",
		Confidence: 0.9,
		Scope:      scope,
	}
	require.NoError(t, store.Save(context.Background(), mem))

	retriever := newTestRetriever(store)

	msgs := []types.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: userQuery},
	}

	results, err := retriever.RetrieveContext(context.Background(), scope, msgs)
	require.NoError(t, err)
	require.NotEmpty(t, results)
	assert.Equal(t, "kubernetes is used for container orchestration", results[0].Content)
}

func TestOmniaRetriever_RetrieveContext_NoMatch(t *testing.T) {
	store := newStore(t)

	scope := testScope(testWorkspace2)

	// Pre-populate with a memory that will NOT match the query.
	mem := &Memory{
		Type:       "fact",
		Content:    "completely unrelated content about widgets",
		Confidence: 0.9,
		Scope:      scope,
	}
	require.NoError(t, store.Save(context.Background(), mem))

	retriever := newTestRetriever(store)

	msgs := []types.Message{
		{Role: "user", Content: "Tell me about kubernetes"},
	}

	results, err := retriever.RetrieveContext(context.Background(), scope, msgs)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestOmniaRetriever_NoUserMessage(t *testing.T) {
	store := newStore(t)

	retriever := newTestRetriever(store)
	scope := testScope(testWorkspace1)

	msgs := []types.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "assistant", Content: "How can I help?"},
	}

	results, err := retriever.RetrieveContext(context.Background(), scope, msgs)
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestOmniaRetriever_EmptyMessages(t *testing.T) {
	store := newStore(t)

	retriever := newTestRetriever(store)
	scope := testScope(testWorkspace1)

	results, err := retriever.RetrieveContext(context.Background(), scope, []types.Message{})
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestOmniaRetriever_LastUserMessageUsed(t *testing.T) {
	store := newStore(t)

	scope := testScope(testWorkspace1)

	// Save a memory whose content contains the last user message as a substring.
	memGo := &Memory{
		Type:       "fact",
		Content:    "goroutines are a core go feature",
		Confidence: 0.9,
		Scope:      scope,
	}
	require.NoError(t, store.Save(context.Background(), memGo))

	retriever := newTestRetriever(store)

	// Last user message "goroutines" should match; earlier "python" should not.
	msgs := []types.Message{
		{Role: "user", Content: "python"},
		{Role: "assistant", Content: "Python is great."},
		{Role: "user", Content: "goroutines"},
	}

	results, err := retriever.RetrieveContext(context.Background(), scope, msgs)
	require.NoError(t, err)
	require.NotEmpty(t, results)
	assert.Equal(t, "goroutines are a core go feature", results[0].Content)
}

// --- helper unit tests -------------------------------------------------------

func TestLastUserMessage_Found(t *testing.T) {
	msgs := []types.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "first user"},
		{Role: "assistant", Content: "response"},
		{Role: "user", Content: "last user"},
	}
	content, ok := lastUserMessage(msgs)
	assert.True(t, ok)
	assert.Equal(t, "last user", content)
}

func TestLastUserMessage_NotFound(t *testing.T) {
	msgs := []types.Message{
		{Role: "system", Content: "sys"},
		{Role: "assistant", Content: "hello"},
	}
	_, ok := lastUserMessage(msgs)
	assert.False(t, ok)
}

func TestLastUserMessage_Empty(t *testing.T) {
	_, ok := lastUserMessage(nil)
	assert.False(t, ok)
}

func TestNewOmniaRetriever(t *testing.T) {
	store := NewPostgresMemoryStore(nil)
	log := zap.New(zap.UseDevMode(true))
	r := NewOmniaRetriever(store, &KeywordStrategy{}, 5, log)
	assert.NotNil(t, r)
}
