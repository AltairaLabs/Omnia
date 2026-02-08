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

	// Ping checks connectivity to the underlying store.
	Ping(ctx context.Context) error

	// Close releases resources held by the provider.
	Close() error
}
