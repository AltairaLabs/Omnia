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

	return cfg, nil
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

// LoadConfig loads configuration, preferring CRD reading and falling back to env vars.
func LoadConfig(ctx context.Context) (*Config, error) {
	name := os.Getenv(EnvAgentName)
	namespace := os.Getenv(EnvNamespace)

	if name != "" && namespace != "" {
		c, err := k8s.NewClient()
		if err == nil {
			cfg, crdErr := LoadFromCRD(ctx, c, name, namespace)
			if crdErr == nil {
				return cfg, nil
			}
			// Fall through to env-based loading
		}
	}

	return LoadFromEnv()
}
