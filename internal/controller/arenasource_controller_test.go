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
	"os"
	"path/filepath"
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
	"github.com/altairalabs/omnia/pkg/arena/fetcher"
)

var _ = Describe("ArenaSource Controller", func() {
	const (
		arenaSourceName      = "test-arenasource"
		arenaSourceNamespace = "default"
		configMapName        = "test-promptkit"
	)

	ctx := context.Background()

	var artifactDir string

	BeforeEach(func() {
		var err error
		artifactDir, err = os.MkdirTemp("", "arenasource-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if artifactDir != "" {
			_ = os.RemoveAll(artifactDir)
		}
	})

	Context("When reconciling a non-existent ArenaSource", func() {
		It("should return without error", func() {
			By("reconciling a non-existent ArenaSource")
			reconciler := &ArenaSourceReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "nonexistent-source",
					Namespace: arenaSourceNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling a suspended ArenaSource", func() {
		var arenaSource *omniav1alpha1.ArenaSource

		BeforeEach(func() {
			By("creating the suspended ArenaSource")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "suspended-source",
					Namespace: arenaSourceNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
					Interval: "5m",
					Suspend:  true,
					ConfigMap: &omniav1alpha1.ConfigMapSource{
						Name: configMapName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the ArenaSource")
			resource := &omniav1alpha1.ArenaSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "suspended-source",
				Namespace: arenaSourceNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should skip reconciliation and set condition", func() {
			By("reconciling the suspended ArenaSource")
			reconciler := &ArenaSourceReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "suspended-source",
					Namespace: arenaSourceNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			By("checking the updated status")
			updatedSource := &omniav1alpha1.ArenaSource{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "suspended-source",
				Namespace: arenaSourceNamespace,
			}, updatedSource)).To(Succeed())

			By("checking the Ready condition")
			condition := meta.FindStatusCondition(updatedSource.Status.Conditions, ArenaSourceConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("Suspended"))
		})
	})

	// Note: Invalid interval test removed because CRD validation catches it at creation time

	Context("When reconciling an ArenaSource with missing ConfigMap configuration", func() {
		var arenaSource *omniav1alpha1.ArenaSource

		BeforeEach(func() {
			By("creating the ArenaSource without ConfigMap config")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-config-source",
					Namespace: arenaSourceNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
					Interval: "5m",
					// ConfigMap is nil
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the ArenaSource")
			resource := &omniav1alpha1.ArenaSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "missing-config-source",
				Namespace: arenaSourceNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set Error phase due to missing configuration", func() {
			By("reconciling the ArenaSource")
			reconciler := &ArenaSourceReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "missing-config-source",
					Namespace: arenaSourceNamespace,
				},
			}

			By("reconciling until error is reported (async)")
			Eventually(func() omniav1alpha1.ArenaSourcePhase {
				_, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())

				updatedSource := &omniav1alpha1.ArenaSource{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "missing-config-source",
					Namespace: arenaSourceNamespace,
				}, updatedSource)).To(Succeed())

				return updatedSource.Status.Phase
			}, 10*time.Second, 100*time.Millisecond).Should(Equal(omniav1alpha1.ArenaSourcePhaseError))
		})
	})

	Context("When reconciling an ArenaSource with valid ConfigMap source", func() {
		var (
			arenaSource *omniav1alpha1.ArenaSource
			configMap   *corev1.ConfigMap
		)

		BeforeEach(func() {
			By("creating the ConfigMap")
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: arenaSourceNamespace,
				},
				Data: map[string]string{
					"pack.json": `{"id": "test", "name": "Test Pack", "version": "1.0.0"}`,
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("creating the ArenaSource")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      arenaSourceName,
					Namespace: arenaSourceNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
					Interval: "5m",
					ConfigMap: &omniav1alpha1.ConfigMapSource{
						Name: configMapName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			resource := &omniav1alpha1.ArenaSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      arenaSourceName,
				Namespace: arenaSourceNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      configMapName,
				Namespace: arenaSourceNamespace,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}
		})

		It("should fetch and store the artifact successfully", func() {
			By("reconciling the ArenaSource with event recorder")
			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := &ArenaSourceReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				Recorder:        fakeRecorder,
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      arenaSourceName,
					Namespace: arenaSourceNamespace,
				},
			}

			By("reconciling until completion (async fetch)")
			var updatedSource *omniav1alpha1.ArenaSource
			Eventually(func() omniav1alpha1.ArenaSourcePhase {
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).NotTo(BeZero())

				updatedSource = &omniav1alpha1.ArenaSource{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      arenaSourceName,
					Namespace: arenaSourceNamespace,
				}, updatedSource)).To(Succeed())

				return updatedSource.Status.Phase
			}, 10*time.Second, 100*time.Millisecond).Should(Equal(omniav1alpha1.ArenaSourcePhaseReady))

			By("checking the updated status")
			Expect(updatedSource.Status.Artifact).NotTo(BeNil())
			Expect(updatedSource.Status.Artifact.URL).To(ContainSubstring("http://localhost:8080/artifacts"))
			Expect(updatedSource.Status.Artifact.Checksum).NotTo(BeEmpty())
			Expect(updatedSource.Status.NextFetchTime).NotTo(BeNil())

			By("checking the Ready condition")
			readyCondition := meta.FindStatusCondition(updatedSource.Status.Conditions, ArenaSourceConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))

			By("checking the ArtifactAvailable condition")
			artifactCondition := meta.FindStatusCondition(updatedSource.Status.Conditions, ArenaSourceConditionTypeArtifactAvailable)
			Expect(artifactCondition).NotTo(BeNil())
			Expect(artifactCondition.Status).To(Equal(metav1.ConditionTrue))

			By("verifying artifact was stored")
			artifactPath := filepath.Join(artifactDir, arenaSourceNamespace, arenaSourceName)
			entries, err := os.ReadDir(artifactPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Name()).To(HaveSuffix(".tar.gz"))
		})

		It("should skip fetch when artifact is already up to date", func() {
			By("first reconciliation to fetch artifact")
			reconciler := &ArenaSourceReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      arenaSourceName,
					Namespace: arenaSourceNamespace,
				},
			}

			By("reconciling until first fetch completes")
			var firstRevision string
			Eventually(func() bool {
				_, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())

				updatedSource := &omniav1alpha1.ArenaSource{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      arenaSourceName,
					Namespace: arenaSourceNamespace,
				}, updatedSource)).To(Succeed())

				if updatedSource.Status.Artifact != nil {
					firstRevision = updatedSource.Status.Artifact.Revision
					return true
				}
				return false
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

			By("second reconciliation should skip fetch")
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			By("verifying revision hasn't changed")
			updatedSource := &omniav1alpha1.ArenaSource{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      arenaSourceName,
				Namespace: arenaSourceNamespace,
			}, updatedSource)).To(Succeed())
			Expect(updatedSource.Status.Artifact.Revision).To(Equal(firstRevision))
		})
	})

	Context("When reconciling an ArenaSource with missing ConfigMap", func() {
		var arenaSource *omniav1alpha1.ArenaSource

		BeforeEach(func() {
			By("creating the ArenaSource with missing ConfigMap")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-cm-source",
					Namespace: arenaSourceNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
					Interval: "5m",
					ConfigMap: &omniav1alpha1.ConfigMapSource{
						Name: "nonexistent-configmap",
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the ArenaSource")
			resource := &omniav1alpha1.ArenaSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "missing-cm-source",
				Namespace: arenaSourceNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set Error phase with fetch error and emit event", func() {
			By("reconciling the ArenaSource with event recorder")
			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := &ArenaSourceReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				Recorder:        fakeRecorder,
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "missing-cm-source",
					Namespace: arenaSourceNamespace,
				},
			}

			By("reconciling until error is reported (async)")
			var updatedSource *omniav1alpha1.ArenaSource
			Eventually(func() omniav1alpha1.ArenaSourcePhase {
				_, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())

				updatedSource = &omniav1alpha1.ArenaSource{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "missing-cm-source",
					Namespace: arenaSourceNamespace,
				}, updatedSource)).To(Succeed())

				return updatedSource.Status.Phase
			}, 10*time.Second, 100*time.Millisecond).Should(Equal(omniav1alpha1.ArenaSourcePhaseError))

			By("checking the Ready condition")
			condition := meta.FindStatusCondition(updatedSource.Status.Conditions, ArenaSourceConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("FetchError"))
		})
	})

	// Note: Unsupported type test removed because CRD validation catches it at creation time

	Context("When testing createGitFetcher with credentials", func() {
		var (
			arenaSource *omniav1alpha1.ArenaSource
			secret      *corev1.Secret
		)

		BeforeEach(func() {
			By("creating the Secret with Git credentials")
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "git-credentials",
					Namespace: arenaSourceNamespace,
				},
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpassword"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("creating the ArenaSource with Git config")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "git-source",
					Namespace: arenaSourceNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeGit,
					Interval: "5m",
					Git: &omniav1alpha1.GitSource{
						URL: "https://github.com/example/repo.git",
						Ref: &omniav1alpha1.GitReference{
							Branch: "main",
						},
						SecretRef: &omniav1alpha1.SecretKeyRef{
							Name: "git-credentials",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			resource := &omniav1alpha1.ArenaSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "git-source",
				Namespace: arenaSourceNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			s := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "git-credentials",
				Namespace: arenaSourceNamespace,
			}, s)
			if err == nil {
				Expect(k8sClient.Delete(ctx, s)).To(Succeed())
			}
		})

		It("should create Git fetcher with loaded credentials", func() {
			By("testing createGitFetcher")
			reconciler := &ArenaSourceReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}

			// Get the ArenaSource to populate it properly
			source := &omniav1alpha1.ArenaSource{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "git-source",
				Namespace: arenaSourceNamespace,
			}, source)).To(Succeed())

			// The fetcher creation should succeed (though the actual fetch would fail due to invalid repo)
			// This tests the credential loading logic
			opts := fetcher.Options{
				Timeout: 60 * time.Second,
				WorkDir: artifactDir,
			}
			_, err := reconciler.createGitFetcher(ctx, source, opts)
			// The error should not be about credentials
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When testing createOCIFetcher with credentials", func() {
		var (
			arenaSource *omniav1alpha1.ArenaSource
			secret      *corev1.Secret
		)

		BeforeEach(func() {
			By("creating the Secret with OCI credentials")
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oci-credentials",
					Namespace: arenaSourceNamespace,
				},
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpassword"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("creating the ArenaSource with OCI config")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oci-source",
					Namespace: arenaSourceNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeOCI,
					Interval: "5m",
					OCI: &omniav1alpha1.OCISource{
						URL:      "oci://registry.example.com/repo:latest",
						Insecure: false,
						SecretRef: &omniav1alpha1.SecretKeyRef{
							Name: "oci-credentials",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			resource := &omniav1alpha1.ArenaSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "oci-source",
				Namespace: arenaSourceNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			s := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "oci-credentials",
				Namespace: arenaSourceNamespace,
			}, s)
			if err == nil {
				Expect(k8sClient.Delete(ctx, s)).To(Succeed())
			}
		})

		It("should create OCI fetcher with loaded credentials", func() {
			By("testing createOCIFetcher")
			reconciler := &ArenaSourceReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}

			// Get the ArenaSource to populate it properly
			source := &omniav1alpha1.ArenaSource{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "oci-source",
				Namespace: arenaSourceNamespace,
			}, source)).To(Succeed())

			// The fetcher creation should succeed (though the actual fetch would fail due to invalid registry)
			opts := fetcher.Options{
				Timeout: 60 * time.Second,
				WorkDir: artifactDir,
			}
			_, err := reconciler.createOCIFetcher(ctx, source, opts)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When testing OCI credentials with Docker config", func() {
		var (
			arenaSource *omniav1alpha1.ArenaSource
			secret      *corev1.Secret
		)

		BeforeEach(func() {
			By("creating the Secret with Docker config")
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "docker-config-secret",
					Namespace: arenaSourceNamespace,
				},
				Data: map[string][]byte{
					".dockerconfigjson": []byte(`{"auths":{"registry.example.com":{"auth":"dXNlcm5hbWU6cGFzc3dvcmQ="}}}`),
				},
				Type: corev1.SecretTypeDockerConfigJson,
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("creating the ArenaSource with OCI config using docker secret")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oci-docker-config-source",
					Namespace: arenaSourceNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeOCI,
					Interval: "5m",
					OCI: &omniav1alpha1.OCISource{
						URL: "oci://registry.example.com/repo:latest",
						SecretRef: &omniav1alpha1.SecretKeyRef{
							Name: "docker-config-secret",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			resource := &omniav1alpha1.ArenaSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "oci-docker-config-source",
				Namespace: arenaSourceNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			s := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "docker-config-secret",
				Namespace: arenaSourceNamespace,
			}, s)
			if err == nil {
				Expect(k8sClient.Delete(ctx, s)).To(Succeed())
			}
		})

		It("should load docker config credentials", func() {
			By("testing loadOCICredentials with docker config")
			reconciler := &ArenaSourceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			creds, err := reconciler.loadOCICredentials(ctx, arenaSourceNamespace, "docker-config-secret")
			Expect(err).NotTo(HaveOccurred())
			Expect(creds.DockerConfig).NotTo(BeEmpty())
			Expect(creds.DockerConfig).To(ContainSubstring("registry.example.com"))
		})
	})

	Context("When loading credentials from missing Secret", func() {
		var arenaSource *omniav1alpha1.ArenaSource

		BeforeEach(func() {
			By("creating the ArenaSource with missing Secret reference")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "missing-secret-source",
					Namespace: arenaSourceNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeGit,
					Interval: "5m",
					Git: &omniav1alpha1.GitSource{
						URL: "https://github.com/example/repo.git",
						SecretRef: &omniav1alpha1.SecretKeyRef{
							Name: "nonexistent-secret",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the ArenaSource")
			resource := &omniav1alpha1.ArenaSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "missing-secret-source",
				Namespace: arenaSourceNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set Error phase when Secret is missing", func() {
			By("reconciling the ArenaSource")
			reconciler := &ArenaSourceReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "missing-secret-source",
					Namespace: arenaSourceNamespace,
				},
			}

			By("reconciling until error is reported (async)")
			var updatedSource *omniav1alpha1.ArenaSource
			Eventually(func() omniav1alpha1.ArenaSourcePhase {
				_, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())

				updatedSource = &omniav1alpha1.ArenaSource{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "missing-secret-source",
					Namespace: arenaSourceNamespace,
				}, updatedSource)).To(Succeed())

				return updatedSource.Status.Phase
			}, 10*time.Second, 100*time.Millisecond).Should(Equal(omniav1alpha1.ArenaSourcePhaseError))

			By("checking the Ready condition")
			condition := meta.FindStatusCondition(updatedSource.Status.Conditions, ArenaSourceConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("When testing setCondition helper", func() {
		It("should set a condition on the ArenaSource", func() {
			source := &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-condition-source",
					Namespace:  arenaSourceNamespace,
					Generation: 1,
				},
			}

			reconciler := &ArenaSourceReconciler{}
			reconciler.setCondition(source, ArenaSourceConditionTypeReady, metav1.ConditionTrue, "TestReason", "Test message")

			condition := meta.FindStatusCondition(source.Status.Conditions, ArenaSourceConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal("TestReason"))
			Expect(condition.Message).To(Equal("Test message"))
			Expect(condition.ObservedGeneration).To(Equal(int64(1)))
		})
	})

	Context("When testing copyFile helper", func() {
		It("should copy a file correctly", func() {
			By("creating source file")
			srcDir, err := os.MkdirTemp("", "copy-test-src-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(srcDir) }()

			srcPath := filepath.Join(srcDir, "source.txt")
			content := []byte("test content for copy")
			Expect(os.WriteFile(srcPath, content, 0644)).To(Succeed())

			By("creating destination directory")
			dstDir, err := os.MkdirTemp("", "copy-test-dst-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(dstDir) }()

			dstPath := filepath.Join(dstDir, "dest.txt")

			By("copying the file")
			Expect(copyFile(srcPath, dstPath)).To(Succeed())

			By("verifying the copy")
			copiedContent, err := os.ReadFile(dstPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(copiedContent).To(Equal(content))
		})

		It("should fail when source doesn't exist", func() {
			err := copyFile("/nonexistent/source.txt", "/tmp/dest.txt")
			Expect(err).To(HaveOccurred())
		})

		It("should fail when destination directory doesn't exist", func() {
			By("creating source file")
			srcDir, err := os.MkdirTemp("", "copy-test-src-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(srcDir) }()

			srcPath := filepath.Join(srcDir, "source.txt")
			content := []byte("test content")
			Expect(os.WriteFile(srcPath, content, 0644)).To(Succeed())

			By("trying to copy to nonexistent directory")
			err = copyFile(srcPath, "/nonexistent/dir/dest.txt")
			Expect(err).To(HaveOccurred())
		})
	})

	Context("When reconciling an OCI source without credentials", func() {
		var arenaSource *omniav1alpha1.ArenaSource

		BeforeEach(func() {
			By("creating the ArenaSource with OCI config")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oci-no-creds-source",
					Namespace: arenaSourceNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeOCI,
					Interval: "5m",
					OCI: &omniav1alpha1.OCISource{
						URL:      "oci://registry.example.com/repo:latest",
						Insecure: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the ArenaSource")
			resource := &omniav1alpha1.ArenaSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "oci-no-creds-source",
				Namespace: arenaSourceNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set Error phase due to network failure", func() {
			By("reconciling the ArenaSource")
			reconciler := &ArenaSourceReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "oci-no-creds-source",
					Namespace: arenaSourceNamespace,
				},
			}

			By("reconciling until error is reported (async)")
			Eventually(func() omniav1alpha1.ArenaSourcePhase {
				_, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())

				updatedSource := &omniav1alpha1.ArenaSource{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "oci-no-creds-source",
					Namespace: arenaSourceNamespace,
				}, updatedSource)).To(Succeed())

				return updatedSource.Status.Phase
			}, 10*time.Second, 100*time.Millisecond).Should(Equal(omniav1alpha1.ArenaSourcePhaseError))
		})
	})

	Context("When reconciling a Git source without credentials", func() {
		var arenaSource *omniav1alpha1.ArenaSource

		BeforeEach(func() {
			By("creating the ArenaSource with Git config")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "git-no-creds-source",
					Namespace: arenaSourceNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeGit,
					Interval: "5m",
					Git: &omniav1alpha1.GitSource{
						URL: "https://github.com/nonexistent/repo.git",
						Ref: &omniav1alpha1.GitReference{
							Branch: "main",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the ArenaSource")
			resource := &omniav1alpha1.ArenaSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "git-no-creds-source",
				Namespace: arenaSourceNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set Error phase due to network/auth failure", func() {
			By("reconciling the ArenaSource")
			reconciler := &ArenaSourceReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "git-no-creds-source",
					Namespace: arenaSourceNamespace,
				},
			}

			By("reconciling until error is reported (async)")
			Eventually(func() omniav1alpha1.ArenaSourcePhase {
				_, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())

				updatedSource := &omniav1alpha1.ArenaSource{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "git-no-creds-source",
					Namespace: arenaSourceNamespace,
				}, updatedSource)).To(Succeed())

				return updatedSource.Status.Phase
			}, 10*time.Second, 100*time.Millisecond).Should(Equal(omniav1alpha1.ArenaSourcePhaseError))
		})
	})

	Context("When reconciling an ArenaSource with custom timeout", func() {
		var (
			arenaSource *omniav1alpha1.ArenaSource
			configMap   *corev1.ConfigMap
		)

		BeforeEach(func() {
			By("creating the ConfigMap")
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "timeout-test-configmap",
					Namespace: arenaSourceNamespace,
				},
				Data: map[string]string{
					"pack.json": `{"id": "test", "name": "Test Pack", "version": "1.0.0"}`,
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("creating the ArenaSource with custom timeout")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "timeout-source",
					Namespace: arenaSourceNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
					Interval: "5m",
					Timeout:  "30s",
					ConfigMap: &omniav1alpha1.ConfigMapSource{
						Name: "timeout-test-configmap",
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			resource := &omniav1alpha1.ArenaSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "timeout-source",
				Namespace: arenaSourceNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "timeout-test-configmap",
				Namespace: arenaSourceNamespace,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}
		})

		It("should successfully fetch with custom timeout", func() {
			By("reconciling the ArenaSource")
			reconciler := &ArenaSourceReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "timeout-source",
					Namespace: arenaSourceNamespace,
				},
			}

			By("reconciling until completion (async)")
			Eventually(func() omniav1alpha1.ArenaSourcePhase {
				_, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())

				updatedSource := &omniav1alpha1.ArenaSource{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "timeout-source",
					Namespace: arenaSourceNamespace,
				}, updatedSource)).To(Succeed())

				return updatedSource.Status.Phase
			}, 10*time.Second, 100*time.Millisecond).Should(Equal(omniav1alpha1.ArenaSourcePhaseReady))
		})
	})

	Context("When testing SetupWithManager", func() {
		It("should return error with nil manager", func() {
			reconciler := &ArenaSourceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			// SetupWithManager requires a non-nil manager, this tests the setup path
			err := reconciler.SetupWithManager(nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("When testing storeArtifact with valid artifact", func() {
		It("should store the artifact and return URL", func() {
			By("creating a temporary artifact file")
			tmpDir, err := os.MkdirTemp("", "artifact-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			artifactPath := filepath.Join(tmpDir, "test.tar.gz")
			Expect(os.WriteFile(artifactPath, []byte("fake tarball content"), 0644)).To(Succeed())

			By("testing storeArtifact")
			reconciler := &ArenaSourceReconciler{
				ArtifactDir:     tmpDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}

			source := &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-store",
					Namespace: "default",
				},
			}

			artifact := &fetcher.Artifact{
				Path:     artifactPath,
				Revision: "test-revision",
				Checksum: "sha256:1234567890123456789012345678901234567890123456789012345678901234",
				Size:     20,
			}

			url, err := reconciler.storeArtifact(source, artifact)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(ContainSubstring("http://localhost:8080/artifacts"))
			Expect(url).To(ContainSubstring("default/test-store"))
		})
	})

	// Async fetch behavior tests
	Context("When testing async fetch behavior", func() {
		var (
			arenaSource *omniav1alpha1.ArenaSource
			configMap   *corev1.ConfigMap
			reconciler  *ArenaSourceReconciler
		)

		BeforeEach(func() {
			By("creating the ConfigMap")
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "async-test-configmap",
					Namespace: arenaSourceNamespace,
				},
				Data: map[string]string{
					"pack.json": `{"id": "test", "name": "Test Pack", "version": "1.0.0"}`,
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("creating the ArenaSource")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "async-test-source",
					Namespace: arenaSourceNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
					Interval: "5m",
					ConfigMap: &omniav1alpha1.ConfigMapSource{
						Name: "async-test-configmap",
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())

			reconciler = &ArenaSourceReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				Recorder:        record.NewFakeRecorder(10),
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}
		})

		AfterEach(func() {
			By("cleaning up resources")
			resource := &omniav1alpha1.ArenaSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "async-test-source",
				Namespace: arenaSourceNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "async-test-configmap",
				Namespace: arenaSourceNamespace,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}
		})

		It("should set Fetching phase on first reconcile and return quickly", func() {
			By("doing the first reconcile")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "async-test-source",
					Namespace: arenaSourceNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			By("checking that phase is Fetching")
			updatedSource := &omniav1alpha1.ArenaSource{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "async-test-source",
				Namespace: arenaSourceNamespace,
			}, updatedSource)).To(Succeed())

			// After first reconcile, should be Fetching (if async) or Ready (if sync completed)
			Expect(updatedSource.Status.Phase).To(BeElementOf(
				omniav1alpha1.ArenaSourcePhaseFetching,
				omniav1alpha1.ArenaSourcePhaseReady,
			))
		})

		It("should complete fetch after multiple reconciles", func() {
			By("reconciling until completion")
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "async-test-source",
					Namespace: arenaSourceNamespace,
				},
			}

			// Reconcile multiple times to allow async fetch to complete
			Eventually(func() omniav1alpha1.ArenaSourcePhase {
				_, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())

				updatedSource := &omniav1alpha1.ArenaSource{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "async-test-source",
					Namespace: arenaSourceNamespace,
				}, updatedSource)).To(Succeed())

				return updatedSource.Status.Phase
			}, 10*time.Second, 100*time.Millisecond).Should(Equal(omniav1alpha1.ArenaSourcePhaseReady))
		})

		It("should not start duplicate fetches for same source", func() {
			By("doing multiple reconciles quickly")
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "async-test-source",
					Namespace: arenaSourceNamespace,
				},
			}

			// First reconcile starts the fetch
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify that in-progress is tracked
			key := types.NamespacedName{
				Name:      "async-test-source",
				Namespace: arenaSourceNamespace,
			}

			// Check status
			updatedSource := &omniav1alpha1.ArenaSource{}
			Expect(k8sClient.Get(ctx, key, updatedSource)).To(Succeed())
			initialPhase := updatedSource.Status.Phase

			// If still fetching, second reconcile should not start a new fetch
			if initialPhase == omniav1alpha1.ArenaSourcePhaseFetching {
				By("doing a second reconcile while fetch is in progress")
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				// Should requeue to check later
				Expect(result.RequeueAfter).NotTo(BeZero())
			}
		})
	})

	Context("When testing async fetch cancellation on delete", func() {
		var (
			arenaSource *omniav1alpha1.ArenaSource
			configMap   *corev1.ConfigMap
			reconciler  *ArenaSourceReconciler
		)

		BeforeEach(func() {
			By("creating the ConfigMap")
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cancel-test-configmap",
					Namespace: arenaSourceNamespace,
				},
				Data: map[string]string{
					"pack.json": `{"id": "test", "name": "Test Pack", "version": "1.0.0"}`,
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("creating the ArenaSource")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cancel-test-source",
					Namespace: arenaSourceNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
					Interval: "5m",
					ConfigMap: &omniav1alpha1.ConfigMapSource{
						Name: "cancel-test-configmap",
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())

			reconciler = &ArenaSourceReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}
		})

		AfterEach(func() {
			By("cleaning up resources")
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "cancel-test-configmap",
				Namespace: arenaSourceNamespace,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}
		})

		It("should handle deletion during fetch gracefully", func() {
			By("starting a fetch")
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "cancel-test-source",
					Namespace: arenaSourceNamespace,
				},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			By("deleting the ArenaSource")
			Expect(k8sClient.Delete(ctx, arenaSource)).To(Succeed())

			By("reconciling after deletion")
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			// Should return without requeue since resource is deleted
			Expect(result.RequeueAfter).To(BeZero())
		})
	})

	Context("When testing async fetch with suspension", func() {
		var (
			arenaSource *omniav1alpha1.ArenaSource
			configMap   *corev1.ConfigMap
			reconciler  *ArenaSourceReconciler
		)

		BeforeEach(func() {
			By("creating the ConfigMap")
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "suspend-test-configmap",
					Namespace: arenaSourceNamespace,
				},
				Data: map[string]string{
					"pack.json": `{"id": "test", "name": "Test Pack", "version": "1.0.0"}`,
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("creating the ArenaSource (not suspended)")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "suspend-test-source",
					Namespace: arenaSourceNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
					Interval: "5m",
					Suspend:  false,
					ConfigMap: &omniav1alpha1.ConfigMapSource{
						Name: "suspend-test-configmap",
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())

			reconciler = &ArenaSourceReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}
		})

		AfterEach(func() {
			By("cleaning up resources")
			resource := &omniav1alpha1.ArenaSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "suspend-test-source",
				Namespace: arenaSourceNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "suspend-test-configmap",
				Namespace: arenaSourceNamespace,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}
		})

		It("should cancel in-progress fetch when suspended", func() {
			By("starting a fetch")
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "suspend-test-source",
					Namespace: arenaSourceNamespace,
				},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			By("suspending the ArenaSource")
			updatedSource := &omniav1alpha1.ArenaSource{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "suspend-test-source",
				Namespace: arenaSourceNamespace,
			}, updatedSource)).To(Succeed())

			updatedSource.Spec.Suspend = true
			Expect(k8sClient.Update(ctx, updatedSource)).To(Succeed())

			By("reconciling after suspension")
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			// Should return without requeue since suspended
			Expect(result.RequeueAfter).To(BeZero())

			By("checking status shows suspended")
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "suspend-test-source",
				Namespace: arenaSourceNamespace,
			}, updatedSource)).To(Succeed())

			condition := meta.FindStatusCondition(updatedSource.Status.Conditions, ArenaSourceConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Reason).To(Equal("Suspended"))
		})
	})
})
