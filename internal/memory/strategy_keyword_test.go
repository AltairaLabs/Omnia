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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface check.
var _ RetrievalStrategy = (*KeywordStrategy)(nil)

func TestKeywordStrategy_Name(t *testing.T) {
	s := &KeywordStrategy{}
	assert.Equal(t, "keyword", s.Name())
}

func TestKeywordStrategy_Retrieve(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	strategy := &KeywordStrategy{}
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	memories := []*Memory{
		{Type: "preference", Content: "prefers dark mode", Confidence: 0.9, Scope: scope},
		{Type: "fact", Content: "works at Acme Corp", Confidence: 0.85, Scope: scope},
		{Type: "preference", Content: "uses vim editor", Confidence: 0.7, Scope: scope},
	}
	for _, m := range memories {
		require.NoError(t, store.Save(ctx, m))
	}

	results, err := strategy.Retrieve(ctx, pool, scope, "dark", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "prefers dark mode", results[0].Content)
}

func TestKeywordStrategy_Retrieve_CaseInsensitive(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	strategy := &KeywordStrategy{}
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	require.NoError(t, store.Save(ctx, &Memory{
		Type:       "fact",
		Content:    "User speaks Spanish",
		Confidence: 0.9,
		Scope:      scope,
	}))

	// ILIKE should match regardless of case.
	results, err := strategy.Retrieve(ctx, pool, scope, "spanish", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "User speaks Spanish", results[0].Content)
}

func TestKeywordStrategy_Retrieve_NoQuery(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	strategy := &KeywordStrategy{}
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	for i := range 3 {
		require.NoError(t, store.Save(ctx, &Memory{
			Type:       "fact",
			Content:    []string{"fact one", "fact two", "fact three"}[i],
			Confidence: 0.9,
			Scope:      scope,
		}))
	}

	// Empty query should return all (up to limit).
	results, err := strategy.Retrieve(ctx, pool, scope, "", 10)
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestKeywordStrategy_Retrieve_Limit(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	strategy := &KeywordStrategy{}
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	for i := range 5 {
		require.NoError(t, store.Save(ctx, &Memory{
			Type:       "fact",
			Content:    []string{"alpha", "beta", "gamma", "delta", "epsilon"}[i],
			Confidence: 0.9,
			Scope:      scope,
		}))
	}

	results, err := strategy.Retrieve(ctx, pool, scope, "", 2)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestKeywordStrategy_Retrieve_DefaultLimit(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	strategy := &KeywordStrategy{}
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	require.NoError(t, store.Save(ctx, &Memory{
		Type:       "fact",
		Content:    "some fact",
		Confidence: 0.9,
		Scope:      scope,
	}))

	// limit=0 should use default (50) and not error.
	results, err := strategy.Retrieve(ctx, pool, scope, "", 0)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestKeywordStrategy_Retrieve_NoResults(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	strategy := &KeywordStrategy{}
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	require.NoError(t, store.Save(ctx, &Memory{
		Type:       "fact",
		Content:    "totally unrelated content",
		Confidence: 0.9,
		Scope:      scope,
	}))

	results, err := strategy.Retrieve(ctx, pool, scope, "xyzzy_no_match", 10)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestKeywordStrategy_Retrieve_ScopeIsolation(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	strategy := &KeywordStrategy{}
	ctx := context.Background()

	scope1 := testScope(testWorkspace1)
	scope2 := testScope(testWorkspace2)

	// Save in workspace 1.
	require.NoError(t, store.Save(ctx, &Memory{
		Type:       "fact",
		Content:    "workspace one fact",
		Confidence: 0.9,
		Scope:      scope1,
	}))

	// Query workspace 2 — should return empty.
	results, err := strategy.Retrieve(ctx, pool, scope2, "workspace", 10)
	require.NoError(t, err)
	assert.Empty(t, results, "workspace 2 should not see workspace 1 memories")
}

func TestKeywordStrategy_Retrieve_MissingWorkspace(t *testing.T) {
	pool := freshDB(t)
	strategy := &KeywordStrategy{}
	ctx := context.Background()

	_, err := strategy.Retrieve(ctx, pool, map[string]string{}, "query", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}
