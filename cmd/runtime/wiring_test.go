/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package main

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"

	pkruntime "github.com/altairalabs/omnia/internal/runtime"
	"github.com/altairalabs/omnia/pkg/policy"
)

// probeServiceName and probeMethod are the gRPC service / method names the
// wiring test registers on the real runtime server. They do not collide with
// the real RuntimeService and exist only to give the wiring test a handler it
// can introspect.
const (
	probeServiceName = "omnia.runtime.test.Probe"
	probeMethodName  = "Echo"
	probeFullMethod  = "/" + probeServiceName + "/" + probeMethodName
)

// TestBuildGRPCServer_PolicyInterceptorWiresUserIDMetadata verifies the
// wiring contract that the runtime's gRPC server has the policy unary
// interceptor installed. It calls buildGRPCServer (the function main() uses
// to construct its server), registers a probe handler, starts the server on
// an in-process bufconn listener, and sends a unary request with the
// x-omnia-user-id metadata header. The handler asserts that
// policy.UserID(ctx) returns the propagated value.
//
// This catches the regression where PolicyUnaryServerInterceptor was defined
// and unit-tested but never registered on the runtime binary's gRPC server
// (issue #714 motivating example).
func TestBuildGRPCServer_PolicyInterceptorWiresUserIDMetadata(t *testing.T) {
	srv := buildGRPCServer(nil)
	defer srv.Stop()

	var (
		capturedUserID    string
		capturedGrantsRaw []string
	)
	// Standard gRPC MethodDesc.Handler: decode the request, then either invoke
	// the user handler directly (no interceptor) or run it through the chain.
	// Skipping the interceptor call bypasses the wiring we want to verify.
	handler := func(_ interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
		in := new(emptypb.Empty)
		if err := dec(in); err != nil {
			return nil, err
		}
		inner := func(ctx context.Context, _ interface{}) (interface{}, error) {
			capturedUserID = policy.UserID(ctx)
			capturedGrantsRaw = policy.ExtractPropagationFields(ctx).ConsentGrants
			return &emptypb.Empty{}, nil
		}
		if interceptor == nil {
			return inner(ctx, in)
		}
		return interceptor(ctx, in, &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: probeFullMethod,
		}, inner)
	}

	srv.RegisterService(&grpc.ServiceDesc{
		ServiceName: probeServiceName,
		HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: probeMethodName, Handler: handler},
		},
		Metadata: "probe.proto",
	}, struct{}{})

	lis := bufconn.Listen(1024 * 1024)
	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx,
		policy.HeaderUserID, "alice",
		policy.HeaderConsentGrants, "memory:location,memory:history",
	)

	if err := conn.Invoke(ctx, probeFullMethod, &emptypb.Empty{}, &emptypb.Empty{}); err != nil {
		t.Fatalf("invoke probe: %v", err)
	}

	if capturedUserID != "alice" {
		t.Errorf("policy.UserID(ctx) = %q, want \"alice\" — "+
			"PolicyUnaryServerInterceptor is not wired on the runtime gRPC server",
			capturedUserID)
	}
	if len(capturedGrantsRaw) != 2 ||
		capturedGrantsRaw[0] != "memory:location" ||
		capturedGrantsRaw[1] != "memory:history" {
		t.Errorf("consent grants not propagated: got %v, want [memory:location memory:history]",
			capturedGrantsRaw)
	}
}

// TestBuildGRPCServer_ReturnsNonNilServer is a minimal smoke test that the
// factory returns a usable server even without a tracing provider.
func TestBuildGRPCServer_ReturnsNonNilServer(t *testing.T) {
	srv := buildGRPCServer(nil)
	if srv == nil {
		t.Fatal("buildGRPCServer returned nil")
	}
	srv.Stop()
}

// TestConfigDerivedServerOpts_WiresMediaBasePath verifies that
// configDerivedServerOpts — the helper cmd/runtime/main.go uses to assemble
// its pkruntime.ServerOption slice — forwards cfg.MediaBasePath to
// pkruntime.WithMediaBasePath. Without this, the runtime server's
// mediaResolver is always nil and `mock://` and `file://` media URL
// resolution silently fails (item 2 of #728).
//
// The test applies the slice to a real pkruntime.NewServer call and asserts
// the resulting server reports a non-nil media resolver via
// HasMediaResolver(). A regression that removes WithMediaBasePath from the
// helper — or stops calling the helper from main.go — fails this test.
func TestConfigDerivedServerOpts_WiresMediaBasePath(t *testing.T) {
	t.Setenv("OMNIA_AGENT_NAME", "wiring-test")
	t.Setenv("OMNIA_NAMESPACE", "wiring")

	cfg := &pkruntime.Config{
		AgentName:     "wiring-test",
		Namespace:     "wiring",
		PromptName:    "default",
		MediaBasePath: t.TempDir(),
	}
	opts := configDerivedServerOpts(cfg)

	srv := pkruntime.NewServer(opts...)
	t.Cleanup(func() { _ = srv.Close() })

	if !srv.HasMediaResolver() {
		t.Error("configDerivedServerOpts does not wire MediaBasePath — " +
			"runtime server has nil mediaResolver, mock:// and file:// URL " +
			"resolution will silently fail")
	}
}

// TestConfigDerivedServerOpts_EmptyMediaBasePathLeavesResolverNil guards the
// inverse: when MediaBasePath is empty, WithMediaBasePath is a no-op and the
// server has no resolver. If a future change makes the option always set a
// resolver even for empty paths, we want to notice because it would change
// production behavior silently.
func TestConfigDerivedServerOpts_EmptyMediaBasePathLeavesResolverNil(t *testing.T) {
	cfg := &pkruntime.Config{AgentName: "wiring-test", Namespace: "wiring"}
	opts := configDerivedServerOpts(cfg)

	srv := pkruntime.NewServer(opts...)
	t.Cleanup(func() { _ = srv.Close() })

	if srv.HasMediaResolver() {
		t.Error("configDerivedServerOpts wires a media resolver even though " +
			"MediaBasePath is empty — WithMediaBasePath semantics changed")
	}
}
