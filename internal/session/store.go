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

// Package session provides session storage for agent conversations.
package session

import (
	"context"
	"errors"
	"time"
)

// Common errors returned by session store implementations.
var (
	// ErrSessionNotFound is returned when a session does not exist.
	ErrSessionNotFound = errors.New("session not found")
	// ErrSessionExpired is returned when a session has expired.
	ErrSessionExpired = errors.New("session expired")
	// ErrInvalidSessionID is returned when a session ID is invalid.
	ErrInvalidSessionID = errors.New("invalid session ID")
)

// MessageRole represents the role of a message sender.
type MessageRole string

const (
	// RoleUser represents a user message.
	RoleUser MessageRole = "user"
	// RoleAssistant represents an assistant/agent message.
	RoleAssistant MessageRole = "assistant"
	// RoleSystem represents a system message.
	RoleSystem MessageRole = "system"
)

// Message represents a single message in a conversation.
type Message struct {
	// ID is the unique identifier for this message.
	ID string `json:"id"`
	// Role indicates who sent the message.
	Role MessageRole `json:"role"`
	// Content is the message content.
	Content string `json:"content"`
	// Timestamp is when the message was created.
	Timestamp time.Time `json:"timestamp"`
	// Metadata contains optional additional data.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Session represents an agent conversation session.
type Session struct {
	// ID is the unique session identifier.
	ID string `json:"id"`
	// AgentName is the name of the agent this session belongs to.
	AgentName string `json:"agentName"`
	// Namespace is the Kubernetes namespace of the agent.
	Namespace string `json:"namespace"`
	// CreatedAt is when the session was created.
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt is when the session was last updated.
	UpdatedAt time.Time `json:"updatedAt"`
	// ExpiresAt is when the session will expire (zero means no expiry).
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
	// Messages contains the conversation history.
	Messages []Message `json:"messages"`
	// State contains arbitrary session state.
	State map[string]string `json:"state,omitempty"`
}

// IsExpired returns true if the session has expired.
func (s *Session) IsExpired() bool {
	if s.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(s.ExpiresAt)
}

// CreateSessionOptions contains options for creating a new session.
type CreateSessionOptions struct {
	// AgentName is the name of the agent.
	AgentName string
	// Namespace is the Kubernetes namespace.
	Namespace string
	// TTL is the time-to-live for the session (zero means no expiry).
	TTL time.Duration
	// InitialState is optional initial state.
	InitialState map[string]string
}

// Store defines the interface for session storage.
type Store interface {
	// CreateSession creates a new session and returns its ID.
	CreateSession(ctx context.Context, opts CreateSessionOptions) (*Session, error)

	// GetSession retrieves a session by ID.
	// Returns ErrSessionNotFound if the session does not exist.
	// Returns ErrSessionExpired if the session has expired.
	GetSession(ctx context.Context, sessionID string) (*Session, error)

	// DeleteSession removes a session.
	// Returns ErrSessionNotFound if the session does not exist.
	DeleteSession(ctx context.Context, sessionID string) error

	// AppendMessage adds a message to the session's conversation history.
	// Returns ErrSessionNotFound if the session does not exist.
	AppendMessage(ctx context.Context, sessionID string, msg Message) error

	// GetMessages retrieves all messages for a session.
	// Returns ErrSessionNotFound if the session does not exist.
	GetMessages(ctx context.Context, sessionID string) ([]Message, error)

	// SetState sets a state value for the session.
	// Returns ErrSessionNotFound if the session does not exist.
	SetState(ctx context.Context, sessionID string, key, value string) error

	// GetState retrieves a state value from the session.
	// Returns empty string if the key does not exist.
	// Returns ErrSessionNotFound if the session does not exist.
	GetState(ctx context.Context, sessionID string, key string) (string, error)

	// RefreshTTL extends the session's expiration time.
	// Returns ErrSessionNotFound if the session does not exist.
	RefreshTTL(ctx context.Context, sessionID string, ttl time.Duration) error

	// Close releases any resources held by the store.
	Close() error
}
