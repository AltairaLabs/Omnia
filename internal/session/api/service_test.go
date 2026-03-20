/*
Copyright 2025.

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
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// --- helpers ---

func newServiceWithRegistry(registry *providers.Registry, auditLogger AuditLogger) *SessionService {
	cfg := ServiceConfig{AuditLogger: auditLogger}
	return NewSessionService(registry, cfg, logr.Discard())
}

func newTestSessionWithMessages(id string, msgs []session.Message) *session.Session {
	return &session.Session{
		ID:            id,
		AgentName:     "test-agent",
		Namespace:     "test-ns",
		WorkspaceName: "test-ws",
		Status:        session.SessionStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Messages:      msgs,
	}
}

// --- NewSessionService ---

func TestNewSessionService_DefaultCacheTTL(t *testing.T) {
	svc := NewSessionService(providers.NewRegistry(), ServiceConfig{}, logr.Discard())
	assert.Equal(t, DefaultCacheTTL, svc.cacheTTL)
}

func TestNewSessionService_CustomCacheTTL(t *testing.T) {
	svc := NewSessionService(providers.NewRegistry(), ServiceConfig{CacheTTL: 5 * time.Minute}, logr.Discard())
	assert.Equal(t, 5*time.Minute, svc.cacheTTL)
}

// --- GetSession ---

func TestGetSession_EmptyID(t *testing.T) {
	registry := providers.NewRegistry()
	registry.SetWarmStore(newMockWarmStore())
	svc := newServiceWithRegistry(registry, nil)
	_, err := svc.GetSession(context.Background(), "")
	assert.ErrorIs(t, err, ErrMissingSessionID)
}

func TestGetSession_WarmPopulatesHot(t *testing.T) {
	hot := newMockHotCache()
	warm := newMockWarmStore()
	sess := &session.Session{ID: "s1", AgentName: "a", Namespace: "n", WorkspaceName: "w"}
	warm.sessions["s1"] = sess

	registry := providers.NewRegistry()
	registry.SetHotCache(hot)
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	got, err := svc.GetSession(context.Background(), "s1")
	require.NoError(t, err)
	assert.Equal(t, "s1", got.ID)
	// Verify hot cache was populated
	assert.Contains(t, hot.sessions, "s1")
}

func TestGetSession_ColdPopulatesHot(t *testing.T) {
	hot := newMockHotCache()
	coldArchive := newMockColdArchive()
	sess := &session.Session{ID: "s1", AgentName: "a", Namespace: "n", WorkspaceName: "w"}
	coldArchive.sessions["s1"] = sess

	registry := providers.NewRegistry()
	registry.SetHotCache(hot)
	registry.SetWarmStore(newMockWarmStore())
	registry.SetColdArchive(coldArchive)
	svc := newServiceWithRegistry(registry, nil)

	got, err := svc.GetSession(context.Background(), "s1")
	require.NoError(t, err)
	assert.Equal(t, "s1", got.ID)
	assert.Contains(t, hot.sessions, "s1")
}

// --- AppendMessage ---

func TestAppendMessage_EmptySessionID(t *testing.T) {
	registry := providers.NewRegistry()
	registry.SetWarmStore(newMockWarmStore())
	svc := newServiceWithRegistry(registry, nil)

	err := svc.AppendMessage(context.Background(), "", &session.Message{})
	assert.ErrorIs(t, err, ErrMissingSessionID)
}

func TestAppendMessage_NoWarmStore(t *testing.T) {
	svc := newServiceWithRegistry(providers.NewRegistry(), nil)
	err := svc.AppendMessage(context.Background(), "s1", &session.Message{})
	assert.ErrorIs(t, err, ErrWarmStoreRequired)
}

func TestAppendMessage_Success(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{ID: "s1"}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	msg := &session.Message{ID: "m1", Role: session.RoleUser, Content: "hello"}
	err := svc.AppendMessage(context.Background(), "s1", msg)
	require.NoError(t, err)
	assert.Len(t, warm.appendedMsgs["s1"], 1)
}

// --- UpdateSessionStatus ---

func TestUpdateSessionStatus_EmptySessionID(t *testing.T) {
	registry := providers.NewRegistry()
	registry.SetWarmStore(newMockWarmStore())
	svc := newServiceWithRegistry(registry, nil)

	err := svc.UpdateSessionStatus(context.Background(), "", session.SessionStatusUpdate{})
	assert.ErrorIs(t, err, ErrMissingSessionID)
}

func TestUpdateSessionStatus_NoWarmStore(t *testing.T) {
	svc := newServiceWithRegistry(providers.NewRegistry(), nil)
	err := svc.UpdateSessionStatus(context.Background(), "s1", session.SessionStatusUpdate{})
	assert.ErrorIs(t, err, ErrWarmStoreRequired)
}

func TestUpdateSessionStatus_AppliesIncrements(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{
		ID:                "s1",
		TotalInputTokens:  100,
		TotalOutputTokens: 50,
		ToolCallCount:     3,
		MessageCount:      5,
	}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	err := svc.UpdateSessionStatus(context.Background(), "s1", session.SessionStatusUpdate{
		SetStatus: session.SessionStatusCompleted,
	})
	require.NoError(t, err)
	require.Len(t, warm.updatedSessions, 1)
	updated := warm.updatedSessions[0]
	assert.Equal(t, session.SessionStatusCompleted, updated.Status)
}

func TestUpdateSessionStatus_SessionNotFound(t *testing.T) {
	warm := newMockWarmStore()
	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	err := svc.UpdateSessionStatus(context.Background(), "nonexistent", session.SessionStatusUpdate{})
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
}

func TestUpdateSessionStatus_NoStatusChange(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{
		ID:     "s1",
		Status: session.SessionStatusActive,
	}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	err := svc.UpdateSessionStatus(context.Background(), "s1", session.SessionStatusUpdate{})
	require.NoError(t, err)
	require.Len(t, warm.updatedSessions, 1)
	assert.Equal(t, session.SessionStatusActive, warm.updatedSessions[0].Status)
}

func TestUpdateSessionStatus_CompletionTransitionPublishes(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{
		ID:     "s1",
		Status: session.SessionStatusActive,
	}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	err := svc.UpdateSessionStatus(context.Background(), "s1", session.SessionStatusUpdate{
		SetStatus: session.SessionStatusCompleted,
	})
	require.NoError(t, err)
	require.Len(t, warm.updatedSessions, 1)
	assert.Equal(t, session.SessionStatusCompleted, warm.updatedSessions[0].Status)
}

func TestUpdateSessionStatus_AlreadyCompletedNoRepublish(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{
		ID:     "s1",
		Status: session.SessionStatusCompleted,
	}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	// Updating a session that's already completed should still work
	err := svc.UpdateSessionStatus(context.Background(), "s1", session.SessionStatusUpdate{
		SetStatus: session.SessionStatusCompleted,
	})
	require.NoError(t, err)
	require.Len(t, warm.updatedSessions, 1)
}

func TestUpdateSessionStatus_NonCompletedStatusSkipsLookup(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{
		ID:     "s1",
		Status: session.SessionStatusActive,
	}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	// Non-completed status should not trigger the completion lookup/publish path
	err := svc.UpdateSessionStatus(context.Background(), "s1", session.SessionStatusUpdate{})
	require.NoError(t, err)
	require.Len(t, warm.updatedSessions, 1)
	assert.Equal(t, session.SessionStatusActive, warm.updatedSessions[0].Status)
}

// --- RefreshTTL ---

func TestRefreshTTL_EmptySessionID(t *testing.T) {
	registry := providers.NewRegistry()
	registry.SetWarmStore(newMockWarmStore())
	svc := newServiceWithRegistry(registry, nil)

	err := svc.RefreshTTL(context.Background(), "", time.Hour)
	assert.ErrorIs(t, err, ErrMissingSessionID)
}

func TestRefreshTTL_NoWarmStore(t *testing.T) {
	svc := newServiceWithRegistry(providers.NewRegistry(), nil)
	err := svc.RefreshTTL(context.Background(), "s1", time.Hour)
	assert.ErrorIs(t, err, ErrWarmStoreRequired)
}

func TestRefreshTTL_Success(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{ID: "s1"}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	err := svc.RefreshTTL(context.Background(), "s1", 2*time.Hour)
	require.NoError(t, err)
	require.Len(t, warm.updatedSessions, 1)
	assert.WithinDuration(t, time.Now().Add(2*time.Hour), warm.updatedSessions[0].ExpiresAt, 5*time.Second)
}

func TestRefreshTTL_SessionNotFound(t *testing.T) {
	warm := newMockWarmStore()
	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	err := svc.RefreshTTL(context.Background(), "nonexistent", time.Hour)
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
}

// --- GetMessages ---

func TestGetMessages_EmptySessionID(t *testing.T) {
	registry := providers.NewRegistry()
	registry.SetWarmStore(newMockWarmStore())
	svc := newServiceWithRegistry(registry, nil)

	_, err := svc.GetMessages(context.Background(), "", providers.MessageQueryOpts{})
	assert.ErrorIs(t, err, ErrMissingSessionID)
}

func TestGetMessages_NotFoundAllTiers(t *testing.T) {
	registry := providers.NewRegistry()
	registry.SetHotCache(newMockHotCache())
	registry.SetWarmStore(newMockWarmStore())
	registry.SetColdArchive(newMockColdArchive())
	svc := newServiceWithRegistry(registry, nil)

	_, err := svc.GetMessages(context.Background(), "nonexistent", providers.MessageQueryOpts{})
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
}

func TestGetMessages_ColdFallbackWithFilter(t *testing.T) {
	coldArchive := newMockColdArchive()
	coldArchive.sessions["s1"] = newTestSessionWithMessages("s1", []session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "a", SequenceNum: 1},
		{ID: "m2", Role: session.RoleAssistant, Content: "b", SequenceNum: 2},
		{ID: "m3", Role: session.RoleUser, Content: "c", SequenceNum: 3},
	})

	registry := providers.NewRegistry()
	registry.SetWarmStore(newMockWarmStore())
	registry.SetColdArchive(coldArchive)
	svc := newServiceWithRegistry(registry, nil)

	// Use role filter which forces cold fallback with in-memory filtering
	msgs, err := svc.GetMessages(context.Background(), "s1", providers.MessageQueryOpts{
		Roles: []session.MessageRole{session.RoleUser},
	})
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
}

// --- CreateSession ---

func TestCreateSession_NoWarmStore(t *testing.T) {
	svc := newServiceWithRegistry(providers.NewRegistry(), nil)
	err := svc.CreateSession(context.Background(), &session.Session{ID: "s1"})
	assert.ErrorIs(t, err, ErrWarmStoreRequired)
}

func TestCreateSession_Success(t *testing.T) {
	warm := newMockWarmStore()
	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	sess := &session.Session{ID: "s1", AgentName: "agent"}
	err := svc.CreateSession(context.Background(), sess)
	require.NoError(t, err)
	assert.Len(t, warm.createdSessions, 1)
}

// --- CreateSession audit ---

func TestCreateSession_AuditEvent(t *testing.T) {
	warm := newMockWarmStore()
	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)

	al := &mockAuditLogger{}
	svc := newServiceWithRegistry(registry, al)

	ctx := withRequestContext(context.Background(), RequestContext{
		IPAddress: "1.2.3.4",
		UserAgent: "test-ua",
	})
	sess := &session.Session{ID: "s1", AgentName: "agent", Namespace: "ns", WorkspaceName: "ws"}
	err := svc.CreateSession(ctx, sess)
	require.NoError(t, err)
	require.Len(t, al.entries, 1)
	assert.Equal(t, "session_created", al.entries[0].EventType)
	assert.Equal(t, "s1", al.entries[0].SessionID)
	assert.Equal(t, "ws", al.entries[0].Workspace)
	assert.Equal(t, "1.2.3.4", al.entries[0].IPAddress)
}

func TestCreateSession_NoAuditOnError(t *testing.T) {
	svc := newServiceWithRegistry(providers.NewRegistry(), &mockAuditLogger{})
	err := svc.CreateSession(context.Background(), &session.Session{ID: "s1"})
	assert.ErrorIs(t, err, ErrWarmStoreRequired)
}

// --- DeleteSession ---

func TestDeleteSession_EmptySessionID(t *testing.T) {
	registry := providers.NewRegistry()
	registry.SetWarmStore(newMockWarmStore())
	svc := newServiceWithRegistry(registry, nil)
	err := svc.DeleteSession(context.Background(), "")
	assert.ErrorIs(t, err, ErrMissingSessionID)
}

func TestDeleteSession_NoWarmStore(t *testing.T) {
	svc := newServiceWithRegistry(providers.NewRegistry(), nil)
	err := svc.DeleteSession(context.Background(), "s1")
	assert.ErrorIs(t, err, ErrWarmStoreRequired)
}

func TestDeleteSession_Success(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{ID: "s1", AgentName: "a", WorkspaceName: "ws"}
	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)

	al := &mockAuditLogger{}
	svc := newServiceWithRegistry(registry, al)

	err := svc.DeleteSession(context.Background(), "s1")
	require.NoError(t, err)
	assert.NotContains(t, warm.sessions, "s1")
	require.Len(t, al.entries, 1)
	assert.Equal(t, "session_deleted", al.entries[0].EventType)
	assert.Equal(t, "ws", al.entries[0].Workspace)
}

func TestDeleteSession_NotFound(t *testing.T) {
	warm := newMockWarmStore()
	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	err := svc.DeleteSession(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
}

func TestDeleteSession_NilAuditLogger(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{ID: "s1"}
	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	err := svc.DeleteSession(context.Background(), "s1")
	require.NoError(t, err)
}

func TestAuditSessionCreated_NilLogger(t *testing.T) {
	svc := newServiceWithRegistry(providers.NewRegistry(), nil)
	svc.auditSessionCreated(context.Background(), &session.Session{ID: "s1"})
}

func TestAuditSessionDeleted_NilLogger(t *testing.T) {
	svc := newServiceWithRegistry(providers.NewRegistry(), nil)
	svc.auditSessionDeleted(context.Background(), "s1", nil, nil)
}

func TestAuditSessionDeleted_WithLogger_NoSession(t *testing.T) {
	al := &mockAuditLogger{}
	svc := newServiceWithRegistry(providers.NewRegistry(), al)
	svc.auditSessionDeleted(context.Background(), "s1", nil, session.ErrSessionNotFound)
	require.Len(t, al.entries, 1)
	assert.Equal(t, "session_deleted", al.entries[0].EventType)
	assert.Equal(t, "s1", al.entries[0].SessionID)
	assert.Equal(t, "", al.entries[0].Workspace)
}

func TestAuditSessionCreated_WithLogger(t *testing.T) {
	al := &mockAuditLogger{}
	svc := newServiceWithRegistry(providers.NewRegistry(), al)
	ctx := withRequestContext(context.Background(), RequestContext{
		IPAddress: "5.6.7.8",
		UserAgent: "ua",
	})
	svc.auditSessionCreated(ctx, &session.Session{
		ID: "s1", WorkspaceName: "ws", AgentName: "ag", Namespace: "ns",
	})
	require.Len(t, al.entries, 1)
	assert.Equal(t, "session_created", al.entries[0].EventType)
	assert.Equal(t, "5.6.7.8", al.entries[0].IPAddress)
	assert.Equal(t, "ws", al.entries[0].Workspace)
}

// --- populateHotCache edge cases ---

func TestPopulateHotCache_SetError_DoesNotPanic(t *testing.T) {
	hot := newMockHotCache()
	hot.setErr = assert.AnError

	registry := providers.NewRegistry()
	registry.SetHotCache(hot)
	registry.SetWarmStore(newMockWarmStore())
	svc := newServiceWithRegistry(registry, nil)

	// Should not panic on set error
	svc.populateHotCache(context.Background(), &session.Session{ID: "s1"})
}

// --- getFrom helpers ---

func TestGetFromHot_NoCacheConfigured(t *testing.T) {
	svc := newServiceWithRegistry(providers.NewRegistry(), nil)
	_, err := svc.getFromHot(context.Background(), "s1")
	assert.Error(t, err)
}

func TestGetFromWarm_NoStoreConfigured(t *testing.T) {
	svc := newServiceWithRegistry(providers.NewRegistry(), nil)
	_, err := svc.getFromWarm(context.Background(), "s1")
	assert.Error(t, err)
}

func TestGetFromCold_NoArchiveConfigured(t *testing.T) {
	svc := newServiceWithRegistry(providers.NewRegistry(), nil)
	_, err := svc.getFromCold(context.Background(), "s1")
	assert.Error(t, err)
}

// --- filterMessages additional cases ---

func TestFilterMessages_EmptyInput(t *testing.T) {
	result := filterMessages(nil, providers.MessageQueryOpts{})
	assert.Empty(t, result)
}

func TestFilterMessages_DescSort(t *testing.T) {
	messages := []session.Message{
		{ID: "m1", SequenceNum: 1},
		{ID: "m2", SequenceNum: 3},
		{ID: "m3", SequenceNum: 2},
	}
	result := filterMessages(messages, providers.MessageQueryOpts{SortOrder: providers.SortDesc})
	require.Len(t, result, 3)
	assert.Equal(t, int32(3), result[0].SequenceNum)
	assert.Equal(t, int32(2), result[1].SequenceNum)
	assert.Equal(t, int32(1), result[2].SequenceNum)
}

func TestFilterMessages_OffsetExactlyAtLength(t *testing.T) {
	messages := []session.Message{
		{ID: "m1", SequenceNum: 1},
		{ID: "m2", SequenceNum: 2},
	}
	result := filterMessages(messages, providers.MessageQueryOpts{Offset: 2})
	assert.Nil(t, result)
}

// --- audit helper edge cases ---

func TestAuditSessionAccess_NilLogger(t *testing.T) {
	svc := newServiceWithRegistry(providers.NewRegistry(), nil)
	// Should not panic with nil logger
	svc.auditSessionAccess(context.Background(), &session.Session{ID: "s1"})
}

func TestAuditMessagesAccess_NilLogger(t *testing.T) {
	svc := newServiceWithRegistry(providers.NewRegistry(), nil)
	// Should not panic
	svc.auditMessagesAccess(context.Background(), "s1", 5)
}

func TestAuditSearch_NilLogger(t *testing.T) {
	svc := newServiceWithRegistry(providers.NewRegistry(), nil)
	// Should not panic
	svc.auditSearch(context.Background(), "query", "ws1", 3)
}

func TestAuditSessionAccess_WithRequestContext(t *testing.T) {
	al := &mockAuditLogger{}
	svc := newServiceWithRegistry(providers.NewRegistry(), al)

	ctx := withRequestContext(context.Background(), RequestContext{
		IPAddress: "1.2.3.4",
		UserAgent: "test-ua",
	})
	svc.auditSessionAccess(ctx, &session.Session{
		ID:            "s1",
		WorkspaceName: "ws1",
		AgentName:     "agent1",
		Namespace:     "ns1",
	})

	require.Len(t, al.entries, 1)
	assert.Equal(t, "session_accessed", al.entries[0].EventType)
	assert.Equal(t, "1.2.3.4", al.entries[0].IPAddress)
	assert.Equal(t, "test-ua", al.entries[0].UserAgent)
	assert.Equal(t, "ws1", al.entries[0].Workspace)
}

func TestAuditMessagesAccess_WithLogger(t *testing.T) {
	al := &mockAuditLogger{}
	svc := newServiceWithRegistry(providers.NewRegistry(), al)

	svc.auditMessagesAccess(context.Background(), "s1", 10)

	require.Len(t, al.entries, 1)
	assert.Equal(t, "session_accessed", al.entries[0].EventType)
	assert.Equal(t, "s1", al.entries[0].SessionID)
	assert.Equal(t, 10, al.entries[0].ResultCount)
}

func TestAuditSearch_WithLogger(t *testing.T) {
	al := &mockAuditLogger{}
	svc := newServiceWithRegistry(providers.NewRegistry(), al)

	svc.auditSearch(context.Background(), "my query", "ws1", 7)

	require.Len(t, al.entries, 1)
	assert.Equal(t, "session_searched", al.entries[0].EventType)
	assert.Equal(t, "my query", al.entries[0].Query)
	assert.Equal(t, "ws1", al.entries[0].Workspace)
	assert.Equal(t, 7, al.entries[0].ResultCount)
}

// --- trackingHotCache for write-through tests ---

// trackingHotCache wraps mockHotCache and tracks write-through calls with synchronization.
type trackingHotCache struct {
	mockHotCache
	mu              sync.Mutex
	setCalls        []*session.Session
	appendCalls     []appendCall
	invalidateCalls []string
	refreshCalls    []string
	done            chan struct{} // closed after first call for sync
}

type appendCall struct {
	sessionID string
	msg       *session.Message
}

func newTrackingHotCache() *trackingHotCache {
	return &trackingHotCache{
		mockHotCache: mockHotCache{sessions: make(map[string]*session.Session)},
		done:         make(chan struct{}, 4), // buffered for multiple calls
	}
}

func (t *trackingHotCache) SetSession(ctx context.Context, s *session.Session, ttl time.Duration) error {
	t.mu.Lock()
	t.setCalls = append(t.setCalls, s)
	t.mu.Unlock()
	t.done <- struct{}{}
	return t.mockHotCache.SetSession(ctx, s, ttl)
}

func (t *trackingHotCache) AppendMessage(ctx context.Context, sessionID string, msg *session.Message) error {
	t.mu.Lock()
	t.appendCalls = append(t.appendCalls, appendCall{sessionID, msg})
	t.mu.Unlock()
	t.done <- struct{}{}
	return t.mockHotCache.AppendMessage(ctx, sessionID, msg)
}

func (t *trackingHotCache) Invalidate(_ context.Context, sessionID string) error {
	t.mu.Lock()
	t.invalidateCalls = append(t.invalidateCalls, sessionID)
	t.mu.Unlock()
	t.done <- struct{}{}
	return nil
}

func (t *trackingHotCache) RefreshTTL(_ context.Context, sessionID string, _ time.Duration) error {
	t.mu.Lock()
	t.refreshCalls = append(t.refreshCalls, sessionID)
	t.mu.Unlock()
	t.done <- struct{}{}
	return nil
}

func (t *trackingHotCache) waitOne() {
	<-t.done
}

// --- pushToHotCache / write-through tests ---

func TestPushToHotCache_NoCacheConfigured(t *testing.T) {
	svc := newServiceWithRegistry(providers.NewRegistry(), nil)
	called := false
	svc.pushToHotCache(func(_ context.Context, _ providers.HotCacheProvider) {
		called = true
	})
	// Give goroutine a chance to run (it shouldn't)
	time.Sleep(10 * time.Millisecond)
	assert.False(t, called, "callback should not be called when no hot cache is configured")
}

func TestCreateSession_WriteThroughToHotCache(t *testing.T) {
	hot := newTrackingHotCache()
	warm := newMockWarmStore()
	registry := providers.NewRegistry()
	registry.SetHotCache(hot)
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	sess := &session.Session{ID: "s1", AgentName: "agent"}
	err := svc.CreateSession(context.Background(), sess)
	require.NoError(t, err)

	hot.waitOne()
	hot.mu.Lock()
	defer hot.mu.Unlock()
	require.Len(t, hot.setCalls, 1)
	assert.Equal(t, "s1", hot.setCalls[0].ID)
}

func TestDeleteSession_InvalidatesHotCache(t *testing.T) {
	hot := newTrackingHotCache()
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{ID: "s1"}
	registry := providers.NewRegistry()
	registry.SetHotCache(hot)
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	err := svc.DeleteSession(context.Background(), "s1")
	require.NoError(t, err)

	hot.waitOne()
	hot.mu.Lock()
	defer hot.mu.Unlock()
	require.Len(t, hot.invalidateCalls, 1)
	assert.Equal(t, "s1", hot.invalidateCalls[0])
}

func TestAppendMessage_WriteThroughToHotCache(t *testing.T) {
	hot := newTrackingHotCache()
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{ID: "s1"}
	registry := providers.NewRegistry()
	registry.SetHotCache(hot)
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	msg := &session.Message{ID: "m1", Role: session.RoleUser, Content: "hello"}
	err := svc.AppendMessage(context.Background(), "s1", msg)
	require.NoError(t, err)

	hot.waitOne()
	hot.mu.Lock()
	defer hot.mu.Unlock()
	require.Len(t, hot.appendCalls, 1)
	assert.Equal(t, "s1", hot.appendCalls[0].sessionID)
	assert.Equal(t, "m1", hot.appendCalls[0].msg.ID)
}

func TestUpdateSessionStatus_RefreshesTTLInHotCache(t *testing.T) {
	hot := newTrackingHotCache()
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{ID: "s1", Status: session.SessionStatusActive}
	registry := providers.NewRegistry()
	registry.SetHotCache(hot)
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	err := svc.UpdateSessionStatus(context.Background(), "s1", session.SessionStatusUpdate{
		SetStatus: session.SessionStatusActive,
	})
	require.NoError(t, err)

	hot.waitOne()
	hot.mu.Lock()
	defer hot.mu.Unlock()
	require.Len(t, hot.refreshCalls, 1)
	assert.Equal(t, "s1", hot.refreshCalls[0])
}

// --- mockWarmStoreWithUpdater implements StatusUpdaterWithResult ---

// mockWarmStoreWithUpdater wraps mockWarmStore and implements StatusUpdaterWithResult
// so the optimized path in UpdateSessionStatus is exercised.
type mockWarmStoreWithUpdater struct {
	mockWarmStore
	result *providers.StatusUpdateResult
	err    error
	called bool
}

func newMockWarmStoreWithUpdater() *mockWarmStoreWithUpdater {
	return &mockWarmStoreWithUpdater{
		mockWarmStore: mockWarmStore{
			sessions:     make(map[string]*session.Session),
			messages:     make(map[string][]*session.Message),
			appendedMsgs: make(map[string][]*session.Message),
		},
	}
}

func (m *mockWarmStoreWithUpdater) UpdateSessionStatusReturning(_ context.Context, _ string, _ session.SessionStatusUpdate) (*providers.StatusUpdateResult, error) {
	m.called = true
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

// --- updateStatusOptimized tests (via StatusUpdaterWithResult) ---

func TestUpdateSessionStatus_Optimized_Applied(t *testing.T) {
	warm := newMockWarmStoreWithUpdater()
	warm.result = &providers.StatusUpdateResult{
		Applied:           true,
		PreviousStatus:    session.SessionStatusActive,
		AgentName:         "test-agent",
		Namespace:         "test-ns",
		PromptPackName:    "pp1",
		PromptPackVersion: "v1",
	}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	err := svc.UpdateSessionStatus(context.Background(), "s1", session.SessionStatusUpdate{
		SetStatus: session.SessionStatusCompleted,
	})
	require.NoError(t, err)
	assert.True(t, warm.called)
	// Verify fallback path was NOT used (no updatedSessions entries).
	assert.Empty(t, warm.updatedSessions)
}

func TestUpdateSessionStatus_Optimized_Skipped(t *testing.T) {
	warm := newMockWarmStoreWithUpdater()
	warm.result = &providers.StatusUpdateResult{
		Applied:        false,
		PreviousStatus: session.SessionStatusCompleted,
	}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	err := svc.UpdateSessionStatus(context.Background(), "s1", session.SessionStatusUpdate{
		SetStatus: session.SessionStatusCompleted,
	})
	require.NoError(t, err)
	assert.True(t, warm.called)
}

func TestUpdateSessionStatus_Optimized_Error(t *testing.T) {
	warm := newMockWarmStoreWithUpdater()
	warm.err = errors.New("db connection lost")

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	err := svc.UpdateSessionStatus(context.Background(), "s1", session.SessionStatusUpdate{
		SetStatus: session.SessionStatusCompleted,
	})
	assert.EqualError(t, err, "db connection lost")
	assert.True(t, warm.called)
}

func TestUpdateSessionStatus_Optimized_CompletionPublishesEvent(t *testing.T) {
	warm := newMockWarmStoreWithUpdater()
	warm.result = &providers.StatusUpdateResult{
		Applied:           true,
		PreviousStatus:    session.SessionStatusActive,
		AgentName:         "test-agent",
		Namespace:         "test-ns",
		PromptPackName:    "pp1",
		PromptPackVersion: "v1",
	}

	pub := &mockEventPublisher{}
	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	cfg := ServiceConfig{EventPublisher: pub}
	svc := NewSessionService(registry, cfg, logr.Discard())

	err := svc.UpdateSessionStatus(context.Background(), "s1", session.SessionStatusUpdate{
		SetStatus: session.SessionStatusCompleted,
	})
	require.NoError(t, err)

	// publishSessionCompleted fires async — wait for the event.
	events := pub.waitForEvents(t, 1, 2*time.Second)
	require.Len(t, events, 1)
	assert.Equal(t, "session.completed", events[0].EventType)
	assert.Equal(t, "s1", events[0].SessionID)
	assert.Equal(t, "test-agent", events[0].AgentName)
	assert.Equal(t, "test-ns", events[0].Namespace)
	assert.Equal(t, "pp1", events[0].PromptPackName)
	assert.Equal(t, "v1", events[0].PromptPackVersion)
}

func TestUpdateSessionStatus_Optimized_AlreadyCompletedNoPublish(t *testing.T) {
	warm := newMockWarmStoreWithUpdater()
	warm.result = &providers.StatusUpdateResult{
		Applied:        true,
		PreviousStatus: session.SessionStatusCompleted,
	}

	pub := &mockEventPublisher{}
	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	cfg := ServiceConfig{EventPublisher: pub}
	svc := NewSessionService(registry, cfg, logr.Discard())

	err := svc.UpdateSessionStatus(context.Background(), "s1", session.SessionStatusUpdate{
		SetStatus: session.SessionStatusCompleted,
	})
	require.NoError(t, err)

	// No event should be published — already completed.
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, pub.getEvents())
}

func TestUpdateSessionStatus_Optimized_NonCompletedStatusNoPublish(t *testing.T) {
	warm := newMockWarmStoreWithUpdater()
	warm.result = &providers.StatusUpdateResult{
		Applied:        true,
		PreviousStatus: session.SessionStatusActive,
	}

	pub := &mockEventPublisher{}
	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	cfg := ServiceConfig{EventPublisher: pub}
	svc := NewSessionService(registry, cfg, logr.Discard())

	err := svc.UpdateSessionStatus(context.Background(), "s1", session.SessionStatusUpdate{
		SetStatus: session.SessionStatusActive,
	})
	require.NoError(t, err)

	// Non-completed status should not publish.
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, pub.getEvents())
}

func TestUpdateSessionStatus_Optimized_RefreshesTTLInHotCache(t *testing.T) {
	hot := newTrackingHotCache()
	warm := newMockWarmStoreWithUpdater()
	warm.result = &providers.StatusUpdateResult{
		Applied:        true,
		PreviousStatus: session.SessionStatusActive,
	}

	registry := providers.NewRegistry()
	registry.SetHotCache(hot)
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	err := svc.UpdateSessionStatus(context.Background(), "s1", session.SessionStatusUpdate{
		SetStatus: session.SessionStatusActive,
	})
	require.NoError(t, err)

	hot.waitOne()
	hot.mu.Lock()
	defer hot.mu.Unlock()
	require.Len(t, hot.refreshCalls, 1)
	assert.Equal(t, "s1", hot.refreshCalls[0])
}

// --- SA-25: Concurrent RefreshTTL ---

func TestRefreshTTL_Concurrent(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{ID: "s1"}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	const goroutines = 10
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = svc.RefreshTTL(context.Background(), "s1", time.Duration(idx+1)*time.Hour)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, err)
		}
	}
}

// --- RecordRuntimeEvent service tests ---

func TestRecordRuntimeEvent_EmptySessionID(t *testing.T) {
	registry := providers.NewRegistry()
	registry.SetWarmStore(newMockWarmStore())
	svc := newServiceWithRegistry(registry, nil)

	err := svc.RecordRuntimeEvent(context.Background(), "", &session.RuntimeEvent{})
	assert.ErrorIs(t, err, ErrMissingSessionID)
}

func TestRecordRuntimeEvent_NoWarmStore(t *testing.T) {
	svc := newServiceWithRegistry(providers.NewRegistry(), nil)
	err := svc.RecordRuntimeEvent(context.Background(), "s1", &session.RuntimeEvent{})
	assert.ErrorIs(t, err, ErrWarmStoreRequired)
}

func TestRecordRuntimeEvent_Success(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{ID: "s1"}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	evt := &session.RuntimeEvent{
		ID:        "evt1",
		SessionID: "s1",
		EventType: "pipeline.started",
	}
	err := svc.RecordRuntimeEvent(context.Background(), "s1", evt)
	require.NoError(t, err)
}
