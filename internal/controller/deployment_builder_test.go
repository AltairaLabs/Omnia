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

	corev1 "k8s.io/api/core/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func newTestPromptPack() *omniav1alpha1.PromptPack {
	pp := &omniav1alpha1.PromptPack{}
	pp.Name = "test-pack"
	pp.Spec.Source.Type = omniav1alpha1.PromptPackSourceTypeConfigMap
	pp.Spec.Source.ConfigMapRef = &corev1.LocalObjectReference{Name: "test-pack-config"}
	return pp
}

func TestBuildA2AContainer(t *testing.T) {
	r := &AgentRuntimeReconciler{}

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "test-agent"
	ar.Namespace = "default"
	ar.Spec.Facade.Type = omniav1alpha1.FacadeTypeA2A
	ar.Spec.PromptPackRef.Name = "test-pack"

	container := r.buildA2AContainer(ar, newTestPromptPack(), nil, 9999)

	if container.Name != FacadeContainerName {
		t.Errorf("container name = %q, want %q", container.Name, FacadeContainerName)
	}
	if len(container.Ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(container.Ports))
	}
	if container.Ports[0].ContainerPort != 9999 {
		t.Errorf("port = %d, want 9999", container.Ports[0].ContainerPort)
	}
	if container.Ports[0].Name != "facade" {
		t.Errorf("port name = %q, want %q", container.Ports[0].Name, "facade")
	}
	if container.ReadinessProbe == nil {
		t.Fatal("expected readiness probe")
	}
	if container.LivenessProbe == nil {
		t.Fatal("expected liveness probe")
	}
	if container.Image != DefaultFacadeImage {
		t.Errorf("image = %q, want %q", container.Image, DefaultFacadeImage)
	}
}

func TestBuildA2AContainer_CustomImage(t *testing.T) {
	r := &AgentRuntimeReconciler{FacadeImage: "custom:latest"}

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.Facade.Type = omniav1alpha1.FacadeTypeA2A

	container := r.buildA2AContainer(ar, newTestPromptPack(), nil, 8080)
	if container.Image != "custom:latest" {
		t.Errorf("image = %q, want %q", container.Image, "custom:latest")
	}
}

func TestBuildA2AContainer_CRDImageOverride(t *testing.T) {
	r := &AgentRuntimeReconciler{FacadeImage: "operator-default:v1"}

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.Facade.Type = omniav1alpha1.FacadeTypeA2A
	ar.Spec.Facade.Image = "crd-override:v2"

	container := r.buildA2AContainer(ar, newTestPromptPack(), nil, 8080)
	if container.Image != "crd-override:v2" {
		t.Errorf("image = %q, want %q", container.Image, "crd-override:v2")
	}
}

func TestBuildA2AEnvVars(t *testing.T) {
	r := &AgentRuntimeReconciler{SessionAPIURL: "http://session-api:8080"}

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "test-agent"
	ar.Namespace = "default"
	ar.Spec.Facade.Type = omniav1alpha1.FacadeTypeA2A
	ar.Spec.A2A = &omniav1alpha1.A2AConfig{
		TaskTTL:         strPtr("2h"),
		ConversationTTL: strPtr("45m"),
	}

	envVars := r.buildA2AEnvVars(ar)

	envMap := make(map[string]string)
	for _, ev := range envVars {
		if ev.Value != "" {
			envMap[ev.Name] = ev.Value
		}
	}

	if envMap["OMNIA_FACADE_TYPE"] != "a2a" {
		t.Errorf("OMNIA_FACADE_TYPE = %q, want %q", envMap["OMNIA_FACADE_TYPE"], "a2a")
	}
	if envMap["OMNIA_A2A_TASK_TTL"] != "2h" {
		t.Errorf("OMNIA_A2A_TASK_TTL = %q, want %q", envMap["OMNIA_A2A_TASK_TTL"], "2h")
	}
	if envMap["OMNIA_A2A_CONVERSATION_TTL"] != "45m" {
		t.Errorf("OMNIA_A2A_CONVERSATION_TTL = %q, want %q", envMap["OMNIA_A2A_CONVERSATION_TTL"], "45m")
	}
	if envMap["SESSION_API_URL"] != "http://session-api:8080" {
		t.Errorf("SESSION_API_URL = %q, want %q", envMap["SESSION_API_URL"], "http://session-api:8080")
	}
	if envMap["OMNIA_HANDLER_MODE"] != "runtime" {
		t.Errorf("OMNIA_HANDLER_MODE = %q, want %q", envMap["OMNIA_HANDLER_MODE"], "runtime")
	}
}

func TestBuildA2AEnvVars_NoA2AConfig(t *testing.T) {
	r := &AgentRuntimeReconciler{}

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.Facade.Type = omniav1alpha1.FacadeTypeA2A

	envVars := r.buildA2AEnvVars(ar)

	envMap := make(map[string]string)
	for _, ev := range envVars {
		if ev.Value != "" {
			envMap[ev.Name] = ev.Value
		}
	}

	if envMap["OMNIA_FACADE_TYPE"] != "a2a" {
		t.Errorf("OMNIA_FACADE_TYPE = %q, want %q", envMap["OMNIA_FACADE_TYPE"], "a2a")
	}
	// No A2A TTL env vars should be set when A2A config is nil
	if _, ok := envMap["OMNIA_A2A_TASK_TTL"]; ok {
		t.Error("OMNIA_A2A_TASK_TTL should not be set when A2A config is nil")
	}
}

func TestBuildA2AEnvVars_WithTracing(t *testing.T) {
	r := &AgentRuntimeReconciler{
		TracingEnabled:  true,
		TracingEndpoint: "otel-collector:4317",
	}

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.Facade.Type = omniav1alpha1.FacadeTypeA2A

	envVars := r.buildA2AEnvVars(ar)

	envMap := make(map[string]string)
	for _, ev := range envVars {
		if ev.Value != "" {
			envMap[ev.Name] = ev.Value
		}
	}

	if envMap["OMNIA_TRACING_ENABLED"] != "true" {
		t.Errorf("OMNIA_TRACING_ENABLED = %q, want %q", envMap["OMNIA_TRACING_ENABLED"], "true")
	}
	if envMap["OMNIA_TRACING_ENDPOINT"] != "otel-collector:4317" {
		t.Errorf("OMNIA_TRACING_ENDPOINT = %q, want %q", envMap["OMNIA_TRACING_ENDPOINT"], "otel-collector:4317")
	}
}

func strPtr(s string) *string { return &s }

func TestDefaultImageForFramework(t *testing.T) {
	tests := []struct {
		name      string
		framework *omniav1alpha1.FrameworkConfig
		want      string
	}{
		{
			name:      "nil framework returns default PromptKit image",
			framework: nil,
			want:      DefaultFrameworkImage,
		},
		{
			name: "LangChain framework returns LangChain image",
			framework: &omniav1alpha1.FrameworkConfig{
				Type: omniav1alpha1.FrameworkTypeLangChain,
			},
			want: DefaultLangChainImage,
		},
		{
			name: "PromptKit framework returns PromptKit image",
			framework: &omniav1alpha1.FrameworkConfig{
				Type: omniav1alpha1.FrameworkTypePromptKit,
			},
			want: DefaultFrameworkImage,
		},
		{
			name: "AutoGen framework returns default image (fallback)",
			framework: &omniav1alpha1.FrameworkConfig{
				Type: omniav1alpha1.FrameworkTypeAutoGen,
			},
			want: DefaultFrameworkImage,
		},
		{
			name: "Unknown framework type returns default image",
			framework: &omniav1alpha1.FrameworkConfig{
				Type: "unknown",
			},
			want: DefaultFrameworkImage,
		},
		{
			name: "Empty framework type returns default image",
			framework: &omniav1alpha1.FrameworkConfig{
				Type: "",
			},
			want: DefaultFrameworkImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultImageForFramework(tt.framework)
			if got != tt.want {
				t.Errorf("defaultImageForFramework() = %v, want %v", got, tt.want)
			}
		})
	}
}
