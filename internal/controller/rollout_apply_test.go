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
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// applyEnvtestCounter gives each spec a unique resource suffix.
var applyEnvtestCounter uint64

var _ = Describe("AgentRuntime applyTrafficRouting dispatch (envtest)", func() {
	var (
		ctx       context.Context
		namespace string
		nextName  = func(prefix string) string {
			n := atomic.AddUint64(&applyEnvtestCounter, 1)
			return fmt.Sprintf("%s-%d", prefix, n)
		}
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = nextName("apply-test")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		})).To(Succeed())
	})

	AfterEach(func() {
		ns := &corev1.Namespace{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, ns); err == nil {
			_ = k8sClient.Delete(ctx, ns)
		}
	})

	// newDeployment builds a minimal Deployment with the given replicas.
	newDeployment := func(name string, replicas int32) *appsv1.Deployment {
		labels := map[string]string{"app": name}
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To(replicas),
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "agent", Image: "busybox"}},
					},
				},
			},
		}
	}

	getReplicas := func(name string) int32 {
		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, dep)).To(Succeed())
		if dep.Spec.Replicas == nil {
			return 0
		}
		return *dep.Spec.Replicas
	}

	It("degrades mode=mesh to replicaWeighted when the mesh is unavailable", func() {
		agentName := nextName("agent")
		candName := candidateDeploymentName(agentName)

		// The canonical total (4) comes from the AgentRuntime spec, not the live
		// stable Deployment — see canonicalReplicaTotal / Fix 2.
		Expect(k8sClient.Create(ctx, newDeployment(agentName, 4))).To(Succeed())
		Expect(k8sClient.Create(ctx, newDeployment(candName, 0))).To(Succeed())

		recorder := record.NewFakeRecorder(10)
		r := &AgentRuntimeReconciler{
			Client:      k8sClient,
			Scheme:      k8sClient.Scheme(),
			MeshEnabled: false, // mesh unavailable → mode=mesh degrades
			Recorder:    recorder,
		}

		ar := &omniav1alpha1.AgentRuntime{
			ObjectMeta: metav1.ObjectMeta{Name: agentName, Namespace: namespace, Generation: 1},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				Runtime: &omniav1alpha1.RuntimeConfig{Replicas: ptr.To(int32(4))},
				Rollout: &omniav1alpha1.RolloutConfig{
					TrafficRouting: &omniav1alpha1.TrafficRoutingConfig{
						Mode: TrafficModeMesh,
					},
				},
			},
		}

		Expect(r.applyTrafficRouting(ctx, ar, 50)).To(Succeed())

		// (a) replicaSplit(4, 50) → candidate=2, stable=2.
		Expect(getReplicas(candName)).To(Equal(int32(2)))
		Expect(getReplicas(agentName)).To(Equal(int32(2)))

		// (b) status reflects the degraded resolved mode + unenforced weight.
		Expect(ar.Status.Rollout).NotTo(BeNil())
		Expect(ar.Status.Rollout.TrafficRoutingMode).To(Equal(TrafficModeReplicaWeighted))
		Expect(ar.Status.Rollout.TrafficWeightEnforced).NotTo(BeNil())
		Expect(*ar.Status.Rollout.TrafficWeightEnforced).To(BeFalse())

		// (c) a TrafficRouting condition with status False exists.
		cond := findCondition(ar.Status.Conditions, ConditionTypeTrafficRouting)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))

		// (d) the Recorder received a TrafficRoutingDegraded event.
		Eventually(recorder.Events).Should(Receive(ContainSubstring("TrafficRoutingDegraded")))
	})

	It("persists TrafficRoutingMode + TrafficWeightEnforced through the reconcile status rebuild", func() {
		agentName := nextName("agent")
		candName := candidateDeploymentName(agentName)

		// Stable carries 4 replicas; candidate exists at 0.
		Expect(k8sClient.Create(ctx, newDeployment(agentName, 4))).To(Succeed())
		Expect(k8sClient.Create(ctx, newDeployment(candName, 0))).To(Succeed())

		// Create a real AgentRuntime so we can Get it back and assert PERSISTED
		// status (not just in-memory).
		port := int32(8080)
		ar := &omniav1alpha1.AgentRuntime{
			ObjectMeta: metav1.ObjectMeta{Name: agentName, Namespace: namespace, Generation: 1},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				PromptPackRef: omniav1alpha1.PromptPackRef{Name: "p", Version: ptr.To("v1")},
				Facade:        omniav1alpha1.FacadeConfig{Type: omniav1alpha1.FacadeTypeWebSocket, Port: &port},
				Providers: []omniav1alpha1.NamedProviderRef{{
					Name:        "default",
					ProviderRef: omniav1alpha1.ProviderRef{Name: "claude-provider"},
				}},
				Runtime: &omniav1alpha1.RuntimeConfig{Replicas: ptr.To(int32(4))},
				Rollout: &omniav1alpha1.RolloutConfig{
					Candidate: &omniav1alpha1.CandidateOverrides{PromptPackVersion: ptr.To("v2")},
					TrafficRouting: &omniav1alpha1.TrafficRoutingConfig{
						Mode: TrafficModeReplicaWeighted,
					},
					Steps: []omniav1alpha1.RolloutStep{
						{SetWeight: ptr.To[int32](50)},
						{SetWeight: ptr.To[int32](100)},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ar)).To(Succeed())

		r := &AgentRuntimeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

		live := &omniav1alpha1.AgentRuntime{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: agentName, Namespace: namespace}, live)).To(Succeed())

		// Exercise the real setWeight path: applyTrafficRouting (which calls
		// setTrafficStatus) followed by reconcileRolloutUpdateStatus (the rebuild
		// that previously dropped the two scalar fields). This is exactly what
		// reconcileRollout does on a setWeight step (rollout.go:110 + :135).
		stepResult := reconcileRolloutSteps(live)
		Expect(r.applyTrafficRouting(ctx, live, stepResult.desiredWeight)).To(Succeed())
		_, err := r.reconcileRolloutUpdateStatus(ctx, live, stepResult)
		Expect(err).NotTo(HaveOccurred())

		// Mirror the controller's terminal Status().Update (a setWeight step has
		// requeueAfter 0, so the outer Reconcile loop persists status at its end).
		// The fields must survive the in-memory rebuild AND a round-trip to the API.
		Expect(k8sClient.Status().Update(ctx, live)).To(Succeed())

		persisted := &omniav1alpha1.AgentRuntime{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: agentName, Namespace: namespace}, persisted)).To(Succeed())
		Expect(persisted.Status.Rollout).NotTo(BeNil())
		Expect(persisted.Status.Rollout.TrafficRoutingMode).To(Equal(TrafficModeReplicaWeighted),
			"resolved traffic mode must survive the status rebuild + persist")
		Expect(persisted.Status.Rollout.TrafficWeightEnforced).NotTo(BeNil(),
			"enforcement flag must survive the status rebuild + persist")
		Expect(*persisted.Status.Rollout.TrafficWeightEnforced).To(BeFalse(),
			"replicaWeighted delivers an approximate (unenforced) weight")
	})

	It("keeps the replica total constant across successive reconciles at the same weight", func() {
		agentName := nextName("agent")
		candName := candidateDeploymentName(agentName)

		// Stable starts at the canonical total of 4 (also stamped on spec).
		Expect(k8sClient.Create(ctx, newDeployment(agentName, 4))).To(Succeed())
		Expect(k8sClient.Create(ctx, newDeployment(candName, 0))).To(Succeed())

		r := &AgentRuntimeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

		ar := &omniav1alpha1.AgentRuntime{
			ObjectMeta: metav1.ObjectMeta{Name: agentName, Namespace: namespace, Generation: 1},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				Runtime: &omniav1alpha1.RuntimeConfig{Replicas: ptr.To(int32(4))},
				Rollout: &omniav1alpha1.RolloutConfig{
					TrafficRouting: &omniav1alpha1.TrafficRoutingConfig{Mode: TrafficModeReplicaWeighted},
				},
			},
		}

		// First reconcile at weight 50 → split(4,50) = candidate 2 / stable 2.
		Expect(r.applyTrafficRouting(ctx, ar, 50)).To(Succeed())
		Expect(getReplicas(candName)).To(Equal(int32(2)))
		Expect(getReplicas(agentName)).To(Equal(int32(2)))

		// Second reconcile at the SAME weight must NOT drift: the canonical total
		// (4) comes from the spec, not the now-shrunken live stable Deployment (2).
		// Before the fix this re-derived total=2 → split(2,50) = stable 1, drift.
		Expect(r.applyTrafficRouting(ctx, ar, 50)).To(Succeed())
		Expect(getReplicas(candName)).To(Equal(int32(2)),
			"candidate must stay at 2, not drift")
		Expect(getReplicas(agentName)).To(Equal(int32(2)),
			"stable must stay at 2, not shrink to 1")
	})
})
