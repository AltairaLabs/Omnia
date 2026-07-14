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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// versionTriggerEnvtestCounter gives each spec a unique resource suffix.
var versionTriggerEnvtestCounter uint64

// TestMaybeTriggerVersionRollout exercises the version-triggered rollout
// (#1838): when a newer PromptPack version appears on the agent's watched
// channel, the controller sets it as spec.rollout.candidate so the existing
// rollout engine canaries it, subject to first-deploy / in-flight /
// idempotency guards.
var _ = Describe("AgentRuntime version-triggered rollout (envtest)", func() {
	var (
		ctx       context.Context
		namespace string
		nextName  = func(prefix string) string {
			n := atomic.AddUint64(&versionTriggerEnvtestCounter, 1)
			return fmt.Sprintf("%s-%d", prefix, n)
		}
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = nextName("version-trigger-test")
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

	newTriggerAR := func(name, providerName, packName string) *omniav1alpha1.AgentRuntime {
		return &omniav1alpha1.AgentRuntime{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				PromptPackRef: omniav1alpha1.PromptPackRef{
					Name:    packName,
					Version: ptr.To("1.0.0"),
				},
				Facades: []omniav1alpha1.FacadeConfig{{
					Type: omniav1alpha1.FacadeTypeWebSocket,
				}},
				Providers: []omniav1alpha1.NamedProviderRef{
					{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: providerName}},
				},
				Rollout: &omniav1alpha1.RolloutConfig{
					Steps:   []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](20)}},
					Trigger: &omniav1alpha1.RolloutTrigger{PromptPackChannel: "stable"},
				},
			},
		}
	}

	newReconciler := func() *AgentRuntimeReconciler {
		return &AgentRuntimeReconciler{
			Client:          k8sClient,
			Scheme:          k8sClient.Scheme(),
			FacadeImage:     testFacadeImage,
			FrameworkImages: promptkitImage("test-runtime:v1.0.0"),
		}
	}

	Context("through the full Reconcile loop", func() {
		It("does not set a candidate on first deploy, then sets one once a newer stable version appears", func() {
			providerName := nextName("provider")
			Expect(k8sClient.Create(ctx, newProvider(providerName))).To(Succeed())

			packName := nextName("pack")
			v1 := newLabeledPack(nextName(packName+"-100"), packName, "1.0.0")
			Expect(k8sClient.Create(ctx, v1)).To(Succeed())

			arName := nextName("ar")
			ar := newTriggerAR(arName, providerName, packName)
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			reconciler := newReconciler()
			key := types.NamespacedName{Name: arName, Namespace: namespace}

			// First reconcile adds the finalizer; second does the real work.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			// First-deploy: activeVersion was nil going into this pass, so no
			// candidate should be set even though the trigger is configured.
			current := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, key, current)).To(Succeed())
			Expect(current.Status.ActiveVersion).NotTo(BeNil())
			Expect(*current.Status.ActiveVersion).To(Equal("1.0.0"))
			Expect(current.Spec.Rollout.Candidate).To(BeNil(),
				"first deploy must not canary: no stable pod existed yet")

			// Publish a newer stable version.
			v2 := newLabeledPack(nextName(packName+"-110"), packName, "1.1.0")
			Expect(k8sClient.Create(ctx, v2)).To(Succeed())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, key, current)).To(Succeed())
			Expect(current.Spec.Rollout.Candidate).NotTo(BeNil())
			Expect(current.Spec.Rollout.Candidate.PromptPackRef).NotTo(BeNil())
			Expect(current.Spec.Rollout.Candidate.PromptPackRef.Name).To(Equal(packName))
			Expect(current.Spec.Rollout.Candidate.PromptPackRef.Version).NotTo(BeNil())
			Expect(*current.Spec.Rollout.Candidate.PromptPackRef.Version).To(Equal("1.1.0"))

			// Idempotent: reconciling again with the same channel-latest must not
			// change (or re-set) the candidate. isRolloutActive is now true
			// (candidate differs from the pinned stable spec), so the next pass's
			// in-flight guard leaves it untouched.
			resourceVersionBefore := current.Spec.Rollout.Candidate.PromptPackRef.Version
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, key, current)).To(Succeed())
			Expect(current.Spec.Rollout.Candidate.PromptPackRef.Version).To(Equal(resourceVersionBefore))
		})

		It("does not set a candidate when the channel has no newer version than active", func() {
			providerName := nextName("provider")
			Expect(k8sClient.Create(ctx, newProvider(providerName))).To(Succeed())

			packName := nextName("pack")
			v1 := newLabeledPack(nextName(packName+"-100"), packName, "1.0.0")
			Expect(k8sClient.Create(ctx, v1)).To(Succeed())

			arName := nextName("ar")
			ar := newTriggerAR(arName, providerName, packName)
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())

			reconciler := newReconciler()
			key := types.NamespacedName{Name: arName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())
			// No newer pack published: reconcile again, still no candidate.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			Expect(err).NotTo(HaveOccurred())

			current := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, key, current)).To(Succeed())
			Expect(current.Spec.Rollout.Candidate).To(BeNil())
		})
	})

	Context("maybeTriggerVersionRollout guards (direct calls)", func() {
		// setActiveVersion persists status.activeVersion directly, modeling an
		// agent that already has a healthy stable pod on that version.
		setActiveVersion := func(ar *omniav1alpha1.AgentRuntime, version string) {
			GinkgoHelper()
			ar.Status.ActiveVersion = &version
			Expect(k8sClient.Status().Update(ctx, ar)).To(Succeed())
		}

		It("does not set a candidate while a manually-triggered rollout is already active", func() {
			providerName := nextName("provider")
			Expect(k8sClient.Create(ctx, newProvider(providerName))).To(Succeed())

			packName := nextName("pack")
			Expect(k8sClient.Create(ctx, newLabeledPack(nextName(packName+"-100"), packName, "1.0.0"))).To(Succeed())
			Expect(k8sClient.Create(ctx, newLabeledPack(nextName(packName+"-110"), packName, "1.1.0"))).To(Succeed())

			arName := nextName("ar")
			ar := newTriggerAR(arName, providerName, packName)
			// Simulate an already-active, manually-set candidate (differs from
			// the pinned stable spec) so isRolloutActive(ar) is true.
			ar.Spec.Rollout.Candidate = &omniav1alpha1.CandidateOverrides{
				PromptPackRef: &omniav1alpha1.PromptPackRef{Name: packName, Version: ptr.To("9.9.9")},
			}
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())
			setActiveVersion(ar, "1.0.0")

			reconciler := newReconciler()
			triggered, err := reconciler.maybeTriggerVersionRollout(ctx, ar)
			Expect(err).NotTo(HaveOccurred())
			Expect(triggered).To(BeFalse())
			Expect(*ar.Spec.Rollout.Candidate.PromptPackRef.Version).To(Equal("9.9.9"),
				"in-flight rollout must not be disturbed by the version trigger")
		})

		It("does not set a candidate while a promotion is in progress", func() {
			providerName := nextName("provider")
			Expect(k8sClient.Create(ctx, newProvider(providerName))).To(Succeed())

			packName := nextName("pack")
			Expect(k8sClient.Create(ctx, newLabeledPack(nextName(packName+"-100"), packName, "1.0.0"))).To(Succeed())
			Expect(k8sClient.Create(ctx, newLabeledPack(nextName(packName+"-110"), packName, "1.1.0"))).To(Succeed())

			arName := nextName("ar")
			ar := newTriggerAR(arName, providerName, packName)
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())
			ar.Status.Rollout = &omniav1alpha1.RolloutStatus{Active: true, Promoting: true}
			setActiveVersion(ar, "1.0.0")

			reconciler := newReconciler()
			triggered, err := reconciler.maybeTriggerVersionRollout(ctx, ar)
			Expect(err).NotTo(HaveOccurred())
			Expect(triggered).To(BeFalse())
			Expect(ar.Spec.Rollout.Candidate).To(BeNil())
		})

		It("does not set a candidate on first deploy (activeVersion nil)", func() {
			providerName := nextName("provider")
			Expect(k8sClient.Create(ctx, newProvider(providerName))).To(Succeed())

			packName := nextName("pack")
			Expect(k8sClient.Create(ctx, newLabeledPack(nextName(packName+"-100"), packName, "1.0.0"))).To(Succeed())
			Expect(k8sClient.Create(ctx, newLabeledPack(nextName(packName+"-110"), packName, "1.1.0"))).To(Succeed())

			arName := nextName("ar")
			ar := newTriggerAR(arName, providerName, packName)
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())
			// ar.Status.ActiveVersion left nil (first deploy).

			reconciler := newReconciler()
			triggered, err := reconciler.maybeTriggerVersionRollout(ctx, ar)
			Expect(err).NotTo(HaveOccurred())
			Expect(triggered).To(BeFalse())
			Expect(ar.Spec.Rollout.Candidate).To(BeNil())
		})

		It("sets the candidate exactly once for a newer channel version, then is idempotent", func() {
			providerName := nextName("provider")
			Expect(k8sClient.Create(ctx, newProvider(providerName))).To(Succeed())

			packName := nextName("pack")
			Expect(k8sClient.Create(ctx, newLabeledPack(nextName(packName+"-100"), packName, "1.0.0"))).To(Succeed())
			Expect(k8sClient.Create(ctx, newLabeledPack(nextName(packName+"-110"), packName, "1.1.0"))).To(Succeed())

			arName := nextName("ar")
			ar := newTriggerAR(arName, providerName, packName)
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())
			setActiveVersion(ar, "1.0.0")

			reconciler := newReconciler()
			triggered, err := reconciler.maybeTriggerVersionRollout(ctx, ar)
			Expect(err).NotTo(HaveOccurred())
			Expect(triggered).To(BeTrue())
			Expect(ar.Spec.Rollout.Candidate).NotTo(BeNil())
			Expect(*ar.Spec.Rollout.Candidate.PromptPackRef.Version).To(Equal("1.1.0"))

			// Second call: isRolloutActive is now true (candidate differs from the
			// pinned stable spec) so the in-flight guard makes this idempotent.
			triggered, err = reconciler.maybeTriggerVersionRollout(ctx, ar)
			Expect(err).NotTo(HaveOccurred())
			Expect(triggered).To(BeFalse())
			Expect(*ar.Spec.Rollout.Candidate.PromptPackRef.Version).To(Equal("1.1.0"))
		})

		It("does not re-canary a just-rolled-back version, but canaries a newer one", func() {
			providerName := nextName("provider")
			Expect(k8sClient.Create(ctx, newProvider(providerName))).To(Succeed())

			packName := nextName("pack")
			Expect(k8sClient.Create(ctx, newLabeledPack(nextName(packName+"-100"), packName, "1.0.0"))).To(Succeed())
			Expect(k8sClient.Create(ctx, newLabeledPack(nextName(packName+"-200"), packName, "2.0.0"))).To(Succeed())

			ar := newTriggerAR(nextName("ar"), providerName, packName)
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())
			setActiveVersion(ar, "1.0.0")

			// Post-rollback state: 2.0.0 was canaried and rolled back, so it is
			// stamped as the last rolled-back version and the candidate is back
			// to stable while activeVersion stays 1.0.0.
			ar.Annotations = map[string]string{lastRolledBackVersionAnnotation: "2.0.0"}
			ar.Spec.Rollout.Candidate = nil

			reconciler := newReconciler()
			triggered, err := reconciler.maybeTriggerVersionRollout(ctx, ar)
			Expect(err).NotTo(HaveOccurred())
			Expect(triggered).To(BeFalse(), "must not re-canary the just-rolled-back 2.0.0")
			Expect(ar.Spec.Rollout.Candidate).To(BeNil())

			// A newer version (3.0.0) appears -> the trigger fires again.
			Expect(k8sClient.Create(ctx, newLabeledPack(nextName(packName+"-300"), packName, "3.0.0"))).To(Succeed())
			triggered, err = reconciler.maybeTriggerVersionRollout(ctx, ar)
			Expect(err).NotTo(HaveOccurred())
			Expect(triggered).To(BeTrue())
			Expect(*ar.Spec.Rollout.Candidate.PromptPackRef.Version).To(Equal("3.0.0"))
		})

		It("does not error when nothing has been published on the channel yet", func() {
			providerName := nextName("provider")
			Expect(k8sClient.Create(ctx, newProvider(providerName))).To(Succeed())

			// No PromptPack objects at all for this packName.
			packName := nextName("pack")

			arName := nextName("ar")
			ar := newTriggerAR(arName, providerName, packName)
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())
			setActiveVersion(ar, "1.0.0")

			reconciler := newReconciler()
			triggered, err := reconciler.maybeTriggerVersionRollout(ctx, ar)
			Expect(err).NotTo(HaveOccurred())
			Expect(triggered).To(BeFalse())
			Expect(ar.Spec.Rollout.Candidate).To(BeNil())
		})

		It("propagates the error when persisting the candidate conflicts", func() {
			providerName := nextName("provider")
			Expect(k8sClient.Create(ctx, newProvider(providerName))).To(Succeed())

			packName := nextName("pack")
			Expect(k8sClient.Create(ctx, newLabeledPack(nextName(packName+"-100"), packName, "1.0.0"))).To(Succeed())
			Expect(k8sClient.Create(ctx, newLabeledPack(nextName(packName+"-110"), packName, "1.1.0"))).To(Succeed())

			arName := nextName("ar")
			ar := newTriggerAR(arName, providerName, packName)
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())
			setActiveVersion(ar, "1.0.0")

			// Make the in-memory ar stale by mutating and persisting a fresh copy
			// out-of-band, so r.Update(ctx, ar) below hits a resourceVersion
			// conflict when maybeTriggerVersionRollout tries to persist the
			// candidate it computed against the stale object.
			fresh := &omniav1alpha1.AgentRuntime{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: arName, Namespace: namespace}, fresh)).To(Succeed())
			fresh.Spec.Rollout.Steps = append(fresh.Spec.Rollout.Steps, omniav1alpha1.RolloutStep{SetWeight: ptr.To[int32](40)})
			Expect(k8sClient.Update(ctx, fresh)).To(Succeed())

			reconciler := newReconciler()
			_, err := reconciler.maybeTriggerVersionRollout(ctx, ar)
			Expect(err).To(HaveOccurred())
		})

		It("does not set a candidate when the channel-latest equals the active version", func() {
			providerName := nextName("provider")
			Expect(k8sClient.Create(ctx, newProvider(providerName))).To(Succeed())

			packName := nextName("pack")
			Expect(k8sClient.Create(ctx, newLabeledPack(nextName(packName+"-100"), packName, "1.0.0"))).To(Succeed())

			arName := nextName("ar")
			ar := newTriggerAR(arName, providerName, packName)
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())
			setActiveVersion(ar, "1.0.0")

			reconciler := newReconciler()
			triggered, err := reconciler.maybeTriggerVersionRollout(ctx, ar)
			Expect(err).NotTo(HaveOccurred())
			Expect(triggered).To(BeFalse())
			Expect(ar.Spec.Rollout.Candidate).To(BeNil())
		})
	})
})
