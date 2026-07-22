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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
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
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeA2A}}
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
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeA2A}}

	container := r.buildA2AContainer(ar, newTestPromptPack(), nil, 8080, nil)
	if container.Image != "custom:latest" {
		t.Errorf("image = %q, want %q", container.Image, "custom:latest")
	}
}

func TestBuildA2AContainer_CRDImageOverride(t *testing.T) {
	r := &AgentRuntimeReconciler{FacadeImage: "operator-default:v1"}

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeA2A, Image: "crd-override:v2"}}

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
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeA2A, A2A: &omniav1alpha1.A2AConfig{
		TaskTTL:         strPtr("2h"),
		ConversationTTL: strPtr("45m"),
	}}}

	envVars := r.buildA2AEnvVars(ar, nil, nil)

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
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeA2A}}

	envVars := r.buildA2AEnvVars(ar, nil, nil)

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
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeA2A}}

	envVars := r.buildA2AEnvVars(ar, nil, nil)

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

func TestBuildA2AEnvVars_InjectsResolvedPromptPackVersion(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeA2A, A2A: &omniav1alpha1.A2AConfig{}}}

	// Standalone A2A writes the session record, so a track:-resolved version must
	// reach it via env (#1847) — the same guarantee as split facade/runtime.
	pack := &omniav1alpha1.PromptPack{Spec: omniav1alpha1.PromptPackSpec{Version: "2.1.0"}}
	envVars := r.buildA2AEnvVars(ar, pack, nil)

	got := ""
	for _, ev := range envVars {
		if ev.Name == envPromptPackVersion {
			got = ev.Value
		}
	}
	if got != "2.1.0" {
		t.Errorf("%s = %q, want %q", envPromptPackVersion, got, "2.1.0")
	}

	// Nil pack must not inject the var (and must not panic).
	for _, ev := range r.buildA2AEnvVars(ar, nil, nil) {
		if ev.Name == envPromptPackVersion {
			t.Errorf("%s must be absent for a nil PromptPack", envPromptPackVersion)
		}
	}
}

func TestBuildA2AEnvVars_WithClients(t *testing.T) {
	r := &AgentRuntimeReconciler{}

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeA2A, A2A: &omniav1alpha1.A2AConfig{
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
	}}}

	clients := []ResolvedA2AClient{
		{Name: "agent-a", URL: "http://agent-a:8080", ExposeAsTools: true, AuthTokenEnv: "OMNIA_A2A_CLIENT_TOKEN_AGENT_A"},
	}

	envVars := r.buildA2AEnvVars(ar, nil, clients)

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

func TestBuiltinDefaultImage(t *testing.T) {
	tests := []struct {
		name          string
		frameworkType string
		want          string
	}{
		{
			name:          "PromptKit returns PromptKit image",
			frameworkType: string(omniav1alpha1.FrameworkTypePromptKit),
			want:          DefaultFrameworkImage,
		},
		{
			// custom-runtime wave 1: LangChain has no built-in image; it must
			// be configured explicitly or block loudly.
			name:          "LangChain has no built-in image",
			frameworkType: string(omniav1alpha1.FrameworkTypeLangChain),
			want:          "",
		},
		{
			// #1206: AutoGen must NOT silently fall back to the PromptKit image.
			name:          "AutoGen has no built-in image",
			frameworkType: string(omniav1alpha1.FrameworkTypeAutoGen),
			want:          "",
		},
		{
			name:          "Unknown framework type has no built-in image",
			frameworkType: "unknown",
			want:          "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := builtinDefaultImage(tt.frameworkType)
			if got != tt.want {
				t.Errorf("builtinDefaultImage(%q) = %q, want %q", tt.frameworkType, got, tt.want)
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
			name: "websocket + a2a facade is dual-protocol",
			ar: func() *omniav1alpha1.AgentRuntime {
				ar := &omniav1alpha1.AgentRuntime{}
				ar.Spec.Facades = []omniav1alpha1.FacadeConfig{
					{Type: omniav1alpha1.FacadeTypeWebSocket},
					{Type: omniav1alpha1.FacadeTypeA2A, A2A: &omniav1alpha1.A2AConfig{}},
				}
				return ar
			}(),
			expected: true,
		},
		{
			name: "websocket only",
			ar: func() *omniav1alpha1.AgentRuntime {
				ar := &omniav1alpha1.AgentRuntime{}
				ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket}}
				return ar
			}(),
			expected: false,
		},
		{
			name: "A2A primary facade (not dual-protocol)",
			ar: func() *omniav1alpha1.AgentRuntime {
				ar := &omniav1alpha1.AgentRuntime{}
				ar.Spec.Facades = []omniav1alpha1.FacadeConfig{
					{Type: omniav1alpha1.FacadeTypeA2A, A2A: &omniav1alpha1.A2AConfig{}},
				}
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

func TestIsMCPEnabled(t *testing.T) {
	tests := []struct {
		name     string
		ar       *omniav1alpha1.AgentRuntime
		expected bool
	}{
		{
			name: "function-mode with mcp facade",
			ar: func() *omniav1alpha1.AgentRuntime {
				ar := &omniav1alpha1.AgentRuntime{}
				ar.Spec.Mode = "function"
				ar.Spec.Facades = []omniav1alpha1.FacadeConfig{
					{Type: omniav1alpha1.FacadeTypeREST},
					{Type: omniav1alpha1.FacadeTypeMCP, MCP: &omniav1alpha1.MCPConfig{Enabled: true}},
				}
				return ar
			}(),
			expected: true,
		},
		{
			name: "function-mode rest only (no mcp facade)",
			ar: func() *omniav1alpha1.AgentRuntime {
				ar := &omniav1alpha1.AgentRuntime{}
				ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeREST}}
				return ar
			}(),
			expected: false,
		},
		{
			name:     "no facades",
			ar:       &omniav1alpha1.AgentRuntime{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMCPEnabled(tt.ar)
			if got != tt.expected {
				t.Errorf("isMCPEnabled() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMCPPort(t *testing.T) {
	custom := int32(9000)
	tests := []struct {
		name     string
		ar       *omniav1alpha1.AgentRuntime
		expected int32
	}{
		{
			name: "custom port",
			ar: func() *omniav1alpha1.AgentRuntime {
				ar := &omniav1alpha1.AgentRuntime{}
				ar.Spec.Facades = []omniav1alpha1.FacadeConfig{
					{Type: omniav1alpha1.FacadeTypeMCP, MCP: &omniav1alpha1.MCPConfig{Enabled: true, Port: &custom}},
				}
				return ar
			}(),
			expected: custom,
		},
		{
			name: "default port when MCP facade without port",
			ar: func() *omniav1alpha1.AgentRuntime {
				ar := &omniav1alpha1.AgentRuntime{}
				ar.Spec.Facades = []omniav1alpha1.FacadeConfig{
					{Type: omniav1alpha1.FacadeTypeMCP, MCP: &omniav1alpha1.MCPConfig{Enabled: true}},
				}
				return ar
			}(),
			expected: DefaultMCPPort,
		},
		{
			name:     "default port when mcp facade absent",
			ar:       &omniav1alpha1.AgentRuntime{},
			expected: DefaultMCPPort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mcpPort(tt.ar)
			if got != tt.expected {
				t.Errorf("mcpPort() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestApplyMCPFacadeOptions_AppendsPortAndEnv(t *testing.T) {
	enabled := true
	port := int32(9500)
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Mode: "function",
			Facades: []omniav1alpha1.FacadeConfig{
				{Type: omniav1alpha1.FacadeTypeREST},
				{Type: omniav1alpha1.FacadeTypeMCP, MCP: &omniav1alpha1.MCPConfig{Enabled: enabled, Port: &port}},
			},
		},
	}
	facade := &corev1.Container{}
	applyMCPFacadeOptions(facade, ar)

	if len(facade.Ports) != 1 || facade.Ports[0].Name != portNameMCP || facade.Ports[0].ContainerPort != 9500 {
		t.Errorf("Ports: %+v want one mcp port :9500", facade.Ports)
	}
	envMap := envVarMap(facade.Env)
	if envMap["OMNIA_MCP_ENABLED"] != "true" {
		t.Errorf("OMNIA_MCP_ENABLED = %q want true", envMap["OMNIA_MCP_ENABLED"])
	}
	if envMap["OMNIA_MCP_PORT"] != "9500" {
		t.Errorf("OMNIA_MCP_PORT = %q want 9500", envMap["OMNIA_MCP_PORT"])
	}
}

func TestApplyMCPFacadeOptions_NoopWhenNoMCPFacade(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Facades: []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeREST}},
		},
	}
	facade := &corev1.Container{}
	applyMCPFacadeOptions(facade, ar)

	if len(facade.Ports) != 0 {
		t.Errorf("Ports must be empty when MCP disabled; got %+v", facade.Ports)
	}
	if len(facade.Env) != 0 {
		t.Errorf("Env must be empty when MCP disabled; got %+v", facade.Env)
	}
}

func TestApplyMCPFacadeOptions_DefaultPortWhenUnset(t *testing.T) {
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Facades: []omniav1alpha1.FacadeConfig{
				{Type: omniav1alpha1.FacadeTypeREST},
				{Type: omniav1alpha1.FacadeTypeMCP, MCP: &omniav1alpha1.MCPConfig{Enabled: true}},
			},
		},
	}
	facade := &corev1.Container{}
	applyMCPFacadeOptions(facade, ar)

	if len(facade.Ports) != 1 || facade.Ports[0].ContainerPort != DefaultMCPPort {
		t.Errorf("Ports: %+v want default port %d", facade.Ports, DefaultMCPPort)
	}
}

func TestAppendMCPServicePort_AppendsWhenEnabled(t *testing.T) {
	port := int32(9500)
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Facades: []omniav1alpha1.FacadeConfig{
				{Type: omniav1alpha1.FacadeTypeREST},
				{Type: omniav1alpha1.FacadeTypeMCP, MCP: &omniav1alpha1.MCPConfig{Enabled: true, Port: &port}},
			},
		},
	}
	got := appendMCPServicePort(nil, ar)
	if len(got) != 1 || got[0].Name != "mcp" || got[0].Port != 9500 {
		t.Errorf("appendMCPServicePort: %+v want one mcp port :9500", got)
	}
}

func TestAppendMCPServicePort_NoopWhenDisabled(t *testing.T) {
	existing := []corev1.ServicePort{{Name: "facade", Port: 8080}}
	ar := &omniav1alpha1.AgentRuntime{}
	got := appendMCPServicePort(existing, ar)
	if len(got) != 1 || got[0].Name != "facade" {
		t.Errorf("appendMCPServicePort: %+v want unchanged single-port slice", got)
	}
}

func TestSetAgentPortAppProtocols(t *testing.T) {
	// Every facade protocol (websocket/a2a/rest/mcp) is HTTP, so all ports get
	// appProtocol=http regardless of name.
	ports := []corev1.ServicePort{{Name: "facade"}, {Name: "a2a"}, {Name: portNameMCP}}
	setAgentPortAppProtocols(ports)
	for _, p := range ports {
		if p.AppProtocol == nil {
			t.Fatalf("port %q has nil appProtocol", p.Name)
		}
		if *p.AppProtocol != appProtocolHTTP {
			t.Errorf("port %q appProtocol = %q want %q", p.Name, *p.AppProtocol, appProtocolHTTP)
		}
	}
}

func TestBuildA2ADualProtocolEnvVars(t *testing.T) {
	r := &AgentRuntimeReconciler{}

	taskTTL := "2h"
	convTTL := "45m"

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{
		{Type: omniav1alpha1.FacadeTypeWebSocket},
		{Type: omniav1alpha1.FacadeTypeA2A, A2A: &omniav1alpha1.A2AConfig{
			Enabled:         true,
			TaskTTL:         &taskTTL,
			ConversationTTL: &convTTL,
			TaskStore: &omniav1alpha1.A2ATaskStoreConfig{
				Type:     omniav1alpha1.A2ATaskStoreRedis,
				RedisURL: "redis://localhost:6379/0",
			},
		}},
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
	redisSecretRef := &corev1.LocalObjectReference{Name: testRedisSecretName}

	a2a := &omniav1alpha1.A2AConfig{
		TaskTTL:         &taskTTL,
		ConversationTTL: &convTTL,
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

	// Verify secret-based env vars exist. A2A data-plane auth is no longer
	// carried on A2AConfig (it moved to spec.externalAuth), so only the redis
	// URL secret remains.
	foundRedis := false
	for _, name := range secretEnvNames {
		if name == "OMNIA_A2A_REDIS_URL" {
			foundRedis = true
		}
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
	redisSecretRef := &corev1.LocalObjectReference{Name: testRedisSecretName}
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
			if ev.ValueFrom != nil && ev.ValueFrom.SecretKeyRef.Name != testRedisSecretName {
				t.Errorf("secret name = %q, want %q", ev.ValueFrom.SecretKeyRef.Name, testRedisSecretName)
			}
		}
	}
}

func TestBuildA2AConfigEnvVars_RedisURL_OnlySecretRef(t *testing.T) {
	redisSecretRef := &corev1.LocalObjectReference{Name: testRedisSecretName}
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
	redisSecretRef := &corev1.LocalObjectReference{Name: testRedisSecretName}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{
		{Type: omniav1alpha1.FacadeTypeWebSocket},
		{Type: omniav1alpha1.FacadeTypeA2A, A2A: &omniav1alpha1.A2AConfig{
			TaskStore: &omniav1alpha1.A2ATaskStoreConfig{
				Type:           omniav1alpha1.A2ATaskStoreRedis,
				RedisURL:       "redis://localhost:6379",
				RedisSecretRef: redisSecretRef,
			},
		}},
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
	redisSecretRef := &corev1.LocalObjectReference{Name: testRedisSecretName}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{
		{Type: omniav1alpha1.FacadeTypeWebSocket},
		{Type: omniav1alpha1.FacadeTypeA2A, A2A: &omniav1alpha1.A2AConfig{
			TaskStore: &omniav1alpha1.A2ATaskStoreConfig{
				Type:           omniav1alpha1.A2ATaskStoreRedis,
				RedisSecretRef: redisSecretRef,
			},
		}},
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
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{
		{Type: omniav1alpha1.FacadeTypeWebSocket},
		{Type: omniav1alpha1.FacadeTypeA2A, A2A: &omniav1alpha1.A2AConfig{
			TaskStore: &omniav1alpha1.A2ATaskStoreConfig{
				Type:     omniav1alpha1.A2ATaskStoreRedis,
				RedisURL: "redis://localhost:6379",
			},
		}},
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

	envVars := r.buildRuntimeEnvVars(ar, nil, nil)

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

	envVars := r.buildRuntimeEnvVars(ar, nil, nil)

	var workspaceUID string
	for _, ev := range envVars {
		if ev.Name == "OMNIA_WORKSPACE_UID" {
			workspaceUID = ev.Value
		}
	}
	assert.Equal(t, "ws-uid-abc-123", workspaceUID)
}

// Service discovery always needs the workspace name, so it is injected even
// when memory is off — unlike the UID, which is memory-only. The value is the
// Workspace's metadata.name ("demo"), never the namespace it owns
// ("omnia-demo"): the name is what RBAC resourceNames match (#1875).
func TestBuildRuntimeEnvVars_InjectsWorkspaceNameWithoutMemory(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))

	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", UID: "ws-uid-abc-123"},
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
	// Memory deliberately not enabled.

	envVars := r.buildRuntimeEnvVars(ar, nil, nil)

	var workspaceName, workspaceUID string
	for _, ev := range envVars {
		switch ev.Name {
		case "OMNIA_WORKSPACE_NAME":
			workspaceName = ev.Value
		case "OMNIA_WORKSPACE_UID":
			workspaceUID = ev.Value
		}
	}
	assert.Equal(t, "demo", workspaceName)
	assert.NotEqual(t, "omnia-demo", workspaceName, "namespace injected instead of workspace name")
	assert.Empty(t, workspaceUID, "UID stays memory-gated")
}

func TestBuildRuntimeEnvVars_MemoryDisabled(t *testing.T) {
	r := &AgentRuntimeReconciler{}

	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "test-agent"
	ar.Namespace = "default"
	// Memory is nil — no memory config.

	envVars := r.buildRuntimeEnvVars(ar, nil, nil)

	for _, ev := range envVars {
		if strings.HasPrefix(ev.Name, "OMNIA_MEMORY_") {
			t.Errorf("unexpected env var %q: no memory config should produce no OMNIA_MEMORY_* vars", ev.Name)
		}
	}
}

// TestRuntimeEnv_ContextURLFromStoreRef verifies that when spec.context is
// configured with a Redis store and a storeRef secret, buildRuntimeEnvVars
// injects OMNIA_CONTEXT_URL sourced from that secret so the runtime can
// connect to Redis for durable context state.
func TestRuntimeEnv_ContextURLFromStoreRef(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Context: &omniav1alpha1.ContextConfig{
				Type: omniav1alpha1.ContextStoreTypeRedis,
				StoreRef: &corev1.LocalObjectReference{
					Name: testRedisSecretName,
				},
			},
		},
	}
	env := r.buildRuntimeEnvVars(ar, nil, nil)

	e := findEnvVar(env, "OMNIA_CONTEXT_URL")
	if e == nil || e.ValueFrom == nil || e.ValueFrom.SecretKeyRef == nil {
		t.Fatalf("OMNIA_CONTEXT_URL not sourced from secret: %+v", e)
	}
	if e.ValueFrom.SecretKeyRef.Name != testRedisSecretName {
		t.Fatalf("wrong secret: got %q, want %q",
			e.ValueFrom.SecretKeyRef.Name, testRedisSecretName)
	}
	if e.ValueFrom.SecretKeyRef.Key != testRedisSecretKey {
		t.Fatalf("wrong secret key: got %q, want %q",
			e.ValueFrom.SecretKeyRef.Key, testRedisSecretKey)
	}
}

// TestRuntimeEnv_NoContextURLForMemory verifies that a memory-backed context
// store does not inject OMNIA_CONTEXT_URL — there is no Redis to connect to.
func TestRuntimeEnv_NoContextURLForMemory(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Context: &omniav1alpha1.ContextConfig{
				Type: omniav1alpha1.ContextStoreTypeMemory,
			},
		},
	}
	env := r.buildRuntimeEnvVars(ar, nil, nil)
	if findEnvVar(env, "OMNIA_CONTEXT_URL") != nil {
		t.Fatal("OMNIA_CONTEXT_URL must not be set for memory store")
	}
}

// TestBuildRuntimeEnvVars_SkillManifestPathKeyedOnResolvedPack is the #1837
// Task 5 regression: OMNIA_PROMPTPACK_MANIFEST_PATH must be keyed on the
// RESOLVED PromptPack's object name, not agentRuntime.Spec.PromptPackRef.Name
// (the logical packName/label value). PromptPackReconciler.reconcileSkills
// writes the manifest to <root>/manifests/<pack.Name>.json using the
// resolved object's own name — if the runtime asked for a path keyed on the
// ref name instead, the two would permanently disagree once PromptPacks are
// label-keyed multi-version objects (ref.Name != resolved object's Name).
func TestBuildRuntimeEnvVars_SkillManifestPathKeyedOnResolvedPack(t *testing.T) {
	r := &AgentRuntimeReconciler{WorkspaceContentPath: "/workspace-content"}
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "mypack"},
		},
	}
	promptPack := &omniav1alpha1.PromptPack{}
	promptPack.Name = "pp-deadbeef01234567"

	env := r.buildRuntimeEnvVars(ar, promptPack, nil)

	e := findEnvVar(env, "OMNIA_PROMPTPACK_MANIFEST_PATH")
	require.NotNil(t, e, "OMNIA_PROMPTPACK_MANIFEST_PATH must be set when WorkspaceContentPath is configured")
	assert.Equal(t, "/workspace-content/manifests/pp-deadbeef01234567.json", e.Value,
		"manifest path must be keyed on the resolved pack's object name, not the ref name %q", ar.Spec.PromptPackRef.Name)
}

// TestBuildRuntimeEnvVars_NoSkillManifestPathWithoutResolvedPack guards the
// nil-safety of the change above: buildRuntimeEnvVars is called with a nil
// promptPack in several existing unit tests, and reconcileReferences can
// return before a pack is resolved — the env var must simply be omitted
// rather than panicking on a nil dereference.
func TestBuildRuntimeEnvVars_NoSkillManifestPathWithoutResolvedPack(t *testing.T) {
	r := &AgentRuntimeReconciler{WorkspaceContentPath: "/workspace-content"}
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "mypack"},
		},
	}

	env := r.buildRuntimeEnvVars(ar, nil, nil)

	assert.Nil(t, findEnvVar(env, "OMNIA_PROMPTPACK_MANIFEST_PATH"),
		"manifest path env var must be omitted, not panic, when no PromptPack has been resolved")
}

// TestBuildRuntimeEnvVars_PromptPackVersion is the #1847 regression: the
// runtime container must receive the RESOLVED PromptPack's concrete version
// via OMNIA_PROMPTPACK_VERSION so a `track:` (or default-stable) AgentRuntime
// — whose spec.promptPackRef.Version is nil — still stamps a concrete
// version on sessions, instead of an empty string that makes the EE eval
// loader re-resolve to stable-max and diverge from what was actually
// deployed for prerelease-track agents.
func TestBuildRuntimeEnvVars_PromptPackVersion(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "mypack"},
		},
	}
	promptPack := &omniav1alpha1.PromptPack{}
	promptPack.Spec.Version = "2.3.0"

	env := r.buildRuntimeEnvVars(ar, promptPack, nil)

	e := findEnvVar(env, "OMNIA_PROMPTPACK_VERSION")
	require.NotNil(t, e, "OMNIA_PROMPTPACK_VERSION must be set for a resolved PromptPack")
	assert.Equal(t, "2.3.0", e.Value)
}

// TestBuildRuntimeEnvVars_NoPromptPackVersionWithoutResolvedPack guards the
// nil-safety of the change above: a nil promptPack (no pack resolved yet)
// must omit the env var rather than panic.
func TestBuildRuntimeEnvVars_NoPromptPackVersionWithoutResolvedPack(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "mypack"},
		},
	}

	env := r.buildRuntimeEnvVars(ar, nil, nil)

	assert.Nil(t, findEnvVar(env, "OMNIA_PROMPTPACK_VERSION"),
		"version env var must be omitted, not panic, when no PromptPack has been resolved")
}

// TestBuildFacadeContainer_PromptPackVersionEnv is the facade half of the
// #1847 fix: the facade container writes the session record (per
// architecture, off the gRPC bus), so the eval-path version stamp comes from
// the FACADE's config, not just the runtime's — the facade container must
// also carry OMNIA_PROMPTPACK_VERSION from the resolved pack.
func TestBuildFacadeContainer_PromptPackVersionEnv(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "test-agent"
	ar.Namespace = "default"

	pp := &omniav1alpha1.PromptPack{}
	pp.Spec.Version = "2.3.0"

	container := r.buildFacadeContainer(ar, pp, 8080)

	e := findEnvVar(container.Env, "OMNIA_PROMPTPACK_VERSION")
	require.NotNil(t, e, "OMNIA_PROMPTPACK_VERSION must be set on the facade container for a resolved PromptPack")
	assert.Equal(t, "2.3.0", e.Value)
}

// TestBuildFacadeContainer_NoPromptPackVersionForUnversionedPack guards the
// empty-version case: a resolved-but-unversioned PromptPack (defensive only —
// version is CRD-required) must omit the env var, not emit an empty value.
func TestBuildFacadeContainer_NoPromptPackVersionForUnversionedPack(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "test-agent"
	ar.Namespace = "default"

	container := r.buildFacadeContainer(ar, &omniav1alpha1.PromptPack{}, 8080)

	assert.Nil(t, findEnvVar(container.Env, "OMNIA_PROMPTPACK_VERSION"),
		"version env var must be omitted when the resolved PromptPack has no version")
}

// TestAppendPromptPackVersionEnv_NilPromptPack is a direct unit test of the
// shared helper's nil-safety (independent of buildFacadeContainer, whose
// volume-mount path has its own, unrelated nil-promptPack invariant): a nil
// promptPack must return the input slice unchanged, not panic.
func TestAppendPromptPackVersionEnv_NilPromptPack(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	in := []corev1.EnvVar{{Name: "EXISTING", Value: "x"}}

	out := r.appendPromptPackVersionEnv(in, nil)

	assert.Equal(t, in, out)
	assert.Nil(t, findEnvVar(out, "OMNIA_PROMPTPACK_VERSION"))
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

// findPromptpackConfigMount returns the promptpack-config VolumeMount
// or nil. Inlined here because the previous shared helper lived in
// mgmt_plane_pubkey_wiring_test.go, which was removed when the JWKS-
// based mgmt-plane validator replaced the per-workspace pubkey
// ConfigMap mirror — and it's only ever asked for promptpack-config.
func findPromptpackConfigMount(mounts []corev1.VolumeMount) *corev1.VolumeMount {
	for i := range mounts {
		if mounts[i].Name == "promptpack-config" {
			return &mounts[i]
		}
	}
	return nil
}

// TestBuildFacadeVolumeMounts_WithConfigMap verifies that a PromptPack backed by a
// ConfigMap produces a promptpack-config volume mount.
func TestBuildFacadeVolumeMounts_WithConfigMap(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	pp := &omniav1alpha1.PromptPack{}
	pp.Spec.Source.Type = omniav1alpha1.PromptPackSourceTypeConfigMap
	pp.Spec.Source.ConfigMapRef = &corev1.LocalObjectReference{Name: "my-pack-config"}

	mounts := r.buildFacadeVolumeMounts(&omniav1alpha1.AgentRuntime{}, pp)

	mount := findPromptpackConfigMount(mounts)
	if mount == nil {
		t.Fatalf("expected promptpack-config mount, got %+v", mounts)
	}
	if mount.MountPath != PromptPackMountPath {
		t.Errorf("mount path = %q, want %q", mount.MountPath, PromptPackMountPath)
	}
	if !mount.ReadOnly {
		t.Error("expected mount to be read-only")
	}
}

// TestBuildFacadeVolumeMounts_NoConfigMapRef verifies that a PromptPack without a
// ConfigMapRef produces no promptpack-config mount.
func TestBuildFacadeVolumeMounts_NoConfigMapRef(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	pp := &omniav1alpha1.PromptPack{}
	pp.Spec.Source.Type = omniav1alpha1.PromptPackSourceTypeConfigMap
	pp.Spec.Source.ConfigMapRef = nil // no ref → no promptpack-config mount

	mounts := r.buildFacadeVolumeMounts(&omniav1alpha1.AgentRuntime{}, pp)

	if m := findPromptpackConfigMount(mounts); m != nil {
		t.Errorf("did not expect promptpack-config mount, got %+v", m)
	}
}

// TestBuildFacadeVolumeMounts_NonConfigMapSource verifies that a PromptPack with a
// non-ConfigMap source type produces no promptpack-config mount.
func TestBuildFacadeVolumeMounts_NonConfigMapSource(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	pp := &omniav1alpha1.PromptPack{}
	pp.Spec.Source.Type = omniav1alpha1.PromptPackSourceType("git")
	pp.Spec.Source.ConfigMapRef = &corev1.LocalObjectReference{Name: "irrelevant"}

	mounts := r.buildFacadeVolumeMounts(&omniav1alpha1.AgentRuntime{}, pp)

	if m := findPromptpackConfigMount(mounts); m != nil {
		t.Errorf("did not expect promptpack-config mount for non-configmap source, got %+v", m)
	}
}

// TestBuildFacadeContainer_VolumeMounts verifies that buildFacadeContainer sets
// the promptpack-config VolumeMount when the PromptPack source is a ConfigMap.
func TestBuildFacadeContainer_VolumeMounts(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "test-agent"
	ar.Namespace = "default"

	pp := &omniav1alpha1.PromptPack{}
	pp.Spec.Source.Type = omniav1alpha1.PromptPackSourceTypeConfigMap
	pp.Spec.Source.ConfigMapRef = &corev1.LocalObjectReference{Name: "pack-config"}

	container := r.buildFacadeContainer(ar, pp, 8080)

	mount := findPromptpackConfigMount(container.VolumeMounts)
	if mount == nil {
		t.Fatalf("expected promptpack-config on facade container, got %+v", container.VolumeMounts)
	}
	if mount.MountPath != PromptPackMountPath {
		t.Errorf("mount path = %q, want %q", mount.MountPath, PromptPackMountPath)
	}
}

// TestBuildFacadeContainer_NoVolumeMounts verifies that buildFacadeContainer has
// no promptpack-config VolumeMount when the PromptPack does not reference a
// ConfigMap.
func TestBuildFacadeContainer_NoVolumeMounts(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "test-agent"
	ar.Namespace = "default"

	pp := &omniav1alpha1.PromptPack{}
	pp.Spec.Source.Type = omniav1alpha1.PromptPackSourceType("git")

	container := r.buildFacadeContainer(ar, pp, 8080)

	if m := findPromptpackConfigMount(container.VolumeMounts); m != nil {
		t.Errorf("did not expect promptpack-config mount, got %+v", m)
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

	hash1 := r.getConfigHash(context.Background(), providers, nil, nil)
	assert.Len(t, hash1, 16)

	// Change model
	provider2 := provider.DeepCopy()
	provider2.Spec.Model = "qwen2.5:7b"
	providers2 := map[string]*omniav1alpha1.Provider{"default": provider2}

	hash2 := r.getConfigHash(context.Background(), providers2, nil, nil)
	assert.Len(t, hash2, 16)
	assert.NotEqual(t, hash1, hash2, "model change should produce different hash")
}

func TestGetConfigHash_FieldSensitivity(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}
	ctx := context.Background()

	baseProvider := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
		Spec: omniav1alpha1.ProviderSpec{
			Type:    "openai",
			Model:   "gpt-4o",
			BaseURL: "https://api.openai.com/v1",
			Defaults: &omniav1alpha1.ProviderDefaults{
				Temperature:   ptr.To("0.7"),
				ContextWindow: ptr.To(int32(128000)),
			},
			Pricing: &omniav1alpha1.ProviderPricing{
				InputCostPer1K:  ptr.To("0.005"),
				OutputCostPer1K: ptr.To("0.015"),
			},
		},
	}

	baseHash := r.getConfigHash(ctx, map[string]*omniav1alpha1.Provider{"default": baseProvider}, nil, nil)
	assert.NotEmpty(t, baseHash, "baseline hash must not be empty")

	cases := []struct {
		name   string
		mutate func(p *omniav1alpha1.Provider)
	}{
		{
			name:   "type",
			mutate: func(p *omniav1alpha1.Provider) { p.Spec.Type = "anthropic" },
		},
		{
			name:   "model",
			mutate: func(p *omniav1alpha1.Provider) { p.Spec.Model = "gpt-4o-mini" },
		},
		{
			name:   "baseURL",
			mutate: func(p *omniav1alpha1.Provider) { p.Spec.BaseURL = "https://custom.example.com/v1" },
		},
		{
			name:   "temperature",
			mutate: func(p *omniav1alpha1.Provider) { p.Spec.Defaults.Temperature = ptr.To("1.0") },
		},
		{
			name:   "contextWindow",
			mutate: func(p *omniav1alpha1.Provider) { p.Spec.Defaults.ContextWindow = ptr.To(int32(32000)) },
		},
		{
			name:   "inputCost",
			mutate: func(p *omniav1alpha1.Provider) { p.Spec.Pricing.InputCostPer1K = ptr.To("0.010") },
		},
		{
			name:   "outputCost",
			mutate: func(p *omniav1alpha1.Provider) { p.Spec.Pricing.OutputCostPer1K = ptr.To("0.030") },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mutated := baseProvider.DeepCopy()
			tc.mutate(mutated)
			hash := r.getConfigHash(ctx, map[string]*omniav1alpha1.Provider{"default": mutated}, nil, nil)
			assert.NotEqual(t, baseHash, hash, "mutating %s should change the hash", tc.name)
		})
	}
}

func TestGetConfigHash_MultiProviderOrder(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}
	ctx := context.Background()

	p1 := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
		Spec:       omniav1alpha1.ProviderSpec{Type: "openai", Model: "gpt-4o"},
	}
	p2 := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "default"},
		Spec:       omniav1alpha1.ProviderSpec{Type: "anthropic", Model: "claude-3-5-sonnet-20241022"},
	}

	hash1 := r.getConfigHash(ctx, map[string]*omniav1alpha1.Provider{"default": p1, "judge": p2}, nil, nil)
	hash2 := r.getConfigHash(ctx, map[string]*omniav1alpha1.Provider{"judge": p2, "default": p1}, nil, nil)

	assert.Equal(t, hash1, hash2, "hash must be deterministic regardless of map iteration order")
}

func TestGetConfigHash_EmptyProviders(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &AgentRuntimeReconciler{Client: fakeClient, Scheme: scheme}
	ctx := context.Background()

	assert.Empty(t, r.getConfigHash(ctx, nil, nil, nil), "nil providers should return empty string")
	assert.Empty(t, r.getConfigHash(ctx, map[string]*omniav1alpha1.Provider{}, nil, nil), "empty providers map should return empty string")
}

func TestGetConfigHash_RollsOnPackOrRegistryChange(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = omniav1alpha1.AddToScheme(scheme)
	r := &AgentRuntimeReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Scheme: scheme}
	ctx := context.Background()

	pack := &omniav1alpha1.PromptPack{ObjectMeta: metav1.ObjectMeta{Name: "p", Generation: 1}}
	reg := &omniav1alpha1.ToolRegistry{ObjectMeta: metav1.ObjectMeta{Name: "r", Generation: 1}}

	base := r.getConfigHash(ctx, nil, pack, reg)
	assert.NotEmpty(t, base, "pack/registry alone must produce a hash (so the pod can roll on their changes)")

	// A ToolRegistry spec change bumps Generation -> the hash must change so the
	// pod rolls and picks up the new tools. This is the reconciliation bug fix:
	// previously the hash ignored the registry entirely.
	regBumped := &omniav1alpha1.ToolRegistry{ObjectMeta: metav1.ObjectMeta{Name: "r", Generation: 2}}
	assert.NotEqual(t, base, r.getConfigHash(ctx, nil, pack, regBumped),
		"a ToolRegistry change must change the config hash")

	packBumped := &omniav1alpha1.PromptPack{ObjectMeta: metav1.ObjectMeta{Name: "p", Generation: 2}}
	assert.NotEqual(t, base, r.getConfigHash(ctx, nil, packBumped, reg),
		"a PromptPack change must change the config hash")
}

// TestGetConfigHash_DiffersAcrossResolvedPackVersions is the #1837 Task 5
// regression: forward resolution now returns a PromptPack whose
// metadata.name is a deterministic pp-<hash> that CHANGES per version, so
// channel-max re-selecting a newer version hands getConfigHash a *different*
// resolved object (a new pp-<hash> name) even when Generation on that fresh
// object is 1, same as the one it replaced. getConfigHash must already be
// hashing the resolved pack's Name (not the ref/logical packName, which is
// stable across versions) for the pod to roll when channel-max re-selects.
func TestGetConfigHash_DiffersAcrossResolvedPackVersions(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = omniav1alpha1.AddToScheme(scheme)
	r := &AgentRuntimeReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Scheme: scheme}
	ctx := context.Background()

	// Same logical pack (Spec.PackName), same Generation — only the resolved
	// object identity differs, exactly as channel-max re-selection produces:
	// a brand new pp-<hash> object for the newly-selected version.
	v1 := &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "pp-aaaaaaaa", Generation: 1},
		Spec:       omniav1alpha1.PromptPackSpec{PackName: "mypack", Version: "1.0.0"},
	}
	v2 := &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: "pp-bbbbbbbb", Generation: 1},
		Spec:       omniav1alpha1.PromptPackSpec{PackName: "mypack", Version: "1.1.0"},
	}

	hashV1 := r.getConfigHash(ctx, nil, v1, nil)
	hashV2 := r.getConfigHash(ctx, nil, v2, nil)
	assert.NotEqual(t, hashV1, hashV2,
		"channel-max re-selecting a newer version must change the config hash so candidate/stable pods roll, even at identical Generation")
}

func TestBuildDeploymentSpec_SelectorExcludesMutableModeLabel(t *testing.T) {
	// Regression for #1108 review B2: Deployment selectors are immutable
	// after creation. The mode label must live ONLY on the pod template,
	// never in Spec.Selector.MatchLabels — otherwise reconcile fails
	// with `field is immutable` when an AgentRuntime's mode changes.
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "selector-test"
	ar.Namespace = "ns"
	ar.Spec.Mode = omniav1alpha1.AgentRuntimeModeFunction
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeREST}}
	ar.Spec.PromptPackRef.Name = "p"

	dep := &appsv1.Deployment{}
	r.buildDeploymentSpec(context.Background(), dep, ar, newTestPromptPack(), nil, "", nil)

	require.NotNil(t, dep.Spec.Selector, "selector must be set")
	if _, ok := dep.Spec.Selector.MatchLabels["omnia.altairalabs.ai/mode"]; ok {
		t.Fatalf("Selector.MatchLabels must NOT include mutable mode label; "+
			"got selector=%v", dep.Spec.Selector.MatchLabels)
	}

	// The mode label MUST still appear on the pod template for ops visibility.
	podLabels := dep.Spec.Template.Labels
	require.Equal(t, "function", podLabels["omnia.altairalabs.ai/mode"],
		"pod template must carry the mode label")

	// Sanity: every selector key is also on the pod (otherwise the
	// Deployment wouldn't be valid).
	for k, v := range dep.Spec.Selector.MatchLabels {
		require.Equal(t, v, podLabels[k],
			"selector key %q must be present and identical on pod template", k)
	}
}

func TestBuildDeploymentSpec_PodOverrides(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "a"
	ar.Namespace = "ns"
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket}}
	ar.Spec.PromptPackRef.Name = "p"
	ar.Spec.PodOverrides = &omniav1alpha1.PodOverrides{
		ServiceAccountName: "wli-sa",
		Annotations:        map[string]string{"azure.workload.identity/use": "true"},
		NodeSelector:       map[string]string{"gpu": "a100"},
		ImagePullSecrets:   []corev1.LocalObjectReference{{Name: "regcred"}},
		ExtraVolumes: []corev1.Volume{{
			Name: "kv",
			VolumeSource: corev1.VolumeSource{
				CSI: &corev1.CSIVolumeSource{Driver: "secrets-store.csi.k8s.io"},
			},
		}},
		ExtraEnvFrom: []corev1.EnvFromSource{{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "kv-secret"}},
		}},
	}

	dep := &appsv1.Deployment{}
	r.buildDeploymentSpec(context.Background(), dep, ar, newTestPromptPack(), nil, "", nil)

	spec := dep.Spec.Template.Spec
	require.Equal(t, "wli-sa", spec.ServiceAccountName, "ServiceAccountName override")
	require.Equal(t, "true", dep.Spec.Template.Annotations["azure.workload.identity/use"], "workload-identity annotation")
	require.Equal(t, "a100", spec.NodeSelector["gpu"], "nodeSelector")
	require.NotEmpty(t, spec.ImagePullSecrets, "imagePullSecrets")
	require.Equal(t, "regcred", spec.ImagePullSecrets[0].Name)

	foundVol := false
	for _, v := range spec.Volumes {
		if v.Name == "kv" && v.CSI != nil {
			foundVol = true
		}
	}
	require.True(t, foundVol, "extraVolume kv must be appended")

	require.GreaterOrEqual(t, len(spec.Containers), 2, "facade+runtime containers")
	for _, c := range spec.Containers {
		foundEnvFrom := false
		for _, e := range c.EnvFrom {
			if e.SecretRef != nil && e.SecretRef.Name == "kv-secret" {
				foundEnvFrom = true
			}
		}
		require.True(t, foundEnvFrom, "container %s missing extraEnvFrom", c.Name)
	}
}

func TestHardenedPodSecurityContext(t *testing.T) {
	sc := hardenedPodSecurityContext()
	require.NotNil(t, sc)
	require.NotNil(t, sc.RunAsNonRoot)
	assert.True(t, *sc.RunAsNonRoot)
	require.NotNil(t, sc.RunAsUser)
	assert.Equal(t, int64(65532), *sc.RunAsUser)
	require.NotNil(t, sc.RunAsGroup)
	assert.Equal(t, int64(65532), *sc.RunAsGroup)
	require.NotNil(t, sc.FSGroup)
	assert.Equal(t, int64(65532), *sc.FSGroup)
	require.NotNil(t, sc.SeccompProfile)
	assert.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, sc.SeccompProfile.Type)
}

func TestHardenedContainerSecurityContext(t *testing.T) {
	sc := hardenedContainerSecurityContext()
	require.NotNil(t, sc)
	require.NotNil(t, sc.AllowPrivilegeEscalation)
	assert.False(t, *sc.AllowPrivilegeEscalation)
	require.NotNil(t, sc.ReadOnlyRootFilesystem)
	assert.True(t, *sc.ReadOnlyRootFilesystem)
	require.NotNil(t, sc.RunAsNonRoot)
	assert.True(t, *sc.RunAsNonRoot)
	require.NotNil(t, sc.Capabilities)
	assert.Equal(t, []corev1.Capability{"ALL"}, sc.Capabilities.Drop)
	require.Empty(t, sc.Capabilities.Add)
	require.NotNil(t, sc.SeccompProfile)
	assert.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, sc.SeccompProfile.Type)
}

func TestBuildDeploymentSpec_HardenedSecurityContext(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Facades: []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket}},
		},
	}
	pp := newTestPromptPack()
	r := &AgentRuntimeReconciler{Scheme: scheme, Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	dep := &appsv1.Deployment{}
	r.buildDeploymentSpec(context.Background(), dep, ar, pp, nil, "", nil)

	// Pod-level SecurityContext is hardened
	spec := dep.Spec.Template.Spec
	require.NotNil(t, spec.SecurityContext, "pod SecurityContext must be set")
	require.NotNil(t, spec.SecurityContext.RunAsNonRoot)
	assert.True(t, *spec.SecurityContext.RunAsNonRoot)
	require.NotNil(t, spec.SecurityContext.SeccompProfile)
	assert.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, spec.SecurityContext.SeccompProfile.Type)

	// Every container has hardened container-level SecurityContext
	require.NotEmpty(t, spec.Containers)
	for _, c := range spec.Containers {
		require.NotNilf(t, c.SecurityContext, "container %s missing SecurityContext", c.Name)
		require.NotNilf(t, c.SecurityContext.ReadOnlyRootFilesystem, "container %s missing ReadOnlyRootFilesystem", c.Name)
		assert.Truef(t, *c.SecurityContext.ReadOnlyRootFilesystem, "container %s must have ReadOnlyRootFilesystem=true", c.Name)
		require.NotNilf(t, c.SecurityContext.AllowPrivilegeEscalation, "container %s missing AllowPrivilegeEscalation", c.Name)
		assert.Falsef(t, *c.SecurityContext.AllowPrivilegeEscalation, "container %s must have AllowPrivilegeEscalation=false", c.Name)
		require.NotNilf(t, c.SecurityContext.Capabilities, "container %s missing Capabilities", c.Name)
		assert.Equalf(t, []corev1.Capability{"ALL"}, c.SecurityContext.Capabilities.Drop, "container %s must drop ALL capabilities", c.Name)
	}
}

// containerPortByName returns the named container port, or nil. Test helper for
// the metrics-port discovery contract assertions below.
func containerPortByName(c *corev1.Container, name string) *corev1.ContainerPort {
	for i := range c.Ports {
		if c.Ports[i].Name == name {
			return &c.Ports[i]
		}
	}
	return nil
}

// TestBuildDeploymentSpec_MetricsPortContract is a regression guard for #1488:
// the facade metrics endpoint (8081) was never scraped because the pod
// advertised prometheus.io/port=8080 (no /metrics there) and relied on a
// sidecar-only merge-metrics assumption. The fix is a port-NAME contract: every
// metrics-serving container declares a port named "metrics", so pod
// service-discovery finds every endpoint regardless of port number. This test
// asserts that contract on the agent pod and that the misleading single-port
// annotations are gone.
func TestBuildDeploymentSpec_MetricsPortContract(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "metrics-contract"
	ar.Namespace = "ns"
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket}}
	ar.Spec.PromptPackRef.Name = "p"

	dep := &appsv1.Deployment{}
	r.buildDeploymentSpec(context.Background(), dep, ar, newTestPromptPack(), nil, "", nil)

	// Both metrics-serving containers must declare a "metrics"-named port so a
	// single name-keyed scrape job / PodMonitor covers the whole pod.
	var facadeC, runtimeC *corev1.Container
	for i := range dep.Spec.Template.Spec.Containers {
		c := &dep.Spec.Template.Spec.Containers[i]
		switch c.Name {
		case FacadeContainerName:
			facadeC = c
		case RuntimeContainerName:
			runtimeC = c
		}
	}
	require.NotNil(t, facadeC, "facade container must be present")
	require.NotNil(t, runtimeC, "runtime container must be present")

	facadeMetrics := containerPortByName(facadeC, metricsPortName)
	require.NotNil(t, facadeMetrics, "facade must declare a %q port", metricsPortName)
	assert.Equal(t, int32(DefaultFacadeHealthPort), facadeMetrics.ContainerPort,
		"facade metrics port number")

	runtimeMetrics := containerPortByName(runtimeC, metricsPortName)
	require.NotNil(t, runtimeMetrics, "runtime must declare a %q port", metricsPortName)
	assert.Equal(t, int32(DefaultRuntimeHealthPort), runtimeMetrics.ContainerPort,
		"runtime metrics port number")

	// The misleading single-port annotations must be gone, and BOTH metrics
	// ports must be excluded from sidecar mTLS so a direct scrape works.
	anno := dep.Spec.Template.Annotations
	_, hasPort := anno["prometheus.io/port"]
	assert.False(t, hasPort, "prometheus.io/port must not be set (cannot express two metrics ports)")
	_, hasMerge := anno["prometheus.istio.io/merge-metrics"]
	assert.False(t, hasMerge, "sidecar-only merge-metrics assumption must be removed")
	assert.Equal(t,
		fmt.Sprintf("%d,%d,%d", DefaultFacadeHealthPort, DefaultRuntimeHealthPort, DefaultPolicyBrokerHealthPort),
		anno["traffic.sidecar.istio.io/excludeInboundPorts"],
		"all three metrics ports must be excluded from sidecar inbound interception")
}

func TestDeployment_GracePeriodFromDrainTimeout(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "drain-timeout-test"
	ar.Namespace = "ns"
	ar.Spec.PromptPackRef.Name = "p"
	d := "2m"
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket, DrainTimeout: &d}}

	dep := &appsv1.Deployment{}
	r.buildDeploymentSpec(context.Background(), dep, ar, newTestPromptPack(), nil, "", nil)

	require.NotNil(t, dep.Spec.Template.Spec.TerminationGracePeriodSeconds)
	got := *dep.Spec.Template.Spec.TerminationGracePeriodSeconds
	want := int64(120 + drainGraceBufferSeconds)
	if got != want {
		t.Fatalf("TerminationGracePeriodSeconds = %d, want %d", got, want)
	}
}

func TestDeployment_GracePeriodDefaultWhenUnset(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "drain-default-test"
	ar.Namespace = "ns"
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket}}
	ar.Spec.PromptPackRef.Name = "p"
	// DrainTimeout intentionally not set

	dep := &appsv1.Deployment{}
	r.buildDeploymentSpec(context.Background(), dep, ar, newTestPromptPack(), nil, "", nil)

	require.NotNil(t, dep.Spec.Template.Spec.TerminationGracePeriodSeconds)
	got := *dep.Spec.Template.Spec.TerminationGracePeriodSeconds
	want := int64(defaultDrainTimeoutSeconds + drainGraceBufferSeconds)
	if got != want {
		t.Fatalf("TerminationGracePeriodSeconds = %d, want %d", got, want)
	}
}

func TestDeployment_GracePeriodDefaultWhenSubSecond(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "drain-subsecond-test"
	ar.Namespace = "ns"
	ar.Spec.PromptPackRef.Name = "p"
	// Sub-second drain timeout: positive but truncates to 0 seconds — must fall to default.
	d := "500ms"
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket, DrainTimeout: &d}}

	dep := &appsv1.Deployment{}
	r.buildDeploymentSpec(context.Background(), dep, ar, newTestPromptPack(), nil, "", nil)

	require.NotNil(t, dep.Spec.Template.Spec.TerminationGracePeriodSeconds)
	got := *dep.Spec.Template.Spec.TerminationGracePeriodSeconds
	want := int64(defaultDrainTimeoutSeconds + drainGraceBufferSeconds)
	if got != want {
		t.Fatalf("TerminationGracePeriodSeconds = %d, want %d (sub-second drainTimeout must fall to default)", got, want)
	}
}

func TestDeployment_GracePeriodClampedAtMax(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "drain-clamp-test"
	ar.Namespace = "ns"
	ar.Spec.PromptPackRef.Name = "p"
	// A misconfigured huge drainTimeout must not stall teardown: the drain
	// window is clamped to maxDrainTimeoutSeconds before adding the buffer.
	d := "1h"
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket, DrainTimeout: &d}}

	dep := &appsv1.Deployment{}
	r.buildDeploymentSpec(context.Background(), dep, ar, newTestPromptPack(), nil, "", nil)

	require.NotNil(t, dep.Spec.Template.Spec.TerminationGracePeriodSeconds)
	got := *dep.Spec.Template.Spec.TerminationGracePeriodSeconds
	want := int64(maxDrainTimeoutSeconds + drainGraceBufferSeconds)
	if got != want {
		t.Fatalf("TerminationGracePeriodSeconds = %d, want %d (1h drainTimeout must clamp to max)", got, want)
	}
}

// TestBuildDeploymentSpec_PolicyBrokerInjectedAndRuntimeActivated is the P2.3a
// wiring guard: when PolicyBrokerImage is set, the pod must gain a
// policy-broker sidecar AND the runtime container's PolicyBrokerClient must be
// activated via POLICY_BROKER_URL — otherwise the sidecar runs but the runtime
// never calls it (silent no-op enforcement).
func TestBuildDeploymentSpec_PolicyBrokerInjectedAndRuntimeActivated(t *testing.T) {
	r := &AgentRuntimeReconciler{PolicyBrokerImage: "ghcr.io/altairalabs/omnia-policy-broker:test"}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "broker-agent"
	ar.Namespace = "ns"
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket}}
	ar.Spec.PromptPackRef.Name = "p"

	dep := &appsv1.Deployment{}
	r.buildDeploymentSpec(context.Background(), dep, ar, newTestPromptPack(), nil, "", nil)

	var brokerC, runtimeC *corev1.Container
	for i := range dep.Spec.Template.Spec.Containers {
		c := &dep.Spec.Template.Spec.Containers[i]
		switch c.Name {
		case PolicyBrokerContainerName:
			brokerC = c
		case RuntimeContainerName:
			runtimeC = c
		}
	}
	require.NotNil(t, brokerC, "policy-broker sidecar must be injected when PolicyBrokerImage is set")
	require.NotNil(t, runtimeC, "runtime container must be present")

	found := false
	for _, e := range runtimeC.Env {
		if e.Name == "POLICY_BROKER_URL" {
			found = true
			assert.Equal(t, fmt.Sprintf("http://localhost:%d", DefaultPolicyBrokerPort), e.Value)
		}
	}
	assert.True(t, found, "runtime container must have POLICY_BROKER_URL set to activate the client")

	failMode := ""
	for _, e := range runtimeC.Env {
		if e.Name == "POLICY_BROKER_FAIL_MODE" {
			failMode = e.Value
		}
	}
	assert.Equal(t, "closed", failMode, "runtime must explicitly set fail-closed mode")
}

// TestBuildDeploymentSpec_PolicyBrokerAbsentByDefault ensures the sidecar and
// runtime env var are both absent when PolicyBrokerImage is unset (the
// non-enterprise / no-broker path), so this feature is a strict no-op unless
// explicitly configured.
func TestBuildDeploymentSpec_PolicyBrokerAbsentByDefault(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{}
	ar.Name = "no-broker-agent"
	ar.Namespace = "ns"
	ar.Spec.Facades = []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket}}
	ar.Spec.PromptPackRef.Name = "p"

	dep := &appsv1.Deployment{}
	r.buildDeploymentSpec(context.Background(), dep, ar, newTestPromptPack(), nil, "", nil)

	for i := range dep.Spec.Template.Spec.Containers {
		c := &dep.Spec.Template.Spec.Containers[i]
		assert.NotEqual(t, PolicyBrokerContainerName, c.Name, "policy-broker sidecar must not be injected without PolicyBrokerImage")
		if c.Name == RuntimeContainerName {
			for _, e := range c.Env {
				assert.NotEqual(t, "POLICY_BROKER_URL", e.Name, "runtime must not receive POLICY_BROKER_URL without PolicyBrokerImage")
			}
		}
	}
}

// TestBuildDeploymentSpec_PolicyBrokerKeepsOwnSecurityContext is the
// retirement-era regression guard (formerly
// TestBuildDeploymentSpec_PolicyProxyKeepsOwnSecurityContext): the hardened
// facade/runtime SecurityContext loop in buildDeploymentSpec must skip the
// policy-broker sidecar, since buildPolicyBrokerContainer configures its own
// SecurityContext (or none) and must not be overwritten.
func TestBuildDeploymentSpec_PolicyBrokerKeepsOwnSecurityContext(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Facades: []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket}},
		},
	}
	pp := newTestPromptPack()
	r := &AgentRuntimeReconciler{
		Scheme:            scheme,
		Client:            fake.NewClientBuilder().WithScheme(scheme).Build(),
		PolicyBrokerImage: "ghcr.io/altairalabs/omnia-policy-broker:test",
	}

	dep := &appsv1.Deployment{}
	r.buildDeploymentSpec(context.Background(), dep, ar, pp, nil, "", nil)

	// Locate the policy-broker sidecar and check its SecurityContext is its
	// own, not hardenedContainerSecurityContext — the sidecar configures its
	// own SC.
	var policyBroker *corev1.Container
	for i := range dep.Spec.Template.Spec.Containers {
		c := &dep.Spec.Template.Spec.Containers[i]
		if c.Name == PolicyBrokerContainerName {
			policyBroker = c
			break
		}
	}
	require.NotNil(t, policyBroker, "policy-broker sidecar must be injected when PolicyBrokerImage is set")
	// Either the policy-broker has no hardened SC (it sets its own or runs
	// with a different profile) or it has one — but the buildDeploymentSpec
	// loop must not overwrite with hardenedContainerSecurityContext.
	hardened := hardenedContainerSecurityContext()
	if policyBroker.SecurityContext != nil {
		assert.NotEqual(t, hardened, policyBroker.SecurityContext, "policy-broker must not be overwritten with the facade/runtime hardened SC")
	}
}
