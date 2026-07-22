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
	AgentUID      string // Kubernetes UID of the AgentRuntime CR; used as agent_id scope for memory.
	Namespace     string
	WorkspaceName string

	// PromptPack configuration
	PromptPackPath      string // Path to the compiled .pack.json file
	PromptPackName      string // Name of the PromptPack CRD (for metrics)
	PromptPackNamespace string // Namespace of the PromptPack CRD (for metrics)
	PromptPackVersion   string // Version of the PromptPack (for tracing)
	PromptName          string // Name of the prompt to use from the pack

	// Function configuration (spec.mode == "function"). Consumed by
	// resolveResponseFormat to constrain the provider's output (#1483).
	Mode             string // AgentRuntime spec.mode ("agent" or "function")
	OutputFormat     string // spec.outputFormat ("", "text", "json", "json_schema"); "" resolves to the default
	OutputSchemaJSON []byte // raw spec.outputSchema bytes, used as the json_schema response-format schema

	// Context store configuration
	ContextType string        // "memory" or "redis"
	ContextURL  string        // Redis URL for context store
	ContextTTL  time.Duration // Context TTL

	// Provider configuration
	ProviderType         string // "claude", "openai", "gemini", "ollama", "mock", "vllm", "voyageai"
	Model                string // Model override (e.g., "claude-3-opus")
	BaseURL              string // Custom base URL for API calls
	ProviderRefName      string // Name of the Provider CRD (for metrics, if using providerRef)
	ProviderRefNamespace string // Namespace of the Provider CRD (for metrics)

	// ProviderAPIKey is the resolved API key for the default (flat) provider,
	// read from its Secret at boot. Carried on the value — NOT written to
	// process env — so same-type providers cannot overwrite each other's key
	// (design §5.3.1). Empty for keyless providers (ollama/mock) and platform
	// providers (which still use env in this wave).
	ProviderAPIKey string

	// Custom headers passed to every provider request.
	// Empty map/nil means no custom headers. Used for gateway providers like OpenRouter.
	Headers map[string]string

	// ExtraProviders carries every non-default spec.providers[] entry resolved
	// by role (e.g. inference, embedding). The default llm provider is flattened
	// into the scalar fields above and does NOT appear here. A later task maps
	// these to PromptKit WithXProvider options.
	ExtraProviders []ResolvedProvider

	// Platform hosting configuration. Empty PlatformType means direct provider access.
	// When set, PlatformType is one of "bedrock", "vertex", "azure".
	PlatformType     string
	PlatformRegion   string
	PlatformProject  string
	PlatformEndpoint string

	// Auth configuration for platform-hosted providers.
	// AuthType is one of "workloadIdentity", "accessKey", "serviceAccount", "servicePrincipal".
	// Empty AuthType means direct provider access (uses API-key credential flow).
	AuthType                  string
	AuthRoleArn               string
	AuthServiceAccountEmail   string
	AuthCredentialsSecretName string // Secret containing platform credentials
	AuthCredentialsSecretKey  string // Optional key within the secret

	// Provider pricing (from CRD, passed to PromptKit for cost calculation)
	InputCostPer1K  float64 // Cost per 1000 input tokens (0 = use provider built-in pricing)
	OutputCostPer1K float64 // Cost per 1000 output tokens (0 = use provider built-in pricing)

	// Context management
	ContextWindow      int    // Token budget for conversation context (0 = no limit)
	TruncationStrategy string // How to handle context overflow: "sliding", "summarize", "custom"

	// Provider timeouts
	ProviderRequestTimeout    time.Duration // Non-streaming HTTP call timeout (0 = provider default)
	ProviderStreamIdleTimeout time.Duration // SSE stream idle timeout (0 = 30s default)

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

	// Memory configuration
	MemoryEnabled bool   // Enable cross-session memory
	MemoryAPIURL  string // URL of memory-api service for memory store
	WorkspaceUID  string // Kubernetes UID of the Workspace CRD (used as workspace_id scope for memory)

	// MemoryRetrievalEnabled is spec.memory.retrieval.enabled (ambient RAG
	// auto-injection). Defaults to true when memory is enabled and the field is
	// unset, preserving existing behavior.
	MemoryRetrievalEnabled bool
	// MemoryToolsEnabled is spec.memory.tools.enabled (memory__remember /
	// memory__recall). Defaults to true when memory is enabled and the field is
	// unset, preserving existing behavior.
	MemoryToolsEnabled bool

	// MemoryStrategy is the retrieval strategy from spec.memory.retrieval.strategy
	// ("keyword"|"semantic"|...). Empty defaults to the existing keyword/FTS path.
	MemoryStrategy string
	// MemoryLimit is spec.memory.retrieval.limit (0 = use retriever default).
	MemoryLimit int
	// MemoryDenyCEL is spec.memory.retrieval.accessFilter.denyCEL (empty = no filter).
	MemoryDenyCEL string

	// Eval configuration
	EvalEnabled bool // Enable real-time evals for PromptKit agents

	// InlineEvalGroups is the set of eval group names that run inline in
	// this runtime. An absent/empty value uses the built-in default
	// (DefaultInlineEvalGroups) — lightweight, deterministic evals only.
	// Worker groups (long-running, external) are resolved on the
	// eval-worker side, not here.
	InlineEvalGroups []string

	// Tracing configuration
	TracingEnabled    bool    // Enable OpenTelemetry tracing
	TracingEndpoint   string  // OTLP collector endpoint (e.g., "localhost:4317")
	TracingSampleRate float64 // Sampling rate (0.0 to 1.0)
	TracingInsecure   bool    // Disable TLS for OTLP connection

	// DuplexAudio is the required realtime audio format for duplex sessions,
	// resolved from spec.duplex.audio. When set, the runtime advertises it as
	// the bounded MediaNegotiation counter-offer in RuntimeHello and prefers it
	// over the client's DuplexStart proposal. Nil means accept the client's
	// proposed format.
	DuplexAudio *DuplexAudioParams

	// Server ports
	GRPCPort   int
	HealthPort int
}

// DuplexAudioParams is the resolved required audio format for duplex sessions.
// A zero value for a field means "no requirement" for that field (the client's
// proposal, or the built-in default, is used instead).
type DuplexAudioParams struct {
	Codec      string
	SampleRate int
	Channels   int
}

// Environment variable names.
const (
	envAgentName         = "OMNIA_AGENT_NAME"
	envNamespace         = "OMNIA_NAMESPACE"
	envPromptPackPath    = "OMNIA_PROMPTPACK_PATH"
	envPromptName        = "OMNIA_PROMPT_NAME"
	envContextURL        = "OMNIA_CONTEXT_URL"
	envContextTTL        = "OMNIA_CONTEXT_TTL"
	envTracingEnabled    = "OMNIA_TRACING_ENABLED"
	envTracingEndpoint   = "OMNIA_TRACING_ENDPOINT"
	envTracingSampleRate = "OMNIA_TRACING_SAMPLE_RATE"
	envTracingInsecure   = "OMNIA_TRACING_INSECURE"
	envGRPCPort          = "OMNIA_GRPC_PORT"
	envHealthPort        = "OMNIA_HEALTH_PORT"
	// envCanaryOverridePath points at the mounted canary override file. Set on
	// candidate pods by the operator; unset on stable / non-rollout pods.
	envCanaryOverridePath = "OMNIA_CANARY_OVERRIDE_PATH"
	// envPromptPackVersion carries the operator-resolved PromptPack's concrete
	// version. Used as a fallback when spec.promptPackRef.Version is nil (a
	// `track:`-selected AgentRuntime), so the eval-path version stamp is
	// always concrete instead of empty (#1847).
	envPromptPackVersion = "OMNIA_PROMPTPACK_VERSION"
	// envWorkspaceUID carries the Workspace CR's UID, injected by the operator
	// when memory is enabled. Preferred over a cluster-wide WorkspaceList so
	// every memory-enabled agent pod does not List all workspaces at startup.
	envWorkspaceUID = "OMNIA_WORKSPACE_UID"
)

// Default values.
const (
	defaultPromptPackPath     = "/etc/omnia/pack/pack.json"
	defaultPromptName         = "default"
	defaultContextType        = "memory"
	defaultContextTTL         = 24 * time.Hour
	defaultMediaBasePath      = "/etc/omnia/media"
	defaultToolsMountPath     = "/etc/omnia/tools"
	defaultToolsConfigFile    = "tools.yaml"
	defaultCanaryOverridePath = "/etc/omnia/canary/override.json"
	defaultGRPCPort           = 9000
	defaultHealthPort         = 9001
)

// Error format constants.
const (
	errFmtInvalidEnvVar = "invalid %s: %w"
)

// Context store type constants.
const (
	ContextTypeMemory = "memory"
	ContextTypeRedis  = "redis"
)

// parseEnvironmentOverrides parses optional environment variable overrides.
// ContextWindow and TruncationStrategy are CRD-derived and are loaded by
// loadProviderDefaults() — do not re-read them from env vars here.
func (cfg *Config) parseEnvironmentOverrides() error {
	if err := cfg.parseTracingSampleRate(); err != nil {
		return err
	}
	if err := cfg.parsePorts(); err != nil {
		return err
	}
	return cfg.parseContextTTL()
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

// parseContextTTL parses the context store TTL from the OMNIA_CONTEXT_TTL
// environment variable. (Distinct from the dashboard's OMNIA_SESSION_TTL auth
// cookie TTL, which is an unrelated concern and never set on the runtime pod.)
func (cfg *Config) parseContextTTL() error {
	ttl := os.Getenv(envContextTTL)
	if ttl == "" {
		return nil
	}
	d, err := time.ParseDuration(ttl)
	if err != nil {
		return fmt.Errorf(errFmtInvalidEnvVar, envContextTTL, err)
	}
	cfg.ContextTTL = d
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
