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

package providers

import (
	"context"
	"time"

	"github.com/altairalabs/omnia/internal/session"
)

// WarmStoreProvider defines the interface for durable, queryable session
// storage (e.g. Postgres with partitioned tables). It is the system of record
// for session data and supports search, pagination, and partition management.
type WarmStoreProvider interface {
	// CreateSession persists a new session. The session ID must already be set.
	// Returns an error if a session with the same ID already exists.
	CreateSession(ctx context.Context, s *session.Session) error

	// GetSession retrieves a session by ID, including metadata but not messages.
	// Returns session.ErrSessionNotFound if the session does not exist.
	GetSession(ctx context.Context, sessionID string) (*session.Session, error)

	// UpdateSession updates an existing session's mutable fields.
	// Returns session.ErrSessionNotFound if the session does not exist.
	UpdateSession(ctx context.Context, s *session.Session) error

	// UpdateSessionStatus atomically updates session lifecycle state.
	// Implementations should use a single atomic SQL statement to avoid
	// read-modify-write race conditions. Returns session.ErrSessionNotFound
	// if the session does not exist.
	UpdateSessionStatus(ctx context.Context, sessionID string, update session.SessionStatusUpdate) error

	// DeleteSession removes a session and all its associated data.
	// Returns session.ErrSessionNotFound if the session does not exist.
	DeleteSession(ctx context.Context, sessionID string) error

	// AppendMessage adds a message to the session's history.
	// Returns session.ErrSessionNotFound if the session does not exist.
	AppendMessage(ctx context.Context, sessionID string, msg *session.Message) error

	// GetMessages retrieves messages for a session with filtering and pagination.
	// Returns session.ErrSessionNotFound if the session does not exist.
	GetMessages(ctx context.Context, sessionID string, opts MessageQueryOpts) ([]*session.Message, error)

	// ListSessions returns a paginated list of sessions matching the given filters.
	ListSessions(ctx context.Context, opts SessionListOpts) (*SessionPage, error)

	// SearchSessions performs full-text search across session content and metadata.
	SearchSessions(ctx context.Context, query string, opts SessionListOpts) (*SessionPage, error)

	// CreatePartition creates a new table partition for the given date range.
	// Returns ErrPartitionExists if the partition already exists.
	CreatePartition(ctx context.Context, date time.Time) error

	// DropPartition removes a table partition for the given date range.
	// Returns ErrPartitionNotFound if the partition does not exist.
	DropPartition(ctx context.Context, date time.Time) error

	// ListPartitions returns metadata about all existing partitions.
	ListPartitions(ctx context.Context) ([]PartitionInfo, error)

	// GetSessionsOlderThan returns sessions last updated before the cutoff,
	// up to batchSize. Used for compaction/archival workflows.
	GetSessionsOlderThan(ctx context.Context, cutoff time.Time, batchSize int) ([]*session.Session, error)

	// DeleteSessionsBatch removes multiple sessions in a single operation.
	DeleteSessionsBatch(ctx context.Context, sessionIDs []string) error

	// RecordToolCall appends a tool call lifecycle event (INSERT only).
	// Returns session.ErrSessionNotFound if the session does not exist.
	RecordToolCall(ctx context.Context, sessionID string, tc *session.ToolCall) error

	// RecordProviderCall appends a provider call lifecycle event (INSERT only).
	// Returns session.ErrSessionNotFound if the session does not exist.
	RecordProviderCall(ctx context.Context, sessionID string, pc *session.ProviderCall) error

	// GetToolCalls retrieves tool calls for a session ordered by created_at.
	// Returns session.ErrSessionNotFound if the session does not exist.
	GetToolCalls(ctx context.Context, sessionID string, opts PaginationOpts) ([]*session.ToolCall, error)

	// GetProviderCalls retrieves provider calls for a session ordered by created_at.
	// Returns session.ErrSessionNotFound if the session does not exist.
	GetProviderCalls(ctx context.Context, sessionID string, opts PaginationOpts) ([]*session.ProviderCall, error)

	// RecordRuntimeEvent records a runtime lifecycle event for the session.
	// Events are immutable (INSERT only, no upsert).
	// Returns session.ErrSessionNotFound if the session does not exist.
	RecordRuntimeEvent(ctx context.Context, sessionID string, evt *session.RuntimeEvent) error

	// GetRuntimeEvents retrieves runtime events for a session ordered by timestamp.
	// Returns session.ErrSessionNotFound if the session does not exist.
	GetRuntimeEvents(ctx context.Context, sessionID string, opts PaginationOpts) ([]*session.RuntimeEvent, error)

	// SaveArtifact persists a binary artifact reference.
	// Reserved for future use — currently has no HTTP route in the session API.
	SaveArtifact(ctx context.Context, artifact *session.Artifact) error

	// GetArtifacts retrieves all artifacts for a message.
	// Reserved for future use — currently has no HTTP route in the session API.
	GetArtifacts(ctx context.Context, messageID string) ([]*session.Artifact, error)

	// GetSessionArtifacts retrieves all artifacts for a session.
	// Reserved for future use — currently has no HTTP route in the session API.
	GetSessionArtifacts(ctx context.Context, sessionID string) ([]*session.Artifact, error)

	// DeleteSessionArtifacts removes all artifacts for a session.
	// Reserved for future use — currently has no HTTP route in the session API.
	DeleteSessionArtifacts(ctx context.Context, sessionID string) error

	// RefreshTTL updates the expires_at and updated_at fields in a single
	// UPDATE without reading the full row first.
	// Returns session.ErrSessionNotFound if the session does not exist.
	RefreshTTL(ctx context.Context, sessionID string, expiresAt time.Time) error

	// Ping checks connectivity to the underlying store.
	Ping(ctx context.Context) error

	// Close releases resources held by the provider.
	Close() error
}

// StatusUpdateResult contains metadata returned by an optimized status update
// so the caller can detect transitions and build events without extra queries.
type StatusUpdateResult struct {
	// Applied is true when the update modified the row (session existed and
	// was not already in a terminal status).
	Applied bool
	// PreviousStatus is the session status before the update was applied.
	PreviousStatus session.SessionStatus
	// AgentName of the updated session.
	AgentName string
	// Namespace of the updated session.
	Namespace string
	// PromptPackName of the updated session.
	PromptPackName string
	// PromptPackVersion of the updated session.
	PromptPackVersion string
}

// StatusUpdaterWithResult is an optional interface that WarmStoreProvider
// implementations can satisfy to return metadata from status updates in a
// single query, avoiding extra GetSession round-trips.
type StatusUpdaterWithResult interface {
	UpdateSessionStatusReturning(ctx context.Context, sessionID string, update session.SessionStatusUpdate) (*StatusUpdateResult, error)
}
