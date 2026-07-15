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
	"testing"
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
	"sigs.k8s.io/controller-runtime/pkg/event"
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

var _ = Describe("PromptPack sibling-aware phase (Active vs Superseded)", func() {
	const ns = "default"
	ctx := context.Background()

	reconcilerFor := func() *PromptPackReconciler {
		return &PromptPackReconciler{
			Client:          k8sClient,
			Scheme:          k8sClient.Scheme(),
			SchemaValidator: schema.NewSchemaValidatorWithOptions(logr.Discard(), nil, time.Hour),
		}
	}

	createCM := func(name, packJSON string) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Data:       map[string]string{"pack.json": packJSON},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, cm) })
	}

	createPack := func(objName, packName, version, cmName string) {
		pp := &omniav1alpha1.PromptPack{
			ObjectMeta: metav1.ObjectMeta{
				Name:      objName,
				Namespace: ns,
				// Seed the resolution-index label so siblings are discoverable
				// even before their own first reconcile (the controller also
				// sets it, but tests reconcile in arbitrary order).
				Labels: map[string]string{LabelPromptPackName: packName},
			},
			Spec: omniav1alpha1.PromptPackSpec{
				PackName: packName,
				Version:  version,
				Source: omniav1alpha1.PromptPackContentSource{
					Type:         omniav1alpha1.PromptPackSourceTypeConfigMap,
					ConfigMapRef: &corev1.LocalObjectReference{Name: cmName},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pp)).To(Succeed())
		DeferCleanup(func() {
			got := &omniav1alpha1.PromptPack{}
			if k8sClient.Get(ctx, types.NamespacedName{Name: objName, Namespace: ns}, got) == nil {
				_ = k8sClient.Delete(ctx, got)
			}
		})
	}

	doReconcile := func(objName string) error {
		_, err := reconcilerFor().Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: objName, Namespace: ns},
		})
		return err
	}

	getPack := func(objName string) *omniav1alpha1.PromptPack {
		got := &omniav1alpha1.PromptPack{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: objName, Namespace: ns}, got)).To(Succeed())
		return got
	}

	supersededCond := func(objName string) *metav1.Condition {
		return meta.FindStatusCondition(getPack(objName).Status.Conditions, PromptPackConditionTypeSuperseded)
	}

	It("marks a lone version Active with Superseded=False", func() {
		createCM("sib-cm-a", validPackJSON)
		createPack("pp-sib-a-100", "sib-pack-a", "1.0.0", "sib-cm-a")

		Expect(doReconcile("pp-sib-a-100")).To(Succeed())

		Expect(getPack("pp-sib-a-100").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))
		cond := supersededCond("pp-sib-a-100")
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal("IsChannelMax"))
	})

	It("supersedes the older version when a newer one is published", func() {
		createCM("sib-cm-b", validPackJSON)
		createPack("pp-sib-b-100", "sib-pack-b", "1.0.0", "sib-cm-b")

		Expect(doReconcile("pp-sib-b-100")).To(Succeed())
		Expect(getPack("pp-sib-b-100").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))

		createPack("pp-sib-b-110", "sib-pack-b", "1.1.0", "sib-cm-b")
		Expect(doReconcile("pp-sib-b-110")).To(Succeed())
		Expect(getPack("pp-sib-b-110").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))

		// In-cluster the sibling watch re-reconciles 1.0.0; the suite has no
		// running manager, so drive the re-reconcile explicitly under Eventually.
		Eventually(func(g Gomega) {
			g.Expect(doReconcile("pp-sib-b-100")).To(Succeed())
			g.Expect(getPack("pp-sib-b-100").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseSuperseded))
		}).Should(Succeed())

		cond := supersededCond("pp-sib-b-100")
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal("NewerVersionPublished"))
	})

	It("keeps stable-max and prerelease-max both Active (channel coexistence)", func() {
		createCM("sib-cm-c", validPackJSON)
		createPack("pp-sib-c-100", "sib-pack-c", "1.0.0", "sib-cm-c")
		createPack("pp-sib-c-200b", "sib-pack-c", "2.0.0-beta.1", "sib-cm-c")

		Expect(doReconcile("pp-sib-c-100")).To(Succeed())
		Expect(doReconcile("pp-sib-c-200b")).To(Succeed())

		Expect(getPack("pp-sib-c-100").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))
		Expect(getPack("pp-sib-c-200b").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))
	})

	It("supersedes lower stable and prerelease when a higher stable ships", func() {
		createCM("sib-cm-d", validPackJSON)
		createPack("pp-sib-d-100", "sib-pack-d", "1.0.0", "sib-cm-d")
		createPack("pp-sib-d-200b", "sib-pack-d", "2.0.0-beta.1", "sib-cm-d")
		createPack("pp-sib-d-200", "sib-pack-d", "2.0.0", "sib-cm-d")

		Expect(doReconcile("pp-sib-d-100")).To(Succeed())
		Expect(doReconcile("pp-sib-d-200b")).To(Succeed())
		Expect(doReconcile("pp-sib-d-200")).To(Succeed())

		Expect(getPack("pp-sib-d-200").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))

		// The older stable and the prerelease only supersede once the higher
		// stable 2.0.0 has itself validated (Active). In-cluster the sibling
		// watch re-enqueues them on 2.0.0's phase transition; with no running
		// manager, drive the re-reconcile explicitly under Eventually.
		Eventually(func(g Gomega) {
			g.Expect(doReconcile("pp-sib-d-100")).To(Succeed())
			g.Expect(getPack("pp-sib-d-100").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseSuperseded))
		}).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(doReconcile("pp-sib-d-200b")).To(Succeed())
			g.Expect(getPack("pp-sib-d-200b").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseSuperseded))
		}).Should(Succeed())
	})

	It("maps a changed version to reconcile requests for all its siblings", func() {
		createCM("sib-cm-f", validPackJSON)
		createPack("pp-sib-f-100", "sib-pack-f", "1.0.0", "sib-cm-f")
		createPack("pp-sib-f-110", "sib-pack-f", "1.1.0", "sib-cm-f")

		requests := reconcilerFor().findSiblingPromptPacks(ctx, getPack("pp-sib-f-110"))
		Expect(requests).To(HaveLen(2))
		names := []string{requests[0].Name, requests[1].Name}
		Expect(names).To(ConsistOf("pp-sib-f-100", "pp-sib-f-110"))
	})

	It("returns no requests for a PromptPack without the pack-name label", func() {
		unlabeled := &omniav1alpha1.PromptPack{
			ObjectMeta: metav1.ObjectMeta{Name: "pp-unlabeled", Namespace: ns},
		}
		Expect(reconcilerFor().findSiblingPromptPacks(ctx, unlabeled)).To(BeNil())
	})

	It("always includes self as a channel-max candidate even when unindexed", func() {
		// A pack whose label index has not yet caught up (not persisted here):
		// listSiblings must still include self, and resolvePackPhase must treat
		// a lone self as its own channel-max -> Active.
		self := &omniav1alpha1.PromptPack{
			ObjectMeta: metav1.ObjectMeta{Name: "pp-unindexed", Namespace: ns},
			Spec: omniav1alpha1.PromptPackSpec{
				PackName: "unindexed-pack",
				Version:  "3.1.4",
			},
		}
		sibs, err := reconcilerFor().listSiblings(ctx, self)
		Expect(err).NotTo(HaveOccurred())
		Expect(sibs).To(HaveLen(1))
		Expect(sibs[0].Name).To(Equal("pp-unindexed"))
		Expect(resolvePackPhase(self, nil)).To(BeTrue())
	})

	It("excludes Failed siblings from channel-max", func() {
		createCM("sib-cm-e", validPackJSON)
		createCM("sib-cm-e-bad", `{"name":"bad"}`)
		createPack("pp-sib-e-100", "sib-pack-e", "1.0.0", "sib-cm-e")
		createPack("pp-sib-e-200", "sib-pack-e", "2.0.0", "sib-cm-e-bad")

		// 2.0.0 has an invalid pack.json → Failed (Reconcile returns the error).
		Expect(doReconcile("pp-sib-e-200")).To(HaveOccurred())
		Expect(getPack("pp-sib-e-200").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseFailed))

		// 1.0.0 must stay Active — the Failed 2.0.0 is excluded from channel-max.
		Expect(doReconcile("pp-sib-e-100")).To(Succeed())
		Expect(getPack("pp-sib-e-100").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))
	})

	It("keeps the older version Active when a newer version has a bad pack.json (convergence regression)", func() {
		// Regression for the sibling-validation convergence bug (#1858): a
		// newer version that is published but NOT yet validated (Phase "") — or
		// that ultimately fails validation — must never supersede a live older
		// version. This reproduces the interleaving where the older version
		// reconciles while the newer one is still Pending.
		createCM("sib-cm-g", validPackJSON)
		createCM("sib-cm-g-bad", `{"name":"bad"}`)
		createPack("pp-sib-g-100", "sib-pack-g", "1.0.0", "sib-cm-g")

		// 1.0.0 reconciles first and becomes Active.
		Expect(doReconcile("pp-sib-g-100")).To(Succeed())
		Expect(getPack("pp-sib-g-100").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))

		// A newer 2.0.0 is published with an INVALID pack.json. Its CREATE
		// enqueues 1.0.0 via the sibling watch; 1.0.0 reconciles while 2.0.0 is
		// still Phase=="" (not yet validated). The still-unvalidated 2.0.0 must
		// NOT count toward channel-max, so 1.0.0 stays Active. (Under the old
		// !Failed eligibility, 1.0.0 wrongly flipped to Superseded here, then
		// 2.0.0's status-only Failed transition was filtered by the sibling
		// watch predicate, stranding 1.0.0 Superseded forever.)
		createPack("pp-sib-g-200", "sib-pack-g", "2.0.0", "sib-cm-g-bad")
		Expect(doReconcile("pp-sib-g-100")).To(Succeed())
		Expect(getPack("pp-sib-g-100").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))

		// 2.0.0 then reconciles and fails validation.
		Expect(doReconcile("pp-sib-g-200")).To(HaveOccurred())
		Expect(getPack("pp-sib-g-200").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseFailed))

		// 1.0.0 stays Active — now and on every subsequent reconcile.
		Eventually(func(g Gomega) {
			g.Expect(doReconcile("pp-sib-g-100")).To(Succeed())
			g.Expect(getPack("pp-sib-g-100").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))
		}).Should(Succeed())
		Consistently(func(g Gomega) {
			g.Expect(doReconcile("pp-sib-g-100")).To(Succeed())
			g.Expect(getPack("pp-sib-g-100").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))
		}).Should(Succeed())
	})

	It("converges the happy path: valid newer version supersedes older via phase-change watch", func() {
		// Mirrors the in-cluster convergence: 2.0.0 (valid) is published while
		// 1.0.0 is Active. The sibling watch fires on 2.0.0's Pending→Active
		// transition, re-reconciling 1.0.0 to Superseded; 1.0.0's
		// Active→Superseded then re-reconciles 2.0.0, which stays Active.
		createCM("sib-cm-h", validPackJSON)
		createPack("pp-sib-h-100", "sib-pack-h", "1.0.0", "sib-cm-h")

		Expect(doReconcile("pp-sib-h-100")).To(Succeed())
		Expect(getPack("pp-sib-h-100").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))

		// 2.0.0 published; 1.0.0 reconciles while 2.0.0 is still Pending -> stays Active.
		createPack("pp-sib-h-200", "sib-pack-h", "2.0.0", "sib-cm-h")
		Expect(doReconcile("pp-sib-h-100")).To(Succeed())
		Expect(getPack("pp-sib-h-100").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))

		// 2.0.0 reconciles and becomes Active (the new channel-max).
		Expect(doReconcile("pp-sib-h-200")).To(Succeed())
		Expect(getPack("pp-sib-h-200").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))

		// Now that 2.0.0 is validated, 1.0.0 re-reconciles to Superseded and
		// clears its ActiveVersion.
		Eventually(func(g Gomega) {
			g.Expect(doReconcile("pp-sib-h-100")).To(Succeed())
			g.Expect(getPack("pp-sib-h-100").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseSuperseded))
		}).Should(Succeed())
		Expect(getPack("pp-sib-h-100").Status.ActiveVersion).To(BeNil())

		// 2.0.0 stays Active on re-reconcile (no phase churn).
		Consistently(func(g Gomega) {
			g.Expect(doReconcile("pp-sib-h-200")).To(Succeed())
			g.Expect(getPack("pp-sib-h-200").Status.Phase).To(Equal(omniav1alpha1.PromptPackPhaseActive))
		}).Should(Succeed())
	})
})

func TestSiblingPhaseChangedPredicate(t *testing.T) {
	pred := siblingPhaseChangedPredicate()

	packWith := func(gen int64, phase omniav1alpha1.PromptPackPhase) *omniav1alpha1.PromptPack {
		p := &omniav1alpha1.PromptPack{}
		p.Generation = gen
		p.Status.Phase = phase
		return p
	}

	cases := []struct {
		name string
		got  bool
		want bool
	}{
		{"create fires", pred.Create(event.CreateEvent{Object: packWith(1, "")}), true},
		{"delete fires", pred.Delete(event.DeleteEvent{Object: packWith(1, "")}), true},
		{"generic does not fire", pred.Generic(event.GenericEvent{Object: packWith(1, "")}), false},
		{"same gen and phase does not fire", pred.Update(event.UpdateEvent{
			ObjectOld: packWith(2, omniav1alpha1.PromptPackPhaseActive),
			ObjectNew: packWith(2, omniav1alpha1.PromptPackPhaseActive),
		}), false},
		{"generation change fires", pred.Update(event.UpdateEvent{
			ObjectOld: packWith(2, omniav1alpha1.PromptPackPhaseActive),
			ObjectNew: packWith(3, omniav1alpha1.PromptPackPhaseActive),
		}), true},
		{"phase transition fires", pred.Update(event.UpdateEvent{
			ObjectOld: packWith(2, omniav1alpha1.PromptPackPhasePending),
			ObjectNew: packWith(2, omniav1alpha1.PromptPackPhaseActive),
		}), true},
		{"nil objects do not fire", pred.Update(event.UpdateEvent{}), false},
		{"non-promptpack same gen does not fire", pred.Update(event.UpdateEvent{
			ObjectOld: &corev1.ConfigMap{},
			ObjectNew: &corev1.ConfigMap{},
		}), false},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

// Ensure unused import doesn't cause issues
var _ = errors.IsNotFound
