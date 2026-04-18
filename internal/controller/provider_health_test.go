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
	"net/http"
	"net/http/httptest"
	"testing"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestResolveHealthURL(t *testing.T) {
	r := &ProviderReconciler{}

	tests := []struct {
		name     string
		provider *omniav1alpha1.Provider
		want     string
	}{
		{
			name: "mock provider returns empty",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					Type: omniav1alpha1.ProviderTypeMock,
				},
			},
			want: "",
		},
		{
			name: "claude uses default endpoint",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					Type: omniav1alpha1.ProviderTypeClaude,
				},
			},
			want: "https://api.anthropic.com",
		},
		{
			name: "openai uses default endpoint",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					Type: omniav1alpha1.ProviderTypeOpenAI,
				},
			},
			want: "https://api.openai.com",
		},
		{
			name: "gemini uses default endpoint",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					Type: omniav1alpha1.ProviderTypeGemini,
				},
			},
			want: "https://generativelanguage.googleapis.com",
		},
		{
			name: "ollama with default base URL appends /api/tags",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					Type:    omniav1alpha1.ProviderTypeOllama,
					BaseURL: "http://ollama:11434",
				},
			},
			want: "http://ollama:11434/api/tags",
		},
		{
			name: "custom base URL overrides default",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					Type:    omniav1alpha1.ProviderTypeClaude,
					BaseURL: "https://custom-proxy.example.com",
				},
			},
			want: "https://custom-proxy.example.com",
		},
		{
			name: "claude on bedrock platform returns empty (platform-hosted)",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					Type: omniav1alpha1.ProviderTypeClaude,
					Platform: &omniav1alpha1.PlatformConfig{
						Type: omniav1alpha1.PlatformTypeBedrock,
					},
				},
			},
			want: "",
		},
		{
			name: "gemini on vertex platform returns empty (platform-hosted)",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					Type: omniav1alpha1.ProviderTypeGemini,
					Platform: &omniav1alpha1.PlatformConfig{
						Type: omniav1alpha1.PlatformTypeVertex,
					},
				},
			},
			want: "",
		},
		{
			name: "openai on azure platform returns empty (platform-hosted)",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					Type: omniav1alpha1.ProviderTypeOpenAI,
					Platform: &omniav1alpha1.PlatformConfig{
						Type:     omniav1alpha1.PlatformTypeAzure,
						Endpoint: "https://example.openai.azure.com",
					},
				},
			},
			want: "",
		},
		{
			name: "vllm without base URL returns empty",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					Type: omniav1alpha1.ProviderTypeVLLM,
				},
			},
			want: "",
		},
		{
			name: "ollama without base URL returns empty",
			provider: &omniav1alpha1.Provider{
				Spec: omniav1alpha1.ProviderSpec{
					Type: omniav1alpha1.ProviderTypeOllama,
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.resolveHealthURL(tt.provider)
			if got != tt.want {
				t.Errorf("resolveHealthURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckEndpointHealth_Reachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := &ProviderReconciler{}
	err := r.checkEndpointHealth(context.Background(), srv.URL)
	if err != nil {
		t.Errorf("checkEndpointHealth() returned error for reachable server: %v", err)
	}
}

func TestCheckEndpointHealth_NonOKStillReachable(t *testing.T) {
	codes := []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound}
	for _, code := range codes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
			}))
			defer srv.Close()

			r := &ProviderReconciler{}
			err := r.checkEndpointHealth(context.Background(), srv.URL)
			if err != nil {
				t.Errorf("checkEndpointHealth() returned error for %d response: %v", code, err)
			}
		})
	}
}

func TestCheckEndpointHealth_Unreachable(t *testing.T) {
	r := &ProviderReconciler{}
	err := r.checkEndpointHealth(context.Background(), "http://127.0.0.1:1")
	if err == nil {
		t.Error("checkEndpointHealth() expected error for unreachable endpoint, got nil")
	}
}
