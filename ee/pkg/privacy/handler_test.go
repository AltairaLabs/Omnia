/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// handlerMockPrefsStore is a mock PreferencesStore for handler tests.
type handlerMockPrefsStore struct {
	getPreferencesFn func(ctx context.Context, userID string) (*Preferences, error)
	setOptOutFn      func(ctx context.Context, userID, scope, target string) error
	removeOptOutFn   func(ctx context.Context, userID, scope, target string) error
}

func (m *handlerMockPrefsStore) GetPreferences(ctx context.Context, userID string) (*Preferences, error) {
	return m.getPreferencesFn(ctx, userID)
}

func (m *handlerMockPrefsStore) SetOptOut(ctx context.Context, userID, scope, target string) error {
	return m.setOptOutFn(ctx, userID, scope, target)
}

func (m *handlerMockPrefsStore) RemoveOptOut(ctx context.Context, userID, scope, target string) error {
	return m.removeOptOutFn(ctx, userID, scope, target)
}

func newTestOptOutHandler(store PreferencesStore) *OptOutHandler {
	log := zap.New(zap.UseDevMode(true))
	return NewOptOutHandler(store, log)
}

func TestHandleSetOptOut_Success(t *testing.T) {
	store := &handlerMockPrefsStore{
		setOptOutFn: func(_ context.Context, _, _, _ string) error {
			return nil
		},
	}
	h := newTestOptOutHandler(store)

	body, _ := json.Marshal(OptOutRequest{UserID: "user1", Scope: ScopeAll})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/opt-out", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.handleSetOptOut(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandleSetOptOut_WorkspaceScope(t *testing.T) {
	store := &handlerMockPrefsStore{
		setOptOutFn: func(_ context.Context, _, scope, target string) error {
			assert.Equal(t, ScopeWorkspace, scope)
			assert.Equal(t, "my-workspace", target)
			return nil
		},
	}
	h := newTestOptOutHandler(store)

	body, _ := json.Marshal(OptOutRequest{
		UserID: "user1", Scope: ScopeWorkspace, Target: "my-workspace",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/opt-out", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.handleSetOptOut(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandleSetOptOut_InvalidBody(t *testing.T) {
	h := newTestOptOutHandler(&handlerMockPrefsStore{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/opt-out", bytes.NewReader([]byte("invalid")))
	rec := httptest.NewRecorder()

	h.handleSetOptOut(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleSetOptOut_MissingUserID(t *testing.T) {
	h := newTestOptOutHandler(&handlerMockPrefsStore{})

	body, _ := json.Marshal(OptOutRequest{Scope: ScopeAll})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/opt-out", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.handleSetOptOut(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleSetOptOut_InvalidScope(t *testing.T) {
	h := newTestOptOutHandler(&handlerMockPrefsStore{})

	body, _ := json.Marshal(OptOutRequest{UserID: "user1", Scope: "invalid"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/opt-out", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.handleSetOptOut(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleSetOptOut_MissingTarget(t *testing.T) {
	h := newTestOptOutHandler(&handlerMockPrefsStore{})

	body, _ := json.Marshal(OptOutRequest{UserID: "user1", Scope: ScopeWorkspace})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/opt-out", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.handleSetOptOut(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleSetOptOut_StoreError(t *testing.T) {
	store := &handlerMockPrefsStore{
		setOptOutFn: func(_ context.Context, _, _, _ string) error {
			return errors.New("db error")
		},
	}
	h := newTestOptOutHandler(store)

	body, _ := json.Marshal(OptOutRequest{UserID: "user1", Scope: ScopeAll})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/opt-out", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.handleSetOptOut(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleRemoveOptOut_Success(t *testing.T) {
	store := &handlerMockPrefsStore{
		removeOptOutFn: func(_ context.Context, _, _, _ string) error {
			return nil
		},
	}
	h := newTestOptOutHandler(store)

	body, _ := json.Marshal(OptOutRequest{UserID: "user1", Scope: ScopeAll})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/privacy/opt-out", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.handleRemoveOptOut(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandleRemoveOptOut_NotFound(t *testing.T) {
	store := &handlerMockPrefsStore{
		removeOptOutFn: func(_ context.Context, _, _, _ string) error {
			return ErrPreferencesNotFound
		},
	}
	h := newTestOptOutHandler(store)

	body, _ := json.Marshal(OptOutRequest{UserID: "missing", Scope: ScopeAll})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/privacy/opt-out", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.handleRemoveOptOut(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleRemoveOptOut_InvalidBody(t *testing.T) {
	h := newTestOptOutHandler(&handlerMockPrefsStore{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/privacy/opt-out", bytes.NewReader([]byte("bad")))
	rec := httptest.NewRecorder()

	h.handleRemoveOptOut(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleRemoveOptOut_StoreError(t *testing.T) {
	store := &handlerMockPrefsStore{
		removeOptOutFn: func(_ context.Context, _, _, _ string) error {
			return errors.New("db error")
		},
	}
	h := newTestOptOutHandler(store)

	body, _ := json.Marshal(OptOutRequest{UserID: "user1", Scope: ScopeAll})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/privacy/opt-out", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.handleRemoveOptOut(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleRemoveOptOut_ValidationError(t *testing.T) {
	h := newTestOptOutHandler(&handlerMockPrefsStore{})

	body, _ := json.Marshal(OptOutRequest{Scope: ScopeAll})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/privacy/opt-out", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.handleRemoveOptOut(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleGetPreferences_Success(t *testing.T) {
	now := time.Now().UTC()
	store := &handlerMockPrefsStore{
		getPreferencesFn: func(_ context.Context, userID string) (*Preferences, error) {
			return &Preferences{
				UserID:           userID,
				OptOutAll:        true,
				OptOutWorkspaces: []string{"ws1"},
				OptOutAgents:     []string{},
				CreatedAt:        now,
				UpdatedAt:        now,
			}, nil
		},
	}
	h := newTestOptOutHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/preferences/user1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var prefs Preferences
	err := json.NewDecoder(rec.Body).Decode(&prefs)
	require.NoError(t, err)
	assert.Equal(t, "user1", prefs.UserID)
	assert.True(t, prefs.OptOutAll)
}

func TestHandleGetPreferences_NotFound(t *testing.T) {
	store := &handlerMockPrefsStore{
		getPreferencesFn: func(_ context.Context, _ string) (*Preferences, error) {
			return nil, ErrPreferencesNotFound
		},
	}
	h := newTestOptOutHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/preferences/missing", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleGetPreferences_StoreError(t *testing.T) {
	store := &handlerMockPrefsStore{
		getPreferencesFn: func(_ context.Context, _ string) (*Preferences, error) {
			return nil, errors.New("db error")
		},
	}
	h := newTestOptOutHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/preferences/user1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestValidateOptOutRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     OptOutRequest
		wantErr bool
	}{
		{
			name:    "valid all scope",
			req:     OptOutRequest{UserID: "u1", Scope: ScopeAll},
			wantErr: false,
		},
		{
			name:    "valid workspace scope",
			req:     OptOutRequest{UserID: "u1", Scope: ScopeWorkspace, Target: "ws1"},
			wantErr: false,
		},
		{
			name:    "valid agent scope",
			req:     OptOutRequest{UserID: "u1", Scope: ScopeAgent, Target: "a1"},
			wantErr: false,
		},
		{name: "missing user ID", req: OptOutRequest{Scope: ScopeAll}, wantErr: true},
		{name: "invalid scope", req: OptOutRequest{UserID: "u1", Scope: "bad"}, wantErr: true},
		{
			name:    "workspace missing target",
			req:     OptOutRequest{UserID: "u1", Scope: ScopeWorkspace},
			wantErr: true,
		},
		{
			name:    "agent missing target",
			req:     OptOutRequest{UserID: "u1", Scope: ScopeAgent},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOptOutRequest(tt.req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOptOutRegisterRoutes(t *testing.T) {
	store := &handlerMockPrefsStore{
		setOptOutFn: func(_ context.Context, _, _, _ string) error {
			return nil
		},
		removeOptOutFn: func(_ context.Context, _, _, _ string) error {
			return nil
		},
		getPreferencesFn: func(_ context.Context, _ string) (*Preferences, error) {
			return nil, ErrPreferencesNotFound
		},
	}
	h := newTestOptOutHandler(store)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// POST opt-out
	body, _ := json.Marshal(OptOutRequest{UserID: "u1", Scope: ScopeAll})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(
		http.MethodPost, "/api/v1/privacy/opt-out", bytes.NewReader(body),
	))
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// DELETE opt-out
	body, _ = json.Marshal(OptOutRequest{UserID: "u1", Scope: ScopeAll})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(
		http.MethodDelete, "/api/v1/privacy/opt-out", bytes.NewReader(body),
	))
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// GET preferences (not found)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/api/v1/privacy/preferences/u1", nil,
	))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
