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

// Package agent provides the agent runtime configuration and initialization.
package agent

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Environment variable names.
const (
	EnvAgentName           = "OMNIA_AGENT_NAME"
	EnvNamespace           = "OMNIA_NAMESPACE"
	EnvPromptPackName      = "OMNIA_PROMPTPACK_NAME"
	EnvPromptPackVersion   = "OMNIA_PROMPTPACK_VERSION"
	EnvFacadeType          = "OMNIA_FACADE_TYPE"
	EnvFacadePort          = "OMNIA_FACADE_PORT"
	EnvHandlerMode         = "OMNIA_HANDLER_MODE"
	EnvProviderAPIKey      = "OMNIA_PROVIDER_API_KEY"
	EnvToolRegistryName    = "OMNIA_TOOLREGISTRY_NAME"
	EnvToolRegistryNS      = "OMNIA_TOOLREGISTRY_NAMESPACE"
	EnvSessionType         = "OMNIA_SESSION_TYPE"
	EnvSessionTTL          = "OMNIA_SESSION_TTL"
	EnvSessionStoreURL     = "OMNIA_SESSION_STORE_URL"
	EnvPromptPackMountPath = "OMNIA_PROMPTPACK_MOUNT_PATH"
	EnvHealthPort          = "OMNIA_HEALTH_PORT"
)

// Default values.
const (
	DefaultFacadePort          = 8080
	DefaultHealthPort          = 8081
	DefaultSessionTTL          = 24 * time.Hour
	DefaultPromptPackMountPath = "/etc/promptpack"
)

// FacadeType represents the type of facade to use.
type FacadeType string

const (
	FacadeTypeWebSocket FacadeType = "websocket"
)

// SessionType represents the type of session store.
type SessionType string

const (
	SessionTypeMemory SessionType = "memory"
	SessionTypeRedis  SessionType = "redis"
)

// HandlerMode represents the message handler mode.
type HandlerMode string

const (
	// HandlerModeEcho echoes back the input message (for testing).
	HandlerModeEcho HandlerMode = "echo"
	// HandlerModeDemo provides canned responses with streaming simulation (for demos).
	HandlerModeDemo HandlerMode = "demo"
	// HandlerModeRuntime uses the runtime framework in the container (production).
	HandlerModeRuntime HandlerMode = "runtime"
)

// Config holds the agent runtime configuration.
type Config struct {
	// AgentName is the name of the agent.
	AgentName string

	// Namespace is the Kubernetes namespace.
	Namespace string

	// PromptPack configuration.
	PromptPackName    string
	PromptPackVersion string
	PromptPackPath    string

	// Facade configuration.
	FacadeType  FacadeType
	FacadePort  int
	HandlerMode HandlerMode

	// Provider configuration.
	ProviderAPIKey string

	// ToolRegistry configuration (optional).
	ToolRegistryName      string
	ToolRegistryNamespace string

	// Session configuration.
	SessionType     SessionType
	SessionTTL      time.Duration
	SessionStoreURL string

	// Health check port.
	HealthPort int
}

// Error format for wrapping validation errors with values.
const errWithValueFmt = "%w: %s"

// Validation errors.
var (
	ErrMissingAgentName    = errors.New("OMNIA_AGENT_NAME is required")
	ErrMissingNamespace    = errors.New("OMNIA_NAMESPACE is required")
	ErrMissingPromptPack   = errors.New("OMNIA_PROMPTPACK_NAME is required")
	ErrMissingProviderKey  = errors.New("OMNIA_PROVIDER_API_KEY is required for runtime handler mode")
	ErrInvalidFacadeType   = errors.New("invalid facade type")
	ErrInvalidHandlerMode  = errors.New("invalid handler mode")
	ErrInvalidSessionType  = errors.New("invalid session type")
	ErrMissingSessionStore = errors.New("OMNIA_SESSION_STORE_URL is required for redis session type")
)

// LoadFromEnv loads configuration from environment variables.
func LoadFromEnv() (*Config, error) {
	cfg := &Config{
		AgentName:             os.Getenv(EnvAgentName),
		Namespace:             os.Getenv(EnvNamespace),
		PromptPackName:        os.Getenv(EnvPromptPackName),
		PromptPackVersion:     os.Getenv(EnvPromptPackVersion),
		PromptPackPath:        getEnvOrDefault(EnvPromptPackMountPath, DefaultPromptPackMountPath),
		ProviderAPIKey:        os.Getenv(EnvProviderAPIKey),
		ToolRegistryName:      os.Getenv(EnvToolRegistryName),
		ToolRegistryNamespace: os.Getenv(EnvToolRegistryNS),
		SessionStoreURL:       os.Getenv(EnvSessionStoreURL),
	}

	// Parse facade type
	facadeType := getEnvOrDefault(EnvFacadeType, string(FacadeTypeWebSocket))
	cfg.FacadeType = FacadeType(facadeType)

	// Parse handler mode
	handlerMode := getEnvOrDefault(EnvHandlerMode, string(HandlerModeRuntime))
	cfg.HandlerMode = HandlerMode(handlerMode)

	// Parse facade port
	facadePort, err := getEnvAsInt(EnvFacadePort, DefaultFacadePort)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", EnvFacadePort, err)
	}
	cfg.FacadePort = facadePort

	// Parse health port
	healthPort, err := getEnvAsInt(EnvHealthPort, DefaultHealthPort)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", EnvHealthPort, err)
	}
	cfg.HealthPort = healthPort

	// Parse session type
	sessionType := getEnvOrDefault(EnvSessionType, string(SessionTypeMemory))
	cfg.SessionType = SessionType(sessionType)

	// Parse session TTL
	sessionTTL, err := getEnvAsDuration(EnvSessionTTL, DefaultSessionTTL)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", EnvSessionTTL, err)
	}
	cfg.SessionTTL = sessionTTL

	return cfg, nil
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.AgentName == "" {
		return ErrMissingAgentName
	}
	if c.Namespace == "" {
		return ErrMissingNamespace
	}
	if c.PromptPackName == "" {
		return ErrMissingPromptPack
	}

	// Validate handler mode
	switch c.HandlerMode {
	case HandlerModeEcho, HandlerModeDemo:
		// Valid, provider API key not required
	case HandlerModeRuntime:
		// Runtime mode requires provider API key
		if c.ProviderAPIKey == "" {
			return ErrMissingProviderKey
		}
	default:
		return fmt.Errorf(errWithValueFmt, ErrInvalidHandlerMode, c.HandlerMode)
	}

	// Validate facade type
	switch c.FacadeType {
	case FacadeTypeWebSocket:
		// Valid
	default:
		return fmt.Errorf(errWithValueFmt, ErrInvalidFacadeType, c.FacadeType)
	}

	// Validate session type
	switch c.SessionType {
	case SessionTypeMemory:
		// Valid, no additional config needed
	case SessionTypeRedis:
		if c.SessionStoreURL == "" {
			return ErrMissingSessionStore
		}
	default:
		return fmt.Errorf(errWithValueFmt, ErrInvalidSessionType, c.SessionType)
	}

	return nil
}

// Helper functions for environment variable parsing.

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) (int, error) {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue, nil
	}
	return strconv.Atoi(valueStr)
}

func getEnvAsDuration(key string, defaultValue time.Duration) (time.Duration, error) {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue, nil
	}
	return time.ParseDuration(valueStr)
}
