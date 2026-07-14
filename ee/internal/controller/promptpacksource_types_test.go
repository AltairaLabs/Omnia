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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

var _ = Describe("PromptPackSource CEL validation", func() {
	const namespace = "default"

	ctx := context.Background()

	deleteIfExists := func(name string) {
		resource := &omniav1alpha1.PromptPackSource{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, resource); err == nil {
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		}
	}

	Context("valid git source", func() {
		const name = "cel-valid-git"

		AfterEach(func() { deleteIfExists(name) })

		It("is accepted", func() {
			source := &omniav1alpha1.PromptPackSource{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Spec: omniav1alpha1.PromptPackSourceSpec{
					Type:     omniav1alpha1.PromptPackSourceTypeGit,
					Git:      &corev1alpha1.GitSource{URL: "https://example.com/repo.git", Path: "packs/mypack"},
					PackName: "mypack",
					Interval: "5m",
				},
			}
			Expect(k8sClient.Create(ctx, source)).To(Succeed())
		})
	})

	Context("git type without a git block", func() {
		const name = "cel-git-missing-block"

		AfterEach(func() { deleteIfExists(name) })

		It("is rejected", func() {
			source := &omniav1alpha1.PromptPackSource{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Spec: omniav1alpha1.PromptPackSourceSpec{
					Type:     omniav1alpha1.PromptPackSourceTypeGit,
					PackName: "mypack",
					Interval: "5m",
				},
			}
			err := k8sClient.Create(ctx, source)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exactly the source block matching type must be set"))
		})
	})

	Context("git type with the oci block also set", func() {
		const name = "cel-git-with-oci-block"

		AfterEach(func() { deleteIfExists(name) })

		It("is rejected", func() {
			source := &omniav1alpha1.PromptPackSource{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Spec: omniav1alpha1.PromptPackSourceSpec{
					Type:     omniav1alpha1.PromptPackSourceTypeGit,
					Git:      &corev1alpha1.GitSource{URL: "https://example.com/repo.git", Path: "packs/mypack"},
					OCI:      &corev1alpha1.OCISource{URL: "oci://registry.example.com/mypack:latest"},
					PackName: "mypack",
					Interval: "5m",
				},
			}
			err := k8sClient.Create(ctx, source)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exactly the source block matching type must be set"))
		})
	})

	Context("invalid interval without a unit", func() {
		const name = "cel-bad-interval"

		AfterEach(func() { deleteIfExists(name) })

		It("is rejected by the pattern", func() {
			source := &omniav1alpha1.PromptPackSource{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Spec: omniav1alpha1.PromptPackSourceSpec{
					Type:     omniav1alpha1.PromptPackSourceTypeGit,
					Git:      &corev1alpha1.GitSource{URL: "https://example.com/repo.git", Path: "packs/mypack"},
					PackName: "mypack",
					Interval: "5",
				},
			}
			err := k8sClient.Create(ctx, source)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("interval"))
		})
	})

	Context("valid oci source", func() {
		const name = "cel-valid-oci"

		AfterEach(func() { deleteIfExists(name) })

		It("is accepted", func() {
			source := &omniav1alpha1.PromptPackSource{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Spec: omniav1alpha1.PromptPackSourceSpec{
					Type:     omniav1alpha1.PromptPackSourceTypeOCI,
					OCI:      &corev1alpha1.OCISource{URL: "oci://registry.example.com/mypack:latest"},
					PackName: "mypack",
					Interval: "5m",
				},
			}
			Expect(k8sClient.Create(ctx, source)).To(Succeed())
		})
	})
})
