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
	"strings"
	"testing"
	"time"

	"github.com/altairalabs/omnia/ee/pkg/analytics"
)

// --- DDL tests ---

func TestSchemaDDL_MemoryEntitiesTable(t *testing.T) {
	ddl := SchemaDDL()
	// memory entities is at index 4
	stmt := ddl[4]
	expectedCols := []string{
		"id", "workspace_id", "virtual_user_id", "agent_id",
		"name", "kind", "source_type", "trust_model",
		"purpose", "forgotten", "created_at", "updated_at",
	}
	for _, col := range expectedCols {
		if !strings.Contains(stmt, col) {
			t.Errorf("memory_entities DDL missing column %q", col)
		}
	}
}

func TestSchemaDDL_MemoryObservationsTable(t *testing.T) {
	ddl := SchemaDDL()
	// memory observations is at index 5
	stmt := ddl[5]
	expectedCols := []string{
		"id", "entity_id", "content", "confidence",
		"source_type", "session_id", "observed_at", "created_at", "access_count",
	}
	for _, col := range expectedCols {
		if !strings.Contains(stmt, col) {
			t.Errorf("memory_observations DDL missing column %q", col)
		}
	}
}

func TestSchemaDDL_MemoryTables_CreateIfNotExists(t *testing.T) {
	ddl := SchemaDDL()
	for _, idx := range []int{4, 5} {
		if !strings.Contains(ddl[idx], "CREATE TABLE IF NOT EXISTS") {
			t.Errorf("DDL[%d] missing CREATE TABLE IF NOT EXISTS", idx)
		}
	}
}

// --- Sync happy path ---

func TestProvider_Sync_MemoryEntities(t *testing.T) {
	now := time.Now().UTC()
	entities := []analytics.MemoryEntityRow{
		{
			ID: "e1", WorkspaceID: "ws1", VirtualUserID: "u1",
			AgentID: "a1", Name: "Alice", Kind: "person",
			SourceType: "manual", TrustModel: "verified",
			Purpose: "contact", Forgotten: false,
			CreatedAt: now.Add(-1 * time.Hour), UpdatedAt: now,
		},
	}

	source := &MockSourceReader{
		ReadMemoryEntitiesFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.MemoryEntityRow, error) {
			return entities, nil
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
		Tables: []string{TableMemoryEntities},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalRows != 1 {
		t.Errorf("expected 1 row synced, got %d", result.TotalRows)
	}
	if len(result.Tables) != 1 {
		t.Fatalf("expected 1 table result, got %d", len(result.Tables))
	}
	if result.Tables[0].Table != TableMemoryEntities {
		t.Errorf("expected table %q, got %q", TableMemoryEntities, result.Tables[0].Table)
	}
	if result.Tables[0].Error != nil {
		t.Errorf("unexpected table error: %v", result.Tables[0].Error)
	}
}

func TestProvider_Sync_MemoryObservations(t *testing.T) {
	now := time.Now().UTC()
	observations := []analytics.MemoryObservationRow{
		{
			ID: "o1", EntityID: "e1", Content: "prefers Go",
			Confidence: 0.9, SourceType: "inferred",
			SessionID: "s1", ObservedAt: now.Add(-30 * time.Minute),
			CreatedAt: now.Add(-30 * time.Minute), AccessCount: 3,
		},
		{
			ID: "o2", EntityID: "e1", Content: "uses vim",
			Confidence: 0.7, SourceType: "manual",
			ObservedAt: now, CreatedAt: now, AccessCount: 1,
		},
	}

	source := &MockSourceReader{
		ReadMemoryObservationsFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.MemoryObservationRow, error) { //nolint:lll
			return observations, nil
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
		Tables: []string{TableMemoryObservations},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalRows != 2 {
		t.Errorf("expected 2 rows synced, got %d", result.TotalRows)
	}
	if result.Tables[0].Table != TableMemoryObservations {
		t.Errorf("expected table %q, got %q", TableMemoryObservations, result.Tables[0].Table)
	}
}

// --- Empty source ---

//nolint:dupl
func TestProvider_Sync_MemoryEntities_Empty(t *testing.T) {
	source := &MockSourceReader{
		ReadMemoryEntitiesFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.MemoryEntityRow, error) {
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
		Tables: []string{TableMemoryEntities},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalRows != 0 {
		t.Errorf("expected 0 rows, got %d", result.TotalRows)
	}
}

//nolint:dupl
func TestProvider_Sync_MemoryObservations_Empty(t *testing.T) {
	source := &MockSourceReader{
		ReadMemoryObservationsFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.MemoryObservationRow, error) { //nolint:lll
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
		Tables: []string{TableMemoryObservations},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalRows != 0 {
		t.Errorf("expected 0 rows, got %d", result.TotalRows)
	}
}

// --- Source read errors ---

func TestProvider_Sync_MemoryEntities_SourceError(t *testing.T) {
	source := &MockSourceReader{
		ReadMemoryEntitiesFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.MemoryEntityRow, error) {
			return nil, errors.New("memory entities read failed")
		},
	}
	assertSyncTableError(t, source, TableMemoryEntities)
}

func TestProvider_Sync_MemoryObservations_SourceError(t *testing.T) {
	source := &MockSourceReader{
		ReadMemoryObservationsFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.MemoryObservationRow, error) { //nolint:lll
			return nil, errors.New("memory observations read failed")
		},
	}
	assertSyncTableError(t, source, TableMemoryObservations)
}

// --- Upsert errors ---

func TestProvider_Sync_MemoryEntities_UpsertError(t *testing.T) {
	source := &MockSourceReader{
		ReadMemoryEntitiesFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.MemoryEntityRow, error) {
			return []analytics.MemoryEntityRow{{ID: "e1", UpdatedAt: time.Now()}}, nil
		},
	}
	assertSyncUpsertError(t, source, TableMemoryEntities)
}

func TestProvider_Sync_MemoryObservations_UpsertError(t *testing.T) {
	source := &MockSourceReader{
		ReadMemoryObservationsFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.MemoryObservationRow, error) { //nolint:lll
			return []analytics.MemoryObservationRow{{ID: "o1", CreatedAt: time.Now()}}, nil
		},
	}
	assertSyncUpsertError(t, source, TableMemoryObservations)
}

// --- Dry run ---

func TestProvider_Sync_MemoryEntities_DryRun(t *testing.T) {
	execCalled := false
	source := &MockSourceReader{
		ReadMemoryEntitiesFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.MemoryEntityRow, error) {
			return []analytics.MemoryEntityRow{{ID: "e1", UpdatedAt: time.Now()}}, nil
		},
	}
	mock := &MockDB{
		ExecFunc: func(_ context.Context, query string, _ ...any) (sql.Result, error) {
			if len(query) > 5 && query[:5] == "MERGE" {
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
		Tables: []string{TableMemoryEntities},
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

// --- Watermark tracking ---

//nolint:dupl
func TestProvider_Sync_MemoryEntities_WatermarkUsesUpdatedAt(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	entities := []analytics.MemoryEntityRow{
		{ID: "e1", UpdatedAt: t1},
		{ID: "e2", UpdatedAt: t2},
	}

	source := &MockSourceReader{
		ReadMemoryEntitiesFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.MemoryEntityRow, error) {
			return entities, nil
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
		Tables: []string{TableMemoryEntities},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Tables[0].WatermarkTo.Equal(t2) {
		t.Errorf("expected watermark %v, got %v", t2, result.Tables[0].WatermarkTo)
	}
}

//nolint:dupl
func TestProvider_Sync_MemoryObservations_WatermarkUsesCreatedAt(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	observations := []analytics.MemoryObservationRow{
		{ID: "o1", CreatedAt: t1},
		{ID: "o2", CreatedAt: t2},
	}

	source := &MockSourceReader{
		ReadMemoryObservationsFunc: func(_ context.Context, _ time.Time, _ int) ([]analytics.MemoryObservationRow, error) { //nolint:lll
			return observations, nil
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
		Tables: []string{TableMemoryObservations},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Tables[0].WatermarkTo.Equal(t2) {
		t.Errorf("expected watermark %v, got %v", t2, result.Tables[0].WatermarkTo)
	}
}
