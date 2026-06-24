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
	"fmt"
	"os"
	"strconv"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/k8s"
)

// LoadFromCRD loads configuration by reading the AgentRuntime CRD directly.
// It uses OMNIA_AGENT_NAME and OMNIA_NAMESPACE (injected via Downward API) to identify the CRD.
// Env-var-only settings (SESSION_API_URL, tracing, handler mode) are still loaded from env.
func LoadFromCRD(ctx context.Context, c client.Client, name, namespace string) (*Config, error) {
	ar, err := k8s.GetAgentRuntime(ctx, c, name, namespace)
	if err != nil {
		return nil, fmt.Errorf("load AgentRuntime CRD: %w", err)
	}

	workspaceName, err := k8s.ResolveWorkspaceName(ctx, c, ar.Labels, namespace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace name: %w", err)
	}

	// Normalise legacy spec.a2a → spec.facade.a2a so downstream loaders
	// read from a single location. The operator's reconciler does the
	// same projection (Task 8); this side covers pods that boot before
	// the reconciler has rewritten the CR.
	v1alpha1.ProjectLegacyFacadeA2A(ar)

	cfg := &Config{
		AgentName:     name,
		Namespace:     namespace,
		WorkspaceName: workspaceName,
	}

	// PromptPack info from CRD
	cfg.PromptPackName = ar.Spec.PromptPackRef.Name
	if ar.Spec.PromptPackRef.Version != nil {
		cfg.PromptPackVersion = *ar.Spec.PromptPackRef.Version
	}
	cfg.PromptPackPath = getEnvOrDefault(EnvPromptPackMountPath, DefaultPromptPackMountPath)

	// Facade config from CRD
	if err := loadFacadeConfigFromCRD(cfg, ar); err != nil {
		return nil, err
	}

	// Mode + Function-specific config (Functions Phase 1, #1102 / #1103).
	// EffectiveMode() defaults empty → "agent" for back-compat with
	// pre-mode AgentRuntimes that predate the rollout.
	cfg.Mode = string(ar.EffectiveMode())
	if ar.IsFunctionMode() {
		if ar.Spec.InputSchema != nil {
			cfg.FunctionInputSchemaJSON = ar.Spec.InputSchema.Raw
		}
		if ar.Spec.OutputSchema != nil {
			cfg.FunctionOutputSchemaJSON = ar.Spec.OutputSchema.Raw
		}
	}

	// Handler mode from env (operator decides this, not CRD)
	cfg.HandlerMode = HandlerMode(getEnvOrDefault(EnvHandlerMode, string(HandlerModeRuntime)))
	cfg.RuntimeAddress = getEnvOrDefault(EnvRuntimeAddress, DefaultRuntimeAddress)

	// Health port from env
	healthPort, err := getEnvAsInt(EnvHealthPort, DefaultHealthPort)
	if err != nil {
		return nil, fmt.Errorf(errFmtInvalidEnv, EnvHealthPort, err)
	}
	cfg.HealthPort = healthPort

	if err := loadContextConfigFromCRD(cfg, ar, namespace); err != nil {
		return nil, err
	}
	loadMediaConfigFromCRD(cfg, ar)

	if err := loadTracingConfigFromEnv(cfg); err != nil {
		return nil, err
	}

	if err := loadA2AConfigFromCRD(cfg, ar); err != nil {
		return nil, err
	}

	loadMCPConfigFromCRD(cfg, ar)

	return cfg, nil
}

// loadA2AConfigFromCRD populates A2A-related config fields from the AgentRuntime CRD.
//
// Reads from spec.facade.a2a. Callers must call ProjectLegacyFacadeA2A
// before invoking this so legacy spec.a2a values are normalised into
// the new location.
func loadA2AConfigFromCRD(cfg *Config, ar *v1alpha1.AgentRuntime) error {
	a2a := ar.Spec.Facade.A2A
	if a2a == nil {
		cfg.A2ATaskTTL = DefaultA2ATaskTTL
		cfg.A2AConversationTTL = DefaultA2AConversationTTL
		return nil
	}

	if err := loadA2ATTLsFromCRD(cfg, a2a); err != nil {
		return err
	}

	cfg.A2AAuthToken = os.Getenv(EnvA2AAuthToken)

	// Dual-protocol: A2A as additional endpoint alongside websocket/grpc.
	cfg.A2AEnabled = a2a.Enabled
	if a2a.Port != nil {
		cfg.A2APort = int(*a2a.Port)
	} else {
		cfg.A2APort = DefaultA2APort
	}

	loadA2ATaskStoreFromCRD(cfg, a2a)

	// Resolved A2A clients are injected as JSON by the operator.
	cfg.A2AClientsJSON = os.Getenv(EnvA2AClients)

	return nil
}

// loadA2ATTLsFromCRD parses A2A TTL durations from the CRD.
func loadA2ATTLsFromCRD(cfg *Config, a2a *v1alpha1.A2AConfig) error {
	if a2a.TaskTTL != nil {
		ttl, err := time.ParseDuration(*a2a.TaskTTL)
		if err != nil {
			return fmt.Errorf("invalid A2A task TTL %q: %w", *a2a.TaskTTL, err)
		}
		cfg.A2ATaskTTL = ttl
	} else {
		cfg.A2ATaskTTL = DefaultA2ATaskTTL
	}

	if a2a.ConversationTTL != nil {
		ttl, err := time.ParseDuration(*a2a.ConversationTTL)
		if err != nil {
			return fmt.Errorf("invalid A2A conversation TTL %q: %w", *a2a.ConversationTTL, err)
		}
		cfg.A2AConversationTTL = ttl
	} else {
		cfg.A2AConversationTTL = DefaultA2AConversationTTL
	}

	return nil
}

// loadA2ATaskStoreFromCRD populates task store config from the CRD or env fallback.
func loadA2ATaskStoreFromCRD(cfg *Config, a2a *v1alpha1.A2AConfig) {
	if a2a.TaskStore != nil {
		cfg.A2ATaskStoreType = string(a2a.TaskStore.Type)
		if a2a.TaskStore.RedisURL != "" {
			cfg.A2ARedisURL = a2a.TaskStore.RedisURL
		}
		// RedisSecretRef is resolved by the operator into OMNIA_A2A_REDIS_URL env var.
		if envURL := os.Getenv(EnvA2ARedisURL); envURL != "" {
			cfg.A2ARedisURL = envURL
		}
	} else {
		cfg.A2ATaskStoreType = getEnvOrDefault(EnvA2ATaskStoreType, "memory")
		cfg.A2ARedisURL = os.Getenv(EnvA2ARedisURL)
	}
}

// loadFacadeConfigFromCRD populates facade-related config fields from the AgentRuntime CRD.
func loadFacadeConfigFromCRD(cfg *Config, ar *v1alpha1.AgentRuntime) error {
	cfg.FacadeType = FacadeType(ar.Spec.Facade.Type)
	if ar.Spec.Facade.Port != nil {
		cfg.FacadePort = int(*ar.Spec.Facade.Port)
	} else {
		cfg.FacadePort = DefaultFacadePort
	}
	if ar.Spec.Facade.ClientToolTimeout != nil {
		cfg.ClientToolTimeout = ar.Spec.Facade.ClientToolTimeout.Duration
	}
	if ar.Spec.Facade.DrainTimeout != nil {
		d, err := time.ParseDuration(*ar.Spec.Facade.DrainTimeout)
		if err != nil {
			return fmt.Errorf("invalid drain timeout %q: %w", *ar.Spec.Facade.DrainTimeout, err)
		}
		cfg.DrainTimeout = d
	}
	return nil
}

// loadContextConfigFromCRD populates context-store-related config fields from the AgentRuntime CRD.
func loadContextConfigFromCRD(cfg *Config, ar *v1alpha1.AgentRuntime, namespace string) error {
	if ar.Spec.Context != nil && ar.Spec.Context.TTL != nil {
		ttl, err := time.ParseDuration(*ar.Spec.Context.TTL)
		if err != nil {
			return fmt.Errorf("invalid context TTL %q: %w", *ar.Spec.Context.TTL, err)
		}
		cfg.SessionTTL = ttl
	} else {
		cfg.SessionTTL = DefaultSessionTTL
	}

	// ToolRegistry from CRD
	if ar.Spec.ToolRegistryRef != nil {
		cfg.ToolRegistryName = ar.Spec.ToolRegistryRef.Name
		if ar.Spec.ToolRegistryRef.Namespace != nil {
			cfg.ToolRegistryNamespace = *ar.Spec.ToolRegistryRef.Namespace
		} else {
			cfg.ToolRegistryNamespace = namespace
		}
	}

	return nil
}

// loadMediaConfigFromCRD populates media-related config fields from the AgentRuntime CRD.
func loadMediaConfigFromCRD(cfg *Config, ar *v1alpha1.AgentRuntime) {
	if ar.Spec.Media != nil && ar.Spec.Media.BasePath != "" {
		cfg.MediaStorageType = MediaStorageTypeLocal
		cfg.MediaStoragePath = ar.Spec.Media.BasePath
	} else {
		cfg.MediaStorageType = MediaStorageType(getEnvOrDefault(EnvMediaStorageType, string(MediaStorageTypeNone)))
		cfg.MediaStoragePath = getEnvOrDefault(EnvMediaStoragePath, DefaultMediaStoragePath)
	}
	cfg.MediaMaxFileSize = DefaultMediaMaxFileSize
	cfg.MediaDefaultTTL = DefaultMediaDefaultTTL
}

// loadTracingConfigFromEnv populates tracing-related config fields from environment variables.
func loadTracingConfigFromEnv(cfg *Config) error {
	cfg.TracingEnabled = os.Getenv(EnvTracingEnabled) == envValueTrue
	cfg.TracingEndpoint = os.Getenv(EnvTracingEndpoint)
	cfg.TracingInsecure = os.Getenv(EnvTracingInsecure) == envValueTrue
	tracingSampleRate, err := getEnvAsFloat64(EnvTracingSampleRate, 1.0)
	if err != nil {
		return fmt.Errorf(errFmtInvalidEnv, EnvTracingSampleRate, err)
	}
	cfg.TracingSampleRate = tracingSampleRate
	return nil
}

// LoadConfig loads configuration from the AgentRuntime CRD.
// OMNIA_AGENT_NAME and OMNIA_NAMESPACE must be set via the Downward API.
// When running outside a Kubernetes cluster (e.g. demo mode, E2E tests),
// the function falls back to a minimal env-based configuration.
func LoadConfig(ctx context.Context) (*Config, error) {
	name := os.Getenv(EnvAgentName)
	namespace := os.Getenv(EnvNamespace)
	if name == "" || namespace == "" {
		return nil, fmt.Errorf("OMNIA_AGENT_NAME and OMNIA_NAMESPACE are required (set via Downward API)")
	}

	c, err := k8s.NewClient()
	if err != nil {
		// No K8s cluster available — fall back to env-based config (demo/test mode).
		// This is expected when running outside a cluster (local dev, E2E).
		return loadFromEnvFallback(name, namespace)
	}

	cfg, err := LoadFromCRD(ctx, c, name, namespace)
	if err != nil {
		// CRD load failed in-cluster — this is a real error, not demo mode.
		// Do not silently fall back; surface it so the operator can fix the misconfiguration.
		return nil, fmt.Errorf("load config from CRD: %w", err)
	}
	return cfg, nil
}

// loadFromEnvFallback builds a minimal Config from environment variables.
// Used when running outside a Kubernetes cluster (demo mode, E2E tests).
func loadFromEnvFallback(name, namespace string) (*Config, error) {
	cfg := &Config{
		AgentName:      name,
		Namespace:      namespace,
		PromptPackName: os.Getenv(EnvPromptPackName),
		PromptPackPath: getEnvOrDefault(EnvPromptPackMountPath, DefaultPromptPackMountPath),
		FacadeType:     FacadeType(getEnvOrDefault(EnvFacadeType, string(FacadeTypeWebSocket))),
		HandlerMode:    HandlerMode(getEnvOrDefault(EnvHandlerMode, string(HandlerModeRuntime))),
		RuntimeAddress: getEnvOrDefault(EnvRuntimeAddress, DefaultRuntimeAddress),
	}

	facadePort, err := getEnvAsInt(EnvFacadePort, DefaultFacadePort)
	if err != nil {
		return nil, fmt.Errorf(errFmtInvalidEnv, EnvFacadePort, err)
	}
	cfg.FacadePort = facadePort

	healthPort, err := getEnvAsInt(EnvHealthPort, DefaultHealthPort)
	if err != nil {
		return nil, fmt.Errorf(errFmtInvalidEnv, EnvHealthPort, err)
	}
	cfg.HealthPort = healthPort

	cfg.SessionTTL = DefaultSessionTTL

	cfg.MediaStorageType = MediaStorageType(getEnvOrDefault(EnvMediaStorageType, string(MediaStorageTypeNone)))
	cfg.MediaStoragePath = getEnvOrDefault(EnvMediaStoragePath, DefaultMediaStoragePath)
	cfg.MediaMaxFileSize = DefaultMediaMaxFileSize
	cfg.MediaDefaultTTL = DefaultMediaDefaultTTL

	if err := loadTracingConfigFromEnv(cfg); err != nil {
		return nil, err
	}

	if err := loadA2AConfigFromEnv(cfg); err != nil {
		return nil, err
	}

	if err := loadMCPConfigFromEnv(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// loadA2AConfigFromEnv populates A2A-related config fields from environment variables.
func loadA2AConfigFromEnv(cfg *Config) error {
	taskTTLStr := os.Getenv(EnvA2ATaskTTL)
	if taskTTLStr != "" {
		ttl, err := time.ParseDuration(taskTTLStr)
		if err != nil {
			return fmt.Errorf(errFmtInvalidEnv, EnvA2ATaskTTL, err)
		}
		cfg.A2ATaskTTL = ttl
	} else {
		cfg.A2ATaskTTL = DefaultA2ATaskTTL
	}

	convTTLStr := os.Getenv(EnvA2AConversationTTL)
	if convTTLStr != "" {
		ttl, err := time.ParseDuration(convTTLStr)
		if err != nil {
			return fmt.Errorf(errFmtInvalidEnv, EnvA2AConversationTTL, err)
		}
		cfg.A2AConversationTTL = ttl
	} else {
		cfg.A2AConversationTTL = DefaultA2AConversationTTL
	}

	cfg.A2AAuthToken = os.Getenv(EnvA2AAuthToken)
	cfg.A2ATaskStoreType = getEnvOrDefault(EnvA2ATaskStoreType, "memory")
	cfg.A2ARedisURL = os.Getenv(EnvA2ARedisURL)
	cfg.A2AEnabled = os.Getenv(EnvA2AEnabled) == envValueTrue
	cfg.A2AClientsJSON = os.Getenv(EnvA2AClients)

	a2aPort, err := getEnvAsInt(EnvA2APort, DefaultA2APort)
	if err != nil {
		return fmt.Errorf(errFmtInvalidEnv, EnvA2APort, err)
	}
	cfg.A2APort = a2aPort

	return nil
}

// loadMCPConfigFromEnv populates MCP-related config fields from environment variables.
func loadMCPConfigFromEnv(cfg *Config) error {
	cfg.MCPEnabled = os.Getenv(EnvMCPEnabled) == envValueTrue
	if v := os.Getenv(EnvMCPPort); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf(errFmtInvalidEnv, EnvMCPPort, fmt.Errorf("invalid port %q", v))
		}
		cfg.MCPPort = port
	} else {
		cfg.MCPPort = DefaultMCPPort
	}
	return nil
}

// loadMCPConfigFromCRD populates MCP-related config fields from the AgentRuntime CRD.
func loadMCPConfigFromCRD(cfg *Config, ar *v1alpha1.AgentRuntime) {
	if ar.Spec.Facade.MCP != nil {
		cfg.MCPEnabled = ar.Spec.Facade.MCP.Enabled
		if ar.Spec.Facade.MCP.Port != nil {
			cfg.MCPPort = int(*ar.Spec.Facade.MCP.Port)
		} else {
			cfg.MCPPort = DefaultMCPPort
		}
	} else {
		cfg.MCPPort = DefaultMCPPort
	}
}
