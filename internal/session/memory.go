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
	mu            sync.RWMutex
	sessions      map[string]*Session
	closed        bool
	toolCalls     map[string][]ToolCall     // keyed by sessionID
	providerCalls map[string][]ProviderCall // keyed by sessionID
	runtimeEvents map[string][]RuntimeEvent // keyed by sessionID
}

// NewMemoryStore creates a new in-memory session store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions:      make(map[string]*Session),
		toolCalls:     make(map[string][]ToolCall),
		providerCalls: make(map[string][]ProviderCall),
		runtimeEvents: make(map[string][]RuntimeEvent),
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
	id := opts.ID
	if id == "" {
		id = uuid.New().String()
	}
	session := &Session{
		ID:        id,
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
	delete(m.toolCalls, sessionID)
	delete(m.providerCalls, sessionID)
	delete(m.runtimeEvents, sessionID)
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
	session.MessageCount++
	if msg.ToolCallID != "" {
		session.ToolCallCount++
	}
	session.TotalInputTokens += int64(msg.InputTokens)
	session.TotalOutputTokens += int64(msg.OutputTokens)
	session.EstimatedCostUSD += msg.CostUSD
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
	m.toolCalls = nil
	m.providerCalls = nil
	m.runtimeEvents = nil
	return nil
}

// UpdateSessionStats atomically increments session-level counters.
func (m *MemoryStore) UpdateSessionStats(ctx context.Context, sessionID string, update SessionStatsUpdate) error {
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

	if update.SetStatus != "" && !isTerminalStatus(session.Status) {
		session.Status = update.SetStatus
	}

	if !update.SetEndedAt.IsZero() {
		session.EndedAt = update.SetEndedAt
	}

	session.UpdatedAt = time.Now()

	return nil
}

// RecordToolCall records or updates a tool call for the session.
func (m *MemoryStore) RecordToolCall(ctx context.Context, sessionID string, tc ToolCall) error {
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

	// Upsert: find existing by ID and replace, or append.
	calls := m.toolCalls[sessionID]
	found := false
	for i, existing := range calls {
		if existing.ID == tc.ID {
			calls[i] = tc
			found = true
			break
		}
	}
	if !found {
		calls = append(calls, tc)
		// Increment tool call count on initial insert.
		session.ToolCallCount++
	}
	m.toolCalls[sessionID] = calls
	session.UpdatedAt = time.Now()

	return nil
}

// RecordProviderCall records or updates a provider call for the session.
func (m *MemoryStore) RecordProviderCall(ctx context.Context, sessionID string, pc ProviderCall) error {
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

	// Upsert: find existing by ID and replace, or append.
	calls := m.providerCalls[sessionID]
	found := false
	for i, existing := range calls {
		if existing.ID == pc.ID {
			// On completion update, apply token/cost deltas to session.
			if existing.Status == ProviderCallStatusPending && pc.Status == ProviderCallStatusCompleted {
				session.TotalInputTokens += pc.InputTokens
				session.TotalOutputTokens += pc.OutputTokens
				session.EstimatedCostUSD += pc.CostUSD
			}
			calls[i] = pc
			found = true
			break
		}
	}
	if !found {
		calls = append(calls, pc)
	}
	m.providerCalls[sessionID] = calls
	session.UpdatedAt = time.Now()

	return nil
}

// GetToolCalls retrieves all tool calls for a session ordered by created_at.
func (m *MemoryStore) GetToolCalls(ctx context.Context, sessionID string) ([]ToolCall, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if sessionID == "" {
		return nil, ErrInvalidSessionID
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, exists := m.sessions[sessionID]; !exists {
		return nil, ErrSessionNotFound
	}

	calls := m.toolCalls[sessionID]
	result := make([]ToolCall, len(calls))
	copy(result, calls)
	return result, nil
}

// GetProviderCalls retrieves all provider calls for a session ordered by created_at.
func (m *MemoryStore) GetProviderCalls(ctx context.Context, sessionID string) ([]ProviderCall, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if sessionID == "" {
		return nil, ErrInvalidSessionID
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, exists := m.sessions[sessionID]; !exists {
		return nil, ErrSessionNotFound
	}

	calls := m.providerCalls[sessionID]
	result := make([]ProviderCall, len(calls))
	copy(result, calls)
	return result, nil
}

// RecordEvalResult records an eval result (no-op storage in memory store — eval results
// are persisted via the session-api eval_results table in production).
func (m *MemoryStore) RecordEvalResult(_ context.Context, _ string, _ EvalResult) error {
	return nil
}

// RecordRuntimeEvent records a runtime lifecycle event for the session.
func (m *MemoryStore) RecordRuntimeEvent(ctx context.Context, sessionID string, evt RuntimeEvent) error {
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

	if evt.ID == "" {
		evt.ID = uuid.New().String()
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}

	m.runtimeEvents[sessionID] = append(m.runtimeEvents[sessionID], evt)
	return nil
}

// GetRuntimeEvents retrieves all runtime events for a session ordered by timestamp.
func (m *MemoryStore) GetRuntimeEvents(ctx context.Context, sessionID string) ([]RuntimeEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if sessionID == "" {
		return nil, ErrInvalidSessionID
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, exists := m.sessions[sessionID]; !exists {
		return nil, ErrSessionNotFound
	}

	events := m.runtimeEvents[sessionID]
	result := make([]RuntimeEvent, len(events))
	copy(result, events)
	return result, nil
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
