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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// newScheme builds a runtime scheme that includes both Kubernetes core
// types and the omnia v1alpha1 types — what every fake-client backed
// AgentRuntime test needs. Inlined here because the previous shared
// helper lived in workspace_mgmt_plane_pubkey_test.go, which was
// removed when the JWKS-based mgmt-plane validator replaced the
// per-workspace pubkey ConfigMap mirror.
func newScheme(t *testing.T) *runtime.Scheme {
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

// fakeJWKSBlob returns a minimal JWKS JSON blob with a single RSA key
// stub. Good enough to satisfy the reconciler's sanity probe without
// needing real cryptographic material.
func fakeJWKSBlob() string {
	return `{"keys":[{"kty":"RSA","kid":"k1","use":"sig","n":"stub","e":"AQAB"}]}`
}

// oidcTestServer spins up an httptest.Server that serves an RFC-8414
// discovery document pointing at its own /jwks endpoint. The returned
// close function MUST be called by the test.
func oidcTestServer(t *testing.T, jwks string) (*httptest.Server, func()) {
	t.Helper()
	mux := http.NewServeMux()
	var serverURL string
	mux.HandleFunc(OIDCDiscoveryPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":   serverURL,
			"jwks_uri": serverURL + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(jwks))
	})
	srv := httptest.NewServer(mux)
	serverURL = srv.URL
	return srv, srv.Close
}

// testAgentName / testAgentNamespace are the conventional identifiers
// used across the reconciler tests — picked once so assertions can
// hard-code derived Secret/label values without drifting as new test
// cases are added.
const (
	testAgentName      = "alpha"
	testAgentNamespace = "ws-ns"
)

func newAgentRuntimeWithOIDC(issuer string) *omniav1alpha1.AgentRuntime {
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testAgentName,
			Namespace:  testAgentNamespace,
			UID:        types.UID("uid-" + testAgentName),
			Generation: 1,
		},
	}
	if issuer != "" {
		ar.Spec.ExternalAuth = &omniav1alpha1.AgentExternalAuth{
			OIDC: &omniav1alpha1.OIDCAuth{
				Issuer:   issuer,
				Audience: "audience-x",
			},
		}
	}
	return ar
}

func newOIDCReconciler(t *testing.T, objs ...client.Object) *AgentRuntimeReconciler {
	t.Helper()
	scheme := newScheme(t)
	fc := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&omniav1alpha1.AgentRuntime{}).
		WithObjects(objs...).
		Build()
	return &AgentRuntimeReconciler{Client: fc, Scheme: scheme}
}

func TestReconcileOIDCJWKS_NotConfigured(t *testing.T) {
	t.Parallel()
	ar := newAgentRuntimeWithOIDC("")
	r := newOIDCReconciler(t, ar)

	next, err := r.reconcileOIDCJWKS(context.Background(), ar)
	if err != nil {
		t.Fatalf("err = %v, want nil when OIDC absent", err)
	}
	if next != 0 {
		t.Errorf("next = %v, want 0 (no refresh when not configured)", next)
	}
}

func TestReconcileOIDCJWKS_DisabledDeletesStaleSecret(t *testing.T) {
	t.Parallel()
	ar := newAgentRuntimeWithOIDC("")
	stale := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      oidcJWKSSecretName("alpha"),
			Namespace: testAgentNamespace,
		},
		Data: map[string][]byte{OIDCJWKSDataKey: []byte("{}")},
	}
	r := newOIDCReconciler(t, ar, stale)

	if _, err := r.reconcileOIDCJWKS(context.Background(), ar); err != nil {
		t.Fatalf("err = %v", err)
	}

	got := &corev1.Secret{}
	err := r.Get(context.Background(),
		types.NamespacedName{Namespace: testAgentNamespace, Name: oidcJWKSSecretName("alpha")},
		got)
	if !apierrors.IsNotFound(err) {
		t.Errorf("stale Secret still present (err=%v); expected NotFound", err)
	}
}

func TestReconcileOIDCJWKS_EmptyIssuerErrors(t *testing.T) {
	t.Parallel()
	ar := newAgentRuntimeWithOIDC("https://issuer.example")
	ar.Spec.ExternalAuth.OIDC.Issuer = "" // force empty post-construction
	r := newOIDCReconciler(t, ar)

	_, err := r.reconcileOIDCJWKS(context.Background(), ar)
	if err == nil {
		t.Fatal("expected error on empty issuer")
	}
	// Condition is set to False/MissingIssuer — verify via in-memory AR.
	cond := findCondition(ar.Status.Conditions, ConditionTypeOIDCJWKSReady)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "MissingIssuer" {
		t.Errorf("OIDCJWKSReady condition = %+v, want False/MissingIssuer", cond)
	}
}

func TestReconcileOIDCJWKS_HappyPathUpsertsSecret(t *testing.T) {
	t.Parallel()
	srv, cleanup := oidcTestServer(t, fakeJWKSBlob())
	defer cleanup()

	ar := newAgentRuntimeWithOIDC(srv.URL)
	r := newOIDCReconciler(t, ar)
	r.OIDCHTTPClient = srv.Client()

	next, err := r.reconcileOIDCJWKS(context.Background(), ar)
	if err != nil {
		t.Fatalf("err = %v, want nil on happy path", err)
	}
	if next != OIDCJWKSRefreshInterval {
		t.Errorf("next = %v, want %v", next, OIDCJWKSRefreshInterval)
	}

	got := &corev1.Secret{}
	if err := r.Get(context.Background(),
		types.NamespacedName{Namespace: testAgentNamespace, Name: oidcJWKSSecretName("alpha")},
		got); err != nil {
		t.Fatalf("secret get: %v", err)
	}
	if blob := string(got.Data[OIDCJWKSDataKey]); blob != fakeJWKSBlob() {
		t.Errorf("secret[%s] = %q, want verbatim issuer JWKS", OIDCJWKSDataKey, blob)
	}
	if got.Labels[labelCredentialKind] != LabelCredentialKindAgentOIDCJWKS {
		t.Errorf("labelCredentialKind = %q, want %q",
			got.Labels[labelCredentialKind], LabelCredentialKindAgentOIDCJWKS)
	}
	if got.Labels[labelAppInstance] != "alpha" {
		t.Errorf("labelAppInstance = %q, want %q", got.Labels[labelAppInstance], "alpha")
	}
	if got.Labels[labelAppManagedBy] != labelValueOmniaOperator {
		t.Errorf("labelAppManagedBy = %q, want %q",
			got.Labels[labelAppManagedBy], labelValueOmniaOperator)
	}
	if len(got.OwnerReferences) != 1 || got.OwnerReferences[0].UID != ar.UID {
		t.Errorf("ownerReferences = %+v, want exactly one pointing at AgentRuntime",
			got.OwnerReferences)
	}
	cond := findCondition(ar.Status.Conditions, ConditionTypeOIDCJWKSReady)
	if cond == nil || cond.Status != metav1.ConditionTrue || cond.Reason != "JWKSUpdated" {
		t.Errorf("OIDCJWKSReady condition = %+v, want True/JWKSUpdated", cond)
	}
}

func TestReconcileOIDCJWKS_DiscoveryFailureSetsCondition(t *testing.T) {
	t.Parallel()
	// Point the issuer at a dead URL so discovery fails.
	ar := newAgentRuntimeWithOIDC("http://127.0.0.1:1") // :1 is blackholed
	r := newOIDCReconciler(t, ar)
	r.OIDCHTTPClient = &http.Client{Timeout: 200 * 1e6} // 200ms — short

	next, err := r.reconcileOIDCJWKS(context.Background(), ar)
	if err != nil {
		t.Fatalf("reconcileOIDCJWKS returned hard error: %v (should be swallowed)", err)
	}
	if next != OIDCJWKSRefreshInterval {
		t.Errorf("next = %v, want %v (retry schedule)", next, OIDCJWKSRefreshInterval)
	}
	cond := findCondition(ar.Status.Conditions, ConditionTypeOIDCJWKSReady)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "DiscoveryFailed" {
		t.Errorf("OIDCJWKSReady condition = %+v, want False/DiscoveryFailed", cond)
	}
}

func TestReconcileOIDCJWKS_DiscoveryMissingJWKSURI(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc(OIDCDiscoveryPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Issuer returns a discovery doc with no jwks_uri field.
		_, _ = w.Write([]byte(`{"issuer":"x"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ar := newAgentRuntimeWithOIDC(srv.URL)
	r := newOIDCReconciler(t, ar)
	r.OIDCHTTPClient = srv.Client()

	next, err := r.reconcileOIDCJWKS(context.Background(), ar)
	if err != nil {
		t.Fatalf("reconcileOIDCJWKS returned hard error: %v", err)
	}
	if next != OIDCJWKSRefreshInterval {
		t.Errorf("next = %v, want retry schedule", next)
	}
	cond := findCondition(ar.Status.Conditions, ConditionTypeOIDCJWKSReady)
	if cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("OIDCJWKSReady condition = %+v, want False", cond)
	}
	if !strings.Contains(cond.Message, "jwks_uri") {
		t.Errorf("condition message = %q, want mention of jwks_uri", cond.Message)
	}
}

func TestReconcileOIDCJWKS_InvalidJWKSJSONFails(t *testing.T) {
	t.Parallel()
	srv, cleanup := oidcTestServer(t, "not json at all")
	defer cleanup()

	ar := newAgentRuntimeWithOIDC(srv.URL)
	r := newOIDCReconciler(t, ar)
	r.OIDCHTTPClient = srv.Client()

	next, err := r.reconcileOIDCJWKS(context.Background(), ar)
	if err != nil {
		t.Fatalf("hard error: %v", err)
	}
	if next != OIDCJWKSRefreshInterval {
		t.Errorf("next = %v, want retry schedule", next)
	}
	cond := findCondition(ar.Status.Conditions, ConditionTypeOIDCJWKSReady)
	if cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("condition = %+v, want False", cond)
	}
}

func TestReconcileOIDCJWKS_ZeroKeysFails(t *testing.T) {
	t.Parallel()
	srv, cleanup := oidcTestServer(t, `{"keys":[]}`)
	defer cleanup()

	ar := newAgentRuntimeWithOIDC(srv.URL)
	r := newOIDCReconciler(t, ar)
	r.OIDCHTTPClient = srv.Client()

	next, err := r.reconcileOIDCJWKS(context.Background(), ar)
	if err != nil {
		t.Fatalf("hard error: %v", err)
	}
	if next != OIDCJWKSRefreshInterval {
		t.Errorf("next = %v, want retry schedule", next)
	}
	cond := findCondition(ar.Status.Conditions, ConditionTypeOIDCJWKSReady)
	if cond == nil || !strings.Contains(cond.Message, "no keys") {
		t.Errorf("condition = %+v, want message mentioning 'no keys'", cond)
	}
}

func TestReconcileOIDCJWKS_JWKSFetchNon200(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	var serverURL string
	mux.HandleFunc(OIDCDiscoveryPath, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":   serverURL,
			"jwks_uri": serverURL + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	serverURL = srv.URL
	defer srv.Close()

	ar := newAgentRuntimeWithOIDC(srv.URL)
	r := newOIDCReconciler(t, ar)
	r.OIDCHTTPClient = srv.Client()

	next, err := r.reconcileOIDCJWKS(context.Background(), ar)
	if err != nil {
		t.Fatalf("hard error: %v", err)
	}
	if next != OIDCJWKSRefreshInterval {
		t.Errorf("next = %v, want retry schedule", next)
	}
	cond := findCondition(ar.Status.Conditions, ConditionTypeOIDCJWKSReady)
	if cond == nil || cond.Status != metav1.ConditionFalse {
		t.Errorf("condition = %+v, want False", cond)
	}
}

func TestReconcileOIDCJWKS_UpsertIsIdempotent(t *testing.T) {
	t.Parallel()
	// After a refresh interval has elapsed, a second reconcile with a
	// different JWKS body overwrites the Secret. The T8 fresh-cache
	// fast path only short-circuits within the refresh window; the
	// injectable clock lets us jump past it.
	first := fakeJWKSBlob()
	second := `{"keys":[{"kty":"RSA","kid":"k2","use":"sig","n":"other","e":"AQAB"}]}`
	var active = first
	mux := http.NewServeMux()
	var serverURL string
	mux.HandleFunc(OIDCDiscoveryPath, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":   serverURL,
			"jwks_uri": serverURL + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, active)
	})
	srv := httptest.NewServer(mux)
	serverURL = srv.URL
	defer srv.Close()

	ar := newAgentRuntimeWithOIDC(srv.URL)
	r := newOIDCReconciler(t, ar)
	r.OIDCHTTPClient = srv.Client()

	baseTime := time.Date(2026, time.April, 22, 0, 0, 0, 0, time.UTC)
	r.JWKSClock = func() time.Time { return baseTime }

	if _, err := r.reconcileOIDCJWKS(context.Background(), ar); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	active = second
	// Advance beyond the refresh interval so the fast-path lets us
	// through to re-fetch.
	r.JWKSClock = func() time.Time { return baseTime.Add(OIDCJWKSRefreshInterval + time.Minute) }
	if _, err := r.reconcileOIDCJWKS(context.Background(), ar); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	got := &corev1.Secret{}
	if err := r.Get(context.Background(),
		types.NamespacedName{Namespace: testAgentNamespace, Name: oidcJWKSSecretName("alpha")},
		got); err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if string(got.Data[OIDCJWKSDataKey]) != second {
		t.Errorf("second reconcile did not overwrite: got %q want %q",
			got.Data[OIDCJWKSDataKey], second)
	}
}

// TestReconcileOIDCJWKS_FreshCacheSkipsFetch proves T8: when the
// existing Secret's fetched-at annotation is within the refresh
// window, the reconciler does NOT perform an HTTP round-trip. Proven
// by pointing the reconciler at a server that would always fail — a
// fresh cache means the failure is never visited.
func TestReconcileOIDCJWKS_FreshCacheSkipsFetch(t *testing.T) {
	t.Parallel()
	calls := 0
	mux := http.NewServeMux()
	var serverURL string
	mux.HandleFunc(OIDCDiscoveryPath, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":   serverURL,
			"jwks_uri": serverURL + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(fakeJWKSBlob()))
	})
	srv := httptest.NewServer(mux)
	serverURL = srv.URL
	defer srv.Close()

	ar := newAgentRuntimeWithOIDC(srv.URL)
	r := newOIDCReconciler(t, ar)
	r.OIDCHTTPClient = srv.Client()
	baseTime := time.Date(2026, time.April, 22, 0, 0, 0, 0, time.UTC)
	r.JWKSClock = func() time.Time { return baseTime }

	// First reconcile populates the Secret (2 HTTP calls: discovery + jwks).
	if _, err := r.reconcileOIDCJWKS(context.Background(), ar); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	initialCalls := calls
	if initialCalls != 2 {
		t.Fatalf("first reconcile: got %d HTTP calls, want 2", initialCalls)
	}

	// Advance the clock by 5 minutes — well within the 6h refresh
	// window. A second reconcile must use the cache.
	r.JWKSClock = func() time.Time { return baseTime.Add(5 * time.Minute) }
	if _, err := r.reconcileOIDCJWKS(context.Background(), ar); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if calls != initialCalls {
		t.Errorf("second reconcile made %d additional HTTP calls — fresh-cache short-circuit failed",
			calls-initialCalls)
	}
}

func TestReconcileOIDCJWKS_DeleteMissingSecretIsNoop(t *testing.T) {
	t.Parallel()
	// OIDC disabled and no stale Secret to clean up — must not error.
	ar := newAgentRuntimeWithOIDC("")
	r := newOIDCReconciler(t, ar)

	if err := r.deleteOIDCJWKSSecretIfPresent(context.Background(), ar); err != nil {
		t.Fatalf("delete when absent should be noop, got err=%v", err)
	}
}

func TestOIDCJWKSSecretName(t *testing.T) {
	t.Parallel()
	got := oidcJWKSSecretName("myagent")
	want := "agent-myagent-oidc-jwks"
	if got != want {
		t.Errorf("oidcJWKSSecretName = %q, want %q", got, want)
	}
}

func TestScheduleOIDCJWKSRefresh(t *testing.T) {
	t.Parallel()
	if res := scheduleOIDCJWKSRefresh(0); res.RequeueAfter != 0 {
		t.Errorf("zero input: got RequeueAfter=%v, want 0", res.RequeueAfter)
	}
	if res := scheduleOIDCJWKSRefresh(OIDCJWKSRefreshInterval); res.RequeueAfter != OIDCJWKSRefreshInterval {
		t.Errorf("non-zero input: got RequeueAfter=%v, want %v",
			res.RequeueAfter, OIDCJWKSRefreshInterval)
	}
}

func TestOIDCHTTPClient_FallsBackToDefault(t *testing.T) {
	t.Parallel()
	r := &AgentRuntimeReconciler{}
	c := r.oidcHTTPClient()
	if c == nil {
		t.Fatal("expected non-nil default client")
	}
	if c.Timeout != OIDCJWKSHTTPTimeout {
		t.Errorf("default timeout = %v, want %v", c.Timeout, OIDCJWKSHTTPTimeout)
	}
}

// findCondition is shared across controller tests (see promptpack_skills_test.go).
