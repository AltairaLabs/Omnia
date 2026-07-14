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
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// The rollout Event helpers are nil-safe (no recorder in some tests) and emit
// the expected type+reason when a recorder is present.
func TestRecordRolloutEventHelpers(t *testing.T) {
	ar := newRolloutTestAR()

	// nil recorder → no panic.
	(&AgentRuntimeReconciler{}).recordRolloutNormal(ar, eventReasonPromoted, "m")
	(&AgentRuntimeReconciler{}).recordRolloutWarning(ar, eventReasonRolledBack, "m")

	rec := record.NewFakeRecorder(5)
	r := &AgentRuntimeReconciler{Recorder: rec}
	r.recordRolloutNormal(ar, eventReasonPromoted, "promoted")
	r.recordRolloutWarning(ar, eventReasonRolledBack, "rolled back")

	var got []string
	for drained := false; !drained; {
		select {
		case e := <-rec.Events:
			got = append(got, e)
		default:
			drained = true
		}
	}
	require.Len(t, got, 2)
	assert.Contains(t, got[0], eventReasonPromoted)
	assert.Contains(t, got[0], "Normal")
	assert.Contains(t, got[1], eventReasonRolledBack)
	assert.Contains(t, got[1], "Warning")
}

func TestDeploymentRolloutComplete(t *testing.T) {
	mk := func(gen, observed int64, want, updated, avail, total int32) *appsv1.Deployment {
		d := &appsv1.Deployment{}
		d.Generation = gen
		d.Spec.Replicas = ptr.To(want)
		d.Status.ObservedGeneration = observed
		d.Status.UpdatedReplicas = updated
		d.Status.AvailableReplicas = avail
		d.Status.Replicas = total
		return d
	}

	assert.True(t, deploymentRolloutComplete(mk(2, 2, 3, 3, 3, 3)), "fully rolled out")
	assert.False(t, deploymentRolloutComplete(mk(2, 1, 3, 3, 3, 3)), "latest generation not yet observed")
	assert.False(t, deploymentRolloutComplete(mk(2, 2, 3, 2, 3, 3)), "not all replicas updated")
	assert.False(t, deploymentRolloutComplete(mk(2, 2, 3, 3, 2, 3)), "not all replicas available")
	assert.False(t, deploymentRolloutComplete(mk(2, 2, 3, 3, 3, 4)), "stale replicas remain")
}

// routeToCandidate is a no-op when there's no mesh to weight: nil traffic
// routing and replica-weighted mode both leave scaling to reconcileDeployment.
func TestRouteToCandidate_NoMeshIsNoOp(t *testing.T) {
	r := &AgentRuntimeReconciler{} // MeshEnabled false → meshAvailable false

	arNil := newRolloutTestAR()
	arNil.Spec.Rollout = &omniav1alpha1.RolloutConfig{}
	require.NoError(t, r.routeToCandidate(context.Background(), arNil))

	arReplica := newRolloutTestAR()
	arReplica.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		TrafficRouting: &omniav1alpha1.TrafficRoutingConfig{Mode: TrafficModeReplicaWeighted},
	}
	require.NoError(t, r.routeToCandidate(context.Background(), arReplica))
}

// While the stable Deployment is still rolling to the new config, promotion
// keeps requeueing and does not finish (candidate keeps serving).
func TestAdvanceOrFinishPromotion_WaitsForStable(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := newRolloutTestAR()
	ar.Status.Rollout = &omniav1alpha1.RolloutStatus{Active: true, Promoting: true}

	stable := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: ar.Name, Namespace: ar.Namespace},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To[int32](2)},
		Status:     appsv1.DeploymentStatus{Replicas: 2, UpdatedReplicas: 1, AvailableReplicas: 1},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).WithObjects(ar, stable).WithStatusSubresource(ar).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	result, err := r.advanceOrFinishPromotion(context.Background(), ar)
	require.NoError(t, err)
	assert.Equal(t, promotePollInterval, result.RequeueAfter, "should keep waiting while stable rolls")
	assert.True(t, ar.Status.Rollout.Promoting, "still promoting")
}

// A missing stable Deployment during promotion is a hard error, not a silent
// finish (which would delete the candidate with nothing serving).
func TestAdvanceOrFinishPromotion_MissingStableErrors(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))

	ar := newRolloutTestAR()
	ar.Status.Rollout = &omniav1alpha1.RolloutStatus{Active: true, Promoting: true}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ar).WithStatusSubresource(ar).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	_, err := r.advanceOrFinishPromotion(context.Background(), ar)
	require.Error(t, err)
}

// finishPromotion must clear spec.rollout.candidate once stable has fully
// rolled to the promoted config (#1838): otherwise a version-triggered
// rollout's candidate lingers, matching the (already-promoted) stable spec
// forever, instead of giving the next trigger reconcile a clean baseline to
// compare the channel-latest against.
func TestFinishPromotion_ClearsCandidate(t *testing.T) {
	scheme := newTestScheme(t)
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := newRolloutTestAR()
	// enterPromotion already advanced spec.promptPackRef to the candidate
	// version by the time finishPromotion runs; model that precondition.
	ar.Spec.PromptPackRef.Version = ptr.To("v2")
	ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Candidate: &omniav1alpha1.CandidateOverrides{
			PromptPackRef: &omniav1alpha1.PromptPackRef{Name: testStablePackName, Version: ptr.To("v2")},
		},
		Steps: []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](100)}},
	}
	ar.Status.Rollout = &omniav1alpha1.RolloutStatus{Active: true, Promoting: true}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).WithObjects(ar).WithStatusSubresource(ar).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	result, err := r.finishPromotion(context.Background(), ar)
	require.NoError(t, err)
	assert.Zero(t, result.RequeueAfter)
	assert.Equal(t, "v2", *ar.Spec.PromptPackRef.Version, "the pin stays advanced to the promoted version")
	assert.Nil(t, ar.Spec.Rollout.Candidate,
		"candidate must be cleared so the next version-trigger reconcile compares against a clean baseline")
	assert.False(t, isRolloutActive(ar), "rollout must be idle once the candidate is cleared")
}
