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
	// ErrArtifactNotFound is returned when a requested artifact does not exist.
	ErrArtifactNotFound = errors.New("artifact not found")
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

// SessionStatus represents the lifecycle state of a session.
type SessionStatus string

const (
	// SessionStatusActive indicates the session is currently in use.
	SessionStatusActive SessionStatus = "active"
	// SessionStatusCompleted indicates the session ended normally.
	SessionStatusCompleted SessionStatus = "completed"
	// SessionStatusError indicates the session ended due to an error.
	SessionStatusError SessionStatus = "error"
	// SessionStatusExpired indicates the session was expired by TTL.
	SessionStatusExpired SessionStatus = "expired"
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
	// InputTokens is the number of input tokens consumed by this message.
	InputTokens int32 `json:"inputTokens,omitempty"`
	// OutputTokens is the number of output tokens produced by this message.
	OutputTokens int32 `json:"outputTokens,omitempty"`
	// ToolCallID links this message to a specific tool call.
	ToolCallID string `json:"toolCallId,omitempty"`
	// SequenceNum is the ordering position within the session.
	SequenceNum int32 `json:"sequenceNum,omitempty"`
}

// Artifact represents a binary attachment (image, audio, video, etc.) associated
// with a message. It maps to the message_artifacts table.
type Artifact struct {
	// ID is the unique identifier for this artifact.
	ID string `json:"id"`
	// MessageID links this artifact to its parent message.
	MessageID string `json:"messageId"`
	// SessionID links this artifact to its parent session.
	SessionID string `json:"sessionId"`
	// Type is the artifact category: image, audio, video, document, file.
	Type string `json:"type"`
	// MIMEType is the MIME type of the binary data.
	MIMEType string `json:"mimeType"`
	// StorageURI is the URI to the binary data in cold storage.
	StorageURI string `json:"storageUri"`
	// SizeBytes is the size of the binary data in bytes.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
	// Filename is the original filename, if known.
	Filename string `json:"filename,omitempty"`
	// Checksum is a SHA-256 integrity checksum.
	Checksum string `json:"checksum,omitempty"`
	// Metadata contains optional additional data.
	Metadata map[string]string `json:"metadata,omitempty"`
	// CreatedAt is when the artifact was created.
	CreatedAt time.Time `json:"createdAt"`
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
	// WorkspaceName is the workspace this session belongs to.
	WorkspaceName string `json:"workspaceName,omitempty"`
	// Status is the lifecycle state of the session.
	Status SessionStatus `json:"status,omitempty"`
	// EndedAt is when the session ended (zero means still active).
	EndedAt time.Time `json:"endedAt,omitempty"`
	// MessageCount is the total number of messages in the session.
	MessageCount int32 `json:"messageCount,omitempty"`
	// ToolCallCount is the total number of tool calls in the session.
	ToolCallCount int32 `json:"toolCallCount,omitempty"`
	// TotalInputTokens is the cumulative input token count.
	TotalInputTokens int64 `json:"totalInputTokens,omitempty"`
	// TotalOutputTokens is the cumulative output token count.
	TotalOutputTokens int64 `json:"totalOutputTokens,omitempty"`
	// EstimatedCostUSD is the estimated cost of the session in USD.
	EstimatedCostUSD float64 `json:"estimatedCostUSD,omitempty"`
	// Tags contains arbitrary labels for categorization and filtering.
	Tags []string `json:"tags,omitempty"`
	// LastMessagePreview is a truncated preview of the last message.
	LastMessagePreview string `json:"lastMessagePreview,omitempty"`
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
	// WorkspaceName is the workspace this session belongs to (used for filtering in the dashboard).
	WorkspaceName string
	// TTL is the time-to-live for the session (zero means no expiry).
	TTL time.Duration
	// InitialState is optional initial state.
	InitialState map[string]string
}

// SessionStatsUpdate contains incremental updates to session-level counters.
// All Add* fields are added to the current values; SetStatus is applied only if non-empty.
type SessionStatsUpdate struct {
	AddInputTokens  int32
	AddOutputTokens int32
	AddCostUSD      float64
	AddToolCalls    int32
	AddMessages     int32
	SetStatus       SessionStatus // empty means no change
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

	// UpdateSessionStats atomically increments session-level counters.
	// Returns ErrSessionNotFound if the session does not exist.
	// Returns ErrSessionExpired if the session has expired.
	UpdateSessionStats(ctx context.Context, sessionID string, update SessionStatsUpdate) error

	// Close releases any resources held by the store.
	Close() error
}
