/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// arenaDevSessionCounter gives each spec a unique resource suffix.
var arenaDevSessionCounter uint64

var _ = Describe("ArenaDevSession Controller", func() {
	var (
		testCtx   context.Context
		namespace string
		nextName  = func(prefix string) string {
			n := atomic.AddUint64(&arenaDevSessionCounter, 1)
			return fmt.Sprintf("%s-%d", prefix, n)
		}
	)

	BeforeEach(func() {
		testCtx = context.Background()
		namespace = nextName("ads-test")
		Expect(k8sClient.Create(testCtx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		})).To(Succeed())
	})

	AfterEach(func() {
		ns := &corev1.Namespace{}
		if err := k8sClient.Get(testCtx, types.NamespacedName{Name: namespace}, ns); err == nil {
			_ = k8sClient.Delete(testCtx, ns)
		}
	})

	baseSession := func(name string) *omniav1alpha1.ArenaDevSession {
		return &omniav1alpha1.ArenaDevSession{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: omniav1alpha1.ArenaDevSessionSpec{
				ProjectID: "test-project",
				Workspace: "test-workspace",
			},
		}
	}

	reconcileOnce := func(r *ArenaDevSessionReconciler, name string) (reconcile.Result, error) {
		return r.Reconcile(testCtx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
		})
	}

	Context("CRD validation", func() {
		// spec.projectId and spec.workspace carry +kubebuilder:validation:Required
		// but no MinLength, so the API server accepts empty strings. These specs
		// document the happy shape; tighter validation would need new markers on
		// the CRD types.

		It("accepts a well-formed session", func() {
			Expect(k8sClient.Create(testCtx, baseSession(nextName("ads")))).To(Succeed())
		})
	})

	Context("reconcile lifecycle", func() {
		It("adds the finalizer, reaches Starting, and creates child resources", func() {
			name := nextName("ads")
			Expect(k8sClient.Create(testCtx, baseSession(name))).To(Succeed())

			r := &ArenaDevSessionReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			// Reconcile #1 adds finalizer and requeues.
			_, err := reconcileOnce(r, name)
			Expect(err).NotTo(HaveOccurred())

			afterFinalizer := &omniav1alpha1.ArenaDevSession{}
			Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: name, Namespace: namespace}, afterFinalizer)).To(Succeed())
			Expect(afterFinalizer.Finalizers).To(ContainElement(ArenaDevSessionFinalizerName))

			// Reconcile #2: initializes status to Pending, then falls through to
			// the switch which picks Pending and runs reconcileStart, creating
			// children and advancing to Starting in one pass.
			_, err = reconcileOnce(r, name)
			Expect(err).NotTo(HaveOccurred())

			final := &omniav1alpha1.ArenaDevSession{}
			Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: name, Namespace: namespace}, final)).To(Succeed())
			Expect(final.Status.Phase).To(Equal(omniav1alpha1.ArenaDevSessionPhaseStarting))

			// Child resources should exist.
			resourceName := r.resourceName(final)
			key := types.NamespacedName{Name: resourceName, Namespace: namespace}
			Expect(k8sClient.Get(testCtx, key, &corev1.ServiceAccount{})).To(Succeed())
			Expect(k8sClient.Get(testCtx, key, &rbacv1.Role{})).To(Succeed())
			Expect(k8sClient.Get(testCtx, key, &rbacv1.RoleBinding{})).To(Succeed())
			Expect(k8sClient.Get(testCtx, key, &appsv1.Deployment{})).To(Succeed())
			Expect(k8sClient.Get(testCtx, key, &corev1.Service{})).To(Succeed())
		})

		It("transitions to Ready and populates endpoint when deployment becomes ready", func() {
			name := nextName("ads")
			Expect(k8sClient.Create(testCtx, baseSession(name))).To(Succeed())

			r := &ArenaDevSessionReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			// Run enough reconciles to reach Starting (adds finalizer, sets phase, creates resources).
			for i := 0; i < 3; i++ {
				_, err := reconcileOnce(r, name)
				Expect(err).NotTo(HaveOccurred())
			}

			// Synthesize deployment-ready status.
			session := &omniav1alpha1.ArenaDevSession{}
			Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: name, Namespace: namespace}, session)).To(Succeed())
			deployKey := types.NamespacedName{Name: r.resourceName(session), Namespace: namespace}
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(testCtx, deployKey, deploy)).To(Succeed())
			deploy.Status.Replicas = 1
			deploy.Status.ReadyReplicas = 1
			Expect(k8sClient.Status().Update(testCtx, deploy)).To(Succeed())

			// Next reconcile should see a ready deployment and flip the session to Ready.
			_, err := reconcileOnce(r, name)
			Expect(err).NotTo(HaveOccurred())

			ready := &omniav1alpha1.ArenaDevSession{}
			Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: name, Namespace: namespace}, ready)).To(Succeed())
			Expect(ready.Status.Phase).To(Equal(omniav1alpha1.ArenaDevSessionPhaseReady))
			Expect(ready.Status.Endpoint).To(ContainSubstring("ws://"))
			Expect(ready.Status.Endpoint).To(ContainSubstring(namespace))
			Expect(ready.Status.ServiceName).To(Equal(r.resourceName(ready)))
			Expect(ready.Status.StartedAt).NotTo(BeNil())
			Expect(ready.Status.LastActivityAt).NotTo(BeNil())
		})

		It("cleans up child resources when the session is deleted", func() {
			name := nextName("ads")
			Expect(k8sClient.Create(testCtx, baseSession(name))).To(Succeed())

			r := &ArenaDevSessionReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			for i := 0; i < 3; i++ {
				_, err := reconcileOnce(r, name)
				Expect(err).NotTo(HaveOccurred())
			}

			session := &omniav1alpha1.ArenaDevSession{}
			Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: name, Namespace: namespace}, session)).To(Succeed())
			resourceName := r.resourceName(session)
			Expect(k8sClient.Delete(testCtx, session)).To(Succeed())

			// Two reconciles: first runs cleanup, second removes the finalizer.
			for i := 0; i < 2; i++ {
				_, err := reconcileOnce(r, name)
				Expect(err).NotTo(HaveOccurred())
			}

			// Session object should be gone.
			gone := &omniav1alpha1.ArenaDevSession{}
			err := k8sClient.Get(testCtx, types.NamespacedName{Name: name, Namespace: namespace}, gone)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(),
				"ArenaDevSession should be GC'd after finalizer removal, got: %v", err)

			// Child resources should be deleted too.
			key := types.NamespacedName{Name: resourceName, Namespace: namespace}
			err = k8sClient.Get(testCtx, key, &appsv1.Deployment{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Get(testCtx, key, &corev1.Service{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Get(testCtx, key, &rbacv1.RoleBinding{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Get(testCtx, key, &rbacv1.Role{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Get(testCtx, key, &corev1.ServiceAccount{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("cleans up a Ready session whose LastActivityAt is past the idle timeout", func() {
			name := nextName("ads")
			s := baseSession(name)
			s.Spec.IdleTimeout = "1s"
			Expect(k8sClient.Create(testCtx, s)).To(Succeed())

			r := &ArenaDevSessionReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			// Advance to Ready.
			for i := 0; i < 3; i++ {
				_, err := reconcileOnce(r, name)
				Expect(err).NotTo(HaveOccurred())
			}
			session := &omniav1alpha1.ArenaDevSession{}
			Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: name, Namespace: namespace}, session)).To(Succeed())
			deployKey := types.NamespacedName{Name: r.resourceName(session), Namespace: namespace}
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(testCtx, deployKey, deploy)).To(Succeed())
			deploy.Status.Replicas = 1
			deploy.Status.ReadyReplicas = 1
			Expect(k8sClient.Status().Update(testCtx, deploy)).To(Succeed())
			_, err := reconcileOnce(r, name)
			Expect(err).NotTo(HaveOccurred())

			// Manually backdate LastActivityAt to trip the idle check.
			ready := &omniav1alpha1.ArenaDevSession{}
			Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: name, Namespace: namespace}, ready)).To(Succeed())
			Expect(ready.Status.Phase).To(Equal(omniav1alpha1.ArenaDevSessionPhaseReady))
			backdated := metav1.NewTime(time.Now().Add(-5 * time.Second))
			ready.Status.LastActivityAt = &backdated
			Expect(k8sClient.Status().Update(testCtx, ready)).To(Succeed())

			// Next reconcile should trigger cleanup.
			_, err = reconcileOnce(r, name)
			Expect(err).NotTo(HaveOccurred())

			after := &omniav1alpha1.ArenaDevSession{}
			Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: name, Namespace: namespace}, after)).To(Succeed())
			Expect(after.Status.Phase).To(Equal(omniav1alpha1.ArenaDevSessionPhaseStopped),
				"idle session should be Stopped after cleanup")
		})

		It("uses the spec.image override on the deployed container", func() {
			name := nextName("ads")
			s := baseSession(name)
			s.Spec.Image = "example.com/custom-dev-console:v9"
			Expect(k8sClient.Create(testCtx, s)).To(Succeed())

			r := &ArenaDevSessionReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			for i := 0; i < 3; i++ {
				_, err := reconcileOnce(r, name)
				Expect(err).NotTo(HaveOccurred())
			}

			session := &omniav1alpha1.ArenaDevSession{}
			Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: name, Namespace: namespace}, session)).To(Succeed())
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(testCtx, types.NamespacedName{
				Name: r.resourceName(session), Namespace: namespace,
			}, deploy)).To(Succeed())
			Expect(deploy.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deploy.Spec.Template.Spec.Containers[0].Image).To(Equal("example.com/custom-dev-console:v9"))
		})
	})

	Context("resourceName", func() {
		It("prefixes short names with 'adc-' and leaves them unhashed", func() {
			r := &ArenaDevSessionReconciler{}
			s := &omniav1alpha1.ArenaDevSession{ObjectMeta: metav1.ObjectMeta{Name: "short"}}
			Expect(r.resourceName(s)).To(Equal("adc-short"))
		})

		It("hashes and truncates names that would exceed the 63-char DNS limit", func() {
			r := &ArenaDevSessionReconciler{}
			longName := strings.Repeat("a", 70)
			s := &omniav1alpha1.ArenaDevSession{ObjectMeta: metav1.ObjectMeta{Name: longName}}
			result := r.resourceName(s)
			Expect(len(result)).To(BeNumerically("<=", 63))
			Expect(result).To(HavePrefix("adc-"))
		})
	})
})
