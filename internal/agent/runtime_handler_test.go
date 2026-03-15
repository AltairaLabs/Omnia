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
	// Server-side tool calls (default execution) should be silently filtered.
	mock := &mockRuntimeServer{
		responses: []*runtimev1.ServerMessage{
			{Message: &runtimev1.ServerMessage_ToolCall{ToolCall: &runtimev1.ToolCall{
				Id:            "call-1",
				Name:          "weather",
				ArgumentsJson: `{"location": "Denver"}`,
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

	// Server-side tool calls are filtered — not forwarded to WebSocket
	assert.Empty(t, writer.toolCalls)
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
	// Server-side tool call with invalid JSON args — filtered, not forwarded.
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

	err = handler.HandleMessage(context.Background(), "session-123", &facade.ClientMessage{Content: "Hello"}, writer)
	require.NoError(t, err)

	// Server-side tool calls are filtered
	assert.Empty(t, writer.toolCalls)
	assert.Equal(t, "Done", writer.doneMsg)
}

func TestRuntimeHandler_HandleMessage_ToolCallEmptyArgs(t *testing.T) {
	// Server-side tool call with empty args — filtered, not forwarded.
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

	err = handler.HandleMessage(context.Background(), "session-123", &facade.ClientMessage{Content: "Hello"}, writer)
	require.NoError(t, err)

	// Server-side tool calls are filtered
	assert.Empty(t, writer.toolCalls)
	assert.Equal(t, "Done", writer.doneMsg)
}

func TestRuntimeHandler_HandleMessage_ToolCallServerSideFiltered(t *testing.T) {
	// Server-side tool calls should be filtered out — they are an internal
	// runtime concern and must not be forwarded to the WebSocket client.
	mock := &mockRuntimeServer{
		responses: []*runtimev1.ServerMessage{
			{Message: &runtimev1.ServerMessage_ToolCall{ToolCall: &runtimev1.ToolCall{
				Id:            "st-1",
				Name:          "weather",
				ArgumentsJson: `{"city":"Denver"}`,
				Execution:     runtimev1.ToolExecution_TOOL_EXECUTION_SERVER,
			}}},
			{Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{FinalContent: "72°F"}}},
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

	err = handler.HandleMessage(context.Background(), "session-123", &facade.ClientMessage{
		Content: "Weather?",
	}, writer)
	require.NoError(t, err)

	// Server-side tool calls must not be forwarded to the client
	assert.Empty(t, writer.toolCalls)
	assert.Equal(t, "72°F", writer.doneMsg)
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

func TestRuntimeHandler_HandleMessage_MediaChunk(t *testing.T) {
	rawData := []byte("fake image data for testing")

	mock := &mockRuntimeServer{
		responses: []*runtimev1.ServerMessage{
			{Message: &runtimev1.ServerMessage_Chunk{Chunk: &runtimev1.Chunk{Content: "Here is an image:"}}},
			{Message: &runtimev1.ServerMessage_MediaChunk{MediaChunk: &runtimev1.MediaChunk{
				MediaId:  "media-1",
				Sequence: 0,
				IsLast:   true,
				MimeType: "image/png",
				Data:     rawData,
			}}},
			{Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{FinalContent: "Here is an image:"}}},
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

	// Verify text chunks were forwarded
	assert.Equal(t, []string{"Here is an image:"}, writer.chunks)

	// Verify media chunk was forwarded with base64-encoded data
	require.Len(t, writer.mediaChunks, 1)
	mc := writer.mediaChunks[0]
	assert.Equal(t, "media-1", mc.MediaID)
	assert.Equal(t, 0, mc.Sequence)
	assert.True(t, mc.IsLast)
	assert.Equal(t, "image/png", mc.MimeType)
	assert.NotEmpty(t, mc.Data, "media chunk data should be base64-encoded")

	// Verify done was sent
	assert.Equal(t, "Here is an image:", writer.doneMsg)
}

// stallingRuntimeServer stalls after receiving the client message — never sends a response.
type stallingRuntimeServer struct {
	runtimev1.UnimplementedRuntimeServiceServer
}

func (s *stallingRuntimeServer) Converse(stream runtimev1.RuntimeService_ConverseServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	// Stall forever — simulates a hung LLM provider
	select {}
}

func (s *stallingRuntimeServer) Health(_ context.Context, _ *runtimev1.HealthRequest) (*runtimev1.HealthResponse, error) {
	return &runtimev1.HealthResponse{Healthy: true, Status: "ok"}, nil
}

func TestRuntimeHandler_HandleMessage_InactivityTimeout(t *testing.T) {
	stallMock := &stallingRuntimeServer{}
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	grpcServer := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(grpcServer, stallMock)
	go func() { _ = grpcServer.Serve(lis) }()
	t.Cleanup(func() { grpcServer.Stop() })

	// Connect with a generous timeout so the gRPC dial succeeds
	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     lis.Addr().String(),
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	handler := NewRuntimeHandler(client)
	writer := &mockResponseWriter{}

	// Use a context with a 2s deadline — the stream will stall and the context
	// will cancel before the 120s inactivity timeout. In production, the
	// inactivity timer fires first (120s << caller timeout).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = handler.HandleMessage(ctx, "session-1", &facade.ClientMessage{
		Type:    facade.MessageTypeMessage,
		Content: "hello",
	}, writer)

	require.Error(t, err, "expected error from stalled stream")
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

// clientToolRuntimeServer simulates a runtime that sends a client-side tool call,
// waits for the result, then sends the final response.
type clientToolRuntimeServer struct {
	runtimev1.UnimplementedRuntimeServiceServer
	toolCall       *runtimev1.ToolCall
	afterResume    []*runtimev1.ServerMessage
	receivedResult *runtimev1.ClientToolResult
}

func (s *clientToolRuntimeServer) Converse(stream runtimev1.RuntimeService_ConverseServer) error {
	// Receive initial message
	if _, err := stream.Recv(); err != nil {
		return err
	}

	// Send client tool call
	if err := stream.Send(&runtimev1.ServerMessage{
		Message: &runtimev1.ServerMessage_ToolCall{ToolCall: s.toolCall},
	}); err != nil {
		return err
	}

	// Wait for client tool result
	msg, err := stream.Recv()
	if err != nil {
		return err
	}
	s.receivedResult = msg.GetClientToolResult()

	// Send remaining responses (after resume)
	for _, resp := range s.afterResume {
		if err := stream.Send(resp); err != nil {
			return err
		}
	}

	return nil
}

func (s *clientToolRuntimeServer) Health(_ context.Context, _ *runtimev1.HealthRequest) (*runtimev1.HealthResponse, error) {
	return &runtimev1.HealthResponse{Healthy: true, Status: "ok"}, nil
}

func TestRuntimeHandler_ClientToolCall(t *testing.T) {
	mock := &clientToolRuntimeServer{
		toolCall: &runtimev1.ToolCall{
			Id:             "ct-1",
			Name:           "get_location",
			ArgumentsJson:  `{"query":"current"}`,
			Execution:      runtimev1.ToolExecution_TOOL_EXECUTION_CLIENT,
			ConsentMessage: "Allow location access?",
			Categories:     []string{"location"},
		},
		afterResume: []*runtimev1.ServerMessage{
			{Message: &runtimev1.ServerMessage_Chunk{Chunk: &runtimev1.Chunk{Content: "Your location is Denver"}}},
			{Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{FinalContent: "Your location is Denver"}}},
		},
	}

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(server, mock)
	go func() { _ = server.Serve(lis) }()
	t.Cleanup(func() { server.Stop() })

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     lis.Addr().String(),
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	handler := NewRuntimeHandler(client)
	toolCallCh := make(chan *facade.ToolCallInfo, 1)
	writer := &mockResponseWriter{toolCallCh: toolCallCh}
	sessionID := "session-ct-1"

	// Start HandleMessage in a goroutine — it will block waiting for the tool result
	errCh := make(chan error, 1)
	go func() {
		errCh <- handler.HandleMessage(context.Background(), sessionID, &facade.ClientMessage{
			Content: "Where am I?",
		}, writer)
	}()

	// Wait for the tool call to be forwarded to the writer
	var tc *facade.ToolCallInfo
	select {
	case tc = <-toolCallCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for tool call")
	}

	// Verify the tool call fields — all WebSocket tool calls are client-side
	assert.Equal(t, "ct-1", tc.ID)
	assert.Equal(t, "get_location", tc.Name)
	assert.Equal(t, "Allow location access?", tc.ConsentMessage)
	assert.Equal(t, []string{"location"}, tc.Categories)

	// Send tool result via the ClientToolRouter interface
	routed := handler.SendToolResult(sessionID, &facade.ClientToolResultInfo{
		CallID: "ct-1",
		Result: map[string]string{"city": "Denver"},
	})
	assert.True(t, routed)

	// Wait for HandleMessage to complete
	err = <-errCh
	require.NoError(t, err)

	// Verify the final response was forwarded
	assert.Equal(t, []string{"Your location is Denver"}, writer.chunks)
	assert.Equal(t, "Your location is Denver", writer.doneMsg)

	// Verify the runtime received the tool result
	require.NotNil(t, mock.receivedResult)
	assert.Equal(t, "ct-1", mock.receivedResult.CallId)
	assert.False(t, mock.receivedResult.IsRejected)
	assert.Contains(t, mock.receivedResult.ResultJson, "Denver")
}

func TestRuntimeHandler_ClientToolCall_Rejected(t *testing.T) {
	mock := &clientToolRuntimeServer{
		toolCall: &runtimev1.ToolCall{
			Id:        "ct-2",
			Name:      "read_file",
			Execution: runtimev1.ToolExecution_TOOL_EXECUTION_CLIENT,
		},
		afterResume: []*runtimev1.ServerMessage{
			{Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{FinalContent: "Tool was denied"}}},
		},
	}

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(server, mock)
	go func() { _ = server.Serve(lis) }()
	t.Cleanup(func() { server.Stop() })

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     lis.Addr().String(),
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	handler := NewRuntimeHandler(client)
	toolCallCh := make(chan *facade.ToolCallInfo, 1)
	writer := &mockResponseWriter{toolCallCh: toolCallCh}
	sessionID := "session-ct-2"

	errCh := make(chan error, 1)
	go func() {
		errCh <- handler.HandleMessage(context.Background(), sessionID, &facade.ClientMessage{
			Content: "Read my file",
		}, writer)
	}()

	// Wait for the tool call to arrive
	select {
	case <-toolCallCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for tool call")
	}

	// Reject the tool call
	routed := handler.SendToolResult(sessionID, &facade.ClientToolResultInfo{
		CallID: "ct-2",
		Error:  "User denied",
	})
	assert.True(t, routed)

	err = <-errCh
	require.NoError(t, err)

	// Verify rejection was sent to runtime
	require.NotNil(t, mock.receivedResult)
	assert.True(t, mock.receivedResult.IsRejected)
	assert.Equal(t, "User denied", mock.receivedResult.RejectionReason)
}

func TestRuntimeHandler_ClientToolCall_Timeout(t *testing.T) {
	mock := &clientToolRuntimeServer{
		toolCall: &runtimev1.ToolCall{
			Id:        "ct-3",
			Name:      "slow_tool",
			Execution: runtimev1.ToolExecution_TOOL_EXECUTION_CLIENT,
		},
		afterResume: []*runtimev1.ServerMessage{
			{Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{FinalContent: "Done"}}},
		},
	}

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(server, mock)
	go func() { _ = server.Serve(lis) }()
	t.Cleanup(func() { server.Stop() })

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     lis.Addr().String(),
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	handler := NewRuntimeHandler(client)
	handler.SetClientToolTimeout(200 * time.Millisecond)
	writer := &mockResponseWriter{}

	err = handler.HandleMessage(context.Background(), "session-ct-3", &facade.ClientMessage{
		Content: "Do something slow",
	}, writer)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "client tool timeout")
}

func TestRuntimeHandler_SendToolResult_NoActiveHandler(t *testing.T) {
	handler := NewRuntimeHandler(nil)
	// No active session — should return false
	routed := handler.SendToolResult("nonexistent", &facade.ClientToolResultInfo{
		CallID: "ct-x",
		Result: "test",
	})
	assert.False(t, routed)
}
