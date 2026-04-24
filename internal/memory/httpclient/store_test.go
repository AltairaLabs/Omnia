/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
	"github.com/altairalabs/omnia/pkg/policy"
)

// mockMemoryAPI creates a test server that mimics the memory-api endpoints.
func mockMemoryAPI(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	// POST /api/v1/memories — Save
	mux.HandleFunc("POST /api/v1/memories", func(w http.ResponseWriter, r *http.Request) {
		var mem pkmemory.Memory
		if err := json.NewDecoder(r.Body).Decode(&mem); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(errorResponse{Error: "bad request"})
			return
		}
		mem.ID = "mem-001"
		mem.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(memoryResponse{Memory: &mem})
	})

	// GET /api/v1/memories/search — Retrieve
	mux.HandleFunc("GET /api/v1/memories/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("workspace") == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(errorResponse{Error: "workspace required"})
			return
		}
		memories := []*pkmemory.Memory{
			{
				ID:      "mem-001",
				Type:    "preference",
				Content: "user prefers dark mode",
				Scope:   map[string]string{"workspace_id": q.Get("workspace")},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(memoryListResponse{Memories: memories, Total: 1})
	})

	// GET /api/v1/memories — List
	mux.HandleFunc("GET /api/v1/memories", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("workspace") == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(errorResponse{Error: "workspace required"})
			return
		}
		memories := []*pkmemory.Memory{
			{
				ID:      "mem-001",
				Type:    "preference",
				Content: "user prefers dark mode",
				Scope:   map[string]string{"workspace_id": q.Get("workspace")},
			},
			{
				ID:      "mem-002",
				Type:    "episodic",
				Content: "discussed deployment strategy",
				Scope:   map[string]string{"workspace_id": q.Get("workspace")},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(memoryListResponse{Memories: memories, Total: 2})
	})

	// DELETE /api/v1/memories/{id} — Delete
	mux.HandleFunc("DELETE /api/v1/memories/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("workspace") == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(errorResponse{Error: "workspace required"})
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// DELETE /api/v1/memories — DeleteAll
	mux.HandleFunc("DELETE /api/v1/memories", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("workspace") == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(errorResponse{Error: "workspace required"})
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	return httptest.NewServer(mux)
}

func TestStore_Save(t *testing.T) {
	srv := mockMemoryAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())

	mem := &pkmemory.Memory{
		Type:       "preference",
		Content:    "user prefers dark mode",
		Confidence: 0.9,
		Scope:      map[string]string{"workspace_id": "ws-1"},
	}

	err := store.Save(context.Background(), mem)
	require.NoError(t, err)
	assert.Equal(t, "mem-001", mem.ID)
	assert.False(t, mem.CreatedAt.IsZero())
}

func TestStore_Save_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(errorResponse{Error: "database error"})
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())

	mem := &pkmemory.Memory{
		Type:    "preference",
		Content: "test",
		Scope:   map[string]string{"workspace_id": "ws-1"},
	}

	err := store.Save(context.Background(), mem)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Contains(t, err.Error(), "database error")
}

func TestStore_Retrieve(t *testing.T) {
	srv := mockMemoryAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())

	scope := map[string]string{"workspace_id": "ws-1"}
	results, err := store.Retrieve(context.Background(), scope, "dark mode", pkmemory.RetrieveOptions{
		Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "mem-001", results[0].ID)
	assert.Equal(t, "user prefers dark mode", results[0].Content)
}

func TestStore_List(t *testing.T) {
	srv := mockMemoryAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())

	scope := map[string]string{"workspace_id": "ws-1"}
	results, err := store.List(context.Background(), scope, pkmemory.ListOptions{
		Limit:  20,
		Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "mem-001", results[0].ID)
	assert.Equal(t, "mem-002", results[1].ID)
}

func TestStore_Delete(t *testing.T) {
	srv := mockMemoryAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())

	scope := map[string]string{"workspace_id": "ws-1"}
	err := store.Delete(context.Background(), scope, "mem-001")
	require.NoError(t, err)
}

func TestStore_DeleteAll(t *testing.T) {
	srv := mockMemoryAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())

	scope := map[string]string{"workspace_id": "ws-1", "user_id": "user-1"}
	err := store.DeleteAll(context.Background(), scope)
	require.NoError(t, err)
}

func TestStore_ConnectionError(t *testing.T) {
	store := NewStore("http://localhost:1", logr.Discard())

	scope := map[string]string{"workspace_id": "ws-1"}

	_, err := store.Retrieve(context.Background(), scope, "test", pkmemory.RetrieveOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retrieve")
}

func TestScopeParams(t *testing.T) {
	scope := map[string]string{
		"workspace_id": "ws-1",
		"user_id":      "user-1",
		"agent_id":     "agent-1",
	}

	params := scopeParams(scope)
	assert.Equal(t, "ws-1", params.Get("workspace"))
	assert.Equal(t, "user-1", params.Get("user_id"))
	assert.Equal(t, "agent-1", params.Get("agent"))
}

func TestScopeParams_MinimalScope(t *testing.T) {
	scope := map[string]string{
		"workspace_id": "ws-1",
	}

	params := scopeParams(scope)
	assert.Equal(t, "ws-1", params.Get("workspace"))
	assert.Empty(t, params.Get("user_id"))
	assert.Empty(t, params.Get("agent"))
}

func TestStore_Save_ForwardsConsentGrants(t *testing.T) {
	var capturedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get("X-Consent-Grants")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"memory":{"id":"m1"}}`))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	ctx := policy.WithConsentGrants(context.Background(), []string{"memory:identity", "memory:preferences"})
	mem := &pkmemory.Memory{Content: "test", Scope: map[string]string{"workspace_id": "ws1"}}
	err := store.Save(ctx, mem)
	require.NoError(t, err)
	assert.Equal(t, "memory:identity,memory:preferences", capturedHeader)
}

func TestStore_Save_ForwardsConsentLayer(t *testing.T) {
	var capturedLayer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedLayer = r.Header.Get("X-Consent-Layer")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"memory":{"id":"m1"}}`))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	ctx := policy.WithConsentLayer(context.Background(), "session")
	mem := &pkmemory.Memory{Content: "test", Scope: map[string]string{"workspace_id": "ws1"}}
	err := store.Save(ctx, mem)
	require.NoError(t, err)
	assert.Equal(t, "session", capturedLayer)
}

func TestStore_Save_NoConsentLayer_NoHeader(t *testing.T) {
	var hasHeader bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hasHeader = r.Header.Get("X-Consent-Layer") != ""
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"memory":{"id":"m1"}}`))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	mem := &pkmemory.Memory{Content: "test", Scope: map[string]string{"workspace_id": "ws1"}}
	require.NoError(t, store.Save(context.Background(), mem))
	assert.False(t, hasHeader)
}

func TestStore_Save_NoConsentGrants_NoHeader(t *testing.T) {
	var hasHeader bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hasHeader = r.Header.Get("X-Consent-Grants") != ""
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"memory":{"id":"m1"}}`))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	mem := &pkmemory.Memory{Content: "test", Scope: map[string]string{"workspace_id": "ws1"}}
	err := store.Save(context.Background(), mem)
	require.NoError(t, err)
	assert.False(t, hasHeader)
}
