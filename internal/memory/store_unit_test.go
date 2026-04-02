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

	"github.com/altairalabs/omnia/internal/pgutil"
)

// --- helper function unit tests (no database needed) -------------------------

func TestCopyScope(t *testing.T) {
	original := map[string]string{"a": "1", "b": "2"}
	copied := copyScope(original)

	assert.Equal(t, original, copied)

	// Mutating the copy should not affect the original.
	copied["c"] = "3"
	assert.NotContains(t, original, "c")
}

func TestCopyScope_Nil(t *testing.T) {
	copied := copyScope(nil)
	assert.NotNil(t, copied)
	assert.Empty(t, copied)
}

func TestScopeOrNil(t *testing.T) {
	scope := map[string]string{"key": "value", "empty": ""}

	val := scopeOrNil(scope, "key")
	require.NotNil(t, val)
	assert.Equal(t, "value", *val)

	// Empty string returns nil.
	assert.Nil(t, scopeOrNil(scope, "empty"))

	// Missing key returns nil.
	assert.Nil(t, scopeOrNil(scope, "missing"))
}

func TestMarshalMetadata(t *testing.T) {
	// Nil metadata returns "{}".
	b, err := marshalMetadata(nil)
	require.NoError(t, err)
	assert.Equal(t, []byte("{}"), b)

	// Empty map returns "{}".
	b, err = marshalMetadata(map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, []byte("{}"), b)

	// Non-empty metadata serializes correctly.
	b, err = marshalMetadata(map[string]any{"key": "val"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"key":"val"}`, string(b))
}

func TestAddScopeFilters(t *testing.T) {
	var qb pgutil.QueryBuilder

	// No user_id — no filter added.
	addScopeFilters(&qb, map[string]string{})
	assert.Empty(t, qb.Args())

	// With user_id — filter added.
	addScopeFilters(&qb, map[string]string{ScopeUserID: "u1"})
	assert.Len(t, qb.Args(), 1)
	assert.Equal(t, "u1", qb.Args()[0])
}

func TestAddTypeFilters(t *testing.T) {
	tests := []struct {
		name     string
		types    []string
		wantArgs int
	}{
		{"no types", nil, 0},
		{"empty slice", []string{}, 0},
		{"single type", []string{"fact"}, 1},
		{"multiple types", []string{"fact", "preference"}, 1}, // ANY($?) uses one arg
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var qb pgutil.QueryBuilder
			addTypeFilters(&qb, tt.types)
			assert.Len(t, qb.Args(), tt.wantArgs)
		})
	}
}

// --- validation tests (nil pool is fine since errors happen before DB call) ---

func TestSave_MissingWorkspace(t *testing.T) {
	store := &PostgresMemoryStore{} // nil pool — validation fails before use
	err := store.Save(context.Background(), &Memory{Scope: map[string]string{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

func TestRetrieve_MissingWorkspace(t *testing.T) {
	store := &PostgresMemoryStore{}
	_, err := store.Retrieve(context.Background(), map[string]string{}, "", RetrieveOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

func TestList_MissingWorkspace(t *testing.T) {
	store := &PostgresMemoryStore{}
	_, err := store.List(context.Background(), map[string]string{}, ListOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

func TestDelete_MissingWorkspace(t *testing.T) {
	store := &PostgresMemoryStore{}
	err := store.Delete(context.Background(), map[string]string{}, "id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

func TestDeleteAll_MissingWorkspace(t *testing.T) {
	store := &PostgresMemoryStore{}
	err := store.DeleteAll(context.Background(), map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

func TestExportAll_MissingWorkspace(t *testing.T) {
	store := &PostgresMemoryStore{} // nil pool — validation fails before use
	_, err := store.ExportAll(context.Background(), map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}

func TestNewPostgresMemoryStore(t *testing.T) {
	store := NewPostgresMemoryStore(nil)
	assert.NotNil(t, store)
}

// --- query builder unit tests ------------------------------------------------

func TestBuildRetrieveQuery_Basic(t *testing.T) {
	scope := map[string]string{ScopeWorkspaceID: "ws-uuid"}
	sql, qb := buildRetrieveQuery(scope, "", RetrieveOptions{})

	assert.Contains(t, sql, "SELECT DISTINCT ON (e.id)")
	assert.Contains(t, sql, "e.forgotten = false")
	assert.Contains(t, sql, "LIMIT")
	assert.Len(t, qb.Args(), 2) // workspace_id + limit
}

func TestBuildRetrieveQuery_WithAllFilters(t *testing.T) {
	scope := map[string]string{ScopeWorkspaceID: "ws-uuid", ScopeUserID: "user-1"}
	sql, qb := buildRetrieveQuery(scope, "search term", RetrieveOptions{
		Types:         []string{"fact"},
		Limit:         10,
		MinConfidence: 0.8,
	})

	assert.Contains(t, sql, "ILIKE")
	assert.Contains(t, sql, "confidence")
	assert.Contains(t, sql, "LIMIT")
	// workspace_id, user_id, kind, confidence, ilike, limit = 6 args
	assert.Len(t, qb.Args(), 6)
}

func TestBuildRetrieveQuery_DefaultLimit(t *testing.T) {
	scope := map[string]string{ScopeWorkspaceID: "ws-uuid"}
	sql, qb := buildRetrieveQuery(scope, "", RetrieveOptions{Limit: 0})

	assert.Contains(t, sql, "LIMIT")
	// Last arg should be 50 (default limit).
	args := qb.Args()
	assert.Equal(t, 50, args[len(args)-1])
}

func TestBuildListQuery_Basic(t *testing.T) {
	scope := map[string]string{ScopeWorkspaceID: "ws-uuid"}
	sql, qb := buildListQuery(scope, ListOptions{})

	assert.Contains(t, sql, "SELECT DISTINCT ON (e.id)")
	assert.Contains(t, sql, "LIMIT")
	assert.Len(t, qb.Args(), 2) // workspace_id + limit
}

func TestBuildListQuery_WithPagination(t *testing.T) {
	scope := map[string]string{ScopeWorkspaceID: "ws-uuid", ScopeUserID: "u1"}
	sql, qb := buildListQuery(scope, ListOptions{Limit: 25, Offset: 10})

	assert.Contains(t, sql, "LIMIT")
	assert.Contains(t, sql, "OFFSET")
	// workspace_id, user_id, limit, offset = 4 args
	assert.Len(t, qb.Args(), 4)
}

func TestBuildListQuery_WithTypeFilter(t *testing.T) {
	scope := map[string]string{ScopeWorkspaceID: "ws-uuid"}
	_, qb := buildListQuery(scope, ListOptions{Types: []string{"pref", "fact"}})

	// workspace_id, ANY(types), limit = 3 args
	assert.Len(t, qb.Args(), 3)
}

func TestBuildDeleteAllQuery_Basic(t *testing.T) {
	scope := map[string]string{ScopeWorkspaceID: "ws-uuid"}
	sql, qb := buildDeleteAllQuery(scope)

	assert.Contains(t, sql, "DELETE FROM memory_entities")
	assert.Contains(t, sql, "workspace_id=$1")
	assert.Len(t, qb.Args(), 1)
}

func TestBuildDeleteAllQuery_WithUserID(t *testing.T) {
	scope := map[string]string{ScopeWorkspaceID: "ws-uuid", ScopeUserID: "u1"}
	sql, qb := buildDeleteAllQuery(scope)

	assert.Contains(t, sql, "virtual_user_id=$2")
	assert.Len(t, qb.Args(), 2)
}

func TestBuildBatchDeleteQuery_Basic(t *testing.T) {
	scope := map[string]string{ScopeWorkspaceID: "ws-uuid"}
	sql, qb := buildBatchDeleteQuery(scope, 500)

	assert.Contains(t, sql, "DELETE FROM memory_entities WHERE id IN")
	assert.Contains(t, sql, "SELECT id FROM memory_entities WHERE 1=1")
	assert.Contains(t, sql, "workspace_id=$1")
	assert.Contains(t, sql, "LIMIT")
	// workspace_id + limit = 2 args
	assert.Len(t, qb.Args(), 2)
	assert.Equal(t, 500, qb.Args()[1])
}

func TestBuildBatchDeleteQuery_WithUserID(t *testing.T) {
	scope := map[string]string{ScopeWorkspaceID: "ws-uuid", ScopeUserID: "u1"}
	sql, qb := buildBatchDeleteQuery(scope, 100)

	assert.Contains(t, sql, "virtual_user_id=$2")
	assert.Contains(t, sql, "LIMIT")
	// workspace_id, virtual_user_id, limit = 3 args
	assert.Len(t, qb.Args(), 3)
	assert.Equal(t, 100, qb.Args()[2])
}

func TestBatchDelete_MissingWorkspace(t *testing.T) {
	store := &PostgresMemoryStore{} // nil pool — validation fails before use
	_, err := store.BatchDelete(context.Background(), map[string]string{}, 500)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace_id")
}
