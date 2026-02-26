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

package facade

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// mockRuntimeServer implements RuntimeServiceServer for testing.
type mockRuntimeServer struct {
	runtimev1.UnimplementedRuntimeServiceServer
	healthy bool
	status  string
}

func (s *mockRuntimeServer) Health(_ context.Context, _ *runtimev1.HealthRequest) (*runtimev1.HealthResponse, error) {
	return &runtimev1.HealthResponse{
		Healthy: s.healthy,
		Status:  s.status,
	}, nil
}

func (s *mockRuntimeServer) Converse(stream runtimev1.RuntimeService_ConverseServer) error {
	// Simple echo for testing
	msg, err := stream.Recv()
	if err != nil {
		return err
	}
	return stream.Send(&runtimev1.ServerMessage{
		Message: &runtimev1.ServerMessage_Done{
			Done: &runtimev1.Done{FinalContent: msg.Content},
		},
	})
}

func startTestServer(t *testing.T, mock *mockRuntimeServer) (string, func()) {
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

func TestNewRuntimeClient_Success(t *testing.T) {
	mock := &mockRuntimeServer{healthy: true, status: "ready"}
	addr, cleanup := startTestServer(t, mock)
	defer cleanup()

	client, err := NewRuntimeClient(RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	assert.NotNil(t, client)
	assert.Equal(t, addr, client.Address())
}

func TestNewRuntimeClient_ConnectionFailure(t *testing.T) {
	// Try to connect to a non-existent server
	_, err := NewRuntimeClient(RuntimeClientConfig{
		Address:     "localhost:59999",
		DialTimeout: 100 * time.Millisecond,
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to runtime")
}

func TestRuntimeClient_Health(t *testing.T) {
	mock := &mockRuntimeServer{healthy: true, status: "all systems go"}
	addr, cleanup := startTestServer(t, mock)
	defer cleanup()

	client, err := NewRuntimeClient(RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	resp, err := client.Health(context.Background())
	require.NoError(t, err)

	assert.True(t, resp.Healthy)
	assert.Equal(t, "all systems go", resp.Status)
}

func TestRuntimeClient_Converse(t *testing.T) {
	mock := &mockRuntimeServer{healthy: true}
	addr, cleanup := startTestServer(t, mock)
	defer cleanup()

	client, err := NewRuntimeClient(RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	stream, err := client.Converse(context.Background())
	require.NoError(t, err)

	// Send a message
	err = stream.Send(&runtimev1.ClientMessage{
		SessionId: "test-session",
		Content:   "Hello",
	})
	require.NoError(t, err)

	err = stream.CloseSend()
	require.NoError(t, err)

	// Receive response
	resp, err := stream.Recv()
	require.NoError(t, err)

	done := resp.GetDone()
	require.NotNil(t, done)
	assert.Equal(t, "Hello", done.FinalContent)
}

func TestRuntimeClient_Close(t *testing.T) {
	mock := &mockRuntimeServer{healthy: true}
	addr, cleanup := startTestServer(t, mock)
	defer cleanup()

	client, err := NewRuntimeClient(RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)

	// Close should succeed
	err = client.Close()
	assert.NoError(t, err)

	// Health check should fail after close
	_, err = client.Health(context.Background())
	assert.Error(t, err)
}

func TestRuntimeClient_Address(t *testing.T) {
	mock := &mockRuntimeServer{healthy: true}
	addr, cleanup := startTestServer(t, mock)
	defer cleanup()

	client, err := NewRuntimeClient(RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	assert.Equal(t, addr, client.Address())
}
