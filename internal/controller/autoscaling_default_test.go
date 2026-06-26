/*
Copyright 2025.

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

	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	testNS            = "ns"
	kindScaledObject  = "ScaledObject"
	kedaSchemaVersion = "v1alpha1"
	defaultGroup      = "default"
)

var errBoom = errors.New("boom")

func scaledObjectGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: kedaAPIGroup, Version: kedaSchemaVersion, Kind: kindScaledObject}
}

// workspaceWithGroupAutoscaling builds a Workspace bound to testNS whose default
// service group carries the given autoscaling default.
func workspaceWithGroupAutoscaling(as *omniav1alpha1.AutoscalingConfig) *omniav1alpha1.Workspace {
	return &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws"},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{Name: testNS},
			Services: []omniav1alpha1.WorkspaceServiceGroup{
				{Name: defaultSvcGroupName, Autoscaling: as},
			},
		},
	}
}

func agentWithServiceGroup(name, group string, as *omniav1alpha1.AutoscalingConfig) *omniav1alpha1.AgentRuntime {
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS},
		Spec:       omniav1alpha1.AgentRuntimeSpec{ServiceGroup: group},
	}
	if as != nil {
		ar.Spec.Runtime = &omniav1alpha1.RuntimeConfig{Autoscaling: as}
	}
	return ar
}

func TestResolveEffectiveAutoscaling(t *testing.T) {
	groupDefault := &omniav1alpha1.AutoscalingConfig{
		Enabled:     true,
		Type:        omniav1alpha1.AutoscalerTypeHPA,
		MinReplicas: ptr.To(int32(2)),
		MaxReplicas: ptr.To(int32(8)),
	}
	agentOwn := &omniav1alpha1.AutoscalingConfig{
		Enabled:     true,
		Type:        omniav1alpha1.AutoscalerTypeKEDA,
		MaxReplicas: ptr.To(int32(20)),
	}

	tests := []struct {
		name      string
		agent     *omniav1alpha1.AgentRuntime
		workspace *omniav1alpha1.Workspace
		want      *omniav1alpha1.AutoscalingConfig
	}{
		{
			name:      "agent's own policy wins over group default",
			agent:     agentWithServiceGroup("a", defaultGroup, agentOwn),
			workspace: workspaceWithGroupAutoscaling(groupDefault),
			want:      agentOwn,
		},
		{
			name:      "agent without policy inherits group default",
			agent:     agentWithServiceGroup("a", defaultGroup, nil),
			workspace: workspaceWithGroupAutoscaling(groupDefault),
			want:      groupDefault,
		},
		{
			name:      "empty serviceGroup resolves the default group",
			agent:     agentWithServiceGroup("a", "", nil),
			workspace: workspaceWithGroupAutoscaling(groupDefault),
			want:      groupDefault,
		},
		{
			name:      "group present but no autoscaling default returns nil",
			agent:     agentWithServiceGroup("a", defaultGroup, nil),
			workspace: workspaceWithGroupAutoscaling(nil),
			want:      nil,
		},
		{
			name:      "no workspace returns nil",
			agent:     agentWithServiceGroup("a", defaultGroup, nil),
			workspace: nil,
			want:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme(t)
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tt.workspace != nil {
				builder = builder.WithObjects(tt.workspace)
			}
			r := &AgentRuntimeReconciler{Client: builder.Build(), Scheme: scheme}

			got := r.resolveEffectiveAutoscaling(t.Context(), tt.agent)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestReconcileAutoscaling_DisabledReportsDisabled(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, autoscalingv2.AddToScheme(scheme))

	agent := agentWithServiceGroup("a", defaultGroup, nil)
	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	cond, err := r.reconcileAutoscaling(t.Context(), agent)
	require.NoError(t, err)
	require.Equal(t, ConditionTypeAutoscalingReady, cond.Type)
	require.Equal(t, metav1.ConditionTrue, cond.Status)
	require.Equal(t, reasonAutoscalingDisabled, cond.Reason)
}

func TestReconcileAutoscaling_InheritsGroupHPA(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, autoscalingv2.AddToScheme(scheme))

	groupDefault := &omniav1alpha1.AutoscalingConfig{
		Enabled:     true,
		Type:        omniav1alpha1.AutoscalerTypeHPA,
		MinReplicas: ptr.To(int32(3)),
		MaxReplicas: ptr.To(int32(9)),
	}
	agent := agentWithServiceGroup("inherits", defaultGroup, nil)
	ws := workspaceWithGroupAutoscaling(groupDefault)

	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws, agent).Build(),
		Scheme: scheme,
	}

	cond, err := r.reconcileAutoscaling(t.Context(), agent)
	require.NoError(t, err)
	require.Equal(t, metav1.ConditionTrue, cond.Status)
	require.Equal(t, reasonAutoscalingScaling, cond.Reason)

	// The inherited HPA must exist with the group's bounds.
	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	require.NoError(t, r.Get(t.Context(), types.NamespacedName{Name: "inherits", Namespace: testNS}, hpa))
	require.NotNil(t, hpa.Spec.MinReplicas)
	require.Equal(t, int32(3), *hpa.Spec.MinReplicas)
	require.Equal(t, int32(9), hpa.Spec.MaxReplicas)
}

func TestReconcileAutoscaling_AgentPolicyWinsOverGroup(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, autoscalingv2.AddToScheme(scheme))

	// Group default requests KEDA, but the agent brings its own HPA policy and
	// must win as a unit — an HPA (not a KEDA ScaledObject) is reconciled.
	groupDefault := &omniav1alpha1.AutoscalingConfig{Enabled: true, Type: omniav1alpha1.AutoscalerTypeKEDA}
	agentOwn := &omniav1alpha1.AutoscalingConfig{
		Enabled:     true,
		Type:        omniav1alpha1.AutoscalerTypeHPA,
		MaxReplicas: ptr.To(int32(5)),
	}
	agent := agentWithServiceGroup("own", defaultGroup, agentOwn)
	ws := workspaceWithGroupAutoscaling(groupDefault)

	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws, agent).Build(),
		Scheme: scheme,
	}

	cond, err := r.reconcileAutoscaling(t.Context(), agent)
	require.NoError(t, err)
	require.Equal(t, metav1.ConditionTrue, cond.Status)
	require.Equal(t, reasonAutoscalingScaling, cond.Reason)

	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	require.NoError(t, r.Get(t.Context(), types.NamespacedName{Name: "own", Namespace: testNS}, hpa))
	require.Equal(t, int32(5), hpa.Spec.MaxReplicas)
}

func TestReconcileAutoscaling_KEDANotInstalled(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, autoscalingv2.AddToScheme(scheme))

	// Simulate KEDA CRDs not installed: any Get of a ScaledObject returns a
	// NoKindMatchError, exactly as a RESTMapper would for an unregistered kind.
	noMatch := &meta.NoKindMatchError{
		GroupKind:        schema.GroupKind{Group: kedaAPIGroup, Kind: kindScaledObject},
		SearchedVersions: []string{kedaSchemaVersion},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if u, ok := obj.(*unstructured.Unstructured); ok && u.GetKind() == kindScaledObject {
					return noMatch
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).Build()

	agent := agentWithServiceGroup("keda-agent", defaultGroup,
		&omniav1alpha1.AutoscalingConfig{Enabled: true, Type: omniav1alpha1.AutoscalerTypeKEDA})

	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	cond, err := r.reconcileAutoscaling(t.Context(), agent)
	// KEDA-not-installed is surfaced via the condition, not a fatal error.
	require.NoError(t, err)
	require.Equal(t, metav1.ConditionFalse, cond.Status)
	require.Equal(t, reasonAutoscalingKEDAMissing, cond.Reason)
}

// schemeWithScaledObject registers the KEDA ScaledObject GVKs as unstructured
// so the fake client can create/get them (simulating KEDA installed).
func schemeWithScaledObject(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := newTestScheme(t)
	require.NoError(t, autoscalingv2.AddToScheme(scheme))
	scheme.AddKnownTypeWithName(scaledObjectGVK(), &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: kedaAPIGroup, Version: kedaSchemaVersion, Kind: kindScaledObject + "List"},
		&unstructured.UnstructuredList{},
	)
	return scheme
}

func TestReconcileAutoscaling_KEDACreatesScaledObject(t *testing.T) {
	scheme := schemeWithScaledObject(t)

	agent := agentWithServiceGroup("keda-ok", defaultGroup,
		&omniav1alpha1.AutoscalingConfig{
			Enabled:     true,
			Type:        omniav1alpha1.AutoscalerTypeKEDA,
			MaxReplicas: ptr.To(int32(7)),
		})

	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}

	cond, err := r.reconcileAutoscaling(t.Context(), agent)
	require.NoError(t, err)
	require.Equal(t, metav1.ConditionTrue, cond.Status)
	require.Equal(t, reasonAutoscalingScaling, cond.Reason)

	// The ScaledObject was created.
	so := &unstructured.Unstructured{}
	so.SetGroupVersionKind(scaledObjectGVK())
	require.NoError(t, r.Get(t.Context(), types.NamespacedName{Name: "keda-ok", Namespace: testNS}, so))
}

func TestReconcileAutoscaling_KEDAGenericErrorIsFatal(t *testing.T) {
	scheme := schemeWithScaledObject(t)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if u, ok := obj.(*unstructured.Unstructured); ok && u.GetKind() == kindScaledObject {
					return apierrors.NewServiceUnavailable("kube-apiserver is down")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).Build()

	agent := agentWithServiceGroup("keda-err", defaultGroup,
		&omniav1alpha1.AutoscalingConfig{Enabled: true, Type: omniav1alpha1.AutoscalerTypeKEDA})

	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	cond, err := r.reconcileAutoscaling(t.Context(), agent)
	// A generic (non-NoMatch) error IS surfaced as an error and reason Error.
	require.Error(t, err)
	require.Equal(t, metav1.ConditionFalse, cond.Status)
	require.Equal(t, reasonAutoscalingError, cond.Reason)
}

func TestReconcileHPA_DisabledDeletesExisting(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, autoscalingv2.AddToScheme(scheme))

	existing := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "gone", Namespace: testNS},
	}
	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build(),
		Scheme: scheme,
	}
	agent := &omniav1alpha1.AgentRuntime{ObjectMeta: metav1.ObjectMeta{Name: "gone", Namespace: testNS}}

	// Disabled config must delete the stale HPA.
	require.NoError(t, r.reconcileHPA(t.Context(), agent, &omniav1alpha1.AutoscalingConfig{Enabled: false}))

	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	err := r.Get(t.Context(), types.NamespacedName{Name: "gone", Namespace: testNS}, hpa)
	require.True(t, apierrors.IsNotFound(err))
}

func TestReconcileAutoscaling_DisabledCleanupErrorIsFatal(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, autoscalingv2.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.DeleteOption) error {
				return apierrors.NewServiceUnavailable("apiserver down")
			},
		}).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	// Disabled policy: cleanupHPA fails, so the condition reports Error.
	agent := agentWithServiceGroup("a", defaultGroup, &omniav1alpha1.AutoscalingConfig{Enabled: false})
	cond, err := r.reconcileAutoscaling(t.Context(), agent)
	require.Error(t, err)
	require.Equal(t, metav1.ConditionFalse, cond.Status)
	require.Equal(t, reasonAutoscalingError, cond.Reason)
}

func TestReconcileAutoscaling_HPAReconcileErrorIsFatal(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, autoscalingv2.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
				if _, ok := obj.(*autoscalingv2.HorizontalPodAutoscaler); ok {
					return apierrors.NewInternalError(errBoom)
				}
				return nil
			},
		}).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	agent := agentWithServiceGroup("hpa-fail", defaultGroup,
		&omniav1alpha1.AutoscalingConfig{Enabled: true, Type: omniav1alpha1.AutoscalerTypeHPA})
	cond, err := r.reconcileAutoscaling(t.Context(), agent)
	require.Error(t, err)
	require.Equal(t, metav1.ConditionFalse, cond.Status)
	require.Equal(t, reasonAutoscalingError, cond.Reason)
}

func TestReconcileAutoscaling_KEDACleanupHPAErrorIsFatal(t *testing.T) {
	scheme := schemeWithScaledObject(t)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.DeleteOption) error {
				if _, ok := obj.(*autoscalingv2.HorizontalPodAutoscaler); ok {
					return apierrors.NewServiceUnavailable("apiserver down")
				}
				return nil
			},
		}).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	// KEDA path first cleans up any HPA; that delete fails here.
	agent := agentWithServiceGroup("keda-cleanup-fail", defaultGroup,
		&omniav1alpha1.AutoscalingConfig{Enabled: true, Type: omniav1alpha1.AutoscalerTypeKEDA})
	cond, err := r.reconcileAutoscaling(t.Context(), agent)
	require.Error(t, err)
	require.Equal(t, metav1.ConditionFalse, cond.Status)
	require.Equal(t, reasonAutoscalingError, cond.Reason)
}

// Sanity: the KEDA NoMatch error we simulate is classified as a no-match so the
// reconcile path treats it as "KEDA not installed" rather than a generic error.
func TestKEDANoMatchClassification(t *testing.T) {
	noMatch := &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: kedaAPIGroup, Kind: kindScaledObject}}
	require.True(t, meta.IsNoMatchError(noMatch))
	require.False(t, meta.IsNoMatchError(apierrors.NewNotFound(schema.GroupResource{}, "x")))
}
