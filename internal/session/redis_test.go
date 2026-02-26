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

package session

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestRedisStore creates a RedisStore backed by miniredis for testing.
func newTestRedisStore(t *testing.T) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return &RedisStore{
		client:    client,
		keyPrefix: "",
	}, mr
}

func TestRedisStore_CreateAndGetSession(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if sess.AgentName != "test-agent" {
		t.Errorf("AgentName = %q, want %q", sess.AgentName, "test-agent")
	}

	got, err := store.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got.ID != sess.ID {
		t.Errorf("GetSession ID = %q, want %q", got.ID, sess.ID)
	}
}

func TestRedisStore_CreateSessionWithTTL(t *testing.T) {
	store, mr := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
		TTL:       10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if sess.ExpiresAt.IsZero() {
		t.Error("expected non-zero ExpiresAt with TTL")
	}

	// Verify key has TTL in Redis
	ttl := mr.TTL(store.sessionKey(sess.ID))
	if ttl == 0 {
		t.Error("expected session key to have TTL")
	}
}

func TestRedisStore_CreateSessionWithInitialState(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName:    "test-agent",
		Namespace:    "default",
		InitialState: map[string]string{"key1": "val1"},
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	got, err := store.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got.State["key1"] != "val1" {
		t.Errorf("State[key1] = %q, want %q", got.State["key1"], "val1")
	}
}

func TestRedisStore_GetSession_NotFound(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	_, err := store.GetSession(context.Background(), "nonexistent")
	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got: %v", err)
	}
}

func TestRedisStore_GetSession_EmptyID(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	_, err := store.GetSession(context.Background(), "")
	if err != ErrInvalidSessionID {
		t.Errorf("expected ErrInvalidSessionID, got: %v", err)
	}
}

func TestRedisStore_DeleteSession(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if err := store.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	_, err = store.GetSession(ctx, sess.ID)
	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound after delete, got: %v", err)
	}
}

func TestRedisStore_DeleteSession_NotFound(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	err := store.DeleteSession(context.Background(), "nonexistent")
	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got: %v", err)
	}
}

func TestRedisStore_DeleteSession_EmptyID(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	err := store.DeleteSession(context.Background(), "")
	if err != ErrInvalidSessionID {
		t.Errorf("expected ErrInvalidSessionID, got: %v", err)
	}
}

func TestRedisStore_AppendAndGetMessages(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	msg := Message{
		Role:    RoleUser,
		Content: "hello",
	}
	if err := store.AppendMessage(ctx, sess.ID, msg); err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}

	messages, err := store.GetMessages(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Content != "hello" {
		t.Errorf("message content = %q, want %q", messages[0].Content, "hello")
	}
	if messages[0].ID == "" {
		t.Error("expected message ID to be generated")
	}
}

func TestRedisStore_AppendMessage_NotFound(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	err := store.AppendMessage(context.Background(), "nonexistent", Message{Content: "hi"})
	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got: %v", err)
	}
}

func TestRedisStore_AppendMessage_EmptyID(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	err := store.AppendMessage(context.Background(), "", Message{Content: "hi"})
	if err != ErrInvalidSessionID {
		t.Errorf("expected ErrInvalidSessionID, got: %v", err)
	}
}

func TestRedisStore_GetMessages_NotFound(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	_, err := store.GetMessages(context.Background(), "nonexistent")
	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got: %v", err)
	}
}

func TestRedisStore_GetMessages_EmptyID(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	_, err := store.GetMessages(context.Background(), "")
	if err != ErrInvalidSessionID {
		t.Errorf("expected ErrInvalidSessionID, got: %v", err)
	}
}

func TestRedisStore_SetAndGetState(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if err := store.SetState(ctx, sess.ID, "color", "blue"); err != nil {
		t.Fatalf("SetState failed: %v", err)
	}

	val, err := store.GetState(ctx, sess.ID, "color")
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if val != "blue" {
		t.Errorf("state value = %q, want %q", val, "blue")
	}
}

func TestRedisStore_GetState_MissingKey(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	val, err := store.GetState(ctx, sess.ID, "nonexistent-key")
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for missing key, got %q", val)
	}
}

func TestRedisStore_SetState_NotFound(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	err := store.SetState(context.Background(), "nonexistent", "k", "v")
	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got: %v", err)
	}
}

func TestRedisStore_SetState_EmptyID(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	err := store.SetState(context.Background(), "", "k", "v")
	if err != ErrInvalidSessionID {
		t.Errorf("expected ErrInvalidSessionID, got: %v", err)
	}
}

func TestRedisStore_GetState_NotFound(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	_, err := store.GetState(context.Background(), "nonexistent", "k")
	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got: %v", err)
	}
}

func TestRedisStore_GetState_EmptyID(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	_, err := store.GetState(context.Background(), "", "k")
	if err != ErrInvalidSessionID {
		t.Errorf("expected ErrInvalidSessionID, got: %v", err)
	}
}

func TestRedisStore_RefreshTTL_Atomic(t *testing.T) {
	store, mr := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
		TTL:       5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Refresh with a longer TTL
	if err := store.RefreshTTL(ctx, sess.ID, 30*time.Minute); err != nil {
		t.Fatalf("RefreshTTL failed: %v", err)
	}

	// Verify TTL was updated
	ttl := mr.TTL(store.sessionKey(sess.ID))
	if ttl < 25*time.Minute {
		t.Errorf("expected TTL > 25m, got %v", ttl)
	}

	// Verify session metadata was updated
	got, err := store.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got.ExpiresAt.Before(time.Now().Add(29 * time.Minute)) {
		t.Error("expected ExpiresAt to be updated")
	}
}

func TestRedisStore_RefreshTTL_Persist(t *testing.T) {
	store, mr := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
		TTL:       5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Refresh with zero TTL (persist)
	if err := store.RefreshTTL(ctx, sess.ID, 0); err != nil {
		t.Fatalf("RefreshTTL failed: %v", err)
	}

	// Verify key is persisted (no TTL)
	ttl := mr.TTL(store.sessionKey(sess.ID))
	if ttl != 0 {
		t.Errorf("expected no TTL after persist, got %v", ttl)
	}
}

func TestRedisStore_RefreshTTL_NotFound(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	err := store.RefreshTTL(context.Background(), "nonexistent", 10*time.Minute)
	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got: %v", err)
	}
}

func TestRedisStore_RefreshTTL_EmptyID(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	err := store.RefreshTTL(context.Background(), "", 10*time.Minute)
	if err != ErrInvalidSessionID {
		t.Errorf("expected ErrInvalidSessionID, got: %v", err)
	}
}

func TestRedisStore_UpdateSessionStats_Atomic(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// First update
	err = store.UpdateSessionStats(ctx, sess.ID, SessionStatsUpdate{
		AddInputTokens:  100,
		AddOutputTokens: 50,
		AddCostUSD:      0.01,
		AddToolCalls:    2,
		AddMessages:     1,
		SetStatus:       SessionStatusActive,
	})
	if err != nil {
		t.Fatalf("UpdateSessionStats failed: %v", err)
	}

	// Second update (should accumulate)
	err = store.UpdateSessionStats(ctx, sess.ID, SessionStatsUpdate{
		AddInputTokens:  200,
		AddOutputTokens: 100,
		AddCostUSD:      0.02,
		AddToolCalls:    3,
		AddMessages:     2,
	})
	if err != nil {
		t.Fatalf("UpdateSessionStats (2nd) failed: %v", err)
	}

	// Verify accumulated stats
	got, err := store.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if got.TotalInputTokens != 300 {
		t.Errorf("TotalInputTokens = %d, want 300", got.TotalInputTokens)
	}
	if got.TotalOutputTokens != 150 {
		t.Errorf("TotalOutputTokens = %d, want 150", got.TotalOutputTokens)
	}
	if got.ToolCallCount != 5 {
		t.Errorf("ToolCallCount = %d, want 5", got.ToolCallCount)
	}
	if got.MessageCount != 3 {
		t.Errorf("MessageCount = %d, want 3", got.MessageCount)
	}
	if got.Status != SessionStatusActive {
		t.Errorf("Status = %q, want %q", got.Status, SessionStatusActive)
	}
}

func TestRedisStore_UpdateSessionStats_NotFound(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	err := store.UpdateSessionStats(context.Background(), "nonexistent", SessionStatsUpdate{
		AddInputTokens: 10,
	})
	if err != ErrSessionNotFound {
		t.Errorf("expected ErrSessionNotFound, got: %v", err)
	}
}

func TestRedisStore_UpdateSessionStats_EmptyID(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	err := store.UpdateSessionStats(context.Background(), "", SessionStatsUpdate{})
	if err != ErrInvalidSessionID {
		t.Errorf("expected ErrInvalidSessionID, got: %v", err)
	}
}

func TestRedisStore_UpdateSessionStats_Expired(t *testing.T) {
	store, mr := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a session that is already expired by manually setting data
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
		TTL:       1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Manually override session data to have an expired expiresAt
	key := store.sessionKey(sess.ID)
	data, err := store.client.Get(ctx, key).Bytes()
	if err != nil {
		t.Fatalf("Get session data failed: %v", err)
	}

	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	s.ExpiresAt = time.Now().Add(-1 * time.Hour) // expired an hour ago
	updatedData, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if err := mr.Set(key, string(updatedData)); err != nil {
		t.Fatalf("mr.Set failed: %v", err)
	}

	err = store.UpdateSessionStats(ctx, sess.ID, SessionStatsUpdate{
		AddInputTokens: 10,
	})
	if err != ErrSessionExpired {
		t.Errorf("expected ErrSessionExpired, got: %v", err)
	}
}

func TestRedisStore_UpdateSessionStats_WithTTL(t *testing.T) {
	store, mr := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
		TTL:       10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	err = store.UpdateSessionStats(ctx, sess.ID, SessionStatsUpdate{
		AddInputTokens: 50,
	})
	if err != nil {
		t.Fatalf("UpdateSessionStats failed: %v", err)
	}

	// Verify TTL is preserved
	ttl := mr.TTL(store.sessionKey(sess.ID))
	if ttl == 0 {
		t.Error("expected TTL to be preserved after stats update")
	}

	// Verify stats were applied
	got, err := store.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got.TotalInputTokens != 50 {
		t.Errorf("TotalInputTokens = %d, want 50", got.TotalInputTokens)
	}
}

func TestRedisStore_UpdateTimestamp_Atomic(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	originalUpdatedAt := sess.UpdatedAt

	// Append a message which triggers updateSessionTimestamp
	time.Sleep(10 * time.Millisecond) // ensure time advances
	err = store.AppendMessage(ctx, sess.ID, Message{
		Role:    RoleUser,
		Content: "test message",
	})
	if err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}

	got, err := store.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if !got.UpdatedAt.After(originalUpdatedAt) {
		t.Error("expected UpdatedAt to advance after AppendMessage")
	}
}

func TestRedisStore_UpdateTimestamp_WithTTL(t *testing.T) {
	store, mr := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
		TTL:       10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Set state triggers updateSessionTimestamp
	err = store.SetState(ctx, sess.ID, "k", "v")
	if err != nil {
		t.Fatalf("SetState failed: %v", err)
	}

	// Verify TTL is preserved
	ttl := mr.TTL(store.sessionKey(sess.ID))
	if ttl == 0 {
		t.Error("expected TTL to be preserved after timestamp update")
	}
}

func TestRedisStore_ParseRedisURL(t *testing.T) {
	cfg, err := ParseRedisURL("redis://localhost:6379/1")
	if err != nil {
		t.Fatalf("ParseRedisURL failed: %v", err)
	}
	if cfg.Addr != "localhost:6379" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, "localhost:6379")
	}
	if cfg.DB != 1 {
		t.Errorf("DB = %d, want 1", cfg.DB)
	}
}

func TestRedisStore_ParseRedisURL_Invalid(t *testing.T) {
	_, err := ParseRedisURL("not-a-url")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestRedisStore_NewRedisStore_ConnectionFailure(t *testing.T) {
	_, err := NewRedisStore(RedisConfig{
		Addr: "localhost:1", // unlikely to have redis here
	})
	if err == nil {
		t.Error("expected error for connection failure")
	}
}

func TestRedisStore_SessionKey(t *testing.T) {
	store := &RedisStore{keyPrefix: "test:"}
	if got := store.sessionKey("abc"); got != "test:session:abc" {
		t.Errorf("sessionKey = %q, want %q", got, "test:session:abc")
	}
	if got := store.messagesKey("abc"); got != "test:session:abc:messages" {
		t.Errorf("messagesKey = %q, want %q", got, "test:session:abc:messages")
	}
	if got := store.stateKey("abc"); got != "test:session:abc:state" {
		t.Errorf("stateKey = %q, want %q", got, "test:session:abc:state")
	}
}

func TestRedisStore_CreateSessionWithTTLAndState(t *testing.T) {
	store, mr := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName:    "test-agent",
		Namespace:    "default",
		TTL:          10 * time.Minute,
		InitialState: map[string]string{"foo": "bar"},
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Verify state key also has TTL
	ttl := mr.TTL(store.stateKey(sess.ID))
	if ttl == 0 {
		t.Error("expected state key to have TTL")
	}
}

func TestRedisStore_UpdateSessionStats_SetStatus(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Update with status change
	err = store.UpdateSessionStats(ctx, sess.ID, SessionStatsUpdate{
		SetStatus: SessionStatusCompleted,
	})
	if err != nil {
		t.Fatalf("UpdateSessionStats failed: %v", err)
	}

	got, err := store.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got.Status != SessionStatusCompleted {
		t.Errorf("Status = %q, want %q", got.Status, SessionStatusCompleted)
	}
}

func TestRedisStore_RefreshTTL_SmallDuration(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Refresh with a very small TTL (less than 1 second)
	if err := store.RefreshTTL(ctx, sess.ID, 500*time.Millisecond); err != nil {
		t.Fatalf("RefreshTTL failed: %v", err)
	}

	// Should still succeed, min TTL is clamped to 1 second
	got, err := store.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got.ExpiresAt.IsZero() {
		t.Error("expected ExpiresAt to be set")
	}
}

func TestRedisStore_AppendMessageWithExistingIDAndTimestamp(t *testing.T) {
	store, _ := newTestRedisStore(t)
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	sess, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	msg := Message{
		ID:        "custom-id",
		Role:      RoleAssistant,
		Content:   "response",
		Timestamp: ts,
	}
	if err := store.AppendMessage(ctx, sess.ID, msg); err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}

	messages, err := store.GetMessages(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if messages[0].ID != "custom-id" {
		t.Errorf("message ID = %q, want %q", messages[0].ID, "custom-id")
	}
	if !messages[0].Timestamp.Equal(ts) {
		t.Errorf("message Timestamp = %v, want %v", messages[0].Timestamp, ts)
	}
}
