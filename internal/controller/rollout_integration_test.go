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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
