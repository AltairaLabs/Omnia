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

package agent

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/altairalabs/omnia/internal/facade"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// mockRuntimeServer implements RuntimeServiceServer for testing.
type mockRuntimeServer struct {
	runtimev1.UnimplementedRuntimeServiceServer
	responses []*runtimev1.ServerMessage
	healthy   bool
	status    string
}

func (s *mockRuntimeServer) Converse(stream runtimev1.RuntimeService_ConverseServer) error {
	// Receive the client message
	_, err := stream.Recv()
	if err != nil {
		return err
	}

	// Send back the configured responses
	for _, resp := range s.responses {
		if err := stream.Send(resp); err != nil {
			return err
		}
	}

	return nil
}

func (s *mockRuntimeServer) Health(_ context.Context, _ *runtimev1.HealthRequest) (*runtimev1.HealthResponse, error) {
	return &runtimev1.HealthResponse{
		Healthy: s.healthy,
		Status:  s.status,
	}, nil
}

func startMockServer(t *testing.T, mock *mockRuntimeServer) (string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(server, mock)

	go func() {
		_ = server.Serve(lis)
	}()

	cleanup := func() {
		server.Stop()
	}

	return lis.Addr().String(), cleanup
}

func TestRuntimeHandler_Name(t *testing.T) {
	handler := NewRuntimeHandler(nil)
	assert.Equal(t, "runtime", handler.Name())
}

func TestRuntimeHandler_HandleMessage_Chunks(t *testing.T) {
	mock := &mockRuntimeServer{
		responses: []*runtimev1.ServerMessage{
			{Message: &runtimev1.ServerMessage_Chunk{Chunk: &runtimev1.Chunk{Content: "Hello "}}},
			{Message: &runtimev1.ServerMessage_Chunk{Chunk: &runtimev1.Chunk{Content: "world"}}},
			{Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{FinalContent: "Hello world"}}},
		},
		healthy: true,
	}

	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	handler := NewRuntimeHandler(client)
	writer := &mockResponseWriter{}

	msg := &facade.ClientMessage{
		Content: "Hello",
	}

	err = handler.HandleMessage(context.Background(), "session-123", msg, writer)
	require.NoError(t, err)

	assert.Equal(t, []string{"Hello ", "world"}, writer.chunks)
	assert.Equal(t, "Hello world", writer.doneMsg)
}

func TestRuntimeHandler_HandleMessage_ToolCall(t *testing.T) {
	mock := &mockRuntimeServer{
		responses: []*runtimev1.ServerMessage{
			{Message: &runtimev1.ServerMessage_ToolCall{ToolCall: &runtimev1.ToolCall{
				Id:            "call-1",
				Name:          "weather",
				ArgumentsJson: `{"location": "Denver"}`,
			}}},
			{Message: &runtimev1.ServerMessage_ToolResult{ToolResult: &runtimev1.ToolResult{
				Id:         "call-1",
				ResultJson: `{"temp": 72}`,
				IsError:    false,
			}}},
			{Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{FinalContent: "It's 72°F"}}},
		},
		healthy: true,
	}

	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	handler := NewRuntimeHandler(client)
	writer := &mockResponseWriter{}

	msg := &facade.ClientMessage{
		Content: "What's the weather?",
	}

	err = handler.HandleMessage(context.Background(), "session-123", msg, writer)
	require.NoError(t, err)

	require.Len(t, writer.toolCalls, 1)
	assert.Equal(t, "call-1", writer.toolCalls[0].ID)
	assert.Equal(t, "weather", writer.toolCalls[0].Name)
	assert.Equal(t, "Denver", writer.toolCalls[0].Arguments["location"])

	require.Len(t, writer.toolResults, 1)
	assert.Equal(t, "call-1", writer.toolResults[0].ID)

	assert.Equal(t, "It's 72°F", writer.doneMsg)
}

func TestRuntimeHandler_HandleMessage_Error(t *testing.T) {
	mock := &mockRuntimeServer{
		responses: []*runtimev1.ServerMessage{
			{Message: &runtimev1.ServerMessage_Error{Error: &runtimev1.Error{
				Code:    "RATE_LIMITED",
				Message: "Too many requests",
			}}},
		},
		healthy: true,
	}

	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	handler := NewRuntimeHandler(client)
	writer := &mockResponseWriter{}

	msg := &facade.ClientMessage{
		Content: "Hello",
	}

	err = handler.HandleMessage(context.Background(), "session-123", msg, writer)
	require.NoError(t, err)

	require.Len(t, writer.errors, 1)
	assert.Equal(t, "RATE_LIMITED", writer.errors[0].code)
	assert.Equal(t, "Too many requests", writer.errors[0].message)
}

func TestRuntimeHandler_Client(t *testing.T) {
	mock := &mockRuntimeServer{healthy: true, status: "ok"}

	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	handler := NewRuntimeHandler(client)
	assert.Equal(t, client, handler.Client())

	// Test health through the exposed client
	resp, err := handler.Client().Health(context.Background())
	require.NoError(t, err)
	assert.True(t, resp.Healthy)
	assert.Equal(t, "ok", resp.Status)
}

func TestRuntimeClient_ConnectionFailure(t *testing.T) {
	_, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     "localhost:99999",
		DialTimeout: 100 * time.Millisecond,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to runtime")
}

func TestRuntimeClient_Health(t *testing.T) {
	mock := &mockRuntimeServer{
		healthy: true,
		status:  "ready",
	}

	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	resp, err := client.Health(context.Background())
	require.NoError(t, err)
	assert.True(t, resp.Healthy)
	assert.Equal(t, "ready", resp.Status)
}

func TestRuntimeHandler_HandleMessage_WithMetadata(t *testing.T) {
	mock := &mockRuntimeServer{
		responses: []*runtimev1.ServerMessage{
			{Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{FinalContent: "Done"}}},
		},
		healthy: true,
	}

	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	handler := NewRuntimeHandler(client)
	writer := &mockResponseWriter{}

	msg := &facade.ClientMessage{
		Content: "Hello",
		Metadata: map[string]string{
			"user_id":    "user-123",
			"request_id": "req-456",
		},
	}

	err = handler.HandleMessage(context.Background(), "session-123", msg, writer)
	require.NoError(t, err)
	assert.Equal(t, "Done", writer.doneMsg)
}

func TestRuntimeHandler_HandleMessage_ToolCallInvalidJSON(t *testing.T) {
	mock := &mockRuntimeServer{
		responses: []*runtimev1.ServerMessage{
			{Message: &runtimev1.ServerMessage_ToolCall{ToolCall: &runtimev1.ToolCall{
				Id:            "call-1",
				Name:          "test_tool",
				ArgumentsJson: "invalid json {",
			}}},
			{Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{FinalContent: "Done"}}},
		},
		healthy: true,
	}

	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	handler := NewRuntimeHandler(client)
	writer := &mockResponseWriter{}

	msg := &facade.ClientMessage{Content: "Hello"}

	err = handler.HandleMessage(context.Background(), "session-123", msg, writer)
	require.NoError(t, err)

	require.Len(t, writer.toolCalls, 1)
	assert.Equal(t, "call-1", writer.toolCalls[0].ID)
	assert.Equal(t, "test_tool", writer.toolCalls[0].Name)
	// Invalid JSON should fallback to raw
	assert.Equal(t, "invalid json {", writer.toolCalls[0].Arguments["raw"])
}

func TestRuntimeHandler_HandleMessage_ToolCallEmptyArgs(t *testing.T) {
	mock := &mockRuntimeServer{
		responses: []*runtimev1.ServerMessage{
			{Message: &runtimev1.ServerMessage_ToolCall{ToolCall: &runtimev1.ToolCall{
				Id:            "call-1",
				Name:          "no_args_tool",
				ArgumentsJson: "",
			}}},
			{Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{FinalContent: "Done"}}},
		},
		healthy: true,
	}

	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	handler := NewRuntimeHandler(client)
	writer := &mockResponseWriter{}

	msg := &facade.ClientMessage{Content: "Hello"}

	err = handler.HandleMessage(context.Background(), "session-123", msg, writer)
	require.NoError(t, err)

	require.Len(t, writer.toolCalls, 1)
	assert.Nil(t, writer.toolCalls[0].Arguments)
}

func TestRuntimeHandler_HandleMessage_ToolResultInvalidJSON(t *testing.T) {
	mock := &mockRuntimeServer{
		responses: []*runtimev1.ServerMessage{
			{Message: &runtimev1.ServerMessage_ToolResult{ToolResult: &runtimev1.ToolResult{
				Id:         "call-1",
				ResultJson: "not valid json",
				IsError:    false,
			}}},
			{Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{FinalContent: "Done"}}},
		},
		healthy: true,
	}

	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	handler := NewRuntimeHandler(client)
	writer := &mockResponseWriter{}

	msg := &facade.ClientMessage{Content: "Hello"}

	err = handler.HandleMessage(context.Background(), "session-123", msg, writer)
	require.NoError(t, err)

	require.Len(t, writer.toolResults, 1)
	assert.Equal(t, "call-1", writer.toolResults[0].ID)
	// Invalid JSON should use raw string as result
	assert.Equal(t, "not valid json", writer.toolResults[0].Result)
}

func TestRuntimeHandler_HandleMessage_ToolResultEmptyJSON(t *testing.T) {
	mock := &mockRuntimeServer{
		responses: []*runtimev1.ServerMessage{
			{Message: &runtimev1.ServerMessage_ToolResult{ToolResult: &runtimev1.ToolResult{
				Id:         "call-1",
				ResultJson: "",
				IsError:    false,
			}}},
			{Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{FinalContent: "Done"}}},
		},
		healthy: true,
	}

	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	handler := NewRuntimeHandler(client)
	writer := &mockResponseWriter{}

	msg := &facade.ClientMessage{Content: "Hello"}

	err = handler.HandleMessage(context.Background(), "session-123", msg, writer)
	require.NoError(t, err)

	require.Len(t, writer.toolResults, 1)
	assert.Equal(t, "call-1", writer.toolResults[0].ID)
	assert.Nil(t, writer.toolResults[0].Result)
}

func TestRuntimeHandler_HandleMessage_ToolResultIsError(t *testing.T) {
	mock := &mockRuntimeServer{
		responses: []*runtimev1.ServerMessage{
			{Message: &runtimev1.ServerMessage_ToolResult{ToolResult: &runtimev1.ToolResult{
				Id:         "call-1",
				ResultJson: `"Something went wrong"`,
				IsError:    true,
			}}},
			{Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{FinalContent: "Done"}}},
		},
		healthy: true,
	}

	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	handler := NewRuntimeHandler(client)
	writer := &mockResponseWriter{}

	msg := &facade.ClientMessage{Content: "Hello"}

	err = handler.HandleMessage(context.Background(), "session-123", msg, writer)
	require.NoError(t, err)

	require.Len(t, writer.toolResults, 1)
	assert.Equal(t, "call-1", writer.toolResults[0].ID)
	assert.Nil(t, writer.toolResults[0].Result)
	assert.Equal(t, "Something went wrong", writer.toolResults[0].Error)
}

func TestRuntimeHandler_HandleMessage_MultimodalResponse(t *testing.T) {
	mock := &mockRuntimeServer{
		responses: []*runtimev1.ServerMessage{
			{Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{
				FinalContent: "Here is an image",
				Parts: []*runtimev1.ContentPart{
					{Type: "text", Text: "Here is an image:"},
					{Type: "image", Media: &runtimev1.MediaContent{
						Data:     "base64encodeddata",
						MimeType: "image/png",
					}},
				},
			}}},
		},
		healthy: true,
	}

	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	handler := NewRuntimeHandler(client)
	writer := &mockResponseWriter{}

	msg := &facade.ClientMessage{Content: "Show me an image"}

	err = handler.HandleMessage(context.Background(), "session-123", msg, writer)
	require.NoError(t, err)

	// Should use WriteDoneWithParts instead of WriteDone
	assert.Empty(t, writer.doneMsg, "text-only WriteDone should not be called")
	require.Len(t, writer.doneParts, 2, "expected 2 content parts")

	// Verify text part
	assert.Equal(t, facade.ContentPartTypeText, writer.doneParts[0].Type)
	assert.Equal(t, "Here is an image:", writer.doneParts[0].Text)

	// Verify image part
	assert.Equal(t, facade.ContentPartTypeImage, writer.doneParts[1].Type)
	require.NotNil(t, writer.doneParts[1].Media)
	assert.Equal(t, "base64encodeddata", writer.doneParts[1].Media.Data)
	assert.Equal(t, "image/png", writer.doneParts[1].Media.MimeType)
}

func TestRuntimeHandler_HandleMessage_MultimodalInput(t *testing.T) {
	var receivedParts []*runtimev1.ContentPart

	mock := &mockRuntimeServer{
		responses: []*runtimev1.ServerMessage{
			{Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{FinalContent: "I see the image"}}},
		},
		healthy: true,
	}

	// Override Converse to capture the received message
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	capturingServer := &capturingRuntimeServer{
		mockRuntimeServer: mock,
		onReceive: func(msg *runtimev1.ClientMessage) {
			receivedParts = msg.Parts
		},
	}
	runtimev1.RegisterRuntimeServiceServer(server, capturingServer)

	go func() {
		_ = server.Serve(lis)
	}()
	defer server.Stop()

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     lis.Addr().String(),
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	handler := NewRuntimeHandler(client)
	writer := &mockResponseWriter{}

	// Send multimodal message
	msg := &facade.ClientMessage{
		Content: "What's in this image?",
		Parts: []facade.ContentPart{
			{Type: facade.ContentPartTypeText, Text: "What's in this image?"},
			{Type: facade.ContentPartTypeImage, Media: &facade.MediaContent{
				Data:     "testbase64data",
				MimeType: "image/jpeg",
			}},
		},
	}

	err = handler.HandleMessage(context.Background(), "session-123", msg, writer)
	require.NoError(t, err)

	// Verify the runtime received the multimodal parts
	require.Len(t, receivedParts, 2, "expected 2 parts to be sent to runtime")

	assert.Equal(t, "text", receivedParts[0].Type)
	assert.Equal(t, "What's in this image?", receivedParts[0].Text)

	assert.Equal(t, "image", receivedParts[1].Type)
	require.NotNil(t, receivedParts[1].Media)
	assert.Equal(t, "testbase64data", receivedParts[1].Media.Data)
	assert.Equal(t, "image/jpeg", receivedParts[1].Media.MimeType)
}

// capturingRuntimeServer wraps mockRuntimeServer to capture received messages.
type capturingRuntimeServer struct {
	*mockRuntimeServer
	onReceive func(*runtimev1.ClientMessage)
}

func (s *capturingRuntimeServer) Converse(stream runtimev1.RuntimeService_ConverseServer) error {
	msg, err := stream.Recv()
	if err != nil {
		return err
	}
	if s.onReceive != nil {
		s.onReceive(msg)
	}

	for _, resp := range s.responses {
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
	return nil
}
