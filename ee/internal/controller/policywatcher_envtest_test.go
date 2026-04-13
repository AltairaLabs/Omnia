/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

var _ = Describe("PolicyWatcher envtest integration", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	// ensureNamespace creates a namespace if it doesn't exist.
	ensureNamespace := func(name string) {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
		_ = k8sClient.Create(ctx, ns)
	}

	Context("When watching SessionPrivacyPolicy CRDs via a real API server", func() {
		It("should create a PolicyWatcher with a controller-runtime client", func() {
			k8s, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
			Expect(err).NotTo(HaveOccurred())

			pw := privacy.NewPolicyWatcher(k8s, logr.Discard())
			pw.SetPollInterval(500 * time.Millisecond)
			Expect(pw).NotTo(BeNil())
		})

		It("should return the global default policy (omnia-system/default)", func() {
			k8s, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
			Expect(err).NotTo(HaveOccurred())

			pw := privacy.NewPolicyWatcher(k8s, logr.Discard())
			pw.SetPollInterval(500 * time.Millisecond)

			watchCtx, watchCancel := context.WithCancel(ctx)
			defer watchCancel()

			go func() {
				defer GinkgoRecover()
				_ = pw.Start(watchCtx)
			}()

			// Initially no policies — GetEffectivePolicy should return nil
			Expect(pw.GetEffectivePolicy("default", "my-agent")).To(BeNil())

			// Create the global default policy in omnia-system
			ensureNamespace("omnia-system")
			policy := &omniav1alpha1.SessionPrivacyPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default",
					Namespace: "omnia-system",
				},
				Spec: omniav1alpha1.SessionPrivacyPolicySpec{
					Recording: omniav1alpha1.RecordingConfig{
						Enabled: true,
						PII: &omniav1alpha1.PIIConfig{
							Redact:   true,
							Patterns: []string{"ssn"},
						},
					},
					UserOptOut: &omniav1alpha1.UserOptOutConfig{Enabled: true},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, policy)
			})

			// The poll loop should pick up the policy within a few seconds
			Eventually(func(g Gomega) {
				ep := pw.GetEffectivePolicy("any-ns", "any-agent")
				g.Expect(ep).NotTo(BeNil())
				g.Expect(ep.Recording.Enabled).To(BeTrue())
				g.Expect(ep.Recording.PII).NotTo(BeNil())
				g.Expect(ep.Recording.PII.Redact).To(BeTrue())
				g.Expect(ep.UserOptOut).NotTo(BeNil())
				g.Expect(ep.UserOptOut.Enabled).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("should detect policy deletion and return nil", func() {
			k8s, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
			Expect(err).NotTo(HaveOccurred())

			pw := privacy.NewPolicyWatcher(k8s, logr.Discard())
			pw.SetPollInterval(500 * time.Millisecond)

			ensureNamespace("omnia-system")
			policy := &omniav1alpha1.SessionPrivacyPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default",
					Namespace: "omnia-system",
				},
				Spec: omniav1alpha1.SessionPrivacyPolicySpec{
					Recording: omniav1alpha1.RecordingConfig{Enabled: true},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			watchCtx, watchCancel := context.WithCancel(ctx)
			defer watchCancel()

			go func() {
				defer GinkgoRecover()
				_ = pw.Start(watchCtx)
			}()

			// Wait until the policy appears in cache
			Eventually(func() *privacy.EffectivePolicy {
				return pw.GetEffectivePolicy("any-ns", "agent")
			}, timeout, interval).ShouldNot(BeNil())

			// Delete the policy
			Expect(k8sClient.Delete(ctx, policy)).To(Succeed())

			// Wait until it disappears from cache
			Eventually(func() *privacy.EffectivePolicy {
				return pw.GetEffectivePolicy("any-ns", "agent")
			}, timeout, interval).Should(BeNil())
		})

		It("should resolve agent-override policy over global default", func() {
			k8s, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
			Expect(err).NotTo(HaveOccurred())

			pw := privacy.NewPolicyWatcher(k8s, logr.Discard())
			pw.SetPollInterval(500 * time.Millisecond)

			watchCtx, watchCancel := context.WithCancel(ctx)
			defer watchCancel()

			go func() {
				defer GinkgoRecover()
				_ = pw.Start(watchCtx)
			}()

			testNS := "envtest-agent-override"
			ensureNamespace("omnia-system")
			ensureNamespace(testNS)

			// Global default: recording enabled
			globalPolicy := &omniav1alpha1.SessionPrivacyPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "omnia-system"},
				Spec: omniav1alpha1.SessionPrivacyPolicySpec{
					Recording: omniav1alpha1.RecordingConfig{Enabled: true},
				},
			}
			Expect(k8sClient.Create(ctx, globalPolicy)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, globalPolicy) })

			// Agent-specific policy: recording disabled
			agentPolicy := &omniav1alpha1.SessionPrivacyPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "strict", Namespace: testNS},
				Spec: omniav1alpha1.SessionPrivacyPolicySpec{
					Recording: omniav1alpha1.RecordingConfig{Enabled: false},
				},
			}
			Expect(k8sClient.Create(ctx, agentPolicy)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, agentPolicy) })

			// AgentRuntime references the strict policy.
			// Must set required fields: facade.type and promptPackRef.name.
			ar := &corev1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: testNS},
			}
			ar.Spec.Facade.Type = corev1alpha1.FacadeTypeWebSocket
			ar.Spec.PromptPackRef = corev1alpha1.PromptPackRef{Name: "placeholder"}
			ar.Spec.PrivacyPolicyRef = &corev1.LocalObjectReference{Name: "strict"}
			Expect(k8sClient.Create(ctx, ar)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, ar) })

			// Eventually the agent override should apply (recording=false)
			Eventually(func(g Gomega) {
				ep := pw.GetEffectivePolicy(testNS, "test-agent")
				g.Expect(ep).NotTo(BeNil())
				g.Expect(ep.Recording.Enabled).To(BeFalse())
			}, timeout, interval).Should(Succeed())

			// A different agent in the same namespace should fall back to global
			Eventually(func(g Gomega) {
				ep := pw.GetEffectivePolicy(testNS, "other-agent")
				g.Expect(ep).NotTo(BeNil())
				g.Expect(ep.Recording.Enabled).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})
})
