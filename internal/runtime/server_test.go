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

package runtime

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// mockProvider implements Provider for testing.
type mockProvider struct {
	responses []StreamEvent
	err       error
}

func (m *mockProvider) Chat(_ context.Context, _ []Message, ch chan<- StreamEvent) error {
	for _, resp := range m.responses {
		ch <- resp
	}
	return m.err
}

// mockSessionStore implements SessionStore for testing.
type mockSessionStore struct {
	history  []Message
	messages []Message
}

func (m *mockSessionStore) GetHistory(_ context.Context, _ string) ([]Message, error) {
	return m.history, nil
}

func (m *mockSessionStore) AppendMessage(_ context.Context, _ string, msg Message) error {
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockSessionStore) CreateSession(_ context.Context, _, _, _ string) error {
	return nil
}

// mockPackLoader implements PackLoader for testing.
type mockPackLoader struct {
	systemPrompt string
	err          error
}

func (m *mockPackLoader) LoadSystemPrompt() (string, error) {
	return m.systemPrompt, m.err
}

func TestNewServer(t *testing.T) {
	log := logr.Discard()
	provider := &mockProvider{}
	sessions := &mockSessionStore{}
	pack := &mockPackLoader{}

	server := NewServer(
		WithLogger(log),
		WithProvider(provider),
		WithSessionStore(sessions),
		WithPackLoader(pack),
		WithAgentInfo("test-agent", "default"),
	)

	assert.NotNil(t, server)
}

func TestServer_Health(t *testing.T) {
	server := NewServer()

	// Initially healthy
	resp, err := server.Health(context.Background(), &runtimev1.HealthRequest{})
	require.NoError(t, err)
	assert.True(t, resp.Healthy)
	assert.Equal(t, "ready", resp.Status)

	// Set unhealthy
	server.SetHealthy(false)
	resp, err = server.Health(context.Background(), &runtimev1.HealthRequest{})
	require.NoError(t, err)
	assert.False(t, resp.Healthy)
	assert.Equal(t, "not ready", resp.Status)

	// Set healthy again
	server.SetHealthy(true)
	resp, err = server.Health(context.Background(), &runtimev1.HealthRequest{})
	require.NoError(t, err)
	assert.True(t, resp.Healthy)
}

func TestServer_Converse_WithChunks(t *testing.T) {
	provider := &mockProvider{
		responses: []StreamEvent{
			{Type: EventChunk, Content: "Hello "},
			{Type: EventChunk, Content: "world!"},
			{Type: EventDone, Usage: &Usage{InputTokens: 10, OutputTokens: 5}},
		},
	}
	sessions := &mockSessionStore{}
	pack := &mockPackLoader{systemPrompt: "You are a helpful assistant."}

	server := NewServer(
		WithLogger(logr.Discard()),
		WithProvider(provider),
		WithSessionStore(sessions),
		WithPackLoader(pack),
		WithAgentInfo("test-agent", "default"),
	)

	// Start gRPC server
	grpcServer := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(grpcServer, server)

	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	go func() {
		_ = grpcServer.Serve(listener)
	}()
	defer grpcServer.Stop()

	// Create client
	conn, err := grpc.NewClient(listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	client := runtimev1.NewRuntimeServiceClient(conn)
	stream, err := client.Converse(context.Background())
	require.NoError(t, err)

	// Send message
	err = stream.Send(&runtimev1.ClientMessage{
		SessionId: "test-session",
		Content:   "Hi there",
	})
	require.NoError(t, err)
	err = stream.CloseSend()
	require.NoError(t, err)

	// Receive responses
	var chunks []string
	var doneMsg *runtimev1.Done

	for {
		msg, err := stream.Recv()
		if err != nil {
			break
		}

		switch m := msg.Message.(type) {
		case *runtimev1.ServerMessage_Chunk:
			chunks = append(chunks, m.Chunk.Content)
		case *runtimev1.ServerMessage_Done:
			doneMsg = m.Done
		}
	}

	assert.Equal(t, []string{"Hello ", "world!"}, chunks)
	require.NotNil(t, doneMsg)
	assert.NotNil(t, doneMsg.Usage)
	assert.Equal(t, int32(10), doneMsg.Usage.InputTokens)
	assert.Equal(t, int32(5), doneMsg.Usage.OutputTokens)

	// Verify session was updated
	assert.Len(t, sessions.messages, 2) // user + assistant
	assert.Equal(t, "user", sessions.messages[0].Role)
	assert.Equal(t, "Hi there", sessions.messages[0].Content)
	assert.Equal(t, "assistant", sessions.messages[1].Role)
	assert.Equal(t, "Hello world!", sessions.messages[1].Content)
}

func TestServer_Converse_WithToolCall(t *testing.T) {
	provider := &mockProvider{
		responses: []StreamEvent{
			{Type: EventToolCall, ToolCall: &ToolCall{
				ID:        "call-1",
				Name:      "weather",
				Arguments: `{"location": "Denver"}`,
			}},
			{Type: EventToolResult, ToolResult: &ToolResult{
				ID:     "call-1",
				Result: `{"temp": 72}`,
			}},
			{Type: EventChunk, Content: "It's 72°F"},
			{Type: EventDone},
		},
	}
	sessions := &mockSessionStore{}

	server := NewServer(
		WithLogger(logr.Discard()),
		WithProvider(provider),
		WithSessionStore(sessions),
		WithAgentInfo("test-agent", "default"),
	)

	// Start gRPC server
	grpcServer := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(grpcServer, server)

	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	go func() {
		_ = grpcServer.Serve(listener)
	}()
	defer grpcServer.Stop()

	// Create client
	conn, err := grpc.NewClient(listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	client := runtimev1.NewRuntimeServiceClient(conn)
	stream, err := client.Converse(context.Background())
	require.NoError(t, err)

	// Send message
	err = stream.Send(&runtimev1.ClientMessage{
		SessionId: "test-session",
		Content:   "What's the weather?",
	})
	require.NoError(t, err)
	err = stream.CloseSend()
	require.NoError(t, err)

	// Receive responses
	var toolCalls []*runtimev1.ToolCall
	var toolResults []*runtimev1.ToolResult

	for {
		msg, err := stream.Recv()
		if err != nil {
			break
		}

		switch m := msg.Message.(type) {
		case *runtimev1.ServerMessage_ToolCall:
			toolCalls = append(toolCalls, m.ToolCall)
		case *runtimev1.ServerMessage_ToolResult:
			toolResults = append(toolResults, m.ToolResult)
		}
	}

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "call-1", toolCalls[0].Id)
	assert.Equal(t, "weather", toolCalls[0].Name)

	require.Len(t, toolResults, 1)
	assert.Equal(t, "call-1", toolResults[0].Id)
}

func TestServer_WaitForReady(t *testing.T) {
	server := NewServer()
	server.SetHealthy(true)

	ctx := context.Background()
	err := server.WaitForReady(ctx, 100*time.Millisecond)
	assert.NoError(t, err)
}

func TestServer_WaitForReady_Timeout(t *testing.T) {
	server := NewServer()
	server.SetHealthy(false)

	ctx := context.Background()
	err := server.WaitForReady(ctx, 50*time.Millisecond)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not ready")
}
