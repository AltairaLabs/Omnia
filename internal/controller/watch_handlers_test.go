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
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
						Type: "claude",
						Credential: &omniav1alpha1.CredentialConfig{
							SecretRef: &omniav1alpha1.SecretKeyRef{Name: "api-key"},
						},
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
						Providers: []omniav1alpha1.NamedProviderRef{
							{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "my-provider"}},
						},
						PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
					},
				},
			},
			expectedCount:  1,
			expectedAgents: []string{"my-agent"},
		},
		{
			name: "credential secret triggers agent reconcile via named provider",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "inline-secret",
					Namespace: "default",
					Labels: map[string]string{
						"omnia.altairalabs.ai/type": "credentials",
					},
				},
			},
			providers: []omniav1alpha1.Provider{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "inline-provider",
						Namespace: "default",
					},
					Spec: omniav1alpha1.ProviderSpec{
						Type: "openai",
						Credential: &omniav1alpha1.CredentialConfig{
							SecretRef: &omniav1alpha1.SecretKeyRef{Name: "inline-secret"},
						},
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
						Providers: []omniav1alpha1.NamedProviderRef{
							{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "inline-provider"}},
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
						Type: "claude",
						Credential: &omniav1alpha1.CredentialConfig{
							SecretRef: &omniav1alpha1.SecretKeyRef{Name: "api-key"},
						},
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
						Providers: []omniav1alpha1.NamedProviderRef{
							{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "my-provider"}},
						},
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
						Type: "claude",
						Credential: &omniav1alpha1.CredentialConfig{
							SecretRef: &omniav1alpha1.SecretKeyRef{Name: "shared-key"},
						},
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
						Providers: []omniav1alpha1.NamedProviderRef{
							{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "shared-provider"}},
						},
						PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent-2",
						Namespace: "default",
					},
					Spec: omniav1alpha1.AgentRuntimeSpec{
						Providers: []omniav1alpha1.NamedProviderRef{
							{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "shared-provider"}},
						},
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

// An agent reconciled before its Workspace exists gets neither the
// workspace-reader binding nor OMNIA_WORKSPACE_NAME, and no longer self-heals at
// pod startup. The Workspace watch is what lets it recover (#1875).
func TestFindAgentRuntimesForWorkspace(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	// Workspace "demo" owns namespace "omnia-demo" — distinct identifiers.
	workspace := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "demo"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "Demo",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "omnia-demo"},
		},
	}

	inWorkspace := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-in-workspace", Namespace: "omnia-demo"},
		Spec:       omniav1alpha1.AgentRuntimeSpec{PromptPackRef: omniav1alpha1.PromptPackRef{Name: "p"}},
	}
	elsewhere := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-elsewhere", Namespace: "other-ns"},
		Spec:       omniav1alpha1.AgentRuntimeSpec{PromptPackRef: omniav1alpha1.PromptPackRef{Name: "p"}},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(workspace, inWorkspace, elsewhere).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	requests := r.findAgentRuntimesForWorkspace(context.Background(), workspace)

	require.Len(t, requests, 1, "only agents in the workspace's namespace are enqueued")
	assert.Equal(t, "agent-in-workspace", requests[0].Name)
	assert.Equal(t, "omnia-demo", requests[0].Namespace)
}

// A Workspace with no namespace configured enqueues nothing rather than
// enqueueing every AgentRuntime in the cluster.
func TestFindAgentRuntimesForWorkspace_NoNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	workspace := &omniav1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "demo"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(workspace).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	assert.Empty(t, r.findAgentRuntimesForWorkspace(context.Background(), workspace))
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
				Providers: []omniav1alpha1.NamedProviderRef{
					{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "test-provider"}},
				},
				PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent-using-other",
				Namespace: "default",
			},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				Providers: []omniav1alpha1.NamedProviderRef{
					{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "other-provider"}},
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
			Type: "openai",
			Credential: &omniav1alpha1.CredentialConfig{
				SecretRef: &omniav1alpha1.SecretKeyRef{Name: "judge-key"},
			},
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

// TestFindAgentRuntimesForPromptPack exercises the unindexed fallback path
// (list-all + local filter). The PromptPack's object name is deliberately a
// hash that differs from spec.packName (matching Phase 1's deterministic
// pp-<hash> naming) to prove matching is keyed on packName, not the object
// name — a ref pointing at the old-style object name must NOT match (#1837).
func TestFindAgentRuntimesForPromptPack(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	promptPack := &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pp-a1b2c3d4e5f6",
			Namespace: "default",
		},
		Spec: omniav1alpha1.PromptPackSpec{
			PackName: "test-pack",
			Version:  "1.0.0",
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
		{
			// Old (pre-#1837) behavior matched on the PromptPack's metadata.name.
			// A ref pointing at the hashed object name must NOT match anymore.
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent-using-object-name",
				Namespace: "default",
			},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				PromptPackRef: omniav1alpha1.PromptPackRef{Name: "pp-a1b2c3d4e5f6"},
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

// TestFindAgentRuntimesForPromptPack_Indexed exercises the indexed path
// (client.MatchingFields). As above, the PromptPack's object name is a hash
// distinct from spec.packName, and a ref keyed on the old object name must
// not match via the index either (#1837).
func TestFindAgentRuntimesForPromptPack_Indexed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	matching := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "match", Namespace: "default"},
		Spec:       omniav1alpha1.AgentRuntimeSpec{PromptPackRef: omniav1alpha1.PromptPackRef{Name: "target-pack"}},
	}
	other := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "default"},
		Spec:       omniav1alpha1.AgentRuntimeSpec{PromptPackRef: omniav1alpha1.PromptPackRef{Name: "other-pack"}},
	}
	oldStyle := &omniav1alpha1.AgentRuntime{
		// References the PromptPack's hashed object name, not its packName; must not match.
		ObjectMeta: metav1.ObjectMeta{Name: "old-style", Namespace: "default"},
		Spec:       omniav1alpha1.AgentRuntimeSpec{PromptPackRef: omniav1alpha1.PromptPackRef{Name: "pp-deadbeef0001"}},
	}

	indexedClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(matching, other, oldStyle).
		WithIndex(&omniav1alpha1.AgentRuntime{}, IndexAgentRuntimeByPromptPack,
			func(obj client.Object) []string {
				return extractPromptPackRef(obj.(*omniav1alpha1.AgentRuntime))
			}).
		Build()

	r := &AgentRuntimeReconciler{Client: indexedClient, Scheme: scheme}

	promptPack := &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "pp-deadbeef0001", Namespace: "default"},
		Spec:       omniav1alpha1.PromptPackSpec{PackName: "target-pack"},
	}

	requests := r.findAgentRuntimesForPromptPack(context.Background(), promptPack)
	assert.Len(t, requests, 1)
	assert.Equal(t, "match", requests[0].Name)
}

// TestFindAgentRuntimesForPromptPack_NewerVersionObject confirms the
// packName-keyed watch (#1837) also drives the version-triggered rollout
// (#1838): the mapper enqueues on packName alone, with no version filter, so
// publishing a NEWER version-object for a tracked pack — not just any
// version-object — still enqueues the referencing AgentRuntime. A regression
// here (e.g. accidentally filtering the watch by version) would mean new
// PromptPack versions never reach maybeTriggerVersionRollout's reconcile.
func TestFindAgentRuntimesForPromptPack_NewerVersionObject(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	stableVersion := "1.0.0"
	v1 := &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "pp-v1-hash", Namespace: "default"},
		Spec:       omniav1alpha1.PromptPackSpec{PackName: "triggered-pack", Version: "1.0.0"},
	}
	v2 := &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "pp-v2-hash", Namespace: "default"},
		Spec:       omniav1alpha1.PromptPackSpec{PackName: "triggered-pack", Version: "1.1.0"},
	}
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-tracking-pack", Namespace: "default"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "triggered-pack", Version: &stableVersion},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(v1, v2, ar).
		Build()

	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	// The watch fires on the NEW (newer) version-object, v2 — not v1.
	requests := r.findAgentRuntimesForPromptPack(context.Background(), v2)

	assert.Len(t, requests, 1)
	assert.Equal(t, "agent-tracking-pack", requests[0].Name)
}

func TestFindAgentRuntimesForToolRegistry(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	otherNS := "tools-ns"

	toolRegistry := &omniav1alpha1.ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tools",
			Namespace: "default",
		},
	}

	agentRuntimes := []omniav1alpha1.AgentRuntime{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "agent-using-tools", Namespace: "default"},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				PromptPackRef:   omniav1alpha1.PromptPackRef{Name: "test-pack"},
				ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{Name: "test-tools"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "agent-using-other-tools", Namespace: "default"},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				PromptPackRef:   omniav1alpha1.PromptPackRef{Name: "test-pack"},
				ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{Name: "other-tools"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "agent-without-tools", Namespace: "default"},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
			},
		},
		{
			// References test-tools but resolves to a different namespace, so it
			// must NOT match a ToolRegistry named test-tools in "default".
			ObjectMeta: metav1.ObjectMeta{Name: "agent-cross-ns-other", Namespace: "default"},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				PromptPackRef:   omniav1alpha1.PromptPackRef{Name: "test-pack"},
				ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{Name: "test-tools", Namespace: &otherNS},
			},
		},
	}

	objs := make([]runtime.Object, 0, 1+len(agentRuntimes))
	objs = append(objs, toolRegistry)
	for i := range agentRuntimes {
		objs = append(objs, &agentRuntimes[i])
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()

	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	requests := r.findAgentRuntimesForToolRegistry(context.Background(), toolRegistry)

	assert.Len(t, requests, 1)
	assert.Equal(t, "agent-using-tools", requests[0].Name)
}

func TestFindAgentRuntimesForToolRegistry_CrossNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	toolsNS := "tools-ns"

	toolRegistry := &omniav1alpha1.ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-tools", Namespace: toolsNS},
	}

	// AgentRuntime in "default" references the ToolRegistry in "tools-ns".
	agent := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-cross-ns", Namespace: "default"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef:   omniav1alpha1.PromptPackRef{Name: "test-pack"},
			ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{Name: "shared-tools", Namespace: &toolsNS},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(toolRegistry, agent).
		Build()

	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	requests := r.findAgentRuntimesForToolRegistry(context.Background(), toolRegistry)

	assert.Len(t, requests, 1)
	assert.Equal(t, "agent-cross-ns", requests[0].Name)
}

func TestFindAgentRuntimesForToolRegistry_Indexed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)

	matching := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "match", Namespace: "default"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{Name: "target-tools"},
		},
	}
	other := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "default"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{Name: "other-tools"},
		},
	}

	indexedClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(matching, other).
		WithIndex(&omniav1alpha1.AgentRuntime{}, IndexAgentRuntimeByToolRegistry,
			func(obj client.Object) []string {
				return extractToolRegistryRef(obj)
			}).
		Build()

	r := &AgentRuntimeReconciler{Client: indexedClient, Scheme: scheme}

	toolRegistry := &omniav1alpha1.ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{Name: "target-tools", Namespace: "default"},
	}

	requests := r.findAgentRuntimesForToolRegistry(context.Background(), toolRegistry)
	assert.Len(t, requests, 1)
	assert.Equal(t, "match", requests[0].Name)
}

func TestGetConfigHash(t *testing.T) {
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
			name: "no provider returns empty hash",
			agentRuntime: &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "default",
				},
			},
			providers:   nil,
			expectEmpty: true,
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
						Type: "claude",
						Credential: &omniav1alpha1.CredentialConfig{
							SecretRef: &omniav1alpha1.SecretKeyRef{Name: "api-secret"},
						},
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
						Type: "claude",
						Credential: &omniav1alpha1.CredentialConfig{
							SecretRef: &omniav1alpha1.SecretKeyRef{Name: "nonexistent-secret"},
						},
					},
				},
			},
			secrets:     nil,
			expectEmpty: false,
		},
		{
			name: "named provider secret returns hash",
			agentRuntime: &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent",
					Namespace: "default",
				},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					Providers: []omniav1alpha1.NamedProviderRef{
						{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "openai-provider"}},
					},
				},
			},
			providers: map[string]*omniav1alpha1.Provider{
				"default": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "openai-provider",
						Namespace: "default",
					},
					Spec: omniav1alpha1.ProviderSpec{
						Type: "openai",
						Credential: &omniav1alpha1.CredentialConfig{
							SecretRef: &omniav1alpha1.SecretKeyRef{Name: "inline-secret"},
						},
					},
				},
			},
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
						Type: "claude",
						Credential: &omniav1alpha1.CredentialConfig{
							SecretRef: &omniav1alpha1.SecretKeyRef{Name: "secret-a"},
						},
					},
				},
				"judge": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "provider-b",
						Namespace: "default",
					},
					Spec: omniav1alpha1.ProviderSpec{
						Type: "openai",
						Credential: &omniav1alpha1.CredentialConfig{
							SecretRef: &omniav1alpha1.SecretKeyRef{Name: "secret-b"},
						},
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

			hash := r.getConfigHash(context.Background(), tt.providers, nil, nil)

			if tt.expectEmpty {
				assert.Empty(t, hash, "hash should be empty when no providers")
			} else {
				// Hash is always 16 chars (truncated)
				assert.Len(t, hash, 16, "hash should be 16 characters")
			}

			// Calculate a baseline hash to compare
			if tt.expectChanged {
				baselineHash := r.getConfigHash(context.Background(), nil, nil, nil)
				assert.NotEqual(t, baselineHash, hash, "hash should differ from baseline when secrets are present")
			}
		})
	}
}

func TestGetConfigHashDeterministic(t *testing.T) {
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
				Type: "claude",
				Credential: &omniav1alpha1.CredentialConfig{
					SecretRef: &omniav1alpha1.SecretKeyRef{Name: "api-secret"},
				},
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
	hash1 := r.getConfigHash(context.Background(), providers, nil, nil)
	hash2 := r.getConfigHash(context.Background(), providers, nil, nil)
	hash3 := r.getConfigHash(context.Background(), providers, nil, nil)

	assert.Equal(t, hash1, hash2, "hash should be deterministic")
	assert.Equal(t, hash2, hash3, "hash should be deterministic")
}
