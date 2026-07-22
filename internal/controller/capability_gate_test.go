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
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/k8s"
	"github.com/altairalabs/omnia/pkg/runtime/contract"
)

// availableDeployment returns a Deployment whose Available condition is True and
// has been for `since`.
func availableDeployment(since time.Duration) *appsv1.Deployment {
	return &appsv1.Deployment{
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{{
				Type:               appsv1.DeploymentAvailable,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(time.Now().Add(-since)),
			}},
		},
	}
}

func duplexAgent() *omniav1alpha1.AgentRuntime {
	return &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Generation: 1},
		Spec:       omniav1alpha1.AgentRuntimeSpec{Duplex: &omniav1alpha1.DuplexConfig{Enabled: true}},
	}
}

func markReported(ar *omniav1alpha1.AgentRuntime, caps []string) {
	ar.Status.RuntimeCapabilities = caps
	meta.SetStatusCondition(&ar.Status.Conditions, metav1.Condition{
		Type: k8s.ConditionRuntimeCapabilitiesReported, Status: metav1.ConditionTrue, Reason: "Reported",
	})
}

func capCond(ar *omniav1alpha1.AgentRuntime) *metav1.Condition {
	return meta.FindStatusCondition(ar.Status.Conditions, ConditionTypeCapabilitiesSatisfied)
}

func TestEnforceCapabilities_MissingSetsFalseAndEvent(t *testing.T) {
	rec := record.NewFakeRecorder(10)
	r := &AgentRuntimeReconciler{Recorder: rec}
	ar := duplexAgent()
	markReported(ar, []string{contract.CapabilityInvoke}) // missing duplex_audio + interruption

	requeue := r.enforceCapabilities(logr.Discard(), ar, availableDeployment(time.Second))

	assert.Equal(t, time.Duration(0), requeue)
	require.NotNil(t, capCond(ar))
	assert.Equal(t, metav1.ConditionFalse, capCond(ar).Status)
	assert.True(t, capabilitiesMismatchForCurrentGen(ar), "deployment builder should now scale to 0")
	select {
	case ev := <-rec.Events:
		assert.Contains(t, ev, reasonCapabilitiesMissing)
	default:
		t.Fatal("expected a Warning event")
	}
}

func TestEnforceCapabilities_SatisfiedSetsTrue(t *testing.T) {
	r := &AgentRuntimeReconciler{Recorder: record.NewFakeRecorder(10)}
	ar := duplexAgent()
	markReported(ar, []string{contract.CapabilityDuplexAudio, contract.CapabilityInterruption})

	require.Equal(t, time.Duration(0), r.enforceCapabilities(logr.Discard(), ar, availableDeployment(time.Second)))
	require.NotNil(t, capCond(ar))
	assert.Equal(t, metav1.ConditionTrue, capCond(ar).Status)
}

func TestEnforceCapabilities_NoRequirementsIsSatisfied(t *testing.T) {
	r := &AgentRuntimeReconciler{Recorder: record.NewFakeRecorder(10)}
	ar := &omniav1alpha1.AgentRuntime{ObjectMeta: metav1.ObjectMeta{Generation: 1}} // no duplex, no function facade

	require.Equal(t, time.Duration(0), r.enforceCapabilities(logr.Discard(), ar, nil))
	assert.Equal(t, metav1.ConditionTrue, capCond(ar).Status)
}

func TestEnforceCapabilities_PendingWithinGraceRequeues(t *testing.T) {
	r := &AgentRuntimeReconciler{Recorder: record.NewFakeRecorder(10)}
	ar := duplexAgent() // required but no report yet

	requeue := r.enforceCapabilities(logr.Discard(), ar, availableDeployment(10*time.Second))
	assert.Greater(t, requeue, time.Duration(0), "should requeue to re-check when grace elapses")
	assert.Equal(t, metav1.ConditionUnknown, capCond(ar).Status)
}

func TestEnforceCapabilities_PendingNotAvailableNoRequeue(t *testing.T) {
	// Required caps but the Deployment isn't Available yet: pending, no time-based
	// requeue — a deployment/status event will re-trigger.
	r := &AgentRuntimeReconciler{Recorder: record.NewFakeRecorder(10)}
	ar := duplexAgent()

	requeue := r.enforceCapabilities(logr.Discard(), ar, &appsv1.Deployment{})
	assert.Equal(t, time.Duration(0), requeue)
	assert.Equal(t, metav1.ConditionUnknown, capCond(ar).Status)
}

func TestEnforceCapabilities_MismatchForGenIsSticky(t *testing.T) {
	// Already False for the current generation (pod scaled to 0, not Available):
	// must NOT flip to pending — that would scale back up and oscillate.
	r := &AgentRuntimeReconciler{Recorder: record.NewFakeRecorder(10)}
	ar := duplexAgent()
	meta.SetStatusCondition(&ar.Status.Conditions, metav1.Condition{
		Type: ConditionTypeCapabilitiesSatisfied, Status: metav1.ConditionFalse,
		ObservedGeneration: 1, Reason: reasonCapabilitiesMissing,
	})

	require.Equal(t, time.Duration(0), r.enforceCapabilities(logr.Discard(), ar, nil))
	assert.Equal(t, metav1.ConditionFalse, capCond(ar).Status, "verdict must stick until the generation changes")
}

func TestDeploymentAvailable(t *testing.T) {
	ok, since := deploymentAvailable(availableDeployment(3*time.Second), time.Now())
	assert.True(t, ok)
	assert.InDelta(t, (3 * time.Second).Seconds(), since.Seconds(), 1)

	no, _ := deploymentAvailable(&appsv1.Deployment{}, time.Now())
	assert.False(t, no)
	nilOK, _ := deploymentAvailable(nil, time.Now())
	assert.False(t, nilOK)
}

func TestBuildDeploymentSpec_ScalesToZeroWhenCapabilitiesMissing(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "a"
	ar.Namespace = "ns"
	ar.Generation = 1
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket}}
	ar.Spec.PromptPackRef.Name = "p"
	meta.SetStatusCondition(&ar.Status.Conditions, metav1.Condition{
		Type: ConditionTypeCapabilitiesSatisfied, Status: metav1.ConditionFalse,
		ObservedGeneration: 1, Reason: reasonCapabilitiesMissing,
	})

	dep := &appsv1.Deployment{}
	r.buildDeploymentSpec(context.Background(), dep, ar, newTestPromptPack(), nil, "", nil)

	require.NotNil(t, dep.Spec.Replicas)
	assert.Equal(t, int32(0), *dep.Spec.Replicas, "mismatch for current generation must scale to 0")
}

func TestEarliestRequeue(t *testing.T) {
	assert.Equal(t, time.Duration(0), earliestRequeue(0, 0))
	assert.Equal(t, 5*time.Second, earliestRequeue(0, 5*time.Second))
	assert.Equal(t, 5*time.Second, earliestRequeue(5*time.Second, 0))
	assert.Equal(t, 3*time.Second, earliestRequeue(3*time.Second, 5*time.Second))
	assert.Equal(t, 3*time.Second, earliestRequeue(5*time.Second, 3*time.Second))
}

func TestCapabilitiesMismatchForCurrentGen(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{ObjectMeta: metav1.ObjectMeta{Generation: 5}}
	assert.False(t, capabilitiesMismatchForCurrentGen(ar), "no condition -> no mismatch")

	meta.SetStatusCondition(&ar.Status.Conditions, metav1.Condition{
		Type: ConditionTypeCapabilitiesSatisfied, Status: metav1.ConditionFalse,
		ObservedGeneration: 5, Reason: "CapabilitiesMissing",
	})
	assert.True(t, capabilitiesMismatchForCurrentGen(ar), "False at current gen -> mismatch")

	ar.Generation = 6 // spec/image changed
	assert.False(t, capabilitiesMismatchForCurrentGen(ar), "stale generation -> no mismatch")

	meta.SetStatusCondition(&ar.Status.Conditions, metav1.Condition{
		Type: ConditionTypeCapabilitiesSatisfied, Status: metav1.ConditionTrue,
		ObservedGeneration: 6, Reason: "CapabilitiesSatisfied",
	})
	assert.False(t, capabilitiesMismatchForCurrentGen(ar), "True -> no mismatch")
}

func TestEvaluateCapabilities(t *testing.T) {
	dup := []string{contract.CapabilityDuplexAudio}
	cases := []struct {
		name         string
		req, adv     []string
		reported     bool
		available    bool
		since, grace time.Duration
		want         capabilityDecision
		wantMissing  []string
	}{
		{"none required", nil, nil, true, true, 0, time.Minute, capsSatisfied, nil},
		{"satisfied", dup, []string{contract.CapabilityDuplexAudio, contract.CapabilityInvoke}, true, true, 0, time.Minute, capsSatisfied, nil},
		{"missing reported", dup, []string{contract.CapabilityInvoke}, true, true, 0, time.Minute, capsMissing, dup},
		{"not available", dup, nil, false, false, 0, time.Minute, capsPending, nil},
		{"within grace", dup, nil, false, true, 10 * time.Second, time.Minute, capsPending, nil},
		{"legacy past grace", dup, nil, false, true, 2 * time.Minute, time.Minute, capsMissing, dup},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, missing := evaluateCapabilities(tc.req, tc.adv, tc.reported, tc.available, tc.since, tc.grace)
			assert.Equal(t, tc.want, got)
			assert.ElementsMatch(t, tc.wantMissing, missing)
		})
	}
}

func TestRequiredCapabilities(t *testing.T) {
	dup := &omniav1alpha1.AgentRuntime{Spec: omniav1alpha1.AgentRuntimeSpec{
		Duplex: &omniav1alpha1.DuplexConfig{Enabled: true},
	}}
	assert.ElementsMatch(t,
		[]string{contract.CapabilityDuplexAudio, contract.CapabilityInterruption},
		requiredCapabilities(dup))

	fn := &omniav1alpha1.AgentRuntime{Spec: omniav1alpha1.AgentRuntimeSpec{
		Facades: []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeMCP}},
	}}
	assert.Equal(t, []string{contract.CapabilityInvoke}, requiredCapabilities(fn))

	rest := &omniav1alpha1.AgentRuntime{Spec: omniav1alpha1.AgentRuntimeSpec{
		Facades: []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeREST}},
	}}
	assert.Equal(t, []string{contract.CapabilityInvoke}, requiredCapabilities(rest))

	plain := &omniav1alpha1.AgentRuntime{Spec: omniav1alpha1.AgentRuntimeSpec{
		Facades: []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket}},
	}}
	assert.Empty(t, requiredCapabilities(plain))
}
