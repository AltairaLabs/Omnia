/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package controller

import (
	"context"
	"os"
	"path/filepath"

	"github.com/alicebob/miniredis/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/aggregator"
	"github.com/altairalabs/omnia/ee/pkg/arena/partitioner"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
	"github.com/altairalabs/omnia/ee/pkg/license"
)

var _ = Describe("ArenaJob Controller", func() {
	const (
		arenaJobName      = "test-arenajob"
		arenaJobNamespace = "default"
		arenaSourceName   = "test-source"
	)

	ctx := context.Background()

	Context("When reconciling a non-existent ArenaJob", func() {
		It("should return without error", func() {
			By("reconciling a non-existent ArenaJob")
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "nonexistent-job",
					Namespace: arenaJobNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling an ArenaJob with missing ArenaSource", func() {
		var arenaJob *omniav1alpha1.ArenaJob

		BeforeEach(func() {
			By("creating the ArenaJob with missing source")
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-source-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{
						Name: "nonexistent-source",
					},
					Type: omniav1alpha1.ArenaJobTypeEvaluation,
				},
			}
			Expect(k8sClient.Create(ctx, arenaJob)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the ArenaJob")
			resource := &omniav1alpha1.ArenaJob{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "missing-source-job",
				Namespace: arenaJobNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set Failed phase and SourceValid condition to false", func() {
			By("reconciling the ArenaJob")
			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := &ArenaJobReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: fakeRecorder,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "missing-source-job",
					Namespace: arenaJobNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedJob := &omniav1alpha1.ArenaJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "missing-source-job",
				Namespace: arenaJobNamespace,
			}, updatedJob)).To(Succeed())

			Expect(updatedJob.Status.Phase).To(Equal(omniav1alpha1.ArenaJobPhaseFailed))

			By("checking the SourceValid condition")
			condition := meta.FindStatusCondition(updatedJob.Status.Conditions, ArenaJobConditionTypeSourceValid)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("When reconciling an ArenaJob with ArenaSource not ready", func() {
		var (
			arenaJob    *omniav1alpha1.ArenaJob
			arenaSource *omniav1alpha1.ArenaSource
		)

		BeforeEach(func() {
			By("creating the ArenaSource in Pending state")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pending-source",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
					Interval: "5m",
					ConfigMap: &omniav1alpha1.ConfigMapSource{
						Name: "test-configmap",
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())

			// Set source to Pending
			arenaSource.Status.Phase = omniav1alpha1.ArenaSourcePhasePending
			Expect(k8sClient.Status().Update(ctx, arenaSource)).To(Succeed())

			By("creating the ArenaJob")
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pending-source-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{
						Name: "pending-source",
					},
					Type: omniav1alpha1.ArenaJobTypeEvaluation,
				},
			}
			Expect(k8sClient.Create(ctx, arenaJob)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			job := &omniav1alpha1.ArenaJob{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "pending-source-job",
				Namespace: arenaJobNamespace,
			}, job)
			if err == nil {
				Expect(k8sClient.Delete(ctx, job)).To(Succeed())
			}

			source := &omniav1alpha1.ArenaSource{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "pending-source",
				Namespace: arenaJobNamespace,
			}, source)
			if err == nil {
				Expect(k8sClient.Delete(ctx, source)).To(Succeed())
			}
		})

		It("should set Failed phase due to source not ready", func() {
			By("reconciling the ArenaJob")
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "pending-source-job",
					Namespace: arenaJobNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedJob := &omniav1alpha1.ArenaJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "pending-source-job",
				Namespace: arenaJobNamespace,
			}, updatedJob)).To(Succeed())

			Expect(updatedJob.Status.Phase).To(Equal(omniav1alpha1.ArenaJobPhaseFailed))
		})
	})

	Context("When reconciling a valid ArenaJob", func() {
		var (
			arenaJob    *omniav1alpha1.ArenaJob
			arenaSource *omniav1alpha1.ArenaSource
		)

		BeforeEach(func() {
			By("creating the ArenaSource in Ready state")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      arenaSourceName,
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
					Interval: "5m",
					ConfigMap: &omniav1alpha1.ConfigMapSource{
						Name: "test-configmap",
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())

			// Set source to Ready with artifact
			arenaSource.Status.Phase = omniav1alpha1.ArenaSourcePhaseReady
			arenaSource.Status.Artifact = &omniav1alpha1.Artifact{
				Revision:       "v1.0.0",
				Checksum:       "sha256:abc123",
				LastUpdateTime: metav1.Now(),
			}
			Expect(k8sClient.Status().Update(ctx, arenaSource)).To(Succeed())

			By("creating the ArenaJob")
			replicas := int32(2)
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      arenaJobName,
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{
						Name: arenaSourceName,
					},
					Type: omniav1alpha1.ArenaJobTypeEvaluation,
					Workers: &omniav1alpha1.WorkerConfig{
						Replicas: replicas,
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaJob)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			job := &omniav1alpha1.ArenaJob{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      arenaJobName,
				Namespace: arenaJobNamespace,
			}, job)
			if err == nil {
				Expect(k8sClient.Delete(ctx, job)).To(Succeed())
			}

			// Clean up the K8s Job
			k8sJob := &batchv1.Job{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      arenaJobName + "-worker",
				Namespace: arenaJobNamespace,
			}, k8sJob)
			if err == nil {
				// Delete with propagation policy
				propagation := metav1.DeletePropagationBackground
				Expect(k8sClient.Delete(ctx, k8sJob, &client.DeleteOptions{
					PropagationPolicy: &propagation,
				})).To(Succeed())
			}

			source := &omniav1alpha1.ArenaSource{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      arenaSourceName,
				Namespace: arenaJobNamespace,
			}, source)
			if err == nil {
				Expect(k8sClient.Delete(ctx, source)).To(Succeed())
			}
		})

		It("should create a K8s Job and set Running phase", func() {
			By("reconciling the ArenaJob")
			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := &ArenaJobReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: fakeRecorder,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      arenaJobName,
					Namespace: arenaJobNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedJob := &omniav1alpha1.ArenaJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      arenaJobName,
				Namespace: arenaJobNamespace,
			}, updatedJob)).To(Succeed())

			Expect(updatedJob.Status.Phase).To(Equal(omniav1alpha1.ArenaJobPhaseRunning))
			Expect(updatedJob.Status.StartTime).NotTo(BeNil())

			By("checking the SourceValid condition")
			sourceCondition := meta.FindStatusCondition(updatedJob.Status.Conditions, ArenaJobConditionTypeSourceValid)
			Expect(sourceCondition).NotTo(BeNil())
			Expect(sourceCondition.Status).To(Equal(metav1.ConditionTrue))

			By("checking the JobCreated condition")
			jobCondition := meta.FindStatusCondition(updatedJob.Status.Conditions, ArenaJobConditionTypeJobCreated)
			Expect(jobCondition).NotTo(BeNil())
			Expect(jobCondition.Status).To(Equal(metav1.ConditionTrue))

			By("verifying the K8s Job was created")
			k8sJob := &batchv1.Job{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      arenaJobName + "-worker",
				Namespace: arenaJobNamespace,
			}, k8sJob)).To(Succeed())

			Expect(*k8sJob.Spec.Parallelism).To(Equal(int32(2)))
			Expect(*k8sJob.Spec.Completions).To(Equal(int32(2)))
			Expect(k8sJob.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(k8sJob.Spec.Template.Spec.Containers[0].Name).To(Equal("worker"))
			Expect(k8sJob.Labels["omnia.altairalabs.ai/job"]).To(Equal(arenaJobName))
		})
	})

	Context("When ArenaJob is already completed", func() {
		var arenaJob *omniav1alpha1.ArenaJob

		BeforeEach(func() {
			By("creating the completed ArenaJob")
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "completed-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{
						Name: arenaSourceName,
					},
					Type: omniav1alpha1.ArenaJobTypeEvaluation,
				},
			}
			Expect(k8sClient.Create(ctx, arenaJob)).To(Succeed())

			// Set status to Succeeded
			arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseSucceeded
			Expect(k8sClient.Status().Update(ctx, arenaJob)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the ArenaJob")
			resource := &omniav1alpha1.ArenaJob{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "completed-job",
				Namespace: arenaJobNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should skip reconciliation", func() {
			By("reconciling the completed ArenaJob")
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "completed-job",
					Namespace: arenaJobNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("verifying status unchanged")
			updatedJob := &omniav1alpha1.ArenaJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "completed-job",
				Namespace: arenaJobNamespace,
			}, updatedJob)).To(Succeed())

			Expect(updatedJob.Status.Phase).To(Equal(omniav1alpha1.ArenaJobPhaseSucceeded))
		})
	})

	Context("When testing setCondition helper", func() {
		It("should set a condition on the ArenaJob", func() {
			job := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-condition-job",
					Namespace:  arenaJobNamespace,
					Generation: 1,
				},
			}

			SetCondition(&job.Status.Conditions, job.Generation, ArenaJobConditionTypeReady, metav1.ConditionTrue, "TestReason", "Test message")

			condition := meta.FindStatusCondition(job.Status.Conditions, ArenaJobConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal("TestReason"))
			Expect(condition.Message).To(Equal("Test message"))
			Expect(condition.ObservedGeneration).To(Equal(int64(1)))
		})
	})

	Context("When testing getJobName helper", func() {
		It("should return correct job name", func() {
			job := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-arena-job",
				},
			}

			reconciler := &ArenaJobReconciler{}
			Expect(reconciler.getJobName(job)).To(Equal("my-arena-job-worker"))
		})
	})

	Context("When testing getWorkerImage helper", func() {
		It("should return configured image when set", func() {
			reconciler := &ArenaJobReconciler{
				WorkerImage: "custom/worker:v1.0",
			}
			Expect(reconciler.getWorkerImage()).To(Equal("custom/worker:v1.0"))
		})

		It("should return default image when not set", func() {
			reconciler := &ArenaJobReconciler{}
			Expect(reconciler.getWorkerImage()).To(Equal(DefaultWorkerImage))
		})
	})

	Context("When testing getWorkerImagePullPolicy helper", func() {
		It("should return configured pull policy when set", func() {
			reconciler := &ArenaJobReconciler{
				WorkerImagePullPolicy: corev1.PullAlways,
			}
			Expect(reconciler.getWorkerImagePullPolicy()).To(Equal(corev1.PullAlways))
		})

		It("should return IfNotPresent when not set", func() {
			reconciler := &ArenaJobReconciler{}
			Expect(reconciler.getWorkerImagePullPolicy()).To(Equal(corev1.PullIfNotPresent))
		})
	})

	Context("When testing findArenaJobsForSource", func() {
		var (
			arenaJob    *omniav1alpha1.ArenaJob
			arenaSource *omniav1alpha1.ArenaSource
		)

		BeforeEach(func() {
			By("creating the ArenaSource")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "watch-source",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
					Interval: "5m",
					ConfigMap: &omniav1alpha1.ConfigMapSource{
						Name: "test-cm",
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())

			By("creating the ArenaJob that references the source")
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "watch-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{
						Name: "watch-source",
					},
					Type: omniav1alpha1.ArenaJobTypeEvaluation,
				},
			}
			Expect(k8sClient.Create(ctx, arenaJob)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			job := &omniav1alpha1.ArenaJob{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "watch-job",
				Namespace: arenaJobNamespace,
			}, job)
			if err == nil {
				Expect(k8sClient.Delete(ctx, job)).To(Succeed())
			}

			source := &omniav1alpha1.ArenaSource{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "watch-source",
				Namespace: arenaJobNamespace,
			}, source)
			if err == nil {
				Expect(k8sClient.Delete(ctx, source)).To(Succeed())
			}
		})

		It("should return reconcile requests for pending ArenaJobs referencing the source", func() {
			By("calling findArenaJobsForSource")
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			requests := reconciler.findArenaJobsForSource(ctx, arenaSource)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("watch-job"))
			Expect(requests[0].Namespace).To(Equal(arenaJobNamespace))
		})

		It("should return nil for non-ArenaSource objects", func() {
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			requests := reconciler.findArenaJobsForSource(ctx, &corev1.Secret{})
			Expect(requests).To(BeNil())
		})
	})

	Context("When testing findArenaJobsForJob", func() {
		It("should return reconcile request for ArenaJob owning the K8s Job", func() {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-worker",
					Namespace: arenaJobNamespace,
					Labels: map[string]string{
						"omnia.altairalabs.ai/job": "my-arena-job",
					},
				},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			requests := reconciler.findArenaJobsForJob(ctx, job)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("my-arena-job"))
			Expect(requests[0].Namespace).To(Equal(arenaJobNamespace))
		})

		It("should return nil for jobs without ArenaJob label", func() {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unrelated-job",
					Namespace: arenaJobNamespace,
				},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			requests := reconciler.findArenaJobsForJob(ctx, job)
			Expect(requests).To(BeNil())
		})

		It("should return nil for non-Job objects", func() {
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			requests := reconciler.findArenaJobsForJob(ctx, &corev1.Secret{})
			Expect(requests).To(BeNil())
		})
	})

	Context("When ArenaSource is in Error phase", func() {
		var (
			arenaJob    *omniav1alpha1.ArenaJob
			arenaSource *omniav1alpha1.ArenaSource
		)

		BeforeEach(func() {
			By("creating the ArenaSource in Error state")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "error-source",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
					Interval: "5m",
					ConfigMap: &omniav1alpha1.ConfigMapSource{
						Name: "test-configmap",
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())

			// Set source to Error
			arenaSource.Status.Phase = omniav1alpha1.ArenaSourcePhaseError
			Expect(k8sClient.Status().Update(ctx, arenaSource)).To(Succeed())

			By("creating the ArenaJob")
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "error-source-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{
						Name: "error-source",
					},
					Type: omniav1alpha1.ArenaJobTypeEvaluation,
				},
			}
			Expect(k8sClient.Create(ctx, arenaJob)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			job := &omniav1alpha1.ArenaJob{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "error-source-job",
				Namespace: arenaJobNamespace,
			}, job)
			if err == nil {
				Expect(k8sClient.Delete(ctx, job)).To(Succeed())
			}

			source := &omniav1alpha1.ArenaSource{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "error-source",
				Namespace: arenaJobNamespace,
			}, source)
			if err == nil {
				Expect(k8sClient.Delete(ctx, source)).To(Succeed())
			}
		})

		It("should set Failed phase due to source error", func() {
			By("reconciling the ArenaJob")
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "error-source-job",
					Namespace: arenaJobNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedJob := &omniav1alpha1.ArenaJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "error-source-job",
				Namespace: arenaJobNamespace,
			}, updatedJob)).To(Succeed())

			Expect(updatedJob.Status.Phase).To(Equal(omniav1alpha1.ArenaJobPhaseFailed))
		})
	})

	Context("When ArenaJob specifies TTL", func() {
		var (
			arenaJob    *omniav1alpha1.ArenaJob
			arenaSource *omniav1alpha1.ArenaSource
		)

		BeforeEach(func() {
			By("creating the ArenaSource in Ready state")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ttl-source",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
					Interval: "5m",
					ConfigMap: &omniav1alpha1.ConfigMapSource{
						Name: "test-configmap",
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())

			arenaSource.Status.Phase = omniav1alpha1.ArenaSourcePhaseReady
			arenaSource.Status.Artifact = &omniav1alpha1.Artifact{
				Revision:       "v1.0.0",
				Checksum:       "sha256:abc123",
				LastUpdateTime: metav1.Now(),
			}
			Expect(k8sClient.Status().Update(ctx, arenaSource)).To(Succeed())

			By("creating the ArenaJob with TTL")
			ttl := int32(300)
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ttl-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{
						Name: "ttl-source",
					},
					Type:                    omniav1alpha1.ArenaJobTypeLoadTest,
					TTLSecondsAfterFinished: &ttl,
				},
			}
			Expect(k8sClient.Create(ctx, arenaJob)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			job := &omniav1alpha1.ArenaJob{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "ttl-job",
				Namespace: arenaJobNamespace,
			}, job)
			if err == nil {
				Expect(k8sClient.Delete(ctx, job)).To(Succeed())
			}

			k8sJob := &batchv1.Job{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "ttl-job-worker",
				Namespace: arenaJobNamespace,
			}, k8sJob)
			if err == nil {
				propagation := metav1.DeletePropagationBackground
				Expect(k8sClient.Delete(ctx, k8sJob, &client.DeleteOptions{
					PropagationPolicy: &propagation,
				})).To(Succeed())
			}

			source := &omniav1alpha1.ArenaSource{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "ttl-source",
				Namespace: arenaJobNamespace,
			}, source)
			if err == nil {
				Expect(k8sClient.Delete(ctx, source)).To(Succeed())
			}
		})

		It("should create K8s Job with TTL set", func() {
			By("reconciling the ArenaJob")
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "ttl-job",
					Namespace: arenaJobNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the K8s Job TTL")
			k8sJob := &batchv1.Job{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "ttl-job-worker",
				Namespace: arenaJobNamespace,
			}, k8sJob)).To(Succeed())

			Expect(k8sJob.Spec.TTLSecondsAfterFinished).NotTo(BeNil())
			Expect(*k8sJob.Spec.TTLSecondsAfterFinished).To(Equal(int32(300)))
		})
	})

	Context("When testing SetupWithManager", func() {
		It("should return error with nil manager", func() {
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			err := reconciler.SetupWithManager(nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("When updating status from completed K8s Job", func() {
		It("should set Succeeded phase when job completes", func() {
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "status-test-job",
					Namespace: arenaJobNamespace,
				},
				Status: omniav1alpha1.ArenaJobStatus{
					Phase: omniav1alpha1.ArenaJobPhaseRunning,
				},
			}

			completions := int32(2)
			k8sJob := &batchv1.Job{
				Spec: batchv1.JobSpec{
					Completions: &completions,
				},
				Status: batchv1.JobStatus{
					Active:    0,
					Succeeded: 2,
					Failed:    0,
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}

			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := &ArenaJobReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: fakeRecorder,
			}

			reconciler.updateStatusFromJob(ctx, arenaJob, k8sJob)

			Expect(arenaJob.Status.Phase).To(Equal(omniav1alpha1.ArenaJobPhaseSucceeded))
			Expect(arenaJob.Status.CompletionTime).NotTo(BeNil())
			Expect(arenaJob.Status.Progress.Total).To(Equal(int32(2)))
			Expect(arenaJob.Status.Progress.Completed).To(Equal(int32(2)))
		})

		It("should set Failed phase when job fails", func() {
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "failed-status-job",
					Namespace: arenaJobNamespace,
				},
				Status: omniav1alpha1.ArenaJobStatus{
					Phase: omniav1alpha1.ArenaJobPhaseRunning,
				},
			}

			completions := int32(2)
			k8sJob := &batchv1.Job{
				Spec: batchv1.JobSpec{
					Completions: &completions,
				},
				Status: batchv1.JobStatus{
					Active:    0,
					Succeeded: 0,
					Failed:    2,
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobFailed,
							Status:  corev1.ConditionTrue,
							Reason:  "BackoffLimitExceeded",
							Message: "Job has reached backoff limit",
						},
					},
				},
			}

			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := &ArenaJobReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: fakeRecorder,
			}

			reconciler.updateStatusFromJob(ctx, arenaJob, k8sJob)

			Expect(arenaJob.Status.Phase).To(Equal(omniav1alpha1.ArenaJobPhaseFailed))
			Expect(arenaJob.Status.CompletionTime).NotTo(BeNil())
			Expect(arenaJob.Status.Progress.Failed).To(Equal(int32(2)))
		})

		It("should update progress when job is still running", func() {
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "running-status-job",
					Namespace: arenaJobNamespace,
				},
				Status: omniav1alpha1.ArenaJobStatus{
					Phase: omniav1alpha1.ArenaJobPhaseRunning,
				},
			}

			completions := int32(4)
			k8sJob := &batchv1.Job{
				Spec: batchv1.JobSpec{
					Completions: &completions,
				},
				Status: batchv1.JobStatus{
					Active:    2,
					Succeeded: 1,
					Failed:    0,
				},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			reconciler.updateStatusFromJob(ctx, arenaJob, k8sJob)

			Expect(arenaJob.Status.Phase).To(Equal(omniav1alpha1.ArenaJobPhaseRunning))
			Expect(arenaJob.Status.ActiveWorkers).To(Equal(int32(2)))
			Expect(arenaJob.Status.Progress.Total).To(Equal(int32(4)))
			Expect(arenaJob.Status.Progress.Completed).To(Equal(int32(1)))
			Expect(arenaJob.Status.Progress.Pending).To(Equal(int32(3)))
		})

		It("should use default completions when not specified", func() {
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-completions-job",
					Namespace: arenaJobNamespace,
				},
				Status: omniav1alpha1.ArenaJobStatus{
					Phase: omniav1alpha1.ArenaJobPhaseRunning,
				},
			}

			k8sJob := &batchv1.Job{
				Spec: batchv1.JobSpec{
					// No Completions specified - should default to 1
				},
				Status: batchv1.JobStatus{
					Active:    1,
					Succeeded: 0,
					Failed:    0,
				},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			reconciler.updateStatusFromJob(ctx, arenaJob, k8sJob)

			Expect(arenaJob.Status.Progress.Total).To(Equal(int32(1)))
		})
	})

	Context("When ArenaJob is cancelled", func() {
		var arenaJob *omniav1alpha1.ArenaJob

		BeforeEach(func() {
			By("creating the cancelled ArenaJob")
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cancelled-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{
						Name: arenaSourceName,
					},
					Type: omniav1alpha1.ArenaJobTypeEvaluation,
				},
			}
			Expect(k8sClient.Create(ctx, arenaJob)).To(Succeed())

			// Set status to Cancelled
			arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseCancelled
			Expect(k8sClient.Status().Update(ctx, arenaJob)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the ArenaJob")
			resource := &omniav1alpha1.ArenaJob{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "cancelled-job",
				Namespace: arenaJobNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should skip reconciliation for cancelled job", func() {
			By("reconciling the cancelled ArenaJob")
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "cancelled-job",
					Namespace: arenaJobNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("verifying status unchanged")
			updatedJob := &omniav1alpha1.ArenaJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "cancelled-job",
				Namespace: arenaJobNamespace,
			}, updatedJob)).To(Succeed())

			Expect(updatedJob.Status.Phase).To(Equal(omniav1alpha1.ArenaJobPhaseCancelled))
		})
	})

	Context("When ArenaJob has already failed", func() {
		var arenaJob *omniav1alpha1.ArenaJob

		BeforeEach(func() {
			By("creating the failed ArenaJob")
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "failed-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{
						Name: arenaSourceName,
					},
					Type: omniav1alpha1.ArenaJobTypeEvaluation,
				},
			}
			Expect(k8sClient.Create(ctx, arenaJob)).To(Succeed())

			// Set status to Failed
			arenaJob.Status.Phase = omniav1alpha1.ArenaJobPhaseFailed
			Expect(k8sClient.Status().Update(ctx, arenaJob)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the ArenaJob")
			resource := &omniav1alpha1.ArenaJob{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "failed-job",
				Namespace: arenaJobNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should skip reconciliation for failed job", func() {
			By("reconciling the failed ArenaJob")
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "failed-job",
					Namespace: arenaJobNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("verifying status unchanged")
			updatedJob := &omniav1alpha1.ArenaJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "failed-job",
				Namespace: arenaJobNamespace,
			}, updatedJob)).To(Succeed())

			Expect(updatedJob.Status.Phase).To(Equal(omniav1alpha1.ArenaJobPhaseFailed))
		})
	})

	Context("When re-reconciling a running ArenaJob with existing K8s Job", func() {
		var (
			arenaJob    *omniav1alpha1.ArenaJob
			arenaSource *omniav1alpha1.ArenaSource
			k8sJob      *batchv1.Job
		)

		BeforeEach(func() {
			By("creating the ArenaSource in Ready state")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rereconcile-source",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
					Interval: "5m",
					ConfigMap: &omniav1alpha1.ConfigMapSource{
						Name: "test-configmap",
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())
			arenaSource.Status.Phase = omniav1alpha1.ArenaSourcePhaseReady
			arenaSource.Status.Artifact = &omniav1alpha1.Artifact{
				Revision:       "v1.0.0",
				Checksum:       "sha256:abc123",
				LastUpdateTime: metav1.Now(),
			}
			Expect(k8sClient.Status().Update(ctx, arenaSource)).To(Succeed())

			By("creating the ArenaJob")
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rereconcile-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{
						Name: "rereconcile-source",
					},
					Type: omniav1alpha1.ArenaJobTypeEvaluation,
				},
			}
			Expect(k8sClient.Create(ctx, arenaJob)).To(Succeed())

			By("creating the K8s Job manually (simulating it already exists)")
			parallelism := int32(1)
			completions := int32(1)
			k8sJob = &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rereconcile-job-worker",
					Namespace: arenaJobNamespace,
					Labels: map[string]string{
						"omnia.altairalabs.ai/job": "rereconcile-job",
					},
				},
				Spec: batchv1.JobSpec{
					Parallelism: &parallelism,
					Completions: &completions,
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{
									Name:  "worker",
									Image: DefaultWorkerImage,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, k8sJob)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			job := &omniav1alpha1.ArenaJob{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "rereconcile-job",
				Namespace: arenaJobNamespace,
			}, job)
			if err == nil {
				Expect(k8sClient.Delete(ctx, job)).To(Succeed())
			}

			bJob := &batchv1.Job{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "rereconcile-job-worker",
				Namespace: arenaJobNamespace,
			}, bJob)
			if err == nil {
				propagation := metav1.DeletePropagationBackground
				Expect(k8sClient.Delete(ctx, bJob, &client.DeleteOptions{
					PropagationPolicy: &propagation,
				})).To(Succeed())
			}

			source := &omniav1alpha1.ArenaSource{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "rereconcile-source",
				Namespace: arenaJobNamespace,
			}, source)
			if err == nil {
				Expect(k8sClient.Delete(ctx, source)).To(Succeed())
			}
		})

		It("should update status from existing job without creating a new one", func() {
			By("reconciling the ArenaJob")
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "rereconcile-job",
					Namespace: arenaJobNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedJob := &omniav1alpha1.ArenaJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "rereconcile-job",
				Namespace: arenaJobNamespace,
			}, updatedJob)).To(Succeed())

			// Should have SourceValid condition
			sourceCondition := meta.FindStatusCondition(updatedJob.Status.Conditions, ArenaJobConditionTypeSourceValid)
			Expect(sourceCondition).NotTo(BeNil())
			Expect(sourceCondition.Status).To(Equal(metav1.ConditionTrue))

			// Should have progress tracking
			Expect(updatedJob.Status.Progress).NotTo(BeNil())
		})
	})

	Context("When testing getOrCreateQueue", func() {
		It("should return existing queue if already set", func() {
			memQueue := queue.NewMemoryQueueWithDefaults()
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Queue:  memQueue,
			}

			q, err := reconciler.getOrCreateQueue()
			Expect(err).NotTo(HaveOccurred())
			Expect(q).To(Equal(memQueue))
		})

		It("should return nil when no Redis address configured", func() {
			reconciler := &ArenaJobReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				RedisAddr: "",
			}

			q, err := reconciler.getOrCreateQueue()
			Expect(err).NotTo(HaveOccurred())
			Expect(q).To(BeNil())
		})

		It("should return error when Redis connection fails", func() {
			reconciler := &ArenaJobReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				RedisAddr: "invalid-host:12345",
			}

			q, err := reconciler.getOrCreateQueue()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to connect to Redis"))
			Expect(q).To(BeNil())
		})

		It("should create and cache queue on successful Redis connection", func() {
			// Start miniredis server
			mr, err := miniredis.Run()
			Expect(err).NotTo(HaveOccurred())
			defer mr.Close()

			reconciler := &ArenaJobReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				RedisAddr: mr.Addr(),
			}

			// First call should create the queue
			q1, err := reconciler.getOrCreateQueue()
			Expect(err).NotTo(HaveOccurred())
			Expect(q1).NotTo(BeNil())

			// Queue should be cached in the reconciler
			Expect(reconciler.Queue).NotTo(BeNil())

			// Second call should return the cached queue
			q2, err := reconciler.getOrCreateQueue()
			Expect(err).NotTo(HaveOccurred())
			Expect(q2).To(Equal(q1))
		})
	})

	Context("When testing enqueueWorkItems", func() {
		It("should skip enqueueing when no queue configured", func() {
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-queue-job",
					Namespace: arenaJobNamespace,
				},
			}

			arenaSource := &omniav1alpha1.ArenaSource{
				Status: omniav1alpha1.ArenaSourceStatus{
					Artifact: &omniav1alpha1.Artifact{
						Revision: "v1.0.0",
						Checksum: "sha256:abc123",
					},
				},
			}

			reconciler := &ArenaJobReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				RedisAddr: "", // No Redis configured
			}

			count, err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaSource, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))
		})

		It("should enqueue work items for providers", func() {
			memQueue := queue.NewMemoryQueueWithDefaults()
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "enqueue-test-job",
					Namespace: arenaJobNamespace,
				},
			}

			arenaSource := &omniav1alpha1.ArenaSource{
				Status: omniav1alpha1.ArenaSourceStatus{
					Artifact: &omniav1alpha1.Artifact{
						Revision: "v1.0.0",
						Checksum: "sha256:abc123",
					},
				},
			}

			// Create provider CRDs
			providerCRDs := []*corev1alpha1.Provider{
				{ObjectMeta: metav1.ObjectMeta{Name: "provider-1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "provider-2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "provider-3"}},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Queue:  memQueue,
			}

			count, err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaSource, providerCRDs)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(3))

			// Verify items were enqueued
			progress, err := memQueue.Progress(ctx, "enqueue-test-job")
			Expect(err).NotTo(HaveOccurred())
			Expect(progress.Total).To(Equal(3))
			Expect(progress.Pending).To(Equal(3))

			// Pop and verify the items
			for range 3 {
				item, err := memQueue.Pop(ctx, "enqueue-test-job")
				Expect(err).NotTo(HaveOccurred())
				Expect(item.JobID).To(Equal("enqueue-test-job"))
				Expect(item.ScenarioID).To(Equal("default"))
				Expect(item.MaxAttempts).To(Equal(3))
			}
		})
	})

	Context("When testing scenario × provider matrix distribution", func() {
		It("should create matrix work items when filesystem content is available", func() {
			// Set up a temp directory with arena config and scenario files
			dir := GinkgoT().TempDir()

			// Create scenario files
			Expect(os.WriteFile(filepath.Join(dir, "billing.scenario.yaml"), []byte(`
metadata:
  name: Billing Test
spec:
  id: billing
`), 0o644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(dir, "auth.scenario.yaml"), []byte(`
metadata:
  name: Auth Test
spec:
  id: auth
`), 0o644)).To(Succeed())

			// Create arena config
			Expect(os.WriteFile(filepath.Join(dir, "config.arena.yaml"), []byte(`
spec:
  scenarios:
    - file: billing.scenario.yaml
    - file: auth.scenario.yaml
`), 0o644)).To(Succeed())

			memQueue := queue.NewMemoryQueueWithDefaults()
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "matrix-test-job",
					Namespace: arenaJobNamespace,
				},
			}

			arenaSource := &omniav1alpha1.ArenaSource{
				Status: omniav1alpha1.ArenaSourceStatus{
					Artifact: &omniav1alpha1.Artifact{
						Revision:    "v1.0.0",
						ContentPath: "test-source",
					},
				},
			}

			providerCRDs := []*corev1alpha1.Provider{
				{ObjectMeta: metav1.ObjectMeta{Name: "openai-gpt4", Namespace: "default"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "claude-sonnet", Namespace: "default"}},
			}

			// Set WorkspaceContentPath so that getContentBasePath resolves to dir
			// Structure: {WorkspaceContentPath}/{workspace}/{namespace}/{contentPath}
			// We need: dir == WorkspaceContentPath/default/default/test-source
			reconciler := &ArenaJobReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				Queue:                memQueue,
				WorkspaceContentPath: filepath.Join(dir, ".."),
			}
			// Actually compute the path so it matches
			// dir needs to be {WorkspaceContentPath}/{workspace}/{namespace}/{contentPath}
			// workspace = namespace for nil client fallback, namespace = "default"
			basePath := filepath.Join(dir, "..", "default", "default", "test-source")
			Expect(os.MkdirAll(basePath, 0o755)).To(Succeed())

			// Copy scenario files and config to the correct path
			for _, f := range []string{"billing.scenario.yaml", "auth.scenario.yaml", "config.arena.yaml"} {
				data, err := os.ReadFile(filepath.Join(dir, f))
				Expect(err).NotTo(HaveOccurred())
				Expect(os.WriteFile(filepath.Join(basePath, f), data, 0o644)).To(Succeed())
			}

			// Update WorkspaceContentPath to parent of the workspace tree
			reconciler.WorkspaceContentPath = filepath.Join(dir, "..")

			count, err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaSource, providerCRDs)
			Expect(err).NotTo(HaveOccurred())
			// 2 scenarios × 2 providers = 4 work items
			Expect(count).To(Equal(4))

			// Verify items in queue
			progress, err := memQueue.Progress(ctx, "matrix-test-job")
			Expect(err).NotTo(HaveOccurred())
			Expect(progress.Total).To(Equal(4))
			Expect(progress.Pending).To(Equal(4))

			// Pop all items and verify scenario × provider combinations
			scenarioProviderPairs := make(map[string]bool)
			for range 4 {
				item, popErr := memQueue.Pop(ctx, "matrix-test-job")
				Expect(popErr).NotTo(HaveOccurred())
				pair := item.ScenarioID + "/" + item.ProviderID
				scenarioProviderPairs[pair] = true
			}
			Expect(scenarioProviderPairs).To(HaveLen(4))
			Expect(scenarioProviderPairs).To(HaveKey("billing/openai-gpt4"))
			Expect(scenarioProviderPairs).To(HaveKey("billing/claude-sonnet"))
			Expect(scenarioProviderPairs).To(HaveKey("auth/openai-gpt4"))
			Expect(scenarioProviderPairs).To(HaveKey("auth/claude-sonnet"))
		})

		It("should fall back to per-provider items when filesystem is unavailable", func() {
			memQueue := queue.NewMemoryQueueWithDefaults()
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fallback-test-job",
					Namespace: arenaJobNamespace,
				},
			}

			arenaSource := &omniav1alpha1.ArenaSource{
				Status: omniav1alpha1.ArenaSourceStatus{
					Artifact: &omniav1alpha1.Artifact{
						Revision: "v1.0.0",
					},
				},
			}

			providerCRDs := []*corev1alpha1.Provider{
				{ObjectMeta: metav1.ObjectMeta{Name: "provider-1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "provider-2"}},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Queue:  memQueue,
				// No WorkspaceContentPath — filesystem unavailable
			}

			count, err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaSource, providerCRDs)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(2))

			// All items should have ScenarioID "default"
			for range 2 {
				item, popErr := memQueue.Pop(ctx, "fallback-test-job")
				Expect(popErr).NotTo(HaveOccurred())
				Expect(item.ScenarioID).To(Equal("default"))
			}
		})

		It("should apply ScenarioFilter include/exclude when creating matrix", func() {
			dir := GinkgoT().TempDir()
			basePath := filepath.Join(dir, "default", "default", "test-source")
			Expect(os.MkdirAll(basePath, 0o755)).To(Succeed())

			// Create scenario files
			Expect(os.WriteFile(filepath.Join(basePath, "billing.scenario.yaml"), []byte(`
spec:
  id: billing
`), 0o644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(basePath, "auth.scenario.yaml"), []byte(`
spec:
  id: auth
`), 0o644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(basePath, "wip-test.scenario.yaml"), []byte(`
spec:
  id: wip-test
`), 0o644)).To(Succeed())

			// Create arena config referencing all three
			Expect(os.WriteFile(filepath.Join(basePath, "config.arena.yaml"), []byte(`
spec:
  scenarios:
    - file: billing.scenario.yaml
    - file: auth.scenario.yaml
    - file: wip-test.scenario.yaml
`), 0o644)).To(Succeed())

			memQueue := queue.NewMemoryQueueWithDefaults()
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "filter-test-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					Scenarios: &omniav1alpha1.ScenarioFilter{
						Exclude: []string{"wip-*"},
					},
				},
			}

			arenaSource := &omniav1alpha1.ArenaSource{
				Status: omniav1alpha1.ArenaSourceStatus{
					Artifact: &omniav1alpha1.Artifact{
						Revision:    "v1.0.0",
						ContentPath: "test-source",
					},
				},
			}

			providerCRDs := []*corev1alpha1.Provider{
				{ObjectMeta: metav1.ObjectMeta{Name: "provider-1", Namespace: "default"}},
			}

			reconciler := &ArenaJobReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				Queue:                memQueue,
				WorkspaceContentPath: dir,
			}

			count, err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaSource, providerCRDs)
			Expect(err).NotTo(HaveOccurred())
			// 2 scenarios (wip-test excluded) × 1 provider = 2 items
			Expect(count).To(Equal(2))

			// Verify wip-test was excluded
			scenarioIDs := make(map[string]bool)
			for range 2 {
				item, popErr := memQueue.Pop(ctx, "filter-test-job")
				Expect(popErr).NotTo(HaveOccurred())
				scenarioIDs[item.ScenarioID] = true
			}
			Expect(scenarioIDs).To(HaveKey("billing"))
			Expect(scenarioIDs).To(HaveKey("auth"))
			Expect(scenarioIDs).NotTo(HaveKey("wip-test"))
		})

		It("should create single default item when no providers", func() {
			memQueue := queue.NewMemoryQueueWithDefaults()
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-providers-job",
					Namespace: arenaJobNamespace,
				},
			}

			arenaSource := &omniav1alpha1.ArenaSource{
				Status: omniav1alpha1.ArenaSourceStatus{
					Artifact: &omniav1alpha1.Artifact{
						Revision: "v1.0.0",
					},
				},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Queue:  memQueue,
			}

			count, err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaSource, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))

			item, err := memQueue.Pop(ctx, "no-providers-job")
			Expect(err).NotTo(HaveOccurred())
			Expect(item.ScenarioID).To(Equal("default"))
			Expect(item.ProviderID).To(BeEmpty())
		})

		It("should fall back to per-provider when no scenarios found on filesystem", func() {
			dir := GinkgoT().TempDir()
			basePath := filepath.Join(dir, "default", "default", "test-source")
			Expect(os.MkdirAll(basePath, 0o755)).To(Succeed())

			// Create arena config with no scenarios section
			Expect(os.WriteFile(filepath.Join(basePath, "config.arena.yaml"), []byte(`
spec:
  providers:
    - name: openai
`), 0o644)).To(Succeed())

			memQueue := queue.NewMemoryQueueWithDefaults()
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-scenarios-job",
					Namespace: arenaJobNamespace,
				},
			}

			arenaSource := &omniav1alpha1.ArenaSource{
				Status: omniav1alpha1.ArenaSourceStatus{
					Artifact: &omniav1alpha1.Artifact{
						Revision:    "v1.0.0",
						ContentPath: "test-source",
					},
				},
			}

			providerCRDs := []*corev1alpha1.Provider{
				{ObjectMeta: metav1.ObjectMeta{Name: "provider-1", Namespace: "default"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "provider-2", Namespace: "default"}},
			}

			reconciler := &ArenaJobReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				Queue:                memQueue,
				WorkspaceContentPath: dir,
			}

			count, err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaSource, providerCRDs)
			Expect(err).NotTo(HaveOccurred())
			// Falls back to per-provider: 2 items with ScenarioID "default"
			Expect(count).To(Equal(2))

			for range 2 {
				item, popErr := memQueue.Pop(ctx, "no-scenarios-job")
				Expect(popErr).NotTo(HaveOccurred())
				Expect(item.ScenarioID).To(Equal("default"))
			}
		})
	})

	Context("When updating status from completed K8s Job with aggregator", func() {
		It("should aggregate results and populate JobResult", func() {
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aggregator-test-job",
					Namespace: arenaJobNamespace,
				},
				Status: omniav1alpha1.ArenaJobStatus{
					Phase: omniav1alpha1.ArenaJobPhaseRunning,
				},
			}

			completions := int32(3)
			k8sJob := &batchv1.Job{
				Spec: batchv1.JobSpec{
					Completions: &completions,
				},
				Status: batchv1.JobStatus{
					Active:    0,
					Succeeded: 3,
					Failed:    0,
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}

			// Create a memory queue with completed items
			memQueue := queue.NewMemoryQueueWithDefaults()
			queueCtx := context.Background()
			items := []queue.WorkItem{
				{ID: "item-1", ScenarioID: "scenario-1", ProviderID: "provider-1"},
				{ID: "item-2", ScenarioID: "scenario-1", ProviderID: "provider-2"},
				{ID: "item-3", ScenarioID: "scenario-2", ProviderID: "provider-1"},
			}
			Expect(memQueue.Push(queueCtx, "aggregator-test-job", items)).To(Succeed())

			// Pop and ack all items with results
			for range 3 {
				item, err := memQueue.Pop(queueCtx, "aggregator-test-job")
				Expect(err).NotTo(HaveOccurred())
				result := []byte(`{"status": "pass", "durationMs": 100, "metrics": {"tokens": 50, "cost": 0.01}}`)
				Expect(memQueue.Ack(queueCtx, "aggregator-test-job", item.ID, result)).To(Succeed())
			}

			// Create aggregator
			agg := aggregator.New(memQueue)

			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := &ArenaJobReconciler{
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				Recorder:   fakeRecorder,
				Queue:      memQueue,
				Aggregator: agg,
			}

			reconciler.updateStatusFromJob(ctx, arenaJob, k8sJob)

			Expect(arenaJob.Status.Phase).To(Equal(omniav1alpha1.ArenaJobPhaseSucceeded))
			Expect(arenaJob.Status.Result).NotTo(BeNil())
			Expect(arenaJob.Status.Result.Summary).NotTo(BeNil())
			Expect(arenaJob.Status.Result.Summary["passRate"]).To(Equal("100.0"))
			Expect(arenaJob.Status.Result.Summary["totalItems"]).To(Equal("3"))
			Expect(arenaJob.Status.Result.Summary["passedItems"]).To(Equal("3"))
			Expect(arenaJob.Status.Result.Summary["failedItems"]).To(Equal("0"))
			Expect(arenaJob.Status.Result.Summary["totalTokens"]).To(Equal("150"))
		})

		It("should handle aggregator errors gracefully", func() {
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aggregator-error-job",
					Namespace: arenaJobNamespace,
				},
				Status: omniav1alpha1.ArenaJobStatus{
					Phase: omniav1alpha1.ArenaJobPhaseRunning,
				},
			}

			completions := int32(1)
			k8sJob := &batchv1.Job{
				Spec: batchv1.JobSpec{
					Completions: &completions,
				},
				Status: batchv1.JobStatus{
					Active:    0,
					Succeeded: 1,
					Failed:    0,
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}

			// Create a memory queue WITHOUT the job (will cause ErrJobNotFound)
			memQueue := queue.NewMemoryQueueWithDefaults()
			agg := aggregator.New(memQueue)

			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := &ArenaJobReconciler{
				Client:     k8sClient,
				Scheme:     k8sClient.Scheme(),
				Recorder:   fakeRecorder,
				Queue:      memQueue,
				Aggregator: agg,
			}

			// Should not panic, should just log error and continue
			reconciler.updateStatusFromJob(ctx, arenaJob, k8sJob)

			// Job should still be marked as succeeded even if aggregation fails
			Expect(arenaJob.Status.Phase).To(Equal(omniav1alpha1.ArenaJobPhaseSucceeded))
			// Result should be nil since aggregation failed
			Expect(arenaJob.Status.Result).To(BeNil())
		})

		It("should work without aggregator configured", func() {
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-aggregator-job",
					Namespace: arenaJobNamespace,
				},
				Status: omniav1alpha1.ArenaJobStatus{
					Phase: omniav1alpha1.ArenaJobPhaseRunning,
				},
			}

			completions := int32(1)
			k8sJob := &batchv1.Job{
				Spec: batchv1.JobSpec{
					Completions: &completions,
				},
				Status: batchv1.JobStatus{
					Active:    0,
					Succeeded: 1,
					Failed:    0,
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}

			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := &ArenaJobReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: fakeRecorder,
				// No Queue or Aggregator configured
			}

			reconciler.updateStatusFromJob(ctx, arenaJob, k8sJob)

			// Job should be marked as succeeded
			Expect(arenaJob.Status.Phase).To(Equal(omniav1alpha1.ArenaJobPhaseSucceeded))
			// Result should be nil since no aggregator
			Expect(arenaJob.Status.Result).To(BeNil())
		})
	})

	Context("When finding ArenaJobs for source", func() {
		It("should return nil when object is not an ArenaSource", func() {
			By("calling findArenaJobsForSource with wrong object type")
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			// Pass a wrong object type (Namespace instead of ArenaSource)
			ns := &corev1.Namespace{}
			result := reconciler.findArenaJobsForSource(ctx, ns)
			Expect(result).To(BeNil())
		})
	})

	Context("When license validation fails", func() {
		var (
			arenaJob    *omniav1alpha1.ArenaJob
			arenaSource *omniav1alpha1.ArenaSource
		)

		BeforeEach(func() {
			By("creating the ArenaSource in Ready state")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "license-test-source",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
					Interval: "5m",
					ConfigMap: &omniav1alpha1.ConfigMapSource{
						Name: "test-cm",
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())

			arenaSource.Status.Phase = omniav1alpha1.ArenaSourcePhaseReady
			arenaSource.Status.Artifact = &omniav1alpha1.Artifact{
				Revision:       "v1.0.0",
				Checksum:       "sha256:abc123",
				LastUpdateTime: metav1.Now(),
			}
			Expect(k8sClient.Status().Update(ctx, arenaSource)).To(Succeed())

			By("creating the ArenaJob with replicas that exceed license limit")
			replicas := int32(2) // OpenCoreLicense has MaxWorkerReplicas: 1
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "license-fail-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{
						Name: "license-test-source",
					},
					Type: omniav1alpha1.ArenaJobTypeEvaluation,
					Workers: &omniav1alpha1.WorkerConfig{
						Replicas: replicas,
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaJob)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			job := &omniav1alpha1.ArenaJob{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "license-fail-job",
				Namespace: arenaJobNamespace,
			}, job)
			if err == nil {
				Expect(k8sClient.Delete(ctx, job)).To(Succeed())
			}

			source := &omniav1alpha1.ArenaSource{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "license-test-source",
				Namespace: arenaJobNamespace,
			}, source)
			if err == nil {
				Expect(k8sClient.Delete(ctx, source)).To(Succeed())
			}
		})

		It("should set Failed phase with LicenseViolation when replicas exceed limit", func() {
			By("creating a license validator (no license secret = OpenCoreLicense)")
			validator, err := license.NewValidator(k8sClient)
			Expect(err).NotTo(HaveOccurred())

			By("reconciling the ArenaJob with license validator")
			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := &ArenaJobReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				Recorder:         fakeRecorder,
				LicenseValidator: validator,
			}

			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "license-fail-job",
					Namespace: arenaJobNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedJob := &omniav1alpha1.ArenaJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "license-fail-job",
				Namespace: arenaJobNamespace,
			}, updatedJob)).To(Succeed())

			Expect(updatedJob.Status.Phase).To(Equal(omniav1alpha1.ArenaJobPhaseFailed))

			By("checking the Ready condition shows license violation")
			condition := meta.FindStatusCondition(updatedJob.Status.Conditions, ArenaJobConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("LicenseViolation"))
		})
	})

	Context("When looking up workspace for namespace", func() {
		It("should return namespace name when client is nil", func() {
			By("calling getWorkspaceForNamespace with nil client")
			reconciler := &ArenaJobReconciler{
				Client: nil,
			}
			result := GetWorkspaceForNamespace(ctx, reconciler.Client, "test-namespace")
			Expect(result).To(Equal("test-namespace"))
		})

		It("should return workspace label when present on namespace", func() {
			By("creating a namespace with workspace label")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns-with-workspace-label",
					Labels: map[string]string{
						"omnia.altairalabs.ai/workspace": "my-workspace",
					},
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, ns)
			})

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			result := GetWorkspaceForNamespace(ctx, reconciler.Client, "ns-with-workspace-label")
			Expect(result).To(Equal("my-workspace"))
		})

		It("should return namespace name when workspace label is missing", func() {
			By("creating a namespace without workspace label")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns-without-workspace-label",
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, ns)
			})

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			result := GetWorkspaceForNamespace(ctx, reconciler.Client, "ns-without-workspace-label")
			Expect(result).To(Equal("ns-without-workspace-label"))
		})

		It("should return namespace name when namespace does not exist", func() {
			By("calling getWorkspaceForNamespace for non-existent namespace")
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			result := GetWorkspaceForNamespace(ctx, reconciler.Client, "non-existent-namespace")
			Expect(result).To(Equal("non-existent-namespace"))
		})
	})

	Context("Provider Override Functions", func() {
		It("should resolve provider overrides using label selectors", func() {
			By("creating provider CRDs with labels")
			provider1 := &corev1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-provider-1",
					Namespace: "default",
					Labels: map[string]string{
						"tier": "production",
						"team": "ml",
					},
				},
				Spec: corev1alpha1.ProviderSpec{
					Type:  "openai",
					Model: "gpt-4",
				},
			}
			Expect(k8sClient.Create(ctx, provider1)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, provider1)
			})

			provider2 := &corev1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-provider-2",
					Namespace: "default",
					Labels: map[string]string{
						"tier": "staging",
						"team": "ml",
					},
				},
				Spec: corev1alpha1.ProviderSpec{
					Type:  "claude",
					Model: "claude-3-sonnet",
				},
			}
			Expect(k8sClient.Create(ctx, provider2)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, provider2)
			})

			By("creating an ArenaJob with provider overrides")
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "job-with-overrides",
					Namespace: "default",
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					ProviderOverrides: map[string]omniav1alpha1.ProviderGroupSelector{
						"default": {
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									"tier": "production",
								},
							},
						},
					},
				},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("resolving provider overrides")
			providersByGroup, err := reconciler.resolveProviderOverrides(ctx, arenaJob)
			Expect(err).NotTo(HaveOccurred())
			Expect(providersByGroup).To(HaveLen(1))
			Expect(providersByGroup["default"]).To(HaveLen(1))
			Expect(providersByGroup["default"][0].Name).To(Equal("test-provider-1"))
		})

		It("should return nil when no provider overrides specified", func() {
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "job-without-overrides",
					Namespace: "default",
				},
				Spec: omniav1alpha1.ArenaJobSpec{},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			providers, err := reconciler.resolveProviderOverrides(ctx, arenaJob)
			Expect(err).NotTo(HaveOccurred())
			Expect(providers).To(BeNil())
		})

		It("should build env vars from provider CRDs with secretRef", func() {
			providerCRDs := []*corev1alpha1.Provider{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "openai-provider",
						Namespace: "default",
					},
					Spec: corev1alpha1.ProviderSpec{
						Type:  "openai",
						Model: "gpt-4",
						SecretRef: &corev1alpha1.SecretKeyRef{
							Name: "custom-openai-secret",
						},
					},
				},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			envVars := reconciler.buildProviderEnvVarsFromCRDs(providerCRDs)
			Expect(envVars).NotTo(BeEmpty())

			// Find the OPENAI_API_KEY env var
			var foundOpenAI bool
			for _, env := range envVars {
				if env.Name == "OPENAI_API_KEY" {
					foundOpenAI = true
					Expect(env.ValueFrom).NotTo(BeNil())
					Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal("custom-openai-secret"))
				}
			}
			Expect(foundOpenAI).To(BeTrue())
		})

		It("should build env vars from provider CRDs without secretRef", func() {
			providerCRDs := []*corev1alpha1.Provider{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "claude-provider",
						Namespace: "default",
					},
					Spec: corev1alpha1.ProviderSpec{
						Type:  "claude",
						Model: "claude-3-opus",
					},
				},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			envVars := reconciler.buildProviderEnvVarsFromCRDs(providerCRDs)
			Expect(envVars).NotTo(BeEmpty())

			// Find the ANTHROPIC_API_KEY env var (claude provider uses ANTHROPIC_API_KEY)
			var foundAnthropic bool
			for _, env := range envVars {
				if env.Name == "ANTHROPIC_API_KEY" {
					foundAnthropic = true
					Expect(env.ValueFrom).NotTo(BeNil())
					// Should use default secret naming convention: ANTHROPIC_API_KEY -> anthropic-api-key
					Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal("anthropic-api-key"))
				}
			}
			Expect(foundAnthropic).To(BeTrue())
		})

		It("should extract provider IDs from CRDs", func() {
			providerCRDs := []*corev1alpha1.Provider{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "provider-alpha",
						Namespace: "default",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "provider-beta",
						Namespace: "default",
					},
				},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			ids := reconciler.getProviderIDsFromCRDs(providerCRDs)
			Expect(ids).To(Equal([]string{"provider-alpha", "provider-beta"}))
		})

		It("should deduplicate providers resolved from multiple groups", func() {
			By("creating a provider that matches multiple selectors")
			provider := &corev1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared-provider",
					Namespace: "default",
					Labels: map[string]string{
						"tier": "production",
						"role": "judge",
					},
				},
				Spec: corev1alpha1.ProviderSpec{
					Type:  "openai",
					Model: "gpt-4",
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, provider)
			})

			By("creating an ArenaJob with multiple groups selecting same provider")
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "job-with-multi-group-overrides",
					Namespace: "default",
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					ProviderOverrides: map[string]omniav1alpha1.ProviderGroupSelector{
						"default": {
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									"tier": "production",
								},
							},
						},
						"judge": {
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									"role": "judge",
								},
							},
						},
					},
				},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("resolving provider overrides - should return per-group")
			providersByGroup, err := reconciler.resolveProviderOverrides(ctx, arenaJob)
			Expect(err).NotTo(HaveOccurred())
			// Should have two groups (default and judge) each with the same provider
			Expect(providersByGroup).To(HaveLen(2))
			Expect(providersByGroup["default"]).To(HaveLen(1))
			Expect(providersByGroup["judge"]).To(HaveLen(1))
			Expect(providersByGroup["default"][0].Name).To(Equal("shared-provider"))
			Expect(providersByGroup["judge"][0].Name).To(Equal("shared-provider"))
		})
	})

	Context("Tool Registry Overrides", func() {
		It("should return nil when no tool registry override is specified", func() {
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "job-without-tool-override",
					Namespace: "default",
				},
				Spec: omniav1alpha1.ArenaJobSpec{},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			toolOverrides, err := reconciler.resolveToolRegistryOverride(ctx, arenaJob)
			Expect(err).NotTo(HaveOccurred())
			Expect(toolOverrides).To(BeNil())
		})

		It("should resolve tools from matching ToolRegistry CRDs", func() {
			By("creating a ToolRegistry with a tool")
			endpoint := "http://weather-service:8080"
			toolRegistry := &corev1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-tool-registry",
					Namespace: "default",
					Labels: map[string]string{
						"environment": "production",
					},
				},
				Spec: corev1alpha1.ToolRegistrySpec{
					Handlers: []corev1alpha1.HandlerDefinition{
						{
							Name: "weather-handler",
							Type: corev1alpha1.HandlerTypeHTTP,
							Tool: &corev1alpha1.ToolDefinition{
								Name:        "get_weather",
								Description: "Get weather data",
								InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object","properties":{"city":{"type":"string"}}}`)},
							},
							HTTPConfig: &corev1alpha1.HTTPConfig{
								Endpoint: endpoint,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, toolRegistry)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, toolRegistry)
			})

			By("creating an ArenaJob with tool registry override")
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "job-with-tool-override",
					Namespace: "default",
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					ToolRegistryOverride: &omniav1alpha1.ToolRegistrySelector{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"environment": "production",
							},
						},
					},
				},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("resolving tool registry override")
			toolOverrides, err := reconciler.resolveToolRegistryOverride(ctx, arenaJob)
			Expect(err).NotTo(HaveOccurred())
			Expect(toolOverrides).To(HaveLen(1))
			Expect(toolOverrides).To(HaveKey("get_weather"))
			Expect(toolOverrides["get_weather"].Endpoint).To(Equal(endpoint))
			Expect(toolOverrides["get_weather"].RegistryName).To(Equal("test-tool-registry"))
		})

		It("should return empty map when no registries match selector", func() {
			By("creating an ArenaJob with non-matching selector")
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "job-with-nonmatching-override",
					Namespace: "default",
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					ToolRegistryOverride: &omniav1alpha1.ToolRegistrySelector{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"environment": "nonexistent",
							},
						},
					},
				},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			toolOverrides, err := reconciler.resolveToolRegistryOverride(ctx, arenaJob)
			Expect(err).NotTo(HaveOccurred())
			Expect(toolOverrides).To(BeNil())
		})
	})

	Context("convertProviderToOverride with platform and auth config", func() {
		It("should pass through platform configuration", func() {
			provider := &corev1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bedrock-provider",
					Namespace: "default",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type:  corev1alpha1.ProviderTypeBedrock,
					Model: "anthropic.claude-3-sonnet-20240229-v1:0",
					Platform: &corev1alpha1.PlatformConfig{
						Type:   corev1alpha1.PlatformTypeAWS,
						Region: "us-east-1",
					},
				},
			}

			override := convertProviderToOverride(provider)

			Expect(override.ID).To(Equal("bedrock-provider"))
			Expect(override.Type).To(Equal("bedrock"))
			Expect(override.Platform).NotTo(BeNil())
			Expect(override.Platform.Type).To(Equal("aws"))
			Expect(override.Platform.Region).To(Equal("us-east-1"))
		})

		It("should pass through auth configuration", func() {
			provider := &corev1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bedrock-wi-provider",
					Namespace: "default",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type:  corev1alpha1.ProviderTypeBedrock,
					Model: "anthropic.claude-3-sonnet-20240229-v1:0",
					Platform: &corev1alpha1.PlatformConfig{
						Type:   corev1alpha1.PlatformTypeAWS,
						Region: "us-east-1",
					},
					Auth: &corev1alpha1.AuthConfig{
						Type:    corev1alpha1.AuthMethodWorkloadIdentity,
						RoleArn: "arn:aws:iam::123456789012:role/my-role",
					},
				},
			}

			override := convertProviderToOverride(provider)

			Expect(override.AuthMethod).To(Equal("workloadIdentity"))
			Expect(override.RoleARN).To(Equal("arn:aws:iam::123456789012:role/my-role"))
			Expect(override.Platform).NotTo(BeNil())
		})

		It("should pass through GCP auth with serviceAccountEmail", func() {
			provider := &corev1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vertex-wi-provider",
					Namespace: "default",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type:  corev1alpha1.ProviderTypeVertex,
					Model: "gemini-1.5-pro",
					Platform: &corev1alpha1.PlatformConfig{
						Type:    corev1alpha1.PlatformTypeGCP,
						Region:  "us-central1",
						Project: "my-project",
					},
					Auth: &corev1alpha1.AuthConfig{
						Type:                corev1alpha1.AuthMethodWorkloadIdentity,
						ServiceAccountEmail: "my-sa@my-project.iam.gserviceaccount.com",
					},
				},
			}

			override := convertProviderToOverride(provider)

			Expect(override.AuthMethod).To(Equal("workloadIdentity"))
			Expect(override.ServiceAccountEmail).To(Equal("my-sa@my-project.iam.gserviceaccount.com"))
			Expect(override.Platform).NotTo(BeNil())
			Expect(override.Platform.Project).To(Equal("my-project"))
		})

		It("should not set auth fields when auth is nil", func() {
			provider := &corev1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "claude-noauth",
					Namespace: "default",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type:  corev1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
				},
			}

			override := convertProviderToOverride(provider)

			Expect(override.AuthMethod).To(BeEmpty())
			Expect(override.RoleARN).To(BeEmpty())
			Expect(override.ServiceAccountEmail).To(BeEmpty())
			Expect(override.Platform).To(BeNil())
		})
	})

	Context("getWorkerServiceAccountName", func() {
		It("should return empty when WorkerServiceAccountName is not configured", func() {
			reconciler := &ArenaJobReconciler{
				Client:                   k8sClient,
				Scheme:                   k8sClient.Scheme(),
				WorkerServiceAccountName: "",
			}

			providerCRDs := []*corev1alpha1.Provider{
				{
					Spec: corev1alpha1.ProviderSpec{
						Type: corev1alpha1.ProviderTypeBedrock,
						Auth: &corev1alpha1.AuthConfig{
							Type: corev1alpha1.AuthMethodWorkloadIdentity,
						},
					},
				},
			}

			Expect(reconciler.getWorkerServiceAccountName(providerCRDs)).To(BeEmpty())
		})

		It("should return SA name when a provider uses workload identity", func() {
			reconciler := &ArenaJobReconciler{
				Client:                   k8sClient,
				Scheme:                   k8sClient.Scheme(),
				WorkerServiceAccountName: "my-arena-worker",
			}

			providerCRDs := []*corev1alpha1.Provider{
				{
					Spec: corev1alpha1.ProviderSpec{
						Type: corev1alpha1.ProviderTypeBedrock,
						Auth: &corev1alpha1.AuthConfig{
							Type: corev1alpha1.AuthMethodWorkloadIdentity,
						},
					},
				},
			}

			Expect(reconciler.getWorkerServiceAccountName(providerCRDs)).To(Equal("my-arena-worker"))
		})

		It("should return empty when no provider uses workload identity", func() {
			reconciler := &ArenaJobReconciler{
				Client:                   k8sClient,
				Scheme:                   k8sClient.Scheme(),
				WorkerServiceAccountName: "my-arena-worker",
			}

			providerCRDs := []*corev1alpha1.Provider{
				{
					Spec: corev1alpha1.ProviderSpec{
						Type: corev1alpha1.ProviderTypeClaude,
						// No auth config
					},
				},
				{
					Spec: corev1alpha1.ProviderSpec{
						Type: corev1alpha1.ProviderTypeBedrock,
						Auth: &corev1alpha1.AuthConfig{
							Type: corev1alpha1.AuthMethodAccessKey,
						},
					},
				},
			}

			Expect(reconciler.getWorkerServiceAccountName(providerCRDs)).To(BeEmpty())
		})

		It("should return SA name when at least one provider uses workload identity among multiple", func() {
			reconciler := &ArenaJobReconciler{
				Client:                   k8sClient,
				Scheme:                   k8sClient.Scheme(),
				WorkerServiceAccountName: "my-arena-worker",
			}

			providerCRDs := []*corev1alpha1.Provider{
				{
					Spec: corev1alpha1.ProviderSpec{
						Type: corev1alpha1.ProviderTypeClaude,
					},
				},
				{
					Spec: corev1alpha1.ProviderSpec{
						Type: corev1alpha1.ProviderTypeBedrock,
						Auth: &corev1alpha1.AuthConfig{
							Type: corev1alpha1.AuthMethodWorkloadIdentity,
						},
					},
				},
			}

			Expect(reconciler.getWorkerServiceAccountName(providerCRDs)).To(Equal("my-arena-worker"))
		})

		It("should return empty when providers list is empty", func() {
			reconciler := &ArenaJobReconciler{
				Client:                   k8sClient,
				Scheme:                   k8sClient.Scheme(),
				WorkerServiceAccountName: "my-arena-worker",
			}

			Expect(reconciler.getWorkerServiceAccountName([]*corev1alpha1.Provider{})).To(BeEmpty())
		})
	})

	Context("When testing fleet mode helpers", func() {
		It("should detect fleet mode correctly", func() {
			// Not fleet mode when execution is nil
			arenaJob := &omniav1alpha1.ArenaJob{}
			Expect(isFleetMode(arenaJob)).To(BeFalse())

			// Not fleet mode when mode is direct
			arenaJob.Spec.Execution = &omniav1alpha1.ExecutionConfig{
				Mode: omniav1alpha1.ExecutionModeDirect,
			}
			Expect(isFleetMode(arenaJob)).To(BeFalse())

			// Fleet mode when mode is fleet
			arenaJob.Spec.Execution = &omniav1alpha1.ExecutionConfig{
				Mode: omniav1alpha1.ExecutionModeFleet,
			}
			Expect(isFleetMode(arenaJob)).To(BeTrue())
		})

		It("should build fleet work items per scenario", func() {
			scenarios := []partitioner.Scenario{
				{ID: "billing", Name: "Billing Test", Path: "billing.scenario.yaml"},
				{ID: "auth", Name: "Auth Test", Path: "auth.scenario.yaml"},
			}

			items := buildFleetWorkItems("fleet-job", "bundle-url", scenarios)
			Expect(items).To(HaveLen(2))
			Expect(items[0].ScenarioID).To(Equal("billing"))
			Expect(items[0].ProviderID).To(BeEmpty())
			Expect(items[0].JobID).To(Equal("fleet-job"))
			Expect(items[1].ScenarioID).To(Equal("auth"))
			Expect(items[1].ProviderID).To(BeEmpty())
		})

		It("should build single default fleet work item when no scenarios", func() {
			items := buildFleetWorkItems("fleet-job", "bundle-url", nil)
			Expect(items).To(HaveLen(1))
			Expect(items[0].ScenarioID).To(Equal("default"))
			Expect(items[0].ProviderID).To(BeEmpty())
		})

		It("should enqueue fleet work items without provider dimension", func() {
			// Set up filesystem content with proper workspace/namespace structure
			baseDir := GinkgoT().TempDir()
			contentDir := filepath.Join(baseDir, "default", arenaJobNamespace, "content")
			Expect(os.MkdirAll(contentDir, 0o755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(contentDir, "billing.scenario.yaml"), []byte(`
metadata:
  name: Billing Test
spec:
  id: billing
`), 0o644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(contentDir, "config.arena.yaml"), []byte(`
spec:
  scenarios:
    - file: billing.scenario.yaml
`), 0o644)).To(Succeed())

			memQueue := queue.NewMemoryQueueWithDefaults()
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fleet-enqueue-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{Name: arenaSourceName},
					Execution: &omniav1alpha1.ExecutionConfig{
						Mode: omniav1alpha1.ExecutionModeFleet,
					},
				},
			}

			arenaSource := &omniav1alpha1.ArenaSource{
				Status: omniav1alpha1.ArenaSourceStatus{
					Artifact: &omniav1alpha1.Artifact{
						Revision:    "v1.0.0",
						Checksum:    "sha256:abc123",
						ContentPath: "content",
					},
				},
			}

			reconciler := &ArenaJobReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				Queue:                memQueue,
				WorkspaceContentPath: baseDir,
			}

			// Even with providers, fleet mode should create scenario-only items
			providerCRDs := []*corev1alpha1.Provider{
				{ObjectMeta: metav1.ObjectMeta{Name: "provider-1"}},
			}

			count, err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaSource, providerCRDs)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))

			item, err := memQueue.Pop(ctx, "fleet-enqueue-job")
			Expect(err).NotTo(HaveOccurred())
			Expect(item.ScenarioID).To(Equal("billing"))
			Expect(item.ProviderID).To(BeEmpty())
		})

		It("should resolve fleet target from AgentRuntime", func() {
			By("creating an AgentRuntime with service endpoint")
			agentRuntime := &corev1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent-runtime",
					Namespace: arenaJobNamespace,
				},
				Spec: corev1alpha1.AgentRuntimeSpec{
					Facade: corev1alpha1.FacadeConfig{
						Type: corev1alpha1.FacadeTypeWebSocket,
					},
					PromptPackRef: corev1alpha1.PromptPackRef{
						Name: "test-prompt-pack",
					},
				},
			}
			Expect(k8sClient.Create(ctx, agentRuntime)).To(Succeed())

			// Update status with service endpoint
			agentRuntime.Status.ServiceEndpoint = "test-agent-runtime.default.svc.cluster.local:8080"
			Expect(k8sClient.Status().Update(ctx, agentRuntime)).To(Succeed())

			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fleet-resolve-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{Name: arenaSourceName},
					Execution: &omniav1alpha1.ExecutionConfig{
						Mode: omniav1alpha1.ExecutionModeFleet,
						Target: &omniav1alpha1.FleetTarget{
							AgentRuntimeRef: corev1alpha1.LocalObjectReference{
								Name: "test-agent-runtime",
							},
						},
					},
				},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			wsURL, err := reconciler.resolveFleetTarget(ctx, arenaJob)
			Expect(err).NotTo(HaveOccurred())
			Expect(wsURL).To(Equal("ws://test-agent-runtime.default.svc.cluster.local:8080/ws"))

			// Cleanup
			Expect(k8sClient.Delete(ctx, agentRuntime)).To(Succeed())
		})

		It("should fail to resolve fleet target when AgentRuntime not found", func() {
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fleet-notfound-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{Name: arenaSourceName},
					Execution: &omniav1alpha1.ExecutionConfig{
						Mode: omniav1alpha1.ExecutionModeFleet,
						Target: &omniav1alpha1.FleetTarget{
							AgentRuntimeRef: corev1alpha1.LocalObjectReference{
								Name: "nonexistent-agent",
							},
						},
					},
				},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.resolveFleetTarget(ctx, arenaJob)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})
})
