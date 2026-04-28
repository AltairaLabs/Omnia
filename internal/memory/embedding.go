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
	"fmt"

	"github.com/go-logr/logr"
)

// EmbeddingService generates embeddings for memory observations and updates pgvector columns.
type EmbeddingService struct {
	store     *PostgresMemoryStore
	provider  EmbeddingProvider
	log       logr.Logger
	modelName string // stamped on every embedding write so the re-embed worker can detect model swaps
}

// NewEmbeddingService creates a new EmbeddingService.
func NewEmbeddingService(store *PostgresMemoryStore, provider EmbeddingProvider, log logr.Logger) *EmbeddingService {
	return &EmbeddingService{store: store, provider: provider, log: log.WithName("embedding")}
}

// WithModelName configures the embedding-model identifier stamped
// on every write. cmd/memory-api wires this to the configured
// Provider CRD name; without it, every EmbedMemory / WriteEmbedding
// call leaves embedding_model NULL and the re-embed worker sees
// the row as stale on its next tick.
func (s *EmbeddingService) WithModelName(name string) *EmbeddingService {
	s.modelName = name
	return s
}

// ModelName returns the configured embedding-model identifier.
// Empty when the service was constructed without a model context.
func (s *EmbeddingService) ModelName() string {
	return s.modelName
}

// Provider returns the underlying embedding provider so other consumers
// (e.g. the EE consent classifier) can share it without needing access
// to internal state. Safe to call after NewEmbeddingService.
func (s *EmbeddingService) Provider() EmbeddingProvider {
	return s.provider
}

// EmbedMemory generates and stores an embedding for the given memory's content.
func (s *EmbeddingService) EmbedMemory(ctx context.Context, mem *Memory) error {
	s.log.V(1).Info("embedding memory", "memoryID", mem.ID, "contentLength", len(mem.Content))

	embeddings, err := s.provider.Embed(ctx, []string{mem.Content})
	if err != nil {
		return fmt.Errorf("memory: embed: %w", err)
	}
	if len(embeddings) == 0 || len(embeddings[0]) == 0 {
		return fmt.Errorf("memory: embed returned empty result")
	}

	if err := s.store.UpdateEmbedding(ctx, mem.ID, embeddings[0], s.modelName); err != nil {
		return fmt.Errorf("memory: store embedding: %w", err)
	}

	s.log.V(1).Info("memory embedded", "memoryID", mem.ID, "dimensions", len(embeddings[0]))
	return nil
}

// WriteEmbedding stores an already-computed embedding vector for an
// entity, skipping the provider Embed round trip. Used by callers
// that computed the embedding for another reason (e.g. dedup
// similarity) and want to avoid embedding the same content twice.
// Stamps the configured model name so the re-embed worker can
// detect a model swap and refresh stale rows.
func (s *EmbeddingService) WriteEmbedding(ctx context.Context, entityID string, vec []float32) error {
	if err := s.store.UpdateEmbedding(ctx, entityID, vec, s.modelName); err != nil {
		return fmt.Errorf("memory: store embedding: %w", err)
	}
	return nil
}
