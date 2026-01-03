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

// Package runtime implements the PromptKit runtime container.
package runtime

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds runtime configuration loaded from environment variables.
type Config struct {
	// Agent identification
	AgentName string
	Namespace string

	// PromptPack configuration
	PromptPackPath string // Path to the compiled .pack.json file
	PromptName     string // Name of the prompt to use from the pack

	// Session configuration
	SessionType string        // "memory" or "redis"
	SessionURL  string        // Redis URL for session store
	SessionTTL  time.Duration // Session TTL

	// Provider configuration
	ProviderType string // "auto", "claude", "openai", "gemini"
	Model        string // Model override (e.g., "claude-3-opus")
	BaseURL      string // Custom base URL for API calls

	// Mock provider configuration (for testing)
	MockProvider   bool   // Enable mock provider instead of real LLM
	MockConfigPath string // Path to mock responses YAML file (optional)

	// Tools configuration
	ToolsConfigPath string // Path to tools.yaml configuration file

	// Tracing configuration
	TracingEnabled    bool    // Enable OpenTelemetry tracing
	TracingEndpoint   string  // OTLP collector endpoint (e.g., "localhost:4317")
	TracingSampleRate float64 // Sampling rate (0.0 to 1.0)
	TracingInsecure   bool    // Disable TLS for OTLP connection

	// Server ports
	GRPCPort   int
	HealthPort int
}

// Environment variable names.
const (
	envAgentName         = "OMNIA_AGENT_NAME"
	envNamespace         = "OMNIA_NAMESPACE"
	envPromptPackPath    = "OMNIA_PROMPTPACK_PATH"
	envPromptName        = "OMNIA_PROMPT_NAME"
	envSessionType       = "OMNIA_SESSION_TYPE"
	envSessionURL        = "OMNIA_SESSION_URL"
	envSessionTTL        = "OMNIA_SESSION_TTL"
	envProviderType      = "OMNIA_PROVIDER_TYPE"
	envProviderModel     = "OMNIA_PROVIDER_MODEL"
	envProviderBaseURL   = "OMNIA_PROVIDER_BASE_URL"
	envMockProvider      = "OMNIA_MOCK_PROVIDER"
	envMockConfigPath    = "OMNIA_MOCK_CONFIG"
	envToolsConfigPath   = "OMNIA_TOOLS_CONFIG"
	envTracingEnabled    = "OMNIA_TRACING_ENABLED"
	envTracingEndpoint   = "OMNIA_TRACING_ENDPOINT"
	envTracingSampleRate = "OMNIA_TRACING_SAMPLE_RATE"
	envTracingInsecure   = "OMNIA_TRACING_INSECURE"
	envGRPCPort          = "OMNIA_GRPC_PORT"
	envHealthPort        = "OMNIA_HEALTH_PORT"
)

// Default values.
const (
	defaultPromptPackPath  = "/etc/omnia/pack/pack.json"
	defaultPromptName      = "default"
	defaultSessionType     = "memory"
	defaultSessionTTL      = 24 * time.Hour
	defaultProviderType    = "auto"
	defaultToolsConfigPath = "/etc/omnia/tools/tools.yaml"
	defaultGRPCPort        = 9000
	defaultHealthPort      = 9001
)

// Session type constants.
const (
	SessionTypeMemory = "memory"
	SessionTypeRedis  = "redis"
)

// Provider type constants.
const (
	ProviderTypeAuto   = "auto"
	ProviderTypeClaude = "claude"
	ProviderTypeOpenAI = "openai"
	ProviderTypeGemini = "gemini"
)

// LoadConfig loads configuration from environment variables.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		AgentName:         os.Getenv(envAgentName),
		Namespace:         os.Getenv(envNamespace),
		PromptPackPath:    getEnvOrDefault(envPromptPackPath, defaultPromptPackPath),
		PromptName:        getEnvOrDefault(envPromptName, defaultPromptName),
		SessionType:       getEnvOrDefault(envSessionType, defaultSessionType),
		SessionURL:        os.Getenv(envSessionURL),
		ProviderType:      getEnvOrDefault(envProviderType, defaultProviderType),
		Model:             os.Getenv(envProviderModel),
		BaseURL:           os.Getenv(envProviderBaseURL),
		MockProvider:      os.Getenv(envMockProvider) == "true",
		MockConfigPath:    os.Getenv(envMockConfigPath),
		ToolsConfigPath:   getEnvOrDefault(envToolsConfigPath, defaultToolsConfigPath),
		TracingEnabled:    os.Getenv(envTracingEnabled) == "true",
		TracingEndpoint:   os.Getenv(envTracingEndpoint),
		TracingSampleRate: 1.0, // Default to sampling all traces
		TracingInsecure:   os.Getenv(envTracingInsecure) == "true",
		GRPCPort:          defaultGRPCPort,
		HealthPort:        defaultHealthPort,
		SessionTTL:        defaultSessionTTL,
	}

	// Parse tracing sample rate
	if rate := os.Getenv(envTracingSampleRate); rate != "" {
		r, err := strconv.ParseFloat(rate, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", envTracingSampleRate, err)
		}
		if r < 0 || r > 1 {
			return nil, fmt.Errorf("invalid %s: must be between 0.0 and 1.0", envTracingSampleRate)
		}
		cfg.TracingSampleRate = r
	}

	// Parse ports
	if port := os.Getenv(envGRPCPort); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", envGRPCPort, err)
		}
		cfg.GRPCPort = p
	}

	if port := os.Getenv(envHealthPort); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", envHealthPort, err)
		}
		cfg.HealthPort = p
	}

	// Parse session TTL
	if ttl := os.Getenv(envSessionTTL); ttl != "" {
		d, err := time.ParseDuration(ttl)
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", envSessionTTL, err)
		}
		cfg.SessionTTL = d
	}

	// Validate required fields
	if cfg.AgentName == "" {
		return nil, fmt.Errorf("%s is required", envAgentName)
	}
	if cfg.Namespace == "" {
		return nil, fmt.Errorf("%s is required", envNamespace)
	}

	// Validate session type
	switch cfg.SessionType {
	case SessionTypeMemory, SessionTypeRedis:
		// Valid
	default:
		return nil, fmt.Errorf("invalid %s: must be 'memory' or 'redis'", envSessionType)
	}

	// Validate Redis URL if using Redis
	if cfg.SessionType == SessionTypeRedis && cfg.SessionURL == "" {
		return nil, fmt.Errorf("%s is required when using Redis sessions", envSessionURL)
	}

	// Validate provider type
	switch cfg.ProviderType {
	case ProviderTypeAuto, ProviderTypeClaude, ProviderTypeOpenAI, ProviderTypeGemini:
		// Valid
	default:
		return nil, fmt.Errorf("invalid %s: must be 'auto', 'claude', 'openai', or 'gemini'", envProviderType)
	}

	return cfg, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
