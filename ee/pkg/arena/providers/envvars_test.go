/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestBuildEnvVarsFromProviders(t *testing.T) {
	t.Run("openai provider with secretRef", func(t *testing.T) {
		providers := []*corev1alpha1.Provider{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-openai",
					Namespace: "test-ns",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type: "openai",
					SecretRef: &corev1alpha1.SecretKeyRef{
						Name: "openai-credentials",
					},
				},
			},
		}

		envVars := BuildEnvVarsFromProviders(providers)

		require.Len(t, envVars, 1)
		assert.Equal(t, "OPENAI_API_KEY", envVars[0].Name)
		assert.NotNil(t, envVars[0].ValueFrom)
		assert.NotNil(t, envVars[0].ValueFrom.SecretKeyRef)
		assert.Equal(t, "openai-credentials", envVars[0].ValueFrom.SecretKeyRef.Name)
		assert.Equal(t, "value", envVars[0].ValueFrom.SecretKeyRef.Key)
		assert.True(t, *envVars[0].ValueFrom.SecretKeyRef.Optional)
	})

	t.Run("provider with custom secret key", func(t *testing.T) {
		providers := []*corev1alpha1.Provider{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-claude",
					Namespace: "test-ns",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type: "claude",
					SecretRef: &corev1alpha1.SecretKeyRef{
						Name: "my-secret",
						Key:  ptr.To("api-key"),
					},
				},
			},
		}

		envVars := BuildEnvVarsFromProviders(providers)

		require.Len(t, envVars, 1)
		assert.Equal(t, "ANTHROPIC_API_KEY", envVars[0].Name)
		assert.Equal(t, "my-secret", envVars[0].ValueFrom.SecretKeyRef.Name)
		assert.Equal(t, "api-key", envVars[0].ValueFrom.SecretKeyRef.Key)
	})

	t.Run("provider without secretRef uses default naming", func(t *testing.T) {
		providers := []*corev1alpha1.Provider{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-openai",
					Namespace: "test-ns",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type: "openai",
					// No SecretRef - should use default secret name
				},
			},
		}

		envVars := BuildEnvVarsFromProviders(providers)

		require.Len(t, envVars, 1)
		assert.Equal(t, "OPENAI_API_KEY", envVars[0].Name)
		assert.Equal(t, "openai-api-key", envVars[0].ValueFrom.SecretKeyRef.Name)
		assert.Equal(t, "value", envVars[0].ValueFrom.SecretKeyRef.Key)
	})

	t.Run("mock provider returns no env vars", func(t *testing.T) {
		providers := []*corev1alpha1.Provider{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-mock",
					Namespace: "test-ns",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type: "mock",
				},
			},
		}

		envVars := BuildEnvVarsFromProviders(providers)

		assert.Len(t, envVars, 0)
	})

	t.Run("ollama provider returns no env vars", func(t *testing.T) {
		providers := []*corev1alpha1.Provider{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "local-ollama",
					Namespace: "test-ns",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type: "ollama",
				},
			},
		}

		envVars := BuildEnvVarsFromProviders(providers)

		assert.Len(t, envVars, 0)
	})

	t.Run("multiple providers of same type deduplicate env vars", func(t *testing.T) {
		providers := []*corev1alpha1.Provider{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openai-1",
					Namespace: "test-ns",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type: "openai",
					SecretRef: &corev1alpha1.SecretKeyRef{
						Name: "openai-creds-1",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openai-2",
					Namespace: "test-ns",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type: "openai",
					SecretRef: &corev1alpha1.SecretKeyRef{
						Name: "openai-creds-2",
					},
				},
			},
		}

		envVars := BuildEnvVarsFromProviders(providers)

		// Should only have one OPENAI_API_KEY, from the first provider
		require.Len(t, envVars, 1)
		assert.Equal(t, "OPENAI_API_KEY", envVars[0].Name)
		assert.Equal(t, "openai-creds-1", envVars[0].ValueFrom.SecretKeyRef.Name)
	})

	t.Run("multiple different providers", func(t *testing.T) {
		providers := []*corev1alpha1.Provider{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-openai",
					Namespace: "test-ns",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type: "openai",
					SecretRef: &corev1alpha1.SecretKeyRef{
						Name: "openai-creds",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-claude",
					Namespace: "test-ns",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type: "claude",
					SecretRef: &corev1alpha1.SecretKeyRef{
						Name: "anthropic-creds",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-mock",
					Namespace: "test-ns",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type: "mock",
				},
			},
		}

		envVars := BuildEnvVarsFromProviders(providers)

		// Should have env vars for openai and claude (mock needs none)
		require.Len(t, envVars, 2)

		envVarNames := make(map[string]string)
		for _, ev := range envVars {
			envVarNames[ev.Name] = ev.ValueFrom.SecretKeyRef.Name
		}

		assert.Equal(t, "openai-creds", envVarNames["OPENAI_API_KEY"])
		assert.Equal(t, "anthropic-creds", envVarNames["ANTHROPIC_API_KEY"])
	})

	t.Run("gemini provider creates multiple env vars", func(t *testing.T) {
		providers := []*corev1alpha1.Provider{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-gemini",
					Namespace: "test-ns",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type: "gemini",
					SecretRef: &corev1alpha1.SecretKeyRef{
						Name: "google-creds",
					},
				},
			},
		}

		envVars := BuildEnvVarsFromProviders(providers)

		// Gemini has both GOOGLE_API_KEY and GEMINI_API_KEY
		require.Len(t, envVars, 2)

		envVarNames := make(map[string]bool)
		for _, ev := range envVars {
			envVarNames[ev.Name] = true
		}

		assert.True(t, envVarNames["GOOGLE_API_KEY"])
		assert.True(t, envVarNames["GEMINI_API_KEY"])
	})

	t.Run("provider with credential.secretRef uses it instead of legacy secretRef", func(t *testing.T) {
		providers := []*corev1alpha1.Provider{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-openai",
					Namespace: "test-ns",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type: "openai",
					Credential: &corev1alpha1.CredentialConfig{
						SecretRef: &corev1alpha1.SecretKeyRef{
							Name: "new-openai-creds",
						},
					},
				},
			},
		}

		envVars := BuildEnvVarsFromProviders(providers)

		require.Len(t, envVars, 1)
		assert.Equal(t, "OPENAI_API_KEY", envVars[0].Name)
		assert.NotNil(t, envVars[0].ValueFrom)
		assert.NotNil(t, envVars[0].ValueFrom.SecretKeyRef)
		assert.Equal(t, "new-openai-creds", envVars[0].ValueFrom.SecretKeyRef.Name)
	})

	t.Run("provider with credential.envVar skips env var creation", func(t *testing.T) {
		providers := []*corev1alpha1.Provider{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-claude",
					Namespace: "test-ns",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type: "claude",
					Credential: &corev1alpha1.CredentialConfig{
						EnvVar: "MY_CUSTOM_KEY",
					},
				},
			},
		}

		envVars := BuildEnvVarsFromProviders(providers)

		assert.Len(t, envVars, 0)
	})

	t.Run("provider with credential.filePath skips env var creation", func(t *testing.T) {
		providers := []*corev1alpha1.Provider{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-claude",
					Namespace: "test-ns",
				},
				Spec: corev1alpha1.ProviderSpec{
					Type: "claude",
					Credential: &corev1alpha1.CredentialConfig{
						FilePath: "/var/secrets/api-key",
					},
				},
			},
		}

		envVars := BuildEnvVarsFromProviders(providers)

		assert.Len(t, envVars, 0)
	})

	t.Run("empty providers list", func(t *testing.T) {
		envVars := BuildEnvVarsFromProviders([]*corev1alpha1.Provider{})
		assert.Len(t, envVars, 0)
	})

	t.Run("nil providers list", func(t *testing.T) {
		envVars := BuildEnvVarsFromProviders(nil)
		assert.Len(t, envVars, 0)
	})
}

func TestFlattenProviderGroups(t *testing.T) {
	t.Run("flattens multiple groups", func(t *testing.T) {
		providersByGroup := map[string][]*corev1alpha1.Provider{
			"group1": {
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "provider-a",
						Namespace: "ns1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "provider-b",
						Namespace: "ns1",
					},
				},
			},
			"group2": {
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "provider-c",
						Namespace: "ns1",
					},
				},
			},
		}

		result := FlattenProviderGroups(providersByGroup)

		assert.Len(t, result, 3)
	})

	t.Run("deduplicates providers across groups", func(t *testing.T) {
		sharedProvider := &corev1alpha1.Provider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "shared-provider",
				Namespace: "ns1",
			},
		}

		providersByGroup := map[string][]*corev1alpha1.Provider{
			"group1": {
				sharedProvider,
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "provider-a",
						Namespace: "ns1",
					},
				},
			},
			"group2": {
				sharedProvider, // Same provider in both groups
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "provider-b",
						Namespace: "ns1",
					},
				},
			},
		}

		result := FlattenProviderGroups(providersByGroup)

		// Should have 3 unique providers, not 4
		assert.Len(t, result, 3)
	})

	t.Run("same name different namespace are distinct", func(t *testing.T) {
		providersByGroup := map[string][]*corev1alpha1.Provider{
			"group1": {
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "provider-a",
						Namespace: "ns1",
					},
				},
			},
			"group2": {
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "provider-a",
						Namespace: "ns2", // Different namespace
					},
				},
			},
		}

		result := FlattenProviderGroups(providersByGroup)

		// Both should be included since they're in different namespaces
		assert.Len(t, result, 2)
	})

	t.Run("empty groups map", func(t *testing.T) {
		result := FlattenProviderGroups(map[string][]*corev1alpha1.Provider{})
		assert.Nil(t, result)
	})

	t.Run("nil groups map", func(t *testing.T) {
		result := FlattenProviderGroups(nil)
		assert.Nil(t, result)
	})

	t.Run("empty group in map", func(t *testing.T) {
		providersByGroup := map[string][]*corev1alpha1.Provider{
			"group1": {},
			"group2": {
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "provider-a",
						Namespace: "ns1",
					},
				},
			},
		}

		result := FlattenProviderGroups(providersByGroup)

		assert.Len(t, result, 1)
	})
}
