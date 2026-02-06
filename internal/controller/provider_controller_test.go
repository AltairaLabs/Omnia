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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

var _ = Describe("Provider Controller", func() {
	const (
		providerName      = "test-provider"
		providerNamespace = "default"
		secretName        = "test-provider-secret"
		timeout           = time.Second * 10
		interval          = time.Millisecond * 250
	)

	Context("When reconciling a Provider", func() {
		var (
			ctx      context.Context
			provider *omniav1alpha1.Provider
			secret   *corev1.Secret
		)

		BeforeEach(func() {
			ctx = context.Background()

			// Create the secret first
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: providerNamespace,
				},
				Data: map[string][]byte{
					"ANTHROPIC_API_KEY": []byte("test-api-key"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up resources
			if provider != nil {
				_ = k8sClient.Delete(ctx, provider)
			}
			if secret != nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should successfully reconcile a Provider with valid secret", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerName,
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					SecretRef: &omniav1alpha1.SecretKeyRef{
						Name: secretName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			// Reconcile
			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      providerName,
					Namespace: providerNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status
			var updatedProvider omniav1alpha1.Provider
			Eventually(func() omniav1alpha1.ProviderPhase {
				_ = k8sClient.Get(ctx, types.NamespacedName{
					Name:      providerName,
					Namespace: providerNamespace,
				}, &updatedProvider)
				return updatedProvider.Status.Phase
			}, timeout, interval).Should(Equal(omniav1alpha1.ProviderPhaseReady))
		})

		It("should fail when secret is not found", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerName + "-nosecret",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					SecretRef: &omniav1alpha1.SecretKeyRef{
						Name: "nonexistent-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			// Reconcile
			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))

			// Clean up
			_ = k8sClient.Delete(ctx, provider)
		})

		It("should fail when secret is missing the expected key", func() {
			// Create a secret without the expected key
			badSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bad-secret",
					Namespace: providerNamespace,
				},
				Data: map[string][]byte{
					"wrong-key": []byte("test-api-key"),
				},
			}
			Expect(k8sClient.Create(ctx, badSecret)).To(Succeed())

			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerName + "-badkey",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					SecretRef: &omniav1alpha1.SecretKeyRef{
						Name: "bad-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			// Reconcile
			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not contain"))

			// Clean up
			_ = k8sClient.Delete(ctx, provider)
			_ = k8sClient.Delete(ctx, badSecret)
		})

		It("should succeed when secret has the specified key", func() {
			// Create a secret with a custom key
			customSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-secret",
					Namespace: providerNamespace,
				},
				Data: map[string][]byte{
					"my-custom-key": []byte("test-api-key"),
				},
			}
			Expect(k8sClient.Create(ctx, customSecret)).To(Succeed())

			customKey := "my-custom-key"
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerName + "-customkey",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					SecretRef: &omniav1alpha1.SecretKeyRef{
						Name: "custom-secret",
						Key:  &customKey,
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			// Reconcile
			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status
			var updatedProvider omniav1alpha1.Provider
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      provider.Name,
				Namespace: providerNamespace,
			}, &updatedProvider)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedProvider.Status.Phase).To(Equal(omniav1alpha1.ProviderPhaseReady))

			// Clean up
			_ = k8sClient.Delete(ctx, provider)
			_ = k8sClient.Delete(ctx, customSecret)
		})

		It("should succeed when provider has no secretRef", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerName + "-nosecretref",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeMock,
					Model: "mock-model",
					// No SecretRef - mock provider doesn't need credentials
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status
			var updatedProvider omniav1alpha1.Provider
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      provider.Name,
				Namespace: providerNamespace,
			}, &updatedProvider)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedProvider.Status.Phase).To(Equal(omniav1alpha1.ProviderPhaseReady))

			// Verify the NoSecretRequired condition is set
			var foundCondition *metav1.Condition
			for i := range updatedProvider.Status.Conditions {
				if updatedProvider.Status.Conditions[i].Type == ProviderConditionTypeSecretFound {
					foundCondition = &updatedProvider.Status.Conditions[i]
					break
				}
			}
			Expect(foundCondition).NotTo(BeNil())
			Expect(foundCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(foundCondition.Reason).To(Equal("NoSecretRequired"))

			// Clean up
			_ = k8sClient.Delete(ctx, provider)
		})

		It("should handle Provider not found", func() {
			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Reconcile a non-existent Provider
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "nonexistent-provider",
					Namespace: providerNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})

		It("should reconcile with ValidateCredentials enabled", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerName + "-validate",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					SecretRef: &omniav1alpha1.SecretKeyRef{
						Name: secretName,
					},
					ValidateCredentials: true,
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status includes LastValidatedAt
			var updatedProvider omniav1alpha1.Provider
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      provider.Name,
				Namespace: providerNamespace,
			}, &updatedProvider)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedProvider.Status.Phase).To(Equal(omniav1alpha1.ProviderPhaseReady))
			Expect(updatedProvider.Status.LastValidatedAt).NotTo(BeNil())

			// Clean up
			_ = k8sClient.Delete(ctx, provider)
		})

		It("should fail when specified custom key doesn't exist in secret", func() {
			customKey := "nonexistent-key"
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerName + "-badcustomkey",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					SecretRef: &omniav1alpha1.SecretKeyRef{
						Name: secretName, // Has ANTHROPIC_API_KEY but not "nonexistent-key"
						Key:  &customKey,
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not contain key"))

			// Clean up
			_ = k8sClient.Delete(ctx, provider)
		})

		It("should successfully reconcile a Provider with credential.secretRef", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerName + "-credsecret",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					Credential: &omniav1alpha1.CredentialConfig{
						SecretRef: &omniav1alpha1.SecretKeyRef{
							Name: secretName,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			var updatedProvider omniav1alpha1.Provider
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      provider.Name,
				Namespace: providerNamespace,
			}, &updatedProvider)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedProvider.Status.Phase).To(Equal(omniav1alpha1.ProviderPhaseReady))

			// Verify CredentialConfigured condition
			var credCond *metav1.Condition
			for i := range updatedProvider.Status.Conditions {
				if updatedProvider.Status.Conditions[i].Type == ProviderConditionTypeCredentialConfigured {
					credCond = &updatedProvider.Status.Conditions[i]
					break
				}
			}
			Expect(credCond).NotTo(BeNil())
			Expect(credCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(credCond.Reason).To(Equal("SecretFound"))

			_ = k8sClient.Delete(ctx, provider)
		})

		It("should successfully reconcile a Provider with credential.envVar", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerName + "-credenv",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					Credential: &omniav1alpha1.CredentialConfig{
						EnvVar: "MY_CLAUDE_API_KEY",
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			var updatedProvider omniav1alpha1.Provider
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      provider.Name,
				Namespace: providerNamespace,
			}, &updatedProvider)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedProvider.Status.Phase).To(Equal(omniav1alpha1.ProviderPhaseReady))

			// Verify CredentialConfigured condition
			var credCond *metav1.Condition
			for i := range updatedProvider.Status.Conditions {
				if updatedProvider.Status.Conditions[i].Type == ProviderConditionTypeCredentialConfigured {
					credCond = &updatedProvider.Status.Conditions[i]
					break
				}
			}
			Expect(credCond).NotTo(BeNil())
			Expect(credCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(credCond.Reason).To(Equal("EnvVarConfigured"))

			_ = k8sClient.Delete(ctx, provider)
		})

		It("should successfully reconcile a Provider with credential.filePath", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerName + "-credfile",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					Credential: &omniav1alpha1.CredentialConfig{
						FilePath: "/var/secrets/api-key",
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			var updatedProvider omniav1alpha1.Provider
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      provider.Name,
				Namespace: providerNamespace,
			}, &updatedProvider)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedProvider.Status.Phase).To(Equal(omniav1alpha1.ProviderPhaseReady))

			// Verify CredentialConfigured condition
			var credCond *metav1.Condition
			for i := range updatedProvider.Status.Conditions {
				if updatedProvider.Status.Conditions[i].Type == ProviderConditionTypeCredentialConfigured {
					credCond = &updatedProvider.Status.Conditions[i]
					break
				}
			}
			Expect(credCond).NotTo(BeNil())
			Expect(credCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(credCond.Reason).To(Equal("FilePathConfigured"))

			_ = k8sClient.Delete(ctx, provider)
		})
	})

	Context("findProvidersForSecret", func() {
		var (
			ctx      context.Context
			provider *omniav1alpha1.Provider
			secret   *corev1.Secret
		)

		BeforeEach(func() {
			ctx = context.Background()

			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mapping-test-secret",
					Namespace: providerNamespace,
				},
				Data: map[string][]byte{
					"ANTHROPIC_API_KEY": []byte("test-api-key"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		})

		AfterEach(func() {
			if provider != nil {
				_ = k8sClient.Delete(ctx, provider)
			}
			if secret != nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should find providers that reference a secret", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mapping-test-provider",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					SecretRef: &omniav1alpha1.SecretKeyRef{
						Name: "mapping-test-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			requests := reconciler.findProvidersForSecret(ctx, secret)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("mapping-test-provider"))
			Expect(requests[0].Namespace).To(Equal(providerNamespace))
		})

		It("should find providers that reference a secret via credential.secretRef", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mapping-test-cred-provider",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					Credential: &omniav1alpha1.CredentialConfig{
						SecretRef: &omniav1alpha1.SecretKeyRef{
							Name: "mapping-test-secret",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			requests := reconciler.findProvidersForSecret(ctx, secret)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("mapping-test-cred-provider"))
			Expect(requests[0].Namespace).To(Equal(providerNamespace))
		})

		It("should return empty when no providers reference the secret", func() {
			otherSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unreferenced-secret",
					Namespace: providerNamespace,
				},
				Data: map[string][]byte{
					"ANTHROPIC_API_KEY": []byte("test-api-key"),
				},
			}
			Expect(k8sClient.Create(ctx, otherSecret)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, otherSecret) }()

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			requests := reconciler.findProvidersForSecret(ctx, otherSecret)
			Expect(requests).To(BeEmpty())
		})
	})

	Context("credential block validation", func() {
		var (
			ctx      context.Context
			provider *omniav1alpha1.Provider
			secret   *corev1.Secret
		)

		BeforeEach(func() {
			ctx = context.Background()

			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cred-test-secret",
					Namespace: providerNamespace,
				},
				Data: map[string][]byte{
					"ANTHROPIC_API_KEY": []byte("test-api-key"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		})

		AfterEach(func() {
			if provider != nil {
				_ = k8sClient.Delete(ctx, provider)
			}
			if secret != nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should succeed with credential.secretRef pointing to valid secret", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cred-secretref-valid",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					Credential: &omniav1alpha1.CredentialConfig{
						SecretRef: &omniav1alpha1.SecretKeyRef{
							Name: "cred-test-secret",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			var updated omniav1alpha1.Provider
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: provider.Name, Namespace: providerNamespace}, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.ProviderPhaseReady))

			var credCondition *metav1.Condition
			for i := range updated.Status.Conditions {
				if updated.Status.Conditions[i].Type == ProviderConditionTypeCredentialConfigured {
					credCondition = &updated.Status.Conditions[i]
					break
				}
			}
			Expect(credCondition).NotTo(BeNil())
			Expect(credCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(credCondition.Reason).To(Equal("SecretFound"))
		})

		It("should fail with credential.secretRef pointing to missing secret", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cred-secretref-missing",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					Credential: &omniav1alpha1.CredentialConfig{
						SecretRef: &omniav1alpha1.SecretKeyRef{
							Name: "nonexistent-cred-secret",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))

			var updated omniav1alpha1.Provider
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: provider.Name, Namespace: providerNamespace}, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.ProviderPhaseError))

			var credCondition *metav1.Condition
			for i := range updated.Status.Conditions {
				if updated.Status.Conditions[i].Type == ProviderConditionTypeCredentialConfigured {
					credCondition = &updated.Status.Conditions[i]
					break
				}
			}
			Expect(credCondition).NotTo(BeNil())
			Expect(credCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(credCondition.Reason).To(Equal("SecretNotFound"))
		})

		It("should fail with credential.secretRef when key is missing from secret", func() {
			badKey := "nonexistent-key"
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cred-secretref-badkey",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					Credential: &omniav1alpha1.CredentialConfig{
						SecretRef: &omniav1alpha1.SecretKeyRef{
							Name: "cred-test-secret",
							Key:  &badKey,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not contain key"))

			var updated omniav1alpha1.Provider
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: provider.Name, Namespace: providerNamespace}, &updated)).To(Succeed())

			var credCondition *metav1.Condition
			for i := range updated.Status.Conditions {
				if updated.Status.Conditions[i].Type == ProviderConditionTypeCredentialConfigured {
					credCondition = &updated.Status.Conditions[i]
					break
				}
			}
			Expect(credCondition).NotTo(BeNil())
			Expect(credCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(credCondition.Reason).To(Equal("SecretKeyMissing"))
		})

		It("should succeed with credential.envVar set to valid name", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cred-envvar-valid",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					Credential: &omniav1alpha1.CredentialConfig{
						EnvVar: "MY_API_KEY",
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			var updated omniav1alpha1.Provider
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: provider.Name, Namespace: providerNamespace}, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.ProviderPhaseReady))

			var credCondition *metav1.Condition
			for i := range updated.Status.Conditions {
				if updated.Status.Conditions[i].Type == ProviderConditionTypeCredentialConfigured {
					credCondition = &updated.Status.Conditions[i]
					break
				}
			}
			Expect(credCondition).NotTo(BeNil())
			Expect(credCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(credCondition.Reason).To(Equal("EnvVarConfigured"))
		})

		It("should reject credential.envVar with invalid name at CRD level", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cred-envvar-invalid",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					Credential: &omniav1alpha1.CredentialConfig{
						EnvVar: "123-BAD",
					},
				},
			}
			err := k8sClient.Create(ctx, provider)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("should match"))
			provider = nil // prevent cleanup of non-existent resource
		})

		It("should succeed with credential.filePath set to valid absolute path", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cred-filepath-valid",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					Credential: &omniav1alpha1.CredentialConfig{
						FilePath: "/etc/secrets/key",
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			var updated omniav1alpha1.Provider
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: provider.Name, Namespace: providerNamespace}, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.ProviderPhaseReady))

			var credCondition *metav1.Condition
			for i := range updated.Status.Conditions {
				if updated.Status.Conditions[i].Type == ProviderConditionTypeCredentialConfigured {
					credCondition = &updated.Status.Conditions[i]
					break
				}
			}
			Expect(credCondition).NotTo(BeNil())
			Expect(credCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(credCondition.Reason).To(Equal("FilePathConfigured"))
		})

		It("should reject credential.filePath with relative path at CRD level", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cred-filepath-relative",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					Credential: &omniav1alpha1.CredentialConfig{
						FilePath: "relative/path",
					},
				},
			}
			err := k8sClient.Create(ctx, provider)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("should match"))
			provider = nil // prevent cleanup of non-existent resource
		})

		It("should reject multiple credential strategies at CRD level", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cred-multiple-strategies",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					Credential: &omniav1alpha1.CredentialConfig{
						SecretRef: &omniav1alpha1.SecretKeyRef{
							Name: "cred-test-secret",
						},
						EnvVar: "MY_API_KEY",
					},
				},
			}
			err := k8sClient.Create(ctx, provider)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at most one credential method"))
			provider = nil // prevent cleanup of non-existent resource
		})

		It("should fail when credential block is empty", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cred-empty-block",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:       omniav1alpha1.ProviderTypeClaude,
					Model:      "claude-sonnet-4-20250514",
					Credential: &omniav1alpha1.CredentialConfig{},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no strategy is specified"))

			var updated omniav1alpha1.Provider
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: provider.Name, Namespace: providerNamespace}, &updated)).To(Succeed())

			var credCondition *metav1.Condition
			for i := range updated.Status.Conditions {
				if updated.Status.Conditions[i].Type == ProviderConditionTypeCredentialConfigured {
					credCondition = &updated.Status.Conditions[i]
					break
				}
			}
			Expect(credCondition).NotTo(BeNil())
			Expect(credCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(credCondition.Reason).To(Equal("NoStrategySpecified"))
		})

		It("should fail when provider requires credentials but none are configured", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cred-required-missing",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					// No SecretRef, no Credential
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("requires credentials"))

			var updated omniav1alpha1.Provider
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: provider.Name, Namespace: providerNamespace}, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.ProviderPhaseError))

			var credCondition *metav1.Condition
			for i := range updated.Status.Conditions {
				if updated.Status.Conditions[i].Type == ProviderConditionTypeCredentialConfigured {
					credCondition = &updated.Status.Conditions[i]
					break
				}
			}
			Expect(credCondition).NotTo(BeNil())
			Expect(credCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(credCondition.Reason).To(Equal("CredentialRequired"))
		})

		It("should succeed when mock provider has no credentials", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cred-mock-nocred",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeMock,
					Model: "mock-model",
					// No credentials - mock doesn't require them
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			var updated omniav1alpha1.Provider
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: provider.Name, Namespace: providerNamespace}, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.ProviderPhaseReady))

			var credCondition *metav1.Condition
			for i := range updated.Status.Conditions {
				if updated.Status.Conditions[i].Type == ProviderConditionTypeCredentialConfigured {
					credCondition = &updated.Status.Conditions[i]
					break
				}
			}
			Expect(credCondition).NotTo(BeNil())
			Expect(credCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(credCondition.Reason).To(Equal("NoCredentialRequired"))
		})

		It("should set LegacySecretRef reason when using legacy secretRef", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cred-legacy-secretref",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					SecretRef: &omniav1alpha1.SecretKeyRef{
						Name: "cred-test-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      provider.Name,
					Namespace: providerNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			var updated omniav1alpha1.Provider
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: provider.Name, Namespace: providerNamespace}, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.ProviderPhaseReady))

			var credCondition *metav1.Condition
			for i := range updated.Status.Conditions {
				if updated.Status.Conditions[i].Type == ProviderConditionTypeCredentialConfigured {
					credCondition = &updated.Status.Conditions[i]
					break
				}
			}
			Expect(credCondition).NotTo(BeNil())
			Expect(credCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(credCondition.Reason).To(Equal("LegacySecretRef"))
		})

		It("should reject both credential and legacy secretRef at CRD level", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cred-precedence",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					SecretRef: &omniav1alpha1.SecretKeyRef{
						Name: "some-other-secret",
					},
					Credential: &omniav1alpha1.CredentialConfig{
						SecretRef: &omniav1alpha1.SecretKeyRef{
							Name: "cred-test-secret",
						},
					},
				},
			}
			err := k8sClient.Create(ctx, provider)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("mutually exclusive"))
			provider = nil // prevent cleanup of non-existent resource
		})
	})

	Context("findProvidersForSecret with credential block", func() {
		var (
			ctx      context.Context
			provider *omniav1alpha1.Provider
			secret   *corev1.Secret
		)

		BeforeEach(func() {
			ctx = context.Background()

			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cred-mapping-secret",
					Namespace: providerNamespace,
				},
				Data: map[string][]byte{
					"ANTHROPIC_API_KEY": []byte("test-api-key"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		})

		AfterEach(func() {
			if provider != nil {
				_ = k8sClient.Delete(ctx, provider)
			}
			if secret != nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should find providers that reference a secret via credential.secretRef", func() {
			provider = &omniav1alpha1.Provider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cred-mapping-provider",
					Namespace: providerNamespace,
				},
				Spec: omniav1alpha1.ProviderSpec{
					Type:  omniav1alpha1.ProviderTypeClaude,
					Model: "claude-sonnet-4-20250514",
					Credential: &omniav1alpha1.CredentialConfig{
						SecretRef: &omniav1alpha1.SecretKeyRef{
							Name: "cred-mapping-secret",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			reconciler := &ProviderReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			requests := reconciler.findProvidersForSecret(ctx, secret)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("cred-mapping-provider"))
			Expect(requests[0].Namespace).To(Equal(providerNamespace))
		})
	})

	Context("getExpectedKeysForProvider", func() {
		It("should return correct keys for Claude", func() {
			keys := getExpectedKeysForProvider(omniav1alpha1.ProviderTypeClaude)
			Expect(keys).To(ContainElement("ANTHROPIC_API_KEY"))
			Expect(keys).To(ContainElement("api-key"))
		})

		It("should return correct keys for OpenAI", func() {
			keys := getExpectedKeysForProvider(omniav1alpha1.ProviderTypeOpenAI)
			Expect(keys).To(ContainElement("OPENAI_API_KEY"))
			Expect(keys).To(ContainElement("api-key"))
		})

		It("should return correct keys for Gemini", func() {
			keys := getExpectedKeysForProvider(omniav1alpha1.ProviderTypeGemini)
			Expect(keys).To(ContainElement("GEMINI_API_KEY"))
			Expect(keys).To(ContainElement("api-key"))
		})

		It("should return default keys for unknown provider types", func() {
			keys := getExpectedKeysForProvider(omniav1alpha1.ProviderType("unknown"))
			Expect(keys).To(ContainElement("api-key"))
			Expect(keys).To(ContainElement("ANTHROPIC_API_KEY"))
			Expect(keys).To(ContainElement("OPENAI_API_KEY"))
			Expect(keys).To(ContainElement("GEMINI_API_KEY"))
		})

		It("should return default keys for mock provider", func() {
			keys := getExpectedKeysForProvider(omniav1alpha1.ProviderTypeMock)
			Expect(keys).To(ContainElement("api-key"))
		})

		It("should return default keys for ollama provider", func() {
			keys := getExpectedKeysForProvider(omniav1alpha1.ProviderTypeOllama)
			Expect(keys).To(ContainElement("api-key"))
		})
	})
})
