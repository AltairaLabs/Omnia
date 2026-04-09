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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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

	container := r.buildA2AContainer(ar, newTestPromptPack(), nil, 9999, nil)

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

	container := r.buildA2AContainer(ar, newTestPromptPack(), nil, 8080, nil)
	if container.Image != "custom:latest" {
		t.Errorf("image = %q, want %q", container.Image, "custom:latest")
	}
}

func TestBuildA2AContainer_CRDImageOverride(t *testing.T) {
	r := &AgentRuntimeReconciler{FacadeImage: "operator-default:v1"}

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.Facade.Type = omniav1alpha1.FacadeTypeA2A
	ar.Spec.Facade.Image = "crd-override:v2"

	container := r.buildA2AContainer(ar, newTestPromptPack(), nil, 8080, nil)
	if container.Image != "crd-override:v2" {
		t.Errorf("image = %q, want %q", container.Image, "crd-override:v2")
	}
}

func TestBuildA2AEnvVars(t *testing.T) {
	r := &AgentRuntimeReconciler{}

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "test-agent"
	ar.Namespace = "default"
	ar.Spec.Facade.Type = omniav1alpha1.FacadeTypeA2A
	ar.Spec.A2A = &omniav1alpha1.A2AConfig{
		TaskTTL:         strPtr("2h"),
		ConversationTTL: strPtr("45m"),
	}

	envVars := r.buildA2AEnvVars(ar, nil)

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
	// SESSION_API_URL is no longer injected by the reconciler; pods discover it via workspace status
	if _, ok := envMap["SESSION_API_URL"]; ok {
		t.Error("SESSION_API_URL should not be set on agent pods; URL is resolved per-workspace")
	}
	if envMap["OMNIA_HANDLER_MODE"] != "runtime" {
		t.Errorf("OMNIA_HANDLER_MODE = %q, want %q", envMap["OMNIA_HANDLER_MODE"], "runtime")
	}
}

func TestBuildA2AEnvVars_NoA2AConfig(t *testing.T) {
	r := &AgentRuntimeReconciler{}

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.Facade.Type = omniav1alpha1.FacadeTypeA2A

	envVars := r.buildA2AEnvVars(ar, nil)

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

	envVars := r.buildA2AEnvVars(ar, nil)

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

func TestBuildA2AEnvVars_WithClients(t *testing.T) {
	r := &AgentRuntimeReconciler{}

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.Facade.Type = omniav1alpha1.FacadeTypeA2A
	ar.Spec.A2A = &omniav1alpha1.A2AConfig{
		Clients: []omniav1alpha1.A2AClientSpec{
			{
				Name:          "agent-a",
				URL:           "http://agent-a:8080",
				ExposeAsTools: true,
				Authentication: &omniav1alpha1.A2AClientAuthConfig{
					SecretRef: &corev1.LocalObjectReference{Name: "agent-a-secret"},
				},
			},
		},
	}

	clients := []ResolvedA2AClient{
		{Name: "agent-a", URL: "http://agent-a:8080", ExposeAsTools: true, AuthTokenEnv: "OMNIA_A2A_CLIENT_TOKEN_AGENT_A"},
	}

	envVars := r.buildA2AEnvVars(ar, clients)

	envMap := make(map[string]string)
	for _, ev := range envVars {
		if ev.Value != "" {
			envMap[ev.Name] = ev.Value
		}
	}

	if envMap["OMNIA_A2A_CLIENTS"] == "" {
		t.Fatal("expected OMNIA_A2A_CLIENTS to be set")
	}

	// Verify the auth token env var is injected from secret.
	found := false
	for _, ev := range envVars {
		if ev.Name == "OMNIA_A2A_CLIENT_TOKEN_AGENT_A" && ev.ValueFrom != nil && ev.ValueFrom.SecretKeyRef != nil {
			found = true
			if ev.ValueFrom.SecretKeyRef.Name != "agent-a-secret" {
				t.Errorf("secret name = %q, want %q", ev.ValueFrom.SecretKeyRef.Name, "agent-a-secret")
			}
			if ev.ValueFrom.SecretKeyRef.Key != "token" {
				t.Errorf("secret key = %q, want %q", ev.ValueFrom.SecretKeyRef.Key, "token")
			}
		}
	}
	if !found {
		t.Error("expected OMNIA_A2A_CLIENT_TOKEN_AGENT_A env var from secret")
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

func TestIsDualProtocol(t *testing.T) {
	tests := []struct {
		name     string
		ar       *omniav1alpha1.AgentRuntime
		expected bool
	}{
		{
			name: "websocket with A2A enabled",
			ar: func() *omniav1alpha1.AgentRuntime {
				ar := &omniav1alpha1.AgentRuntime{}
				ar.Spec.Facade.Type = omniav1alpha1.FacadeTypeWebSocket
				ar.Spec.A2A = &omniav1alpha1.A2AConfig{Enabled: true}
				return ar
			}(),
			expected: true,
		},
		{
			name: "websocket without A2A",
			ar: func() *omniav1alpha1.AgentRuntime {
				ar := &omniav1alpha1.AgentRuntime{}
				ar.Spec.Facade.Type = omniav1alpha1.FacadeTypeWebSocket
				return ar
			}(),
			expected: false,
		},
		{
			name: "websocket with A2A disabled",
			ar: func() *omniav1alpha1.AgentRuntime {
				ar := &omniav1alpha1.AgentRuntime{}
				ar.Spec.Facade.Type = omniav1alpha1.FacadeTypeWebSocket
				ar.Spec.A2A = &omniav1alpha1.A2AConfig{Enabled: false}
				return ar
			}(),
			expected: false,
		},
		{
			name: "A2A primary facade (not dual-protocol)",
			ar: func() *omniav1alpha1.AgentRuntime {
				ar := &omniav1alpha1.AgentRuntime{}
				ar.Spec.Facade.Type = omniav1alpha1.FacadeTypeA2A
				ar.Spec.A2A = &omniav1alpha1.A2AConfig{Enabled: true}
				return ar
			}(),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDualProtocol(tt.ar)
			if got != tt.expected {
				t.Errorf("isDualProtocol() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBuildA2ADualProtocolEnvVars(t *testing.T) {
	r := &AgentRuntimeReconciler{}

	taskTTL := "2h"
	convTTL := "45m"

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.A2A = &omniav1alpha1.A2AConfig{
		Enabled:         true,
		TaskTTL:         &taskTTL,
		ConversationTTL: &convTTL,
		TaskStore: &omniav1alpha1.A2ATaskStoreConfig{
			Type:     omniav1alpha1.A2ATaskStoreRedis,
			RedisURL: "redis://localhost:6379/0",
		},
	}

	envVars := r.buildA2ADualProtocolEnvVars(ar)

	envMap := map[string]string{}
	for _, e := range envVars {
		if e.Value != "" {
			envMap[e.Name] = e.Value
		}
	}

	if envMap["OMNIA_A2A_TASK_TTL"] != "2h" {
		t.Errorf("expected task TTL 2h, got %s", envMap["OMNIA_A2A_TASK_TTL"])
	}
	if envMap["OMNIA_A2A_CONVERSATION_TTL"] != "45m" {
		t.Errorf("expected conversation TTL 45m, got %s", envMap["OMNIA_A2A_CONVERSATION_TTL"])
	}
	if envMap["OMNIA_A2A_TASK_STORE_TYPE"] != "redis" {
		t.Errorf("expected task store type redis, got %s", envMap["OMNIA_A2A_TASK_STORE_TYPE"])
	}
	if envMap["OMNIA_A2A_REDIS_URL"] != "redis://localhost:6379/0" {
		t.Errorf("expected redis URL, got %s", envMap["OMNIA_A2A_REDIS_URL"])
	}
}

func TestBuildA2AConfigEnvVars_FullConfig(t *testing.T) {
	taskTTL := "1h"
	convTTL := "30m"
	secretRef := &corev1.LocalObjectReference{Name: "auth-secret"}
	redisSecretRef := &corev1.LocalObjectReference{Name: "redis-secret"}

	a2a := &omniav1alpha1.A2AConfig{
		TaskTTL:         &taskTTL,
		ConversationTTL: &convTTL,
		Authentication: &omniav1alpha1.A2AAuthConfig{
			SecretRef: secretRef,
		},
		TaskStore: &omniav1alpha1.A2ATaskStoreConfig{
			Type:           omniav1alpha1.A2ATaskStoreRedis,
			RedisSecretRef: redisSecretRef,
		},
	}

	envVars := buildA2AConfigEnvVars(a2a)

	envMap := make(map[string]string)
	var secretEnvNames []string
	for _, ev := range envVars {
		if ev.Value != "" {
			envMap[ev.Name] = ev.Value
		}
		if ev.ValueFrom != nil && ev.ValueFrom.SecretKeyRef != nil {
			secretEnvNames = append(secretEnvNames, ev.Name)
		}
	}

	if envMap["OMNIA_A2A_TASK_TTL"] != "1h" {
		t.Errorf("task TTL = %q, want %q", envMap["OMNIA_A2A_TASK_TTL"], "1h")
	}
	if envMap["OMNIA_A2A_CONVERSATION_TTL"] != "30m" {
		t.Errorf("conversation TTL = %q, want %q", envMap["OMNIA_A2A_CONVERSATION_TTL"], "30m")
	}
	if envMap["OMNIA_A2A_TASK_STORE_TYPE"] != "redis" {
		t.Errorf("task store type = %q, want %q", envMap["OMNIA_A2A_TASK_STORE_TYPE"], "redis")
	}

	// Verify secret-based env vars exist.
	foundAuth := false
	foundRedis := false
	for _, name := range secretEnvNames {
		if name == "OMNIA_A2A_AUTH_TOKEN" {
			foundAuth = true
		}
		if name == "OMNIA_A2A_REDIS_URL" {
			foundRedis = true
		}
	}
	if !foundAuth {
		t.Error("expected OMNIA_A2A_AUTH_TOKEN from secret")
	}
	if !foundRedis {
		t.Error("expected OMNIA_A2A_REDIS_URL from secret")
	}
}

func TestBuildA2AConfigEnvVars_NilConfig(t *testing.T) {
	envVars := buildA2AConfigEnvVars(nil)
	if len(envVars) != 0 {
		t.Errorf("expected no env vars for nil config, got %d", len(envVars))
	}
}

func TestBuildA2AClientEnvVars_Empty(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{}
	envVars := buildA2AClientEnvVars(ar, nil)
	if envVars != nil {
		t.Errorf("expected nil for empty clients, got %d env vars", len(envVars))
	}
}

// countRedisURLEnvVars returns how many OMNIA_A2A_REDIS_URL env vars exist.
func countRedisURLEnvVars(envVars []corev1.EnvVar) int {
	n := 0
	for _, ev := range envVars {
		if ev.Name == "OMNIA_A2A_REDIS_URL" {
			n++
		}
	}
	return n
}

func TestBuildA2AConfigEnvVars_RedisURL_BothSet(t *testing.T) {
	redisSecretRef := &corev1.LocalObjectReference{Name: "redis-secret"}
	a2a := &omniav1alpha1.A2AConfig{
		TaskStore: &omniav1alpha1.A2ATaskStoreConfig{
			Type:           omniav1alpha1.A2ATaskStoreRedis,
			RedisURL:       "redis://localhost:6379",
			RedisSecretRef: redisSecretRef,
		},
	}

	envVars := buildA2AConfigEnvVars(a2a)

	if c := countRedisURLEnvVars(envVars); c != 1 {
		t.Errorf("expected exactly 1 OMNIA_A2A_REDIS_URL, got %d", c)
	}
	// Secret ref takes precedence — the env var must use ValueFrom.
	for _, ev := range envVars {
		if ev.Name == "OMNIA_A2A_REDIS_URL" {
			if ev.ValueFrom == nil || ev.ValueFrom.SecretKeyRef == nil {
				t.Error("expected OMNIA_A2A_REDIS_URL from secret ref, got plain value")
			}
			if ev.ValueFrom != nil && ev.ValueFrom.SecretKeyRef.Name != "redis-secret" {
				t.Errorf("secret name = %q, want %q", ev.ValueFrom.SecretKeyRef.Name, "redis-secret")
			}
		}
	}
}

func TestBuildA2AConfigEnvVars_RedisURL_OnlySecretRef(t *testing.T) {
	redisSecretRef := &corev1.LocalObjectReference{Name: "redis-secret"}
	a2a := &omniav1alpha1.A2AConfig{
		TaskStore: &omniav1alpha1.A2ATaskStoreConfig{
			Type:           omniav1alpha1.A2ATaskStoreRedis,
			RedisSecretRef: redisSecretRef,
		},
	}

	envVars := buildA2AConfigEnvVars(a2a)

	if c := countRedisURLEnvVars(envVars); c != 1 {
		t.Errorf("expected exactly 1 OMNIA_A2A_REDIS_URL, got %d", c)
	}
	for _, ev := range envVars {
		if ev.Name == "OMNIA_A2A_REDIS_URL" {
			if ev.ValueFrom == nil || ev.ValueFrom.SecretKeyRef == nil {
				t.Error("expected OMNIA_A2A_REDIS_URL from secret ref")
			}
		}
	}
}

func TestBuildA2AConfigEnvVars_RedisURL_OnlyPlainURL(t *testing.T) {
	a2a := &omniav1alpha1.A2AConfig{
		TaskStore: &omniav1alpha1.A2ATaskStoreConfig{
			Type:     omniav1alpha1.A2ATaskStoreRedis,
			RedisURL: "redis://localhost:6379",
		},
	}

	envVars := buildA2AConfigEnvVars(a2a)

	if c := countRedisURLEnvVars(envVars); c != 1 {
		t.Errorf("expected exactly 1 OMNIA_A2A_REDIS_URL, got %d", c)
	}
	for _, ev := range envVars {
		if ev.Name == "OMNIA_A2A_REDIS_URL" {
			if ev.Value != "redis://localhost:6379" {
				t.Errorf("plain URL = %q, want %q", ev.Value, "redis://localhost:6379")
			}
		}
	}
}

func TestBuildA2ADualProtocolEnvVars_RedisURL_BothSet(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	redisSecretRef := &corev1.LocalObjectReference{Name: "redis-secret"}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.A2A = &omniav1alpha1.A2AConfig{
		TaskStore: &omniav1alpha1.A2ATaskStoreConfig{
			Type:           omniav1alpha1.A2ATaskStoreRedis,
			RedisURL:       "redis://localhost:6379",
			RedisSecretRef: redisSecretRef,
		},
	}

	envVars := r.buildA2ADualProtocolEnvVars(ar)

	if c := countRedisURLEnvVars(envVars); c != 1 {
		t.Errorf("expected exactly 1 OMNIA_A2A_REDIS_URL, got %d", c)
	}
	for _, ev := range envVars {
		if ev.Name == "OMNIA_A2A_REDIS_URL" {
			if ev.ValueFrom == nil || ev.ValueFrom.SecretKeyRef == nil {
				t.Error("expected OMNIA_A2A_REDIS_URL from secret ref, got plain value")
			}
		}
	}
}

func TestBuildA2ADualProtocolEnvVars_RedisURL_OnlySecretRef(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	redisSecretRef := &corev1.LocalObjectReference{Name: "redis-secret"}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.A2A = &omniav1alpha1.A2AConfig{
		TaskStore: &omniav1alpha1.A2ATaskStoreConfig{
			Type:           omniav1alpha1.A2ATaskStoreRedis,
			RedisSecretRef: redisSecretRef,
		},
	}

	envVars := r.buildA2ADualProtocolEnvVars(ar)

	if c := countRedisURLEnvVars(envVars); c != 1 {
		t.Errorf("expected exactly 1 OMNIA_A2A_REDIS_URL, got %d", c)
	}
	for _, ev := range envVars {
		if ev.Name == "OMNIA_A2A_REDIS_URL" {
			if ev.ValueFrom == nil || ev.ValueFrom.SecretKeyRef == nil {
				t.Error("expected OMNIA_A2A_REDIS_URL from secret ref")
			}
		}
	}
}

func TestBuildA2ADualProtocolEnvVars_RedisURL_OnlyPlainURL(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.A2A = &omniav1alpha1.A2AConfig{
		TaskStore: &omniav1alpha1.A2ATaskStoreConfig{
			Type:     omniav1alpha1.A2ATaskStoreRedis,
			RedisURL: "redis://localhost:6379",
		},
	}

	envVars := r.buildA2ADualProtocolEnvVars(ar)

	if c := countRedisURLEnvVars(envVars); c != 1 {
		t.Errorf("expected exactly 1 OMNIA_A2A_REDIS_URL, got %d", c)
	}
	for _, ev := range envVars {
		if ev.Name == "OMNIA_A2A_REDIS_URL" {
			if ev.Value != "redis://localhost:6379" {
				t.Errorf("plain URL = %q, want %q", ev.Value, "redis://localhost:6379")
			}
		}
	}
}

func TestBuildA2ADualProtocolEnvVars_NilA2A(t *testing.T) {
	r := &AgentRuntimeReconciler{}

	ar := &omniav1alpha1.AgentRuntime{}
	// A2A is nil.

	envVars := r.buildA2ADualProtocolEnvVars(ar)
	if len(envVars) != 0 {
		t.Errorf("expected no env vars for nil A2A, got %d", len(envVars))
	}
}

func TestBuildRuntimeEnvVars_MemoryNoEnvVars(t *testing.T) {
	// Memory config is read from the CRD directly by config_crd.go.
	// The operator no longer injects OMNIA_MEMORY_* env vars.
	r := &AgentRuntimeReconciler{}

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "test-agent"
	ar.Namespace = "default"
	ar.Spec.Memory = &omniav1alpha1.MemoryConfig{
		Enabled: true,
	}

	envVars := r.buildRuntimeEnvVars(ar, nil)

	for _, ev := range envVars {
		if strings.HasPrefix(ev.Name, "OMNIA_MEMORY_") {
			t.Errorf("unexpected env var %q: memory config is read from CRD, not env vars", ev.Name)
		}
	}
}

func TestBuildRuntimeEnvVars_MemoryWithWorkspaceUID(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))

	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "demo",
			UID:  "ws-uid-abc-123",
		},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "Demo",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "omnia-demo"},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(ws).Build()
	r := &AgentRuntimeReconciler{Client: k8sClient}

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "test-agent"
	ar.Namespace = "omnia-demo"
	ar.Spec.Memory = &omniav1alpha1.MemoryConfig{Enabled: true}

	envVars := r.buildRuntimeEnvVars(ar, nil)

	var workspaceUID string
	for _, ev := range envVars {
		if ev.Name == "OMNIA_WORKSPACE_UID" {
			workspaceUID = ev.Value
		}
	}
	assert.Equal(t, "ws-uid-abc-123", workspaceUID)
}

func TestBuildRuntimeEnvVars_MemoryDisabled(t *testing.T) {
	r := &AgentRuntimeReconciler{}

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "test-agent"
	ar.Namespace = "default"
	// Memory is nil — no memory config.

	envVars := r.buildRuntimeEnvVars(ar, nil)

	for _, ev := range envVars {
		if strings.HasPrefix(ev.Name, "OMNIA_MEMORY_") {
			t.Errorf("unexpected env var %q: no memory config should produce no OMNIA_MEMORY_* vars", ev.Name)
		}
	}
}

func TestResolveSessionURLForWorkspace(t *testing.T) {
	sc := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(sc))

	makeWorkspace := func(name, namespace string, services []omniav1alpha1.ServiceGroupStatus) *omniav1alpha1.Workspace {
		ws := &omniav1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: omniav1alpha1.WorkspaceSpec{
				DisplayName: name,
				Namespace:   omniav1alpha1.NamespaceConfig{Name: namespace},
			},
		}
		ws.Status.Services = services
		return ws
	}

	tests := []struct {
		name         string
		workspaces   []*omniav1alpha1.Workspace
		namespace    string
		serviceGroup string
		want         string
	}{
		{
			name: "returns session URL when workspace matches and service group is ready",
			workspaces: []*omniav1alpha1.Workspace{
				makeWorkspace("acme", "acme-ns", []omniav1alpha1.ServiceGroupStatus{
					{Name: "default", SessionURL: "http://session-acme-default:8080", Ready: true},
				}),
			},
			namespace:    "acme-ns",
			serviceGroup: "default",
			want:         "http://session-acme-default:8080",
		},
		{
			name: "returns empty when service group is not ready",
			workspaces: []*omniav1alpha1.Workspace{
				makeWorkspace("acme", "acme-ns", []omniav1alpha1.ServiceGroupStatus{
					{Name: "default", SessionURL: "http://session-acme-default:8080", Ready: false},
				}),
			},
			namespace:    "acme-ns",
			serviceGroup: "default",
			want:         "",
		},
		{
			name: "returns empty when no workspace matches the namespace",
			workspaces: []*omniav1alpha1.Workspace{
				makeWorkspace("acme", "acme-ns", []omniav1alpha1.ServiceGroupStatus{
					{Name: "default", SessionURL: "http://session-acme-default:8080", Ready: true},
				}),
			},
			namespace:    "other-ns",
			serviceGroup: "default",
			want:         "",
		},
		{
			name: "returns empty when service group name does not match",
			workspaces: []*omniav1alpha1.Workspace{
				makeWorkspace("acme", "acme-ns", []omniav1alpha1.ServiceGroupStatus{
					{Name: "default", SessionURL: "http://session-acme-default:8080", Ready: true},
				}),
			},
			namespace:    "acme-ns",
			serviceGroup: "premium",
			want:         "",
		},
		{
			name: "matches correct service group among multiple",
			workspaces: []*omniav1alpha1.Workspace{
				makeWorkspace("acme", "acme-ns", []omniav1alpha1.ServiceGroupStatus{
					{Name: "default", SessionURL: "http://session-acme-default:8080", Ready: true},
					{Name: "premium", SessionURL: "http://session-acme-premium:8080", Ready: true},
				}),
			},
			namespace:    "acme-ns",
			serviceGroup: "premium",
			want:         "http://session-acme-premium:8080",
		},
		{
			name:         "returns empty when no workspaces exist",
			workspaces:   nil,
			namespace:    "acme-ns",
			serviceGroup: "default",
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := make([]runtime.Object, len(tt.workspaces))
			for i, ws := range tt.workspaces {
				objs[i] = ws
			}
			cl := fake.NewClientBuilder().WithScheme(sc).WithRuntimeObjects(objs...).WithStatusSubresource(&omniav1alpha1.Workspace{}).Build()
			// Populate status using the fake client's status writer.
			for _, ws := range tt.workspaces {
				assert.NoError(t, cl.Status().Update(context.Background(), ws))
			}

			r := &AgentRuntimeReconciler{Client: cl}
			got := r.resolveSessionURLForWorkspace(context.Background(), tt.namespace, tt.serviceGroup)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBuildFacadeVolumeMounts_WithConfigMap verifies that a PromptPack backed by a
// ConfigMap produces a single promptpack-config volume mount.
func TestBuildFacadeVolumeMounts_WithConfigMap(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	pp := &omniav1alpha1.PromptPack{}
	pp.Spec.Source.Type = omniav1alpha1.PromptPackSourceTypeConfigMap
	pp.Spec.Source.ConfigMapRef = &corev1.LocalObjectReference{Name: "my-pack-config"}

	mounts := r.buildFacadeVolumeMounts(pp)

	if len(mounts) != 1 {
		t.Fatalf("expected 1 volume mount, got %d", len(mounts))
	}
	if mounts[0].Name != "promptpack-config" {
		t.Errorf("mount name = %q, want %q", mounts[0].Name, "promptpack-config")
	}
	if mounts[0].MountPath != PromptPackMountPath {
		t.Errorf("mount path = %q, want %q", mounts[0].MountPath, PromptPackMountPath)
	}
	if !mounts[0].ReadOnly {
		t.Error("expected mount to be read-only")
	}
}

// TestBuildFacadeVolumeMounts_NoConfigMapRef verifies that a PromptPack without a
// ConfigMapRef produces no volume mounts.
func TestBuildFacadeVolumeMounts_NoConfigMapRef(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	pp := &omniav1alpha1.PromptPack{}
	pp.Spec.Source.Type = omniav1alpha1.PromptPackSourceTypeConfigMap
	pp.Spec.Source.ConfigMapRef = nil // no ref → nothing to mount

	mounts := r.buildFacadeVolumeMounts(pp)

	if len(mounts) != 0 {
		t.Errorf("expected 0 volume mounts, got %d", len(mounts))
	}
}

// TestBuildFacadeVolumeMounts_NonConfigMapSource verifies that a PromptPack with a
// non-ConfigMap source type produces no volume mounts.
func TestBuildFacadeVolumeMounts_NonConfigMapSource(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	pp := &omniav1alpha1.PromptPack{}
	pp.Spec.Source.Type = omniav1alpha1.PromptPackSourceType("git")
	pp.Spec.Source.ConfigMapRef = &corev1.LocalObjectReference{Name: "irrelevant"}

	mounts := r.buildFacadeVolumeMounts(pp)

	if len(mounts) != 0 {
		t.Errorf("expected 0 volume mounts for non-configmap source, got %d", len(mounts))
	}
}

// TestBuildFacadeContainer_VolumeMounts verifies that buildFacadeContainer sets
// VolumeMounts when the PromptPack source is a ConfigMap.
func TestBuildFacadeContainer_VolumeMounts(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "test-agent"
	ar.Namespace = "default"

	pp := &omniav1alpha1.PromptPack{}
	pp.Spec.Source.Type = omniav1alpha1.PromptPackSourceTypeConfigMap
	pp.Spec.Source.ConfigMapRef = &corev1.LocalObjectReference{Name: "pack-config"}

	container := r.buildFacadeContainer(ar, pp, 8080)

	if len(container.VolumeMounts) != 1 {
		t.Fatalf("expected 1 volume mount on facade container, got %d", len(container.VolumeMounts))
	}
	if container.VolumeMounts[0].Name != "promptpack-config" {
		t.Errorf("mount name = %q, want %q", container.VolumeMounts[0].Name, "promptpack-config")
	}
	if container.VolumeMounts[0].MountPath != PromptPackMountPath {
		t.Errorf("mount path = %q, want %q", container.VolumeMounts[0].MountPath, PromptPackMountPath)
	}
}

// TestBuildFacadeContainer_NoVolumeMounts verifies that buildFacadeContainer has no
// VolumeMounts when the PromptPack does not reference a ConfigMap.
func TestBuildFacadeContainer_NoVolumeMounts(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "test-agent"
	ar.Namespace = "default"

	pp := &omniav1alpha1.PromptPack{}
	pp.Spec.Source.Type = omniav1alpha1.PromptPackSourceType("git")

	container := r.buildFacadeContainer(ar, pp, 8080)

	if len(container.VolumeMounts) != 0 {
		t.Errorf("expected 0 volume mounts on facade container, got %d", len(container.VolumeMounts))
	}
}

func TestGetConfigHash_ProviderModelChange(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	provider := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
		Spec: omniav1alpha1.ProviderSpec{
			Type:  "ollama",
			Model: "qwen2.5:3b",
		},
	}
	providers := map[string]*omniav1alpha1.Provider{"default": provider}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}

	hash1 := r.getConfigHash(context.Background(), providers)
	assert.Len(t, hash1, 16)

	// Change model
	provider2 := provider.DeepCopy()
	provider2.Spec.Model = "qwen2.5:7b"
	providers2 := map[string]*omniav1alpha1.Provider{"default": provider2}

	hash2 := r.getConfigHash(context.Background(), providers2)
	assert.Len(t, hash2, 16)
	assert.NotEqual(t, hash1, hash2, "model change should produce different hash")
}
