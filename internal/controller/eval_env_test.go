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

func TestBuildEvalEnvVars(t *testing.T) {
	tests := []struct {
		name       string
		evalConfig *omniav1alpha1.EvalConfig
		wantLen    int
		wantKeys   []string
	}{
		{
			name:       "nil config returns nil",
			evalConfig: nil,
			wantLen:    0,
		},
		{
			name:       "disabled config returns nil",
			evalConfig: &omniav1alpha1.EvalConfig{Enabled: false},
			wantLen:    0,
		},
		{
			name: "enabled with defaults",
			evalConfig: &omniav1alpha1.EvalConfig{
				Enabled: true,
			},
			wantLen:  3, // enabled + default sampling + llm judge sampling
			wantKeys: []string{envEvalsEnabled, envEvalsSamplingDef, envEvalsSamplingJudge},
		},
		{
			name: "enabled with custom sampling rates",
			evalConfig: &omniav1alpha1.EvalConfig{
				Enabled: true,
				Sampling: &omniav1alpha1.EvalSampling{
					DefaultRate:  ptr.To(int32(50)),
					LLMJudgeRate: ptr.To(int32(5)),
				},
			},
			wantLen:  3,
			wantKeys: []string{envEvalsEnabled, envEvalsSamplingDef, envEvalsSamplingJudge},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildEvalEnvVars(tt.evalConfig)
			if len(got) != tt.wantLen {
				t.Errorf("buildEvalEnvVars() returned %d env vars, want %d", len(got), tt.wantLen)
			}

			envMap := make(map[string]string)
			for _, e := range got {
				envMap[e.Name] = e.Value
			}

			for _, key := range tt.wantKeys {
				if _, ok := envMap[key]; !ok {
					t.Errorf("buildEvalEnvVars() missing key %s", key)
				}
			}
		})
	}
}

func TestBuildEvalEnvVars_Values(t *testing.T) {
	evalConfig := &omniav1alpha1.EvalConfig{
		Enabled: true,
		Sampling: &omniav1alpha1.EvalSampling{
			DefaultRate:  ptr.To(int32(75)),
			LLMJudgeRate: ptr.To(int32(20)),
		},
	}

	got := buildEvalEnvVars(evalConfig)
	envMap := make(map[string]string)
	for _, e := range got {
		envMap[e.Name] = e.Value
	}

	if envMap[envEvalsEnabled] != labelValueTrue {
		t.Errorf("OMNIA_EVALS_ENABLED = %q, want %q", envMap[envEvalsEnabled], labelValueTrue)
	}

	if envMap[envEvalsSamplingDef] != "75" {
		t.Errorf("OMNIA_EVALS_SAMPLING_DEFAULT = %q, want %q", envMap[envEvalsSamplingDef], "75")
	}

	if envMap[envEvalsSamplingJudge] != "20" {
		t.Errorf("OMNIA_EVALS_SAMPLING_LLM_JUDGE = %q, want %q", envMap[envEvalsSamplingJudge], "20")
	}
}

func TestBuildEvalSamplingEnvVars_Defaults(t *testing.T) {
	got := buildEvalSamplingEnvVars(nil)
	envMap := make(map[string]string)
	for _, e := range got {
		envMap[e.Name] = e.Value
	}

	if envMap[envEvalsSamplingDef] != "100" {
		t.Errorf("default rate = %q, want %q", envMap[envEvalsSamplingDef], "100")
	}
	if envMap[envEvalsSamplingJudge] != "10" {
		t.Errorf("llm judge rate = %q, want %q", envMap[envEvalsSamplingJudge], "10")
	}
}

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
