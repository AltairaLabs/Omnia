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

package redis

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// Ensure Provider satisfies HotCacheProvider at test-compilation time.
var _ providers.HotCacheProvider = (*Provider)(nil)

func setupTestProvider(t *testing.T) (*Provider, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	p := NewFromClient(client, DefaultOptions())
	return p, mr
}

func testSession() *session.Session {
	now := time.Now().Truncate(time.Second)
	return &session.Session{
		ID:                "sess-1",
		AgentName:         "agent-a",
		Namespace:         "ns-1",
		CreatedAt:         now,
		UpdatedAt:         now,
		Status:            session.SessionStatusActive,
		WorkspaceName:     "ws-1",
		MessageCount:      5,
		TotalInputTokens:  100,
		TotalOutputTokens: 200,
		Tags:              []string{"prod", "v2"},
	}
}

func testMessage(id string, seq int32) *session.Message {
	return &session.Message{
		ID:          id,
		Role:        session.RoleUser,
		Content:     "hello " + id,
		Timestamp:   time.Now().Truncate(time.Second),
		SequenceNum: seq,
	}
}

// ---------------------------------------------------------------------------
// GetSession / SetSession
// ---------------------------------------------------------------------------

func TestSetGetSession(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	s := testSession()
	if err := p.SetSession(ctx, s, 0); err != nil {
		t.Fatalf("SetSession: %v", err)
	}

	got, err := p.GetSession(ctx, s.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.ID != s.ID {
		t.Errorf("ID = %q, want %q", got.ID, s.ID)
	}
	if got.AgentName != s.AgentName {
		t.Errorf("AgentName = %q, want %q", got.AgentName, s.AgentName)
	}
	if got.Namespace != s.Namespace {
		t.Errorf("Namespace = %q, want %q", got.Namespace, s.Namespace)
	}
	if got.Status != s.Status {
		t.Errorf("Status = %q, want %q", got.Status, s.Status)
	}
	if got.WorkspaceName != s.WorkspaceName {
		t.Errorf("WorkspaceName = %q, want %q", got.WorkspaceName, s.WorkspaceName)
	}
	if got.MessageCount != s.MessageCount {
		t.Errorf("MessageCount = %d, want %d", got.MessageCount, s.MessageCount)
	}
	if got.TotalInputTokens != s.TotalInputTokens {
		t.Errorf("TotalInputTokens = %d, want %d", got.TotalInputTokens, s.TotalInputTokens)
	}
	if len(got.Tags) != len(s.Tags) {
		t.Errorf("Tags len = %d, want %d", len(got.Tags), len(s.Tags))
	}
	if !got.CreatedAt.Equal(s.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, s.CreatedAt)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	_, err := p.GetSession(ctx, "nonexistent")
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
}

func TestSetSession_Upsert(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	s := testSession()
	if err := p.SetSession(ctx, s, 0); err != nil {
		t.Fatalf("SetSession: %v", err)
	}

	s.AgentName = "agent-b"
	s.Status = session.SessionStatusCompleted
	if err := p.SetSession(ctx, s, 0); err != nil {
		t.Fatalf("SetSession upsert: %v", err)
	}

	got, err := p.GetSession(ctx, s.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.AgentName != "agent-b" {
		t.Errorf("AgentName = %q, want %q", got.AgentName, "agent-b")
	}
	if got.Status != session.SessionStatusCompleted {
		t.Errorf("Status = %q, want %q", got.Status, session.SessionStatusCompleted)
	}
}

func TestSetSession_WithTTL(t *testing.T) {
	p, mr := setupTestProvider(t)
	ctx := context.Background()

	s := testSession()
	if err := p.SetSession(ctx, s, 10*time.Second); err != nil {
		t.Fatalf("SetSession: %v", err)
	}

	// Should exist before TTL.
	if _, err := p.GetSession(ctx, s.ID); err != nil {
		t.Fatalf("GetSession before expiry: %v", err)
	}

	// Fast-forward past TTL.
	mr.FastForward(11 * time.Second)

	_, err := p.GetSession(ctx, s.ID)
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error after expiry = %v, want %v", err, session.ErrSessionNotFound)
	}
}

func TestSetSession_ZeroTTL(t *testing.T) {
	p, mr := setupTestProvider(t)
	ctx := context.Background()

	s := testSession()
	if err := p.SetSession(ctx, s, 0); err != nil {
		t.Fatalf("SetSession: %v", err)
	}

	// Should survive arbitrary time advance.
	mr.FastForward(24 * time.Hour)

	if _, err := p.GetSession(ctx, s.ID); err != nil {
		t.Fatalf("GetSession after 24h: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteSession
// ---------------------------------------------------------------------------

func TestDeleteSession(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	s := testSession()
	_ = p.SetSession(ctx, s, 0)
	_ = p.AppendMessage(ctx, s.ID, testMessage("m1", 1))

	if err := p.DeleteSession(ctx, s.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	_, err := p.GetSession(ctx, s.ID)
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("GetSession after delete = %v, want %v", err, session.ErrSessionNotFound)
	}

	// Messages should also be gone.
	msgs, err := p.GetRecentMessages(ctx, s.ID, 0)
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("GetRecentMessages after delete: err=%v, msgs=%d", err, len(msgs))
	}
}

func TestDeleteSession_NotFound(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	err := p.DeleteSession(ctx, "nonexistent")
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
}

// ---------------------------------------------------------------------------
// AppendMessage / GetRecentMessages
// ---------------------------------------------------------------------------

func TestAppendMessage(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	s := testSession()
	_ = p.SetSession(ctx, s, 0)

	msg := testMessage("m1", 1)
	if err := p.AppendMessage(ctx, s.ID, msg); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}

	msgs, err := p.GetRecentMessages(ctx, s.ID, 0)
	if err != nil {
		t.Fatalf("GetRecentMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("count = %d, want 1", len(msgs))
	}
	if msgs[0].ID != "m1" {
		t.Errorf("ID = %q, want %q", msgs[0].ID, "m1")
	}
	if msgs[0].Content != msg.Content {
		t.Errorf("Content = %q, want %q", msgs[0].Content, msg.Content)
	}
}

func TestAppendMessage_NotFound(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	err := p.AppendMessage(ctx, "nonexistent", testMessage("m1", 1))
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
}

func TestGetRecentMessages_Limit(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	s := testSession()
	_ = p.SetSession(ctx, s, 0)

	for i := 1; i <= 10; i++ {
		_ = p.AppendMessage(ctx, s.ID, testMessage(fmt.Sprintf("m%d", i), int32(i)))
	}

	msgs, err := p.GetRecentMessages(ctx, s.ID, 3)
	if err != nil {
		t.Fatalf("GetRecentMessages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("count = %d, want 3", len(msgs))
	}
	// Should be the last 3 (chronological order).
	if msgs[0].SequenceNum != 8 {
		t.Errorf("first seq = %d, want 8", msgs[0].SequenceNum)
	}
	if msgs[2].SequenceNum != 10 {
		t.Errorf("last seq = %d, want 10", msgs[2].SequenceNum)
	}
}

func TestGetRecentMessages_All(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	s := testSession()
	_ = p.SetSession(ctx, s, 0)

	for i := 1; i <= 5; i++ {
		_ = p.AppendMessage(ctx, s.ID, testMessage(fmt.Sprintf("m%d", i), int32(i)))
	}

	msgs, err := p.GetRecentMessages(ctx, s.ID, 0)
	if err != nil {
		t.Fatalf("GetRecentMessages: %v", err)
	}
	if len(msgs) != 5 {
		t.Errorf("count = %d, want 5", len(msgs))
	}
}

func TestGetRecentMessages_NotFound(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	_, err := p.GetRecentMessages(ctx, "nonexistent", 10)
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
}

func TestGetRecentMessages_Empty(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	s := testSession()
	_ = p.SetSession(ctx, s, 0)

	msgs, err := p.GetRecentMessages(ctx, s.ID, 0)
	if err != nil {
		t.Fatalf("GetRecentMessages: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("count = %d, want 0", len(msgs))
	}
}

func TestMaxMessagesPerSession(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	p := NewFromClient(client, Options{
		KeyPrefix:             defaultKeyPrefix,
		MaxMessagesPerSession: 5,
	})

	ctx := context.Background()
	s := testSession()
	_ = p.SetSession(ctx, s, 0)

	for i := 1; i <= 10; i++ {
		_ = p.AppendMessage(ctx, s.ID, testMessage(fmt.Sprintf("m%d", i), int32(i)))
	}

	msgs, err := p.GetRecentMessages(ctx, s.ID, 0)
	if err != nil {
		t.Fatalf("GetRecentMessages: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("count = %d, want 5 (capped)", len(msgs))
	}
	// Should keep the most recent 5.
	if msgs[0].SequenceNum != 6 {
		t.Errorf("first seq = %d, want 6", msgs[0].SequenceNum)
	}
	if msgs[4].SequenceNum != 10 {
		t.Errorf("last seq = %d, want 10", msgs[4].SequenceNum)
	}
}

// ---------------------------------------------------------------------------
// RefreshTTL
// ---------------------------------------------------------------------------

func TestRefreshTTL(t *testing.T) {
	p, mr := setupTestProvider(t)
	ctx := context.Background()

	s := testSession()
	_ = p.SetSession(ctx, s, 10*time.Second)

	if err := p.RefreshTTL(ctx, s.ID, 60*time.Second); err != nil {
		t.Fatalf("RefreshTTL: %v", err)
	}

	// Should survive past original TTL.
	mr.FastForward(15 * time.Second)

	if _, err := p.GetSession(ctx, s.ID); err != nil {
		t.Fatalf("GetSession after RefreshTTL: %v", err)
	}
}

func TestRefreshTTL_RemoveExpiry(t *testing.T) {
	p, mr := setupTestProvider(t)
	ctx := context.Background()

	s := testSession()
	_ = p.SetSession(ctx, s, 10*time.Second)

	// Remove expiry.
	if err := p.RefreshTTL(ctx, s.ID, 0); err != nil {
		t.Fatalf("RefreshTTL(0): %v", err)
	}

	mr.FastForward(24 * time.Hour)

	if _, err := p.GetSession(ctx, s.ID); err != nil {
		t.Fatalf("GetSession after PERSIST: %v", err)
	}
}

func TestRefreshTTL_NotFound(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	err := p.RefreshTTL(ctx, "nonexistent", time.Hour)
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
}

func TestRefreshTTL_BothKeys(t *testing.T) {
	p, mr := setupTestProvider(t)
	ctx := context.Background()

	s := testSession()
	_ = p.SetSession(ctx, s, 0)
	_ = p.AppendMessage(ctx, s.ID, testMessage("m1", 1))

	// Set TTL on both keys.
	if err := p.RefreshTTL(ctx, s.ID, 30*time.Second); err != nil {
		t.Fatalf("RefreshTTL: %v", err)
	}

	mr.FastForward(31 * time.Second)

	// Both session and messages should be gone.
	_, err := p.GetSession(ctx, s.ID)
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("session should be expired, got %v", err)
	}

	// Check messages key directly.
	n, _ := p.client.Exists(ctx, p.messagesKey(s.ID)).Result()
	if n != 0 {
		t.Error("messages key should be expired")
	}
}

// ---------------------------------------------------------------------------
// Invalidate
// ---------------------------------------------------------------------------

func TestInvalidate(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	s := testSession()
	_ = p.SetSession(ctx, s, 0)
	_ = p.AppendMessage(ctx, s.ID, testMessage("m1", 1))

	if err := p.Invalidate(ctx, s.ID); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}

	_, err := p.GetSession(ctx, s.ID)
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("GetSession after Invalidate = %v, want %v", err, session.ErrSessionNotFound)
	}
}

func TestInvalidate_Idempotent(t *testing.T) {
	p, _ := setupTestProvider(t)
	ctx := context.Background()

	// Should NOT error even if session doesn't exist.
	if err := p.Invalidate(ctx, "nonexistent"); err != nil {
		t.Errorf("Invalidate on nonexistent = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// Ping / Close
// ---------------------------------------------------------------------------

func TestPing(t *testing.T) {
	p, _ := setupTestProvider(t)
	if err := p.Ping(context.Background()); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

func TestClose_OwnsClient(t *testing.T) {
	mr := miniredis.RunT(t)
	p, err := New(Config{
		Addrs:     []string{mr.Addr()},
		KeyPrefix: defaultKeyPrefix,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Client should be closed — Ping should fail.
	if err := p.Ping(context.Background()); err == nil {
		t.Error("Ping after Close should fail")
	}
}

func TestClose_SharedClient(t *testing.T) {
	p, _ := setupTestProvider(t)

	// Close should be a no-op.
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Client should still be usable.
	if err := p.Ping(context.Background()); err != nil {
		t.Errorf("Ping after shared Close: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Key prefix
// ---------------------------------------------------------------------------

func TestKeyPrefix(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	p := NewFromClient(client, Options{KeyPrefix: "custom:"})
	ctx := context.Background()

	s := testSession()
	_ = p.SetSession(ctx, s, 0)

	// Verify the key in miniredis uses the custom prefix.
	keys := mr.Keys()
	if !slices.Contains(keys, "custom:session:{"+s.ID+"}") {
		t.Errorf("expected key with custom prefix, got keys: %v", keys)
	}
}

// ---------------------------------------------------------------------------
// Config defaults
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.KeyPrefix != defaultKeyPrefix {
		t.Errorf("KeyPrefix = %q, want %q", cfg.KeyPrefix, defaultKeyPrefix)
	}
	if cfg.MaxRetries != defaultMaxRetries {
		t.Errorf("MaxRetries = %d, want %d", cfg.MaxRetries, defaultMaxRetries)
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	if opts.KeyPrefix != defaultKeyPrefix {
		t.Errorf("KeyPrefix = %q, want %q", opts.KeyPrefix, defaultKeyPrefix)
	}
	if opts.MaxMessagesPerSession != 0 {
		t.Errorf("MaxMessagesPerSession = %d, want 0", opts.MaxMessagesPerSession)
	}
}

// ---------------------------------------------------------------------------
// New — connection error
// ---------------------------------------------------------------------------

func TestNew_ConnectionError(t *testing.T) {
	_, err := New(Config{
		Addrs:     []string{"localhost:1"}, // unlikely to have a Redis here
		KeyPrefix: defaultKeyPrefix,
	})
	if err == nil {
		t.Fatal("New with invalid addr should fail")
	}
}

// ---------------------------------------------------------------------------
// AppendMessage syncs TTL to messages key
// ---------------------------------------------------------------------------

func TestAppendMessage_SyncsTTL(t *testing.T) {
	p, mr := setupTestProvider(t)
	ctx := context.Background()

	s := testSession()
	_ = p.SetSession(ctx, s, 30*time.Second)

	_ = p.AppendMessage(ctx, s.ID, testMessage("m1", 1))

	// Both keys should expire around the same time.
	mr.FastForward(31 * time.Second)

	n, _ := p.client.Exists(ctx, p.messagesKey(s.ID)).Result()
	if n != 0 {
		t.Error("messages key should have expired with session TTL")
	}
}
