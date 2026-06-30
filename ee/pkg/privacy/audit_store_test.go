/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/ee/pkg/audit"
)

// mockAuditPool is a dbPool that records the last Exec call so InsertEvents can
// be tested without a real database.
type mockAuditPool struct {
	lastQuery string
	lastArgs  []any
	tag       pgconn.CommandTag
	err       error
	execCalls int
}

func (m *mockAuditPool) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	m.execCalls++
	m.lastQuery = sql
	m.lastArgs = args
	return m.tag, m.err
}

func (m *mockAuditPool) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row { return nil }

func (m *mockAuditPool) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *mockAuditPool) Begin(_ context.Context) (pgx.Tx, error) { return nil, nil }

func TestAuditStore_InsertEvents_BuildsIdempotentBatch(t *testing.T) {
	m := &mockAuditPool{tag: pgconn.NewCommandTag("INSERT 0 2")}
	store := NewAuditStore(m)

	events := []*audit.Entry{
		{ID: 1, Timestamp: time.Now(), EventType: audit.EventMemoryWriteBlocked, Workspace: "ws", UserID: "u1"},
		{ID: 2, Timestamp: time.Now(), EventType: audit.EventPIIRedacted},
	}
	n, err := store.InsertEvents(context.Background(), "memory-api", events)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, 1, m.execCalls)
	require.Contains(t, m.lastQuery, "INSERT INTO audit_log")
	require.Contains(t, m.lastQuery, "ON CONFLICT (source_service, source_id) DO NOTHING")
	require.Len(t, m.lastArgs, 2*auditColumnsPerRow)
	require.Equal(t, "memory-api", m.lastArgs[0]) // source_service of the first row
}

func TestAuditStore_InsertEvents_EmptyAndNilAreNoOps(t *testing.T) {
	m := &mockAuditPool{}
	store := NewAuditStore(m)

	n, err := store.InsertEvents(context.Background(), "memory-api", nil)
	require.NoError(t, err)
	require.Equal(t, 0, n)

	n, err = store.InsertEvents(context.Background(), "memory-api", []*audit.Entry{nil, nil})
	require.NoError(t, err)
	require.Equal(t, 0, n)

	require.Equal(t, 0, m.execCalls, "no DB round-trip when there are no rows to insert")
}

func TestAuditStore_InsertEvents_RequiresSourceService(t *testing.T) {
	store := NewAuditStore(&mockAuditPool{})
	_, err := store.InsertEvents(context.Background(), "", []*audit.Entry{{ID: 1}})
	require.Error(t, err)
}

func TestNullIfEmpty(t *testing.T) {
	require.Nil(t, nullIfEmpty(""))
	require.Equal(t, "value", nullIfEmpty("value"))
}

func TestMarshalMetadata(t *testing.T) {
	b, err := marshalMetadata(nil)
	require.NoError(t, err)
	require.Equal(t, "{}", string(b))

	b, err = marshalMetadata(map[string]string{"category": "memory:health"})
	require.NoError(t, err)
	require.Contains(t, string(b), `"category":"memory:health"`)
}
