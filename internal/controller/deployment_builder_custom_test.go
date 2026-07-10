/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// customFacadeImage is the third-party image a type:custom facade runs in place
// of the built-in agent binary.
const customFacadeImage = "ghcr.io/thirdparty/custom-facade:v1"

// customFacadeAR builds an agent whose only facade is type:custom. This is the
// bring-your-own-container case (#1768): the operator runs the supplied image as
// the facade container, wired to the runtime sidecar like a built-in agent-mode
// facade.
func customFacadeAR() *omniav1alpha1.AgentRuntime {
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "custom-agent"
	ar.Namespace = "default"
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{
		{Type: omniav1alpha1.FacadeTypeCustom, Image: customFacadeImage},
	}
	ar.Spec.PromptPackRef.Name = "test-pack"
	return ar
}

// containerByName returns the named container from a pod spec, or nil.
func containerByName(cs []corev1.Container, name string) *corev1.Container {
	for i := range cs {
		if cs[i].Name == name {
			return &cs[i]
		}
	}
	return nil
}

// TestPrimaryFacade_RecognizesCustom asserts a custom facade is selected as the
// primary surface (so image/port/env/management/exposure all key off it). Before
// #1768 primaryFacade only knew websocket/rest/a2a and returned nil for a
// custom-only agent, silently defaulting the facade image and disabling the
// management plane.
func TestPrimaryFacade_RecognizesCustom(t *testing.T) {
	f := primaryFacade(customFacadeAR())
	require.NotNil(t, f, "a custom-only agent must have a primary facade")
	assert.Equal(t, omniav1alpha1.FacadeTypeCustom, f.Type)
	assert.Equal(t, customFacadeImage, f.Image)
}

// TestBuildDeploymentSpec_CustomFacade_PodAssembly is the core wiring test:
// (a) the custom image is the facade container, (b) the runtime sidecar is
// present, (c) the facade container gets the runtime gRPC address, JWKS URL, and
// downward-API identity env.
func TestBuildDeploymentSpec_CustomFacade_PodAssembly(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	const jwksURL = "https://dashboard.omnia-system.svc/.well-known/jwks.json"
	r := &AgentRuntimeReconciler{
		Scheme:           scheme,
		Client:           fake.NewClientBuilder().WithScheme(scheme).Build(),
		MgmtPlaneJWKSURL: jwksURL,
	}

	dep := &appsv1.Deployment{}
	r.buildDeploymentSpec(context.Background(), dep, customFacadeAR(), newTestPromptPack(), nil, "", nil)

	containers := dep.Spec.Template.Spec.Containers

	// (a) the third-party image runs as the facade container.
	facade := containerByName(containers, FacadeContainerName)
	require.NotNil(t, facade, "facade container must be present")
	assert.Equal(t, customFacadeImage, facade.Image,
		"custom facade must run the third-party image, not the built-in default")

	// (b) the runtime sidecar sits next to it (custom is an agent-mode surface,
	// not standalone-a2a — it dispatches to the runtime over gRPC).
	require.NotNil(t, containerByName(containers, RuntimeContainerName),
		"runtime sidecar must be present alongside a custom facade")

	// (c) wiring env injected into the facade container.
	env := envMap(facade.Env)
	assert.Equal(t, fmt.Sprintf("localhost:%d", DefaultRuntimeGRPCPort), env["OMNIA_RUNTIME_ADDRESS"],
		"facade must be pointed at the runtime sidecar gRPC address")
	assert.Equal(t, jwksURL, env[EnvMgmtPlaneJWKSURL],
		"facade must receive the mgmt-plane JWKS URL")
	if _, ok := env[envOmniaAgentName]; !ok {
		t.Errorf("facade env missing %s (agent identity via downward API)", envOmniaAgentName)
	}
	if _, ok := env[envOmniaNamespace]; !ok {
		t.Errorf("facade env missing %s (namespace via downward API)", envOmniaNamespace)
	}
}

// TestBuildDeploymentSpec_CustomFacade_ImageRequired confirms the custom image
// always wins over the operator-default facade image.
func TestBuildDeploymentSpec_CustomFacade_ImageOverridesOperatorDefault(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	r := &AgentRuntimeReconciler{
		Scheme:      scheme,
		Client:      fake.NewClientBuilder().WithScheme(scheme).Build(),
		FacadeImage: "ghcr.io/altairalabs/omnia-facade:operator-default",
	}
	dep := &appsv1.Deployment{}
	r.buildDeploymentSpec(context.Background(), dep, customFacadeAR(), newTestPromptPack(), nil, "", nil)

	facade := containerByName(dep.Spec.Template.Spec.Containers, FacadeContainerName)
	require.NotNil(t, facade)
	assert.Equal(t, customFacadeImage, facade.Image,
		"the custom image must override the operator-default facade image")
}

// TestCustomFacade_ManagementPlane covers (e): a custom facade with the default
// (enabled) management plane gets the facade-mgmt twin Service port and is
// advertised on status.managementEndpoints; disabling it drops both.
func TestCustomFacade_ManagementPlane(t *testing.T) {
	t.Run("enabled by default → facade-mgmt port + status", func(t *testing.T) {
		ar := customFacadeAR()

		ports := appendManagementServicePorts(nil, ar)
		got := portNames(ports)
		assert.Equal(t, int32(DefaultInternalFacadePort), got[portNameFacadeMgmt],
			"custom facade must get the facade-mgmt twin Service port")

		me := managementEndpointsStatus(ar)
		require.NotNil(t, me, "custom facade must advertise a management endpoint")
		require.NotNil(t, me.WS)
		assert.Equal(t, int32(DefaultInternalFacadePort), *me.WS)
	})

	t.Run("managementPlane=false → no twin port, no status", func(t *testing.T) {
		ar := customFacadeAR()
		off := false
		ar.Spec.Facades[0].ManagementPlane = &off

		assert.Empty(t, appendManagementServicePorts(nil, ar),
			"an external-only custom facade must expose no mgmt twin port")
		assert.Nil(t, managementEndpointsStatus(ar),
			"an external-only custom facade must not advertise a management endpoint")
	})
}

// TestCustomFacade_ExposeHTTPRoute covers (d): a custom facade with
// expose.enabled produces an external HTTPRoute pointing the Gateway at the
// agent's facade Service on the facade port.
func TestCustomFacade_ExposeHTTPRoute(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "chat"
	ar.Namespace = exNS
	ar.UID = types.UID("uid-custom")
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{
		Type:   omniav1alpha1.FacadeTypeCustom,
		Image:  customFacadeImage,
		Expose: &omniav1alpha1.FacadeExposeConfig{Enabled: true},
	}}

	r := configuredReconciler(t, ar)
	require.NoError(t, r.reconcileFacadeRoute(context.Background(), ar))

	route := &gatewayv1.HTTPRoute{}
	require.NoError(t, r.Get(context.Background(),
		types.NamespacedName{Namespace: exNS, Name: exRouteName}, route))

	require.Len(t, route.Spec.Hostnames, 1)
	assert.Equal(t, "chat."+exNS+"."+exBaseDomain, string(route.Spec.Hostnames[0]))
	require.Len(t, route.Spec.Rules, 1)
	br := route.Spec.Rules[0].BackendRefs[0]
	assert.Equal(t, "chat", string(br.Name))
	require.NotNil(t, br.Port)
	assert.Equal(t, int32(DefaultFacadePort), *br.Port,
		"custom facade route must target the facade port")
}
