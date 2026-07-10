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

package facade_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/pkg/facade"
	"github.com/altairalabs/omnia/pkg/facade/auth"
	"github.com/altairalabs/omnia/pkg/policy"
)

const testToken = "s3cr3t-shared-token"

func newSharedTokenChain(t *testing.T) facade.Chain {
	t.Helper()
	v, err := auth.NewSharedTokenValidator(testToken)
	if err != nil {
		t.Fatalf("NewSharedTokenValidator: %v", err)
	}
	return facade.Chain{v}
}

func TestAuthenticate_AdmitsValidCredentialAndAttachesIdentity(t *testing.T) {
	var gotIdentity *facade.Identity
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdentity = facade.IdentityFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	h := facade.Authenticate(newSharedTokenChain(t), next, facade.WithLogger(logr.Discard()))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotIdentity == nil {
		t.Fatal("expected an identity attached to the request context")
	}
}

func TestAuthenticate_RejectsMissingCredential(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := facade.Authenticate(newSharedTokenChain(t), next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestIdentityFromContext_NilWhenAbsent(t *testing.T) {
	if id := facade.IdentityFromContext(context.Background()); id != nil {
		t.Fatalf("expected nil identity, got %+v", id)
	}
}

func TestPropagateIdentity_IdentityWorkspaceWinsOverScope(t *testing.T) {
	id := &facade.Identity{
		Origin:    policy.OriginManagementPlane,
		Workspace: "token-workspace",
		Claims:    map[string]string{"tier": "gold"},
	}
	scope := facade.IdentityScope{
		AgentName: "agent-a",
		Namespace: "ns-a",
		Workspace: "deployed-workspace",
		RequestID: "req-1",
		UserID:    "user-1",
	}
	ctx := facade.PropagateIdentity(context.Background(), id, scope)

	md := facade.OutboundMetadata(ctx)
	if md[policy.HeaderWorkspace] != "token-workspace" {
		t.Fatalf("expected token workspace to win, got %q", md[policy.HeaderWorkspace])
	}
	if md[policy.HeaderOrigin] != policy.OriginManagementPlane {
		t.Fatalf("expected origin from identity, got %q", md[policy.HeaderOrigin])
	}
	if md[policy.HeaderClaimPrefix+"tier"] != "gold" {
		t.Fatalf("expected claim header, got %q", md[policy.HeaderClaimPrefix+"tier"])
	}
	if facade.IdentityFromContext(ctx) == nil {
		t.Fatal("expected identity round-tripped into context")
	}
}

func TestPropagateIdentity_FallsBackToScopeWorkspaceAndNilIdentity(t *testing.T) {
	scope := facade.IdentityScope{
		AgentName: "agent-b",
		Namespace: "ns-b",
		Workspace: "deployed-workspace",
	}
	ctx := facade.PropagateIdentity(context.Background(), nil, scope)

	md := facade.OutboundMetadata(ctx)
	if md[policy.HeaderWorkspace] != "deployed-workspace" {
		t.Fatalf("expected fallback workspace, got %q", md[policy.HeaderWorkspace])
	}
	if _, ok := md[policy.HeaderOrigin]; ok {
		t.Fatalf("expected no origin for nil identity, got %q", md[policy.HeaderOrigin])
	}
	if facade.IdentityFromContext(ctx) != nil {
		t.Fatal("expected no identity for nil id")
	}
}

func TestPropagateIdentity_FoldsScopeRolesIntoClaims(t *testing.T) {
	// scope.UserRoles folds into identity.claims.role (the structured role
	// field was removed in #1775), surfacing as the x-omnia-claim-role header.
	scope := facade.IdentityScope{
		AgentName: "agent-c",
		UserID:    "user-3",
		UserRoles: "admin,editor",
	}
	ctx := facade.PropagateIdentity(context.Background(), nil, scope)

	if got := facade.OutboundMetadata(ctx)[policy.HeaderClaimPrefix+"role"]; got != "admin,editor" {
		t.Fatalf("x-omnia-claim-role = %q, want admin,editor (scope roles fold into the role claim)", got)
	}
}

func TestPropagateIdentity_ScopeRolesDoNotClobberExplicitRoleClaim(t *testing.T) {
	// An explicit "role" claim on the admitting identity wins over the scope's
	// coarse UserRoles fallback.
	id := &facade.Identity{
		Origin: policy.OriginAPIKey,
		Claims: map[string]string{"role": "viewer"},
	}
	scope := facade.IdentityScope{AgentName: "agent-d", UserID: "user-4", UserRoles: "admin"}
	ctx := facade.PropagateIdentity(context.Background(), id, scope)

	if got := facade.OutboundMetadata(ctx)[policy.HeaderClaimPrefix+"role"]; got != "viewer" {
		t.Fatalf("x-omnia-claim-role = %q, want the explicit id claim \"viewer\" to win over scope \"admin\"", got)
	}
}

func TestNewMgmtPlaneValidator_RequiresJWKSURL(t *testing.T) {
	if _, err := facade.NewMgmtPlaneValidator(""); err == nil {
		t.Fatal("expected error for empty JWKS URL")
	}
	v, err := facade.NewMgmtPlaneValidator("http://dashboard.svc/api/auth/jwks")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v == nil {
		t.Fatal("expected a validator")
	}
}

func TestNewSessionRecorder_Constructs(t *testing.T) {
	rec := facade.NewSessionRecorder("http://session-api:8080", logr.Discard())
	if rec == nil {
		t.Fatal("expected a recorder")
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
