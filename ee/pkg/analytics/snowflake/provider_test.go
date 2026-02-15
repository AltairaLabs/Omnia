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
	"errors"
	"testing"
	"time"

	"github.com/altairalabs/omnia/ee/pkg/analytics"
)

// --- Mock types ---

// MockRow implements the Row interface for testing.
type MockRow struct {
	ScanFunc func(dest ...any) error
}

func (r *MockRow) Scan(dest ...any) error { return r.ScanFunc(dest...) }

// MockResult implements sql.Result for testing.
type MockResult struct {
	lastInsertID int64
	rowsAffected int64
}

func (r MockResult) LastInsertId() (int64, error) { return r.lastInsertID, nil }
func (r MockResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

// MockDB implements the DB interface for testing.
type MockDB struct {
	ExecFunc     func(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryFunc    func(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowFunc func(ctx context.Context, query string, args ...any) Row
	PingFunc     func(ctx context.Context) error
	CloseFunc    func() error
}

func (m *MockDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if m.ExecFunc != nil {
		return m.ExecFunc(ctx, query, args...)
	}
	return MockResult{rowsAffected: 1}, nil
}

func (m *MockDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if m.QueryFunc != nil {
		return m.QueryFunc(ctx, query, args...)
	}
	return nil, nil
}

func (m *MockDB) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	if m.QueryRowFunc != nil {
		return m.QueryRowFunc(ctx, query, args...)
	}
	return &MockRow{ScanFunc: func(_ ...any) error { return sql.ErrNoRows }}
}

func (m *MockDB) PingContext(ctx context.Context) error {
	if m.PingFunc != nil {
		return m.PingFunc(ctx)
	}
	return nil
}

func (m *MockDB) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// MockSourceReader implements analytics.SourceReader for testing.
type MockSourceReader struct {
	ReadSessionsFunc    func(ctx context.Context, after time.Time, limit int) ([]analytics.SessionRow, error)
	ReadMessagesFunc    func(ctx context.Context, after time.Time, limit int) ([]analytics.MessageRow, error)
	ReadEvalResultsFunc func(ctx context.Context, after time.Time, limit int) ([]analytics.EvalResultRow, error)
}

func (m *MockSourceReader) ReadSessions(
	ctx context.Context, after time.Time, limit int,
) ([]analytics.SessionRow, error) {
	if m.ReadSessionsFunc != nil {
		return m.ReadSessionsFunc(ctx, after, limit)
	}
	return nil, nil
}

func (m *MockSourceReader) ReadMessages(
	ctx context.Context, after time.Time, limit int,
) ([]analytics.MessageRow, error) {
	if m.ReadMessagesFunc != nil {
		return m.ReadMessagesFunc(ctx, after, limit)
	}
	return nil, nil
}

func (m *MockSourceReader) ReadEvalResults(
	ctx context.Context, after time.Time, limit int,
) ([]analytics.EvalResultRow, error) {
	if m.ReadEvalResultsFunc != nil {
		return m.ReadEvalResultsFunc(ctx, after, limit)
	}
	return nil, nil
}

// --- Helper ---

func validConfig() *Config {
	return &Config{
		Account:          "test-account",
		User:             "test-user",
		Password:         "test-pass",
		Database:         "test-db",
		Warehouse:        "test-wh",
		Schema:           "PUBLIC",
		DefaultBatchSize: 100,
	}
}

// --- Tests ---

func TestNewProvider(t *testing.T) {
	cfg := validConfig()
	src := &MockSourceReader{}
	p := NewProvider(cfg, src)
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if p.config != cfg {
		t.Error("config not set")
	}
	if p.source != src {
		t.Error("source not set")
	}
}

func TestProvider_Init_Success(t *testing.T) {
	mock := &MockDB{}
	cfg := validConfig()
	p := newProviderWithDB(cfg, &MockSourceReader{}, mock)

	err := p.Init(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.inited {
		t.Error("expected inited to be true")
	}
}

func TestProvider_Init_InvalidConfig(t *testing.T) {
	cfg := &Config{} // missing required fields
	p := newProviderWithDB(cfg, &MockSourceReader{}, &MockDB{})

	err := p.Init(context.Background())
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestProvider_Init_PingFailure(t *testing.T) {
	mock := &MockDB{
		PingFunc: func(_ context.Context) error {
			return errors.New("connection refused")
		},
	}
	p := newProviderWithDB(validConfig(), &MockSourceReader{}, mock)

	err := p.Init(context.Background())
	if err == nil {
		t.Fatal("expected ping error")
	}
}

func TestProvider_Init_SchemaFailure(t *testing.T) {
	execCount := 0
	mock := &MockDB{
		ExecFunc: func(_ context.Context, _ string, _ ...any) (sql.Result, error) {
			execCount++
			if execCount == 2 {
				return nil, errors.New("schema creation failed")
			}
			return MockResult{rowsAffected: 1}, nil
		},
	}
	p := newProviderWithDB(validConfig(), &MockSourceReader{}, mock)

	err := p.Init(context.Background())
	if err == nil {
		t.Fatal("expected schema error")
	}
}

func TestProvider_Init_AlreadyClosed(t *testing.T) {
	p := newProviderWithDB(validConfig(), &MockSourceReader{}, &MockDB{})
	p.closed = true

	err := p.Init(context.Background())
	if !errors.Is(err, analytics.ErrAlreadyClosed) {
		t.Errorf("expected ErrAlreadyClosed, got %v", err)
	}
}

func TestProvider_Ping_Success(t *testing.T) {
	mock := &MockDB{}
	p := newProviderWithDB(validConfig(), &MockSourceReader{}, mock)
	p.inited = true

	err := p.Ping(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_Ping_NotInitialized(t *testing.T) {
	p := newProviderWithDB(validConfig(), &MockSourceReader{}, &MockDB{})

	err := p.Ping(context.Background())
	if !errors.Is(err, analytics.ErrNotInitialized) {
		t.Errorf("expected ErrNotInitialized, got %v", err)
	}
}

func TestProvider_Ping_Closed(t *testing.T) {
	p := newProviderWithDB(validConfig(), &MockSourceReader{}, &MockDB{})
	p.closed = true

	err := p.Ping(context.Background())
	if !errors.Is(err, analytics.ErrAlreadyClosed) {
		t.Errorf("expected ErrAlreadyClosed, got %v", err)
	}
}

func TestProvider_GetWatermark_Success(t *testing.T) {
	expected := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	mock := &MockDB{
		QueryRowFunc: func(_ context.Context, _ string, _ ...any) Row {
			return &MockRow{ScanFunc: func(dest ...any) error {
				*(dest[0].(*time.Time)) = expected
				return nil
			}}
		},
	}
	p := newProviderWithDB(validConfig(), &MockSourceReader{}, mock)
	p.inited = true

	wm, err := p.GetWatermark(context.Background(), TableSessions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !wm.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, wm)
	}
}

func TestProvider_GetWatermark_NotInitialized(t *testing.T) {
	p := newProviderWithDB(validConfig(), &MockSourceReader{}, &MockDB{})

	_, err := p.GetWatermark(context.Background(), TableSessions)
	if !errors.Is(err, analytics.ErrNotInitialized) {
		t.Errorf("expected ErrNotInitialized, got %v", err)
	}
}

func TestProvider_Sync_Sessions(t *testing.T) {
	now := time.Now().UTC()
	sessions := []analytics.SessionRow{
		{
			SessionID: "s1", WorkspaceID: "ws1", AgentID: "a1",
			Status: "active", Namespace: "default",
			CreatedAt: now.Add(-1 * time.Hour), UpdatedAt: now,
			MessageCount: 5, TotalInputTokens: 100, TotalOutputTokens: 200,
			Tags: []string{"prod"}, Metadata: map[string]string{"env": "prod"},
		},
	}

	source := &MockSourceReader{
		ReadSessionsFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.SessionRow, error) {
			return sessions, nil
		},
		ReadMessagesFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.MessageRow, error) {
			return nil, nil
		},
	}

	mock := &MockDB{
		QueryRowFunc: func(_ context.Context, _ string, _ ...any) Row {
			return &MockRow{ScanFunc: func(_ ...any) error { return sql.ErrNoRows }}
		},
	}

	p := newProviderWithDB(validConfig(), source, mock)
	p.inited = true

	result, err := p.Sync(context.Background(), analytics.SyncOpts{
		Tables: []string{TableSessions},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalRows != 1 {
		t.Errorf("expected 1 row synced, got %d", result.TotalRows)
	}
	if len(result.Tables) != 1 {
		t.Errorf("expected 1 table result, got %d", len(result.Tables))
	}
}

func TestProvider_Sync_Messages(t *testing.T) {
	now := time.Now().UTC()
	messages := []analytics.MessageRow{
		{
			MessageID: "m1", SessionID: "s1", Role: "user",
			Content: "hello", InputTokens: 10, CreatedAt: now,
		},
		{
			MessageID: "m2", SessionID: "s1", Role: "assistant",
			Content: "hi there", OutputTokens: 15, SequenceNum: 1, CreatedAt: now,
		},
	}

	source := &MockSourceReader{
		ReadMessagesFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.MessageRow, error) {
			return messages, nil
		},
	}

	mock := &MockDB{
		QueryRowFunc: func(_ context.Context, _ string, _ ...any) Row {
			return &MockRow{ScanFunc: func(_ ...any) error { return sql.ErrNoRows }}
		},
	}

	p := newProviderWithDB(validConfig(), source, mock)
	p.inited = true

	result, err := p.Sync(context.Background(), analytics.SyncOpts{
		Tables: []string{TableMessages},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalRows != 2 {
		t.Errorf("expected 2 rows synced, got %d", result.TotalRows)
	}
}

func TestProvider_Sync_EvalResults(t *testing.T) {
	now := time.Now().UTC()
	score := 0.95
	dur := 150
	evalResults := []analytics.EvalResultRow{
		{
			ID:             "e1",
			SessionID:      "s1",
			MessageID:      "m1",
			AgentName:      "agent-a",
			Namespace:      "default",
			PromptPackName: "pp1",
			EvalID:         "eval-a",
			EvalType:       "llm-judge",
			Trigger:        "on-message",
			Passed:         true,
			Score:          &score,
			Details:        `{"reason":"good"}`,
			DurationMs:     &dur,
			Source:         "runtime",
			CreatedAt:      now,
		},
	}

	source := &MockSourceReader{
		ReadEvalResultsFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.EvalResultRow, error) {
			return evalResults, nil
		},
	}

	mock := &MockDB{
		QueryRowFunc: func(_ context.Context, _ string, _ ...any) Row {
			return &MockRow{ScanFunc: func(_ ...any) error { return sql.ErrNoRows }}
		},
	}

	p := newProviderWithDB(validConfig(), source, mock)
	p.inited = true

	result, err := p.Sync(context.Background(), analytics.SyncOpts{
		Tables: []string{TableEvalResults},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalRows != 1 {
		t.Errorf("expected 1 row synced, got %d", result.TotalRows)
	}
	if len(result.Tables) != 1 {
		t.Errorf("expected 1 table result, got %d", len(result.Tables))
	}
	if result.Tables[0].Table != TableEvalResults {
		t.Errorf("expected table %q, got %q", TableEvalResults, result.Tables[0].Table)
	}
}

func TestProvider_Sync_EvalResults_Empty(t *testing.T) {
	source := &MockSourceReader{
		ReadEvalResultsFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.EvalResultRow, error) {
			return nil, nil
		},
	}
	mock := &MockDB{
		QueryRowFunc: func(_ context.Context, _ string, _ ...any) Row {
			return &MockRow{ScanFunc: func(_ ...any) error { return sql.ErrNoRows }}
		},
	}

	p := newProviderWithDB(validConfig(), source, mock)
	p.inited = true

	result, err := p.Sync(context.Background(), analytics.SyncOpts{
		Tables: []string{TableEvalResults},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalRows != 0 {
		t.Errorf("expected 0 rows, got %d", result.TotalRows)
	}
}

func TestProvider_Sync_EvalResults_SourceError(t *testing.T) {
	source := &MockSourceReader{
		ReadEvalResultsFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.EvalResultRow, error) {
			return nil, errors.New("eval source read failed")
		},
	}
	assertSyncTableError(t, source, TableEvalResults)
}

func TestProvider_Sync_EvalResults_UpsertError(t *testing.T) {
	source := &MockSourceReader{
		ReadEvalResultsFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.EvalResultRow, error) {
			return []analytics.EvalResultRow{{ID: "e1", CreatedAt: time.Now()}}, nil
		},
	}
	assertSyncUpsertError(t, source, TableEvalResults)
}

func TestMarshalEvalDetails(t *testing.T) {
	if got := marshalEvalDetails(""); got != "{}" {
		t.Errorf("empty details: got %q, want {}", got)
	}
	if got := marshalEvalDetails(`{"a":1}`); got != `{"a":1}` {
		t.Errorf("non-empty details: got %q, want {\"a\":1}", got)
	}
}

func TestProvider_Sync_DryRun(t *testing.T) {
	execCalled := false
	source := &MockSourceReader{
		ReadSessionsFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.SessionRow, error) {
			return []analytics.SessionRow{{SessionID: "s1", UpdatedAt: time.Now()}}, nil
		},
		ReadMessagesFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.MessageRow, error) {
			return nil, nil
		},
	}

	mock := &MockDB{
		ExecFunc: func(_ context.Context, query string, _ ...any) (sql.Result, error) {
			// Only MERGE queries indicate actual writes
			if len(query) > 10 && query[:5] == "MERGE" {
				execCalled = true
			}
			return MockResult{rowsAffected: 1}, nil
		},
		QueryRowFunc: func(_ context.Context, _ string, _ ...any) Row {
			return &MockRow{ScanFunc: func(_ ...any) error { return sql.ErrNoRows }}
		},
	}

	p := newProviderWithDB(validConfig(), source, mock)
	p.inited = true

	result, err := p.Sync(context.Background(), analytics.SyncOpts{
		Tables: []string{TableSessions},
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalRows != 1 {
		t.Errorf("expected 1 row counted in dry run, got %d", result.TotalRows)
	}
	if execCalled {
		t.Error("expected no MERGE exec calls in dry run mode")
	}
}

func TestProvider_Sync_NotInitialized(t *testing.T) {
	p := newProviderWithDB(validConfig(), &MockSourceReader{}, &MockDB{})

	_, err := p.Sync(context.Background(), analytics.SyncOpts{})
	if !errors.Is(err, analytics.ErrNotInitialized) {
		t.Errorf("expected ErrNotInitialized, got %v", err)
	}
}

func TestProvider_Sync_Closed(t *testing.T) {
	p := newProviderWithDB(validConfig(), &MockSourceReader{}, &MockDB{})
	p.closed = true

	_, err := p.Sync(context.Background(), analytics.SyncOpts{})
	if !errors.Is(err, analytics.ErrAlreadyClosed) {
		t.Errorf("expected ErrAlreadyClosed, got %v", err)
	}
}

func TestProvider_Sync_AllTables(t *testing.T) {
	source := &MockSourceReader{
		ReadSessionsFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.SessionRow, error) {
			return nil, nil
		},
		ReadMessagesFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.MessageRow, error) {
			return nil, nil
		},
		ReadEvalResultsFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.EvalResultRow, error) {
			return nil, nil
		},
	}
	mock := &MockDB{
		QueryRowFunc: func(_ context.Context, _ string, _ ...any) Row {
			return &MockRow{ScanFunc: func(_ ...any) error { return sql.ErrNoRows }}
		},
	}

	p := newProviderWithDB(validConfig(), source, mock)
	p.inited = true

	result, err := p.Sync(context.Background(), analytics.SyncOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Tables) != 3 {
		t.Errorf("expected 3 table results (all tables), got %d", len(result.Tables))
	}
}

// assertSyncTableError verifies that a source-read error is surfaced as a per-table error.
func assertSyncTableError(t *testing.T, source *MockSourceReader, table string) {
	t.Helper()
	mock := &MockDB{
		QueryRowFunc: func(_ context.Context, _ string, _ ...any) Row {
			return &MockRow{ScanFunc: func(_ ...any) error { return sql.ErrNoRows }}
		},
	}
	p := newProviderWithDB(validConfig(), source, mock)
	p.inited = true

	result, err := p.Sync(context.Background(), analytics.SyncOpts{
		Tables: []string{table},
	})
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	if result.Tables[0].Error == nil {
		t.Error("expected per-table error")
	}
}

// assertSyncUpsertError verifies that an upsert error is surfaced as a per-table error.
func assertSyncUpsertError(t *testing.T, source *MockSourceReader, table string) {
	t.Helper()
	mock := &MockDB{
		ExecFunc: func(_ context.Context, query string, _ ...any) (sql.Result, error) {
			if len(query) > 5 && query[:5] == "MERGE" {
				return nil, errors.New("upsert failed")
			}
			return MockResult{rowsAffected: 1}, nil
		},
		QueryRowFunc: func(_ context.Context, _ string, _ ...any) Row {
			return &MockRow{ScanFunc: func(_ ...any) error { return sql.ErrNoRows }}
		},
	}
	p := newProviderWithDB(validConfig(), source, mock)
	p.inited = true

	result, err := p.Sync(context.Background(), analytics.SyncOpts{
		Tables: []string{table},
	})
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	if result.Tables[0].Error == nil {
		t.Error("expected per-table error from upsert failure")
	}
}

func TestProvider_Sync_SourceReadError(t *testing.T) {
	source := &MockSourceReader{
		ReadSessionsFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.SessionRow, error) {
			return nil, errors.New("source read failed")
		},
	}
	assertSyncTableError(t, source, TableSessions)
}

func TestProvider_Sync_UpsertError(t *testing.T) {
	source := &MockSourceReader{
		ReadSessionsFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.SessionRow, error) {
			return []analytics.SessionRow{{SessionID: "s1", UpdatedAt: time.Now()}}, nil
		},
	}
	assertSyncUpsertError(t, source, TableSessions)
}

func TestProvider_Close(t *testing.T) {
	closeCalled := false
	mock := &MockDB{
		CloseFunc: func() error {
			closeCalled = true
			return nil
		},
	}
	p := newProviderWithDB(validConfig(), &MockSourceReader{}, mock)

	err := p.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !closeCalled {
		t.Error("expected db.Close to be called")
	}
	if !p.closed {
		t.Error("expected closed to be true")
	}
}

func TestProvider_Close_AlreadyClosed(t *testing.T) {
	p := newProviderWithDB(validConfig(), &MockSourceReader{}, &MockDB{})
	p.closed = true

	err := p.Close()
	if !errors.Is(err, analytics.ErrAlreadyClosed) {
		t.Errorf("expected ErrAlreadyClosed, got %v", err)
	}
}

func TestProvider_Close_NilDB(t *testing.T) {
	p := &Provider{config: validConfig()}

	err := p.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLatestTime(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	if got := latestTime(t1, t2); !got.Equal(t2) {
		t.Errorf("expected %v, got %v", t2, got)
	}
	if got := latestTime(t2, t1); !got.Equal(t2) {
		t.Errorf("expected %v, got %v", t2, got)
	}
	if got := latestTime(t1, t1); !got.Equal(t1) {
		t.Errorf("expected %v, got %v", t1, got)
	}
}

func TestFilterTables(t *testing.T) {
	tests := []struct {
		name      string
		requested []string
		available []string
		expected  int
	}{
		{"empty returns all", nil, AllTables, 3},
		{"filter sessions", []string{TableSessions}, AllTables, 1},
		{"filter eval_results", []string{TableEvalResults}, AllTables, 1},
		{"filter unknown", []string{"unknown"}, AllTables, 0},
		{"case insensitive", []string{"OMNIA_SESSIONS"}, AllTables, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterTables(tt.requested, tt.available)
			if len(result) != tt.expected {
				t.Errorf("expected %d tables, got %d: %v", tt.expected, len(result), result)
			}
		})
	}
}

func TestMarshalSessionJSON(t *testing.T) {
	row := &analytics.SessionRow{
		Tags:     []string{"a", "b"},
		Metadata: map[string]string{"k": "v"},
	}
	tags, meta := marshalSessionJSON(row)
	if tags != `["a","b"]` {
		t.Errorf("unexpected tags JSON: %s", tags)
	}
	if meta != `{"k":"v"}` {
		t.Errorf("unexpected metadata JSON: %s", meta)
	}
}

func TestMarshalSessionJSON_Empty(t *testing.T) {
	row := &analytics.SessionRow{}
	tags, meta := marshalSessionJSON(row)
	if tags != "[]" {
		t.Errorf("expected empty tags '[]', got %s", tags)
	}
	if meta != "{}" {
		t.Errorf("expected empty metadata '{}', got %s", meta)
	}
}

func TestProvider_Sync_DefaultBatchSize(t *testing.T) {
	source := &MockSourceReader{
		ReadSessionsFunc: func(_ context.Context, _ time.Time, limit int) ([]analytics.SessionRow, error) {
			if limit != 100 {
				t.Errorf("expected batch size 100, got %d", limit)
			}
			return nil, nil
		},
		ReadMessagesFunc: func(_ context.Context, _ time.Time, limit int) ([]analytics.MessageRow, error) {
			if limit != 100 {
				t.Errorf("expected batch size 100, got %d", limit)
			}
			return nil, nil
		},
		ReadEvalResultsFunc: func(_ context.Context, _ time.Time, limit int) ([]analytics.EvalResultRow, error) {
			if limit != 100 {
				t.Errorf("expected batch size 100, got %d", limit)
			}
			return nil, nil
		},
	}
	mock := &MockDB{
		QueryRowFunc: func(_ context.Context, _ string, _ ...any) Row {
			return &MockRow{ScanFunc: func(_ ...any) error { return sql.ErrNoRows }}
		},
	}

	p := newProviderWithDB(validConfig(), source, mock)
	p.inited = true

	_, err := p.Sync(context.Background(), analytics.SyncOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_Sync_CustomBatchSize(t *testing.T) {
	source := &MockSourceReader{
		ReadSessionsFunc: func(_ context.Context, _ time.Time, limit int) ([]analytics.SessionRow, error) {
			if limit != 500 {
				t.Errorf("expected batch size 500, got %d", limit)
			}
			return nil, nil
		},
	}
	mock := &MockDB{
		QueryRowFunc: func(_ context.Context, _ string, _ ...any) Row {
			return &MockRow{ScanFunc: func(_ ...any) error { return sql.ErrNoRows }}
		},
	}

	p := newProviderWithDB(validConfig(), source, mock)
	p.inited = true

	_, err := p.Sync(context.Background(), analytics.SyncOpts{
		Tables:    []string{TableSessions},
		BatchSize: 500,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
