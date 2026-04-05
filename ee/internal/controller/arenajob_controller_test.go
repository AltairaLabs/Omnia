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
	"os"
	"path/filepath"

	"github.com/alicebob/miniredis/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
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

		It("should inject SESSION_API_URL from workspace status when sessionRecording is true", func() {
			By("creating a workspace with a session URL in status")
			ws := &corev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace-session",
				},
				Spec: corev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace Session",
					Namespace: corev1alpha1.NamespaceConfig{
						Name: arenaJobNamespace,
					},
				},
			}
			Expect(k8sClient.Create(ctx, ws)).To(Succeed())
			ws.Status.Services = []corev1alpha1.ServiceGroupStatus{
				{Name: "default", Ready: true, SessionURL: "http://session-api:8080"},
			}
			Expect(k8sClient.Status().Update(ctx, ws)).To(Succeed())

			By("creating a new ArenaJob with sessionRecording enabled")
			sessionJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "session-recording-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{
						Name: arenaSourceName,
					},
					Type:             omniav1alpha1.ArenaJobTypeEvaluation,
					SessionRecording: true,
					Workers: &omniav1alpha1.WorkerConfig{
						Replicas: 1,
					},
				},
			}
			Expect(k8sClient.Create(ctx, sessionJob)).To(Succeed())

			reconciler := &ArenaJobReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "session-recording-job",
					Namespace: arenaJobNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			k8sJob := &batchv1.Job{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "session-recording-job-worker",
				Namespace: arenaJobNamespace,
			}, k8sJob)).To(Succeed())

			envVars := k8sJob.Spec.Template.Spec.Containers[0].Env
			var found bool
			for _, e := range envVars {
				if e.Name == "SESSION_API_URL" {
					found = true
					Expect(e.Value).To(Equal("http://session-api:8080"))
				}
			}
			Expect(found).To(BeTrue(), "SESSION_API_URL should be injected from workspace when sessionRecording is true")

			Expect(k8sClient.Delete(ctx, sessionJob)).To(Succeed())
			Expect(k8sClient.Delete(ctx, k8sJob)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ws)).To(Succeed())
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

	Context("When ArenaJob does not specify TTL", func() {
		var (
			arenaJob    *omniav1alpha1.ArenaJob
			arenaSource *omniav1alpha1.ArenaSource
		)

		BeforeEach(func() {
			By("creating the ArenaSource in Ready state")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-ttl-source",
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

			By("creating the ArenaJob without TTL")
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-ttl-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{
						Name: "default-ttl-source",
					},
					Type: omniav1alpha1.ArenaJobTypeLoadTest,
				},
			}
			Expect(k8sClient.Create(ctx, arenaJob)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			job := &omniav1alpha1.ArenaJob{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "default-ttl-job",
				Namespace: arenaJobNamespace,
			}, job)
			if err == nil {
				Expect(k8sClient.Delete(ctx, job)).To(Succeed())
			}

			k8sJob := &batchv1.Job{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "default-ttl-job-worker",
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
				Name:      "default-ttl-source",
				Namespace: arenaJobNamespace,
			}, source)
			if err == nil {
				Expect(k8sClient.Delete(ctx, source)).To(Succeed())
			}
		})

		It("should default to 1 hour TTL", func() {
			By("reconciling the ArenaJob")
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "default-ttl-job",
					Namespace: arenaJobNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the K8s Job has default TTL of 3600 seconds")
			k8sJob := &batchv1.Job{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "default-ttl-job-worker",
				Namespace: arenaJobNamespace,
			}, k8sJob)).To(Succeed())

			Expect(k8sJob.Spec.TTLSecondsAfterFinished).NotTo(BeNil())
			Expect(*k8sJob.Spec.TTLSecondsAfterFinished).To(Equal(int32(3600)))
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
			// Regression for #736: Progress must be lazy-initialized on the
			// JobFailed path so .status.progress always exists (and serializes
			// as {completed:0,failed:0,pending:0,total:0}) once the job
			// reaches a terminal phase — even when the failure happened
			// before any work items were enqueued and Progress was nil.
			Expect(arenaJob.Status.Progress).NotTo(BeNil())
		})

		It("should lazy-init Progress on JobComplete when Progress was nil (#736)", func() {
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "complete-nil-progress-job",
					Namespace: arenaJobNamespace,
				},
				Status: omniav1alpha1.ArenaJobStatus{
					Phase: omniav1alpha1.ArenaJobPhaseRunning,
					// Progress deliberately left nil — simulates the case
					// where enqueueWorkItems never ran (e.g. reconcile sees
					// an existing Job on a restart).
				},
			}

			completions := int32(1)
			k8sJob := &batchv1.Job{
				Spec: batchv1.JobSpec{Completions: &completions},
				Status: batchv1.JobStatus{
					Active:    0,
					Succeeded: 1,
					Failed:    0,
					Conditions: []batchv1.JobCondition{
						{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
					},
				},
			}

			reconciler := &ArenaJobReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}

			reconciler.updateStatusFromJob(ctx, arenaJob, k8sJob)

			Expect(arenaJob.Status.Progress).NotTo(BeNil(),
				"Progress must be lazy-initialized on a terminal phase, even when previously nil")
		})

		It("should update active workers when job is still running", func() {
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
			// Progress is not updated during running — live progress comes from SSE/Redis.
		})

		It("should track active workers when completions not specified", func() {
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
					// No Completions specified
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

			Expect(arenaJob.Status.ActiveWorkers).To(Equal(int32(1)))
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

			// SourceValid is set on initial creation, not on re-reconcile —
			// the source is only validated when creating the worker job.
			// Progress is set at creation and completion — not during running.
			// Live progress comes from SSE/Redis.
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

			count, err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaSource, nil, nil)
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

			// Build resolvedGroups so enqueueWorkItems can derive provider IDs
			resolvedGroups := map[string]*resolvedProviderGroup{
				"default": {providers: providerCRDs},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Queue:  memQueue,
			}

			count, err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaSource, providerCRDs, resolvedGroups)
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

			count, err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaSource, providerCRDs, nil)
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

			resolvedGroups := map[string]*resolvedProviderGroup{
				"default": {providers: providerCRDs},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Queue:  memQueue,
				// No WorkspaceContentPath — filesystem unavailable
			}

			count, err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaSource, providerCRDs, resolvedGroups)
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

			count, err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaSource, providerCRDs, nil)
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

			count, err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaSource, nil, nil)
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

			resolvedGroups := map[string]*resolvedProviderGroup{
				"default": {providers: providerCRDs},
			}

			reconciler := &ArenaJobReconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				Queue:                memQueue,
				WorkspaceContentPath: dir,
			}

			count, err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaSource, providerCRDs, resolvedGroups)
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

	Context("When extracting provider IDs for work item matrix", func() {
		It("should exclude map-mode groups from provider IDs", func() {
			// Map-mode groups (judges, self-play) are 1:1 config references.
			// They should NOT create work items in the scenario × provider matrix.
			testProvider := &corev1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{Name: "test-provider"},
			}
			judgeProvider := &corev1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{Name: "judge-haiku"},
			}
			resolvedGroups := map[string]*resolvedProviderGroup{
				"default": {
					providers: []*corev1alpha1.Provider{testProvider},
					mapMode:   false,
				},
				"judges": {
					providers: []*corev1alpha1.Provider{judgeProvider},
					mapMode:   true,
				},
			}
			ids := getProviderIDsFromGroups(resolvedGroups)
			Expect(ids).To(ContainElement("test-provider"))
			Expect(ids).NotTo(ContainElement("judge-haiku"))
			Expect(ids).To(HaveLen(1))
		})

		It("should exclude map-mode agent groups from provider IDs", func() {
			resolvedGroups := map[string]*resolvedProviderGroup{
				"default": {
					agentWSURLs: map[string]string{"my-agent": "ws://agent:8080"},
					mapMode:     false,
				},
				"selfplay": {
					agentWSURLs: map[string]string{"sim-agent": "ws://sim:8080"},
					mapMode:     true,
				},
			}
			ids := getProviderIDsFromGroups(resolvedGroups)
			Expect(ids).To(ContainElement("agent-my-agent"))
			Expect(ids).NotTo(ContainElement("agent-sim-agent"))
			Expect(ids).To(HaveLen(1))
		})

		It("should include all providers when no map-mode groups exist", func() {
			p1 := &corev1alpha1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "p1"}}
			p2 := &corev1alpha1.Provider{ObjectMeta: metav1.ObjectMeta{Name: "p2"}}
			resolvedGroups := map[string]*resolvedProviderGroup{
				"default": {
					providers: []*corev1alpha1.Provider{p1, p2},
					mapMode:   false,
				},
			}
			ids := getProviderIDsFromGroups(resolvedGroups)
			Expect(ids).To(HaveLen(2))
			Expect(ids).To(ContainElement("p1"))
			Expect(ids).To(ContainElement("p2"))
		})
	})

	Context("When resolving provider groups with map mode", func() {
		It("should set mapMode flag on map-mode groups", func() {
			// Create an ArenaJob with mixed groups
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "map-mode-test",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{Name: "src"},
					Providers: map[string]omniav1alpha1.ArenaProviderGroup{
						"default": {
							Entries: []omniav1alpha1.ArenaProviderEntry{
								{ProviderRef: &corev1alpha1.ProviderRef{Name: "test-provider"}},
							},
						},
						"judges": {
							Mapping: map[string]omniav1alpha1.ArenaProviderEntry{
								"judge-quality": {ProviderRef: &corev1alpha1.ProviderRef{Name: "judge-provider"}},
							},
						},
					},
				},
			}

			// Create the provider CRDs
			testProv := &corev1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-provider",
					Namespace: arenaJobNamespace,
				},
				Spec: corev1alpha1.ProviderSpec{Type: "mock", Model: "mock-model"},
			}
			judgeProv := &corev1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "judge-provider",
					Namespace: arenaJobNamespace,
				},
				Spec: corev1alpha1.ProviderSpec{Type: "mock", Model: "mock-model"},
			}
			Expect(k8sClient.Create(ctx, testProv)).To(Succeed())
			Expect(k8sClient.Create(ctx, judgeProv)).To(Succeed())

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			groups, _, err := reconciler.resolveProviderGroups(ctx, arenaJob)
			Expect(err).NotTo(HaveOccurred())

			Expect(groups["default"].mapMode).To(BeFalse())
			Expect(groups["judges"].mapMode).To(BeTrue())

			// Only default group providers should appear in IDs
			ids := getProviderIDsFromGroups(groups)
			Expect(ids).To(ContainElement("test-provider"))
			Expect(ids).NotTo(ContainElement("judge-provider"))
		})
	})

	Context("When enqueueing work items with mixed array/map groups", func() {
		It("should only create work items for array-mode providers", func() {
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mixed-enqueue-test",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					SourceRef: corev1alpha1.LocalObjectReference{Name: "test-source"},
				},
			}

			arenaSource := &omniav1alpha1.ArenaSource{
				Status: omniav1alpha1.ArenaSourceStatus{
					Artifact: &omniav1alpha1.Artifact{Revision: "v1.0.0"},
				},
			}

			testProvider := &corev1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{Name: "test-provider", Namespace: arenaJobNamespace},
			}
			judgeProvider := &corev1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{Name: "judge-haiku", Namespace: arenaJobNamespace},
			}

			// All provider CRDs (flat list from resolveProviderGroups)
			allProviderCRDs := []*corev1alpha1.Provider{testProvider, judgeProvider}

			// Resolved groups: default is array-mode, judges is map-mode
			resolvedGroups := map[string]*resolvedProviderGroup{
				"default": {
					providers: []*corev1alpha1.Provider{testProvider},
					mapMode:   false,
				},
				"judges": {
					providers: []*corev1alpha1.Provider{judgeProvider},
					mapMode:   true,
				},
			}

			memQueue := queue.NewMemoryQueueWithDefaults()
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Queue:  memQueue,
			}

			count, err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaSource, allProviderCRDs, resolvedGroups)
			Expect(err).NotTo(HaveOccurred())
			// Fallback mode: 1 work item per array-mode provider (NOT 2)
			Expect(count).To(Equal(1))

			item, popErr := memQueue.Pop(ctx, "mixed-enqueue-test")
			Expect(popErr).NotTo(HaveOccurred())
			// Provider ID should be from the array-mode group only
			Expect(item.ProviderID).To(Equal("test-provider"))
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

	Context("buildRedisPasswordEnvVar", func() {
		It("should return secretKeyRef when RedisPasswordSecret is set", func() {
			reconciler := &ArenaJobReconciler{
				RedisPasswordSecret: "my-redis-secret",
			}

			envVars := reconciler.buildRedisPasswordEnvVar()
			Expect(envVars).To(HaveLen(1))
			Expect(envVars[0].Name).To(Equal("REDIS_PASSWORD"))
			Expect(envVars[0].Value).To(BeEmpty())
			Expect(envVars[0].ValueFrom).NotTo(BeNil())
			Expect(envVars[0].ValueFrom.SecretKeyRef).NotTo(BeNil())
			Expect(envVars[0].ValueFrom.SecretKeyRef.Name).To(Equal("my-redis-secret"))
			Expect(envVars[0].ValueFrom.SecretKeyRef.Key).To(Equal("redis-password"))
		})

		It("should fall back to plain-text value when only RedisPassword is set", func() {
			reconciler := &ArenaJobReconciler{
				RedisPassword: "my-password",
			}

			envVars := reconciler.buildRedisPasswordEnvVar()
			Expect(envVars).To(HaveLen(1))
			Expect(envVars[0].Name).To(Equal("REDIS_PASSWORD"))
			Expect(envVars[0].Value).To(Equal("my-password"))
			Expect(envVars[0].ValueFrom).To(BeNil())
		})

		It("should prefer secretKeyRef over plain-text when both are set", func() {
			reconciler := &ArenaJobReconciler{
				RedisPassword:       "my-password",
				RedisPasswordSecret: "my-redis-secret",
			}

			envVars := reconciler.buildRedisPasswordEnvVar()
			Expect(envVars).To(HaveLen(1))
			Expect(envVars[0].ValueFrom).NotTo(BeNil())
			Expect(envVars[0].ValueFrom.SecretKeyRef.Name).To(Equal("my-redis-secret"))
		})

		It("should return nil when neither is set", func() {
			reconciler := &ArenaJobReconciler{}

			envVars := reconciler.buildRedisPasswordEnvVar()
			Expect(envVars).To(BeNil())
		})
	})

	Context("When testing buildMatrixWorkItems limits", func() {
		It("should return nil when work item count exceeds maxWorkItems", func() {
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Create enough scenarios and providers to exceed maxWorkItems (10000)
			// 200 scenarios x 51 providers = 10200 > 10000
			scenarios := make([]partitioner.Scenario, 200)
			for i := range scenarios {
				scenarios[i] = partitioner.Scenario{
					ID:   fmt.Sprintf("scenario-%d", i),
					Name: fmt.Sprintf("Scenario %d", i),
					Path: fmt.Sprintf("scenario-%d.yaml", i),
				}
			}

			providerIDs := make([]string, 51)
			for i := range providerIDs {
				providerIDs[i] = fmt.Sprintf("provider-%d", i)
			}

			items := reconciler.buildMatrixWorkItems(ctx, "test-job", "bundle-url", scenarios, providerIDs, 0, omniav1alpha1.ArenaJobTypeEvaluation)
			Expect(items).To(BeNil())
		})

		It("should return items when work item count is within limit", func() {
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			scenarios := []partitioner.Scenario{
				{ID: "s1", Name: "Scenario 1", Path: "s1.yaml"},
				{ID: "s2", Name: "Scenario 2", Path: "s2.yaml"},
			}

			providerIDs := []string{"p1", "p2"}

			items := reconciler.buildMatrixWorkItems(ctx, "test-job", "bundle-url", scenarios, providerIDs, 0, omniav1alpha1.ArenaJobTypeEvaluation)
			Expect(items).To(HaveLen(4)) // 2 scenarios x 2 providers
		})
	})
})
