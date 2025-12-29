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
	"testing"
	"time"
)

const (
	testAgentName = "test-agent"
	testNamespace = "test-namespace"
	testStateKey  = "key1"
	testStateVal  = "value1"
)

func TestMemoryStoreCreateSession(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	session, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: testAgentName,
		Namespace: testNamespace,
	})

	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if session.ID == "" {
		t.Error("Session ID should not be empty")
	}

	if session.AgentName != testAgentName {
		t.Errorf("AgentName = %v, want %v", session.AgentName, testAgentName)
	}

	if session.Namespace != testNamespace {
		t.Errorf("Namespace = %v, want %v", session.Namespace, testNamespace)
	}

	if session.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}

	if len(session.Messages) != 0 {
		t.Errorf("Messages should be empty, got %d", len(session.Messages))
	}
}

func TestMemoryStoreCreateSessionWithTTL(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	ttl := 1 * time.Hour

	session, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: testAgentName,
		Namespace: testNamespace,
		TTL:       ttl,
	})

	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if session.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should not be zero when TTL is set")
	}

	expectedExpiry := session.CreatedAt.Add(ttl)
	if session.ExpiresAt.Before(expectedExpiry.Add(-time.Second)) || session.ExpiresAt.After(expectedExpiry.Add(time.Second)) {
		t.Errorf("ExpiresAt = %v, want approximately %v", session.ExpiresAt, expectedExpiry)
	}
}

func TestMemoryStoreCreateSessionWithInitialState(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	initialState := map[string]string{
		testStateKey: testStateVal,
		"key2":       "value2",
	}

	session, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName:    testAgentName,
		Namespace:    testNamespace,
		InitialState: initialState,
	})

	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if len(session.State) != 2 {
		t.Errorf("State length = %d, want 2", len(session.State))
	}

	if session.State[testStateKey] != testStateVal {
		t.Errorf("State[%s] = %v, want %v", testStateKey, session.State[testStateKey], testStateVal)
	}
}

func TestMemoryStoreGetSession(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	created, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: testAgentName,
		Namespace: testNamespace,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	retrieved, err := store.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if retrieved.ID != created.ID {
		t.Errorf("Retrieved ID = %v, want %v", retrieved.ID, created.ID)
	}

	if retrieved.AgentName != created.AgentName {
		t.Errorf("Retrieved AgentName = %v, want %v", retrieved.AgentName, created.AgentName)
	}
}

func TestMemoryStoreGetSessionNotFound(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	_, err := store.GetSession(ctx, "non-existent-id")
	if err != ErrSessionNotFound {
		t.Errorf("GetSession error = %v, want %v", err, ErrSessionNotFound)
	}
}

func TestMemoryStoreGetSessionInvalidID(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	_, err := store.GetSession(ctx, "")
	if err != ErrInvalidSessionID {
		t.Errorf("GetSession error = %v, want %v", err, ErrInvalidSessionID)
	}
}

func TestMemoryStoreGetSessionExpired(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	session, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: testAgentName,
		Namespace: testNamespace,
		TTL:       1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Wait for session to expire
	time.Sleep(10 * time.Millisecond)

	_, err = store.GetSession(ctx, session.ID)
	if err != ErrSessionExpired {
		t.Errorf("GetSession error = %v, want %v", err, ErrSessionExpired)
	}
}

func TestMemoryStoreDeleteSession(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	session, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: testAgentName,
		Namespace: testNamespace,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	err = store.DeleteSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	_, err = store.GetSession(ctx, session.ID)
	if err != ErrSessionNotFound {
		t.Errorf("GetSession after delete error = %v, want %v", err, ErrSessionNotFound)
	}
}

func TestMemoryStoreDeleteSessionNotFound(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	err := store.DeleteSession(ctx, "non-existent-id")
	if err != ErrSessionNotFound {
		t.Errorf("DeleteSession error = %v, want %v", err, ErrSessionNotFound)
	}
}

func TestMemoryStoreAppendMessage(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	session, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: testAgentName,
		Namespace: testNamespace,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	msg := Message{
		Role:    RoleUser,
		Content: "Hello, world!",
	}

	err = store.AppendMessage(ctx, session.ID, msg)
	if err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}

	messages, err := store.GetMessages(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Messages length = %d, want 1", len(messages))
	}

	if messages[0].Content != msg.Content {
		t.Errorf("Message content = %v, want %v", messages[0].Content, msg.Content)
	}

	if messages[0].Role != msg.Role {
		t.Errorf("Message role = %v, want %v", messages[0].Role, msg.Role)
	}

	if messages[0].ID == "" {
		t.Error("Message ID should be auto-generated")
	}

	if messages[0].Timestamp.IsZero() {
		t.Error("Message timestamp should be auto-set")
	}
}

func TestMemoryStoreAppendMessageNotFound(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	err := store.AppendMessage(ctx, "non-existent-id", Message{
		Role:    RoleUser,
		Content: "Hello",
	})
	if err != ErrSessionNotFound {
		t.Errorf("AppendMessage error = %v, want %v", err, ErrSessionNotFound)
	}
}

func TestMemoryStoreGetMessages(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	session, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: testAgentName,
		Namespace: testNamespace,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	messages := []Message{
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAssistant, Content: "Hi there!"},
		{Role: RoleUser, Content: "How are you?"},
	}

	for _, msg := range messages {
		if err := store.AppendMessage(ctx, session.ID, msg); err != nil {
			t.Fatalf("AppendMessage failed: %v", err)
		}
	}

	retrieved, err := store.GetMessages(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if len(retrieved) != len(messages) {
		t.Fatalf("Retrieved messages length = %d, want %d", len(retrieved), len(messages))
	}

	for i, msg := range retrieved {
		if msg.Content != messages[i].Content {
			t.Errorf("Message[%d] content = %v, want %v", i, msg.Content, messages[i].Content)
		}
	}
}

func TestMemoryStoreSetState(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	session, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: testAgentName,
		Namespace: testNamespace,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	err = store.SetState(ctx, session.ID, testStateKey, testStateVal)
	if err != nil {
		t.Fatalf("SetState failed: %v", err)
	}

	value, err := store.GetState(ctx, session.ID, testStateKey)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}

	if value != testStateVal {
		t.Errorf("State value = %v, want %v", value, testStateVal)
	}
}

func TestMemoryStoreGetStateNotFound(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	session, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: testAgentName,
		Namespace: testNamespace,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	value, err := store.GetState(ctx, session.ID, "non-existent-key")
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}

	if value != "" {
		t.Errorf("State value = %v, want empty string", value)
	}
}

func TestMemoryStoreRefreshTTL(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	session, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: testAgentName,
		Namespace: testNamespace,
		TTL:       1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	originalExpiry := session.ExpiresAt

	// Wait a bit and refresh with a longer TTL
	time.Sleep(10 * time.Millisecond)

	newTTL := 2 * time.Hour
	err = store.RefreshTTL(ctx, session.ID, newTTL)
	if err != nil {
		t.Fatalf("RefreshTTL failed: %v", err)
	}

	updated, err := store.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if !updated.ExpiresAt.After(originalExpiry) {
		t.Error("ExpiresAt should be extended after RefreshTTL")
	}
}

func TestMemoryStoreRefreshTTLRemoveExpiry(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	session, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: testAgentName,
		Namespace: testNamespace,
		TTL:       1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Refresh with zero TTL to remove expiry
	err = store.RefreshTTL(ctx, session.ID, 0)
	if err != nil {
		t.Fatalf("RefreshTTL failed: %v", err)
	}

	updated, err := store.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if !updated.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be zero after RefreshTTL with 0 duration")
	}
}

func TestMemoryStoreCleanupExpired(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// Create a session that expires quickly
	_, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: testAgentName,
		Namespace: testNamespace,
		TTL:       1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Create a session that doesn't expire
	_, err = store.CreateSession(ctx, CreateSessionOptions{
		AgentName: testAgentName,
		Namespace: testNamespace,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if store.Count() != 2 {
		t.Fatalf("Count = %d, want 2", store.Count())
	}

	// Wait for first session to expire
	time.Sleep(10 * time.Millisecond)

	cleaned := store.CleanupExpired()
	if cleaned != 1 {
		t.Errorf("CleanupExpired = %d, want 1", cleaned)
	}

	if store.Count() != 1 {
		t.Errorf("Count after cleanup = %d, want 1", store.Count())
	}
}

func TestMemoryStoreClose(t *testing.T) {
	store := NewMemoryStore()

	ctx := context.Background()

	_, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: testAgentName,
		Namespace: testNamespace,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	err = store.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Operations after close should fail
	_, err = store.CreateSession(ctx, CreateSessionOptions{
		AgentName: testAgentName,
		Namespace: testNamespace,
	})
	if err == nil {
		t.Error("CreateSession should fail after Close")
	}
}

func TestMemoryStoreConcurrentAccess(t *testing.T) {
	store := NewMemoryStore()
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	session, err := store.CreateSession(ctx, CreateSessionOptions{
		AgentName: testAgentName,
		Namespace: testNamespace,
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = store.AppendMessage(ctx, session.ID, Message{
					Role:    RoleUser,
					Content: "test message",
				})
				_, _ = store.GetMessages(ctx, session.ID)
				_ = store.SetState(ctx, session.ID, "key", "value")
				_, _ = store.GetState(ctx, session.ID, "key")
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify session is still valid
	_, err = store.GetSession(ctx, session.ID)
	if err != nil {
		t.Errorf("GetSession after concurrent access failed: %v", err)
	}
}

func TestSessionIsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "zero expiry",
			expiresAt: time.Time{},
			want:      false,
		},
		{
			name:      "future expiry",
			expiresAt: time.Now().Add(1 * time.Hour),
			want:      false,
		},
		{
			name:      "past expiry",
			expiresAt: time.Now().Add(-1 * time.Hour),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{ExpiresAt: tt.expiresAt}
			if got := s.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMessageRoleConstants(t *testing.T) {
	tests := []struct {
		role     MessageRole
		expected string
	}{
		{RoleUser, "user"},
		{RoleAssistant, "assistant"},
		{RoleSystem, "system"},
	}

	for _, tt := range tests {
		if string(tt.role) != tt.expected {
			t.Errorf("MessageRole = %v, want %v", tt.role, tt.expected)
		}
	}
}
