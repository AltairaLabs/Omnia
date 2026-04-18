/*
Copyright 2026.

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

package runtime

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/k8s"
)

func buildTestClient(objs ...runtime.Object) client.Client {
	scheme := k8s.Scheme()
	// Include a namespace with the workspace label so ResolveWorkspaceName
	// can resolve via the fallback path (namespace label lookup).
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-ns",
			Labels: map[string]string{"omnia.altairalabs.ai/workspace": "test-ws"},
		},
	}
	allObjs := append([]runtime.Object{ns}, objs...)
	return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(allObjs...).Build()
}

func TestLoadFromCRD_NamedProviders(t *testing.T) {
	secretKey := "my-key"
	provider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "claude-provider",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.ProviderSpec{
			Type:    v1alpha1.ProviderTypeClaude,
			Model:   "claude-sonnet-4-20250514",
			BaseURL: "https://api.anthropic.com",
			Credential: &v1alpha1.CredentialConfig{
				SecretRef: &v1alpha1.SecretKeyRef{
					Name: "claude-secret",
					Key:  &secretKey,
				},
			},
			Defaults: &v1alpha1.ProviderDefaults{
				ContextWindow:      int32Ptr(200000),
				TruncationStrategy: v1alpha1.TruncationStrategySliding,
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "claude-secret",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"my-key": []byte("sk-ant-test-key"),
		},
	}

	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
			Providers: []v1alpha1.NamedProviderRef{
				{
					Name:        "default",
					ProviderRef: v1alpha1.ProviderRef{Name: "claude-provider"},
				},
			},
			Evals: &v1alpha1.EvalConfig{Enabled: true},
			Media: &v1alpha1.MediaConfig{BasePath: "/custom/media"},
			Session: &v1alpha1.SessionConfig{
				Type: v1alpha1.SessionStoreTypeMemory,
				TTL:  strPtr("2h"),
			},
		},
	}

	c := buildTestClient(ar, provider, secret)
	cfg, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.NoError(t, err)

	assert.Equal(t, "test-agent", cfg.AgentName)
	assert.Equal(t, "test-ns", cfg.Namespace)
	assert.Equal(t, "test-pack", cfg.PromptPackName)
	assert.Equal(t, "claude", cfg.ProviderType)
	assert.Equal(t, "claude-sonnet-4-20250514", cfg.Model)
	assert.Equal(t, "https://api.anthropic.com", cfg.BaseURL)
	assert.Equal(t, "claude-provider", cfg.ProviderRefName)
	assert.Equal(t, "test-ns", cfg.ProviderRefNamespace)
	assert.Equal(t, 200000, cfg.ContextWindow)
	assert.Equal(t, "sliding", cfg.TruncationStrategy)
	assert.True(t, cfg.EvalEnabled)
	assert.Equal(t, "/custom/media", cfg.MediaBasePath)
	assert.Equal(t, "memory", cfg.SessionType)

	// Verify API key env var was set
	assert.Equal(t, "sk-ant-test-key", os.Getenv("ANTHROPIC_API_KEY"))
	t.Cleanup(func() { os.Unsetenv("ANTHROPIC_API_KEY") })
}

func TestLoadFromCRD_SingleProvider(t *testing.T) {
	provider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openai-provider",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.ProviderSpec{
			Type:  v1alpha1.ProviderTypeOpenAI,
			Model: "gpt-4o",
			SecretRef: &v1alpha1.SecretKeyRef{
				Name: "openai-secret",
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openai-secret",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"OPENAI_API_KEY": []byte("sk-openai-test"),
		},
	}

	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
			Providers: []v1alpha1.NamedProviderRef{
				{Name: "default", ProviderRef: v1alpha1.ProviderRef{Name: "openai-provider"}},
			},
		},
	}

	c := buildTestClient(ar, provider, secret)
	cfg, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.NoError(t, err)

	assert.Equal(t, "openai", cfg.ProviderType)
	assert.Equal(t, "gpt-4o", cfg.Model)
	assert.Equal(t, "sk-openai-test", os.Getenv("OPENAI_API_KEY"))
	t.Cleanup(func() { os.Unsetenv("OPENAI_API_KEY") })
}

func TestLoadFromCRD_NoProviders(t *testing.T) {
	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
		},
	}

	c := buildTestClient(ar)
	cfg, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.NoError(t, err)

	assert.Empty(t, cfg.ProviderType)
	assert.Empty(t, cfg.Model)
	assert.Empty(t, cfg.ProviderRefName)
}

func TestLoadFromCRD_MockProviderAnnotation(t *testing.T) {
	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
			Annotations: map[string]string{
				"omnia.altairalabs.ai/mock-provider": "true",
			},
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
		},
	}

	c := buildTestClient(ar)
	cfg, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.NoError(t, err)

	assert.True(t, cfg.MockProvider)
}

func TestLoadFromCRD_MockConfigPathAnnotation(t *testing.T) {
	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
			Annotations: map[string]string{
				"omnia.altairalabs.ai/mock-provider":    "true",
				"omnia.altairalabs.ai/mock-config-path": "/etc/omnia/mock/responses.yaml",
			},
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
		},
	}

	c := buildTestClient(ar)
	cfg, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.NoError(t, err)

	assert.True(t, cfg.MockProvider)
	assert.Equal(t, "/etc/omnia/mock/responses.yaml", cfg.MockConfigPath)
}

func TestLoadFromCRD_MockProviderType(t *testing.T) {
	provider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mock-provider",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.ProviderSpec{
			Type: v1alpha1.ProviderTypeMock,
		},
	}

	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
			Providers: []v1alpha1.NamedProviderRef{
				{Name: "default", ProviderRef: v1alpha1.ProviderRef{Name: "mock-provider"}},
			},
		},
	}

	c := buildTestClient(ar, provider)
	cfg, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.NoError(t, err)

	assert.Equal(t, "mock", cfg.ProviderType)
	assert.True(t, cfg.MockProvider, "MockProvider should be auto-enabled for mock type")
}

func TestLoadFromCRD_CredentialSecretRef(t *testing.T) {
	customKey := "custom-api-key"
	provider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gemini-provider",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.ProviderSpec{
			Type: v1alpha1.ProviderTypeGemini,
			Credential: &v1alpha1.CredentialConfig{
				SecretRef: &v1alpha1.SecretKeyRef{
					Name: "gemini-secret",
					Key:  &customKey,
				},
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gemini-secret",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"custom-api-key": []byte("gemini-key-value"),
		},
	}

	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
			Providers: []v1alpha1.NamedProviderRef{
				{Name: "default", ProviderRef: v1alpha1.ProviderRef{Name: "gemini-provider"}},
			},
		},
	}

	c := buildTestClient(ar, provider, secret)
	cfg, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.NoError(t, err)

	assert.Equal(t, "gemini", cfg.ProviderType)
	assert.Equal(t, "gemini-key-value", os.Getenv("GEMINI_API_KEY"))
	t.Cleanup(func() { os.Unsetenv("GEMINI_API_KEY") })
}

func TestLoadFromCRD_NamedProvidersSortedFallback(t *testing.T) {
	provider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "alpha-provider",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.ProviderSpec{
			Type:  v1alpha1.ProviderTypeOllama,
			Model: "sorted-first",
		},
	}

	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
			Providers: []v1alpha1.NamedProviderRef{
				{
					Name:        "beta",
					ProviderRef: v1alpha1.ProviderRef{Name: "alpha-provider"},
				},
				{
					Name:        "alpha",
					ProviderRef: v1alpha1.ProviderRef{Name: "alpha-provider"},
				},
			},
		},
	}

	c := buildTestClient(ar, provider)
	cfg, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.NoError(t, err)

	// "alpha" sorts before "beta", so alpha's provider is used
	assert.Equal(t, "ollama", cfg.ProviderType)
	assert.Equal(t, "sorted-first", cfg.Model)
}

func TestLoadFromCRD_AgentRuntimeNotFound(t *testing.T) {
	c := buildTestClient()
	_, err := LoadFromCRD(context.Background(), c, "missing", "test-ns")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AgentRuntime")
}

func TestLoadFromCRD_ProviderNotFound(t *testing.T) {
	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
			Providers: []v1alpha1.NamedProviderRef{
				{Name: "default", ProviderRef: v1alpha1.ProviderRef{Name: "missing-provider"}},
			},
		},
	}

	c := buildTestClient(ar)
	_, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve provider")
}

func TestLoadFromCRD_SecretNotFound(t *testing.T) {
	provider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "claude-provider",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.ProviderSpec{
			Type: v1alpha1.ProviderTypeClaude,
			SecretRef: &v1alpha1.SecretKeyRef{
				Name: "missing-secret",
			},
		},
	}

	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
			Providers: []v1alpha1.NamedProviderRef{
				{Name: "default", ProviderRef: v1alpha1.ProviderRef{Name: "claude-provider"}},
			},
		},
	}

	c := buildTestClient(ar, provider)
	_, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider secret")
}

func TestLoadFromCRD_SecretMissingKey(t *testing.T) {
	provider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "claude-provider",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.ProviderSpec{
			Type: v1alpha1.ProviderTypeClaude,
			SecretRef: &v1alpha1.SecretKeyRef{
				Name: "claude-secret",
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "claude-secret",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"WRONG_KEY": []byte("value"),
		},
	}

	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
			Providers: []v1alpha1.NamedProviderRef{
				{Name: "default", ProviderRef: v1alpha1.ProviderRef{Name: "claude-provider"}},
			},
		},
	}

	c := buildTestClient(ar, provider, secret)
	_, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not contain key")
}

func TestDetermineSecretKey(t *testing.T) {
	tests := []struct {
		name         string
		ref          *v1alpha1.SecretKeyRef
		providerType v1alpha1.ProviderType
		want         string
	}{
		{
			name:         "explicit key",
			ref:          &v1alpha1.SecretKeyRef{Name: "s", Key: strPtr("custom-key")},
			providerType: v1alpha1.ProviderTypeClaude,
			want:         "custom-key",
		},
		{
			name:         "claude default",
			ref:          &v1alpha1.SecretKeyRef{Name: "s"},
			providerType: v1alpha1.ProviderTypeClaude,
			want:         "ANTHROPIC_API_KEY",
		},
		{
			name:         "openai default",
			ref:          &v1alpha1.SecretKeyRef{Name: "s"},
			providerType: v1alpha1.ProviderTypeOpenAI,
			want:         "OPENAI_API_KEY",
		},
		{
			name:         "gemini default",
			ref:          &v1alpha1.SecretKeyRef{Name: "s"},
			providerType: v1alpha1.ProviderTypeGemini,
			want:         "GEMINI_API_KEY",
		},
		{
			name:         "unknown provider falls back to api-key",
			ref:          &v1alpha1.SecretKeyRef{Name: "s"},
			providerType: "unknown",
			want:         "api-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := k8s.DetermineSecretKey(tt.ref, tt.providerType)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLoadFromCRD_TracingEnvVars(t *testing.T) {
	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
		},
	}

	t.Setenv("OMNIA_TRACING_ENABLED", "true")
	t.Setenv("OMNIA_TRACING_ENDPOINT", "alloy:4317")
	t.Setenv("OMNIA_TRACING_INSECURE", "true")
	t.Setenv("OMNIA_TRACING_SAMPLE_RATE", "0.5")

	c := buildTestClient(ar)
	cfg, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.NoError(t, err)

	assert.True(t, cfg.TracingEnabled)
	assert.Equal(t, "alloy:4317", cfg.TracingEndpoint)
	assert.True(t, cfg.TracingInsecure)
	assert.InDelta(t, 0.5, cfg.TracingSampleRate, 0.001)
}

func TestLoadFromCRD_TracingDisabledByDefault(t *testing.T) {
	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
		},
	}

	// Ensure tracing env vars are not set
	t.Setenv("OMNIA_TRACING_ENABLED", "")
	t.Setenv("OMNIA_TRACING_ENDPOINT", "")
	t.Setenv("OMNIA_TRACING_INSECURE", "")

	c := buildTestClient(ar)
	cfg, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.NoError(t, err)

	assert.False(t, cfg.TracingEnabled)
	assert.Empty(t, cfg.TracingEndpoint)
	assert.False(t, cfg.TracingInsecure)
	assert.InDelta(t, 1.0, cfg.TracingSampleRate, 0.001, "default sample rate should be 1.0")
}

func TestLoadFromCRD_ToolRegistryRef(t *testing.T) {
	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
			ToolRegistryRef: &v1alpha1.ToolRegistryRef{
				Name: "demo-tools",
			},
		},
	}

	c := buildTestClient(ar)
	cfg, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.NoError(t, err)

	assert.Equal(t, "/etc/omnia/tools/tools.yaml", cfg.ToolsConfigPath)
}

func TestLoadFromCRD_NoToolRegistryRef(t *testing.T) {
	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
		},
	}

	c := buildTestClient(ar)
	cfg, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.NoError(t, err)

	assert.Empty(t, cfg.ToolsConfigPath)
}

func TestLoadProviderPricing(t *testing.T) {
	t.Run("nil pricing", func(t *testing.T) {
		cfg := &Config{}
		require.NoError(t, loadProviderPricing(cfg, nil))
		assert.Equal(t, 0.0, cfg.InputCostPer1K)
		assert.Equal(t, 0.0, cfg.OutputCostPer1K)
	})

	t.Run("both rates set", func(t *testing.T) {
		cfg := &Config{}
		require.NoError(t, loadProviderPricing(cfg, &v1alpha1.ProviderPricing{
			InputCostPer1K:  strPtr("0.003"),
			OutputCostPer1K: strPtr("0.015"),
		}))
		assert.InDelta(t, 0.003, cfg.InputCostPer1K, 1e-9)
		assert.InDelta(t, 0.015, cfg.OutputCostPer1K, 1e-9)
	})

	t.Run("only input set", func(t *testing.T) {
		cfg := &Config{}
		require.NoError(t, loadProviderPricing(cfg, &v1alpha1.ProviderPricing{
			InputCostPer1K: strPtr("0.003"),
		}))
		assert.InDelta(t, 0.003, cfg.InputCostPer1K, 1e-9)
		assert.Equal(t, 0.0, cfg.OutputCostPer1K)
	})

	t.Run("invalid string returns error", func(t *testing.T) {
		cfg := &Config{}
		err := loadProviderPricing(cfg, &v1alpha1.ProviderPricing{
			InputCostPer1K: strPtr("not-a-number"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "inputCostPer1K")
	})
}

func TestLoadFromCRD_ProviderPricing(t *testing.T) {
	provider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ollama-provider",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.ProviderSpec{
			Type:    v1alpha1.ProviderTypeOllama,
			Model:   "llama3",
			BaseURL: "http://ollama:11434",
			Pricing: &v1alpha1.ProviderPricing{
				InputCostPer1K:  strPtr("0.001"),
				OutputCostPer1K: strPtr("0.002"),
			},
		},
	}

	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
			Providers: []v1alpha1.NamedProviderRef{
				{Name: "default", ProviderRef: v1alpha1.ProviderRef{Name: "ollama-provider"}},
			},
		},
	}

	c := buildTestClient(ar, provider)
	cfg, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.NoError(t, err)

	assert.Equal(t, "ollama", cfg.ProviderType)
	assert.InDelta(t, 0.001, cfg.InputCostPer1K, 1e-9)
	assert.InDelta(t, 0.002, cfg.OutputCostPer1K, 1e-9)
}

func TestLoadFromCRD_MemoryEnabled(t *testing.T) {
	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
			Memory: &v1alpha1.MemoryConfig{
				Enabled: true,
			},
		},
	}

	// Service discovery resolves both URLs via env vars when both are set.
	t.Setenv("SESSION_API_URL", "http://omnia-session-api.omnia-system:8080")
	t.Setenv("MEMORY_API_URL", "http://omnia-memory-api.omnia-system:8080")

	c := buildTestClient(ar)
	cfg, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.NoError(t, err)

	assert.True(t, cfg.MemoryEnabled)
	assert.Equal(t, "http://omnia-memory-api.omnia-system:8080", cfg.MemoryAPIURL)
}

func TestLoadFromCRD_MemoryDisabled(t *testing.T) {
	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
		},
	}

	c := buildTestClient(ar)
	cfg, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.NoError(t, err)

	assert.False(t, cfg.MemoryEnabled)
	assert.Empty(t, cfg.MemoryAPIURL)
}

func TestLoadFromCRD_MemoryEnvOverride(t *testing.T) {
	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
			Memory: &v1alpha1.MemoryConfig{
				Enabled: true,
			},
		},
	}

	// Service discovery uses MEMORY_API_URL directly (no derivation from session URL).
	t.Setenv("SESSION_API_URL", "http://omnia-session-api.omnia-system:8080")
	t.Setenv("MEMORY_API_URL", "http://custom-memory-api:9090")

	c := buildTestClient(ar)
	cfg, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.NoError(t, err)

	assert.True(t, cfg.MemoryEnabled)
	assert.Equal(t, "http://custom-memory-api:9090", cfg.MemoryAPIURL)
}

func TestInjectAWSAccessKey(t *testing.T) {
	t.Run("sets required env vars", func(t *testing.T) {
		t.Setenv("AWS_ACCESS_KEY_ID", "")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "")
		err := injectAWSAccessKey(map[string][]byte{
			"AWS_ACCESS_KEY_ID":     []byte("AKIA-test"),
			"AWS_SECRET_ACCESS_KEY": []byte("secret-test"),
		}, "ns", "name")
		require.NoError(t, err)
		assert.Equal(t, "AKIA-test", os.Getenv("AWS_ACCESS_KEY_ID"))
		assert.Equal(t, "secret-test", os.Getenv("AWS_SECRET_ACCESS_KEY"))
	})

	t.Run("sets session token when provided", func(t *testing.T) {
		t.Setenv("AWS_SESSION_TOKEN", "")
		err := injectAWSAccessKey(map[string][]byte{
			"AWS_ACCESS_KEY_ID":     []byte("AKIA-test"),
			"AWS_SECRET_ACCESS_KEY": []byte("secret-test"),
			"AWS_SESSION_TOKEN":     []byte("session-test"),
		}, "ns", "name")
		require.NoError(t, err)
		assert.Equal(t, "session-test", os.Getenv("AWS_SESSION_TOKEN"))
	})

	t.Run("errors when access key id missing", func(t *testing.T) {
		err := injectAWSAccessKey(map[string][]byte{
			"AWS_SECRET_ACCESS_KEY": []byte("secret-test"),
		}, "ns", "name")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AWS_ACCESS_KEY_ID")
	})

	t.Run("errors when secret access key missing", func(t *testing.T) {
		err := injectAWSAccessKey(map[string][]byte{
			"AWS_ACCESS_KEY_ID": []byte("AKIA-test"),
		}, "ns", "name")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AWS_SECRET_ACCESS_KEY")
	})
}

func TestInjectAzureServicePrincipal(t *testing.T) {
	t.Run("sets all three env vars", func(t *testing.T) {
		for _, k := range []string{"AZURE_TENANT_ID", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET"} {
			t.Setenv(k, "")
		}
		err := injectAzureServicePrincipal(map[string][]byte{
			"AZURE_TENANT_ID":     []byte("tenant"),
			"AZURE_CLIENT_ID":     []byte("client"),
			"AZURE_CLIENT_SECRET": []byte("secret"),
		}, "ns", "name")
		require.NoError(t, err)
		assert.Equal(t, "tenant", os.Getenv("AZURE_TENANT_ID"))
		assert.Equal(t, "client", os.Getenv("AZURE_CLIENT_ID"))
		assert.Equal(t, "secret", os.Getenv("AZURE_CLIENT_SECRET"))
	})

	t.Run("errors when a required key missing", func(t *testing.T) {
		err := injectAzureServicePrincipal(map[string][]byte{
			"AZURE_TENANT_ID": []byte("tenant"),
			"AZURE_CLIENT_ID": []byte("client"),
		}, "ns", "name")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "AZURE_CLIENT_SECRET")
	})
}

func TestInjectGCPServiceAccount(t *testing.T) {
	t.Run("writes a secure temp file and sets env", func(t *testing.T) {
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
		json := []byte(`{"type":"service_account"}`)
		err := injectGCPServiceAccount(
			map[string][]byte{"credentials.json": json},
			&v1alpha1.SecretKeyRef{Name: "gcp"},
		)
		require.NoError(t, err)
		path := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
		require.NotEmpty(t, path)
		defer func() { _ = os.Remove(path) }()
		written, rErr := os.ReadFile(path) //nolint:gosec // test-only path
		require.NoError(t, rErr)
		assert.Equal(t, json, written)
	})

	t.Run("uses custom secret key when provided", func(t *testing.T) {
		custom := "my-sa.json"
		err := injectGCPServiceAccount(
			map[string][]byte{custom: []byte(`{}`)},
			&v1alpha1.SecretKeyRef{Name: "gcp", Key: &custom},
		)
		require.NoError(t, err)
		defer func() { _ = os.Remove(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")) }()
	})

	t.Run("errors when secret key missing", func(t *testing.T) {
		err := injectGCPServiceAccount(
			map[string][]byte{"other-key": []byte(`{}`)},
			&v1alpha1.SecretKeyRef{Name: "gcp"},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "credentials.json")
	})
}

func TestInjectPlatformCredentials(t *testing.T) {
	build := func(platform v1alpha1.PlatformType, auth v1alpha1.AuthMethod, secretData map[string][]byte) *v1alpha1.Provider {
		var credRef *v1alpha1.SecretKeyRef
		if secretData != nil {
			credRef = &v1alpha1.SecretKeyRef{Name: "creds"}
		}
		return &v1alpha1.Provider{
			ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "test-ns"},
			Spec: v1alpha1.ProviderSpec{
				Type:     v1alpha1.ProviderTypeClaude,
				Platform: &v1alpha1.PlatformConfig{Type: platform},
				Auth:     &v1alpha1.AuthConfig{Type: auth, CredentialsSecretRef: credRef},
			},
		}
	}

	t.Run("workloadIdentity is a no-op", func(t *testing.T) {
		c := buildTestClient()
		p := build(v1alpha1.PlatformTypeBedrock, v1alpha1.AuthMethodWorkloadIdentity, nil)
		p.Spec.Auth.CredentialsSecretRef = nil
		err := injectPlatformCredentials(context.Background(), c, p)
		require.NoError(t, err)
	})

	t.Run("bedrock + accessKey reads secret and sets env", func(t *testing.T) {
		t.Setenv("AWS_ACCESS_KEY_ID", "")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "test-ns"},
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("AKIA-test"),
				"AWS_SECRET_ACCESS_KEY": []byte("secret-test"),
			},
		}
		c := buildTestClient(secret)
		p := build(v1alpha1.PlatformTypeBedrock, v1alpha1.AuthMethodAccessKey,
			secret.Data)

		require.NoError(t, injectPlatformCredentials(context.Background(), c, p))
		assert.Equal(t, "AKIA-test", os.Getenv("AWS_ACCESS_KEY_ID"))
	})

	t.Run("errors when credentialsSecretRef nil for non-WI", func(t *testing.T) {
		c := buildTestClient()
		p := build(v1alpha1.PlatformTypeBedrock, v1alpha1.AuthMethodAccessKey, nil)
		p.Spec.Auth.CredentialsSecretRef = nil
		err := injectPlatformCredentials(context.Background(), c, p)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "credentialsSecretRef")
	})

	t.Run("errors when secret missing in cluster", func(t *testing.T) {
		c := buildTestClient()
		p := build(v1alpha1.PlatformTypeBedrock, v1alpha1.AuthMethodAccessKey,
			map[string][]byte{})
		err := injectPlatformCredentials(context.Background(), c, p)
		require.Error(t, err)
	})

	t.Run("rejects unsupported platform/auth combo", func(t *testing.T) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "test-ns"},
			Data:       map[string][]byte{},
		}
		c := buildTestClient(secret)
		// bedrock+servicePrincipal is not a valid combo (CEL rejects this at
		// admission but the runtime guards it defensively).
		p := build(v1alpha1.PlatformTypeBedrock, v1alpha1.AuthMethodServicePrincipal,
			secret.Data)
		err := injectPlatformCredentials(context.Background(), c, p)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported platform/auth")
	})
}

func TestLoadPlatformAndAuthConfig(t *testing.T) {
	t.Run("platform fields populate Config", func(t *testing.T) {
		cfg := &Config{}
		loadPlatformConfig(cfg, &v1alpha1.PlatformConfig{
			Type:     v1alpha1.PlatformTypeVertex,
			Region:   "us-central1",
			Project:  "my-project",
			Endpoint: "https://example",
		})
		assert.Equal(t, "vertex", cfg.PlatformType)
		assert.Equal(t, "us-central1", cfg.PlatformRegion)
		assert.Equal(t, "my-project", cfg.PlatformProject)
		assert.Equal(t, "https://example", cfg.PlatformEndpoint)
	})

	t.Run("nil platform is no-op", func(t *testing.T) {
		cfg := &Config{PlatformType: "unchanged"}
		loadPlatformConfig(cfg, nil)
		assert.Equal(t, "unchanged", cfg.PlatformType)
	})

	t.Run("auth fields populate Config", func(t *testing.T) {
		k := "my-key"
		cfg := &Config{}
		loadAuthConfig(cfg, &v1alpha1.AuthConfig{
			Type:                 v1alpha1.AuthMethodAccessKey,
			RoleArn:              "arn:aws:iam::1:role/x",
			ServiceAccountEmail:  "sa@p.iam",
			CredentialsSecretRef: &v1alpha1.SecretKeyRef{Name: "creds", Key: &k},
		})
		assert.Equal(t, "accessKey", cfg.AuthType)
		assert.Equal(t, "arn:aws:iam::1:role/x", cfg.AuthRoleArn)
		assert.Equal(t, "sa@p.iam", cfg.AuthServiceAccountEmail)
		assert.Equal(t, "creds", cfg.AuthCredentialsSecretName)
		assert.Equal(t, "my-key", cfg.AuthCredentialsSecretKey)
	})

	t.Run("nil auth is no-op", func(t *testing.T) {
		cfg := &Config{AuthType: "unchanged"}
		loadAuthConfig(cfg, nil)
		assert.Equal(t, "unchanged", cfg.AuthType)
	})
}

func strPtr(s string) *string {
	return &s
}

func int32Ptr(i int32) *int32 {
	return &i
}
