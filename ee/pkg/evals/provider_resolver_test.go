/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/session/api"
	"github.com/altairalabs/omnia/pkg/k8s"
)

const testNamespace = "test-ns"

func buildFakeClient(objs ...runtime.Object) *fake.ClientBuilder {
	scheme := k8s.Scheme()
	return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...)
}

func TestResolveProviderSpecs_Success(t *testing.T) {
	ns := testNamespace
	secretKey := "ANTHROPIC_API_KEY"

	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "my-agent", Namespace: ns},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
			Providers: []v1alpha1.NamedProviderRef{
				{
					Name:        "default",
					ProviderRef: v1alpha1.ProviderRef{Name: "claude-provider"},
				},
			},
		},
	}

	provider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "claude-provider", Namespace: ns},
		Spec: v1alpha1.ProviderSpec{
			Type:  "claude",
			Model: "claude-sonnet-4-20250514",
			Credential: &v1alpha1.CredentialConfig{
				SecretRef: &v1alpha1.SecretKeyRef{Name: "claude-secret"},
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "claude-secret", Namespace: ns},
		Data: map[string][]byte{
			secretKey: []byte("sk-test-key-123"),
		},
	}

	c := buildFakeClient(ar, provider, secret).Build()
	resolver := NewProviderResolver(c)

	specs, err := resolver.ResolveProviderSpecs(context.Background(), "my-agent", ns)
	require.NoError(t, err)
	require.Len(t, specs, 1)

	spec := specs["default"]
	assert.Equal(t, "default", spec.ID)
	assert.Equal(t, "claude", spec.Type)
	assert.Equal(t, "claude-sonnet-4-20250514", spec.Model)
	assert.NotNil(t, spec.Credential)
	assert.Equal(t, "api_key", spec.Credential.Type())
}

func TestResolveProviderSpecs_MultipleProviders(t *testing.T) {
	ns := testNamespace

	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: ns},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
			Providers: []v1alpha1.NamedProviderRef{
				{
					Name:        "default",
					ProviderRef: v1alpha1.ProviderRef{Name: "claude-prov"},
				},
				{
					Name:        "judge",
					ProviderRef: v1alpha1.ProviderRef{Name: "openai-prov"},
				},
			},
		},
	}

	claudeProvider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "claude-prov", Namespace: ns},
		Spec: v1alpha1.ProviderSpec{
			Type:  "claude",
			Model: "claude-sonnet-4-20250514",
			Credential: &v1alpha1.CredentialConfig{
				SecretRef: &v1alpha1.SecretKeyRef{Name: "claude-secret"},
			},
		},
	}

	openaiProvider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "openai-prov", Namespace: ns},
		Spec: v1alpha1.ProviderSpec{
			Type:  "openai",
			Model: "gpt-4o",
			Credential: &v1alpha1.CredentialConfig{
				SecretRef: &v1alpha1.SecretKeyRef{Name: "openai-secret"},
			},
		},
	}

	claudeSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "claude-secret", Namespace: ns},
		Data:       map[string][]byte{"ANTHROPIC_API_KEY": []byte("sk-claude")},
	}

	openaiSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "openai-secret", Namespace: ns},
		Data:       map[string][]byte{"OPENAI_API_KEY": []byte("sk-openai")},
	}

	c := buildFakeClient(ar, claudeProvider, openaiProvider, claudeSecret, openaiSecret).Build()
	resolver := NewProviderResolver(c)

	specs, err := resolver.ResolveProviderSpecs(context.Background(), "agent", ns)
	require.NoError(t, err)
	require.Len(t, specs, 2)

	assert.Equal(t, "claude", specs["default"].Type)
	assert.Equal(t, "openai", specs["judge"].Type)
}

func TestResolveProviderSpecs_NoProviders(t *testing.T) {
	ns := testNamespace

	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: ns},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		},
	}

	c := buildFakeClient(ar).Build()
	resolver := NewProviderResolver(c)

	specs, err := resolver.ResolveProviderSpecs(context.Background(), "agent", ns)
	require.NoError(t, err)
	assert.Nil(t, specs)
}

func TestResolveProviderSpecs_MockProviderNoCredential(t *testing.T) {
	ns := testNamespace

	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: ns},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
			Providers: []v1alpha1.NamedProviderRef{
				{
					Name:        "default",
					ProviderRef: v1alpha1.ProviderRef{Name: "mock-prov"},
				},
			},
		},
	}

	mockProvider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "mock-prov", Namespace: ns},
		Spec: v1alpha1.ProviderSpec{
			Type: "mock",
		},
	}

	c := buildFakeClient(ar, mockProvider).Build()
	resolver := NewProviderResolver(c)

	specs, err := resolver.ResolveProviderSpecs(context.Background(), "agent", ns)
	require.NoError(t, err)
	require.Len(t, specs, 1)
	assert.Equal(t, "mock", specs["default"].Type)
	assert.Nil(t, specs["default"].Credential)
}

func TestResolveProviderSpecs_Cache(t *testing.T) {
	ns := testNamespace

	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: ns},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
			Providers: []v1alpha1.NamedProviderRef{
				{
					Name:        "default",
					ProviderRef: v1alpha1.ProviderRef{Name: "mock-prov"},
				},
			},
		},
	}

	mockProvider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "mock-prov", Namespace: ns},
		Spec:       v1alpha1.ProviderSpec{Type: "mock"},
	}

	c := buildFakeClient(ar, mockProvider).Build()
	resolver := NewProviderResolver(c)

	specs1, err := resolver.ResolveProviderSpecs(context.Background(), "agent", ns)
	require.NoError(t, err)

	specs2, err := resolver.ResolveProviderSpecs(context.Background(), "agent", ns)
	require.NoError(t, err)

	// Both should return the same cached map
	assert.Equal(t, specs1, specs2)
}

func TestResolveProviderSpecs_AgentNotFound(t *testing.T) {
	c := buildFakeClient().Build()
	resolver := NewProviderResolver(c)

	_, err := resolver.ResolveProviderSpecs(context.Background(), "nonexistent", "ns")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get AgentRuntime")
}

func TestResolveProviderSpecs_SecretMissing(t *testing.T) {
	ns := testNamespace

	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: ns},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
			Providers: []v1alpha1.NamedProviderRef{
				{
					Name:        "default",
					ProviderRef: v1alpha1.ProviderRef{Name: "claude-prov"},
				},
			},
		},
	}

	provider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "claude-prov", Namespace: ns},
		Spec: v1alpha1.ProviderSpec{
			Type: "claude",
			Credential: &v1alpha1.CredentialConfig{
				SecretRef: &v1alpha1.SecretKeyRef{Name: "missing-secret"},
			},
		},
	}

	c := buildFakeClient(ar, provider).Build()
	resolver := NewProviderResolver(c)

	_, err := resolver.ResolveProviderSpecs(context.Background(), "agent", ns)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve credential")
}

func TestResolveProviderSpecs_WithDefaults(t *testing.T) {
	ns := testNamespace
	temp := "0.7"
	topP := "0.9"
	maxTokens := int32(4096)

	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: ns},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
			Providers: []v1alpha1.NamedProviderRef{
				{
					Name:        "default",
					ProviderRef: v1alpha1.ProviderRef{Name: "prov"},
				},
			},
		},
	}

	provider := &v1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "prov", Namespace: ns},
		Spec: v1alpha1.ProviderSpec{
			Type: "mock",
			Defaults: &v1alpha1.ProviderDefaults{
				Temperature: &temp,
				TopP:        &topP,
				MaxTokens:   &maxTokens,
			},
		},
	}

	c := buildFakeClient(ar, provider).Build()
	resolver := NewProviderResolver(c)

	specs, err := resolver.ResolveProviderSpecs(context.Background(), "agent", ns)
	require.NoError(t, err)

	spec := specs["default"]
	assert.InDelta(t, 0.7, float64(spec.Defaults.Temperature), 0.01)
	assert.InDelta(t, 0.9, float64(spec.Defaults.TopP), 0.01)
	assert.Equal(t, 4096, spec.Defaults.MaxTokens)
}

func TestConvertDefaults(t *testing.T) {
	temp := "0.5"
	topP := "0.8"
	maxTokens := int32(2048)

	d := &v1alpha1.ProviderDefaults{
		Temperature: &temp,
		TopP:        &topP,
		MaxTokens:   &maxTokens,
	}

	pd := convertDefaults(d)
	assert.InDelta(t, 0.5, float64(pd.Temperature), 0.01)
	assert.InDelta(t, 0.8, float64(pd.TopP), 0.01)
	assert.Equal(t, 2048, pd.MaxTokens)
}

func TestConvertDefaults_NilFields(t *testing.T) {
	d := &v1alpha1.ProviderDefaults{}
	pd := convertDefaults(d)
	assert.Zero(t, pd.Temperature)
	assert.Zero(t, pd.TopP)
	assert.Zero(t, pd.MaxTokens)
}

func TestBuildCredential_Claude(t *testing.T) {
	cred := buildCredential("sk-test", "claude")
	assert.NotNil(t, cred)
	assert.Equal(t, "api_key", cred.Type())
}

func TestBuildCredential_OpenAI(t *testing.T) {
	cred := buildCredential("sk-test", "openai")
	assert.NotNil(t, cred)
	assert.Equal(t, "api_key", cred.Type())
}

func TestBuildCredential_UnknownProvider(t *testing.T) {
	cred := buildCredential("sk-test", "unknown")
	assert.NotNil(t, cred)
	assert.Equal(t, "api_key", cred.Type())
}

func TestResolveProviders_NilResolver(t *testing.T) {
	w := &EvalWorker{logger: testLogger()}
	event := api.SessionEvent{AgentName: "agent", Namespace: "ns"}
	specs := w.resolveProviders(context.Background(), event)
	assert.Nil(t, specs)
}

func TestResolveProviders_EmptyAgentName(t *testing.T) {
	c := buildFakeClient().Build()
	w := &EvalWorker{
		logger:           testLogger(),
		providerResolver: NewProviderResolver(c),
	}
	event := api.SessionEvent{Namespace: "ns"}
	specs := w.resolveProviders(context.Background(), event)
	assert.Nil(t, specs)
}
