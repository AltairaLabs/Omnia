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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

var _ = Describe("PromptPack Controller", func() {
	const (
		promptPackName      = "test-promptpack"
		promptPackNamespace = "default"
		configMapName       = "test-prompts"
	)

	ctx := context.Background()

	Context("When reconciling a PromptPack with valid ConfigMap source", func() {
		var (
			promptPack *omniav1alpha1.PromptPack
			configMap  *corev1.ConfigMap
		)

		BeforeEach(func() {
			By("creating the ConfigMap for the PromptPack")
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: promptPackNamespace,
				},
				Data: map[string]string{
					"system.txt": "You are a helpful assistant.",
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("creating the PromptPack resource")
			promptPack = &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptPackName,
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: configMapName,
						},
					},
					Version: "1.0.0",
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the PromptPack")
			resource := &omniav1alpha1.PromptPack{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      promptPackName,
				Namespace: promptPackNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			By("cleaning up the ConfigMap")
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      configMapName,
				Namespace: promptPackNamespace,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}
		})

		It("should set phase to Active and set SourceValid condition", func() {
			By("reconciling the PromptPack")
			reconciler := &PromptPackReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      promptPackName,
					Namespace: promptPackNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedPP := &omniav1alpha1.PromptPack{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      promptPackName,
				Namespace: promptPackNamespace,
			}, updatedPP)).To(Succeed())

			Expect(updatedPP.Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))
			Expect(updatedPP.Status.ActiveVersion).NotTo(BeNil())
			Expect(*updatedPP.Status.ActiveVersion).To(Equal("1.0.0"))

			By("checking the SourceValid condition")
			condition := meta.FindStatusCondition(updatedPP.Status.Conditions, PromptPackConditionTypeSourceValid)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal("SourceValid"))
		})
	})

	Context("When reconciling a PromptPack with missing ConfigMap", func() {
		var promptPack *omniav1alpha1.PromptPack

		BeforeEach(func() {
			By("creating the PromptPack resource without the ConfigMap")
			promptPack = &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-configmap-pack",
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: "nonexistent-configmap",
						},
					},
					Version: "1.0.0",
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the PromptPack")
			resource := &omniav1alpha1.PromptPack{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "missing-configmap-pack",
				Namespace: promptPackNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set phase to Failed and set SourceValid condition to false", func() {
			By("reconciling the PromptPack")
			reconciler := &PromptPackReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "missing-configmap-pack",
					Namespace: promptPackNamespace,
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))

			By("checking the updated status")
			updatedPP := &omniav1alpha1.PromptPack{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "missing-configmap-pack",
				Namespace: promptPackNamespace,
			}, updatedPP)).To(Succeed())

			Expect(updatedPP.Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseFailed))

			By("checking the SourceValid condition")
			condition := meta.FindStatusCondition(updatedPP.Status.Conditions, PromptPackConditionTypeSourceValid)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("SourceValidationFailed"))
		})
	})

	Context("When reconciling a PromptPack with canary rollout", func() {
		var (
			promptPack *omniav1alpha1.PromptPack
			configMap  *corev1.ConfigMap
		)

		BeforeEach(func() {
			By("creating the ConfigMap for the PromptPack")
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "canary-prompts",
					Namespace: promptPackNamespace,
				},
				Data: map[string]string{
					"system.txt": "You are a helpful assistant v2.",
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("creating the PromptPack with canary rollout")
			weight := int32(20)
			promptPack = &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "canary-pack",
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: "canary-prompts",
						},
					},
					Version: "2.0.0",
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyCanary,
						Canary: &omniav1alpha1.CanaryConfig{
							Weight: weight,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			resource := &omniav1alpha1.PromptPack{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "canary-pack",
				Namespace: promptPackNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "canary-prompts",
				Namespace: promptPackNamespace,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}
		})

		It("should set phase to Canary and track canary weight", func() {
			By("reconciling the PromptPack")
			reconciler := &PromptPackReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "canary-pack",
					Namespace: promptPackNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedPP := &omniav1alpha1.PromptPack{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "canary-pack",
				Namespace: promptPackNamespace,
			}, updatedPP)).To(Succeed())

			Expect(updatedPP.Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseCanary))
			Expect(updatedPP.Status.CanaryVersion).NotTo(BeNil())
			Expect(*updatedPP.Status.CanaryVersion).To(Equal("2.0.0"))
			Expect(updatedPP.Status.CanaryWeight).NotTo(BeNil())
			Expect(*updatedPP.Status.CanaryWeight).To(Equal(int32(20)))
		})
	})

	Context("When a PromptPack is referenced by AgentRuntimes", func() {
		var (
			promptPack   *omniav1alpha1.PromptPack
			agentRuntime *omniav1alpha1.AgentRuntime
			configMap    *corev1.ConfigMap
		)

		BeforeEach(func() {
			By("creating the ConfigMap")
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ref-prompts",
					Namespace: promptPackNamespace,
				},
				Data: map[string]string{
					"system.txt": "Test prompt.",
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("creating the PromptPack")
			promptPack = &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "referenced-pack",
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: "ref-prompts",
						},
					},
					Version: "1.0.0",
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())

			By("creating an AgentRuntime that references the PromptPack")
			agentRuntime = &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: "referenced-pack",
					},
					Facade: omniav1alpha1.FacadeConfig{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					},
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "provider-secret",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			ar := &omniav1alpha1.AgentRuntime{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-agent",
				Namespace: promptPackNamespace,
			}, ar)
			if err == nil {
				Expect(k8sClient.Delete(ctx, ar)).To(Succeed())
			}

			pp := &omniav1alpha1.PromptPack{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "referenced-pack",
				Namespace: promptPackNamespace,
			}, pp)
			if err == nil {
				Expect(k8sClient.Delete(ctx, pp)).To(Succeed())
			}

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "ref-prompts",
				Namespace: promptPackNamespace,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}
		})

		It("should find the referencing AgentRuntime and set notification condition", func() {
			By("reconciling the PromptPack")
			reconciler := &PromptPackReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "referenced-pack",
					Namespace: promptPackNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedPP := &omniav1alpha1.PromptPack{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "referenced-pack",
				Namespace: promptPackNamespace,
			}, updatedPP)).To(Succeed())

			By("checking the AgentsNotified condition")
			condition := meta.FindStatusCondition(updatedPP.Status.Conditions, PromptPackConditionTypeAgentsNotified)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal("AgentsNotified"))
			Expect(condition.Message).To(ContainSubstring("1 AgentRuntime"))
		})
	})

	Context("When reconciling a non-existent PromptPack", func() {
		It("should return without error", func() {
			By("reconciling a non-existent PromptPack")
			reconciler := &PromptPackReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "nonexistent-pack",
					Namespace: promptPackNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When PromptPack has missing configMapRef", func() {
		var promptPack *omniav1alpha1.PromptPack

		BeforeEach(func() {
			By("creating the PromptPack without configMapRef")
			promptPack = &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-ref-pack",
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						// ConfigMapRef is nil
					},
					Version: "1.0.0",
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the PromptPack")
			resource := &omniav1alpha1.PromptPack{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "no-ref-pack",
				Namespace: promptPackNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should fail validation and set Failed phase", func() {
			By("reconciling the PromptPack")
			reconciler := &PromptPackReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "no-ref-pack",
					Namespace: promptPackNamespace,
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("configMapRef is required"))

			By("checking the updated status")
			updatedPP := &omniav1alpha1.PromptPack{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "no-ref-pack",
				Namespace: promptPackNamespace,
			}, updatedPP)).To(Succeed())

			Expect(updatedPP.Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseFailed))
		})
	})

	Context("When canary weight reaches 100%", func() {
		var (
			promptPack *omniav1alpha1.PromptPack
			configMap  *corev1.ConfigMap
		)

		BeforeEach(func() {
			By("creating the ConfigMap")
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "full-canary-prompts",
					Namespace: promptPackNamespace,
				},
				Data: map[string]string{
					"system.txt": "Full canary prompt.",
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("creating the PromptPack with 100% canary weight")
			promptPack = &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "full-canary-pack",
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: "full-canary-prompts",
						},
					},
					Version: "3.0.0",
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyCanary,
						Canary: &omniav1alpha1.CanaryConfig{
							Weight: 100,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			pp := &omniav1alpha1.PromptPack{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "full-canary-pack",
				Namespace: promptPackNamespace,
			}, pp)
			if err == nil {
				Expect(k8sClient.Delete(ctx, pp)).To(Succeed())
			}

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "full-canary-prompts",
				Namespace: promptPackNamespace,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}
		})

		It("should promote to Active phase", func() {
			By("reconciling the PromptPack")
			reconciler := &PromptPackReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "full-canary-pack",
					Namespace: promptPackNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedPP := &omniav1alpha1.PromptPack{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "full-canary-pack",
				Namespace: promptPackNamespace,
			}, updatedPP)).To(Succeed())

			Expect(updatedPP.Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))
			Expect(updatedPP.Status.ActiveVersion).NotTo(BeNil())
			Expect(*updatedPP.Status.ActiveVersion).To(Equal("3.0.0"))
			Expect(updatedPP.Status.CanaryVersion).To(BeNil())
			Expect(updatedPP.Status.CanaryWeight).To(BeNil())
		})
	})

	Context("When ConfigMap is empty", func() {
		var (
			promptPack *omniav1alpha1.PromptPack
			configMap  *corev1.ConfigMap
		)

		BeforeEach(func() {
			By("creating an empty ConfigMap")
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-configmap",
					Namespace: promptPackNamespace,
				},
				// No data
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("creating the PromptPack")
			promptPack = &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-cm-pack",
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: "empty-configmap",
						},
					},
					Version: "1.0.0",
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			pp := &omniav1alpha1.PromptPack{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "empty-cm-pack",
				Namespace: promptPackNamespace,
			}, pp)
			if err == nil {
				Expect(k8sClient.Delete(ctx, pp)).To(Succeed())
			}

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "empty-configmap",
				Namespace: promptPackNamespace,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}
		})

		It("should fail validation with empty ConfigMap error", func() {
			By("reconciling the PromptPack")
			reconciler := &PromptPackReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "empty-cm-pack",
					Namespace: promptPackNamespace,
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("is empty"))

			By("checking the updated status")
			updatedPP := &omniav1alpha1.PromptPack{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "empty-cm-pack",
				Namespace: promptPackNamespace,
			}, updatedPP)).To(Succeed())

			Expect(updatedPP.Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseFailed))
		})
	})

	Context("When testing findPromptPacksForConfigMap", func() {
		var (
			promptPack *omniav1alpha1.PromptPack
			configMap  *corev1.ConfigMap
		)

		BeforeEach(func() {
			By("creating the ConfigMap")
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "watch-test-configmap",
					Namespace: promptPackNamespace,
				},
				Data: map[string]string{
					"system.txt": "Watch test prompt.",
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("creating the PromptPack that references the ConfigMap")
			promptPack = &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "watch-test-pack",
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: "watch-test-configmap",
						},
					},
					Version: "1.0.0",
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			pp := &omniav1alpha1.PromptPack{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "watch-test-pack",
				Namespace: promptPackNamespace,
			}, pp)
			if err == nil {
				Expect(k8sClient.Delete(ctx, pp)).To(Succeed())
			}

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "watch-test-configmap",
				Namespace: promptPackNamespace,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}
		})

		It("should return reconcile requests for PromptPacks referencing the ConfigMap", func() {
			By("calling findPromptPacksForConfigMap")
			reconciler := &PromptPackReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			requests := reconciler.findPromptPacksForConfigMap(ctx, configMap)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("watch-test-pack"))
			Expect(requests[0].Namespace).To(Equal(promptPackNamespace))
		})
	})
})

// Ensure unused import doesn't cause issues
var _ = errors.IsNotFound
