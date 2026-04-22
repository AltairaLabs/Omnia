/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package httpclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetrieve_RoutesToMultiTierWhenAgentIDPresent(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		require.Equal(t, http.MethodPost, r.Method)
		b, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(b, &gotBody))
		_, _ = w.Write([]byte(`{"memories":[{"id":"m-1","tier":"user","content":"c"}],"total":1}`))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	scope := map[string]string{
		"workspace_id": "ws-1",
		"user_id":      "u-1",
		"agent_id":     "a-1",
	}

	mems, err := store.Retrieve(context.Background(), scope, "dark mode", pkmemory.RetrieveOptions{Limit: 5})
	require.NoError(t, err)

	assert.Equal(t, "/api/v1/memories/retrieve", gotPath)
	assert.Equal(t, "ws-1", gotBody["workspace_id"])
	assert.Equal(t, "a-1", gotBody["agent_id"])
	assert.Equal(t, "dark mode", gotBody["query"])
	require.Len(t, mems, 1)
	assert.Equal(t, "m-1", mems[0].ID)
}

func TestRetrieve_FallsBackToSearchWhenNoAgentID(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"memories":[],"total":0}`))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	scope := map[string]string{"workspace_id": "ws-1", "user_id": "u-1"}

	_, err := store.Retrieve(context.Background(), scope, "q", pkmemory.RetrieveOptions{})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(gotPath, "/api/v1/memories/search"), "expected search fallback, got %s", gotPath)
}

func TestRetrieveMultiTier_Direct(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"memories":[{"id":"m-1","tier":"institutional","content":"c","score":0.87}],"total":1}`))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	res, err := store.RetrieveMultiTier(context.Background(), MultiTierRequest{
		WorkspaceID: "ws-1",
		UserID:      "u-1",
		AgentID:     "a-1",
		Limit:       5,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Total)
	require.Len(t, res.Memories, 1)
	assert.Equal(t, "institutional", res.Memories[0].Tier)
	assert.Equal(t, 0.87, res.Memories[0].Score)
}

func TestRetrieveMultiTier_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"db down"}`))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	_, err := store.RetrieveMultiTier(context.Background(), MultiTierRequest{WorkspaceID: "ws-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}

func TestRetrieveMultiTier_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	_, err := store.RetrieveMultiTier(context.Background(), MultiTierRequest{WorkspaceID: "ws-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestRetrieveMultiTier_NormalizesNilMemories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"total":0}`))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	res, err := store.RetrieveMultiTier(context.Background(), MultiTierRequest{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	assert.NotNil(t, res.Memories)
	assert.Empty(t, res.Memories)
}

func TestRetrieveMultiTier_NetworkError(t *testing.T) {
	store := NewStore("http://127.0.0.1:1", logr.Discard())
	_, err := store.RetrieveMultiTier(context.Background(), MultiTierRequest{WorkspaceID: "ws-1"})
	require.Error(t, err)
}
