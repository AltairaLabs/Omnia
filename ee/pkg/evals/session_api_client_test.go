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
)

func TestHTTPSessionAPIClient_GetSession_Success(t *testing.T) {
	sess := &session.Session{
		ID:        "s1",
		AgentName: "test-agent",
		Namespace: "ns",
		CreatedAt: time.Now().Truncate(time.Second),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/sessions/s1", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionResponse{Session: sess})
	}))
	defer server.Close()

	client := NewHTTPSessionAPIClient(server.URL)
	result, err := client.GetSession(context.Background(), "s1")

	require.NoError(t, err)
	assert.Equal(t, "s1", result.ID)
	assert.Equal(t, "test-agent", result.AgentName)
	assert.Equal(t, "ns", result.Namespace)
}

func TestHTTPSessionAPIClient_GetSession_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewHTTPSessionAPIClient(server.URL)
	_, err := client.GetSession(context.Background(), "nonexistent")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestHTTPSessionAPIClient_GetSession_ConnectionError(t *testing.T) {
	client := NewHTTPSessionAPIClient("http://localhost:1")
	_, err := client.GetSession(context.Background(), "s1")

	require.Error(t, err)
}

func TestHTTPSessionAPIClient_GetSessionMessages_Success(t *testing.T) {
	msgs := []*session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "hello"},
		{ID: "m2", Role: session.RoleAssistant, Content: "hi there"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/sessions/s1/messages", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(messagesResponse{Messages: msgs, HasMore: false})
	}))
	defer server.Close()

	client := NewHTTPSessionAPIClient(server.URL)
	result, err := client.GetSessionMessages(context.Background(), "s1")

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

	client := NewHTTPSessionAPIClient(server.URL)
	_, err := client.GetSessionMessages(context.Background(), "s1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestHTTPSessionAPIClient_GetSessionMessages_ConnectionError(t *testing.T) {
	client := NewHTTPSessionAPIClient("http://localhost:1")
	_, err := client.GetSessionMessages(context.Background(), "s1")

	require.Error(t, err)
}

func TestHTTPSessionAPIClient_GetSessionMessages_NilMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Respond with null messages array.
		_ = json.NewEncoder(w).Encode(messagesResponse{Messages: nil})
	}))
	defer server.Close()

	client := NewHTTPSessionAPIClient(server.URL)
	result, err := client.GetSessionMessages(context.Background(), "s1")

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestHTTPSessionAPIClient_WriteEvalResults_Success(t *testing.T) {
	var received []*api.EvalResult

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/eval-results", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewHTTPSessionAPIClient(server.URL)

	score := 0.8
	results := []*api.EvalResult{
		{
			SessionID: "s1",
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
	assert.Equal(t, "e1", received[0].EvalID)
	assert.Equal(t, "worker", received[0].Source)
}

func TestHTTPSessionAPIClient_WriteEvalResults_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewHTTPSessionAPIClient(server.URL)
	err := client.WriteEvalResults(context.Background(), []*api.EvalResult{{EvalID: "e1"}})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestHTTPSessionAPIClient_WriteEvalResults_ConnectionError(t *testing.T) {
	client := NewHTTPSessionAPIClient("http://localhost:1")
	err := client.WriteEvalResults(context.Background(), []*api.EvalResult{{EvalID: "e1"}})

	require.Error(t, err)
}

func TestHTTPSessionAPIClient_WriteEvalResults_BadRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewHTTPSessionAPIClient(server.URL)
	err := client.WriteEvalResults(context.Background(), []*api.EvalResult{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}

func TestNewHTTPSessionAPIClient(t *testing.T) {
	client := NewHTTPSessionAPIClient("http://example.com")

	assert.Equal(t, "http://example.com", client.baseURL)
	assert.NotNil(t, client.httpClient)
	assert.Equal(t, defaultHTTPTimeout, client.httpClient.Timeout)
}
