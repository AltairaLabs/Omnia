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

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/facade/auth"
	"github.com/altairalabs/omnia/pkg/policy"
)

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := omniav1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add omnia scheme: %v", err)
	}
	return scheme
}

// TestBuildExternalChain_AgentRuntimeFoundBuildsValidators exercises the
// "AgentRuntime found" branch of buildExternalChain end-to-end via
// buildDataPlaneValidators (clientKeys/oidc unset, edgeTrust set — edgeTrust is
// pure-Go so this needs no Secret fixture). No mgmt validator is present.
func TestBuildExternalChain_AgentRuntimeFoundBuildsValidators(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				EdgeTrust: &omniav1alpha1.EdgeTrustAuth{},
			},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ar).Build()

	chain, err := buildExternalChain(context.Background(), fc, logr.Discard(), "agent", "ns")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got, want := len(chain), 1; got != want {
		t.Fatalf("chain length = %d, want %d (edgeTrust only, no mgmt)", got, want)
	}

	// Prove the validator really is wired by exercising the chain end-to-end.
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set(auth.DefaultEdgeSubjectHeader, "alice")
	id, err := chain.Run(context.Background(), r)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := id.Origin, policy.OriginEdgeTrust; got != want {
		t.Errorf("Origin = %q, want %q (edgeTrust should admit)", got, want)
	}
}

// TestBuildExternalChain_ClientKeysBuildsValidator covers buildClientKeyValidator:
// a spec with clientKeys (defaultRole set) builds a SecretBackedKeyStore-backed
// validator, an unknown bearer falls through it, AND a known key admits with the
// configured defaultRole surfaced as identity.claims.role — proving the CRD's
// defaultRole actually wires through to the validator, not just that a validator exists.
func TestBuildExternalChain_ClientKeysBuildsValidator(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns", UID: "agent-uid-1"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				ClientKeys: &omniav1alpha1.ClientKeysAuth{
					DefaultRole:        "viewer",
					TrustEndUserHeader: true,
				},
			},
		},
	}
	// A known key with no scopes → the validator applies the configured defaultRole.
	// Owned by the AgentRuntime so it passes the store's ownerRef gate.
	const rawKey = "omk_wired_client_key"
	keySecret := newClientKeySecret("agent-agent-clientkey-k1", "agent", sha256Bytes(rawKey), "", "")
	keySecret.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "omnia.altairalabs.ai/v1alpha1", Kind: "AgentRuntime", Name: "agent", UID: "agent-uid-1",
	}}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ar, keySecret).Build()

	chain, err := buildExternalChain(context.Background(), fc, logr.Discard(), "agent", "ns")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got, want := len(chain), 1; got != want {
		t.Fatalf("chain length = %d, want %d (clientKeys only)", got, want)
	}

	// Unknown bearer falls through.
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Authorization", "Bearer not-a-known-key")
	if _, err := chain.Run(context.Background(), r); !errors.Is(err, auth.ErrNoCredential) {
		t.Fatalf("Run err = %v, want ErrNoCredential (unknown key falls through)", err)
	}

	// Known key admits, and the configured defaultRole surfaces as identity.claims.role.
	r2 := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r2.Header.Set("Authorization", "Bearer "+rawKey)
	id, err := chain.Run(context.Background(), r2)
	if err != nil {
		t.Fatalf("Run(known key) err = %v, want admit", err)
	}
	if got, want := id.Origin, policy.OriginClientKey; got != want {
		t.Errorf("Origin = %q, want %q", got, want)
	}
	if got, want := id.Claims["role"], "viewer"; got != want {
		t.Errorf("Claims[role] = %q, want %q (CRD defaultRole must wire through)", got, want)
	}
}

// TestBuildExternalChain_ClientKeysStoreInitErrorPropagates covers the
// error paths in buildClientKeyValidator and its caller: the scheme carries the
// AgentRuntime kind (so the initial Get succeeds) but not core types, so the
// client-key store's Secret List fails and the error must propagate out.
func TestBuildExternalChain_ClientKeysStoreInitErrorPropagates(t *testing.T) {
	t.Parallel()
	scheme := runtime.NewScheme()
	if err := omniav1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add omnia scheme: %v", err)
	}
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns", UID: "u1"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				ClientKeys: &omniav1alpha1.ClientKeysAuth{},
			},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ar).Build()

	if _, err := buildExternalChain(context.Background(), fc, logr.Discard(), "agent", "ns"); err == nil {
		t.Fatal("expected error when the client-key store's Secret List fails")
	}
}

func TestBuildEdgeTrustValidator_UnsetReturnsNil(t *testing.T) {
	t.Parallel()
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				// EdgeTrust unset → no validator added
			},
		},
	}
	if v := buildEdgeTrustValidator(logr.Discard(), ar); v != nil {
		t.Errorf("expected nil validator when spec.externalAuth.edgeTrust unset, got %v", v)
	}
}

func TestBuildEdgeTrustValidator_EmptyConfigUsesDefaults(t *testing.T) {
	// spec.externalAuth.edgeTrust: {} (no HeaderMapping, no ClaimsFromHeaders)
	// should still produce a functional validator using the shipped
	// Istio default mapping.
	t.Parallel()
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				EdgeTrust: &omniav1alpha1.EdgeTrustAuth{},
			},
		},
	}
	v := buildEdgeTrustValidator(logr.Discard(), ar)
	if v == nil {
		t.Fatal("expected non-nil validator for empty EdgeTrustAuth block")
	}

	// Prove it uses the default Istio mapping by exercising it.
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set(auth.DefaultEdgeSubjectHeader, "alice")
	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got, want := id.Origin, policy.OriginEdgeTrust; got != want {
		t.Errorf("Origin = %q, want %q", got, want)
	}
}

func TestBuildEdgeTrustValidator_HeaderMappingPropagates(t *testing.T) {
	// Custom HeaderMapping on the CRD should be honoured by the built
	// validator: a request with the chart's default x-user-id header
	// should NOT admit, but a request with the custom header should.
	t.Parallel()
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				EdgeTrust: &omniav1alpha1.EdgeTrustAuth{
					HeaderMapping: &omniav1alpha1.EdgeTrustHeaderMapping{
						Subject: "X-Custom-Subject",
						EndUser: "X-Custom-EndUser",
						Email:   "X-Custom-Email",
					},
				},
			},
		},
	}
	v := buildEdgeTrustValidator(logr.Discard(), ar)
	if v == nil {
		t.Fatal("expected non-nil validator")
	}

	// Default header ignored.
	r1 := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r1.Header.Set(auth.DefaultEdgeSubjectHeader, "should-not-admit")
	if _, err := v.Validate(context.Background(), r1); err == nil {
		t.Error("expected default header to be ignored after override")
	}

	// Custom header admits.
	r2 := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r2.Header.Set("X-Custom-Subject", "bob")
	if _, err := v.Validate(context.Background(), r2); err != nil {
		t.Errorf("custom header should admit: %v", err)
	}
}

func TestBuildEdgeTrustValidator_NoHeaderMappingRoleFallsBackToDefault(t *testing.T) {
	// The CRD no longer has a HeaderMapping.Role override field (removed
	// alongside the dead Identity.Role plumbing). This proves edgeTrust
	// still surfaces the role claim end-to-end via its own internal
	// default header (DefaultEdgeRoleHeader = "x-user-roles") even with
	// HeaderMapping entirely unset.
	t.Parallel()
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				EdgeTrust: &omniav1alpha1.EdgeTrustAuth{
					// HeaderMapping intentionally unset.
				},
			},
		},
	}
	v := buildEdgeTrustValidator(logr.Discard(), ar)
	if v == nil {
		t.Fatal("expected non-nil validator")
	}

	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set(auth.DefaultEdgeSubjectHeader, "alice")
	r.Header.Set("x-user-roles", "admin,editor")
	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got, want := id.Claims["role"], "admin,editor"; got != want {
		t.Errorf("Claims[role] = %q, want %q (edge must still surface role via its internal default header)", got, want)
	}
}

func TestBuildEdgeTrustValidator_ClaimsFromHeadersPropagate(t *testing.T) {
	t.Parallel()
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				EdgeTrust: &omniav1alpha1.EdgeTrustAuth{
					ClaimsFromHeaders: map[string]string{
						"X-User-Groups": "groups",
					},
				},
			},
		},
	}
	v := buildEdgeTrustValidator(logr.Discard(), ar)
	if v == nil {
		t.Fatal("expected non-nil validator")
	}

	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set(auth.DefaultEdgeSubjectHeader, "alice")
	r.Header.Set("X-User-Groups", "finance,eng")
	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got, want := id.Claims["groups"], "finance,eng"; got != want {
		t.Errorf("Claims[groups] = %q, want %q (CRD-configured ClaimsFromHeaders must plumb through)", got, want)
	}
}

// stubMgmtValidator is a minimal Validator for chain-composition tests.
type stubMgmtValidator struct {
	id *policy.AuthenticatedIdentity
}

func (s *stubMgmtValidator) Validate(_ context.Context, _ *http.Request) (*policy.AuthenticatedIdentity, error) {
	if s.id != nil {
		return s.id, nil
	}
	return &policy.AuthenticatedIdentity{Origin: policy.OriginManagementPlane}, nil
}

func TestBuildMgmtChain(t *testing.T) {
	t.Parallel()
	if got := buildMgmtChain(nil); len(got) != 0 {
		t.Errorf("buildMgmtChain(nil) length = %d, want 0", len(got))
	}
	if got := buildMgmtChain(&stubMgmtValidator{}); len(got) != 1 {
		t.Errorf("buildMgmtChain(validator) length = %d, want 1", len(got))
	}
}

func TestBuildExternalChain_NoK8sReturnsEmpty(t *testing.T) {
	t.Parallel()
	chain, err := buildExternalChain(context.Background(), nil, logr.Discard(), "agent", "ns")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(chain) != 0 {
		t.Errorf("chain length = %d, want 0 (no k8s, no validators)", len(chain))
	}
}
