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

// autoRollbackEnvtestCounter gives each spec a unique resource suffix.
var autoRollbackEnvtestCounter uint64

var _ = Describe("AgentRuntime Rollout Auto-Rollback (envtest)", func() {
	var (
		ctx       context.Context
		namespace string
		nextName  = func(prefix string) string {
			n := atomic.AddUint64(&autoRollbackEnvtestCounter, 1)
			return fmt.Sprintf("%s-%d", prefix, n)
		}
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = nextName("ar-test")
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

	newPromptPack := func(name string) *omniav1alpha1.PromptPack {
		return &omniav1alpha1.PromptPack{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: omniav1alpha1.PromptPackSpec{
				Source: omniav1alpha1.PromptPackSource{
					Type:         omniav1alpha1.PromptPackSourceTypeConfigMap,
					ConfigMapRef: &corev1.LocalObjectReference{Name: name + "-config"},
				},
				Version: "1.0.0",
			},
		}
	}

	baseAR := func(name string) *omniav1alpha1.AgentRuntime {
		port := int32(8080)
		return &omniav1alpha1.AgentRuntime{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				PromptPackRef: omniav1alpha1.PromptPackRef{
					Name:    "support-pack",
					Version: ptr.To("v1"),
				},
				Facade: omniav1alpha1.FacadeConfig{
					Type: omniav1alpha1.FacadeTypeWebSocket,
					Port: &port,
				},
				Providers: []omniav1alpha1.NamedProviderRef{{
					Name:        "default",
					ProviderRef: omniav1alpha1.ProviderRef{Name: "claude-provider"},
				}},
			},
		}
	}

	// markCandidateUnhealthy patches the given Deployment's status so
	// shouldAutoRollback returns true. envtest has no kubelet so we have to
	// synthesize the status ourselves.
	markCandidateUnhealthy := func(deploy *appsv1.Deployment) {
		deploy.Status.UnavailableReplicas = 1
		deploy.Status.ReadyReplicas = 0
		Expect(k8sClient.Status().Update(ctx, deploy)).To(Succeed())
	}

	It("auto-rolls back when candidate pods are unhealthy and mode is automatic", func() {
		arName := nextName("ar")
		packName := nextName("pack")

		pp := newPromptPack(packName)
		Expect(k8sClient.Create(ctx, pp)).To(Succeed())

		ar := baseAR(arName)
		ar.Spec.PromptPackRef.Name = packName
		ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
			Candidate: &omniav1alpha1.CandidateOverrides{
				PromptPackVersion: ptr.To("v2"),
			},
			Steps: []omniav1alpha1.RolloutStep{
				{SetWeight: ptr.To[int32](25)},
			},
			Rollback: &omniav1alpha1.RollbackConfig{
				Mode: omniav1alpha1.RollbackModeAutomatic,
			},
		}
		Expect(k8sClient.Create(ctx, ar)).To(Succeed())

		r := &AgentRuntimeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

		// First reconcile — creates the candidate Deployment so we can then
		// synthesize an unhealthy status on it.
		live := &omniav1alpha1.AgentRuntime{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, live)).To(Succeed())
		_, err := r.reconcileRollout(ctx, live, pp, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		candDeploy := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name: candidateDeploymentName(arName), Namespace: namespace,
		}, candDeploy)).To(Succeed())
		markCandidateUnhealthy(candDeploy)

		// Re-fetch AR (first reconcile advanced currentStep in-memory but
		// didn't persist; refetch gives us a clean view).
		live2 := &omniav1alpha1.AgentRuntime{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, live2)).To(Succeed())

		_, err = r.reconcileRollout(ctx, live2, pp, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		// Candidate PromptPackVersion should have been reverted to match stable.
		after := &omniav1alpha1.AgentRuntime{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, after)).To(Succeed())
		Expect(after.Spec.Rollout).NotTo(BeNil())
		Expect(after.Spec.Rollout.Candidate).NotTo(BeNil())
		Expect(after.Spec.Rollout.Candidate.PromptPackVersion).NotTo(BeNil())
		Expect(*after.Spec.Rollout.Candidate.PromptPackVersion).To(Equal("v1"),
			"candidate should be reverted to stable version after auto-rollback")

		// Candidate Deployment should have been deleted.
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name: candidateDeploymentName(arName), Namespace: namespace,
		}, &appsv1.Deployment{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(),
			"candidate Deployment should be deleted after auto-rollback, got: %v", err)

		// Status should report inactive with the rollback reason.
		Expect(after.Status.Rollout).NotTo(BeNil())
		Expect(after.Status.Rollout.Active).To(BeFalse())
		Expect(after.Status.Rollout.Message).To(ContainSubstring("auto-rollback"))
		Expect(after.Status.Rollout.Message).To(ContainSubstring("pod unhealthy"))
	})

	It("does NOT auto-roll back when candidate is unhealthy but mode is manual", func() {
		arName := nextName("ar")
		packName := nextName("pack")

		pp := newPromptPack(packName)
		Expect(k8sClient.Create(ctx, pp)).To(Succeed())

		ar := baseAR(arName)
		ar.Spec.PromptPackRef.Name = packName
		ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
			Candidate: &omniav1alpha1.CandidateOverrides{
				PromptPackVersion: ptr.To("v2"),
			},
			Steps: []omniav1alpha1.RolloutStep{
				{SetWeight: ptr.To[int32](25)},
			},
			Rollback: &omniav1alpha1.RollbackConfig{
				Mode: omniav1alpha1.RollbackModeManual,
			},
		}
		Expect(k8sClient.Create(ctx, ar)).To(Succeed())

		r := &AgentRuntimeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		live := &omniav1alpha1.AgentRuntime{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, live)).To(Succeed())
		_, err := r.reconcileRollout(ctx, live, pp, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		candDeploy := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name: candidateDeploymentName(arName), Namespace: namespace,
		}, candDeploy)).To(Succeed())
		markCandidateUnhealthy(candDeploy)

		live2 := &omniav1alpha1.AgentRuntime{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, live2)).To(Succeed())
		_, err = r.reconcileRollout(ctx, live2, pp, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		// Candidate should still be v2 — manual mode does not auto-revert.
		after := &omniav1alpha1.AgentRuntime{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, after)).To(Succeed())
		Expect(after.Spec.Rollout.Candidate.PromptPackVersion).NotTo(BeNil())
		Expect(*after.Spec.Rollout.Candidate.PromptPackVersion).To(Equal("v2"),
			"manual rollback mode should leave candidate intact")

		// Candidate Deployment should still exist.
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name: candidateDeploymentName(arName), Namespace: namespace,
		}, &appsv1.Deployment{})).To(Succeed())
	})

	It("does NOT auto-roll back when candidate is healthy, even in automatic mode", func() {
		arName := nextName("ar")
		packName := nextName("pack")

		pp := newPromptPack(packName)
		Expect(k8sClient.Create(ctx, pp)).To(Succeed())

		ar := baseAR(arName)
		ar.Spec.PromptPackRef.Name = packName
		ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
			Candidate: &omniav1alpha1.CandidateOverrides{
				PromptPackVersion: ptr.To("v2"),
			},
			Steps: []omniav1alpha1.RolloutStep{
				{SetWeight: ptr.To[int32](25)},
			},
			Rollback: &omniav1alpha1.RollbackConfig{
				Mode: omniav1alpha1.RollbackModeAutomatic,
			},
		}
		Expect(k8sClient.Create(ctx, ar)).To(Succeed())

		r := &AgentRuntimeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		live := &omniav1alpha1.AgentRuntime{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, live)).To(Succeed())
		_, err := r.reconcileRollout(ctx, live, pp, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		// Mark candidate explicitly healthy — ready replicas > 0, no unavailable.
		candDeploy := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name: candidateDeploymentName(arName), Namespace: namespace,
		}, candDeploy)).To(Succeed())
		candDeploy.Status.Replicas = 1
		candDeploy.Status.ReadyReplicas = 1
		candDeploy.Status.UnavailableReplicas = 0
		Expect(k8sClient.Status().Update(ctx, candDeploy)).To(Succeed())

		live2 := &omniav1alpha1.AgentRuntime{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, live2)).To(Succeed())
		_, err = r.reconcileRollout(ctx, live2, pp, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		after := &omniav1alpha1.AgentRuntime{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, after)).To(Succeed())
		Expect(*after.Spec.Rollout.Candidate.PromptPackVersion).To(Equal("v2"),
			"healthy candidate should not trigger auto-rollback")
	})
})
