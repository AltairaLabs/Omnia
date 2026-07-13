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

	// Facade config from CRD: derive the flat Config from spec.facades.
	if err := loadFacadesFromCRD(cfg, ar); err != nil {
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

	applyManagementPlanePorts(cfg, ar.Spec.Facades)

	return cfg, nil
}

// applyManagementPlanePorts sets the internal twin-listener ports the facades
// serve behind the mgmt-plane-only chain. They are infrastructure constants
// (not CRD fields), gated per-facade on facades[].managementPlane (default
// true). A facade with managementPlane:false leaves its surface's internal port
// at zero and no internal listener is started for it.
//
// Must run after loadFacadesFromCRD so cfg.A2AEnabled (a2a primary vs secondary)
// is known: a standalone a2a primary maps to the facade twin port, a secondary
// a2a maps to the a2a twin port.
func applyManagementPlanePorts(cfg *Config, facades []v1alpha1.FacadeConfig) {
	for i := range facades {
		f := &facades[i]
		if !f.ManagementPlaneEnabled() {
			continue
		}
		switch f.Type {
		case v1alpha1.FacadeTypeWebSocket, v1alpha1.FacadeTypeREST:
			cfg.InternalFacadePort = DefaultInternalFacadePort
		case v1alpha1.FacadeTypeA2A:
			if cfg.A2AEnabled {
				cfg.InternalA2APort = DefaultInternalA2APort
			} else {
				cfg.InternalFacadePort = DefaultInternalFacadePort
			}
		case v1alpha1.FacadeTypeMCP:
			cfg.InternalMCPPort = DefaultInternalMCPPort
		}
	}
}

// loadInternalPortsFromEnv reads the internal twin-listener ports from the
// environment (demo/E2E fallback path, where there is no CRD to derive them
// from). Unset means zero — no internal listener for that surface.
func loadInternalPortsFromEnv(cfg *Config) error {
	for _, p := range []struct {
		env string
		dst *int
	}{
		{EnvInternalFacadePort, &cfg.InternalFacadePort},
		{EnvInternalA2APort, &cfg.InternalA2APort},
		{EnvInternalMCPPort, &cfg.InternalMCPPort},
	} {
		v, err := getEnvAsInt(p.env, 0)
		if err != nil {
			return fmt.Errorf(errFmtInvalidEnv, p.env, err)
		}
		*p.dst = v
	}
	return nil
}

// loadFacadesFromCRD derives the flat facade-related Config from spec.facades.
// Each facade entry is a single protocol surface; the primary (the listener
// main.go dispatches to) is the websocket facade in agent mode, the rest facade
// in function mode, or the a2a facade when it is the only agent-mode facade. An
// a2a or mcp facade present alongside a primary becomes a secondary listener
// (today's dual-protocol shape), so the flat Config (FacadeType/FacadePort,
// A2AEnabled/A2APort, MCPEnabled/MCPPort) is unchanged and cmd/agent startup is
// untouched.
func loadFacadesFromCRD(cfg *Config, ar *v1alpha1.AgentRuntime) error {
	facades := ar.Spec.Facades
	wsF := findFacade(facades, v1alpha1.FacadeTypeWebSocket)
	a2aF := findFacade(facades, v1alpha1.FacadeTypeA2A)
	restF := findFacade(facades, v1alpha1.FacadeTypeREST)
	mcpF := findFacade(facades, v1alpha1.FacadeTypeMCP)

	primary := primaryFacade(wsF, restF, a2aF)
	if primary == nil {
		return fmt.Errorf("spec.facades has no primary facade (websocket, rest, or a2a)")
	}
	if err := applyPrimaryFacade(cfg, primary); err != nil {
		return err
	}
	if err := applyA2AFacade(cfg, a2aF, wsF != nil); err != nil {
		return err
	}
	applyMCPFacade(cfg, mcpF)
	return nil
}

// findFacade returns the facade of the given type, or nil if absent. CEL
// guarantees at most one facade per type.
func findFacade(facades []v1alpha1.FacadeConfig, t v1alpha1.FacadeType) *v1alpha1.FacadeConfig {
	for i := range facades {
		if facades[i].Type == t {
			return &facades[i]
		}
	}
	return nil
}

// primaryFacade picks the listener main.go dispatches to: websocket (agent) >
// rest (function) > a2a (standalone agent).
func primaryFacade(wsF, restF, a2aF *v1alpha1.FacadeConfig) *v1alpha1.FacadeConfig {
	switch {
	case wsF != nil:
		return wsF
	case restF != nil:
		return restF
	default:
		return a2aF
	}
}

// applyPrimaryFacade copies the primary facade's type, port, and timeouts into
// the flat Config.
func applyPrimaryFacade(cfg *Config, f *v1alpha1.FacadeConfig) error {
	cfg.FacadeType = FacadeType(f.Type)
	cfg.FacadePort = int32PtrOr(f.Port, DefaultFacadePort)
	if f.ClientToolTimeout != nil {
		cfg.ClientToolTimeout = f.ClientToolTimeout.Duration
	}
	if f.DrainTimeout != nil {
		d, err := time.ParseDuration(*f.DrainTimeout)
		if err != nil {
			return fmt.Errorf("invalid drain timeout %q: %w", *f.DrainTimeout, err)
		}
		cfg.DrainTimeout = d
	}
	return nil
}

// applyA2AFacade loads the A2A TTLs, task store, clients, and dual-protocol port
// from the a2a facade entry. wsPrimary reports whether a websocket facade is the
// primary; when true the a2a facade is a secondary listener (A2AEnabled) on
// A2APort, otherwise a2a is itself the primary (on FacadePort) and A2AEnabled
// stays false.
func applyA2AFacade(cfg *Config, f *v1alpha1.FacadeConfig, wsPrimary bool) error {
	cfg.A2ATaskTTL = DefaultA2ATaskTTL
	cfg.A2AConversationTTL = DefaultA2AConversationTTL
	cfg.A2ATaskStoreType = getEnvOrDefault(EnvA2ATaskStoreType, "memory")
	cfg.A2ARedisURL = os.Getenv(EnvA2ARedisURL)
	cfg.A2APort = DefaultA2APort
	if f == nil {
		return nil
	}
	cfg.A2AEnabled = wsPrimary
	// Resolved A2A clients are injected as JSON by the operator.
	cfg.A2AClientsJSON = os.Getenv(EnvA2AClients)

	a2a := f.A2A
	if a2a == nil {
		return nil
	}
	if err := loadA2ATTLsFromCRD(cfg, a2a); err != nil {
		return err
	}
	cfg.A2APort = int32PtrOr(a2a.Port, DefaultA2APort)
	loadA2ATaskStoreFromCRD(cfg, a2a)
	return nil
}

// applyMCPFacade enables the MCP secondary listener and sets its port from the
// mcp facade entry.
func applyMCPFacade(cfg *Config, f *v1alpha1.FacadeConfig) {
	cfg.MCPPort = DefaultMCPPort
	if f == nil {
		return
	}
	cfg.MCPEnabled = true
	if f.MCP != nil && f.MCP.Port != nil {
		cfg.MCPPort = int(*f.MCP.Port)
	}
}

// int32PtrOr returns *p as an int, or def when p is nil.
func int32PtrOr(p *int32, def int) int {
	if p != nil {
		return int(*p)
	}
	return def
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
	cfg.MediaUploadURLTTL = getEnvDuration(EnvMediaUploadURLTTL, DefaultMediaUploadURLTTL)
	cfg.MediaDownloadURLTTL = getEnvDuration(EnvMediaDownloadURLTTL, DefaultMediaDownloadURLTTL)
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
	cfg.MediaUploadURLTTL = getEnvDuration(EnvMediaUploadURLTTL, DefaultMediaUploadURLTTL)
	cfg.MediaDownloadURLTTL = getEnvDuration(EnvMediaDownloadURLTTL, DefaultMediaDownloadURLTTL)

	if err := loadTracingConfigFromEnv(cfg); err != nil {
		return nil, err
	}

	if err := loadA2AConfigFromEnv(cfg); err != nil {
		return nil, err
	}

	if err := loadMCPConfigFromEnv(cfg); err != nil {
		return nil, err
	}

	if err := loadInternalPortsFromEnv(cfg); err != nil {
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
