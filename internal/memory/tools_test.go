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
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time check that PostgresMemoryStore implements ToolProvider.
var _ ToolProvider = (*PostgresMemoryStore)(nil)

func TestPostgresMemoryStore_Tools(t *testing.T) {
	store := newStore(t)
	tools := store.Tools()
	require.Len(t, tools, 2)

	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	assert.Contains(t, names, toolNameRelated)
	assert.Contains(t, names, toolNameTimeline)

	// Each tool should have a non-nil handler.
	for _, tool := range tools {
		assert.NotNil(t, tool.Handler, "handler for %s should not be nil", tool.Name)
		assert.NotEmpty(t, tool.Description)
	}
}

func TestPostgresMemoryStore_Related(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	ctx := context.Background()

	// Insert two entities directly.
	var srcID, tgtID string
	err := pool.QueryRow(ctx, `
		INSERT INTO memory_entities (workspace_id, name, kind)
		VALUES ($1, 'Alice', 'person')
		RETURNING id`,
		testWorkspace1,
	).Scan(&srcID)
	require.NoError(t, err)

	err = pool.QueryRow(ctx, `
		INSERT INTO memory_entities (workspace_id, name, kind)
		VALUES ($1, 'Acme Corp', 'organization')
		RETURNING id`,
		testWorkspace1,
	).Scan(&tgtID)
	require.NoError(t, err)

	// Insert a relation between them.
	_, err = pool.Exec(ctx, `
		INSERT INTO memory_relations (workspace_id, source_entity_id, target_entity_id, relation_type, weight)
		VALUES ($1, $2, $3, 'works_at', 0.9)`,
		testWorkspace1, srcID, tgtID,
	)
	require.NoError(t, err)

	// Call the handler.
	params, err := json.Marshal(map[string]any{"entity_id": srcID})
	require.NoError(t, err)

	result, err := store.handleRelated(ctx, params)
	require.NoError(t, err)

	var rows []relatedResult
	require.NoError(t, json.Unmarshal([]byte(result), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, tgtID, rows[0].ID)
	assert.Equal(t, "Acme Corp", rows[0].Name)
	assert.Equal(t, "organization", rows[0].Kind)
	assert.Equal(t, "works_at", rows[0].RelationType)
	assert.InDelta(t, 0.9, rows[0].Weight, 0.001)
}

func TestPostgresMemoryStore_Related_WithTypeFilter(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	ctx := context.Background()

	var srcID, tgt1ID, tgt2ID string
	err := pool.QueryRow(ctx, `
		INSERT INTO memory_entities (workspace_id, name, kind)
		VALUES ($1, 'Bob', 'person') RETURNING id`,
		testWorkspace1,
	).Scan(&srcID)
	require.NoError(t, err)

	err = pool.QueryRow(ctx, `
		INSERT INTO memory_entities (workspace_id, name, kind)
		VALUES ($1, 'TechCorp', 'organization') RETURNING id`,
		testWorkspace1,
	).Scan(&tgt1ID)
	require.NoError(t, err)

	err = pool.QueryRow(ctx, `
		INSERT INTO memory_entities (workspace_id, name, kind)
		VALUES ($1, 'Jane', 'person') RETURNING id`,
		testWorkspace1,
	).Scan(&tgt2ID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO memory_relations (workspace_id, source_entity_id, target_entity_id, relation_type, weight)
		VALUES ($1, $2, $3, 'works_at', 0.8),
		       ($1, $2, $4, 'knows', 0.6)`,
		testWorkspace1, srcID, tgt1ID, tgt2ID,
	)
	require.NoError(t, err)

	// Filter to only 'knows' relations.
	params, err := json.Marshal(map[string]any{
		"entity_id":      srcID,
		"relation_types": []string{"knows"},
	})
	require.NoError(t, err)

	result, err := store.handleRelated(ctx, params)
	require.NoError(t, err)

	var rows []relatedResult
	require.NoError(t, json.Unmarshal([]byte(result), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "knows", rows[0].RelationType)
}

func TestPostgresMemoryStore_Related_NoResults(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	ctx := context.Background()

	// Insert an entity with no relations.
	var entityID string
	err := pool.QueryRow(ctx, `
		INSERT INTO memory_entities (workspace_id, name, kind)
		VALUES ($1, 'Loner', 'person') RETURNING id`,
		testWorkspace1,
	).Scan(&entityID)
	require.NoError(t, err)

	params, err := json.Marshal(map[string]any{"entity_id": entityID})
	require.NoError(t, err)

	result, err := store.handleRelated(ctx, params)
	require.NoError(t, err)

	var rows []relatedResult
	require.NoError(t, json.Unmarshal([]byte(result), &rows))
	assert.Empty(t, rows)
	assert.Equal(t, "[]", result)
}

func TestPostgresMemoryStore_Timeline(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	ctx := context.Background()

	// Insert an entity.
	var entityID string
	err := pool.QueryRow(ctx, `
		INSERT INTO memory_entities (workspace_id, name, kind)
		VALUES ($1, 'Timeline Test Entity', 'fact') RETURNING id`,
		testWorkspace1,
	).Scan(&entityID)
	require.NoError(t, err)

	// Insert two observations with explicit timestamps so order is deterministic.
	t1 := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Millisecond)
	t2 := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Millisecond)

	_, err = pool.Exec(ctx, `
		INSERT INTO memory_observations (entity_id, content, confidence, source_type, observed_at)
		VALUES ($1, 'older observation', 0.7, 'manual', $2),
		       ($1, 'newer observation', 0.9, 'conversation_extraction', $3)`,
		entityID, t1, t2,
	)
	require.NoError(t, err)

	params, err := json.Marshal(map[string]any{"entity_id": entityID, "limit": 10})
	require.NoError(t, err)

	result, err := store.handleTimeline(ctx, params)
	require.NoError(t, err)

	var rows []timelineResult
	require.NoError(t, json.Unmarshal([]byte(result), &rows))
	require.Len(t, rows, 2)

	// Should be ordered newest first.
	assert.Equal(t, "newer observation", rows[0].Content)
	assert.Equal(t, "older observation", rows[1].Content)
	assert.InDelta(t, 0.9, rows[0].Confidence, 0.001)
	assert.Equal(t, "conversation_extraction", rows[0].SourceType)
	assert.Nil(t, rows[0].ValidUntil)
}

func TestPostgresMemoryStore_Timeline_NoResults(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	ctx := context.Background()

	// Insert an entity but no observations.
	var entityID string
	err := pool.QueryRow(ctx, `
		INSERT INTO memory_entities (workspace_id, name, kind)
		VALUES ($1, 'Empty Entity', 'fact') RETURNING id`,
		testWorkspace1,
	).Scan(&entityID)
	require.NoError(t, err)

	params, err := json.Marshal(map[string]any{"entity_id": entityID})
	require.NoError(t, err)

	result, err := store.handleTimeline(ctx, params)
	require.NoError(t, err)

	var rows []timelineResult
	require.NoError(t, json.Unmarshal([]byte(result), &rows))
	assert.Empty(t, rows)
	assert.Equal(t, "[]", result)
}

func TestPostgresMemoryStore_Related_InvalidParams(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	_, err := store.handleRelated(ctx, json.RawMessage(`{not valid json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid params")
}

func TestPostgresMemoryStore_Related_MissingEntityID(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	_, err := store.handleRelated(ctx, json.RawMessage(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entity_id is required")
}

func TestPostgresMemoryStore_Timeline_InvalidParams(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	_, err := store.handleTimeline(ctx, json.RawMessage(`{not valid json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid params")
}

func TestPostgresMemoryStore_Timeline_MissingEntityID(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	_, err := store.handleTimeline(ctx, json.RawMessage(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entity_id is required")
}

func TestPostgresMemoryStore_Timeline_DefaultLimit(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	ctx := context.Background()

	var entityID string
	err := pool.QueryRow(ctx, `
		INSERT INTO memory_entities (workspace_id, name, kind)
		VALUES ($1, 'Limit Test', 'fact') RETURNING id`,
		testWorkspace1,
	).Scan(&entityID)
	require.NoError(t, err)

	// Insert 15 observations.
	for i := range 15 {
		obs_time := time.Now().Add(time.Duration(-i) * time.Hour)
		_, err = pool.Exec(ctx, `
			INSERT INTO memory_observations (entity_id, content, confidence, observed_at)
			VALUES ($1, $2, 0.8, $3)`,
			entityID, fmt.Sprintf("observation %d", i), obs_time,
		)
		require.NoError(t, err)
	}

	// No explicit limit — should default to 10.
	params, err := json.Marshal(map[string]any{"entity_id": entityID})
	require.NoError(t, err)

	result, err := store.handleTimeline(ctx, params)
	require.NoError(t, err)

	var rows []timelineResult
	require.NoError(t, json.Unmarshal([]byte(result), &rows))
	assert.Len(t, rows, 10)
}
