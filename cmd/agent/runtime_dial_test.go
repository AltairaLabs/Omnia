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

package main

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"

	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// stubRuntimeServer implements just enough of RuntimeServiceServer to
// answer Health (which dialRuntime / RuntimeClient call as part of
// connection setup) and to echo back Invoke for the wiring test.
type stubRuntimeServer struct {
	runtimev1.UnimplementedRuntimeServiceServer
	healthErr error
}

func (s *stubRuntimeServer) Health(_ context.Context, _ *runtimev1.HealthRequest) (*runtimev1.HealthResponse, error) {
	if s.healthErr != nil {
		return nil, s.healthErr
	}
	return &runtimev1.HealthResponse{Healthy: true, Status: "ok"}, nil
}

func (s *stubRuntimeServer) Invoke(
	_ context.Context,
	req *runtimev1.InvocationRequest,
) (*runtimev1.InvocationResponse, error) {
	// Echo the input back as the output, with the invocation_id round-tripped.
	return &runtimev1.InvocationResponse{
		OutputJson:   req.GetInputJson(),
		InvocationId: req.GetInvocationId(),
		DurationMs:   1,
	}, nil
}

// startStubRuntimeOnTCP boots a real gRPC server on a free localhost
// port and returns "host:port" plus a stop func. Real TCP — not
// bufconn — because dialRuntime uses RuntimeClient which configures
// keepalive + interceptors that don't always co-operate with
// bufconn's in-process plumbing.
func startStubRuntimeOnTCP(t *testing.T, stub *stubRuntimeServer) (string, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(srv, stub)
	// gRPC Health Check Protocol — RuntimeClient also probes this
	// (some paths use the standard reflection-style health, others
	// use our typed Health RPC). Register both for safety.
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)
	healthgrpc.RegisterHealthServer(srv, healthServer)
	go func() {
		_ = srv.Serve(lis)
	}()
	return lis.Addr().String(), func() {
		srv.GracefulStop()
		_ = lis.Close()
	}
}

func TestDialRuntime_SucceedsAgainstRealStub(t *testing.T) {
	addr, stop := startStubRuntimeOnTCP(t, &stubRuntimeServer{})
	defer stop()

	cfg := newDialRuntimeConfig(addr, nil)
	client, err := dialRuntime(cfg, logr.Discard())
	if err != nil {
		t.Fatalf("dialRuntime: %v", err)
	}
	if client == nil {
		t.Fatalf("dialRuntime returned nil client")
	}
	_ = client.Close()
}

func TestDialRuntime_RetriesUntilCapAndFails(t *testing.T) {
	// Address that nothing is listening on. dialRuntime should attempt
	// maxRetries times, then return the last error.
	sleeps := 0
	cfg := dialRuntimeConfig{
		address:        "127.0.0.1:1", // closed port, fast-fail
		maxRetries:     3,
		initialBackoff: time.Millisecond,
		backoffCap:     2 * time.Millisecond,
		dialTimeout:    50 * time.Millisecond,
		sleep:          func(time.Duration) { sleeps++ },
	}
	_, err := dialRuntime(cfg, logr.Discard())
	if err == nil {
		t.Fatalf("expected error after exhausting retries")
	}
	// Sleeps run between attempts only, so maxRetries-1 = 2.
	if sleeps != 2 {
		t.Errorf("sleeps = %d, want %d", sleeps, 2)
	}
}

func TestDialRuntime_TestSleepHookIsInvokedBetweenAttempts(t *testing.T) {
	// dialRuntime sleeps only between attempts. With 5 attempts there
	// should be exactly 4 sleeps; the loop must not sleep AFTER the
	// final failure (no point) or BEFORE the first attempt (no reason).
	sleeps := 0
	cfg := dialRuntimeConfig{
		address:        "127.0.0.1:1",
		maxRetries:     5,
		initialBackoff: time.Millisecond,
		backoffCap:     2 * time.Millisecond,
		dialTimeout:    50 * time.Millisecond,
		sleep:          func(time.Duration) { sleeps++ },
	}
	_, _ = dialRuntime(cfg, logr.Discard())
	if sleeps != 4 {
		t.Errorf("sleeps = %d, want %d (one sleep between each pair of attempts)", sleeps, 4)
	}
}
