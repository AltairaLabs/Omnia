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
	return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
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

func TestLoadFromCRD_LegacyProviderRef(t *testing.T) {
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
			ProviderRef:   &v1alpha1.ProviderRef{Name: "openai-provider"},
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

func TestLoadFromCRD_InlineProvider(t *testing.T) {
	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "test-ns",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-pack"},
			Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
			Provider: &v1alpha1.ProviderConfig{
				Type:    v1alpha1.ProviderTypeOllama,
				Model:   "llama3",
				BaseURL: "http://localhost:11434",
				Config: &v1alpha1.ProviderDefaults{
					ContextWindow:      int32Ptr(8192),
					TruncationStrategy: v1alpha1.TruncationStrategySummarize,
				},
			},
		},
	}

	c := buildTestClient(ar)
	cfg, err := LoadFromCRD(context.Background(), c, "test-agent", "test-ns")
	require.NoError(t, err)

	assert.Equal(t, "ollama", cfg.ProviderType)
	assert.Equal(t, "llama3", cfg.Model)
	assert.Equal(t, "http://localhost:11434", cfg.BaseURL)
	assert.Equal(t, 8192, cfg.ContextWindow)
	assert.Equal(t, "summarize", cfg.TruncationStrategy)
	// No provider ref for inline
	assert.Empty(t, cfg.ProviderRefName)
}

func TestLoadFromCRD_NoProvider(t *testing.T) {
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
			ProviderRef:   &v1alpha1.ProviderRef{Name: "mock-provider"},
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
			ProviderRef:   &v1alpha1.ProviderRef{Name: "gemini-provider"},
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
			ProviderRef:   &v1alpha1.ProviderRef{Name: "missing-provider"},
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
			ProviderRef:   &v1alpha1.ProviderRef{Name: "claude-provider"},
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
			ProviderRef:   &v1alpha1.ProviderRef{Name: "claude-provider"},
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

func strPtr(s string) *string {
	return &s
}

func int32Ptr(i int32) *int32 {
	return &i
}
