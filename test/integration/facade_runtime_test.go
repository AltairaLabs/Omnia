//go:build integration

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

// Package integration contains integration tests that verify communication
// between facade and runtime containers without requiring a Kubernetes cluster.
package integration

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/altairalabs/omnia/internal/agent"
	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/internal/runtime"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// TestFacadeRuntimeGRPCCommunication tests the gRPC communication between
// the facade container and runtime container.
func TestFacadeRuntimeGRPCCommunication(t *testing.T) {
	// Create a temporary PromptPack file for the runtime
	tmpDir := t.TempDir()
	packPath := filepath.Join(tmpDir, "test.promptpack")
	writePromptPack(t, packPath)

	// Start the runtime gRPC server
	runtimeAddr, runtimeCleanup := startRuntimeServer(t, packPath)
	defer runtimeCleanup()

	// Create a RuntimeClient to connect facade to runtime
	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     runtimeAddr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err, "failed to create runtime client")
	defer func() { _ = client.Close() }()

	// Create the RuntimeHandler (used by facade)
	handler := agent.NewRuntimeHandler(client)

	t.Run("health_check", func(t *testing.T) {
		resp, err := client.Health(context.Background())
		require.NoError(t, err)
		assert.True(t, resp.Healthy)
		assert.Equal(t, "ready", resp.Status)
	})

	t.Run("simple_conversation", func(t *testing.T) {
		writer := &mockResponseWriter{}

		msg := &facade.ClientMessage{
			Content: "Hello, world!",
		}

		err := handler.HandleMessage(context.Background(), "session-1", msg, writer)
		require.NoError(t, err, "HandleMessage failed")

		// With mock provider, we should get a response
		assert.NotEmpty(t, writer.doneMsg, "expected done message")
	})

	t.Run("conversation_with_metadata", func(t *testing.T) {
		writer := &mockResponseWriter{}

		msg := &facade.ClientMessage{
			Content: "Test with metadata",
			Metadata: map[string]string{
				"user_id":    "user-123",
				"request_id": "req-456",
			},
		}

		err := handler.HandleMessage(context.Background(), "session-2", msg, writer)
		require.NoError(t, err, "HandleMessage with metadata failed")
		assert.NotEmpty(t, writer.doneMsg)
	})

	t.Run("multiple_messages_same_session", func(t *testing.T) {
		sessionID := "session-3"

		// First message
		writer1 := &mockResponseWriter{}
		err := handler.HandleMessage(context.Background(), sessionID, &facade.ClientMessage{
			Content: "First message",
		}, writer1)
		require.NoError(t, err)
		assert.NotEmpty(t, writer1.doneMsg)

		// Second message to same session
		writer2 := &mockResponseWriter{}
		err = handler.HandleMessage(context.Background(), sessionID, &facade.ClientMessage{
			Content: "Second message",
		}, writer2)
		require.NoError(t, err)
		assert.NotEmpty(t, writer2.doneMsg)
	})

	t.Run("concurrent_sessions", func(t *testing.T) {
		const numSessions = 5
		done := make(chan error, numSessions)

		for i := 0; i < numSessions; i++ {
			go func(sessionNum int) {
				writer := &mockResponseWriter{}
				msg := &facade.ClientMessage{
					Content: "Concurrent test message",
				}
				sessionID := "concurrent-session-" + string(rune('0'+sessionNum))
				done <- handler.HandleMessage(context.Background(), sessionID, msg, writer)
			}(i)
		}

		// Wait for all to complete
		for i := 0; i < numSessions; i++ {
			err := <-done
			assert.NoError(t, err, "concurrent session %d failed", i)
		}
	})
}

// TestRuntimeClientReconnection tests that the facade can handle runtime restarts.
func TestRuntimeClientReconnection(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := filepath.Join(tmpDir, "test.promptpack")
	writePromptPack(t, packPath)

	// Start initial runtime server
	runtimeAddr, runtimeCleanup := startRuntimeServer(t, packPath)

	// Create client
	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     runtimeAddr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	// Verify initial connection works
	resp, err := client.Health(context.Background())
	require.NoError(t, err)
	assert.True(t, resp.Healthy)

	// Stop the runtime
	runtimeCleanup()

	// Health check should fail now
	_, err = client.Health(context.Background())
	assert.Error(t, err, "expected error after runtime shutdown")
}

// TestRuntimeClientConnectionFailure tests behavior when runtime is unavailable.
func TestRuntimeClientConnectionFailure(t *testing.T) {
	_, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     "localhost:59999", // Non-existent port
		DialTimeout: 100 * time.Millisecond,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to runtime")
}

// startRuntimeServer starts a real runtime gRPC server and returns its address.
func startRuntimeServer(t *testing.T, packPath string) (string, func()) {
	t.Helper()

	// Find an available port
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	// Create the runtime server with mock provider
	runtimeServer := runtime.NewServer(
		runtime.WithLogger(logr.Discard()),
		runtime.WithPackPath(packPath),
		runtime.WithPromptName("default"),
		runtime.WithMockProvider(true),
	)

	// Create gRPC server and register runtime service
	grpcServer := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(grpcServer, runtimeServer)

	// Start serving in background
	go func() {
		_ = grpcServer.Serve(lis)
	}()

	cleanup := func() {
		grpcServer.GracefulStop()
		_ = runtimeServer.Close()
	}

	return lis.Addr().String(), cleanup
}

// writePromptPack creates a minimal valid PromptPack file for testing.
func writePromptPack(t *testing.T, path string) {
	t.Helper()

	content := `{
		"id": "test-pack",
		"name": "test-pack",
		"version": "1.0.0",
		"template_engine": {
			"version": "v1",
			"syntax": "{{variable}}"
		},
		"prompts": {
			"default": {
				"id": "default",
				"name": "default",
				"version": "1.0.0",
				"system_template": "You are a test assistant for integration testing."
			}
		}
	}`

	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
}

// mockResponseWriter implements facade.ResponseWriter for testing.
type mockResponseWriter struct {
	chunks      []string
	chunkParts  [][]facade.ContentPart
	doneMsg     string
	doneParts   []facade.ContentPart
	toolCalls   []*facade.ToolCallInfo
	toolResults []*facade.ToolResultInfo
	errors      []struct{ code, message string }
}

func (m *mockResponseWriter) WriteChunk(content string) error {
	m.chunks = append(m.chunks, content)
	return nil
}

func (m *mockResponseWriter) WriteChunkWithParts(parts []facade.ContentPart) error {
	m.chunkParts = append(m.chunkParts, parts)
	return nil
}

func (m *mockResponseWriter) WriteDone(content string) error {
	m.doneMsg = content
	return nil
}

func (m *mockResponseWriter) WriteDoneWithParts(parts []facade.ContentPart) error {
	m.doneParts = parts
	return nil
}

func (m *mockResponseWriter) WriteToolCall(info *facade.ToolCallInfo) error {
	m.toolCalls = append(m.toolCalls, info)
	return nil
}

func (m *mockResponseWriter) WriteToolResult(info *facade.ToolResultInfo) error {
	m.toolResults = append(m.toolResults, info)
	return nil
}

func (m *mockResponseWriter) WriteError(code, message string) error {
	m.errors = append(m.errors, struct{ code, message string }{code, message})
	return nil
}
