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

package httpclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
	"github.com/altairalabs/omnia/internal/session/httpclient"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// integrationWarmStore is an in-memory warm store that records all operations
// so we can verify the full HTTP client → handler → service → warm store chain.
type integrationWarmStore struct {
	mu       sync.Mutex
	sessions map[string]*session.Session
	messages map[string][]*session.Message
}

func newIntegrationWarmStore() *integrationWarmStore {
	return &integrationWarmStore{
		sessions: make(map[string]*session.Session),
		messages: make(map[string][]*session.Message),
	}
}

func (w *integrationWarmStore) CreateSession(_ context.Context, s *session.Session) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, exists := w.sessions[s.ID]; exists {
		return session.ErrSessionNotFound // reuse existing error
	}
	w.sessions[s.ID] = s
	return nil
}

func (w *integrationWarmStore) GetSession(_ context.Context, id string) (*session.Session, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	s, ok := w.sessions[id]
	if !ok {
		return nil, session.ErrSessionNotFound
	}
	return s, nil
}

func (w *integrationWarmStore) UpdateSession(_ context.Context, s *session.Session) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.sessions[s.ID]; !ok {
		return session.ErrSessionNotFound
	}
	w.sessions[s.ID] = s
	return nil
}

func (w *integrationWarmStore) AppendMessage(_ context.Context, sessionID string, msg *session.Message) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.sessions[sessionID]; !ok {
		return session.ErrSessionNotFound
	}
	w.messages[sessionID] = append(w.messages[sessionID], msg)
	return nil
}

func (w *integrationWarmStore) UpdateSessionStats(_ context.Context, sessionID string, update session.SessionStatsUpdate) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	s, ok := w.sessions[sessionID]
	if !ok {
		return session.ErrSessionNotFound
	}
	s.TotalInputTokens += int64(update.AddInputTokens)
	s.TotalOutputTokens += int64(update.AddOutputTokens)
	s.EstimatedCostUSD += update.AddCostUSD
	s.ToolCallCount += update.AddToolCalls
	s.MessageCount += update.AddMessages
	if update.SetStatus != "" {
		s.Status = update.SetStatus
	}
	return nil
}

func (w *integrationWarmStore) DeleteSession(_ context.Context, id string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.sessions, id)
	delete(w.messages, id)
	return nil
}

func (w *integrationWarmStore) GetMessages(_ context.Context, sessionID string, _ providers.MessageQueryOpts) ([]*session.Message, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.sessions[sessionID]; !ok {
		return nil, session.ErrSessionNotFound
	}
	return w.messages[sessionID], nil
}

func (w *integrationWarmStore) ListSessions(context.Context, providers.SessionListOpts) (*providers.SessionPage, error) {
	return &providers.SessionPage{}, nil
}

func (w *integrationWarmStore) SearchSessions(context.Context, string, providers.SessionListOpts) (*providers.SessionPage, error) {
	return &providers.SessionPage{}, nil
}

func (w *integrationWarmStore) CreatePartition(context.Context, time.Time) error { return nil }
func (w *integrationWarmStore) DropPartition(context.Context, time.Time) error   { return nil }
func (w *integrationWarmStore) ListPartitions(context.Context) ([]providers.PartitionInfo, error) {
	return nil, nil
}
func (w *integrationWarmStore) GetSessionsOlderThan(context.Context, time.Time, int) ([]*session.Session, error) {
	return nil, nil
}
func (w *integrationWarmStore) DeleteSessionsBatch(context.Context, []string) error { return nil }
func (w *integrationWarmStore) SaveArtifact(context.Context, *session.Artifact) error {
	return nil
}
func (w *integrationWarmStore) GetArtifacts(context.Context, string) ([]*session.Artifact, error) {
	return nil, nil
}
func (w *integrationWarmStore) GetSessionArtifacts(context.Context, string) ([]*session.Artifact, error) {
	return nil, nil
}
func (w *integrationWarmStore) DeleteSessionArtifacts(context.Context, string) error { return nil }
func (w *integrationWarmStore) Ping(context.Context) error                           { return nil }
func (w *integrationWarmStore) Close() error                                         { return nil }

// getSessionCount returns the number of sessions in the warm store.
func (w *integrationWarmStore) getSessionCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.sessions)
}

// getMessages returns all messages for a session.
func (w *integrationWarmStore) getMessages(sessionID string) []*session.Message {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.messages[sessionID]
}

// getSession returns a session by ID.
func (w *integrationWarmStore) getSession(id string) *session.Session {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.sessions[id]
}

// startIntegrationServer creates a real session-api handler backed by the given
// warm store and returns an httptest.Server and the HTTP client store.
func startIntegrationServer(t *testing.T, warmStore providers.WarmStoreProvider) *httpclient.Store {
	t.Helper()

	registry := providers.NewRegistry()
	registry.SetWarmStore(warmStore)

	svc := api.NewSessionService(registry, api.ServiceConfig{}, logr.Discard())
	handler := api.NewHandler(svc, logr.Discard())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return httpclient.NewStore(srv.URL, logr.Discard())
}

// TestIntegration_FullRecordingChain verifies the complete data flow:
// httpclient.Store → HTTP → session-api handler → service → warm store.
// This is the test that would have caught "no session data from agents".
func TestIntegration_FullRecordingChain(t *testing.T) {
	warmStore := newIntegrationWarmStore()
	store := startIntegrationServer(t, warmStore)
	ctx := context.Background()

	// 1. Create session (as facade.ensureSession would)
	sess, err := store.CreateSession(ctx, session.CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
		TTL:       30 * time.Minute,
	})
	require.NoError(t, err, "CreateSession should succeed")
	require.NotEmpty(t, sess.ID, "session ID should be set")

	// Verify session landed in warm store
	require.Equal(t, 1, warmStore.getSessionCount(), "warm store should have 1 session")
	stored := warmStore.getSession(sess.ID)
	require.NotNil(t, stored, "session should exist in warm store")
	assert.Equal(t, "test-agent", stored.AgentName)
	assert.Equal(t, "default", stored.Namespace)
	assert.Equal(t, session.SessionStatusActive, stored.Status)
	assert.False(t, stored.ExpiresAt.IsZero(), "TTL should set ExpiresAt")

	// 2. Append user message (as facade.processMessage would)
	err = store.AppendMessage(ctx, sess.ID, session.Message{
		Role:      session.RoleUser,
		Content:   "Hello, agent!",
		Timestamp: time.Now(),
	})
	require.NoError(t, err, "AppendMessage (user) should succeed")

	// 3. Append assistant message (as recordingResponseWriter.recordDone would)
	err = store.AppendMessage(ctx, sess.ID, session.Message{
		Role:         session.RoleAssistant,
		Content:      "Hello! How can I help?",
		Timestamp:    time.Now(),
		InputTokens:  10,
		OutputTokens: 8,
		Metadata: map[string]string{
			"latency_ms": "150",
		},
	})
	require.NoError(t, err, "AppendMessage (assistant) should succeed")

	// 4. Update session stats (as recordingResponseWriter.recordDone would)
	err = store.UpdateSessionStats(ctx, sess.ID, session.SessionStatsUpdate{
		AddInputTokens:  10,
		AddOutputTokens: 8,
		AddMessages:     2,
	})
	require.NoError(t, err, "UpdateSessionStats should succeed")

	// 5. Verify messages landed in warm store
	msgs := warmStore.getMessages(sess.ID)
	require.Len(t, msgs, 2, "warm store should have 2 messages")

	assert.Equal(t, session.RoleUser, msgs[0].Role)
	assert.Equal(t, "Hello, agent!", msgs[0].Content)

	assert.Equal(t, session.RoleAssistant, msgs[1].Role)
	assert.Equal(t, "Hello! How can I help?", msgs[1].Content)
	assert.Equal(t, int32(10), msgs[1].InputTokens)
	assert.Equal(t, int32(8), msgs[1].OutputTokens)

	// 6. Verify session stats were updated
	updated := warmStore.getSession(sess.ID)
	assert.Equal(t, int64(10), updated.TotalInputTokens)
	assert.Equal(t, int64(8), updated.TotalOutputTokens)
	assert.Equal(t, int32(2), updated.MessageCount)

	// 7. Refresh TTL (as facade.ensureSession would on reconnect)
	expiresAtBefore := warmStore.getSession(sess.ID).ExpiresAt
	err = store.RefreshTTL(ctx, sess.ID, 1*time.Hour)
	require.NoError(t, err, "RefreshTTL should succeed")

	refreshed := warmStore.getSession(sess.ID)
	assert.True(t, refreshed.ExpiresAt.After(expiresAtBefore),
		"RefreshTTL should extend ExpiresAt (before=%v, after=%v)",
		expiresAtBefore, refreshed.ExpiresAt)
}

// TestIntegration_ToolCallRecording verifies tool call and tool result messages
// are correctly persisted through the full chain.
func TestIntegration_ToolCallRecording(t *testing.T) {
	warmStore := newIntegrationWarmStore()
	store := startIntegrationServer(t, warmStore)
	ctx := context.Background()

	sess, err := store.CreateSession(ctx, session.CreateSessionOptions{
		AgentName: "tool-agent",
		Namespace: "default",
	})
	require.NoError(t, err)

	// Tool call message (as recordingResponseWriter.WriteToolCall would)
	err = store.AppendMessage(ctx, sess.ID, session.Message{
		Role:       session.RoleAssistant,
		Content:    `{"name":"search","arguments":{"query":"weather"}}`,
		ToolCallID: "tc-1",
		Timestamp:  time.Now(),
		Metadata:   map[string]string{"type": "tool_call"},
	})
	require.NoError(t, err)

	// Tool result message (as recordingResponseWriter.WriteToolResult would)
	err = store.AppendMessage(ctx, sess.ID, session.Message{
		Role:       session.RoleSystem,
		Content:    `{"temperature":72,"conditions":"sunny"}`,
		ToolCallID: "tc-1",
		Timestamp:  time.Now(),
		Metadata:   map[string]string{"type": "tool_result"},
	})
	require.NoError(t, err)

	// Update stats for tool call
	err = store.UpdateSessionStats(ctx, sess.ID, session.SessionStatsUpdate{
		AddToolCalls: 1,
		AddMessages:  2,
	})
	require.NoError(t, err)

	msgs := warmStore.getMessages(sess.ID)
	require.Len(t, msgs, 2)
	assert.Equal(t, "tc-1", msgs[0].ToolCallID)
	assert.Equal(t, "tool_call", msgs[0].Metadata["type"])
	assert.Equal(t, "tc-1", msgs[1].ToolCallID)
	assert.Equal(t, "tool_result", msgs[1].Metadata["type"])

	updated := warmStore.getSession(sess.ID)
	assert.Equal(t, int32(1), updated.ToolCallCount)
	assert.Equal(t, int32(2), updated.MessageCount)
}

// TestIntegration_NoWarmStore verifies that the HTTP client gets meaningful
// errors (not silent failures) when the warm store is not configured.
func TestIntegration_NoWarmStore(t *testing.T) {
	// Registry with NO warm store — this is the likely production failure mode.
	registry := providers.NewRegistry()

	svc := api.NewSessionService(registry, api.ServiceConfig{}, logr.Discard())
	handler := api.NewHandler(svc, logr.Discard())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	store := httpclient.NewStore(srv.URL, logr.Discard())
	ctx := context.Background()

	// All operations should fail with meaningful errors, not panic or hang.
	_, err := store.CreateSession(ctx, session.CreateSessionOptions{
		AgentName: "test",
		Namespace: "default",
	})
	require.Error(t, err, "CreateSession should fail without warm store")
	assert.Contains(t, err.Error(), "503", "should get 503 Service Unavailable")
}
