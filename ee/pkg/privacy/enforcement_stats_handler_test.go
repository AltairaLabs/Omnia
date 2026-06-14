/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
*/

package privacy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newEnforcementStatsMux(store *PreferencesPostgresStore) *http.ServeMux {
	mux := http.NewServeMux()
	NewEnforcementStatsHandler(store, logr.Discard()).RegisterRoutes(mux)
	return mux
}

func TestEnforcementStatsHandler_MissingWorkspace_400(t *testing.T) {
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(_ ...any) error { return nil }}
		},
	}
	mux := newEnforcementStatsMux(NewPreferencesStore(pool))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/enforcement-stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestEnforcementStatsHandler_OK(t *testing.T) {
	// Covers both the populated happy path and the empty-result case:
	// an empty store result is a valid 200 with zeros, not an error.
	cases := []struct {
		name             string
		blocked, redacts int64
	}{
		{name: "populated", blocked: 5, redacts: 9},
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
			mux := newEnforcementStatsMux(NewPreferencesStore(pool))

			r := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/enforcement-stats?workspace=ws", nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			require.Equal(t, http.StatusOK, w.Code)

			var got EnforcementStats
			require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
			assert.Equal(t, tc.blocked, got.PIIBlocked)
			assert.Equal(t, tc.redacts, got.Redactions)
		})
	}
}

func TestEnforcementStatsHandler_StoreError_500(t *testing.T) {
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	mux := newEnforcementStatsMux(NewPreferencesStore(pool))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/enforcement-stats?workspace=ws", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
