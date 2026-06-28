/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// consentStringRows is a minimal pgx.Rows implementation for unit tests that
// returns a slice of strings. Only Close, Next, Scan and Err are called by
// ListUsersByConsent.
type consentStringRows struct {
	data    []string
	idx     int
	iterErr error
	scanErr error
}

func (m *consentStringRows) Close()                                       {}
func (m *consentStringRows) Err() error                                   { return m.iterErr }
func (m *consentStringRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (m *consentStringRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (m *consentStringRows) Values() ([]any, error)                       { return nil, nil }
func (m *consentStringRows) RawValues() [][]byte                          { return nil }
func (m *consentStringRows) Conn() *pgx.Conn                              { return nil }

func (m *consentStringRows) Next() bool {
	if m.idx < len(m.data) {
		m.idx++
		return true
	}
	return false
}

func (m *consentStringRows) Scan(dest ...any) error {
	if m.scanErr != nil {
		return m.scanErr
	}
	if len(dest) > 0 {
		*dest[0].(*string) = m.data[m.idx-1]
	}
	return nil
}

// consentQueryMockPool is a dbPool implementation with a queryFn hook for Query.
// Exec and QueryRow are no-ops; they are not used by ListUsersByConsent.
type consentQueryMockPool struct {
	queryFn func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (m *consentQueryMockPool) Exec(
	_ context.Context, _ string, _ ...any,
) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (m *consentQueryMockPool) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return &prefsMockRow{scanFn: func(_ ...any) error { return nil }}
}

func (m *consentQueryMockPool) Query(
	ctx context.Context, sql string, args ...any,
) (pgx.Rows, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, sql, args...)
	}
	return &consentStringRows{}, nil
}

// --- ListUsersByConsent unit tests ---

func TestListUsersByConsent_Granted_ReturnsMatchingUsers(t *testing.T) {
	pool := &consentQueryMockPool{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &consentStringRows{data: []string{"user-a", "user-b"}}, nil
		},
	}
	store := NewPreferencesStore(pool)
	ids, err := store.ListUsersByConsent(context.Background(), ConsentMemoryHealth, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"user-a", "user-b"}, ids)
}

func TestListUsersByConsent_NotGranted_ExcludesNoRowUsers(t *testing.T) {
	// granted=false: only users WITH a row who have NOT granted the category are
	// returned. The SQL WHERE clause naturally excludes users who have no row at
	// all, so this test verifies the returned set is exactly what the DB returns.
	pool := &consentQueryMockPool{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			// Simulate two users who have a row but did NOT grant memory:health.
			// A hypothetical "user-norow" with no preferences row never appears here.
			return &consentStringRows{data: []string{"user-c", "user-d"}}, nil
		},
	}
	store := NewPreferencesStore(pool)
	ids, err := store.ListUsersByConsent(context.Background(), ConsentMemoryHealth, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"user-c", "user-d"}, ids)
}

func TestListUsersByConsent_EmptyResult_ReturnsEmptySlice(t *testing.T) {
	pool := &consentQueryMockPool{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &consentStringRows{}, nil // no rows
		},
	}
	store := NewPreferencesStore(pool)
	ids, err := store.ListUsersByConsent(context.Background(), ConsentMemoryPreferences, true)
	require.NoError(t, err)
	assert.NotNil(t, ids)
	assert.Empty(t, ids)
}

func TestListUsersByConsent_InvalidCategory_Error(t *testing.T) {
	store := NewPreferencesStore(&consentQueryMockPool{})
	_, err := store.ListUsersByConsent(
		context.Background(), ConsentCategory("invalid:cat"), true,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown consent category")
}

func TestListUsersByConsent_QueryError(t *testing.T) {
	pool := &consentQueryMockPool{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return nil, errors.New("db down")
		},
	}
	store := NewPreferencesStore(pool)
	_, err := store.ListUsersByConsent(context.Background(), ConsentMemoryHealth, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}

func TestListUsersByConsent_ScanError(t *testing.T) {
	pool := &consentQueryMockPool{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &consentStringRows{data: []string{"u1"}, scanErr: errors.New("scan fail")}, nil
		},
	}
	store := NewPreferencesStore(pool)
	_, err := store.ListUsersByConsent(context.Background(), ConsentMemoryHealth, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scan fail")
}

func TestListUsersByConsent_IterError(t *testing.T) {
	pool := &consentQueryMockPool{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &consentStringRows{iterErr: errors.New("iter fail")}, nil
		},
	}
	store := NewPreferencesStore(pool)
	_, err := store.ListUsersByConsent(context.Background(), ConsentMemoryHealth, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "iter fail")
}
