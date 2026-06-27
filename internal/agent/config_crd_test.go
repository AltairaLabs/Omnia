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

package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/k8s"
)

// testNamespace creates a Namespace with the workspace label set.
// Required because ResolveWorkspaceName now returns an error if the namespace
// cannot be read (instead of silently returning "").
func testNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"omnia.altairalabs.ai/workspace": name},
		},
	}
}

func newFakeAgentRuntime(name, namespace string, spec v1alpha1.AgentRuntimeSpec) *v1alpha1.AgentRuntime {
	return &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}
}

func TestLoadFromCRD_HappyPath(t *testing.T) {
	ar := newFakeAgentRuntime("my-agent", "prod", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{
			Name:    "my-pack",
			Version: ptr.To("v1.0.0"),
		},
		Facades: []v1alpha1.FacadeConfig{{
			Type: v1alpha1.FacadeTypeWebSocket,
			Port: ptr.To(int32(9090)),
		}},
		Context: &v1alpha1.ContextConfig{
			Type: v1alpha1.ContextStoreTypeRedis,
			TTL:  ptr.To("2h"),
		},
		ToolRegistryRef: &v1alpha1.ToolRegistryRef{
			Name:      "my-tools",
			Namespace: ptr.To("shared"),
		},
		Media: &v1alpha1.MediaConfig{
			BasePath: "/custom/media",
		},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()
	ctx := context.Background()

	cfg, err := LoadFromCRD(ctx, c, "my-agent", "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.AgentName != "my-agent" {
		t.Errorf("AgentName = %q, want %q", cfg.AgentName, "my-agent")
	}
	if cfg.Namespace != "prod" {
		t.Errorf("Namespace = %q, want %q", cfg.Namespace, "prod")
	}
	if cfg.PromptPackName != "my-pack" {
		t.Errorf("PromptPackName = %q, want %q", cfg.PromptPackName, "my-pack")
	}
	if cfg.PromptPackVersion != "v1.0.0" {
		t.Errorf("PromptPackVersion = %q, want %q", cfg.PromptPackVersion, "v1.0.0")
	}
	if cfg.FacadeType != FacadeTypeWebSocket {
		t.Errorf("FacadeType = %q, want %q", cfg.FacadeType, FacadeTypeWebSocket)
	}
	if cfg.FacadePort != 9090 {
		t.Errorf("FacadePort = %d, want %d", cfg.FacadePort, 9090)
	}
	if cfg.SessionTTL != 2*time.Hour {
		t.Errorf("SessionTTL = %v, want %v", cfg.SessionTTL, 2*time.Hour)
	}
	if cfg.ToolRegistryName != "my-tools" {
		t.Errorf("ToolRegistryName = %q, want %q", cfg.ToolRegistryName, "my-tools")
	}
	if cfg.ToolRegistryNamespace != "shared" {
		t.Errorf("ToolRegistryNamespace = %q, want %q", cfg.ToolRegistryNamespace, "shared")
	}
	if cfg.MediaStorageType != MediaStorageTypeLocal {
		t.Errorf("MediaStorageType = %q, want %q", cfg.MediaStorageType, MediaStorageTypeLocal)
	}
	if cfg.MediaStoragePath != "/custom/media" {
		t.Errorf("MediaStoragePath = %q, want %q", cfg.MediaStoragePath, "/custom/media")
	}
	if cfg.HandlerMode != HandlerModeRuntime {
		t.Errorf("HandlerMode = %q, want %q", cfg.HandlerMode, HandlerModeRuntime)
	}
	if cfg.HealthPort != DefaultHealthPort {
		t.Errorf("HealthPort = %d, want %d", cfg.HealthPort, DefaultHealthPort)
	}
}

func TestLoadFromCRD_NotFound(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).Build()
	ctx := context.Background()

	_, err := LoadFromCRD(ctx, c, "nonexistent", "default")
	if err == nil {
		t.Fatal("expected error for not-found CRD")
	}
}

func TestLoadFromCRD_PromptPackVersionNil(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{
			Name: "pack",
			// Version is nil
		},
		Facades: []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeWebSocket}},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PromptPackVersion != "" {
		t.Errorf("PromptPackVersion = %q, want empty string", cfg.PromptPackVersion)
	}
}

func TestLoadFromCRD_FacadePortDefault(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facades: []v1alpha1.FacadeConfig{{
			Type: v1alpha1.FacadeTypeWebSocket,
			// Port is nil — should default
		}},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.FacadePort != DefaultFacadePort {
		t.Errorf("FacadePort = %d, want default %d", cfg.FacadePort, DefaultFacadePort)
	}
}

func TestLoadFromCRD_InvalidSessionTTL(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facades:       []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeWebSocket}},
		Context: &v1alpha1.ContextConfig{
			Type: v1alpha1.ContextStoreTypeMemory,
			TTL:  ptr.To("not-a-duration"),
		},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	_, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err == nil {
		t.Fatal("expected error for invalid TTL")
	}
}

func TestLoadFromCRD_SessionNil(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facades:       []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeWebSocket}},
		// Session is nil
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SessionTTL != DefaultSessionTTL {
		t.Errorf("SessionTTL = %v, want default %v", cfg.SessionTTL, DefaultSessionTTL)
	}
}

func TestLoadFromCRD_SessionTTLNil(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facades:       []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeWebSocket}},
		Context: &v1alpha1.ContextConfig{
			Type: v1alpha1.ContextStoreTypeMemory,
			// TTL is nil — should default
		},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SessionTTL != DefaultSessionTTL {
		t.Errorf("SessionTTL = %v, want default %v", cfg.SessionTTL, DefaultSessionTTL)
	}
}

func TestLoadConfigFromCRD_DrainTimeout(t *testing.T) {
	d := "2m"
	ar := &v1alpha1.AgentRuntime{Spec: v1alpha1.AgentRuntimeSpec{
		Facades: []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeWebSocket, DrainTimeout: &d}},
	}}
	cfg := &Config{}
	if err := loadFacadesFromCRD(cfg, ar); err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DrainTimeout != 2*time.Minute {
		t.Fatalf("DrainTimeout = %v, want 2m", cfg.DrainTimeout)
	}
}

func TestLoadConfigFromCRD_DrainTimeout_Invalid(t *testing.T) {
	d := "bad-duration"
	ar := &v1alpha1.AgentRuntime{Spec: v1alpha1.AgentRuntimeSpec{
		Facades: []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeWebSocket, DrainTimeout: &d}},
	}}
	cfg := &Config{}
	err := loadFacadesFromCRD(cfg, ar)
	if err == nil {
		t.Fatal("expected error for invalid drain timeout")
	}
	if got := err.Error(); !strings.Contains(got, "invalid drain timeout") {
		t.Errorf("error = %q, want it to contain %q", got, "invalid drain timeout")
	}
}

func TestLoadFromCRD_ToolRegistryNoNamespace(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "mynamespace", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facades:       []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeWebSocket}},
		ToolRegistryRef: &v1alpha1.ToolRegistryRef{
			Name: "tools",
			// Namespace is nil — should default to agent namespace
		},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "agent", "mynamespace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ToolRegistryNamespace != "mynamespace" {
		t.Errorf("ToolRegistryNamespace = %q, want %q", cfg.ToolRegistryNamespace, "mynamespace")
	}
}

func TestLoadFromCRD_NoToolRegistry(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facades:       []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeWebSocket}},
		// ToolRegistryRef is nil
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ToolRegistryName != "" {
		t.Errorf("ToolRegistryName = %q, want empty", cfg.ToolRegistryName)
	}
}

func TestLoadFromCRD_MediaFromEnvFallback(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facades:       []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeWebSocket}},
		// Media is nil — should fall back to env
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MediaStorageType != MediaStorageTypeNone {
		t.Errorf("MediaStorageType = %q, want %q", cfg.MediaStorageType, MediaStorageTypeNone)
	}
	if cfg.MediaMaxFileSize != DefaultMediaMaxFileSize {
		t.Errorf("MediaMaxFileSize = %d, want %d", cfg.MediaMaxFileSize, DefaultMediaMaxFileSize)
	}
	if cfg.MediaDefaultTTL != DefaultMediaDefaultTTL {
		t.Errorf("MediaDefaultTTL = %v, want %v", cfg.MediaDefaultTTL, DefaultMediaDefaultTTL)
	}
}

func TestLoadFromCRD_MediaEmptyBasePath(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facades:       []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeWebSocket}},
		Media: &v1alpha1.MediaConfig{
			BasePath: "", // empty — should fall back to env
		},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty basepath means env fallback, so MediaStorageType defaults to "none"
	if cfg.MediaStorageType != MediaStorageTypeNone {
		t.Errorf("MediaStorageType = %q, want %q", cfg.MediaStorageType, MediaStorageTypeNone)
	}
}

func TestLoadTracingConfigFromEnv(t *testing.T) {
	t.Run("tracing enabled", func(t *testing.T) {
		t.Setenv(EnvTracingEnabled, "true")
		t.Setenv(EnvTracingEndpoint, "http://jaeger:4317")
		t.Setenv(EnvTracingInsecure, "true")
		t.Setenv(EnvTracingSampleRate, "0.5")

		cfg := &Config{}
		err := loadTracingConfigFromEnv(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !cfg.TracingEnabled {
			t.Error("TracingEnabled = false, want true")
		}
		if cfg.TracingEndpoint != "http://jaeger:4317" {
			t.Errorf("TracingEndpoint = %q, want %q", cfg.TracingEndpoint, "http://jaeger:4317")
		}
		if !cfg.TracingInsecure {
			t.Error("TracingInsecure = false, want true")
		}
		if cfg.TracingSampleRate != 0.5 {
			t.Errorf("TracingSampleRate = %f, want 0.5", cfg.TracingSampleRate)
		}
	})

	t.Run("tracing disabled defaults", func(t *testing.T) {
		// Clear env vars — t.Setenv automatically restores
		cfg := &Config{}
		err := loadTracingConfigFromEnv(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.TracingEnabled {
			t.Error("TracingEnabled = true, want false")
		}
		if cfg.TracingSampleRate != 1.0 {
			t.Errorf("TracingSampleRate = %f, want 1.0", cfg.TracingSampleRate)
		}
	})

	t.Run("invalid sample rate", func(t *testing.T) {
		t.Setenv(EnvTracingSampleRate, "not-a-float")

		cfg := &Config{}
		err := loadTracingConfigFromEnv(cfg)
		if err == nil {
			t.Fatal("expected error for invalid sample rate")
		}
	})
}

func TestLoadFromCRD_InvalidHealthPort(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facades:       []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeWebSocket}},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	t.Setenv(EnvHealthPort, "not-a-number")

	_, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err == nil {
		t.Fatal("expected error for invalid health port")
	}
}

func TestLoadFromCRD_TracingError(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facades:       []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeWebSocket}},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	t.Setenv(EnvTracingSampleRate, "invalid")

	_, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err == nil {
		t.Fatal("expected error for invalid tracing sample rate")
	}
}

func TestLoadFromCRD_HandlerModeFromEnv(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facades:       []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeWebSocket}},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	t.Setenv(EnvHandlerMode, "echo")

	cfg, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HandlerMode != HandlerModeEcho {
		t.Errorf("HandlerMode = %q, want %q", cfg.HandlerMode, HandlerModeEcho)
	}
}

func TestLoadConfig_MissingEnvVars(t *testing.T) {
	// When OMNIA_AGENT_NAME and OMNIA_NAMESPACE are not set, LoadConfig returns an error
	_, err := LoadConfig(context.Background())
	if err == nil {
		t.Fatal("expected error when env vars are missing")
	}
	if got := err.Error(); got != "OMNIA_AGENT_NAME and OMNIA_NAMESPACE are required (set via Downward API)" {
		t.Errorf("error = %q, want required env vars message", got)
	}
}

func TestLoadConfig_MissingNamespaceOnly(t *testing.T) {
	t.Setenv(EnvAgentName, "my-agent")
	// OMNIA_NAMESPACE not set

	_, err := LoadConfig(context.Background())
	if err == nil {
		t.Fatal("expected error when namespace is missing")
	}
}

func TestLoadConfig_MissingAgentNameOnly(t *testing.T) {
	t.Setenv(EnvNamespace, "my-ns")
	// OMNIA_AGENT_NAME not set

	_, err := LoadConfig(context.Background())
	if err == nil {
		t.Fatal("expected error when agent name is missing")
	}
}

func TestLoadConfig_FallbackToEnv(t *testing.T) {
	// When K8s is unavailable, LoadConfig should fall back to env-based config.
	// Force no-cluster by unsetting kubeconfig so NewClient fails.
	t.Setenv("KUBECONFIG", "/nonexistent")
	t.Setenv(EnvAgentName, "test-agent")
	t.Setenv(EnvNamespace, "test-ns")
	t.Setenv(EnvHandlerMode, "demo")
	t.Setenv(EnvFacadePort, "8080")
	t.Setenv(EnvHealthPort, "8081")
	cfg, err := LoadConfig(context.Background())
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if cfg.AgentName != "test-agent" {
		t.Errorf("AgentName = %q, want %q", cfg.AgentName, "test-agent")
	}
	if cfg.Namespace != "test-ns" {
		t.Errorf("Namespace = %q, want %q", cfg.Namespace, "test-ns")
	}
	if cfg.HandlerMode != HandlerModeDemo {
		t.Errorf("HandlerMode = %q, want %q", cfg.HandlerMode, HandlerModeDemo)
	}
	if cfg.FacadePort != 8080 {
		t.Errorf("FacadePort = %d, want 8080", cfg.FacadePort)
	}
}

func TestLoadFromEnvFallback_Defaults(t *testing.T) {
	cfg, err := loadFromEnvFallback("agent-1", "ns-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AgentName != "agent-1" {
		t.Errorf("AgentName = %q, want %q", cfg.AgentName, "agent-1")
	}
	if cfg.Namespace != "ns-1" {
		t.Errorf("Namespace = %q, want %q", cfg.Namespace, "ns-1")
	}
	if cfg.FacadePort != DefaultFacadePort {
		t.Errorf("FacadePort = %d, want %d", cfg.FacadePort, DefaultFacadePort)
	}
	if cfg.HealthPort != DefaultHealthPort {
		t.Errorf("HealthPort = %d, want %d", cfg.HealthPort, DefaultHealthPort)
	}
	if cfg.MediaStorageType != MediaStorageTypeNone {
		t.Errorf("MediaStorageType = %q, want %q", cfg.MediaStorageType, MediaStorageTypeNone)
	}
}

func TestLoadFromEnvFallback_InvalidFacadePort(t *testing.T) {
	t.Setenv(EnvFacadePort, "not-a-number")
	_, err := loadFromEnvFallback("agent", "ns")
	if err == nil {
		t.Fatal("expected error for invalid facade port")
	}
}

func TestLoadFromEnvFallback_InvalidHealthPort(t *testing.T) {
	t.Setenv(EnvHealthPort, "not-a-number")
	_, err := loadFromEnvFallback("agent", "ns")
	if err == nil {
		t.Fatal("expected error for invalid health port")
	}
}

func TestLoadA2AConfigFromCRD_Defaults(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facades:       []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeA2A}},
	})
	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.A2ATaskTTL != DefaultA2ATaskTTL {
		t.Errorf("A2ATaskTTL = %v, want %v", cfg.A2ATaskTTL, DefaultA2ATaskTTL)
	}
	if cfg.A2AConversationTTL != DefaultA2AConversationTTL {
		t.Errorf("A2AConversationTTL = %v, want %v", cfg.A2AConversationTTL, DefaultA2AConversationTTL)
	}
}

func TestLoadA2AConfigFromCRD_CustomTTLs(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facades: []v1alpha1.FacadeConfig{{
			Type: v1alpha1.FacadeTypeA2A,
			A2A: &v1alpha1.A2AConfig{
				TaskTTL:         ptr.To("2h"),
				ConversationTTL: ptr.To("45m"),
			},
		}},
	})
	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.A2ATaskTTL != 2*time.Hour {
		t.Errorf("A2ATaskTTL = %v, want %v", cfg.A2ATaskTTL, 2*time.Hour)
	}
	if cfg.A2AConversationTTL != 45*time.Minute {
		t.Errorf("A2AConversationTTL = %v, want %v", cfg.A2AConversationTTL, 45*time.Minute)
	}
}

func TestLoadA2AConfigFromCRD_InvalidTaskTTL(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facades: []v1alpha1.FacadeConfig{{
			Type: v1alpha1.FacadeTypeA2A,
			A2A: &v1alpha1.A2AConfig{
				TaskTTL: ptr.To("not-a-duration"),
			},
		}},
	})
	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	_, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err == nil {
		t.Fatal("expected error for invalid task TTL")
	}
}

func TestLoadA2AConfigFromCRD_InvalidConversationTTL(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facades: []v1alpha1.FacadeConfig{{
			Type: v1alpha1.FacadeTypeA2A,
			A2A: &v1alpha1.A2AConfig{
				ConversationTTL: ptr.To("not-a-duration"),
			},
		}},
	})
	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	_, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err == nil {
		t.Fatal("expected error for invalid conversation TTL")
	}
}

func TestLoadA2AConfigFromEnv_CustomTTLs(t *testing.T) {
	t.Setenv(EnvA2ATaskTTL, "3h")
	t.Setenv(EnvA2AConversationTTL, "15m")

	cfg := &Config{}
	err := loadA2AConfigFromEnv(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.A2ATaskTTL != 3*time.Hour {
		t.Errorf("A2ATaskTTL = %v, want %v", cfg.A2ATaskTTL, 3*time.Hour)
	}
	if cfg.A2AConversationTTL != 15*time.Minute {
		t.Errorf("A2AConversationTTL = %v, want %v", cfg.A2AConversationTTL, 15*time.Minute)
	}
}

func TestLoadA2AConfigFromEnv_Defaults(t *testing.T) {
	cfg := &Config{}
	err := loadA2AConfigFromEnv(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.A2ATaskTTL != DefaultA2ATaskTTL {
		t.Errorf("A2ATaskTTL = %v, want %v", cfg.A2ATaskTTL, DefaultA2ATaskTTL)
	}
	if cfg.A2AConversationTTL != DefaultA2AConversationTTL {
		t.Errorf("A2AConversationTTL = %v, want %v", cfg.A2AConversationTTL, DefaultA2AConversationTTL)
	}
}

func TestLoadA2AConfigFromEnv_InvalidTaskTTL(t *testing.T) {
	t.Setenv(EnvA2ATaskTTL, "bad")
	cfg := &Config{}
	err := loadA2AConfigFromEnv(cfg)
	if err == nil {
		t.Fatal("expected error for invalid A2A task TTL")
	}
}

func TestLoadA2AConfigFromEnv_InvalidConversationTTL(t *testing.T) {
	t.Setenv(EnvA2AConversationTTL, "bad")
	cfg := &Config{}
	err := loadA2AConfigFromEnv(cfg)
	if err == nil {
		t.Fatal("expected error for invalid A2A conversation TTL")
	}
}

func TestConfigValidate_A2AFacadeType(t *testing.T) {
	cfg := &Config{
		AgentName:        "a",
		Namespace:        "n",
		PromptPackName:   "p",
		FacadeType:       FacadeTypeA2A,
		HandlerMode:      HandlerModeRuntime,
		MediaStorageType: MediaStorageTypeNone,
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected A2A facade type to be valid, got: %v", err)
	}
}

func TestLoadMCPConfigFromEnv_Defaults(t *testing.T) {
	cfg := &Config{}
	if err := loadMCPConfigFromEnv(cfg); err != nil {
		t.Fatalf("loadMCPConfigFromEnv: %v", err)
	}
	if cfg.MCPEnabled {
		t.Errorf("MCPEnabled default: got true want false")
	}
	if cfg.MCPPort != DefaultMCPPort {
		t.Errorf("MCPPort default: got %d want %d", cfg.MCPPort, DefaultMCPPort)
	}
}

func TestLoadMCPConfigFromEnv_Enabled(t *testing.T) {
	t.Setenv(EnvMCPEnabled, "true")
	t.Setenv(EnvMCPPort, "9000")

	cfg := &Config{}
	if err := loadMCPConfigFromEnv(cfg); err != nil {
		t.Fatalf("loadMCPConfigFromEnv: %v", err)
	}
	if !cfg.MCPEnabled {
		t.Error("MCPEnabled: got false want true")
	}
	if cfg.MCPPort != 9000 {
		t.Errorf("MCPPort: got %d want 9000", cfg.MCPPort)
	}
}

func TestLoadMCPConfigFromEnv_InvalidPortOutOfRange(t *testing.T) {
	t.Setenv(EnvMCPPort, "70000")

	cfg := &Config{}
	if err := loadMCPConfigFromEnv(cfg); err == nil {
		t.Fatal("expected error for out-of-range port")
	}
}

func TestLoadMCPConfigFromEnv_InvalidPortNotANumber(t *testing.T) {
	t.Setenv(EnvMCPPort, "not-a-port")

	cfg := &Config{}
	if err := loadMCPConfigFromEnv(cfg); err == nil {
		t.Fatal("expected error for non-numeric port")
	}
}

func TestLoadMCPConfigFromEnv_InvalidPortZero(t *testing.T) {
	t.Setenv(EnvMCPPort, "0")

	cfg := &Config{}
	if err := loadMCPConfigFromEnv(cfg); err == nil {
		t.Fatal("expected error for zero port")
	}
}

func TestLoadMCPConfigFromCRD_FromFacade(t *testing.T) {
	port := int32(9500)
	ar := newFakeAgentRuntime("fn", "default", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "p"},
		Mode:          "function",
		Facades: []v1alpha1.FacadeConfig{
			{Type: v1alpha1.FacadeTypeREST},
			{Type: v1alpha1.FacadeTypeMCP, MCP: &v1alpha1.MCPConfig{Enabled: true, Port: &port}},
		},
	})
	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "fn", "default")
	if err != nil {
		t.Fatalf("LoadFromCRD: %v", err)
	}
	if !cfg.MCPEnabled {
		t.Error("MCPEnabled: got false want true")
	}
	if cfg.MCPPort != 9500 {
		t.Errorf("MCPPort: got %d want 9500", cfg.MCPPort)
	}
}

func TestLoadMCPConfigFromCRD_DefaultPortWhenUnset(t *testing.T) {
	ar := newFakeAgentRuntime("fn", "default", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "p"},
		Mode:          "function",
		Facades: []v1alpha1.FacadeConfig{
			{Type: v1alpha1.FacadeTypeREST},
			{Type: v1alpha1.FacadeTypeMCP, MCP: &v1alpha1.MCPConfig{Enabled: true}},
		},
	})
	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "fn", "default")
	if err != nil {
		t.Fatalf("LoadFromCRD: %v", err)
	}
	if cfg.MCPPort != DefaultMCPPort {
		t.Errorf("MCPPort default: got %d want %d", cfg.MCPPort, DefaultMCPPort)
	}
}

func TestLoadFromEnvFallback_InvalidMCPPort(t *testing.T) {
	t.Setenv(EnvMCPPort, "99999")
	_, err := loadFromEnvFallback("agent", "ns")
	if err == nil {
		t.Fatal("expected error for invalid MCP port in env fallback")
	}
}

func TestLoadMCPConfigFromCRD_NilMCP(t *testing.T) {
	ar := newFakeAgentRuntime("fn", "default", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "p"},
		Facades: []v1alpha1.FacadeConfig{{
			Type: v1alpha1.FacadeTypeWebSocket,
			// no mcp facade
		}},
	})
	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "fn", "default")
	if err != nil {
		t.Fatalf("LoadFromCRD: %v", err)
	}
	if cfg.MCPEnabled {
		t.Error("MCPEnabled: got true want false")
	}
	if cfg.MCPPort != DefaultMCPPort {
		t.Errorf("MCPPort default: got %d want %d", cfg.MCPPort, DefaultMCPPort)
	}
}

func TestApplyManagementPlanePorts(t *testing.T) {
	// a2aEnabled pre-seeds cfg.A2AEnabled so applyManagementPlanePorts maps a
	// secondary (dual-protocol) a2a facade to the a2a twin port rather than the
	// facade twin port. With a2aEnabled false a lone a2a facade is the primary
	// and maps to the facade twin port.
	tests := []struct {
		name       string
		facades    []v1alpha1.FacadeConfig
		a2aEnabled bool
		wantFacade int
		wantA2A    int
		wantMCP    int
	}{
		{
			name:       "websocket only, mgmt default true",
			facades:    []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeWebSocket}},
			wantFacade: DefaultInternalFacadePort,
		},
		{
			name:       "websocket with explicit mgmt true",
			facades:    []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeWebSocket, ManagementPlane: ptr.To(true)}},
			wantFacade: DefaultInternalFacadePort,
		},
		{
			name: "websocket + secondary a2a adds a2a internal port",
			facades: []v1alpha1.FacadeConfig{
				{Type: v1alpha1.FacadeTypeWebSocket},
				{Type: v1alpha1.FacadeTypeA2A},
			},
			a2aEnabled: true,
			wantFacade: DefaultInternalFacadePort,
			wantA2A:    DefaultInternalA2APort,
		},
		{
			name: "rest + mcp adds mcp internal port",
			facades: []v1alpha1.FacadeConfig{
				{Type: v1alpha1.FacadeTypeREST},
				{Type: v1alpha1.FacadeTypeMCP},
			},
			wantFacade: DefaultInternalFacadePort,
			wantMCP:    DefaultInternalMCPPort,
		},
		{
			name:       "standalone a2a primary maps to facade twin port",
			facades:    []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeA2A}},
			wantFacade: DefaultInternalFacadePort,
		},
		{
			name: "managementPlane false on websocket leaves facade port zero",
			facades: []v1alpha1.FacadeConfig{
				{Type: v1alpha1.FacadeTypeWebSocket, ManagementPlane: ptr.To(false)},
				{Type: v1alpha1.FacadeTypeA2A},
			},
			a2aEnabled: true,
			wantFacade: 0,
			wantA2A:    DefaultInternalA2APort,
		},
		{
			name: "managementPlane false on every facade disables all",
			facades: []v1alpha1.FacadeConfig{
				{Type: v1alpha1.FacadeTypeWebSocket, ManagementPlane: ptr.To(false)},
				{Type: v1alpha1.FacadeTypeA2A, ManagementPlane: ptr.To(false)},
				{Type: v1alpha1.FacadeTypeMCP, ManagementPlane: ptr.To(false)},
			},
			a2aEnabled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{A2AEnabled: tt.a2aEnabled}
			applyManagementPlanePorts(cfg, tt.facades)
			if cfg.InternalFacadePort != tt.wantFacade {
				t.Errorf("InternalFacadePort = %d, want %d", cfg.InternalFacadePort, tt.wantFacade)
			}
			if cfg.InternalA2APort != tt.wantA2A {
				t.Errorf("InternalA2APort = %d, want %d", cfg.InternalA2APort, tt.wantA2A)
			}
			if cfg.InternalMCPPort != tt.wantMCP {
				t.Errorf("InternalMCPPort = %d, want %d", cfg.InternalMCPPort, tt.wantMCP)
			}
		})
	}
}

// TestLoadFacadesFromCRD_DerivedConfig exercises the multi-facade → flat Config
// derivation contract (#1576): which facade is primary, when A2A/MCP become
// secondary listeners, and the resulting ports.
func TestLoadFacadesFromCRD_DerivedConfig(t *testing.T) {
	tests := []struct {
		name         string
		facades      []v1alpha1.FacadeConfig
		wantFacade   FacadeType
		wantA2A      bool
		wantA2APort  int
		wantMCP      bool
		wantMCPPort  int
		wantFacadeNo int
	}{
		{
			name:         "websocket only",
			facades:      []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeWebSocket}},
			wantFacade:   FacadeTypeWebSocket,
			wantA2A:      false,
			wantFacadeNo: DefaultFacadePort,
		},
		{
			name: "websocket + a2a secondary",
			facades: []v1alpha1.FacadeConfig{
				{Type: v1alpha1.FacadeTypeWebSocket},
				{Type: v1alpha1.FacadeTypeA2A},
			},
			wantFacade:   FacadeTypeWebSocket,
			wantA2A:      true,
			wantA2APort:  DefaultA2APort,
			wantFacadeNo: DefaultFacadePort,
		},
		{
			name:         "standalone a2a primary",
			facades:      []v1alpha1.FacadeConfig{{Type: v1alpha1.FacadeTypeA2A}},
			wantFacade:   FacadeTypeA2A,
			wantA2A:      false,
			wantFacadeNo: DefaultFacadePort,
		},
		{
			name: "function mode rest + mcp",
			facades: []v1alpha1.FacadeConfig{
				{Type: v1alpha1.FacadeTypeREST},
				{Type: v1alpha1.FacadeTypeMCP},
			},
			wantFacade:   FacadeTypeREST,
			wantMCP:      true,
			wantMCPPort:  DefaultMCPPort,
			wantFacadeNo: DefaultFacadePort,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := &v1alpha1.AgentRuntime{Spec: v1alpha1.AgentRuntimeSpec{Facades: tt.facades}}
			cfg := &Config{}
			if err := loadFacadesFromCRD(cfg, ar); err != nil {
				t.Fatalf("loadFacadesFromCRD: %v", err)
			}
			assertFlatConfig(t, cfg, tt.wantFacade, tt.wantFacadeNo,
				tt.wantA2A, tt.wantA2APort, tt.wantMCP, tt.wantMCPPort)

			// Verify the per-facade management-plane port derivation against the
			// same facade slice (the production LoadFromCRD calls these in
			// sequence).
			applyManagementPlanePorts(cfg, tt.facades)
			assertMgmtPorts(t, cfg, tt.wantA2A, tt.wantMCP)
		})
	}
}

func assertFlatConfig(
	t *testing.T, cfg *Config, wantFacade FacadeType, wantPort int,
	wantA2A bool, wantA2APort int, wantMCP bool, wantMCPPort int,
) {
	t.Helper()
	if cfg.FacadeType != wantFacade {
		t.Errorf("FacadeType = %q, want %q", cfg.FacadeType, wantFacade)
	}
	if cfg.FacadePort != wantPort {
		t.Errorf("FacadePort = %d, want %d", cfg.FacadePort, wantPort)
	}
	if cfg.A2AEnabled != wantA2A {
		t.Errorf("A2AEnabled = %v, want %v", cfg.A2AEnabled, wantA2A)
	}
	if wantA2A && cfg.A2APort != wantA2APort {
		t.Errorf("A2APort = %d, want %d", cfg.A2APort, wantA2APort)
	}
	if cfg.MCPEnabled != wantMCP {
		t.Errorf("MCPEnabled = %v, want %v", cfg.MCPEnabled, wantMCP)
	}
	if wantMCP && cfg.MCPPort != wantMCPPort {
		t.Errorf("MCPPort = %d, want %d", cfg.MCPPort, wantMCPPort)
	}
}

func assertMgmtPorts(t *testing.T, cfg *Config, wantA2A, wantMCP bool) {
	t.Helper()
	if cfg.InternalFacadePort != DefaultInternalFacadePort {
		t.Errorf("InternalFacadePort = %d, want %d", cfg.InternalFacadePort, DefaultInternalFacadePort)
	}
	if wantA2A && cfg.InternalA2APort != DefaultInternalA2APort {
		t.Errorf("InternalA2APort = %d, want %d", cfg.InternalA2APort, DefaultInternalA2APort)
	}
	if wantMCP && cfg.InternalMCPPort != DefaultInternalMCPPort {
		t.Errorf("InternalMCPPort = %d, want %d", cfg.InternalMCPPort, DefaultInternalMCPPort)
	}
}

func TestLoadInternalPortsFromEnv(t *testing.T) {
	t.Setenv(EnvInternalFacadePort, "18080")
	t.Setenv(EnvInternalA2APort, "19999")
	cfg := &Config{}
	if err := loadInternalPortsFromEnv(cfg); err != nil {
		t.Fatalf("loadInternalPortsFromEnv: %v", err)
	}
	if cfg.InternalFacadePort != 18080 {
		t.Errorf("InternalFacadePort = %d, want 18080", cfg.InternalFacadePort)
	}
	if cfg.InternalA2APort != 19999 {
		t.Errorf("InternalA2APort = %d, want 19999", cfg.InternalA2APort)
	}
	if cfg.InternalMCPPort != 0 {
		t.Errorf("InternalMCPPort = %d, want 0 (unset)", cfg.InternalMCPPort)
	}
}

func TestLoadInternalPortsFromEnv_Invalid(t *testing.T) {
	t.Setenv(EnvInternalFacadePort, "not-a-number")
	if err := loadInternalPortsFromEnv(&Config{}); err == nil {
		t.Error("expected error for malformed internal port, got nil")
	}
}
