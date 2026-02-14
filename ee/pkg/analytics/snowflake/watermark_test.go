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
)

func TestGetWatermark_Success(t *testing.T) {
	expected := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	mock := &MockDB{
		QueryRowFunc: func(_ context.Context, _ string, _ ...any) Row {
			return &MockRow{ScanFunc: func(dest ...any) error {
				*(dest[0].(*time.Time)) = expected
				return nil
			}}
		},
	}

	result, err := getWatermark(context.Background(), mock, TableSessions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestGetWatermark_NoRows(t *testing.T) {
	mock := &MockDB{
		QueryRowFunc: func(_ context.Context, _ string, _ ...any) Row {
			return &MockRow{ScanFunc: func(_ ...any) error {
				return sql.ErrNoRows
			}}
		},
	}

	result, err := getWatermark(context.Background(), mock, TableSessions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsZero() {
		t.Errorf("expected zero time, got %v", result)
	}
}

func TestGetWatermark_Error(t *testing.T) {
	dbErr := errors.New("connection failed")
	mock := &MockDB{
		QueryRowFunc: func(_ context.Context, _ string, _ ...any) Row {
			return &MockRow{ScanFunc: func(_ ...any) error {
				return dbErr
			}}
		},
	}

	_, err := getWatermark(context.Background(), mock, TableSessions)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, dbErr) {
		t.Errorf("expected %v, got %v", dbErr, err)
	}
}

func TestSetWatermark_Success(t *testing.T) {
	called := false
	mock := &MockDB{
		ExecFunc: func(_ context.Context, query string, args ...any) (sql.Result, error) {
			called = true
			if len(args) != 3 {
				t.Errorf("expected 3 args, got %d", len(args))
			}
			return MockResult{rowsAffected: 1}, nil
		},
	}

	err := setWatermark(context.Background(), mock, TableSessions, time.Now(), 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("ExecContext was not called")
	}
}

func TestSetWatermark_Error(t *testing.T) {
	dbErr := errors.New("write failed")
	mock := &MockDB{
		ExecFunc: func(_ context.Context, _ string, _ ...any) (sql.Result, error) {
			return nil, dbErr
		},
	}

	err := setWatermark(context.Background(), mock, TableSessions, time.Now(), 0)
	if !errors.Is(err, dbErr) {
		t.Errorf("expected %v, got %v", dbErr, err)
	}
}
