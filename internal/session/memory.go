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
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore implements Store using in-memory storage.
// This implementation is thread-safe and suitable for testing
// and single-instance development deployments.
type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	closed   bool
}

// NewMemoryStore creates a new in-memory session store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions: make(map[string]*Session),
	}
}

// CreateSession creates a new session and returns it.
func (m *MemoryStore) CreateSession(ctx context.Context, opts CreateSessionOptions) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, errors.New("store is closed")
	}

	now := time.Now()
	session := &Session{
		ID:        uuid.New().String(),
		AgentName: opts.AgentName,
		Namespace: opts.Namespace,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []Message{},
		State:     make(map[string]string),
	}

	if opts.TTL > 0 {
		session.ExpiresAt = now.Add(opts.TTL)
	}

	if opts.InitialState != nil {
		for k, v := range opts.InitialState {
			session.State[k] = v
		}
	}

	m.sessions[session.ID] = session

	// Return a copy to prevent external modification
	return m.copySession(session), nil
}

// GetSession retrieves a session by ID.
func (m *MemoryStore) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if sessionID == "" {
		return nil, ErrInvalidSessionID
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	if session.IsExpired() {
		return nil, ErrSessionExpired
	}

	return m.copySession(session), nil
}

// DeleteSession removes a session.
func (m *MemoryStore) DeleteSession(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if sessionID == "" {
		return ErrInvalidSessionID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[sessionID]; !exists {
		return ErrSessionNotFound
	}

	delete(m.sessions, sessionID)
	return nil
}

// AppendMessage adds a message to the session's conversation history.
func (m *MemoryStore) AppendMessage(ctx context.Context, sessionID string, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if sessionID == "" {
		return ErrInvalidSessionID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	if session.IsExpired() {
		return ErrSessionExpired
	}

	// Generate message ID if not provided
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}

	// Set timestamp if not provided
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	session.Messages = append(session.Messages, msg)
	session.UpdatedAt = time.Now()

	return nil
}

// GetMessages retrieves all messages for a session.
func (m *MemoryStore) GetMessages(ctx context.Context, sessionID string) ([]Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if sessionID == "" {
		return nil, ErrInvalidSessionID
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	if session.IsExpired() {
		return nil, ErrSessionExpired
	}

	// Return a copy of messages
	messages := make([]Message, len(session.Messages))
	copy(messages, session.Messages)
	return messages, nil
}

// SetState sets a state value for the session.
func (m *MemoryStore) SetState(ctx context.Context, sessionID string, key, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if sessionID == "" {
		return ErrInvalidSessionID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	if session.IsExpired() {
		return ErrSessionExpired
	}

	if session.State == nil {
		session.State = make(map[string]string)
	}

	session.State[key] = value
	session.UpdatedAt = time.Now()

	return nil
}

// GetState retrieves a state value from the session.
func (m *MemoryStore) GetState(ctx context.Context, sessionID string, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if sessionID == "" {
		return "", ErrInvalidSessionID
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return "", ErrSessionNotFound
	}

	if session.IsExpired() {
		return "", ErrSessionExpired
	}

	return session.State[key], nil
}

// RefreshTTL extends the session's expiration time.
func (m *MemoryStore) RefreshTTL(ctx context.Context, sessionID string, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if sessionID == "" {
		return ErrInvalidSessionID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	if ttl > 0 {
		session.ExpiresAt = time.Now().Add(ttl)
	} else {
		session.ExpiresAt = time.Time{} // No expiry
	}
	session.UpdatedAt = time.Now()

	return nil
}

// Close releases resources held by the store.
func (m *MemoryStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true
	m.sessions = nil
	return nil
}

// CleanupExpired removes all expired sessions.
// This method can be called periodically to free memory.
func (m *MemoryStore) CleanupExpired() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for id, session := range m.sessions {
		if session.IsExpired() {
			delete(m.sessions, id)
			count++
		}
	}
	return count
}

// Count returns the number of sessions in the store.
func (m *MemoryStore) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// copySession creates a deep copy of a session.
func (m *MemoryStore) copySession(s *Session) *Session {
	cp := &Session{
		ID:                 s.ID,
		AgentName:          s.AgentName,
		Namespace:          s.Namespace,
		CreatedAt:          s.CreatedAt,
		UpdatedAt:          s.UpdatedAt,
		ExpiresAt:          s.ExpiresAt,
		Messages:           make([]Message, len(s.Messages)),
		State:              make(map[string]string),
		WorkspaceName:      s.WorkspaceName,
		Status:             s.Status,
		EndedAt:            s.EndedAt,
		MessageCount:       s.MessageCount,
		ToolCallCount:      s.ToolCallCount,
		TotalInputTokens:   s.TotalInputTokens,
		TotalOutputTokens:  s.TotalOutputTokens,
		EstimatedCostUSD:   s.EstimatedCostUSD,
		LastMessagePreview: s.LastMessagePreview,
	}

	if len(s.Tags) > 0 {
		cp.Tags = make([]string, len(s.Tags))
		copy(cp.Tags, s.Tags)
	}

	for i, msg := range s.Messages {
		cp.Messages[i] = msg
		if msg.Metadata != nil {
			cp.Messages[i].Metadata = make(map[string]string)
			for k, v := range msg.Metadata {
				cp.Messages[i].Metadata[k] = v
			}
		}
	}

	for k, v := range s.State {
		cp.State[k] = v
	}

	return cp
}

// Ensure MemoryStore implements Store interface.
var _ Store = (*MemoryStore)(nil)
