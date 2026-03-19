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
	"encoding/json"
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

// isTerminalStatus returns true if the status is a terminal state
// that should not be overwritten by subsequent transitions.
func isTerminalStatus(s SessionStatus) bool {
	return s == SessionStatusCompleted || s == SessionStatusError || s == SessionStatusExpired
}

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
	// HasMedia indicates whether this message contains media attachments.
	HasMedia bool `json:"hasMedia,omitempty"`
	// MediaTypes lists the distinct media types (e.g., ["image", "audio"]).
	MediaTypes []string `json:"mediaTypes,omitempty"`
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
	// Width is the media width in pixels (images/video).
	Width int32 `json:"width,omitempty"`
	// Height is the media height in pixels (images/video).
	Height int32 `json:"height,omitempty"`
	// DurationMs is the media duration in milliseconds (audio/video).
	DurationMs int32 `json:"durationMs,omitempty"`
	// Channels is the number of audio channels.
	Channels int32 `json:"channels,omitempty"`
	// SampleRate is the audio sample rate in Hz.
	SampleRate int32 `json:"sampleRate,omitempty"`
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
	// PromptPackName is the PromptPack associated with this session's agent.
	PromptPackName string `json:"promptPackName,omitempty"`
	// PromptPackVersion is the version of the PromptPack at session creation.
	PromptPackVersion string `json:"promptPackVersion,omitempty"`
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
	// ID is an optional pre-generated session ID. When set, the store uses this
	// ID instead of generating a new one. This supports deferred persistence:
	// the facade generates a UUID on connect (for immediate client response) but
	// only persists the session on the first message.
	ID string
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
	// PromptPackName is the PromptPack associated with this session's agent.
	PromptPackName string
	// PromptPackVersion is the version of the PromptPack at session creation.
	PromptPackVersion string
}

// SessionStatsUpdate contains incremental updates to session-level counters.
// All Add* fields are added to the current values; SetStatus is applied only if non-empty.
// SetEndedAt, when non-zero, records when the session ended.
type SessionStatsUpdate struct {
	AddInputTokens  int32
	AddOutputTokens int32
	AddCostUSD      float64
	AddToolCalls    int32
	AddMessages     int32
	SetStatus       SessionStatus // empty means no change
	SetEndedAt      time.Time     // zero means no change
}

// ToolCallStatus represents the lifecycle state of a tool call.
type ToolCallStatus string

const (
	// ToolCallStatusPending indicates the tool call is in progress.
	ToolCallStatusPending ToolCallStatus = "pending"
	// ToolCallStatusSuccess indicates the tool call completed successfully.
	ToolCallStatusSuccess ToolCallStatus = "success"
	// ToolCallStatusError indicates the tool call failed.
	ToolCallStatusError ToolCallStatus = "error"
)

// ToolCallExecution indicates where the tool call is executed.
type ToolCallExecution string

const (
	// ToolCallExecutionServer indicates the tool runs on the server (runtime).
	ToolCallExecutionServer ToolCallExecution = "server"
	// ToolCallExecutionClient indicates the tool runs on the client (browser).
	ToolCallExecutionClient ToolCallExecution = "client"
)

// ToolCall represents a single tool invocation within a session.
type ToolCall struct {
	// ID is the unique identifier for this tool call record.
	ID string `json:"id"`
	// SessionID links this tool call to its parent session.
	SessionID string `json:"sessionId"`
	// CallID is the provider-assigned tool call identifier.
	CallID string `json:"callId"`
	// Name is the tool name.
	Name string `json:"name"`
	// Arguments contains the tool input parameters.
	Arguments map[string]any `json:"arguments,omitempty"`
	// Result contains the tool output (nil while pending).
	Result any `json:"result,omitempty"`
	// Status is the lifecycle state of the tool call.
	Status ToolCallStatus `json:"status"`
	// DurationMs is the execution duration in milliseconds.
	DurationMs int64 `json:"durationMs,omitempty"`
	// Execution indicates where the tool runs (server or client).
	Execution ToolCallExecution `json:"execution,omitempty"`
	// ErrorMessage contains the error details when status is error.
	ErrorMessage string `json:"errorMessage,omitempty"`
	// Labels contains arbitrary key-value pairs for categorization.
	Labels map[string]string `json:"labels,omitempty"`
	// CreatedAt is when the tool call was initiated.
	CreatedAt time.Time `json:"createdAt"`
}

// ProviderCallStatus represents the lifecycle state of a provider call.
type ProviderCallStatus string

const (
	// ProviderCallStatusPending indicates the provider call is in progress.
	ProviderCallStatusPending ProviderCallStatus = "pending"
	// ProviderCallStatusCompleted indicates the provider call completed.
	ProviderCallStatusCompleted ProviderCallStatus = "completed"
	// ProviderCallStatusFailed indicates the provider call failed.
	ProviderCallStatusFailed ProviderCallStatus = "failed"
)

// ProviderCall represents a single LLM provider invocation within a session.
type ProviderCall struct {
	// ID is the unique identifier for this provider call record.
	ID string `json:"id"`
	// SessionID links this provider call to its parent session.
	SessionID string `json:"sessionId"`
	// Provider is the LLM provider name (e.g., "anthropic", "openai").
	Provider string `json:"provider"`
	// Model is the model identifier (e.g., "claude-sonnet-4-20250514").
	Model string `json:"model"`
	// Status is the lifecycle state of the provider call.
	Status ProviderCallStatus `json:"status"`
	// InputTokens is the number of input tokens consumed.
	InputTokens int64 `json:"inputTokens,omitempty"`
	// OutputTokens is the number of output tokens produced.
	OutputTokens int64 `json:"outputTokens,omitempty"`
	// CachedTokens is the number of cached input tokens.
	CachedTokens int64 `json:"cachedTokens,omitempty"`
	// CostUSD is the estimated cost in USD.
	CostUSD float64 `json:"costUsd,omitempty"`
	// DurationMs is the call duration in milliseconds.
	DurationMs int64 `json:"durationMs,omitempty"`
	// FinishReason is the provider-reported finish reason.
	FinishReason string `json:"finishReason,omitempty"`
	// ToolCallCount is the number of tool calls in the response.
	ToolCallCount int32 `json:"toolCallCount,omitempty"`
	// ErrorMessage contains the error details when status is failed.
	ErrorMessage string `json:"errorMessage,omitempty"`
	// Labels contains arbitrary key-value pairs for categorization.
	Labels map[string]string `json:"labels,omitempty"`
	// CreatedAt is when the provider call was initiated.
	CreatedAt time.Time `json:"createdAt"`
}

// EvalResult represents a single evaluation result.
// Written by both the runtime (inline evals) and the arena eval worker.
type EvalResult struct {
	// ID is auto-generated by the database if empty.
	ID                string          `json:"id,omitempty"`
	SessionID         string          `json:"sessionId"`
	MessageID         string          `json:"messageId,omitempty"`
	AgentName         string          `json:"agentName,omitempty"`
	Namespace         string          `json:"namespace,omitempty"`
	PromptPackName    string          `json:"promptpackName,omitempty"`
	PromptPackVersion string          `json:"promptpackVersion,omitempty"`
	EvalID            string          `json:"evalId"`
	EvalType          string          `json:"evalType"`
	Trigger           string          `json:"trigger"`
	Passed            bool            `json:"passed"`
	Score             *float64        `json:"score,omitempty"`
	Details           json.RawMessage `json:"details,omitempty"`
	DurationMs        *int            `json:"durationMs,omitempty"`
	JudgeTokens       *int            `json:"judgeTokens,omitempty"`
	JudgeCostUSD      *float64        `json:"judgeCostUsd,omitempty"`
	Source            string          `json:"source"`
	CreatedAt         time.Time       `json:"createdAt"`
}

// RuntimeEvent represents a lifecycle event from the PromptKit runtime
// (pipeline, stage, middleware, validation, workflow, context/state events).
// These are persisted in a dedicated table rather than as system messages.
type RuntimeEvent struct {
	// ID is the unique identifier for this event.
	ID string `json:"id"`
	// SessionID links this event to its parent session.
	SessionID string `json:"sessionId"`
	// EventType is the PromptKit event type (e.g., "pipeline.started").
	EventType string `json:"eventType"`
	// Data contains the event payload as arbitrary JSON.
	Data map[string]any `json:"data,omitempty"`
	// DurationMs is the event duration in milliseconds (for completed events).
	DurationMs int64 `json:"durationMs,omitempty"`
	// ErrorMessage contains error details for failed events.
	ErrorMessage string `json:"errorMessage,omitempty"`
	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`
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

	// RecordToolCall records or updates a tool call for the session.
	// Uses upsert semantics: if a record with the same ID exists, it is updated.
	// Returns ErrSessionNotFound if the session does not exist.
	RecordToolCall(ctx context.Context, sessionID string, tc ToolCall) error

	// RecordProviderCall records or updates a provider call for the session.
	// Uses upsert semantics: if a record with the same ID exists, it is updated.
	// Returns ErrSessionNotFound if the session does not exist.
	RecordProviderCall(ctx context.Context, sessionID string, pc ProviderCall) error

	// GetToolCalls retrieves all tool calls for a session ordered by created_at.
	// Returns ErrSessionNotFound if the session does not exist.
	GetToolCalls(ctx context.Context, sessionID string) ([]ToolCall, error)

	// GetProviderCalls retrieves all provider calls for a session ordered by created_at.
	// Returns ErrSessionNotFound if the session does not exist.
	GetProviderCalls(ctx context.Context, sessionID string) ([]ProviderCall, error)

	// RecordEvalResult records one or more eval results for the session.
	// Both the runtime (inline evals) and the arena worker write through this method.
	RecordEvalResult(ctx context.Context, sessionID string, result EvalResult) error

	// RecordRuntimeEvent records a runtime lifecycle event for the session.
	// Events are immutable (append-only, no upsert).
	// Returns ErrSessionNotFound if the session does not exist.
	RecordRuntimeEvent(ctx context.Context, sessionID string, evt RuntimeEvent) error

	// GetRuntimeEvents retrieves all runtime events for a session ordered by timestamp.
	// Returns ErrSessionNotFound if the session does not exist.
	GetRuntimeEvents(ctx context.Context, sessionID string) ([]RuntimeEvent, error)

	// Close releases any resources held by the store.
	Close() error
}
