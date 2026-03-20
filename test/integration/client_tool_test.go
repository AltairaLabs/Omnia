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

package integration

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/altairalabs/omnia/internal/agent"
	"github.com/altairalabs/omnia/internal/facade"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// concurrentMockWriter is a thread-safe ResponseWriter for tests where
// HandleMessage runs in a goroutine while assertions read from the writer.
type concurrentMockWriter struct {
	mu        sync.Mutex
	chunks    []string
	doneMsg   string
	toolCalls []*facade.ToolCallInfo
}

func (m *concurrentMockWriter) WriteChunk(content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chunks = append(m.chunks, content)
	return nil
}
func (m *concurrentMockWriter) WriteChunkWithParts(_ []facade.ContentPart) error { return nil }
func (m *concurrentMockWriter) WriteDone(content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.doneMsg = content
	return nil
}
func (m *concurrentMockWriter) WriteDoneWithParts(_ []facade.ContentPart) error { return nil }
func (m *concurrentMockWriter) WriteToolCall(info *facade.ToolCallInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolCalls = append(m.toolCalls, info)
	return nil
}
func (m *concurrentMockWriter) WriteToolResult(_ *facade.ToolResultInfo) error { return nil }
func (m *concurrentMockWriter) WriteError(_, _ string) error                   { return nil }
func (m *concurrentMockWriter) WriteUploadReady(_ *facade.UploadReadyInfo) error {
	return nil
}
func (m *concurrentMockWriter) WriteUploadComplete(_ *facade.UploadCompleteInfo) error {
	return nil
}
func (m *concurrentMockWriter) WriteMediaChunk(_ *facade.MediaChunkInfo) error { return nil }
func (m *concurrentMockWriter) SupportsBinary() bool                           { return false }
func (m *concurrentMockWriter) WriteBinaryMediaChunk(_ [facade.MediaIDSize]byte, _ uint32, _ bool, _ string, _ []byte) error {
	return nil
}

func (m *concurrentMockWriter) getToolCalls() []*facade.ToolCallInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*facade.ToolCallInfo, len(m.toolCalls))
	copy(out, m.toolCalls)
	return out
}

func (m *concurrentMockWriter) getDoneMsg() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.doneMsg
}

// clientToolServer simulates a runtime that sends a client-side tool call,
// waits for the client to respond, then sends the final response.
type clientToolServer struct {
	runtimev1.UnimplementedRuntimeServiceServer
}

func (s *clientToolServer) Converse(stream runtimev1.RuntimeService_ConverseServer) error {
	// Receive the user message
	if _, err := stream.Recv(); err != nil {
		return err
	}

	// Send a client-side tool call
	if err := stream.Send(&runtimev1.ServerMessage{
		Message: &runtimev1.ServerMessage_ToolCall{ToolCall: &runtimev1.ToolCall{
			Id:             "ct-integ-1",
			Name:           "get_user_location",
			ArgumentsJson:  `{"accuracy":"high"}`,
			Execution:      runtimev1.ToolExecution_TOOL_EXECUTION_CLIENT,
			ConsentMessage: "Allow location access?",
			Categories:     []string{"location"},
		}},
	}); err != nil {
		return err
	}

	// Wait for the client tool result
	msg, err := stream.Recv()
	if err != nil {
		return err
	}

	// Build response based on whether the tool was approved or rejected
	result := msg.GetClientToolResult()
	var finalContent string
	if result != nil && result.IsRejected {
		finalContent = "I can't determine your location without permission."
	} else {
		finalContent = "Based on your coordinates, you are in Denver, Colorado."
	}

	// Send text chunk + done
	if err := stream.Send(&runtimev1.ServerMessage{
		Message: &runtimev1.ServerMessage_Chunk{Chunk: &runtimev1.Chunk{Content: finalContent}},
	}); err != nil {
		return err
	}
	return stream.Send(&runtimev1.ServerMessage{
		Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{FinalContent: finalContent}},
	})
}

func (s *clientToolServer) Health(_ context.Context, _ *runtimev1.HealthRequest) (*runtimev1.HealthResponse, error) {
	return &runtimev1.HealthResponse{Healthy: true, Status: "ok"}, nil
}

// TestClientToolExecution tests the full client-side tool execution flow
// through the RuntimeHandler (facade ↔ runtime gRPC communication).
func TestClientToolExecution(t *testing.T) {
	// Start a mock runtime that sends client tool calls
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(grpcServer, &clientToolServer{})
	go func() { _ = grpcServer.Serve(lis) }()
	t.Cleanup(func() { grpcServer.GracefulStop() })

	// Create RuntimeClient + RuntimeHandler
	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     lis.Addr().String(),
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	handler := agent.NewRuntimeHandler(client)

	t.Run("approve", func(t *testing.T) {
		writer := &concurrentMockWriter{}
		sessionID := "ct-approve-1"

		errCh := make(chan error, 1)
		go func() {
			errCh <- handler.HandleMessage(context.Background(), sessionID, &facade.ClientMessage{
				Content: "Where am I?",
			}, writer)
		}()

		// Wait for the client tool call to be forwarded to the writer
		require.Eventually(t, func() bool {
			return len(writer.getToolCalls()) > 0
		}, 5*time.Second, 50*time.Millisecond, "tool call never arrived at writer")

		tc := writer.getToolCalls()[0]
		assert.Equal(t, "get_user_location", tc.Name)
		assert.Equal(t, "Allow location access?", tc.ConsentMessage)
		assert.Equal(t, []string{"location"}, tc.Categories)

		// Send the tool result back via the ClientToolRouter
		routed := handler.SendToolResult(sessionID, &facade.ClientToolResultInfo{
			CallID: tc.ID,
			Result: map[string]interface{}{"lat": 39.7392, "lng": -104.9903},
		})
		require.True(t, routed, "tool result should route to active handler")

		// Wait for completion
		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("HandleMessage did not complete")
		}

		assert.Equal(t, "Based on your coordinates, you are in Denver, Colorado.", writer.getDoneMsg())
	})

	t.Run("reject", func(t *testing.T) {
		writer := &concurrentMockWriter{}
		sessionID := "ct-reject-1"

		errCh := make(chan error, 1)
		go func() {
			errCh <- handler.HandleMessage(context.Background(), sessionID, &facade.ClientMessage{
				Content: "Where am I?",
			}, writer)
		}()

		require.Eventually(t, func() bool {
			return len(writer.getToolCalls()) > 0
		}, 5*time.Second, 50*time.Millisecond, "tool call never arrived at writer")

		tc := writer.getToolCalls()[0]

		routed := handler.SendToolResult(sessionID, &facade.ClientToolResultInfo{
			CallID: tc.ID,
			Error:  "User denied location access",
		})
		require.True(t, routed)

		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("HandleMessage did not complete")
		}

		assert.Equal(t, "I can't determine your location without permission.", writer.getDoneMsg())
	})

	t.Run("timeout", func(t *testing.T) {
		writer := &concurrentMockWriter{}
		sessionID := "ct-timeout-1"
		handler.SetClientToolTimeout(500 * time.Millisecond)
		defer handler.SetClientToolTimeout(60 * time.Second)

		errCh := make(chan error, 1)
		go func() {
			errCh <- handler.HandleMessage(context.Background(), sessionID, &facade.ClientMessage{
				Content: "Where am I?",
			}, writer)
		}()

		// Wait for the tool call to arrive, then ACK it so the handler
		// enters Phase 2 (clientToolTimeout) instead of auto-rejecting.
		require.Eventually(t, func() bool {
			return len(writer.getToolCalls()) > 0
		}, 5*time.Second, 50*time.Millisecond, "tool call never arrived at writer")

		tc := writer.getToolCalls()[0]
		handler.AckToolCall(sessionID, tc.ID)

		// Phase 2 should timeout after 500ms
		select {
		case err := <-errCh:
			require.Error(t, err)
			assert.Contains(t, err.Error(), "client tool timeout")
		case <-time.After(5 * time.Second):
			t.Fatal("HandleMessage did not complete after timeout")
		}
	})

	t.Run("no_active_handler", func(t *testing.T) {
		routed := handler.SendToolResult("nonexistent-session", &facade.ClientToolResultInfo{
			CallID: "fake",
			Result: "test",
		})
		assert.False(t, routed)
	})
}
