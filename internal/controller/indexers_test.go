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
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestExtractProviderRefs_ProvidersField(t *testing.T) {
	ns := "ns1"
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: ns},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Providers: []omniav1alpha1.NamedProviderRef{
				{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "openai-prod"}},
				{Name: "judge", ProviderRef: omniav1alpha1.ProviderRef{Name: "claude-prod"}},
			},
		},
	}

	refs := extractProviderRefs(ar)
	assert.ElementsMatch(t, []string{"ns1/openai-prod", "ns1/claude-prod"}, refs)
}

func TestExtractProviderRefs_CrossNamespace(t *testing.T) {
	otherNS := "shared"
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "app"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Providers: []omniav1alpha1.NamedProviderRef{
				{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{
					Name:      "openai-prod",
					Namespace: &otherNS,
				}},
			},
		},
	}

	refs := extractProviderRefs(ar)
	assert.Equal(t, []string{"shared/openai-prod"}, refs)
}

func TestExtractProviderRefs_SingleProvider(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns1"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Providers: []omniav1alpha1.NamedProviderRef{
				{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "legacy-provider"}},
			},
		},
	}

	refs := extractProviderRefs(ar)
	assert.Equal(t, []string{"ns1/legacy-provider"}, refs)
}

func TestExtractProviderRefs_Deduplicates(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns1"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Providers: []omniav1alpha1.NamedProviderRef{
				{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "same-provider"}},
				{Name: "judge", ProviderRef: omniav1alpha1.ProviderRef{Name: "same-provider"}},
			},
		},
	}

	refs := extractProviderRefs(ar)
	assert.Equal(t, []string{"ns1/same-provider"}, refs)
}

func TestExtractProviderRefs_Empty(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns1"},
		Spec:       omniav1alpha1.AgentRuntimeSpec{},
	}

	refs := extractProviderRefs(ar)
	assert.Empty(t, refs)
}

func TestExtractPromptPackRef(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns1"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "my-prompts"},
		},
	}

	refs := extractPromptPackRef(ar)
	assert.Equal(t, []string{"my-prompts"}, refs)
}

func TestExtractPromptPackRef_Empty(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns1"},
		Spec:       omniav1alpha1.AgentRuntimeSpec{},
	}

	refs := extractPromptPackRef(ar)
	assert.Empty(t, refs)
}

func TestProviderRefKey(t *testing.T) {
	ns := "other-ns"
	tests := []struct {
		name      string
		ref       omniav1alpha1.ProviderRef
		defaultNS string
		expected  string
	}{
		{
			name:      "uses default namespace",
			ref:       omniav1alpha1.ProviderRef{Name: "my-provider"},
			defaultNS: "default",
			expected:  "default/my-provider",
		},
		{
			name:      "uses explicit namespace",
			ref:       omniav1alpha1.ProviderRef{Name: "my-provider", Namespace: &ns},
			defaultNS: "default",
			expected:  "other-ns/my-provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, providerRefKey(tt.ref, tt.defaultNS))
		})
	}
}
