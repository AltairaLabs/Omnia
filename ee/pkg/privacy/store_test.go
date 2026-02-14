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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// prefsMockRow implements pgx.Row for testing.
type prefsMockRow struct {
	scanFn func(dest ...any) error
}

func (r *prefsMockRow) Scan(dest ...any) error {
	return r.scanFn(dest...)
}

// prefsMockPool implements dbPool for testing preferences store.
type prefsMockPool struct {
	execFn     func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (m *prefsMockPool) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return m.execFn(ctx, sql, arguments...)
}

func (m *prefsMockPool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return m.queryRowFn(ctx, sql, args...)
}

func (m *prefsMockPool) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, nil
}

func TestGetPreferences_Found(t *testing.T) {
	now := time.Now().UTC()
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(dest ...any) error {
				*dest[0].(*bool) = true
				*dest[1].(*[]string) = []string{"ws1"}
				*dest[2].(*[]string) = []string{"agent1"}
				*dest[3].(*time.Time) = now
				*dest[4].(*time.Time) = now
				return nil
			}}
		},
	}

	store := NewPreferencesStore(pool)
	prefs, err := store.GetPreferences(context.Background(), "user1")
	require.NoError(t, err)
	assert.Equal(t, "user1", prefs.UserID)
	assert.True(t, prefs.OptOutAll)
	assert.Equal(t, []string{"ws1"}, prefs.OptOutWorkspaces)
	assert.Equal(t, []string{"agent1"}, prefs.OptOutAgents)
}

func TestGetPreferences_NotFound(t *testing.T) {
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}

	store := NewPreferencesStore(pool)
	_, err := store.GetPreferences(context.Background(), "missing")
	assert.ErrorIs(t, err, ErrPreferencesNotFound)
}

func TestGetPreferences_NilSlices(t *testing.T) {
	now := time.Now().UTC()
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(dest ...any) error {
				*dest[0].(*bool) = false
				*dest[1].(*[]string) = nil
				*dest[2].(*[]string) = nil
				*dest[3].(*time.Time) = now
				*dest[4].(*time.Time) = now
				return nil
			}}
		},
	}

	store := NewPreferencesStore(pool)
	prefs, err := store.GetPreferences(context.Background(), "user1")
	require.NoError(t, err)
	assert.NotNil(t, prefs.OptOutWorkspaces)
	assert.NotNil(t, prefs.OptOutAgents)
	assert.Empty(t, prefs.OptOutWorkspaces)
	assert.Empty(t, prefs.OptOutAgents)
}

func TestGetPreferences_DBError(t *testing.T) {
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(_ ...any) error {
				return errors.New("connection refused")
			}}
		},
	}

	store := NewPreferencesStore(pool)
	_, err := store.GetPreferences(context.Background(), "user1")
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrPreferencesNotFound)
}

func TestSetOptOut_All(t *testing.T) {
	pool := &prefsMockPool{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}

	store := NewPreferencesStore(pool)
	err := store.SetOptOut(context.Background(), "user1", ScopeAll, "")
	assert.NoError(t, err)
}

func TestSetOptOut_Workspace(t *testing.T) {
	pool := &prefsMockPool{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}

	store := NewPreferencesStore(pool)
	err := store.SetOptOut(context.Background(), "user1", ScopeWorkspace, "ws1")
	assert.NoError(t, err)
}

func TestSetOptOut_Agent(t *testing.T) {
	pool := &prefsMockPool{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}

	store := NewPreferencesStore(pool)
	err := store.SetOptOut(context.Background(), "user1", ScopeAgent, "agent1")
	assert.NoError(t, err)
}

func TestSetOptOut_InvalidScope(t *testing.T) {
	store := NewPreferencesStore(&prefsMockPool{})
	err := store.SetOptOut(context.Background(), "user1", "invalid", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid opt-out scope")
}

func TestRemoveOptOut_All(t *testing.T) {
	pool := &prefsMockPool{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}

	store := NewPreferencesStore(pool)
	err := store.RemoveOptOut(context.Background(), "user1", ScopeAll, "")
	assert.NoError(t, err)
}

func TestRemoveOptOut_All_NotFound(t *testing.T) {
	pool := &prefsMockPool{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		},
	}

	store := NewPreferencesStore(pool)
	err := store.RemoveOptOut(context.Background(), "missing", ScopeAll, "")
	assert.ErrorIs(t, err, ErrPreferencesNotFound)
}

func TestRemoveOptOut_Workspace(t *testing.T) {
	pool := &prefsMockPool{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}

	store := NewPreferencesStore(pool)
	err := store.RemoveOptOut(context.Background(), "user1", ScopeWorkspace, "ws1")
	assert.NoError(t, err)
}

func TestRemoveOptOut_Agent(t *testing.T) {
	pool := &prefsMockPool{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}

	store := NewPreferencesStore(pool)
	err := store.RemoveOptOut(context.Background(), "user1", ScopeAgent, "agent1")
	assert.NoError(t, err)
}

func TestRemoveOptOut_InvalidScope(t *testing.T) {
	store := NewPreferencesStore(&prefsMockPool{})
	err := store.RemoveOptOut(context.Background(), "user1", "invalid", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid opt-out scope")
}

func TestRemoveOptOut_DBError(t *testing.T) {
	pool := &prefsMockPool{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, errors.New("connection refused")
		},
	}

	store := NewPreferencesStore(pool)
	err := store.RemoveOptOut(context.Background(), "user1", ScopeWorkspace, "ws1")
	assert.Error(t, err)
}

func TestSetOptOut_ExecError(t *testing.T) {
	pool := &prefsMockPool{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, errors.New("connection refused")
		},
	}

	store := NewPreferencesStore(pool)
	err := store.SetOptOut(context.Background(), "user1", ScopeAll, "")
	assert.Error(t, err)
}
