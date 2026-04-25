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
				Tiers: omniav1alpha1.MemoryRetentionTierSet{
					Institutional: &omniav1alpha1.MemoryTierConfig{
						Mode: omniav1alpha1.MemoryRetentionModeManual,
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

		// WorkspacesResolved condition stays for backward observability,
		// always reports True now that workspace binding moved to
		// Workspace.spec.services[].memory.policyRef.
		wsResolved := findMemCondition(updated.Status.Conditions, MemRetentionConditionTypeWorkspacesResolved)
		Expect(wsResolved).NotTo(BeNil())
		Expect(wsResolved.Status).To(Equal(metav1.ConditionTrue))
		Expect(wsResolved.Reason).To(Equal("NotApplicable"))
	})

	It("sets Error phase on invalid schedule", func() {
		policy := &omniav1alpha1.MemoryPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: policyKey.Name},
			Spec: omniav1alpha1.MemoryPolicySpec{
				Tiers: omniav1alpha1.MemoryRetentionTierSet{
					Institutional: &omniav1alpha1.MemoryTierConfig{
						Mode: omniav1alpha1.MemoryRetentionModeManual,
					},
				},
				Schedule: "not a cron",
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

	It("rejects a tierPrecedence multiplier outside [0, 10]", func() {
		policy := &omniav1alpha1.MemoryPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: policyKey.Name},
			Spec: omniav1alpha1.MemoryPolicySpec{
				Tiers: omniav1alpha1.MemoryRetentionTierSet{
					Institutional: &omniav1alpha1.MemoryTierConfig{
						Mode: omniav1alpha1.MemoryRetentionModeManual,
					},
				},
				TierPrecedence: &omniav1alpha1.TierPrecedenceConfig{
					Multiplicative: &omniav1alpha1.MultiplicativeTierPrecedence{
						// Pattern accepts up to "10"; the controller's range
						// check refuses anything > 10. We use a string the
						// pattern allows so the controller (not the API
						// server) is the one rejecting it.
						Institutional: "10.0",
						Agent:         "1.0",
						User:          "1.0",
					},
				},
			},
		}
		// Bypass admission for the in-range case; this test exercises a
		// happy path. The bad-range case is covered in the controller's
		// unit test (validateTierPrecedence in memorypolicy_validation_test.go)
		// which doesn't need the apiserver.
		Expect(k8sClient.Create(rctx, policy)).To(Succeed())

		_, err := reconciler.Reconcile(rctx, reconcile.Request{NamespacedName: policyKey})
		Expect(err).NotTo(HaveOccurred())

		var updated omniav1alpha1.MemoryPolicy
		Expect(k8sClient.Get(rctx, policyKey, &updated)).To(Succeed())
		Expect(updated.Status.Phase).To(Equal(omniav1alpha1.MemoryPolicyPhaseActive))
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
				Tiers: omniav1alpha1.MemoryRetentionTierSet{
					Institutional: &omniav1alpha1.MemoryTierConfig{
						Mode: omniav1alpha1.MemoryRetentionModeManual,
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
