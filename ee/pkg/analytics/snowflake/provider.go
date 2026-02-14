/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package snowflake

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/altairalabs/omnia/ee/pkg/analytics"

	// Register the Snowflake driver for database/sql.
	_ "github.com/snowflakedb/gosnowflake"
)

// Row abstracts *sql.Row for testability.
type Row interface {
	Scan(dest ...any) error
}

// DB abstracts database/sql operations for testability.
type DB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) Row
	PingContext(ctx context.Context) error
	Close() error
}

// sqlDBAdapter wraps *sql.DB to satisfy the DB interface,
// since *sql.DB.QueryRowContext returns *sql.Row, not our Row interface.
type sqlDBAdapter struct {
	db *sql.DB
}

func (a *sqlDBAdapter) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return a.db.ExecContext(ctx, query, args...)
}

func (a *sqlDBAdapter) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return a.db.QueryContext(ctx, query, args...)
}

func (a *sqlDBAdapter) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	return a.db.QueryRowContext(ctx, query, args...)
}

func (a *sqlDBAdapter) PingContext(ctx context.Context) error {
	return a.db.PingContext(ctx)
}

func (a *sqlDBAdapter) Close() error {
	return a.db.Close()
}

// SQL query constants for session and message upserts.
const (
	sessionMergeQuery = `MERGE INTO omnia_sessions t USING (SELECT
		? AS session_id, ? AS workspace_id, ? AS agent_id, ? AS user_id,
		? AS status, ? AS namespace, ? AS created_at, ? AS updated_at,
		? AS message_count, ? AS total_input_tokens, ? AS total_output_tokens,
		PARSE_JSON(?) AS tags, PARSE_JSON(?) AS metadata
	) s ON t.session_id = s.session_id
	WHEN MATCHED THEN UPDATE SET
		workspace_id = s.workspace_id, agent_id = s.agent_id, user_id = s.user_id,
		status = s.status, namespace = s.namespace, updated_at = s.updated_at,
		message_count = s.message_count, total_input_tokens = s.total_input_tokens,
		total_output_tokens = s.total_output_tokens, tags = s.tags, metadata = s.metadata
	WHEN NOT MATCHED THEN INSERT (session_id, workspace_id, agent_id, user_id, status,
		namespace, created_at, updated_at, message_count, total_input_tokens,
		total_output_tokens, tags, metadata)
		VALUES (s.session_id, s.workspace_id, s.agent_id, s.user_id, s.status,
		s.namespace, s.created_at, s.updated_at, s.message_count, s.total_input_tokens,
		s.total_output_tokens, s.tags, s.metadata)`

	messageMergeQuery = `MERGE INTO omnia_messages t USING (SELECT
		? AS message_id, ? AS session_id, ? AS role, ? AS content,
		? AS input_tokens, ? AS output_tokens, ? AS sequence_num, ? AS created_at
	) s ON t.message_id = s.message_id AND t.session_id = s.session_id
	WHEN MATCHED THEN UPDATE SET
		role = s.role, content = s.content, input_tokens = s.input_tokens,
		output_tokens = s.output_tokens, sequence_num = s.sequence_num
	WHEN NOT MATCHED THEN INSERT (message_id, session_id, role, content,
		input_tokens, output_tokens, sequence_num, created_at)
		VALUES (s.message_id, s.session_id, s.role, s.content,
		s.input_tokens, s.output_tokens, s.sequence_num, s.created_at)`
)

// Provider implements analytics.SyncProvider for Snowflake.
type Provider struct {
	config *Config
	source analytics.SourceReader
	db     DB
	mu     sync.RWMutex
	closed bool
	inited bool
}

// NewProvider creates a new Snowflake sync provider.
// The source reader provides session data to sync. The DB connection is
// established during Init().
func NewProvider(cfg *Config, source analytics.SourceReader) *Provider {
	return &Provider{
		config: cfg,
		source: source,
	}
}

// newProviderWithDB creates a provider with a pre-existing DB connection (for testing).
func newProviderWithDB(cfg *Config, source analytics.SourceReader, db DB) *Provider {
	return &Provider{
		config: cfg,
		source: source,
		db:     db,
	}
}

// Init establishes the Snowflake connection and creates schema tables.
func (p *Provider) Init(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return analytics.ErrAlreadyClosed
	}

	if err := p.config.Validate(); err != nil {
		return fmt.Errorf("snowflake config: %w", err)
	}

	if p.db == nil {
		db, err := sql.Open("snowflake", p.config.DSN())
		if err != nil {
			return fmt.Errorf("snowflake open: %w", err)
		}
		p.db = &sqlDBAdapter{db: db}
	}

	if err := p.db.PingContext(ctx); err != nil {
		return fmt.Errorf("snowflake ping: %w", err)
	}

	if err := p.ensureSchema(ctx); err != nil {
		return fmt.Errorf("snowflake schema: %w", err)
	}

	p.inited = true
	return nil
}

// ensureSchema creates the analytics tables if they do not exist.
func (p *Provider) ensureSchema(ctx context.Context) error {
	for _, ddl := range SchemaDDL() {
		if _, err := p.db.ExecContext(ctx, ddl); err != nil {
			return err
		}
	}
	return nil
}

// Ping verifies the Snowflake connection is alive.
func (p *Provider) Ping(ctx context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return analytics.ErrAlreadyClosed
	}
	if !p.inited {
		return analytics.ErrNotInitialized
	}
	return p.db.PingContext(ctx)
}

// GetWatermark returns the last sync timestamp for a given table.
func (p *Provider) GetWatermark(ctx context.Context, table string) (time.Time, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return time.Time{}, analytics.ErrAlreadyClosed
	}
	if !p.inited {
		return time.Time{}, analytics.ErrNotInitialized
	}
	return getWatermark(ctx, p.db, table)
}

// Sync performs an incremental sync of session data to Snowflake.
func (p *Provider) Sync(ctx context.Context, opts analytics.SyncOpts) (*analytics.SyncResult, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, analytics.ErrAlreadyClosed
	}
	if !p.inited {
		p.mu.RUnlock()
		return nil, analytics.ErrNotInitialized
	}
	p.mu.RUnlock()

	start := time.Now()
	tables := opts.Tables
	if len(tables) == 0 {
		tables = AllTables
	}

	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = p.config.DefaultBatchSize
	}

	result := &analytics.SyncResult{}
	for _, table := range tables {
		tr := p.syncTable(ctx, table, batchSize, opts.DryRun)
		result.Tables = append(result.Tables, tr)
		result.TotalRows += tr.RowsSynced
		result.WatermarkTo = latestTime(result.WatermarkTo, tr.WatermarkTo)
		if result.WatermarkFrom.IsZero() || (!tr.WatermarkFrom.IsZero() && tr.WatermarkFrom.Before(result.WatermarkFrom)) {
			result.WatermarkFrom = tr.WatermarkFrom
		}
	}
	result.Duration = time.Since(start)
	return result, nil
}

// syncTable syncs a single table. Extracted for cognitive complexity.
func (p *Provider) syncTable(ctx context.Context, table string, batchSize int, dryRun bool) analytics.TableSyncResult {
	tr := analytics.TableSyncResult{Table: table}

	wm, err := getWatermark(ctx, p.db, table)
	if err != nil {
		tr.Error = fmt.Errorf("get watermark for %s: %w", table, err)
		return tr
	}
	tr.WatermarkFrom = wm

	rowCount, maxTime, err := p.syncTableData(ctx, table, wm, batchSize, dryRun)
	if err != nil {
		tr.Error = err
		return tr
	}

	tr.RowsSynced = rowCount
	tr.WatermarkTo = maxTime

	if !dryRun && rowCount > 0 {
		if err := setWatermark(ctx, p.db, table, maxTime, rowCount); err != nil {
			tr.Error = fmt.Errorf("set watermark for %s: %w", table, err)
		}
	}
	return tr
}

// syncTableData reads data from source and writes to Snowflake for a single table.
func (p *Provider) syncTableData(
	ctx context.Context, table string, after time.Time, batchSize int, dryRun bool,
) (int64, time.Time, error) {
	switch table {
	case TableSessions:
		return p.syncSessions(ctx, after, batchSize, dryRun)
	case TableMessages:
		return p.syncMessages(ctx, after, batchSize, dryRun)
	default:
		return 0, after, fmt.Errorf("unknown table: %s", table)
	}
}

// syncSessions syncs session rows from source to Snowflake.
func (p *Provider) syncSessions(
	ctx context.Context, after time.Time, limit int, dryRun bool,
) (int64, time.Time, error) {
	rows, err := p.source.ReadSessions(ctx, after, limit)
	if err != nil {
		return 0, after, fmt.Errorf("read sessions: %w", err)
	}
	if len(rows) == 0 {
		return 0, after, nil
	}

	maxTime := after
	if !dryRun {
		for i := range rows {
			if err := p.upsertSession(ctx, &rows[i]); err != nil {
				return int64(i), maxTime, fmt.Errorf("upsert session %s: %w", rows[i].SessionID, err)
			}
			maxTime = latestTime(maxTime, rows[i].UpdatedAt)
		}
	} else {
		for i := range rows {
			maxTime = latestTime(maxTime, rows[i].UpdatedAt)
		}
	}
	return int64(len(rows)), maxTime, nil
}

// syncMessages syncs message rows from source to Snowflake.
func (p *Provider) syncMessages(
	ctx context.Context, after time.Time, limit int, dryRun bool,
) (int64, time.Time, error) {
	rows, err := p.source.ReadMessages(ctx, after, limit)
	if err != nil {
		return 0, after, fmt.Errorf("read messages: %w", err)
	}
	if len(rows) == 0 {
		return 0, after, nil
	}

	maxTime := after
	if !dryRun {
		for i := range rows {
			if err := p.upsertMessage(ctx, &rows[i]); err != nil {
				return int64(i), maxTime, fmt.Errorf("upsert message %s: %w", rows[i].MessageID, err)
			}
			maxTime = latestTime(maxTime, rows[i].CreatedAt)
		}
	} else {
		for i := range rows {
			maxTime = latestTime(maxTime, rows[i].CreatedAt)
		}
	}
	return int64(len(rows)), maxTime, nil
}

// upsertSession merges a single session row into Snowflake.
func (p *Provider) upsertSession(ctx context.Context, row *analytics.SessionRow) error {
	tagsJSON, metaJSON := marshalSessionJSON(row)
	_, err := p.db.ExecContext(ctx, sessionMergeQuery,
		row.SessionID, row.WorkspaceID, row.AgentID, row.UserID,
		row.Status, row.Namespace, row.CreatedAt, row.UpdatedAt,
		row.MessageCount, row.TotalInputTokens, row.TotalOutputTokens,
		tagsJSON, metaJSON,
	)
	return err
}

// marshalSessionJSON converts tags and metadata to JSON strings for Snowflake.
func marshalSessionJSON(row *analytics.SessionRow) (string, string) {
	tagsJSON := "[]"
	if len(row.Tags) > 0 {
		if b, err := json.Marshal(row.Tags); err == nil {
			tagsJSON = string(b)
		}
	}
	metaJSON := "{}"
	if len(row.Metadata) > 0 {
		if b, err := json.Marshal(row.Metadata); err == nil {
			metaJSON = string(b)
		}
	}
	return tagsJSON, metaJSON
}

// upsertMessage merges a single message row into Snowflake.
func (p *Provider) upsertMessage(ctx context.Context, row *analytics.MessageRow) error {
	_, err := p.db.ExecContext(ctx, messageMergeQuery,
		row.MessageID, row.SessionID, row.Role, row.Content,
		row.InputTokens, row.OutputTokens, row.SequenceNum, row.CreatedAt,
	)
	return err
}

// Close releases the Snowflake connection.
func (p *Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return analytics.ErrAlreadyClosed
	}
	p.closed = true

	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

// latestTime returns the later of two times.
func latestTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// filterTables returns the intersection of requested and available tables.
// If requested is empty, returns all available tables.
func filterTables(requested, available []string) []string {
	if len(requested) == 0 {
		return available
	}
	avail := make(map[string]bool, len(available))
	for _, t := range available {
		avail[t] = true
	}
	var result []string
	for _, t := range requested {
		if avail[strings.ToLower(t)] {
			result = append(result, strings.ToLower(t))
		}
	}
	return result
}
