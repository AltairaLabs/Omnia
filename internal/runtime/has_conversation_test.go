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

package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/statestore"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// errStore is a Store whose Load fails with a transport-style error, standing
// in for an unreachable Redis. Only Load is exercised; the remaining methods
// exist to satisfy the interface.
type errStore struct {
	statestore.Store
	err error
}

func (e *errStore) Load(context.Context, string) (*statestore.ConversationState, error) {
	return nil, e.err
}

// nilStore returns a miss the way a custom store might — nil state, nil error —
// rather than via the ErrNotFound sentinel the bundled stores use.
type nilStore struct {
	statestore.Store
}

func (n *nilStore) Load(context.Context, string) (*statestore.ConversationState, error) {
	return nil, nil
}

func TestHasConversation_RequiresSessionID(t *testing.T) {
	server := NewServer(WithLogger(logr.Discard()))

	_, err := server.HasConversation(context.Background(), &runtimev1.HasConversationRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// A conversation held by an in-flight Converse stream must report resumable
// even when the store knows nothing about it, because getOrCreateConversation
// short-circuits on the process-local map before consulting the store.
func TestHasConversation_LiveConversationShortCircuitsStore(t *testing.T) {
	server := NewServer(WithLogger(logr.Discard()))
	server.conversations["live-session"] = nil

	resp, err := server.HasConversation(context.Background(),
		&runtimev1.HasConversationRequest{SessionId: "live-session"})

	require.NoError(t, err)
	assert.Equal(t, runtimev1.ResumeState_RESUME_STATE_RESUMABLE, resp.State)
}

func TestHasConversation_StoreStates(t *testing.T) {
	saved := statestore.NewMemoryStore()
	require.NoError(t, saved.Save(context.Background(), &statestore.ConversationState{ID: "saved-session"}))

	tests := []struct {
		name  string
		store statestore.Store
		id    string
		want  runtimev1.ResumeState
	}{
		{
			name:  "state present is resumable",
			store: saved,
			id:    "saved-session",
			want:  runtimev1.ResumeState_RESUME_STATE_RESUMABLE,
		},
		{
			// The bug this guards: MemoryStore signals a miss with ErrNotFound,
			// so treating every error as a store failure would report an expired
			// session as UNAVAILABLE and never expire anything.
			name:  "absent state is not found, not unavailable",
			store: saved,
			id:    "never-existed",
			want:  runtimev1.ResumeState_RESUME_STATE_NOT_FOUND,
		},
		{
			name:  "nil state without error is not found",
			store: &nilStore{},
			id:    "any-session",
			want:  runtimev1.ResumeState_RESUME_STATE_NOT_FOUND,
		},
		{
			// An unreachable store must never present as expiry — the context
			// may be perfectly intact.
			name:  "store failure is unavailable",
			store: &errStore{err: errors.New("dial tcp: connection refused")},
			id:    "any-session",
			want:  runtimev1.ResumeState_RESUME_STATE_UNAVAILABLE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer(WithLogger(logr.Discard()), WithStateStore(tt.store))

			resp, err := server.HasConversation(context.Background(),
				&runtimev1.HasConversationRequest{SessionId: tt.id})

			require.NoError(t, err)
			assert.Equal(t, tt.want, resp.State)
		})
	}
}

// Probing must not extend the life of what it probes. The memory store treats
// its TTL as idle time and refreshes it on Load, so a Load-based probe would
// keep a memory-backed conversation alive indefinitely across reconnects while
// leaving a Redis-backed one — whose TTL runs from the last write — untouched.
// The same spec.context.ttl would then mean different things per backend.
func TestHasConversation_ProbeDoesNotExtendContextLifetime(t *testing.T) {
	const ttl = 150 * time.Millisecond

	store := statestore.NewMemoryStore(statestore.WithMemoryTTL(ttl))
	t.Cleanup(store.Close)

	ctx := context.Background()
	require.NoError(t, store.Save(ctx, &statestore.ConversationState{ID: "conv"}))

	server := NewServer(WithLogger(logr.Discard()), WithStateStore(store))
	req := &runtimev1.HasConversationRequest{SessionId: "conv"}

	// Probe repeatedly while the conversation is alive. Each of these would
	// reset the idle clock if the probe read through Load.
	deadline := time.Now().Add(ttl)
	for time.Now().Before(deadline) {
		resp, err := server.HasConversation(ctx, req)
		require.NoError(t, err)
		require.Equal(t, runtimev1.ResumeState_RESUME_STATE_RESUMABLE, resp.State)
		time.Sleep(ttl / 5)
	}

	// Past the configured idle window with no intervening use, so it must be
	// gone despite all the probing.
	time.Sleep(ttl / 2)

	resp, err := server.HasConversation(ctx, req)
	require.NoError(t, err)
	require.Equal(t, runtimev1.ResumeState_RESUME_STATE_NOT_FOUND, resp.State,
		"probing kept the conversation alive, so the probe is not a pure read")
}

// No store configured means sdk.Resume would fail with ErrNoStateStore for every
// session. That is a runtime misconfiguration, not a set of expired sessions,
// so it must not be reported as NOT_FOUND.
func TestHasConversation_NoStoreConfiguredIsUnavailable(t *testing.T) {
	server := NewServer(WithLogger(logr.Discard()))

	resp, err := server.HasConversation(context.Background(),
		&runtimev1.HasConversationRequest{SessionId: "any-session"})

	require.NoError(t, err)
	assert.Equal(t, runtimev1.ResumeState_RESUME_STATE_UNAVAILABLE, resp.State)
	assert.Contains(t, resp.Detail, "no state store")
}

// The contract that makes this RPC worth having: its verdict must match what
// sdk.Resume actually does against the same store. Asserted directly against
// the SDK's load-and-check rather than against a restatement of it, so a change
// in PromptKit's resume semantics fails here instead of silently desynchronising
// the facade's resume decision from the runtime's.
func TestHasConversation_AgreesWithResumePrecondition(t *testing.T) {
	store := statestore.NewMemoryStore()
	require.NoError(t, store.Save(context.Background(), &statestore.ConversationState{ID: "present"}))

	server := NewServer(WithLogger(logr.Discard()), WithStateStore(store))

	for _, id := range []string{"present", "absent"} {
		t.Run(id, func(t *testing.T) {
			// sdk.Resume's precondition, verbatim (sdk.go:934-940): it resumes
			// iff Load yields a non-nil state with no error.
			state, loadErr := store.Load(context.Background(), id)
			resumeWouldSucceed := loadErr == nil && state != nil

			resp, err := server.HasConversation(context.Background(),
				&runtimev1.HasConversationRequest{SessionId: id})
			require.NoError(t, err)

			probeSaysResumable := resp.State == runtimev1.ResumeState_RESUME_STATE_RESUMABLE
			assert.Equal(t, resumeWouldSucceed, probeSaysResumable,
				"probe verdict must match sdk.Resume's precondition for %q", id)
		})
	}
}
