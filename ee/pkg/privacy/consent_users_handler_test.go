/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newConsentUsersMux(pool *consentQueryMockPool) *http.ServeMux {
	mux := http.NewServeMux()
	store := NewPreferencesStore(pool)
	NewConsentUsersHandler(store, logr.Discard()).RegisterRoutes(mux)
	return mux
}

func TestConsentUsersHandler_HappyPath(t *testing.T) {
	pool := &consentQueryMockPool{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &consentStringRows{data: []string{"u1", "u2"}}, nil
		},
	}
	mux := newConsentUsersMux(pool)

	r := httptest.NewRequest(http.MethodGet,
		"/api/v1/privacy/consent/users?category=memory:health&granted=true", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)

	var got consentUsersResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, "memory:health", got.Category)
	assert.True(t, got.Granted)
	assert.Equal(t, []string{"u1", "u2"}, got.UserIDs)
}

func TestConsentUsersHandler_NotGranted_HappyPath(t *testing.T) {
	pool := &consentQueryMockPool{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &consentStringRows{data: []string{"u3"}}, nil
		},
	}
	mux := newConsentUsersMux(pool)

	r := httptest.NewRequest(http.MethodGet,
		"/api/v1/privacy/consent/users?category=analytics:aggregate&granted=false", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK, w.Code)

	var got consentUsersResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, "analytics:aggregate", got.Category)
	assert.False(t, got.Granted)
	assert.Equal(t, []string{"u3"}, got.UserIDs)
}

func TestConsentUsersHandler_MissingCategory_400(t *testing.T) {
	mux := newConsentUsersMux(&consentQueryMockPool{})
	r := httptest.NewRequest(http.MethodGet,
		"/api/v1/privacy/consent/users?granted=true", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestConsentUsersHandler_InvalidCategory_400(t *testing.T) {
	mux := newConsentUsersMux(&consentQueryMockPool{})
	r := httptest.NewRequest(http.MethodGet,
		"/api/v1/privacy/consent/users?category=invalid:xyz&granted=true", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestConsentUsersHandler_MissingGranted_400(t *testing.T) {
	mux := newConsentUsersMux(&consentQueryMockPool{})
	r := httptest.NewRequest(http.MethodGet,
		"/api/v1/privacy/consent/users?category=memory:health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestConsentUsersHandler_InvalidGranted_400(t *testing.T) {
	mux := newConsentUsersMux(&consentQueryMockPool{})
	r := httptest.NewRequest(http.MethodGet,
		"/api/v1/privacy/consent/users?category=memory:health&granted=maybe", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestConsentUsersHandler_StoreError_500(t *testing.T) {
	pool := &consentQueryMockPool{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return nil, errors.New("db error")
		},
	}
	mux := newConsentUsersMux(pool)
	r := httptest.NewRequest(http.MethodGet,
		"/api/v1/privacy/consent/users?category=memory:health&granted=true", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
