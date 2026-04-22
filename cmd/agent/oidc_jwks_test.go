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
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/facade/auth"
)

const testJWKSKid = "rotating-1"

// buildTestJWKSBlob returns a JWKS JSON blob plus the RSA private key
// it was generated from — tests can then sign JWTs with the private
// half and verify via the public JWKS. The kid is fixed to testJWKSKid
// so tests can refer to it without threading a parameter.
func buildTestJWKSBlob(t *testing.T) ([]byte, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	jwks := auth.JWKS{Keys: []auth.JSONWebKey{{
		Kty: "RSA",
		Kid: testJWKSKid,
		Alg: "RS256",
		N:   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}}}
	blob, err := json.Marshal(jwks)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return blob, key
}

func TestSecretBackedJWKSStore_LoadsInitialJWKS(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	blob, _ := buildTestJWKSBlob(t)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-x-oidc-jwks",
			Namespace: "ns",
		},
		Data: map[string][]byte{OIDCJWKSDataKey: blob},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	store, err := NewSecretBackedJWKSStore(context.Background(), fc, "ns", "agent-x-oidc-jwks", logr.Discard(),
		WithJWKSRefreshInterval(time.Hour))
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	defer store.Stop()

	if _, ok := store.Lookup(testJWKSKid); !ok {
		t.Error("expected key to be present after initial load")
	}
}

func TestSecretBackedJWKSStore_MissingSecretErrors(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()

	_, err := NewSecretBackedJWKSStore(context.Background(), fc, "ns", "agent-x-oidc-jwks", logr.Discard(),
		WithJWKSRefreshInterval(time.Hour))
	if err == nil {
		t.Error("expected fatal error when JWKS Secret is missing")
	}
}

func TestSecretBackedJWKSStore_MissingDataKeyErrors(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-x-oidc-jwks",
			Namespace: "ns",
		},
		Data: map[string][]byte{"some-other-key": []byte("ignored")},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	_, err := NewSecretBackedJWKSStore(context.Background(), fc, "ns", "agent-x-oidc-jwks", logr.Discard(),
		WithJWKSRefreshInterval(time.Hour))
	if err == nil {
		t.Errorf("expected error when Secret missing %q data key", OIDCJWKSDataKey)
	}
}

func TestSecretBackedJWKSStore_MalformedJWKSErrors(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-x-oidc-jwks",
			Namespace: "ns",
		},
		Data: map[string][]byte{OIDCJWKSDataKey: []byte("not-json")},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	_, err := NewSecretBackedJWKSStore(context.Background(), fc, "ns", "agent-x-oidc-jwks", logr.Discard(),
		WithJWKSRefreshInterval(time.Hour))
	if err == nil {
		t.Error("expected error on malformed JWKS blob")
	}
}

func TestSecretBackedJWKSStore_ClockOption(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	blob, _ := buildTestJWKSBlob(t)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "jwks", Namespace: "ns"},
		Data:       map[string][]byte{OIDCJWKSDataKey: blob},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	fixed := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)

	store, err := NewSecretBackedJWKSStore(context.Background(), fc, "ns", "jwks", logr.Discard(),
		WithJWKSRefreshInterval(time.Hour),
		WithJWKSClock(func() time.Time { return fixed }),
	)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	defer store.Stop()

	store.mu.RLock()
	got := store.lastRefresh
	store.mu.RUnlock()
	if !got.Equal(fixed) {
		t.Errorf("lastRefresh = %v, want %v", got, fixed)
	}
}

func TestSecretBackedJWKSStore_LookupReturnsFalseForUnknownKid(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	blob, _ := buildTestJWKSBlob(t)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "jwks", Namespace: "ns"},
		Data:       map[string][]byte{OIDCJWKSDataKey: blob},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	store, err := NewSecretBackedJWKSStore(context.Background(), fc, "ns", "jwks", logr.Discard(),
		WithJWKSRefreshInterval(time.Hour))
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	defer store.Stop()

	if _, ok := store.Lookup("not-in-jwks"); ok {
		t.Error("expected miss on unknown kid — IdP rotation scenario")
	}
}

func TestOIDCJWKSSecretNameFor(t *testing.T) {
	t.Parallel()
	got := OIDCJWKSSecretNameFor("my-agent")
	want := "agent-my-agent-oidc-jwks"
	if got != want {
		t.Errorf("OIDCJWKSSecretNameFor = %q, want %q", got, want)
	}
}

func TestBuildOIDCValidator_UnsetReturnsNil(t *testing.T) {
	t.Parallel()
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{}, // no OIDC
		},
	}
	scheme := newTestScheme(t)
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()

	v, err := buildOIDCValidator(context.Background(), fc, logr.Discard(), ar)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v != nil {
		t.Errorf("expected nil validator when OIDC unset, got %v", v)
	}
}

func TestBuildOIDCValidator_MissingIssuerOrAudienceErrors(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Issuer missing.
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				OIDC: &omniav1alpha1.OIDCAuth{Audience: "omnia"},
			},
		},
	}
	if _, err := buildOIDCValidator(context.Background(), fc, logr.Discard(), ar); err == nil {
		t.Error("expected error on empty issuer")
	}

	// Audience missing.
	ar.Spec.ExternalAuth.OIDC = &omniav1alpha1.OIDCAuth{Issuer: "https://idp.example.com"}
	if _, err := buildOIDCValidator(context.Background(), fc, logr.Discard(), ar); err == nil {
		t.Error("expected error on empty audience")
	}
}

func TestBuildOIDCValidator_MissingJWKSSecretErrors(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				OIDC: &omniav1alpha1.OIDCAuth{
					Issuer:   "https://idp.example.com",
					Audience: "omnia",
				},
			},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	if _, err := buildOIDCValidator(context.Background(), fc, logr.Discard(), ar); err == nil {
		t.Error("expected error when JWKS Secret is absent — operator must populate it")
	}
}

func TestBuildOIDCValidator_HappyPath(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	blob, _ := buildTestJWKSBlob(t)
	jwksSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-myagent-oidc-jwks",
			Namespace: "ns",
		},
		Data: map[string][]byte{OIDCJWKSDataKey: blob},
	}
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "myagent", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				OIDC: &omniav1alpha1.OIDCAuth{
					Issuer:   "https://idp.example.com",
					Audience: "omnia",
				},
			},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(jwksSecret).Build()

	v, err := buildOIDCValidator(context.Background(), fc, logr.Discard(), ar)
	if err != nil {
		t.Fatalf("buildOIDCValidator: %v", err)
	}
	if v == nil {
		t.Fatal("nil validator on happy path")
	}
}

func TestBuildOIDCValidator_ClaimMappingPropagates(t *testing.T) {
	// The CRD's OIDCClaimMapping must reach the auth.OIDCClaimMapping
	// inside the validator; a regression would silently default to `sub`
	// and break customers using service tokens with actor claims.
	t.Parallel()
	scheme := newTestScheme(t)
	blob, _ := buildTestJWKSBlob(t)
	jwksSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-a-oidc-jwks",
			Namespace: "ns",
		},
		Data: map[string][]byte{OIDCJWKSDataKey: blob},
	}
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				OIDC: &omniav1alpha1.OIDCAuth{
					Issuer:   "https://idp.example.com",
					Audience: "omnia",
					ClaimMapping: &omniav1alpha1.OIDCClaimMapping{
						Subject: "user_id",
						Role:    "tier",
						EndUser: "actor",
					},
				},
			},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(jwksSecret).Build()

	v, err := buildOIDCValidator(context.Background(), fc, logr.Discard(), ar)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if v == nil {
		t.Fatal("nil validator")
	}
	// Validator is opaque — we can't introspect its mapping directly
	// without exposing private fields. The behavioural test for the
	// mapping (a token whose claims use the custom names admits
	// correctly) lives in internal/facade/auth/oidc_test.go:
	// TestOIDCValidator_CustomClaimMapping. Here we just prove the
	// hook-up doesn't panic or error.
}
