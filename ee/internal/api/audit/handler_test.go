/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/ee/pkg/audit"
)

// mockQuerier implements auditQuerier for testing.
type mockQuerier struct {
	result *audit.QueryResult
	err    error
	opts   audit.QueryOpts // captured from last call
}

func (m *mockQuerier) Query(_ context.Context, opts audit.QueryOpts) (*audit.QueryResult, error) {
	m.opts = opts
	return m.result, m.err
}

func TestParseIntParam(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		param      string
		defaultVal int
		want       int
	}{
		{"present", "/test?limit=42", "limit", 10, 42},
		{"missing", "/test", "limit", 10, 10},
		{"invalid", "/test?limit=abc", "limit", 10, 10},
		{"negative", "/test?limit=-5", "limit", 10, 10},
		{"zero", "/test?limit=0", "limit", 10, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			got := parseIntParam(req, tt.param, tt.defaultVal)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "bad request")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp errorResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	assert.NoError(t, err)
	assert.Equal(t, "bad request", resp.Error)
}

func TestWriteError_InternalServerError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusInternalServerError, "internal server error")

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleQuery_InvalidFromTime(t *testing.T) {
	h := &Handler{log: logr.Discard()}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/sessions?from=bad", nil)
	rec := httptest.NewRecorder()
	h.handleQuery(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp errorResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	assert.Contains(t, resp.Error, "from")
}

func TestHandleQuery_InvalidToTime(t *testing.T) {
	h := &Handler{log: logr.Discard()}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/sessions?to=bad", nil)
	rec := httptest.NewRecorder()
	h.handleQuery(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp errorResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	assert.Contains(t, resp.Error, "to")
}

func TestNewHandler(t *testing.T) {
	h := NewHandler(nil, logr.Discard())
	assert.NotNil(t, h)
}

func TestRegisterRoutes(t *testing.T) {
	h := NewHandler(nil, logr.Discard())
	mux := http.NewServeMux()
	// Should not panic.
	h.RegisterRoutes(mux)
}

func TestHandleQuery_Success(t *testing.T) {
	mq := &mockQuerier{
		result: &audit.QueryResult{
			Entries: []*audit.Entry{{ID: 1, EventType: "session_accessed"}},
			Total:   1,
			HasMore: false,
		},
	}
	h := &Handler{logger: mq, log: logr.Discard()}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/sessions?sessionId=abc&userId=u1&workspace=ws", nil)
	rec := httptest.NewRecorder()
	h.handleQuery(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var result audit.QueryResult
	err := json.NewDecoder(rec.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Total)
	assert.Len(t, result.Entries, 1)

	// Verify opts were passed.
	assert.Equal(t, "abc", mq.opts.SessionID)
	assert.Equal(t, "u1", mq.opts.UserID)
	assert.Equal(t, "ws", mq.opts.Workspace)
}

func TestHandleQuery_EventTypes(t *testing.T) {
	mq := &mockQuerier{
		result: &audit.QueryResult{Entries: []*audit.Entry{}, Total: 0},
	}
	h := &Handler{logger: mq, log: logr.Discard()}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/sessions?eventTypes=session_accessed,session_searched", nil)
	rec := httptest.NewRecorder()
	h.handleQuery(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"session_accessed", "session_searched"}, mq.opts.EventTypes)
}

func TestHandleQuery_FromTo(t *testing.T) {
	mq := &mockQuerier{
		result: &audit.QueryResult{Entries: []*audit.Entry{}, Total: 0},
	}
	h := &Handler{logger: mq, log: logr.Discard()}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/sessions?from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	h.handleQuery(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, mq.opts.From.IsZero())
	assert.False(t, mq.opts.To.IsZero())
}

func TestHandleQuery_QueryError(t *testing.T) {
	mq := &mockQuerier{err: fmt.Errorf("db down")}
	h := &Handler{logger: mq, log: logr.Discard()}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/sessions", nil)
	rec := httptest.NewRecorder()
	h.handleQuery(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var resp errorResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	assert.Equal(t, "internal server error", resp.Error)
}
