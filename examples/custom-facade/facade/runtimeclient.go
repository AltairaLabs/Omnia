/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package facade

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// RuntimeClient speaks the runtime gRPC contract (RuntimeService) directly.
// The facade dials the runtime sidecar at OMNIA_RUNTIME_ADDRESS and, on each
// caller request, opens a Converse stream with the caller's identity attached
// as x-omnia-* metadata. This is the hop the runtime's policy interceptor
// reads to populate identity for tool execution + the policy broker.
type RuntimeClient struct {
	conn      *grpc.ClientConn
	client    runtimev1.RuntimeServiceClient
	agentName string
}

// Dial connects to the runtime gRPC endpoint (e.g. "localhost:9000"). The
// connection is plaintext because facade<->runtime is an intra-pod localhost
// hop; TLS is terminated at the pod boundary.
func Dial(address, agentName string) (*RuntimeClient, error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("custom-facade: dial runtime %q: %w", address, err)
	}
	return &RuntimeClient{
		conn:      conn,
		client:    runtimev1.NewRuntimeServiceClient(conn),
		agentName: agentName,
	}, nil
}

// Close releases the underlying gRPC connection.
func (c *RuntimeClient) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// Converse runs a single-turn conversation against the runtime on behalf of
// the authenticated principal. It attaches the principal's identity as
// outbound x-omnia-* gRPC metadata, sends one user message, and concatenates
// the streamed text chunks into the returned reply.
//
// This is the minimal happy-path shape: send a ClientMessage, drain the
// ServerMessage stream, stop on Done. A production facade would also relay
// tool calls (ServerMessage_ToolCall) back to its client and feed
// ClientToolResult in — the metadata/identity contract is identical.
func (c *RuntimeClient) Converse(ctx context.Context, p *Principal, sessionID, text string) (string, error) {
	ctx = metadata.NewOutgoingContext(ctx, metadata.New(p.OutboundMetadata(c.agentName)))

	stream, err := c.client.Converse(ctx)
	if err != nil {
		return "", fmt.Errorf("custom-facade: open Converse: %w", err)
	}

	if err := stream.Send(&runtimev1.ClientMessage{SessionId: sessionID, Content: text}); err != nil {
		return "", fmt.Errorf("custom-facade: send message: %w", err)
	}
	if err := stream.CloseSend(); err != nil {
		return "", fmt.Errorf("custom-facade: close send: %w", err)
	}

	return drainConverse(stream)
}

// drainConverse reads ServerMessages until Done / EOF, accumulating text
// chunks and surfacing the first Error the runtime reports.
func drainConverse(stream grpc.BidiStreamingClient[runtimev1.ClientMessage, runtimev1.ServerMessage]) (string, error) {
	var reply strings.Builder
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return reply.String(), nil
		}
		if err != nil {
			return reply.String(), fmt.Errorf("custom-facade: recv: %w", err)
		}
		switch m := msg.Message.(type) {
		case *runtimev1.ServerMessage_Chunk:
			reply.WriteString(m.Chunk.GetContent())
		case *runtimev1.ServerMessage_Error:
			return reply.String(), fmt.Errorf("custom-facade: runtime error: %s", m.Error.GetMessage())
		case *runtimev1.ServerMessage_Done:
			return reply.String(), nil
		}
	}
}
