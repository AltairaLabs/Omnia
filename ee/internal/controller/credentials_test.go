/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Credentials", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	Describe("LoadGitCredentials", func() {
		It("should load HTTPS credentials from a secret", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "git-https-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"username": []byte("my-user"),
					"password": []byte("my-pass"),
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

			creds, err := LoadGitCredentials(ctx, fakeClient, "default", "git-https-secret")
			Expect(err).NotTo(HaveOccurred())
			Expect(creds.Username).To(Equal("my-user"))
			Expect(creds.Password).To(Equal("my-pass"))
			Expect(creds.PrivateKey).To(BeNil())
			Expect(creds.KnownHosts).To(BeNil())
		})

		It("should load SSH credentials from a secret", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "git-ssh-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"identity":    []byte("ssh-private-key-data"),
					"known_hosts": []byte("github.com ssh-rsa AAAA..."),
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

			creds, err := LoadGitCredentials(ctx, fakeClient, "default", "git-ssh-secret")
			Expect(err).NotTo(HaveOccurred())
			Expect(creds.PrivateKey).To(Equal([]byte("ssh-private-key-data")))
			Expect(creds.KnownHosts).To(Equal([]byte("github.com ssh-rsa AAAA...")))
			Expect(creds.Username).To(BeEmpty())
			Expect(creds.Password).To(BeEmpty())
		})

		It("should load combined HTTPS and SSH credentials", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "git-combined-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"username":    []byte("my-user"),
					"password":    []byte("my-pass"),
					"identity":    []byte("ssh-key"),
					"known_hosts": []byte("hosts"),
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

			creds, err := LoadGitCredentials(ctx, fakeClient, "default", "git-combined-secret")
			Expect(err).NotTo(HaveOccurred())
			Expect(creds.Username).To(Equal("my-user"))
			Expect(creds.Password).To(Equal("my-pass"))
			Expect(creds.PrivateKey).To(Equal([]byte("ssh-key")))
			Expect(creds.KnownHosts).To(Equal([]byte("hosts")))
		})

		It("should return error when secret does not exist", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			_, err := LoadGitCredentials(ctx, fakeClient, "default", "nonexistent-secret")
			Expect(err).To(HaveOccurred())
		})

		It("should return empty credentials for secret with no relevant keys", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"unrelated-key": []byte("some-value"),
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

			creds, err := LoadGitCredentials(ctx, fakeClient, "default", "empty-secret")
			Expect(err).NotTo(HaveOccurred())
			Expect(creds.Username).To(BeEmpty())
			Expect(creds.Password).To(BeEmpty())
			Expect(creds.PrivateKey).To(BeNil())
			Expect(creds.KnownHosts).To(BeNil())
		})
	})

	Describe("LoadOCICredentials", func() {
		It("should load basic auth credentials from a secret", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oci-basic-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"username": []byte("registry-user"),
					"password": []byte("registry-pass"),
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

			creds, err := LoadOCICredentials(ctx, fakeClient, "default", "oci-basic-secret")
			Expect(err).NotTo(HaveOccurred())
			Expect(creds.Username).To(Equal("registry-user"))
			Expect(creds.Password).To(Equal("registry-pass"))
			Expect(creds.DockerConfig).To(BeNil())
		})

		It("should load Docker config credentials from a secret", func() {
			dockerConfig := []byte(`{"auths":{"registry.example.com":{"auth":"base64encoded"}}}`)
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oci-docker-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					".dockerconfigjson": dockerConfig,
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

			creds, err := LoadOCICredentials(ctx, fakeClient, "default", "oci-docker-secret")
			Expect(err).NotTo(HaveOccurred())
			Expect(creds.DockerConfig).To(Equal(dockerConfig))
			Expect(creds.Username).To(BeEmpty())
			Expect(creds.Password).To(BeEmpty())
		})

		It("should return error when secret does not exist", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			_, err := LoadOCICredentials(ctx, fakeClient, "default", "nonexistent-secret")
			Expect(err).To(HaveOccurred())
		})

		It("should return empty credentials for secret with no relevant keys", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-oci-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"unrelated-key": []byte("some-value"),
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

			creds, err := LoadOCICredentials(ctx, fakeClient, "default", "empty-oci-secret")
			Expect(err).NotTo(HaveOccurred())
			Expect(creds.Username).To(BeEmpty())
			Expect(creds.Password).To(BeEmpty())
			Expect(creds.DockerConfig).To(BeNil())
		})

		It("should load combined basic auth and Docker config credentials", func() {
			dockerConfig := []byte(`{"auths":{}}`)
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "oci-combined-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"username":          []byte("user"),
					"password":          []byte("pass"),
					".dockerconfigjson": dockerConfig,
				},
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

			creds, err := LoadOCICredentials(ctx, fakeClient, "default", "oci-combined-secret")
			Expect(err).NotTo(HaveOccurred())
			Expect(creds.Username).To(Equal("user"))
			Expect(creds.Password).To(Equal("pass"))
			Expect(creds.DockerConfig).To(Equal(dockerConfig))
		})
	})
})
