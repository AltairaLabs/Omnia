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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/policy"
)

var _ = Describe("ToolPolicy Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	var (
		reconciler *ToolPolicyReconciler
		evaluator  *policy.Evaluator
	)

	BeforeEach(func() {
		var err error
		evaluator, err = policy.NewEvaluator()
		Expect(err).NotTo(HaveOccurred())

		reconciler = &ToolPolicyReconciler{
			Client:    k8sClient,
			Scheme:    k8sClient.Scheme(),
			Recorder:  &record.FakeRecorder{Events: make(chan string, 100)},
			Evaluator: evaluator,
		}
	})

	Context("When reconciling a valid ToolPolicy", func() {
		It("should set Active phase and compile rules", func() {
			policyName := "test-valid-policy"
			tp := &omniav1alpha1.ToolPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      policyName,
					Namespace: "default",
				},
				Spec: omniav1alpha1.ToolPolicySpec{
					Selector: omniav1alpha1.ToolPolicySelector{
						Registry: "customer-tools",
						Tools:    []string{"process_refund"},
					},
					Rules: []omniav1alpha1.PolicyRule{
						{
							Name: "amount-limit",
							Deny: omniav1alpha1.PolicyRuleDeny{
								CEL:     "int(headers['X-Omnia-Param-Amount']) > 1000",
								Message: "Refunds over $1000 not allowed",
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

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      policyName,
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			updated := &omniav1alpha1.ToolPolicy{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      policyName,
					Namespace: "default",
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal(omniav1alpha1.ToolPolicyPhaseActive))
				g.Expect(updated.Status.RuleCount).To(Equal(int32(1)))
			}, timeout, interval).Should(Succeed())

			Expect(evaluator.PolicyCount()).To(Equal(1))
		})
	})

	Context("When reconciling a ToolPolicy with invalid CEL", func() {
		It("should set Error phase", func() {
			policyName := "test-invalid-cel"
			tp := &omniav1alpha1.ToolPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      policyName,
					Namespace: "default",
				},
				Spec: omniav1alpha1.ToolPolicySpec{
					Selector: omniav1alpha1.ToolPolicySelector{
						Registry: "test-registry",
					},
					Rules: []omniav1alpha1.PolicyRule{
						{
							Name: "bad-rule",
							Deny: omniav1alpha1.PolicyRuleDeny{
								CEL:     "this is not valid CEL %%%",
								Message: "should not compile",
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

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      policyName,
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			updated := &omniav1alpha1.ToolPolicy{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      policyName,
					Namespace: "default",
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal(omniav1alpha1.ToolPolicyPhaseError))
				g.Expect(updated.Status.RuleCount).To(Equal(int32(0)))
			}, timeout, interval).Should(Succeed())

			Expect(evaluator.PolicyCount()).To(Equal(0))
		})
	})

	Context("When a ToolPolicy is deleted", func() {
		It("should remove the policy from the evaluator", func() {
			policyName := "test-delete-policy"
			tp := &omniav1alpha1.ToolPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      policyName,
					Namespace: "default",
				},
				Spec: omniav1alpha1.ToolPolicySpec{
					Selector: omniav1alpha1.ToolPolicySelector{
						Registry: "test-registry",
					},
					Rules: []omniav1alpha1.PolicyRule{
						{
							Name: "rule1",
							Deny: omniav1alpha1.PolicyRuleDeny{
								CEL:     "true",
								Message: "deny all",
							},
						},
					},
					Mode:      omniav1alpha1.PolicyModeEnforce,
					OnFailure: omniav1alpha1.OnFailureDeny,
				},
			}

			Expect(k8sClient.Create(ctx, tp)).To(Succeed())
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      policyName,
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(evaluator.PolicyCount()).To(Equal(1))

			Expect(k8sClient.Delete(ctx, tp)).To(Succeed())

			Eventually(func(g Gomega) {
				_, reconcileErr := reconciler.Reconcile(context.Background(), ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      policyName,
						Namespace: "default",
					},
				})
				g.Expect(reconcileErr).NotTo(HaveOccurred())
				g.Expect(evaluator.PolicyCount()).To(Equal(0))
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("When reconciling a ToolPolicy with multiple rules", func() {
		It("should compile all rules and set correct ruleCount", func() {
			policyName := "test-multi-rule"
			tp := &omniav1alpha1.ToolPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      policyName,
					Namespace: "default",
				},
				Spec: omniav1alpha1.ToolPolicySpec{
					Selector: omniav1alpha1.ToolPolicySelector{
						Registry: "test-registry",
						Tools:    []string{"tool_a", "tool_b"},
					},
					Rules: []omniav1alpha1.PolicyRule{
						{
							Name: "rule-1",
							Deny: omniav1alpha1.PolicyRuleDeny{
								CEL:     "int(headers['X-Omnia-Param-Amount']) > 1000",
								Message: "Amount limit",
							},
						},
						{
							Name: "rule-2",
							Deny: omniav1alpha1.PolicyRuleDeny{
								CEL:     "headers['X-Omnia-Agent-Name'] == 'restricted'",
								Message: "Agent restricted",
							},
						},
						{
							Name: "rule-3",
							Deny: omniav1alpha1.PolicyRuleDeny{
								CEL:     "!('admin' in headers['X-Omnia-User-Roles'].split(','))",
								Message: "Admin required",
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

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      policyName,
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			updated := &omniav1alpha1.ToolPolicy{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      policyName,
					Namespace: "default",
				}, updated)).To(Succeed())
				g.Expect(updated.Status.Phase).To(Equal(omniav1alpha1.ToolPolicyPhaseActive))
				g.Expect(updated.Status.RuleCount).To(Equal(int32(3)))
			}, timeout, interval).Should(Succeed())
		})
	})
})
