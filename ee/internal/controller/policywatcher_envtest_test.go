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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

var _ = Describe("PolicyWatcher envtest integration", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When watching SessionPrivacyPolicy CRDs via a real API server", func() {
		It("should create a PolicyWatcher with the envtest config", func() {
			pw, err := privacy.NewPolicyWatcher(cfg, logr.Discard())
			Expect(err).NotTo(HaveOccurred())
			Expect(pw).NotTo(BeNil())
		})

		It("should sync cache and detect created policies", func() {
			pw, err := privacy.NewPolicyWatcher(cfg, logr.Discard())
			Expect(err).NotTo(HaveOccurred())

			watchCtx, watchCancel := context.WithCancel(ctx)
			defer watchCancel()

			err = pw.Start(watchCtx)
			Expect(err).NotTo(HaveOccurred())

			// Initially no policies — GetEffectivePolicy should return nil
			Expect(pw.GetEffectivePolicy("default", "my-agent")).To(BeNil())

			// Create a global SessionPrivacyPolicy via the real K8s API
			policy := &omniav1alpha1.SessionPrivacyPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "envtest-global-policy",
				},
				Spec: omniav1alpha1.SessionPrivacyPolicySpec{
					Level: omniav1alpha1.PolicyLevelGlobal,
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

			// The informer should pick up the policy within a few seconds
			Eventually(func(g Gomega) {
				ep := pw.GetEffectivePolicy("default", "my-agent")
				g.Expect(ep).NotTo(BeNil())
				g.Expect(ep.Recording.Enabled).To(BeTrue())
				g.Expect(ep.Recording.PII).NotTo(BeNil())
				g.Expect(ep.Recording.PII.Redact).To(BeTrue())
				g.Expect(ep.UserOptOut).NotTo(BeNil())
				g.Expect(ep.UserOptOut.Enabled).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("should detect policy deletion", func() {
			pw, err := privacy.NewPolicyWatcher(cfg, logr.Discard())
			Expect(err).NotTo(HaveOccurred())

			watchCtx, watchCancel := context.WithCancel(ctx)
			defer watchCancel()

			// Create the policy before starting the watcher
			policy := &omniav1alpha1.SessionPrivacyPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "envtest-delete-policy",
				},
				Spec: omniav1alpha1.SessionPrivacyPolicySpec{
					Level: omniav1alpha1.PolicyLevelGlobal,
					Recording: omniav1alpha1.RecordingConfig{
						Enabled: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			err = pw.Start(watchCtx)
			Expect(err).NotTo(HaveOccurred())

			// Wait until the policy appears in cache
			Eventually(func() *privacy.EffectivePolicy {
				return pw.GetEffectivePolicy("default", "agent")
			}, timeout, interval).ShouldNot(BeNil())

			// Delete the policy
			Expect(k8sClient.Delete(ctx, policy)).To(Succeed())

			// Wait until it disappears from cache
			Eventually(func() *privacy.EffectivePolicy {
				return pw.GetEffectivePolicy("default", "agent")
			}, timeout, interval).Should(BeNil())
		})

		It("should observe workspace-scoped policies", func() {
			pw, err := privacy.NewPolicyWatcher(cfg, logr.Discard())
			Expect(err).NotTo(HaveOccurred())

			watchCtx, watchCancel := context.WithCancel(ctx)
			defer watchCancel()

			err = pw.Start(watchCtx)
			Expect(err).NotTo(HaveOccurred())

			// Create a global policy as the parent
			globalPolicy := &omniav1alpha1.SessionPrivacyPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "envtest-ws-global",
				},
				Spec: omniav1alpha1.SessionPrivacyPolicySpec{
					Level: omniav1alpha1.PolicyLevelGlobal,
					Recording: omniav1alpha1.RecordingConfig{
						Enabled: true,
						PII: &omniav1alpha1.PIIConfig{
							Redact:   true,
							Patterns: []string{"ssn"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, globalPolicy)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, globalPolicy)
			})

			// Create a workspace-scoped policy
			wsPolicy := &omniav1alpha1.SessionPrivacyPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "envtest-ws-policy",
				},
				Spec: omniav1alpha1.SessionPrivacyPolicySpec{
					Level:        omniav1alpha1.PolicyLevelWorkspace,
					WorkspaceRef: &corev1alpha1.LocalObjectReference{Name: "test-ns"},
					Recording: omniav1alpha1.RecordingConfig{
						Enabled: true,
						PII: &omniav1alpha1.PIIConfig{
							Redact:  true,
							Encrypt: true,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, wsPolicy)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, wsPolicy)
			})

			// The effective policy for test-ns should merge global + workspace
			Eventually(func(g Gomega) {
				ep := pw.GetEffectivePolicy("test-ns", "some-agent")
				g.Expect(ep).NotTo(BeNil())
				g.Expect(ep.Recording.PII).NotTo(BeNil())
				g.Expect(ep.Recording.PII.Redact).To(BeTrue())
				g.Expect(ep.Recording.PII.Encrypt).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			// A different namespace should only get the global policy
			Eventually(func(g Gomega) {
				ep := pw.GetEffectivePolicy("other-ns", "some-agent")
				g.Expect(ep).NotTo(BeNil())
				g.Expect(ep.Recording.PII).NotTo(BeNil())
				g.Expect(ep.Recording.PII.Redact).To(BeTrue())
				g.Expect(ep.Recording.PII.Encrypt).To(BeFalse())
			}, timeout, interval).Should(Succeed())
		})
	})
})
