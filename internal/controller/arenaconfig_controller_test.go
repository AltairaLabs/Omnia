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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

var _ = Describe("ArenaConfig Controller", func() {
	const (
		arenaConfigName      = "test-arenaconfig"
		arenaConfigNamespace = "default"
		arenaSourceName      = "test-source"
		providerName         = "test-provider"
		toolRegistryName     = "test-toolregistry"
	)

	ctx := context.Background()

	Context("When reconciling a non-existent ArenaConfig", func() {
		It("should return without error", func() {
			By("reconciling a non-existent ArenaConfig")
			reconciler := &ArenaConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "nonexistent-config",
					Namespace: arenaConfigNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling a suspended ArenaConfig", func() {
		var arenaConfig *omniav1alpha1.ArenaConfig

		BeforeEach(func() {
			By("creating the suspended ArenaConfig")
			arenaConfig = &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "suspended-config",
					Namespace: arenaConfigNamespace,
				},
				Spec: omniav1alpha1.ArenaConfigSpec{
					SourceRef: omniav1alpha1.LocalObjectReference{
						Name: arenaSourceName,
					},
					Providers: []omniav1alpha1.NamespacedObjectReference{
						{Name: providerName},
					},
					Suspend: true,
				},
			}
			Expect(k8sClient.Create(ctx, arenaConfig)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the ArenaConfig")
			resource := &omniav1alpha1.ArenaConfig{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "suspended-config",
				Namespace: arenaConfigNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should skip validation and set condition", func() {
			By("reconciling the suspended ArenaConfig")
			reconciler := &ArenaConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "suspended-config",
					Namespace: arenaConfigNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Duration(0)))

			By("checking the updated status")
			updatedConfig := &omniav1alpha1.ArenaConfig{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "suspended-config",
				Namespace: arenaConfigNamespace,
			}, updatedConfig)).To(Succeed())

			By("checking the Ready condition")
			condition := meta.FindStatusCondition(updatedConfig.Status.Conditions, ArenaConfigConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("Suspended"))
		})
	})

	Context("When reconciling an ArenaConfig with missing ArenaSource", func() {
		var arenaConfig *omniav1alpha1.ArenaConfig

		BeforeEach(func() {
			By("creating the ArenaConfig with missing source")
			arenaConfig = &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-source-config",
					Namespace: arenaConfigNamespace,
				},
				Spec: omniav1alpha1.ArenaConfigSpec{
					SourceRef: omniav1alpha1.LocalObjectReference{
						Name: "nonexistent-source",
					},
					Providers: []omniav1alpha1.NamespacedObjectReference{
						{Name: providerName},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaConfig)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the ArenaConfig")
			resource := &omniav1alpha1.ArenaConfig{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "missing-source-config",
				Namespace: arenaConfigNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set Invalid phase and SourceResolved condition to false", func() {
			By("reconciling the ArenaConfig")
			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := &ArenaConfigReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: fakeRecorder,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "missing-source-config",
					Namespace: arenaConfigNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedConfig := &omniav1alpha1.ArenaConfig{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "missing-source-config",
				Namespace: arenaConfigNamespace,
			}, updatedConfig)).To(Succeed())

			Expect(updatedConfig.Status.Phase).To(Equal(omniav1alpha1.ArenaConfigPhaseInvalid))

			By("checking the SourceResolved condition")
			condition := meta.FindStatusCondition(updatedConfig.Status.Conditions, ArenaConfigConditionTypeSourceResolved)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("When reconciling an ArenaConfig with ready ArenaSource but no providers", func() {
		var (
			arenaConfig *omniav1alpha1.ArenaConfig
			arenaSource *omniav1alpha1.ArenaSource
		)

		BeforeEach(func() {
			By("creating the ArenaSource")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ready-source",
					Namespace: arenaConfigNamespace,
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

			// Update source status to Ready
			arenaSource.Status.Phase = omniav1alpha1.ArenaSourcePhaseReady
			arenaSource.Status.Artifact = &omniav1alpha1.Artifact{
				Revision:       "v1.0.0",
				URL:            "http://localhost:8080/artifacts/test.tar.gz",
				LastUpdateTime: metav1.Now(),
			}
			Expect(k8sClient.Status().Update(ctx, arenaSource)).To(Succeed())

			By("creating the ArenaConfig without providers")
			arenaConfig = &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-providers-config",
					Namespace: arenaConfigNamespace,
				},
				Spec: omniav1alpha1.ArenaConfigSpec{
					SourceRef: omniav1alpha1.LocalObjectReference{
						Name: "ready-source",
					},
					// No providers specified
				},
			}
			Expect(k8sClient.Create(ctx, arenaConfig)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			resource := &omniav1alpha1.ArenaConfig{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "no-providers-config",
				Namespace: arenaConfigNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			source := &omniav1alpha1.ArenaSource{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "ready-source",
				Namespace: arenaConfigNamespace,
			}, source)
			if err == nil {
				Expect(k8sClient.Delete(ctx, source)).To(Succeed())
			}
		})

		It("should set Invalid phase due to no providers", func() {
			By("reconciling the ArenaConfig")
			reconciler := &ArenaConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "no-providers-config",
					Namespace: arenaConfigNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedConfig := &omniav1alpha1.ArenaConfig{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "no-providers-config",
				Namespace: arenaConfigNamespace,
			}, updatedConfig)).To(Succeed())

			Expect(updatedConfig.Status.Phase).To(Equal(omniav1alpha1.ArenaConfigPhaseInvalid))
		})
	})

	Context("When reconciling a fully valid ArenaConfig", func() {
		var (
			arenaConfig *omniav1alpha1.ArenaConfig
			arenaSource *omniav1alpha1.ArenaSource
			provider    *omniav1alpha1.Provider
			secret      *corev1.Secret
		)

		BeforeEach(func() {
			By("creating the secret for provider")
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "provider-secret",
					Namespace: arenaConfigNamespace,
				},
				Data: map[string][]byte{
					"api-key": []byte("test-api-key"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("creating the Provider")
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerName,
					Namespace: arenaConfigNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type: omniav1alpha1.ProviderTypeClaude,
					SecretRef: &omniav1alpha1.SecretKeyRef{
						Name: "provider-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			// Update provider status to Ready
			provider.Status.Phase = omniav1alpha1.ProviderPhaseReady
			Expect(k8sClient.Status().Update(ctx, provider)).To(Succeed())

			By("creating the ArenaSource")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      arenaSourceName,
					Namespace: arenaConfigNamespace,
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

			// Update source status to Ready with artifact
			arenaSource.Status.Phase = omniav1alpha1.ArenaSourcePhaseReady
			arenaSource.Status.Artifact = &omniav1alpha1.Artifact{
				Revision:       "v1.0.0",
				URL:            "http://localhost:8080/artifacts/test.tar.gz",
				LastUpdateTime: metav1.Now(),
			}
			Expect(k8sClient.Status().Update(ctx, arenaSource)).To(Succeed())

			By("creating the ArenaConfig")
			arenaConfig = &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      arenaConfigName,
					Namespace: arenaConfigNamespace,
				},
				Spec: omniav1alpha1.ArenaConfigSpec{
					SourceRef: omniav1alpha1.LocalObjectReference{
						Name: arenaSourceName,
					},
					Providers: []omniav1alpha1.NamespacedObjectReference{
						{Name: providerName},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaConfig)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			config := &omniav1alpha1.ArenaConfig{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      arenaConfigName,
				Namespace: arenaConfigNamespace,
			}, config)
			if err == nil {
				Expect(k8sClient.Delete(ctx, config)).To(Succeed())
			}

			source := &omniav1alpha1.ArenaSource{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      arenaSourceName,
				Namespace: arenaConfigNamespace,
			}, source)
			if err == nil {
				Expect(k8sClient.Delete(ctx, source)).To(Succeed())
			}

			prov := &omniav1alpha1.Provider{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      providerName,
				Namespace: arenaConfigNamespace,
			}, prov)
			if err == nil {
				Expect(k8sClient.Delete(ctx, prov)).To(Succeed())
			}

			s := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "provider-secret",
				Namespace: arenaConfigNamespace,
			}, s)
			if err == nil {
				Expect(k8sClient.Delete(ctx, s)).To(Succeed())
			}
		})

		It("should set Ready phase and resolve all references", func() {
			By("reconciling the ArenaConfig")
			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := &ArenaConfigReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: fakeRecorder,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      arenaConfigName,
					Namespace: arenaConfigNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedConfig := &omniav1alpha1.ArenaConfig{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      arenaConfigName,
				Namespace: arenaConfigNamespace,
			}, updatedConfig)).To(Succeed())

			Expect(updatedConfig.Status.Phase).To(Equal(omniav1alpha1.ArenaConfigPhaseReady))
			Expect(updatedConfig.Status.ResolvedSource).NotTo(BeNil())
			Expect(updatedConfig.Status.ResolvedSource.Revision).To(Equal("v1.0.0"))
			Expect(updatedConfig.Status.ResolvedProviders).To(HaveLen(1))
			Expect(updatedConfig.Status.ResolvedProviders[0]).To(Equal(providerName))
			Expect(updatedConfig.Status.LastValidatedAt).NotTo(BeNil())

			By("checking the Ready condition")
			readyCondition := meta.FindStatusCondition(updatedConfig.Status.Conditions, ArenaConfigConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))

			By("checking the SourceResolved condition")
			sourceCondition := meta.FindStatusCondition(updatedConfig.Status.Conditions, ArenaConfigConditionTypeSourceResolved)
			Expect(sourceCondition).NotTo(BeNil())
			Expect(sourceCondition.Status).To(Equal(metav1.ConditionTrue))

			By("checking the ProvidersValid condition")
			providerCondition := meta.FindStatusCondition(updatedConfig.Status.Conditions, ArenaConfigConditionTypeProvidersValid)
			Expect(providerCondition).NotTo(BeNil())
			Expect(providerCondition.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("When ArenaSource is not ready", func() {
		var (
			arenaConfig *omniav1alpha1.ArenaConfig
			arenaSource *omniav1alpha1.ArenaSource
		)

		BeforeEach(func() {
			By("creating the ArenaSource in Pending state")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pending-source",
					Namespace: arenaConfigNamespace,
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

			// Set source to Pending (not Ready)
			arenaSource.Status.Phase = omniav1alpha1.ArenaSourcePhasePending
			arenaSource.Status.Artifact = &omniav1alpha1.Artifact{
				Revision:       "v1.0.0",
				URL:            "http://localhost:8080/artifacts/test.tar.gz",
				LastUpdateTime: metav1.Now(),
			}
			Expect(k8sClient.Status().Update(ctx, arenaSource)).To(Succeed())

			By("creating the ArenaConfig")
			arenaConfig = &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pending-source-config",
					Namespace: arenaConfigNamespace,
				},
				Spec: omniav1alpha1.ArenaConfigSpec{
					SourceRef: omniav1alpha1.LocalObjectReference{
						Name: "pending-source",
					},
					Providers: []omniav1alpha1.NamespacedObjectReference{
						{Name: providerName},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaConfig)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			config := &omniav1alpha1.ArenaConfig{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "pending-source-config",
				Namespace: arenaConfigNamespace,
			}, config)
			if err == nil {
				Expect(k8sClient.Delete(ctx, config)).To(Succeed())
			}

			source := &omniav1alpha1.ArenaSource{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "pending-source",
				Namespace: arenaConfigNamespace,
			}, source)
			if err == nil {
				Expect(k8sClient.Delete(ctx, source)).To(Succeed())
			}
		})

		It("should set Pending phase and wait for source", func() {
			By("reconciling the ArenaConfig")
			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := &ArenaConfigReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: fakeRecorder,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "pending-source-config",
					Namespace: arenaConfigNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedConfig := &omniav1alpha1.ArenaConfig{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "pending-source-config",
				Namespace: arenaConfigNamespace,
			}, updatedConfig)).To(Succeed())

			Expect(updatedConfig.Status.Phase).To(Equal(omniav1alpha1.ArenaConfigPhasePending))

			By("checking the SourceResolved condition")
			condition := meta.FindStatusCondition(updatedConfig.Status.Conditions, ArenaConfigConditionTypeSourceResolved)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("SourceNotReady"))
		})
	})

	Context("When Provider is not found", func() {
		var (
			arenaConfig *omniav1alpha1.ArenaConfig
			arenaSource *omniav1alpha1.ArenaSource
		)

		BeforeEach(func() {
			By("creating the ArenaSource")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-source-2",
					Namespace: arenaConfigNamespace,
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
				URL:            "http://localhost:8080/artifacts/test.tar.gz",
				LastUpdateTime: metav1.Now(),
			}
			Expect(k8sClient.Status().Update(ctx, arenaSource)).To(Succeed())

			By("creating the ArenaConfig with missing provider")
			arenaConfig = &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-provider-config",
					Namespace: arenaConfigNamespace,
				},
				Spec: omniav1alpha1.ArenaConfigSpec{
					SourceRef: omniav1alpha1.LocalObjectReference{
						Name: "valid-source-2",
					},
					Providers: []omniav1alpha1.NamespacedObjectReference{
						{Name: "nonexistent-provider"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaConfig)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			config := &omniav1alpha1.ArenaConfig{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "missing-provider-config",
				Namespace: arenaConfigNamespace,
			}, config)
			if err == nil {
				Expect(k8sClient.Delete(ctx, config)).To(Succeed())
			}

			source := &omniav1alpha1.ArenaSource{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "valid-source-2",
				Namespace: arenaConfigNamespace,
			}, source)
			if err == nil {
				Expect(k8sClient.Delete(ctx, source)).To(Succeed())
			}
		})

		It("should set Invalid phase with provider error", func() {
			By("reconciling the ArenaConfig")
			reconciler := &ArenaConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "missing-provider-config",
					Namespace: arenaConfigNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedConfig := &omniav1alpha1.ArenaConfig{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "missing-provider-config",
				Namespace: arenaConfigNamespace,
			}, updatedConfig)).To(Succeed())

			Expect(updatedConfig.Status.Phase).To(Equal(omniav1alpha1.ArenaConfigPhaseInvalid))

			By("checking the ProvidersValid condition")
			condition := meta.FindStatusCondition(updatedConfig.Status.Conditions, ArenaConfigConditionTypeProvidersValid)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("When testing setCondition helper", func() {
		It("should set a condition on the ArenaConfig", func() {
			config := &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-condition-config",
					Namespace:  arenaConfigNamespace,
					Generation: 1,
				},
			}

			reconciler := &ArenaConfigReconciler{}
			reconciler.setCondition(config, ArenaConfigConditionTypeReady, metav1.ConditionTrue, "TestReason", "Test message")

			condition := meta.FindStatusCondition(config.Status.Conditions, ArenaConfigConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal("TestReason"))
			Expect(condition.Message).To(Equal("Test message"))
			Expect(condition.ObservedGeneration).To(Equal(int64(1)))
		})
	})

	Context("When testing findArenaConfigsForSource", func() {
		var (
			arenaConfig *omniav1alpha1.ArenaConfig
			arenaSource *omniav1alpha1.ArenaSource
		)

		BeforeEach(func() {
			By("creating the ArenaSource")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "watch-test-source",
					Namespace: arenaConfigNamespace,
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

			By("creating the ArenaConfig that references the source")
			arenaConfig = &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "watch-test-config",
					Namespace: arenaConfigNamespace,
				},
				Spec: omniav1alpha1.ArenaConfigSpec{
					SourceRef: omniav1alpha1.LocalObjectReference{
						Name: "watch-test-source",
					},
					Providers: []omniav1alpha1.NamespacedObjectReference{
						{Name: providerName},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaConfig)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			config := &omniav1alpha1.ArenaConfig{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "watch-test-config",
				Namespace: arenaConfigNamespace,
			}, config)
			if err == nil {
				Expect(k8sClient.Delete(ctx, config)).To(Succeed())
			}

			source := &omniav1alpha1.ArenaSource{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "watch-test-source",
				Namespace: arenaConfigNamespace,
			}, source)
			if err == nil {
				Expect(k8sClient.Delete(ctx, source)).To(Succeed())
			}
		})

		It("should return reconcile requests for ArenaConfigs referencing the source", func() {
			By("calling findArenaConfigsForSource")
			reconciler := &ArenaConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			requests := reconciler.findArenaConfigsForSource(ctx, arenaSource)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("watch-test-config"))
			Expect(requests[0].Namespace).To(Equal(arenaConfigNamespace))
		})
	})

	Context("When testing SetupWithManager", func() {
		It("should return error with nil manager", func() {
			reconciler := &ArenaConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			err := reconciler.SetupWithManager(nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("When testing findArenaConfigsForProvider", func() {
		var (
			arenaConfig *omniav1alpha1.ArenaConfig
			provider    *omniav1alpha1.Provider
		)

		BeforeEach(func() {
			By("creating the Provider")
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "watch-provider",
					Namespace: arenaConfigNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type: omniav1alpha1.ProviderTypeClaude,
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			By("creating the ArenaConfig that references the provider")
			arenaConfig = &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "provider-watch-config",
					Namespace: arenaConfigNamespace,
				},
				Spec: omniav1alpha1.ArenaConfigSpec{
					SourceRef: omniav1alpha1.LocalObjectReference{
						Name: arenaSourceName,
					},
					Providers: []omniav1alpha1.NamespacedObjectReference{
						{Name: "watch-provider"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaConfig)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			config := &omniav1alpha1.ArenaConfig{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "provider-watch-config",
				Namespace: arenaConfigNamespace,
			}, config)
			if err == nil {
				Expect(k8sClient.Delete(ctx, config)).To(Succeed())
			}

			prov := &omniav1alpha1.Provider{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "watch-provider",
				Namespace: arenaConfigNamespace,
			}, prov)
			if err == nil {
				Expect(k8sClient.Delete(ctx, prov)).To(Succeed())
			}
		})

		It("should return reconcile requests for ArenaConfigs referencing the provider", func() {
			By("calling findArenaConfigsForProvider")
			reconciler := &ArenaConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			requests := reconciler.findArenaConfigsForProvider(ctx, provider)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("provider-watch-config"))
			Expect(requests[0].Namespace).To(Equal(arenaConfigNamespace))
		})

		It("should return nil for non-Provider objects", func() {
			reconciler := &ArenaConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			requests := reconciler.findArenaConfigsForProvider(ctx, &corev1.Secret{})
			Expect(requests).To(BeNil())
		})
	})

	Context("When reconciling an ArenaConfig with ToolRegistries", func() {
		var (
			arenaConfig  *omniav1alpha1.ArenaConfig
			arenaSource  *omniav1alpha1.ArenaSource
			provider     *omniav1alpha1.Provider
			toolRegistry *omniav1alpha1.ToolRegistry
		)

		BeforeEach(func() {
			By("creating the Provider")
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "toolreg-provider",
					Namespace: arenaConfigNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type: omniav1alpha1.ProviderTypeClaude,
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())
			provider.Status.Phase = omniav1alpha1.ProviderPhaseReady
			Expect(k8sClient.Status().Update(ctx, provider)).To(Succeed())

			By("creating the ToolRegistry")
			sseEndpoint := "http://localhost:8080/sse"
			toolRegistry = &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      toolRegistryName,
					Namespace: arenaConfigNamespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "test-handler",
							Type: omniav1alpha1.HandlerTypeMCP,
							MCPConfig: &omniav1alpha1.MCPConfig{
								Transport: omniav1alpha1.MCPTransportSSE,
								Endpoint:  &sseEndpoint,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, toolRegistry)).To(Succeed())
			toolRegistry.Status.Phase = omniav1alpha1.ToolRegistryPhaseReady
			Expect(k8sClient.Status().Update(ctx, toolRegistry)).To(Succeed())

			By("creating the ArenaSource")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "toolreg-source",
					Namespace: arenaConfigNamespace,
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
				URL:            "http://localhost:8080/artifacts/test.tar.gz",
				LastUpdateTime: metav1.Now(),
			}
			Expect(k8sClient.Status().Update(ctx, arenaSource)).To(Succeed())

			By("creating the ArenaConfig with ToolRegistry")
			arenaConfig = &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "toolreg-config",
					Namespace: arenaConfigNamespace,
				},
				Spec: omniav1alpha1.ArenaConfigSpec{
					SourceRef: omniav1alpha1.LocalObjectReference{
						Name: "toolreg-source",
					},
					Providers: []omniav1alpha1.NamespacedObjectReference{
						{Name: "toolreg-provider"},
					},
					ToolRegistries: []omniav1alpha1.NamespacedObjectReference{
						{Name: toolRegistryName},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaConfig)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			config := &omniav1alpha1.ArenaConfig{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "toolreg-config",
				Namespace: arenaConfigNamespace,
			}, config)
			if err == nil {
				Expect(k8sClient.Delete(ctx, config)).To(Succeed())
			}

			source := &omniav1alpha1.ArenaSource{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "toolreg-source",
				Namespace: arenaConfigNamespace,
			}, source)
			if err == nil {
				Expect(k8sClient.Delete(ctx, source)).To(Succeed())
			}

			prov := &omniav1alpha1.Provider{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "toolreg-provider",
				Namespace: arenaConfigNamespace,
			}, prov)
			if err == nil {
				Expect(k8sClient.Delete(ctx, prov)).To(Succeed())
			}

			registry := &omniav1alpha1.ToolRegistry{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      toolRegistryName,
				Namespace: arenaConfigNamespace,
			}, registry)
			if err == nil {
				Expect(k8sClient.Delete(ctx, registry)).To(Succeed())
			}
		})

		It("should set Ready phase with valid ToolRegistry", func() {
			By("reconciling the ArenaConfig")
			reconciler := &ArenaConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "toolreg-config",
					Namespace: arenaConfigNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedConfig := &omniav1alpha1.ArenaConfig{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "toolreg-config",
				Namespace: arenaConfigNamespace,
			}, updatedConfig)).To(Succeed())

			Expect(updatedConfig.Status.Phase).To(Equal(omniav1alpha1.ArenaConfigPhaseReady))

			By("checking the ToolRegistriesValid condition")
			condition := meta.FindStatusCondition(updatedConfig.Status.Conditions, ArenaConfigConditionTypeToolRegsValid)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("When ToolRegistry is not found", func() {
		var (
			arenaConfig *omniav1alpha1.ArenaConfig
			arenaSource *omniav1alpha1.ArenaSource
			provider    *omniav1alpha1.Provider
		)

		BeforeEach(func() {
			By("creating the Provider")
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "toolreg-missing-provider",
					Namespace: arenaConfigNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type: omniav1alpha1.ProviderTypeClaude,
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())
			provider.Status.Phase = omniav1alpha1.ProviderPhaseReady
			Expect(k8sClient.Status().Update(ctx, provider)).To(Succeed())

			By("creating the ArenaSource")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "toolreg-missing-source",
					Namespace: arenaConfigNamespace,
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
				URL:            "http://localhost:8080/artifacts/test.tar.gz",
				LastUpdateTime: metav1.Now(),
			}
			Expect(k8sClient.Status().Update(ctx, arenaSource)).To(Succeed())

			By("creating the ArenaConfig with missing ToolRegistry")
			arenaConfig = &omniav1alpha1.ArenaConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "toolreg-missing-config",
					Namespace: arenaConfigNamespace,
				},
				Spec: omniav1alpha1.ArenaConfigSpec{
					SourceRef: omniav1alpha1.LocalObjectReference{
						Name: "toolreg-missing-source",
					},
					Providers: []omniav1alpha1.NamespacedObjectReference{
						{Name: "toolreg-missing-provider"},
					},
					ToolRegistries: []omniav1alpha1.NamespacedObjectReference{
						{Name: "nonexistent-registry"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaConfig)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			config := &omniav1alpha1.ArenaConfig{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "toolreg-missing-config",
				Namespace: arenaConfigNamespace,
			}, config)
			if err == nil {
				Expect(k8sClient.Delete(ctx, config)).To(Succeed())
			}

			source := &omniav1alpha1.ArenaSource{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "toolreg-missing-source",
				Namespace: arenaConfigNamespace,
			}, source)
			if err == nil {
				Expect(k8sClient.Delete(ctx, source)).To(Succeed())
			}

			prov := &omniav1alpha1.Provider{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "toolreg-missing-provider",
				Namespace: arenaConfigNamespace,
			}, prov)
			if err == nil {
				Expect(k8sClient.Delete(ctx, prov)).To(Succeed())
			}
		})

		It("should set Invalid phase with ToolRegistry error", func() {
			By("reconciling the ArenaConfig")
			reconciler := &ArenaConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "toolreg-missing-config",
					Namespace: arenaConfigNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedConfig := &omniav1alpha1.ArenaConfig{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "toolreg-missing-config",
				Namespace: arenaConfigNamespace,
			}, updatedConfig)).To(Succeed())

			Expect(updatedConfig.Status.Phase).To(Equal(omniav1alpha1.ArenaConfigPhaseInvalid))

			By("checking the ToolRegistriesValid condition")
			condition := meta.FindStatusCondition(updatedConfig.Status.Conditions, ArenaConfigConditionTypeToolRegsValid)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("When findArenaConfigsForSource receives non-ArenaSource object", func() {
		It("should return nil", func() {
			reconciler := &ArenaConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			requests := reconciler.findArenaConfigsForSource(ctx, &corev1.Secret{})
			Expect(requests).To(BeNil())
		})
	})
})
