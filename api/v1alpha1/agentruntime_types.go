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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PromptPackRef references a PromptPack to use for this agent runtime.
type PromptPackRef struct {
	// name is the name of the PromptPack resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// version specifies a specific version of the PromptPack to use.
	// If not specified, the track field is used instead.
	// +optional
	Version *string `json:"version,omitempty"`

	// track specifies which release track to follow (e.g., "stable", "canary").
	// Only used if version is not specified.
	// +kubebuilder:default="stable"
	// +optional
	Track *string `json:"track,omitempty"`
}

// FacadeType defines the type of facade for client connections.
// +kubebuilder:validation:Enum=websocket;grpc;a2a
type FacadeType string

const (
	// FacadeTypeWebSocket uses WebSocket for client connections.
	FacadeTypeWebSocket FacadeType = "websocket"
	// FacadeTypeGRPC uses gRPC for client connections.
	FacadeTypeGRPC FacadeType = "grpc"
	// FacadeTypeA2A uses the A2A JSON-RPC protocol for agent-to-agent communication.
	FacadeTypeA2A FacadeType = "a2a"
)

// HandlerMode defines the message handler mode for the facade.
// +kubebuilder:validation:Enum=echo;demo;runtime
type HandlerMode string

const (
	// HandlerModeEcho echoes back the input message (for testing).
	HandlerModeEcho HandlerMode = "echo"
	// HandlerModeDemo provides canned responses with streaming simulation (for demos).
	HandlerModeDemo HandlerMode = "demo"
	// HandlerModeRuntime uses the runtime framework in the container (production).
	HandlerModeRuntime HandlerMode = "runtime"
)

// FacadeConfig defines the configuration for the client-facing facade.
type FacadeConfig struct {
	// type specifies the facade protocol type.
	// +kubebuilder:validation:Required
	// +kubebuilder:default="websocket"
	Type FacadeType `json:"type"`

	// port is the port number for the facade service.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default=8080
	// +optional
	Port *int32 `json:"port,omitempty"`

	// handler specifies the message handler mode.
	// "echo" returns input messages back (for testing connectivity).
	// "demo" provides streaming responses with simulated tool calls (for demos).
	// "runtime" uses the runtime framework in the container (default, for production).
	// +kubebuilder:default="runtime"
	// +optional
	Handler *HandlerMode `json:"handler,omitempty"`

	// image overrides the default facade container image.
	// Use this to specify a custom facade image or private registry.
	// +optional
	Image string `json:"image,omitempty"`

	// extraEnv defines additional environment variables for the facade container.
	// Use this for debugging (e.g., LOG_LEVEL=debug) or custom configuration.
	// +optional
	ExtraEnv []corev1.EnvVar `json:"extraEnv,omitempty"`

	// clientToolTimeout is the max time to wait for client tool responses per turn.
	// Defaults to 60s.
	// +optional
	ClientToolTimeout *metav1.Duration `json:"clientToolTimeout,omitempty"`
}

// ToolRegistryRef references a ToolRegistry resource.
type ToolRegistryRef struct {
	// name is the name of the ToolRegistry resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// namespace is the namespace of the ToolRegistry resource.
	// If not specified, the same namespace as the AgentRuntime is used.
	// +optional
	Namespace *string `json:"namespace,omitempty"`
}

// SessionStoreType defines the type of session store.
// +kubebuilder:validation:Enum=memory;redis;postgres
type SessionStoreType string

const (
	// SessionStoreTypeMemory uses in-memory storage (not recommended for production).
	SessionStoreTypeMemory SessionStoreType = "memory"
	// SessionStoreTypeRedis uses Redis for session storage.
	SessionStoreTypeRedis SessionStoreType = "redis"
	// SessionStoreTypePostgres uses PostgreSQL for session storage.
	SessionStoreTypePostgres SessionStoreType = "postgres"
)

// SessionConfig defines the configuration for session management.
type SessionConfig struct {
	// type specifies the session store backend.
	// +kubebuilder:validation:Required
	// +kubebuilder:default="memory"
	Type SessionStoreType `json:"type"`

	// storeRef references a secret containing connection details for the session store.
	// Required for redis and postgres store types.
	// +optional
	StoreRef *corev1.LocalObjectReference `json:"storeRef,omitempty"`

	// ttl is the time-to-live for sessions in duration format (e.g., "24h", "30m").
	// +kubebuilder:default="24h"
	// +optional
	TTL *string `json:"ttl,omitempty"`
}

// AutoscalerType defines the type of autoscaler to use.
// +kubebuilder:validation:Enum=hpa;keda
type AutoscalerType string

const (
	// AutoscalerTypeHPA uses standard Kubernetes HPA.
	AutoscalerTypeHPA AutoscalerType = "hpa"
	// AutoscalerTypeKEDA uses KEDA for advanced scaling (requires KEDA installed).
	AutoscalerTypeKEDA AutoscalerType = "keda"
)

// KEDATrigger defines a KEDA scaling trigger.
type KEDATrigger struct {
	// type is the KEDA trigger type (e.g., "prometheus", "cron").
	// +kubebuilder:validation:Required
	Type string `json:"type"`

	// metadata contains trigger-specific configuration.
	// For prometheus: serverAddress, query, threshold
	// For cron: timezone, start, end, desiredReplicas
	// +kubebuilder:validation:Required
	Metadata map[string]string `json:"metadata"`
}

// KEDAConfig defines KEDA-specific autoscaling configuration.
type KEDAConfig struct {
	// pollingInterval is the interval in seconds to check triggers. Defaults to 30.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=30
	// +optional
	PollingInterval *int32 `json:"pollingInterval,omitempty"`

	// cooldownPeriod is the wait period in seconds after last trigger before scaling down. Defaults to 300.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=300
	// +optional
	CooldownPeriod *int32 `json:"cooldownPeriod,omitempty"`

	// triggers is the list of KEDA triggers for scaling.
	// If empty, a default Prometheus trigger for connections is configured.
	// +optional
	Triggers []KEDATrigger `json:"triggers,omitempty"`

	// connectionThreshold is the target number of active connections per pod
	// for the default KEDA Prometheus trigger. Only used when triggers is empty.
	// Defaults to 200 for text workloads. Set lower (e.g., 20) for audio/media workloads.
	// +kubebuilder:validation:Minimum=1
	// +optional
	ConnectionThreshold *int32 `json:"connectionThreshold,omitempty"`
}

// AutoscalingConfig defines horizontal pod autoscaling settings.
// Agents are typically I/O bound (waiting on LLM API calls), not CPU bound.
// Memory-based scaling is the default since each connection/session uses memory.
type AutoscalingConfig struct {
	// enabled specifies whether autoscaling is enabled.
	// When enabled, the autoscaler will manage replica count instead of spec.runtime.replicas.
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// type specifies which autoscaler to use. Defaults to "hpa".
	// Use "keda" for advanced scaling (scale to zero, Prometheus metrics, cron).
	// +kubebuilder:default="hpa"
	// +optional
	Type AutoscalerType `json:"type,omitempty"`

	// minReplicas is the minimum number of replicas.
	// For KEDA, set to 0 to enable scale-to-zero.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// maxReplicas is the maximum number of replicas.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=100
	// +optional
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`

	// targetMemoryUtilizationPercentage is the target average memory utilization.
	// Memory is the primary scaling metric since each WebSocket connection and
	// session consumes memory. Defaults to 70%. Only used for HPA type.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=70
	// +optional
	TargetMemoryUtilizationPercentage *int32 `json:"targetMemoryUtilizationPercentage,omitempty"`

	// targetCPUUtilizationPercentage is the target average CPU utilization.
	// CPU is a secondary metric since agents are typically I/O bound.
	// Set to nil to disable CPU-based scaling. Defaults to 90% as a safety valve.
	// Only used for HPA type.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=90
	// +optional
	TargetCPUUtilizationPercentage *int32 `json:"targetCPUUtilizationPercentage,omitempty"`

	// scaleDownStabilizationSeconds is the number of seconds to wait before
	// scaling down after a scale-up. This prevents thrashing when connections
	// are bursty. Defaults to 300 (5 minutes). Only used for HPA type.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=3600
	// +kubebuilder:default=300
	// +optional
	ScaleDownStabilizationSeconds *int32 `json:"scaleDownStabilizationSeconds,omitempty"`

	// keda contains KEDA-specific configuration. Only used when type is "keda".
	// +optional
	KEDA *KEDAConfig `json:"keda,omitempty"`
}

// ProviderType defines the LLM provider type.
// +kubebuilder:validation:Enum=claude;openai;gemini;ollama;mock;bedrock;vertex;azure-ai
type ProviderType string

const (
	// ProviderTypeClaude uses Anthropic's Claude models.
	ProviderTypeClaude ProviderType = "claude"
	// ProviderTypeOpenAI uses OpenAI's GPT models.
	ProviderTypeOpenAI ProviderType = "openai"
	// ProviderTypeGemini uses Google's Gemini models.
	ProviderTypeGemini ProviderType = "gemini"
	// ProviderTypeOllama uses locally-hosted Ollama models.
	// Does not require secretRef. Requires baseURL to be set.
	ProviderTypeOllama ProviderType = "ollama"
	// ProviderTypeMock uses PromptKit's mock provider for testing.
	// Does not require secretRef. Returns canned responses based on scenario.
	ProviderTypeMock ProviderType = "mock"
	// ProviderTypeBedrock uses AWS Bedrock for LLM access.
	// Uses IAM-based authentication; does not require traditional API key credentials.
	ProviderTypeBedrock ProviderType = "bedrock"
	// ProviderTypeVertex uses GCP Vertex AI for LLM access.
	// Uses workload identity or service account credentials.
	ProviderTypeVertex ProviderType = "vertex"
	// ProviderTypeAzureAI uses Azure AI Foundry for LLM access.
	// Uses Azure-native authentication.
	ProviderTypeAzureAI ProviderType = "azure-ai"
)

// TruncationStrategy defines how to handle context overflow.
// +kubebuilder:validation:Enum=sliding;summarize;custom
type TruncationStrategy string

const (
	// TruncationStrategySliding removes oldest messages first (default).
	TruncationStrategySliding TruncationStrategy = "sliding"
	// TruncationStrategySummarize summarizes old messages before removing.
	TruncationStrategySummarize TruncationStrategy = "summarize"
	// TruncationStrategyCustom delegates to custom runtime implementation.
	TruncationStrategyCustom TruncationStrategy = "custom"
)

// ProviderDefaults defines tuning parameters for the LLM provider.
type ProviderDefaults struct {
	// temperature controls randomness in responses (0.0-2.0).
	// Lower values make output more focused and deterministic.
	// Specified as a string to support decimal values (e.g., "0.7").
	// +optional
	Temperature *string `json:"temperature,omitempty"`

	// topP controls nucleus sampling (0.0-1.0).
	// Specified as a string to support decimal values (e.g., "0.9").
	// +optional
	TopP *string `json:"topP,omitempty"`

	// maxTokens limits the maximum number of tokens in the response.
	// +optional
	MaxTokens *int32 `json:"maxTokens,omitempty"`

	// contextWindow is the model's maximum context size in tokens.
	// When conversation history exceeds this budget, truncation is applied.
	// If not specified, no automatic truncation is performed.
	// +optional
	ContextWindow *int32 `json:"contextWindow,omitempty"`

	// truncationStrategy defines how to handle context overflow.
	// - sliding: Remove oldest messages first (default)
	// - summarize: Summarize old messages before removing
	// - custom: Delegate to custom runtime implementation
	// +kubebuilder:default=sliding
	// +optional
	TruncationStrategy TruncationStrategy `json:"truncationStrategy,omitempty"`
}

// ProviderPricing defines cost tracking configuration for the provider.
type ProviderPricing struct {
	// inputCostPer1K is the cost per 1000 input tokens (e.g., "0.003").
	// +optional
	InputCostPer1K *string `json:"inputCostPer1K,omitempty"`

	// outputCostPer1K is the cost per 1000 output tokens (e.g., "0.015").
	// +optional
	OutputCostPer1K *string `json:"outputCostPer1K,omitempty"`

	// cachedCostPer1K is the cost per 1000 cached tokens (e.g., "0.0003").
	// Cached tokens have reduced cost with some providers.
	// +optional
	CachedCostPer1K *string `json:"cachedCostPer1K,omitempty"`
}

// NamedProviderRef associates a name with a Provider CRD reference.
// The name is used to look up providers by role (e.g. "default", "judge").
type NamedProviderRef struct {
	// name is the logical name for this provider (e.g. "default", "judge", "embeddings").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// providerRef references the Provider CRD.
	// +kubebuilder:validation:Required
	ProviderRef ProviderRef `json:"providerRef"`
}

// ProviderRef references a Provider resource.
type ProviderRef struct {
	// name is the name of the Provider resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// namespace is the namespace of the Provider resource.
	// If not specified, the same namespace as the AgentRuntime is used.
	// +optional
	Namespace *string `json:"namespace,omitempty"`
}

// ProviderConfig defines the LLM provider configuration.
type ProviderConfig struct {
	// type specifies the provider type.
	// "claude", "openai", "gemini", "ollama", or "mock".
	// +kubebuilder:validation:Required
	Type ProviderType `json:"type"`

	// model specifies the model identifier (e.g., "claude-sonnet-4-20250514", "gpt-4o").
	// If not specified, the provider's default model is used.
	// +optional
	Model string `json:"model,omitempty"`

	// secretRef references a Secret containing API credentials.
	// The secret should contain a key matching the provider's expected env var:
	// - ANTHROPIC_API_KEY for Claude
	// - OPENAI_API_KEY for OpenAI
	// - GEMINI_API_KEY or GOOGLE_API_KEY for Gemini
	// Or use "api-key" as a generic key name.
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`

	// baseURL overrides the provider's default API endpoint.
	// Useful for proxies or self-hosted models.
	// +optional
	BaseURL string `json:"baseURL,omitempty"`

	// config contains provider tuning parameters.
	// +optional
	Config *ProviderDefaults `json:"config,omitempty"`

	// pricing configures cost tracking for this provider.
	// If not specified, PromptKit's built-in pricing is used.
	// +optional
	Pricing *ProviderPricing `json:"pricing,omitempty"`

	// additionalConfig contains provider-specific settings passed to PromptKit.
	// For Ollama: "keep_alive" (e.g., "5m") to keep model loaded between requests.
	// For Mock: "mock_config" path to mock responses YAML file.
	// +optional
	AdditionalConfig map[string]string `json:"additionalConfig,omitempty"`
}

// FrameworkType defines which agent framework to use.
// +kubebuilder:validation:Enum=promptkit;langchain;autogen;custom
type FrameworkType string

const (
	// FrameworkTypePromptKit uses AltairaLabs' PromptKit framework.
	FrameworkTypePromptKit FrameworkType = "promptkit"
	// FrameworkTypeLangChain uses the LangChain framework.
	FrameworkTypeLangChain FrameworkType = "langchain"
	// FrameworkTypeAutoGen uses Microsoft's AutoGen framework.
	FrameworkTypeAutoGen FrameworkType = "autogen"
	// FrameworkTypeCustom uses a user-provided container image.
	FrameworkTypeCustom FrameworkType = "custom"
)

// FrameworkConfig specifies which agent framework to use.
// This enables Omnia's "no vendor lock-in" promise by supporting multiple frameworks.
type FrameworkConfig struct {
	// type specifies the agent framework to use.
	// +kubebuilder:validation:Required
	// +kubebuilder:default="promptkit"
	Type FrameworkType `json:"type"`

	// version specifies the framework version to use.
	// If not specified, the latest supported version is used.
	// +optional
	Version string `json:"version,omitempty"`

	// image overrides the default container image for the framework.
	// Required when type is "custom".
	// For built-in frameworks, this allows using a custom build or private registry.
	// +optional
	Image string `json:"image,omitempty"`
}

// MediaConfig defines configuration for media file resolution.
type MediaConfig struct {
	// basePath is the base directory for resolving mock:// URLs.
	// Defaults to /etc/omnia/media if not specified.
	// +optional
	// +kubebuilder:default="/etc/omnia/media"
	BasePath string `json:"basePath,omitempty"`
}

// ConsoleConfig defines configuration for the dashboard console UI.
type ConsoleConfig struct {
	// allowedAttachmentTypes specifies MIME types allowed for file uploads.
	// Supports specific types ("image/png") and wildcards ("image/*").
	// If not specified, defaults to common types: image/*, audio/*, application/pdf, text/plain, text/markdown.
	// +optional
	AllowedAttachmentTypes []string `json:"allowedAttachmentTypes,omitempty"`

	// allowedExtensions specifies file extensions as fallback for browsers with generic MIME types.
	// If not specified, extensions are inferred from allowedAttachmentTypes.
	// +optional
	AllowedExtensions []string `json:"allowedExtensions,omitempty"`

	// maxFileSize is the maximum file size in bytes for attachments.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10485760
	// +optional
	MaxFileSize *int64 `json:"maxFileSize,omitempty"`

	// maxFiles is the maximum number of files that can be attached to a single message.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=20
	// +kubebuilder:default=5
	// +optional
	MaxFiles *int32 `json:"maxFiles,omitempty"`

	// mediaRequirements defines provider-specific requirements for different media types.
	// When not specified, the dashboard applies sensible defaults based on the provider type.
	// +optional
	MediaRequirements *MediaRequirements `json:"mediaRequirements,omitempty"`
}

// Dimensions represents width and height in pixels.
type Dimensions struct {
	// width in pixels.
	// +kubebuilder:validation:Minimum=1
	Width int32 `json:"width"`

	// height in pixels.
	// +kubebuilder:validation:Minimum=1
	Height int32 `json:"height"`
}

// ImageRequirements defines requirements for image media.
type ImageRequirements struct {
	// maxSizeBytes is the maximum file size in bytes for images.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxSizeBytes *int64 `json:"maxSizeBytes,omitempty"`

	// maxDimensions specifies the maximum width and height.
	// Images exceeding these will need to be resized.
	// +optional
	MaxDimensions *Dimensions `json:"maxDimensions,omitempty"`

	// recommendedDimensions specifies optimal dimensions for best results.
	// +optional
	RecommendedDimensions *Dimensions `json:"recommendedDimensions,omitempty"`

	// supportedFormats lists supported image formats (e.g., "png", "jpeg", "gif", "webp").
	// +optional
	SupportedFormats []string `json:"supportedFormats,omitempty"`

	// preferredFormat is the format that yields best results with this provider.
	// +optional
	PreferredFormat string `json:"preferredFormat,omitempty"`

	// compressionGuidance provides guidance on image compression.
	// +kubebuilder:validation:Enum=none;lossless;lossy-high;lossy-medium;lossy-low
	// +optional
	CompressionGuidance string `json:"compressionGuidance,omitempty"`
}

// VideoRequirements defines requirements for video media.
type VideoRequirements struct {
	// maxDurationSeconds is the maximum video duration.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxDurationSeconds *int32 `json:"maxDurationSeconds,omitempty"`

	// supportsSegmentSelection indicates if the provider supports selecting video segments.
	// +optional
	SupportsSegmentSelection bool `json:"supportsSegmentSelection,omitempty"`

	// processingMode indicates how video is processed.
	// +kubebuilder:validation:Enum=frames;transcription;both;native
	// +optional
	ProcessingMode string `json:"processingMode,omitempty"`

	// frameExtractionInterval is the interval in seconds between extracted frames.
	// Only applicable when processingMode includes "frames".
	// +kubebuilder:validation:Minimum=1
	// +optional
	FrameExtractionInterval *int32 `json:"frameExtractionInterval,omitempty"`
}

// AudioRequirements defines requirements for audio media.
type AudioRequirements struct {
	// maxDurationSeconds is the maximum audio duration.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxDurationSeconds *int32 `json:"maxDurationSeconds,omitempty"`

	// recommendedSampleRate is the optimal sample rate in Hz.
	// +kubebuilder:validation:Minimum=1
	// +optional
	RecommendedSampleRate *int32 `json:"recommendedSampleRate,omitempty"`

	// supportsSegmentSelection indicates if the provider supports selecting audio segments.
	// +optional
	SupportsSegmentSelection bool `json:"supportsSegmentSelection,omitempty"`
}

// DocumentRequirements defines requirements for document media.
type DocumentRequirements struct {
	// maxPages is the maximum number of pages that can be processed.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxPages *int32 `json:"maxPages,omitempty"`

	// supportsOCR indicates if the provider supports OCR for scanned documents.
	// +optional
	SupportsOCR bool `json:"supportsOCR,omitempty"`
}

// MediaRequirements defines provider-specific requirements for different media types.
// These requirements help the dashboard optimize file handling and provide user guidance.
type MediaRequirements struct {
	// image defines requirements for image files.
	// +optional
	Image *ImageRequirements `json:"image,omitempty"`

	// video defines requirements for video files.
	// +optional
	Video *VideoRequirements `json:"video,omitempty"`

	// audio defines requirements for audio files.
	// +optional
	Audio *AudioRequirements `json:"audio,omitempty"`

	// document defines requirements for document files (PDFs, etc.).
	// +optional
	Document *DocumentRequirements `json:"document,omitempty"`
}

// A2AConfig configures the A2A (Agent-to-Agent) protocol facade.
// When facade.type is "a2a", this is the primary protocol.
// When facade.type is "websocket" or "grpc", set enabled: true to add A2A
// as an additional endpoint alongside the primary facade.
type A2AConfig struct {
	// enabled adds A2A as an additional endpoint alongside the primary facade.
	// Only meaningful when facade.type is NOT "a2a" (i.e., websocket or grpc).
	// When facade.type is "a2a", A2A is always the primary protocol regardless of this field.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// port is the TCP port for the A2A endpoint in dual-protocol mode.
	// Defaults to 9999. Only used when enabled is true and facade.type is not "a2a".
	// +kubebuilder:default=9999
	// +optional
	Port *int32 `json:"port,omitempty"`

	// agentCard configures the Agent Card served at /.well-known/agent.json.
	// +optional
	AgentCard *AgentCardSpec `json:"agentCard,omitempty"`

	// taskTTL is how long completed/failed/canceled tasks are retained before eviction.
	// Uses Go duration format (e.g., "1h", "30m"). Defaults to "1h".
	// +kubebuilder:default="1h"
	// +optional
	TaskTTL *string `json:"taskTTL,omitempty"`

	// conversationTTL is how long idle conversations are retained before eviction.
	// Uses Go duration format (e.g., "30m", "1h"). Defaults to "30m".
	// +kubebuilder:default="30m"
	// +optional
	ConversationTTL *string `json:"conversationTTL,omitempty"`

	// authentication configures request authentication for the A2A endpoint.
	// +optional
	Authentication *A2AAuthConfig `json:"authentication,omitempty"`

	// taskStore configures the task persistence backend.
	// Defaults to in-memory. Set type to "redis" for persistence across restarts.
	// +optional
	TaskStore *A2ATaskStoreConfig `json:"taskStore,omitempty"`

	// clients configures connections to other A2A agents.
	// Each client can reference an in-cluster AgentRuntime or an external URL.
	// +optional
	Clients []A2AClientSpec `json:"clients,omitempty"`
}

// A2ATaskStoreType represents the backend type for A2A task storage.
// +kubebuilder:validation:Enum=memory;redis
type A2ATaskStoreType string

const (
	// A2ATaskStoreMemory uses an in-memory task store (default).
	A2ATaskStoreMemory A2ATaskStoreType = "memory"

	// A2ATaskStoreRedis uses Redis for task persistence.
	A2ATaskStoreRedis A2ATaskStoreType = "redis"
)

// A2ATaskStoreConfig configures the A2A task persistence backend.
type A2ATaskStoreConfig struct {
	// type is the task store backend type. Defaults to "memory".
	// +kubebuilder:default="memory"
	// +kubebuilder:validation:Enum=memory;redis
	// +optional
	Type A2ATaskStoreType `json:"type,omitempty"`

	// redisURL is the Redis connection URL when type is "redis".
	// Format: redis://[:password@]host:port[/db]
	// +optional
	RedisURL string `json:"redisURL,omitempty"`

	// redisSecretRef references a Secret containing a Redis connection URL
	// in a key named "url". Takes precedence over redisURL.
	// +optional
	RedisSecretRef *corev1.LocalObjectReference `json:"redisSecretRef,omitempty"`
}

// AgentCardSpec configures the A2A Agent Card.
type AgentCardSpec struct {
	// name is the agent's display name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// description is a human-readable description of the agent.
	// +optional
	Description string `json:"description,omitempty"`

	// version is the agent's version string.
	// +optional
	Version string `json:"version,omitempty"`

	// organization is the name of the organization that provides this agent.
	// +optional
	Organization string `json:"organization,omitempty"`

	// skills lists the agent's capabilities for discovery.
	// +optional
	Skills []AgentSkillSpec `json:"skills,omitempty"`

	// capabilities describes protocol features the agent supports.
	// +optional
	Capabilities *AgentCapabilitiesSpec `json:"capabilities,omitempty"`

	// defaultInputModes lists supported input content types (e.g., "text", "audio").
	// +optional
	DefaultInputModes []string `json:"defaultInputModes,omitempty"`

	// defaultOutputModes lists supported output content types (e.g., "text", "audio").
	// +optional
	DefaultOutputModes []string `json:"defaultOutputModes,omitempty"`
}

// AgentSkillSpec describes a specific skill an agent can perform.
type AgentSkillSpec struct {
	// id is the unique identifier for this skill.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	// name is the human-readable name for this skill.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// description explains what this skill does.
	// +optional
	Description string `json:"description,omitempty"`

	// tags are keywords for categorization and search.
	// +optional
	Tags []string `json:"tags,omitempty"`

	// examples provides example prompts for this skill.
	// +optional
	Examples []string `json:"examples,omitempty"`
}

// AgentCapabilitiesSpec describes A2A protocol features the agent supports.
type AgentCapabilitiesSpec struct {
	// streaming indicates whether the agent supports streaming responses via SSE.
	// +optional
	Streaming bool `json:"streaming,omitempty"`

	// pushNotifications indicates whether the agent supports push notifications.
	// +optional
	PushNotifications bool `json:"pushNotifications,omitempty"`
}

// A2AAuthConfig configures authentication for A2A requests.
type A2AAuthConfig struct {
	// secretRef references a Secret containing a bearer token.
	// The secret should contain a key named "token".
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// A2AClientSpec configures an A2A client connection to another agent.
type A2AClientSpec struct {
	// name is a unique identifier for this client within the agent.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// agentRuntimeRef references another AgentRuntime in the cluster.
	// The controller resolves this to a service URL using the target's status.
	// Mutually exclusive with url.
	// +optional
	AgentRuntimeRef *AgentRuntimeClientRef `json:"agentRuntimeRef,omitempty"`

	// url is the direct URL of an external A2A agent endpoint.
	// Used instead of agentRuntimeRef for agents outside the cluster.
	// Mutually exclusive with agentRuntimeRef.
	// +optional
	URL string `json:"url,omitempty"`

	// exposeAsTools registers the remote agent's skills as local tools
	// via PromptKit's A2A Tool Bridge.
	// +optional
	ExposeAsTools bool `json:"exposeAsTools,omitempty"`

	// authentication configures credentials for outgoing calls to this agent.
	// +optional
	Authentication *A2AClientAuthConfig `json:"authentication,omitempty"`
}

// AgentRuntimeClientRef references another AgentRuntime resource.
type AgentRuntimeClientRef struct {
	// name is the AgentRuntime resource name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// namespace is the namespace of the target AgentRuntime.
	// Defaults to the same namespace as the referencing AgentRuntime.
	// +optional
	Namespace *string `json:"namespace,omitempty"`
}

// A2AClientAuthConfig configures authentication for outgoing A2A calls.
type A2AClientAuthConfig struct {
	// secretRef references a Secret containing a bearer token (key: "token").
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// A2AClientStatus reports the resolution state of an A2A client connection.
type A2AClientStatus struct {
	// name matches the client name from the spec.
	Name string `json:"name"`

	// resolvedURL is the resolved A2A endpoint URL.
	// +optional
	ResolvedURL string `json:"resolvedURL,omitempty"`

	// ready indicates whether the client was successfully resolved.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// error contains the resolution error message, if any.
	// +optional
	Error string `json:"error,omitempty"`
}

// A2AStatus holds A2A-specific status information.
type A2AStatus struct {
	// agentCardURL is the URL where the agent card is served.
	// +optional
	AgentCardURL string `json:"agentCardURL,omitempty"`

	// endpoint is the A2A JSON-RPC endpoint URL.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// clients reports the resolution status of each configured A2A client.
	// +optional
	Clients []A2AClientStatus `json:"clients,omitempty"`
}

// RuntimeConfig defines deployment-related settings.
type RuntimeConfig struct {
	// replicas is the desired number of agent runtime pods.
	// This field is ignored when autoscaling is enabled.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// autoscaling configures horizontal pod autoscaling for the agent.
	// +optional
	Autoscaling *AutoscalingConfig `json:"autoscaling,omitempty"`

	// resources defines compute resource requirements for the agent container.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// nodeSelector is a map of node labels for pod scheduling.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// tolerations are tolerations for pod scheduling.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// affinity defines affinity rules for pod scheduling.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// volumes defines additional volumes to mount in the runtime pod.
	// Use this to mount PVCs, ConfigMaps, or Secrets for media files or mock configurations.
	// +optional
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// volumeMounts defines additional volume mounts for the runtime container.
	// Each mount must reference a volume defined in the volumes field.
	// +optional
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`

	// extraEnv defines additional environment variables for the runtime container.
	// Use this for debugging (e.g., LOG_LEVEL=debug) or custom configuration.
	// +optional
	ExtraEnv []corev1.EnvVar `json:"extraEnv,omitempty"`
}

// EvalConfig configures realtime eval execution for this agent.
type EvalConfig struct {
	// enabled activates eval execution for this agent's sessions.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// sampling configures eval sampling rates to control cost.
	// +optional
	Sampling *EvalSampling `json:"sampling,omitempty"`

	// rateLimit configures eval execution rate limits.
	// +optional
	RateLimit *EvalRateLimit `json:"rateLimit,omitempty"`

	// sessionCompletion configures how session completion is detected
	// for on_session_complete evals.
	// +optional
	SessionCompletion *SessionCompletionConfig `json:"sessionCompletion,omitempty"`
}

// EvalSampling configures sampling rates for evals.
type EvalSampling struct {
	// defaultRate is the default sampling percentage (0-100) for all evals.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=100
	// +optional
	DefaultRate *int32 `json:"defaultRate,omitempty"`

	// extendedRate is the sampling percentage (0-100) for extended evals
	// (model-powered evaluations that call an external service).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=10
	// +optional
	ExtendedRate *int32 `json:"extendedRate,omitempty"`
}

// EvalRateLimit configures rate limits for eval execution.
type EvalRateLimit struct {
	// maxEvalsPerSecond is the maximum number of evals to execute per second.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=50
	// +optional
	MaxEvalsPerSecond *int32 `json:"maxEvalsPerSecond,omitempty"`

	// maxConcurrentJudgeCalls is the maximum concurrent LLM judge API calls.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=5
	// +optional
	MaxConcurrentJudgeCalls *int32 `json:"maxConcurrentJudgeCalls,omitempty"`
}

// SessionCompletionConfig configures session completion detection for evals.
type SessionCompletionConfig struct {
	// inactivityTimeout is the duration after the last message before a session
	// is considered complete. Uses Go duration format (e.g., "5m", "1h").
	// +kubebuilder:default="5m"
	// +optional
	InactivityTimeout *string `json:"inactivityTimeout,omitempty"`
}

// MemoryConfig defines the memory settings for an AgentRuntime.
type MemoryConfig struct {
	// Enabled controls whether cross-session memory is active.
	// Memory is disabled by default.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Purpose defines the platform-enforced purpose tag for memories.
	// Must be one of: personalisation, support_continuity, safety.
	// +kubebuilder:validation:Enum=personalisation;support_continuity;safety
	// +optional
	Purpose string `json:"purpose,omitempty"`

	// Extraction configures automatic memory extraction from conversations.
	// +optional
	Extraction *MemoryExtractionConfig `json:"extraction,omitempty"`

	// Retention configures memory TTL.
	// +optional
	Retention *MemoryRetentionConfig `json:"retention,omitempty"`

	// Retrieval configures memory retrieval behavior.
	// +optional
	Retrieval *MemoryRetrievalConfig `json:"retrieval,omitempty"`

	// Embedding configures the embedding provider for semantic memory retrieval.
	// +optional
	Embedding *MemoryEmbeddingConfig `json:"embedding,omitempty"`
}

// MemoryExtractionConfig controls how memories are extracted from conversations.
type MemoryExtractionConfig struct {
	// Enabled controls whether extraction runs post-conversation.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Model overrides the extraction model. Empty uses the conversation model.
	// +optional
	Model string `json:"model,omitempty"`
}

// MemoryRetentionConfig controls memory lifecycle.
type MemoryRetentionConfig struct {
	// DefaultTTL is the default time-to-live for memories (e.g., "720h" for 30 days).
	// +optional
	DefaultTTL string `json:"defaultTTL,omitempty"`

	// MaxTTL is the maximum allowed TTL (e.g., "8760h" for 365 days).
	// +optional
	MaxTTL string `json:"maxTTL,omitempty"`
}

// MemoryRetrievalConfig controls memory retrieval behavior.
type MemoryRetrievalConfig struct {
	// Strategy selects the retrieval mode.
	// +kubebuilder:validation:Enum=keyword;semantic;graph;composite
	// +optional
	Strategy string `json:"strategy,omitempty"`

	// Limit is the maximum number of memories injected per turn.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=50
	// +optional
	Limit *int32 `json:"limit,omitempty"`
}

// MemoryEmbeddingConfig configures the embedding provider for memory.
type MemoryEmbeddingConfig struct {
	// Provider specifies the embedding provider type (e.g., openai, gemini, voyageai).
	// +kubebuilder:validation:Enum=openai;gemini;voyageai
	// +optional
	Provider string `json:"provider,omitempty"`

	// Model overrides the default embedding model for the provider.
	// +optional
	Model string `json:"model,omitempty"`

	// SecretRef references the Kubernetes secret containing the API key.
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// AgentRuntimeSpec defines the desired state of AgentRuntime.
type AgentRuntimeSpec struct {
	// framework specifies which agent framework to use.
	// Supports PromptKit, LangChain, AutoGen, or a custom image.
	// If not specified, defaults to PromptKit.
	// +optional
	Framework *FrameworkConfig `json:"framework,omitempty"`

	// promptPackRef references the PromptPack containing agent prompts and configuration.
	// +kubebuilder:validation:Required
	PromptPackRef PromptPackRef `json:"promptPackRef"`

	// facade configures the client-facing connection interface.
	// +kubebuilder:validation:Required
	Facade FacadeConfig `json:"facade"`

	// toolRegistryRef optionally references a ToolRegistry for available tools.
	// +optional
	ToolRegistryRef *ToolRegistryRef `json:"toolRegistryRef,omitempty"`

	// session configures session management and storage.
	// +optional
	Session *SessionConfig `json:"session,omitempty"`

	// runtime configures deployment settings like replicas and resources.
	// +optional
	Runtime *RuntimeConfig `json:"runtime,omitempty"`

	// media configures media file resolution for mock provider responses.
	// +optional
	Media *MediaConfig `json:"media,omitempty"`

	// providers is a list of named provider references.
	// Each entry maps a logical name to a Provider CRD.
	// The "default" name is used as the primary provider for the runtime.
	// +optional
	// +listType=map
	// +listMapKey=name
	Providers []NamedProviderRef `json:"providers,omitempty"`

	// evals configures realtime eval execution for this agent's sessions.
	// +optional
	Evals *EvalConfig `json:"evals,omitempty"`

	// console configures the dashboard console UI settings.
	// Use this to customize allowed file attachment types and size limits.
	// +optional
	Console *ConsoleConfig `json:"console,omitempty"`

	// a2a configures the A2A (Agent-to-Agent) protocol.
	// When facade.type is "a2a", this is the primary protocol configuration.
	// When facade.type is "websocket" or "grpc", set a2a.enabled: true to add
	// A2A as an additional endpoint on a separate port (default 9999).
	// +optional
	A2A *A2AConfig `json:"a2a,omitempty"`

	// memory configures cross-session memory for this agent.
	// +optional
	Memory *MemoryConfig `json:"memory,omitempty"`

	// extraPodAnnotations defines additional annotations to add to the agent pods.
	// Use this for integrations like service meshes, logging agents, or monitoring tools.
	// +optional
	ExtraPodAnnotations map[string]string `json:"extraPodAnnotations,omitempty"`
}

// AgentRuntimePhase represents the current phase of the AgentRuntime.
// +kubebuilder:validation:Enum=Pending;Running;Failed
type AgentRuntimePhase string

const (
	// AgentRuntimePhasePending indicates the runtime is being set up.
	AgentRuntimePhasePending AgentRuntimePhase = "Pending"
	// AgentRuntimePhaseRunning indicates the runtime is operational.
	AgentRuntimePhaseRunning AgentRuntimePhase = "Running"
	// AgentRuntimePhaseFailed indicates the runtime has failed.
	AgentRuntimePhaseFailed AgentRuntimePhase = "Failed"
)

// ReplicaStatus tracks the number of replicas.
type ReplicaStatus struct {
	// desired is the desired number of replicas.
	Desired int32 `json:"desired"`

	// ready is the number of ready replicas.
	Ready int32 `json:"ready"`

	// available is the number of available replicas.
	Available int32 `json:"available"`
}

// AgentRuntimeStatus defines the observed state of AgentRuntime.
type AgentRuntimeStatus struct {
	// phase represents the current lifecycle phase of the AgentRuntime.
	// +optional
	Phase AgentRuntimePhase `json:"phase,omitempty"`

	// replicas tracks the replica counts for the deployment.
	// +optional
	Replicas *ReplicaStatus `json:"replicas,omitempty"`

	// activeVersion is the currently deployed PromptPack version.
	// +optional
	ActiveVersion *string `json:"activeVersion,omitempty"`

	// serviceEndpoint is the internal Kubernetes service endpoint for the agent facade.
	// Format: {name}.{namespace}.svc.cluster.local:{port}
	// This can be used by dashboard or other services to connect to the agent.
	// +optional
	ServiceEndpoint string `json:"serviceEndpoint,omitempty"`

	// a2a holds A2A-specific status information when facade.type is "a2a".
	// +optional
	A2A *A2AStatus `json:"a2a,omitempty"`

	// conditions represent the current state of the AgentRuntime resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.replicas.ready`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.activeVersion`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AgentRuntime is the Schema for the agentruntimes API.
// It defines a deployment of a PromptKit agent with its associated configuration.
type AgentRuntime struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AgentRuntime
	// +required
	Spec AgentRuntimeSpec `json:"spec"`

	// status defines the observed state of AgentRuntime
	// +optional
	Status AgentRuntimeStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AgentRuntimeList contains a list of AgentRuntime.
type AgentRuntimeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AgentRuntime `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentRuntime{}, &AgentRuntimeList{})
}
