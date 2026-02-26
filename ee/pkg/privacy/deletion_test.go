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
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// --- Mock implementations ---------------------------------------------------

// MockDeletionStore is an in-memory mock for DeletionStore.
type MockDeletionStore struct {
	mu       sync.RWMutex
	requests map[string]*DeletionRequest
}

func NewMockDeletionStore() *MockDeletionStore {
	return &MockDeletionStore{requests: make(map[string]*DeletionRequest)}
}

func (m *MockDeletionStore) CreateRequest(_ context.Context, req *DeletionRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[req.ID] = req
	return nil
}

func (m *MockDeletionStore) GetRequest(_ context.Context, id string) (*DeletionRequest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	req, ok := m.requests[id]
	if !ok {
		return nil, ErrRequestNotFound
	}
	// Return a copy to avoid data races with concurrent mutations.
	cp := *req
	return &cp, nil
}

func (m *MockDeletionStore) UpdateRequest(_ context.Context, req *DeletionRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[req.ID] = req
	return nil
}

func (m *MockDeletionStore) ListRequestsByUser(_ context.Context, userID string) ([]*DeletionRequest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*DeletionRequest
	for _, req := range m.requests {
		if req.UserID == userID {
			result = append(result, req)
		}
	}
	return result, nil
}

// MockSessionDeleter is a mock for SessionDeleter.
type MockSessionDeleter struct {
	Sessions    map[string][]string // userID+workspace -> sessionIDs
	DeleteError error               // error to return on delete
	FailIDs     map[string]bool     // session IDs that should fail on delete
}

func NewMockSessionDeleter() *MockSessionDeleter {
	return &MockSessionDeleter{
		Sessions: make(map[string][]string),
		FailIDs:  make(map[string]bool),
	}
}

func (m *MockSessionDeleter) ListSessionsByUser(_ context.Context, userID string, workspace string) ([]string, error) {
	key := userID + "|" + workspace
	return m.Sessions[key], nil
}

func (m *MockSessionDeleter) DeleteSession(_ context.Context, sessionID string) error {
	if m.DeleteError != nil {
		return m.DeleteError
	}
	if m.FailIDs[sessionID] {
		return errors.New("delete failed for session")
	}
	return nil
}

// MockAuditLogger captures audit events for assertions.
type MockAuditLogger struct {
	mu     sync.Mutex
	Events []*api.AuditEntry
}

func (m *MockAuditLogger) LogEvent(_ context.Context, entry *api.AuditEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Events = append(m.Events, entry)
}

// --- Helper to build service for tests --------------------------------------

func newTestService(store *MockDeletionStore, deleter *MockSessionDeleter, audit *MockAuditLogger) *DeletionService {
	var al AuditLogger
	if audit != nil {
		al = audit
	}
	return NewDeletionService(store, deleter, al, logr.Discard())
}

// --- CreateRequest tests ----------------------------------------------------

func TestCreateRequest_Success(t *testing.T) {
	store := NewMockDeletionStore()
	deleter := NewMockSessionDeleter()
	audit := &MockAuditLogger{}
	svc := newTestService(store, deleter, audit)

	req, err := svc.CreateRequest(context.Background(), &CreateDeletionRequest{
		UserID: "user-1",
		Reason: "gdpr_erasure",
		Scope:  "all",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, req.ID)
	assert.Equal(t, "user-1", req.UserID)
	assert.Equal(t, "gdpr_erasure", req.Reason)
	assert.Equal(t, "all", req.Scope)
	assert.Equal(t, "pending", req.Status)
	assert.NotZero(t, req.CreatedAt)

	// Verify audit event was logged.
	assert.Len(t, audit.Events, 1)
	assert.Equal(t, "deletion_requested", audit.Events[0].EventType)
}

func TestCreateRequest_MissingUserID(t *testing.T) {
	store := NewMockDeletionStore()
	svc := newTestService(store, NewMockSessionDeleter(), nil)

	_, err := svc.CreateRequest(context.Background(), &CreateDeletionRequest{
		Reason: "gdpr_erasure",
	})

	assert.ErrorIs(t, err, ErrMissingUserID)
}

func TestCreateRequest_InvalidReason(t *testing.T) {
	store := NewMockDeletionStore()
	svc := newTestService(store, NewMockSessionDeleter(), nil)

	_, err := svc.CreateRequest(context.Background(), &CreateDeletionRequest{
		UserID: "user-1",
		Reason: "invalid_reason",
	})

	assert.ErrorIs(t, err, ErrInvalidReason)
}

func TestCreateRequest_InvalidScope(t *testing.T) {
	store := NewMockDeletionStore()
	svc := newTestService(store, NewMockSessionDeleter(), nil)

	_, err := svc.CreateRequest(context.Background(), &CreateDeletionRequest{
		UserID: "user-1",
		Reason: "ccpa_delete",
		Scope:  "invalid_scope",
	})

	assert.ErrorIs(t, err, ErrInvalidScope)
}

func TestCreateRequest_DefaultScope(t *testing.T) {
	store := NewMockDeletionStore()
	svc := newTestService(store, NewMockSessionDeleter(), nil)

	req, err := svc.CreateRequest(context.Background(), &CreateDeletionRequest{
		UserID: "user-1",
		Reason: "user_request",
	})

	require.NoError(t, err)
	assert.Equal(t, "all", req.Scope)
}

func TestCreateRequest_AllReasons(t *testing.T) {
	reasons := []string{"gdpr_erasure", "ccpa_delete", "user_request"}
	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			store := NewMockDeletionStore()
			svc := newTestService(store, NewMockSessionDeleter(), nil)
			req, err := svc.CreateRequest(context.Background(), &CreateDeletionRequest{
				UserID: "user-1",
				Reason: reason,
				Scope:  "all",
			})
			require.NoError(t, err)
			assert.Equal(t, reason, req.Reason)
		})
	}
}

func TestCreateRequest_AllScopes(t *testing.T) {
	scopes := []string{"all", "workspace", "date_range"}
	for _, scope := range scopes {
		t.Run(scope, func(t *testing.T) {
			store := NewMockDeletionStore()
			svc := newTestService(store, NewMockSessionDeleter(), nil)
			req, err := svc.CreateRequest(context.Background(), &CreateDeletionRequest{
				UserID: "user-1",
				Reason: "gdpr_erasure",
				Scope:  scope,
			})
			require.NoError(t, err)
			assert.Equal(t, scope, req.Scope)
		})
	}
}

// --- ProcessRequest tests ---------------------------------------------------

func TestProcessRequest_HappyPath(t *testing.T) {
	store := NewMockDeletionStore()
	deleter := NewMockSessionDeleter()
	audit := &MockAuditLogger{}
	svc := newTestService(store, deleter, audit)

	deleter.Sessions["user-1|"] = []string{"sess-1", "sess-2", "sess-3"}

	req, err := svc.CreateRequest(context.Background(), &CreateDeletionRequest{
		UserID: "user-1",
		Reason: "gdpr_erasure",
		Scope:  "all",
	})
	require.NoError(t, err)

	err = svc.ProcessRequest(context.Background(), req.ID)
	require.NoError(t, err)

	// Verify the request was completed.
	updated, err := store.GetRequest(context.Background(), req.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", updated.Status)
	assert.Equal(t, 3, updated.SessionsDeleted)
	assert.Empty(t, updated.Errors)
	assert.NotNil(t, updated.StartedAt)
	assert.NotNil(t, updated.CompletedAt)

	// Verify audit events: deletion_requested + deletion_completed.
	assert.Len(t, audit.Events, 2)
	assert.Equal(t, "deletion_requested", audit.Events[0].EventType)
	assert.Equal(t, "deletion_completed", audit.Events[1].EventType)
}

func TestProcessRequest_PartialFailure(t *testing.T) {
	store := NewMockDeletionStore()
	deleter := NewMockSessionDeleter()
	audit := &MockAuditLogger{}
	svc := newTestService(store, deleter, audit)

	deleter.Sessions["user-1|"] = []string{"sess-1", "sess-2", "sess-3"}
	deleter.FailIDs["sess-2"] = true

	req, err := svc.CreateRequest(context.Background(), &CreateDeletionRequest{
		UserID: "user-1",
		Reason: "gdpr_erasure",
		Scope:  "all",
	})
	require.NoError(t, err)

	err = svc.ProcessRequest(context.Background(), req.ID)
	require.NoError(t, err)

	updated, err := store.GetRequest(context.Background(), req.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", updated.Status)
	assert.Equal(t, 2, updated.SessionsDeleted)
	assert.Len(t, updated.Errors, 1)
	assert.Contains(t, updated.Errors[0], "sess-2")
}

func TestProcessRequest_NotFound(t *testing.T) {
	store := NewMockDeletionStore()
	svc := newTestService(store, NewMockSessionDeleter(), nil)

	err := svc.ProcessRequest(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, ErrRequestNotFound)
}

func TestProcessRequest_AlreadyProcessing(t *testing.T) {
	store := NewMockDeletionStore()
	deleter := NewMockSessionDeleter()
	svc := newTestService(store, deleter, nil)

	req, err := svc.CreateRequest(context.Background(), &CreateDeletionRequest{
		UserID: "user-1",
		Reason: "gdpr_erasure",
		Scope:  "all",
	})
	require.NoError(t, err)

	// Manually set status to in_progress.
	req.Status = "in_progress"
	require.NoError(t, store.UpdateRequest(context.Background(), req))

	err = svc.ProcessRequest(context.Background(), req.ID)
	assert.ErrorIs(t, err, ErrAlreadyProcessing)
}

func TestProcessRequest_NoSessions(t *testing.T) {
	store := NewMockDeletionStore()
	deleter := NewMockSessionDeleter()
	svc := newTestService(store, deleter, nil)

	// No sessions for this user.
	req, err := svc.CreateRequest(context.Background(), &CreateDeletionRequest{
		UserID: "user-no-sessions",
		Reason: "gdpr_erasure",
		Scope:  "all",
	})
	require.NoError(t, err)

	err = svc.ProcessRequest(context.Background(), req.ID)
	require.NoError(t, err)

	updated, err := store.GetRequest(context.Background(), req.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", updated.Status)
	assert.Equal(t, 0, updated.SessionsDeleted)
}

func TestProcessRequest_WithWorkspace(t *testing.T) {
	store := NewMockDeletionStore()
	deleter := NewMockSessionDeleter()
	svc := newTestService(store, deleter, nil)

	deleter.Sessions["user-1|my-workspace"] = []string{"sess-w1", "sess-w2"}

	req, err := svc.CreateRequest(context.Background(), &CreateDeletionRequest{
		UserID:    "user-1",
		Reason:    "user_request",
		Scope:     "workspace",
		Workspace: "my-workspace",
	})
	require.NoError(t, err)

	err = svc.ProcessRequest(context.Background(), req.ID)
	require.NoError(t, err)

	updated, err := store.GetRequest(context.Background(), req.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", updated.Status)
	assert.Equal(t, 2, updated.SessionsDeleted)
}

func TestProcessRequest_NilAuditLogger(t *testing.T) {
	store := NewMockDeletionStore()
	deleter := NewMockSessionDeleter()
	svc := newTestService(store, deleter, nil)

	deleter.Sessions["user-1|"] = []string{"sess-1"}

	req, err := svc.CreateRequest(context.Background(), &CreateDeletionRequest{
		UserID: "user-1",
		Reason: "gdpr_erasure",
		Scope:  "all",
	})
	require.NoError(t, err)

	// Should not panic with nil audit logger.
	err = svc.ProcessRequest(context.Background(), req.ID)
	require.NoError(t, err)
}

// --- GetRequest tests -------------------------------------------------------

func TestGetRequest_Found(t *testing.T) {
	store := NewMockDeletionStore()
	svc := newTestService(store, NewMockSessionDeleter(), nil)

	created, err := svc.CreateRequest(context.Background(), &CreateDeletionRequest{
		UserID: "user-1",
		Reason: "gdpr_erasure",
		Scope:  "all",
	})
	require.NoError(t, err)

	found, err := svc.GetRequest(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
	assert.Equal(t, "user-1", found.UserID)
}

func TestGetRequest_NotFound(t *testing.T) {
	store := NewMockDeletionStore()
	svc := newTestService(store, NewMockSessionDeleter(), nil)

	_, err := svc.GetRequest(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, ErrRequestNotFound)
}

// --- ListRequestsByUser tests -----------------------------------------------

func TestListRequestsByUser_Found(t *testing.T) {
	store := NewMockDeletionStore()
	svc := newTestService(store, NewMockSessionDeleter(), nil)

	_, err := svc.CreateRequest(context.Background(), &CreateDeletionRequest{
		UserID: "user-1",
		Reason: "gdpr_erasure",
		Scope:  "all",
	})
	require.NoError(t, err)

	_, err = svc.CreateRequest(context.Background(), &CreateDeletionRequest{
		UserID: "user-1",
		Reason: "ccpa_delete",
		Scope:  "all",
	})
	require.NoError(t, err)

	// Different user, should not appear.
	_, err = svc.CreateRequest(context.Background(), &CreateDeletionRequest{
		UserID: "user-2",
		Reason: "gdpr_erasure",
		Scope:  "all",
	})
	require.NoError(t, err)

	requests, err := svc.ListRequestsByUser(context.Background(), "user-1")
	require.NoError(t, err)
	assert.Len(t, requests, 2)
	for _, req := range requests {
		assert.Equal(t, "user-1", req.UserID)
	}
}

func TestListRequestsByUser_Empty(t *testing.T) {
	store := NewMockDeletionStore()
	svc := newTestService(store, NewMockSessionDeleter(), nil)

	requests, err := svc.ListRequestsByUser(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Empty(t, requests)
}

// --- Handler HTTP tests -----------------------------------------------------

func newTestHandler() (*DeletionHandler, *MockDeletionStore, *MockSessionDeleter) {
	store := NewMockDeletionStore()
	deleter := NewMockSessionDeleter()
	audit := &MockAuditLogger{}
	svc := NewDeletionService(store, deleter, audit, logr.Discard())
	handler := NewDeletionHandler(svc, logr.Discard())
	return handler, store, deleter
}

func TestHandleCreate_Success(t *testing.T) {
	store := NewMockDeletionStore()
	deleter := NewMockSessionDeleter()
	deleter.Sessions["user-1|"] = []string{"sess-1"}
	audit := &MockAuditLogger{}
	svc := NewDeletionService(store, deleter, audit, logr.Discard())
	handler := NewDeletionHandler(svc, logr.Discard())

	body := `{"userId":"user-1","reason":"gdpr_erasure","scope":"all"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/deletion-request", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var resp DeletionRequest
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.ID)
	assert.Equal(t, "user-1", resp.UserID)
	assert.Equal(t, "pending", resp.Status)

	// Wait for the background goroutine to complete to avoid data races.
	assert.Eventually(t, func() bool {
		updated, getErr := store.GetRequest(context.Background(), resp.ID)
		return getErr == nil && updated.Status != "pending" && updated.Status != "in_progress"
	}, 2*time.Second, 10*time.Millisecond)
}

func TestHandleCreate_InvalidBody(t *testing.T) {
	handler, _, _ := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/deletion-request", bytes.NewBufferString("{invalid"))
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleCreate_MissingUserID(t *testing.T) {
	handler, _, _ := newTestHandler()

	body := `{"reason":"gdpr_erasure","scope":"all"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/deletion-request", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleCreate_InvalidReason(t *testing.T) {
	handler, _, _ := newTestHandler()

	body := `{"userId":"user-1","reason":"bad","scope":"all"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/deletion-request", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleGet_Success(t *testing.T) {
	handler, store, _ := newTestHandler()

	dr := &DeletionRequest{
		ID:        "req-123",
		UserID:    "user-1",
		Reason:    "gdpr_erasure",
		Scope:     "all",
		Status:    "completed",
		CreatedAt: time.Now().UTC(),
		Errors:    []string{},
	}
	require.NoError(t, store.CreateRequest(context.Background(), dr))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/deletion-request/req-123", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp DeletionRequest
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "req-123", resp.ID)
	assert.Equal(t, "completed", resp.Status)
}

func TestHandleGet_NotFound(t *testing.T) {
	handler, _, _ := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/deletion-request/nonexistent", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleList_Success(t *testing.T) {
	handler, store, _ := newTestHandler()

	for i, id := range []string{"req-1", "req-2"} {
		dr := &DeletionRequest{
			ID:        id,
			UserID:    "user-1",
			Reason:    "gdpr_erasure",
			Scope:     "all",
			Status:    "completed",
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Minute),
			Errors:    []string{},
		}
		require.NoError(t, store.CreateRequest(context.Background(), dr))
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/deletion-requests?user_id=user-1", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []*DeletionRequest
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Len(t, resp, 2)
}

func TestHandleList_MissingUserID(t *testing.T) {
	handler, _, _ := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/deletion-requests", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleList_Empty(t *testing.T) {
	handler, _, _ := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/deletion-requests?user_id=nobody", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp []*DeletionRequest
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Empty(t, resp)
}

func TestMapErrorToStatus(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{"not found", ErrRequestNotFound, http.StatusNotFound},
		{"required field", ErrMissingUserID, http.StatusBadRequest},
		{"invalid reason", ErrInvalidReason, http.StatusBadRequest},
		{"already processing", ErrAlreadyProcessing, http.StatusConflict},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, mapErrorToStatus(tc.err))
		})
	}
}

func TestWriteJSONError(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSONError(w, http.StatusBadRequest, "test error")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var resp errorResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "test error", resp.Error)
}

// --- WarmStoreSessionDeleter tests ------------------------------------------

// MockWarmStoreProvider implements providers.WarmStoreProvider for testing.
type MockWarmStoreProvider struct {
	sessions   []*session.Session
	listErr    error
	deleteErr  error
	deletedIDs []string
}

func (m *MockWarmStoreProvider) CreateSession(
	_ context.Context, _ *session.Session,
) error {
	return nil
}

func (m *MockWarmStoreProvider) GetSession(
	_ context.Context, _ string,
) (*session.Session, error) {
	return nil, nil
}

func (m *MockWarmStoreProvider) UpdateSession(
	_ context.Context, _ *session.Session,
) error {
	return nil
}

func (m *MockWarmStoreProvider) UpdateSessionStats(
	_ context.Context, _ string, _ session.SessionStatsUpdate,
) error {
	return nil
}

func (m *MockWarmStoreProvider) DeleteSession(
	_ context.Context, id string,
) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deletedIDs = append(m.deletedIDs, id)
	return nil
}

func (m *MockWarmStoreProvider) AppendMessage(
	_ context.Context, _ string, _ *session.Message,
) error {
	return nil
}

func (m *MockWarmStoreProvider) GetMessages(
	_ context.Context, _ string, _ providers.MessageQueryOpts,
) ([]*session.Message, error) {
	return nil, nil
}

func (m *MockWarmStoreProvider) ListSessions(
	_ context.Context, _ providers.SessionListOpts,
) (*providers.SessionPage, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return &providers.SessionPage{
		Sessions:   m.sessions,
		TotalCount: int64(len(m.sessions)),
	}, nil
}

func (m *MockWarmStoreProvider) SearchSessions(
	_ context.Context, _ string, _ providers.SessionListOpts,
) (*providers.SessionPage, error) {
	return nil, nil
}

func (m *MockWarmStoreProvider) CreatePartition(
	_ context.Context, _ time.Time,
) error {
	return nil
}

func (m *MockWarmStoreProvider) DropPartition(
	_ context.Context, _ time.Time,
) error {
	return nil
}

func (m *MockWarmStoreProvider) ListPartitions(
	_ context.Context,
) ([]providers.PartitionInfo, error) {
	return nil, nil
}

func (m *MockWarmStoreProvider) GetSessionsOlderThan(
	_ context.Context, _ time.Time, _ int,
) ([]*session.Session, error) {
	return nil, nil
}

func (m *MockWarmStoreProvider) DeleteSessionsBatch(
	_ context.Context, _ []string,
) error {
	return nil
}

func (m *MockWarmStoreProvider) SaveArtifact(
	_ context.Context, _ *session.Artifact,
) error {
	return nil
}

func (m *MockWarmStoreProvider) GetArtifacts(
	_ context.Context, _ string,
) ([]*session.Artifact, error) {
	return nil, nil
}

func (m *MockWarmStoreProvider) GetSessionArtifacts(
	_ context.Context, _ string,
) ([]*session.Artifact, error) {
	return nil, nil
}

func (m *MockWarmStoreProvider) DeleteSessionArtifacts(
	_ context.Context, _ string,
) error {
	return nil
}

func (m *MockWarmStoreProvider) Ping(_ context.Context) error {
	return nil
}

func (m *MockWarmStoreProvider) Close() error {
	return nil
}

func TestWarmStoreSessionDeleter_ListSessionsByUser(t *testing.T) {
	mock := &MockWarmStoreProvider{
		sessions: []*session.Session{{ID: "s1"}, {ID: "s2"}},
	}
	deleter := NewWarmStoreSessionDeleter(mock)

	ids, err := deleter.ListSessionsByUser(context.Background(), "user-1", "ws")
	require.NoError(t, err)
	assert.Equal(t, []string{"s1", "s2"}, ids)
}

func TestWarmStoreSessionDeleter_ListSessionsByUser_Error(t *testing.T) {
	mock := &MockWarmStoreProvider{listErr: errors.New("list failed")}
	deleter := NewWarmStoreSessionDeleter(mock)

	_, err := deleter.ListSessionsByUser(context.Background(), "user-1", "")
	assert.Error(t, err)
}

func TestWarmStoreSessionDeleter_ListSessionsByUser_Empty(t *testing.T) {
	mock := &MockWarmStoreProvider{sessions: []*session.Session{}}
	deleter := NewWarmStoreSessionDeleter(mock)

	ids, err := deleter.ListSessionsByUser(context.Background(), "user-1", "")
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestWarmStoreSessionDeleter_DeleteSession(t *testing.T) {
	mock := &MockWarmStoreProvider{}
	deleter := NewWarmStoreSessionDeleter(mock)

	err := deleter.DeleteSession(context.Background(), "s1")
	require.NoError(t, err)
	assert.Equal(t, []string{"s1"}, mock.deletedIDs)
}

func TestWarmStoreSessionDeleter_DeleteSession_Error(t *testing.T) {
	mock := &MockWarmStoreProvider{deleteErr: errors.New("delete failed")}
	deleter := NewWarmStoreSessionDeleter(mock)

	err := deleter.DeleteSession(context.Background(), "s1")
	assert.Error(t, err)
}

func TestNewWarmStoreSessionDeleter(t *testing.T) {
	deleter := NewWarmStoreSessionDeleter(&MockWarmStoreProvider{})
	assert.NotNil(t, deleter)
}

// --- failRequest tests ------------------------------------------------------

func TestProcessRequest_ListSessionsError(t *testing.T) {
	store := NewMockDeletionStore()
	deleter := NewMockSessionDeleter()
	audit := &MockAuditLogger{}
	svc := newTestService(store, deleter, audit)

	// Override ListSessionsByUser to return an error.
	failDeleter := &failingListDeleter{listErr: errors.New("storage unavailable")}
	svc.deleter = failDeleter

	req, err := svc.CreateRequest(context.Background(), &CreateDeletionRequest{
		UserID: "user-1",
		Reason: "gdpr_erasure",
		Scope:  "all",
	})
	require.NoError(t, err)

	err = svc.ProcessRequest(context.Background(), req.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deletion failed")

	updated, getErr := store.GetRequest(context.Background(), req.ID)
	require.NoError(t, getErr)
	assert.Equal(t, "failed", updated.Status)
	assert.NotNil(t, updated.CompletedAt)
}

// failingListDeleter returns an error on ListSessionsByUser.
type failingListDeleter struct {
	listErr error
}

func (f *failingListDeleter) ListSessionsByUser(_ context.Context, _ string, _ string) ([]string, error) {
	return nil, f.listErr
}

func (f *failingListDeleter) DeleteSession(_ context.Context, _ string) error {
	return nil
}

// --- handleList error path test ---------------------------------------------

func TestHandleList_ServiceError(t *testing.T) {
	store := NewMockDeletionStore()
	deleter := NewMockSessionDeleter()
	audit := &MockAuditLogger{}
	// Create service with a store that fails on ListRequestsByUser.
	failStore := &failingListStore{}
	svc := NewDeletionService(failStore, deleter, audit, logr.Discard())
	handler := NewDeletionHandler(svc, logr.Discard())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/deletion-requests?user_id=user-1", nil)
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	_ = store // keep the compiler happy
}

// failingListStore fails on ListRequestsByUser.
type failingListStore struct {
	MockDeletionStore
}

func (f *failingListStore) ListRequestsByUser(_ context.Context, _ string) ([]*DeletionRequest, error) {
	return nil, errors.New("db connection lost")
}

// --- PostgresDeletionStore helper tests -------------------------------------

// mockPgxRow implements pgx.Row for testing scanDeletionRequest error paths.
type mockPgxRow struct {
	err error
}

func (m *mockPgxRow) Scan(_ ...any) error { return m.err }

func TestScanDeletionRequest_ErrNoRows(t *testing.T) {
	row := &mockPgxRow{err: pgx.ErrNoRows}
	_, err := scanDeletionRequest(row)
	assert.ErrorIs(t, err, ErrRequestNotFound)
}

func TestScanDeletionRequest_OtherError(t *testing.T) {
	row := &mockPgxRow{err: errors.New("connection refused")}
	_, err := scanDeletionRequest(row)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scan deletion request")
}

func TestNullableString(t *testing.T) {
	assert.Nil(t, nullableString(""))
	result := nullableString("hello")
	require.NotNil(t, result)
	assert.Equal(t, "hello", *result)
}

func TestNewPostgresDeletionStore(t *testing.T) {
	store := NewPostgresDeletionStore(nil)
	assert.NotNil(t, store)
}

// --- PostgresDeletionStore mock-based CRUD tests ----------------------------

// mockDBPool implements the dbPool interface for unit tests.
type mockDBPool struct {
	execErr    error
	queryRowFn func() pgx.Row
	queryFn    func() (pgx.Rows, error)
}

func (m *mockDBPool) Exec(
	_ context.Context, _ string, _ ...any,
) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag(""), m.execErr
}

func (m *mockDBPool) QueryRow(
	_ context.Context, _ string, _ ...any,
) pgx.Row {
	if m.queryRowFn != nil {
		return m.queryRowFn()
	}
	return &mockPgxRow{err: pgx.ErrNoRows}
}

func (m *mockDBPool) Query(
	_ context.Context, _ string, _ ...any,
) (pgx.Rows, error) {
	if m.queryFn != nil {
		return m.queryFn()
	}
	return nil, errors.New("not implemented")
}

// newStoreFromPool creates a PostgresDeletionStore with a mock dbPool.
func newStoreFromPool(pool dbPool) *PostgresDeletionStore {
	return &PostgresDeletionStore{pool: pool}
}

func TestPostgresDeletionStore_CreateRequest_Success(t *testing.T) {
	store := newStoreFromPool(&mockDBPool{})

	req := &DeletionRequest{
		ID:        "req-1",
		UserID:    "user-1",
		Reason:    "gdpr_erasure",
		Scope:     "all",
		Status:    "pending",
		CreatedAt: time.Now().UTC(),
		Errors:    []string{},
	}
	err := store.CreateRequest(context.Background(), req)
	assert.NoError(t, err)
}

func TestPostgresDeletionStore_CreateRequest_ExecError(t *testing.T) {
	store := newStoreFromPool(&mockDBPool{
		execErr: errors.New("connection refused"),
	})

	req := &DeletionRequest{
		ID:     "req-1",
		Errors: []string{},
	}
	err := store.CreateRequest(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insert deletion request")
}

func TestPostgresDeletionStore_GetRequest_NotFound(t *testing.T) {
	store := newStoreFromPool(&mockDBPool{})

	_, err := store.GetRequest(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, ErrRequestNotFound)
}

func TestPostgresDeletionStore_UpdateRequest_Success(t *testing.T) {
	store := newStoreFromPool(&mockDBPool{})

	req := &DeletionRequest{
		ID:     "req-1",
		Status: "completed",
		Errors: []string{},
	}
	err := store.UpdateRequest(context.Background(), req)
	assert.NoError(t, err)
}

func TestPostgresDeletionStore_UpdateRequest_ExecError(t *testing.T) {
	store := newStoreFromPool(&mockDBPool{
		execErr: errors.New("connection refused"),
	})

	req := &DeletionRequest{
		ID:     "req-1",
		Errors: []string{},
	}
	err := store.UpdateRequest(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "update deletion request")
}

func TestPostgresDeletionStore_ListByUser_QueryError(t *testing.T) {
	store := newStoreFromPool(&mockDBPool{
		queryFn: func() (pgx.Rows, error) {
			return nil, errors.New("query failed")
		},
	})

	_, err := store.ListRequestsByUser(
		context.Background(), "user-1",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "query deletion requests")
}

func TestPostgresDeletionStore_ListByUser_Empty(t *testing.T) {
	store := newStoreFromPool(&mockDBPool{
		queryFn: func() (pgx.Rows, error) {
			return &mockRows{rows: nil}, nil
		},
	})

	result, err := store.ListRequestsByUser(
		context.Background(), "user-1",
	)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestPostgresDeletionStore_ListByUser_ScanError(t *testing.T) {
	store := newStoreFromPool(&mockDBPool{
		queryFn: func() (pgx.Rows, error) {
			return &mockRows{
				rows:    []bool{true}, // one row
				scanErr: errors.New("bad column"),
			}, nil
		},
	})

	_, err := store.ListRequestsByUser(
		context.Background(), "user-1",
	)
	assert.Error(t, err)
}

func TestPostgresDeletionStore_ListByUser_RowsErr(t *testing.T) {
	store := newStoreFromPool(&mockDBPool{
		queryFn: func() (pgx.Rows, error) {
			return &mockRows{
				rows:   nil,
				rowErr: errors.New("iteration error"),
			}, nil
		},
	})

	_, err := store.ListRequestsByUser(
		context.Background(), "user-1",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "iterate deletion requests")
}

func TestScanDeletionRequest_Success(t *testing.T) {
	now := time.Now().UTC()
	errorsJSON := []byte(`["error1"]`)
	workspace := "my-ws"

	row := &mockSuccessRow{
		values: []any{
			"req-1", "user-1", "gdpr_erasure", "all",
			&workspace, (*time.Time)(nil), (*time.Time)(nil),
			"completed", now, &now, &now, 5, errorsJSON,
		},
	}
	req, err := scanDeletionRequest(row)
	require.NoError(t, err)
	assert.Equal(t, "req-1", req.ID)
	assert.Equal(t, "my-ws", req.Workspace)
	assert.Equal(t, []string{"error1"}, req.Errors)
	assert.Equal(t, 5, req.SessionsDeleted)
}

func TestScanDeletionRequest_NilWorkspace(t *testing.T) {
	now := time.Now().UTC()
	row := &mockSuccessRow{
		values: []any{
			"req-1", "user-1", "gdpr_erasure", "all",
			(*string)(nil), (*time.Time)(nil), (*time.Time)(nil),
			"pending", now, (*time.Time)(nil),
			(*time.Time)(nil), 0, []byte(`[]`),
		},
	}
	req, err := scanDeletionRequest(row)
	require.NoError(t, err)
	assert.Empty(t, req.Workspace)
	assert.Empty(t, req.Errors)
}

// mockSuccessRow implements pgx.Row and scans mock values.
type mockSuccessRow struct {
	values []any
}

func (m *mockSuccessRow) Scan(dest ...any) error {
	if len(dest) != len(m.values) {
		return fmt.Errorf(
			"scan: expected %d args, got %d",
			len(m.values), len(dest),
		)
	}
	for i, v := range m.values {
		switch d := dest[i].(type) {
		case *string:
			if s, ok := v.(string); ok {
				*d = s
			}
		case **string:
			if s, ok := v.(*string); ok {
				*d = s
			}
		case *int:
			if n, ok := v.(int); ok {
				*d = n
			}
		case *time.Time:
			if t, ok := v.(time.Time); ok {
				*d = t
			}
		case **time.Time:
			if t, ok := v.(*time.Time); ok {
				*d = t
			}
		case *[]byte:
			if b, ok := v.([]byte); ok {
				*d = b
			}
		}
	}
	return nil
}

// mockRows implements pgx.Rows for testing ListRequestsByUser.
type mockRows struct {
	rows    []bool // one entry per row
	idx     int
	scanErr error
	rowErr  error
}

func (m *mockRows) Next() bool {
	if m.idx < len(m.rows) {
		m.idx++
		return true
	}
	return false
}

func (m *mockRows) Scan(_ ...any) error {
	if m.scanErr != nil {
		return m.scanErr
	}
	return pgx.ErrNoRows // triggers ErrRequestNotFound
}

func (m *mockRows) Close()                                       {}
func (m *mockRows) Err() error                                   { return m.rowErr }
func (m *mockRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("") }
func (m *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (m *mockRows) RawValues() [][]byte                          { return nil }
func (m *mockRows) Values() ([]any, error)                       { return nil, nil }
func (m *mockRows) Conn() *pgx.Conn                              { return nil }
