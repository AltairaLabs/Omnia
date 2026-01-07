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
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

var _ = Describe("AgentRuntime Controller", func() {
	const (
		timeout         = time.Second * 10
		interval        = time.Millisecond * 250
		anthropicAPIKey = "ANTHROPIC_API_KEY"
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
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				FacadeImage:  "test-facade:v1.0.0",
				RuntimeImage: "test-runtime:v1.0.0",
			}
		})

		AfterEach(func() {
			// Clean up HPA
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			err := k8sClient.Get(ctx, agentRuntimeKey, hpa)
			if err == nil {
				_ = k8sClient.Delete(ctx, hpa)
			}

			// Clean up AgentRuntime
			agentRuntime := &omniav1alpha1.AgentRuntime{}
			err = k8sClient.Get(ctx, agentRuntimeKey, agentRuntime)
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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
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

			By("verifying both facade and runtime containers exist")
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(2))

			// Find facade container
			var facadeContainer, runtimeContainer *corev1.Container
			for i := range deployment.Spec.Template.Spec.Containers {
				c := &deployment.Spec.Template.Spec.Containers[i]
				switch c.Name {
				case FacadeContainerName:
					facadeContainer = c
				case RuntimeContainerName:
					runtimeContainer = c
				}
			}
			Expect(facadeContainer).NotTo(BeNil(), "facade container should exist")
			Expect(runtimeContainer).NotTo(BeNil(), "runtime container should exist")

			Expect(facadeContainer.Image).To(Equal("test-facade:v1.0.0"))
			Expect(facadeContainer.Ports).To(HaveLen(2)) // facade port + health port
			Expect(facadeContainer.Ports[0].ContainerPort).To(Equal(int32(DefaultFacadePort)))

			Expect(runtimeContainer.Image).To(Equal("test-runtime:v1.0.0"))
			Expect(runtimeContainer.Ports).To(HaveLen(2)) // gRPC port + health port
			Expect(runtimeContainer.Ports[0].ContainerPort).To(Equal(int32(DefaultRuntimeGRPCPort)))

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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
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

			By("creating an AgentRuntime with claude provider")
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
					Provider: &omniav1alpha1.ProviderConfig{
						Type: omniav1alpha1.ProviderTypeClaude,
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
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

			// Find facade and runtime containers
			var facadeContainer, runtimeContainer *corev1.Container
			for i := range deployment.Spec.Template.Spec.Containers {
				c := &deployment.Spec.Template.Spec.Containers[i]
				switch c.Name {
				case FacadeContainerName:
					facadeContainer = c
				case RuntimeContainerName:
					runtimeContainer = c
				}
			}
			Expect(facadeContainer).NotTo(BeNil())
			Expect(runtimeContainer).NotTo(BeNil())

			// Check facade container env vars
			facadeEnvMap := make(map[string]corev1.EnvVar)
			for _, env := range facadeContainer.Env {
				facadeEnvMap[env.Name] = env
			}
			Expect(facadeEnvMap["OMNIA_AGENT_NAME"].Value).To(Equal(agentRuntimeKey.Name))
			Expect(facadeEnvMap["OMNIA_NAMESPACE"].Value).To(Equal(agentRuntimeKey.Namespace))
			Expect(facadeEnvMap["OMNIA_PROMPTPACK_NAME"].Value).To(Equal(promptPackKey.Name))
			Expect(facadeEnvMap["OMNIA_PROMPTPACK_VERSION"].Value).To(Equal("2.1.0"))
			Expect(facadeEnvMap["OMNIA_FACADE_TYPE"].Value).To(Equal(string(omniav1alpha1.FacadeTypeWebSocket)))
			Expect(facadeEnvMap["OMNIA_FACADE_PORT"].Value).To(Equal("8080"))
			Expect(facadeEnvMap["OMNIA_HANDLER_MODE"].Value).To(Equal("runtime"))
			Expect(facadeEnvMap["OMNIA_RUNTIME_ADDRESS"].Value).To(Equal("localhost:9000"))

			// Check runtime container env vars - API key should be on runtime
			runtimeEnvMap := make(map[string]corev1.EnvVar)
			for _, env := range runtimeContainer.Env {
				runtimeEnvMap[env.Name] = env
			}
			Expect(runtimeEnvMap["OMNIA_AGENT_NAME"].Value).To(Equal(agentRuntimeKey.Name))
			Expect(runtimeEnvMap["OMNIA_GRPC_PORT"].Value).To(Equal("9000"))
			// Provider type should be set
			Expect(runtimeEnvMap["OMNIA_PROVIDER_TYPE"].Value).To(Equal("claude"))
			// For claude provider, check ANTHROPIC_API_KEY from secret
			// The env var may appear twice (one with key matching env name, one with api-key fallback)
			var foundAnthropicKey bool
			for _, env := range runtimeContainer.Env {
				if env.Name == anthropicAPIKey && env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal("test-secret"))
					foundAnthropicKey = true
				}
			}
			Expect(foundAnthropicKey).To(BeTrue(), "Expected ANTHROPIC_API_KEY env var from secret")
		})

		It("should respect the facade handler mode when specified", func() {
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

			By("creating an AgentRuntime with demo handler mode")
			demoMode := omniav1alpha1.HandlerModeDemo
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
						Type:    omniav1alpha1.FacadeTypeWebSocket,
						Handler: &demoMode,
					},
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying the facade container has demo handler mode")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, deployment)
			}, timeout, interval).Should(Succeed())

			var facadeContainer *corev1.Container
			for i := range deployment.Spec.Template.Spec.Containers {
				c := &deployment.Spec.Template.Spec.Containers[i]
				if c.Name == FacadeContainerName {
					facadeContainer = c
					break
				}
			}
			Expect(facadeContainer).NotTo(BeNil())

			facadeEnvMap := make(map[string]corev1.EnvVar)
			for _, env := range facadeContainer.Env {
				facadeEnvMap[env.Name] = env
			}

			// Handler mode should be "demo"
			Expect(facadeEnvMap["OMNIA_HANDLER_MODE"].Value).To(Equal("demo"))

			// Runtime address should NOT be set for non-runtime handlers
			_, hasRuntimeAddress := facadeEnvMap["OMNIA_RUNTIME_ADDRESS"]
			Expect(hasRuntimeAddress).To(BeFalse(), "OMNIA_RUNTIME_ADDRESS should not be set for demo handler")
		})

		It("should set all provider configuration environment variables", func() {
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

			By("creating an AgentRuntime with full provider config")
			temperature := "0.7"
			topP := "0.9"
			maxTokens := int32(4096)
			inputCost := "0.003"
			outputCost := "0.015"
			cachedCost := "0.001"
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
					Provider: &omniav1alpha1.ProviderConfig{
						Type:    omniav1alpha1.ProviderTypeOpenAI,
						Model:   "gpt-4o",
						BaseURL: "https://api.openai.com/v1",
						SecretRef: &corev1.LocalObjectReference{
							Name: "openai-secret",
						},
						Config: &omniav1alpha1.ProviderDefaults{
							Temperature: &temperature,
							TopP:        &topP,
							MaxTokens:   &maxTokens,
						},
						Pricing: &omniav1alpha1.ProviderPricing{
							InputCostPer1K:  &inputCost,
							OutputCostPer1K: &outputCost,
							CachedCostPer1K: &cachedCost,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying all provider environment variables are set")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, deployment)
			}, timeout, interval).Should(Succeed())

			// Find runtime container
			var runtimeContainer *corev1.Container
			for i := range deployment.Spec.Template.Spec.Containers {
				c := &deployment.Spec.Template.Spec.Containers[i]
				if c.Name == RuntimeContainerName {
					runtimeContainer = c
					break
				}
			}
			Expect(runtimeContainer).NotTo(BeNil())

			// Build env var map
			runtimeEnvMap := make(map[string]corev1.EnvVar)
			for _, env := range runtimeContainer.Env {
				runtimeEnvMap[env.Name] = env
			}

			// Check provider type
			Expect(runtimeEnvMap["OMNIA_PROVIDER_TYPE"].Value).To(Equal("openai"))

			// Check model
			Expect(runtimeEnvMap["OMNIA_PROVIDER_MODEL"].Value).To(Equal("gpt-4o"))

			// Check base URL
			Expect(runtimeEnvMap["OMNIA_PROVIDER_BASE_URL"].Value).To(Equal("https://api.openai.com/v1"))

			// Check provider config
			Expect(runtimeEnvMap["OMNIA_PROVIDER_TEMPERATURE"].Value).To(Equal("0.7"))
			Expect(runtimeEnvMap["OMNIA_PROVIDER_TOP_P"].Value).To(Equal("0.9"))
			Expect(runtimeEnvMap["OMNIA_PROVIDER_MAX_TOKENS"].Value).To(Equal("4096"))

			// Check pricing config
			Expect(runtimeEnvMap["OMNIA_PROVIDER_INPUT_COST"].Value).To(Equal("0.003"))
			Expect(runtimeEnvMap["OMNIA_PROVIDER_OUTPUT_COST"].Value).To(Equal("0.015"))
			Expect(runtimeEnvMap["OMNIA_PROVIDER_CACHED_COST"].Value).To(Equal("0.001"))

			// Check OpenAI API key is injected
			var foundOpenAIKey bool
			for _, env := range runtimeContainer.Env {
				if env.Name == "OPENAI_API_KEY" && env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal("openai-secret"))
					foundOpenAIKey = true
				}
			}
			Expect(foundOpenAIKey).To(BeTrue(), "Expected OPENAI_API_KEY env var from secret")
		})

		It("should handle nil provider config gracefully", func() {
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

			By("creating an AgentRuntime without provider config")
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
					// No Provider config - should default to auto
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying default provider type is set")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, deployment)
			}, timeout, interval).Should(Succeed())

			// Find runtime container
			var runtimeContainer *corev1.Container
			for i := range deployment.Spec.Template.Spec.Containers {
				c := &deployment.Spec.Template.Spec.Containers[i]
				if c.Name == RuntimeContainerName {
					runtimeContainer = c
					break
				}
			}
			Expect(runtimeContainer).NotTo(BeNil())

			// Build env var map
			runtimeEnvMap := make(map[string]corev1.EnvVar)
			for _, env := range runtimeContainer.Env {
				runtimeEnvMap[env.Name] = env
			}

			// Check provider type defaults to auto
			Expect(runtimeEnvMap["OMNIA_PROVIDER_TYPE"].Value).To(Equal("auto"))

			// Optional fields should not be present
			_, hasModel := runtimeEnvMap["OMNIA_PROVIDER_MODEL"]
			Expect(hasModel).To(BeFalse())
			_, hasBaseURL := runtimeEnvMap["OMNIA_PROVIDER_BASE_URL"]
			Expect(hasBaseURL).To(BeFalse())
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

		It("should use default images when FacadeImage and RuntimeImage are not set", func() {
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

			By("creating an AgentRuntime with reconciler without images set")
			defaultReconciler := &AgentRuntimeReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				// FacadeImage and RuntimeImage not set - should use defaults
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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			_, _ = defaultReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = defaultReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying the Deployment uses default images")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, deployment)
			}, timeout, interval).Should(Succeed())

			// Find containers by name
			var facadeContainer, runtimeContainer *corev1.Container
			for i := range deployment.Spec.Template.Spec.Containers {
				c := &deployment.Spec.Template.Spec.Containers[i]
				switch c.Name {
				case FacadeContainerName:
					facadeContainer = c
				case RuntimeContainerName:
					runtimeContainer = c
				}
			}
			Expect(facadeContainer).NotTo(BeNil())
			Expect(runtimeContainer).NotTo(BeNil())
			Expect(facadeContainer.Image).To(Equal(DefaultFacadeImage))
			Expect(runtimeContainer.Image).To(Equal(DefaultRuntimeImage))
		})

		It("should use CRD image overrides when specified", func() {
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

			By("creating an AgentRuntime with CRD image overrides")
			customFacadeImage := "my-registry.io/custom-facade:v1.0.0"
			customRuntimeImage := "my-registry.io/custom-runtime:v2.0.0"

			// Reconciler has operator-level defaults set
			reconcilerWithDefaults := &AgentRuntimeReconciler{
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				FacadeImage:  "operator-default-facade:latest",
				RuntimeImage: "operator-default-runtime:latest",
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
						Type:  omniav1alpha1.FacadeTypeWebSocket,
						Image: customFacadeImage, // CRD override
					},
					Framework: &omniav1alpha1.FrameworkConfig{
						Type:  omniav1alpha1.FrameworkTypeCustom,
						Image: customRuntimeImage, // CRD override
					},
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			_, _ = reconcilerWithDefaults.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = reconcilerWithDefaults.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying the Deployment uses CRD image overrides, not operator defaults")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, deployment)
			}, timeout, interval).Should(Succeed())

			// Find containers by name
			var facadeContainer, runtimeContainer *corev1.Container
			for i := range deployment.Spec.Template.Spec.Containers {
				c := &deployment.Spec.Template.Spec.Containers[i]
				switch c.Name {
				case FacadeContainerName:
					facadeContainer = c
				case RuntimeContainerName:
					runtimeContainer = c
				}
			}
			Expect(facadeContainer).NotTo(BeNil())
			Expect(runtimeContainer).NotTo(BeNil())
			Expect(facadeContainer.Image).To(Equal(customFacadeImage), "Facade should use CRD override")
			Expect(runtimeContainer.Image).To(Equal(customRuntimeImage), "Runtime should use CRD override")
		})

		It("should allow partial CRD image overrides (facade only)", func() {
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

			By("creating an AgentRuntime with only facade image override")
			customFacadeImage := "my-registry.io/partial-facade:v3.0.0"
			operatorRuntimeImage := "operator-runtime:latest"

			reconcilerWithDefaults := &AgentRuntimeReconciler{
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				FacadeImage:  "operator-facade:latest",
				RuntimeImage: operatorRuntimeImage,
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
						Type:  omniav1alpha1.FacadeTypeWebSocket,
						Image: customFacadeImage, // Only facade is overridden
					},
					// Framework.Image is NOT set - should fall back to operator default
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			_, _ = reconcilerWithDefaults.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = reconcilerWithDefaults.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying the Deployment uses mixed images")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, deployment)
			}, timeout, interval).Should(Succeed())

			// Find containers by name
			var facadeContainer, runtimeContainer *corev1.Container
			for i := range deployment.Spec.Template.Spec.Containers {
				c := &deployment.Spec.Template.Spec.Containers[i]
				switch c.Name {
				case FacadeContainerName:
					facadeContainer = c
				case RuntimeContainerName:
					runtimeContainer = c
				}
			}
			Expect(facadeContainer).NotTo(BeNil())
			Expect(runtimeContainer).NotTo(BeNil())
			Expect(facadeContainer.Image).To(Equal(customFacadeImage), "Facade should use CRD override")
			Expect(runtimeContainer.Image).To(Equal(operatorRuntimeImage), "Runtime should use operator default")
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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
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
			toolRegistry := &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      toolRegistryKey.Name,
					Namespace: toolRegistryKey.Namespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "test-handler",
							Type: omniav1alpha1.HandlerTypeHTTP,
							HTTPConfig: &omniav1alpha1.HTTPConfig{
								Endpoint: "http://tool.example.com",
							},
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "test_tool",
								Description: "A test tool",
								InputSchema: apiextensionsv1.JSON{
									Raw: []byte(`{"type":"object"}`),
								},
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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
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

			// Find runtime container (ToolRegistry env vars are on runtime)
			var runtimeContainer *corev1.Container
			for i := range deployment.Spec.Template.Spec.Containers {
				c := &deployment.Spec.Template.Spec.Containers[i]
				if c.Name == RuntimeContainerName {
					runtimeContainer = c
					break
				}
			}
			Expect(runtimeContainer).NotTo(BeNil())

			envMap := make(map[string]corev1.EnvVar)
			for _, env := range runtimeContainer.Env {
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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
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

			// Find runtime container (volume mounts are on runtime)
			var runtimeContainer *corev1.Container
			for i := range deployment.Spec.Template.Spec.Containers {
				c := &deployment.Spec.Template.Spec.Containers[i]
				if c.Name == RuntimeContainerName {
					runtimeContainer = c
					break
				}
			}
			Expect(runtimeContainer).NotTo(BeNil())

			// Check volume mounts on runtime container
			Expect(runtimeContainer.VolumeMounts).To(HaveLen(1))
			Expect(runtimeContainer.VolumeMounts[0].Name).To(Equal("promptpack-config"))
			Expect(runtimeContainer.VolumeMounts[0].MountPath).To(Equal(PromptPackMountPath))
			Expect(runtimeContainer.VolumeMounts[0].ReadOnly).To(BeTrue())

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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
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

		It("should create HPA when HPA autoscaling is enabled", func() {
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

			By("creating an AgentRuntime with HPA autoscaling")
			minReplicas := int32(2)
			maxReplicas := int32(8)
			targetCPU := int32(75)
			targetMemory := int32(80)
			scaleDownStabilization := int32(120)

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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
					},
					Runtime: &omniav1alpha1.RuntimeConfig{
						Autoscaling: &omniav1alpha1.AutoscalingConfig{
							Enabled:                           true,
							Type:                              omniav1alpha1.AutoscalerTypeHPA,
							MinReplicas:                       &minReplicas,
							MaxReplicas:                       &maxReplicas,
							TargetCPUUtilizationPercentage:    &targetCPU,
							TargetMemoryUtilizationPercentage: &targetMemory,
							ScaleDownStabilizationSeconds:     &scaleDownStabilization,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying HPA was created")
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, hpa)
			}, timeout, interval).Should(Succeed())

			Expect(hpa.Spec.ScaleTargetRef.Name).To(Equal(agentRuntimeKey.Name))
			Expect(hpa.Spec.ScaleTargetRef.Kind).To(Equal("Deployment"))
			Expect(*hpa.Spec.MinReplicas).To(Equal(minReplicas))
			Expect(hpa.Spec.MaxReplicas).To(Equal(maxReplicas))

			// Verify metrics
			Expect(hpa.Spec.Metrics).To(HaveLen(2))

			// Verify behavior
			Expect(hpa.Spec.Behavior).NotTo(BeNil())
			Expect(hpa.Spec.Behavior.ScaleDown).NotTo(BeNil())
			Expect(*hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds).To(Equal(scaleDownStabilization))
		})

		It("should use default HPA values when not specified", func() {
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

			By("creating an AgentRuntime with minimal HPA config")
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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
					},
					Runtime: &omniav1alpha1.RuntimeConfig{
						Autoscaling: &omniav1alpha1.AutoscalingConfig{
							Enabled: true,
							Type:    omniav1alpha1.AutoscalerTypeHPA,
							// All other values should use defaults
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying HPA has default values")
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, hpa)
			}, timeout, interval).Should(Succeed())

			// Defaults: minReplicas=1, maxReplicas=10
			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(1)))
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(10)))

			// Default scaleDown stabilization = 300 seconds
			Expect(*hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds).To(Equal(int32(300)))
		})

		It("should clean up HPA when autoscaling is disabled", func() {
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

			By("creating an AgentRuntime with HPA")
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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
					},
					Runtime: &omniav1alpha1.RuntimeConfig{
						Autoscaling: &omniav1alpha1.AutoscalingConfig{
							Enabled: true,
							Type:    omniav1alpha1.AutoscalerTypeHPA,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling to create HPA")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying HPA exists")
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, hpa)
			}, timeout, interval).Should(Succeed())

			By("disabling autoscaling")
			updated := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, agentRuntimeKey, updated)).To(Succeed())
			updated.Spec.Runtime.Autoscaling.Enabled = false
			Expect(k8sClient.Update(ctx, updated)).To(Succeed())

			By("reconciling again")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying HPA was deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, agentRuntimeKey, hpa)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("should clean up HPA when Runtime is nil", func() {
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

			By("creating an AgentRuntime without runtime config")
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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
					},
					// Runtime is nil - should not create HPA
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying no HPA was created")
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			err = k8sClient.Get(ctx, agentRuntimeKey, hpa)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should handle KEDA autoscaling type gracefully when KEDA is not installed", func() {
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

			By("creating an AgentRuntime with KEDA autoscaling")
			minReplicas := int32(0)
			maxReplicas := int32(5)
			pollingInterval := int32(15)
			cooldownPeriod := int32(60)

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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
					},
					Runtime: &omniav1alpha1.RuntimeConfig{
						Autoscaling: &omniav1alpha1.AutoscalingConfig{
							Enabled:     true,
							Type:        omniav1alpha1.AutoscalerTypeKEDA,
							MinReplicas: &minReplicas,
							MaxReplicas: &maxReplicas,
							KEDA: &omniav1alpha1.KEDAConfig{
								PollingInterval: &pollingInterval,
								CooldownPeriod:  &cooldownPeriod,
								Triggers: []omniav1alpha1.KEDATrigger{
									{
										Type: "prometheus",
										Metadata: map[string]string{
											"serverAddress": "http://prometheus:9090",
											"query":         "test_metric",
											"threshold":     "5",
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling the AgentRuntime - should not fail even though KEDA is not installed")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			// The reconciliation should succeed because autoscaling errors are logged but don't fail reconciliation
			Expect(err).NotTo(HaveOccurred())

			By("verifying Deployment was still created")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, deployment)
			}, timeout, interval).Should(Succeed())
		})

		It("should handle switching from KEDA to HPA autoscaling", func() {
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

			By("creating an AgentRuntime with KEDA autoscaling first")
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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
					},
					Runtime: &omniav1alpha1.RuntimeConfig{
						Autoscaling: &omniav1alpha1.AutoscalingConfig{
							Enabled: true,
							Type:    omniav1alpha1.AutoscalerTypeKEDA,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling with KEDA type")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("switching to HPA autoscaling")
			updated := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, agentRuntimeKey, updated)).To(Succeed())
			updated.Spec.Runtime.Autoscaling.Type = omniav1alpha1.AutoscalerTypeHPA
			Expect(k8sClient.Update(ctx, updated)).To(Succeed())

			By("reconciling again - should create HPA")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying HPA was created")
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, hpa)
			}, timeout, interval).Should(Succeed())
		})

		It("should return early when AgentRuntime is not found", func() {
			By("reconciling a non-existent AgentRuntime")
			nonExistentKey := types.NamespacedName{
				Name:      "non-existent-agent",
				Namespace: "default",
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nonExistentKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})

		It("should handle deletion with finalizer", func() {
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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling to add finalizer")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying finalizer was added")
			updated := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, agentRuntimeKey, updated)).To(Succeed())
			Expect(updated.Finalizers).To(ContainElement(FinalizerName))

			By("deleting the AgentRuntime")
			Expect(k8sClient.Delete(ctx, updated)).To(Succeed())

			By("reconciling the deletion")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying AgentRuntime is gone")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, agentRuntimeKey, &omniav1alpha1.AgentRuntime{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("should update HPA when autoscaling config changes", func() {
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

			By("creating an AgentRuntime with HPA")
			minReplicas := int32(1)
			maxReplicas := int32(5)
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
					Provider: &omniav1alpha1.ProviderConfig{
						SecretRef: &corev1.LocalObjectReference{
							Name: "test-secret",
						},
					},
					Runtime: &omniav1alpha1.RuntimeConfig{
						Autoscaling: &omniav1alpha1.AutoscalingConfig{
							Enabled:     true,
							Type:        omniav1alpha1.AutoscalerTypeHPA,
							MinReplicas: &minReplicas,
							MaxReplicas: &maxReplicas,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			By("reconciling to create HPA")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})

			By("verifying initial HPA config")
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			Eventually(func() error {
				return k8sClient.Get(ctx, agentRuntimeKey, hpa)
			}, timeout, interval).Should(Succeed())
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(5)))

			By("updating autoscaling config")
			updated := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, agentRuntimeKey, updated)).To(Succeed())
			newMaxReplicas := int32(15)
			updated.Spec.Runtime.Autoscaling.MaxReplicas = &newMaxReplicas
			Expect(k8sClient.Update(ctx, updated)).To(Succeed())

			By("reconciling again")
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying HPA was updated")
			Eventually(func() int32 {
				hpa := &autoscalingv2.HorizontalPodAutoscaler{}
				if err := k8sClient.Get(ctx, agentRuntimeKey, hpa); err != nil {
					return 0
				}
				return hpa.Spec.MaxReplicas
			}, timeout, interval).Should(Equal(int32(15)))
		})
	})

	Context("When using providerRef", func() {
		var (
			ctx             context.Context
			agentRuntimeKey types.NamespacedName
			promptPackKey   types.NamespacedName
			providerKey     types.NamespacedName
			reconciler      *AgentRuntimeReconciler
		)

		BeforeEach(func() {
			ctx = context.Background()
			agentRuntimeKey = types.NamespacedName{
				Name:      "test-provider-ref-agent",
				Namespace: "default",
			}
			promptPackKey = types.NamespacedName{
				Name:      "test-provider-ref-promptpack",
				Namespace: "default",
			}
			providerKey = types.NamespacedName{
				Name:      "test-provider-ref",
				Namespace: "default",
			}
			reconciler = &AgentRuntimeReconciler{
				Client:       k8sClient,
				Scheme:       k8sClient.Scheme(),
				FacadeImage:  "test-facade:latest",
				RuntimeImage: "test-runtime:latest",
			}
		})

		AfterEach(func() {
			// Clean up all resources
			agentRuntime := &omniav1alpha1.AgentRuntime{}
			if err := k8sClient.Get(ctx, agentRuntimeKey, agentRuntime); err == nil {
				_ = k8sClient.Delete(ctx, agentRuntime)
			}
			promptPack := &omniav1alpha1.PromptPack{}
			if err := k8sClient.Get(ctx, promptPackKey, promptPack); err == nil {
				_ = k8sClient.Delete(ctx, promptPack)
			}
			provider := &omniav1alpha1.Provider{}
			if err := k8sClient.Get(ctx, providerKey, provider); err == nil {
				_ = k8sClient.Delete(ctx, provider)
			}
			configMap := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: promptPackKey.Name + "-prompts", Namespace: promptPackKey.Namespace}, configMap); err == nil {
				_ = k8sClient.Delete(ctx, configMap)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: "provider-secret", Namespace: "default"}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should fetch provider from same namespace", func() {
			By("creating the secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "provider-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"ANTHROPIC_API_KEY": []byte("test-key"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("creating the Provider")
			provider := &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerKey.Name,
					Namespace: providerKey.Namespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					SecretRef: omniav1alpha1.SecretKeyRef{
						Name: "provider-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			By("creating the AgentRuntime with providerRef")
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRuntimeKey.Name,
					Namespace: agentRuntimeKey.Namespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					ProviderRef: &omniav1alpha1.ProviderRef{
						Name: providerKey.Name,
					},
				},
			}

			fetchedProvider, err := reconciler.fetchProvider(ctx, agentRuntime)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetchedProvider).NotTo(BeNil())
			Expect(fetchedProvider.Spec.Type).To(Equal(omniav1alpha1.ProviderTypeClaude))
			Expect(fetchedProvider.Spec.Model).To(Equal("claude-sonnet-4-20250514"))
		})

		It("should fetch provider from different namespace", func() {
			By("creating a namespace for cross-namespace test")
			otherNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "provider-ns",
				},
			}
			// Ignore error if namespace exists
			_ = k8sClient.Create(ctx, otherNs)

			By("creating the secret in other namespace")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "provider-secret",
					Namespace: "provider-ns",
				},
				Data: map[string][]byte{
					"ANTHROPIC_API_KEY": []byte("test-key"),
				},
			}
			// Ignore error if secret exists
			_ = k8sClient.Create(ctx, secret)

			By("creating the Provider in other namespace")
			crossNsProvider := &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cross-ns-provider",
					Namespace: "provider-ns",
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeOpenAI,
					Model: "gpt-4o",
					SecretRef: omniav1alpha1.SecretKeyRef{
						Name: "provider-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, crossNsProvider)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, crossNsProvider) }()

			By("creating the AgentRuntime with cross-namespace providerRef")
			crossNs := "provider-ns"
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRuntimeKey.Name,
					Namespace: agentRuntimeKey.Namespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					ProviderRef: &omniav1alpha1.ProviderRef{
						Name:      "cross-ns-provider",
						Namespace: &crossNs,
					},
				},
			}

			fetchedProvider, err := reconciler.fetchProvider(ctx, agentRuntime)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetchedProvider).NotTo(BeNil())
			Expect(fetchedProvider.Spec.Type).To(Equal(omniav1alpha1.ProviderTypeOpenAI))
			Expect(fetchedProvider.Spec.Model).To(Equal("gpt-4o"))
		})

		It("should fail when provider does not exist", func() {
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentRuntimeKey.Name,
					Namespace: agentRuntimeKey.Namespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					ProviderRef: &omniav1alpha1.ProviderRef{
						Name: "nonexistent-provider",
					},
				},
			}

			_, err := reconciler.fetchProvider(ctx, agentRuntime)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get Provider"))
		})

	})
})

// Test helper functions and buildKEDATriggers (unit tests, no envtest required)
var _ = Describe("AgentRuntime Controller Unit Tests", func() {
	const (
		anthropicAPIKey = "ANTHROPIC_API_KEY"
	)

	Describe("ptr helper function", func() {
		It("should return a pointer to an int32 value", func() {
			val := int32(42)
			result := ptr(val)
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(int32(42)))
		})

		It("should return a pointer to a bool value", func() {
			val := true
			result := ptr(val)
			Expect(result).NotTo(BeNil())
			Expect(*result).To(BeTrue())
		})

		It("should return a pointer to a string value", func() {
			val := "test"
			result := ptr(val)
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal("test"))
		})
	})

	Describe("ptrSelectPolicy helper function", func() {
		It("should return a pointer to MaxChangePolicySelect", func() {
			result := ptrSelectPolicy(autoscalingv2.MaxChangePolicySelect)
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(autoscalingv2.MaxChangePolicySelect))
		})

		It("should return a pointer to MinChangePolicySelect", func() {
			result := ptrSelectPolicy(autoscalingv2.MinChangePolicySelect)
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(autoscalingv2.MinChangePolicySelect))
		})

		It("should return a pointer to DisabledPolicySelect", func() {
			result := ptrSelectPolicy(autoscalingv2.DisabledPolicySelect)
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(autoscalingv2.DisabledPolicySelect))
		})
	})

	Describe("buildKEDATriggers function", func() {
		var reconciler *AgentRuntimeReconciler

		BeforeEach(func() {
			reconciler = &AgentRuntimeReconciler{}
		})

		It("should return default Prometheus trigger when no custom triggers specified", func() {
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "test-ns",
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					Runtime: &omniav1alpha1.RuntimeConfig{
						Autoscaling: &omniav1alpha1.AutoscalingConfig{
							Enabled: true,
							Type:    omniav1alpha1.AutoscalerTypeKEDA,
						},
					},
				},
			}

			triggers := reconciler.buildKEDATriggers(agentRuntime)

			Expect(triggers).To(HaveLen(1))
			trigger := triggers[0].(map[string]interface{})
			Expect(trigger["type"]).To(Equal("prometheus"))

			metadata := trigger["metadata"].(map[string]interface{})
			Expect(metadata["serverAddress"]).To(Equal("http://omnia-prometheus-server.omnia-system.svc.cluster.local/prometheus"))
			Expect(metadata["query"]).To(ContainSubstring("test-agent"))
			Expect(metadata["query"]).To(ContainSubstring("test-ns"))
			Expect(metadata["threshold"]).To(Equal("10"))
		})

		It("should return default trigger when KEDA config is nil", func() {
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-agent",
					Namespace: "production",
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					Runtime: &omniav1alpha1.RuntimeConfig{
						Autoscaling: &omniav1alpha1.AutoscalingConfig{
							Enabled: true,
							Type:    omniav1alpha1.AutoscalerTypeKEDA,
							KEDA:    nil, // Explicitly nil
						},
					},
				},
			}

			triggers := reconciler.buildKEDATriggers(agentRuntime)

			Expect(triggers).To(HaveLen(1))
			trigger := triggers[0].(map[string]interface{})
			Expect(trigger["type"]).To(Equal("prometheus"))
		})

		It("should return custom triggers when specified", func() {
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "test-ns",
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					Runtime: &omniav1alpha1.RuntimeConfig{
						Autoscaling: &omniav1alpha1.AutoscalingConfig{
							Enabled: true,
							Type:    omniav1alpha1.AutoscalerTypeKEDA,
							KEDA: &omniav1alpha1.KEDAConfig{
								Triggers: []omniav1alpha1.KEDATrigger{
									{
										Type: "prometheus",
										Metadata: map[string]string{
											"serverAddress": "http://custom-prometheus:9090",
											"query":         "custom_metric",
											"threshold":     "5",
										},
									},
									{
										Type: "rabbitmq",
										Metadata: map[string]string{
											"queueName":   "tasks",
											"queueLength": "10",
										},
									},
								},
							},
						},
					},
				},
			}

			triggers := reconciler.buildKEDATriggers(agentRuntime)

			Expect(triggers).To(HaveLen(2))

			// First trigger
			trigger1 := triggers[0].(map[string]interface{})
			Expect(trigger1["type"]).To(Equal("prometheus"))
			metadata1 := trigger1["metadata"].(map[string]interface{})
			Expect(metadata1["serverAddress"]).To(Equal("http://custom-prometheus:9090"))
			Expect(metadata1["query"]).To(Equal("custom_metric"))
			Expect(metadata1["threshold"]).To(Equal("5"))

			// Second trigger
			trigger2 := triggers[1].(map[string]interface{})
			Expect(trigger2["type"]).To(Equal("rabbitmq"))
			metadata2 := trigger2["metadata"].(map[string]interface{})
			Expect(metadata2["queueName"]).To(Equal("tasks"))
			Expect(metadata2["queueLength"]).To(Equal("10"))
		})

		It("should prefer custom triggers over defaults when triggers list is not empty", func() {
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "test-ns",
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					Runtime: &omniav1alpha1.RuntimeConfig{
						Autoscaling: &omniav1alpha1.AutoscalingConfig{
							Enabled: true,
							Type:    omniav1alpha1.AutoscalerTypeKEDA,
							KEDA: &omniav1alpha1.KEDAConfig{
								PollingInterval: ptr(int32(15)),
								CooldownPeriod:  ptr(int32(60)),
								Triggers: []omniav1alpha1.KEDATrigger{
									{
										Type: "cpu",
										Metadata: map[string]string{
											"type":  "Utilization",
											"value": "80",
										},
									},
								},
							},
						},
					},
				},
			}

			triggers := reconciler.buildKEDATriggers(agentRuntime)

			// Should use custom trigger, not default
			Expect(triggers).To(HaveLen(1))
			trigger := triggers[0].(map[string]interface{})
			Expect(trigger["type"]).To(Equal("cpu"))
		})

		It("should use default trigger when triggers list is empty", func() {
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "test-ns",
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					Runtime: &omniav1alpha1.RuntimeConfig{
						Autoscaling: &omniav1alpha1.AutoscalingConfig{
							Enabled: true,
							Type:    omniav1alpha1.AutoscalerTypeKEDA,
							KEDA: &omniav1alpha1.KEDAConfig{
								PollingInterval: ptr(int32(15)),
								Triggers:        []omniav1alpha1.KEDATrigger{}, // Empty list
							},
						},
					},
				},
			}

			triggers := reconciler.buildKEDATriggers(agentRuntime)

			// Should fall back to default prometheus trigger
			Expect(triggers).To(HaveLen(1))
			trigger := triggers[0].(map[string]interface{})
			Expect(trigger["type"]).To(Equal("prometheus"))
		})
	})

	Context("buildRuntimeEnvVars", func() {
		var reconciler *AgentRuntimeReconciler

		BeforeEach(func() {
			reconciler = &AgentRuntimeReconciler{
				FacadeImage:  "test-facade:v1.0.0",
				RuntimeImage: "test-runtime:v1.0.0",
			}
		})

		It("should include mock provider env var when annotation is set", func() {
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "test-ns",
					Annotations: map[string]string{
						MockProviderAnnotation: "true",
					},
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: "test-pack",
					},
				},
			}

			promptPack := &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pack",
					Namespace: "test-ns",
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Version: "1.0.0",
				},
			}

			envVars := reconciler.buildRuntimeEnvVars(agentRuntime, promptPack, nil, nil)

			// Find the mock provider env var
			var found bool
			for _, env := range envVars {
				if env.Name == "OMNIA_MOCK_PROVIDER" && env.Value == "true" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "OMNIA_MOCK_PROVIDER env var should be set")
		})

		It("should not include mock provider env var when annotation is not set", func() {
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "test-ns",
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: "test-pack",
					},
				},
			}

			promptPack := &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pack",
					Namespace: "test-ns",
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Version: "1.0.0",
				},
			}

			envVars := reconciler.buildRuntimeEnvVars(agentRuntime, promptPack, nil, nil)

			// Ensure mock provider env var is NOT set
			for _, env := range envVars {
				Expect(env.Name).NotTo(Equal("OMNIA_MOCK_PROVIDER"), "OMNIA_MOCK_PROVIDER should not be set without annotation")
			}
		})

		It("should not include mock provider env var when annotation is false", func() {
			agentRuntime := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "test-ns",
					Annotations: map[string]string{
						MockProviderAnnotation: "false",
					},
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: "test-pack",
					},
				},
			}

			promptPack := &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pack",
					Namespace: "test-ns",
				},
				Spec: omniav1alpha1.PromptPackSpec{
					Version: "1.0.0",
				},
			}

			envVars := reconciler.buildRuntimeEnvVars(agentRuntime, promptPack, nil, nil)

			// Ensure mock provider env var is NOT set
			for _, env := range envVars {
				Expect(env.Name).NotTo(Equal("OMNIA_MOCK_PROVIDER"), "OMNIA_MOCK_PROVIDER should not be set when annotation is false")
			}
		})
	})

	Context("buildToolsConfig", func() {
		var reconciler *AgentRuntimeReconciler

		BeforeEach(func() {
			reconciler = &AgentRuntimeReconciler{
				FacadeImage:  "test-facade:v1.0.0",
				RuntimeImage: "test-runtime:v1.0.0",
			}
		})

		It("should build config from available handlers", func() {
			timeout := "30s"
			retries := int32(3)

			toolRegistry := &omniav1alpha1.ToolRegistry{
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "handler1",
							Type: omniav1alpha1.HandlerTypeHTTP,
							HTTPConfig: &omniav1alpha1.HTTPConfig{
								Endpoint: "http://tool1-service:8080/api",
							},
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "tool1",
								Description: "Test tool description",
								InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
							},
							Timeout: &timeout,
							Retries: &retries,
						},
					},
				},
				Status: omniav1alpha1.ToolRegistryStatus{
					DiscoveredTools: []omniav1alpha1.DiscoveredTool{
						{
							Name:        "tool1",
							HandlerName: "handler1",
							Endpoint:    "http://tool1-service:8080/api",
							Status:      omniav1alpha1.ToolStatusAvailable,
						},
					},
				},
			}

			config := reconciler.buildToolsConfig(toolRegistry)

			Expect(config.Handlers).To(HaveLen(1))
			Expect(config.Handlers[0].Name).To(Equal("handler1"))
			Expect(config.Handlers[0].Type).To(Equal("http"))
			Expect(config.Handlers[0].Timeout).To(Equal(timeout))
			Expect(config.Handlers[0].Retries).To(Equal(retries))
			Expect(config.Handlers[0].HTTPConfig).NotTo(BeNil())
			Expect(config.Handlers[0].HTTPConfig.Endpoint).To(Equal("http://tool1-service:8080/api"))
			Expect(config.Handlers[0].Tool).NotTo(BeNil())
			Expect(config.Handlers[0].Tool.Name).To(Equal("tool1"))
		})

		It("should skip unavailable handlers", func() {
			toolRegistry := &omniav1alpha1.ToolRegistry{
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name:       "handler1",
							Type:       omniav1alpha1.HandlerTypeHTTP,
							HTTPConfig: &omniav1alpha1.HTTPConfig{Endpoint: "http://tool1:8080"},
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "tool1",
								Description: "Tool 1",
								InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
							},
						},
						{
							Name:       "handler2",
							Type:       omniav1alpha1.HandlerTypeHTTP,
							HTTPConfig: &omniav1alpha1.HTTPConfig{Endpoint: "http://tool2:8080"},
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "tool2",
								Description: "Tool 2",
								InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
							},
						},
					},
				},
				Status: omniav1alpha1.ToolRegistryStatus{
					DiscoveredTools: []omniav1alpha1.DiscoveredTool{
						{
							Name:        "tool1",
							HandlerName: "handler1",
							Endpoint:    "http://tool1:8080",
							Status:      omniav1alpha1.ToolStatusAvailable,
						},
						{
							Name:        "tool2",
							HandlerName: "handler2",
							Endpoint:    "http://tool2:8080",
							Status:      omniav1alpha1.ToolStatusUnavailable,
						},
					},
				},
			}

			config := reconciler.buildToolsConfig(toolRegistry)

			Expect(config.Handlers).To(HaveLen(1))
			Expect(config.Handlers[0].Name).To(Equal("handler1"))
		})

		It("should handle discovered tools with matching handlers", func() {
			toolRegistry := &omniav1alpha1.ToolRegistry{
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name:       "orphan-handler",
							Type:       omniav1alpha1.HandlerTypeHTTP,
							HTTPConfig: &omniav1alpha1.HTTPConfig{Endpoint: "http://orphan:8080"},
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "orphan",
								Description: "Orphan tool",
								InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
							},
						},
					},
				},
				Status: omniav1alpha1.ToolRegistryStatus{
					DiscoveredTools: []omniav1alpha1.DiscoveredTool{
						{
							Name:        "orphan",
							HandlerName: "orphan-handler",
							Endpoint:    "http://orphan:8080",
							Status:      omniav1alpha1.ToolStatusAvailable,
						},
					},
				},
			}

			config := reconciler.buildToolsConfig(toolRegistry)

			Expect(config.Handlers).To(HaveLen(1))
			Expect(config.Handlers[0].Name).To(Equal("orphan-handler"))
			Expect(config.Handlers[0].Type).To(Equal("http"))
			Expect(config.Handlers[0].Timeout).To(BeEmpty())
			Expect(config.Handlers[0].Retries).To(Equal(int32(0)))
		})

		It("should handle empty tool registry", func() {
			toolRegistry := &omniav1alpha1.ToolRegistry{
				Spec:   omniav1alpha1.ToolRegistrySpec{},
				Status: omniav1alpha1.ToolRegistryStatus{},
			}

			config := reconciler.buildToolsConfig(toolRegistry)

			Expect(config.Handlers).To(BeEmpty())
		})

		It("should handle gRPC handler type", func() {
			toolRegistry := &omniav1alpha1.ToolRegistry{
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "grpc-handler",
							Type: omniav1alpha1.HandlerTypeGRPC,
							GRPCConfig: &omniav1alpha1.GRPCConfig{
								Endpoint: "grpc://grpc-service:9090",
							},
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "grpc_tool",
								Description: "A gRPC tool",
								InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
							},
						},
					},
				},
				Status: omniav1alpha1.ToolRegistryStatus{
					DiscoveredTools: []omniav1alpha1.DiscoveredTool{
						{
							Name:        "grpc_tool",
							HandlerName: "grpc-handler",
							Endpoint:    "grpc://grpc-service:9090",
							Status:      omniav1alpha1.ToolStatusAvailable,
						},
					},
				},
			}

			config := reconciler.buildToolsConfig(toolRegistry)

			Expect(config.Handlers).To(HaveLen(1))
			Expect(config.Handlers[0].Name).To(Equal("grpc-handler"))
			Expect(config.Handlers[0].Type).To(Equal("grpc"))
			Expect(config.Handlers[0].GRPCConfig).NotTo(BeNil())
			Expect(config.Handlers[0].GRPCConfig.Endpoint).To(Equal("grpc://grpc-service:9090"))
		})

		It("should handle MCP handler type with SSE transport", func() {
			endpoint := "http://mcp-server:8080/sse"
			toolRegistry := &omniav1alpha1.ToolRegistry{
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "mcp-handler",
							Type: omniav1alpha1.HandlerTypeMCP,
							MCPConfig: &omniav1alpha1.MCPConfig{
								Transport: omniav1alpha1.MCPTransportSSE,
								Endpoint:  &endpoint,
							},
						},
					},
				},
				Status: omniav1alpha1.ToolRegistryStatus{
					DiscoveredTools: []omniav1alpha1.DiscoveredTool{
						{
							Name:        "mcp-tool",
							HandlerName: "mcp-handler",
							Endpoint:    endpoint,
							Status:      omniav1alpha1.ToolStatusAvailable,
						},
					},
				},
			}

			config := reconciler.buildToolsConfig(toolRegistry)

			Expect(config.Handlers).To(HaveLen(1))
			Expect(config.Handlers[0].Name).To(Equal("mcp-handler"))
			Expect(config.Handlers[0].Type).To(Equal("mcp"))
			Expect(config.Handlers[0].HTTPConfig).To(BeNil())
			Expect(config.Handlers[0].MCPConfig).NotTo(BeNil())
			Expect(config.Handlers[0].MCPConfig.Transport).To(Equal("sse"))
			Expect(config.Handlers[0].MCPConfig.Endpoint).To(Equal(endpoint))
		})

		It("should handle MCP handler type with stdio transport", func() {
			command := "/usr/local/bin/mcp-server"
			workDir := "/app"
			toolRegistry := &omniav1alpha1.ToolRegistry{
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "mcp-stdio-handler",
							Type: omniav1alpha1.HandlerTypeMCP,
							MCPConfig: &omniav1alpha1.MCPConfig{
								Transport: omniav1alpha1.MCPTransportStdio,
								Command:   &command,
								Args:      []string{"--verbose", "--port=8080"},
								WorkDir:   &workDir,
								Env:       map[string]string{"DEBUG": "true"},
							},
						},
					},
				},
				Status: omniav1alpha1.ToolRegistryStatus{
					DiscoveredTools: []omniav1alpha1.DiscoveredTool{
						{
							Name:        "mcp-stdio-tool",
							HandlerName: "mcp-stdio-handler",
							Endpoint:    "stdio://mcp-server",
							Status:      omniav1alpha1.ToolStatusAvailable,
						},
					},
				},
			}

			config := reconciler.buildToolsConfig(toolRegistry)

			Expect(config.Handlers).To(HaveLen(1))
			Expect(config.Handlers[0].Name).To(Equal("mcp-stdio-handler"))
			Expect(config.Handlers[0].Type).To(Equal("mcp"))
			Expect(config.Handlers[0].HTTPConfig).To(BeNil())
			Expect(config.Handlers[0].MCPConfig).NotTo(BeNil())
			Expect(config.Handlers[0].MCPConfig.Transport).To(Equal("stdio"))
			Expect(config.Handlers[0].MCPConfig.Command).To(Equal(command))
			Expect(config.Handlers[0].MCPConfig.Args).To(Equal([]string{"--verbose", "--port=8080"}))
			Expect(config.Handlers[0].MCPConfig.WorkDir).To(Equal(workDir))
			Expect(config.Handlers[0].MCPConfig.Env).To(HaveKeyWithValue("DEBUG", "true"))
		})
	})

	Context("buildProviderEnvVarsFromCRD", func() {
		It("should build env vars from Provider CRD with all fields", func() {
			temperature := "0.7"
			topP := "0.9"
			maxTokens := int32(4096)
			inputCost := "0.003"
			outputCost := "0.015"
			cachedCost := "0.0003"

			provider := &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					Type:    omniav1alpha1.ProviderTypeClaude,
					Model:   "claude-sonnet-4-20250514",
					BaseURL: "https://api.anthropic.com",
					SecretRef: omniav1alpha1.SecretKeyRef{
						Name: "test-secret",
					},
					Defaults: &omniav1alpha1.ProviderDefaults{
						Temperature: &temperature,
						TopP:        &topP,
						MaxTokens:   &maxTokens,
					},
					Pricing: &omniav1alpha1.ProviderPricing{
						InputCostPer1K:  &inputCost,
						OutputCostPer1K: &outputCost,
						CachedCostPer1K: &cachedCost,
					},
				},
			}

			envVars := buildProviderEnvVarsFromCRD(provider)

			// Check all env vars are present
			envMap := make(map[string]string)
			for _, env := range envVars {
				if env.Value != "" {
					envMap[env.Name] = env.Value
				}
			}

			Expect(envMap["OMNIA_PROVIDER_TYPE"]).To(Equal("claude"))
			Expect(envMap["OMNIA_PROVIDER_MODEL"]).To(Equal("claude-sonnet-4-20250514"))
			Expect(envMap["OMNIA_PROVIDER_BASE_URL"]).To(Equal("https://api.anthropic.com"))
			Expect(envMap["OMNIA_PROVIDER_TEMPERATURE"]).To(Equal("0.7"))
			Expect(envMap["OMNIA_PROVIDER_TOP_P"]).To(Equal("0.9"))
			Expect(envMap["OMNIA_PROVIDER_MAX_TOKENS"]).To(Equal("4096"))
			Expect(envMap["OMNIA_PROVIDER_INPUT_COST"]).To(Equal("0.003"))
			Expect(envMap["OMNIA_PROVIDER_OUTPUT_COST"]).To(Equal("0.015"))
			Expect(envMap["OMNIA_PROVIDER_CACHED_COST"]).To(Equal("0.0003"))
		})

		It("should build env vars from Provider CRD with minimal fields", func() {
			provider := &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					Type: omniav1alpha1.ProviderTypeOpenAI,
					SecretRef: omniav1alpha1.SecretKeyRef{
						Name: "test-secret",
					},
				},
			}

			envVars := buildProviderEnvVarsFromCRD(provider)

			// Check that provider type is set
			var foundType bool
			for _, env := range envVars {
				if env.Name == "OMNIA_PROVIDER_TYPE" && env.Value == "openai" {
					foundType = true
					break
				}
			}
			Expect(foundType).To(BeTrue())
		})

		It("should use custom secret key when specified", func() {
			customKey := "my-api-key"
			provider := &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					Type: omniav1alpha1.ProviderTypeClaude,
					SecretRef: omniav1alpha1.SecretKeyRef{
						Name: "test-secret",
						Key:  &customKey,
					},
				},
			}

			envVars := buildProviderEnvVarsFromCRD(provider)

			// Find the ANTHROPIC_API_KEY env var and check it uses the custom key
			var foundKey bool
			for _, env := range envVars {
				if env.Name == anthropicAPIKey && env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					Expect(env.ValueFrom.SecretKeyRef.Key).To(Equal("my-api-key"))
					foundKey = true
					break
				}
			}
			Expect(foundKey).To(BeTrue())
		})
	})

	Context("buildSecretEnvVarsWithKey", func() {
		It("should create env var with correct name for Claude", func() {
			secretRef := &corev1.LocalObjectReference{Name: "test-secret"}
			envVars := buildSecretEnvVarsWithKey(secretRef, omniav1alpha1.ProviderTypeClaude, "custom-key")

			Expect(envVars).To(HaveLen(1))
			Expect(envVars[0].Name).To(Equal(anthropicAPIKey))
			Expect(envVars[0].ValueFrom.SecretKeyRef.Key).To(Equal("custom-key"))
			Expect(envVars[0].ValueFrom.SecretKeyRef.Name).To(Equal("test-secret"))
		})

		It("should create env var with correct name for OpenAI", func() {
			secretRef := &corev1.LocalObjectReference{Name: "test-secret"}
			envVars := buildSecretEnvVarsWithKey(secretRef, omniav1alpha1.ProviderTypeOpenAI, "custom-key")

			Expect(envVars).To(HaveLen(1))
			Expect(envVars[0].Name).To(Equal("OPENAI_API_KEY"))
			Expect(envVars[0].ValueFrom.SecretKeyRef.Key).To(Equal("custom-key"))
		})

		It("should create env var with correct name for Gemini", func() {
			secretRef := &corev1.LocalObjectReference{Name: "test-secret"}
			envVars := buildSecretEnvVarsWithKey(secretRef, omniav1alpha1.ProviderTypeGemini, "custom-key")

			Expect(envVars).To(HaveLen(1))
			Expect(envVars[0].Name).To(Equal("GEMINI_API_KEY"))
			Expect(envVars[0].ValueFrom.SecretKeyRef.Key).To(Equal("custom-key"))
		})

		It("should default to ANTHROPIC_API_KEY for auto provider", func() {
			secretRef := &corev1.LocalObjectReference{Name: "test-secret"}
			envVars := buildSecretEnvVarsWithKey(secretRef, omniav1alpha1.ProviderTypeAuto, "custom-key")

			Expect(envVars).To(HaveLen(1))
			Expect(envVars[0].Name).To(Equal(anthropicAPIKey))
		})
	})
})
