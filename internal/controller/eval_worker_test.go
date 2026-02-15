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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

var _ = Describe("Eval Worker Reconciliation", func() {
	const namespace = "default"

	var (
		ctx        context.Context
		reconciler *AgentRuntimeReconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		reconciler = &AgentRuntimeReconciler{
			Client:          k8sClient,
			Scheme:          k8sClient.Scheme(),
			FacadeImage:     "test-facade:v1.0.0",
			FrameworkImage:  "test-runtime:v1.0.0",
			SessionAPIURL:   "http://session-api:8080",
			RedisAddr:       "redis:6379",
			EvalWorkerImage: "test-eval-worker:v1.0.0",
		}
	})

	Context("eval worker Deployment lifecycle", func() {
		var (
			promptPackKey   types.NamespacedName
			agentRuntimeKey types.NamespacedName
		)

		BeforeEach(func() {
			promptPackKey = types.NamespacedName{
				Name:      "eval-test-promptpack",
				Namespace: namespace,
			}
			agentRuntimeKey = types.NamespacedName{
				Name:      "eval-test-agent",
				Namespace: namespace,
			}

			// Create shared PromptPack
			pp := &omniav1alpha1.PromptPack{
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
			Expect(k8sClient.Create(ctx, pp)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up eval worker Deployment
			dep := &appsv1.Deployment{}
			key := types.NamespacedName{Name: EvalWorkerDeploymentName, Namespace: namespace}
			if err := k8sClient.Get(ctx, key, dep); err == nil {
				_ = k8sClient.Delete(ctx, dep)
			}

			// Clean up AgentRuntime
			ar := &omniav1alpha1.AgentRuntime{}
			if err := k8sClient.Get(ctx, agentRuntimeKey, ar); err == nil {
				ar.Finalizers = nil
				_ = k8sClient.Update(ctx, ar)
				_ = k8sClient.Delete(ctx, ar)
			}

			// Clean up PromptPack
			pp := &omniav1alpha1.PromptPack{}
			if err := k8sClient.Get(ctx, promptPackKey, pp); err == nil {
				_ = k8sClient.Delete(ctx, pp)
			}
		})

		It("should create eval worker for non-PromptKit agent with evals enabled", func() {
			ar := newTestAgentRuntime(agentRuntimeKey, promptPackKey.Name)
			ar.Spec.Framework = &omniav1alpha1.FrameworkConfig{
				Type:  omniav1alpha1.FrameworkTypeLangChain,
				Image: "test-langchain:v1",
			}
			ar.Spec.Evals = &omniav1alpha1.EvalConfig{Enabled: true}
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			// First reconcile: add finalizer
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())
			// Second reconcile: create resources
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())

			// Verify eval worker Deployment was created
			dep := &appsv1.Deployment{}
			key := types.NamespacedName{Name: EvalWorkerDeploymentName, Namespace: namespace}
			Expect(k8sClient.Get(ctx, key, dep)).To(Succeed())
			Expect(dep.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal("test-eval-worker:v1.0.0"))

			// Verify env vars
			envMap := envVarMap(dep.Spec.Template.Spec.Containers[0].Env)
			Expect(envMap[envNamespace]).To(Equal(namespace))
			Expect(envMap[envRedisAddr]).To(Equal("redis:6379"))
			Expect(envMap[envSessionAPIURL]).To(Equal("http://session-api:8080"))
		})

		It("should NOT create eval worker for PromptKit agent with evals enabled", func() {
			ar := newTestAgentRuntime(agentRuntimeKey, promptPackKey.Name)
			// Default framework is PromptKit
			ar.Spec.Evals = &omniav1alpha1.EvalConfig{Enabled: true}
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())

			// Verify no eval worker Deployment exists
			dep := &appsv1.Deployment{}
			key := types.NamespacedName{Name: EvalWorkerDeploymentName, Namespace: namespace}
			err = k8sClient.Get(ctx, key, dep)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should NOT create eval worker when evals are not enabled", func() {
			ar := newTestAgentRuntime(agentRuntimeKey, promptPackKey.Name)
			ar.Spec.Framework = &omniav1alpha1.FrameworkConfig{
				Type:  omniav1alpha1.FrameworkTypeLangChain,
				Image: "test-langchain:v1",
			}
			// No evals config
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())

			dep := &appsv1.Deployment{}
			key := types.NamespacedName{Name: EvalWorkerDeploymentName, Namespace: namespace}
			err = k8sClient.Get(ctx, key, dep)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should update eval worker Deployment when reconciled again", func() {
			ar := newTestAgentRuntime(agentRuntimeKey, promptPackKey.Name)
			ar.Spec.Framework = &omniav1alpha1.FrameworkConfig{
				Type:  omniav1alpha1.FrameworkTypeLangChain,
				Image: "test-langchain:v1",
			}
			ar.Spec.Evals = &omniav1alpha1.EvalConfig{Enabled: true}
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			// First + second reconcile: create
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())

			// Verify created
			dep := &appsv1.Deployment{}
			key := types.NamespacedName{Name: EvalWorkerDeploymentName, Namespace: namespace}
			Expect(k8sClient.Get(ctx, key, dep)).To(Succeed())

			// Change image and reconcile again — should update
			reconciler.EvalWorkerImage = "updated-worker:v2"
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, key, dep)).To(Succeed())
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal("updated-worker:v2"))
		})

		It("should delete eval worker when evals are disabled", func() {
			ar := newTestAgentRuntime(agentRuntimeKey, promptPackKey.Name)
			ar.Spec.Framework = &omniav1alpha1.FrameworkConfig{
				Type:  omniav1alpha1.FrameworkTypeLangChain,
				Image: "test-langchain:v1",
			}
			ar.Spec.Evals = &omniav1alpha1.EvalConfig{Enabled: true}
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			// Create eval worker
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())

			dep := &appsv1.Deployment{}
			key := types.NamespacedName{Name: EvalWorkerDeploymentName, Namespace: namespace}
			Expect(k8sClient.Get(ctx, key, dep)).To(Succeed())

			// Disable evals
			Expect(k8sClient.Get(ctx, agentRuntimeKey, ar)).To(Succeed())
			ar.Spec.Evals.Enabled = false
			Expect(k8sClient.Update(ctx, ar)).To(Succeed())

			// Reconcile again — should delete eval worker
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: agentRuntimeKey})
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, key, dep)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should use default eval worker image when none specified", func() {
			reconciler.EvalWorkerImage = ""
			Expect(reconciler.evalWorkerImage()).To(Equal(DefaultEvalWorkerImage))
		})

		It("should use custom eval worker image when specified", func() {
			reconciler.EvalWorkerImage = "custom-worker:v2"
			Expect(reconciler.evalWorkerImage()).To(Equal("custom-worker:v2"))
		})
	})
})

// newTestAgentRuntime creates a minimal AgentRuntime for testing.
func newTestAgentRuntime(
	key types.NamespacedName,
	promptPackName string,
) *omniav1alpha1.AgentRuntime {
	return &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{
				Name: promptPackName,
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
}

// envVarMap converts a slice of EnvVars to a map for easy assertion.
func envVarMap(envVars []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(envVars))
	for _, e := range envVars {
		m[e.Name] = e.Value
	}
	return m
}
