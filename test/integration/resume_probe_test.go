//go:build integration

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

package integration

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/statestore"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/internal/runtime"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// startRuntimeServerWithStore starts a real runtime gRPC server backed by the
// given state store, so the resume probe is answered by the same component and
// the same store that sdk.Resume would consult.
func startRuntimeServerWithStore(t *testing.T, packPath string, store statestore.Store) (string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	runtimeServer := runtime.NewServer(
		runtime.WithLogger(logr.Discard()),
		runtime.WithPackPath(packPath),
		runtime.WithPromptName("default"),
		runtime.WithMockProvider(true),
		runtime.WithStateStore(store),
	)

	grpcServer := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(grpcServer, runtimeServer)

	go func() {
		_ = grpcServer.Serve(lis)
	}()

	return lis.Addr().String(), func() {
		grpcServer.GracefulStop()
		_ = runtimeServer.Close()
	}
}

// The resume decision crosses a process boundary in production: the facade asks
// the runtime over gRPC. Unit tests on either side can both pass while the RPC
// is unregistered or the wire enum is mistranslated, so this exercises the real
// client against the real server over a real connection.
func TestResumeProbe_AcrossRealGRPC(t *testing.T) {
	packPath := filepath.Join(t.TempDir(), "test.pack.json")
	writePromptPack(t, packPath)

	store := statestore.NewMemoryStore()
	t.Cleanup(store.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	require.NoError(t, store.Save(ctx, &statestore.ConversationState{ID: "live-conversation"}))

	addr, cleanup := startRuntimeServerWithStore(t, packPath, store)
	defer cleanup()

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 10 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	t.Run("a session with surviving context is resumable", func(t *testing.T) {
		state, err := client.HasConversation(ctx, "live-conversation")
		require.NoError(t, err)
		require.Equal(t, facade.ResumeStateResumable, state)
	})

	t.Run("a session with no context reports not found", func(t *testing.T) {
		// The #1876 case: session-api may still hold a row for this id, but the
		// working context is gone, so the conversation cannot be continued.
		state, err := client.HasConversation(ctx, "expired-conversation")
		require.NoError(t, err)
		require.Equal(t, facade.ResumeStateNotFound, state)
	})
}

// An unreachable runtime must surface as an error the facade reports as a
// server fault. It must never be mistaken for an expired session, which would
// discard a conversation whose context is intact.
func TestResumeProbe_UnreachableRuntimeIsNotExpiry(t *testing.T) {
	packPath := filepath.Join(t.TempDir(), "test.pack.json")
	writePromptPack(t, packPath)

	store := statestore.NewMemoryStore()
	t.Cleanup(store.Close)

	addr, cleanup := startRuntimeServerWithStore(t, packPath, store)

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 2 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	cleanup() // runtime goes away

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	state, probeErr := client.HasConversation(ctx, "any-session")
	require.Error(t, probeErr)
	require.NotEqual(t, facade.ResumeStateNotFound, state,
		"an unreachable runtime must not be reported as an expired session")
}
