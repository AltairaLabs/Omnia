/*
Copyright 2026.

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

package conformance

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/altairalabs/omnia/pkg/runtime/contract"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// dialFake registers srv on an in-process bufconn gRPC server and returns a
// client connection to it. The server and connection are cleaned up with t.
func dialFake(t *testing.T, srv runtimev1.RuntimeServiceServer) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(s, srv)
	go func() { _ = s.Serve(lis) }()
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(); s.Stop() })
	return conn
}

// runAgainst dials srv and runs the full suite against it.
func runAgainst(t *testing.T, srv runtimev1.RuntimeServiceServer) Result {
	t.Helper()
	return Run(context.Background(), Config{Conn: dialFake(t, srv)})
}

// findCheck returns the named check, failing the test if it is absent.
func findCheck(t *testing.T, res Result, name string) CheckResult {
	t.Helper()
	for _, c := range res.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("check %q not found in result %+v", name, res.Checks)
	return CheckResult{}
}

func helloFrame() *runtimev1.ServerMessage {
	return &runtimev1.ServerMessage{Message: &runtimev1.ServerMessage_RuntimeHello{
		RuntimeHello: &runtimev1.RuntimeHello{Capabilities: contract.KnownCapabilities()},
	}}
}

func doneFrame() *runtimev1.ServerMessage {
	return &runtimev1.ServerMessage{Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{}}}
}

// conformantFake is a minimal fully-conformant runtime: healthy, advertises the
// known capability set, sends RuntimeHello first (with a media counter-offer on
// the duplex path), ends text turns with done, and answers Invoke.
type conformantFake struct {
	runtimev1.UnimplementedRuntimeServiceServer
}

func (conformantFake) Health(context.Context, *runtimev1.HealthRequest) (*runtimev1.HealthResponse, error) {
	return &runtimev1.HealthResponse{
		Healthy:         true,
		ContractVersion: contract.Version,
		Capabilities:    contract.KnownCapabilities(),
	}, nil
}

func (conformantFake) Converse(stream runtimev1.RuntimeService_ConverseServer) error {
	msg, err := stream.Recv()
	if err != nil {
		return err
	}
	if msg.GetDuplexStart() != nil {
		return stream.Send(&runtimev1.ServerMessage{Message: &runtimev1.ServerMessage_RuntimeHello{
			RuntimeHello: &runtimev1.RuntimeHello{
				Capabilities: contract.KnownCapabilities(),
				Media:        &runtimev1.MediaNegotiation{Codec: "pcm", SampleRate: 16000, Channels: 1},
			},
		}})
	}
	if err := stream.Send(helloFrame()); err != nil {
		return err
	}
	return stream.Send(doneFrame())
}

func (conformantFake) Invoke(context.Context, *runtimev1.InvocationRequest) (*runtimev1.InvocationResponse, error) {
	return &runtimev1.InvocationResponse{OutputJson: "{}"}, nil
}

func TestRun_HealthContractCheck_Passes(t *testing.T) {
	res := runAgainst(t, conformantFake{})
	c := findCheck(t, res, "health/contract")
	require.Equal(t, StatusPass, c.Status, c.Detail)
}

// unhealthyFake reports healthy=false.
type unhealthyFake struct{ conformantFake }

func (unhealthyFake) Health(context.Context, *runtimev1.HealthRequest) (*runtimev1.HealthResponse, error) {
	return &runtimev1.HealthResponse{Healthy: false, ContractVersion: contract.Version}, nil
}

func TestRun_HealthUnhealthy_Fails(t *testing.T) {
	res := runAgainst(t, unhealthyFake{})
	c := findCheck(t, res, "health/contract")
	require.Equal(t, StatusFail, c.Status)
	require.False(t, res.Passed)
}

// badVersionFake advertises a non-semver contract version.
type badVersionFake struct{ conformantFake }

func (badVersionFake) Health(context.Context, *runtimev1.HealthRequest) (*runtimev1.HealthResponse, error) {
	return &runtimev1.HealthResponse{Healthy: true, ContractVersion: "v1", Capabilities: contract.KnownCapabilities()}, nil
}

func TestRun_BadVersion_Fails(t *testing.T) {
	res := runAgainst(t, badVersionFake{})
	c := findCheck(t, res, "health/contract")
	require.Equal(t, StatusFail, c.Status)
	require.Contains(t, c.Detail, "semver")
}

// errHealthFake returns a transport error from Health (nothing embedded, so the
// UnimplementedRuntimeServiceServer returns codes.Unimplemented).
type errHealthFake struct {
	runtimev1.UnimplementedRuntimeServiceServer
}

func TestRun_HealthTransportError_Fails(t *testing.T) {
	res := runAgainst(t, errHealthFake{})
	c := findCheck(t, res, "health/contract")
	require.Equal(t, StatusFail, c.Status)
	require.Contains(t, c.Detail, "Health RPC failed")
}

// ── Converse checks (Task 2) ──────────────────────────────────────────────

func TestRun_HelloFirst_Passes(t *testing.T) {
	res := runAgainst(t, conformantFake{})
	require.Equal(t, StatusPass, findCheck(t, res, "hello-first").Status)
	require.Equal(t, StatusPass, findCheck(t, res, "text-turn-shape").Status)
	require.Equal(t, StatusPass, findCheck(t, res, "graceful-malformed-input").Status)
}

// noHelloFake sends a done with no preceding RuntimeHello.
type noHelloFake struct{ conformantFake }

func (noHelloFake) Converse(stream runtimev1.RuntimeService_ConverseServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	return stream.Send(doneFrame())
}

func TestRun_HelloFirst_FailsWhenNoHello(t *testing.T) {
	res := runAgainst(t, noHelloFake{})
	c := findCheck(t, res, "hello-first")
	require.Equal(t, StatusFail, c.Status)
	require.False(t, res.Passed)
}

// doneBeforeHelloFake emits done before the hello.
type doneBeforeHelloFake struct{ conformantFake }

func (doneBeforeHelloFake) Converse(stream runtimev1.RuntimeService_ConverseServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	if err := stream.Send(doneFrame()); err != nil {
		return err
	}
	return stream.Send(helloFrame())
}

func TestRun_TextTurn_FailsWhenDoneBeforeHello(t *testing.T) {
	res := runAgainst(t, doneBeforeHelloFake{})
	c := findCheck(t, res, "text-turn-shape")
	require.Equal(t, StatusFail, c.Status)
	require.Contains(t, c.Detail, "before")
}

// crashConverseFake tears the stream down with an internal error.
type crashConverseFake struct{ conformantFake }

func (crashConverseFake) Converse(stream runtimev1.RuntimeService_ConverseServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	return status.Error(codes.Internal, "boom")
}

func TestRun_MalformedInput_FailsOnCrash(t *testing.T) {
	res := runAgainst(t, crashConverseFake{})
	c := findCheck(t, res, "graceful-malformed-input")
	require.Equal(t, StatusFail, c.Status)
	require.False(t, res.Passed)
}

// legacyFake is healthy but advertises no capabilities and sends no hello.
type legacyFake struct{ conformantFake }

func (legacyFake) Health(context.Context, *runtimev1.HealthRequest) (*runtimev1.HealthResponse, error) {
	return &runtimev1.HealthResponse{Healthy: true, ContractVersion: contract.Version}, nil
}

func (legacyFake) Converse(stream runtimev1.RuntimeService_ConverseServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	return stream.Send(doneFrame())
}

func TestRun_LegacyRuntime_SkipsHelloChecks(t *testing.T) {
	res := runAgainst(t, legacyFake{})
	require.Equal(t, StatusSkip, findCheck(t, res, "hello-first").Status)
	require.Equal(t, StatusSkip, findCheck(t, res, "text-turn-shape").Status)
	require.True(t, res.Passed, "a legacy runtime must not fail the run")
}
