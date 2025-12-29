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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

var _ = Describe("AgentRuntime Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When reconciling AgentRuntime", func() {
		var (
			ctx             context.Context
			agentRuntimeKey types.NamespacedName
			promptPackKey   types.NamespacedName
			reconciler      *AgentRuntimeReconciler
		)

		BeforeEach(func() {
			ctx = context.Background()
			agentRuntimeKey = types.NamespacedName{
				Name:      "test-agent-runtime",
				Namespace: "default",
			}
			promptPackKey = types.NamespacedName{
				Name:      "test-promptpack",
				Namespace: "default",
			}
			reconciler = &AgentRuntimeReconciler{
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				AgentImage: "test-agent:v1.0.0",
			}
		})

		AfterEach(func() {
			// Clean up AgentRuntime
			agentRuntime := &omniav1alpha1.AgentRuntime{}
			err := k8sClient.Get(ctx, agentRuntimeKey, agentRuntime)
			if err == nil {
				// Remove finalizer first to allow deletion
				agentRuntime.Finalizers = nil
				_ = k8sClient.Update(ctx, agentRuntime)
				_ = k8sClient.Delete(ctx, agentRuntime)
			}

			// Clean up PromptPack
			promptPack := &omniav1alpha1.PromptPack{}
			err = k8sClient.Get(ctx, promptPackKey, promptPack)
			if err == nil {
				_ = k8sClient.Delete(ctx, promptPack)
			}

			// Wait for cleanup
			Eventually(func() bool {
				err := k8sClient.Get(ctx, agentRuntimeKey, &omniav1alpha1.AgentRuntime{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("should fail when PromptPack is missing", func() {
			By("creating an AgentRuntime without a PromptPack")
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRuntimeKey.Name,
					Namespace: agentRuntimeKey.Namespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: "nonexistent-promptpack",
					},
					Facade: omniav1alpha1.FacadeConfig{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					},
					ProviderSecretRef: corev1.LocalObjectReference{
						Name: "test-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime - first adds finalizer")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("reconciling again - now should fail on missing PromptPack")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("PromptPack"))

			By("checking the status is set to Failed")
			Eventually(func() omniav1alpha1.AgentRuntimePhase {
				updated := &omniav1alpha1.AgentRuntime{}
				if err := k8sClient.Get(ctx, agentRuntimeKey, updated); err != nil {
					return ""
				}
				return updated.Status.Phase
			}, timeout, interval).Should(Equal(omniav1alpha1.AgentRuntimePhaseFailed))
		})

		It("should create Deployment and Service when PromptPack exists", func() {
			By("creating a PromptPack")
			promptPack := &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptPackKey.Name,
					Namespace: promptPackKey.Namespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Version: "1.0.0",
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
					},
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())

			By("creating an AgentRuntime")
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRuntimeKey.Name,
					Namespace: agentRuntimeKey.Namespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: promptPackKey.Name,
					},
					Facade: omniav1alpha1.FacadeConfig{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					},
					ProviderSecretRef: corev1.LocalObjectReference{
						Name: "test-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			// First reconcile adds finalizer
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Second reconcile creates resources
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the Deployment was created")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, deployment)
			}, timeout, interval).Should(Succeed())

			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.Name).To(Equal(AgentContainerName))
			Expect(container.Image).To(Equal("test-agent:v1.0.0"))
			Expect(container.Ports).To(HaveLen(1))
			Expect(container.Ports[0].ContainerPort).To(Equal(int32(DefaultFacadePort)))

			By("verifying the Service was created")
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, service)
			}, timeout, interval).Should(Succeed())

			Expect(service.Spec.Ports).To(HaveLen(1))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(DefaultFacadePort)))
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))

			By("verifying owner references are set")
			Expect(deployment.OwnerReferences).To(HaveLen(1))
			Expect(deployment.OwnerReferences[0].Name).To(Equal(agentRuntimeKey.Name))
			Expect(service.OwnerReferences).To(HaveLen(1))
			Expect(service.OwnerReferences[0].Name).To(Equal(agentRuntimeKey.Name))
		})

		It("should set status conditions correctly", func() {
			By("creating a PromptPack")
			promptPack := &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptPackKey.Name,
					Namespace: promptPackKey.Namespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Version: "1.0.0",
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
					},
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())

			By("creating an AgentRuntime")
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRuntimeKey.Name,
					Namespace: agentRuntimeKey.Namespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: promptPackKey.Name,
					},
					Facade: omniav1alpha1.FacadeConfig{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					},
					ProviderSecretRef: corev1.LocalObjectReference{
						Name: "test-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			// First reconcile adds finalizer
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			// Second reconcile creates resources
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying status conditions")
			Eventually(func() bool {
				updated := &omniav1alpha1.AgentRuntime{}
				if err := k8sClient.Get(ctx, agentRuntimeKey, updated); err != nil {
					return false
				}
				promptPackCond := meta.FindStatusCondition(updated.Status.Conditions, ConditionTypePromptPackReady)
				deploymentCond := meta.FindStatusCondition(updated.Status.Conditions, ConditionTypeDeploymentReady)
				serviceCond := meta.FindStatusCondition(updated.Status.Conditions, ConditionTypeServiceReady)
				return promptPackCond != nil && promptPackCond.Status == metav1.ConditionTrue &&
					deploymentCond != nil && deploymentCond.Status == metav1.ConditionTrue &&
					serviceCond != nil && serviceCond.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue())

			By("verifying active version is set")
			updated := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, agentRuntimeKey, updated)).To(Succeed())
			Expect(updated.Status.ActiveVersion).NotTo(BeNil())
			Expect(*updated.Status.ActiveVersion).To(Equal("1.0.0"))
		})

		It("should add and remove finalizer correctly", func() {
			By("creating a PromptPack")
			promptPack := &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptPackKey.Name,
					Namespace: promptPackKey.Namespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Version: "1.0.0",
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
					},
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())

			By("creating an AgentRuntime")
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRuntimeKey.Name,
					Namespace: agentRuntimeKey.Namespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: promptPackKey.Name,
					},
					Facade: omniav1alpha1.FacadeConfig{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					},
					ProviderSecretRef: corev1.LocalObjectReference{
						Name: "test-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling to add finalizer")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("verifying finalizer is added")
			updated := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, agentRuntimeKey, updated)).To(Succeed())
			Expect(updated.Finalizers).To(ContainElement(FinalizerName))

			By("deleting the AgentRuntime")
			Expect(k8sClient.Delete(ctx, updated)).To(Succeed())

			By("reconciling to handle deletion")
			// Refetch to get deletion timestamp
			Expect(k8sClient.Get(ctx, agentRuntimeKey, updated)).To(Succeed())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the resource is deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, agentRuntimeKey, &omniav1alpha1.AgentRuntime{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("should use custom port when specified", func() {
			By("creating a PromptPack")
			promptPack := &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptPackKey.Name,
					Namespace: promptPackKey.Namespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Version: "1.0.0",
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
					},
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())

			By("creating an AgentRuntime with custom port")
			customPort := int32(9090)
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRuntimeKey.Name,
					Namespace: agentRuntimeKey.Namespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: promptPackKey.Name,
					},
					Facade: omniav1alpha1.FacadeConfig{
						Type: omniav1alpha1.FacadeTypeWebSocket,
						Port: &customPort,
					},
					ProviderSecretRef: corev1.LocalObjectReference{
						Name: "test-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying the Deployment uses custom port")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, deployment)
			}, timeout, interval).Should(Succeed())
			Expect(deployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(customPort))

			By("verifying the Service uses custom port")
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, service)
			}, timeout, interval).Should(Succeed())
			Expect(service.Spec.Ports[0].Port).To(Equal(customPort))
		})

		It("should use custom replicas when specified", func() {
			By("creating a PromptPack")
			promptPack := &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptPackKey.Name,
					Namespace: promptPackKey.Namespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Version: "1.0.0",
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
					},
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())

			By("creating an AgentRuntime with custom replicas")
			replicas := int32(3)
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRuntimeKey.Name,
					Namespace: agentRuntimeKey.Namespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: promptPackKey.Name,
					},
					Facade: omniav1alpha1.FacadeConfig{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					},
					Runtime: &omniav1alpha1.RuntimeConfig{
						Replicas: &replicas,
					},
					ProviderSecretRef: corev1.LocalObjectReference{
						Name: "test-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying the Deployment uses custom replicas")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, deployment)
			}, timeout, interval).Should(Succeed())
			Expect(*deployment.Spec.Replicas).To(Equal(replicas))
		})

		It("should set environment variables correctly", func() {
			By("creating a PromptPack")
			promptPack := &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptPackKey.Name,
					Namespace: promptPackKey.Namespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Version: "2.1.0",
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
					},
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())

			By("creating an AgentRuntime")
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRuntimeKey.Name,
					Namespace: agentRuntimeKey.Namespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: promptPackKey.Name,
					},
					Facade: omniav1alpha1.FacadeConfig{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					},
					ProviderSecretRef: corev1.LocalObjectReference{
						Name: "test-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying environment variables are set")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, deployment)
			}, timeout, interval).Should(Succeed())

			envVars := deployment.Spec.Template.Spec.Containers[0].Env
			envMap := make(map[string]corev1.EnvVar)
			for _, env := range envVars {
				envMap[env.Name] = env
			}

			Expect(envMap["OMNIA_AGENT_NAME"].Value).To(Equal(agentRuntimeKey.Name))
			Expect(envMap["OMNIA_NAMESPACE"].Value).To(Equal(agentRuntimeKey.Namespace))
			Expect(envMap["OMNIA_PROMPTPACK_NAME"].Value).To(Equal(promptPackKey.Name))
			Expect(envMap["OMNIA_PROMPTPACK_VERSION"].Value).To(Equal("2.1.0"))
			Expect(envMap["OMNIA_FACADE_TYPE"].Value).To(Equal(string(omniav1alpha1.FacadeTypeWebSocket)))
			Expect(envMap["OMNIA_FACADE_PORT"].Value).To(Equal("8080"))

			// Verify secret reference for API key
			Expect(envMap["OMNIA_PROVIDER_API_KEY"].ValueFrom).NotTo(BeNil())
			Expect(envMap["OMNIA_PROVIDER_API_KEY"].ValueFrom.SecretKeyRef.Name).To(Equal("test-secret"))
			Expect(envMap["OMNIA_PROVIDER_API_KEY"].ValueFrom.SecretKeyRef.Key).To(Equal("api-key"))
		})

		It("should handle non-existent resource gracefully", func() {
			By("reconciling a non-existent AgentRuntime")
			nonExistentKey := types.NamespacedName{
				Name:      "non-existent",
				Namespace: "default",
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nonExistentKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})

		It("should use default image when AgentImage is not set", func() {
			By("creating a PromptPack")
			promptPack := &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptPackKey.Name,
					Namespace: promptPackKey.Namespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Version: "1.0.0",
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
					},
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())

			By("creating an AgentRuntime with reconciler without AgentImage")
			defaultReconciler := &AgentRuntimeReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				// AgentImage not set - should use default
			}

			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRuntimeKey.Name,
					Namespace: agentRuntimeKey.Namespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: promptPackKey.Name,
					},
					Facade: omniav1alpha1.FacadeConfig{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					},
					ProviderSecretRef: corev1.LocalObjectReference{
						Name: "test-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			_, _ = defaultReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = defaultReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying the Deployment uses default image")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, deployment)
			}, timeout, interval).Should(Succeed())
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(DefaultAgentImage))
		})

		It("should set correct labels on Deployment and Service", func() {
			By("creating a PromptPack")
			promptPack := &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptPackKey.Name,
					Namespace: promptPackKey.Namespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Version: "1.0.0",
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
					},
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())

			By("creating an AgentRuntime")
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRuntimeKey.Name,
					Namespace: agentRuntimeKey.Namespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: promptPackKey.Name,
					},
					Facade: omniav1alpha1.FacadeConfig{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					},
					ProviderSecretRef: corev1.LocalObjectReference{
						Name: "test-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying Deployment labels")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, deployment)
			}, timeout, interval).Should(Succeed())

			Expect(deployment.Labels["app.kubernetes.io/name"]).To(Equal("omnia-agent"))
			Expect(deployment.Labels["app.kubernetes.io/instance"]).To(Equal(agentRuntimeKey.Name))
			Expect(deployment.Labels["app.kubernetes.io/managed-by"]).To(Equal("omnia-operator"))

			By("verifying Service labels")
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, service)
			}, timeout, interval).Should(Succeed())

			Expect(service.Labels["app.kubernetes.io/name"]).To(Equal("omnia-agent"))
			Expect(service.Labels["app.kubernetes.io/instance"]).To(Equal(agentRuntimeKey.Name))
			Expect(service.Labels["app.kubernetes.io/managed-by"]).To(Equal("omnia-operator"))
		})

		It("should handle ToolRegistry reference", func() {
			By("creating a PromptPack")
			promptPack := &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptPackKey.Name,
					Namespace: promptPackKey.Namespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Version: "1.0.0",
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
					},
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())

			By("creating a ToolRegistry")
			toolRegistryKey := types.NamespacedName{
				Name:      "test-toolregistry",
				Namespace: "default",
			}
			toolURL := "http://tool.example.com"
			toolRegistry := &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      toolRegistryKey.Name,
					Namespace: toolRegistryKey.Namespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Tools: []omniav1alpha1.ToolDefinition{
						{
							Name: "test-tool",
							Type: omniav1alpha1.ToolTypeHTTP,
							Endpoint: omniav1alpha1.ToolEndpoint{
								URL: &toolURL,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, toolRegistry)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, toolRegistry) }()

			By("creating an AgentRuntime with ToolRegistry reference")
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRuntimeKey.Name,
					Namespace: agentRuntimeKey.Namespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: promptPackKey.Name,
					},
					Facade: omniav1alpha1.FacadeConfig{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					},
					ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{
						Name: toolRegistryKey.Name,
					},
					ProviderSecretRef: corev1.LocalObjectReference{
						Name: "test-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying ToolRegistry environment variables")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, deployment)
			}, timeout, interval).Should(Succeed())

			envVars := deployment.Spec.Template.Spec.Containers[0].Env
			envMap := make(map[string]corev1.EnvVar)
			for _, env := range envVars {
				envMap[env.Name] = env
			}

			Expect(envMap["OMNIA_TOOLREGISTRY_NAME"].Value).To(Equal(toolRegistryKey.Name))
			Expect(envMap["OMNIA_TOOLREGISTRY_NAMESPACE"].Value).To(Equal(toolRegistryKey.Namespace))

			By("verifying ToolRegistryReady condition")
			updated := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, agentRuntimeKey, updated)).To(Succeed())
			toolRegistryCond := meta.FindStatusCondition(updated.Status.Conditions, ConditionTypeToolRegistryReady)
			Expect(toolRegistryCond).NotTo(BeNil())
			Expect(toolRegistryCond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should handle missing ToolRegistry gracefully", func() {
			By("creating a PromptPack")
			promptPack := &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptPackKey.Name,
					Namespace: promptPackKey.Namespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Version: "1.0.0",
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
					},
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())

			By("creating an AgentRuntime with non-existent ToolRegistry reference")
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRuntimeKey.Name,
					Namespace: agentRuntimeKey.Namespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: promptPackKey.Name,
					},
					Facade: omniav1alpha1.FacadeConfig{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					},
					ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{
						Name: "nonexistent-toolregistry",
					},
					ProviderSecretRef: corev1.LocalObjectReference{
						Name: "test-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime - should succeed despite missing ToolRegistry")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			// ToolRegistry is optional, so reconciliation should still succeed
			Expect(err).NotTo(HaveOccurred())

			By("verifying ToolRegistryReady condition is False")
			updated := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, agentRuntimeKey, updated)).To(Succeed())
			toolRegistryCond := meta.FindStatusCondition(updated.Status.Conditions, ConditionTypeToolRegistryReady)
			Expect(toolRegistryCond).NotTo(BeNil())
			Expect(toolRegistryCond.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should handle session config with TTL", func() {
			By("creating a PromptPack")
			promptPack := &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptPackKey.Name,
					Namespace: promptPackKey.Namespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Version: "1.0.0",
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
					},
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())

			By("creating an AgentRuntime with session config")
			sessionTTL := "1h"
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRuntimeKey.Name,
					Namespace: agentRuntimeKey.Namespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: promptPackKey.Name,
					},
					Facade: omniav1alpha1.FacadeConfig{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					},
					Session: &omniav1alpha1.SessionConfig{
						Type: omniav1alpha1.SessionStoreTypeRedis,
						TTL:  &sessionTTL,
						StoreRef: &corev1.LocalObjectReference{
							Name: "redis-secret",
						},
					},
					ProviderSecretRef: corev1.LocalObjectReference{
						Name: "test-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying session environment variables")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, deployment)
			}, timeout, interval).Should(Succeed())

			envVars := deployment.Spec.Template.Spec.Containers[0].Env
			envMap := make(map[string]corev1.EnvVar)
			for _, env := range envVars {
				envMap[env.Name] = env
			}

			Expect(envMap["OMNIA_SESSION_TYPE"].Value).To(Equal(string(omniav1alpha1.SessionStoreTypeRedis)))
			Expect(envMap["OMNIA_SESSION_TTL"].Value).To(Equal("1h"))
			Expect(envMap["OMNIA_SESSION_STORE_URL"].ValueFrom).NotTo(BeNil())
			Expect(envMap["OMNIA_SESSION_STORE_URL"].ValueFrom.SecretKeyRef.Name).To(Equal("redis-secret"))
			Expect(envMap["OMNIA_SESSION_STORE_URL"].ValueFrom.SecretKeyRef.Key).To(Equal("url"))
		})

		It("should mount ConfigMap volume for PromptPack", func() {
			By("creating a PromptPack with ConfigMap source")
			promptPack := &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptPackKey.Name,
					Namespace: promptPackKey.Namespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Version: "1.0.0",
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: "prompts-config",
						},
					},
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())

			By("creating an AgentRuntime")
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRuntimeKey.Name,
					Namespace: agentRuntimeKey.Namespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: promptPackKey.Name,
					},
					Facade: omniav1alpha1.FacadeConfig{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					},
					ProviderSecretRef: corev1.LocalObjectReference{
						Name: "test-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying ConfigMap volume is mounted")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, deployment)
			}, timeout, interval).Should(Succeed())

			// Check volume mounts
			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(1))
			Expect(container.VolumeMounts[0].Name).To(Equal("promptpack-config"))
			Expect(container.VolumeMounts[0].MountPath).To(Equal("/etc/omnia/prompts"))
			Expect(container.VolumeMounts[0].ReadOnly).To(BeTrue())

			// Check volumes
			Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Volumes[0].Name).To(Equal("promptpack-config"))
			Expect(deployment.Spec.Template.Spec.Volumes[0].ConfigMap.Name).To(Equal("prompts-config"))
		})

		It("should handle ToolRegistry in different namespace", func() {
			By("creating a PromptPack")
			promptPack := &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      promptPackKey.Name,
					Namespace: promptPackKey.Namespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Version: "1.0.0",
					Source: omniav1alpha1.PromptPackSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
					},
					Rollout: omniav1alpha1.RolloutStrategy{
						Type: omniav1alpha1.RolloutStrategyImmediate,
					},
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())

			By("creating an AgentRuntime with cross-namespace ToolRegistry reference")
			otherNS := "other-namespace"
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRuntimeKey.Name,
					Namespace: agentRuntimeKey.Namespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: promptPackKey.Name,
					},
					Facade: omniav1alpha1.FacadeConfig{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					},
					ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{
						Name:      "cross-ns-toolregistry",
						Namespace: &otherNS,
					},
					ProviderSecretRef: corev1.LocalObjectReference{
						Name: "test-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling - should fail to find ToolRegistry in other namespace")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			// ToolRegistry is optional, so reconciliation should still succeed
			Expect(err).NotTo(HaveOccurred())

			By("verifying ToolRegistryReady condition reflects the failure")
			updated := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, agentRuntimeKey, updated)).To(Succeed())
			toolRegistryCond := meta.FindStatusCondition(updated.Status.Conditions, ConditionTypeToolRegistryReady)
			Expect(toolRegistryCond).NotTo(BeNil())
			Expect(toolRegistryCond.Status).To(Equal(metav1.ConditionFalse))
		})
	})
})
