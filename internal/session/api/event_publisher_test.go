/*
Copyright 2026 Altaira Labs.

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

// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// --- mock event publisher ---------------------------------------------------

type mockEventPublisher struct {
	mu     sync.Mutex
	events []SessionEvent
	err    error
}

func (m *mockEventPublisher) PublishMessageEvent(_ context.Context, event SessionEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventPublisher) Close() error { return nil }

func (m *mockEventPublisher) getEvents() []SessionEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]SessionEvent, len(m.events))
	copy(cp, m.events)
	return cp
}

// waitForEvents polls until the publisher has at least n events or the timeout elapses.
func (m *mockEventPublisher) waitForEvents(t *testing.T, n int, timeout time.Duration) []SessionEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		events := m.getEvents()
		if len(events) >= n {
			return events
		}
		time.Sleep(5 * time.Millisecond)
	}
	events := m.getEvents()
	require.GreaterOrEqual(t, len(events), n, "timed out waiting for %d events, got %d", n, len(events))
	return events
}

// --- helper -----------------------------------------------------------------

func newServiceWithPublisher(registry *providers.Registry, pub EventPublisher) *SessionService {
	cfg := ServiceConfig{EventPublisher: pub}
	return NewSessionService(registry, cfg, logr.Discard())
}

// --- AppendMessage event tests ----------------------------------------------

func TestAppendMessage_AssistantPublishesEvent(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{
		ID:        "s1",
		AgentName: "test-agent",
		Namespace: "test-ns",
	}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	pub := &mockEventPublisher{}
	svc := newServiceWithPublisher(registry, pub)

	msg := &session.Message{ID: "m1", Role: session.RoleAssistant, Content: "hello"}
	err := svc.AppendMessage(context.Background(), "s1", msg)
	require.NoError(t, err)

	events := pub.waitForEvents(t, 1, 2*time.Second)
	assert.Equal(t, "message.assistant", events[0].EventType)
	assert.Equal(t, "s1", events[0].SessionID)
	assert.Equal(t, "test-agent", events[0].AgentName)
	assert.Equal(t, "test-ns", events[0].Namespace)
	assert.Equal(t, "m1", events[0].MessageID)
	assert.Equal(t, "assistant", events[0].MessageRole)
	assert.NotEmpty(t, events[0].Timestamp)
}

func TestAppendMessage_UserDoesNotPublish(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{ID: "s1"}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	pub := &mockEventPublisher{}
	svc := newServiceWithPublisher(registry, pub)

	msg := &session.Message{ID: "m1", Role: session.RoleUser, Content: "hello"}
	err := svc.AppendMessage(context.Background(), "s1", msg)
	require.NoError(t, err)

	// Give async goroutine time to run (should not publish).
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, pub.getEvents())
}

func TestAppendMessage_SystemDoesNotPublish(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{ID: "s1"}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	pub := &mockEventPublisher{}
	svc := newServiceWithPublisher(registry, pub)

	msg := &session.Message{ID: "m1", Role: session.RoleSystem, Content: "system"}
	err := svc.AppendMessage(context.Background(), "s1", msg)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, pub.getEvents())
}

func TestAppendMessage_NilPublisherDoesNotPanic(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{ID: "s1"}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithPublisher(registry, nil)

	msg := &session.Message{ID: "m1", Role: session.RoleAssistant, Content: "hello"}
	err := svc.AppendMessage(context.Background(), "s1", msg)
	require.NoError(t, err)
}

func TestAppendMessage_PublishErrorDoesNotFailRequest(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{
		ID:        "s1",
		AgentName: "a",
		Namespace: "n",
	}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	pub := &mockEventPublisher{err: errors.New("redis down")}
	svc := newServiceWithPublisher(registry, pub)

	msg := &session.Message{ID: "m1", Role: session.RoleAssistant, Content: "hello"}
	err := svc.AppendMessage(context.Background(), "s1", msg)
	require.NoError(t, err) // Must succeed even though publish fails.
}

// --- UpdateSessionStats event tests -----------------------------------------

func TestUpdateSessionStats_CompletedPublishesEvent(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{
		ID:        "s1",
		AgentName: "test-agent",
		Namespace: "test-ns",
		Status:    session.SessionStatusActive,
	}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	pub := &mockEventPublisher{}
	svc := newServiceWithPublisher(registry, pub)

	err := svc.UpdateSessionStats(context.Background(), "s1", session.SessionStatsUpdate{
		SetStatus: session.SessionStatusCompleted,
	})
	require.NoError(t, err)

	events := pub.waitForEvents(t, 1, 2*time.Second)
	assert.Equal(t, "session.completed", events[0].EventType)
	assert.Equal(t, "s1", events[0].SessionID)
	assert.Equal(t, "test-agent", events[0].AgentName)
	assert.Equal(t, "test-ns", events[0].Namespace)
	assert.NotEmpty(t, events[0].Timestamp)
}

func TestUpdateSessionStats_NoEventOnNonCompletedStatus(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{
		ID:     "s1",
		Status: session.SessionStatusActive,
	}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	pub := &mockEventPublisher{}
	svc := newServiceWithPublisher(registry, pub)

	err := svc.UpdateSessionStats(context.Background(), "s1", session.SessionStatsUpdate{
		AddInputTokens: 10,
	})
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, pub.getEvents())
}

func TestUpdateSessionStats_NoEventWhenAlreadyCompleted(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{
		ID:     "s1",
		Status: session.SessionStatusCompleted,
	}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	pub := &mockEventPublisher{}
	svc := newServiceWithPublisher(registry, pub)

	// Re-setting to completed when already completed should not publish.
	err := svc.UpdateSessionStats(context.Background(), "s1", session.SessionStatsUpdate{
		SetStatus: session.SessionStatusCompleted,
	})
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, pub.getEvents())
}

func TestUpdateSessionStats_NilPublisherDoesNotPanic(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{
		ID:     "s1",
		Status: session.SessionStatusActive,
	}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithPublisher(registry, nil)

	err := svc.UpdateSessionStats(context.Background(), "s1", session.SessionStatsUpdate{
		SetStatus: session.SessionStatusCompleted,
	})
	require.NoError(t, err)
}

// --- StreamKey helper test --------------------------------------------------

func TestStreamKey(t *testing.T) {
	assert.Equal(t, "omnia:eval-events:my-ns", StreamKey("my-ns"))
}

// --- RedisEventPublisher tests (with miniredis) ----------------------------

func TestRedisEventPublisher_Close(t *testing.T) {
	pub := &RedisEventPublisher{}
	assert.NoError(t, pub.Close())
}

func TestRedisEventPublisher_PublishMessageEvent(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	pub := NewRedisEventPublisher(client, logr.Discard())

	event := SessionEvent{
		EventType:   "message.assistant",
		SessionID:   "s1",
		AgentName:   "agent-1",
		Namespace:   "test-ns",
		MessageID:   "m1",
		MessageRole: "assistant",
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	err := pub.PublishMessageEvent(context.Background(), event)
	require.NoError(t, err)

	// Verify the message was written to the correct stream.
	streamKey := StreamKey("test-ns")
	msgs, err := client.XRange(context.Background(), streamKey, "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, msgs, 1)

	payload := msgs[0].Values["payload"].(string)
	var decoded SessionEvent
	err = json.Unmarshal([]byte(payload), &decoded)
	require.NoError(t, err)
	assert.Equal(t, "s1", decoded.SessionID)
	assert.Equal(t, "message.assistant", decoded.EventType)
	assert.Equal(t, "test-ns", decoded.Namespace)
}

func TestRedisEventPublisher_PublishMultipleEvents(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	pub := NewRedisEventPublisher(client, logr.Discard())

	for i := 0; i < 3; i++ {
		event := SessionEvent{
			EventType: "message.assistant",
			SessionID: "s1",
			Namespace: "ns",
			Timestamp: time.Now().Format(time.RFC3339),
		}
		require.NoError(t, pub.PublishMessageEvent(context.Background(), event))
	}

	msgs, err := client.XRange(context.Background(), StreamKey("ns"), "-", "+").Result()
	require.NoError(t, err)
	assert.Len(t, msgs, 3)
}

func TestNewRedisEventPublisher(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()

	pub := NewRedisEventPublisher(client, logr.Discard())
	assert.NotNil(t, pub)
	assert.NotNil(t, pub.client)
}

// --- lookupSessionMetadata edge cases ---------------------------------------

func TestLookupSessionMetadata_NoWarmStore(t *testing.T) {
	svc := newServiceWithPublisher(providers.NewRegistry(), nil)
	sess := svc.lookupSessionMetadata(context.Background(), "s1")
	assert.NotNil(t, sess)
	assert.Empty(t, sess.ID)
}

func TestLookupSessionMetadata_SessionNotFound(t *testing.T) {
	warm := newMockWarmStore()
	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithPublisher(registry, nil)

	sess := svc.lookupSessionMetadata(context.Background(), "nonexistent")
	assert.NotNil(t, sess)
	assert.Empty(t, sess.ID)
}

func TestLookupSessionMetadata_Success(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{
		ID:        "s1",
		AgentName: "agent",
		Namespace: "ns",
	}
	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithPublisher(registry, nil)

	sess := svc.lookupSessionMetadata(context.Background(), "s1")
	assert.Equal(t, "s1", sess.ID)
	assert.Equal(t, "agent", sess.AgentName)
}
