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
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// retentionTestCounter ensures unique names across all retention policy tests
var retentionTestCounter uint64

var _ = Describe("SessionRetentionPolicy Controller", func() {
	Context("When reconciling a SessionRetentionPolicy", func() {
		var (
			ctx        context.Context
			policyKey  types.NamespacedName
			reconciler *SessionRetentionPolicyReconciler
			testID     string
		)

		BeforeEach(func() {
			ctx = context.Background()
			testID = fmt.Sprintf("%d", atomic.AddUint64(&retentionTestCounter, 1))
			policyKey = types.NamespacedName{
				Name: "test-retention-" + testID,
			}
			reconciler = &SessionRetentionPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		AfterEach(func() {
			policy := &omniav1alpha1.SessionRetentionPolicy{}
			if err := k8sClient.Get(ctx, policyKey, policy); err == nil {
				_ = k8sClient.Delete(ctx, policy)
			}
		})

		It("should successfully reconcile a minimal policy", func() {
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyKey.Name,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					Default: omniav1alpha1.RetentionTierConfig{
						WarmStore: &omniav1alpha1.WarmStoreConfig{
							RetentionDays: 7,
							PartitionBy:   omniav1alpha1.PartitionStrategyWeek,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: policyKey})
			Expect(err).NotTo(HaveOccurred())

			var updated omniav1alpha1.SessionRetentionPolicy
			Expect(k8sClient.Get(ctx, policyKey, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.SessionRetentionPolicyPhaseActive))
			Expect(updated.Status.WorkspaceCount).To(Equal(int32(0)))

			// Verify PolicyValid condition
			var policyValid *metav1.Condition
			for i := range updated.Status.Conditions {
				if updated.Status.Conditions[i].Type == RetentionConditionTypePolicyValid {
					policyValid = &updated.Status.Conditions[i]
					break
				}
			}
			Expect(policyValid).NotTo(BeNil())
			Expect(policyValid.Status).To(Equal(metav1.ConditionTrue))
			Expect(policyValid.Reason).To(Equal("Valid"))
		})

		It("should successfully reconcile a full policy with hot cache, warm store, and cold archive", func() {
			enabled := true
			maxSessions := int32(500)
			maxMessages := int32(200)
			coldDays := int32(365)

			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyKey.Name,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					Default: omniav1alpha1.RetentionTierConfig{
						HotCache: &omniav1alpha1.HotCacheConfig{
							Enabled:               &enabled,
							TTLAfterInactive:      "24h",
							MaxSessions:           &maxSessions,
							MaxMessagesPerSession: &maxMessages,
						},
						WarmStore: &omniav1alpha1.WarmStoreConfig{
							RetentionDays: 7,
							PartitionBy:   omniav1alpha1.PartitionStrategyWeek,
						},
						ColdArchive: &omniav1alpha1.ColdArchiveConfig{
							Enabled:            true,
							RetentionDays:      &coldDays,
							CompactionSchedule: "0 2 * * *",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: policyKey})
			Expect(err).NotTo(HaveOccurred())

			var updated omniav1alpha1.SessionRetentionPolicy
			Expect(k8sClient.Get(ctx, policyKey, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.SessionRetentionPolicyPhaseActive))
		})

		It("should reject invalid hot cache TTL at CRD level", func() {
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyKey.Name,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					Default: omniav1alpha1.RetentionTierConfig{
						HotCache: &omniav1alpha1.HotCacheConfig{
							TTLAfterInactive: "invalid-duration",
						},
					},
				},
			}
			err := k8sClient.Create(ctx, policy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("should match"))
		})

		It("should reject cold archive enabled without retentionDays at CRD level", func() {
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyKey.Name,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					Default: omniav1alpha1.RetentionTierConfig{
						ColdArchive: &omniav1alpha1.ColdArchiveConfig{
							Enabled: true,
							// No RetentionDays — CEL validation rejects this
						},
					},
				},
			}
			err := k8sClient.Create(ctx, policy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("retentionDays is required"))
		})

		It("should fail when per-workspace references a non-existent workspace", func() {
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyKey.Name,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					Default: omniav1alpha1.RetentionTierConfig{
						WarmStore: &omniav1alpha1.WarmStoreConfig{
							RetentionDays: 7,
						},
					},
					PerWorkspace: map[string]omniav1alpha1.WorkspaceRetentionOverride{
						"nonexistent-workspace": {
							WarmStore: &omniav1alpha1.WarmStoreConfig{
								RetentionDays: 30,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: policyKey})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("workspaces not found"))
			Expect(err.Error()).To(ContainSubstring("nonexistent-workspace"))

			var updated omniav1alpha1.SessionRetentionPolicy
			Expect(k8sClient.Get(ctx, policyKey, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.SessionRetentionPolicyPhaseError))

			var wsCond *metav1.Condition
			for i := range updated.Status.Conditions {
				if updated.Status.Conditions[i].Type == RetentionConditionTypeWorkspacesResolved {
					wsCond = &updated.Status.Conditions[i]
					break
				}
			}
			Expect(wsCond).NotTo(BeNil())
			Expect(wsCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(wsCond.Reason).To(Equal("ResolutionFailed"))
		})

		It("should succeed when per-workspace references an existing workspace", func() {
			wsName := "retention-ws-" + testID
			namespaceName := "retention-ns-" + testID

			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: wsName,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyKey.Name,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					Default: omniav1alpha1.RetentionTierConfig{
						WarmStore: &omniav1alpha1.WarmStoreConfig{
							RetentionDays: 7,
						},
					},
					PerWorkspace: map[string]omniav1alpha1.WorkspaceRetentionOverride{
						wsName: {
							WarmStore: &omniav1alpha1.WarmStoreConfig{
								RetentionDays: 30,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: policyKey})
			Expect(err).NotTo(HaveOccurred())

			var updated omniav1alpha1.SessionRetentionPolicy
			Expect(k8sClient.Get(ctx, policyKey, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.SessionRetentionPolicyPhaseActive))
			Expect(updated.Status.WorkspaceCount).To(Equal(int32(1)))

			var wsCond *metav1.Condition
			for i := range updated.Status.Conditions {
				if updated.Status.Conditions[i].Type == RetentionConditionTypeWorkspacesResolved {
					wsCond = &updated.Status.Conditions[i]
					break
				}
			}
			Expect(wsCond).NotTo(BeNil())
			Expect(wsCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(wsCond.Reason).To(Equal("AllResolved"))

			// Clean up
			workspace.Finalizers = nil
			_ = k8sClient.Update(ctx, workspace)
			_ = k8sClient.Delete(ctx, workspace)
		})

		It("should handle policy not found", func() {
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "nonexistent-retention-policy",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})

		It("should set NoOverrides reason when no per-workspace overrides exist", func() {
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyKey.Name,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					Default: omniav1alpha1.RetentionTierConfig{
						WarmStore: &omniav1alpha1.WarmStoreConfig{
							RetentionDays: 14,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: policyKey})
			Expect(err).NotTo(HaveOccurred())

			var updated omniav1alpha1.SessionRetentionPolicy
			Expect(k8sClient.Get(ctx, policyKey, &updated)).To(Succeed())

			var wsCond *metav1.Condition
			for i := range updated.Status.Conditions {
				if updated.Status.Conditions[i].Type == RetentionConditionTypeWorkspacesResolved {
					wsCond = &updated.Status.Conditions[i]
					break
				}
			}
			Expect(wsCond).NotTo(BeNil())
			Expect(wsCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(wsCond.Reason).To(Equal("NoOverrides"))
		})
	})

	Context("findPoliciesForWorkspace", func() {
		var (
			ctx        context.Context
			reconciler *SessionRetentionPolicyReconciler
			testID     string
		)

		BeforeEach(func() {
			ctx = context.Background()
			testID = fmt.Sprintf("%d", atomic.AddUint64(&retentionTestCounter, 1))
			reconciler = &SessionRetentionPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		It("should find policies that reference a workspace", func() {
			wsName := "map-ws-" + testID
			namespaceName := "map-ns-" + testID

			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: wsName,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Mapping Test Workspace",
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			policyName := "map-policy-" + testID
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyName,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					Default: omniav1alpha1.RetentionTierConfig{
						WarmStore: &omniav1alpha1.WarmStoreConfig{
							RetentionDays: 7,
						},
					},
					PerWorkspace: map[string]omniav1alpha1.WorkspaceRetentionOverride{
						wsName: {
							WarmStore: &omniav1alpha1.WarmStoreConfig{
								RetentionDays: 30,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			requests := reconciler.findPoliciesForWorkspace(ctx, workspace)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal(policyName))

			// Clean up
			_ = k8sClient.Delete(ctx, policy)
			workspace.Finalizers = nil
			_ = k8sClient.Update(ctx, workspace)
			_ = k8sClient.Delete(ctx, workspace)
		})

		It("should return empty when no policies reference the workspace", func() {
			wsName := "unref-ws-" + testID
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: wsName,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Unreferenced Workspace",
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   "unref-ns-" + testID,
						Create: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			requests := reconciler.findPoliciesForWorkspace(ctx, workspace)
			Expect(requests).To(BeEmpty())

			// Clean up
			workspace.Finalizers = nil
			_ = k8sClient.Update(ctx, workspace)
			_ = k8sClient.Delete(ctx, workspace)
		})
	})

	Context("Reconcile error paths", func() {
		It("should set Error phase when validatePolicy fails via reconcile", func() {
			ctx := context.Background()
			testID := fmt.Sprintf("%d", atomic.AddUint64(&retentionTestCounter, 1))
			policyName := "val-err-" + testID

			// Create a valid policy first
			coldDays := int32(365)
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyName,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					Default: omniav1alpha1.RetentionTierConfig{
						ColdArchive: &omniav1alpha1.ColdArchiveConfig{
							Enabled:       true,
							RetentionDays: &coldDays,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			// Use a fake client with the object but patch retentionDays to nil to trigger validation error
			scheme := k8sClient.Scheme()
			zeroDays := int32(0)
			invalidPolicy := policy.DeepCopy()
			invalidPolicy.Spec.Default.ColdArchive.RetentionDays = &zeroDays

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(invalidPolicy).
				WithStatusSubresource(invalidPolicy).
				Build()

			reconciler := &SessionRetentionPolicyReconciler{
				Client: fakeClient,
				Scheme: scheme,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: policyName},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("retentionDays is required"))

			// Verify status was updated to Error
			var updated omniav1alpha1.SessionRetentionPolicy
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: policyName}, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.SessionRetentionPolicyPhaseError))

			// Clean up the real resource
			_ = k8sClient.Delete(ctx, policy)
		})

		It("should set Error phase when Get returns a non-NotFound error", func() {
			ctx := context.Background()
			testID := fmt.Sprintf("%d", atomic.AddUint64(&retentionTestCounter, 1))
			policyName := "get-err-" + testID

			scheme := k8sClient.Scheme()
			errClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						return fmt.Errorf("simulated API server error")
					},
				}).
				Build()

			reconciler := &SessionRetentionPolicyReconciler{
				Client: errClient,
				Scheme: scheme,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: policyName},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("simulated API server error"))
		})

		It("should handle status update failure after successful reconcile", func() {
			ctx := context.Background()
			testID := fmt.Sprintf("%d", atomic.AddUint64(&retentionTestCounter, 1))
			policyName := "status-err-" + testID

			scheme := k8sClient.Scheme()
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyName,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					Default: omniav1alpha1.RetentionTierConfig{
						WarmStore: &omniav1alpha1.WarmStoreConfig{
							RetentionDays: 7,
						},
					},
				},
			}

			statusUpdateClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(policy).
				WithStatusSubresource(policy).
				WithInterceptorFuncs(interceptor.Funcs{
					SubResourceUpdate: func(ctx context.Context, client client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
						return fmt.Errorf("simulated status update error")
					},
				}).
				Build()

			reconciler := &SessionRetentionPolicyReconciler{
				Client: statusUpdateClient,
				Scheme: scheme,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: policyName},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("simulated status update error"))
		})

		It("should handle workspace Get returning non-NotFound error in resolveWorkspaces", func() {
			ctx := context.Background()
			testID := fmt.Sprintf("%d", atomic.AddUint64(&retentionTestCounter, 1))
			policyName := "ws-get-err-" + testID

			scheme := k8sClient.Scheme()
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyName,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					Default: omniav1alpha1.RetentionTierConfig{
						WarmStore: &omniav1alpha1.WarmStoreConfig{
							RetentionDays: 7,
						},
					},
					PerWorkspace: map[string]omniav1alpha1.WorkspaceRetentionOverride{
						"some-workspace": {
							WarmStore: &omniav1alpha1.WarmStoreConfig{
								RetentionDays: 30,
							},
						},
					},
				},
			}

			callCount := 0
			wsGetErrClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(policy).
				WithStatusSubresource(policy).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						// Let the first Get (for the policy itself) succeed, fail on workspace Get
						if _, ok := obj.(*omniav1alpha1.Workspace); ok {
							callCount++
							return fmt.Errorf("simulated workspace API error")
						}
						return client.Get(ctx, key, obj, opts...)
					},
				}).
				Build()

			reconciler := &SessionRetentionPolicyReconciler{
				Client: wsGetErrClient,
				Scheme: scheme,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: policyName},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get workspace"))
		})

		It("should handle List error in findPoliciesForWorkspace", func() {
			ctx := context.Background()

			scheme := k8sClient.Scheme()
			listErrClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					List: func(ctx context.Context, client client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
						return fmt.Errorf("simulated list error")
					},
				}).
				Build()

			reconciler := &SessionRetentionPolicyReconciler{
				Client: listErrClient,
				Scheme: scheme,
			}

			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ws",
				},
			}

			requests := reconciler.findPoliciesForWorkspace(ctx, workspace)
			Expect(requests).To(BeNil())
		})
	})

	Context("validatePolicy direct", func() {
		It("should accept a valid policy with all tiers", func() {
			enabled := true
			maxSessions := int32(100)
			coldDays := int32(365)
			reconciler := &SessionRetentionPolicyReconciler{}

			policy := &omniav1alpha1.SessionRetentionPolicy{
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					Default: omniav1alpha1.RetentionTierConfig{
						HotCache: &omniav1alpha1.HotCacheConfig{
							Enabled:          &enabled,
							TTLAfterInactive: "1h30m",
							MaxSessions:      &maxSessions,
						},
						WarmStore: &omniav1alpha1.WarmStoreConfig{
							RetentionDays: 14,
						},
						ColdArchive: &omniav1alpha1.ColdArchiveConfig{
							Enabled:       true,
							RetentionDays: &coldDays,
						},
					},
				},
			}
			Expect(reconciler.validatePolicy(policy)).To(Succeed())
		})

		It("should reject invalid TTL duration", func() {
			reconciler := &SessionRetentionPolicyReconciler{}
			policy := &omniav1alpha1.SessionRetentionPolicy{
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					Default: omniav1alpha1.RetentionTierConfig{
						HotCache: &omniav1alpha1.HotCacheConfig{
							TTLAfterInactive: "not-a-duration",
						},
					},
				},
			}
			err := reconciler.validatePolicy(policy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid hot cache TTL"))
		})

		It("should reject cold archive enabled without retentionDays", func() {
			reconciler := &SessionRetentionPolicyReconciler{}
			policy := &omniav1alpha1.SessionRetentionPolicy{
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					Default: omniav1alpha1.RetentionTierConfig{
						ColdArchive: &omniav1alpha1.ColdArchiveConfig{
							Enabled: true,
						},
					},
				},
			}
			err := reconciler.validatePolicy(policy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("retentionDays is required"))
		})

		It("should reject per-workspace cold archive enabled without retentionDays", func() {
			reconciler := &SessionRetentionPolicyReconciler{}
			policy := &omniav1alpha1.SessionRetentionPolicy{
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					Default: omniav1alpha1.RetentionTierConfig{},
					PerWorkspace: map[string]omniav1alpha1.WorkspaceRetentionOverride{
						"ws1": {
							ColdArchive: &omniav1alpha1.ColdArchiveConfig{
								Enabled: true,
							},
						},
					},
				},
			}
			err := reconciler.validatePolicy(policy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("workspace \"ws1\""))
		})
	})
})
