/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package promptkit

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
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

// #728 item 4 UPDATE: runtime ToolPolicy evaluator wiring is now live.
//
// The #728 backlog listed "Runtime doesn't check ToolPolicy before executing
// tools" as a wiring gap, on the basis that ToolPolicy enforcement lived only
// in the separate policy-proxy sidecar (retired in P2.4 — it never worked
// and is replaced by the policy-broker, ee/cmd/policy-broker/). That gap is
// closed as of the ToolPolicy-broker work (P2.1-P2.3,
// docs/local-backlog/2026-07-05-toolpolicy-enforcement-phase2-design.md):
// internal/runtime/tools.OmniaExecutor.dispatch (omnia_executor.go) now calls
// enforcePolicy on every tool call, which asks a PolicyBrokerClient
// (policy_broker_client.go) for a decision from the real
// ee/pkg/policy.BrokerHandler (P2.1) over POLICY_BROKER_URL — a real hook
// point for in-process (well, in-process-per-tool-call) policy evaluation
// does exist now.
//
// The wiring assertion for this lives in internal/runtime/tools rather than
// here: OmniaExecutor.policyBroker is unexported, and NewOmniaExecutor is
// constructed inside internal/runtime.Server.InitializeTools (server.go),
// not directly in cmd/runtime, so an in-package test is the least-brittle
// place to assert the client is always wired and enabled/disabled by
// POLICY_BROKER_URL. See:
//   - internal/runtime/tools/omnia_executor_policy_test.go — TestNewOmniaExecutor_WiresPolicyBrokerClient
//     (construction-time wiring) plus the TestDispatch_PolicyBroker* tests
//     (behavioral proof that dispatch enforces broker decisions).
//   - test/integration/policy_broker_test.go — TestPolicyBrokerEndToEnd_RealBrokerDeniesToolCall,
//     the end-to-end proof against a REAL ee/pkg/policy.BrokerHandler and
//     REAL CEL evaluation (not a mock broker).

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

// TestConfigDerivedServerOpts_WiresAgentUID asserts that cfg.AgentUID is
// applied on the resulting Server via WithAgentUID. Without this option the
// runtime's memory scope never carries agent_id, which silently disables the
// multi-tier retrieval routing in the httpclient — callers get only the flat
// user-scoped search endpoint.
func TestConfigDerivedServerOpts_WiresAgentUID(t *testing.T) {
	const wantUID = "44444444-4444-4444-4444-444444444444"
	cfg := &pkruntime.Config{
		AgentName: "wiring-test",
		AgentUID:  wantUID,
		Namespace: "wiring",
	}
	opts := configDerivedServerOpts(cfg)

	srv := pkruntime.NewServer(opts...)
	t.Cleanup(func() { _ = srv.Close() })

	if got := pkruntime.ServerAgentUID(srv); got != wantUID {
		t.Errorf("AgentUID not propagated: got %q, want %q", got, wantUID)
	}
}

// TestConfigDerivedServerOpts_WiresMemoryRetrieval asserts that
// cfg.MemoryStrategy, cfg.MemoryDenyCEL, and cfg.MemoryLimit are forwarded to
// the runtime server via WithMemoryRetrieval. Without this wiring, per-turn
// retrieval always falls back to keyword FTS, the access deny-filter is never
// applied, and the episodic limit is always the default (10), even when
// spec.memory.retrieval is configured on the AgentRuntime CRD.
//
// This is a true wiring assertion: configDerivedServerOpts is the function
// main() uses to build its option slice; applying it to a real pkruntime.Server
// and reading back via ServerMemoryRetrieval() confirms the option actually
// reaches the struct fields that conversation.go reads when building the
// CompositeRetriever.
func TestConfigDerivedServerOpts_WiresMemoryRetrieval(t *testing.T) {
	const (
		wantStrategy = "semantic"
		wantDenyCEL  = `metadata.url.contains("restricted")`
		wantLimit    = 25
	)
	cfg := &pkruntime.Config{
		AgentName:      "wiring-test",
		Namespace:      "wiring",
		MemoryStrategy: wantStrategy,
		MemoryDenyCEL:  wantDenyCEL,
		MemoryLimit:    wantLimit,
	}
	opts := configDerivedServerOpts(cfg)

	srv := pkruntime.NewServer(opts...)
	t.Cleanup(func() { _ = srv.Close() })

	gotStrategy, gotDenyCEL, gotLimit := pkruntime.ServerMemoryRetrieval(srv)
	if gotStrategy != wantStrategy {
		t.Errorf("MemoryStrategy not propagated: got %q, want %q — "+
			"WithMemoryRetrieval is missing from configDerivedServerOpts",
			gotStrategy, wantStrategy)
	}
	if gotDenyCEL != wantDenyCEL {
		t.Errorf("MemoryDenyCEL not propagated: got %q, want %q — "+
			"WithMemoryRetrieval is missing from configDerivedServerOpts",
			gotDenyCEL, wantDenyCEL)
	}
	if gotLimit != wantLimit {
		t.Errorf("MemoryLimit not propagated: got %d, want %d — "+
			"WithMemoryRetrieval limit arg is missing from configDerivedServerOpts",
			gotLimit, wantLimit)
	}
}

// TestConfigDerivedServerOpts_EmptyMemoryRetrievalIsHarmless guards that an
// empty MemoryStrategy, MemoryDenyCEL, and zero MemoryLimit (the defaults when
// the CRD fields are absent) propagate cleanly without panicking or setting
// unexpected values. The retriever defaults to keyword FTS and defaultEpisodicLimit
// when these fields are unset.
func TestConfigDerivedServerOpts_EmptyMemoryRetrievalIsHarmless(t *testing.T) {
	cfg := &pkruntime.Config{AgentName: "wiring-test", Namespace: "wiring"}
	opts := configDerivedServerOpts(cfg)

	srv := pkruntime.NewServer(opts...)
	t.Cleanup(func() { _ = srv.Close() })

	gotStrategy, gotDenyCEL, gotLimit := pkruntime.ServerMemoryRetrieval(srv)
	if gotStrategy != "" {
		t.Errorf("expected empty strategy for unconfigured memory, got %q", gotStrategy)
	}
	if gotDenyCEL != "" {
		t.Errorf("expected empty denyCEL for unconfigured memory, got %q", gotDenyCEL)
	}
	if gotLimit != 0 {
		t.Errorf("expected 0 limit for unconfigured memory, got %d", gotLimit)
	}
}

// TestConfigDerivedServerOpts_WiresOutputFormat is a wiring assertion that the
// function-mode response format reaches the server: setting it on Config,
// running configDerivedServerOpts, and reading back via ServerOutputFormat()
// confirms WithFunctionOutputFormat is actually in the option list (#1483).
func TestConfigDerivedServerOpts_WiresOutputFormat(t *testing.T) {
	cfg := &pkruntime.Config{
		AgentName:        "wiring-test",
		Namespace:        "wiring",
		Mode:             "function",
		OutputFormat:     "json_schema",
		OutputSchemaJSON: []byte(`{"type":"object"}`),
	}
	opts := configDerivedServerOpts(cfg)

	srv := pkruntime.NewServer(opts...)
	t.Cleanup(func() { _ = srv.Close() })

	gotMode, gotFormat := pkruntime.ServerOutputFormat(srv)
	if gotMode != "function" {
		t.Errorf("Mode not propagated: got %q, want %q — "+
			"WithFunctionOutputFormat is missing from configDerivedServerOpts", gotMode, "function")
	}
	if gotFormat != "json_schema" {
		t.Errorf("OutputFormat not propagated: got %q, want %q — "+
			"WithFunctionOutputFormat is missing from configDerivedServerOpts", gotFormat, "json_schema")
	}
}

// TestMediaStorageServerOpts_WiresLocalBackend is the wiring test for #1817
// Task 4-5's critical gap: internal/runtime.WithMediaStorage /
// HasMediaStorage were defined and unit-tested, but cmd/runtime/main.go never
// called WithMediaStorage — the runtime built its serverOpts slice without
// ever constructing a media.Storage, so the whole storage_ref attachment
// feature was inert in production even though it worked in every
// internal/runtime unit test.
//
// This test calls mediaStorageServerOpts — the exact helper main() calls —
// with OMNIA_MEDIA_STORAGE_TYPE=local pointed at a temp dir, applies the
// resulting options to a real pkruntime.Server, and asserts
// HasMediaStorage() is true. Deleting the `serverOpts = append(serverOpts,
// mediaOpts...)` line from main(), or the WithMediaStorage call inside
// mediaStorageServerOpts/initMediaStorage, makes this test fail.
func TestMediaStorageServerOpts_WiresLocalBackend(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_TYPE", "local")
	t.Setenv("OMNIA_MEDIA_STORAGE_PATH", t.TempDir())

	opts, cleanup := mediaStorageServerOpts(logr.Discard())
	if cleanup != nil {
		t.Cleanup(cleanup)
	}
	if len(opts) == 0 {
		t.Fatal("mediaStorageServerOpts returned no options for a configured local media backend")
	}

	srv := pkruntime.NewServer(opts...)
	t.Cleanup(func() { _ = srv.Close() })

	if !srv.HasMediaStorage() {
		t.Error("HasMediaStorage() is false after mediaStorageServerOpts — " +
			"cmd/runtime is not calling WithMediaStorage, so storage_ref " +
			"attachments are inert in production (#1817)")
	}
}

// TestMediaStorageServerOpts_DisabledWhenUnset is the negative counterpart:
// with OMNIA_MEDIA_STORAGE_TYPE unset (defaults to "none"), mediaStorageServerOpts
// must return no options and the resulting server must report
// HasMediaStorage() == false. Guards against a future change that always
// wires a store regardless of configuration.
func TestMediaStorageServerOpts_DisabledWhenUnset(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_TYPE", "")

	opts, cleanup := mediaStorageServerOpts(logr.Discard())
	if cleanup != nil {
		t.Cleanup(cleanup)
	}
	if len(opts) != 0 {
		t.Fatalf("mediaStorageServerOpts returned %d options when media storage is unset, want 0", len(opts))
	}

	srv := pkruntime.NewServer(opts...)
	t.Cleanup(func() { _ = srv.Close() })

	if srv.HasMediaStorage() {
		t.Error("HasMediaStorage() is true even though media storage was never configured")
	}
}
