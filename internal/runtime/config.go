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

	"github.com/altairalabs/omnia/pkg/provider"
)

// Config holds runtime configuration loaded from environment variables.
type Config struct {
	// Agent identification
	AgentName string
	Namespace string

	// PromptPack configuration
	PromptPackPath      string // Path to the compiled .pack.json file
	PromptPackName      string // Name of the PromptPack CRD (for metrics)
	PromptPackNamespace string // Namespace of the PromptPack CRD (for metrics)
	PromptName          string // Name of the prompt to use from the pack

	// Session configuration
	SessionType string        // "memory" or "redis"
	SessionURL  string        // Redis URL for session store
	SessionTTL  time.Duration // Session TTL

	// Provider configuration
	ProviderType         string // "claude", "openai", "gemini", "ollama", "mock"
	Model                string // Model override (e.g., "claude-3-opus")
	BaseURL              string // Custom base URL for API calls
	ProviderRefName      string // Name of the Provider CRD (for metrics, if using providerRef)
	ProviderRefNamespace string // Namespace of the Provider CRD (for metrics)

	// Context management
	ContextWindow      int    // Token budget for conversation context (0 = no limit)
	TruncationStrategy string // How to handle context overflow: "sliding", "summarize", "custom"

	// Mock provider configuration (for testing)
	MockProvider   bool   // Enable mock provider instead of real LLM
	MockConfigPath string // Path to mock responses YAML file (optional)
	MediaBasePath  string // Base path for resolving mock:// URLs to media files

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
	envAgentName              = "OMNIA_AGENT_NAME"
	envNamespace              = "OMNIA_NAMESPACE"
	envPromptPackPath         = "OMNIA_PROMPTPACK_PATH"
	envPromptPackName         = "OMNIA_PROMPTPACK_NAME"
	envPromptPackNamespace    = "OMNIA_PROMPTPACK_NAMESPACE"
	envPromptName             = "OMNIA_PROMPT_NAME"
	envSessionType            = "OMNIA_SESSION_TYPE"
	envSessionURL             = "OMNIA_SESSION_URL"
	envSessionTTL             = "OMNIA_SESSION_TTL"
	envProviderType           = "OMNIA_PROVIDER_TYPE"
	envProviderModel          = "OMNIA_PROVIDER_MODEL"
	envProviderBaseURL        = "OMNIA_PROVIDER_BASE_URL"
	envProviderRefName        = "OMNIA_PROVIDER_REF_NAME"
	envProviderRefNamespace   = "OMNIA_PROVIDER_REF_NAMESPACE"
	envContextWindow          = "OMNIA_CONTEXT_WINDOW"
	envTruncationStrategy     = "OMNIA_TRUNCATION_STRATEGY"
	envMockProvider           = "OMNIA_MOCK_PROVIDER"
	envMockConfigPath         = "OMNIA_MOCK_CONFIG"
	envProviderMockConfigPath = "OMNIA_PROVIDER_MOCK_CONFIG" // From additionalConfig
	envMediaBasePath          = "OMNIA_MEDIA_BASE_PATH"
	envToolsConfigPath        = "OMNIA_TOOLS_CONFIG"
	envTracingEnabled         = "OMNIA_TRACING_ENABLED"
	envTracingEndpoint        = "OMNIA_TRACING_ENDPOINT"
	envTracingSampleRate      = "OMNIA_TRACING_SAMPLE_RATE"
	envTracingInsecure        = "OMNIA_TRACING_INSECURE"
	envGRPCPort               = "OMNIA_GRPC_PORT"
	envHealthPort             = "OMNIA_HEALTH_PORT"
)

// Default values.
const (
	defaultPromptPackPath  = "/etc/omnia/pack/pack.json"
	defaultPromptName      = "default"
	defaultSessionType     = "memory"
	defaultSessionTTL      = 24 * time.Hour
	defaultProviderType    = "" // Provider type must be explicitly set
	defaultMediaBasePath   = "/etc/omnia/media"
	defaultToolsConfigPath = "" // Empty by default; only set when OMNIA_TOOLS_CONFIG_PATH is provided
	defaultGRPCPort        = 9000
	defaultHealthPort      = 9001
)

// Error format constants.
const (
	errFmtInvalidEnvVar = "invalid %s: %w"
)

// Session type constants.
const (
	SessionTypeMemory = "memory"
	SessionTypeRedis  = "redis"
)

// LoadConfig loads configuration from environment variables.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		AgentName:            os.Getenv(envAgentName),
		Namespace:            os.Getenv(envNamespace),
		PromptPackPath:       getEnvOrDefault(envPromptPackPath, defaultPromptPackPath),
		PromptPackName:       os.Getenv(envPromptPackName),
		PromptPackNamespace:  os.Getenv(envPromptPackNamespace),
		PromptName:           getEnvOrDefault(envPromptName, defaultPromptName),
		SessionType:          getEnvOrDefault(envSessionType, defaultSessionType),
		SessionURL:           os.Getenv(envSessionURL),
		ProviderType:         getEnvOrDefault(envProviderType, defaultProviderType),
		Model:                os.Getenv(envProviderModel),
		BaseURL:              os.Getenv(envProviderBaseURL),
		ProviderRefName:      os.Getenv(envProviderRefName),
		ProviderRefNamespace: os.Getenv(envProviderRefNamespace),
		TruncationStrategy:   os.Getenv(envTruncationStrategy),
		MockProvider:         os.Getenv(envMockProvider) == "true",
		MockConfigPath:       getEnvOrDefault(envMockConfigPath, os.Getenv(envProviderMockConfigPath)),
		MediaBasePath:        getEnvOrDefault(envMediaBasePath, defaultMediaBasePath),
		ToolsConfigPath:      getEnvOrDefault(envToolsConfigPath, defaultToolsConfigPath),
		TracingEnabled:       os.Getenv(envTracingEnabled) == "true",
		TracingEndpoint:      os.Getenv(envTracingEndpoint),
		TracingSampleRate:    1.0, // Default to sampling all traces
		TracingInsecure:      os.Getenv(envTracingInsecure) == "true",
		GRPCPort:             defaultGRPCPort,
		HealthPort:           defaultHealthPort,
		SessionTTL:           defaultSessionTTL,
	}

	if err := cfg.parseEnvironmentOverrides(); err != nil {
		return nil, err
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// Auto-enable mock provider when provider type is "mock"
	// This allows provider.type: mock to work as a first-class selection
	if cfg.ProviderType == string(provider.TypeMock) {
		cfg.MockProvider = true
	}

	return cfg, nil
}

// parseEnvironmentOverrides parses optional environment variable overrides.
func (cfg *Config) parseEnvironmentOverrides() error {
	if err := cfg.parseTracingSampleRate(); err != nil {
		return err
	}
	if err := cfg.parsePorts(); err != nil {
		return err
	}
	if err := cfg.parseContextWindow(); err != nil {
		return err
	}
	return cfg.parseSessionTTL()
}

// parseContextWindow parses the context window size from environment.
func (cfg *Config) parseContextWindow() error {
	ctx := os.Getenv(envContextWindow)
	if ctx == "" {
		return nil
	}
	c, err := strconv.Atoi(ctx)
	if err != nil {
		return fmt.Errorf(errFmtInvalidEnvVar, envContextWindow, err)
	}
	if c < 0 {
		return fmt.Errorf("invalid %s: must be positive", envContextWindow)
	}
	cfg.ContextWindow = c
	return nil
}

// parseTracingSampleRate parses the tracing sample rate from environment.
func (cfg *Config) parseTracingSampleRate() error {
	rate := os.Getenv(envTracingSampleRate)
	if rate == "" {
		return nil
	}
	r, err := strconv.ParseFloat(rate, 64)
	if err != nil {
		return fmt.Errorf(errFmtInvalidEnvVar, envTracingSampleRate, err)
	}
	if r < 0 || r > 1 {
		return fmt.Errorf("invalid %s: must be between 0.0 and 1.0", envTracingSampleRate)
	}
	cfg.TracingSampleRate = r
	return nil
}

// parsePorts parses GRPC and health port overrides.
func (cfg *Config) parsePorts() error {
	if port := os.Getenv(envGRPCPort); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf(errFmtInvalidEnvVar, envGRPCPort, err)
		}
		cfg.GRPCPort = p
	}
	if port := os.Getenv(envHealthPort); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf(errFmtInvalidEnvVar, envHealthPort, err)
		}
		cfg.HealthPort = p
	}
	return nil
}

// parseSessionTTL parses the session TTL from environment.
func (cfg *Config) parseSessionTTL() error {
	ttl := os.Getenv(envSessionTTL)
	if ttl == "" {
		return nil
	}
	d, err := time.ParseDuration(ttl)
	if err != nil {
		return fmt.Errorf(errFmtInvalidEnvVar, envSessionTTL, err)
	}
	cfg.SessionTTL = d
	return nil
}

// validate validates the configuration.
func (cfg *Config) validate() error {
	if err := cfg.validateRequiredFields(); err != nil {
		return err
	}
	if err := cfg.validateSessionConfig(); err != nil {
		return err
	}
	return cfg.validateProviderType()
}

// validateRequiredFields checks that required fields are set.
func (cfg *Config) validateRequiredFields() error {
	if cfg.AgentName == "" {
		return fmt.Errorf("%s is required", envAgentName)
	}
	if cfg.Namespace == "" {
		return fmt.Errorf("%s is required", envNamespace)
	}
	return nil
}

// validateSessionConfig validates session configuration.
func (cfg *Config) validateSessionConfig() error {
	switch cfg.SessionType {
	case SessionTypeMemory, SessionTypeRedis:
		// Valid
	default:
		return fmt.Errorf("invalid %s: must be 'memory' or 'redis'", envSessionType)
	}
	if cfg.SessionType == SessionTypeRedis && cfg.SessionURL == "" {
		return fmt.Errorf("%s is required when using Redis sessions", envSessionURL)
	}
	return nil
}

// validateProviderType validates the provider type.
// Empty provider type is allowed (means no provider configured).
func (cfg *Config) validateProviderType() error {
	if cfg.ProviderType == "" {
		return nil // Empty is valid - no provider configured
	}
	if !provider.Type(cfg.ProviderType).IsValid() {
		return fmt.Errorf("invalid %s: must be one of %v", envProviderType, provider.ValidTypes)
	}
	return nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
