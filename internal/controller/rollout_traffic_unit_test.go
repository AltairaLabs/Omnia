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
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	testRolloutNamespace = "default"
	// testExternalVSName matches newTestIstioConfig's VirtualService reference.
	testExternalVSName = "my-vs"
)

// istioRESTMapper returns a RESTMapper that resolves the Istio networking GVKs,
// so meshAvailable() reports the mesh as usable in fake-client tests.
func istioRESTMapper() meta.RESTMapper {
	m := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Group: istioNetworkingGroup, Version: "v1"}})
	for _, kind := range []string{istioVirtualServiceKind, istioDestinationRuleKind} {
		m.Add(schema.GroupVersionKind{Group: istioNetworkingGroup, Version: "v1", Kind: kind}, meta.RESTScopeNamespace)
	}
	return m
}

// trafficTestAR returns an AgentRuntime with the given replicas + traffic mode.
func trafficTestAR(name string, replicas int32, mode string) *omniav1alpha1.AgentRuntime {
	return &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testRolloutNamespace, Generation: 1},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Runtime: &omniav1alpha1.RuntimeConfig{Replicas: ptr.To(replicas)},
			Rollout: &omniav1alpha1.RolloutConfig{
				TrafficRouting: &omniav1alpha1.TrafficRoutingConfig{Mode: mode},
			},
		},
	}
}

// minimalDeployment builds a Deployment with the given replicas for fake-client tests.
func minimalDeployment(name string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testRolloutNamespace},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To(replicas)},
	}
}

// --- canonicalReplicaTotal ---

func TestCanonicalReplicaTotal(t *testing.T) {
	cases := []struct {
		name string
		ar   *omniav1alpha1.AgentRuntime
		want int32
	}{
		{"nil-runtime", &omniav1alpha1.AgentRuntime{}, 1},
		{"nil-replicas", &omniav1alpha1.AgentRuntime{Spec: omniav1alpha1.AgentRuntimeSpec{Runtime: &omniav1alpha1.RuntimeConfig{}}}, 1},
		{"zero-replicas", trafficTestAR("a", 0, TrafficModeReplicaWeighted), 1},
		{"set-replicas", trafficTestAR("a", 4, TrafficModeReplicaWeighted), 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, canonicalReplicaTotal(tc.ar))
		})
	}
}

// --- replicaSplit clamp branch ---

func TestReplicaSplit_ClampOverTotal(t *testing.T) {
	// weight > 100 rounds candidate above total → clamped to total.
	cand, stable, delivered := replicaSplit(4, 150)
	assert.Equal(t, int32(4), cand)
	assert.Equal(t, int32(0), stable)
	assert.Equal(t, int32(100), delivered)
}

// --- scaleDeployment ---

func TestScaleDeployment_Scales(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	dep := minimalDeployment(testEvalAgentName, 3)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	require.NoError(t, r.scaleDeployment(context.Background(), testRolloutNamespace, testEvalAgentName, 1))

	got := &appsv1.Deployment{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: testEvalAgentName, Namespace: testRolloutNamespace}, got))
	assert.Equal(t, int32(1), *got.Spec.Replicas)
}

func TestScaleDeployment_NoOpAtTarget(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	dep := minimalDeployment(testEvalAgentName, 2)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	require.NoError(t, r.scaleDeployment(context.Background(), testRolloutNamespace, testEvalAgentName, 2))

	got := &appsv1.Deployment{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: testEvalAgentName, Namespace: testRolloutNamespace}, got))
	assert.Equal(t, int32(2), *got.Spec.Replicas)
}

func TestScaleDeployment_MissingTolerated(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	// Missing candidate Deployment is tolerated (no-op, no error).
	assert.NoError(t, r.scaleDeployment(context.Background(), testRolloutNamespace, "missing", 0))
}

func TestScaleDeployment_GetErrorPropagates(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("get boom")
			},
		}).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	assert.Error(t, r.scaleDeployment(context.Background(), testRolloutNamespace, testEvalAgentName, 1))
}

func TestScaleDeployment_UpdateErrorPropagates(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	dep := minimalDeployment(testEvalAgentName, 3)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
				return errors.New("update boom")
			},
		}).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	assert.Error(t, r.scaleDeployment(context.Background(), testRolloutNamespace, testEvalAgentName, 1))
}

func TestReconcileReplicaWeighting_CandidateScaleErrorPropagates(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	cand := minimalDeployment(candidateDeploymentName(testEvalAgentName), 0)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cand).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
				return errors.New("scale boom")
			},
		}).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	ar := trafficTestAR(testEvalAgentName, 4, TrafficModeReplicaWeighted)
	_, err := r.reconcileReplicaWeighting(context.Background(), ar, 50)
	assert.Error(t, err)
}

// --- reconcileReplicaWeighting ---

func TestReconcileReplicaWeighting_Scales(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	stable := minimalDeployment(testEvalAgentName, 4)
	cand := minimalDeployment(candidateDeploymentName(testEvalAgentName), 0)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stable, cand).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	ar := trafficTestAR(testEvalAgentName, 4, TrafficModeReplicaWeighted)
	delivered, err := r.reconcileReplicaWeighting(context.Background(), ar, 50)
	require.NoError(t, err)
	assert.Equal(t, int32(50), delivered)

	gotCand := &appsv1.Deployment{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: candidateDeploymentName(testEvalAgentName), Namespace: testRolloutNamespace}, gotCand))
	assert.Equal(t, int32(2), *gotCand.Spec.Replicas)
}

func TestReconcileReplicaWeighting_ApproximatedWeightLogs(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	stable := minimalDeployment(testEvalAgentName, 3)
	cand := minimalDeployment(candidateDeploymentName(testEvalAgentName), 0)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stable, cand).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	ar := trafficTestAR(testEvalAgentName, 3, TrafficModeReplicaWeighted)
	// split(3,50) → candidate 2 / delivered 67 (approximated, exercises the log path).
	delivered, err := r.reconcileReplicaWeighting(context.Background(), ar, 50)
	require.NoError(t, err)
	assert.Equal(t, int32(67), delivered)
}

// --- meshAvailable ---

func TestMeshAvailable(t *testing.T) {
	scheme := newTestSchemeWithIstioAll(t)

	// Chart flag off → unavailable regardless of CRDs.
	rOff := &AgentRuntimeReconciler{
		Client:      fake.NewClientBuilder().WithScheme(scheme).WithRESTMapper(istioRESTMapper()).Build(),
		Scheme:      scheme,
		MeshEnabled: false,
	}
	assert.False(t, rOff.meshAvailable(context.Background()))

	// Flag on + Istio CRDs served → available.
	rOn := &AgentRuntimeReconciler{
		Client:      fake.NewClientBuilder().WithScheme(scheme).WithRESTMapper(istioRESTMapper()).Build(),
		Scheme:      scheme,
		MeshEnabled: true,
	}
	assert.True(t, rOn.meshAvailable(context.Background()))

	// Flag on but Istio CRDs absent from the mapper → unavailable.
	emptyMapper := meta.NewDefaultRESTMapper(nil)
	rNoCRD := &AgentRuntimeReconciler{
		Client:      fake.NewClientBuilder().WithScheme(scheme).WithRESTMapper(emptyMapper).Build(),
		Scheme:      scheme,
		MeshEnabled: true,
	}
	assert.False(t, rNoCRD.meshAvailable(context.Background()))
}

// --- reconcileMeshRouting + upsertUnstructured ---

func TestReconcileMeshRouting_CreatesAndUpdates(t *testing.T) {
	scheme := newTestSchemeWithIstioAll(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	ar := trafficTestAR(testEvalAgentName, 1, TrafficModeMesh)
	// First reconcile creates the VS + DR.
	require.NoError(t, r.reconcileMeshRouting(context.Background(), ar, 30))

	vs := newMeshObj(istioVirtualServiceKind)
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Name: meshRoutingName(testEvalAgentName), Namespace: testRolloutNamespace}, vs))
	assert.NotEmpty(t, vs.GetOwnerReferences(), "VS must carry the AgentRuntime owner ref")
	assert.Equal(t, int64(30), canaryWeightFromVS(t, vs))

	dr := newMeshObj(istioDestinationRuleKind)
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Name: meshRoutingName(testEvalAgentName), Namespace: testRolloutNamespace}, dr))
	assert.NotEmpty(t, dr.GetOwnerReferences())

	// Second reconcile at a new weight updates the existing VS (covers update branch).
	require.NoError(t, r.reconcileMeshRouting(context.Background(), ar, 70))
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Name: meshRoutingName(testEvalAgentName), Namespace: testRolloutNamespace}, vs))
	assert.Equal(t, int64(70), canaryWeightFromVS(t, vs))
}

func TestUpsertUnstructured_NoMatchNoOp(t *testing.T) {
	scheme := newTestSchemeWithIstioAll(t)
	noMatch := errors.New(`no matches for kind "VirtualService" in version "networking.istio.io/v1"`)
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return noMatch
			},
		}).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	// CRDs absent (no-match on Get) → no-op, no error.
	assert.NoError(t, r.upsertUnstructured(context.Background(), newMeshObj(istioVirtualServiceKind)))
}

func TestUpsertUnstructured_GetErrorPropagates(t *testing.T) {
	scheme := newTestSchemeWithIstioAll(t)
	boom := errors.New("boom")
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return boom
			},
		}).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	assert.Error(t, r.upsertUnstructured(context.Background(), newMeshObj(istioVirtualServiceKind)))
}

func TestUpsertUnstructured_CreateErrorPropagates(t *testing.T) {
	scheme := newTestSchemeWithIstioAll(t)
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return errors.New("create boom")
			},
		}).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	obj := newMeshObj(istioVirtualServiceKind)
	obj.SetName("agent-rollout")
	obj.SetNamespace(testRolloutNamespace)
	assert.Error(t, r.upsertUnstructured(context.Background(), obj))
}

func TestUpsertUnstructured_UpdateErrorPropagates(t *testing.T) {
	scheme := newTestSchemeWithIstioAll(t)
	existing := newMeshObj(istioVirtualServiceKind)
	existing.SetName("agent-rollout")
	existing.SetNamespace(testRolloutNamespace)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
				return errors.New("update boom")
			},
		}).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	obj := newMeshObj(istioVirtualServiceKind)
	obj.SetName("agent-rollout")
	obj.SetNamespace(testRolloutNamespace)
	assert.Error(t, r.upsertUnstructured(context.Background(), obj))
}

// --- applyTrafficRouting mesh + replica branches ---

func TestApplyTrafficRouting_MeshBranch(t *testing.T) {
	scheme := newTestSchemeWithIstioAll(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithRESTMapper(istioRESTMapper()).Build()
	reg := prometheus.NewRegistry()
	r := &AgentRuntimeReconciler{
		Client:         c,
		Scheme:         scheme,
		MeshEnabled:    true,
		RolloutMetrics: NewRolloutMetrics(reg),
		Recorder:       record.NewFakeRecorder(10),
	}

	ar := trafficTestAR(testEvalAgentName, 1, TrafficModeMesh)
	require.NoError(t, r.applyTrafficRouting(context.Background(), ar, 40))

	require.NotNil(t, ar.Status.Rollout)
	assert.Equal(t, TrafficModeMesh, ar.Status.Rollout.TrafficRoutingMode)
	require.NotNil(t, ar.Status.Rollout.TrafficWeightEnforced)
	assert.True(t, *ar.Status.Rollout.TrafficWeightEnforced)
}

func TestApplyTrafficRouting_ReplicaBranch(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	stable := minimalDeployment(testEvalAgentName, 4)
	cand := minimalDeployment(candidateDeploymentName(testEvalAgentName), 0)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stable, cand).Build()
	reg := prometheus.NewRegistry()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme, RolloutMetrics: NewRolloutMetrics(reg)}

	ar := trafficTestAR(testEvalAgentName, 4, TrafficModeReplicaWeighted)
	require.NoError(t, r.applyTrafficRouting(context.Background(), ar, 50))

	require.NotNil(t, ar.Status.Rollout)
	assert.Equal(t, TrafficModeReplicaWeighted, ar.Status.Rollout.TrafficRoutingMode)
	require.NotNil(t, ar.Status.Rollout.TrafficWeightEnforced)
	assert.False(t, *ar.Status.Rollout.TrafficWeightEnforced)
}

// --- applyTrafficRouting external NotFound degrade ---

func TestApplyTrafficRouting_ExternalMissingVSDegrades(t *testing.T) {
	scheme := newTestSchemeWithIstioNetworking(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	reg := prometheus.NewRegistry()
	r := &AgentRuntimeReconciler{
		Client:         c,
		Scheme:         scheme,
		RolloutMetrics: NewRolloutMetrics(reg),
		Recorder:       recorder,
	}

	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: testEvalAgentName, Namespace: testRolloutNamespace, Generation: 1},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Rollout: &omniav1alpha1.RolloutConfig{
				TrafficRouting: &omniav1alpha1.TrafficRoutingConfig{
					Mode:  TrafficModeExternal,
					Istio: newTestIstioConfig(),
				},
			},
		},
	}

	require.NoError(t, r.applyTrafficRouting(context.Background(), ar, 25))

	// Missing referenced VS → degrade trio (status unenforced + condition + event).
	require.NotNil(t, ar.Status.Rollout)
	require.NotNil(t, ar.Status.Rollout.TrafficWeightEnforced)
	assert.False(t, *ar.Status.Rollout.TrafficWeightEnforced)
	cond := findCondition(ar.Status.Conditions, ConditionTypeTrafficRouting)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.NotEmpty(t, recorder.Events)
}

func TestApplyTrafficRouting_ExternalGetErrorPropagates(t *testing.T) {
	scheme := newTestSchemeWithIstioNetworking(t)
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("read conflict")
			},
		}).Build()
	reg := prometheus.NewRegistry()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme, RolloutMetrics: NewRolloutMetrics(reg)}

	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: testEvalAgentName, Namespace: testRolloutNamespace, Generation: 1},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Rollout: &omniav1alpha1.RolloutConfig{
				TrafficRouting: &omniav1alpha1.TrafficRoutingConfig{
					Mode:  TrafficModeExternal,
					Istio: newTestIstioConfig(),
				},
			},
		},
	}
	// A non-NotFound error (read conflict) must propagate, not degrade.
	assert.Error(t, r.applyTrafficRouting(context.Background(), ar, 20))
}

// --- resetTrafficRoutingForMode ---

func TestResetTrafficRoutingForMode_Mesh(t *testing.T) {
	scheme := newTestSchemeWithIstioAll(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithRESTMapper(istioRESTMapper()).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme, MeshEnabled: true}

	ar := trafficTestAR(testEvalAgentName, 1, TrafficModeMesh)
	require.NoError(t, r.resetTrafficRoutingForMode(context.Background(), ar))

	vs := newMeshObj(istioVirtualServiceKind)
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Name: meshRoutingName(testEvalAgentName), Namespace: testRolloutNamespace}, vs))
	assert.Equal(t, int64(0), canaryWeightFromVS(t, vs), "reset → 0% canary")
}

func TestResetTrafficRoutingForMode_External(t *testing.T) {
	scheme := newTestSchemeWithIstioNetworking(t)
	route := makeRoute("primary", 70, 30)
	vs := newTestVirtualService(testExternalVSName, testRolloutNamespace, []interface{}{route})
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vs).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: testEvalAgentName, Namespace: testRolloutNamespace, Generation: 1},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Rollout: &omniav1alpha1.RolloutConfig{
				TrafficRouting: &omniav1alpha1.TrafficRoutingConfig{
					Mode:  TrafficModeExternal,
					Istio: newTestIstioConfig(),
				},
			},
		},
	}
	require.NoError(t, r.resetTrafficRoutingForMode(context.Background(), ar))

	got := newMeshObj(istioVirtualServiceKind)
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: testExternalVSName, Namespace: testRolloutNamespace}, got))
	routes, _, _ := unstructured.NestedSlice(got.Object, "spec", "http")
	dests, _, _ := unstructured.NestedSlice(routes[0].(map[string]interface{}), fieldRoute)
	for _, d := range dests {
		dm := d.(map[string]interface{})
		subset, _, _ := unstructured.NestedString(dm, fieldDestination, fieldSubset)
		w, _, _ := unstructured.NestedInt64(dm, fieldWeight)
		if subset == trackCanary {
			assert.Equal(t, int64(0), w, "external reset → 0% canary")
		}
	}
}

func TestResetTrafficRoutingForMode_Replica(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	stable := minimalDeployment(testEvalAgentName, 4)
	cand := minimalDeployment(candidateDeploymentName(testEvalAgentName), 2)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stable, cand).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	ar := trafficTestAR(testEvalAgentName, 4, TrafficModeReplicaWeighted)
	require.NoError(t, r.resetTrafficRoutingForMode(context.Background(), ar))

	gotCand := &appsv1.Deployment{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: candidateDeploymentName(testEvalAgentName), Namespace: testRolloutNamespace}, gotCand))
	assert.Equal(t, int32(0), *gotCand.Spec.Replicas, "candidate scaled to 0 on reset")
}

// --- helpers ---

func newMeshObj(kind string) *unstructured.Unstructured {
	o := &unstructured.Unstructured{}
	o.SetAPIVersion(istioNetworkingAPIVersion)
	o.SetKind(kind)
	return o
}

// canaryWeightFromVS extracts the canary subset weight from an owned VirtualService.
func canaryWeightFromVS(t *testing.T, vs *unstructured.Unstructured) int64 {
	t.Helper()
	routes, _, _ := unstructured.NestedSlice(vs.Object, "spec", "http")
	require.NotEmpty(t, routes)
	dests, _, _ := unstructured.NestedSlice(routes[0].(map[string]interface{}), fieldRoute)
	for _, d := range dests {
		dm := d.(map[string]interface{})
		subset, _, _ := unstructured.NestedString(dm, fieldDestination, fieldSubset)
		if subset == trackCanary {
			w, _, _ := unstructured.NestedInt64(dm, fieldWeight)
			return w
		}
	}
	t.Fatal("canary subset not found in VS")
	return -1
}
