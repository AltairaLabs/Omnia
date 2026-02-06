/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package server

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// newTestK8sProviderLoader creates a K8sProviderLoader with a fake client for testing.
func newTestK8sProviderLoader(t *testing.T, namespace string, objs ...runtime.Object) *K8sProviderLoader {
	scheme := runtime.NewScheme()
	err := corev1alpha1.AddToScheme(scheme)
	require.NoError(t, err)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()

	return &K8sProviderLoader{
		client:    fakeClient,
		log:       logr.Discard(),
		namespace: namespace,
	}
}

// TestK8sProviderLoaderNamespace tests the Namespace method.
func TestK8sProviderLoaderNamespace(t *testing.T) {
	loader := newTestK8sProviderLoader(t, "test-namespace")
	assert.Equal(t, "test-namespace", loader.Namespace())
}

// TestLoadProvidersEmpty tests loading providers when none exist.
func TestLoadProvidersEmpty(t *testing.T) {
	loader := newTestK8sProviderLoader(t, "test-namespace")

	providers, err := loader.LoadProviders(context.Background())
	require.NoError(t, err)
	assert.Empty(t, providers)
}

// TestLoadProvidersWithReadyProvider tests loading a ready provider.
func TestLoadProvidersWithReadyProvider(t *testing.T) {
	provider := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-provider",
			Namespace: "test-namespace",
		},
		Spec: corev1alpha1.ProviderSpec{
			Type:  corev1alpha1.ProviderTypeOpenAI,
			Model: "gpt-4",
		},
		Status: corev1alpha1.ProviderStatus{
			Phase: corev1alpha1.ProviderPhaseReady,
		},
	}

	loader := newTestK8sProviderLoader(t, "test-namespace", provider)

	providers, err := loader.LoadProviders(context.Background())
	require.NoError(t, err)
	require.Len(t, providers, 1)
	assert.Contains(t, providers, "test-provider")
	assert.Equal(t, "openai", providers["test-provider"].Type)
	assert.Equal(t, "gpt-4", providers["test-provider"].Model)
}

// TestLoadProvidersSkipsNotReady tests that non-ready providers are skipped.
func TestLoadProvidersSkipsNotReady(t *testing.T) {
	readyProvider := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ready-provider",
			Namespace: "test-namespace",
		},
		Spec: corev1alpha1.ProviderSpec{
			Type:  corev1alpha1.ProviderTypeOpenAI,
			Model: "gpt-4",
		},
		Status: corev1alpha1.ProviderStatus{
			Phase: corev1alpha1.ProviderPhaseReady,
		},
	}

	errorProvider := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "error-provider",
			Namespace: "test-namespace",
		},
		Spec: corev1alpha1.ProviderSpec{
			Type:  corev1alpha1.ProviderTypeClaude,
			Model: "claude-3",
		},
		Status: corev1alpha1.ProviderStatus{
			Phase: corev1alpha1.ProviderPhaseError,
		},
	}

	loader := newTestK8sProviderLoader(t, "test-namespace", readyProvider, errorProvider)

	providers, err := loader.LoadProviders(context.Background())
	require.NoError(t, err)
	require.Len(t, providers, 1)
	assert.Contains(t, providers, "ready-provider")
	assert.NotContains(t, providers, "error-provider")
}

// TestLoadProvidersForNamespaceSecurityCheck tests that cross-namespace access is denied.
func TestLoadProvidersForNamespaceSecurityCheck(t *testing.T) {
	loader := newTestK8sProviderLoader(t, "allowed-namespace")

	_, err := loader.LoadProvidersForNamespace(context.Background(), "other-namespace")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot access providers in namespace")
}

// TestLoadProvidersForNamespaceEmptyFallback tests empty namespace falls back to loader's namespace.
func TestLoadProvidersForNamespaceEmptyFallback(t *testing.T) {
	provider := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-provider",
			Namespace: "test-namespace",
		},
		Spec: corev1alpha1.ProviderSpec{
			Type:  corev1alpha1.ProviderTypeOpenAI,
			Model: "gpt-4",
		},
		Status: corev1alpha1.ProviderStatus{
			Phase: corev1alpha1.ProviderPhaseReady,
		},
	}

	loader := newTestK8sProviderLoader(t, "test-namespace", provider)

	// Empty namespace should fall back to loader's namespace
	providers, err := loader.LoadProvidersForNamespace(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, providers, 1)
}

// TestConvertProviderBasic tests basic provider conversion.
func TestConvertProviderBasic(t *testing.T) {
	loader := newTestK8sProviderLoader(t, "test-namespace")

	provider := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-provider",
			Namespace: "test-namespace",
		},
		Spec: corev1alpha1.ProviderSpec{
			Type:    corev1alpha1.ProviderTypeOpenAI,
			Model:   "gpt-4-turbo",
			BaseURL: "https://api.openai.com/v1",
		},
	}

	result := loader.convertProvider(provider)

	assert.Equal(t, "my-provider", result.ID)
	assert.Equal(t, "openai", result.Type)
	assert.Equal(t, "gpt-4-turbo", result.Model)
	assert.Equal(t, "https://api.openai.com/v1", result.BaseURL)
}

// TestConvertProviderWithDefaults tests provider conversion with defaults.
func TestConvertProviderWithDefaults(t *testing.T) {
	loader := newTestK8sProviderLoader(t, "test-namespace")

	temp := "0.7"
	maxTokens := int32(2048)
	provider := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "provider-with-defaults",
			Namespace: "test-namespace",
		},
		Spec: corev1alpha1.ProviderSpec{
			Type:  corev1alpha1.ProviderTypeClaude,
			Model: "claude-3-opus",
			Defaults: &corev1alpha1.ProviderDefaults{
				Temperature: &temp,
				MaxTokens:   &maxTokens,
			},
		},
	}

	result := loader.convertProvider(provider)

	assert.Equal(t, "claude", result.Type)
	assert.InDelta(t, 0.7, float64(result.Defaults.Temperature), 0.01)
	assert.Equal(t, 2048, result.Defaults.MaxTokens)
}

// TestConvertProviderWithInvalidTemperature tests handling invalid temperature.
func TestConvertProviderWithInvalidTemperature(t *testing.T) {
	loader := newTestK8sProviderLoader(t, "test-namespace")

	invalidTemp := "not-a-number"
	provider := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "provider-invalid-temp",
			Namespace: "test-namespace",
		},
		Spec: corev1alpha1.ProviderSpec{
			Type:  corev1alpha1.ProviderTypeOpenAI,
			Model: "gpt-4",
			Defaults: &corev1alpha1.ProviderDefaults{
				Temperature: &invalidTemp,
			},
		},
	}

	result := loader.convertProvider(provider)

	// Temperature should remain at zero value since parsing failed
	assert.Equal(t, float32(0), result.Defaults.Temperature)
}

// TestConvertProviderWithCredentialEnv tests provider with credential env var.
func TestConvertProviderWithCredentialEnv(t *testing.T) {
	// Set the environment variable that would be set by the controller
	t.Setenv("OPENAI_API_KEY", "test-key")

	loader := newTestK8sProviderLoader(t, "test-namespace")

	provider := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openai-provider",
			Namespace: "test-namespace",
		},
		Spec: corev1alpha1.ProviderSpec{
			Type:  corev1alpha1.ProviderTypeOpenAI,
			Model: "gpt-4",
		},
	}

	result := loader.convertProvider(provider)

	require.NotNil(t, result.Credential)
	assert.Equal(t, "OPENAI_API_KEY", result.Credential.CredentialEnv)
}

// TestConvertProviderWithoutCredentialEnv tests provider when credential env is not set.
func TestConvertProviderWithoutCredentialEnv(t *testing.T) {
	// Make sure the env var is not set (t.Setenv will restore it after the test)
	t.Setenv("OPENAI_API_KEY", "")

	loader := newTestK8sProviderLoader(t, "test-namespace")

	provider := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openai-provider",
			Namespace: "test-namespace",
		},
		Spec: corev1alpha1.ProviderSpec{
			Type:  corev1alpha1.ProviderTypeOpenAI,
			Model: "gpt-4",
		},
	}

	result := loader.convertProvider(provider)

	// Credential should be nil when env var is not set
	assert.Nil(t, result.Credential)
}

// TestNewK8sProviderLoaderMissingEnv tests that loader fails without POD_NAMESPACE.
func TestNewK8sProviderLoaderMissingEnv(t *testing.T) {
	// Ensure POD_NAMESPACE is not set (t.Setenv will restore it after the test)
	t.Setenv("POD_NAMESPACE", "")

	_, err := NewK8sProviderLoader(logr.Discard())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "POD_NAMESPACE environment variable not set")
}

// TestLoadProvidersMultiple tests loading multiple providers.
func TestLoadProvidersMultiple(t *testing.T) {
	providers := []runtime.Object{
		&corev1alpha1.Provider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openai-provider",
				Namespace: "test-namespace",
			},
			Spec: corev1alpha1.ProviderSpec{
				Type:  corev1alpha1.ProviderTypeOpenAI,
				Model: "gpt-4",
			},
			Status: corev1alpha1.ProviderStatus{
				Phase: corev1alpha1.ProviderPhaseReady,
			},
		},
		&corev1alpha1.Provider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "claude-provider",
				Namespace: "test-namespace",
			},
			Spec: corev1alpha1.ProviderSpec{
				Type:  corev1alpha1.ProviderTypeClaude,
				Model: "claude-3",
			},
			Status: corev1alpha1.ProviderStatus{
				Phase: corev1alpha1.ProviderPhaseReady,
			},
		},
	}

	loader := newTestK8sProviderLoader(t, "test-namespace", providers...)

	result, err := loader.LoadProviders(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Contains(t, result, "openai-provider")
	assert.Contains(t, result, "claude-provider")
}

// TestConvertProviderWithCredentialEnvVar tests provider with credential.envVar set.
func TestConvertProviderWithCredentialEnvVar(t *testing.T) {
	t.Setenv("CUSTOM_API_KEY", "test-key-value")

	loader := newTestK8sProviderLoader(t, "test-namespace")

	provider := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "envvar-provider",
			Namespace: "test-namespace",
		},
		Spec: corev1alpha1.ProviderSpec{
			Type:  corev1alpha1.ProviderTypeClaude,
			Model: "claude-3",
			Credential: &corev1alpha1.CredentialConfig{
				EnvVar: "CUSTOM_API_KEY",
			},
		},
	}

	result := loader.convertProvider(provider)

	require.NotNil(t, result.Credential)
	assert.Equal(t, "CUSTOM_API_KEY", result.Credential.CredentialEnv)
}

// TestConvertProviderWithCredentialEnvVarNotSet tests provider with credential.envVar when env is not set.
func TestConvertProviderWithCredentialEnvVarNotSet(t *testing.T) {
	t.Setenv("CUSTOM_API_KEY", "")

	loader := newTestK8sProviderLoader(t, "test-namespace")

	provider := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "envvar-provider-unset",
			Namespace: "test-namespace",
		},
		Spec: corev1alpha1.ProviderSpec{
			Type:  corev1alpha1.ProviderTypeClaude,
			Model: "claude-3",
			Credential: &corev1alpha1.CredentialConfig{
				EnvVar: "CUSTOM_API_KEY",
			},
		},
	}

	result := loader.convertProvider(provider)

	assert.Nil(t, result.Credential)
}

// TestConvertProviderWithCredentialFilePath tests provider with credential.filePath.
func TestConvertProviderWithCredentialFilePath(t *testing.T) {
	loader := newTestK8sProviderLoader(t, "test-namespace")

	provider := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "filepath-provider",
			Namespace: "test-namespace",
		},
		Spec: corev1alpha1.ProviderSpec{
			Type:  corev1alpha1.ProviderTypeClaude,
			Model: "claude-3",
			Credential: &corev1alpha1.CredentialConfig{
				FilePath: "/var/secrets/api-key",
			},
		},
	}

	result := loader.convertProvider(provider)

	require.NotNil(t, result.Credential)
	assert.Equal(t, "/var/secrets/api-key", result.Credential.CredentialFile)
}

// TestConvertProviderWithCredentialSecretRef tests provider with credential.secretRef and env set.
func TestConvertProviderWithCredentialSecretRef(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key-from-secret")

	loader := newTestK8sProviderLoader(t, "test-namespace")

	provider := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secretref-provider",
			Namespace: "test-namespace",
		},
		Spec: corev1alpha1.ProviderSpec{
			Type:  corev1alpha1.ProviderTypeClaude,
			Model: "claude-3",
			Credential: &corev1alpha1.CredentialConfig{
				SecretRef: &corev1alpha1.SecretKeyRef{
					Name: "my-secret",
				},
			},
		},
	}

	result := loader.convertProvider(provider)

	require.NotNil(t, result.Credential)
	assert.Equal(t, "ANTHROPIC_API_KEY", result.Credential.CredentialEnv)
}

// TestConvertProviderWithCredentialSecretRefNotSet tests provider with credential.secretRef when env is not set.
func TestConvertProviderWithCredentialSecretRefNotSet(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	loader := newTestK8sProviderLoader(t, "test-namespace")

	provider := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secretref-provider-unset",
			Namespace: "test-namespace",
		},
		Spec: corev1alpha1.ProviderSpec{
			Type:  corev1alpha1.ProviderTypeClaude,
			Model: "claude-3",
			Credential: &corev1alpha1.CredentialConfig{
				SecretRef: &corev1alpha1.SecretKeyRef{
					Name: "my-secret",
				},
			},
		},
	}

	result := loader.convertProvider(provider)

	assert.Nil(t, result.Credential)
}

// TestConvertProviderWithLegacySecretRef tests provider with legacy top-level secretRef.
func TestConvertProviderWithLegacySecretRef(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-legacy-key")

	loader := newTestK8sProviderLoader(t, "test-namespace")

	provider := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-provider",
			Namespace: "test-namespace",
		},
		Spec: corev1alpha1.ProviderSpec{
			Type:  corev1alpha1.ProviderTypeOpenAI,
			Model: "gpt-4",
			SecretRef: &corev1alpha1.SecretKeyRef{
				Name: "my-secret",
			},
		},
	}

	result := loader.convertProvider(provider)

	require.NotNil(t, result.Credential)
	assert.Equal(t, "OPENAI_API_KEY", result.Credential.CredentialEnv)
}

// TestConvertProviderWithLegacySecretRefNotSet tests legacy secretRef when env is not set.
func TestConvertProviderWithLegacySecretRefNotSet(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	loader := newTestK8sProviderLoader(t, "test-namespace")

	provider := &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-provider-unset",
			Namespace: "test-namespace",
		},
		Spec: corev1alpha1.ProviderSpec{
			Type:  corev1alpha1.ProviderTypeOpenAI,
			Model: "gpt-4",
			SecretRef: &corev1alpha1.SecretKeyRef{
				Name: "my-secret",
			},
		},
	}

	result := loader.convertProvider(provider)

	assert.Nil(t, result.Credential)
}

// TestDevConsoleConstants tests that the constants are set correctly.
func TestDevConsoleConstants(t *testing.T) {
	assert.Equal(t, "/tmp/arena-dev-console-output", devConsoleOutputDir)
	assert.Equal(t, "/tmp/arena-dev-console", devConsoleConfigDir)
}
