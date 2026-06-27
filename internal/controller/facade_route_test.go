/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	exGateway    = "omnia-agents"
	exGatewayNS  = "omnia-system"
	exBaseDomain = "agents.example.com"
	exNS         = "ws1"
	exRouteName  = "chat-facade"
)

func newGatewayTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(s))
	require.NoError(t, gatewayv1.Install(s))
	return s
}

func exposedAgent(name string, expose *omniav1alpha1.FacadeExposeConfig) *omniav1alpha1.AgentRuntime {
	return &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: exNS, UID: types.UID("uid-" + name)},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Facades: []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket, Expose: expose}},
		},
	}
}

func configuredReconciler(t *testing.T, objs ...client.Object) *AgentRuntimeReconciler {
	t.Helper()
	s := newGatewayTestScheme(t)
	return &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build(),
		Scheme: s,
		DefaultExposure: DefaultExposureConfig{
			BaseDomain: exBaseDomain, GatewayName: exGateway, GatewayNamespace: exGatewayNS,
		},
	}
}

func TestExposeDecision(t *testing.T) {
	on := &omniav1alpha1.FacadeExposeConfig{Enabled: true}
	cfg := DefaultExposureConfig{BaseDomain: exBaseDomain, GatewayName: exGateway}
	cases := []struct {
		name     string
		exposure DefaultExposureConfig
		agent    *omniav1alpha1.AgentRuntime
		want     bool
		wantHost string
	}{
		{"opted in → generated host", cfg, exposedAgent("a", on), true, "a.ws1." + exBaseDomain},
		{"host override", cfg, exposedAgent("a", &omniav1alpha1.FacadeExposeConfig{Enabled: true, Host: "custom.example.com"}), true, "custom.example.com"},
		{"not configured → no", DefaultExposureConfig{}, exposedAgent("a", on), false, ""},
		{"no expose block → no", cfg, exposedAgent("a", nil), false, ""},
		{"expose disabled → no", cfg, exposedAgent("a", &omniav1alpha1.FacadeExposeConfig{Enabled: false}), false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &AgentRuntimeReconciler{DefaultExposure: tc.exposure}
			got, host, _ := r.exposeDecision(tc.agent)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantHost, host)
		})
	}
}

func TestReconcileFacadeRoute_CreatesWhenOptedIn(t *testing.T) {
	agent := exposedAgent("chat", &omniav1alpha1.FacadeExposeConfig{Enabled: true})
	r := configuredReconciler(t, agent)
	require.NoError(t, r.reconcileFacadeRoute(context.Background(), agent))

	route := &gatewayv1.HTTPRoute{}
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Namespace: exNS, Name: exRouteName}, route))
	// host-based, owner-ref'd, backendRef → the agent's facade Service on 8080.
	require.Len(t, route.Spec.Hostnames, 1)
	assert.Equal(t, "chat.ws1."+exBaseDomain, string(route.Spec.Hostnames[0]))
	assert.Equal(t, exGateway, string(route.Spec.ParentRefs[0].Name))
	require.Len(t, route.Spec.Rules, 1)
	br := route.Spec.Rules[0].BackendRefs[0]
	assert.Equal(t, "chat", string(br.Name))
	require.NotNil(t, br.Port)
	assert.Equal(t, int32(8080), *br.Port)
	assert.True(t, metav1.IsControlledBy(route, agent))
	// root path, no rewrite (host-based → valid:true for #1559 discovery).
	assert.Equal(t, "/", *route.Spec.Rules[0].Matches[0].Path.Value)
	assert.Empty(t, route.Spec.Rules[0].Filters)
}

func TestReconcileFacadeRoute_DeletesWhenDisabled(t *testing.T) {
	agent := exposedAgent("chat", &omniav1alpha1.FacadeExposeConfig{Enabled: true})
	r := configuredReconciler(t, agent)
	require.NoError(t, r.reconcileFacadeRoute(context.Background(), agent))

	// Opt out and re-reconcile → the owned route is removed.
	agent.Spec.Facades[0].Expose.Enabled = false
	require.NoError(t, r.reconcileFacadeRoute(context.Background(), agent))
	err := r.Get(context.Background(), types.NamespacedName{Namespace: exNS, Name: exRouteName}, &gatewayv1.HTTPRoute{})
	assert.True(t, client.IgnoreNotFound(err) == nil && err != nil, "route should be gone")
}

func TestReconcileFacadeRoute_NoopWhenNotConfigured(t *testing.T) {
	agent := exposedAgent("chat", &omniav1alpha1.FacadeExposeConfig{Enabled: true})
	s := newGatewayTestScheme(t)
	r := &AgentRuntimeReconciler{Client: fake.NewClientBuilder().WithScheme(s).WithObjects(agent).Build(), Scheme: s}
	require.NoError(t, r.reconcileFacadeRoute(context.Background(), agent))
	err := r.Get(context.Background(), types.NamespacedName{Namespace: exNS, Name: exRouteName}, &gatewayv1.HTTPRoute{})
	assert.True(t, err != nil, "no Gateway configured → no route created")
}

func TestReconcileFacadeRoute_LeavesUnownedRoute(t *testing.T) {
	// A hand-written route of the same name (no owner ref) must not be adopted or deleted.
	manual := &gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: exRouteName, Namespace: exNS}}
	agent := exposedAgent("chat", &omniav1alpha1.FacadeExposeConfig{Enabled: false})
	r := configuredReconciler(t, agent, manual)
	require.NoError(t, r.reconcileFacadeRoute(context.Background(), agent))

	got := &gatewayv1.HTTPRoute{}
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Namespace: exNS, Name: exRouteName}, got))
	assert.Empty(t, got.OwnerReferences, "operator must not adopt a hand-written route")
}
