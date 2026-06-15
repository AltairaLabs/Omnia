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
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// workerIdentityCounter gives each spec a unique namespace suffix so the
// per-job arena-worker RBAC objects (which share a fixed name) don't leak
// across specs — envtest has no garbage collector to reap them by ownerRef.
var workerIdentityCounter uint64

var _ = Describe("ArenaJob worker cloud identity", func() {
	const (
		runtimeSA  = "omnia-runtime-wi"
		wiLabelKey = "azure.workload.identity/use"
		wiLabelVal = "true"

		// Shared fixture literals — extracted so goconst doesn't flag these
		// heavily-used test strings as new occurrences.
		configMapName    = "test-configmap"
		artifactRevision = "v1.0.0"
		artifactChecksum = "sha256:abc123"
	)

	var (
		ctx       context.Context
		namespace string
		nextName  = func(prefix string) string {
			n := atomic.AddUint64(&workerIdentityCounter, 1)
			return fmt.Sprintf("%s-%d", prefix, n)
		}
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = nextName("aj-wi")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		})).To(Succeed())
	})

	AfterEach(func() {
		ns := &corev1.Namespace{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, ns); err == nil {
			_ = k8sClient.Delete(ctx, ns)
		}
	})

	// newReadySource creates an ArenaSource already in the Ready phase with an
	// artifact, which is the precondition for the ArenaJob reconciler to build
	// the worker Job.
	newReadySource := func(name string) {
		source := &omniav1alpha1.ArenaSource{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: omniav1alpha1.ArenaSourceSpec{
				Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
				Interval: "5m",
				ConfigMap: &corev1alpha1.ConfigMapSource{
					Name: configMapName,
				},
			},
		}
		Expect(k8sClient.Create(ctx, source)).To(Succeed())
		source.Status.Phase = omniav1alpha1.ArenaSourcePhaseReady
		source.Status.Artifact = &omniav1alpha1.Artifact{
			Revision:       artifactRevision,
			Checksum:       artifactChecksum,
			LastUpdateTime: metav1.Now(),
		}
		Expect(k8sClient.Status().Update(ctx, source)).To(Succeed())
	}

	newEvalJob := func(name, sourceName string) {
		job := &omniav1alpha1.ArenaJob{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: omniav1alpha1.ArenaJobSpec{
				SourceRef: corev1alpha1.LocalObjectReference{Name: sourceName},
				Type:      omniav1alpha1.ArenaJobTypeEvaluation,
				Workers:   &omniav1alpha1.WorkerConfig{Replicas: 1},
			},
		}
		Expect(k8sClient.Create(ctx, job)).To(Succeed())
	}

	reconcileJob := func(r *ArenaJobReconciler, name string) {
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
		})
		Expect(err).NotTo(HaveOccurred())
	}

	getWorkerJob := func(jobName string) *batchv1.Job {
		k8sJob := &batchv1.Job{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      jobName + "-worker",
			Namespace: namespace,
		}, k8sJob)).To(Succeed())
		return k8sJob
	}

	Context("when no worker ServiceAccount is configured (default)", func() {
		It("runs the worker under the bespoke arena-worker SA bound to its Role", func() {
			sourceName := nextName("src")
			jobName := nextName("job")
			newReadySource(sourceName)
			newEvalJob(jobName, sourceName)

			r := &ArenaJobReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}
			reconcileJob(r, jobName)

			// The worker pod runs as the operator-created per-job SA.
			k8sJob := getWorkerJob(jobName)
			Expect(k8sJob.Spec.Template.Spec.ServiceAccountName).To(Equal(arenaWorkerRBACName))

			// That SA, its Role, and a RoleBinding to it all exist.
			rbacKey := types.NamespacedName{Name: arenaWorkerRBACName, Namespace: namespace}
			Expect(k8sClient.Get(ctx, rbacKey, &corev1.ServiceAccount{})).To(Succeed())
			rb := &rbacv1.RoleBinding{}
			Expect(k8sClient.Get(ctx, rbacKey, rb)).To(Succeed())
			Expect(rb.Subjects).To(HaveLen(1))
			Expect(rb.Subjects[0].Name).To(Equal(arenaWorkerRBACName))
		})
	})

	Context("when a worker ServiceAccount + pod labels are configured", func() {
		It("runs the worker as that SA with the WI opt-in label and rebinds the Role", func() {
			sourceName := nextName("src")
			jobName := nextName("job")
			newReadySource(sourceName)
			newEvalJob(jobName, sourceName)

			r := &ArenaJobReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				Recorder:             record.NewFakeRecorder(10),
				WorkerServiceAccount: runtimeSA,
				WorkerPodLabels:      map[string]string{wiLabelKey: wiLabelVal},
			}
			reconcileJob(r, jobName)

			// The worker pod runs as the workspace runtime SA so it inherits the
			// workspace cloud identity, with the cloud-identity webhook opt-in
			// label stamped onto the pod template.
			k8sJob := getWorkerJob(jobName)
			Expect(k8sJob.Spec.Template.Spec.ServiceAccountName).To(Equal(runtimeSA))
			Expect(k8sJob.Spec.Template.Labels).To(HaveKeyWithValue(wiLabelKey, wiLabelVal))

			// The worker Role is bound to the configured SA so it carries the
			// namespace-scoped CRD-read perms the worker needs.
			rbacKey := types.NamespacedName{Name: arenaWorkerRBACName, Namespace: namespace}
			rb := &rbacv1.RoleBinding{}
			Expect(k8sClient.Get(ctx, rbacKey, rb)).To(Succeed())
			Expect(rb.Subjects).To(HaveLen(1))
			Expect(rb.Subjects[0].Name).To(Equal(runtimeSA))

			// No bespoke arena-worker ServiceAccount is created — the
			// externally-managed workspace SA is reused as-is.
			saErr := k8sClient.Get(ctx, rbacKey, &corev1.ServiceAccount{})
			Expect(apierrors.IsNotFound(saErr)).To(BeTrue())
		})
	})
})
