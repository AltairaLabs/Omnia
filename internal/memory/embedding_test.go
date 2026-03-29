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
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type mockEmbeddingProvider struct {
	embedFn    func(ctx context.Context, texts []string) ([][]float32, error)
	dimensions int
}

func (m *mockEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return m.embedFn(ctx, texts)
}

func (m *mockEmbeddingProvider) Dimensions() int { return m.dimensions }

func TestEmbeddingService_EmbedMemory(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	mem := &Memory{
		Type:    "fact",
		Content: "Paris is the capital of France",
		Scope:   map[string]string{ScopeWorkspaceID: "b0000000-0000-0000-0000-000000000001"},
	}
	require.NoError(t, store.Save(ctx, mem))
	require.NotEmpty(t, mem.ID)

	embedding := make([]float32, 1536)
	for i := range embedding {
		embedding[i] = float32(i) * 0.001
	}

	provider := &mockEmbeddingProvider{
		dimensions: 1536,
		embedFn: func(_ context.Context, texts []string) ([][]float32, error) {
			require.Len(t, texts, 1)
			assert.Equal(t, mem.Content, texts[0])
			return [][]float32{embedding}, nil
		},
	}

	log := zap.New(zap.UseDevMode(true))
	svc := NewEmbeddingService(store, provider, log)

	err := svc.EmbedMemory(ctx, mem)
	require.NoError(t, err)

	// Verify via direct SQL that the embedding is non-null.
	var hasEmbedding bool
	err = store.Pool().QueryRow(ctx, `
		SELECT embedding IS NOT NULL
		FROM memory_observations
		WHERE entity_id = $1
		ORDER BY observed_at DESC
		LIMIT 1`, mem.ID).Scan(&hasEmbedding)
	require.NoError(t, err)
	assert.True(t, hasEmbedding, "embedding should be non-null after EmbedMemory")
}

func TestEmbeddingService_ProviderError(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	mem := &Memory{
		Type:    "fact",
		Content: "some content",
		Scope:   map[string]string{ScopeWorkspaceID: "b0000000-0000-0000-0000-000000000002"},
	}
	require.NoError(t, store.Save(ctx, mem))

	providerErr := errors.New("embedding API unavailable")
	provider := &mockEmbeddingProvider{
		dimensions: 1536,
		embedFn: func(_ context.Context, _ []string) ([][]float32, error) {
			return nil, providerErr
		},
	}

	log := zap.New(zap.UseDevMode(true))
	svc := NewEmbeddingService(store, provider, log)

	err := svc.EmbedMemory(ctx, mem)
	require.Error(t, err)
	assert.ErrorIs(t, err, providerErr)
}

func TestEmbeddingService_EmptyResult(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	mem := &Memory{
		Type:    "fact",
		Content: "another content",
		Scope:   map[string]string{ScopeWorkspaceID: "b0000000-0000-0000-0000-000000000003"},
	}
	require.NoError(t, store.Save(ctx, mem))

	provider := &mockEmbeddingProvider{
		dimensions: 1536,
		embedFn: func(_ context.Context, _ []string) ([][]float32, error) {
			return [][]float32{}, nil
		},
	}

	log := zap.New(zap.UseDevMode(true))
	svc := NewEmbeddingService(store, provider, log)

	err := svc.EmbedMemory(ctx, mem)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "embed returned empty result")
}
