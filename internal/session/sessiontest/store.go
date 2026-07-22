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

// Package sessiontest provides a test double for session.Store.
//
// It is deliberately its own package so misuse is visible at the import line: a
// production file importing "sessiontest" is obviously wrong in review and
// trivially greppable in CI. It previously lived in package session as
// MemoryStore, where nothing distinguished it from a real implementation — and
// the agent wired it in as a fallback when session-api could not be discovered,
// producing an archive that silently discarded every write.
package sessiontest

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	sessionpkg "github.com/altairalabs/omnia/internal/session"
)

// The implementation below is the original, unchanged. These aliases let it
// keep referring to the session types unqualified, so this file stays a
// verbatim move rather than a rewrite.
type (
	Session                = sessionpkg.Session
	SessionRecordOptions   = sessionpkg.SessionRecordOptions
	DecorateSessionOptions = sessionpkg.DecorateSessionOptions
	SessionStatusUpdate    = sessionpkg.SessionStatusUpdate
	Message                = sessionpkg.Message
	ToolCall               = sessionpkg.ToolCall
	ProviderCall           = sessionpkg.ProviderCall
	RuntimeEvent           = sessionpkg.RuntimeEvent
	EvalResult             = sessionpkg.EvalResult
)

const (
	ToolCallStatusPending       = sessionpkg.ToolCallStatusPending
	ProviderCallStatusCompleted = sessionpkg.ProviderCallStatusCompleted
)

var (
	ErrSessionNotFound  = sessionpkg.ErrSessionNotFound
	ErrSessionExpired   = sessionpkg.ErrSessionExpired
	ErrInvalidSessionID = sessionpkg.ErrInvalidSessionID
)

// Store implements session.Store in memory, for tests. It is thread-safe.
//
// It is NOT a deployment option. Session data is read back through session-api
// — the dashboard queries that, not an agent's process — so an agent holding
// this store records into a map nothing ever reads. That was the failure mode
// behind #1223: the agent looked healthy while every session, token and cost
// figure went missing.
type Store struct {
	mu            sync.RWMutex
	sessions      map[string]*Session
	closed        bool
	toolCalls     map[string][]ToolCall     // keyed by sessionID
	providerCalls map[string][]ProviderCall // keyed by sessionID
	runtimeEvents map[string][]RuntimeEvent // keyed by sessionID
}

// NewStore creates a new in-memory session store.
func NewStore() *Store {
	return &Store{
		sessions:      make(map[string]*Session),
		toolCalls:     make(map[string][]ToolCall),
		providerCalls: make(map[string][]ProviderCall),
		runtimeEvents: make(map[string][]RuntimeEvent),
	}
}

// CreateSession creates a new session and returns it.
func (m *Store) EnsureSessionRecord(ctx context.Context, opts SessionRecordOptions) (*Session, error) {
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
		ID:            id,
		AgentName:     opts.AgentName,
		Namespace:     opts.Namespace,
		CreatedAt:     now,
		UpdatedAt:     now,
		Messages:      []Message{},
		State:         make(map[string]string),
		CohortID:      opts.CohortID,
		Variant:       opts.Variant,
		VirtualUserID: opts.VirtualUserID,
	}

	if opts.TTL > 0 {
		session.ExpiresAt = now.Add(opts.TTL)
	}

	if opts.InitialState != nil {
		for k, v := range opts.InitialState {
			session.State[k] = v
		}
	}

	if len(opts.Tags) > 0 {
		session.Tags = append([]string(nil), opts.Tags...)
	}

	m.sessions[session.ID] = session

	// Return a copy to prevent external modification
	return m.copySession(session), nil
}

// GetSession retrieves a session by ID.
func (m *Store) GetSession(ctx context.Context, sessionID string) (*Session, error) {
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
func (m *Store) DeleteSession(ctx context.Context, sessionID string) error {
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
func (m *Store) AppendMessage(ctx context.Context, sessionID string, msg Message) error {
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
	// Only increment message_count here. Token/cost counters are derived from
	// RecordProviderCall; tool_call_count from RecordToolCall.
	if msg.ToolCallID == "" {
		session.MessageCount++
	}
	session.UpdatedAt = time.Now()

	return nil
}

// GetMessages retrieves all messages for a session.
func (m *Store) GetMessages(ctx context.Context, sessionID string) ([]Message, error) {
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

// RefreshTTL extends the session's expiration time.
func (m *Store) RefreshTTL(ctx context.Context, sessionID string, ttl time.Duration) error {
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
func (m *Store) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true
	m.sessions = nil
	m.toolCalls = nil
	m.providerCalls = nil
	m.runtimeEvents = nil
	return nil
}

// UpdateSessionStatus atomically increments session-level counters.
func (m *Store) UpdateSessionStatus(ctx context.Context, sessionID string, update SessionStatusUpdate) error {
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

	if update.SetStatus != "" && !sessionpkg.IsTerminalStatus(session.Status) {
		session.Status = update.SetStatus
	}

	if !update.SetEndedAt.IsZero() {
		session.EndedAt = update.SetEndedAt
	}

	session.UpdatedAt = time.Now()

	return nil
}

// DecorateSession merges tags and state into an existing session without
// touching counters or lifecycle status. Tag merges are idempotent.
func (m *Store) DecorateSession(ctx context.Context, sessionID string, opts DecorateSessionOptions) error {
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

	session.Tags = mergeSessionTags(session.Tags, opts.RemoveTags, opts.AddTags)

	if len(opts.MergeState) > 0 {
		if session.State == nil {
			session.State = make(map[string]string, len(opts.MergeState))
		}
		for k, v := range opts.MergeState {
			session.State[k] = v
		}
	}

	session.UpdatedAt = time.Now()

	return nil
}

// mergeSessionTags drops removeTags from existing, then appends addTags that are
// not already present (deduped, order-preserving).
func mergeSessionTags(existing, removeTags, addTags []string) []string {
	remove := make(map[string]struct{}, len(removeTags))
	for _, t := range removeTags {
		remove[t] = struct{}{}
	}
	seen := make(map[string]struct{}, len(existing))
	kept := existing[:0:0]
	for _, t := range existing {
		if _, drop := remove[t]; drop {
			continue
		}
		kept = append(kept, t)
		seen[t] = struct{}{}
	}
	for _, t := range addTags {
		if _, ok := seen[t]; ok {
			continue
		}
		kept = append(kept, t)
		seen[t] = struct{}{}
	}
	return kept
}

// RecordToolCall records or updates a tool call for the session.
func (m *Store) RecordToolCall(ctx context.Context, sessionID string, tc ToolCall) error {
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

	// Each lifecycle event is a separate row. Only "pending" (the initial start)
	// increments the tool call count.
	m.toolCalls[sessionID] = append(m.toolCalls[sessionID], tc)
	if tc.Status == ToolCallStatusPending {
		session.ToolCallCount++
	}
	session.UpdatedAt = time.Now()

	return nil
}

// RecordProviderCall records or updates a provider call for the session.
func (m *Store) RecordProviderCall(ctx context.Context, sessionID string, pc ProviderCall) error {
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

	// Each lifecycle event is a separate row. Only completed calls add
	// tokens/cost to session counters.
	m.providerCalls[sessionID] = append(m.providerCalls[sessionID], pc)
	if pc.Status == ProviderCallStatusCompleted {
		session.TotalInputTokens += pc.InputTokens
		session.TotalOutputTokens += pc.OutputTokens
		session.EstimatedCostUSD += pc.CostUSD
	}
	session.UpdatedAt = time.Now()

	return nil
}

// GetToolCalls retrieves tool calls for a session ordered by created_at.
func (m *Store) GetToolCalls(ctx context.Context, sessionID string, limit, offset int) ([]ToolCall, error) {
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
	return paginateSlice(result, limit, offset), nil
}

// GetProviderCalls retrieves provider calls for a session ordered by created_at.
func (m *Store) GetProviderCalls(ctx context.Context, sessionID string, limit, offset int) ([]ProviderCall, error) {
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
	return paginateSlice(result, limit, offset), nil
}

// RecordEvalResult records an eval result (no-op storage in memory store — eval results
// are persisted via the session-api eval_results table in production).
func (m *Store) RecordEvalResult(_ context.Context, _ string, _ EvalResult) error {
	return nil
}

// RecordRuntimeEvent records a runtime lifecycle event for the session.
func (m *Store) RecordRuntimeEvent(ctx context.Context, sessionID string, evt RuntimeEvent) error {
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

// GetRuntimeEvents retrieves runtime events for a session ordered by timestamp.
func (m *Store) GetRuntimeEvents(ctx context.Context, sessionID string, limit, offset int) ([]RuntimeEvent, error) {
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
	return paginateSlice(result, limit, offset), nil
}

// paginateSlice applies offset and limit to a slice.
// A zero limit returns all items after the offset.
func paginateSlice[T any](items []T, limit, offset int) []T {
	if offset > 0 {
		if offset >= len(items) {
			return items[:0]
		}
		items = items[offset:]
	}
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	return items
}

// CleanupExpired removes all expired sessions.
// This method can be called periodically to free memory.
func (m *Store) CleanupExpired() int {
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
func (m *Store) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// copySession creates a deep copy of a session.
func (m *Store) copySession(s *Session) *Session {
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
		PromptPackName:     s.PromptPackName,
		PromptPackVersion:  s.PromptPackVersion,
		CohortID:           s.CohortID,
		Variant:            s.Variant,
		VirtualUserID:      s.VirtualUserID,
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

// Ensure Store satisfies the session.Store interface.
var _ sessionpkg.Store = (*Store)(nil)
