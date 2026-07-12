/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// rolloutEnvtestCounter gives each spec a unique resource suffix.
var rolloutEnvtestCounter uint64

// testRolloutPodImage is a throwaway image for hand-built test Deployments
// (envtest never schedules pods, so the image is never pulled).
const testRolloutPodImage = "busybox"

var _ = Describe("AgentRuntime Rollout (envtest)", func() {
	var (
		ctx       context.Context
		namespace string
		nextName  = func(prefix string) string {
			n := atomic.AddUint64(&rolloutEnvtestCounter, 1)
			return fmt.Sprintf("%s-%d", prefix, n)
		}
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = nextName("rollout-test")
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

	// baseAR builds an AgentRuntime that passes core field validation. Specs
	// then attach a rollout block to exercise rollout-specific paths.
	baseAR := func(name string) *omniav1alpha1.AgentRuntime {
		port := int32(8080)
		return &omniav1alpha1.AgentRuntime{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				PromptPackRef: omniav1alpha1.PromptPackRef{
					Name:    testStablePackName,
					Version: ptr.To("v1"),
				},
				Facades: []omniav1alpha1.FacadeConfig{{
					Type: omniav1alpha1.FacadeTypeWebSocket,
					Port: &port,
				}},
				Providers: []omniav1alpha1.NamedProviderRef{{
					Name:        "default",
					ProviderRef: omniav1alpha1.ProviderRef{Name: "claude-provider"},
				}},
			},
		}
	}

	Context("CEL + field validation (API server enforcement)", func() {
		It("rejects an empty rollout step list", func() {
			ar := baseAR(nextName("ar"))
			ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
				Steps: []omniav1alpha1.RolloutStep{},
			}
			err := k8sClient.Create(ctx, ar)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(),
				"expected 400 Invalid, got: %v", err)
		})

		It("rejects setWeight greater than 100", func() {
			ar := baseAR(nextName("ar"))
			ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
				Steps: []omniav1alpha1.RolloutStep{
					{SetWeight: ptr.To[int32](101)},
				},
			}
			err := k8sClient.Create(ctx, ar)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("rejects setWeight less than 0", func() {
			ar := baseAR(nextName("ar"))
			ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
				Steps: []omniav1alpha1.RolloutStep{
					{SetWeight: ptr.To[int32](-1)},
				},
			}
			err := k8sClient.Create(ctx, ar)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("rejects an analysis step with an empty template name", func() {
			ar := baseAR(nextName("ar"))
			ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
				Steps: []omniav1alpha1.RolloutStep{
					{Analysis: &omniav1alpha1.RolloutAnalysisStep{TemplateName: ""}},
				},
			}
			err := k8sClient.Create(ctx, ar)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("rejects an invalid rollback mode enum value", func() {
			ar := baseAR(nextName("ar"))
			ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
				Steps: []omniav1alpha1.RolloutStep{
					{SetWeight: ptr.To[int32](50)},
				},
				Rollback: &omniav1alpha1.RollbackConfig{
					Mode: omniav1alpha1.RollbackMode("bogus"),
				},
			}
			err := k8sClient.Create(ctx, ar)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("rejects istio traffic routing with an empty VirtualService routes list", func() {
			ar := baseAR(nextName("ar"))
			ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
				Steps: []omniav1alpha1.RolloutStep{
					{SetWeight: ptr.To[int32](50)},
				},
				TrafficRouting: &omniav1alpha1.TrafficRoutingConfig{
					Istio: &omniav1alpha1.IstioTrafficRouting{
						VirtualService: omniav1alpha1.IstioVirtualServiceRef{
							Name:   "my-vs",
							Routes: []string{},
						},
						DestinationRule: omniav1alpha1.IstioDestinationRuleRef{Name: "my-dr"},
					},
				},
			}
			err := k8sClient.Create(ctx, ar)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("accepts a well-formed rollout config", func() {
			ar := baseAR(nextName("ar"))
			ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
				Candidate: &omniav1alpha1.CandidateOverrides{
					PromptPackRef: &omniav1alpha1.PromptPackRef{Name: testStablePackName, Version: ptr.To("v2")},
				},
				Steps: []omniav1alpha1.RolloutStep{
					{SetWeight: ptr.To[int32](20)},
					{Pause: &omniav1alpha1.RolloutPause{Duration: ptr.To("5m")}},
					{SetWeight: ptr.To[int32](100)},
				},
			}
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())
		})
	})

	Context("phase progression against real API server", func() {
		// newPromptPack creates a minimal PromptPack suitable for
		// buildDeploymentSpec to reference. The rollout reconcile path needs
		// one non-nil for candidate deployment construction.
		newPromptPack := func(name string) *omniav1alpha1.PromptPack {
			return &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Spec: omniav1alpha1.PromptPackSpec{
					PackName: "test-pack",
					Source: omniav1alpha1.PromptPackContentSource{
						Type:         omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{Name: name + "-config"},
					},
					Version: "1.0.0",
				},
			}
		}

		It("first reconcile creates candidate Deployment and advances currentStep", func() {
			arName := nextName("ar")
			packName := nextName("pack")

			pp := newPromptPack(packName)
			Expect(k8sClient.Create(ctx, pp)).To(Succeed())

			ar := baseAR(arName)
			ar.Spec.PromptPackRef.Name = packName
			ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
				Candidate: &omniav1alpha1.CandidateOverrides{
					PromptPackRef: &omniav1alpha1.PromptPackRef{Name: packName, Version: ptr.To("v2")},
				},
				Steps: []omniav1alpha1.RolloutStep{
					{SetWeight: ptr.To[int32](25)},
					{SetWeight: ptr.To[int32](100)},
				},
			}
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			r := &AgentRuntimeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			live := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, live)).To(Succeed())
			_, err := r.reconcileRollout(ctx, live, pp, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: candidateDeploymentName(arName), Namespace: namespace,
			}, &appsv1.Deployment{})).To(Succeed(),
				"candidate Deployment should be created on first reconcile")

			Expect(live.Status.Rollout).NotTo(BeNil())
			Expect(live.Status.Rollout.Active).To(BeTrue())
			Expect(live.Status.Rollout.CurrentStep).NotTo(BeNil())
			Expect(*live.Status.Rollout.CurrentStep).To(Equal(int32(1)),
				"setWeight step auto-advances currentStep to next index")
		})

		It("promotes candidate overrides into stable spec once all steps are complete", func() {
			arName := nextName("ar")
			packName := nextName("pack")

			pp := newPromptPack(packName)
			Expect(k8sClient.Create(ctx, pp)).To(Succeed())

			ar := baseAR(arName)
			ar.Spec.PromptPackRef.Name = packName
			ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
				Candidate: &omniav1alpha1.CandidateOverrides{
					PromptPackRef: &omniav1alpha1.PromptPackRef{Name: packName, Version: ptr.To("v2")},
				},
				Steps: []omniav1alpha1.RolloutStep{
					{SetWeight: ptr.To[int32](25)},
					{SetWeight: ptr.To[int32](100)},
				},
			}
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			// Seed status so the reconciler sees "past last step" (2 >= len(steps)=2)
			// and triggers promotion directly. We don't care about the intermediate
			// setWeight reconciles here — other tests cover those.
			live := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, live)).To(Succeed())
			past := int32(2)
			live.Status.Rollout = &omniav1alpha1.RolloutStatus{
				Active:      true,
				CurrentStep: &past,
			}
			Expect(k8sClient.Status().Update(ctx, live)).To(Succeed())

			r := &AgentRuntimeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, live)).To(Succeed())

			// --- Phase 1: promote ENTERS promotion (zero-downtime). ---
			_, err := r.reconcileRollout(ctx, live, pp, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			entered := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, entered)).To(Succeed())
			Expect(entered.Spec.PromptPackRef.Version).NotTo(BeNil())
			Expect(*entered.Spec.PromptPackRef.Version).To(Equal("v2"),
				"spec advances to candidate version on promote")
			Expect(entered.Status.Rollout).NotTo(BeNil())
			Expect(entered.Status.Rollout.Promoting).To(BeTrue(), "should be promoting")

			// Candidate must STILL be serving while stable rolls — not deleted.
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: candidateDeploymentName(arName), Namespace: namespace,
			}, &appsv1.Deployment{})).To(Succeed(), "candidate must still serve during promotion")

			// The stable Deployment rolls to the new config and becomes healthy.
			// envtest has no Deployment controller, so we create it and mark it
			// complete by hand.
			stable := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: arName, Namespace: namespace},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To[int32](1),
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{testAppLabelKey: arName}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{testAppLabelKey: arName}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: testRolloutPodImage}}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, stable)).To(Succeed())
			stable.Status = appsv1.DeploymentStatus{
				ObservedGeneration: stable.Generation,
				Replicas:           1,
				UpdatedReplicas:    1,
				ReadyReplicas:      1,
				AvailableReplicas:  1,
			}
			Expect(k8sClient.Status().Update(ctx, stable)).To(Succeed())

			// --- Phase 2: stable healthy → promotion finishes. ---
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, live)).To(Succeed())
			_, err = r.reconcileRollout(ctx, live, pp, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			afterPromote := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, afterPromote)).To(Succeed())

			// Candidate Deployment is deleted only now that stable serves new config.
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name: candidateDeploymentName(arName), Namespace: namespace,
			}, &appsv1.Deployment{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue(),
				"candidate deployment should be deleted after promotion finishes, got: %v", err)

			Expect(afterPromote.Status.Rollout).NotTo(BeNil())
			Expect(afterPromote.Status.Rollout.Active).To(BeFalse())
			Expect(afterPromote.Status.Rollout.Promoting).To(BeFalse())
			Expect(afterPromote.Status.Rollout.Message).To(Equal("promoted"))
		})

		It("holds the replica-weighted split across pause reconciles (does not re-inflate)", func() {
			arName := nextName("ar")
			packName := nextName("pack")

			pp := newPromptPack(packName)
			Expect(k8sClient.Create(ctx, pp)).To(Succeed())

			ar := baseAR(arName)
			ar.Spec.PromptPackRef.Name = packName
			ar.Spec.Runtime = &omniav1alpha1.RuntimeConfig{Replicas: ptr.To[int32](4)}
			ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
				Candidate: &omniav1alpha1.CandidateOverrides{
					PromptPackRef: &omniav1alpha1.PromptPackRef{Name: packName, Version: ptr.To("v2")},
				},
				// setWeight 50 then an indefinite pause — the canary spends ~all
				// its life in the pause, which is exactly where the builder used
				// to re-inflate both Deployments back to the canonical total.
				Steps: []omniav1alpha1.RolloutStep{
					{SetWeight: ptr.To[int32](50)},
					{Pause: &omniav1alpha1.RolloutPause{}},
				},
				TrafficRouting: &omniav1alpha1.TrafficRoutingConfig{
					Mode: TrafficModeReplicaWeighted,
				},
			}
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			// MeshEnabled defaults false → resolveTrafficMode yields replicaWeighted.
			r := &AgentRuntimeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Reuse one in-memory object across reconciles: the setWeight→pause
			// path advances status only in memory (reconcileRolloutUpdateStatus
			// persists only when it requeues), mirroring how the main loop carries
			// status forward within a reconcile.
			live := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, live)).To(Succeed())

			canaryName := candidateDeploymentName(arName)
			replicasOf := func(name string) int32 {
				dep := &appsv1.Deployment{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, dep)).To(Succeed())
				if dep.Spec.Replicas == nil {
					return -1
				}
				return *dep.Spec.Replicas
			}

			// One reconcile = stable build (main loop) then rollout (candidate
			// build + traffic weighting), as the real Reconcile orders them.
			reconcileOnce := func() {
				_, err := r.reconcileDeployment(ctx, live, pp, nil, nil)
				Expect(err).NotTo(HaveOccurred())
				_, err = r.reconcileRollout(ctx, live, pp, nil, nil)
				Expect(err).NotTo(HaveOccurred())
			}

			// Reconcile #1 — step 0 setWeight 50: split 4 → 2 stable / 2 canary.
			reconcileOnce()
			Expect(replicasOf(arName)).To(Equal(int32(2)), "stable scaled to 2 on setWeight")
			Expect(replicasOf(canaryName)).To(Equal(int32(2)), "canary scaled to 2 on setWeight")
			Expect(live.Status.Rollout.CurrentStep).NotTo(BeNil())
			Expect(*live.Status.Rollout.CurrentStep).To(Equal(int32(1)), "advanced into the pause step")

			// Reconciles #2 and #3 — step 1 paused indefinitely: the split MUST
			// hold. Before the fix the builder re-stamped the canonical total (4)
			// every reconcile, collapsing the data plane to 4/4 (1:1 ≈ 50:50).
			reconcileOnce()
			Expect(replicasOf(arName)).To(Equal(int32(2)), "stable held at 2 during pause (not re-inflated to 4)")
			Expect(replicasOf(canaryName)).To(Equal(int32(2)), "canary held at 2 during pause (not re-inflated to 4)")

			reconcileOnce()
			Expect(replicasOf(arName)).To(Equal(int32(2)), "stable still 2 after repeated pause reconciles")
			Expect(replicasOf(canaryName)).To(Equal(int32(2)), "canary still 2 after repeated pause reconciles")
		})

		It("reconcileRolloutIdle is a clean no-op when no rollout is configured", func() {
			arName := nextName("ar")
			ar := baseAR(arName)
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			r := &AgentRuntimeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.reconcileRollout(ctx, ar, nil, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			// No candidate Deployment should exist.
			candDeploy := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name: candidateDeploymentName(arName), Namespace: namespace,
			}, candDeploy)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			// RolloutActive condition should be set False on the in-memory object.
			expectRolloutCondition(ar, ConditionTypeRolloutActive, metav1.ConditionFalse)
		})

		It("idle path deletes a leftover candidate Deployment", func() {
			arName := nextName("ar")
			ar := baseAR(arName)
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			// Simulate a leftover candidate Deployment from a previous rollout.
			leftover := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      candidateDeploymentName(arName),
					Namespace: namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "leftover"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "leftover"}},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "c", Image: "busybox"}},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, leftover)).To(Succeed())

			r := &AgentRuntimeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.reconcileRollout(ctx, ar, nil, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name: candidateDeploymentName(arName), Namespace: namespace,
			}, &appsv1.Deployment{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue(),
				"leftover candidate Deployment should have been deleted")
		})
	})
})

func expectRolloutCondition(ar *omniav1alpha1.AgentRuntime, condType string, want metav1.ConditionStatus) {
	GinkgoHelper()
	for _, c := range ar.Status.Conditions {
		if c.Type == condType {
			Expect(c.Status).To(Equal(want),
				"condition %q status mismatch (reason=%s message=%s)",
				condType, c.Reason, c.Message)
			return
		}
	}
	Fail(fmt.Sprintf("condition %q not present", condType))
}
