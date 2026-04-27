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

	// pgvector ANN: ORDER BY o.embedding <=> $vec LIMIT N is what
	// unlocks the HNSW index. DISTINCT ON / GROUP BY in front of the
	// vector ORDER BY forces a seq-scan + sort. We over-fetch
	// (limit × 4) into the inner ANN, then dedup by entity_id and
	// truncate to the caller's limit.
	rows, err := pool.Query(ctx, `
        SELECT `+selectEntityCols+`, `+selectObserveCols+`
        FROM memory_entities `+entityTableAlias+`
        JOIN (
            SELECT entity_id, observation_id
            FROM (
                SELECT o.entity_id,
                       o.id AS observation_id,
                       row_number() OVER (PARTITION BY o.entity_id ORDER BY o.embedding <=> $2) AS rn
                FROM memory_observations o
                JOIN memory_entities e2 ON e2.id = o.entity_id
                    AND e2.workspace_id = $1
                    AND e2.forgotten = false
                WHERE o.superseded_by IS NULL
                  AND (o.valid_until IS NULL OR o.valid_until > now())
                  AND o.embedding IS NOT NULL
                ORDER BY o.embedding <=> $2
                LIMIT $3 * 4
            ) ann
            WHERE rn = 1
            LIMIT $3
        ) picked ON picked.entity_id = e.id
        JOIN memory_observations o ON o.id = picked.observation_id
        WHERE `+colEntityForgot+`
          AND e.workspace_id = $1`,
		wsID, pgvector.NewVector(embeddings[0]), limit)
	if err != nil {
		return nil, fmt.Errorf("memory: semantic query: %w", err)
	}
	defer rows.Close()

	return scanMemories(rows, scope)
}
