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

func newStatsHandlerMux(store *PreferencesPostgresStore) *http.ServeMux {
	mux := http.NewServeMux()
	NewConsentStatsHandler(store, logr.Discard()).RegisterRoutes(mux)
	return mux
}

func TestConsentStatsHandler_MissingWorkspace_400(t *testing.T) {
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(_ ...any) error { return nil }}
		},
	}
	mux := newStatsHandlerMux(NewPreferencesStore(pool))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/consent/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestConsentStatsHandler_HappyPath(t *testing.T) {
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(dest ...any) error {
				*dest[0].(*int64) = 1
				*dest[1].(*int64) = 0
				*dest[2].(*[]byte) = []byte(`{"analytics:aggregate":1}`)
				return nil
			}}
		},
	}
	mux := newStatsHandlerMux(NewPreferencesStore(pool))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/consent/stats?workspace=ws", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)

	var got ConsentStats
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, int64(1), got.TotalUsers)
	assert.Equal(t, map[string]int64{"analytics:aggregate": 1}, got.GrantsByCategory)
}

func TestConsentStatsHandler_StoreError_500(t *testing.T) {
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	mux := newStatsHandlerMux(NewPreferencesStore(pool))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/consent/stats?workspace=ws", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
