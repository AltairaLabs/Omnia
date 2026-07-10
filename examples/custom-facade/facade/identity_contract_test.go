/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package facade_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/altairalabs/omnia/examples/custom-facade/facade"
	"github.com/altairalabs/omnia/internal/runtime"
	"github.com/altairalabs/omnia/pkg/policy"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// capturingRuntime is a stub RuntimeService server that records the policy
// propagation fields the real runtime interceptor rehydrates from the incoming
// gRPC metadata. It is the runtime side of the identity contract: whatever the
// facade emits as x-omnia-* metadata must arrive here as PropagationFields.
type capturingRuntime struct {
	runtimev1.UnimplementedRuntimeServiceServer

	mu     sync.Mutex
	fields policy.PropagationFields
}

func (c *capturingRuntime) Converse(stream runtimev1.RuntimeService_ConverseServer) error {
	// Read the first client message so the stream is fully established.
	if _, err := stream.Recv(); err != nil {
		return err
	}
	c.mu.Lock()
	c.fields = policy.ExtractPropagationFields(stream.Context())
	c.mu.Unlock()
	// Reply with a Done so the facade's drain loop terminates cleanly.
	return stream.Send(&runtimev1.ServerMessage{
		Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{}},
	})
}

func (c *capturingRuntime) captured() policy.PropagationFields {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.fields
}

// startRuntime boots the stub RuntimeService with the REAL runtime policy
// interceptors installed, on an ephemeral localhost port.
func startRuntime(t *testing.T) (string, *capturingRuntime, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(runtime.PolicyUnaryServerInterceptor()),
		grpc.StreamInterceptor(runtime.PolicyStreamServerInterceptor()),
	)
	capture := &capturingRuntime{}
	runtimev1.RegisterRuntimeServiceServer(srv, capture)
	go func() { _ = srv.Serve(lis) }()
	return lis.Addr().String(), capture, srv.Stop
}

// TestCustomFacade_RuntimeSeesIdentity is the core runtime-side contract test:
// when the reference facade authenticates its own protocol and emits the flat
// x-omnia-* metadata over the runtime gRPC hop, the runtime's policy
// interceptor must rehydrate the caller's id, roles, full claim map, origin
// and workspace.
func TestCustomFacade_RuntimeSeesIdentity(t *testing.T) {
	addr, capture, stop := startRuntime(t)
	defer stop()

	const agentName = "support-bot"
	rc, err := facade.Dial(addr, agentName)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = rc.Close() }()

	auth := facade.NewAuthenticator(map[string]*facade.Principal{
		"tok": {
			UserID:    "user-42",
			Roles:     []string{policy.RoleAdmin, policy.RoleEditor},
			Workspace: "acme",
			Origin:    policy.OriginSharedToken,
			Claims:    map[string]string{"tier": "gold", "team": "finance", "region": "emea"},
		},
	})
	principal, err := auth.Authenticate("Bearer tok")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := rc.Converse(ctx, principal, "sess-1", "hello"); err != nil {
		t.Fatalf("converse: %v", err)
	}

	got := capture.captured()
	if got.UserID != "user-42" {
		t.Errorf("UserID = %q, want user-42", got.UserID)
	}
	if got.UserRoles != "admin,editor" {
		t.Errorf("UserRoles = %q, want admin,editor", got.UserRoles)
	}
	if got.Origin != policy.OriginSharedToken {
		t.Errorf("Origin = %q, want %q", got.Origin, policy.OriginSharedToken)
	}
	if got.Workspace != "acme" {
		t.Errorf("Workspace = %q, want acme", got.Workspace)
	}
	if got.AgentName != agentName {
		t.Errorf("AgentName = %q, want %q", got.AgentName, agentName)
	}
	// Full claim map must arrive verbatim — not a subset.
	wantClaims := map[string]string{"tier": "gold", "team": "finance", "region": "emea"}
	if len(got.Claims) != len(wantClaims) {
		t.Fatalf("Claims = %#v, want %#v", got.Claims, wantClaims)
	}
	for k, v := range wantClaims {
		if got.Claims[k] != v {
			t.Errorf("Claims[%q] = %q, want %q", k, got.Claims[k], v)
		}
	}
}

// TestCustomFacade_AnonymousEmitsNoIdentity confirms the negative baseline:
// an unauthenticated request never reaches Converse, so the facade never
// emits identity. Here we assert the emission helper for a bare principal
// produces no identity headers, so a missing credential cannot masquerade as
// an authenticated one on the wire.
func TestCustomFacade_AnonymousEmitsNoIdentity(t *testing.T) {
	empty := &facade.Principal{}
	md := empty.OutboundMetadata("")
	for _, k := range []string{policy.HeaderUserID, policy.HeaderUserRoles, policy.HeaderOrigin, policy.HeaderWorkspace} {
		if v, ok := md[k]; ok {
			t.Errorf("empty principal emitted %q=%q, want absent", k, v)
		}
	}
}
