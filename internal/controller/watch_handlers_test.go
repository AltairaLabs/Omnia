/*
Copyright 2025.

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

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestFindAgentRuntimesForSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tests := []struct {
		name           string
		secret         *corev1.Secret
		providers      []omniav1alpha1.Provider
		agentRuntimes  []omniav1alpha1.AgentRuntime
		expectedCount  int
		expectedAgents []string
	}{
		{
			name: "secret without credentials label is ignored",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "regular-secret",
					Namespace: "default",
				},
			},
			expectedCount: 0,
		},
		{
			name: "credential secret triggers agent reconcile via provider",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-key",
					Namespace: "default",
					Labels: map[string]string{
						"omnia.altairalabs.ai/type": "credentials",
					},
				},
			},
			providers: []omniav1alpha1.Provider{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-provider",
						Namespace: "default",
					},
					Spec: omniav1alpha1.ProviderSpec{
						Type:      "claude",
						SecretRef: &omniav1alpha1.SecretKeyRef{Name: "api-key"},
					},
				},
			},
			agentRuntimes: []omniav1alpha1.AgentRuntime{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-agent",
						Namespace: "default",
					},
					Spec: omniav1alpha1.AgentRuntimeSpec{
						ProviderRef:   &omniav1alpha1.ProviderRef{Name: "my-provider"},
						PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
					},
				},
			},
			expectedCount:  1,
			expectedAgents: []string{"my-agent"},
		},
		{
			name: "credential secret triggers agent reconcile via inline provider",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "inline-secret",
					Namespace: "default",
					Labels: map[string]string{
						"omnia.altairalabs.ai/type": "credentials",
					},
				},
			},
			agentRuntimes: []omniav1alpha1.AgentRuntime{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "inline-agent",
						Namespace: "default",
					},
					Spec: omniav1alpha1.AgentRuntimeSpec{
						Provider: &omniav1alpha1.ProviderConfig{
							Type:      "openai",
							SecretRef: &corev1.LocalObjectReference{Name: "inline-secret"},
						},
						PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
					},
				},
			},
			expectedCount:  1,
			expectedAgents: []string{"inline-agent"},
		},
		{
			name: "secret in different namespace does not trigger agent",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-key",
					Namespace: "other-ns",
					Labels: map[string]string{
						"omnia.altairalabs.ai/type": "credentials",
					},
				},
			},
			providers: []omniav1alpha1.Provider{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-provider",
						Namespace: "default",
					},
					Spec: omniav1alpha1.ProviderSpec{
						Type:      "claude",
						SecretRef: &omniav1alpha1.SecretKeyRef{Name: "api-key"},
					},
				},
			},
			agentRuntimes: []omniav1alpha1.AgentRuntime{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-agent",
						Namespace: "default",
					},
					Spec: omniav1alpha1.AgentRuntimeSpec{
						ProviderRef:   &omniav1alpha1.ProviderRef{Name: "my-provider"},
						PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
					},
				},
			},
			expectedCount: 0,
		},
		{
			name: "multiple agents using same secret",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared-key",
					Namespace: "default",
					Labels: map[string]string{
						"omnia.altairalabs.ai/type": "credentials",
					},
				},
			},
			providers: []omniav1alpha1.Provider{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "shared-provider",
						Namespace: "default",
					},
					Spec: omniav1alpha1.ProviderSpec{
						Type:      "claude",
						SecretRef: &omniav1alpha1.SecretKeyRef{Name: "shared-key"},
					},
				},
			},
			agentRuntimes: []omniav1alpha1.AgentRuntime{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent-1",
						Namespace: "default",
					},
					Spec: omniav1alpha1.AgentRuntimeSpec{
						ProviderRef:   &omniav1alpha1.ProviderRef{Name: "shared-provider"},
						PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent-2",
						Namespace: "default",
					},
					Spec: omniav1alpha1.AgentRuntimeSpec{
						ProviderRef:   &omniav1alpha1.ProviderRef{Name: "shared-provider"},
						PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
					},
				},
			},
			expectedCount:  2,
			expectedAgents: []string{"agent-1", "agent-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build fake client with objects
			objs := []runtime.Object{tt.secret}
			for i := range tt.providers {
				objs = append(objs, &tt.providers[i])
			}
			for i := range tt.agentRuntimes {
				objs = append(objs, &tt.agentRuntimes[i])
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objs...).
				Build()

			r := &AgentRuntimeReconciler{
				Client: fakeClient,
				Scheme: scheme,
			}

			requests := r.findAgentRuntimesForSecret(context.Background(), tt.secret)

			assert.Len(t, requests, tt.expectedCount)

			if tt.expectedAgents != nil {
				names := make([]string, len(requests))
				for i, req := range requests {
					names[i] = req.Name
				}
				for _, expected := range tt.expectedAgents {
					assert.Contains(t, names, expected)
				}
			}
		})
	}
}

func TestFindAgentRuntimesForProvider(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	provider := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-provider",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ProviderSpec{
			Type: "claude",
		},
	}

	agentRuntimes := []omniav1alpha1.AgentRuntime{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent-using-provider",
				Namespace: "default",
			},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				ProviderRef:   &omniav1alpha1.ProviderRef{Name: "test-provider"},
				PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent-using-other",
				Namespace: "default",
			},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				ProviderRef:   &omniav1alpha1.ProviderRef{Name: "other-provider"},
				PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
			},
		},
	}

	objs := []runtime.Object{provider}
	for i := range agentRuntimes {
		objs = append(objs, &agentRuntimes[i])
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	requests := r.findAgentRuntimesForProvider(context.Background(), provider)

	assert.Len(t, requests, 1)
	assert.Equal(t, "agent-using-provider", requests[0].Name)
}

func TestFindAgentRuntimesForProvider_NamedProviders(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	provider := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "judge-provider",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ProviderSpec{
			Type: "openai",
		},
	}

	agentRuntimes := []omniav1alpha1.AgentRuntime{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent-with-named-providers",
				Namespace: "default",
			},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				Providers: []omniav1alpha1.NamedProviderRef{
					{
						Name:        "default",
						ProviderRef: omniav1alpha1.ProviderRef{Name: "main-provider"},
					},
					{
						Name:        "judge",
						ProviderRef: omniav1alpha1.ProviderRef{Name: "judge-provider"},
					},
				},
				PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent-without-judge",
				Namespace: "default",
			},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				Providers: []omniav1alpha1.NamedProviderRef{
					{
						Name:        "default",
						ProviderRef: omniav1alpha1.ProviderRef{Name: "main-provider"},
					},
				},
				PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
			},
		},
	}

	objs := []runtime.Object{provider}
	for i := range agentRuntimes {
		objs = append(objs, &agentRuntimes[i])
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	requests := r.findAgentRuntimesForProvider(context.Background(), provider)

	assert.Len(t, requests, 1)
	assert.Equal(t, "agent-with-named-providers", requests[0].Name)
}

func TestFindAgentRuntimesForSecret_NamedProviders(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "judge-key",
			Namespace: "default",
			Labels: map[string]string{
				"omnia.altairalabs.ai/type": "credentials",
			},
		},
	}

	provider := omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "judge-provider",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ProviderSpec{
			Type:      "openai",
			SecretRef: &omniav1alpha1.SecretKeyRef{Name: "judge-key"},
		},
	}

	agentRuntime := omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-with-judge",
			Namespace: "default",
		},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Providers: []omniav1alpha1.NamedProviderRef{
				{
					Name:        "default",
					ProviderRef: omniav1alpha1.ProviderRef{Name: "main-provider"},
				},
				{
					Name:        "judge",
					ProviderRef: omniav1alpha1.ProviderRef{Name: "judge-provider"},
				},
			},
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(secret, &provider, &agentRuntime).
		Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	requests := r.findAgentRuntimesForSecret(context.Background(), secret)

	assert.Len(t, requests, 1)
	assert.Equal(t, "agent-with-judge", requests[0].Name)
}

func TestFindAgentRuntimesForPromptPack(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	promptPack := &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pack",
			Namespace: "default",
		},
		Spec: omniav1alpha1.PromptPackSpec{
			Version: "1.0.0",
		},
	}

	agentRuntimes := []omniav1alpha1.AgentRuntime{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent-using-pack",
				Namespace: "default",
			},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent-using-other",
				Namespace: "default",
			},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				PromptPackRef: omniav1alpha1.PromptPackRef{Name: "other-pack"},
			},
		},
	}

	objs := []runtime.Object{promptPack}
	for i := range agentRuntimes {
		objs = append(objs, &agentRuntimes[i])
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	requests := r.findAgentRuntimesForPromptPack(context.Background(), promptPack)

	assert.Len(t, requests, 1)
	assert.Equal(t, "agent-using-pack", requests[0].Name)
}

func TestGetSecretHash(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tests := []struct {
		name          string
		agentRuntime  *omniav1alpha1.AgentRuntime
		providers     map[string]*omniav1alpha1.Provider
		secrets       []*corev1.Secret
		expectEmpty   bool
		expectChanged bool // If true, hash should be different from empty
	}{
		{
			name: "no provider returns empty-ish hash",
			agentRuntime: &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "default",
				},
			},
			providers:   nil,
			expectEmpty: false, // Returns a hash even with no data
		},
		{
			name: "provider with secret returns hash",
			agentRuntime: &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "default",
				},
			},
			providers: map[string]*omniav1alpha1.Provider{
				"default": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-provider",
						Namespace: "default",
					},
					Spec: omniav1alpha1.ProviderSpec{
						Type:      "claude",
						SecretRef: &omniav1alpha1.SecretKeyRef{Name: "api-secret"},
					},
				},
			},
			secrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "api-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"API_KEY": []byte("test-key-value"),
					},
				},
			},
			expectEmpty:   false,
			expectChanged: true,
		},
		{
			name: "provider with missing secret still returns hash",
			agentRuntime: &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "default",
				},
			},
			providers: map[string]*omniav1alpha1.Provider{
				"default": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-provider",
						Namespace: "default",
					},
					Spec: omniav1alpha1.ProviderSpec{
						Type:      "claude",
						SecretRef: &omniav1alpha1.SecretKeyRef{Name: "nonexistent-secret"},
					},
				},
			},
			secrets:     nil,
			expectEmpty: false,
		},
		{
			name: "inline provider secret returns hash",
			agentRuntime: &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "default",
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					Provider: &omniav1alpha1.ProviderConfig{
						Type:      "openai",
						SecretRef: &corev1.LocalObjectReference{Name: "inline-secret"},
					},
				},
			},
			providers: nil,
			secrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "inline-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"OPENAI_API_KEY": []byte("openai-test-key"),
					},
				},
			},
			expectEmpty:   false,
			expectChanged: true,
		},
		{
			name: "provider without secretRef returns base hash",
			agentRuntime: &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "default",
				},
			},
			providers: map[string]*omniav1alpha1.Provider{
				"default": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mock-provider",
						Namespace: "default",
					},
					Spec: omniav1alpha1.ProviderSpec{
						Type: "mock",
						// No SecretRef
					},
				},
			},
			secrets:     nil,
			expectEmpty: false,
		},
		{
			name: "multiple providers hash all secrets",
			agentRuntime: &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "default",
				},
			},
			providers: map[string]*omniav1alpha1.Provider{
				"default": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "provider-a",
						Namespace: "default",
					},
					Spec: omniav1alpha1.ProviderSpec{
						Type:      "claude",
						SecretRef: &omniav1alpha1.SecretKeyRef{Name: "secret-a"},
					},
				},
				"judge": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "provider-b",
						Namespace: "default",
					},
					Spec: omniav1alpha1.ProviderSpec{
						Type:      "openai",
						SecretRef: &omniav1alpha1.SecretKeyRef{Name: "secret-b"},
					},
				},
			},
			secrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-a",
						Namespace: "default",
					},
					Data: map[string][]byte{"KEY": []byte("val-a")},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-b",
						Namespace: "default",
					},
					Data: map[string][]byte{"KEY": []byte("val-b")},
				},
			},
			expectEmpty:   false,
			expectChanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build fake client with objects
			var objs []runtime.Object
			for _, p := range tt.providers {
				objs = append(objs, p)
			}
			for _, s := range tt.secrets {
				objs = append(objs, s)
			}
			objs = append(objs, tt.agentRuntime)

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objs...).
				Build()

			r := &AgentRuntimeReconciler{
				Client: fakeClient,
				Scheme: scheme,
			}

			hash := r.getSecretHash(context.Background(), tt.agentRuntime, tt.providers)

			// Hash is always 16 chars (truncated)
			assert.Len(t, hash, 16, "hash should be 16 characters")

			// Calculate a baseline hash to compare
			if tt.expectChanged {
				baselineAgent := &omniav1alpha1.AgentRuntime{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "baseline-agent",
						Namespace: "default",
					},
				}
				baselineHash := r.getSecretHash(context.Background(), baselineAgent, nil)
				assert.NotEqual(t, baselineHash, hash, "hash should differ from baseline when secrets are present")
			}
		})
	}
}

func TestGetSecretHashDeterministic(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	agentRuntime := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
	}

	providers := map[string]*omniav1alpha1.Provider{
		"default": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-provider",
				Namespace: "default",
			},
			Spec: omniav1alpha1.ProviderSpec{
				Type:      "claude",
				SecretRef: &omniav1alpha1.SecretKeyRef{Name: "api-secret"},
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"B_KEY": []byte("value-b"),
			"A_KEY": []byte("value-a"),
			"C_KEY": []byte("value-c"),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(providers["default"], secret, agentRuntime).
		Build()

	r := &AgentRuntimeReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Call multiple times to verify determinism
	hash1 := r.getSecretHash(context.Background(), agentRuntime, providers)
	hash2 := r.getSecretHash(context.Background(), agentRuntime, providers)
	hash3 := r.getSecretHash(context.Background(), agentRuntime, providers)

	assert.Equal(t, hash1, hash2, "hash should be deterministic")
	assert.Equal(t, hash2, hash3, "hash should be deterministic")
}
