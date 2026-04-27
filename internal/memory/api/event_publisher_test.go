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

package api

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/memory"
)

// --- mockMemoryStore --------------------------------------------------------

// mockMemoryStore is a minimal in-memory implementation of memory.Store used
// to test event publishing without requiring a real database.
type mockMemoryStore struct {
	mu      sync.Mutex
	saved   []*memory.Memory
	saveErr error
}

func (m *mockMemoryStore) Save(_ context.Context, mem *memory.Memory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveErr != nil {
		return m.saveErr
	}
	if mem.ID == "" {
		mem.ID = "mock-id-001"
	}
	m.saved = append(m.saved, mem)
	return nil
}

func (m *mockMemoryStore) SaveWithResult(ctx context.Context, mem *memory.Memory) (*memory.SaveResult, error) {
	if err := m.Save(ctx, mem); err != nil {
		return nil, err
	}
	return &memory.SaveResult{ID: mem.ID, Action: memory.SaveActionAdded}, nil
}

func (m *mockMemoryStore) FindSimilarObservations(_ context.Context, _ map[string]string,
	_ []float32, _ int, _ float64,
) ([]memory.SimilarObservation, error) {
	return nil, nil
}

func (m *mockMemoryStore) AppendObservationToEntity(_ context.Context, entityID string, mem *memory.Memory) ([]string, error) {
	mem.ID = entityID
	return nil, nil
}

func (m *mockMemoryStore) GetMemory(_ context.Context, _ map[string]string, entityID string) (*memory.Memory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mem := range m.saved {
		if mem.ID == entityID {
			return mem, nil
		}
	}
	return nil, memory.ErrNotFound
}

func (m *mockMemoryStore) LinkEntities(_ context.Context, _ map[string]string,
	_, _, _ string, _ float64,
) (string, error) {
	return "rel-mock", nil
}

func (m *mockMemoryStore) Retrieve(_ context.Context, _ map[string]string, _ string, _ memory.RetrieveOptions) ([]*memory.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStore) List(_ context.Context, _ map[string]string, _ memory.ListOptions) ([]*memory.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStore) Delete(_ context.Context, _ map[string]string, _ string) error {
	return nil
}

func (m *mockMemoryStore) DeleteAll(_ context.Context, _ map[string]string) error {
	return nil
}

func (m *mockMemoryStore) ExportAll(_ context.Context, _ map[string]string) ([]*memory.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStore) RetrieveMultiTier(_ context.Context, _ memory.MultiTierRequest) (*memory.MultiTierResult, error) {
	return &memory.MultiTierResult{Memories: []*memory.MultiTierMemory{}, Total: 0}, nil
}

func (m *mockMemoryStore) BatchDelete(_ context.Context, _ map[string]string, _ int) (int, error) {
	return 0, nil
}

func (m *mockMemoryStore) SaveInstitutional(_ context.Context, _ *memory.Memory) error { return nil }

func (m *mockMemoryStore) ListInstitutional(_ context.Context, _ string, _ memory.ListOptions) ([]*memory.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStore) DeleteInstitutional(_ context.Context, _, _ string) error { return nil }

func (m *mockMemoryStore) SaveAgentScoped(_ context.Context, _ *memory.Memory) error { return nil }

func (m *mockMemoryStore) ListAgentScoped(_ context.Context, _, _ string, _ memory.ListOptions) ([]*memory.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStore) DeleteAgentScoped(_ context.Context, _, _, _ string) error { return nil }

func (m *mockMemoryStore) FindCompactionCandidates(_ context.Context, _ memory.FindCompactionCandidatesOptions) ([]memory.CompactionCandidate, error) {
	return nil, nil
}

func (m *mockMemoryStore) SaveCompactionSummary(_ context.Context, _ memory.CompactionSummary) (string, error) {
	return "", nil
}

// --- mockMemoryEventPublisher -----------------------------------------------

// mockMemoryEventPublisher records published events and can be configured to
// return an error.
type mockMemoryEventPublisher struct {
	mu     sync.Mutex
	events []MemoryEvent
	err    error
	ch     chan MemoryEvent // optional notification channel
}

func newMockMemoryEventPublisher(bufSize int) *mockMemoryEventPublisher {
	return &mockMemoryEventPublisher{ch: make(chan MemoryEvent, bufSize)}
}

func (m *mockMemoryEventPublisher) PublishMemoryEvent(_ context.Context, event MemoryEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, event)
	if m.ch != nil {
		m.ch <- event
	}
	return nil
}

func (m *mockMemoryEventPublisher) Close() error { return nil }

func (m *mockMemoryEventPublisher) waitForEvent(t *testing.T, timeout time.Duration) MemoryEvent {
	t.Helper()
	select {
	case ev := <-m.ch:
		return ev
	case <-time.After(timeout):
		t.Fatal("timed out waiting for memory event")
		return MemoryEvent{}
	}
}

// --- RedisMemoryEventPublisher tests ----------------------------------------

func TestRedisMemoryEventPublisher_Close(t *testing.T) {
	pub := &RedisMemoryEventPublisher{}
	assert.NoError(t, pub.Close())
}

func TestRedisMemoryEventPublisher_Publish(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	pub := NewRedisMemoryEventPublisher(client, logr.Discard())

	event := MemoryEvent{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    "mem-001",
		WorkspaceID: "ws-abc",
		UserID:      "user-1",
		Kind:        "preference",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	err := pub.PublishMemoryEvent(context.Background(), event)
	require.NoError(t, err)

	streamKey := MemoryStreamKey("ws-abc")
	msgs, err := client.XRange(context.Background(), streamKey, "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, msgs, 1)

	payload := msgs[0].Values["payload"].(string)
	var decoded MemoryEvent
	require.NoError(t, json.Unmarshal([]byte(payload), &decoded))
	assert.Equal(t, "mem-001", decoded.MemoryID)
	assert.Equal(t, eventTypeMemoryCreated, decoded.EventType)
	assert.Equal(t, "ws-abc", decoded.WorkspaceID)
	assert.Equal(t, "user-1", decoded.UserID)
	assert.Equal(t, "preference", decoded.Kind)
}

func TestRedisMemoryEventPublisher_PublishError(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})

	pub := NewRedisMemoryEventPublisher(client, logr.Discard())

	// Close miniredis to force a connection error.
	mr.Close()

	event := MemoryEvent{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    "mem-002",
		WorkspaceID: "ws-abc",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	err := pub.PublishMemoryEvent(context.Background(), event)
	assert.Error(t, err)
}

func TestMemoryStreamKey(t *testing.T) {
	assert.Equal(t, "omnia:memory-events:my-workspace", MemoryStreamKey("my-workspace"))
}

func TestMemoryEvent_OptionalFieldsOmitted(t *testing.T) {
	event := MemoryEvent{
		EventType:   eventTypeMemoryDeleted,
		MemoryID:    "mem-003",
		WorkspaceID: "ws-xyz",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(event)
	require.NoError(t, err)
	s := string(data)
	assert.NotContains(t, s, "userId")
	assert.NotContains(t, s, "agentId")
	assert.NotContains(t, s, "traceparent")
}

// --- MemoryService publish-on-save tests ------------------------------------

func TestMemoryService_PublishesOnSave(t *testing.T) {
	store := &mockMemoryStore{}
	pub := newMockMemoryEventPublisher(1)
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetEventPublisher(pub)

	mem := &memory.Memory{
		Type:       "preference",
		Content:    "dark mode",
		Confidence: 0.9,
		Scope: map[string]string{
			memory.ScopeWorkspaceID: "ws-test",
			memory.ScopeUserID:      "user-99",
		},
	}

	err := svc.SaveMemory(context.Background(), mem)
	require.NoError(t, err)

	ev := pub.waitForEvent(t, 2*time.Second)
	assert.Equal(t, eventTypeMemoryCreated, ev.EventType)
	assert.Equal(t, "ws-test", ev.WorkspaceID)
	assert.Equal(t, "user-99", ev.UserID)
	assert.Equal(t, "preference", ev.Kind)
	assert.NotEmpty(t, ev.MemoryID)
	assert.NotEmpty(t, ev.Timestamp)
}

func TestMemoryService_PublishesOnDelete(t *testing.T) {
	store := &mockMemoryStore{}
	pub := newMockMemoryEventPublisher(1)
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetEventPublisher(pub)

	scope := map[string]string{
		memory.ScopeWorkspaceID: "ws-delete",
		memory.ScopeUserID:      "user-42",
	}

	err := svc.DeleteMemory(context.Background(), scope, "mem-to-delete")
	require.NoError(t, err)

	ev := pub.waitForEvent(t, 2*time.Second)
	assert.Equal(t, eventTypeMemoryDeleted, ev.EventType)
	assert.Equal(t, "mem-to-delete", ev.MemoryID)
	assert.Equal(t, "ws-delete", ev.WorkspaceID)
	assert.Equal(t, "user-42", ev.UserID)
	assert.NotEmpty(t, ev.Timestamp)
}

func TestMemoryService_NilPublisherDoesNotPanic(t *testing.T) {
	store := &mockMemoryStore{}
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	// No publisher set — should not panic.

	mem := &memory.Memory{
		Type:       "fact",
		Content:    "test",
		Confidence: 0.8,
		Scope:      map[string]string{memory.ScopeWorkspaceID: "ws-nil"},
	}

	err := svc.SaveMemory(context.Background(), mem)
	require.NoError(t, err)

	err = svc.DeleteMemory(context.Background(), mem.Scope, mem.ID)
	require.NoError(t, err)
}

func TestMemoryService_PublishErrorDoesNotFailSave(t *testing.T) {
	store := &mockMemoryStore{}
	pub := newMockMemoryEventPublisher(0)
	pub.err = assert.AnError
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetEventPublisher(pub)

	mem := &memory.Memory{
		Type:       "fact",
		Content:    "test",
		Confidence: 0.8,
		Scope:      map[string]string{memory.ScopeWorkspaceID: "ws-err"},
	}

	// Save must succeed even when publish fails.
	err := svc.SaveMemory(context.Background(), mem)
	require.NoError(t, err)

	// Give goroutine time to run.
	time.Sleep(50 * time.Millisecond)
}
