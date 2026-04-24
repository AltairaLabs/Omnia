/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
*/

package privacy

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreferencesStore_Stats_Empty(t *testing.T) {
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(dest ...any) error {
				*dest[0].(*int64) = 0
				*dest[1].(*int64) = 0
				*dest[2].(*[]byte) = []byte("{}")
				return nil
			}}
		},
	}
	store := NewPreferencesStore(pool)
	stats, err := store.Stats(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.TotalUsers)
	assert.Equal(t, int64(0), stats.OptedOutAll)
	assert.Empty(t, stats.GrantsByCategory)
}

func TestPreferencesStore_Stats_PopulatedJSON(t *testing.T) {
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(dest ...any) error {
				*dest[0].(*int64) = 4
				*dest[1].(*int64) = 1
				*dest[2].(*[]byte) = []byte(`{"analytics:aggregate":1,"memory:context":2,"memory:health":1}`)
				return nil
			}}
		},
	}
	store := NewPreferencesStore(pool)
	stats, err := store.Stats(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(4), stats.TotalUsers)
	assert.Equal(t, int64(1), stats.OptedOutAll)
	assert.Equal(t, map[string]int64{
		"analytics:aggregate": 1,
		"memory:context":      2,
		"memory:health":       1,
	}, stats.GrantsByCategory)
}

func TestPreferencesStore_Stats_QueryError(t *testing.T) {
	want := errors.New("db down")
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(_ ...any) error { return want }}
		},
	}
	store := NewPreferencesStore(pool)
	_, err := store.Stats(context.Background())
	require.Error(t, err)
	require.ErrorIs(t, err, want)
}

func TestPreferencesStore_Stats_BadJSON(t *testing.T) {
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(dest ...any) error {
				*dest[0].(*int64) = 1
				*dest[1].(*int64) = 0
				*dest[2].(*[]byte) = []byte("not-json")
				return nil
			}}
		},
	}
	store := NewPreferencesStore(pool)
	_, err := store.Stats(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode")
}

func TestPreferencesStore_Stats_NilJSON_TreatedAsEmpty(t *testing.T) {
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(dest ...any) error {
				*dest[0].(*int64) = 0
				*dest[1].(*int64) = 0
				*dest[2].(*[]byte) = nil
				return nil
			}}
		},
	}
	store := NewPreferencesStore(pool)
	stats, err := store.Stats(context.Background())
	require.NoError(t, err)
	assert.Empty(t, stats.GrantsByCategory)
}
