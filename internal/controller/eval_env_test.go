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

package controller

import (
	"testing"

	"k8s.io/utils/ptr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestIsPromptKit(t *testing.T) {
	tests := []struct {
		name string
		spec *omniav1alpha1.AgentRuntimeSpec
		want bool
	}{
		{
			name: "nil framework defaults to PromptKit",
			spec: &omniav1alpha1.AgentRuntimeSpec{},
			want: true,
		},
		{
			name: "explicit PromptKit",
			spec: &omniav1alpha1.AgentRuntimeSpec{
				Framework: &omniav1alpha1.FrameworkConfig{
					Type: omniav1alpha1.FrameworkTypePromptKit,
				},
			},
			want: true,
		},
		{
			name: "LangChain is not PromptKit",
			spec: &omniav1alpha1.AgentRuntimeSpec{
				Framework: &omniav1alpha1.FrameworkConfig{
					Type: omniav1alpha1.FrameworkTypeLangChain,
				},
			},
			want: false,
		},
		{
			name: "Custom is not PromptKit",
			spec: &omniav1alpha1.AgentRuntimeSpec{
				Framework: &omniav1alpha1.FrameworkConfig{
					Type: omniav1alpha1.FrameworkTypeCustom,
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPromptKit(tt.spec); got != tt.want {
				t.Errorf("isPromptKit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasEvalsEnabled(t *testing.T) {
	tests := []struct {
		name string
		spec *omniav1alpha1.AgentRuntimeSpec
		want bool
	}{
		{
			name: "nil evals",
			spec: &omniav1alpha1.AgentRuntimeSpec{},
			want: false,
		},
		{
			name: "evals disabled",
			spec: &omniav1alpha1.AgentRuntimeSpec{
				Evals: &omniav1alpha1.EvalConfig{Enabled: false},
			},
			want: false,
		},
		{
			name: "evals enabled",
			spec: &omniav1alpha1.AgentRuntimeSpec{
				Evals: &omniav1alpha1.EvalConfig{Enabled: true},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasEvalsEnabled(tt.spec); got != tt.want {
				t.Errorf("hasEvalsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEffectiveSecretRef(t *testing.T) {
	tests := []struct {
		name     string
		provider *omniav1alpha1.Provider
		wantName string
		wantKey  *string
		wantNil  bool
	}{
		{
			name: "no secret refs returns nil",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{},
			},
			wantNil: true,
		},
		{
			name: "legacy secretRef only",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					SecretRef: &omniav1alpha1.SecretKeyRef{
						Name: "legacy-secret",
					},
				},
			},
			wantName: "legacy-secret",
		},
		{
			name: "credential.secretRef only",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					Credential: &omniav1alpha1.CredentialConfig{
						SecretRef: &omniav1alpha1.SecretKeyRef{
							Name: "cred-secret",
							Key:  ptr.To("my-key"),
						},
					},
				},
			},
			wantName: "cred-secret",
			wantKey:  ptr.To("my-key"),
		},
		{
			name: "credential.secretRef preferred over legacy",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					SecretRef: &omniav1alpha1.SecretKeyRef{
						Name: "legacy-secret",
					},
					Credential: &omniav1alpha1.CredentialConfig{
						SecretRef: &omniav1alpha1.SecretKeyRef{
							Name: "cred-secret",
						},
					},
				},
			},
			wantName: "cred-secret",
		},
		{
			name: "credential without secretRef falls back to legacy",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					SecretRef: &omniav1alpha1.SecretKeyRef{
						Name: "legacy-secret",
					},
					Credential: &omniav1alpha1.CredentialConfig{},
				},
			},
			wantName: "legacy-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectiveSecretRef(tt.provider)
			if tt.wantNil {
				if got != nil {
					t.Errorf("effectiveSecretRef() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("effectiveSecretRef() = nil, want non-nil")
			}
			if got.Name != tt.wantName {
				t.Errorf("effectiveSecretRef().Name = %q, want %q", got.Name, tt.wantName)
			}
			if tt.wantKey != nil {
				if got.Key == nil || *got.Key != *tt.wantKey {
					t.Errorf("effectiveSecretRef().Key = %v, want %q", got.Key, *tt.wantKey)
				}
			}
		})
	}
}
