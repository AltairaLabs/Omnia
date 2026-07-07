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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

var _ = Describe("ToolRegistry Admission Validation", func() {
	const (
		namespace             = "default"
		defaultBackoffMultStr = "2.0"
	)

	// minimalHandler returns a valid HandlerDefinition with an HTTP config
	// and no retry policy. Callers mutate it to test specific validation rules.
	minimalHandler := func(name string) omniav1alpha1.HandlerDefinition {
		return omniav1alpha1.HandlerDefinition{
			Name: name,
			Type: omniav1alpha1.HandlerTypeHTTP,
			HTTPConfig: &omniav1alpha1.HTTPConfig{
				Endpoint: "http://example.com",
				Method:   "GET",
			},
			Tool: &omniav1alpha1.ToolDefinition{
				Name:        "my_tool",
				Description: "Test tool",
				InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
			},
		}
	}

	Context("Happy path", func() {
		It("should accept a valid HTTP retry policy and round-trip it", func() {
			backoffMult := defaultBackoffMultStr
			handler := minimalHandler("h")
			handler.Timeout = &metav1.Duration{Duration: 30 * time.Second}
			handler.HTTPConfig.RetryPolicy = &omniav1alpha1.HTTPRetryPolicy{
				MaxAttempts:       3,
				BackoffMultiplier: &backoffMult,
				RetryOn:           []int32{502, 503},
			}

			tr := &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "admission-happy",
					Namespace: namespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{handler},
				},
			}

			Expect(k8sClient.Create(ctx, tr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, tr) }()

			fetched := &omniav1alpha1.ToolRegistry{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "admission-happy", Namespace: namespace,
			}, fetched)).To(Succeed())

			rp := fetched.Spec.Handlers[0].HTTPConfig.RetryPolicy
			Expect(rp).NotTo(BeNil())
			Expect(rp.MaxAttempts).To(Equal(int32(3)))
		})
	})

	Context("BackoffMultiplier pattern validation", func() {
		It("should reject a non-numeric backoffMultiplier", func() {
			badMult := "abc"
			handler := minimalHandler("h")
			handler.HTTPConfig.RetryPolicy = &omniav1alpha1.HTTPRetryPolicy{
				MaxAttempts:       3,
				BackoffMultiplier: &badMult,
			}

			tr := &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "admission-bad-mult",
					Namespace: namespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{handler},
				},
			}

			err := k8sClient.Create(ctx, tr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("should match"))
		})
	})

	Context("MaxAttempts range validation", func() {
		It("should reject maxAttempts above 10", func() {
			handler := minimalHandler("h")
			handler.HTTPConfig.RetryPolicy = &omniav1alpha1.HTTPRetryPolicy{
				MaxAttempts: 11,
			}

			tr := &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "admission-max-over",
					Namespace: namespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{handler},
				},
			}

			err := k8sClient.Create(ctx, tr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("10"))
		})

		It("should reject maxAttempts below 1", func() {
			handler := minimalHandler("h")
			handler.HTTPConfig.RetryPolicy = &omniav1alpha1.HTTPRetryPolicy{
				MaxAttempts: 0,
			}

			tr := &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "admission-max-under",
					Namespace: namespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{handler},
				},
			}

			err := k8sClient.Create(ctx, tr)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Kubebuilder defaults", func() {
		It("should populate default initialBackoff, backoffMultiplier, and maxBackoff", func() {
			handler := minimalHandler("h")
			handler.HTTPConfig.RetryPolicy = &omniav1alpha1.HTTPRetryPolicy{
				MaxAttempts: 3,
			}

			tr := &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "admission-defaults",
					Namespace: namespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{handler},
				},
			}

			Expect(k8sClient.Create(ctx, tr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, tr) }()

			fetched := &omniav1alpha1.ToolRegistry{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "admission-defaults", Namespace: namespace,
			}, fetched)).To(Succeed())

			rp := fetched.Spec.Handlers[0].HTTPConfig.RetryPolicy
			Expect(rp).NotTo(BeNil())
			Expect(rp.InitialBackoff).NotTo(BeNil())
			Expect(rp.InitialBackoff.Duration).To(Equal(100 * time.Millisecond))
			Expect(rp.BackoffMultiplier).NotTo(BeNil())
			Expect(*rp.BackoffMultiplier).To(Equal(defaultBackoffMultStr))
			Expect(rp.MaxBackoff).NotTo(BeNil())
			Expect(rp.MaxBackoff.Duration).To(Equal(30 * time.Second))
		})
	})

	Context("Auth stanza validation (CEL)", func() {
		newSecretRef := func() *omniav1alpha1.SecretKeySelector {
			return &omniav1alpha1.SecretKeySelector{Name: "creds", Key: "token"}
		}
		trWith := func(name string, h omniav1alpha1.HandlerDefinition) *omniav1alpha1.ToolRegistry {
			return &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Spec:       omniav1alpha1.ToolRegistrySpec{Handlers: []omniav1alpha1.HandlerDefinition{h}},
			}
		}

		It("accepts a bearer auth stanza with secretRef", func() {
			h := minimalHandler("h")
			h.Auth = &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeBearer, SecretRef: newSecretRef()}
			tr := trWith("auth-ok", h)
			Expect(k8sClient.Create(ctx, tr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, tr) }()
		})

		It("rejects bearer auth without secretRef", func() {
			h := minimalHandler("h")
			h.Auth = &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeBearer}
			err := k8sClient.Create(ctx, trWith("auth-no-secret", h))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("requires secretRef"))
		})

		It("rejects both the auth stanza and a legacy authType", func() {
			h := minimalHandler("h")
			bearer := "bearer"
			h.HTTPConfig.AuthType = &bearer
			h.Auth = &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeBearer, SecretRef: newSecretRef()}
			err := k8sClient.Create(ctx, trWith("auth-both-type", h))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not both"))
		})

		It("rejects both the auth stanza and a legacy authSecretRef without authType", func() {
			// Regression guard: legacy auth was enabled by authSecretRef presence,
			// with authType optional. The both-set rule must catch this shape, else
			// a legacy secretRef + auth:{type:none} silently drops auth.
			h := minimalHandler("h")
			h.HTTPConfig.AuthSecretRef = newSecretRef()
			h.Auth = &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeNone}
			err := k8sClient.Create(ctx, trWith("auth-both-secretref", h))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not both"))
		})

		It("rejects an unsupported auth type (serviceAccount not yet wired)", func() {
			h := minimalHandler("h")
			h.Auth = &omniav1alpha1.ToolAuth{Type: "serviceAccount"}
			err := k8sClient.Create(ctx, trWith("auth-unsupported", h))
			Expect(err).To(HaveOccurred())
		})
	})
})
