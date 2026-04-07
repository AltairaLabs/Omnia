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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func newTestSchemeWithIstioNetworking(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "networking.istio.io", Version: "v1", Kind: "VirtualService"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "networking.istio.io", Version: "v1", Kind: "VirtualServiceList"},
		&unstructured.UnstructuredList{},
	)
	return scheme
}

func newTestIstioConfig() *omniav1alpha1.IstioTrafficRouting {
	return &omniav1alpha1.IstioTrafficRouting{
		VirtualService: omniav1alpha1.IstioVirtualServiceRef{
			Name:   "my-vs",
			Routes: []string{"primary"},
		},
		DestinationRule: omniav1alpha1.IstioDestinationRuleRef{
			Name:            "my-dr",
			StableSubset:    "stable",
			CandidateSubset: "canary",
		},
	}
}

func newTestVirtualService(name, namespace string, routes []interface{}) *unstructured.Unstructured {
	vs := &unstructured.Unstructured{}
	vs.SetAPIVersion(istioNetworkingAPIVersion)
	vs.SetKind(istioVirtualServiceKind)
	vs.SetName(name)
	vs.SetNamespace(namespace)
	vs.Object["spec"] = map[string]interface{}{
		"http": routes,
	}
	return vs
}

func makeRoute(name string, stableWeight, canaryWeight int64) map[string]interface{} {
	return map[string]interface{}{
		"name": name,
		"route": []interface{}{
			map[string]interface{}{
				"destination": map[string]interface{}{
					"host":   "my-svc",
					"subset": "stable",
				},
				"weight": stableWeight,
			},
			map[string]interface{}{
				"destination": map[string]interface{}{
					"host":   "my-svc",
					"subset": "canary",
				},
				"weight": canaryWeight,
			},
		},
	}
}

func TestPatchVirtualServiceWeights(t *testing.T) {
	scheme := newTestSchemeWithIstioNetworking(t)
	route := makeRoute("primary", 100, 0)
	vs := newTestVirtualService("my-vs", "default", []interface{}{route})

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vs).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	istioConfig := newTestIstioConfig()
	err := r.patchVirtualServiceWeights(context.Background(), "default", istioConfig, 20)
	require.NoError(t, err)

	// Verify weights were updated
	updated := &unstructured.Unstructured{}
	updated.SetAPIVersion(istioNetworkingAPIVersion)
	updated.SetKind(istioVirtualServiceKind)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "my-vs", Namespace: "default"}, updated)
	require.NoError(t, err)

	httpRoutes, found, err := unstructured.NestedSlice(updated.Object, "spec", "http")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, httpRoutes, 1)

	r0 := httpRoutes[0].(map[string]interface{})
	dests := r0["route"].([]interface{})
	stableDest := dests[0].(map[string]interface{})
	canaryDest := dests[1].(map[string]interface{})
	assert.Equal(t, int64(80), stableDest["weight"])
	assert.Equal(t, int64(20), canaryDest["weight"])
}

func TestPatchVirtualServiceWeights_NoCRD(t *testing.T) {
	// Simulate Istio CRDs not installed by returning a "no matches for kind" error.
	scheme := newTestSchemeWithIstioNetworking(t)
	noMatchErr := fmt.Errorf("no matches for kind %q in version %q", "VirtualService", "networking.istio.io/v1")
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				return noMatchErr
			},
		}).
		Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	istioConfig := newTestIstioConfig()
	err := r.patchVirtualServiceWeights(context.Background(), "default", istioConfig, 20)
	assert.NoError(t, err) // graceful no-op
}

func TestResetVirtualServiceWeights(t *testing.T) {
	scheme := newTestSchemeWithIstioNetworking(t)
	route := makeRoute("primary", 50, 50)
	vs := newTestVirtualService("my-vs", "default", []interface{}{route})

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vs).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	istioConfig := newTestIstioConfig()
	err := r.resetTrafficRouting(context.Background(), "default", istioConfig)
	require.NoError(t, err)

	// Verify weights were reset to 100/0
	updated := &unstructured.Unstructured{}
	updated.SetAPIVersion(istioNetworkingAPIVersion)
	updated.SetKind(istioVirtualServiceKind)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "my-vs", Namespace: "default"}, updated)
	require.NoError(t, err)

	httpRoutes, found, err := unstructured.NestedSlice(updated.Object, "spec", "http")
	require.NoError(t, err)
	require.True(t, found)

	r0 := httpRoutes[0].(map[string]interface{})
	dests := r0["route"].([]interface{})
	stableDest := dests[0].(map[string]interface{})
	canaryDest := dests[1].(map[string]interface{})
	assert.Equal(t, int64(100), stableDest["weight"])
	assert.Equal(t, int64(0), canaryDest["weight"])
}

func TestPatchVirtualServiceWeights_MultipleRoutes(t *testing.T) {
	scheme := newTestSchemeWithIstioNetworking(t)
	targetRoute := makeRoute("primary", 100, 0)
	otherRoute := makeRoute("secondary", 100, 0)
	vs := newTestVirtualService("my-vs", "default", []interface{}{targetRoute, otherRoute})

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vs).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	// Only "primary" is in the targets list.
	istioConfig := newTestIstioConfig()
	err := r.patchVirtualServiceWeights(context.Background(), "default", istioConfig, 30)
	require.NoError(t, err)

	updated := &unstructured.Unstructured{}
	updated.SetAPIVersion(istioNetworkingAPIVersion)
	updated.SetKind(istioVirtualServiceKind)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "my-vs", Namespace: "default"}, updated)
	require.NoError(t, err)

	httpRoutes, found, err := unstructured.NestedSlice(updated.Object, "spec", "http")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, httpRoutes, 2)

	// Primary route should be patched
	primary := httpRoutes[0].(map[string]interface{})
	primaryDests := primary["route"].([]interface{})
	assert.Equal(t, int64(70), primaryDests[0].(map[string]interface{})["weight"])
	assert.Equal(t, int64(30), primaryDests[1].(map[string]interface{})["weight"])

	// Secondary route should be untouched
	secondary := httpRoutes[1].(map[string]interface{})
	secondaryDests := secondary["route"].([]interface{})
	assert.Equal(t, int64(100), secondaryDests[0].(map[string]interface{})["weight"])
	assert.Equal(t, int64(0), secondaryDests[1].(map[string]interface{})["weight"])
}

func TestIsTargetRoute(t *testing.T) {
	assert.True(t, isTargetRoute("primary", []string{"primary", "secondary"}))
	assert.False(t, isTargetRoute("other", []string{"primary", "secondary"}))
	assert.False(t, isTargetRoute("", []string{"primary"}))
}

func TestPatchRouteWeights(t *testing.T) {
	route := makeRoute("test", 100, 0)
	patchRouteWeights(route, "stable", "canary", 60, 40)

	dests := route["route"].([]interface{})
	assert.Equal(t, int64(60), dests[0].(map[string]interface{})["weight"])
	assert.Equal(t, int64(40), dests[1].(map[string]interface{})["weight"])
}

func TestPatchRouteWeights_NoMatchingSubset(t *testing.T) {
	route := map[string]interface{}{
		"name": "test",
		"route": []interface{}{
			map[string]interface{}{
				"destination": map[string]interface{}{
					"host":   "my-svc",
					"subset": "blue",
				},
				"weight": int64(100),
			},
		},
	}
	patchRouteWeights(route, "stable", "canary", 80, 20)

	// Weight should be unchanged — no matching subsets.
	dests := route["route"].([]interface{})
	assert.Equal(t, int64(100), dests[0].(map[string]interface{})["weight"])
}

func TestPatchRouteWeights_NoRouteKey(t *testing.T) {
	route := map[string]interface{}{
		"name": "test",
	}
	// Should not panic.
	patchRouteWeights(route, "stable", "canary", 80, 20)
}

func TestPatchRouteWeights_InvalidRouteType(t *testing.T) {
	route := map[string]interface{}{
		"name":  "test",
		"route": "not-a-slice",
	}
	// Should not panic — route value is not []interface{}.
	patchRouteWeights(route, "stable", "canary", 80, 20)
}

func TestPatchRouteWeights_InvalidDestinationType(t *testing.T) {
	route := map[string]interface{}{
		"name": "test",
		"route": []interface{}{
			"not-a-map",
		},
	}
	// Should not panic — dest entry is not map[string]interface{}.
	patchRouteWeights(route, "stable", "canary", 80, 20)
}

func TestPatchVirtualServiceWeights_NoSpecHTTP(t *testing.T) {
	scheme := newTestSchemeWithIstioNetworking(t)

	// VirtualService with no spec.http field.
	vs := &unstructured.Unstructured{}
	vs.SetAPIVersion(istioNetworkingAPIVersion)
	vs.SetKind(istioVirtualServiceKind)
	vs.SetName("my-vs")
	vs.SetNamespace("default")
	vs.Object["spec"] = map[string]interface{}{}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vs).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	istioConfig := newTestIstioConfig()
	err := r.patchVirtualServiceWeights(context.Background(), "default", istioConfig, 20)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no spec.http routes")
}

func TestPatchVirtualServiceWeights_GetError(t *testing.T) {
	scheme := newTestSchemeWithIstioNetworking(t)
	// No VS object — Get will return NotFound.
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	istioConfig := newTestIstioConfig()
	err := r.patchVirtualServiceWeights(context.Background(), "default", istioConfig, 20)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get VirtualService")
}

func TestPatchVirtualServiceWeights_UpdateError(t *testing.T) {
	scheme := newTestSchemeWithIstioNetworking(t)
	route := makeRoute("primary", 100, 0)
	vs := newTestVirtualService("my-vs", "default", []interface{}{route})

	updateErr := fmt.Errorf("update conflict")
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vs).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				return updateErr
			},
		}).
		Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	istioConfig := newTestIstioConfig()
	err := r.patchVirtualServiceWeights(context.Background(), "default", istioConfig, 20)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "update VirtualService")
}

func TestPatchVirtualServiceWeights_NonTargetRouteSkipped(t *testing.T) {
	scheme := newTestSchemeWithIstioNetworking(t)
	// Route with a non-matching name — should be skipped.
	route := map[string]interface{}{
		"name": "other-route",
		"route": []interface{}{
			map[string]interface{}{
				"destination": map[string]interface{}{
					"host":   "my-svc",
					"subset": "stable",
				},
				"weight": int64(100),
			},
		},
	}
	vs := newTestVirtualService("my-vs", "default", []interface{}{route})

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vs).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	istioConfig := newTestIstioConfig()
	err := r.patchVirtualServiceWeights(context.Background(), "default", istioConfig, 30)
	require.NoError(t, err)

	// Verify weights were NOT changed on the non-target route.
	updated := &unstructured.Unstructured{}
	updated.SetAPIVersion(istioNetworkingAPIVersion)
	updated.SetKind(istioVirtualServiceKind)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "my-vs", Namespace: "default"}, updated)
	require.NoError(t, err)

	httpRoutes, _, _ := unstructured.NestedSlice(updated.Object, "spec", "http")
	r0 := httpRoutes[0].(map[string]interface{})
	dests := r0["route"].([]interface{})
	assert.Equal(t, int64(100), dests[0].(map[string]interface{})["weight"])
}

func TestPatchVirtualServiceWeights_InvalidRouteEntry(t *testing.T) {
	scheme := newTestSchemeWithIstioNetworking(t)
	// Route entry that is not a map — should be skipped without error.
	vs := newTestVirtualService("my-vs", "default", []interface{}{
		"not-a-map",
		makeRoute("primary", 100, 0),
	})

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vs).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	istioConfig := newTestIstioConfig()
	err := r.patchVirtualServiceWeights(context.Background(), "default", istioConfig, 25)
	require.NoError(t, err)
}

func TestIsTargetRoute_EmptyTargets(t *testing.T) {
	assert.False(t, isTargetRoute("primary", nil))
	assert.False(t, isTargetRoute("primary", []string{}))
}
