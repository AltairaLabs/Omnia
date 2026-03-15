/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
	"github.com/altairalabs/omnia/pkg/sessionapi"
)

// newTestClient creates an HTTPSessionAPIClient pointed at the given test server.
func newTestClient(t *testing.T, serverURL string) *HTTPSessionAPIClient {
	t.Helper()
	client, err := NewHTTPSessionAPIClient(serverURL)
	require.NoError(t, err)
	return client
}

func TestHTTPSessionAPIClient_GetSession_Success(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/sessions/550e8400-e29b-41d4-a716-446655440000", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionapi.SessionResponse{
			Session: &sessionapi.Session{
				Id:        uuidPtr("550e8400-e29b-41d4-a716-446655440000"),
				AgentName: ptr("test-agent"),
				Namespace: ptr("ns"),
				CreatedAt: &now,
			},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.GetSession(context.Background(), "550e8400-e29b-41d4-a716-446655440000")

	require.NoError(t, err)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", result.ID)
	assert.Equal(t, "test-agent", result.AgentName)
	assert.Equal(t, "ns", result.Namespace)
}

func TestHTTPSessionAPIClient_GetSession_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetSession(context.Background(), "550e8400-e29b-41d4-a716-446655440000")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestHTTPSessionAPIClient_GetSession_ConnectionError(t *testing.T) {
	client, err := NewHTTPSessionAPIClient("http://localhost:1")
	require.NoError(t, err)
	_, err = client.GetSession(context.Background(), "550e8400-e29b-41d4-a716-446655440000")

	require.Error(t, err)
}

func TestHTTPSessionAPIClient_GetSessionMessages_Success(t *testing.T) {
	role := sessionapi.User
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/sessions/550e8400-e29b-41d4-a716-446655440000/messages", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionapi.MessagesResponse{
			Messages: &[]sessionapi.Message{
				{Id: ptr("m1"), Role: &role, Content: ptr("hello")},
				{Id: ptr("m2"), Role: &role, Content: ptr("hi there")},
			},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.GetSessionMessages(context.Background(), "550e8400-e29b-41d4-a716-446655440000")

	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "m1", result[0].ID)
	assert.Equal(t, session.RoleUser, result[0].Role)
	assert.Equal(t, "m2", result[1].ID)
	assert.Equal(t, "hi there", result[1].Content)
}

func TestHTTPSessionAPIClient_GetSessionMessages_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetSessionMessages(context.Background(), "550e8400-e29b-41d4-a716-446655440000")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestHTTPSessionAPIClient_GetSessionMessages_ConnectionError(t *testing.T) {
	client, err := NewHTTPSessionAPIClient("http://localhost:1")
	require.NoError(t, err)
	_, err = client.GetSessionMessages(context.Background(), "550e8400-e29b-41d4-a716-446655440000")

	require.Error(t, err)
}

func TestHTTPSessionAPIClient_GetSessionMessages_NilMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionapi.MessagesResponse{Messages: nil})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.GetSessionMessages(context.Background(), "550e8400-e29b-41d4-a716-446655440000")

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestHTTPSessionAPIClient_WriteEvalResults_Success(t *testing.T) {
	var received []sessionapi.EvalResult

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/eval-results", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	score := 0.8
	results := []*api.EvalResult{
		{
			SessionID: "550e8400-e29b-41d4-a716-446655440000",
			EvalID:    "e1",
			EvalType:  "contains",
			Passed:    true,
			Score:     &score,
			Source:    "worker",
		},
	}

	err := client.WriteEvalResults(context.Background(), results)

	require.NoError(t, err)
	require.Len(t, received, 1)
	assert.Equal(t, "e1", deref(received[0].EvalId))
	assert.Equal(t, "worker", deref(received[0].Source))
}

func TestHTTPSessionAPIClient_WriteEvalResults_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.WriteEvalResults(context.Background(), []*api.EvalResult{{EvalID: "e1"}})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestHTTPSessionAPIClient_WriteEvalResults_ConnectionError(t *testing.T) {
	client, err := NewHTTPSessionAPIClient("http://localhost:1")
	require.NoError(t, err)
	err = client.WriteEvalResults(context.Background(), []*api.EvalResult{{EvalID: "e1"}})

	require.Error(t, err)
}

func TestHTTPSessionAPIClient_WriteEvalResults_BadRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.WriteEvalResults(context.Background(), []*api.EvalResult{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}

func TestHTTPSessionAPIClient_ListEvalResults_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/eval-results", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "false", r.URL.Query().Get("passed"))
		assert.Equal(t, "10", r.URL.Query().Get("limit"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionapi.EvalResultListResponse{
			Results: &[]sessionapi.EvalResult{
				{Id: ptr("er1"), EvalType: ptr("contains"), Passed: ptr(false)},
				{Id: ptr("er2"), EvalType: ptr("contains"), Passed: ptr(false)},
			},
			Total:   ptr(int64(2)),
			HasMore: ptr(false),
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	passed := false
	got, err := client.ListEvalResults(context.Background(), api.EvalResultListOpts{
		Passed: &passed,
		Limit:  10,
	})

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "er1", got[0].ID)
	assert.Equal(t, "er2", got[1].ID)
}

func TestHTTPSessionAPIClient_ListEvalResults_WithAllParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		assert.Equal(t, "true", q.Get("passed"))
		assert.Equal(t, "5", q.Get("limit"))
		assert.Equal(t, "10", q.Get("offset"))
		assert.Equal(t, "my-agent", q.Get("agentName"))
		assert.Equal(t, "default", q.Get("namespace"))
		assert.Equal(t, "eval-1", q.Get("evalId"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionapi.EvalResultListResponse{
			Results: &[]sessionapi.EvalResult{},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	passed := true
	_, err := client.ListEvalResults(context.Background(), api.EvalResultListOpts{
		Passed:    &passed,
		Limit:     5,
		Offset:    10,
		AgentName: "my-agent",
		Namespace: "default",
		EvalID:    "eval-1",
	})

	require.NoError(t, err)
}

func TestHTTPSessionAPIClient_ListEvalResults_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListEvalResults(context.Background(), api.EvalResultListOpts{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestHTTPSessionAPIClient_ListEvalResults_ConnectionError(t *testing.T) {
	client, err := NewHTTPSessionAPIClient("http://localhost:1")
	require.NoError(t, err)
	_, err = client.ListEvalResults(context.Background(), api.EvalResultListOpts{})

	require.Error(t, err)
}

func TestHTTPSessionAPIClient_GetSessionEvalResults_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/sessions/550e8400-e29b-41d4-a716-446655440000/eval-results", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionapi.EvalResultSessionResponse{
			Results: &[]sessionapi.EvalResult{
				{Id: ptr("er1"), EvalType: ptr("contains"), Passed: ptr(true)},
				{Id: ptr("er2"), EvalType: ptr("tone"), Passed: ptr(false), MessageId: ptr("m2")},
			},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	got, err := client.GetSessionEvalResults(context.Background(), "550e8400-e29b-41d4-a716-446655440000")

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "er1", got[0].ID)
	assert.True(t, got[0].Passed)
	assert.Equal(t, "m2", got[1].MessageID)
}

func TestHTTPSessionAPIClient_GetSessionEvalResults_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetSessionEvalResults(context.Background(), "550e8400-e29b-41d4-a716-446655440000")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestHTTPSessionAPIClient_GetSessionEvalResults_ConnectionError(t *testing.T) {
	client, err := NewHTTPSessionAPIClient("http://localhost:1")
	require.NoError(t, err)
	_, err = client.GetSessionEvalResults(context.Background(), "550e8400-e29b-41d4-a716-446655440000")

	require.Error(t, err)
}

func TestNewHTTPSessionAPIClient(t *testing.T) {
	client, err := NewHTTPSessionAPIClient("http://example.com")

	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, client.client)
}

func TestNewHTTPSessionAPIClient_InvalidSessionID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	_, err := client.GetSession(context.Background(), "not-a-uuid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid session ID")
}

// --- helpers used by tests (re-export from sessionapi to avoid import cycle) ---

func ptr[T any](v T) *T { return &v }

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

func uuidPtr(s string) *sessionapi.SessionID {
	var id sessionapi.SessionID
	if err := id.UnmarshalText([]byte(s)); err != nil {
		return nil
	}
	return &id
}
