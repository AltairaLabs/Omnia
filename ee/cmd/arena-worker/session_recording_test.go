/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
)

// mockSessionStore implements session.Store for testing session creation and status updates.
type mockSessionStore struct {
	mu              sync.Mutex
	createdSessions []session.CreateSessionOptions
	statusUpdates   map[string]session.SessionStatusUpdate
	messages        map[string][]session.Message
	providerCalls   map[string][]session.ProviderCall
}

func newMockStore() *mockSessionStore {
	return &mockSessionStore{
		statusUpdates: make(map[string]session.SessionStatusUpdate),
		messages:      make(map[string][]session.Message),
		providerCalls: make(map[string][]session.ProviderCall),
	}
}

func (m *mockSessionStore) CreateSession(
	_ context.Context, opts session.CreateSessionOptions,
) (*session.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createdSessions = append(m.createdSessions, opts)
	return &session.Session{ID: opts.ID, Status: session.SessionStatusActive}, nil
}

func (m *mockSessionStore) UpdateSessionStatus(_ context.Context, id string, update session.SessionStatusUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusUpdates[id] = update
	return nil
}

func (m *mockSessionStore) AppendMessage(_ context.Context, sessionID string, msg session.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[sessionID] = append(m.messages[sessionID], msg)
	return nil
}

func (m *mockSessionStore) RecordProviderCall(_ context.Context, sessionID string, pc session.ProviderCall) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providerCalls[sessionID] = append(m.providerCalls[sessionID], pc)
	return nil
}

func (m *mockSessionStore) RecordToolCall(_ context.Context, _ string, _ session.ToolCall) error {
	return nil
}
func (m *mockSessionStore) GetSession(_ context.Context, _ string) (*session.Session, error) {
	return nil, session.ErrSessionNotFound
}
func (m *mockSessionStore) GetMessages(_ context.Context, _ string) ([]session.Message, error) {
	return nil, nil
}
func (m *mockSessionStore) SetState(_ context.Context, _, _, _ string) error { return nil }
func (m *mockSessionStore) GetState(_ context.Context, _, _ string) (string, error) {
	return "", nil
}
func (m *mockSessionStore) RefreshTTL(_ context.Context, _ string, _ time.Duration) error {
	return nil
}
func (m *mockSessionStore) DeleteSession(_ context.Context, _ string) error { return nil }
func (m *mockSessionStore) GetToolCalls(_ context.Context, _ string, _, _ int) ([]session.ToolCall, error) {
	return nil, nil
}
func (m *mockSessionStore) GetProviderCalls(_ context.Context, _ string, _, _ int) ([]session.ProviderCall, error) {
	return nil, nil
}
func (m *mockSessionStore) RecordEvalResult(_ context.Context, _ string, _ session.EvalResult) error {
	return nil
}
func (m *mockSessionStore) RecordRuntimeEvent(_ context.Context, _ string, _ session.RuntimeEvent) error {
	return nil
}
func (m *mockSessionStore) GetRuntimeEvents(_ context.Context, _ string, _, _ int) ([]session.RuntimeEvent, error) {
	return nil, nil
}
func (m *mockSessionStore) Close() error { return nil }

func (m *mockSessionStore) getCreatedSessions() []session.CreateSessionOptions {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]session.CreateSessionOptions{}, m.createdSessions...)
}

func (m *mockSessionStore) getStatusUpdates() map[string]session.SessionStatusUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]session.SessionStatusUpdate)
	for k, v := range m.statusUpdates {
		out[k] = v
	}
	return out
}

// failingSessionStore returns an error on CreateSession.
type failingSessionStore struct{ mockSessionStore }

func (f *failingSessionStore) CreateSession(
	_ context.Context, _ session.CreateSessionOptions,
) (*session.Session, error) {
	return nil, errors.New("connection refused")
}

func TestRunIDToUUID(t *testing.T) {
	// Deterministic: same input → same output
	id1 := runIDToUUID("2026-03-21T15-04-05Z_openai_us-east_support_a1b2c3d4_0001")
	id2 := runIDToUUID("2026-03-21T15-04-05Z_openai_us-east_support_a1b2c3d4_0001")
	assert.Equal(t, id1, id2, "same runID should produce same UUID")

	// Different inputs → different outputs
	id3 := runIDToUUID("2026-03-21T15-04-05Z_openai_us-east_support_a1b2c3d4_0002")
	assert.NotEqual(t, id1, id3, "different runIDs should produce different UUIDs")

	// Valid UUID format (36 chars with hyphens)
	assert.Len(t, id1, 36, "should be valid UUID length")
}

func TestArenaSessionManager_OnEvent_CreatesSession(t *testing.T) {
	store := newMockStore()
	mgr := newArenaSessionManager(store, logr.Discard(), arenaSessionMetadata{
		JobName:    "test-job",
		Namespace:  "default",
		Scenario:   "customer-support",
		ProviderID: "openai-gpt4",
		JobType:    "arena",
	}, "test-item")

	event := &events.Event{
		Type:      events.EventProviderCallCompleted,
		SessionID: "run-001",
		Timestamp: time.Now(),
		Data: &events.ProviderCallCompletedData{
			Provider:     "openai",
			Model:        "gpt-4o",
			InputTokens:  100,
			OutputTokens: 50,
			Cost:         0.005,
			Duration:     time.Second,
		},
	}

	mgr.OnEvent(event)

	// Wait briefly for async OmniaEventStore write
	time.Sleep(100 * time.Millisecond)

	sessions := store.getCreatedSessions()
	require.Len(t, sessions, 1, "should create exactly one session")

	expectedUUID := runIDToUUID("test-item:run-001")
	assert.Equal(t, expectedUUID, sessions[0].ID, "session ID should be UUID5 of runID")
	assert.Equal(t, "test-job", sessions[0].AgentName)
	assert.Equal(t, "default", sessions[0].Namespace)
	assert.Contains(t, sessions[0].Tags, "source:arena")
	assert.Contains(t, sessions[0].Tags, "arena-job:test-job")
	assert.Contains(t, sessions[0].Tags, "scenario:customer-support")
	assert.Contains(t, sessions[0].Tags, "provider:openai-gpt4")
	assert.Equal(t, "test-job", sessions[0].InitialState["arena.job"])
	assert.Equal(t, "customer-support", sessions[0].InitialState["arena.scenario"])
	assert.Equal(t, "run-001", sessions[0].InitialState["arena.run_id"])
}

func TestArenaSessionManager_OnEvent_DeduplicatesSessions(t *testing.T) {
	store := newMockStore()
	mgr := newArenaSessionManager(store, logr.Discard(), arenaSessionMetadata{
		JobName:    "test-job",
		Namespace:  "default",
		ProviderID: "openai",
	}, "test-item")

	for i := 0; i < 5; i++ {
		mgr.OnEvent(&events.Event{
			Type:      events.EventProviderCallCompleted,
			SessionID: "run-001",
			Timestamp: time.Now(),
			Data:      &events.ProviderCallCompletedData{Provider: "openai"},
		})
	}

	time.Sleep(100 * time.Millisecond)

	sessions := store.getCreatedSessions()
	assert.Len(t, sessions, 1, "should create session only once despite 5 events")
}

func TestArenaSessionManager_OnEvent_MultipleRuns(t *testing.T) {
	store := newMockStore()
	mgr := newArenaSessionManager(store, logr.Discard(), arenaSessionMetadata{
		JobName:    "test-job",
		Namespace:  "default",
		ProviderID: "openai",
	}, "test-item")

	mgr.OnEvent(&events.Event{
		Type:      events.EventProviderCallCompleted,
		SessionID: "run-001",
		Timestamp: time.Now(),
		Data:      &events.ProviderCallCompletedData{Provider: "openai"},
	})
	mgr.OnEvent(&events.Event{
		Type:      events.EventProviderCallCompleted,
		SessionID: "run-002",
		Timestamp: time.Now(),
		Data:      &events.ProviderCallCompletedData{Provider: "anthropic"},
	})

	time.Sleep(100 * time.Millisecond)

	sessions := store.getCreatedSessions()
	assert.Len(t, sessions, 2, "should create one session per unique runID")
}

func TestArenaSessionManager_OnEvent_SkipsEmptySessionID(t *testing.T) {
	store := newMockStore()
	mgr := newArenaSessionManager(store, logr.Discard(), arenaSessionMetadata{}, "test-item")

	mgr.OnEvent(&events.Event{
		Type:      events.EventProviderCallCompleted,
		SessionID: "",
		Timestamp: time.Now(),
		Data:      &events.ProviderCallCompletedData{},
	})

	sessions := store.getCreatedSessions()
	assert.Empty(t, sessions, "should not create session for empty SessionID")
}

func TestArenaSessionManager_CompleteAll(t *testing.T) {
	store := newMockStore()
	mgr := newArenaSessionManager(store, logr.Discard(), arenaSessionMetadata{
		JobName:    "test-job",
		Namespace:  "default",
		ProviderID: "openai",
	}, "test-item")

	// Create two sessions
	mgr.OnEvent(&events.Event{
		Type:      events.EventProviderCallCompleted,
		SessionID: "run-001",
		Timestamp: time.Now(),
		Data:      &events.ProviderCallCompletedData{Provider: "openai"},
	})
	mgr.OnEvent(&events.Event{
		Type:      events.EventProviderCallCompleted,
		SessionID: "run-002",
		Timestamp: time.Now(),
		Data:      &events.ProviderCallCompletedData{Provider: "openai"},
	})

	time.Sleep(100 * time.Millisecond)

	mgr.CompleteAll(context.Background())

	updates := store.getStatusUpdates()
	uuid1 := runIDToUUID("test-item:run-001")
	uuid2 := runIDToUUID("test-item:run-002")

	require.Contains(t, updates, uuid1, "should complete run-001 session")
	require.Contains(t, updates, uuid2, "should complete run-002 session")
	assert.Equal(t, session.SessionStatusCompleted, updates[uuid1].SetStatus)
	assert.Equal(t, session.SessionStatusCompleted, updates[uuid2].SetStatus)
	assert.False(t, updates[uuid1].SetEndedAt.IsZero(), "should set ended_at")
}

func TestArenaSessionManager_CompleteAll_FailedRun(t *testing.T) {
	store := newMockStore()
	mgr := newArenaSessionManager(store, logr.Discard(), arenaSessionMetadata{
		JobName:    "test-job",
		Namespace:  "default",
		ProviderID: "openai",
	}, "test-item")

	// Create a session that succeeds
	mgr.OnEvent(&events.Event{
		Type:      events.EventProviderCallCompleted,
		SessionID: "run-pass",
		Timestamp: time.Now(),
		Data:      &events.ProviderCallCompletedData{Provider: "openai"},
	})

	// Create a session that fails
	mgr.OnEvent(&events.Event{
		Type:      events.EventProviderCallCompleted,
		SessionID: "run-fail",
		Timestamp: time.Now(),
		Data:      &events.ProviderCallCompletedData{Provider: "openai"},
	})
	mgr.OnEvent(&events.Event{
		Type:      events.EventType("arena.run.failed"),
		SessionID: "run-fail",
		Timestamp: time.Now(),
	})

	time.Sleep(100 * time.Millisecond)

	mgr.CompleteAll(context.Background())

	updates := store.getStatusUpdates()
	uuidPass := runIDToUUID("test-item:run-pass")
	uuidFail := runIDToUUID("test-item:run-fail")

	require.Contains(t, updates, uuidPass)
	require.Contains(t, updates, uuidFail)
	assert.Equal(t, session.SessionStatusCompleted, updates[uuidPass].SetStatus, "passed run should be completed")
	assert.Equal(t, session.SessionStatusError, updates[uuidFail].SetStatus, "failed run should be error")
}

func TestArenaSessionManager_OnEvent_CreateSessionError(t *testing.T) {
	store := &failingSessionStore{}
	mgr := newArenaSessionManager(store, logr.Discard(), arenaSessionMetadata{
		JobName:    "test-job",
		Namespace:  "default",
		ProviderID: "openai",
	}, "test-item")

	// Should not panic on CreateSession error
	mgr.OnEvent(&events.Event{
		Type:      events.EventProviderCallCompleted,
		SessionID: "run-fail",
		Timestamp: time.Now(),
		Data:      &events.ProviderCallCompletedData{Provider: "openai"},
	})

	// Second event for same runID should retry (entry was deleted on error)
	mgr.OnEvent(&events.Event{
		Type:      events.EventProviderCallCompleted,
		SessionID: "run-fail",
		Timestamp: time.Now(),
		Data:      &events.ProviderCallCompletedData{Provider: "openai"},
	})

	// CompleteAll should handle empty sessions gracefully (no sessions created due to errors)
	mgr.CompleteAll(context.Background())

	// No sessions should have been created since CreateSession always fails
	assert.Empty(t, store.getCreatedSessions(), "no sessions created when store fails")
}

func TestArenaSessionManager_SessionIDs(t *testing.T) {
	t.Run("returns empty for no sessions", func(t *testing.T) {
		store := newMockStore()
		mgr := newArenaSessionManager(store, logr.Discard(), arenaSessionMetadata{}, "test-item")
		assert.Empty(t, mgr.SessionIDs())
	})

	t.Run("returns all created session IDs", func(t *testing.T) {
		store := newMockStore()
		mgr := newArenaSessionManager(store, logr.Discard(), arenaSessionMetadata{
			JobName:   "test-job",
			Namespace: "default",
		}, "test-item")

		mgr.OnEvent(&events.Event{
			Type:      events.EventProviderCallCompleted,
			SessionID: "run-a",
			Timestamp: time.Now(),
			Data:      &events.ProviderCallCompletedData{Provider: "openai"},
		})
		mgr.OnEvent(&events.Event{
			Type:      events.EventProviderCallCompleted,
			SessionID: "run-b",
			Timestamp: time.Now(),
			Data:      &events.ProviderCallCompletedData{Provider: "openai"},
		})

		time.Sleep(100 * time.Millisecond)

		ids := mgr.SessionIDs()
		assert.Len(t, ids, 2)
		assert.Contains(t, ids, runIDToUUID("test-item:run-a"))
		assert.Contains(t, ids, runIDToUUID("test-item:run-b"))
	})
}

func TestArenaSessionManager_MetadataTags(t *testing.T) {
	t.Run("includes arena job metadata in tags and initial state", func(t *testing.T) {
		store := newMockStore()
		mgr := newArenaSessionManager(store, logr.Discard(), arenaSessionMetadata{
			JobName:       "my-arena-job",
			Namespace:     "prod-ns",
			WorkspaceName: "my-workspace",
			Scenario:      "support-scenario",
			ProviderID:    "gpt-4o",
			JobType:       "arena",
			TrialIndex:    "3",
		}, "test-item")

		mgr.OnEvent(&events.Event{
			Type:      events.EventProviderCallCompleted,
			SessionID: "run-meta",
			Timestamp: time.Now(),
			Data:      &events.ProviderCallCompletedData{Provider: "openai"},
		})

		time.Sleep(100 * time.Millisecond)

		sessions := store.getCreatedSessions()
		require.Len(t, sessions, 1)

		s := sessions[0]

		// Verify tags
		assert.Contains(t, s.Tags, "source:arena")
		assert.Contains(t, s.Tags, "arena-job:my-arena-job")
		assert.Contains(t, s.Tags, "scenario:support-scenario")
		assert.Contains(t, s.Tags, "provider:gpt-4o")
		assert.Contains(t, s.Tags, "trial:3")

		// Verify initial state includes new metadata keys
		assert.Equal(t, "my-arena-job", s.InitialState["arena.job.name"])
		assert.Equal(t, "prod-ns", s.InitialState["arena.job.namespace"])
		assert.Equal(t, "support-scenario", s.InitialState["arena.scenario.id"])
		assert.Equal(t, "gpt-4o", s.InitialState["arena.provider.id"])
		assert.Equal(t, "3", s.InitialState["arena.trial.index"])

		// Verify backward-compatible keys still present
		assert.Equal(t, "my-arena-job", s.InitialState["arena.job"])
		assert.Equal(t, "support-scenario", s.InitialState["arena.scenario"])
		assert.Equal(t, "gpt-4o", s.InitialState["arena.provider"])
		assert.Equal(t, "arena", s.InitialState["arena.type"])
		assert.Equal(t, "run-meta", s.InitialState["arena.run_id"])
	})

	t.Run("omits trial tag and state when trial index is empty", func(t *testing.T) {
		store := newMockStore()
		mgr := newArenaSessionManager(store, logr.Discard(), arenaSessionMetadata{
			JobName:    "job-no-trial",
			Namespace:  "default",
			Scenario:   "s1",
			ProviderID: "p1",
			JobType:    "arena",
		}, "test-item")

		mgr.OnEvent(&events.Event{
			Type:      events.EventProviderCallCompleted,
			SessionID: "run-no-trial",
			Timestamp: time.Now(),
			Data:      &events.ProviderCallCompletedData{Provider: "openai"},
		})

		time.Sleep(100 * time.Millisecond)

		sessions := store.getCreatedSessions()
		require.Len(t, sessions, 1)

		s := sessions[0]
		for _, tag := range s.Tags {
			assert.NotContains(t, tag, "trial:")
		}
		_, hasTrial := s.InitialState["arena.trial.index"]
		assert.False(t, hasTrial)
	})
}

func TestArenaSessionManager_CompleteAll_UpdateError(t *testing.T) {
	store := &mockSessionStore{
		statusUpdates: make(map[string]session.SessionStatusUpdate),
		messages:      make(map[string][]session.Message),
		providerCalls: make(map[string][]session.ProviderCall),
	}
	mgr := newArenaSessionManager(store, logr.Discard(), arenaSessionMetadata{
		JobName:    "test-job",
		Namespace:  "default",
		ProviderID: "openai",
	}, "test-item")

	// Create a session
	mgr.OnEvent(&events.Event{
		Type:      events.EventProviderCallCompleted,
		SessionID: "run-err",
		Timestamp: time.Now(),
		Data:      &events.ProviderCallCompletedData{Provider: "openai"},
	})

	time.Sleep(100 * time.Millisecond)

	// Verify CompleteAll doesn't panic and marks sessions completed
	mgr.CompleteAll(context.Background())

	updates := store.getStatusUpdates()
	uuid := runIDToUUID("test-item:run-err")
	assert.Equal(t, session.SessionStatusCompleted, updates[uuid].SetStatus)
}
