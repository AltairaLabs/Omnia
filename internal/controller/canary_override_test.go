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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func canaryTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	return scheme
}

// The candidate override CM must carry the candidate's provider refs (not
// stable's), under the json field name the runtime reads, owner-ref'd for GC.
func TestReconcileCanaryOverrideConfigMap_WritesCandidateProviders(t *testing.T) {
	scheme := canaryTestScheme(t)
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "rag"
	ar.Namespace = "default"
	ar.Spec.Providers = []omniav1alpha1.NamedProviderRef{
		{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "p-stable"}},
	}
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			ProviderRefs: []omniav1alpha1.NamedProviderRef{
				{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "p-candidate"}},
			},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ar).Build()
	r := &AgentRuntimeReconciler{Client: fc, Scheme: scheme}

	require.NoError(t, r.reconcileCanaryOverrideConfigMap(context.Background(), ar))

	cm := &corev1.ConfigMap{}
	require.NoError(t, fc.Get(context.Background(),
		types.NamespacedName{Name: "rag" + CanaryConfigMapSuffix, Namespace: "default"}, cm))

	// Contract: the JSON deserializes to the same shape the runtime reads
	// (field name "providerRefs") and carries the candidate provider.
	raw := cm.Data[CanaryOverrideFileName]
	assert.Contains(t, raw, "providerRefs")
	var ov canaryOverride
	require.NoError(t, json.Unmarshal([]byte(raw), &ov))
	require.Len(t, ov.ProviderRefs, 1)
	assert.Equal(t, "p-candidate", ov.ProviderRefs[0].ProviderRef.Name)

	// Owner-ref'd to the AgentRuntime for garbage collection.
	require.Len(t, cm.OwnerReferences, 1)
	assert.Equal(t, "rag", cm.OwnerReferences[0].Name)
}

// When the owner type isn't registered, SetControllerReference fails and the
// error propagates (rather than writing an un-GC'able CM).
func TestReconcileCanaryOverrideConfigMap_OwnerRefError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme)) // AgentRuntime deliberately absent
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "rag"
	ar.Namespace = "default"
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &AgentRuntimeReconciler{Client: fc, Scheme: scheme}

	require.Error(t, r.reconcileCanaryOverrideConfigMap(context.Background(), ar))
}

// mountCanaryOverride adds the override volume to the pod and the mount to
// every container, at the path the runtime reads.
func TestMountCanaryOverride_AddsVolumeAndMounts(t *testing.T) {
	dep := &appsv1.Deployment{}
	dep.Spec.Template.Spec.Containers = []corev1.Container{
		{Name: RuntimeContainerName},
		{Name: FacadeContainerName},
	}

	mountCanaryOverride(dep, "rag")

	require.Len(t, dep.Spec.Template.Spec.Volumes, 1)
	vol := dep.Spec.Template.Spec.Volumes[0]
	assert.Equal(t, canaryOverrideVolumeName, vol.Name)
	require.NotNil(t, vol.ConfigMap)
	assert.Equal(t, "rag"+CanaryConfigMapSuffix, vol.ConfigMap.Name)

	for _, c := range dep.Spec.Template.Spec.Containers {
		require.Len(t, c.VolumeMounts, 1, "container %s", c.Name)
		assert.Equal(t, CanaryOverrideMountPath, c.VolumeMounts[0].MountPath)
		assert.True(t, c.VolumeMounts[0].ReadOnly)
	}
}
