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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// sampleMessages returns a small conversation for use in extraction tests.
func sampleMessages() []SimpleMessage {
	return []SimpleMessage{
		{Role: "user", Content: "I really enjoy working with Kubernetes"},
		{Role: "assistant", Content: "Kubernetes is a powerful orchestration platform"},
	}
}

// TestMemoryEndToEnd exercises the full stack:
// populator → extractor → store → retriever → delete → deleteAll.
func TestMemoryEndToEnd(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)
	log := zap.New(zap.UseDevMode(true))

	// --- extraction ---
	populator := NewConversationPopulator()
	extractor := NewOmniaExtractor(store, populator, log)

	memories, err := extractor.Extract(ctx, scope, sampleMessages())
	require.NoError(t, err)
	require.NotEmpty(t, memories, "extractor should produce at least one memory")

	for _, m := range memories {
		assert.NotEmpty(t, m.ID, "saved memory should have an ID")
	}

	// --- retrieve via store (tool-facing) ---
	retrieved, err := store.Retrieve(ctx, scope, "Kubernetes", RetrieveOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, retrieved, "store.Retrieve should find the extracted memory")

	// --- retrieve via OmniaRetriever (RAG-facing) ---
	strategy := &KeywordStrategy{}
	retriever := NewOmniaRetriever(store, strategy, 10, log)

	// The ConversationPopulator stores content like:
	// "User asked: I really enjoy working with Kubernetes | ..."
	// The retriever uses lastUserMessage as the ILIKE query, so we need a word
	// that appears in the stored content verbatim.
	ragMessages := []SimpleMessage{
		{Role: "user", Content: "Kubernetes"},
	}
	ragResults, err := retriever.RetrieveContext(ctx, scope, ragMessages)
	require.NoError(t, err)
	require.NotEmpty(t, ragResults, "OmniaRetriever should find stored memories")

	// --- list all ---
	listed, err := store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	assert.NotEmpty(t, listed, "List should return at least one memory")

	// --- delete one ---
	targetID := listed[0].ID
	err = store.Delete(ctx, scope, targetID)
	require.NoError(t, err)

	// Verify it's gone from retrieval.
	afterDelete, err := store.Retrieve(ctx, scope, "", RetrieveOptions{})
	require.NoError(t, err)
	for _, m := range afterDelete {
		assert.NotEqual(t, targetID, m.ID, "deleted memory should not appear")
	}

	// --- deleteAll (DSAR) ---
	err = store.DeleteAll(ctx, scope)
	require.NoError(t, err)

	empty, err := store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, empty, "List should be empty after DeleteAll")
}

// TestMemoryWorkspaceIsolation verifies that memories saved in one workspace
// are not visible from another workspace.
func TestMemoryWorkspaceIsolation(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	scope1 := testScope(testWorkspace1)
	scope2 := testScope(testWorkspace2)

	// Save a memory in workspace-1.
	mem := &Memory{
		Type:       "fact",
		Content:    "workspace isolation test fact",
		Confidence: 0.9,
		Scope:      scope1,
	}
	require.NoError(t, store.Save(ctx, mem))
	require.NotEmpty(t, mem.ID)

	// Retrieve from workspace-2 — should be empty.
	results, err := store.Retrieve(ctx, scope2, "isolation", RetrieveOptions{})
	require.NoError(t, err)
	assert.Empty(t, results, "workspace-2 should not see workspace-1 memories")

	// List from workspace-2 — should also be empty.
	listed, err := store.List(ctx, scope2, ListOptions{})
	require.NoError(t, err)
	assert.Empty(t, listed, "workspace-2 list should be empty")

	// Retrieve from workspace-1 — should find the memory.
	results, err = store.Retrieve(ctx, scope1, "isolation", RetrieveOptions{})
	require.NoError(t, err)
	assert.NotEmpty(t, results, "workspace-1 should find its own memory")
}

// TestPopulatorToRetriever exercises the full pipeline from ConversationPopulator
// through to OmniaRetriever via KeywordStrategy.
func TestPopulatorToRetriever(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)
	log := zap.New(zap.UseDevMode(true))

	// Populate via ConversationPopulator and save directly to store.
	populator := NewConversationPopulator()
	source := PopulationSource{
		Scope:    scope,
		Messages: sampleMessages(),
	}
	result, err := populator.Populate(ctx, source)
	require.NoError(t, err)

	// Save entities/observations to store.
	entityIdx := buildEntityIndex(result.Entities)
	for _, obs := range result.Observations {
		mem := buildMemory(scope, obs, entityIdx[obs.EntityName])
		require.NoError(t, store.Save(ctx, mem))
	}

	// Create retriever with KeywordStrategy.
	strategy := &KeywordStrategy{}
	retriever := NewOmniaRetriever(store, strategy, 10, log)

	// RetrieveContext with messages that match the stored observation content.
	// ConversationPopulator stores content like:
	//   "User asked: I really enjoy working with Kubernetes | ..."
	// The retriever extracts lastUserMessage as the ILIKE query, so "Kubernetes"
	// matches the stored content.
	queryMessages := []SimpleMessage{
		{Role: "user", Content: "Kubernetes"},
	}
	found, err := retriever.RetrieveContext(ctx, scope, queryMessages)
	require.NoError(t, err)
	assert.NotEmpty(t, found, "retriever should find memories matching 'Kubernetes'")
}

// TestToolProvider_EndToEnd exercises the Tools() integration: relations and timeline.
func TestToolProvider_EndToEnd(t *testing.T) {
	pool := freshDB(t)
	store := NewPostgresMemoryStore(pool)
	ctx := context.Background()

	// Save two entities via store to get their IDs.
	mem1 := &Memory{
		Type:       "person",
		Content:    "Alice",
		Confidence: 0.9,
		Scope:      testScope(testWorkspace1),
	}
	require.NoError(t, store.Save(ctx, mem1))
	require.NotEmpty(t, mem1.ID)

	mem2 := &Memory{
		Type:       "organization",
		Content:    "Acme Corp",
		Confidence: 0.9,
		Scope:      testScope(testWorkspace1),
	}
	require.NoError(t, store.Save(ctx, mem2))
	require.NotEmpty(t, mem2.ID)

	// Create a relation between them directly in DB.
	_, err := pool.Exec(ctx, `
		INSERT INTO memory_relations (workspace_id, source_entity_id, target_entity_id, relation_type, weight)
		VALUES ($1, $2, $3, 'works_at', 0.9)`,
		testWorkspace1, mem1.ID, mem2.ID,
	)
	require.NoError(t, err)

	// Verify Tools() returns the expected tool definitions.
	tools := store.Tools()
	require.Len(t, tools, 2)
	toolNames := make([]string, len(tools))
	for i, tool := range tools {
		toolNames[i] = tool.Name
	}
	assert.Contains(t, toolNames, toolNameRelated)
	assert.Contains(t, toolNames, toolNameTimeline)

	// Call handleRelated — verify the relation is found.
	relParams, err := json.Marshal(map[string]any{"entity_id": mem1.ID})
	require.NoError(t, err)

	relResult, err := store.handleRelated(ctx, relParams)
	require.NoError(t, err)

	var relRows []relatedResult
	require.NoError(t, json.Unmarshal([]byte(relResult), &relRows))
	require.Len(t, relRows, 1)
	assert.Equal(t, mem2.ID, relRows[0].ID)
	assert.Equal(t, "works_at", relRows[0].RelationType)
	assert.InDelta(t, 0.9, relRows[0].Weight, 0.001)

	// Add an observation directly to mem1 with a source_type for timeline.
	_, err = pool.Exec(ctx, `
		INSERT INTO memory_observations (entity_id, content, confidence, source_type)
		VALUES ($1, 'Alice joined Acme Corp in 2025', 0.85, 'conversation_extraction')`,
		mem1.ID,
	)
	require.NoError(t, err)

	// Call handleTimeline — verify the observation is returned.
	tlParams, err := json.Marshal(map[string]any{"entity_id": mem1.ID, "limit": 10})
	require.NoError(t, err)

	tlResult, err := store.handleTimeline(ctx, tlParams)
	require.NoError(t, err)

	var tlRows []timelineResult
	require.NoError(t, json.Unmarshal([]byte(tlResult), &tlRows))

	// At least the explicitly inserted observation should appear.
	found := false
	for _, row := range tlRows {
		if row.Content == "Alice joined Acme Corp in 2025" {
			found = true
			assert.InDelta(t, 0.85, row.Confidence, 0.001)
			assert.Equal(t, "conversation_extraction", row.SourceType)
		}
	}
	assert.True(t, found, "timeline should contain the directly-inserted observation")
}
