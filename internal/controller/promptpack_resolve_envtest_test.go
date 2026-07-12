/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	stderrors "errors"
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// promptPackResolveEnvtestCounter gives each spec a unique resource suffix.
var promptPackResolveEnvtestCounter uint64

// TestReconcileForwardResolution exercises the FULL Reconcile() path (through
// reconcileReferences -> resolvePromptPack), the hard cutover from
// name-keyed Get to label+version/track List+select (#1837 Task 2). Unlike
// the rollout envtest suites, which inject an already-resolved PromptPack
// directly and never touch resolvePromptPack for the stable ref, these specs
// verify the stable ref is actually resolved by the reconciler.
var _ = Describe("AgentRuntime forward PromptPack resolution (envtest)", func() {
	var (
		ctx       context.Context
		namespace string
		nextName  = func(prefix string) string {
			n := atomic.AddUint64(&promptPackResolveEnvtestCounter, 1)
			return fmt.Sprintf("%s-%d", prefix, n)
		}
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = nextName("promptpack-resolve-test")
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

	// newLabeledPack creates a real PromptPack sharing packName as its label
	// value, with a distinct object name and ConfigMap ref per version so the
	// mounted volume identifies which version the reconciler resolved.
	newLabeledPack := func(objName, packName, version string) *omniav1alpha1.PromptPack {
		return &omniav1alpha1.PromptPack{
			ObjectMeta: metav1.ObjectMeta{
				Name:      objName,
				Namespace: namespace,
				Labels:    map[string]string{LabelPromptPackName: packName},
			},
			Spec: omniav1alpha1.PromptPackSpec{
				PackName: packName,
				Version:  version,
				Source: omniav1alpha1.PromptPackContentSource{
					Type:         omniav1alpha1.PromptPackSourceTypeConfigMap,
					ConfigMapRef: &corev1.LocalObjectReference{Name: objName + "-config"},
				},
			},
		}
	}

	newProvider := func(name string) *omniav1alpha1.Provider {
		return &omniav1alpha1.Provider{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: omniav1alpha1.ProviderSpec{
				Type:  omniav1alpha1.ProviderTypeClaude,
				Model: "claude-sonnet-4-20250514",
				Credential: &omniav1alpha1.CredentialConfig{
					SecretRef: &omniav1alpha1.SecretKeyRef{Name: "test-secret"},
				},
			},
		}
	}

	newAgentRuntime := func(name, providerName string, ref omniav1alpha1.PromptPackRef) *omniav1alpha1.AgentRuntime {
		return &omniav1alpha1.AgentRuntime{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				PromptPackRef: ref,
				Facades: []omniav1alpha1.FacadeConfig{{
					Type: omniav1alpha1.FacadeTypeWebSocket,
				}},
				Providers: []omniav1alpha1.NamedProviderRef{
					{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: providerName}},
				},
			},
		}
	}

	// mountedConfigMap returns the ConfigMap name mounted at the PromptPack
	// content volume of the stable Deployment, so specs can assert which
	// PromptPack version the reconciler actually resolved.
	mountedConfigMap := func(deployName string) string {
		GinkgoHelper()
		deploy := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: deployName, Namespace: namespace}, deploy)).To(Succeed())
		for _, v := range deploy.Spec.Template.Spec.Volumes {
			if v.Name == promptpackConfigVolumeName && v.ConfigMap != nil {
				return v.ConfigMap.Name
			}
		}
		return ""
	}

	Context("channel resolution", func() {
		It("resolves the highest non-prerelease version on the stable channel", func() {
			providerName := nextName("provider")
			Expect(k8sClient.Create(ctx, newProvider(providerName))).To(Succeed())

			packName := "mypack"
			p1 := newLabeledPack(nextName("mypack-100"), packName, "1.0.0")
			p2 := newLabeledPack(nextName("mypack-110"), packName, "1.1.0")
			Expect(k8sClient.Create(ctx, p1)).To(Succeed())
			Expect(k8sClient.Create(ctx, p2)).To(Succeed())

			arName := nextName("ar")
			ar := newAgentRuntime(arName, providerName, omniav1alpha1.PromptPackRef{
				Name:  packName,
				Track: ptr.To("stable"),
			})
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			reconciler := &AgentRuntimeReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				FacadeImage:     testFacadeImage,
				FrameworkImages: promptkitImage("test-runtime:v1.0.0"),
			}
			key := types.NamespacedName{Name: arName, Namespace: namespace}

			// First reconcile adds the finalizer; second does the real work.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			Expect(mountedConfigMap(arName)).To(Equal(p2.Spec.Source.ConfigMapRef.Name),
				"channel-max on track=stable should resolve the highest version (1.1.0), not the first-created")
		})
	})

	Context("exact version pin", func() {
		It("resolves exactly the pinned version, not the channel-max", func() {
			providerName := nextName("provider")
			Expect(k8sClient.Create(ctx, newProvider(providerName))).To(Succeed())

			packName := "mypack"
			p1 := newLabeledPack(nextName("mypack-100"), packName, "1.0.0")
			p2 := newLabeledPack(nextName("mypack-110"), packName, "1.1.0")
			Expect(k8sClient.Create(ctx, p1)).To(Succeed())
			Expect(k8sClient.Create(ctx, p2)).To(Succeed())

			arName := nextName("ar")
			ar := newAgentRuntime(arName, providerName, omniav1alpha1.PromptPackRef{
				Name:    packName,
				Version: ptr.To("1.0.0"),
			})
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			reconciler := &AgentRuntimeReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				FacadeImage:     testFacadeImage,
				FrameworkImages: promptkitImage("test-runtime:v1.0.0"),
			}
			key := types.NamespacedName{Name: arName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			Expect(mountedConfigMap(arName)).To(Equal(p1.Spec.Source.ConfigMapRef.Name),
				"an exact version pin must resolve that version even though a newer one exists")
		})
	})

	Context("no matching PromptPack", func() {
		It("leaves PromptPackReady=False with a not-found reason", func() {
			providerName := nextName("provider")
			Expect(k8sClient.Create(ctx, newProvider(providerName))).To(Succeed())

			packName := "mypack"
			p1 := newLabeledPack(nextName("mypack-100"), packName, "1.0.0")
			Expect(k8sClient.Create(ctx, p1)).To(Succeed())

			arName := nextName("ar")
			ar := newAgentRuntime(arName, providerName, omniav1alpha1.PromptPackRef{
				Name:    packName,
				Version: ptr.To("9.9.9"),
			})
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			reconciler := &AgentRuntimeReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				FacadeImage:     testFacadeImage,
				FrameworkImages: promptkitImage("test-runtime:v1.0.0"),
			}
			key := types.NamespacedName{Name: arName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).To(HaveOccurred())
			Expect(stderrors.Is(err, errNoMatchingPromptPack)).To(BeTrue(),
				"an unmatched version pin must fail with errNoMatchingPromptPack: %v", err)

			updated := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, key, updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.AgentRuntimePhaseFailed))

			found := false
			for _, c := range updated.Status.Conditions {
				if c.Type == ConditionTypePromptPackReady {
					found = true
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal("PromptPackNotFound"))
				}
			}
			Expect(found).To(BeTrue(), "PromptPackReady condition should be present")
		})
	})

	Context("invalid ref: both version and track set", func() {
		// A CEL XValidation rule on PromptPackRef (#1837 review) now rejects
		// version+track at the apiserver, before this object can ever reach
		// the reconciler — so the object can no longer be Created via the real
		// API to reach the reconcile-time defensive check below. That
		// reconcile-time check (selectPromptPack's both-set rejection,
		// reported as PromptPackRefInvalid) is retained as defense-in-depth
		// for objects persisted before this CRD validation existed; this spec
		// now asserts the earlier, apiserver-level rejection instead.
		It("rejects the AgentRuntime at admission with a mutual-exclusion CEL error", func() {
			providerName := nextName("provider")
			Expect(k8sClient.Create(ctx, newProvider(providerName))).To(Succeed())

			packName := "mypack"
			p1 := newLabeledPack(nextName("mypack-100"), packName, "1.0.0")
			Expect(k8sClient.Create(ctx, p1)).To(Succeed())

			arName := nextName("ar")
			ar := newAgentRuntime(arName, providerName, omniav1alpha1.PromptPackRef{
				Name:    packName,
				Version: ptr.To("1.0.0"),
				Track:   ptr.To("stable"),
			})
			Expect(k8sClient.Create(ctx, ar)).To(MatchError(ContainSubstring("mutually exclusive")))
		})
	})
})
