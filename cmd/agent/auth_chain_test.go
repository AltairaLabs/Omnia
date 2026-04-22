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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/facade/auth"
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

func TestBuildAuthChain_NoK8sClientReturnsMgmtOnly(t *testing.T) {
	t.Parallel()
	mgmt := &stubMgmtValidator{id: &policy.AuthenticatedIdentity{Origin: policy.OriginManagementPlane}}
	chain, err := buildAuthChain(context.Background(), nil, logr.Discard(), "agent", "ns", mgmt)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got, want := len(chain), 1; got != want {
		t.Fatalf("chain length = %d, want %d", got, want)
	}
}

func TestBuildAuthChain_NoK8sClientNoMgmtReturnsNil(t *testing.T) {
	t.Parallel()
	chain, err := buildAuthChain(context.Background(), nil, logr.Discard(), "agent", "ns", nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if chain != nil {
		t.Errorf("chain = %v, want nil when no validators", chain)
	}
}

func TestBuildAuthChain_AgentNotFoundFallsBackToMgmt(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	mgmt := &stubMgmtValidator{}
	chain, err := buildAuthChain(context.Background(), fc, logr.Discard(), "missing-agent", "ns", mgmt)
	if err != nil {
		t.Fatalf("err = %v, want nil (NotFound is expected during pod startup)", err)
	}
	if got, want := len(chain), 1; got != want {
		t.Errorf("chain length = %d, want %d (mgmt-plane only)", got, want)
	}
}

func TestBuildAuthChain_ExternalAuthUnsetReturnsMgmtOnly(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
		// No ExternalAuth — preserves PR 1c default of mgmt-plane only.
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ar).Build()
	mgmt := &stubMgmtValidator{}
	chain, err := buildAuthChain(context.Background(), fc, logr.Discard(), "agent", "ns", mgmt)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got, want := len(chain), 1; got != want {
		t.Errorf("chain length = %d, want %d", got, want)
	}
}

func TestBuildAuthChain_SharedTokenAddsValidatorBeforeMgmt(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-token", Namespace: "ns"},
		Data:       map[string][]byte{"token": []byte("opaque-bearer")},
	}
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				SharedToken: &omniav1alpha1.SharedTokenAuth{
					SecretRef: corev1.LocalObjectReference{Name: "shared-token"},
				},
			},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ar, secret).Build()
	mgmt := &stubMgmtValidator{}

	chain, err := buildAuthChain(context.Background(), fc, logr.Discard(), "agent", "ns", mgmt)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got, want := len(chain), 2; got != want {
		t.Fatalf("chain length = %d, want %d (sharedToken + mgmt)", got, want)
	}

	// Prove sharedToken really is wired by exercising the chain end-to-end:
	// a request with the right Bearer should admit via shared-token, NOT
	// via mgmt-plane (mgmt-plane would also admit because the stub admits
	// everything; we want to prove order).
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Authorization", "Bearer opaque-bearer")
	id, err := chain.Run(context.Background(), r)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := id.Origin, policy.OriginSharedToken; got != want {
		t.Errorf("Origin = %q, want %q (sharedToken should win first)", got, want)
	}
}

// TestBuildAuthChain_LegacyA2AAuthenticationProjects proves B1 is fixed:
// an AgentRuntime that uses only the deprecated
// spec.a2a.authentication.secretRef (with no spec.externalAuth) must
// produce a data-plane chain containing the sharedToken validator. The
// reconciler runs the projection on an in-memory copy that never gets
// persisted, so cmd/agent has to re-run it at startup or the customer's
// A2A traffic 401s after PR 3's default flip.
func TestBuildAuthChain_LegacyA2AAuthenticationProjects(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "legacy-token", Namespace: "ns"},
		Data:       map[string][]byte{"token": []byte("legacy-bearer")},
	}
	// Only the deprecated shape is set — the new spec.externalAuth field
	// is deliberately nil to mirror a CR written before the redesign.
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			A2A: &omniav1alpha1.A2AConfig{
				Authentication: &omniav1alpha1.A2AAuthConfig{
					SecretRef: &corev1.LocalObjectReference{Name: "legacy-token"},
				},
			},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ar, secret).Build()

	chain, err := buildAuthChain(context.Background(), fc, logr.Discard(), "agent", "ns", nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got, want := len(chain), 1; got != want {
		t.Fatalf("chain length = %d, want %d (sharedToken projected from legacy a2a)", got, want)
	}

	// Exercise the chain end-to-end: a request with the legacy bearer
	// must admit via sharedToken and come out tagged as such.
	r := httptest.NewRequest(http.MethodGet, "/a2a", nil)
	r.Header.Set("Authorization", "Bearer legacy-bearer")
	id, err := chain.Run(context.Background(), r)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := id.Origin, policy.OriginSharedToken; got != want {
		t.Errorf("Origin = %q, want %q", got, want)
	}
}

// TestBuildAuthChain_ExternalAuthWinsOverLegacy proves the precedence
// rule in ProjectLegacyA2AAuth: when both shapes are set, the new
// externalAuth.sharedToken stays untouched (operators who migrated
// deliberately must not get silently overwritten).
func TestBuildAuthChain_ExternalAuthWinsOverLegacy(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "new-token", Namespace: "ns"},
		Data:       map[string][]byte{"token": []byte("new-bearer")},
	}
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			A2A: &omniav1alpha1.A2AConfig{
				Authentication: &omniav1alpha1.A2AAuthConfig{
					SecretRef: &corev1.LocalObjectReference{Name: "legacy-token"},
				},
			},
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				SharedToken: &omniav1alpha1.SharedTokenAuth{
					SecretRef: corev1.LocalObjectReference{Name: "new-token"},
				},
			},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ar, newSecret).Build()

	chain, err := buildAuthChain(context.Background(), fc, logr.Discard(), "agent", "ns", nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got, want := len(chain), 1; got != want {
		t.Fatalf("chain length = %d, want %d", got, want)
	}

	// Only the new bearer admits — confirms the legacy secret was NOT
	// projected on top of the already-set externalAuth.
	r := httptest.NewRequest(http.MethodGet, "/a2a", nil)
	r.Header.Set("Authorization", "Bearer new-bearer")
	if _, err := chain.Run(context.Background(), r); err != nil {
		t.Errorf("new bearer should admit: %v", err)
	}
	r2 := httptest.NewRequest(http.MethodGet, "/a2a", nil)
	r2.Header.Set("Authorization", "Bearer legacy-bearer")
	if _, err := chain.Run(context.Background(), r2); err == nil {
		t.Error("legacy bearer must NOT admit when externalAuth.sharedToken is explicitly set")
	}
}

func TestBuildAuthChain_SharedTokenSecretMissingErrors(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				SharedToken: &omniav1alpha1.SharedTokenAuth{
					SecretRef: corev1.LocalObjectReference{Name: "absent"},
				},
			},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ar).Build()

	if _, err := buildAuthChain(context.Background(), fc, logr.Discard(), "agent", "ns", nil); err == nil {
		t.Error("expected error when sharedToken secret is missing")
	}
}

func TestBuildAuthChain_SharedTokenSecretMissingTokenKeyErrors(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	badSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "wrong-keys", Namespace: "ns"},
		Data:       map[string][]byte{"value": []byte("xx")},
	}
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				SharedToken: &omniav1alpha1.SharedTokenAuth{
					SecretRef: corev1.LocalObjectReference{Name: "wrong-keys"},
				},
			},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ar, badSecret).Build()
	if _, err := buildAuthChain(context.Background(), fc, logr.Discard(), "agent", "ns", nil); err == nil {
		t.Error("expected error when sharedToken secret missing the 'token' data key")
	}
}

func TestBuildAuthChain_TrustEndUserHeaderPropagates(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: "ns"},
		Data:       map[string][]byte{"token": []byte("tok")},
	}
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				SharedToken: &omniav1alpha1.SharedTokenAuth{
					SecretRef:          corev1.LocalObjectReference{Name: "shared"},
					TrustEndUserHeader: true,
				},
			},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ar, secret).Build()
	chain, err := buildAuthChain(context.Background(), fc, logr.Discard(), "agent", "ns", nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Authorization", "Bearer tok")
	r.Header.Set(auth.EndUserHeader, "alice@example.com")
	id, err := chain.Run(context.Background(), r)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := id.EndUser, "alice@example.com"; got != want {
		t.Errorf("EndUser = %q, want %q (trustEndUserHeader=true should propagate)", got, want)
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
						Role:    "X-Custom-Role",
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

func TestBuildAuthChain_EdgeTrustJoinsAfterSharedToken(t *testing.T) {
	// End-to-end: buildDataPlaneValidators on an AgentRuntime with both
	// sharedToken AND edgeTrust configured produces a chain where
	// sharedToken comes first (matching request that presents a Bearer
	// admits via sharedToken) and edgeTrust is present for requests
	// carrying only an x-user-id header.
	t.Parallel()
	scheme := newTestScheme(t)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: "ns"},
		Data:       map[string][]byte{"token": []byte("bearer-value")},
	}
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				SharedToken: &omniav1alpha1.SharedTokenAuth{
					SecretRef: corev1.LocalObjectReference{Name: "shared"},
				},
				EdgeTrust: &omniav1alpha1.EdgeTrustAuth{},
			},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ar, secret).Build()
	chain, err := buildAuthChain(context.Background(), fc, logr.Discard(), "a", "ns", nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got, want := len(chain), 2; got != want {
		t.Fatalf("chain length = %d, want %d (sharedToken + edgeTrust)", got, want)
	}

	// Request with Bearer admits via sharedToken.
	r1 := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r1.Header.Set("Authorization", "Bearer bearer-value")
	id1, err := chain.Run(context.Background(), r1)
	if err != nil {
		t.Fatalf("Run (bearer): %v", err)
	}
	if got, want := id1.Origin, policy.OriginSharedToken; got != want {
		t.Errorf("Origin = %q, want %q (Bearer must admit via sharedToken)", got, want)
	}

	// Request with just x-user-id admits via edgeTrust.
	r2 := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r2.Header.Set(auth.DefaultEdgeSubjectHeader, "alice")
	id2, err := chain.Run(context.Background(), r2)
	if err != nil {
		t.Fatalf("Run (edge): %v", err)
	}
	if got, want := id2.Origin, policy.OriginEdgeTrust; got != want {
		t.Errorf("Origin = %q, want %q (edge header must admit via edgeTrust)", got, want)
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
