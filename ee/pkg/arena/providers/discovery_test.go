/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package providers

import (
	"context"
	"os"
	"testing"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetAPIKeyEnvVars(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
		expected     []string
	}{
		{
			name:         "claude provider",
			providerType: "claude",
			expected:     []string{"ANTHROPIC_API_KEY"},
		},
		{
			name:         "openai provider",
			providerType: "openai",
			expected:     []string{"OPENAI_API_KEY"},
		},
		{
			name:         "gemini provider has fallback",
			providerType: "gemini",
			expected:     []string{"GOOGLE_API_KEY", "GEMINI_API_KEY"},
		},
		{
			name:         "mock provider no credentials",
			providerType: "mock",
			expected:     []string{},
		},
		{
			name:         "ollama no credentials",
			providerType: "ollama",
			expected:     []string{},
		},
		{
			name:         "unknown provider gets standard naming",
			providerType: "newprovider",
			expected:     []string{"NEWPROVIDER_API_KEY"},
		},
		{
			name:         "case insensitive",
			providerType: "CLAUDE",
			expected:     []string{"ANTHROPIC_API_KEY"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetAPIKeyEnvVars(tt.providerType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnvVarToSecretName(t *testing.T) {
	tests := []struct {
		envVar   string
		expected string
	}{
		{"OPENAI_API_KEY", "openai-api-key"},
		{"ANTHROPIC_API_KEY", "anthropic-api-key"},
		{"GOOGLE_API_KEY", "google-api-key"},
		{"AWS_ACCESS_KEY_ID", "aws-access-key-id"},
	}

	for _, tt := range tests {
		t.Run(tt.envVar, func(t *testing.T) {
			result := EnvVarToSecretName(tt.envVar)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSecretNameToEnvVar(t *testing.T) {
	tests := []struct {
		secretName string
		expected   string
	}{
		{"openai-api-key", "OPENAI_API_KEY"},
		{"anthropic-api-key", "ANTHROPIC_API_KEY"},
		{"google-api-key", "GOOGLE_API_KEY"},
	}

	for _, tt := range tests {
		t.Run(tt.secretName, func(t *testing.T) {
			result := SecretNameToEnvVar(tt.secretName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetSecretRefsForProvider(t *testing.T) {
	refs := GetSecretRefsForProvider("openai")
	require.Len(t, refs, 1)
	assert.Equal(t, "OPENAI_API_KEY", refs[0].EnvVar)
	assert.Equal(t, "openai-api-key", refs[0].SecretName)
	assert.Equal(t, "value", refs[0].Key)

	refs = GetSecretRefsForProvider("gemini")
	require.Len(t, refs, 2)
	assert.Equal(t, "GOOGLE_API_KEY", refs[0].EnvVar)
	assert.Equal(t, "GEMINI_API_KEY", refs[1].EnvVar)
}

func TestDiscoverAvailableProviders(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Create fake secrets
	secrets := []corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openai-api-key",
				Namespace: "test-ns",
			},
			Data: map[string][]byte{
				"value": []byte("sk-test"),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "anthropic-api-key",
				Namespace: "test-ns",
			},
			Data: map[string][]byte{
				"value": []byte("sk-ant-test"),
			},
		},
	}

	objs := make([]runtime.Object, 0, len(secrets))
	for i := range secrets {
		objs = append(objs, &secrets[i])
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()

	discovery := NewDiscovery(fakeClient, "test-ns")

	cfg := &config.Config{
		LoadedProviders: map[string]*config.Provider{
			"gpt-4": {
				ID:   "gpt-4",
				Type: "openai",
			},
			"claude-3": {
				ID:   "claude-3",
				Type: "claude",
			},
			"gemini-pro": {
				ID:   "gemini-pro",
				Type: "gemini",
			},
			"mock-provider": {
				ID:   "mock-provider",
				Type: "mock",
			},
		},
	}

	available, err := discovery.DiscoverAvailableProviders(context.Background(), cfg)
	require.NoError(t, err)

	// Should include openai, claude (have secrets) and mock (no credentials needed)
	assert.Contains(t, available, "gpt-4")
	assert.Contains(t, available, "claude-3")
	assert.Contains(t, available, "mock-provider")
	// Gemini should NOT be available (no secret for GOOGLE_API_KEY or GEMINI_API_KEY)
	assert.NotContains(t, available, "gemini-pro")
}

func TestGetMissingCredentials(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openai-api-key",
			Namespace: "test-ns",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(&secret).
		Build()

	discovery := NewDiscovery(fakeClient, "test-ns")

	cfg := &config.Config{
		LoadedProviders: map[string]*config.Provider{
			"gpt-4": {
				ID:   "gpt-4",
				Type: "openai",
			},
			"claude-3": {
				ID:   "claude-3",
				Type: "claude",
			},
		},
	}

	missing, err := discovery.GetMissingCredentials(context.Background(), cfg)
	require.NoError(t, err)

	// OpenAI has secret, Claude does not
	assert.NotContains(t, missing, "gpt-4")
	assert.Contains(t, missing, "claude-3")
	assert.Equal(t, []string{"ANTHROPIC_API_KEY"}, missing["claude-3"])
}

func TestValidateProviderCredentials(t *testing.T) {
	cfg := &config.Config{
		LoadedProviders: map[string]*config.Provider{
			"test-openai": {
				ID:   "test-openai",
				Type: "openai",
			},
			"test-mock": {
				ID:   "test-mock",
				Type: "mock",
			},
		},
	}

	t.Run("missing credentials", func(t *testing.T) {
		// Ensure env var is not set
		_ = os.Unsetenv("OPENAI_API_KEY")

		err := ValidateProviderCredentials(cfg, "test-openai")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing credentials")
	})

	t.Run("credentials present", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "sk-test")

		err := ValidateProviderCredentials(cfg, "test-openai")
		assert.NoError(t, err)
	})

	t.Run("mock provider needs no credentials", func(t *testing.T) {
		err := ValidateProviderCredentials(cfg, "test-mock")
		assert.NoError(t, err)
	})

	t.Run("provider not found", func(t *testing.T) {
		err := ValidateProviderCredentials(cfg, "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("explicit CredentialEnv set and present", func(t *testing.T) {
		t.Setenv("CUSTOM_API_KEY", "my-key")
		explicitCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"custom-provider": {
					ID:   "custom-provider",
					Type: "openai",
					Credential: &config.CredentialConfig{
						CredentialEnv: "CUSTOM_API_KEY",
					},
				},
			},
		}
		err := ValidateProviderCredentials(explicitCfg, "custom-provider")
		assert.NoError(t, err)
	})

	t.Run("explicit CredentialEnv set but missing", func(t *testing.T) {
		_ = os.Unsetenv("CUSTOM_API_KEY_MISSING")
		explicitCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"custom-provider": {
					ID:   "custom-provider",
					Type: "openai",
					Credential: &config.CredentialConfig{
						CredentialEnv: "CUSTOM_API_KEY_MISSING",
					},
				},
			},
		}
		err := ValidateProviderCredentials(explicitCfg, "custom-provider")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CUSTOM_API_KEY_MISSING")
	})

	t.Run("explicit CredentialFile set and exists", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "credential-test-*")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name()) //nolint:errcheck
		require.NoError(t, tmpFile.Close())

		explicitCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"file-provider": {
					ID:   "file-provider",
					Type: "openai",
					Credential: &config.CredentialConfig{
						CredentialFile: tmpFile.Name(),
					},
				},
			},
		}
		err = ValidateProviderCredentials(explicitCfg, "file-provider")
		assert.NoError(t, err)
	})

	t.Run("explicit CredentialFile set but missing", func(t *testing.T) {
		explicitCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"file-provider": {
					ID:   "file-provider",
					Type: "openai",
					Credential: &config.CredentialConfig{
						CredentialFile: "/nonexistent/path/to/credential",
					},
				},
			},
		}
		err := ValidateProviderCredentials(explicitCfg, "file-provider")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "credential file")
	})

	t.Run("explicit APIKey set", func(t *testing.T) {
		explicitCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"apikey-provider": {
					ID:   "apikey-provider",
					Type: "openai",
					Credential: &config.CredentialConfig{
						APIKey: "sk-hardcoded-key",
					},
				},
			},
		}
		err := ValidateProviderCredentials(explicitCfg, "apikey-provider")
		assert.NoError(t, err)
	})

	t.Run("no credential config falls back to legacy", func(t *testing.T) {
		_ = os.Unsetenv("OPENAI_API_KEY")
		legacyCfg := &config.Config{
			LoadedProviders: map[string]*config.Provider{
				"legacy-provider": {
					ID:   "legacy-provider",
					Type: "openai",
				},
			},
		}
		err := ValidateProviderCredentials(legacyCfg, "legacy-provider")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing credentials")
	})
}

func TestValidateProviderCredentialsByType(t *testing.T) {
	t.Run("missing credentials", func(t *testing.T) {
		_ = os.Unsetenv("ANTHROPIC_API_KEY")

		err := ValidateProviderCredentialsByType("claude")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing credentials")
	})

	t.Run("credentials present", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")

		err := ValidateProviderCredentialsByType("claude")
		assert.NoError(t, err)
	})

	t.Run("fallback env var works", func(t *testing.T) {
		_ = os.Unsetenv("GOOGLE_API_KEY")
		t.Setenv("GEMINI_API_KEY", "test-key")

		err := ValidateProviderCredentialsByType("gemini")
		assert.NoError(t, err)
	})
}
