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
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

// EmbeddingProvider generates dense vector embeddings for text inputs.
// Implementations must be safe for concurrent use.
type EmbeddingProvider interface {
	// Embed returns one embedding vector per input text, in the same order.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// Dimensions returns the vector length produced by this provider.
	Dimensions() int
}

// SemanticStrategy retrieves memories using pgvector cosine similarity.
// Requires an EmbeddingProvider to embed the query text.
type SemanticStrategy struct {
	provider EmbeddingProvider
}

// NewSemanticStrategy creates a SemanticStrategy backed by the given provider.
func NewSemanticStrategy(provider EmbeddingProvider) *SemanticStrategy {
	return &SemanticStrategy{provider: provider}
}

// Name returns the strategy identifier.
func (s *SemanticStrategy) Name() string { return "semantic" }

// Retrieve returns the top-limit memories ranked by cosine similarity to the query.
// The query is embedded via the provider, then a pgvector <=> (cosine distance) query
// picks the best-matching observation per entity.
func (s *SemanticStrategy) Retrieve(ctx context.Context, pool *pgxpool.Pool, scope map[string]string, query string, limit int) ([]*Memory, error) {
	if s.provider == nil {
		return nil, fmt.Errorf("memory: semantic retrieval requires an embedding provider")
	}

	// Embed the query.
	embeddings, err := s.provider.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("memory: embed query: %w", err)
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("memory: embed returned empty result")
	}

	if limit <= 0 {
		limit = 5
	}
	wsID := scope[ScopeWorkspaceID]
	if wsID == "" {
		return nil, errors.New(errWorkspaceRequired)
	}

	// pgvector cosine distance: <=> operator, ORDER BY ascending = most similar first.
	// DISTINCT ON (e.id) picks the observation with smallest cosine distance per entity.
	rows, err := pool.Query(ctx, `
        SELECT DISTINCT ON (e.id) `+selectEntityCols+`, `+selectObserveCols+`
        FROM memory_entities `+entityTableAlias+observationJoin+`
        WHERE `+colEntityForgot+`
          AND e.workspace_id = $1
          AND o.embedding IS NOT NULL
        ORDER BY e.id, o.embedding <=> $2
        LIMIT $3`,
		wsID, pgvector.NewVector(embeddings[0]), limit)
	if err != nil {
		return nil, fmt.Errorf("memory: semantic query: %w", err)
	}
	defer rows.Close()

	return scanMemories(rows, scope)
}
