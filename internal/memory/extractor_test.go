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

// --- mock implementations ----------------------------------------------------

// mockStore is a Store mock that captures Save calls.
type mockStore struct {
	saves   []*Memory
	saveErr error
	failAt  int // fail on the Nth Save call (1-indexed; 0 = never)
}

func (m *mockStore) Save(_ context.Context, mem *Memory) error {
	callNum := len(m.saves) + 1
	if m.failAt > 0 && callNum == m.failAt {
		return m.saveErr
	}
	// Simulate the real store mutating the Memory with an ID.
	mem.ID = "mock-id"
	m.saves = append(m.saves, mem)
	return nil
}

func (m *mockStore) Retrieve(_ context.Context, _ map[string]string, _ string, _ RetrieveOptions) ([]*Memory, error) {
	return nil, nil
}

func (m *mockStore) List(_ context.Context, _ map[string]string, _ ListOptions) ([]*Memory, error) {
	return nil, nil
}

func (m *mockStore) Delete(_ context.Context, _ map[string]string, _ string) error {
	return nil
}

func (m *mockStore) DeleteAll(_ context.Context, _ map[string]string) error {
	return nil
}

// mockPopulator is a MemoryPopulator mock.
type mockPopulator struct {
	result *PopulationResult
	err    error
}

func (m *mockPopulator) Populate(_ context.Context, _ PopulationSource) (*PopulationResult, error) {
	return m.result, m.err
}

func (m *mockPopulator) SourceType() string { return "mock" }
func (m *mockPopulator) TrustModel() string { return "deterministic" }

// --- helpers ------------------------------------------------------------------

func newTestExtractor(store Store, pop MemoryPopulator) *OmniaExtractor {
	log := zap.New(zap.UseDevMode(true))
	return NewOmniaExtractor(store, pop, log)
}

func basicScope() map[string]string {
	return testScope(testWorkspace1)
}

// --- tests -------------------------------------------------------------------

func TestOmniaExtractor_Extract(t *testing.T) {
	pop := &mockPopulator{
		result: &PopulationResult{
			Entities: []EntityRecord{
				{Name: "Alice", Kind: "person", Metadata: map[string]any{"role": "engineer"}},
			},
			Observations: []ObservationRecord{
				{EntityName: "Alice", Content: "Alice prefers Go", Confidence: 0.9, SessionID: "sess-1"},
			},
		},
	}
	store := &mockStore{}
	ext := newTestExtractor(store, pop)

	msgs := []SimpleMessage{
		{Role: "user", Content: "I am Alice"},
		{Role: "assistant", Content: "Hi Alice"},
	}

	memories, err := ext.Extract(context.Background(), basicScope(), msgs)
	require.NoError(t, err)
	require.Len(t, memories, 1)

	// Returned memory should have been mutated by the mock store (ID set).
	assert.Equal(t, "mock-id", memories[0].ID)
	assert.Equal(t, "person", memories[0].Type)
	assert.Equal(t, "Alice prefers Go", memories[0].Content)
	assert.InDelta(t, 0.9, memories[0].Confidence, 0.001)
	assert.Equal(t, "sess-1", memories[0].SessionID)
	assert.Equal(t, map[string]any{"role": "engineer"}, memories[0].Metadata)

	// Store should have received exactly one Save call.
	require.Len(t, store.saves, 1)
}

func TestOmniaExtractor_MultipleObservations(t *testing.T) {
	pop := &mockPopulator{
		result: &PopulationResult{
			Entities: []EntityRecord{
				{Name: "Bob", Kind: "person"},
				{Name: "Project X", Kind: "project"},
			},
			Observations: []ObservationRecord{
				{EntityName: "Bob", Content: "Bob is a developer", Confidence: 0.8},
				{EntityName: "Project X", Content: "Project X uses Kubernetes", Confidence: 0.7},
			},
		},
	}
	store := &mockStore{}
	ext := newTestExtractor(store, pop)

	memories, err := ext.Extract(context.Background(), basicScope(), nil)
	require.NoError(t, err)
	require.Len(t, memories, 2)
	assert.Len(t, store.saves, 2)
}

func TestOmniaExtractor_EmptyMessages(t *testing.T) {
	pop := &mockPopulator{
		result: &PopulationResult{
			Entities: []EntityRecord{
				{Name: "Anon", Kind: "person"},
			},
			Observations: []ObservationRecord{
				{EntityName: "Anon", Content: "no message context", Confidence: 0.5},
			},
		},
	}
	store := &mockStore{}
	ext := newTestExtractor(store, pop)

	// Empty messages — populator is still called.
	memories, err := ext.Extract(context.Background(), basicScope(), []SimpleMessage{})
	require.NoError(t, err)
	require.Len(t, memories, 1)
	assert.Len(t, store.saves, 1)
}

func TestOmniaExtractor_PopulatorError(t *testing.T) {
	pop := &mockPopulator{
		err: errors.New("LLM unavailable"),
	}
	store := &mockStore{}
	ext := newTestExtractor(store, pop)

	_, err := ext.Extract(context.Background(), basicScope(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LLM unavailable")
	assert.Empty(t, store.saves)
}

func TestOmniaExtractor_SaveError(t *testing.T) {
	pop := &mockPopulator{
		result: &PopulationResult{
			Entities: []EntityRecord{
				{Name: "Alice", Kind: "person"},
				{Name: "Bob", Kind: "person"},
			},
			Observations: []ObservationRecord{
				{EntityName: "Alice", Content: "Alice is helpful", Confidence: 0.9},
				{EntityName: "Bob", Content: "Bob is remote", Confidence: 0.7},
			},
		},
	}
	store := &mockStore{
		saveErr: errors.New("db connection lost"),
		failAt:  2, // fail on second Save
	}
	ext := newTestExtractor(store, pop)

	_, err := ext.Extract(context.Background(), basicScope(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db connection lost")
	// First save succeeded, second failed.
	assert.Len(t, store.saves, 1)
}

func TestOmniaExtractor_ObservationWithUnknownEntity(t *testing.T) {
	// Observation references an entity not in the Entities list.
	// Entity index will return zero-value EntityRecord (Kind=""), which is valid.
	pop := &mockPopulator{
		result: &PopulationResult{
			Entities: []EntityRecord{},
			Observations: []ObservationRecord{
				{EntityName: "Unknown", Content: "some content", Confidence: 0.6},
			},
		},
	}
	store := &mockStore{}
	ext := newTestExtractor(store, pop)

	memories, err := ext.Extract(context.Background(), basicScope(), nil)
	require.NoError(t, err)
	require.Len(t, memories, 1)
	assert.Equal(t, "", memories[0].Type) // zero-value kind
}

// --- helper unit tests -------------------------------------------------------

func TestBuildEntityIndex(t *testing.T) {
	entities := []EntityRecord{
		{Name: "Alice", Kind: "person"},
		{Name: "Project", Kind: "project"},
	}
	idx := buildEntityIndex(entities)
	assert.Len(t, idx, 2)
	assert.Equal(t, "person", idx["Alice"].Kind)
	assert.Equal(t, "project", idx["Project"].Kind)
}

func TestBuildEntityIndex_Empty(t *testing.T) {
	idx := buildEntityIndex(nil)
	assert.NotNil(t, idx)
	assert.Empty(t, idx)
}

func TestBuildMemory(t *testing.T) {
	scope := map[string]string{ScopeWorkspaceID: "ws-1"}
	obs := ObservationRecord{
		EntityName: "Alice",
		Content:    "Alice is a Go developer",
		Confidence: 0.85,
		SessionID:  "sess-42",
	}
	ent := EntityRecord{
		Name:     "Alice",
		Kind:     "person",
		Metadata: map[string]any{"team": "infra"},
	}

	mem := buildMemory(scope, obs, ent)
	assert.Equal(t, "person", mem.Type)
	assert.Equal(t, "Alice is a Go developer", mem.Content)
	assert.InDelta(t, 0.85, mem.Confidence, 0.001)
	assert.Equal(t, "sess-42", mem.SessionID)
	assert.Equal(t, scope, mem.Scope)
	assert.Equal(t, map[string]any{"team": "infra"}, mem.Metadata)
}
