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

// Package runtime provides the PromptKit runtime for executing agent conversations.
package runtime

import (
	"errors"
	"os"
	"strconv"
	"time"
)

// Environment variable names for runtime configuration.
const (
	EnvAgentName       = "OMNIA_AGENT_NAME"
	EnvNamespace       = "OMNIA_NAMESPACE"
	EnvSessionType     = "OMNIA_SESSION_TYPE"
	EnvSessionURL      = "OMNIA_SESSION_URL"
	EnvSessionTTL      = "OMNIA_SESSION_TTL"
	EnvProviderAPIKey  = "OMNIA_PROVIDER_API_KEY"
	EnvProviderType    = "OMNIA_PROVIDER_TYPE"
	EnvPromptPackPath  = "OMNIA_PROMPTPACK_PATH"
	EnvToolsConfigPath = "OMNIA_TOOLS_CONFIG"
	EnvGRPCPort        = "OMNIA_GRPC_PORT"
	EnvHealthPort      = "OMNIA_HEALTH_PORT"
)

// Default values for configuration.
const (
	DefaultPromptPackPath  = "/var/promptpacks"
	DefaultToolsConfigPath = "/etc/omnia/tools.yaml"
	DefaultGRPCPort        = 9000
	DefaultHealthPort      = 9001
	DefaultSessionTTL      = 24 * time.Hour
	DefaultProviderType    = "openai"
)

// Session types.
const (
	SessionTypeMemory = "memory"
	SessionTypeRedis  = "redis"
)

// Provider types.
const (
	ProviderTypeOpenAI    = "openai"
	ProviderTypeAnthropic = "anthropic"
)

// Configuration errors.
var (
	ErrMissingAgentName    = errors.New("OMNIA_AGENT_NAME is required")
	ErrMissingNamespace    = errors.New("OMNIA_NAMESPACE is required")
	ErrMissingProviderKey  = errors.New("OMNIA_PROVIDER_API_KEY is required")
	ErrMissingSessionURL   = errors.New("OMNIA_SESSION_URL is required for redis session type")
	ErrInvalidSessionType  = errors.New("invalid session type: must be 'memory' or 'redis'")
	ErrInvalidProviderType = errors.New("invalid provider type: must be 'openai' or 'anthropic'")
)

// Config holds the runtime configuration.
type Config struct {
	// AgentName is the name of this agent instance.
	AgentName string
	// Namespace is the Kubernetes namespace.
	Namespace string

	// SessionType is the session store type (memory or redis).
	SessionType string
	// SessionURL is the Redis connection URL (for redis session type).
	SessionURL string
	// SessionTTL is the session time-to-live.
	SessionTTL time.Duration

	// ProviderType is the LLM provider (openai or anthropic).
	ProviderType string
	// ProviderAPIKey is the API key for the LLM provider.
	ProviderAPIKey string

	// PromptPackPath is the path to the mounted PromptPack directory.
	PromptPackPath string
	// ToolsConfigPath is the path to the tools configuration file.
	ToolsConfigPath string

	// GRPCPort is the port for the gRPC server.
	GRPCPort int
	// HealthPort is the port for health check endpoints.
	HealthPort int
}

// LoadConfig loads configuration from environment variables.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		AgentName:       os.Getenv(EnvAgentName),
		Namespace:       os.Getenv(EnvNamespace),
		SessionType:     getEnvOrDefault(EnvSessionType, SessionTypeMemory),
		SessionURL:      os.Getenv(EnvSessionURL),
		ProviderType:    getEnvOrDefault(EnvProviderType, DefaultProviderType),
		ProviderAPIKey:  os.Getenv(EnvProviderAPIKey),
		PromptPackPath:  getEnvOrDefault(EnvPromptPackPath, DefaultPromptPackPath),
		ToolsConfigPath: getEnvOrDefault(EnvToolsConfigPath, DefaultToolsConfigPath),
		GRPCPort:        DefaultGRPCPort,
		HealthPort:      DefaultHealthPort,
		SessionTTL:      DefaultSessionTTL,
	}

	// Parse gRPC port
	if portStr := os.Getenv(EnvGRPCPort); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, errors.New("invalid OMNIA_GRPC_PORT: " + err.Error())
		}
		cfg.GRPCPort = port
	}

	// Parse health port
	if portStr := os.Getenv(EnvHealthPort); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, errors.New("invalid OMNIA_HEALTH_PORT: " + err.Error())
		}
		cfg.HealthPort = port
	}

	// Parse session TTL
	if ttlStr := os.Getenv(EnvSessionTTL); ttlStr != "" {
		ttl, err := time.ParseDuration(ttlStr)
		if err != nil {
			return nil, errors.New("invalid OMNIA_SESSION_TTL: " + err.Error())
		}
		cfg.SessionTTL = ttl
	}

	return cfg, cfg.Validate()
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	if c.AgentName == "" {
		return ErrMissingAgentName
	}
	if c.Namespace == "" {
		return ErrMissingNamespace
	}
	if c.ProviderAPIKey == "" {
		return ErrMissingProviderKey
	}

	// Validate session type
	switch c.SessionType {
	case SessionTypeMemory, SessionTypeRedis:
		// valid
	default:
		return ErrInvalidSessionType
	}

	// Redis requires a URL
	if c.SessionType == SessionTypeRedis && c.SessionURL == "" {
		return ErrMissingSessionURL
	}

	// Validate provider type
	switch c.ProviderType {
	case ProviderTypeOpenAI, ProviderTypeAnthropic:
		// valid
	default:
		return ErrInvalidProviderType
	}

	return nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
