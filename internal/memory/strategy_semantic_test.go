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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface check.
var _ RetrievalStrategy = (*SemanticStrategy)(nil)

func TestSemanticStrategy_Name(t *testing.T) {
	s := NewSemanticStrategy(nil)
	assert.Equal(t, "semantic", s.Name())
}

func TestSemanticStrategy_NilProvider(t *testing.T) {
	s := NewSemanticStrategy(nil)
	ctx := context.Background()
	_, err := s.Retrieve(ctx, nil, map[string]string{ScopeWorkspaceID: testWorkspace1}, "query", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "embedding provider")
}

// embeddingDims matches the vector(1536) column definition in the memory schema migration.
const embeddingDims = 1536

// unitEmbedding returns a 1536-dim vector with value 1.0 at index 0, 0.0 elsewhere.
func unitEmbedding() []float32 {
	v := make([]float32, embeddingDims)
	v[0] = 1.0
	return v
}

// nearEmbedding returns a 1536-dim vector similar to unitEmbedding (high cosine similarity).
func nearEmbedding() []float32 {
	v := make([]float32, embeddingDims)
	v[0] = 0.9
	v[1] = 0.1
	return v
}

func TestSemanticStrategy_ProviderError(t *testing.T) {
	providerErr := errors.New("embedding service unavailable")
	provider := &mockEmbeddingProvider{
		dimensions: embeddingDims,
		embedFn: func(_ context.Context, _ []string) ([][]float32, error) {
			return nil, providerErr
		},
	}
	s := NewSemanticStrategy(provider)
	pool := freshDB(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	_, err := s.Retrieve(ctx, pool, scope, "query", 10)
	require.Error(t, err)
	assert.ErrorIs(t, err, providerErr)
}

func TestSemanticStrategy_Retrieve(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// Save a memory.
	mem := &Memory{
		Type:       "fact",
		Content:    "user prefers dark mode",
		Confidence: 0.9,
		Scope:      scope,
	}
	require.NoError(t, store.Save(ctx, mem))
	require.NotEmpty(t, mem.ID)

	// Set its embedding.
	require.NoError(t, store.UpdateEmbedding(ctx, mem.ID, unitEmbedding()))

	// Provider returns a very similar embedding.
	provider := &mockEmbeddingProvider{
		dimensions: embeddingDims,
		embedFn: func(_ context.Context, _ []string) ([][]float32, error) {
			return [][]float32{nearEmbedding()}, nil
		},
	}

	s := NewSemanticStrategy(provider)
	results, err := s.Retrieve(ctx, pool, scope, "dark mode", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, mem.ID, results[0].ID)
	assert.Equal(t, "user prefers dark mode", results[0].Content)
}

func TestSemanticStrategy_NoEmbeddings(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// Save a memory without setting any embedding.
	mem := &Memory{
		Type:       "fact",
		Content:    "user has no embedding set",
		Confidence: 0.9,
		Scope:      scope,
	}
	require.NoError(t, store.Save(ctx, mem))

	provider := &mockEmbeddingProvider{
		dimensions: embeddingDims,
		embedFn: func(_ context.Context, _ []string) ([][]float32, error) {
			return [][]float32{unitEmbedding()}, nil
		},
	}

	s := NewSemanticStrategy(provider)
	results, err := s.Retrieve(ctx, pool, scope, "query", 10)
	require.NoError(t, err)
	assert.Empty(t, results, "memories without embeddings should not be returned")
}

func TestSemanticStrategy_MissingWorkspace(t *testing.T) {
	pool := freshDB(t)
	provider := &mockEmbeddingProvider{
		dimensions: embeddingDims,
		embedFn: func(_ context.Context, _ []string) ([][]float32, error) {
			return [][]float32{unitEmbedding()}, nil
		},
	}
	s := NewSemanticStrategy(provider)
	ctx := context.Background()

	_, err := s.Retrieve(ctx, pool, map[string]string{}, "query", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

func TestSemanticStrategy_DefaultLimit(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	// Save a memory with an embedding.
	mem := &Memory{
		Type:       "fact",
		Content:    "default limit test",
		Confidence: 0.9,
		Scope:      scope,
	}
	require.NoError(t, store.Save(ctx, mem))
	require.NoError(t, store.UpdateEmbedding(ctx, mem.ID, unitEmbedding()))

	provider := &mockEmbeddingProvider{
		dimensions: embeddingDims,
		embedFn: func(_ context.Context, _ []string) ([][]float32, error) {
			return [][]float32{unitEmbedding()}, nil
		},
	}

	s := NewSemanticStrategy(provider)
	// limit=0 should apply default (5) and not error.
	results, err := s.Retrieve(ctx, pool, scope, "test", 0)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}
