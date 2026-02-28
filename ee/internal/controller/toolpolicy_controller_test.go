/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

var _ = Describe("ToolPolicy Controller", func() {
	const testNamespace = "default"

	Context("When reconciling a ToolPolicy", func() {
		It("should set Active phase for valid CEL expressions", func() {
			tp := &omniav1alpha1.ToolPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-policy",
					Namespace: testNamespace,
				},
				Spec: omniav1alpha1.ToolPolicySpec{
					Selector: omniav1alpha1.ToolPolicySelector{
						Registry: "tools",
						Tools:    []string{"refund"},
					},
					Rules: []omniav1alpha1.PolicyRule{
						{
							Name: "amount-limit",
							Deny: omniav1alpha1.CELDenyRule{
								CEL:     "int(headers['X-Amount']) > 1000",
								Message: "Amount exceeds $1000",
							},
						},
					},
					Mode:      omniav1alpha1.PolicyModeEnforce,
					OnFailure: omniav1alpha1.OnFailureActionDeny,
				},
			}

			Expect(k8sClient.Create(ctx, tp)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, tp)
			})

			reconciler := &ToolPolicyReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "valid-policy",
					Namespace: testNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &omniav1alpha1.ToolPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "valid-policy",
				Namespace: testNamespace,
			}, updated)).To(Succeed())

			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.ToolPolicyPhaseActive))
			Expect(updated.Status.RuleCount).To(Equal(int32(1)))
		})

		It("should set Error phase for invalid CEL expressions", func() {
			tp := &omniav1alpha1.ToolPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-policy",
					Namespace: testNamespace,
				},
				Spec: omniav1alpha1.ToolPolicySpec{
					Selector: omniav1alpha1.ToolPolicySelector{
						Registry: "tools",
					},
					Rules: []omniav1alpha1.PolicyRule{
						{
							Name: "bad-rule",
							Deny: omniav1alpha1.CELDenyRule{
								CEL:     "!@#invalid syntax",
								Message: "should not compile",
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, tp)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, tp)
			})

			reconciler := &ToolPolicyReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "invalid-policy",
					Namespace: testNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &omniav1alpha1.ToolPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "invalid-policy",
				Namespace: testNamespace,
			}, updated)).To(Succeed())

			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.ToolPolicyPhaseError))
			Expect(updated.Status.RuleCount).To(Equal(int32(0)))
		})

		It("should handle not found gracefully", func() {
			reconciler := &ToolPolicyReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "nonexistent",
					Namespace: testNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should set Error for mixed valid and invalid rules", func() {
			tp := &omniav1alpha1.ToolPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mixed-policy",
					Namespace: testNamespace,
				},
				Spec: omniav1alpha1.ToolPolicySpec{
					Selector: omniav1alpha1.ToolPolicySelector{
						Registry: "tools",
					},
					Rules: []omniav1alpha1.PolicyRule{
						{
							Name: "good-rule",
							Deny: omniav1alpha1.CELDenyRule{
								CEL:     "headers['X-Test'] == 'val'",
								Message: "valid",
							},
						},
						{
							Name: "bad-rule",
							Deny: omniav1alpha1.CELDenyRule{
								CEL:     "invalid @@@ syntax",
								Message: "invalid",
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, tp)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, tp)
			})

			reconciler := &ToolPolicyReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "mixed-policy",
					Namespace: testNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &omniav1alpha1.ToolPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "mixed-policy",
				Namespace: testNamespace,
			}, updated)).To(Succeed())

			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.ToolPolicyPhaseError))
		})
	})

	Context("When testing setCondition", func() {
		It("should add a new condition", func() {
			conditions := []metav1.Condition{}
			setCondition(&conditions, metav1.Condition{
				Type:   "Ready",
				Status: metav1.ConditionTrue,
			})
			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Type).To(Equal("Ready"))
		})

		It("should update an existing condition", func() {
			conditions := []metav1.Condition{
				{
					Type:   "Ready",
					Status: metav1.ConditionFalse,
					Reason: "OldReason",
				},
			}
			setCondition(&conditions, metav1.Condition{
				Type:   "Ready",
				Status: metav1.ConditionTrue,
				Reason: "NewReason",
			})
			Expect(conditions).To(HaveLen(1))
			Expect(conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(conditions[0].Reason).To(Equal("NewReason"))
		})
	})

	Context("When testing SetupWithManager", func() {
		It("should fail with nil manager", func() {
			reconciler := &ToolPolicyReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}
			err := reconciler.SetupWithManager(nil)
			Expect(err).To(HaveOccurred())
		})
	})
})

// Ensure context is available (from suite_test.go).
var _ context.Context = ctx
