/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package audit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/altairalabs/omnia/ee/pkg/metrics"
	"github.com/altairalabs/omnia/internal/session/api"
)

// newTestLogger creates a Logger with no workers for unit testing.
func newTestLogger(bufSize int, m *metrics.AuditMetrics) *Logger {
	log := zap.New(zap.UseDevMode(true))
	l := NewLogger(nil, log, m, LoggerConfig{
		BufferSize: bufSize,
		Workers:    0,
	})
	close(l.stopCh)
	l.wg.Wait()
	l.buffer = make(chan *Entry, bufSize)
	return l
}

func TestLogEvent_EnqueuesEntry(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewAuditMetricsWithRegistry(reg)
	l := newTestLogger(10, m)

	l.LogEvent(context.Background(), &api.AuditEntry{
		EventType: "session_accessed",
		SessionID: "test-session-1",
		Workspace: "test-ws",
	})

	select {
	case entry := <-l.buffer:
		assert.Equal(t, "session_accessed", entry.EventType)
		assert.Equal(t, "test-session-1", entry.SessionID)
		assert.Equal(t, "test-ws", entry.Workspace)
		assert.False(t, entry.Timestamp.IsZero())
	default:
		t.Fatal("expected entry in buffer")
	}
}

func TestLogEvent_DropsWhenBufferFull(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewAuditMetricsWithRegistry(reg)
	l := newTestLogger(1, m)

	// Fill the buffer.
	l.LogEvent(context.Background(), &api.AuditEntry{EventType: "session_accessed"})

	// This one should be dropped.
	l.LogEvent(context.Background(), &api.AuditEntry{EventType: "session_accessed"})

	assert.Len(t, l.buffer, 1, "buffer should still have only 1 entry")
}

func TestLogEvent_NilMetrics(t *testing.T) {
	l := newTestLogger(10, nil)

	// Should not panic with nil metrics.
	l.LogEvent(context.Background(), &api.AuditEntry{
		EventType: "session_accessed",
		SessionID: "test-session-1",
	})

	select {
	case entry := <-l.buffer:
		assert.Equal(t, "session_accessed", entry.EventType)
	default:
		t.Fatal("expected entry in buffer")
	}
}

func TestEntryConversion(t *testing.T) {
	l := newTestLogger(10, nil)

	before := time.Now().UTC()

	l.LogEvent(context.Background(), &api.AuditEntry{
		EventType:   "session_searched",
		Workspace:   "prod",
		Query:       "kubernetes error",
		ResultCount: 5,
		IPAddress:   "10.0.0.1",
		UserAgent:   "Mozilla/5.0",
		Metadata:    map[string]string{"source": "dashboard"},
	})

	after := time.Now().UTC()

	entry := <-l.buffer
	assert.Equal(t, "session_searched", entry.EventType)
	assert.Equal(t, "prod", entry.Workspace)
	assert.Equal(t, "kubernetes error", entry.Query)
	assert.Equal(t, 5, entry.ResultCount)
	assert.Equal(t, "10.0.0.1", entry.IPAddress)
	assert.Equal(t, "Mozilla/5.0", entry.UserAgent)
	assert.Equal(t, "dashboard", entry.Metadata["source"])
	assert.True(t, entry.Timestamp.After(before) || entry.Timestamp.Equal(before))
	assert.True(t, entry.Timestamp.Before(after) || entry.Timestamp.Equal(after))
}

func TestLogEvent_AllFields(t *testing.T) {
	l := newTestLogger(10, nil)

	l.LogEvent(context.Background(), &api.AuditEntry{
		EventType:   "session_accessed",
		SessionID:   "sess-1",
		Workspace:   "ws-1",
		AgentName:   "agent-1",
		Namespace:   "ns-1",
		Query:       "test query",
		ResultCount: 10,
		IPAddress:   "192.168.1.1",
		UserAgent:   "test-agent",
		Metadata:    map[string]string{"key": "value"},
	})

	entry := <-l.buffer
	assert.Equal(t, "sess-1", entry.SessionID)
	assert.Equal(t, "ws-1", entry.Workspace)
	assert.Equal(t, "agent-1", entry.AgentName)
	assert.Equal(t, "ns-1", entry.Namespace)
	assert.Equal(t, "test query", entry.Query)
	assert.Equal(t, 10, entry.ResultCount)
	assert.Equal(t, "192.168.1.1", entry.IPAddress)
	assert.Equal(t, "test-agent", entry.UserAgent)
	assert.Equal(t, map[string]string{"key": "value"}, entry.Metadata)
}

func TestNullHelpers(t *testing.T) {
	assert.Nil(t, nullString(""))
	assert.Equal(t, "hello", *nullString("hello"))

	assert.Nil(t, nullInt(0))
	assert.Equal(t, 42, *nullInt(42))

	assert.Equal(t, "", derefString(nil))
	s := "test"
	assert.Equal(t, "test", derefString(&s))
}

func TestQueryBuilder(t *testing.T) {
	qb := &queryBuilder{}
	assert.Equal(t, "", qb.where())

	qb.add("session_id = $?", "abc")
	qb.add("workspace = $?", "prod")
	assert.Equal(t, " AND session_id = $1 AND workspace = $2", qb.where())
	assert.Equal(t, []any{"abc", "prod"}, qb.args)

	query := qb.appendPagination("SELECT * FROM t WHERE 1=1"+qb.where(), 10, 5)
	assert.Contains(t, query, "LIMIT $3")
	assert.Contains(t, query, "OFFSET $4")
}

func TestBuildQueryFilters(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Hour)

	tests := []struct {
		name        string
		opts        QueryOpts
		wantClauses int
	}{
		{"empty", QueryOpts{}, 0},
		{"session_id", QueryOpts{SessionID: "abc"}, 1},
		{"user_id", QueryOpts{UserID: "user1"}, 1},
		{"workspace", QueryOpts{Workspace: "ws"}, 1},
		{"event_types", QueryOpts{EventTypes: []string{"session_accessed"}}, 1},
		{"from", QueryOpts{From: now}, 1},
		{"to", QueryOpts{To: later}, 1},
		{"all_filters", QueryOpts{
			SessionID:  "abc",
			UserID:     "user1",
			Workspace:  "ws",
			EventTypes: []string{"session_accessed", "session_searched"},
			From:       now,
			To:         later,
		}, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := buildQueryFilters(tt.opts)
			assert.Len(t, qb.clauses, tt.wantClauses)
			assert.Len(t, qb.args, tt.wantClauses)
		})
	}
}

func TestBuildBatchInsert(t *testing.T) {
	now := time.Now()
	entries := []*Entry{
		{
			Timestamp: now,
			EventType: "session_accessed",
			SessionID: "sess-1",
			Workspace: "ws-1",
		},
		{
			Timestamp: now,
			EventType: "session_searched",
			Query:     "test",
			Metadata:  map[string]string{"key": "val"},
		},
	}

	query, args := buildBatchInsert(entries)
	assert.Contains(t, query, "INSERT INTO audit_log")
	assert.Contains(t, query, "$1")
	assert.Contains(t, query, "$26") // 2 entries * 13 cols = 26 params
	assert.Len(t, args, 26)
}

func TestBuildBatchInsert_SingleEntry(t *testing.T) {
	entry := &Entry{
		Timestamp: time.Now(),
		EventType: "session_accessed",
	}

	query, args := buildBatchInsert([]*Entry{entry})
	assert.Contains(t, query, "INSERT INTO audit_log")
	assert.Len(t, args, 13)
}

func TestBuildBatchInsert_WithMetadata(t *testing.T) {
	entry := &Entry{
		Timestamp: time.Now(),
		EventType: "session_accessed",
		Metadata:  map[string]string{"source": "dashboard"},
	}

	_, args := buildBatchInsert([]*Entry{entry})
	// Last arg should be the metadata JSON.
	metadataJSON := args[12].([]byte)
	assert.Contains(t, string(metadataJSON), "dashboard")
}

func TestBuildBatchInsert_EmptyMetadata(t *testing.T) {
	entry := &Entry{
		Timestamp: time.Now(),
		EventType: "session_accessed",
	}

	_, args := buildBatchInsert([]*Entry{entry})
	metadataJSON := args[12].([]byte)
	assert.Equal(t, "{}", string(metadataJSON))
}

func TestNewLogger_DefaultConfig(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	l := NewLogger(nil, log, nil, LoggerConfig{})
	require.NotNil(t, l)
	assert.Equal(t, DefaultBufferSize, l.cfg.BufferSize)
	assert.Equal(t, DefaultWorkers, l.cfg.Workers)
	assert.Equal(t, DefaultBatchSize, l.cfg.BatchSize)
	assert.Equal(t, DefaultFlushInterval, l.cfg.FlushInterval)
	require.NoError(t, l.Close())
}

func TestNewLogger_CustomConfig(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	l := NewLogger(nil, log, nil, LoggerConfig{
		BufferSize:    100,
		Workers:       1,
		BatchSize:     10,
		FlushInterval: time.Second,
	})
	require.NotNil(t, l)
	assert.Equal(t, 100, l.cfg.BufferSize)
	assert.Equal(t, 1, l.cfg.Workers)
	assert.Equal(t, 10, l.cfg.BatchSize)
	assert.Equal(t, time.Second, l.cfg.FlushInterval)
	require.NoError(t, l.Close())
}

func TestClose_DrainsPendingEntries(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	// Create a logger with workers but no pool — writeBatch will fail but Close should still drain.
	l := NewLogger(nil, log, nil, LoggerConfig{
		BufferSize:    10,
		Workers:       1,
		BatchSize:     100,
		FlushInterval: time.Hour, // long interval so timer doesn't flush
	})

	// Enqueue an entry.
	l.LogEvent(context.Background(), &api.AuditEntry{
		EventType: "session_accessed",
	})

	// Close should complete without hanging.
	err := l.Close()
	require.NoError(t, err)
}

func TestFlushBatch_EmptyBatch(t *testing.T) {
	l := newTestLogger(1, nil)
	// Should not panic.
	l.flushBatch(nil)
	l.flushBatch([]*Entry{})
}

// mockRow implements the Scan interface for testing scanEntry.
type mockRow struct {
	values []any
	err    error
}

func (m *mockRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	// Copy values to dest pointers.
	for i, v := range m.values {
		switch d := dest[i].(type) {
		case *int64:
			*d = v.(int64)
		case *time.Time:
			*d = v.(time.Time)
		case *string:
			*d = v.(string)
		case **string:
			if v == nil {
				*d = nil
			} else {
				s := v.(string)
				*d = &s
			}
		case **int:
			if v == nil {
				*d = nil
			} else {
				n := v.(int)
				*d = &n
			}
		case *[]byte:
			if v == nil {
				*d = nil
			} else {
				*d = v.([]byte)
			}
		}
	}
	return nil
}

func TestScanEntry_OK(t *testing.T) {
	now := time.Now()
	row := &mockRow{
		values: []any{
			int64(1), now, "session_accessed",
			"sess-1", "user-1", "ws-1", "agent-1", "ns-1",
			"test query", 5, "10.0.0.1", "TestBrowser", "compliance",
			[]byte(`{"key":"val"}`),
		},
	}

	entry, err := scanEntry(row)
	require.NoError(t, err)
	assert.Equal(t, int64(1), entry.ID)
	assert.Equal(t, now, entry.Timestamp)
	assert.Equal(t, "session_accessed", entry.EventType)
	assert.Equal(t, "sess-1", entry.SessionID)
	assert.Equal(t, "user-1", entry.UserID)
	assert.Equal(t, "ws-1", entry.Workspace)
	assert.Equal(t, "agent-1", entry.AgentName)
	assert.Equal(t, "ns-1", entry.Namespace)
	assert.Equal(t, "test query", entry.Query)
	assert.Equal(t, 5, entry.ResultCount)
	assert.Equal(t, "10.0.0.1", entry.IPAddress)
	assert.Equal(t, "TestBrowser", entry.UserAgent)
	assert.Equal(t, "compliance", entry.Reason)
	assert.Equal(t, "val", entry.Metadata["key"])
}

func TestScanEntry_NilFields(t *testing.T) {
	now := time.Now()
	row := &mockRow{
		values: []any{
			int64(1), now, "session_accessed",
			nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil,
			nil,
		},
	}

	entry, err := scanEntry(row)
	require.NoError(t, err)
	assert.Equal(t, "", entry.SessionID)
	assert.Equal(t, "", entry.UserID)
	assert.Equal(t, 0, entry.ResultCount)
	assert.Nil(t, entry.Metadata)
}

func TestScanEntry_Error(t *testing.T) {
	row := &mockRow{err: fmt.Errorf("scan failed")}
	_, err := scanEntry(row)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scan row")
}

// mockRows implements the rows interface for testing scanEntries.
type mockRows struct {
	rows []*mockRow
	idx  int
	err  error
}

func (m *mockRows) Next() bool {
	return m.idx < len(m.rows)
}

func (m *mockRows) Scan(dest ...any) error {
	row := m.rows[m.idx]
	m.idx++
	return row.Scan(dest...)
}

func (m *mockRows) Err() error {
	return m.err
}

func TestScanEntries_OK(t *testing.T) {
	now := time.Now()
	rows := &mockRows{
		rows: []*mockRow{
			{values: []any{
				int64(1), now, "session_accessed",
				"sess-1", nil, nil, nil, nil,
				nil, nil, nil, nil, nil, nil,
			}},
			{values: []any{
				int64(2), now, "session_searched",
				nil, nil, "ws-1", nil, nil,
				"query", nil, nil, nil, nil, nil,
			}},
		},
	}

	entries, err := scanEntries(rows)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	assert.Equal(t, "session_accessed", entries[0].EventType)
	assert.Equal(t, "session_searched", entries[1].EventType)
}

func TestScanEntries_Empty(t *testing.T) {
	rows := &mockRows{}
	entries, err := scanEntries(rows)
	require.NoError(t, err)
	assert.NotNil(t, entries)
	assert.Empty(t, entries)
}

func TestScanEntries_IterError(t *testing.T) {
	rows := &mockRows{err: fmt.Errorf("iteration error")}
	_, err := scanEntries(rows)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "iterate rows")
}

func TestWriteBatch_NilPool(t *testing.T) {
	l := newTestLogger(1, nil)
	// Should not panic with nil pool.
	l.writeBatch([]*Entry{{EventType: "test"}})
}

func TestWriteBatch_NilPoolWithMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewAuditMetricsWithRegistry(reg)
	l := newTestLogger(1, m)
	// Should record metrics but not panic with nil pool.
	l.writeBatch([]*Entry{{EventType: "test"}})
}

func TestWriteBatch_Empty(t *testing.T) {
	l := newTestLogger(1, nil)
	// Should not panic.
	l.writeBatch(nil)
	l.writeBatch([]*Entry{})
}

func TestWorker_StopDrainsBuffer(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	l := NewLogger(nil, log, nil, LoggerConfig{
		BufferSize:    10,
		Workers:       1,
		BatchSize:     100,
		FlushInterval: time.Hour,
	})

	// Enqueue several entries.
	for i := range 5 {
		l.LogEvent(context.Background(), &api.AuditEntry{
			EventType: "session_accessed",
			SessionID: fmt.Sprintf("sess-%d", i),
		})
	}

	// Close should drain all entries without hanging.
	require.NoError(t, l.Close())
}

func TestWorker_BatchFlushOnSize(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	l := NewLogger(nil, log, nil, LoggerConfig{
		BufferSize:    100,
		Workers:       1,
		BatchSize:     2, // small batch to trigger batch-size flush
		FlushInterval: time.Hour,
	})

	// Enqueue enough to trigger batch flush.
	for i := range 4 {
		l.LogEvent(context.Background(), &api.AuditEntry{
			EventType: "session_accessed",
			SessionID: fmt.Sprintf("sess-%d", i),
		})
	}

	// Give worker time to process.
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, l.Close())
}

func TestWorker_TickerFlush(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	l := NewLogger(nil, log, nil, LoggerConfig{
		BufferSize:    10,
		Workers:       1,
		BatchSize:     100,                   // large batch so it won't flush by size
		FlushInterval: 10 * time.Millisecond, // short interval to trigger ticker
	})

	l.LogEvent(context.Background(), &api.AuditEntry{
		EventType: "session_accessed",
	})

	// Wait for ticker to flush.
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, l.Close())
}

func TestDrainBuffer_EmptyBuffer(t *testing.T) {
	l := newTestLogger(10, nil)
	// drainBuffer on empty buffer should return immediately.
	batch := l.drainBuffer(nil)
	assert.Empty(t, batch)
}

// --- mock dbPool for testing writeBatch and Query ---

type mockDBPool struct {
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
	queryFunc    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (m *mockDBPool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, sql, args...)
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (m *mockDBPool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, sql, args...)
	}
	return &mockPgxRow{val: int64(0)}
}

func (m *mockDBPool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, sql, args...)
	}
	return &mockPgxRows{}, nil
}

// mockPgxRow implements pgx.Row for count queries.
type mockPgxRow struct {
	val int64
	err error
}

func (r *mockPgxRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if p, ok := dest[0].(*int64); ok {
		*p = r.val
	}
	return nil
}

// mockPgxRows implements pgx.Rows for data queries.
type mockPgxRows struct{}

func (r *mockPgxRows) Close()                                       {}
func (r *mockPgxRows) Err() error                                   { return nil }
func (r *mockPgxRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *mockPgxRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *mockPgxRows) Next() bool                                   { return false }
func (r *mockPgxRows) Scan(_ ...any) error                          { return nil }
func (r *mockPgxRows) Values() ([]any, error)                       { return nil, nil }
func (r *mockPgxRows) RawValues() [][]byte                          { return nil }
func (r *mockPgxRows) Conn() *pgx.Conn                              { return nil }

func TestWriteBatch_WithMockPool(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewAuditMetricsWithRegistry(reg)
	log := zap.New(zap.UseDevMode(true))

	var capturedSQL string
	pool := &mockDBPool{
		execFunc: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			capturedSQL = sql
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}

	l := &Logger{
		pool:    pool,
		buffer:  make(chan *Entry, 10),
		stopCh:  make(chan struct{}),
		metrics: m,
		log:     log,
		cfg:     LoggerConfig{BatchSize: 10},
	}

	l.writeBatch([]*Entry{
		{Timestamp: time.Now(), EventType: "session_accessed", SessionID: "sess-1"},
	})

	assert.Contains(t, capturedSQL, "INSERT INTO audit_log")
}

func TestWriteBatch_ExecError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewAuditMetricsWithRegistry(reg)
	log := zap.New(zap.UseDevMode(true))

	pool := &mockDBPool{
		execFunc: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, fmt.Errorf("exec failed")
		},
	}

	l := &Logger{
		pool:    pool,
		buffer:  make(chan *Entry, 10),
		stopCh:  make(chan struct{}),
		metrics: m,
		log:     log,
		cfg:     LoggerConfig{BatchSize: 10},
	}

	l.writeBatch([]*Entry{
		{Timestamp: time.Now(), EventType: "session_accessed"},
	})

	// Verify writeErrors metric was incremented.
	counter, err := m.WriteErrors.GetMetricWithLabelValues("session_accessed")
	require.NoError(t, err)
	metric := &dto.Metric{}
	require.NoError(t, counter.Write(metric))
	assert.Equal(t, float64(1), metric.GetCounter().GetValue())
}

func TestQuery_OK(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewAuditMetricsWithRegistry(reg)
	log := zap.New(zap.UseDevMode(true))

	pool := &mockDBPool{
		queryRowFunc: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockPgxRow{val: 0}
		},
		queryFunc: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockPgxRows{}, nil
		},
	}

	l := &Logger{
		pool:    pool,
		buffer:  make(chan *Entry, 10),
		stopCh:  make(chan struct{}),
		metrics: m,
		log:     log,
		cfg:     LoggerConfig{BatchSize: 10},
	}

	result, err := l.Query(context.Background(), QueryOpts{
		SessionID: "sess-1",
		Limit:     10,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(0), result.Total)
	assert.Empty(t, result.Entries)
	assert.False(t, result.HasMore)
}

func TestQuery_CountError(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))

	pool := &mockDBPool{
		queryRowFunc: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockPgxRow{err: fmt.Errorf("count failed")}
		},
	}

	l := &Logger{
		pool: pool,
		log:  log,
		cfg:  LoggerConfig{BatchSize: 10},
	}

	_, err := l.Query(context.Background(), QueryOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "count query")
}

func TestQuery_DataError(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))

	pool := &mockDBPool{
		queryRowFunc: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockPgxRow{val: 5}
		},
		queryFunc: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return nil, fmt.Errorf("query failed")
		},
	}

	l := &Logger{
		pool: pool,
		log:  log,
		cfg:  LoggerConfig{BatchSize: 10},
	}

	_, err := l.Query(context.Background(), QueryOpts{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "data query")
}

func TestQuery_NilMetrics(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))

	pool := &mockDBPool{
		queryRowFunc: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockPgxRow{val: 0}
		},
		queryFunc: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockPgxRows{}, nil
		},
	}

	l := &Logger{
		pool: pool,
		log:  log,
		cfg:  LoggerConfig{BatchSize: 10},
	}

	result, err := l.Query(context.Background(), QueryOpts{Limit: 10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

// --- retention tests ---

func TestNewLogger_RetentionDaysZero(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	l := NewLogger(nil, log, nil, LoggerConfig{RetentionDays: 0})
	require.NotNil(t, l)
	assert.Equal(t, 0, l.cfg.RetentionDays)
	require.NoError(t, l.Close())
}

func TestNewLogger_RetentionDaysSet(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	l := NewLogger(nil, log, nil, LoggerConfig{RetentionDays: 90})
	require.NotNil(t, l)
	assert.Equal(t, 90, l.cfg.RetentionDays)
	require.NoError(t, l.Close())
}

func TestDeleteExpiredEntries_NilPool(t *testing.T) {
	l := newTestLogger(1, nil)
	l.cfg.RetentionDays = 30
	// Should not panic with nil pool.
	l.deleteExpiredEntries()
}

func TestDeleteExpiredEntries_ZeroRetention(t *testing.T) {
	l := newTestLogger(1, nil)
	l.cfg.RetentionDays = 0
	// Should return early with zero retention.
	l.deleteExpiredEntries()
}

func TestDeleteExpiredEntries_WithMockPool(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	var capturedSQL string
	pool := &mockDBPool{
		execFunc: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			capturedSQL = sql
			return pgconn.NewCommandTag("DELETE 5"), nil
		},
	}
	l := &Logger{
		pool:   pool,
		buffer: make(chan *Entry, 10),
		stopCh: make(chan struct{}),
		log:    log,
		cfg:    LoggerConfig{RetentionDays: 30, BatchSize: 10},
	}
	l.deleteExpiredEntries()
	assert.Contains(t, capturedSQL, "DELETE FROM audit_log")
}

func TestDeleteExpiredEntries_ExecError(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	pool := &mockDBPool{
		execFunc: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, fmt.Errorf("exec failed")
		},
	}
	l := &Logger{
		pool:   pool,
		buffer: make(chan *Entry, 10),
		stopCh: make(chan struct{}),
		log:    log,
		cfg:    LoggerConfig{RetentionDays: 30, BatchSize: 10},
	}
	// Should not panic on error.
	l.deleteExpiredEntries()
}

func TestQuery_LimitClamp(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))

	pool := &mockDBPool{
		queryRowFunc: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockPgxRow{val: 0}
		},
		queryFunc: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockPgxRows{}, nil
		},
	}

	l := &Logger{pool: pool, log: log, cfg: LoggerConfig{BatchSize: 10}}

	// Limit 0 should be clamped to 1 (then to min(1,500)=1).
	result, err := l.Query(context.Background(), QueryOpts{Limit: 0})
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Limit >500 should be clamped to 500.
	result, err = l.Query(context.Background(), QueryOpts{Limit: 1000})
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Negative offset should be clamped to 0.
	result, err = l.Query(context.Background(), QueryOpts{Offset: -10})
	require.NoError(t, err)
	assert.NotNil(t, result)
}
