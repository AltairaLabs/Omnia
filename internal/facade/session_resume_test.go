/*
Copyright 2025-2026.

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

package facade

import (
	"context"
	"errors"
	"testing"

	"github.com/altairalabs/omnia/internal/session/sessiontest"
	"github.com/go-logr/logr"
)

// probeHandler is a MessageHandler that also implements ResumeProber, standing
// in for the runtime-backed handler.
type probeHandler struct {
	state ResumeState
	err   error
	calls []string
}

func (p *probeHandler) Name() string { return "probe" }

func (p *probeHandler) HandleMessage(context.Context, string, *ClientMessage, ResponseWriter) error {
	return nil
}

func (p *probeHandler) HasConversation(_ context.Context, sessionID string) (ResumeState, error) {
	p.calls = append(p.calls, sessionID)
	return p.state, p.err
}

func newResumeServer(t *testing.T, h MessageHandler) (*Server, *ensureSessionStore) {
	t.Helper()
	backing := sessiontest.NewStore()
	t.Cleanup(func() { _ = backing.Close() })
	store := &ensureSessionStore{Store: backing}
	return NewServer(DefaultServerConfig(), store, h, logr.Discard()), store
}

// A context that survives lets the conversation continue.
func TestEnsureSession_ResumableContextProceeds(t *testing.T) {
	handler := &probeHandler{state: ResumeStateResumable}
	server, _ := newResumeServer(t, handler)
	conn := &Connection{agentName: "agent", namespace: "default", workspaceName: "ws"}

	sessionID, err := server.ensureSession(context.Background(), conn, "prior-session", logr.Discard())
	if err != nil {
		t.Fatalf("ensureSession: %v", err)
	}
	if sessionID != "prior-session" {
		t.Fatalf("sessionID = %q, want %q", sessionID, "prior-session")
	}
	if len(handler.calls) != 1 || handler.calls[0] != "prior-session" {
		t.Fatalf("probe calls = %v, want [prior-session]", handler.calls)
	}
}

// The bug #1876 exists to fix: the context is gone, so the client must be told
// rather than silently handed a session with no history.
func TestEnsureSession_ExpiredContextReportsExpiry(t *testing.T) {
	handler := &probeHandler{state: ResumeStateNotFound}
	server, store := newResumeServer(t, handler)
	conn := &Connection{agentName: "agent", namespace: "default", workspaceName: "ws"}

	_, err := server.ensureSession(context.Background(), conn, "prior-session", logr.Discard())
	if !errors.Is(err, errSessionExpired) {
		t.Fatalf("ensureSession error = %v, want errSessionExpired", err)
	}
	if store.createCalls != 0 {
		t.Fatalf("EnsureSessionRecord called %d times, want 0", store.createCalls)
	}
}

// An unreachable context store must not present as an expiry — the context may
// be perfectly intact, and telling the client it expired would discard it.
func TestEnsureSession_UnavailableStoreIsNotExpiry(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler *probeHandler
	}{
		{"store unavailable", &probeHandler{state: ResumeStateUnavailable}},
		{"probe transport error", &probeHandler{err: errors.New("runtime unreachable")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, store := newResumeServer(t, tc.handler)
			conn := &Connection{agentName: "agent", namespace: "default", workspaceName: "ws"}

			_, err := server.ensureSession(context.Background(), conn, "prior-session", logr.Discard())
			if err == nil {
				t.Fatal("ensureSession error = nil, want an error")
			}
			if errors.Is(err, errSessionExpired) {
				t.Fatal("ensureSession reported expiry for an unavailable store")
			}
			if store.createCalls != 0 {
				t.Fatalf("EnsureSessionRecord called %d times, want 0", store.createCalls)
			}
		})
	}
}

// The id this connection minted and announced in `connected` is echoed back by
// clients on their very first message. Probing for it would find nothing and
// reject the opening turn of every new conversation.
func TestEnsureSession_OwnAnnouncedIDIsNotProbed(t *testing.T) {
	handler := &probeHandler{state: ResumeStateNotFound}
	server, store := newResumeServer(t, handler)
	conn := &Connection{
		agentName:     "agent",
		namespace:     "default",
		workspaceName: "ws",
		sessionID:     "minted-by-this-connection",
	}

	sessionID, err := server.ensureSession(context.Background(), conn,
		"minted-by-this-connection", logr.Discard())
	if err != nil {
		t.Fatalf("ensureSession: %v", err)
	}
	if sessionID != "minted-by-this-connection" {
		t.Fatalf("sessionID = %q, want %q", sessionID, "minted-by-this-connection")
	}
	if len(handler.calls) != 0 {
		t.Fatalf("probe calls = %v, want none", handler.calls)
	}
	if store.createCalls != 1 {
		t.Fatalf("EnsureSessionRecord called %d times, want 1", store.createCalls)
	}
}

// ensureSession runs for every message, so an established session must cost no
// session-api traffic and no further probing. Without the short-circuit this
// writes to the archive once per turn.
func TestEnsureSession_EstablishedSessionCostsNothing(t *testing.T) {
	handler := &probeHandler{state: ResumeStateResumable}
	server, store := newResumeServer(t, handler)
	conn := &Connection{agentName: "agent", namespace: "default", workspaceName: "ws"}

	// First message: probe resolves the context, archive row is written.
	sessionID, err := server.ensureSession(context.Background(), conn, "prior-session", logr.Discard())
	if err != nil {
		t.Fatalf("ensureSession(first): %v", err)
	}
	// processMessage stamps the connection once the session is established.
	conn.mu.Lock()
	conn.sessionID = sessionID
	conn.sessionPersisted = true
	conn.mu.Unlock()

	for i := range 5 {
		if _, err := server.ensureSession(context.Background(), conn, sessionID, logr.Discard()); err != nil {
			t.Fatalf("ensureSession(message %d): %v", i+2, err)
		}
	}

	if store.createCalls != 1 {
		t.Fatalf("EnsureSessionRecord called %d times across 6 messages, want 1", store.createCalls)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("probe called %d times across 6 messages, want 1", len(handler.calls))
	}
}

// A runtime built against an older contract version does not serve
// HasConversation, so gRPC answers Unimplemented. That is "cannot answer", not
// "gone" — failing the message would break resume against every such runtime,
// and reporting an expiry would discard a conversation that may be intact.
func TestEnsureSession_OlderRuntimeContractDegrades(t *testing.T) {
	handler := &probeHandler{err: ErrProbeUnsupported}
	server, store := newResumeServer(t, handler)
	conn := &Connection{agentName: "agent", namespace: "default", workspaceName: "ws"}

	sessionID, err := server.ensureSession(context.Background(), conn, "prior-session", logr.Discard())
	if err != nil {
		t.Fatalf("an older-contract runtime must not fail the message: %v", err)
	}
	if sessionID != "prior-session" {
		t.Fatalf("sessionID = %q, want %q", sessionID, "prior-session")
	}
	if store.createCalls != 1 {
		t.Fatalf("EnsureSessionRecord called %d times, want 1", store.createCalls)
	}
}

// A handler with no runtime behind it has no context store to consult, so the
// facade must let the session through rather than invent an expiry.
func TestEnsureSession_NonProbingHandlerAllowsSession(t *testing.T) {
	server, store := newResumeServer(t, nil)
	conn := &Connection{agentName: "agent", namespace: "default", workspaceName: "ws"}

	sessionID, err := server.ensureSession(context.Background(), conn, "prior-session", logr.Discard())
	if err != nil {
		t.Fatalf("ensureSession: %v", err)
	}
	if sessionID != "prior-session" {
		t.Fatalf("sessionID = %q, want %q", sessionID, "prior-session")
	}
	if store.createCalls != 1 {
		t.Fatalf("EnsureSessionRecord called %d times, want 1", store.createCalls)
	}
}

// With no archive configured — session-api undiscoverable — a conversation must
// still work. Resumability comes from the context store, so the archive is a
// sink the facade can do without; previously this path was papered over by an
// in-memory store that satisfied the interface and discarded every write.
func TestEnsureSession_NoArchiveConfigured(t *testing.T) {
	handler := &probeHandler{state: ResumeStateResumable}
	server := NewServer(DefaultServerConfig(), nil, handler, logr.Discard())
	conn := &Connection{
		agentName:     "agent",
		namespace:     "default",
		workspaceName: "ws",
		sessionID:     "minted-by-connection",
	}

	// The id the connection already holds is the session.
	sessionID, err := server.ensureSession(context.Background(), conn, "minted-by-connection", logr.Discard())
	if err != nil {
		t.Fatalf("a missing archive must not fail the turn: %v", err)
	}
	if sessionID != "minted-by-connection" {
		t.Fatalf("sessionID = %q, want minted-by-connection", sessionID)
	}

	// A resume request is still answered by the context store, not the archive.
	resumed, err := server.ensureSession(context.Background(), conn, "prior-session", logr.Discard())
	if err != nil {
		t.Fatalf("resume must still work without an archive: %v", err)
	}
	if resumed != "prior-session" {
		t.Fatalf("sessionID = %q, want prior-session", resumed)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("expected the context store to be consulted once, got %v", handler.calls)
	}
}

// And an expired context must still be reported honestly with no archive.
func TestEnsureSession_NoArchiveStillReportsExpiry(t *testing.T) {
	handler := &probeHandler{state: ResumeStateNotFound}
	server := NewServer(DefaultServerConfig(), nil, handler, logr.Discard())
	conn := &Connection{agentName: "agent", namespace: "default", workspaceName: "ws"}

	if _, err := server.ensureSession(context.Background(), conn, "prior-session", logr.Discard()); !errors.Is(err, errSessionExpired) {
		t.Fatalf("expected errSessionExpired, got %v", err)
	}
}
