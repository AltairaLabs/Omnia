/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"io"
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/policy"
)

var _ = Describe("ToolPolicy Watcher envtest integration", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	discardLogger := func() *slog.Logger {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	Context("When using policy.Watcher with a real K8s API server", func() {
		It("should perform initial load of existing ToolPolicies", func() {
			eval, err := policy.NewEvaluator()
			Expect(err).NotTo(HaveOccurred())

			// Create a ToolPolicy via the real API
			tp := &omniav1alpha1.ToolPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "envtest-watcher-load",
					Namespace: "default",
				},
				Spec: omniav1alpha1.ToolPolicySpec{
					Selector: omniav1alpha1.ToolPolicySelector{
						Registry: "test-registry",
					},
					Rules: []omniav1alpha1.PolicyRule{
						{
							Name: "deny-all",
							Deny: omniav1alpha1.PolicyRuleDeny{
								CEL:     "true",
								Message: "always deny",
							},
						},
					},
					Mode:      omniav1alpha1.PolicyModeEnforce,
					OnFailure: omniav1alpha1.OnFailureDeny,
				},
			}
			Expect(k8sClient.Create(ctx, tp)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, tp)
			})

			// Create watcher using the envtest k8sClient
			w := policy.NewWatcher(
				eval,
				k8sClient,
				k8sClient.Scheme(),
				"default",
				discardLogger(),
			)
			Expect(w).NotTo(BeNil())

			// Start in a goroutine (blocks in pollLoop) and cancel after
			// the evaluator picks up the policy.
			watchCtx, watchCancel := context.WithCancel(ctx)
			defer watchCancel()

			go func() {
				defer GinkgoRecover()
				_ = w.Start(watchCtx)
			}()

			// The watcher's initial load should compile the policy
			Eventually(func() int {
				return eval.PolicyCount()
			}, timeout, interval).Should(Equal(1))
		})

		It("should load multiple policies cluster-wide", func() {
			eval, err := policy.NewEvaluator()
			Expect(err).NotTo(HaveOccurred())

			tp1 := &omniav1alpha1.ToolPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "envtest-watcher-multi1",
					Namespace: "default",
				},
				Spec: omniav1alpha1.ToolPolicySpec{
					Selector: omniav1alpha1.ToolPolicySelector{
						Registry: "registry-a",
					},
					Rules: []omniav1alpha1.PolicyRule{
						{
							Name: "rule-a",
							Deny: omniav1alpha1.PolicyRuleDeny{
								CEL:     "true",
								Message: "deny",
							},
						},
					},
					Mode:      omniav1alpha1.PolicyModeEnforce,
					OnFailure: omniav1alpha1.OnFailureDeny,
				},
			}
			Expect(k8sClient.Create(ctx, tp1)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, tp1)
			})

			tp2 := &omniav1alpha1.ToolPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "envtest-watcher-multi2",
					Namespace: "default",
				},
				Spec: omniav1alpha1.ToolPolicySpec{
					Selector: omniav1alpha1.ToolPolicySelector{
						Registry: "registry-b",
					},
					Rules: []omniav1alpha1.PolicyRule{
						{
							Name: "rule-b",
							Deny: omniav1alpha1.PolicyRuleDeny{
								CEL:     "false",
								Message: "allow",
							},
						},
					},
					Mode:      omniav1alpha1.PolicyModeEnforce,
					OnFailure: omniav1alpha1.OnFailureDeny,
				},
			}
			Expect(k8sClient.Create(ctx, tp2)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, tp2)
			})

			// Cluster-wide watcher (empty namespace)
			w := policy.NewWatcher(
				eval,
				k8sClient,
				k8sClient.Scheme(),
				"", // cluster-wide
				discardLogger(),
			)

			watchCtx, watchCancel := context.WithCancel(ctx)
			defer watchCancel()

			go func() {
				defer GinkgoRecover()
				_ = w.Start(watchCtx)
			}()

			// Both policies should be loaded
			Eventually(func() int {
				return eval.PolicyCount()
			}, timeout, interval).Should(BeNumerically(">=", 2))
		})

		It("should skip policies with invalid CEL without erroring", func() {
			eval, err := policy.NewEvaluator()
			Expect(err).NotTo(HaveOccurred())

			// Create a valid policy
			validTP := &omniav1alpha1.ToolPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "envtest-watcher-validcel",
					Namespace: "default",
				},
				Spec: omniav1alpha1.ToolPolicySpec{
					Selector: omniav1alpha1.ToolPolicySelector{
						Registry: "valid-registry",
					},
					Rules: []omniav1alpha1.PolicyRule{
						{
							Name: "valid-rule",
							Deny: omniav1alpha1.PolicyRuleDeny{
								CEL:     "true",
								Message: "deny",
							},
						},
					},
					Mode:      omniav1alpha1.PolicyModeEnforce,
					OnFailure: omniav1alpha1.OnFailureDeny,
				},
			}
			Expect(k8sClient.Create(ctx, validTP)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, validTP)
			})

			// Create a policy with invalid CEL
			invalidTP := &omniav1alpha1.ToolPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "envtest-watcher-badcel",
					Namespace: "default",
				},
				Spec: omniav1alpha1.ToolPolicySpec{
					Selector: omniav1alpha1.ToolPolicySelector{
						Registry: "invalid-registry",
					},
					Rules: []omniav1alpha1.PolicyRule{
						{
							Name: "bad-rule",
							Deny: omniav1alpha1.PolicyRuleDeny{
								CEL:     "invalid CEL %%%",
								Message: "should fail compilation",
							},
						},
					},
					Mode:      omniav1alpha1.PolicyModeEnforce,
					OnFailure: omniav1alpha1.OnFailureDeny,
				},
			}
			Expect(k8sClient.Create(ctx, invalidTP)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, invalidTP)
			})

			w := policy.NewWatcher(
				eval,
				k8sClient,
				k8sClient.Scheme(),
				"default",
				discardLogger(),
			)

			watchCtx, watchCancel := context.WithCancel(ctx)
			defer watchCancel()

			go func() {
				defer GinkgoRecover()
				_ = w.Start(watchCtx)
			}()

			// Only the valid policy should be compiled; invalid CEL is skipped
			Eventually(func() int {
				return eval.PolicyCount()
			}, timeout, interval).Should(BeNumerically(">=", 1))

			// The invalid policy should not be compiled —
			// verify via evaluator that only 1 policy is present (not 2)
			Consistently(func() int {
				return eval.PolicyCount()
			}, time.Second, interval).Should(Equal(1))
		})
	})
})
