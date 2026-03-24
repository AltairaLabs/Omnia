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
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/altairalabs/omnia/pkg/k8s"
)

// Config holds runtime configuration loaded from environment variables.
type Config struct {
	// Agent identification
	AgentName     string
	Namespace     string
	WorkspaceName string

	// PromptPack configuration
	PromptPackPath      string // Path to the compiled .pack.json file
	PromptPackName      string // Name of the PromptPack CRD (for metrics)
	PromptPackNamespace string // Namespace of the PromptPack CRD (for metrics)
	PromptPackVersion   string // Version of the PromptPack (for tracing)
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
	ToolsConfigPath       string // Path to tools.yaml configuration file
	ToolRegistryName      string // Name of the ToolRegistry CRD (for metadata enrichment)
	ToolRegistryNamespace string // Namespace of the ToolRegistry CRD

	// Session recording (Pattern C)
	SessionAPIURL string // URL of the session-api service for event recording

	// Eval configuration
	EvalEnabled bool // Enable real-time evals for PromptKit agents

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
	envSessionURL        = "OMNIA_SESSION_URL"
	envSessionTTL        = "OMNIA_SESSION_TTL"
	envContextWindow     = "OMNIA_CONTEXT_WINDOW"
	envSessionAPIURL     = "SESSION_API_URL"
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
	defaultMediaBasePath   = "/etc/omnia/media"
	defaultToolsMountPath  = "/etc/omnia/tools"
	defaultToolsConfigFile = "tools.yaml"
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

// LoadConfigWithContext loads configuration from the AgentRuntime CRD.
// OMNIA_AGENT_NAME and OMNIA_NAMESPACE must be set via the Downward API.
func LoadConfigWithContext(ctx context.Context) (*Config, error) {
	name := os.Getenv(envAgentName)
	namespace := os.Getenv(envNamespace)
	if name == "" || namespace == "" {
		return nil, fmt.Errorf("OMNIA_AGENT_NAME and OMNIA_NAMESPACE are required (set via Downward API)")
	}

	c, err := k8s.NewClient()
	if err != nil {
		return nil, fmt.Errorf("create k8s client: %w", err)
	}

	return LoadFromCRD(ctx, c, name, namespace)
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
