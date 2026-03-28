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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/altairalabs/omnia/internal/pgutil"
)

// KeywordStrategy retrieves memories using ILIKE substring matching on observation
// content. This is the default v1 strategy — no embeddings required.
type KeywordStrategy struct{}

// Name returns the strategy identifier.
func (k *KeywordStrategy) Name() string { return "keyword" }

// Retrieve queries memory_entities joined with memory_observations, filtering by
// scope and an ILIKE match on content, ordered by observed_at DESC.
func (k *KeywordStrategy) Retrieve(ctx context.Context, pool *pgxpool.Pool, scope map[string]string, query string, limit int) ([]*Memory, error) {
	if scope[ScopeWorkspaceID] == "" {
		return nil, fmt.Errorf(errWorkspaceRequired)
	}

	sql, qb := buildKeywordQuery(scope, query, limit)

	rows, err := pool.Query(ctx, sql, qb.Args()...)
	if err != nil {
		return nil, fmt.Errorf("memory: keyword retrieve query: %w", err)
	}
	defer rows.Close()

	return scanMemories(rows, scope)
}

// buildKeywordQuery constructs the SQL and arguments for a keyword Retrieve call.
func buildKeywordQuery(scope map[string]string, query string, limit int) (string, *pgutil.QueryBuilder) {
	qb := buildBaseMemoryQuery(scope, nil)

	if query != "" {
		qb.Add("o.content ILIKE $?", "%"+query+"%")
	}

	return formatMemorySQL(qb, limit, 0), qb
}
