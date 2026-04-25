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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/metrics"
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
				policy.Finalizers = nil
				_ = k8sClient.Update(ctx, policy)
				_ = k8sClient.Delete(ctx, policy)
			}
		})

		It("should successfully reconcile a minimal policy", func() {
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyKey.Name,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					WarmStore: &omniav1alpha1.WarmStoreConfig{
						RetentionDays: 7,
						PartitionBy:   omniav1alpha1.PartitionStrategyWeek,
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
					HotCache: &omniav1alpha1.HotCacheConfig{
						TTLAfterInactive: "invalid-duration",
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
					ColdArchive: &omniav1alpha1.ColdArchiveConfig{
						Enabled: true,
						// No RetentionDays — CEL validation rejects this
					},
				},
			}
			err := k8sClient.Create(ctx, policy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("retentionDays is required"))
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

		It("should set NotApplicable reason on WorkspacesResolved (binding moved to Workspace.policyRef)", func() {
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyKey.Name,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					WarmStore: &omniav1alpha1.WarmStoreConfig{
						RetentionDays: 14,
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
			Expect(wsCond.Reason).To(Equal("NotApplicable"))
		})

		It("should add finalizer on first reconcile", func() {
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyKey.Name,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					WarmStore: &omniav1alpha1.WarmStoreConfig{
						RetentionDays: 7,
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: policyKey})
			Expect(err).NotTo(HaveOccurred())

			var updated omniav1alpha1.SessionRetentionPolicy
			Expect(k8sClient.Get(ctx, policyKey, &updated)).To(Succeed())
			Expect(updated.Finalizers).To(ContainElement(retentionPolicyFinalizer))
		})

		It("should set Ready condition True when all checks pass", func() {
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyKey.Name,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					WarmStore: &omniav1alpha1.WarmStoreConfig{
						RetentionDays: 7,
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: policyKey})
			Expect(err).NotTo(HaveOccurred())

			var updated omniav1alpha1.SessionRetentionPolicy
			Expect(k8sClient.Get(ctx, policyKey, &updated)).To(Succeed())

			var readyCond *metav1.Condition
			for i := range updated.Status.Conditions {
				if updated.Status.Conditions[i].Type == RetentionConditionTypeReady {
					readyCond = &updated.Status.Conditions[i]
					break
				}
			}
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("AllChecksPass"))
		})

		It("should set Ready condition False on validation error", func() {
			// Use fake client to bypass CRD validation
			scheme := k8sClient.Scheme()
			zeroDays := int32(0)
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:       policyKey.Name,
					Finalizers: []string{retentionPolicyFinalizer},
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					ColdArchive: &omniav1alpha1.ColdArchiveConfig{
						Enabled:       true,
						RetentionDays: &zeroDays,
					},
				},
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(policy).
				WithStatusSubresource(policy).
				Build()

			fakeReconciler := &SessionRetentionPolicyReconciler{
				Client: fakeClient,
				Scheme: scheme,
			}

			_, err := fakeReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: policyKey,
			})
			Expect(err).To(HaveOccurred())

			var updated omniav1alpha1.SessionRetentionPolicy
			Expect(fakeClient.Get(ctx, policyKey, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.SessionRetentionPolicyPhaseError))

			var readyCond *metav1.Condition
			for i := range updated.Status.Conditions {
				if updated.Status.Conditions[i].Type == RetentionConditionTypeReady {
					readyCond = &updated.Status.Conditions[i]
					break
				}
			}
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ValidationFailed"))
		})

		It("should emit events on validation success", func() {
			fakeRecorder := record.NewFakeRecorder(10)
			reconciler.Recorder = fakeRecorder

			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyKey.Name,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					WarmStore: &omniav1alpha1.WarmStoreConfig{
						RetentionDays: 7,
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: policyKey})
			Expect(err).NotTo(HaveOccurred())

			// Drain events and check for PolicyValidated
			var events []string
			for {
				select {
				case event := <-fakeRecorder.Events:
					events = append(events, event)
				default:
					goto done
				}
			}
		done:
			Expect(events).To(ContainElement(ContainSubstring(RetentionEventReasonValidated)))
			Expect(events).To(ContainElement(ContainSubstring(RetentionEventReasonActive)))
		})

		It("should emit warning event on validation failure", func() {
			fakeRecorder := record.NewFakeRecorder(10)
			scheme := k8sClient.Scheme()
			zeroDays := int32(0)
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:       policyKey.Name,
					Finalizers: []string{retentionPolicyFinalizer},
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					ColdArchive: &omniav1alpha1.ColdArchiveConfig{
						Enabled:       true,
						RetentionDays: &zeroDays,
					},
				},
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(policy).
				WithStatusSubresource(policy).
				Build()

			fakeReconciler := &SessionRetentionPolicyReconciler{
				Client:   fakeClient,
				Scheme:   scheme,
				Recorder: fakeRecorder,
			}

			_, err := fakeReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: policyKey})
			Expect(err).To(HaveOccurred())

			var events []string
			for {
				select {
				case event := <-fakeRecorder.Events:
					events = append(events, event)
				default:
					goto done
				}
			}
		done:
			Expect(events).To(ContainElement(ContainSubstring(RetentionEventReasonValidationFailed)))
		})
	})

	Context("ConfigMap projection", func() {
		var (
			ctx        context.Context
			policyKey  types.NamespacedName
			reconciler *SessionRetentionPolicyReconciler
			testID     string
			testNS     string
		)

		BeforeEach(func() {
			ctx = context.Background()
			testID = fmt.Sprintf("%d", atomic.AddUint64(&retentionTestCounter, 1))
			testNS = "default" // envtest always has "default" namespace
			policyKey = types.NamespacedName{
				Name: "test-cm-" + testID,
			}
			reconciler = &SessionRetentionPolicyReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Namespace: testNS,
			}
		})

		AfterEach(func() {
			policy := &omniav1alpha1.SessionRetentionPolicy{}
			if err := k8sClient.Get(ctx, policyKey, policy); err == nil {
				policy.Finalizers = nil
				_ = k8sClient.Update(ctx, policy)
				_ = k8sClient.Delete(ctx, policy)
			}
			// Clean up ConfigMap
			cm := &corev1.ConfigMap{}
			cmKey := types.NamespacedName{Name: retentionConfigMapName(policyKey.Name), Namespace: testNS}
			if err := k8sClient.Get(ctx, cmKey, cm); err == nil {
				_ = k8sClient.Delete(ctx, cm)
			}
		})

		It("should create ConfigMap on reconcile", func() {
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyKey.Name,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					WarmStore: &omniav1alpha1.WarmStoreConfig{
						RetentionDays: 14,
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: policyKey})
			Expect(err).NotTo(HaveOccurred())

			// Verify ConfigMap was created
			var cm corev1.ConfigMap
			cmKey := types.NamespacedName{
				Name:      retentionConfigMapName(policyKey.Name),
				Namespace: testNS,
			}
			Expect(k8sClient.Get(ctx, cmKey, &cm)).To(Succeed())
			Expect(cm.Labels[labelAppManagedBy]).To(Equal(labelValueOmniaOperator))
			Expect(cm.Labels[labelOmniaComp]).To(Equal("retention-config"))
			Expect(cm.Data).To(HaveKey("retention.yaml"))
			Expect(cm.Data["retention.yaml"]).To(ContainSubstring("retentionDays: 14"))
		})

		It("should update ConfigMap on policy change", func() {
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyKey.Name,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					WarmStore: &omniav1alpha1.WarmStoreConfig{
						RetentionDays: 7,
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: policyKey})
			Expect(err).NotTo(HaveOccurred())

			// Update policy
			var existing omniav1alpha1.SessionRetentionPolicy
			Expect(k8sClient.Get(ctx, policyKey, &existing)).To(Succeed())
			existing.Spec.WarmStore.RetentionDays = 30
			Expect(k8sClient.Update(ctx, &existing)).To(Succeed())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: policyKey})
			Expect(err).NotTo(HaveOccurred())

			// Verify ConfigMap reflects new value
			var cm corev1.ConfigMap
			cmKey := types.NamespacedName{
				Name:      retentionConfigMapName(policyKey.Name),
				Namespace: testNS,
			}
			Expect(k8sClient.Get(ctx, cmKey, &cm)).To(Succeed())
			Expect(cm.Data["retention.yaml"]).To(ContainSubstring("retentionDays: 30"))
		})

		It("should delete ConfigMap on policy deletion via finalizer", func() {
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyKey.Name,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					WarmStore: &omniav1alpha1.WarmStoreConfig{
						RetentionDays: 7,
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			// First reconcile — adds finalizer and creates ConfigMap
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: policyKey})
			Expect(err).NotTo(HaveOccurred())

			// Verify ConfigMap exists
			cmKey := types.NamespacedName{
				Name:      retentionConfigMapName(policyKey.Name),
				Namespace: testNS,
			}
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, cmKey, &cm)).To(Succeed())

			// Delete policy (finalizer will block actual deletion)
			Expect(k8sClient.Delete(ctx, policy)).To(Succeed())

			// Reconcile again — should handle deletion
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: policyKey})
			Expect(err).NotTo(HaveOccurred())

			// Verify ConfigMap is gone
			err = k8sClient.Get(ctx, cmKey, &cm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("should not create ConfigMap when Namespace is empty", func() {
			noNSReconciler := &SessionRetentionPolicyReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Namespace: "",
			}

			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: policyKey.Name,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					WarmStore: &omniav1alpha1.WarmStoreConfig{
						RetentionDays: 7,
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			_, err := noNSReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: policyKey})
			Expect(err).NotTo(HaveOccurred())

			// ConfigMap should not exist
			cmKey := types.NamespacedName{
				Name:      retentionConfigMapName(policyKey.Name),
				Namespace: "default",
			}
			var cm corev1.ConfigMap
			err = k8sClient.Get(ctx, cmKey, &cm)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Metrics recording", func() {
		// Create metrics once per Context to avoid duplicate registration with promauto
		var testMetrics *metrics.RetentionMetrics

		BeforeEach(func() {
			if testMetrics == nil {
				testMetrics = metrics.NewRetentionMetrics()
				testMetrics.Initialize()
			}
		})

		It("should record metrics on successful reconcile", func() {
			ctx := context.Background()
			testID := fmt.Sprintf("%d", atomic.AddUint64(&retentionTestCounter, 1))
			policyName := "metrics-ok-" + testID

			fakeRecorder := record.NewFakeRecorder(10)

			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:       policyName,
					Finalizers: []string{retentionPolicyFinalizer},
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					WarmStore: &omniav1alpha1.WarmStoreConfig{
						RetentionDays: 7,
					},
				},
			}

			scheme := k8sClient.Scheme()
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(policy).
				WithStatusSubresource(policy).
				Build()

			reconciler := &SessionRetentionPolicyReconciler{
				Client:   fakeClient,
				Scheme:   scheme,
				Recorder: fakeRecorder,
				Metrics:  testMetrics,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: policyName},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should record error metrics on validation failure", func() {
			ctx := context.Background()
			testID := fmt.Sprintf("%d", atomic.AddUint64(&retentionTestCounter, 1))
			policyName := "metrics-err-" + testID

			zeroDays := int32(0)
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:       policyName,
					Finalizers: []string{retentionPolicyFinalizer},
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					ColdArchive: &omniav1alpha1.ColdArchiveConfig{
						Enabled:       true,
						RetentionDays: &zeroDays,
					},
				},
			}

			scheme := k8sClient.Scheme()
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(policy).
				WithStatusSubresource(policy).
				Build()

			reconciler := &SessionRetentionPolicyReconciler{
				Client:  fakeClient,
				Scheme:  scheme,
				Metrics: testMetrics,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: policyName},
			})
			Expect(err).To(HaveOccurred())
			// Metrics are nil-safe; just verify no panic
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
					ColdArchive: &omniav1alpha1.ColdArchiveConfig{
						Enabled:       true,
						RetentionDays: &coldDays,
					},
				},
			}
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			// Use a fake client with the object but patch retentionDays to nil to trigger validation error
			scheme := k8sClient.Scheme()
			zeroDays := int32(0)
			invalidPolicy := policy.DeepCopy()
			invalidPolicy.Spec.ColdArchive.RetentionDays = &zeroDays
			invalidPolicy.Finalizers = []string{retentionPolicyFinalizer}

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
			realPolicy := &omniav1alpha1.SessionRetentionPolicy{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: policyName}, realPolicy); err == nil {
				realPolicy.Finalizers = nil
				_ = k8sClient.Update(ctx, realPolicy)
				_ = k8sClient.Delete(ctx, realPolicy)
			}
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
					Name:       policyName,
					Finalizers: []string{retentionPolicyFinalizer},
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					WarmStore: &omniav1alpha1.WarmStoreConfig{
						RetentionDays: 7,
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

		It("should remove finalizer and clean up ConfigMap on deletion", func() {
			ctx := context.Background()
			testID := fmt.Sprintf("%d", atomic.AddUint64(&retentionTestCounter, 1))
			policyName := "delete-" + testID

			scheme := k8sClient.Scheme()
			now := metav1.Now()
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:              policyName,
					Finalizers:        []string{retentionPolicyFinalizer},
					DeletionTimestamp: &now,
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					WarmStore: &omniav1alpha1.WarmStoreConfig{RetentionDays: 7},
				},
			}
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      retentionConfigMapName(policyName),
					Namespace: "test-ns",
				},
			}
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(policy, cm).
				Build()
			reconciler := &SessionRetentionPolicyReconciler{
				Client:    c,
				Scheme:    scheme,
				Recorder:  record.NewFakeRecorder(16),
				Namespace: "test-ns",
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: policyName},
			})
			Expect(err).NotTo(HaveOccurred())

			// ConfigMap should be deleted.
			err = c.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: "test-ns"}, &corev1.ConfigMap{})
			Expect(err).To(HaveOccurred())
		})

		It("should handle Get error other than NotFound", func() {
			ctx := context.Background()
			scheme := k8sClient.Scheme()
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						return fmt.Errorf("simulated get error")
					},
				}).
				Build()
			reconciler := &SessionRetentionPolicyReconciler{Client: c, Scheme: scheme}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "any"},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("simulated get error"))
		})

		It("should propagate finalizer Update errors", func() {
			ctx := context.Background()
			scheme := k8sClient.Scheme()
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "no-finalizer"},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					WarmStore: &omniav1alpha1.WarmStoreConfig{RetentionDays: 7},
				},
			}
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(policy).
				WithInterceptorFuncs(interceptor.Funcs{
					Update: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
						return fmt.Errorf("simulated finalizer update error")
					},
				}).
				Build()
			reconciler := &SessionRetentionPolicyReconciler{Client: c, Scheme: scheme}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "no-finalizer"},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("simulated finalizer update error"))
		})

		It("should propagate ConfigMap sync errors when Namespace is set", func() {
			ctx := context.Background()
			scheme := k8sClient.Scheme()
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "cm-sync-fail",
					Finalizers: []string{retentionPolicyFinalizer},
				},
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					WarmStore: &omniav1alpha1.WarmStoreConfig{RetentionDays: 7},
				},
			}
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(policy).
				WithStatusSubresource(policy).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						if _, ok := obj.(*corev1.ConfigMap); ok {
							return fmt.Errorf("simulated ConfigMap create error")
						}
						return cl.Create(ctx, obj, opts...)
					},
				}).
				Build()
			reconciler := &SessionRetentionPolicyReconciler{
				Client:    c,
				Scheme:    scheme,
				Recorder:  record.NewFakeRecorder(16),
				Namespace: "test-ns",
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "cm-sync-fail"},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("simulated ConfigMap create error"))

			var updated omniav1alpha1.SessionRetentionPolicy
			Expect(c.Get(ctx, types.NamespacedName{Name: "cm-sync-fail"}, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.SessionRetentionPolicyPhaseError))
		})
	})

	Context("deleteRetentionConfigMap direct", func() {
		It("should be a no-op when Namespace is empty", func() {
			reconciler := &SessionRetentionPolicyReconciler{}
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "noop"},
			}
			Expect(reconciler.deleteRetentionConfigMap(context.Background(), policy)).To(Succeed())
		})

		It("should swallow NotFound when ConfigMap does not exist", func() {
			scheme := k8sClient.Scheme()
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler := &SessionRetentionPolicyReconciler{
				Client: c, Scheme: scheme, Namespace: "test-ns",
			}
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "missing-cm"},
			}
			Expect(reconciler.deleteRetentionConfigMap(context.Background(), policy)).To(Succeed())
		})

		It("should propagate non-NotFound delete errors", func() {
			scheme := k8sClient.Scheme()
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Delete: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						return fmt.Errorf("simulated delete error")
					},
				}).
				Build()
			reconciler := &SessionRetentionPolicyReconciler{
				Client: c, Scheme: scheme, Namespace: "test-ns",
			}
			policy := &omniav1alpha1.SessionRetentionPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "broken-cm"},
			}
			err := reconciler.deleteRetentionConfigMap(context.Background(), policy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("simulated delete error"))
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
			}
			Expect(reconciler.validatePolicy(policy)).To(Succeed())
		})

		It("should reject invalid TTL duration", func() {
			reconciler := &SessionRetentionPolicyReconciler{}
			policy := &omniav1alpha1.SessionRetentionPolicy{
				Spec: omniav1alpha1.SessionRetentionPolicySpec{
					HotCache: &omniav1alpha1.HotCacheConfig{
						TTLAfterInactive: "not-a-duration",
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
					ColdArchive: &omniav1alpha1.ColdArchiveConfig{
						Enabled: true,
					},
				},
			}
			err := reconciler.validatePolicy(policy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("retentionDays is required"))
		})

	})
})
