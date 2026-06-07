/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package main

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func newTestSchemeForEmbedding(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	return s
}

// TestCreateEmbeddingProviderFromCRD_Gemini proves the memory-api can build a
// Gemini embedding provider from a Provider CRD + Secret. Without this the
// memory recall path silently falls back to "no embeddings", which the FTS
// keyword path covers but semantic search does not.
func TestCreateEmbeddingProviderFromCRD_Gemini(t *testing.T) {
	t.Parallel()
	provider := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "gemini-embed", Namespace: "dev-agents"},
		Spec: omniav1alpha1.ProviderSpec{
			Type:  omniav1alpha1.ProviderTypeGemini,
			Model: "text-embedding-004",
			Credential: &omniav1alpha1.CredentialConfig{
				SecretRef: &omniav1alpha1.SecretKeyRef{Name: "gemini-key"},
			},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "gemini-key", Namespace: "dev-agents"},
		Data:       map[string][]byte{"GEMINI_API_KEY": []byte("test-key")},
	}
	c := fake.NewClientBuilder().
		WithScheme(newTestSchemeForEmbedding(t)).
		WithObjects(provider, secret).
		Build()

	p, err := createEmbeddingProviderFromCRD(context.Background(), c, provider, "dev-agents", logr.Discard())
	require.NoError(t, err)
	assert.NotNil(t, p, "expected a non-nil EmbeddingProvider for gemini")
}

// TestCreateEmbeddingProviderFromCRD_GeminiMissingSecret returns an error so
// the operator sees a misconfiguration instead of memory-api silently
// shipping with no embeddings.
func TestCreateEmbeddingProviderFromCRD_GeminiMissingSecret(t *testing.T) {
	t.Parallel()
	provider := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "gemini-embed", Namespace: "dev-agents"},
		Spec: omniav1alpha1.ProviderSpec{
			Type:  omniav1alpha1.ProviderTypeGemini,
			Model: "text-embedding-004",
			Credential: &omniav1alpha1.CredentialConfig{
				SecretRef: &omniav1alpha1.SecretKeyRef{Name: "missing"},
			},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(newTestSchemeForEmbedding(t)).
		WithObjects(provider).
		Build()

	_, err := createEmbeddingProviderFromCRD(context.Background(), c, provider, "dev-agents", logr.Discard())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

// TestCreateEmbeddingProviderFromCRD_GeminiMissingCredential rejects a
// Provider that doesn't reference any Secret — the chat path silently
// works without one (env-var fallback) but the embedding path needs the
// API key passed in directly to PromptKit's NewEmbeddingProvider.
func TestCreateEmbeddingProviderFromCRD_GeminiMissingCredential(t *testing.T) {
	t.Parallel()
	provider := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "gemini-embed", Namespace: "dev-agents"},
		Spec: omniav1alpha1.ProviderSpec{
			Type:  omniav1alpha1.ProviderTypeGemini,
			Model: "text-embedding-004",
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(newTestSchemeForEmbedding(t)).
		WithObjects(provider).
		Build()

	_, err := createEmbeddingProviderFromCRD(context.Background(), c, provider, "dev-agents", logr.Discard())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credential")
}

// TestCreateEmbeddingProviderFromCRD_OllamaStillWorks guards against
// regressing the existing ollama path while adding gemini support.
func TestCreateEmbeddingProviderFromCRD_OllamaStillWorks(t *testing.T) {
	t.Parallel()
	provider := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "ollama", Namespace: "dev-agents"},
		Spec: omniav1alpha1.ProviderSpec{
			Type:    omniav1alpha1.ProviderTypeOllama,
			Model:   "nomic-embed-text",
			BaseURL: "http://ollama:11434",
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(newTestSchemeForEmbedding(t)).
		WithObjects(provider).
		Build()

	p, err := createEmbeddingProviderFromCRD(context.Background(), c, provider, "dev-agents", logr.Discard())
	require.NoError(t, err)
	assert.NotNil(t, p)
}

// TestCreateEmbeddingProviderFromCRD_AzureWorkloadIdentity proves the keyless
// path: an openai embedding Provider hosted on Azure with auth=workloadIdentity
// builds with NO Secret present. embeddingCredentialForCRD returns an
// AzureCredential (token applied per request) instead of reading a secretRef,
// and the platform/config is passed to PromptKit so it resolves the Azure
// endpoint. Regressing this silently breaks keyless Azure OpenAI embeddings.
func TestCreateEmbeddingProviderFromCRD_AzureWorkloadIdentity(t *testing.T) {
	t.Parallel()
	provider := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "azure-embed", Namespace: "dev-agents"},
		Spec: omniav1alpha1.ProviderSpec{
			Type:  omniav1alpha1.ProviderTypeOpenAI,
			Role:  omniav1alpha1.ProviderRoleEmbedding,
			Model: "text-embedding-3-small",
			Platform: &omniav1alpha1.PlatformConfig{
				Type:     omniav1alpha1.PlatformTypeAzure,
				Endpoint: "https://example.openai.azure.com",
			},
			Auth: &omniav1alpha1.AuthConfig{
				Type: omniav1alpha1.AuthMethodWorkloadIdentity,
			},
		},
	}
	// No Secret object — the WI path must not require one.
	c := fake.NewClientBuilder().
		WithScheme(newTestSchemeForEmbedding(t)).
		WithObjects(provider).
		Build()

	p, err := createEmbeddingProviderFromCRD(context.Background(), c, provider, "dev-agents", logr.Discard())
	require.NoError(t, err, "keyless Azure WI embedding provider must build without a Secret")
	assert.NotNil(t, p)
}

// TestEmbeddingCredentialForCRD_WorkloadIdentityNeedsNoSecret asserts the
// credential helper bypasses the secretRef requirement for workload identity —
// the unit-level contract behind the integration test above.
func TestEmbeddingCredentialForCRD_WorkloadIdentityNeedsNoSecret(t *testing.T) {
	t.Parallel()
	provider := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "azure-embed", Namespace: "dev-agents"},
		Spec: omniav1alpha1.ProviderSpec{
			Type: omniav1alpha1.ProviderTypeOpenAI,
			Role: omniav1alpha1.ProviderRoleEmbedding,
			Platform: &omniav1alpha1.PlatformConfig{
				Type:     omniav1alpha1.PlatformTypeAzure,
				Endpoint: "https://example.openai.azure.com",
			},
			Auth: &omniav1alpha1.AuthConfig{Type: omniav1alpha1.AuthMethodWorkloadIdentity},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(newTestSchemeForEmbedding(t)).
		WithObjects(provider).
		Build()

	cred, err := embeddingCredentialForCRD(context.Background(), c, provider, "dev-agents")
	require.NoError(t, err)
	require.NotNil(t, cred)
	assert.Equal(t, "azure", cred.Type(), "expected an Azure (token) credential, not an API key")
}

// TestCreateEmbeddingProviderFromCRD_UnsupportedType verifies that
// types PromptKit doesn't have a registered embedding factory for
// (e.g. claude — it has no embedding model) surface the factory's
// error rather than silently falling through. We give the provider a
// credential so the test exercises the factory-dispatch path, not the
// upstream credential check.
func TestCreateEmbeddingProviderFromCRD_UnsupportedType(t *testing.T) {
	t.Parallel()
	provider := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "claude", Namespace: "dev-agents"},
		Spec: omniav1alpha1.ProviderSpec{
			Type:  omniav1alpha1.ProviderTypeClaude,
			Model: "claude-sonnet",
			Credential: &omniav1alpha1.CredentialConfig{
				SecretRef: &omniav1alpha1.SecretKeyRef{Name: "claude-key"},
			},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "claude-key", Namespace: "dev-agents"},
		Data:       map[string][]byte{"ANTHROPIC_API_KEY": []byte("test-key")},
	}
	c := fake.NewClientBuilder().
		WithScheme(newTestSchemeForEmbedding(t)).
		WithObjects(provider, secret).
		Build()

	_, err := createEmbeddingProviderFromCRD(context.Background(), c, provider, "dev-agents", logr.Discard())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported embedding provider type")
}
