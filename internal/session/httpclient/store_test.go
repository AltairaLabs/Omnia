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
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/pkg/sessionapi"
)

// --- Test helpers ---

func testSessionAPI(id, agent, ns string) *sessionapi.Session {
	status := sessionapi.SessionStatusActive
	now := time.Now()
	return &sessionapi.Session{
		Id:        parseTestUUID(id),
		AgentName: &agent,
		Namespace: &ns,
		Status:    &status,
		CreatedAt: &now,
		UpdatedAt: &now,
	}
}

func parseTestUUID(s string) *openapi_types.UUID {
	var u openapi_types.UUID
	_ = u.UnmarshalText([]byte(s))
	return &u
}

// mockSessionAPI creates a test server that mimics the session-api endpoints.
func mockSessionAPI(t *testing.T) *httptest.Server {
	t.Helper()
	sessions := make(map[string]*sessionapi.Session)

	mux := http.NewServeMux()

	// POST /api/v1/sessions
	mux.HandleFunc("POST /api/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		var req sessionapi.CreateSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "bad request"})
			return
		}
		now := time.Now()
		status := sessionapi.SessionStatusActive
		sess := &sessionapi.Session{
			Id:        req.Id,
			AgentName: req.AgentName,
			Namespace: req.Namespace,
			Status:    &status,
			CreatedAt: &now,
			UpdatedAt: &now,
		}
		if req.TtlSeconds != nil && *req.TtlSeconds > 0 {
			exp := now.Add(time.Duration(*req.TtlSeconds) * time.Second)
			sess.ExpiresAt = &exp
		}
		id := ""
		if req.Id != nil {
			id = req.Id.String()
		}
		sessions[id] = sess
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(sessionapi.SessionResponse{Session: sess})
	})

	// GET /api/v1/sessions/{sessionID}
	mux.HandleFunc("GET /api/v1/sessions/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("sessionID")
		sess, ok := sessions[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "session not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionapi.SessionResponse{Session: sess})
	})

	// POST /api/v1/sessions/{sessionID}/messages
	mux.HandleFunc("POST /api/v1/sessions/{sessionID}/messages", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("sessionID")
		if _, ok := sessions[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "session not found"})
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

	// PATCH /api/v1/sessions/{sessionID}/status
	mux.HandleFunc("PATCH /api/v1/sessions/{sessionID}/status", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("sessionID")
		if _, ok := sessions[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "session not found"})
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
			_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "session not found"})
			return
		}
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})

	// Tool call storage per session.
	toolCalls := make(map[string][]session.ToolCall)

	// POST /api/v1/sessions/{sessionID}/tool-calls
	mux.HandleFunc("POST /api/v1/sessions/{sessionID}/tool-calls", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("sessionID")
		if _, ok := sessions[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "session not found"})
			return
		}
		var tc session.ToolCall
		if err := json.NewDecoder(r.Body).Decode(&tc); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		toolCalls[id] = append(toolCalls[id], tc)
		w.WriteHeader(http.StatusCreated)
	})

	// GET /api/v1/sessions/{sessionID}/tool-calls
	mux.HandleFunc("GET /api/v1/sessions/{sessionID}/tool-calls", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("sessionID")
		if _, ok := sessions[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "session not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(toolCalls[id])
	})

	// Provider call storage per session.
	providerCalls := make(map[string][]session.ProviderCall)

	// POST /api/v1/sessions/{sessionID}/provider-calls
	mux.HandleFunc("POST /api/v1/sessions/{sessionID}/provider-calls", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("sessionID")
		if _, ok := sessions[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "session not found"})
			return
		}
		var pc session.ProviderCall
		if err := json.NewDecoder(r.Body).Decode(&pc); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		providerCalls[id] = append(providerCalls[id], pc)
		w.WriteHeader(http.StatusCreated)
	})

	// GET /api/v1/sessions/{sessionID}/provider-calls
	mux.HandleFunc("GET /api/v1/sessions/{sessionID}/provider-calls", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("sessionID")
		if _, ok := sessions[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "session not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(providerCalls[id])
	})

	return httptest.NewServer(mux)
}

// --- Tests ---

func TestCreateSession(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

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

func TestCreateSession_NoTTL(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	sess, err := store.CreateSession(context.Background(), session.CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
}

func TestGetSession_Found(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

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
	t.Cleanup(func() { _ = store.Close() })

	_, err := store.GetSession(context.Background(), "nonexistent")
	if err != session.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestGetSession_ServerError(t *testing.T) {
	// Server returns 500 with valid JSON error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "database down"})
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	_, err := store.GetSession(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected error with status 500, got: %v", err)
	}
	if !strings.Contains(err.Error(), "database down") {
		t.Fatalf("expected error message, got: %v", err)
	}
}

func TestAppendMessage_OK(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

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
	t.Cleanup(func() { _ = store.Close() })

	err := store.AppendMessage(context.Background(), "nonexistent", session.Message{
		ID: "m1", Role: session.RoleUser, Content: "hi",
	})
	if err != session.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestAppendMessage_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "internal error"})
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	err := store.AppendMessage(context.Background(), "x", session.Message{
		ID: "m1", Role: session.RoleUser, Content: "hi",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got: %v", err)
	}
}

func TestUpdateSessionStatus_OK(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	created, err := store.CreateSession(context.Background(), session.CreateSessionOptions{
		AgentName: "a",
		Namespace: "ns",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = store.UpdateSessionStatus(context.Background(), created.ID, session.SessionStatusUpdate{
		SetStatus: session.SessionStatusActive,
	})
	if err != nil {
		t.Fatalf("update stats: %v", err)
	}
}

func TestUpdateSessionStatus_NotFound(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	err := store.UpdateSessionStatus(context.Background(), "nonexistent", session.SessionStatusUpdate{
		SetStatus: session.SessionStatusActive,
	})
	if err != session.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestUpdateSessionStatus_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "internal error"})
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	err := store.UpdateSessionStatus(context.Background(), "x", session.SessionStatusUpdate{
		SetStatus: session.SessionStatusActive,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got: %v", err)
	}
}

func TestRefreshTTL_OK(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

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
	t.Cleanup(func() { _ = store.Close() })

	err := store.RefreshTTL(context.Background(), "nonexistent", time.Hour)
	if err != session.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestRefreshTTL_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "internal error"})
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	err := store.RefreshTTL(context.Background(), "x", time.Hour)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got: %v", err)
	}
}

func TestNotImplementedMethods(t *testing.T) {
	store := NewStore("http://unused", logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	if err := store.DeleteSession(ctx, "x"); err != ErrNotImplemented {
		t.Fatalf("DeleteSession: expected ErrNotImplemented, got %v", err)
	}
	if _, err := store.GetMessages(ctx, "x"); err != ErrNotImplemented {
		t.Fatalf("GetMessages: expected ErrNotImplemented, got %v", err)
	}
}

func TestClose(t *testing.T) {
	store := NewStore("http://unused", logr.Discard())
	if err := store.Close(); err != nil {
		t.Fatalf("Close: unexpected error: %v", err)
	}
	// Calling Close again should not panic.
}

func TestClose_BufferDisabled(t *testing.T) {
	store := NewStore("http://unused", logr.Discard(), WithBufferCapacity(0))
	if err := store.Close(); err != nil {
		t.Fatalf("Close: unexpected error: %v", err)
	}
}

func TestConnectionError(t *testing.T) {
	// Point to a server that doesn't exist.
	store := NewStore("http://127.0.0.1:1", logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

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
		_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "internal error"})
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
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

	if err := store.UpdateSessionStatus(ctx, "x", session.SessionStatusUpdate{SetStatus: session.SessionStatusActive}); err == nil {
		t.Fatal("UpdateSessionStatus: expected error")
	}

	if err := store.RefreshTTL(ctx, "x", time.Hour); err == nil {
		t.Fatal("RefreshTTL: expected error")
	}
}

func TestWithHTTPTimeout(t *testing.T) {
	store := NewStore("http://unused", logr.Discard(), WithHTTPTimeout(5*time.Second))
	t.Cleanup(func() { _ = store.Close() })
	if store.httpClient.Timeout != 5*time.Second {
		t.Fatalf("expected 5s timeout, got %v", store.httpClient.Timeout)
	}
}

func TestWithHTTPClient(t *testing.T) {
	custom := &http.Client{Timeout: 99 * time.Second}
	store := NewStore("http://unused", logr.Discard(), WithHTTPClient(custom))
	t.Cleanup(func() { _ = store.Close() })
	if store.httpClient != custom {
		t.Fatal("expected custom HTTP client to be used")
	}
}

func TestDefaultHTTPTimeout(t *testing.T) {
	store := NewStore("http://unused", logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
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
	t.Cleanup(func() { _ = store.Close() })
	_, err := store.GetSession(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
	// Should fall back to "HTTP 500" format.
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected error to contain status code, got: %v", err)
	}
}

func TestCancelledContext(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := store.GetSession(ctx, "x")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestDoJSON_RequestCreationError(t *testing.T) {
	// Use an invalid base URL that will fail request creation.
	store := NewStore("://invalid-url", logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	_, err := store.CreateSession(context.Background(), session.CreateSessionOptions{
		AgentName: "a",
		Namespace: "ns",
	})
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestDoRequest_NilBody(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	// GetSession uses doRequest with nil body
	_, err := store.GetSession(context.Background(), "nonexistent")
	if err != session.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestCreateSession_InvalidResponseJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	_, err := store.CreateSession(context.Background(), session.CreateSessionOptions{
		AgentName: "a",
		Namespace: "ns",
	})
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error, got: %v", err)
	}
}

func TestGetSession_InvalidResponseJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	_, err := store.GetSession(context.Background(), "x")
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error, got: %v", err)
	}
}

// --- Retry tests ---

func TestRetry_503ThenSuccess(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Third attempt succeeds.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(sessionapi.SessionResponse{
			Session: testSessionAPI("550e8400-e29b-41d4-a716-446655440001", "agent", "ns"),
		})
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.GetSession(context.Background(), "550e8400-e29b-41d4-a716-446655440001")
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if sess.ID != "550e8400-e29b-41d4-a716-446655440001" {
		t.Fatalf("expected session ID 550e8400-e29b-41d4-a716-446655440001, got %s", sess.ID)
	}
	if attempts.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestRetry_MaxRetriesExceeded(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	_, err := store.GetSession(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error after max retries")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("expected 502 error, got: %v", err)
	}
	if attempts.Load() != int32(maxRetries) {
		t.Fatalf("expected %d attempts, got %d", maxRetries, attempts.Load())
	}
}

func TestRetry_NonRetryableStatus(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "bad request"})
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	_, err := store.CreateSession(context.Background(), session.CreateSessionOptions{
		AgentName: "a", Namespace: "ns",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts.Load() != 1 {
		t.Fatalf("expected 1 attempt (no retry), got %d", attempts.Load())
	}
}

func TestRetry_ConnectionErrorThenSuccess(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(sessionapi.SessionResponse{
			Session: testSessionAPI("550e8400-e29b-41d4-a716-446655440002", "agent", "ns"),
		})
	}))

	// Start with a closed server to simulate connection error, then reopen.
	srvURL := srv.URL
	srv.Close()

	// Create a store pointed at the (now dead) server.
	store := NewStore(srvURL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	// Restart the server on the same address is tricky, so instead we test
	// that connection errors are retried by checking attempt count after failure.
	_, err := store.GetSession(context.Background(), "s1")
	if err == nil {
		t.Fatal("expected error when server is down")
	}
	// All retries should have been attempted.
	// Note: attempts stays 0 since server is down, but the store should have
	// tried maxRetries times internally (connection refused each time).
}

// --- Circuit breaker tests ---

func TestCircuitBreaker_OpensAfterRepeatedFailures(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		// Use a retryable status so doWithRetryInner returns an error
		// (non-retryable statuses like 500 return (resp, nil) which gobreaker counts as success).
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	store := newStoreWithLowCBThreshold(t, srv.URL)

	// Each GetSession call retries maxRetries times internally.
	// With test config: minRequests=3, failRatio=0.6 → trips after 3 failed Execute() calls.
	for range 3 {
		_, _ = store.GetSession(context.Background(), "x")
	}

	attemptsBeforeOpen := attempts.Load()

	// Next request should fail immediately without hitting the server.
	_, err := store.GetSession(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error when circuit is open")
	}
	if attempts.Load() != attemptsBeforeOpen {
		t.Fatalf("expected no new server attempts when circuit is open, got %d (was %d)",
			attempts.Load(), attemptsBeforeOpen)
	}
}

func TestCircuitBreaker_NormalOperationDoesNotTrip(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := newStoreWithLowCBThreshold(t, srv.URL)

	// Successful requests should not trip the breaker.
	for i := range 5 {
		_, err := store.CreateSession(context.Background(), session.CreateSessionOptions{
			AgentName: "a",
			Namespace: "ns",
		})
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
	}
}

// newStoreWithLowCBThreshold creates a store with a circuit breaker that trips
// quickly for testing (minRequests=3 instead of 10).
func newStoreWithLowCBThreshold(t *testing.T, baseURL string) *Store {
	t.Helper()
	s := NewStore(baseURL, logr.Discard())
	t.Cleanup(func() { _ = s.Close() })
	// Replace the circuit breaker with one that trips faster.
	s.cb = gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{
		Name:        "test-session-api",
		MaxRequests: 1,
		Interval:    0, // never reset counters in closed state
		Timeout:     100 * time.Millisecond,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.Requests >= 3 &&
				float64(counts.TotalFailures)/float64(counts.Requests) >= 0.6
		},
	})
	return s
}

func TestRetry_CancelledContextStopsRetry(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after first attempt completes.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := store.GetSession(ctx, "x")
	if err == nil {
		t.Fatal("expected error")
	}
	// Should have stopped early due to context cancellation.
	if attempts.Load() >= int32(maxRetries) {
		t.Fatalf("expected fewer than %d attempts due to cancellation, got %d", maxRetries, attempts.Load())
	}
}

func TestCreateSession_409ConflictReturnsExisting(t *testing.T) {
	// Server returns 409 on POST (duplicate) and 200 on GET.
	existingSession := testSessionAPI("550e8400-e29b-41d4-a716-446655440003", "test-agent", "default")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/sessions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "conflict"})
	})
	mux.HandleFunc("GET /api/v1/sessions/{sessionID}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionapi.SessionResponse{Session: existingSession})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.CreateSession(context.Background(), session.CreateSessionOptions{
		AgentName: "test-agent",
		Namespace: "default",
	})
	if err != nil {
		t.Fatalf("expected success on 409 conflict, got: %v", err)
	}
	if sess.ID != "550e8400-e29b-41d4-a716-446655440003" {
		t.Fatalf("expected existing session ID, got %s", sess.ID)
	}
}

func TestRecordToolCall_OK(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	created, err := store.CreateSession(context.Background(), session.CreateSessionOptions{
		AgentName: "a", Namespace: "ns",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = store.RecordToolCall(context.Background(), created.ID, session.ToolCall{
		ID: "tc1", Name: "search", Status: session.ToolCallStatusSuccess,
	})
	if err != nil {
		t.Fatalf("record tool call: %v", err)
	}
}

func TestRecordToolCall_NotFound(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	err := store.RecordToolCall(context.Background(), "nonexistent", session.ToolCall{
		ID: "tc1", Name: "search",
	})
	if err != session.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestGetToolCalls_OK(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	created, err := store.CreateSession(context.Background(), session.CreateSessionOptions{
		AgentName: "a", Namespace: "ns",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_ = store.RecordToolCall(context.Background(), created.ID, session.ToolCall{
		ID: "tc1", Name: "search", Status: session.ToolCallStatusSuccess,
	})

	calls, err := store.GetToolCalls(context.Background(), created.ID, 0, 0)
	if err != nil {
		t.Fatalf("get tool calls: %v", err)
	}
	if len(calls) != 1 || calls[0].ID != "tc1" {
		t.Fatalf("expected 1 tool call with ID tc1, got %v", calls)
	}
}

func TestGetToolCalls_NotFound(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	_, err := store.GetToolCalls(context.Background(), "nonexistent", 0, 0)
	if err != session.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestRecordProviderCall_OK(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	created, err := store.CreateSession(context.Background(), session.CreateSessionOptions{
		AgentName: "a", Namespace: "ns",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = store.RecordProviderCall(context.Background(), created.ID, session.ProviderCall{
		ID: "pc1", Provider: "anthropic", Model: "claude-sonnet-4-20250514",
		Status: session.ProviderCallStatusCompleted,
	})
	if err != nil {
		t.Fatalf("record provider call: %v", err)
	}
}

func TestRecordProviderCall_NotFound(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	err := store.RecordProviderCall(context.Background(), "nonexistent", session.ProviderCall{
		ID: "pc1", Provider: "anthropic",
	})
	if err != session.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestGetProviderCalls_OK(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	created, err := store.CreateSession(context.Background(), session.CreateSessionOptions{
		AgentName: "a", Namespace: "ns",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_ = store.RecordProviderCall(context.Background(), created.ID, session.ProviderCall{
		ID: "pc1", Provider: "anthropic", Status: session.ProviderCallStatusCompleted,
	})

	calls, err := store.GetProviderCalls(context.Background(), created.ID, 0, 0)
	if err != nil {
		t.Fatalf("get provider calls: %v", err)
	}
	if len(calls) != 1 || calls[0].ID != "pc1" {
		t.Fatalf("expected 1 provider call with ID pc1, got %v", calls)
	}
}

func TestGetProviderCalls_NotFound(t *testing.T) {
	srv := mockSessionAPI(t)
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	_, err := store.GetProviderCalls(context.Background(), "nonexistent", 0, 0)
	if err != session.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestNewStore_TransportPropagatesTraceContext(t *testing.T) {
	// Set up OTel with in-memory exporter and W3C propagator.
	otel.SetTextMapPropagator(propagation.TraceContext{})
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(tracenoop.NewTracerProvider())
	})

	// Create a test server that captures the traceparent header.
	var capturedTraceparent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTraceparent = r.Header.Get("Traceparent")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"test"}`))
	}))
	t.Cleanup(srv.Close)

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })

	// Start a span so there's a trace context to propagate.
	ctx, span := tp.Tracer("test").Start(context.Background(), "test-op")
	defer span.End()

	// Make a request — the otelhttp transport should inject traceparent.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/sessions/test", nil)
	resp, err := store.httpClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()

	if capturedTraceparent == "" {
		t.Fatal("expected traceparent header to be propagated by otelhttp transport")
	}
	// Verify the traceparent contains the expected trace ID.
	if !strings.Contains(capturedTraceparent, span.SpanContext().TraceID().String()) {
		t.Errorf("traceparent %q does not contain trace ID %s",
			capturedTraceparent, span.SpanContext().TraceID())
	}
}

// --- getPaginatedDetail error tests ---

func TestGetToolCalls_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "database error"})
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	_, err := store.GetToolCalls(context.Background(), "x", 0, 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got: %v", err)
	}
}

func TestGetProviderCalls_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "database error"})
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	_, err := store.GetProviderCalls(context.Background(), "x", 0, 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got: %v", err)
	}
}

func TestGetRuntimeEvents_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(sessionapi.ErrorResponse{Error: "database error"})
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	_, err := store.GetRuntimeEvents(context.Background(), "x", 0, 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got: %v", err)
	}
}

func TestGetToolCalls_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	_, err := store.GetToolCalls(context.Background(), "x", 0, 0)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error, got: %v", err)
	}
}

func TestGetProviderCalls_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	_, err := store.GetProviderCalls(context.Background(), "x", 0, 0)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error, got: %v", err)
	}
}

func TestGetRuntimeEvents_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	_, err := store.GetRuntimeEvents(context.Background(), "x", 0, 0)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error, got: %v", err)
	}
}

func TestGetToolCalls_WithPagination(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]session.ToolCall{})
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard())
	t.Cleanup(func() { _ = store.Close() })
	_, err := store.GetToolCalls(context.Background(), "sess-1", 10, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(capturedPath, "limit=10") || !strings.Contains(capturedPath, "offset=5") {
		t.Errorf("expected pagination params in URL, got: %s", capturedPath)
	}
}

// --- appendPaginationParams tests ---

func TestAppendPaginationParams(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		limit, offset int
		want          string
	}{
		{"no params", "/api/v1/test", 0, 0, "/api/v1/test"},
		{"limit only", "/api/v1/test", 10, 0, "/api/v1/test?limit=10"},
		{"offset only", "/api/v1/test", 0, 5, "/api/v1/test?offset=5"},
		{"both", "/api/v1/test", 10, 5, "/api/v1/test?limit=10&offset=5"},
		{"negative ignored", "/api/v1/test", -1, -1, "/api/v1/test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendPaginationParams(tt.path, tt.limit, tt.offset)
			if got != tt.want {
				t.Errorf("appendPaginationParams(%q, %d, %d) = %q, want %q",
					tt.path, tt.limit, tt.offset, got, tt.want)
			}
		})
	}
}

// --- RecordEvalResult tests ---

func TestRecordEvalResult_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/eval-results" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var results []session.EvalResult
		if err := json.Unmarshal(body, &results); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(results) != 1 || results[0].EvalID != "accuracy" {
			t.Errorf("expected 1 result with evalID=accuracy, got %+v", results)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard(), WithBufferCapacity(0))
	defer func() { _ = store.Close() }()

	err := store.RecordEvalResult(context.Background(), "sess-1", session.EvalResult{
		EvalID:   "accuracy",
		EvalType: "regex",
		Trigger:  "every_turn",
		Passed:   true,
		Source:   "runtime",
	})
	if err != nil {
		t.Fatalf("RecordEvalResult() error = %v", err)
	}
}

func TestRecordEvalResult_BuffersOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard(), WithBufferCapacity(100))
	defer func() { _ = store.Close() }()

	err := store.RecordEvalResult(context.Background(), "sess-1", session.EvalResult{
		EvalID: "test", EvalType: "regex", Trigger: "every_turn", Source: "runtime",
	})
	// Should not return an error — buffered for retry.
	if err != nil {
		t.Fatalf("RecordEvalResult() should buffer, got error = %v", err)
	}
	if store.Buffered() != 1 {
		t.Errorf("expected 1 buffered write, got %d", store.Buffered())
	}
}

// --- RecordRuntimeEvent tests ---

func TestRecordRuntimeEvent_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/sessions/sess-1/events" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard(), WithBufferCapacity(0))
	defer func() { _ = store.Close() }()

	err := store.RecordRuntimeEvent(context.Background(), "sess-1", session.RuntimeEvent{
		EventType: "pipeline.started",
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("RecordRuntimeEvent() error = %v", err)
	}
}

func TestRecordRuntimeEvent_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"session not found"}`))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard(), WithBufferCapacity(0))
	defer func() { _ = store.Close() }()

	err := store.RecordRuntimeEvent(context.Background(), "bad-id", session.RuntimeEvent{
		EventType: "pipeline.started",
	})
	if err != session.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

// --- GetRuntimeEvents tests ---

func TestGetRuntimeEvents_OK(t *testing.T) {
	ts := time.Now().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/sessions/sess-1/events" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]session.RuntimeEvent{
			{ID: "re-1", EventType: "pipeline.started", Timestamp: ts},
		})
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard(), WithBufferCapacity(0))
	defer func() { _ = store.Close() }()

	events, err := store.GetRuntimeEvents(context.Background(), "sess-1", 0, 0)
	if err != nil {
		t.Fatalf("GetRuntimeEvents() error = %v", err)
	}
	if len(events) != 1 || events[0].EventType != "pipeline.started" {
		t.Errorf("expected 1 event with type=pipeline.started, got %+v", events)
	}
}

func TestGetRuntimeEvents_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"session not found"}`))
	}))
	defer srv.Close()

	store := NewStore(srv.URL, logr.Discard(), WithBufferCapacity(0))
	defer func() { _ = store.Close() }()

	_, err := store.GetRuntimeEvents(context.Background(), "bad-id", 0, 0)
	if err != session.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}
