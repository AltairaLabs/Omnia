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
	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// rolloutAnalysisEnvtestCounter gives each spec a unique resource suffix.
var rolloutAnalysisEnvtestCounter uint64

var _ = Describe("AgentRuntime Rollout Analysis (envtest)", func() {
	var (
		ctx       context.Context
		namespace string
		nextName  = func(prefix string) string {
			n := atomic.AddUint64(&rolloutAnalysisEnvtestCounter, 1)
			return fmt.Sprintf("%s-%d", prefix, n)
		}
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = nextName("ra-test")
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

	Context("RolloutAnalysis CRD validation (API server enforcement)", func() {
		It("rejects a RolloutAnalysis with an empty metrics list", func() {
			ra := &eev1alpha1.RolloutAnalysis{
				ObjectMeta: metav1.ObjectMeta{Name: nextName("ra"), Namespace: namespace},
				Spec: eev1alpha1.RolloutAnalysisSpec{
					Metrics: []eev1alpha1.AnalysisMetric{},
				},
			}
			err := k8sClient.Create(ctx, ra)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("rejects a metric with an empty name", func() {
			ra := &eev1alpha1.RolloutAnalysis{
				ObjectMeta: metav1.ObjectMeta{Name: nextName("ra"), Namespace: namespace},
				Spec: eev1alpha1.RolloutAnalysisSpec{
					Metrics: []eev1alpha1.AnalysisMetric{{
						Name:             "",
						Interval:         "1m",
						Count:            1,
						SuccessCondition: "result[0] >= 0.9",
						Provider: eev1alpha1.MetricProvider{
							Prometheus: &eev1alpha1.PrometheusProvider{
								Address: "http://p:9090", Query: "up",
							},
						},
					}},
				},
			}
			err := k8sClient.Create(ctx, ra)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("rejects a metric with count=0", func() {
			ra := &eev1alpha1.RolloutAnalysis{
				ObjectMeta: metav1.ObjectMeta{Name: nextName("ra"), Namespace: namespace},
				Spec: eev1alpha1.RolloutAnalysisSpec{
					Metrics: []eev1alpha1.AnalysisMetric{{
						Name:             "err-rate",
						Interval:         "1m",
						Count:            0,
						SuccessCondition: "result[0] <= 0.05",
						Provider: eev1alpha1.MetricProvider{
							Prometheus: &eev1alpha1.PrometheusProvider{
								Address: "http://p:9090", Query: "up",
							},
						},
					}},
				},
			}
			err := k8sClient.Create(ctx, ra)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("accepts a well-formed RolloutAnalysis", func() {
			ra := &eev1alpha1.RolloutAnalysis{
				ObjectMeta: metav1.ObjectMeta{Name: nextName("ra"), Namespace: namespace},
				Spec: eev1alpha1.RolloutAnalysisSpec{
					Metrics: []eev1alpha1.AnalysisMetric{{
						Name:             "err-rate",
						Interval:         "1m",
						Count:            3,
						SuccessCondition: "result[0] <= 0.05",
						Provider: eev1alpha1.MetricProvider{
							Prometheus: &eev1alpha1.PrometheusProvider{
								Address: "http://prom:9090",
								Query:   "rate(errors[1m])",
							},
						},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, ra)).To(Succeed())
		})
	})

	Context("reconcileRolloutAnalysis against real API server", func() {
		It("requeues without rolling back when the referenced template is missing", func() {
			arName := nextName("ar")
			ar := baseAR(arName)
			ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
				Candidate: &omniav1alpha1.CandidateOverrides{
					PromptPackVersion: ptr.To("v2"),
				},
				Steps: []omniav1alpha1.RolloutStep{
					{Analysis: &omniav1alpha1.RolloutAnalysisStep{TemplateName: "missing-template"}},
				},
			}
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			r := &AgentRuntimeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			live := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, live)).To(Succeed())

			result, err := r.reconcileRolloutAnalysis(ctx, live, rolloutStepResult{
				active:       true,
				currentStep:  0,
				analysis:     true,
				analysisName: "missing-template",
			})
			// runAnalysis error → handler logs + requeues in 30s, no error returned.
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Spec must not have been rolled back — candidate still present.
			afterReq := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, afterReq)).To(Succeed())
			Expect(afterReq.Spec.Rollout).NotTo(BeNil())
			Expect(afterReq.Spec.Rollout.Candidate).NotTo(BeNil(),
				"missing template should not trigger rollback — template may appear on retry")
		})
	})

	Context("analysis fail handlers against real API server", func() {
		It("handleAnalysisManualPause writes a manual-pause status without rollback", func() {
			arName := nextName("ar")
			ar := baseAR(arName)
			ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
				Candidate: &omniav1alpha1.CandidateOverrides{
					PromptPackVersion: ptr.To("v2"),
				},
				Steps: []omniav1alpha1.RolloutStep{
					{Analysis: &omniav1alpha1.RolloutAnalysisStep{TemplateName: "some-template"}},
				},
				Rollback: &omniav1alpha1.RollbackConfig{Mode: omniav1alpha1.RollbackModeManual},
			}
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			r := &AgentRuntimeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			live := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, live)).To(Succeed())

			_, err := r.handleAnalysisManualPause(ctx, live, 0, "error-rate too high")
			Expect(err).NotTo(HaveOccurred())

			after := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, after)).To(Succeed())
			Expect(after.Status.Rollout).NotTo(BeNil())
			Expect(after.Status.Rollout.Active).To(BeTrue(),
				"manual-pause keeps the rollout active awaiting operator action")
			Expect(after.Status.Rollout.Message).To(ContainSubstring("analysis failed"))
			Expect(after.Status.Rollout.Message).To(ContainSubstring("error-rate too high"))

			// Spec must not have been rolled back.
			Expect(after.Spec.Rollout).NotTo(BeNil())
			Expect(after.Spec.Rollout.Candidate).NotTo(BeNil())
			Expect(after.Spec.Rollout.Candidate.PromptPackVersion).NotTo(BeNil())
			Expect(*after.Spec.Rollout.Candidate.PromptPackVersion).To(Equal("v2"))
		})

		It("handleAnalysisAutoRollback reverts candidate, deletes candidate Deployment, and marks rollout inactive", func() {
			arName := nextName("ar")
			ar := baseAR(arName)
			ar.Spec.Rollout = &omniav1alpha1.RolloutConfig{
				Candidate: &omniav1alpha1.CandidateOverrides{
					// Candidate wants v2 — rollback should revert candidate to v1 (matching stable).
					PromptPackVersion: ptr.To("v2"),
				},
				Steps: []omniav1alpha1.RolloutStep{
					{Analysis: &omniav1alpha1.RolloutAnalysisStep{TemplateName: "some-template"}},
				},
				Rollback: &omniav1alpha1.RollbackConfig{Mode: omniav1alpha1.RollbackModeAutomatic},
			}
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			// Simulate a leftover candidate Deployment that should be cleaned up.
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
			live := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, live)).To(Succeed())

			_, err := r.handleAnalysisAutoRollback(ctx, live, "error-rate too high")
			Expect(err).NotTo(HaveOccurred())

			// Spec: candidate's PromptPackVersion should now match stable's (v1) —
			// candidateDiffers() returns false so isRolloutActive() is false.
			after := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, after)).To(Succeed())
			Expect(after.Spec.Rollout).NotTo(BeNil())
			Expect(after.Spec.Rollout.Candidate).NotTo(BeNil())
			Expect(after.Spec.Rollout.Candidate.PromptPackVersion).NotTo(BeNil())
			Expect(*after.Spec.Rollout.Candidate.PromptPackVersion).To(Equal("v1"),
				"candidate should be reverted to stable version after rollback")

			// Candidate Deployment should have been deleted.
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name: candidateDeploymentName(arName), Namespace: namespace,
			}, &appsv1.Deployment{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue(),
				"candidate Deployment should be deleted after analysis auto-rollback, got: %v", err)

			// Status: inactive with rollback message.
			Expect(after.Status.Rollout).NotTo(BeNil())
			Expect(after.Status.Rollout.Active).To(BeFalse())
			Expect(after.Status.Rollout.Message).To(ContainSubstring("auto-rollback"))
			Expect(after.Status.Rollout.Message).To(ContainSubstring("error-rate too high"))
		})
	})
})
