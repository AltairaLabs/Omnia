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
