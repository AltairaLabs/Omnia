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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentRuntimeMode defines how the AgentRuntime is invoked. Default is
// "agent" — a conversational, session-driven runtime fronted by the
// WebSocket facade. Set to "function" to expose the pack as a one-shot,
// structured-I/O HTTP endpoint (see Phase 1 of Functions, #1102 / #1103).
// "inference" is intentionally not used here so we can reuse that name
// later for a more generic Provider role. Function mode requires a 'rest'
// facade (optionally an 'mcp' facade) via CEL.
// +kubebuilder:validation:Enum=agent;function
type AgentRuntimeMode string

const (
	// AgentRuntimeModeAgent is the conversational runtime mode.
	AgentRuntimeModeAgent AgentRuntimeMode = "agent"
	// AgentRuntimeModeFunction is the one-shot, structured-I/O runtime mode.
	// Requires spec.inputSchema and spec.outputSchema; requires a "rest"
	// facade (optionally an "mcp" facade) via CEL.
	AgentRuntimeModeFunction AgentRuntimeMode = "function"
)

// PromptPackRef references a PromptPack to use for this agent runtime.
// +kubebuilder:validation:XValidation:rule="!(has(self.version) && has(self.track))",message="promptPackRef.version and promptPackRef.track are mutually exclusive"
type PromptPackRef struct {
	// name is the name of the PromptPack resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// version pins an exact PromptPack version. Mutually exclusive with track.
	// +optional
	Version *string `json:"version,omitempty"`

	// track follows a release channel instead of pinning a version:
	// "stable" selects the highest non-prerelease version; "prerelease" selects
	// the highest version overall. Mutually exclusive with version.
	// +kubebuilder:validation:Enum=stable;prerelease
	// +optional
	Track *string `json:"track,omitempty"`
}

// FacadeType defines the protocol a single facade speaks. An AgentRuntime
// composes one or more single-protocol facades via spec.facades.
// +kubebuilder:validation:Enum=websocket;a2a;rest;mcp;custom
type FacadeType string

const (
	// FacadeTypeWebSocket uses WebSocket for client connections (agent mode).
	FacadeTypeWebSocket FacadeType = "websocket"
	// FacadeTypeA2A uses the A2A JSON-RPC protocol for agent-to-agent communication (agent mode).
	FacadeTypeA2A FacadeType = "a2a"
	// FacadeTypeREST uses a one-shot HTTP/REST endpoint (POST /functions/{name}).
	// Only valid for mode=function, which serves structured request/response over
	// HTTP rather than a persistent client connection.
	FacadeTypeREST FacadeType = "rest"
	// FacadeTypeMCP serves the Model Context Protocol (Streamable HTTP) surface.
	// Only valid alongside a rest facade in mode=function.
	FacadeTypeMCP FacadeType = "mcp"
	// FacadeTypeCustom is a third-party / bring-your-own-container facade
	// surface. The operator does not know the protocol it speaks; it just runs
	// the supplied image as the facade container. Requires spec.facades[].image
	// (CEL-validated). Like websocket, it is a long-lived connection surface and
	// is only valid in mode=agent. The port, expose, managementPlane, extraEnv,
	// drainTimeout, and clientToolTimeout fields all apply unchanged.
	FacadeTypeCustom FacadeType = "custom"
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

	// drainTimeout is how long the facade keeps serving active realtime calls
	// after receiving SIGTERM (rollout/drain/scale-down) before cleanly tearing
	// down the remainder. Duration format (e.g. "30s", "2m"). New calls stop
	// immediately on drain regardless. Defaults to 30s when unset.
	// +optional
	DrainTimeout *string `json:"drainTimeout,omitempty"`

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

	// a2a configures the A2A protocol surface. Only meaningful on a
	// type=a2a facade. When the agent also has a websocket facade, A2A is a
	// secondary listener on a2a.port (default 9999); when a2a is the only
	// agent-mode facade it is the primary listener (default 8080). Carries
	// the A2A TTLs, task store, agent-card, and outbound clients.
	// +optional
	A2A *A2AConfig `json:"a2a,omitempty"`

	// mcp configures the MCP (Model Context Protocol) surface. Only
	// meaningful on a type=mcp facade (function mode). The pod serves
	// Streamable HTTP MCP on mcp.port (default 9998) alongside the function
	// rest facade.
	// +optional
	MCP *MCPConfig `json:"mcp,omitempty"`

	// managementPlane gates this facade's internal management-plane twin
	// listener. Default true: the operator allocates an internal port,
	// publishes it under status.managementEndpoints, and the facade serves a
	// management-plane-only auth chain there (the dashboard's "Try this agent"
	// and other in-cluster callers dial it). Set false for an external-only
	// facade — no internal listener, no *-mgmt Service port, no status entry;
	// the external listener is unaffected. Replaces the former agent-global
	// externalAuth.allowManagementPlane.
	// +optional
	ManagementPlane *bool `json:"managementPlane,omitempty"`

	// expose opts this agent into operator-provisioned external exposure.
	// Opt-in: an agent is never externally reachable unless this is set AND the
	// platform has a default-exposure Gateway configured (Helm
	// `defaultExposure`). When both hold, the operator creates a host-based
	// HTTPRoute targeting this agent's facade Service, surfaced via
	// status.facade.endpoints (#1553). Exposure does NOT add authentication —
	// spec.externalAuth is still the gate; an exposed agent with no externalAuth
	// validators is management-plane-only at the facade.
	// +optional
	Expose *FacadeExposeConfig `json:"expose,omitempty"`
}

// ManagementPlaneEnabled reports whether this facade serves an internal
// management-plane twin listener. A nil managementPlane field means the
// permissive default (true); only an explicit managementPlane:false opts out.
func (f *FacadeConfig) ManagementPlaneEnabled() bool {
	if f == nil || f.ManagementPlane == nil {
		return true
	}
	return *f.ManagementPlane
}

// FacadeExposeConfig opts an agent into operator-provisioned external exposure
// (#1553). See FacadeConfig.expose.
type FacadeExposeConfig struct {
	// enabled creates an external HTTPRoute for this agent. Requires the platform
	// to have a default-exposure Gateway configured (Helm `defaultExposure`);
	// when no Gateway is configured this is a no-op. Defaults to false.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// host overrides the generated hostname (`{name}.{namespace}.{baseDomain}`).
	// Set for a custom domain on an individual agent. Must be a hostname the
	// configured Gateway's listener accepts (e.g. covered by its TLS cert).
	// +optional
	Host string `json:"host,omitempty"`
}

// ToolRegistryRef references a ToolRegistry resource.
type ToolRegistryRef struct {
	// name is the name of the ToolRegistry resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// namespace is the namespace of the ToolRegistry resource.
	// If not specified, the same namespace as the AgentRuntime is used.
	//
	// Cross-namespace references are NOT supported: it must either be omitted
	// or equal the AgentRuntime's own namespace. The agent pod's Role is
	// namespace-scoped, so a foreign namespace is unreadable from the pod and
	// registry-scoped ToolPolicies would not match. An AgentRuntime declaring
	// one is rejected with ToolRegistryCrossNamespace.
	// +optional
	Namespace *string `json:"namespace,omitempty"`
}

// ContextStoreType defines the type of context store. The runtime context is
// the working LLM context (turns concatenated into each provider call); a
// durable backend lets a fresh pod resume it via sdk.Resume. Only fast/instant
// stores are supported here — NOT a relational DB. (Long-term conversation
// archival is a separate concern owned by session-api, not this field.)
// +kubebuilder:validation:Enum=memory;redis
type ContextStoreType string

const (
	// ContextStoreTypeMemory uses in-memory storage (ephemeral; lost on pod restart).
	ContextStoreTypeMemory ContextStoreType = "memory"
	// ContextStoreTypeRedis uses Redis for durable, cross-pod-resumable context storage.
	ContextStoreTypeRedis ContextStoreType = "redis"
)

// ContextConfig defines the configuration for the runtime context store.
// +kubebuilder:validation:XValidation:rule="self.type == 'memory' || has(self.storeRef)",message="spec.context.storeRef is required when context.type is 'redis'"
type ContextConfig struct {
	// type specifies the context store backend.
	// +kubebuilder:validation:Required
	// +kubebuilder:default="memory"
	Type ContextStoreType `json:"type"`

	// storeRef references a secret containing connection details for the context store.
	// Required for the redis store type (the secret must hold a "url" key).
	// +optional
	StoreRef *corev1.LocalObjectReference `json:"storeRef,omitempty"`

	// ttl is the time-to-live for context entries in duration format (e.g., "24h", "30m").
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

// ProviderType defines the provider vendor / wire protocol.
// Hyperscaler hosting (Bedrock/Vertex/Azure) is expressed via spec.platform
// on the Provider CRD, not as a provider type. The provider type describes
// the wire protocol the runtime uses; the role (spec.role) is orthogonal.
// +kubebuilder:validation:Enum=claude;openai;gemini;ollama;mock;vllm;voyageai;cartesia;elevenlabs;imagen;huggingface
type ProviderType string

const (
	// ProviderTypeClaude uses Anthropic's Claude message format.
	// Can run direct-to-Anthropic or hosted on AWS Bedrock via spec.platform.
	ProviderTypeClaude ProviderType = "claude"
	// ProviderTypeOpenAI uses OpenAI's chat completions format.
	// Can run direct-to-OpenAI or hosted on Azure AI Foundry via spec.platform.
	ProviderTypeOpenAI ProviderType = "openai"
	// ProviderTypeGemini uses Google's Gemini format.
	// Can run direct-to-Google or hosted on GCP Vertex AI via spec.platform.
	ProviderTypeGemini ProviderType = "gemini"
	// ProviderTypeOllama uses locally-hosted Ollama models.
	// Does not require secretRef. Requires baseURL to be set.
	ProviderTypeOllama ProviderType = "ollama"
	// ProviderTypeMock uses PromptKit's mock provider for testing.
	// Does not require secretRef. Returns canned responses based on scenario.
	// LLM-role only; no mock factory is registered for embedding/tts/stt/image.
	ProviderTypeMock ProviderType = "mock"
	// ProviderTypeVLLM uses a vLLM-served OpenAI-compatible endpoint.
	// Requires baseURL. Auth is typically via custom headers (spec.headers).
	ProviderTypeVLLM ProviderType = "vllm"
	// ProviderTypeVoyageAI uses Voyage AI embedding models. Embedding-role only.
	// Requires an API key via secretRef (VOYAGE_API_KEY).
	ProviderTypeVoyageAI ProviderType = "voyageai"
	// ProviderTypeCartesia uses Cartesia TTS. TTS-role only.
	ProviderTypeCartesia ProviderType = "cartesia"
	// ProviderTypeElevenLabs uses ElevenLabs TTS. TTS-role only.
	ProviderTypeElevenLabs ProviderType = "elevenlabs"
	// ProviderTypeImagen uses Google's Imagen image-generation model. Image-role only.
	ProviderTypeImagen ProviderType = "imagen"
	// ProviderTypeHuggingFace uses HuggingFace Inference Endpoints/API. Inference-role only.
	ProviderTypeHuggingFace ProviderType = "huggingface"
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

	// requestTimeout caps the wall-clock duration of non-streaming provider
	// HTTP calls (Predict, embeddings). Does not apply to streaming calls.
	// Go duration string, e.g. "2m", "90s". Defaults to the provider's
	// built-in default (typically 60s) when unset.
	// +optional
	RequestTimeout string `json:"requestTimeout,omitempty"`

	// streamIdleTimeout bounds how long an SSE streaming body may remain
	// silent (no bytes) between reads before the stream is aborted. The
	// timer resets on every byte received, so legitimately long-running
	// streams are not affected. Useful for slow local models (e.g. Ollama
	// CPU inference) where first-token latency can exceed the default 30s.
	// Go duration string, e.g. "120s", "2m". Defaults to 30s when unset.
	// +optional
	StreamIdleTimeout string `json:"streamIdleTimeout,omitempty"`
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

	// role declares the role the AgentRuntime expects the referenced
	// Provider to fulfil. The controller asserts at reconcile time that
	// the referenced Provider's `spec.role` matches; mismatch puts the
	// AgentRuntime in Phase=Error with ProvidersReady=False.
	//
	// Defaults to 'llm' for back-compat with existing AgentRuntimes
	// (which were authored before per-ref roles existed).
	// +optional
	// +kubebuilder:default=llm
	Role ProviderRole `json:"role,omitempty"`

	// requiredCapabilities lists capabilities the provider must support for
	// this binding. If the provider does not advertise all listed capabilities,
	// the AgentRuntime enters a Pending phase with a descriptive condition.
	// +optional
	RequiredCapabilities []ProviderCapability `json:"requiredCapabilities,omitempty"`
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

// FrameworkType defines which agent framework to use.
// +kubebuilder:validation:Enum=promptkit;langchain;custom
type FrameworkType string

const (
	// FrameworkTypePromptKit uses AltairaLabs' PromptKit framework. This is
	// the only framework built, tested, and released by this repo.
	FrameworkTypePromptKit FrameworkType = "promptkit"
	// FrameworkTypeLangChain uses a LangChain runtime image. UNSUPPORTED:
	// no image is built by this repo and none is defaulted, so
	// spec.framework.image (or an operator --framework-image entry) is
	// required. Not built, tested, or released by this repo — such an
	// image implements omnia.runtime.v1 independently and may lag the
	// contract.
	FrameworkTypeLangChain FrameworkType = "langchain"
	// FrameworkTypeCustom uses a user-provided container image implementing
	// the omnia.runtime.v1 gRPC contract. Requires spec.framework.image.
	FrameworkTypeCustom FrameworkType = "custom"
)

// FrameworkConfig specifies which agent framework to use. PromptKit is the only
// framework this repo builds, tests, and releases; any other type requires a
// bring-your-own image that implements the omnia.runtime.v1 gRPC contract
// (verifiable with the runtime-conformance suite).
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

	// storage configures the media-storage backend the operator provisions
	// into the facade (uploads) and runtime (attachment-reference resolution)
	// containers. When unset, media storage is disabled.
	// +optional
	Storage *MediaStorageConfig `json:"storage,omitempty"`
}

// MediaStorageConfig selects and configures the media-storage backend.
// +kubebuilder:validation:XValidation:rule="self.type != 's3' || has(self.s3)",message="type s3 requires spec.media.storage.s3"
// +kubebuilder:validation:XValidation:rule="self.type != 'gcs' || has(self.gcs)",message="type gcs requires spec.media.storage.gcs"
// +kubebuilder:validation:XValidation:rule="self.type != 'azure' || has(self.azure)",message="type azure requires spec.media.storage.azure"
// +kubebuilder:validation:XValidation:rule="self.type != 'local' || has(self.local)",message="type local requires spec.media.storage.local"
type MediaStorageConfig struct {
	// type selects the storage backend.
	// +kubebuilder:validation:Enum=none;local;s3;gcs;azure
	// +kubebuilder:default=none
	Type string `json:"type"`

	// +optional
	Local *LocalMediaBackend `json:"local,omitempty"`
	// +optional
	S3 *S3MediaBackend `json:"s3,omitempty"`
	// +optional
	GCS *GCSMediaBackend `json:"gcs,omitempty"`
	// +optional
	Azure *AzureMediaBackend `json:"azure,omitempty"`

	// defaultTTL is the retention TTL for stored media (Go duration).
	// +optional
	DefaultTTL *metav1.Duration `json:"defaultTTL,omitempty"`
	// uploadURLTTL bounds presigned upload URLs.
	// +optional
	UploadURLTTL *metav1.Duration `json:"uploadURLTTL,omitempty"`
	// downloadURLTTL bounds presigned download URLs.
	// +optional
	DownloadURLTTL *metav1.Duration `json:"downloadURLTTL,omitempty"`
	// maxFileSizeBytes caps a single stored object.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxFileSizeBytes *int64 `json:"maxFileSizeBytes,omitempty"`

	// secretRef supplies explicit credentials (S3 AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY,
	// Azure account key). Omit for keyless workload-identity access. GCS uses
	// workload identity only.
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// LocalMediaBackend stores media on a facade-served local path.
type LocalMediaBackend struct {
	// basePath is the on-disk directory the facade serves media from and the
	// runtime resolves references against. When volumeClaim is set it is the
	// mount path for that PVC.
	// +kubebuilder:validation:MinLength=1
	BasePath string `json:"basePath"`

	// volumeClaim names an existing PersistentVolumeClaim to mount at basePath
	// in both the facade and runtime containers. Use a ReadWriteMany PVC so
	// uploads written by the facade are durable and readable by the runtime
	// across pods. When empty, no volume is provisioned — the local backend
	// then requires a writable basePath (the agent containers have a read-only
	// root filesystem), so an explicit volumeClaim (or podOverrides volume) is
	// needed for local storage to function.
	// +optional
	VolumeClaim string `json:"volumeClaim,omitempty"`
}

// S3MediaBackend configures S3 / S3-compatible (MinIO) storage.
type S3MediaBackend struct {
	// +kubebuilder:validation:MinLength=1
	Bucket string `json:"bucket"`
	// +optional
	Region string `json:"region,omitempty"`
	// +optional
	Prefix string `json:"prefix,omitempty"`
	// endpoint, when set (e.g. MinIO), forces path-style addressing.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
}

// GCSMediaBackend configures Google Cloud Storage (workload identity / ADC).
type GCSMediaBackend struct {
	// +kubebuilder:validation:MinLength=1
	Bucket string `json:"bucket"`
	// +optional
	Prefix string `json:"prefix,omitempty"`
}

// AzureMediaBackend configures Azure Blob Storage.
type AzureMediaBackend struct {
	// +kubebuilder:validation:MinLength=1
	Account string `json:"account"`
	// +kubebuilder:validation:MinLength=1
	Container string `json:"container"`
	// +optional
	Prefix string `json:"prefix,omitempty"`
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

	// channels is the audio channel count the client should capture/play (1=mono).
	Channels *int32 `json:"channels,omitempty"`

	// format is the PCM sample format the client should send, e.g. "pcm16".
	Format string `json:"format,omitempty"`
}

// DuplexConfig declares that an agent supports realtime bidirectional
// (duplex) media. The dashboard renders the voice "call" console instead of
// the text composer when Enabled is true. Mode reserves video for the future.
type DuplexConfig struct {
	// enabled turns on the realtime voice console for this agent.
	Enabled bool `json:"enabled,omitempty"`

	// mode is the duplex modality. "audio" (default) streams voice only;
	// "audiovideo" additionally streams the browser camera (not yet implemented).
	// +kubebuilder:validation:Enum=audio;audiovideo
	// +kubebuilder:default=audio
	Mode string `json:"mode,omitempty"`

	// audio declares the realtime audio format the runtime requires for this
	// agent's duplex sessions (recommendedSampleRate / channels / format). The
	// runtime advertises it as a bounded counter-offer in RuntimeHello; the
	// facade relays it to the client, which captures at that format, or the
	// session fails at open when the client cannot satisfy it. Unset means the
	// runtime accepts the client's proposed format.
	// +optional
	Audio *AudioRequirements `json:"audio,omitempty"`
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

	// taskStore configures the task persistence backend.
	// Defaults to in-memory. Set type to "redis" for persistence across restarts.
	// +optional
	TaskStore *A2ATaskStoreConfig `json:"taskStore,omitempty"`

	// clients configures connections to other A2A agents.
	// Each client can reference an in-cluster AgentRuntime or an external URL.
	// +optional
	Clients []A2AClientSpec `json:"clients,omitempty"`
}

// MCPConfig configures the optional MCP server facade for function-mode
// AgentRuntimes. Enabling it adds a Streamable HTTP MCP listener on
// port (default 9998) alongside the existing HTTP function route.
//
// Distinct from MCPClientConfig (in toolregistry_types.go), which
// configures Omnia as an MCP *client* connecting to an external MCP
// server as a tool source. MCPConfig is the server side; MCPClientConfig
// is the client side.
type MCPConfig struct {
	// enabled turns the MCP server on. Default false.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// port is the listen port for the MCP server. Default 9998.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port *int32 `json:"port,omitempty"`
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

	// inline configures the evals that run synchronously inside the runtime
	// (Pattern C). Lightweight evals (contains, regex, deterministic
	// scorers) belong here. Defaults to groups=["fast-running"] when unset —
	// disjoint from the worker default so no eval runs on both paths.
	// +optional
	Inline *EvalPathConfig `json:"inline,omitempty"`

	// worker configures the evals that run out-of-band in the eval-worker
	// (Pattern A). LLM-judge evals and other long-running or external
	// checks belong here. Defaults to groups=["long-running","external"]
	// when unset.
	// +optional
	Worker *EvalPathConfig `json:"worker,omitempty"`

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

	// podOverrides customizes the namespace-level eval-worker Pod. Last
	// AgentRuntime to reconcile wins (eval-worker is per-namespace, not
	// per-CRD).
	// +optional
	PodOverrides *PodOverrides `json:"podOverrides,omitempty"`
}

// EvalPathConfig configures one execution path (inline runtime or worker)
// for eval execution.
//
// Groups filters evals by their declared or auto-classified group names.
// PromptKit auto-classifies handlers into "fast-running", "long-running",
// and "external"; every eval also carries the "default" group. Authors
// may add custom groups via EvalDef.Groups in the pack.
//
// An eval runs on a path when at least one of its groups is in the path's
// Groups list. An absent or empty Groups uses the built-in default for
// the path. To disable evals entirely for an agent, set
// EvalConfig.Enabled=false rather than using an empty list.
type EvalPathConfig struct {
	// groups is the set of eval group names this path executes. An
	// absent or empty list uses the built-in default for the path
	// (see EvalConfig.Inline and EvalConfig.Worker).
	// +optional
	Groups []string `json:"groups,omitempty"`
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

	// Retrieval configures memory retrieval behavior.
	// +optional
	Retrieval *MemoryRetrievalConfig `json:"retrieval,omitempty"`

	// Tools configures the memory tools (memory__remember / memory__recall)
	// exposed to the LLM. Independent of retrieval: an agent can have ambient
	// RAG without the tools, or the tools without RAG.
	// +optional
	Tools *MemoryToolsConfig `json:"tools,omitempty"`
}

// MemoryRetrievalConfig controls memory retrieval behavior.
type MemoryRetrievalConfig struct {
	// Enabled controls whether ambient RAG retrieval runs — the per-turn
	// auto-injection of relevant memories into the prompt. Defaults to true
	// (when unset) so memory.enabled keeps its existing behavior. Set to false
	// to keep the memory tools without auto-injecting memories every turn.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Strategy selects the retrieval mode.
	// +kubebuilder:validation:Enum=keyword;semantic;composite
	// +optional
	Strategy string `json:"strategy,omitempty"`

	// Limit is the maximum number of memories injected per turn.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=50
	// +optional
	Limit *int32 `json:"limit,omitempty"`

	// AccessFilter configures retrieval-time access control (a deny filter).
	// +optional
	AccessFilter *MemoryAccessFilterConfig `json:"accessFilter,omitempty"`
}

// MemoryAccessFilterConfig configures a retrieval-time deny filter evaluated
// per retrieved memory item's metadata. It is the governance seam: an indexed
// but restricted document's items can be dropped from retrieval.
type MemoryAccessFilterConfig struct {
	// DenyCEL is a CEL expression over `metadata` (a map<string, dyn> of the
	// retrieved item's metadata). Items for which it evaluates to true are
	// dropped. Empty disables filtering. Example:
	// metadata.url.contains("restricted")
	// +optional
	DenyCEL string `json:"denyCEL,omitempty"`
}

// MemoryToolsConfig controls the memory tools exposed to the LLM.
type MemoryToolsConfig struct {
	// Enabled controls whether the memory tools (memory__remember /
	// memory__recall) are active. Defaults to true (when unset) so
	// memory.enabled keeps its existing behavior. Set to false for read-only
	// ambient RAG over a curated store without letting the agent write or
	// explicitly recall.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// AgentRuntimeSpec defines the desired state of AgentRuntime.
//
// Mode-conditional validations (Functions, #1102):
// +kubebuilder:validation:XValidation:rule="self.mode != 'function' || has(self.inputSchema)",message="spec.inputSchema is required when spec.mode is 'function'"
// +kubebuilder:validation:XValidation:rule="self.mode != 'function' || has(self.outputSchema)",message="spec.outputSchema is required when spec.mode is 'function'"
// +kubebuilder:validation:XValidation:rule="self.mode == 'function' || !has(self.inputSchema)",message="spec.inputSchema is only valid when spec.mode is 'function'"
// +kubebuilder:validation:XValidation:rule="self.mode == 'function' || !has(self.outputSchema)",message="spec.outputSchema is only valid when spec.mode is 'function'"
// +kubebuilder:validation:XValidation:rule="self.mode == 'function' || !has(self.outputFormat)",message="spec.outputFormat is only valid when spec.mode is 'function'"
// Facade composition validations (#1576). Each rule is guarded by
// has(self.facades) so a CR without spec.facades short-circuits to valid (#1815):
// MinItems=1 + Required already reject absent facades on create/update, but the
// unguarded rules erred with "no such key: facades" on any legacy CR lacking the
// field, which wedged the operator's finalizer-removal update and left the CR
// stuck Terminating. The guard lets deletion proceed; create-time validation is
// unaffected because the field is Required there.
// +kubebuilder:validation:XValidation:rule="!has(self.facades) || self.facades.all(f, self.facades.exists_one(g, g.type == f.type))",message="spec.facades must not contain duplicate facade types"
// +kubebuilder:validation:XValidation:rule="!has(self.facades) || self.mode != 'agent' || self.facades.all(f, f.type == 'websocket' || f.type == 'a2a' || f.type == 'custom')",message="mode 'agent' allows only 'websocket', 'a2a' and 'custom' facades"
// +kubebuilder:validation:XValidation:rule="!has(self.facades) || self.mode != 'function' || self.facades.all(f, f.type == 'rest' || f.type == 'mcp')",message="mode 'function' allows only 'rest' and 'mcp' facades"
// +kubebuilder:validation:XValidation:rule="!has(self.facades) || self.mode != 'function' || self.facades.exists_one(f, f.type == 'rest')",message="mode 'function' requires exactly one 'rest' facade"
// +kubebuilder:validation:XValidation:rule="!has(self.facades) || self.facades.all(f, f.type != 'custom' || (has(f.image) && size(f.image) > 0))",message="facade type 'custom' requires spec.facades[].image"
// Version-triggered rollout (#1838): rollout.trigger canaries new PromptPack
// versions and requires a version-pinned promptPackRef; it is mutually exclusive
// with promptPackRef.track (which auto-updates the stable pods instead).
// +kubebuilder:validation:XValidation:rule="!(has(self.rollout) && has(self.rollout.trigger)) || (has(self.promptPackRef) && has(self.promptPackRef.version) && !has(self.promptPackRef.track))",message="spec.rollout.trigger requires a version-pinned spec.promptPackRef and is mutually exclusive with promptPackRef.track"
type AgentRuntimeSpec struct {
	// mode controls how the AgentRuntime is invoked. "agent" (default) is
	// the existing conversational runtime (websocket and/or a2a facades);
	// "function" exposes the pack as a one-shot, structured-I/O HTTP endpoint
	// at POST /functions/{name} (a rest facade, optionally with an mcp
	// facade). When set to "function", spec.inputSchema and spec.outputSchema
	// are required.
	// +optional
	// +kubebuilder:default="agent"
	Mode AgentRuntimeMode `json:"mode,omitempty"`

	// inputSchema is the JSON Schema that incoming Function payloads are
	// validated against. Required when spec.mode is 'function'; forbidden
	// otherwise (CEL-gated). Stored as a raw JSON object; consumers
	// validate via santhosh-tekuri/jsonschema.
	// +optional
	// +kubebuilder:validation:Type=object
	// +kubebuilder:pruning:PreserveUnknownFields
	InputSchema *apiextensionsv1.JSON `json:"inputSchema,omitempty"`

	// outputSchema is the JSON Schema that the Function's response is
	// validated against before being returned to the caller. A
	// non-conforming model output is rejected with HTTP 502 and the raw
	// output is returned in the response body for debugging (no
	// in-runtime retry). Required when spec.mode is 'function'; forbidden
	// otherwise (CEL-gated).
	// +optional
	// +kubebuilder:validation:Type=object
	// +kubebuilder:pruning:PreserveUnknownFields
	OutputSchema *apiextensionsv1.JSON `json:"outputSchema,omitempty"`

	// outputFormat controls how the model is asked to format its response in
	// function mode. "text" = free-form (validated post-hoc by the facade),
	// "json" = provider JSON mode (valid JSON, shape unenforced), "json_schema"
	// = provider structured output bound to outputSchema. When unset on a
	// function-mode runtime it defaults to "json_schema". Forbidden when
	// spec.mode is not 'function' (CEL-gated).
	// +optional
	// +kubebuilder:validation:Enum=text;json;json_schema
	OutputFormat string `json:"outputFormat,omitempty"`

	// framework specifies which agent framework to use.
	// Defaults to PromptKit, the only framework this repo builds and tests.
	// Any other type (langchain, custom) requires an explicit image
	// via spec.framework.image or an operator --framework-image entry, and
	// must implement the omnia.runtime.v1 gRPC contract.
	// +optional
	Framework *FrameworkConfig `json:"framework,omitempty"`

	// promptPackRef references the PromptPack containing agent prompts and configuration.
	// +kubebuilder:validation:Required
	PromptPackRef PromptPackRef `json:"promptPackRef"`

	// facades composes one or more single-protocol facades on top of the
	// shared agent substrate. Each entry is one protocol surface (websocket,
	// a2a, rest, or mcp); all run co-resident in the same pod. Agent mode uses
	// websocket and/or a2a; function mode uses rest (required) and optionally
	// mcp. Must be non-empty with no duplicate types (CEL-validated).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=4
	// +listType=atomic
	Facades []FacadeConfig `json:"facades"`

	// toolRegistryRef optionally references a ToolRegistry for available tools.
	// +optional
	ToolRegistryRef *ToolRegistryRef `json:"toolRegistryRef,omitempty"`

	// context configures the runtime context store.
	// +optional
	Context *ContextConfig `json:"context,omitempty"`

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

	// duplex configures the realtime voice/duplex console for this agent.
	// +optional
	Duplex *DuplexConfig `json:"duplex,omitempty"`

	// externalAuth configures authentication for data-plane traffic to
	// this agent's facades (external apps streaming via WebSocket or A2A).
	// When unset, the agent is reachable only from the management plane
	// (the dashboard's debug view) — no customer traffic until at least
	// one validator is filled in. Applied to each facade's external auth
	// chain.
	// +optional
	ExternalAuth *AgentExternalAuth `json:"externalAuth,omitempty"`

	// memory configures cross-session memory for this agent.
	// +optional
	Memory *MemoryConfig `json:"memory,omitempty"`

	// extraPodAnnotations defines additional annotations to add to the agent pods.
	// Use this for integrations like service meshes, logging agents, or monitoring tools.
	// +optional
	ExtraPodAnnotations map[string]string `json:"extraPodAnnotations,omitempty"`

	// serviceGroup references a service group defined in the parent Workspace's spec.services[].name.
	// The controller resolves the session-api and memory-api endpoints from that group.
	// Defaults to "default".
	// +kubebuilder:default="default"
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
	// +kubebuilder:validation:MaxLength=63
	// +optional
	ServiceGroup string `json:"serviceGroup,omitempty"`

	// privacyPolicyRef references a SessionPrivacyPolicy in this agent's namespace
	// that overrides the service group's policy (if any) for this specific agent.
	// +optional
	PrivacyPolicyRef *corev1.LocalObjectReference `json:"privacyPolicyRef,omitempty"`

	// rollout configures a progressive delivery rollout for this AgentRuntime.
	// When nil, no rollout is active and all traffic goes to the current spec.
	// +optional
	Rollout *RolloutConfig `json:"rollout,omitempty"`

	// podOverrides lets operators customize the facade+runtime pod's
	// scheduling, service account, labels, annotations, volumes, and env
	// sourced from CSI secret-stores or ConfigMaps.
	//
	// Pod-level fields apply to the pod. Container-level fields
	// (extraEnv, extraEnvFrom, extraVolumeMounts) apply to both the facade
	// and runtime containers but NOT to operator-injected sidecars
	// (e.g. policy-broker). Per-container env overrides remain available
	// via spec.facades[].extraEnv and spec.runtime.extraEnv.
	// +optional
	PodOverrides *PodOverrides `json:"podOverrides,omitempty"`
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

	// managementEndpoints reports the internal (management-plane) listener ports
	// the facades serve, one entry per surface whose facade has
	// managementPlane enabled (the default). A surface's field is nil when no
	// such facade exists or its managementPlane is false. The dashboard and
	// in-cluster callers read these to dial the agent over the management plane
	// — they never compute the port from the external port.
	// +optional
	ManagementEndpoints *ManagementEndpoints `json:"managementEndpoints,omitempty"`

	// a2a holds A2A-specific status information when an a2a facade is present.
	// +optional
	A2A *A2AStatus `json:"a2a,omitempty"`

	// facade reports externally-reachable endpoints derived from observed
	// Gateway API HTTPRoutes. Empty => the agent is reachable only in-cluster.
	// +optional
	Facade *FacadeStatus `json:"facade,omitempty"`

	// rollout reports the current state of an active rollout, if any.
	// +optional
	Rollout *RolloutStatus `json:"rollout,omitempty"`

	// conditions represent the current state of the AgentRuntime resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// runtimeCapabilities are the contract features the running runtime
	// advertises, self-reported at startup. An open set — the control plane
	// displays whatever is reported. Empty means the runtime predates capability
	// advertisement (legacy) or has not reported yet.
	// +optional
	RuntimeCapabilities []string `json:"runtimeCapabilities,omitempty"`

	// observedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// ManagementEndpoints holds the internal management-plane listener ports the
// facades serve (in-cluster only) for each facade with managementPlane
// enabled. A surface's field is nil when that surface is absent or its
// managementPlane is false. Callers read these to reach the agent over the
// management plane.
type ManagementEndpoints struct {
	// ws is the internal WebSocket management-plane port.
	// +optional
	WS *int32 `json:"ws,omitempty"`

	// a2a is the internal A2A management-plane port (dual-protocol agents).
	// +optional
	A2A *int32 `json:"a2a,omitempty"`

	// mcp is the internal MCP management-plane port (function-mode agents).
	// +optional
	MCP *int32 `json:"mcp,omitempty"`
}

// Facade protocol values for FacadeEndpoint.Protocol.
const (
	FacadeProtocolWebSocket = "websocket"
	FacadeProtocolA2A       = "a2a"
	FacadeProtocolMCP       = "mcp"
	FacadeProtocolREST      = "rest"
)

// FacadeEndpointReasonPrefixNotStripped is set on Valid=false endpoints whose
// route uses a non-root PathPrefix without a URLRewrite ReplacePrefixMatch filter.
const FacadeEndpointReasonPrefixNotStripped = "path prefix not stripped before facade"

// FacadeStatus holds externally-reachable endpoints derived from observed
// Gateway API HTTPRoutes. An empty Endpoints list means the agent is reachable
// only inside the cluster.
type FacadeStatus struct {
	// endpoints are the external URLs derived from HTTPRoutes that target this
	// agent's facade Service. Empty => cluster-internal only.
	// +optional
	Endpoints []FacadeEndpoint `json:"endpoints,omitempty"`
}

// FacadeEndpoint is one externally-reachable URL for the agent's facade,
// derived from an observed HTTPRoute. Auth is NOT included here; it is read
// from spec.externalAuth (agent-global) by consumers.
type FacadeEndpoint struct {
	// protocol is the facade protocol this endpoint serves.
	// +kubebuilder:validation:Enum=websocket;a2a;mcp;rest
	Protocol string `json:"protocol"`
	// url is the client-facing connection URL, e.g. wss://agents.example.com/my-agent/ws
	URL string `json:"url"`
	// scheme is the URL scheme: ws, wss, http, or https.
	Scheme string `json:"scheme"`
	// host is the route hostname.
	Host string `json:"host"`
	// path is the external path including the protocol's canonical suffix.
	Path string `json:"path"`
	// port is the Service backend port the route targets.
	Port int32 `json:"port"`
	// routeName is the name of the HTTPRoute this endpoint was derived from.
	RouteName string `json:"routeName"`
	// routeNamespace is the namespace of that HTTPRoute.
	RouteNamespace string `json:"routeNamespace"`
	// valid is false when the endpoint is advertised but will not actually
	// connect (e.g. a path prefix that is not stripped before the facade).
	Valid bool `json:"valid"`
	// reason explains why valid is false.
	// +optional
	Reason string `json:"reason,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=agent;ar
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

// EffectiveMode returns the runtime's declared mode, defaulting to
// AgentRuntimeModeAgent when unset for back-compat with pre-mode
// AgentRuntimes. Safe to call on a nil receiver (returns agent).
func (ar *AgentRuntime) EffectiveMode() AgentRuntimeMode {
	if ar == nil || ar.Spec.Mode == "" {
		return AgentRuntimeModeAgent
	}
	return ar.Spec.Mode
}

// IsFunctionMode is shorthand for EffectiveMode() == AgentRuntimeModeFunction.
func (ar *AgentRuntime) IsFunctionMode() bool {
	return ar.EffectiveMode() == AgentRuntimeModeFunction
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
