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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreferencesStore_EnforcementStats(t *testing.T) {
	// Populated and empty both return cleanly; an all-zeros result is
	// valid data, not an error.
	cases := []struct {
		name             string
		blocked, redacts int64
	}{
		{name: "populated", blocked: 3, redacts: 7},
		{name: "empty", blocked: 0, redacts: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := &prefsMockPool{
				queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
					return &prefsMockRow{scanFn: func(dest ...any) error {
						*dest[0].(*int64) = tc.blocked
						*dest[1].(*int64) = tc.redacts
						return nil
					}}
				},
			}
			store := NewPreferencesStore(pool)
			stats, err := store.EnforcementStats(context.Background(), "ws")
			require.NoError(t, err)
			assert.Equal(t, tc.blocked, stats.PIIBlocked)
			assert.Equal(t, tc.redacts, stats.Redactions)
		})
	}
}

func TestPreferencesStore_EnforcementStats_QueryError(t *testing.T) {
	want := errors.New("db down")
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(_ ...any) error { return want }}
		},
	}
	store := NewPreferencesStore(pool)
	_, err := store.EnforcementStats(context.Background(), "ws")
	require.Error(t, err)
	require.ErrorIs(t, err, want)
}
