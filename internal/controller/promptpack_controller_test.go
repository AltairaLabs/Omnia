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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/schema"
)

// validPackJSON is a valid PromptPack JSON that passes schema validation
const validPackJSON = `{
	"id": "test-pack",
	"name": "Test Pack",
	"version": "1.0.0",
	"template_engine": {
		"version": "v1",
		"syntax": "{{variable}}"
	},
	"prompts": {
		"default": {
			"id": "default",
			"name": "Default Prompt",
			"version": "1.0.0",
			"system_template": "You are a helpful assistant."
		}
	}
}`

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
					"pack.json": validPackJSON,
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
					PackName: "test-pack",
					Source: omniav1alpha1.PromptPackContentSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: configMapName,
						},
					},
					Version: "1.0.0",
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
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				SchemaValidator: schema.NewSchemaValidatorWithOptions(logr.Discard(), nil, time.Hour),
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
					PackName: "test-pack",
					Source: omniav1alpha1.PromptPackContentSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: "nonexistent-configmap",
						},
					},
					Version: "1.0.0",
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
					"pack.json": validPackJSON,
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("creating the PromptPack")
			promptPack = &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "canary-pack",
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					PackName: "test-pack",
					Source: omniav1alpha1.PromptPackContentSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: "canary-prompts",
						},
					},
					Version: "2.0.0",
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
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				SchemaValidator: schema.NewSchemaValidatorWithOptions(logr.Discard(), nil, time.Hour),
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

			Expect(updatedPP.Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))
			Expect(updatedPP.Status.ActiveVersion).NotTo(BeNil())
			Expect(*updatedPP.Status.ActiveVersion).To(Equal("2.0.0"))
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
					"pack.json": validPackJSON,
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
					PackName: "test-pack",
					Source: omniav1alpha1.PromptPackContentSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: "ref-prompts",
						},
					},
					Version: "1.0.0",
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
						// References the pack's logical packName, NOT its
						// metadata.name ("referenced-pack") — proves matching
						// is keyed on spec.packName (#1837).
						Name: "test-pack",
					},
					Facades: []omniav1alpha1.FacadeConfig{{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					}},
					Providers: []omniav1alpha1.NamedProviderRef{
						{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "test-provider"}},
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
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				SchemaValidator: schema.NewSchemaValidatorWithOptions(logr.Discard(), nil, time.Hour),
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

	Context("When the PromptPack's object name is a hash distinct from spec.packName", func() {
		// Phase 1 makes metadata.name a deterministic pp-<hash>, no longer
		// equal to spec.packName. findReferencingAgentRuntimes must match
		// AgentRuntimes by spec.packName; a ref that (incorrectly) points
		// at the hashed object name must NOT match (#1837).
		var (
			hashedPack *omniav1alpha1.PromptPack
			matchingAR *omniav1alpha1.AgentRuntime
			oldStyleAR *omniav1alpha1.AgentRuntime
			hashedCM   *corev1.ConfigMap
		)

		BeforeEach(func() {
			By("creating the ConfigMap")
			hashedCM = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hashed-prompts",
					Namespace: promptPackNamespace,
				},
				Data: map[string]string{
					"pack.json": validPackJSON,
				},
			}
			Expect(k8sClient.Create(ctx, hashedCM)).To(Succeed())

			By("creating a PromptPack whose object name is a hash, not the packName")
			hashedPack = &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pp-9f8e7d6c5b4a",
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					PackName: "hashed-pack",
					Source: omniav1alpha1.PromptPackContentSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: "hashed-prompts",
						},
					},
					Version: "1.0.0",
				},
			}
			Expect(k8sClient.Create(ctx, hashedPack)).To(Succeed())

			By("creating an AgentRuntime that references the pack by packName")
			matchingAR = &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "matching-agent",
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: "hashed-pack",
					},
					Facades: []omniav1alpha1.FacadeConfig{{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					}},
					Providers: []omniav1alpha1.NamedProviderRef{
						{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "test-provider"}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, matchingAR)).To(Succeed())

			By("creating an AgentRuntime that references the pack's object name (old, no-longer-valid behavior)")
			oldStyleAR = &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "old-style-agent",
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name: "pp-9f8e7d6c5b4a",
					},
					Facades: []omniav1alpha1.FacadeConfig{{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					}},
					Providers: []omniav1alpha1.NamedProviderRef{
						{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "test-provider"}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, oldStyleAR)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			for _, name := range []string{"matching-agent", "old-style-agent"} {
				ar := &omniav1alpha1.AgentRuntime{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: promptPackNamespace}, ar)
				if err == nil {
					Expect(k8sClient.Delete(ctx, ar)).To(Succeed())
				}
			}

			pp := &omniav1alpha1.PromptPack{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "pp-9f8e7d6c5b4a",
				Namespace: promptPackNamespace,
			}, pp)
			if err == nil {
				Expect(k8sClient.Delete(ctx, pp)).To(Succeed())
			}

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "hashed-prompts",
				Namespace: promptPackNamespace,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}
		})

		It("matches AgentRuntimes by spec.packName, not the PromptPack's object name", func() {
			reconciler := &PromptPackReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				SchemaValidator: schema.NewSchemaValidatorWithOptions(logr.Discard(), nil, time.Hour),
			}

			referencing, err := reconciler.findReferencingAgentRuntimes(ctx, hashedPack)
			Expect(err).NotTo(HaveOccurred())
			Expect(referencing).To(HaveLen(1))
			Expect(referencing[0].Name).To(Equal("matching-agent"))
		})
	})

	Context("When an AgentRuntime pins an exact PromptPack version", func() {
		// #1837 Task 4 carried finding: the exact-Version-pin branch of
		// findReferencingAgentRuntimes had zero coverage. Verify both the hit
		// (pinned version matches the reconciling PromptPack object's version)
		// and the miss (pinned version does not match) cases.
		var (
			pinnedPack *omniav1alpha1.PromptPack
			hitAR      *omniav1alpha1.AgentRuntime
			missAR     *omniav1alpha1.AgentRuntime
			pinnedCM   *corev1.ConfigMap
		)

		BeforeEach(func() {
			By("creating the ConfigMap")
			pinnedCM = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pinned-prompts",
					Namespace: promptPackNamespace,
				},
				Data: map[string]string{
					"pack.json": validPackJSON,
				},
			}
			Expect(k8sClient.Create(ctx, pinnedCM)).To(Succeed())

			By("creating a PromptPack at version 1.0.0")
			pinnedPack = &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pp-pinned-version",
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					PackName: "pinned-pack",
					Source: omniav1alpha1.PromptPackContentSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: "pinned-prompts",
						},
					},
					Version: "1.0.0",
				},
			}
			Expect(k8sClient.Create(ctx, pinnedPack)).To(Succeed())

			By("creating an AgentRuntime that pins the matching version")
			hitAR = &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pinned-hit-agent",
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name:    "pinned-pack",
						Version: ptr.To("1.0.0"),
					},
					Facades: []omniav1alpha1.FacadeConfig{{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					}},
					Providers: []omniav1alpha1.NamedProviderRef{
						{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "test-provider"}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, hitAR)).To(Succeed())

			By("creating an AgentRuntime that pins a different version")
			missAR = &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pinned-miss-agent",
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{
						Name:    "pinned-pack",
						Version: ptr.To("2.0.0"),
					},
					Facades: []omniav1alpha1.FacadeConfig{{
						Type: omniav1alpha1.FacadeTypeWebSocket,
					}},
					Providers: []omniav1alpha1.NamedProviderRef{
						{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "test-provider"}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, missAR)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			for _, name := range []string{"pinned-hit-agent", "pinned-miss-agent"} {
				ar := &omniav1alpha1.AgentRuntime{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: promptPackNamespace}, ar)
				if err == nil {
					Expect(k8sClient.Delete(ctx, ar)).To(Succeed())
				}
			}

			pp := &omniav1alpha1.PromptPack{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "pp-pinned-version",
				Namespace: promptPackNamespace,
			}, pp)
			if err == nil {
				Expect(k8sClient.Delete(ctx, pp)).To(Succeed())
			}

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "pinned-prompts",
				Namespace: promptPackNamespace,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}
		})

		It("matches only the AgentRuntime whose pinned version equals the PromptPack's version", func() {
			reconciler := &PromptPackReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				SchemaValidator: schema.NewSchemaValidatorWithOptions(logr.Discard(), nil, time.Hour),
			}

			referencing, err := reconciler.findReferencingAgentRuntimes(ctx, pinnedPack)
			Expect(err).NotTo(HaveOccurred())
			Expect(referencing).To(HaveLen(1))
			Expect(referencing[0].Name).To(Equal("pinned-hit-agent"))
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
					PackName: "test-pack",
					Source: omniav1alpha1.PromptPackContentSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						// ConfigMapRef is nil
					},
					Version: "1.0.0",
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
					"pack.json": validPackJSON,
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
					PackName: "test-pack",
					Source: omniav1alpha1.PromptPackContentSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: "full-canary-prompts",
						},
					},
					Version: "3.0.0",
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
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				SchemaValidator: schema.NewSchemaValidatorWithOptions(logr.Discard(), nil, time.Hour),
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
					PackName: "test-pack",
					Source: omniav1alpha1.PromptPackContentSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: "empty-configmap",
						},
					},
					Version: "1.0.0",
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
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				SchemaValidator: schema.NewSchemaValidatorWithOptions(logr.Discard(), nil, time.Hour),
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
					"pack.json": validPackJSON,
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
					PackName: "test-pack",
					Source: omniav1alpha1.PromptPackContentSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: "watch-test-configmap",
						},
					},
					Version: "1.0.0",
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
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				SchemaValidator: schema.NewSchemaValidatorWithOptions(logr.Discard(), nil, time.Hour),
			}

			requests := reconciler.findPromptPacksForConfigMap(ctx, configMap)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("watch-test-pack"))
			Expect(requests[0].Namespace).To(Equal(promptPackNamespace))
		})
	})

	Context("When ConfigMap is missing pack.json key", func() {
		var (
			promptPack *omniav1alpha1.PromptPack
			configMap  *corev1.ConfigMap
		)

		BeforeEach(func() {
			By("creating a ConfigMap without pack.json")
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-packjson-configmap",
					Namespace: promptPackNamespace,
				},
				Data: map[string]string{
					"system.txt": "This is not pack.json",
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("creating the PromptPack")
			promptPack = &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-packjson-pack",
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					PackName: "test-pack",
					Source: omniav1alpha1.PromptPackContentSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: "no-packjson-configmap",
						},
					},
					Version: "1.0.0",
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			pp := &omniav1alpha1.PromptPack{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "no-packjson-pack",
				Namespace: promptPackNamespace,
			}, pp)
			if err == nil {
				Expect(k8sClient.Delete(ctx, pp)).To(Succeed())
			}

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "no-packjson-configmap",
				Namespace: promptPackNamespace,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}
		})

		It("should fail validation with missing pack.json error", func() {
			By("reconciling the PromptPack")
			reconciler := &PromptPackReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				SchemaValidator: schema.NewSchemaValidatorWithOptions(logr.Discard(), nil, time.Hour),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "no-packjson-pack",
					Namespace: promptPackNamespace,
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("pack.json"))

			By("checking the updated status")
			updatedPP := &omniav1alpha1.PromptPack{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "no-packjson-pack",
				Namespace: promptPackNamespace,
			}, updatedPP)).To(Succeed())

			Expect(updatedPP.Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseFailed))
		})
	})

	Context("When pack.json fails schema validation", func() {
		var (
			promptPack *omniav1alpha1.PromptPack
			configMap  *corev1.ConfigMap
		)

		BeforeEach(func() {
			By("creating a ConfigMap with invalid pack.json")
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-schema-configmap",
					Namespace: promptPackNamespace,
				},
				Data: map[string]string{
					// Invalid pack.json: missing required fields
					"pack.json": `{"name": "test"}`,
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("creating the PromptPack")
			promptPack = &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-schema-pack",
					Namespace: promptPackNamespace,
				},
				Spec: omniav1alpha1.PromptPackSpec{
					PackName: "test-pack",
					Source: omniav1alpha1.PromptPackContentSource{
						Type: omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{
							Name: "invalid-schema-configmap",
						},
					},
					Version: "1.0.0",
				},
			}
			Expect(k8sClient.Create(ctx, promptPack)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			pp := &omniav1alpha1.PromptPack{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "invalid-schema-pack",
				Namespace: promptPackNamespace,
			}, pp)
			if err == nil {
				Expect(k8sClient.Delete(ctx, pp)).To(Succeed())
			}

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "invalid-schema-configmap",
				Namespace: promptPackNamespace,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}
		})

		It("should fail validation with schema error", func() {
			By("reconciling the PromptPack")
			reconciler := &PromptPackReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				SchemaValidator: schema.NewSchemaValidatorWithOptions(logr.Discard(), nil, time.Hour),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "invalid-schema-pack",
					Namespace: promptPackNamespace,
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid pack.json"))

			By("checking the updated status")
			updatedPP := &omniav1alpha1.PromptPack{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "invalid-schema-pack",
				Namespace: promptPackNamespace,
			}, updatedPP)).To(Succeed())

			Expect(updatedPP.Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseFailed))

			By("checking the SchemaValid condition is set to False")
			schemaCondition := meta.FindStatusCondition(updatedPP.Status.Conditions, PromptPackConditionTypeSchemaValid)
			Expect(schemaCondition).NotTo(BeNil())
			Expect(schemaCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(schemaCondition.Reason).To(Equal("SchemaValidationFailed"))
		})
	})

	Describe("PromptPack identity and immutability", func() {
		const immutabilityNamespace = "default"

		newPack := func(name, packName, version string) *omniav1alpha1.PromptPack {
			return &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: immutabilityNamespace},
				Spec: omniav1alpha1.PromptPackSpec{
					PackName: packName,
					Version:  version,
					Source: omniav1alpha1.PromptPackContentSource{
						Type:         omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{Name: "cm"},
					},
				},
			}
		}

		cleanup := func(name string) {
			p := &omniav1alpha1.PromptPack{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: immutabilityNamespace}, p)
			if err == nil {
				Expect(k8sClient.Delete(ctx, p)).To(Succeed())
			}
		}

		It("requires spec.packName", func() {
			p := newPack("pp-req", "", "1.0.0")
			Expect(k8sClient.Create(ctx, p)).ToNot(Succeed())
		})

		It("rejects mutating spec.version after create", func() {
			p := newPack("pp-imm-ver", "mypack", "1.0.0")
			defer cleanup(p.Name)
			Expect(k8sClient.Create(ctx, p)).To(Succeed())
			p.Spec.Version = "1.0.1"
			Expect(k8sClient.Update(ctx, p)).ToNot(Succeed())
		})

		It("rejects mutating spec.packName after create", func() {
			p := newPack("pp-imm-name", "mypack", "2.0.0")
			defer cleanup(p.Name)
			Expect(k8sClient.Create(ctx, p)).To(Succeed())
			p.Spec.PackName = "other"
			Expect(k8sClient.Update(ctx, p)).ToNot(Succeed())
		})

		It("rejects mutating spec.source after create (whole-spec freeze)", func() {
			p := newPack("pp-imm-src", "mypack", "1.2.3")
			defer cleanup(p.Name)
			Expect(k8sClient.Create(ctx, p)).To(Succeed())
			p.Spec.Source.ConfigMapRef.Name = "cm2"
			Expect(k8sClient.Update(ctx, p)).ToNot(Succeed())
		})

		It("sets the promptpack label from spec.packName", func() {
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "pp-label-cm", Namespace: immutabilityNamespace},
				Data:       map[string]string{"pack.json": validPackJSON},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())
			defer func() {
				Expect(k8sClient.Delete(ctx, configMap)).To(Succeed())
			}()

			p := &omniav1alpha1.PromptPack{
				ObjectMeta: metav1.ObjectMeta{Name: "pp-label", Namespace: immutabilityNamespace},
				Spec: omniav1alpha1.PromptPackSpec{
					PackName: "mypack",
					Version:  "1.0.0",
					Source: omniav1alpha1.PromptPackContentSource{
						Type:         omniav1alpha1.PromptPackSourceTypeConfigMap,
						ConfigMapRef: &corev1.LocalObjectReference{Name: "pp-label-cm"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, p)).To(Succeed())
			defer cleanup(p.Name)

			reconciler := &PromptPackReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				SchemaValidator: schema.NewSchemaValidatorWithOptions(logr.Discard(), nil, time.Hour),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: p.Name, Namespace: immutabilityNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			got := &omniav1alpha1.PromptPack{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: p.Name, Namespace: immutabilityNamespace}, got)).To(Succeed())
			Expect(got.Labels).To(HaveKeyWithValue("omnia.altairalabs.ai/promptpack", "mypack"))
		})
	})
})

// Ensure unused import doesn't cause issues
var _ = errors.IsNotFound
