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
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/k8s"
)

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
		Facade: v1alpha1.FacadeConfig{
			Type: v1alpha1.FacadeTypeWebSocket,
			Port: ptr.To(int32(9090)),
		},
		Session: &v1alpha1.SessionConfig{
			Type: v1alpha1.SessionStoreTypeRedis,
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

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar).Build()
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
	if cfg.SessionType != SessionTypeRedis {
		t.Errorf("SessionType = %q, want %q", cfg.SessionType, SessionTypeRedis)
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
		Facade: v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar).Build()

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
		Facade: v1alpha1.FacadeConfig{
			Type: v1alpha1.FacadeTypeWebSocket,
			// Port is nil — should default
		},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar).Build()

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
		Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
		Session: &v1alpha1.SessionConfig{
			Type: v1alpha1.SessionStoreTypeMemory,
			TTL:  ptr.To("not-a-duration"),
		},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar).Build()

	_, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err == nil {
		t.Fatal("expected error for invalid TTL")
	}
}

func TestLoadFromCRD_SessionNil(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
		// Session is nil
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SessionType != SessionTypeMemory {
		t.Errorf("SessionType = %q, want %q", cfg.SessionType, SessionTypeMemory)
	}
	if cfg.SessionTTL != DefaultSessionTTL {
		t.Errorf("SessionTTL = %v, want default %v", cfg.SessionTTL, DefaultSessionTTL)
	}
}

func TestLoadFromCRD_SessionTTLNil(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
		Session: &v1alpha1.SessionConfig{
			Type: v1alpha1.SessionStoreTypeMemory,
			// TTL is nil — should default
		},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SessionTTL != DefaultSessionTTL {
		t.Errorf("SessionTTL = %v, want default %v", cfg.SessionTTL, DefaultSessionTTL)
	}
}

func TestLoadFromCRD_ToolRegistryNoNamespace(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "mynamespace", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
		ToolRegistryRef: &v1alpha1.ToolRegistryRef{
			Name: "tools",
			// Namespace is nil — should default to agent namespace
		},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar).Build()

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
		Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
		// ToolRegistryRef is nil
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar).Build()

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
		Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
		// Media is nil — should fall back to env
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar).Build()

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
		Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
		Media: &v1alpha1.MediaConfig{
			BasePath: "", // empty — should fall back to env
		},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar).Build()

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
		Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar).Build()

	t.Setenv(EnvHealthPort, "not-a-number")

	_, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err == nil {
		t.Fatal("expected error for invalid health port")
	}
}

func TestLoadFromCRD_TracingError(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar).Build()

	t.Setenv(EnvTracingSampleRate, "invalid")

	_, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err == nil {
		t.Fatal("expected error for invalid tracing sample rate")
	}
}

func TestLoadFromCRD_SessionStoreURL(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
		Session: &v1alpha1.SessionConfig{
			Type: v1alpha1.SessionStoreTypeRedis,
			TTL:  ptr.To("1h"),
		},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar).Build()

	t.Setenv(EnvSessionStoreURL, "redis://localhost:6379")

	cfg, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SessionStoreURL != "redis://localhost:6379" {
		t.Errorf("SessionStoreURL = %q, want %q", cfg.SessionStoreURL, "redis://localhost:6379")
	}
}

func TestLoadFromCRD_HandlerModeFromEnv(t *testing.T) {
	ar := newFakeAgentRuntime("agent", "ns", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "pack"},
		Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).WithRuntimeObjects(ar).Build()

	t.Setenv(EnvHandlerMode, "echo")

	cfg, err := LoadFromCRD(context.Background(), c, "agent", "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HandlerMode != HandlerModeEcho {
		t.Errorf("HandlerMode = %q, want %q", cfg.HandlerMode, HandlerModeEcho)
	}
}

func TestLoadConfig_FallbackToEnv(t *testing.T) {
	// When OMNIA_AGENT_NAME and OMNIA_NAMESPACE are not set, LoadConfig falls back to LoadFromEnv
	t.Setenv(EnvAgentName, "env-agent")
	t.Setenv(EnvNamespace, "env-ns")
	t.Setenv(EnvPromptPackName, "env-pack")

	cfg, err := LoadConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Since k8s.NewClient() will fail outside a cluster, it falls back to LoadFromEnv
	if cfg.AgentName != "env-agent" {
		t.Errorf("AgentName = %q, want %q", cfg.AgentName, "env-agent")
	}
}

func TestLoadConfig_EmptyEnv(t *testing.T) {
	// When both env vars are empty, falls back to LoadFromEnv immediately
	cfg, err := LoadConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// LoadFromEnv succeeds with all defaults (empty name/namespace)
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}
