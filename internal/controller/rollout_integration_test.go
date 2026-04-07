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
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestReconcileRollout_Active_CreatesCandidateDeployment(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{SetWeight: ptr.To[int32](20)},
			{SetWeight: ptr.To[int32](50)},
		},
	}
	ar.Status.ActiveVersion = ptr.To("v1")

	pp := newTestPromptPack()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.reconcileRollout(context.Background(), ar, pp, nil, nil)
	require.NoError(t, err)

	// Candidate Deployment should exist.
	deploy := &appsv1.Deployment{}
	key := types.NamespacedName{
		Name:      candidateDeploymentName(ar.Name),
		Namespace: ar.Namespace,
	}
	err = fakeClient.Get(context.Background(), key, deploy)
	require.NoError(t, err, "candidate deployment should be created")
	assert.Equal(t, "canary", deploy.Spec.Template.Labels[labelOmniaTrack])

	// Status should be updated.
	require.NotNil(t, ar.Status.Rollout)
	assert.True(t, ar.Status.Rollout.Active)
	assert.Equal(t, "v1", ar.Status.Rollout.StableVersion)
	assert.Equal(t, "v2", ar.Status.Rollout.CandidateVersion)

	// No requeue needed for setWeight (non-pause).
	assert.Zero(t, result.RequeueAfter)

	// RolloutActive condition should be set.
	assertCondition(t, ar.Status.Conditions, ConditionTypeRolloutActive, metav1.ConditionTrue)
}

func TestReconcileRollout_Idle_DeletesCandidateDeployment(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := newRolloutTestAR()
	// No rollout config — idle.

	// Pre-existing candidate Deployment (leftover from previous rollout).
	existing := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      candidateDeploymentName(ar.Name),
			Namespace: ar.Namespace,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.reconcileRollout(context.Background(), ar, nil, nil, nil)
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)

	// Candidate Deployment should be deleted.
	deploy := &appsv1.Deployment{}
	key := types.NamespacedName{
		Name:      candidateDeploymentName(ar.Name),
		Namespace: ar.Namespace,
	}
	err = fakeClient.Get(context.Background(), key, deploy)
	assert.Error(t, err, "candidate deployment should be deleted")

	// Status rollout should be nil.
	assert.Nil(t, ar.Status.Rollout)

	// RolloutActive condition should be False.
	assertCondition(t, ar.Status.Conditions, ConditionTypeRolloutActive, metav1.ConditionFalse)
}

func TestReconcileRollout_Promotion(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{SetWeight: ptr.To[int32](50)},
		},
	}
	// Status at step 1 (past last step index 0) — triggers promotion.
	ar.Status.Rollout = &omniav1alpha1.RolloutStatus{
		Active:      true,
		CurrentStep: ptr.To[int32](1),
	}
	ar.Status.ActiveVersion = ptr.To("v1")

	pp := newTestPromptPack()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ar).
		WithStatusSubresource(ar).
		Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// First create the candidate deployment so promotion can delete it.
	_, err := r.reconcileCandidateDeployment(context.Background(), ar, pp, nil, nil)
	require.NoError(t, err)

	result, err := r.reconcileRollout(context.Background(), ar, pp, nil, nil)
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)

	// Spec should be promoted: version is now v2.
	assert.Equal(t, "v2", *ar.Spec.PromptPackRef.Version)

	// Rollout status should be inactive.
	require.NotNil(t, ar.Status.Rollout)
	assert.False(t, ar.Status.Rollout.Active)
	assert.Equal(t, "promoted", ar.Status.Rollout.Message)

	// Candidate deployment should be deleted.
	deploy := &appsv1.Deployment{}
	key := types.NamespacedName{
		Name:      candidateDeploymentName(ar.Name),
		Namespace: ar.Namespace,
	}
	err = fakeClient.Get(context.Background(), key, deploy)
	assert.Error(t, err, "candidate deployment should be deleted after promotion")

	// RolloutActive condition should be False.
	assertCondition(t, ar.Status.Conditions, ConditionTypeRolloutActive, metav1.ConditionFalse)
}

func TestReconcileRollout_PauseStep_RequeuesAfterDuration(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{Pause: &omniav1alpha1.RolloutPause{Duration: ptr.To("30s")}},
		},
	}
	ar.Status.ActiveVersion = ptr.To("v1")

	pp := newTestPromptPack()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ar).
		WithStatusSubresource(ar).
		Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.reconcileRollout(context.Background(), ar, pp, nil, nil)
	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "pause step should trigger requeue")
}

func TestReconcileRolloutIdle_NoCandidateDeployment(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := newRolloutTestAR()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Should not error even when no candidate deployment exists.
	result, err := r.reconcileRollout(context.Background(), ar, nil, nil, nil)
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)
	assert.Nil(t, ar.Status.Rollout)
}

func TestReconcileRollout_Promotion_PersistsSpec(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{SetWeight: ptr.To[int32](100)},
		},
	}
	ar.Status.Rollout = &omniav1alpha1.RolloutStatus{
		Active:      true,
		CurrentStep: ptr.To[int32](1),
	}
	ar.Status.ActiveVersion = ptr.To("v1")

	pp := newTestPromptPack()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ar).
		WithStatusSubresource(ar).
		Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Create candidate so promotion can delete it.
	_, err := r.reconcileCandidateDeployment(context.Background(), ar, pp, nil, nil)
	require.NoError(t, err)

	_, err = r.reconcileRollout(context.Background(), ar, pp, nil, nil)
	require.NoError(t, err)

	// Re-read from fake client to verify spec was persisted.
	persisted := &omniav1alpha1.AgentRuntime{}
	key := types.NamespacedName{Name: ar.Name, Namespace: ar.Namespace}
	require.NoError(t, fakeClient.Get(context.Background(), key, persisted))
	assert.Equal(t, "v2", *persisted.Spec.PromptPackRef.Version, "promoted version should be persisted")
}

func TestReconcileRollout_SetWeightZero_Advances(t *testing.T) {
	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{SetWeight: ptr.To[int32](0)},
			{SetWeight: ptr.To[int32](50)},
		},
	}

	result := reconcileRolloutSteps(ar)
	assert.True(t, result.active)
	assert.Equal(t, int32(0), result.currentStep)
	assert.Equal(t, int32(0), result.desiredWeight)
	// Verify the step is not blocked — reconcileRolloutUpdateStatus should advance.
	assert.False(t, result.paused)
	assert.False(t, result.analysis)
}

func TestCandidateDeployment_SelectorIncludesTrack(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](10)}},
	}

	pp := newTestPromptPack()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	deploy, err := r.reconcileCandidateDeployment(context.Background(), ar, pp, nil, nil)
	require.NoError(t, err)

	// Selector must include track=canary for disjoint pod ownership.
	assert.Equal(t, "canary", deploy.Spec.Selector.MatchLabels[labelOmniaTrack],
		"candidate selector must include track label")
	assert.Equal(t, "canary", deploy.Spec.Template.Labels[labelOmniaTrack],
		"candidate pod template must include track label")
}

func TestReconcileRollout_MetricsRecorded(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{SetWeight: ptr.To[int32](100)},
		},
	}
	ar.Status.Rollout = &omniav1alpha1.RolloutStatus{
		Active:      true,
		CurrentStep: ptr.To[int32](1),
	}
	ar.Status.ActiveVersion = ptr.To("v1")

	pp := newTestPromptPack()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ar).
		WithStatusSubresource(ar).
		Build()

	reg := prometheus.NewRegistry()
	metrics := NewRolloutMetrics(reg)

	r := &AgentRuntimeReconciler{
		Client:         fakeClient,
		Scheme:         scheme,
		RolloutMetrics: metrics,
	}

	// Create candidate so promotion can delete it.
	_, err := r.reconcileCandidateDeployment(context.Background(), ar, pp, nil, nil)
	require.NoError(t, err)

	_, err = r.reconcileRollout(context.Background(), ar, pp, nil, nil)
	require.NoError(t, err)

	// Verify promotion metric was incremented.
	m, err := reg.Gather()
	require.NoError(t, err)
	found := false
	for _, mf := range m {
		if mf.GetName() == metricRolloutPromotions {
			found = true
			assert.Greater(t, mf.GetMetric()[0].GetCounter().GetValue(), float64(0))
		}
	}
	assert.True(t, found, "promotion metric should be registered and incremented")
}

func TestReconcileRollout_AutoRollback_UnhealthyCandidate(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{SetWeight: ptr.To[int32](20)},
		},
		Rollback: &omniav1alpha1.RollbackConfig{
			Mode: omniav1alpha1.RollbackModeAutomatic,
		},
	}
	ar.Status.ActiveVersion = ptr.To("v1")

	pp := newTestPromptPack()

	// Pre-create a candidate Deployment with unhealthy status.
	candidateDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      candidateDeploymentName(ar.Name),
			Namespace: ar.Namespace,
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:       0,
			UnavailableReplicas: 1,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ar, candidateDeploy).
		WithStatusSubresource(ar).
		Build()

	reg := prometheus.NewRegistry()
	metrics := NewRolloutMetrics(reg)

	r := &AgentRuntimeReconciler{
		Client:         fakeClient,
		Scheme:         scheme,
		RolloutMetrics: metrics,
	}

	result, err := r.reconcileRollout(context.Background(), ar, pp, nil, nil)
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)

	// Rollout should be inactive after auto-rollback.
	require.NotNil(t, ar.Status.Rollout)
	assert.False(t, ar.Status.Rollout.Active)
	assert.Contains(t, ar.Status.Rollout.Message, "auto-rollback")

	// Candidate deployment should be deleted.
	deploy := &appsv1.Deployment{}
	key := types.NamespacedName{
		Name:      candidateDeploymentName(ar.Name),
		Namespace: ar.Namespace,
	}
	err = fakeClient.Get(context.Background(), key, deploy)
	assert.Error(t, err, "candidate deployment should be deleted after auto-rollback")

	// RolloutActive condition should be False.
	assertCondition(t, ar.Status.Conditions, ConditionTypeRolloutActive, metav1.ConditionFalse)

	// Rollback metric should be recorded.
	m, err := reg.Gather()
	require.NoError(t, err)
	found := false
	for _, mf := range m {
		if mf.GetName() == metricRolloutRollbacks {
			found = true
			assert.Greater(t, mf.GetMetric()[0].GetCounter().GetValue(), float64(0))
		}
	}
	assert.True(t, found, "rollback metric should be registered and incremented")
}

func TestReconcileRollout_AutoRollback_HealthyCandidate_Continues(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{SetWeight: ptr.To[int32](20)},
		},
		Rollback: &omniav1alpha1.RollbackConfig{
			Mode: omniav1alpha1.RollbackModeAutomatic,
		},
	}
	ar.Status.ActiveVersion = ptr.To("v1")

	pp := newTestPromptPack()

	// Pre-create a healthy candidate Deployment.
	candidateDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      candidateDeploymentName(ar.Name),
			Namespace: ar.Namespace,
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:       1,
			UnavailableReplicas: 0,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ar, candidateDeploy).
		WithStatusSubresource(ar).
		Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.reconcileRollout(context.Background(), ar, pp, nil, nil)
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)

	// Rollout should remain active — healthy candidate should not trigger rollback.
	require.NotNil(t, ar.Status.Rollout)
	assert.True(t, ar.Status.Rollout.Active)

	// Candidate deployment should still exist.
	deploy := &appsv1.Deployment{}
	key := types.NamespacedName{
		Name:      candidateDeploymentName(ar.Name),
		Namespace: ar.Namespace,
	}
	require.NoError(t, fakeClient.Get(context.Background(), key, deploy))
}

func TestReconcileRollout_Idle_WithMetrics(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := newRolloutTestAR()
	// No rollout config — idle.

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	reg := prometheus.NewRegistry()
	metrics := NewRolloutMetrics(reg)

	r := &AgentRuntimeReconciler{
		Client:         fakeClient,
		Scheme:         scheme,
		RolloutMetrics: metrics,
	}

	_, err := r.reconcileRollout(context.Background(), ar, nil, nil, nil)
	require.NoError(t, err)

	// Active metric should be 0 for idle.
	m, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range m {
		if mf.GetName() == metricRolloutActive {
			assert.Equal(t, float64(0), mf.GetMetric()[0].GetGauge().GetValue())
		}
	}
}

func TestReconcileRolloutUpdateStatus_NilActiveVersion(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{SetWeight: ptr.To[int32](20)},
		},
	}
	// ActiveVersion not set — stableVersion should be empty.
	ar.Status.ActiveVersion = nil

	pp := newTestPromptPack()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	_, err := r.reconcileRollout(context.Background(), ar, pp, nil, nil)
	require.NoError(t, err)

	require.NotNil(t, ar.Status.Rollout)
	assert.True(t, ar.Status.Rollout.Active)
	assert.Empty(t, ar.Status.Rollout.StableVersion)
	assert.Equal(t, "v2", ar.Status.Rollout.CandidateVersion)
}

func TestReconcileRollout_Active_WithIstioTrafficRouting(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "networking.istio.io", Version: "v1", Kind: "VirtualService"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "networking.istio.io", Version: "v1", Kind: "VirtualServiceList"},
		&unstructured.UnstructuredList{},
	)

	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{SetWeight: ptr.To[int32](30)},
		},
		TrafficRouting: &omniav1alpha1.TrafficRoutingConfig{
			Istio: &omniav1alpha1.IstioTrafficRouting{
				VirtualService: omniav1alpha1.IstioVirtualServiceRef{
					Name:   "my-vs",
					Routes: []string{"primary"},
				},
				DestinationRule: omniav1alpha1.IstioDestinationRuleRef{
					Name:            "my-dr",
					StableSubset:    "stable",
					CandidateSubset: "canary",
				},
			},
		},
	}
	ar.Status.ActiveVersion = ptr.To("v1")

	// Create the VirtualService.
	route := makeRoute("primary", 100, 0)
	vs := newTestVirtualService("my-vs", "default", []interface{}{route})

	pp := newTestPromptPack()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vs).
		Build()

	reg := prometheus.NewRegistry()
	metrics := NewRolloutMetrics(reg)

	r := &AgentRuntimeReconciler{
		Client:         fakeClient,
		Scheme:         scheme,
		RolloutMetrics: metrics,
	}

	_, err := r.reconcileRollout(context.Background(), ar, pp, nil, nil)
	require.NoError(t, err)

	// Verify VS weights were updated.
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
	assert.Equal(t, int64(70), stableDest["weight"])
	assert.Equal(t, int64(30), canaryDest["weight"])
}

func TestReconcileRollout_Idle_ResetsIstioTrafficRouting(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "networking.istio.io", Version: "v1", Kind: "VirtualService"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "networking.istio.io", Version: "v1", Kind: "VirtualServiceList"},
		&unstructured.UnstructuredList{},
	)

	ar := newRolloutTestAR()
	// Rollout with no candidate — idle, but traffic routing is configured.
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Steps: []omniav1alpha1.RolloutStep{
			{SetWeight: ptr.To[int32](30)},
		},
		TrafficRouting: &omniav1alpha1.TrafficRoutingConfig{
			Istio: &omniav1alpha1.IstioTrafficRouting{
				VirtualService: omniav1alpha1.IstioVirtualServiceRef{
					Name:   "my-vs",
					Routes: []string{"primary"},
				},
				DestinationRule: omniav1alpha1.IstioDestinationRuleRef{
					Name:            "my-dr",
					StableSubset:    "stable",
					CandidateSubset: "canary",
				},
			},
		},
	}

	// VS with canary weight still set from previous rollout.
	route := makeRoute("primary", 70, 30)
	vs := newTestVirtualService("my-vs", "default", []interface{}{route})

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vs).
		Build()

	reg := prometheus.NewRegistry()
	metrics := NewRolloutMetrics(reg)

	r := &AgentRuntimeReconciler{
		Client:         fakeClient,
		Scheme:         scheme,
		RolloutMetrics: metrics,
	}

	_, err := r.reconcileRollout(context.Background(), ar, nil, nil, nil)
	require.NoError(t, err)

	// VS should be reset to 100/0.
	updated := &unstructured.Unstructured{}
	updated.SetAPIVersion(istioNetworkingAPIVersion)
	updated.SetKind(istioVirtualServiceKind)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "my-vs", Namespace: "default"}, updated)
	require.NoError(t, err)

	httpRoutes, _, _ := unstructured.NestedSlice(updated.Object, "spec", "http")
	r0 := httpRoutes[0].(map[string]interface{})
	dests := r0["route"].([]interface{})
	assert.Equal(t, int64(100), dests[0].(map[string]interface{})["weight"])
	assert.Equal(t, int64(0), dests[1].(map[string]interface{})["weight"])
}

func TestReconcileRollout_StickySession_PatchesDR(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "networking.istio.io", Version: "v1", Kind: "VirtualService"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "networking.istio.io", Version: "v1", Kind: "VirtualServiceList"},
		&unstructured.UnstructuredList{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "networking.istio.io", Version: "v1", Kind: "DestinationRule"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "networking.istio.io", Version: "v1", Kind: "DestinationRuleList"},
		&unstructured.UnstructuredList{},
	)

	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{SetWeight: ptr.To[int32](30)},
		},
		StickySession: &omniav1alpha1.StickySessionConfig{HashOn: "x-user-id"},
		TrafficRouting: &omniav1alpha1.TrafficRoutingConfig{
			Istio: &omniav1alpha1.IstioTrafficRouting{
				VirtualService: omniav1alpha1.IstioVirtualServiceRef{
					Name:   "my-vs",
					Routes: []string{"primary"},
				},
				DestinationRule: omniav1alpha1.IstioDestinationRuleRef{
					Name:            "my-dr",
					StableSubset:    "stable",
					CandidateSubset: "canary",
				},
			},
		},
	}
	ar.Status.ActiveVersion = ptr.To("v1")

	route := makeRoute("primary", 100, 0)
	vs := newTestVirtualService("my-vs", "default", []interface{}{route})
	dr := newTestDestinationRule()
	pp := newTestPromptPack()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vs, dr).
		Build()

	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	_, err := r.reconcileRollout(context.Background(), ar, pp, nil, nil)
	require.NoError(t, err)

	// Verify DR was patched with the consistent hash header.
	updatedDR := &unstructured.Unstructured{}
	updatedDR.SetAPIVersion(istioNetworkingAPIVersion)
	updatedDR.SetKind(istioDestinationRuleKind)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "my-dr", Namespace: "default"}, updatedDR)
	require.NoError(t, err)

	val, found, err := unstructured.NestedString(updatedDR.Object,
		"spec", "trafficPolicy", "loadBalancer", "consistentHash", "httpHeaderName")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "x-user-id", val)
}

func TestReconcileRollout_Promotion_RemovesDRConsistentHash(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "networking.istio.io", Version: "v1", Kind: "VirtualService"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "networking.istio.io", Version: "v1", Kind: "VirtualServiceList"},
		&unstructured.UnstructuredList{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "networking.istio.io", Version: "v1", Kind: "DestinationRule"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "networking.istio.io", Version: "v1", Kind: "DestinationRuleList"},
		&unstructured.UnstructuredList{},
	)

	ar := newRolloutTestAR()
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackVersion: ptr.To("v2"),
		},
		Steps: []omniav1alpha1.RolloutStep{
			{SetWeight: ptr.To[int32](100)},
		},
		TrafficRouting: &omniav1alpha1.TrafficRoutingConfig{
			Istio: &omniav1alpha1.IstioTrafficRouting{
				VirtualService: omniav1alpha1.IstioVirtualServiceRef{
					Name:   "my-vs",
					Routes: []string{"primary"},
				},
				DestinationRule: omniav1alpha1.IstioDestinationRuleRef{
					Name:            "my-dr",
					StableSubset:    "stable",
					CandidateSubset: "canary",
				},
			},
		},
	}
	// At step 1 (past last step) — triggers promotion.
	ar.Status.Rollout = &omniav1alpha1.RolloutStatus{
		Active:      true,
		CurrentStep: ptr.To[int32](1),
	}
	ar.Status.ActiveVersion = ptr.To("v1")

	route := makeRoute("primary", 100, 0)
	vs := newTestVirtualService("my-vs", "default", []interface{}{route})

	// DR with a pre-existing consistentHash block.
	dr := newTestDestinationRule()
	dr.Object["spec"] = map[string]interface{}{
		"trafficPolicy": map[string]interface{}{
			"loadBalancer": map[string]interface{}{
				"consistentHash": map[string]interface{}{
					"httpHeaderName": "x-user-id",
				},
			},
		},
	}

	pp := newTestPromptPack()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ar, vs, dr).
		WithStatusSubresource(ar).
		Build()

	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	// Create candidate so promotion can delete it.
	_, err := r.reconcileCandidateDeployment(context.Background(), ar, pp, nil, nil)
	require.NoError(t, err)

	_, err = r.reconcileRollout(context.Background(), ar, pp, nil, nil)
	require.NoError(t, err)

	// Verify consistentHash block was removed after promotion.
	updatedDR := &unstructured.Unstructured{}
	updatedDR.SetAPIVersion(istioNetworkingAPIVersion)
	updatedDR.SetKind(istioDestinationRuleKind)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "my-dr", Namespace: "default"}, updatedDR)
	require.NoError(t, err)

	_, found, err := unstructured.NestedFieldNoCopy(updatedDR.Object,
		"spec", "trafficPolicy", "loadBalancer", "consistentHash")
	require.NoError(t, err)
	assert.False(t, found, "consistentHash should be removed after promotion")
}

// assertCondition checks that a condition with the given type and status exists.
func assertCondition(t *testing.T, conditions []metav1.Condition, condType string, status metav1.ConditionStatus) {
	t.Helper()
	for _, c := range conditions {
		if c.Type == condType {
			assert.Equal(t, status, c.Status, "condition %s status mismatch", condType)
			return
		}
	}
	t.Errorf("condition %s not found", condType)
}
