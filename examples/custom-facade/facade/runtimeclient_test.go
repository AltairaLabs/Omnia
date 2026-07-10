/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package facade_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/altairalabs/omnia/examples/custom-facade/facade"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// converseHandler drives a scriptedRuntime's reply for a single test.
type converseHandler func(stream runtimev1.RuntimeService_ConverseServer) error

// scriptedRuntime is a stub RuntimeService whose Converse behaviour is driven by
// a per-test handler, so the client's chunk-accumulation, runtime-error and
// stream-error branches can each be exercised.
type scriptedRuntime struct {
	runtimev1.UnimplementedRuntimeServiceServer
	handler converseHandler
}

func (s *scriptedRuntime) Converse(stream runtimev1.RuntimeService_ConverseServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	return s.handler(stream)
}

func startScriptedRuntime(t *testing.T, handler converseHandler) (string, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(srv, &scriptedRuntime{handler: handler})
	go func() { _ = srv.Serve(lis) }()
	return lis.Addr().String(), srv.Stop
}

func dialScripted(t *testing.T, addr string) *facade.RuntimeClient {
	t.Helper()
	rc, err := facade.Dial(addr, "support-bot")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = rc.Close() })
	return rc
}

func converse(t *testing.T, rc *facade.RuntimeClient) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return rc.Converse(ctx, &facade.Principal{UserID: "u1"}, "sess-1", "hello")
}

// Converse must accumulate streamed text chunks in order and stop on Done.
func TestConverse_AccumulatesChunks(t *testing.T) {
	addr, stop := startScriptedRuntime(t, func(stream runtimev1.RuntimeService_ConverseServer) error {
		for _, part := range []string{"Hello, ", "world"} {
			if err := stream.Send(&runtimev1.ServerMessage{
				Message: &runtimev1.ServerMessage_Chunk{Chunk: &runtimev1.Chunk{Content: part}},
			}); err != nil {
				return err
			}
		}
		return stream.Send(&runtimev1.ServerMessage{
			Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{}},
		})
	})
	defer stop()

	reply, err := converse(t, dialScripted(t, addr))
	if err != nil {
		t.Fatalf("converse: %v", err)
	}
	if reply != "Hello, world" {
		t.Errorf("reply = %q, want %q", reply, "Hello, world")
	}
}

// A ServerMessage_Error must surface as an error carrying the runtime's message,
// alongside whatever text arrived before it.
func TestConverse_SurfacesRuntimeError(t *testing.T) {
	addr, stop := startScriptedRuntime(t, func(stream runtimev1.RuntimeService_ConverseServer) error {
		if err := stream.Send(&runtimev1.ServerMessage{
			Message: &runtimev1.ServerMessage_Chunk{Chunk: &runtimev1.Chunk{Content: "partial"}},
		}); err != nil {
			return err
		}
		return stream.Send(&runtimev1.ServerMessage{
			Message: &runtimev1.ServerMessage_Error{Error: &runtimev1.Error{Message: "boom"}},
		})
	})
	defer stop()

	reply, err := converse(t, dialScripted(t, addr))
	if err == nil {
		t.Fatalf("expected error, got reply %q", reply)
	}
	if reply != "partial" {
		t.Errorf("reply = %q, want partial-before-error", reply)
	}
}

// A stream that ends with a non-EOF gRPC status (the handler returning an error)
// must surface as a recv error, not a clean drain.
func TestConverse_StreamError(t *testing.T) {
	addr, stop := startScriptedRuntime(t, func(_ runtimev1.RuntimeService_ConverseServer) error {
		return errors.New("runtime exploded")
	})
	defer stop()

	if _, err := converse(t, dialScripted(t, addr)); err == nil {
		t.Fatal("expected recv error from an aborted stream")
	}
}

// Conversing after the runtime has gone away must surface a transport error
// rather than hang or return an empty reply.
func TestConverse_ServerDown(t *testing.T) {
	addr, stop := startScriptedRuntime(t, func(_ runtimev1.RuntimeService_ConverseServer) error { return nil })
	rc := dialScripted(t, addr)
	stop() // kill the runtime before conversing

	if _, err := converse(t, rc); err == nil {
		t.Fatal("expected a transport error conversing with a downed runtime")
	}
}
