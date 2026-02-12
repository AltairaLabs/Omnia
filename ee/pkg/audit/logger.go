/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/altairalabs/omnia/ee/pkg/metrics"
	"github.com/altairalabs/omnia/internal/session/api"
)

const (
	// DefaultBufferSize is the default capacity of the async event buffer.
	DefaultBufferSize = 1024
	// DefaultWorkers is the default number of background writer goroutines.
	DefaultWorkers = 2
	// DefaultBatchSize is the maximum number of entries written per batch.
	DefaultBatchSize = 50
	// DefaultFlushInterval is the maximum time between batch writes.
	DefaultFlushInterval = 500 * time.Millisecond
)

// LoggerConfig configures the audit Logger.
type LoggerConfig struct {
	BufferSize    int
	Workers       int
	BatchSize     int
	FlushInterval time.Duration
}

// dbPool abstracts the database operations needed by the audit logger.
// This allows mocking in unit tests while using *pgxpool.Pool in production.
type dbPool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Logger implements api.AuditLogger with async buffered writes to PostgreSQL.
type Logger struct {
	pool    dbPool
	buffer  chan *Entry
	stopCh  chan struct{}
	wg      sync.WaitGroup
	metrics *metrics.AuditMetrics
	log     logr.Logger
	cfg     LoggerConfig
}

// Compile-time interface check.
var _ api.AuditLogger = (*Logger)(nil)

// NewLogger creates a new audit Logger that writes to PostgreSQL asynchronously.
func NewLogger(pool *pgxpool.Pool, log logr.Logger, m *metrics.AuditMetrics, cfg LoggerConfig) *Logger {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = DefaultBufferSize
	}
	if cfg.Workers <= 0 {
		cfg.Workers = DefaultWorkers
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = DefaultBatchSize
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = DefaultFlushInterval
	}

	var db dbPool
	if pool != nil {
		db = pool
	}

	l := &Logger{
		pool:    db,
		buffer:  make(chan *Entry, cfg.BufferSize),
		stopCh:  make(chan struct{}),
		metrics: m,
		log:     log.WithName("audit-logger"),
		cfg:     cfg,
	}

	for i := range cfg.Workers {
		_ = i
		l.wg.Add(1)
		go l.worker()
	}

	return l
}

// LogEvent converts an api.AuditEntry to an internal Entry and enqueues it.
// Non-blocking: if the buffer is full, the entry is dropped with a metric increment.
func (l *Logger) LogEvent(_ context.Context, entry *api.AuditEntry) {
	e := &Entry{
		Timestamp:   time.Now().UTC(),
		EventType:   entry.EventType,
		SessionID:   entry.SessionID,
		Workspace:   entry.Workspace,
		AgentName:   entry.AgentName,
		Namespace:   entry.Namespace,
		Query:       entry.Query,
		ResultCount: entry.ResultCount,
		IPAddress:   entry.IPAddress,
		UserAgent:   entry.UserAgent,
		Metadata:    entry.Metadata,
	}

	if l.metrics != nil {
		l.metrics.EventsTotal.WithLabelValues(entry.EventType).Inc()
	}

	select {
	case l.buffer <- e:
	default:
		if l.metrics != nil {
			l.metrics.BufferDrops.WithLabelValues(entry.EventType).Inc()
		}
		l.log.V(1).Info("audit buffer full, dropping entry", "eventType", entry.EventType)
	}
}

// Query performs a synchronous query against the audit log table.
func (l *Logger) Query(ctx context.Context, opts QueryOpts) (*QueryResult, error) {
	if l.metrics != nil {
		l.metrics.QueriesTotal.Inc()
		start := time.Now()
		defer func() {
			l.metrics.QueryDuration.Observe(time.Since(start).Seconds())
		}()
	}

	qb := buildQueryFilters(opts)
	where := qb.where()

	// Count total matching entries.
	var total int64
	if err := l.pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_log WHERE 1=1"+where, qb.args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("audit: count query: %w", err)
	}

	limit := max(opts.Limit, 1)
	limit = min(limit, 500)
	offset := max(opts.Offset, 0)

	dataQuery := `SELECT id, timestamp, event_type, session_id, user_id,
		workspace, agent_name, namespace, query, result_count,
		host(ip_address), user_agent, reason, metadata
		FROM audit_log WHERE 1=1` + where + ` ORDER BY timestamp DESC`
	dataQuery = qb.appendPagination(dataQuery, limit, offset)

	rows, err := l.pool.Query(ctx, dataQuery, qb.args...)
	if err != nil {
		return nil, fmt.Errorf("audit: data query: %w", err)
	}
	defer rows.Close()

	entries, err := scanEntries(rows)
	if err != nil {
		return nil, err
	}

	return &QueryResult{
		Entries: entries,
		Total:   total,
		HasMore: int64(offset)+int64(len(entries)) < total,
	}, nil
}

// Close stops background workers and drains the buffer.
func (l *Logger) Close() error {
	close(l.stopCh)
	l.wg.Wait()
	return nil
}

// worker drains the buffer channel and batch-inserts entries into PostgreSQL.
func (l *Logger) worker() {
	defer l.wg.Done()

	batch := make([]*Entry, 0, l.cfg.BatchSize)
	ticker := time.NewTicker(l.cfg.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case entry, ok := <-l.buffer:
			if !ok {
				l.flushBatch(batch)
				return
			}
			batch = append(batch, entry)
			if len(batch) >= l.cfg.BatchSize {
				l.writeBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				l.writeBatch(batch)
				batch = batch[:0]
			}

		case <-l.stopCh:
			batch = l.drainBuffer(batch)
			l.flushBatch(batch)
			return
		}
	}
}

// drainBuffer reads all remaining entries from the buffer channel.
func (l *Logger) drainBuffer(batch []*Entry) []*Entry {
	for {
		select {
		case entry, ok := <-l.buffer:
			if !ok {
				return batch
			}
			batch = append(batch, entry)
			if len(batch) >= l.cfg.BatchSize {
				l.writeBatch(batch)
				batch = batch[:0]
			}
		default:
			return batch
		}
	}
}

// flushBatch writes any remaining entries in the batch.
func (l *Logger) flushBatch(batch []*Entry) {
	if len(batch) > 0 {
		l.writeBatch(batch)
	}
}

// writeBatch inserts a slice of entries into the audit_log table.
func (l *Logger) writeBatch(entries []*Entry) {
	if len(entries) == 0 || l.pool == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	query, args := buildBatchInsert(entries)
	_, err := l.pool.Exec(ctx, query, args...)
	duration := time.Since(start)

	eventType := entries[0].EventType
	if l.metrics != nil {
		l.metrics.WriteDuration.WithLabelValues(eventType).Observe(duration.Seconds())
	}

	if err != nil {
		if l.metrics != nil {
			l.metrics.WriteErrors.WithLabelValues(eventType).Inc()
		}
		l.log.Error(err, "failed to write audit batch", "count", len(entries))
	}
}

// --- query helpers ----------------------------------------------------------

// buildQueryFilters constructs WHERE clause filters from QueryOpts.
func buildQueryFilters(opts QueryOpts) *queryBuilder {
	qb := &queryBuilder{}
	if opts.SessionID != "" {
		qb.add("session_id = $?", opts.SessionID)
	}
	if opts.UserID != "" {
		qb.add("user_id = $?", opts.UserID)
	}
	if opts.Workspace != "" {
		qb.add("workspace = $?", opts.Workspace)
	}
	if len(opts.EventTypes) > 0 {
		qb.add("event_type = ANY($?)", opts.EventTypes)
	}
	if !opts.From.IsZero() {
		qb.add("timestamp >= $?", opts.From)
	}
	if !opts.To.IsZero() {
		qb.add("timestamp < $?", opts.To)
	}
	return qb
}

// scanEntries reads all Entry rows from the result set.
func scanEntries(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]*Entry, error) {
	var entries []*Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit: iterate rows: %w", err)
	}
	if entries == nil {
		entries = []*Entry{}
	}
	return entries, nil
}

// scanEntry scans a single row into an Entry.
func scanEntry(row interface{ Scan(dest ...any) error }) (*Entry, error) {
	var e Entry
	var sessionID, userID, workspace, agentName, ns, query, ipAddr, userAgent, reason *string
	var resultCount *int
	var metadataJSON []byte

	if err := row.Scan(
		&e.ID, &e.Timestamp, &e.EventType,
		&sessionID, &userID, &workspace, &agentName, &ns,
		&query, &resultCount, &ipAddr, &userAgent, &reason, &metadataJSON,
	); err != nil {
		return nil, fmt.Errorf("audit: scan row: %w", err)
	}

	e.SessionID = derefString(sessionID)
	e.UserID = derefString(userID)
	e.Workspace = derefString(workspace)
	e.AgentName = derefString(agentName)
	e.Namespace = derefString(ns)
	e.Query = derefString(query)
	e.IPAddress = derefString(ipAddr)
	e.UserAgent = derefString(userAgent)
	e.Reason = derefString(reason)
	if resultCount != nil {
		e.ResultCount = *resultCount
	}
	if len(metadataJSON) > 0 {
		_ = json.Unmarshal(metadataJSON, &e.Metadata)
	}

	return &e, nil
}

// --- batch insert helpers ---------------------------------------------------

// buildBatchInsert constructs a multi-row INSERT statement for the given entries.
func buildBatchInsert(entries []*Entry) (string, []any) {
	const cols = 13
	values := make([]string, 0, len(entries))
	args := make([]any, 0, len(entries)*cols)

	for i, e := range entries {
		base := i * cols
		placeholders := make([]string, cols)
		for j := range cols {
			placeholders[j] = "$" + strconv.Itoa(base+j+1)
		}
		values = append(values, "("+strings.Join(placeholders, ", ")+")")

		var metadataJSON []byte
		if len(e.Metadata) > 0 {
			metadataJSON, _ = json.Marshal(e.Metadata)
		} else {
			metadataJSON = []byte("{}")
		}

		args = append(args,
			e.Timestamp, e.EventType,
			nullString(e.SessionID), nullString(e.UserID),
			nullString(e.Workspace), nullString(e.AgentName),
			nullString(e.Namespace), nullString(e.Query),
			nullInt(e.ResultCount), nullString(e.IPAddress),
			nullString(e.UserAgent), nullString(e.Reason),
			metadataJSON,
		)
	}

	query := `INSERT INTO audit_log (
		timestamp, event_type, session_id, user_id,
		workspace, agent_name, namespace, query,
		result_count, ip_address, user_agent, reason, metadata
	) VALUES ` + strings.Join(values, ", ")

	return query, args
}

// --- helpers ----------------------------------------------------------------

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullInt(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// queryBuilder is a minimal helper for building parameterized WHERE clauses.
type queryBuilder struct {
	clauses []string
	args    []any
}

func (qb *queryBuilder) add(clause string, arg any) {
	qb.args = append(qb.args, arg)
	qb.clauses = append(qb.clauses, strings.ReplaceAll(clause, "$?", "$"+strconv.Itoa(len(qb.args))))
}

func (qb *queryBuilder) where() string {
	if len(qb.clauses) == 0 {
		return ""
	}
	return " AND " + strings.Join(qb.clauses, " AND ")
}

func (qb *queryBuilder) appendPagination(query string, limit, offset int) string {
	if limit > 0 {
		qb.args = append(qb.args, limit)
		query += " LIMIT $" + strconv.Itoa(len(qb.args))
	}
	if offset > 0 {
		qb.args = append(qb.args, offset)
		query += " OFFSET $" + strconv.Itoa(len(qb.args))
	}
	return query
}
