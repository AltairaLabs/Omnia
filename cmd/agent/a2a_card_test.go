/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	facadea2a "github.com/altairalabs/omnia/internal/facade/a2a"
)

const (
	testA2AExternalURL = "https://agents.example.com/demo/rag-hero/a2a"
	testA2ANamespace   = "demo"
	testA2AName        = "rag-hero"
)

// newA2ATestRuntime builds an AgentRuntime whose status.facade.endpoints carry
// the given endpoints, persisted via the fake client's status subresource.
func newA2ATestRuntime(t *testing.T, eps []omniav1alpha1.FacadeEndpoint) *omniav1alpha1.AgentRuntime {
	t.Helper()
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: testA2AName, Namespace: testA2ANamespace},
	}
	if eps != nil {
		ar.Status.Facade = &omniav1alpha1.FacadeStatus{Endpoints: eps}
	}
	return ar
}

func TestResolveA2AExternalURL_FromStatus(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	ar := newA2ATestRuntime(t, []omniav1alpha1.FacadeEndpoint{
		{
			Protocol: omniav1alpha1.FacadeProtocolA2A,
			URL:      testA2AExternalURL,
			Valid:    true,
		},
	})
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ar).
		WithStatusSubresource(ar).
		Build()
	// The fake client clears status on create unless we write it via the
	// status subresource explicitly.
	if err := c.Status().Update(context.Background(), ar); err != nil {
		t.Fatalf("status update: %v", err)
	}

	got := resolveA2AExternalURL(context.Background(), c, testA2ANamespace, testA2AName)
	if got != testA2AExternalURL {
		t.Errorf("resolveA2AExternalURL = %q, want %q", got, testA2AExternalURL)
	}
}

func TestResolveA2AExternalURL_NoEndpointsReturnsEmpty(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	// nil Status.Facade — the agent is reachable only in-cluster.
	ar := newA2ATestRuntime(t, nil)
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ar).
		WithStatusSubresource(ar).
		Build()

	if got := resolveA2AExternalURL(context.Background(), c, testA2ANamespace, testA2AName); got != "" {
		t.Errorf("resolveA2AExternalURL = %q, want \"\" (no endpoints)", got)
	}
}

func TestResolveA2AExternalURL_GetErrorReturnsEmpty(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)
	// No objects: Get returns NotFound, so the resolver must yield "".
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	if got := resolveA2AExternalURL(context.Background(), c, testA2ANamespace, testA2AName); got != "" {
		t.Errorf("resolveA2AExternalURL = %q, want \"\" (get error)", got)
	}
}

func TestResolveA2AExternalURL_IgnoresInvalidAndNonA2A(t *testing.T) {
	t.Parallel()
	scheme := newTestScheme(t)

	// Only a websocket endpoint and an INVALID a2a endpoint → no usable a2a URL.
	arNoValid := newA2ATestRuntime(t, []omniav1alpha1.FacadeEndpoint{
		{
			Protocol: omniav1alpha1.FacadeProtocolWebSocket,
			URL:      "wss://agents.example.com/demo/rag-hero/ws",
			Valid:    true,
		},
		{
			Protocol: omniav1alpha1.FacadeProtocolA2A,
			URL:      testA2AExternalURL,
			Valid:    false,
		},
	})
	cNoValid := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(arNoValid).
		WithStatusSubresource(arNoValid).
		Build()
	if err := cNoValid.Status().Update(context.Background(), arNoValid); err != nil {
		t.Fatalf("status update: %v", err)
	}
	if got := resolveA2AExternalURL(context.Background(), cNoValid, testA2ANamespace, testA2AName); got != "" {
		t.Errorf("resolveA2AExternalURL = %q, want \"\" (only invalid/non-a2a endpoints)", got)
	}

	// A valid a2a endpoint alongside a websocket entry → returns the a2a URL.
	arValid := newA2ATestRuntime(t, []omniav1alpha1.FacadeEndpoint{
		{
			Protocol: omniav1alpha1.FacadeProtocolWebSocket,
			URL:      "wss://agents.example.com/demo/rag-hero/ws",
			Valid:    true,
		},
		{
			Protocol: omniav1alpha1.FacadeProtocolA2A,
			URL:      testA2AExternalURL,
			Valid:    true,
		},
	})
	cValid := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(arValid).
		WithStatusSubresource(arValid).
		Build()
	if err := cValid.Status().Update(context.Background(), arValid); err != nil {
		t.Fatalf("status update: %v", err)
	}
	got := resolveA2AExternalURL(context.Background(), cValid, testA2ANamespace, testA2AName)
	if got != testA2AExternalURL {
		t.Errorf("resolveA2AExternalURL = %q, want %q (valid a2a endpoint)", got, testA2AExternalURL)
	}
}

// TestCRDCardProvider_InterfaceURLFnOverridesEndpoint exercises the card
// provider override path (internal/facade/a2a): a non-empty interfaceURLFn
// result replaces the in-cluster SupportedInterfaces URL, while an empty
// result keeps the in-cluster URL baked in at construction (#1576).
func TestCRDCardProvider_InterfaceURLFnOverridesEndpoint(t *testing.T) {
	t.Parallel()
	spec := &omniav1alpha1.AgentCardSpec{Name: "rag-hero"}

	// Override returns a non-empty external URL: the card reflects it.
	overridden := facadea2a.NewCRDCardProvider(spec, "http://x.ns.svc:8080").
		WithInterfaceURLFn(func() string { return "https://ext/a2a" })
	card, err := overridden.AgentCard(nil)
	if err != nil {
		t.Fatalf("AgentCard: %v", err)
	}
	if len(card.SupportedInterfaces) == 0 {
		t.Fatal("expected at least one supported interface")
	}
	if got, want := card.SupportedInterfaces[0].URL, "https://ext/a2a"; got != want {
		t.Errorf("interface URL = %q, want %q (external override)", got, want)
	}

	// Override returns "": the in-cluster URL (endpoint + "/a2a") is kept.
	fallback := facadea2a.NewCRDCardProvider(spec, "http://x.ns.svc:8080").
		WithInterfaceURLFn(func() string { return "" })
	fbCard, err := fallback.AgentCard(nil)
	if err != nil {
		t.Fatalf("AgentCard: %v", err)
	}
	if len(fbCard.SupportedInterfaces) == 0 {
		t.Fatal("expected at least one supported interface")
	}
	if got, want := fbCard.SupportedInterfaces[0].URL, "http://x.ns.svc:8080/a2a"; got != want {
		t.Errorf("interface URL = %q, want %q (in-cluster fallback)", got, want)
	}
}
