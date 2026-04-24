/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"context"
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

var memRetentionTestCounter uint64

var _ = Describe("MemoryPolicy Controller", func() {
	var (
		rctx       context.Context
		policyKey  types.NamespacedName
		reconciler *MemoryPolicyReconciler
		testID     string
	)

	BeforeEach(func() {
		rctx = context.Background()
		testID = fmt.Sprintf("%d", atomic.AddUint64(&memRetentionTestCounter, 1))
		policyKey = types.NamespacedName{Name: "test-mem-retention-" + testID}
		reconciler = &MemoryPolicyReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: record.NewFakeRecorder(16),
		}
	})

	AfterEach(func() {
		policy := &omniav1alpha1.MemoryPolicy{}
		if err := k8sClient.Get(rctx, policyKey, policy); err == nil {
			_ = k8sClient.Delete(rctx, policy)
		}
	})

	It("reconciles a minimal policy to Active", func() {
		policy := &omniav1alpha1.MemoryPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: policyKey.Name},
			Spec: omniav1alpha1.MemoryPolicySpec{
				Default: omniav1alpha1.MemoryRetentionDefaults{
					Tiers: omniav1alpha1.MemoryRetentionTierSet{
						Institutional: &omniav1alpha1.MemoryTierConfig{
							Mode: omniav1alpha1.MemoryRetentionModeManual,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(rctx, policy)).To(Succeed())

		_, err := reconciler.Reconcile(rctx, reconcile.Request{NamespacedName: policyKey})
		Expect(err).NotTo(HaveOccurred())

		var updated omniav1alpha1.MemoryPolicy
		Expect(k8sClient.Get(rctx, policyKey, &updated)).To(Succeed())
		Expect(updated.Status.Phase).To(Equal(omniav1alpha1.MemoryPolicyPhaseActive))
		Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))
		Expect(updated.Status.WorkspaceCount).To(Equal(int32(0)))

		valid := findMemCondition(updated.Status.Conditions, MemRetentionConditionTypePolicyValid)
		Expect(valid).NotTo(BeNil())
		Expect(valid.Status).To(Equal(metav1.ConditionTrue))

		ready := findMemCondition(updated.Status.Conditions, MemRetentionConditionTypeReady)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))

		wsResolved := findMemCondition(updated.Status.Conditions, MemRetentionConditionTypeWorkspacesResolved)
		Expect(wsResolved).NotTo(BeNil())
		Expect(wsResolved.Reason).To(Equal("NoOverrides"))
	})

	It("sets Error phase on invalid schedule", func() {
		policy := &omniav1alpha1.MemoryPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: policyKey.Name},
			Spec: omniav1alpha1.MemoryPolicySpec{
				Default: omniav1alpha1.MemoryRetentionDefaults{
					Tiers: omniav1alpha1.MemoryRetentionTierSet{
						Institutional: &omniav1alpha1.MemoryTierConfig{
							Mode: omniav1alpha1.MemoryRetentionModeManual,
						},
					},
					Schedule: "not a cron",
				},
			},
		}
		Expect(k8sClient.Create(rctx, policy)).To(Succeed())

		_, err := reconciler.Reconcile(rctx, reconcile.Request{NamespacedName: policyKey})
		Expect(err).To(HaveOccurred())

		var updated omniav1alpha1.MemoryPolicy
		Expect(k8sClient.Get(rctx, policyKey, &updated)).To(Succeed())
		Expect(updated.Status.Phase).To(Equal(omniav1alpha1.MemoryPolicyPhaseError))

		valid := findMemCondition(updated.Status.Conditions, MemRetentionConditionTypePolicyValid)
		Expect(valid).NotTo(BeNil())
		Expect(valid.Status).To(Equal(metav1.ConditionFalse))
		Expect(valid.Reason).To(Equal("ValidationFailed"))
	})

	It("flags missing per-workspace references", func() {
		policy := &omniav1alpha1.MemoryPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: policyKey.Name},
			Spec: omniav1alpha1.MemoryPolicySpec{
				Default: omniav1alpha1.MemoryRetentionDefaults{
					Tiers: omniav1alpha1.MemoryRetentionTierSet{
						Institutional: &omniav1alpha1.MemoryTierConfig{
							Mode: omniav1alpha1.MemoryRetentionModeManual,
						},
					},
				},
				PerWorkspace: map[string]omniav1alpha1.MemoryWorkspaceRetentionOverride{
					"does-not-exist-" + testID: {},
				},
			},
		}
		Expect(k8sClient.Create(rctx, policy)).To(Succeed())

		_, err := reconciler.Reconcile(rctx, reconcile.Request{NamespacedName: policyKey})
		Expect(err).To(HaveOccurred())

		var updated omniav1alpha1.MemoryPolicy
		Expect(k8sClient.Get(rctx, policyKey, &updated)).To(Succeed())
		Expect(updated.Status.Phase).To(Equal(omniav1alpha1.MemoryPolicyPhaseError))
		Expect(updated.Status.WorkspaceCount).To(Equal(int32(0)))

		wsResolved := findMemCondition(updated.Status.Conditions, MemRetentionConditionTypeWorkspacesResolved)
		Expect(wsResolved).NotTo(BeNil())
		Expect(wsResolved.Status).To(Equal(metav1.ConditionFalse))
		Expect(wsResolved.Reason).To(Equal("ResolutionFailed"))
	})

	It("reconciles Active when referenced workspace exists", func() {
		wsName := "mrp-ws-" + testID
		ws := &omniav1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: wsName},
			Spec: omniav1alpha1.WorkspaceSpec{
				DisplayName: "test workspace " + testID,
				Namespace:   omniav1alpha1.NamespaceConfig{Name: wsName},
			},
		}
		Expect(k8sClient.Create(rctx, ws)).To(Succeed())
		defer func() {
			_ = k8sClient.Delete(rctx, ws)
		}()

		policy := &omniav1alpha1.MemoryPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: policyKey.Name},
			Spec: omniav1alpha1.MemoryPolicySpec{
				Default: omniav1alpha1.MemoryRetentionDefaults{
					Tiers: omniav1alpha1.MemoryRetentionTierSet{
						Institutional: &omniav1alpha1.MemoryTierConfig{
							Mode: omniav1alpha1.MemoryRetentionModeManual,
						},
					},
				},
				PerWorkspace: map[string]omniav1alpha1.MemoryWorkspaceRetentionOverride{
					wsName: {},
				},
			},
		}
		Expect(k8sClient.Create(rctx, policy)).To(Succeed())

		_, err := reconciler.Reconcile(rctx, reconcile.Request{NamespacedName: policyKey})
		Expect(err).NotTo(HaveOccurred())

		var updated omniav1alpha1.MemoryPolicy
		Expect(k8sClient.Get(rctx, policyKey, &updated)).To(Succeed())
		Expect(updated.Status.Phase).To(Equal(omniav1alpha1.MemoryPolicyPhaseActive))
		Expect(updated.Status.WorkspaceCount).To(Equal(int32(1)))

		wsResolved := findMemCondition(updated.Status.Conditions, MemRetentionConditionTypeWorkspacesResolved)
		Expect(wsResolved).NotTo(BeNil())
		Expect(wsResolved.Status).To(Equal(metav1.ConditionTrue))
		Expect(wsResolved.Reason).To(Equal("AllResolved"))
	})

	It("returns empty result when the policy is deleted", func() {
		result, err := reconciler.Reconcile(rctx,
			reconcile.Request{NamespacedName: types.NamespacedName{Name: "missing-" + testID}})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))
	})

	It("skips reconciliation when DeletionTimestamp is set", func() {
		// Create a policy, add finalizer, then delete — Reconcile should
		// early-return with no status update.
		policy := &omniav1alpha1.MemoryPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:       policyKey.Name,
				Finalizers: []string{"test-hold"},
			},
			Spec: omniav1alpha1.MemoryPolicySpec{
				Default: omniav1alpha1.MemoryRetentionDefaults{
					Tiers: omniav1alpha1.MemoryRetentionTierSet{
						Institutional: &omniav1alpha1.MemoryTierConfig{
							Mode: omniav1alpha1.MemoryRetentionModeManual,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(rctx, policy)).To(Succeed())
		Expect(k8sClient.Delete(rctx, policy)).To(Succeed())

		result, err := reconciler.Reconcile(rctx, reconcile.Request{NamespacedName: policyKey})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))

		// Cleanup: drop finalizer so AfterEach can delete.
		var held omniav1alpha1.MemoryPolicy
		Expect(k8sClient.Get(rctx, policyKey, &held)).To(Succeed())
		held.Finalizers = nil
		Expect(k8sClient.Update(rctx, &held)).To(Succeed())
	})

	It("returns only policies that reference the given workspace", func() {
		wsName := "mrp-ws-match-" + testID
		otherWs := "mrp-ws-other-" + testID

		Expect(k8sClient.Create(rctx, &omniav1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: wsName},
			Spec: omniav1alpha1.WorkspaceSpec{
				DisplayName: wsName,
				Namespace:   omniav1alpha1.NamespaceConfig{Name: wsName},
			},
		})).To(Succeed())
		Expect(k8sClient.Create(rctx, &omniav1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: otherWs},
			Spec: omniav1alpha1.WorkspaceSpec{
				DisplayName: otherWs,
				Namespace:   omniav1alpha1.NamespaceConfig{Name: otherWs},
			},
		})).To(Succeed())

		matching := &omniav1alpha1.MemoryPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "mrp-match-" + testID},
			Spec: omniav1alpha1.MemoryPolicySpec{
				Default: omniav1alpha1.MemoryRetentionDefaults{
					Tiers: omniav1alpha1.MemoryRetentionTierSet{
						Institutional: &omniav1alpha1.MemoryTierConfig{
							Mode: omniav1alpha1.MemoryRetentionModeManual,
						},
					},
				},
				PerWorkspace: map[string]omniav1alpha1.MemoryWorkspaceRetentionOverride{
					wsName: {},
				},
			},
		}
		unrelated := &omniav1alpha1.MemoryPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "mrp-unrelated-" + testID},
			Spec: omniav1alpha1.MemoryPolicySpec{
				Default: omniav1alpha1.MemoryRetentionDefaults{
					Tiers: omniav1alpha1.MemoryRetentionTierSet{
						Institutional: &omniav1alpha1.MemoryTierConfig{
							Mode: omniav1alpha1.MemoryRetentionModeManual,
						},
					},
				},
				PerWorkspace: map[string]omniav1alpha1.MemoryWorkspaceRetentionOverride{
					otherWs: {},
				},
			},
		}
		Expect(k8sClient.Create(rctx, matching)).To(Succeed())
		Expect(k8sClient.Create(rctx, unrelated)).To(Succeed())
		defer func() {
			_ = k8sClient.Delete(rctx, matching)
			_ = k8sClient.Delete(rctx, unrelated)
			_ = k8sClient.Delete(rctx, &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: wsName},
			})
			_ = k8sClient.Delete(rctx, &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: otherWs},
			})
		}()

		requests := reconciler.findPoliciesForWorkspace(rctx, &omniav1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: wsName},
		})
		Expect(requests).To(HaveLen(1))
		Expect(requests[0].NamespacedName.Name).To(Equal(matching.Name))
	})

	It("returns nil from findPoliciesForWorkspace when called with a non-Workspace", func() {
		requests := reconciler.findPoliciesForWorkspace(rctx, &omniav1alpha1.MemoryPolicy{})
		Expect(requests).To(BeNil())
	})

	It("emitEvent is a no-op when Recorder is nil", func() {
		nilRec := &MemoryPolicyReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
		// Should not panic.
		nilRec.emitEvent(&omniav1alpha1.MemoryPolicy{},
			"Normal", "TestReason", "test")
	})
})

func findMemCondition(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
}
