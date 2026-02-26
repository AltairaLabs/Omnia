/*
Copyright 2025.

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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/session"
)

// --- Test helpers ---

// mockSessionAPI creates a test server that mimics the session-api endpoints.
func mockSessionAPI(t *testing.T) *httptest.Server {
	t.Helper()
	sessions := make(map[string]*session.Session)

	mux := http.NewServeMux()

	// POST /api/v1/sessions
	mux.HandleFunc("POST /api/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		var req createSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(errorResponse{Error: "bad request"})
			return
		}
		now := time.Now()
		sess := &session.Session{
			ID:        req.ID,
			AgentName: req.AgentName,
			Namespace: req.Namespace,
			Status:    session.SessionStatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if req.TTLSeconds > 0 {
			sess.ExpiresAt = now.Add(time.Duration(req.TTLSeconds) * time.Second)
		}
		sessions[sess.ID] = sess
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(sessionResponse{Session: sess})
	})

	// GET /api/v1/sessions/{sessionID}
	mux.HandleFunc("GET /api/v1/sessions/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("sessionID")
		sess, ok := sessions[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(errorResponse{Error: "session not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionResponse{Session: sess})
	})

	// POST /api/v1/sessions/{sessionID}/messages
	mux.HandleFunc("POST /api/v1/sessions/{sessionID}/messages", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("sessionID")
		if _, ok := sessions[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(errorResponse{Error: "session not found"})
			return
		}
		// Consume body to validate it's valid JSON.
		var msg session.Message
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})

	// PATCH /api/v1/sessions/{sessionID}/stats
	mux.HandleFunc("PATCH /api/v1/sessions/{sessionID}/stats", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("sessionID")
		if _, ok := sessions[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(errorResponse{Error: "session not found"})
			return
		}
		// Consume body.
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})

	// POST /api/v1/sessions/{sessionID}/ttl
	mux.HandleFunc("POST /api/v1/sessions/{sessionID}/ttl", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("sessionID")
		if _, ok := sessions[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(errorResponse{Error: "session not found"})
			return
		}
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})

	return httptest.NewServer(mux)
}

// --- Tests ---

func TestCreateSession(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())

	sess, err := store.CreateSession(context.Background(), session.CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
		TTL:       30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if sess.AgentName != "test-agent" {
		t.Fatalf("expected agent test-agent, got %s", sess.AgentName)
	}
	if sess.ExpiresAt.IsZero() {
		t.Fatal("expected non-zero ExpiresAt with TTL")
	}
}

func TestGetSession_Found(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())

	// Create a session first.
	created, err := store.CreateSession(context.Background(), session.CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Now get it.
	sess, err := store.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if sess.ID != created.ID {
		t.Fatalf("expected ID %s, got %s", created.ID, sess.ID)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())

	_, err := store.GetSession(context.Background(), "nonexistent")
	if err != session.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestAppendMessage_OK(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())

	created, err := store.CreateSession(context.Background(), session.CreateSessionOptions{
		AgentName: "a",
		Namespace: "ns",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = store.AppendMessage(context.Background(), created.ID, session.Message{
		ID:      "m1",
		Role:    session.RoleUser,
		Content: "hello",
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
}

func TestAppendMessage_NotFound(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())

	err := store.AppendMessage(context.Background(), "nonexistent", session.Message{
		ID: "m1", Role: session.RoleUser, Content: "hi",
	})
	if err != session.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestUpdateSessionStats_OK(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())

	created, err := store.CreateSession(context.Background(), session.CreateSessionOptions{
		AgentName: "a",
		Namespace: "ns",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = store.UpdateSessionStats(context.Background(), created.ID, session.SessionStatsUpdate{
		AddInputTokens:  100,
		AddOutputTokens: 50,
		AddMessages:     1,
	})
	if err != nil {
		t.Fatalf("update stats: %v", err)
	}
}

func TestUpdateSessionStats_NotFound(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())

	err := store.UpdateSessionStats(context.Background(), "nonexistent", session.SessionStatsUpdate{
		AddMessages: 1,
	})
	if err != session.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestRefreshTTL_OK(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())

	created, err := store.CreateSession(context.Background(), session.CreateSessionOptions{
		AgentName: "a",
		Namespace: "ns",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = store.RefreshTTL(context.Background(), created.ID, 2*time.Hour)
	if err != nil {
		t.Fatalf("refresh TTL: %v", err)
	}
}

func TestRefreshTTL_NotFound(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())

	err := store.RefreshTTL(context.Background(), "nonexistent", time.Hour)
	if err != session.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestNotImplementedMethods(t *testing.T) {
	store := NewStore("http://unused", logr.Discard())
	ctx := context.Background()

	if err := store.DeleteSession(ctx, "x"); err != ErrNotImplemented {
		t.Fatalf("DeleteSession: expected ErrNotImplemented, got %v", err)
	}
	if _, err := store.GetMessages(ctx, "x"); err != ErrNotImplemented {
		t.Fatalf("GetMessages: expected ErrNotImplemented, got %v", err)
	}
	if err := store.SetState(ctx, "x", "k", "v"); err != ErrNotImplemented {
		t.Fatalf("SetState: expected ErrNotImplemented, got %v", err)
	}
	if _, err := store.GetState(ctx, "x", "k"); err != ErrNotImplemented {
		t.Fatalf("GetState: expected ErrNotImplemented, got %v", err)
	}
}

func TestClose(t *testing.T) {
	store := NewStore("http://unused", logr.Discard())
	if err := store.Close(); err != nil {
		t.Fatalf("Close: unexpected error: %v", err)
	}
}

func TestConnectionError(t *testing.T) {
	// Point to a server that doesn't exist.
	store := NewStore("http://127.0.0.1:1", logr.Discard())

	_, err := store.GetSession(context.Background(), "x")
	if err == nil {
		t.Fatal("expected connection error")
	}
}

func TestServerErrorResponses(t *testing.T) {
	// Server that returns 500 for everything.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(errorResponse{Error: "internal error"})
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	ctx := context.Background()

	_, err := store.CreateSession(ctx, session.CreateSessionOptions{AgentName: "a", Namespace: "ns"})
	if err == nil {
		t.Fatal("CreateSession: expected error")
	}

	_, err = store.GetSession(ctx, "x")
	if err == nil {
		t.Fatal("GetSession: expected error")
	}

	if err := store.AppendMessage(ctx, "x", session.Message{ID: "m1", Role: session.RoleUser, Content: "hi"}); err == nil {
		t.Fatal("AppendMessage: expected error")
	}

	if err := store.UpdateSessionStats(ctx, "x", session.SessionStatsUpdate{AddMessages: 1}); err == nil {
		t.Fatal("UpdateSessionStats: expected error")
	}

	if err := store.RefreshTTL(ctx, "x", time.Hour); err == nil {
		t.Fatal("RefreshTTL: expected error")
	}
}

func TestWithHTTPTimeout(t *testing.T) {
	store := NewStore("http://unused", logr.Discard(), WithHTTPTimeout(5*time.Second))
	if store.httpClient.Timeout != 5*time.Second {
		t.Fatalf("expected 5s timeout, got %v", store.httpClient.Timeout)
	}
}

func TestWithHTTPClient(t *testing.T) {
	custom := &http.Client{Timeout: 99 * time.Second}
	store := NewStore("http://unused", logr.Discard(), WithHTTPClient(custom))
	if store.httpClient != custom {
		t.Fatal("expected custom HTTP client to be used")
	}
}

func TestDefaultHTTPTimeout(t *testing.T) {
	store := NewStore("http://unused", logr.Discard())
	if store.httpClient.Timeout != DefaultHTTPTimeout {
		t.Fatalf("expected default timeout %v, got %v", DefaultHTTPTimeout, store.httpClient.Timeout)
	}
}

func TestReadErrorInvalidJSON(t *testing.T) {
	// Server that returns 500 with non-JSON body.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	_, err := store.GetSession(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
	// Should fall back to "HTTP 500" format.
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected error to contain status code, got: %v", err)
	}
}
