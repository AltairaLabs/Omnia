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

var _ = Describe("GetWorkspaceForNamespace", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	It("should return workspace name from namespace label", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-namespace",
				Labels: map[string]string{
					labelWorkspace: "my-workspace",
				},
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

		result := GetWorkspaceForNamespace(ctx, fakeClient, "my-namespace")
		Expect(result).To(Equal("my-workspace"))
	})

	It("should fallback to namespace name when label is missing", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "no-label-ns",
				Labels: map[string]string{},
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

		result := GetWorkspaceForNamespace(ctx, fakeClient, "no-label-ns")
		Expect(result).To(Equal("no-label-ns"))
	})

	It("should fallback to namespace name when namespace does not exist", func() {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		result := GetWorkspaceForNamespace(ctx, fakeClient, "nonexistent-ns")
		Expect(result).To(Equal("nonexistent-ns"))
	})

	It("should fallback to namespace name when client is nil", func() {
		result := GetWorkspaceForNamespace(ctx, nil, "any-namespace")
		Expect(result).To(Equal("any-namespace"))
	})

	It("should fallback when workspace label is empty string", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "empty-label-ns",
				Labels: map[string]string{
					labelWorkspace: "",
				},
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()

		result := GetWorkspaceForNamespace(ctx, fakeClient, "empty-label-ns")
		Expect(result).To(Equal("empty-label-ns"))
	})
})
