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

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

var _ = Describe("ArenaJob Controller", func() {
	const (
		arenaJobName      = "test-arenajob"
		arenaJobNamespace = "default"
		arenaConfigName   = "test-config"
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

	Context("When reconciling an ArenaJob with missing ArenaConfig", func() {
		var arenaJob *omniav1alpha1.ArenaJob

		BeforeEach(func() {
			By("creating the ArenaJob with missing config")
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-config-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					ConfigRef: omniav1alpha1.LocalObjectReference{
						Name: "nonexistent-config",
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
				Name:      "missing-config-job",
				Namespace: arenaJobNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set Failed phase and ConfigValid condition to false", func() {
			By("reconciling the ArenaJob")
			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := &ArenaJobReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: fakeRecorder,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "missing-config-job",
					Namespace: arenaJobNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedJob := &omniav1alpha1.ArenaJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "missing-config-job",
				Namespace: arenaJobNamespace,
			}, updatedJob)).To(Succeed())

			Expect(updatedJob.Status.Phase).To(Equal(omniav1alpha1.ArenaJobPhaseFailed))

			By("checking the ConfigValid condition")
			condition := meta.FindStatusCondition(updatedJob.Status.Conditions, ArenaJobConditionTypeConfigValid)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("When reconciling an ArenaJob with ArenaConfig not ready", func() {
		var (
			arenaJob    *omniav1alpha1.ArenaJob
			arenaConfig *omniav1alpha1.ArenaConfig
		)

		BeforeEach(func() {
			By("creating the ArenaConfig in Pending state")
			arenaConfig = &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pending-config",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaConfigSpec{
					SourceRef: omniav1alpha1.LocalObjectReference{
						Name: arenaSourceName,
					},
					Providers: []omniav1alpha1.NamespacedObjectReference{
						{Name: "test-provider"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaConfig)).To(Succeed())

			// Set config to Pending
			arenaConfig.Status.Phase = omniav1alpha1.ArenaConfigPhasePending
			Expect(k8sClient.Status().Update(ctx, arenaConfig)).To(Succeed())

			By("creating the ArenaJob")
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pending-config-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					ConfigRef: omniav1alpha1.LocalObjectReference{
						Name: "pending-config",
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
				Name:      "pending-config-job",
				Namespace: arenaJobNamespace,
			}, job)
			if err == nil {
				Expect(k8sClient.Delete(ctx, job)).To(Succeed())
			}

			config := &omniav1alpha1.ArenaConfig{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "pending-config",
				Namespace: arenaJobNamespace,
			}, config)
			if err == nil {
				Expect(k8sClient.Delete(ctx, config)).To(Succeed())
			}
		})

		It("should set Failed phase due to config not ready", func() {
			By("reconciling the ArenaJob")
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "pending-config-job",
					Namespace: arenaJobNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedJob := &omniav1alpha1.ArenaJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "pending-config-job",
				Namespace: arenaJobNamespace,
			}, updatedJob)).To(Succeed())

			Expect(updatedJob.Status.Phase).To(Equal(omniav1alpha1.ArenaJobPhaseFailed))
		})
	})

	Context("When reconciling a valid ArenaJob", func() {
		var (
			arenaJob    *omniav1alpha1.ArenaJob
			arenaConfig *omniav1alpha1.ArenaConfig
		)

		BeforeEach(func() {
			By("creating the ArenaConfig in Ready state")
			arenaConfig = &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      arenaConfigName,
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaConfigSpec{
					SourceRef: omniav1alpha1.LocalObjectReference{
						Name: arenaSourceName,
					},
					Providers: []omniav1alpha1.NamespacedObjectReference{
						{Name: "test-provider"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaConfig)).To(Succeed())

			// Set config to Ready with resolved source
			arenaConfig.Status.Phase = omniav1alpha1.ArenaConfigPhaseReady
			arenaConfig.Status.ResolvedSource = &omniav1alpha1.ResolvedSource{
				Revision: "v1.0.0",
				URL:      "http://localhost:8080/artifacts/test.tar.gz",
			}
			arenaConfig.Status.ResolvedProviders = []string{"test-provider"}
			Expect(k8sClient.Status().Update(ctx, arenaConfig)).To(Succeed())

			By("creating the ArenaJob")
			replicas := int32(2)
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      arenaJobName,
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					ConfigRef: omniav1alpha1.LocalObjectReference{
						Name: arenaConfigName,
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

			config := &omniav1alpha1.ArenaConfig{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      arenaConfigName,
				Namespace: arenaJobNamespace,
			}, config)
			if err == nil {
				Expect(k8sClient.Delete(ctx, config)).To(Succeed())
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

			By("checking the ConfigValid condition")
			configCondition := meta.FindStatusCondition(updatedJob.Status.Conditions, ArenaJobConditionTypeConfigValid)
			Expect(configCondition).NotTo(BeNil())
			Expect(configCondition.Status).To(Equal(metav1.ConditionTrue))

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
					ConfigRef: omniav1alpha1.LocalObjectReference{
						Name: arenaConfigName,
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

			reconciler := &ArenaJobReconciler{}
			reconciler.setCondition(job, ArenaJobConditionTypeReady, metav1.ConditionTrue, "TestReason", "Test message")

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

	Context("When testing findArenaJobsForConfig", func() {
		var (
			arenaJob    *omniav1alpha1.ArenaJob
			arenaConfig *omniav1alpha1.ArenaConfig
		)

		BeforeEach(func() {
			By("creating the ArenaConfig")
			arenaConfig = &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "watch-config",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaConfigSpec{
					SourceRef: omniav1alpha1.LocalObjectReference{
						Name: arenaSourceName,
					},
					Providers: []omniav1alpha1.NamespacedObjectReference{
						{Name: "test-provider"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaConfig)).To(Succeed())

			By("creating the ArenaJob that references the config")
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "watch-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					ConfigRef: omniav1alpha1.LocalObjectReference{
						Name: "watch-config",
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

			config := &omniav1alpha1.ArenaConfig{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "watch-config",
				Namespace: arenaJobNamespace,
			}, config)
			if err == nil {
				Expect(k8sClient.Delete(ctx, config)).To(Succeed())
			}
		})

		It("should return reconcile requests for pending ArenaJobs referencing the config", func() {
			By("calling findArenaJobsForConfig")
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			requests := reconciler.findArenaJobsForConfig(ctx, arenaConfig)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("watch-job"))
			Expect(requests[0].Namespace).To(Equal(arenaJobNamespace))
		})

		It("should return nil for non-ArenaConfig objects", func() {
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			requests := reconciler.findArenaJobsForConfig(ctx, &corev1.Secret{})
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

	Context("When ArenaConfig is in Invalid phase", func() {
		var (
			arenaJob    *omniav1alpha1.ArenaJob
			arenaConfig *omniav1alpha1.ArenaConfig
		)

		BeforeEach(func() {
			By("creating the ArenaConfig in Invalid state")
			arenaConfig = &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-config",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaConfigSpec{
					SourceRef: omniav1alpha1.LocalObjectReference{
						Name: arenaSourceName,
					},
					Providers: []omniav1alpha1.NamespacedObjectReference{
						{Name: "test-provider"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaConfig)).To(Succeed())

			// Set config to Invalid
			arenaConfig.Status.Phase = omniav1alpha1.ArenaConfigPhaseInvalid
			Expect(k8sClient.Status().Update(ctx, arenaConfig)).To(Succeed())

			By("creating the ArenaJob")
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-config-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					ConfigRef: omniav1alpha1.LocalObjectReference{
						Name: "invalid-config",
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
				Name:      "invalid-config-job",
				Namespace: arenaJobNamespace,
			}, job)
			if err == nil {
				Expect(k8sClient.Delete(ctx, job)).To(Succeed())
			}

			config := &omniav1alpha1.ArenaConfig{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "invalid-config",
				Namespace: arenaJobNamespace,
			}, config)
			if err == nil {
				Expect(k8sClient.Delete(ctx, config)).To(Succeed())
			}
		})

		It("should set Failed phase due to invalid config", func() {
			By("reconciling the ArenaJob")
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "invalid-config-job",
					Namespace: arenaJobNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedJob := &omniav1alpha1.ArenaJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "invalid-config-job",
				Namespace: arenaJobNamespace,
			}, updatedJob)).To(Succeed())

			Expect(updatedJob.Status.Phase).To(Equal(omniav1alpha1.ArenaJobPhaseFailed))
		})
	})

	Context("When ArenaJob specifies TTL", func() {
		var (
			arenaJob    *omniav1alpha1.ArenaJob
			arenaConfig *omniav1alpha1.ArenaConfig
		)

		BeforeEach(func() {
			By("creating the ArenaConfig in Ready state")
			arenaConfig = &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ttl-config",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaConfigSpec{
					SourceRef: omniav1alpha1.LocalObjectReference{
						Name: arenaSourceName,
					},
					Providers: []omniav1alpha1.NamespacedObjectReference{
						{Name: "test-provider"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaConfig)).To(Succeed())

			arenaConfig.Status.Phase = omniav1alpha1.ArenaConfigPhaseReady
			arenaConfig.Status.ResolvedSource = &omniav1alpha1.ResolvedSource{
				Revision: "v1.0.0",
				URL:      "http://localhost:8080/artifacts/test.tar.gz",
			}
			Expect(k8sClient.Status().Update(ctx, arenaConfig)).To(Succeed())

			By("creating the ArenaJob with TTL")
			ttl := int32(300)
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ttl-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					ConfigRef: omniav1alpha1.LocalObjectReference{
						Name: "ttl-config",
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

			config := &omniav1alpha1.ArenaConfig{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "ttl-config",
				Namespace: arenaJobNamespace,
			}, config)
			if err == nil {
				Expect(k8sClient.Delete(ctx, config)).To(Succeed())
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
					ConfigRef: omniav1alpha1.LocalObjectReference{
						Name: arenaConfigName,
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
					ConfigRef: omniav1alpha1.LocalObjectReference{
						Name: arenaConfigName,
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
			arenaConfig *omniav1alpha1.ArenaConfig
			k8sJob      *batchv1.Job
		)

		BeforeEach(func() {
			By("creating the ArenaConfig in Ready state")
			arenaConfig = &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rereconcile-config",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaConfigSpec{
					SourceRef: omniav1alpha1.LocalObjectReference{
						Name: arenaSourceName,
					},
					Providers: []omniav1alpha1.NamespacedObjectReference{
						{Name: "test-provider"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaConfig)).To(Succeed())
			arenaConfig.Status.Phase = omniav1alpha1.ArenaConfigPhaseReady
			arenaConfig.Status.ResolvedSource = &omniav1alpha1.ResolvedSource{
				Revision: "v1.0.0",
				URL:      "http://localhost:8080/artifacts/test.tar.gz",
			}
			Expect(k8sClient.Status().Update(ctx, arenaConfig)).To(Succeed())

			By("creating the ArenaJob")
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rereconcile-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					ConfigRef: omniav1alpha1.LocalObjectReference{
						Name: "rereconcile-config",
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

			config := &omniav1alpha1.ArenaConfig{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "rereconcile-config",
				Namespace: arenaJobNamespace,
			}, config)
			if err == nil {
				Expect(k8sClient.Delete(ctx, config)).To(Succeed())
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

			// Should have ConfigValid condition
			configCondition := meta.FindStatusCondition(updatedJob.Status.Conditions, ArenaJobConditionTypeConfigValid)
			Expect(configCondition).NotTo(BeNil())
			Expect(configCondition.Status).To(Equal(metav1.ConditionTrue))

			// Should have progress tracking
			Expect(updatedJob.Status.Progress).NotTo(BeNil())
		})
	})
})
