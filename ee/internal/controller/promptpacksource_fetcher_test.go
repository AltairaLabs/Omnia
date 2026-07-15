/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// These are plain (non-Ginkgo) unit tests for buildFetcher/gitFetcher/ociFetcher.
// None of the cases here set a SecretRef with a real backing Secret, so no
// network or envtest cluster is required — NewGitFetcher/NewOCIFetcher only
// build config structs; the actual clone/pull happens in Fetch/LatestRevision,
// which are exercised separately via the fakeFetcher-based Ginkgo specs.

func TestBuildFetcher_Git(t *testing.T) {
	r := &PromptPackSourceReconciler{}
	src := &omniav1alpha1.PromptPackSource{
		Spec: omniav1alpha1.PromptPackSourceSpec{
			Type: omniav1alpha1.PromptPackSourceTypeGit,
			Git: &corev1alpha1.GitSource{
				URL:  "https://example.com/repo.git",
				Path: packJSONKey,
				Ref:  &corev1alpha1.GitReference{Branch: "main"},
			},
		},
	}

	fetcher, err := r.buildFetcher(context.Background(), src)
	require.NoError(t, err)
	require.NotNil(t, fetcher)
	assert.Equal(t, "git", fetcher.Type())
}

func TestBuildFetcher_OCI(t *testing.T) {
	r := &PromptPackSourceReconciler{}
	src := &omniav1alpha1.PromptPackSource{
		Spec: omniav1alpha1.PromptPackSourceSpec{
			Type: omniav1alpha1.PromptPackSourceTypeOCI,
			OCI: &corev1alpha1.OCISource{
				URL: "oci://example.com/pack:1.0.0",
			},
		},
	}

	fetcher, err := r.buildFetcher(context.Background(), src)
	require.NoError(t, err)
	require.NotNil(t, fetcher)
	assert.Equal(t, "oci", fetcher.Type())
}

func TestBuildFetcher_UnknownType(t *testing.T) {
	r := &PromptPackSourceReconciler{}
	src := &omniav1alpha1.PromptPackSource{
		Spec: omniav1alpha1.PromptPackSourceSpec{Type: "bogus"},
	}

	fetcher, err := r.buildFetcher(context.Background(), src)
	assert.Error(t, err)
	assert.Nil(t, fetcher)
	assert.Contains(t, err.Error(), "unknown source type")
}

func TestGitFetcher_MissingGitBlock(t *testing.T) {
	r := &PromptPackSourceReconciler{}
	src := &omniav1alpha1.PromptPackSource{
		Spec: omniav1alpha1.PromptPackSourceSpec{Type: omniav1alpha1.PromptPackSourceTypeGit},
	}

	fetcher, err := r.buildFetcher(context.Background(), src)
	assert.Error(t, err)
	assert.Nil(t, fetcher)
	assert.Contains(t, err.Error(), "git source missing spec.git")
}

func TestOCIFetcher_MissingOCIBlock(t *testing.T) {
	r := &PromptPackSourceReconciler{}
	src := &omniav1alpha1.PromptPackSource{
		Spec: omniav1alpha1.PromptPackSourceSpec{Type: omniav1alpha1.PromptPackSourceTypeOCI},
	}

	fetcher, err := r.buildFetcher(context.Background(), src)
	assert.Error(t, err)
	assert.Nil(t, fetcher)
	assert.Contains(t, err.Error(), "oci source missing spec.oci")
}

// newFakeClient builds a scheme-registered fake client with no seeded objects —
// used to exercise the credential-load error branches without a real Secret.
func newFakeClient(t *testing.T) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).Build()
}

func TestGitFetcher_SecretRefNotFound(t *testing.T) {
	r := &PromptPackSourceReconciler{Client: newFakeClient(t)}
	src := &omniav1alpha1.PromptPackSource{
		Spec: omniav1alpha1.PromptPackSourceSpec{
			Type: omniav1alpha1.PromptPackSourceTypeGit,
			Git: &corev1alpha1.GitSource{
				URL:       "https://example.com/repo.git",
				SecretRef: &corev1alpha1.SecretKeyRef{Name: "missing-secret"},
			},
		},
	}

	fetcher, err := r.buildFetcher(context.Background(), src)
	assert.Error(t, err)
	assert.Nil(t, fetcher)
	assert.Contains(t, err.Error(), "load git credentials")
}

func TestOCIFetcher_SecretRefNotFound(t *testing.T) {
	r := &PromptPackSourceReconciler{Client: newFakeClient(t)}
	src := &omniav1alpha1.PromptPackSource{
		Spec: omniav1alpha1.PromptPackSourceSpec{
			Type: omniav1alpha1.PromptPackSourceTypeOCI,
			OCI: &corev1alpha1.OCISource{
				URL:       "oci://example.com/pack:1.0.0",
				SecretRef: &corev1alpha1.SecretKeyRef{Name: "missing-secret"},
			},
		},
	}

	fetcher, err := r.buildFetcher(context.Background(), src)
	assert.Error(t, err)
	assert.Nil(t, fetcher)
	assert.Contains(t, err.Error(), "load oci credentials")
}
