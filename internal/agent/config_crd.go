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

	cfg := &Config{
		AgentName: name,
		Namespace: namespace,
	}

	// PromptPack info from CRD
	cfg.PromptPackName = ar.Spec.PromptPackRef.Name
	if ar.Spec.PromptPackRef.Version != nil {
		cfg.PromptPackVersion = *ar.Spec.PromptPackRef.Version
	}
	cfg.PromptPackPath = getEnvOrDefault(EnvPromptPackMountPath, DefaultPromptPackMountPath)

	// Facade config from CRD
	cfg.FacadeType = FacadeType(ar.Spec.Facade.Type)
	if ar.Spec.Facade.Port != nil {
		cfg.FacadePort = int(*ar.Spec.Facade.Port)
	} else {
		cfg.FacadePort = DefaultFacadePort
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

	if err := loadSessionConfigFromCRD(cfg, ar, namespace); err != nil {
		return nil, err
	}
	loadMediaConfigFromCRD(cfg, ar)

	if err := loadTracingConfigFromEnv(cfg); err != nil {
		return nil, err
	}

	if err := loadA2AConfigFromCRD(cfg, ar); err != nil {
		return nil, err
	}

	return cfg, nil
}

// loadA2AConfigFromCRD populates A2A-related config fields from the AgentRuntime CRD.
func loadA2AConfigFromCRD(cfg *Config, ar *v1alpha1.AgentRuntime) error {
	if ar.Spec.A2A == nil {
		cfg.A2ATaskTTL = DefaultA2ATaskTTL
		cfg.A2AConversationTTL = DefaultA2AConversationTTL
		return nil
	}

	if ar.Spec.A2A.TaskTTL != nil {
		ttl, err := time.ParseDuration(*ar.Spec.A2A.TaskTTL)
		if err != nil {
			return fmt.Errorf("invalid A2A task TTL %q: %w", *ar.Spec.A2A.TaskTTL, err)
		}
		cfg.A2ATaskTTL = ttl
	} else {
		cfg.A2ATaskTTL = DefaultA2ATaskTTL
	}

	if ar.Spec.A2A.ConversationTTL != nil {
		ttl, err := time.ParseDuration(*ar.Spec.A2A.ConversationTTL)
		if err != nil {
			return fmt.Errorf("invalid A2A conversation TTL %q: %w", *ar.Spec.A2A.ConversationTTL, err)
		}
		cfg.A2AConversationTTL = ttl
	} else {
		cfg.A2AConversationTTL = DefaultA2AConversationTTL
	}

	cfg.A2AAuthToken = os.Getenv(EnvA2AAuthToken)

	// Dual-protocol: A2A as additional endpoint alongside websocket/grpc.
	cfg.A2AEnabled = ar.Spec.A2A.Enabled
	if ar.Spec.A2A.Port != nil {
		cfg.A2APort = int(*ar.Spec.A2A.Port)
	} else {
		cfg.A2APort = DefaultA2APort
	}

	// Task store config from CRD.
	if ar.Spec.A2A.TaskStore != nil {
		cfg.A2ATaskStoreType = string(ar.Spec.A2A.TaskStore.Type)
		if ar.Spec.A2A.TaskStore.RedisURL != "" {
			cfg.A2ARedisURL = ar.Spec.A2A.TaskStore.RedisURL
		}
		// RedisSecretRef is resolved by the operator into OMNIA_A2A_REDIS_URL env var.
		if envURL := os.Getenv(EnvA2ARedisURL); envURL != "" {
			cfg.A2ARedisURL = envURL
		}
	} else {
		cfg.A2ATaskStoreType = getEnvOrDefault(EnvA2ATaskStoreType, "memory")
		cfg.A2ARedisURL = os.Getenv(EnvA2ARedisURL)
	}

	// Resolved A2A clients are injected as JSON by the operator.
	cfg.A2AClientsJSON = os.Getenv(EnvA2AClients)

	return nil
}

// loadSessionConfigFromCRD populates session-related config fields from the AgentRuntime CRD.
func loadSessionConfigFromCRD(cfg *Config, ar *v1alpha1.AgentRuntime, namespace string) error {
	if ar.Spec.Session != nil {
		cfg.SessionType = SessionType(ar.Spec.Session.Type)
		if ar.Spec.Session.TTL != nil {
			ttl, err := time.ParseDuration(*ar.Spec.Session.TTL)
			if err != nil {
				return fmt.Errorf("invalid session TTL %q: %w", *ar.Spec.Session.TTL, err)
			}
			cfg.SessionTTL = ttl
		} else {
			cfg.SessionTTL = DefaultSessionTTL
		}
	} else {
		cfg.SessionType = SessionTypeMemory
		cfg.SessionTTL = DefaultSessionTTL
	}
	cfg.SessionStoreURL = os.Getenv(EnvSessionStoreURL)

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
		// No K8s cluster available — fall back to env-based config (demo/test mode)
		return loadFromEnvFallback(name, namespace)
	}

	cfg, err := LoadFromCRD(ctx, c, name, namespace)
	if err != nil {
		// CRD unavailable — fall back to env-based config (demo/test mode)
		return loadFromEnvFallback(name, namespace)
	}
	return cfg, nil
}

// loadFromEnvFallback builds a minimal Config from environment variables.
// Used when running outside a Kubernetes cluster (demo mode, E2E tests).
func loadFromEnvFallback(name, namespace string) (*Config, error) {
	cfg := &Config{
		AgentName:       name,
		Namespace:       namespace,
		PromptPackName:  os.Getenv(EnvPromptPackName),
		PromptPackPath:  getEnvOrDefault(EnvPromptPackMountPath, DefaultPromptPackMountPath),
		FacadeType:      FacadeType(getEnvOrDefault(EnvFacadeType, string(FacadeTypeWebSocket))),
		HandlerMode:     HandlerMode(getEnvOrDefault(EnvHandlerMode, string(HandlerModeRuntime))),
		RuntimeAddress:  getEnvOrDefault(EnvRuntimeAddress, DefaultRuntimeAddress),
		SessionStoreURL: os.Getenv(EnvSessionStoreURL),
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

	sessionType := getEnvOrDefault(EnvSessionType, string(SessionTypeMemory))
	cfg.SessionType = SessionType(sessionType)
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
