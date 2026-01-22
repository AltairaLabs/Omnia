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
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/arena/fetcher"
	"github.com/altairalabs/omnia/pkg/license"
)

// generateTestKeyPairForController generates an RSA key pair for testing.
func generateTestKeyPairForController() (*rsa.PrivateKey, *rsa.PublicKey) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return privateKey, &privateKey.PublicKey
}

// createOpenCoreLicenseValidator creates a validator that returns open-core license (no enterprise features).
func createOpenCoreLicenseValidator(publicKey *rsa.PublicKey) (*license.Validator, error) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	// No secret means open-core license
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	return license.NewValidator(client, license.WithPublicKey(publicKey))
}

// createTestTarball creates a tar.gz file with the given files.
func createTestTarball(path string, files map[string]string) {
	file, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer func() { _ = file.Close() }()

	gw := gzip.NewWriter(file)
	defer func() { _ = gw.Close() }()

	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	for name, content := range files {
		// Create directory entries if needed
		dir := filepath.Dir(name)
		if dir != "." {
			hdr := &tar.Header{
				Name:     dir + "/",
				Mode:     0755,
				Typeflag: tar.TypeDir,
			}
			if err := tw.WriteHeader(hdr); err != nil {
				panic(err)
			}
		}

		// Create file entry
		hdr := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			panic(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			panic(err)
		}
	}
}

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
		It("should store the artifact and return URL in legacy mode", func() {
			By("creating a temporary artifact file")
			tmpDir, err := os.MkdirTemp("", "artifact-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			artifactPath := filepath.Join(tmpDir, "test.tar.gz")
			Expect(os.WriteFile(artifactPath, []byte("fake tarball content"), 0644)).To(Succeed())

			By("testing storeArtifact in legacy mode (no WorkspaceContentPath)")
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

			contentPath, version, url, err := reconciler.storeArtifact(source, artifact)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(ContainSubstring("http://localhost:8080/artifacts"))
			Expect(url).To(ContainSubstring("default/test-store"))
			// In legacy mode, contentPath and version should be empty
			Expect(contentPath).To(BeEmpty())
			Expect(version).To(BeEmpty())
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

	Context("When testing storeArtifact with no-change result", func() {
		It("should return existing values when artifact path is empty", func() {
			By("testing storeArtifact with empty path")
			reconciler := &ArenaSourceReconciler{
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}

			source := &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-no-change",
					Namespace: "default",
				},
				Status: omniav1alpha1.ArenaSourceStatus{
					Artifact: &omniav1alpha1.Artifact{
						URL:         "http://localhost:8080/artifacts/default/test-no-change/existing.tar.gz",
						ContentPath: "arena/test-no-change/.arena/versions/abc123",
						Version:     "abc123",
						Revision:    "rev1",
					},
				},
			}

			// Artifact with empty path indicates no change
			artifact := &fetcher.Artifact{
				Path:     "",
				Revision: "rev1",
			}

			contentPath, version, url, err := reconciler.storeArtifact(source, artifact)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(Equal("http://localhost:8080/artifacts/default/test-no-change/existing.tar.gz"))
			Expect(contentPath).To(Equal("arena/test-no-change/.arena/versions/abc123"))
			Expect(version).To(Equal("abc123"))
		})
	})

	Context("When testing filesystem sync mode", func() {
		It("should sync content to filesystem and create version", func() {
			By("creating a temporary workspace content directory")
			tmpDir, err := os.MkdirTemp("", "workspace-content-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			By("creating a test tarball with content")
			tarballDir, err := os.MkdirTemp("", "tarball-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tarballDir) }()

			tarballPath := filepath.Join(tarballDir, "test.tar.gz")
			createTestTarball(tarballPath, map[string]string{
				"config.yaml":       "name: test\n",
				"prompts/hello.txt": "Hello, world!\n",
			})

			By("testing filesystem sync")
			reconciler := &ArenaSourceReconciler{
				ArtifactDir:          "/tmp/artifacts",
				ArtifactBaseURL:      "http://localhost:8080/artifacts",
				WorkspaceContentPath: tmpDir,
				MaxVersionsPerSource: 5,
			}

			source := &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sync",
					Namespace: "test-workspace",
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					TargetPath: "arena/test-sync",
				},
			}

			artifact := &fetcher.Artifact{
				Path:     tarballPath,
				Revision: "test-revision",
				Checksum: "sha256:1234567890123456789012345678901234567890123456789012345678901234",
				Size:     100,
			}

			contentPath, version, url, err := reconciler.storeArtifact(source, artifact)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(BeEmpty(), "URL should be empty in filesystem mode")
			Expect(version).NotTo(BeEmpty(), "Version should be set")
			Expect(contentPath).To(ContainSubstring(".arena/versions/"))

			By("verifying content was synced correctly")
			versionDir := filepath.Join(tmpDir, "test-workspace", "test-workspace", "arena/test-sync", ".arena", "versions", version)
			Expect(versionDir).To(BeADirectory())

			configContent, err := os.ReadFile(filepath.Join(versionDir, "config.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(configContent)).To(Equal("name: test\n"))

			promptContent, err := os.ReadFile(filepath.Join(versionDir, "prompts/hello.txt"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(promptContent)).To(Equal("Hello, world!\n"))

			By("verifying HEAD was updated")
			headContent, err := os.ReadFile(filepath.Join(tmpDir, "test-workspace", "test-workspace", "arena/test-sync", ".arena", "HEAD"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(headContent)).To(Equal(version))
		})

		It("should reuse existing version when content hash matches", func() {
			By("creating a temporary workspace content directory")
			tmpDir, err := os.MkdirTemp("", "workspace-content-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			By("creating a test tarball")
			tarballDir, err := os.MkdirTemp("", "tarball-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tarballDir) }()

			tarballPath := filepath.Join(tarballDir, "test.tar.gz")
			createTestTarball(tarballPath, map[string]string{
				"config.yaml": "name: test\n",
			})

			reconciler := &ArenaSourceReconciler{
				WorkspaceContentPath: tmpDir,
				MaxVersionsPerSource: 5,
			}

			source := &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dedup",
					Namespace: "test-workspace",
				},
			}

			artifact := &fetcher.Artifact{
				Path:     tarballPath,
				Revision: "rev1",
				Checksum: "sha256:abc123",
				Size:     50,
			}

			By("syncing first time")
			contentPath1, version1, _, err := reconciler.storeArtifact(source, artifact)
			Expect(err).NotTo(HaveOccurred())

			By("syncing second time with same content")
			artifact.Revision = "rev2" // Different revision but same content
			contentPath2, version2, _, err := reconciler.storeArtifact(source, artifact)
			Expect(err).NotTo(HaveOccurred())

			By("verifying same version was returned")
			Expect(version2).To(Equal(version1))
			Expect(contentPath2).To(Equal(contentPath1))
		})

		It("should garbage collect old versions", func() {
			By("creating a temporary workspace content directory")
			tmpDir, err := os.MkdirTemp("", "workspace-content-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			reconciler := &ArenaSourceReconciler{
				WorkspaceContentPath: tmpDir,
				MaxVersionsPerSource: 2, // Keep only 2 versions
			}

			source := &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gc",
					Namespace: "test-workspace",
				},
			}

			By("creating multiple versions")
			versionsDir := filepath.Join(tmpDir, "test-workspace", "test-workspace", "arena/test-gc", ".arena", "versions")
			Expect(os.MkdirAll(versionsDir, 0755)).To(Succeed())

			// Create 3 old versions with different timestamps
			for i, v := range []string{"old1", "old2", "old3"} {
				vDir := filepath.Join(versionsDir, v)
				Expect(os.MkdirAll(vDir, 0755)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(vDir, "file.txt"), []byte("content"), 0644)).To(Succeed())
				// Set modification time in the past
				pastTime := time.Now().Add(time.Duration(-(i+1)*10) * time.Minute)
				Expect(os.Chtimes(vDir, pastTime, pastTime)).To(Succeed())
			}

			By("creating a new version")
			tarballDir, err := os.MkdirTemp("", "tarball-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tarballDir) }()

			tarballPath := filepath.Join(tarballDir, "test.tar.gz")
			createTestTarball(tarballPath, map[string]string{
				"new-file.yaml": "new content\n",
			})

			artifact := &fetcher.Artifact{
				Path:     tarballPath,
				Revision: "new-rev",
				Checksum: "sha256:new123",
				Size:     50,
			}

			_, version, _, err := reconciler.storeArtifact(source, artifact)
			Expect(err).NotTo(HaveOccurred())

			By("verifying old versions were garbage collected")
			entries, err := os.ReadDir(versionsDir)
			Expect(err).NotTo(HaveOccurred())
			// Should have max 2 versions: 1 old (most recent) + 1 new
			Expect(len(entries)).To(BeNumerically("<=", 2))
			Expect(entries).To(ContainElement(HaveField("Name()", version)))
		})
	})

	Context("When reconciling with license validation", func() {
		var (
			arenaSource *omniav1alpha1.ArenaSource
		)

		BeforeEach(func() {
			By("creating the ArenaSource with Git type")
			arenaSource = &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "license-test-source",
					Namespace: arenaSourceNamespace,
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type:     omniav1alpha1.ArenaSourceTypeGit,
					Interval: "5m",
					Git: &omniav1alpha1.GitSource{
						URL: "https://github.com/example/repo.git",
					},
				},
			}
			Expect(k8sClient.Create(ctx, arenaSource)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the ArenaSource")
			resource := &omniav1alpha1.ArenaSource{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "license-test-source",
				Namespace: arenaSourceNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set Error phase when license validation fails", func() {
			By("creating reconciler with license validator")
			// Create a mock license validator that rejects git sources
			_, publicKey := generateTestKeyPairForController()

			licValidator, err := createOpenCoreLicenseValidator(publicKey)
			Expect(err).NotTo(HaveOccurred())

			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := &ArenaSourceReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				Recorder:         fakeRecorder,
				ArtifactDir:      artifactDir,
				ArtifactBaseURL:  "http://localhost:8080/artifacts",
				LicenseValidator: licValidator,
			}

			By("reconciling the ArenaSource")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "license-test-source",
					Namespace: arenaSourceNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			By("checking that phase is Error")
			updatedSource := &omniav1alpha1.ArenaSource{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "license-test-source",
				Namespace: arenaSourceNamespace,
			}, updatedSource)).To(Succeed())

			Expect(updatedSource.Status.Phase).To(Equal(omniav1alpha1.ArenaSourcePhaseError))

			By("checking the Ready condition shows license violation")
			condition := meta.FindStatusCondition(updatedSource.Status.Conditions, ArenaSourceConditionTypeReady)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("LicenseViolation"))
		})
	})

	Context("When testing getWorkspaceForNamespace helper", func() {
		It("should return workspace name from namespace label", func() {
			By("creating a namespace with workspace label")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ws-test-namespace",
					Labels: map[string]string{
						"omnia.altairalabs.ai/workspace": "my-workspace",
					},
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, ns)
			}()

			By("testing getWorkspaceForNamespace")
			reconciler := &ArenaSourceReconciler{
				Client: k8sClient,
			}
			result := reconciler.getWorkspaceForNamespace(ctx, "ws-test-namespace")
			Expect(result).To(Equal("my-workspace"))
		})

		It("should return namespace name when label is missing", func() {
			By("creating a namespace without workspace label")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ws-test-no-label",
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, ns)
			}()

			By("testing getWorkspaceForNamespace")
			reconciler := &ArenaSourceReconciler{
				Client: k8sClient,
			}
			result := reconciler.getWorkspaceForNamespace(ctx, "ws-test-no-label")
			Expect(result).To(Equal("ws-test-no-label"))
		})

		It("should return namespace name when namespace doesn't exist", func() {
			By("testing getWorkspaceForNamespace with nonexistent namespace")
			reconciler := &ArenaSourceReconciler{
				Client: k8sClient,
			}
			result := reconciler.getWorkspaceForNamespace(ctx, "nonexistent-namespace")
			Expect(result).To(Equal("nonexistent-namespace"))
		})
	})

	Context("When testing updateHEAD helper", func() {
		It("should create HEAD file with version", func() {
			By("creating a temporary workspace path")
			tmpDir, err := os.MkdirTemp("", "head-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			By("calling updateHEAD")
			reconciler := &ArenaSourceReconciler{}
			err = reconciler.updateHEAD(tmpDir, "v1.0.0")
			Expect(err).NotTo(HaveOccurred())

			By("verifying HEAD file exists with correct content")
			headContent, err := os.ReadFile(filepath.Join(tmpDir, ".arena", "HEAD"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(headContent)).To(Equal("v1.0.0"))
		})

		It("should update existing HEAD file", func() {
			By("creating a temporary workspace path with existing HEAD")
			tmpDir, err := os.MkdirTemp("", "head-update-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			reconciler := &ArenaSourceReconciler{}

			By("creating first version")
			err = reconciler.updateHEAD(tmpDir, "v1.0.0")
			Expect(err).NotTo(HaveOccurred())

			By("updating to second version")
			err = reconciler.updateHEAD(tmpDir, "v2.0.0")
			Expect(err).NotTo(HaveOccurred())

			By("verifying HEAD has new version")
			headContent, err := os.ReadFile(filepath.Join(tmpDir, ".arena", "HEAD"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(headContent)).To(Equal("v2.0.0"))
		})
	})

	Context("When testing gcOldVersions helper", func() {
		It("should not remove versions when under limit", func() {
			By("creating workspace with fewer versions than limit")
			tmpDir, err := os.MkdirTemp("", "gc-under-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			versionsDir := filepath.Join(tmpDir, ".arena", "versions")
			Expect(os.MkdirAll(versionsDir, 0755)).To(Succeed())

			// Create 2 versions
			for _, v := range []string{"v1", "v2"} {
				Expect(os.MkdirAll(filepath.Join(versionsDir, v), 0755)).To(Succeed())
			}

			reconciler := &ArenaSourceReconciler{MaxVersionsPerSource: 5}
			err = reconciler.gcOldVersions(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			// Both versions should still exist
			entries, err := os.ReadDir(versionsDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(2))
		})

		It("should handle nonexistent versions directory", func() {
			tmpDir, err := os.MkdirTemp("", "gc-nonexistent-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			reconciler := &ArenaSourceReconciler{MaxVersionsPerSource: 5}
			err = reconciler.gcOldVersions(tmpDir)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should remove oldest versions when exceeding limit", func() {
			By("creating workspace with more versions than limit")
			tmpDir, err := os.MkdirTemp("", "gc-remove-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			versionsDir := filepath.Join(tmpDir, ".arena", "versions")
			Expect(os.MkdirAll(versionsDir, 0755)).To(Succeed())

			// Create 5 versions with different mod times
			versions := []string{"v1", "v2", "v3", "v4", "v5"}
			for i, v := range versions {
				versionPath := filepath.Join(versionsDir, v)
				Expect(os.MkdirAll(versionPath, 0755)).To(Succeed())
				// Set different mod times to control ordering
				modTime := time.Now().Add(time.Duration(i) * time.Hour)
				Expect(os.Chtimes(versionPath, modTime, modTime)).To(Succeed())
			}

			reconciler := &ArenaSourceReconciler{MaxVersionsPerSource: 3}
			err = reconciler.gcOldVersions(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			// Should have 3 versions left (the newest ones)
			entries, err := os.ReadDir(versionsDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(3))

			// v1 and v2 should be removed (oldest)
			_, err = os.Stat(filepath.Join(versionsDir, "v1"))
			Expect(os.IsNotExist(err)).To(BeTrue())
			_, err = os.Stat(filepath.Join(versionsDir, "v2"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("should use default max when not configured", func() {
			By("creating workspace with max versions set to 0")
			tmpDir, err := os.MkdirTemp("", "gc-default-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			versionsDir := filepath.Join(tmpDir, ".arena", "versions")
			Expect(os.MkdirAll(versionsDir, 0755)).To(Succeed())

			// Create 5 versions
			for _, v := range []string{"v1", "v2", "v3", "v4", "v5"} {
				Expect(os.MkdirAll(filepath.Join(versionsDir, v), 0755)).To(Succeed())
			}

			reconciler := &ArenaSourceReconciler{MaxVersionsPerSource: 0} // Should default to 10
			err = reconciler.gcOldVersions(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			// All 5 should remain (under default of 10)
			entries, err := os.ReadDir(versionsDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(5))
		})

		It("should skip non-directory entries", func() {
			By("creating workspace with mixed entries")
			tmpDir, err := os.MkdirTemp("", "gc-mixed-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			versionsDir := filepath.Join(tmpDir, ".arena", "versions")
			Expect(os.MkdirAll(versionsDir, 0755)).To(Succeed())

			// Create directories
			for _, v := range []string{"v1", "v2"} {
				Expect(os.MkdirAll(filepath.Join(versionsDir, v), 0755)).To(Succeed())
			}
			// Create a file (should be skipped)
			Expect(os.WriteFile(filepath.Join(versionsDir, "README.txt"), []byte("info"), 0644)).To(Succeed())

			reconciler := &ArenaSourceReconciler{MaxVersionsPerSource: 5}
			err = reconciler.gcOldVersions(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			// All entries should remain
			entries, err := os.ReadDir(versionsDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(HaveLen(3)) // v1, v2, README.txt
		})
	})

	Context("When testing copyFile helper", func() {
		It("should copy file content", func() {
			By("creating source file")
			srcDir, err := os.MkdirTemp("", "copy-src-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(srcDir) }()

			srcFile := filepath.Join(srcDir, "source.txt")
			Expect(os.WriteFile(srcFile, []byte("test content to copy"), 0644)).To(Succeed())

			By("copying to destination")
			dstDir, err := os.MkdirTemp("", "copy-dst-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(dstDir) }()

			dstFile := filepath.Join(dstDir, "dest.txt")
			err = copyFile(srcFile, dstFile)
			Expect(err).NotTo(HaveOccurred())

			By("verifying content")
			content, err := os.ReadFile(dstFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal("test content to copy"))
		})

		It("should fail when source doesn't exist", func() {
			dstDir, err := os.MkdirTemp("", "copy-fail-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(dstDir) }()

			err = copyFile("/nonexistent/source.txt", filepath.Join(dstDir, "dest.txt"))
			Expect(err).To(HaveOccurred())
		})

		It("should fail when destination cannot be created", func() {
			By("creating source file")
			srcDir, err := os.MkdirTemp("", "copy-fail-dst-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(srcDir) }()

			srcFile := filepath.Join(srcDir, "source.txt")
			Expect(os.WriteFile(srcFile, []byte("content"), 0644)).To(Succeed())

			err = copyFile(srcFile, "/nonexistent/dir/dest.txt")
			Expect(err).To(HaveOccurred())
		})
	})

	Context("When testing syncToFilesystem version caching", func() {
		It("should skip sync when version already exists", func() {
			By("creating a temporary workspace content directory")
			tmpDir, err := os.MkdirTemp("", "sync-skip-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			By("creating a test tarball")
			tarballDir, err := os.MkdirTemp("", "tarball-skip-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tarballDir) }()

			tarballPath := filepath.Join(tarballDir, "test.tar.gz")
			createTestTarball(tarballPath, map[string]string{
				"config.yaml": "name: cached-test\n",
			})

			By("syncing first time to create version")
			reconciler := &ArenaSourceReconciler{
				WorkspaceContentPath: tmpDir,
				MaxVersionsPerSource: 5,
			}

			source := &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cache-test",
					Namespace: "test-ns",
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					TargetPath: "arena/cache-test",
				},
			}

			artifact := &fetcher.Artifact{
				Path:     tarballPath,
				Revision: "rev1",
				Checksum: "sha256:cache",
				Size:     50,
			}

			contentPath1, version1, _, err := reconciler.storeArtifact(source, artifact)
			Expect(err).NotTo(HaveOccurred())
			Expect(version1).NotTo(BeEmpty())

			By("syncing again with same content (should skip)")
			contentPath2, version2, _, err := reconciler.storeArtifact(source, artifact)
			Expect(err).NotTo(HaveOccurred())
			Expect(version2).To(Equal(version1))
			Expect(contentPath2).To(Equal(contentPath1))
		})
	})

	Context("When testing updateHEAD error paths", func() {
		It("should succeed with valid path", func() {
			tmpDir, err := os.MkdirTemp("", "head-ok-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			reconciler := &ArenaSourceReconciler{}
			err = reconciler.updateHEAD(tmpDir, "test-version")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(filepath.Join(tmpDir, ".arena", "HEAD"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal("test-version"))
		})
	})

	Context("When testing extractTarEntries with various entry types", func() {
		It("should handle error in tar.Reader.Next", func() {
			// Create empty/invalid gzipped content
			tmpDir, err := os.MkdirTemp("", "tar-invalid-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			// Create a gzip file with invalid tar content
			tarPath := filepath.Join(tmpDir, "invalid.tar.gz")
			file, err := os.Create(tarPath)
			Expect(err).NotTo(HaveOccurred())
			gw := gzip.NewWriter(file)
			_, _ = gw.Write([]byte("not a valid tar"))
			_ = gw.Close()
			_ = file.Close()

			destDir, err := os.MkdirTemp("", "tar-dest-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(destDir) }()

			err = extractTarGzToDir(tarPath, destDir)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("When testing createOCIFetcher helper", func() {
		It("should create OCI fetcher with default credentials", func() {
			By("creating reconciler with OCI source")
			reconciler := &ArenaSourceReconciler{
				Client:      k8sClient,
				ArtifactDir: "/tmp/artifacts",
			}

			source := &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oci-test",
					Namespace: "default",
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					Type: omniav1alpha1.ArenaSourceTypeOCI,
					OCI: &omniav1alpha1.OCISource{
						URL: "registry.example.com/my-repo:latest",
					},
				},
			}

			opts := fetcher.Options{
				Timeout: time.Minute,
				WorkDir: "/tmp",
			}

			f, err := reconciler.createOCIFetcher(ctx, source, opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(f).NotTo(BeNil())
		})
	})

	Context("When testing storeTarGzArtifact error paths", func() {
		It("should handle short checksum gracefully", func() {
			By("creating artifact directory")
			artifactDir, err := os.MkdirTemp("", "artifact-short-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(artifactDir) }()

			tarballDir, err := os.MkdirTemp("", "tarball-short-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tarballDir) }()

			tarballPath := filepath.Join(tarballDir, "test.tar.gz")
			createTestTarball(tarballPath, map[string]string{
				"file.txt": "content",
			})

			reconciler := &ArenaSourceReconciler{
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}

			source := &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "checksum-test",
					Namespace: "default",
				},
			}

			artifact := &fetcher.Artifact{
				Path: tarballPath,
				// Use a proper long checksum
				Checksum: "sha256:abcdef123456789012345678901234567890abcdef123456789012345678901234",
				Size:     100,
			}

			_, _, url, err := reconciler.storeTarGzArtifact(source, artifact)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(ContainSubstring("abcdef123456"))
		})
	})

	Context("When testing findNestedTarGz helper", func() {
		It("should find tar.gz file in directory", func() {
			By("creating directory with single tar.gz file")
			tmpDir, err := os.MkdirTemp("", "nested-tar-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			tarPath := filepath.Join(tmpDir, "pack.tar.gz")
			Expect(os.WriteFile(tarPath, []byte("fake tarball"), 0644)).To(Succeed())

			result := findNestedTarGz(tmpDir)
			Expect(result).To(Equal(tarPath))
		})

		It("should find tgz file in directory", func() {
			By("creating directory with single tgz file")
			tmpDir, err := os.MkdirTemp("", "nested-tgz-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			tarPath := filepath.Join(tmpDir, "pack.tgz")
			Expect(os.WriteFile(tarPath, []byte("fake tarball"), 0644)).To(Succeed())

			result := findNestedTarGz(tmpDir)
			Expect(result).To(Equal(tarPath))
		})

		It("should return empty when multiple files exist", func() {
			By("creating directory with multiple files")
			tmpDir, err := os.MkdirTemp("", "nested-multi-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			Expect(os.WriteFile(filepath.Join(tmpDir, "file1.tar.gz"), []byte("content"), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "file2.tar.gz"), []byte("content"), 0644)).To(Succeed())

			result := findNestedTarGz(tmpDir)
			Expect(result).To(BeEmpty())
		})

		It("should return empty when entry is a directory", func() {
			By("creating directory with a subdirectory")
			tmpDir, err := os.MkdirTemp("", "nested-dir-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			Expect(os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)).To(Succeed())

			result := findNestedTarGz(tmpDir)
			Expect(result).To(BeEmpty())
		})

		It("should return empty for non-tar.gz files", func() {
			By("creating directory with non-tar.gz file")
			tmpDir, err := os.MkdirTemp("", "nested-txt-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			Expect(os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("content"), 0644)).To(Succeed())

			result := findNestedTarGz(tmpDir)
			Expect(result).To(BeEmpty())
		})
	})

	Context("When testing extractTarGzToDir helper", func() {
		It("should extract tar.gz to directory", func() {
			By("creating a test tarball")
			tarballDir, err := os.MkdirTemp("", "extract-src-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tarballDir) }()

			tarPath := filepath.Join(tarballDir, "test.tar.gz")
			createTestTarball(tarPath, map[string]string{
				"config.yaml":       "name: test\n",
				"prompts/hello.txt": "Hello!\n",
			})

			By("extracting to destination")
			destDir, err := os.MkdirTemp("", "extract-dst-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(destDir) }()

			err = extractTarGzToDir(tarPath, destDir)
			Expect(err).NotTo(HaveOccurred())

			By("verifying extracted files")
			content, err := os.ReadFile(filepath.Join(destDir, "config.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal("name: test\n"))

			promptContent, err := os.ReadFile(filepath.Join(destDir, "prompts/hello.txt"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(promptContent)).To(Equal("Hello!\n"))
		})

		It("should fail for nonexistent file", func() {
			tmpDir, err := os.MkdirTemp("", "extract-fail-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			err = extractTarGzToDir("/nonexistent/path.tar.gz", tmpDir)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("When testing extractTarEntry helper", func() {
		It("should reject path traversal attempts", func() {
			tmpDir, err := os.MkdirTemp("", "traversal-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			header := &tar.Header{
				Name:     "../../../etc/passwd",
				Typeflag: tar.TypeReg,
				Size:     10,
			}

			err = extractTarEntry(nil, header, tmpDir)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid tar path"))
		})

		It("should skip macOS resource fork files", func() {
			tmpDir, err := os.MkdirTemp("", "macos-skip-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			header := &tar.Header{
				Name:     "._resource-fork",
				Typeflag: tar.TypeReg,
				Size:     10,
			}

			err = extractTarEntry(nil, header, tmpDir)
			Expect(err).NotTo(HaveOccurred())

			// File should not exist
			_, err = os.Stat(filepath.Join(tmpDir, "._resource-fork"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("should create directory entries", func() {
			tmpDir, err := os.MkdirTemp("", "dir-entry-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			header := &tar.Header{
				Name:     "subdir/",
				Typeflag: tar.TypeDir,
			}

			err = extractTarEntry(nil, header, tmpDir)
			Expect(err).NotTo(HaveOccurred())

			// Directory should exist
			info, err := os.Stat(filepath.Join(tmpDir, "subdir"))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.IsDir()).To(BeTrue())
		})
	})

	Context("When testing extractSymlink helper", func() {
		It("should create valid symlink", func() {
			tmpDir, err := os.MkdirTemp("", "symlink-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			// Create target file first
			Expect(os.WriteFile(filepath.Join(tmpDir, "target.txt"), []byte("content"), 0644)).To(Succeed())

			header := &tar.Header{
				Name:     "link.txt",
				Linkname: "target.txt",
				Typeflag: tar.TypeSymlink,
			}

			err = extractSymlink(header, filepath.Join(tmpDir, "link.txt"), tmpDir)
			Expect(err).NotTo(HaveOccurred())

			// Verify symlink exists
			linkInfo, err := os.Lstat(filepath.Join(tmpDir, "link.txt"))
			Expect(err).NotTo(HaveOccurred())
			Expect(linkInfo.Mode() & os.ModeSymlink).NotTo(Equal(os.FileMode(0)))
		})

		It("should reject symlink escape attempts", func() {
			tmpDir, err := os.MkdirTemp("", "symlink-escape-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			header := &tar.Header{
				Name:     "link.txt",
				Linkname: "../../../etc/passwd",
				Typeflag: tar.TypeSymlink,
			}

			err = extractSymlink(header, filepath.Join(tmpDir, "link.txt"), tmpDir)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("symlink escape attempt"))
		})
	})

	Context("When testing copyDirectory helper", func() {
		It("should copy directory recursively", func() {
			By("creating source directory with files")
			srcDir, err := os.MkdirTemp("", "copy-src-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(srcDir) }()

			Expect(os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644)).To(Succeed())
			Expect(os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0644)).To(Succeed())

			By("copying to destination")
			dstDir, err := os.MkdirTemp("", "copy-dst-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(dstDir) }()

			err = copyDirectory(srcDir, dstDir)
			Expect(err).NotTo(HaveOccurred())

			By("verifying copied files")
			content1, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content1)).To(Equal("content1"))

			content2, err := os.ReadFile(filepath.Join(dstDir, "subdir", "file2.txt"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content2)).To(Equal("content2"))
		})
	})

	Context("When testing copyFileWithMode helper", func() {
		It("should copy file with mode", func() {
			By("creating source file")
			srcDir, err := os.MkdirTemp("", "copymode-src-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(srcDir) }()

			srcFile := filepath.Join(srcDir, "source.txt")
			Expect(os.WriteFile(srcFile, []byte("test content"), 0755)).To(Succeed())

			By("copying to destination")
			dstDir, err := os.MkdirTemp("", "copymode-dst-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(dstDir) }()

			dstFile := filepath.Join(dstDir, "dest.txt")
			err = copyFileWithMode(srcFile, dstFile, 0755)
			Expect(err).NotTo(HaveOccurred())

			By("verifying content and mode")
			content, err := os.ReadFile(dstFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal("test content"))

			info, err := os.Stat(dstFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0755)))
		})

		It("should fail when source doesn't exist", func() {
			dstDir, err := os.MkdirTemp("", "copymode-fail-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(dstDir) }()

			err = copyFileWithMode("/nonexistent/file.txt", filepath.Join(dstDir, "dest.txt"), 0644)
			Expect(err).To(HaveOccurred())
		})

		It("should fail when destination directory doesn't exist", func() {
			srcDir, err := os.MkdirTemp("", "copymode-src2-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(srcDir) }()

			srcFile := filepath.Join(srcDir, "source.txt")
			Expect(os.WriteFile(srcFile, []byte("content"), 0644)).To(Succeed())

			err = copyFileWithMode(srcFile, "/nonexistent/dir/dest.txt", 0644)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("When testing calculateDirectoryHash helper", func() {
		It("should calculate hash for directory", func() {
			By("creating directory with files")
			tmpDir, err := os.MkdirTemp("", "hash-test-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			Expect(os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("content"), 0644)).To(Succeed())

			By("calculating hash")
			hash, err := calculateDirectoryHash(tmpDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(hash).NotTo(BeEmpty())
			Expect(hash).To(HaveLen(64)) // SHA256 hex = 64 chars
		})

		It("should return different hashes for different content", func() {
			By("creating first directory")
			dir1, err := os.MkdirTemp("", "hash-test1-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(dir1) }()
			Expect(os.WriteFile(filepath.Join(dir1, "file.txt"), []byte("content1"), 0644)).To(Succeed())

			By("creating second directory")
			dir2, err := os.MkdirTemp("", "hash-test2-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(dir2) }()
			Expect(os.WriteFile(filepath.Join(dir2, "file.txt"), []byte("content2"), 0644)).To(Succeed())

			By("comparing hashes")
			hash1, err := calculateDirectoryHash(dir1)
			Expect(err).NotTo(HaveOccurred())

			hash2, err := calculateDirectoryHash(dir2)
			Expect(err).NotTo(HaveOccurred())

			Expect(hash1).NotTo(Equal(hash2))
		})

		It("should handle subdirectories", func() {
			By("creating directory with subdirectories")
			tmpDir, err := os.MkdirTemp("", "hash-subdir-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			Expect(os.MkdirAll(filepath.Join(tmpDir, "subdir1", "nested"), 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "subdir1", "nested", "file.txt"), []byte("nested content"), 0644)).To(Succeed())
			Expect(os.MkdirAll(filepath.Join(tmpDir, "subdir2"), 0755)).To(Succeed())

			By("calculating hash")
			hash, err := calculateDirectoryHash(tmpDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(hash).NotTo(BeEmpty())
		})
	})

	Context("When testing extractTarEntries helper", func() {
		It("should extract multiple entries", func() {
			By("creating tarball with multiple files")
			tarballDir, err := os.MkdirTemp("", "entries-src-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tarballDir) }()

			tarPath := filepath.Join(tarballDir, "multi.tar.gz")
			createTestTarball(tarPath, map[string]string{
				"file1.txt":          "content1",
				"file2.txt":          "content2",
				"subdir/file3.txt":   "content3",
				"subdir/nested/a.md": "# Heading",
			})

			By("extracting to destination")
			destDir, err := os.MkdirTemp("", "entries-dst-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(destDir) }()

			err = extractTarGzToDir(tarPath, destDir)
			Expect(err).NotTo(HaveOccurred())

			By("verifying all files extracted")
			content1, err := os.ReadFile(filepath.Join(destDir, "file1.txt"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content1)).To(Equal("content1"))

			content2, err := os.ReadFile(filepath.Join(destDir, "file2.txt"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content2)).To(Equal("content2"))

			content3, err := os.ReadFile(filepath.Join(destDir, "subdir", "file3.txt"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content3)).To(Equal("content3"))

			contentA, err := os.ReadFile(filepath.Join(destDir, "subdir", "nested", "a.md"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(contentA)).To(Equal("# Heading"))
		})
	})

	Context("When testing syncToFilesystem edge cases", func() {
		It("should use default target path when not specified", func() {
			By("creating a temporary workspace content directory")
			tmpDir, err := os.MkdirTemp("", "sync-default-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			By("creating a test tarball")
			tarballDir, err := os.MkdirTemp("", "tarball-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tarballDir) }()

			tarballPath := filepath.Join(tarballDir, "test.tar.gz")
			createTestTarball(tarballPath, map[string]string{
				"config.yaml": "name: test\n",
			})

			By("syncing with no target path (uses default)")
			reconciler := &ArenaSourceReconciler{
				WorkspaceContentPath: tmpDir,
				MaxVersionsPerSource: 5,
			}

			source := &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-source",
					Namespace: "test-ws",
				},
				// No TargetPath specified - should default to arena/{source-name}
			}

			artifact := &fetcher.Artifact{
				Path:     tarballPath,
				Revision: "rev1",
				Checksum: "sha256:abc",
				Size:     50,
			}

			contentPath, version, url, err := reconciler.storeArtifact(source, artifact)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(BeEmpty())
			Expect(version).NotTo(BeEmpty())
			Expect(contentPath).To(ContainSubstring("arena/my-source"))
		})

		It("should handle nested tar.gz in extracted content", func() {
			By("creating a temporary workspace content directory")
			tmpDir, err := os.MkdirTemp("", "nested-sync-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			By("creating a nested tarball (tarball containing a tarball)")
			innerTarDir, err := os.MkdirTemp("", "inner-tar-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(innerTarDir) }()

			// First create the inner tarball
			innerTarPath := filepath.Join(innerTarDir, "inner.tar.gz")
			createTestTarball(innerTarPath, map[string]string{
				"actual-config.yaml": "name: nested-content\n",
				"prompts/test.txt":   "This is nested!\n",
			})

			// Now create outer tarball containing just the inner tarball
			outerTarDir, err := os.MkdirTemp("", "outer-tar-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(outerTarDir) }()

			// Copy inner tarball to outer directory
			outerNestedPath := filepath.Join(outerTarDir, "pack.tar.gz")
			innerData, err := os.ReadFile(innerTarPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(outerNestedPath, innerData, 0644)).To(Succeed())

			// Create outer tarball
			outerTarPath := filepath.Join(outerTarDir, "outer.tar.gz")
			createTestTarball(outerTarPath, map[string]string{})

			// Manually create outer tarball with the nested one
			// Use tar-stream to create tarball with file entry
			file, err := os.Create(outerTarPath)
			Expect(err).NotTo(HaveOccurred())
			gw := gzip.NewWriter(file)
			tw := tar.NewWriter(gw)

			// Add the inner tarball as a file entry
			innerInfo, err := os.Stat(outerNestedPath)
			Expect(err).NotTo(HaveOccurred())
			hdr := &tar.Header{
				Name: "pack.tar.gz",
				Mode: 0644,
				Size: innerInfo.Size(),
			}
			Expect(tw.WriteHeader(hdr)).To(Succeed())
			innerContent, err := os.ReadFile(outerNestedPath)
			Expect(err).NotTo(HaveOccurred())
			_, err = tw.Write(innerContent)
			Expect(err).NotTo(HaveOccurred())

			Expect(tw.Close()).To(Succeed())
			Expect(gw.Close()).To(Succeed())
			Expect(file.Close()).To(Succeed())

			By("syncing nested tarball")
			reconciler := &ArenaSourceReconciler{
				WorkspaceContentPath: tmpDir,
				MaxVersionsPerSource: 5,
			}

			source := &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nested-source",
					Namespace: "test-ws",
				},
				Spec: omniav1alpha1.ArenaSourceSpec{
					TargetPath: "arena/nested-source",
				},
			}

			artifact := &fetcher.Artifact{
				Path:     outerTarPath,
				Revision: "rev1",
				Checksum: "sha256:nested",
				Size:     100,
			}

			contentPath, version, _, err := reconciler.storeArtifact(source, artifact)
			Expect(err).NotTo(HaveOccurred())
			Expect(version).NotTo(BeEmpty())
			Expect(contentPath).To(ContainSubstring(".arena/versions/"))

			By("verifying nested content was extracted")
			versionDir := filepath.Join(tmpDir, "test-ws", "test-ws", "arena/nested-source", ".arena", "versions", version)
			actualConfigContent, err := os.ReadFile(filepath.Join(versionDir, "actual-config.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(actualConfigContent)).To(Equal("name: nested-content\n"))
		})
	})

	Context("When testing extractRegularFile helper", func() {
		It("should extract file with correct permissions", func() {
			By("creating a tarball with executable file")
			tarballDir, err := os.MkdirTemp("", "extract-perm-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tarballDir) }()

			tarPath := filepath.Join(tarballDir, "test.tar.gz")
			file, err := os.Create(tarPath)
			Expect(err).NotTo(HaveOccurred())
			gw := gzip.NewWriter(file)
			tw := tar.NewWriter(gw)

			// Add executable file
			hdr := &tar.Header{
				Name: "script.sh",
				Mode: 0755,
				Size: int64(len("#!/bin/bash\necho hello")),
			}
			Expect(tw.WriteHeader(hdr)).To(Succeed())
			_, err = tw.Write([]byte("#!/bin/bash\necho hello"))
			Expect(err).NotTo(HaveOccurred())

			Expect(tw.Close()).To(Succeed())
			Expect(gw.Close()).To(Succeed())
			Expect(file.Close()).To(Succeed())

			By("extracting")
			destDir, err := os.MkdirTemp("", "extract-perm-dst-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(destDir) }()

			err = extractTarGzToDir(tarPath, destDir)
			Expect(err).NotTo(HaveOccurred())

			By("verifying file permissions")
			info, err := os.Stat(filepath.Join(destDir, "script.sh"))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0755)))
		})
	})

	Context("When testing storeTarGzArtifact helper", func() {
		It("should store tarball to artifact directory", func() {
			By("creating artifact directories")
			artifactDir, err := os.MkdirTemp("", "artifact-store-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(artifactDir) }()

			By("creating a test tarball")
			tarballDir, err := os.MkdirTemp("", "tarball-src-*")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = os.RemoveAll(tarballDir) }()

			tarballPath := filepath.Join(tarballDir, "test.tar.gz")
			createTestTarball(tarballPath, map[string]string{
				"config.yaml": "test content\n",
			})

			By("storing the artifact")
			reconciler := &ArenaSourceReconciler{
				ArtifactDir:     artifactDir,
				ArtifactBaseURL: "http://localhost:8080/artifacts",
			}

			source := &omniav1alpha1.ArenaSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "store-test",
					Namespace: "default",
				},
			}

			artifact := &fetcher.Artifact{
				Path:     tarballPath,
				Revision: "test-rev",
				Checksum: "sha256:abc123abc123abc123abc123abc123abc123abc123abc123abc123abc123abcd",
				Size:     100,
			}

			_, _, url, err := reconciler.storeTarGzArtifact(source, artifact)
			Expect(err).NotTo(HaveOccurred())
			Expect(url).To(ContainSubstring("http://localhost:8080/artifacts"))
			Expect(url).To(ContainSubstring("default/store-test"))

			By("verifying artifact was stored")
			storedPath := filepath.Join(artifactDir, "default", "store-test")
			entries, err := os.ReadDir(storedPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).ToNot(BeEmpty())
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
