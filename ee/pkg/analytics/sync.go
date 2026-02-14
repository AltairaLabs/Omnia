/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

// Package analytics provides interfaces and implementations for syncing
// session data to external analytics platforms (e.g. Snowflake) for OLAP queries.
package analytics

import (
	"context"
	"errors"
	"time"
)

// Common errors returned by sync providers.
var (
	// ErrNotInitialized is returned when Sync is called before Init.
	ErrNotInitialized = errors.New("sync provider not initialized")

	// ErrAlreadyClosed is returned when operations are attempted on a closed provider.
	ErrAlreadyClosed = errors.New("sync provider already closed")

	// ErrNoTables is returned when the provider has no tables configured.
	ErrNoTables = errors.New("no tables configured for sync")
)

// SyncProvider defines the interface for analytics sync destinations.
type SyncProvider interface {
	// Init establishes the connection and ensures the destination schema exists.
	Init(ctx context.Context) error

	// Sync performs an incremental sync of session data to the destination.
	Sync(ctx context.Context, opts SyncOpts) (*SyncResult, error)

	// GetWatermark returns the last synced timestamp for a given table.
	GetWatermark(ctx context.Context, table string) (time.Time, error)

	// Ping verifies the connection to the destination is alive.
	Ping(ctx context.Context) error

	// Close releases all resources held by the provider.
	Close() error
}

// SyncOpts configures a single sync run.
type SyncOpts struct {
	// BatchSize controls the maximum number of rows per batch. Zero means use provider default.
	BatchSize int
	// Tables restricts the sync to specific tables. Empty means all configured tables.
	Tables []string
	// DryRun, when true, simulates the sync without writing data.
	DryRun bool
}

// SyncResult contains the outcome of a sync run.
type SyncResult struct {
	// Tables contains per-table sync results.
	Tables []TableSyncResult
	// TotalRows is the total number of rows synced across all tables.
	TotalRows int64
	// Duration is the wall-clock time the sync took.
	Duration time.Duration
	// WatermarkFrom is the starting watermark for this sync run.
	WatermarkFrom time.Time
	// WatermarkTo is the ending watermark after this sync run.
	WatermarkTo time.Time
}

// TableSyncResult contains the outcome for a single table.
type TableSyncResult struct {
	// Table is the destination table name.
	Table string
	// RowsSynced is the number of rows written.
	RowsSynced int64
	// WatermarkFrom is the starting watermark for this table.
	WatermarkFrom time.Time
	// WatermarkTo is the ending watermark for this table.
	WatermarkTo time.Time
	// Error is set if this table's sync failed.
	Error error
}

// SourceReader provides data to be synced. Implementations read from the
// source database (e.g. Postgres) and return rows for the destination.
type SourceReader interface {
	// ReadSessions returns sessions updated after the given watermark, up to limit rows.
	ReadSessions(ctx context.Context, after time.Time, limit int) ([]SessionRow, error)
	// ReadMessages returns messages created after the given watermark, up to limit rows.
	ReadMessages(ctx context.Context, after time.Time, limit int) ([]MessageRow, error)
}

// SessionRow is a flattened session record for analytics sync.
type SessionRow struct {
	SessionID         string
	WorkspaceID       string
	AgentID           string
	UserID            string
	Status            string
	Namespace         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	MessageCount      int32
	TotalInputTokens  int64
	TotalOutputTokens int64
	Tags              []string
	Metadata          map[string]string
}

// MessageRow is a flattened message record for analytics sync.
type MessageRow struct {
	MessageID    string
	SessionID    string
	Role         string
	Content      string
	InputTokens  int32
	OutputTokens int32
	SequenceNum  int32
	CreatedAt    time.Time
}
