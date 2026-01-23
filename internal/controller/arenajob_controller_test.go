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

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/arena/aggregator"
	"github.com/altairalabs/omnia/pkg/arena/queue"
	"github.com/altairalabs/omnia/pkg/license"
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

			arenaConfig := &omniav1alpha1.ArenaConfig{
				Status: omniav1alpha1.ArenaConfigStatus{
					ResolvedProviders: []string{"provider-1"},
					ResolvedSource: &omniav1alpha1.ResolvedSource{
						URL: "http://example.com/artifact.tar.gz",
					},
				},
			}

			reconciler := &ArenaJobReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				RedisAddr: "", // No Redis configured
			}

			err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaConfig, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip enqueueing when no providers configured", func() {
			memQueue := queue.NewMemoryQueueWithDefaults()
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-providers-job",
					Namespace: arenaJobNamespace,
				},
			}

			arenaConfig := &omniav1alpha1.ArenaConfig{
				Status: omniav1alpha1.ArenaConfigStatus{
					ResolvedProviders: []string{}, // No providers
					ResolvedSource: &omniav1alpha1.ResolvedSource{
						URL: "http://example.com/artifact.tar.gz",
					},
				},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Queue:  memQueue,
			}

			err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaConfig, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should enqueue work items for each provider", func() {
			memQueue := queue.NewMemoryQueueWithDefaults()
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "enqueue-test-job",
					Namespace: arenaJobNamespace,
				},
			}

			arenaConfig := &omniav1alpha1.ArenaConfig{
				Status: omniav1alpha1.ArenaConfigStatus{
					ResolvedProviders: []string{"provider-1", "provider-2", "provider-3"},
					ResolvedSource: &omniav1alpha1.ResolvedSource{
						URL: "http://example.com/artifact.tar.gz",
					},
				},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Queue:  memQueue,
			}

			err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaConfig, nil)
			Expect(err).NotTo(HaveOccurred())

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
				Expect(item.BundleURL).To(Equal("http://example.com/artifact.tar.gz"))
				Expect(item.MaxAttempts).To(Equal(3))
			}
		})

		It("should handle nil resolved source gracefully", func() {
			memQueue := queue.NewMemoryQueueWithDefaults()
			arenaJob := &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nil-source-job",
					Namespace: arenaJobNamespace,
				},
			}

			arenaConfig := &omniav1alpha1.ArenaConfig{
				Status: omniav1alpha1.ArenaConfigStatus{
					ResolvedProviders: []string{"provider-1"},
					ResolvedSource:    nil, // No resolved source
				},
			}

			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				Queue:  memQueue,
			}

			err := reconciler.enqueueWorkItems(ctx, arenaJob, arenaConfig, nil)
			Expect(err).NotTo(HaveOccurred())

			// Verify item was enqueued with empty bundle URL
			item, err := memQueue.Pop(ctx, "nil-source-job")
			Expect(err).NotTo(HaveOccurred())
			Expect(item.BundleURL).To(Equal(""))
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

	Context("When finding ArenaJobs for config", func() {
		It("should return nil when object is not an ArenaConfig", func() {
			By("calling findArenaJobsForConfig with wrong object type")
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			// Pass a wrong object type (Namespace instead of ArenaConfig)
			ns := &corev1.Namespace{}
			result := reconciler.findArenaJobsForConfig(ctx, ns)
			Expect(result).To(BeNil())
		})
	})

	Context("When license validation fails", func() {
		var (
			arenaJob    *omniav1alpha1.ArenaJob
			arenaConfig *omniav1alpha1.ArenaConfig
		)

		BeforeEach(func() {
			By("creating the ArenaConfig in Ready state")
			arenaConfig = &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "license-test-config",
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
			arenaConfig.Status.ResolvedProviders = []string{"test-provider"}
			Expect(k8sClient.Status().Update(ctx, arenaConfig)).To(Succeed())

			By("creating the ArenaJob with replicas that exceed license limit")
			replicas := int32(2) // OpenCoreLicense has MaxWorkerReplicas: 1
			arenaJob = &omniav1alpha1.ArenaJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "license-fail-job",
					Namespace: arenaJobNamespace,
				},
				Spec: omniav1alpha1.ArenaJobSpec{
					ConfigRef: omniav1alpha1.LocalObjectReference{
						Name: "license-test-config",
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

			config := &omniav1alpha1.ArenaConfig{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "license-test-config",
				Namespace: arenaJobNamespace,
			}, config)
			if err == nil {
				Expect(k8sClient.Delete(ctx, config)).To(Succeed())
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
			result := reconciler.getWorkspaceForNamespace(ctx, "test-namespace")
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
			result := reconciler.getWorkspaceForNamespace(ctx, "ns-with-workspace-label")
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
			result := reconciler.getWorkspaceForNamespace(ctx, "ns-without-workspace-label")
			Expect(result).To(Equal("ns-without-workspace-label"))
		})

		It("should return namespace name when namespace does not exist", func() {
			By("calling getWorkspaceForNamespace for non-existent namespace")
			reconciler := &ArenaJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			result := reconciler.getWorkspaceForNamespace(ctx, "non-existent-namespace")
			Expect(result).To(Equal("non-existent-namespace"))
		})
	})

	Context("Provider Override Functions", func() {
		It("should resolve provider overrides using label selectors", func() {
			By("creating provider CRDs with labels")
			provider1 := &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-provider-1",
					Namespace: "default",
					Labels: map[string]string{
						"tier": "production",
						"team": "ml",
					},
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  "openai",
					Model: "gpt-4",
				},
			}
			Expect(k8sClient.Create(ctx, provider1)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, provider1)
			})

			provider2 := &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-provider-2",
					Namespace: "default",
					Labels: map[string]string{
						"tier": "staging",
						"team": "ml",
					},
				},
				Spec: omniav1alpha1.ProviderSpec{
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
			providers, err := reconciler.resolveProviderOverrides(ctx, arenaJob)
			Expect(err).NotTo(HaveOccurred())
			Expect(providers).To(HaveLen(1))
			Expect(providers[0].Name).To(Equal("test-provider-1"))
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
			providerCRDs := []*omniav1alpha1.Provider{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "openai-provider",
						Namespace: "default",
					},
					Spec: omniav1alpha1.ProviderSpec{
						Type:  "openai",
						Model: "gpt-4",
						SecretRef: &omniav1alpha1.SecretKeyRef{
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
			providerCRDs := []*omniav1alpha1.Provider{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "claude-provider",
						Namespace: "default",
					},
					Spec: omniav1alpha1.ProviderSpec{
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
			providerCRDs := []*omniav1alpha1.Provider{
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
			provider := &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared-provider",
					Namespace: "default",
					Labels: map[string]string{
						"tier": "production",
						"role": "judge",
					},
				},
				Spec: omniav1alpha1.ProviderSpec{
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

			By("resolving provider overrides - should deduplicate")
			providers, err := reconciler.resolveProviderOverrides(ctx, arenaJob)
			Expect(err).NotTo(HaveOccurred())
			Expect(providers).To(HaveLen(1)) // Should only have one provider despite matching both selectors
			Expect(providers[0].Name).To(Equal("shared-provider"))
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
			toolRegistry := &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-tool-registry",
					Namespace: "default",
					Labels: map[string]string{
						"environment": "production",
					},
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "weather-handler",
							Type: omniav1alpha1.HandlerTypeHTTP,
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "get_weather",
								Description: "Get weather data",
								InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object","properties":{"city":{"type":"string"}}}`)},
							},
							HTTPConfig: &omniav1alpha1.HTTPConfig{
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
})
