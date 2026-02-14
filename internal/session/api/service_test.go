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

// --- UpdateSessionStats ---

func TestUpdateSessionStats_EmptySessionID(t *testing.T) {
	registry := providers.NewRegistry()
	registry.SetWarmStore(newMockWarmStore())
	svc := newServiceWithRegistry(registry, nil)

	err := svc.UpdateSessionStats(context.Background(), "", session.SessionStatsUpdate{})
	assert.ErrorIs(t, err, ErrMissingSessionID)
}

func TestUpdateSessionStats_NoWarmStore(t *testing.T) {
	svc := newServiceWithRegistry(providers.NewRegistry(), nil)
	err := svc.UpdateSessionStats(context.Background(), "s1", session.SessionStatsUpdate{})
	assert.ErrorIs(t, err, ErrWarmStoreRequired)
}

func TestUpdateSessionStats_AppliesIncrements(t *testing.T) {
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

	err := svc.UpdateSessionStats(context.Background(), "s1", session.SessionStatsUpdate{
		AddInputTokens:  50,
		AddOutputTokens: 30,
		AddToolCalls:    2,
		AddMessages:     1,
		AddCostUSD:      0.01,
		SetStatus:       session.SessionStatusCompleted,
	})
	require.NoError(t, err)
	require.Len(t, warm.updatedSessions, 1)
	updated := warm.updatedSessions[0]
	assert.Equal(t, int64(150), updated.TotalInputTokens)
	assert.Equal(t, int64(80), updated.TotalOutputTokens)
	assert.Equal(t, int32(5), updated.ToolCallCount)
	assert.Equal(t, int32(6), updated.MessageCount)
	assert.Equal(t, session.SessionStatusCompleted, updated.Status)
}

func TestUpdateSessionStats_SessionNotFound(t *testing.T) {
	warm := newMockWarmStore()
	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	err := svc.UpdateSessionStats(context.Background(), "nonexistent", session.SessionStatsUpdate{})
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
}

func TestUpdateSessionStats_NoStatusChange(t *testing.T) {
	warm := newMockWarmStore()
	warm.sessions["s1"] = &session.Session{
		ID:     "s1",
		Status: session.SessionStatusActive,
	}

	registry := providers.NewRegistry()
	registry.SetWarmStore(warm)
	svc := newServiceWithRegistry(registry, nil)

	err := svc.UpdateSessionStats(context.Background(), "s1", session.SessionStatsUpdate{
		AddInputTokens: 10,
	})
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
