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

package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/AltairaLabs/PromptKit/runtime/evals"
	pkmetrics "github.com/AltairaLabs/PromptKit/runtime/metrics"
	"github.com/AltairaLabs/PromptKit/runtime/statestore"

	// Register all providers via blank imports
	// TODO: PromptKit should provide a "providers/all" package for convenience
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/claude"
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/gemini"
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/ollama"
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/openai"
	"github.com/AltairaLabs/PromptKit/sdk"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
	pkskills "github.com/AltairaLabs/PromptKit/runtime/skills"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/media"
	"github.com/altairalabs/omnia/internal/runtime/skills"
	"github.com/altairalabs/omnia/internal/runtime/tools"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/runtime/contract"
)

// Server implements the RuntimeService gRPC server.
// It wraps the PromptKit SDK to handle LLM conversations.
type Server struct {
	runtimev1.UnimplementedRuntimeServiceServer

	log               logr.Logger
	sdkLogger         *slog.Logger
	packPath          string
	agentName         string
	agentUID          string
	namespace         string
	promptPackName    string
	promptPackVersion string
	promptName        string
	stateStore        statestore.Store
	mockProvider      bool
	mockConfigPath    string
	mode              string // AgentRuntime spec.mode; gates function response-format (#1483)
	outputFormat      string // spec.outputFormat
	outputSchemaJSON  []byte // spec.outputSchema bytes for json_schema mode
	sdkOptions        []sdk.Option
	conversations     map[string]*sdk.Conversation
	turnIndices       map[string]int      // Track turn count per session
	unsubscribeFns    map[string][]func() // Event bus unsubscribe functions per session
	conversationMu    sync.RWMutex
	healthy           bool
	mu                sync.RWMutex

	// Tool management
	toolExecutor     *tools.OmniaExecutor
	toolsConfigPath  string
	toolsInitialized bool

	// Tracing
	tracingProvider *tracing.Provider

	// Evals
	evalCollector    *pkmetrics.Collector
	evalDefs         []evals.EvalDef
	inlineEvalGroups []string // nil/empty => DefaultInlineEvalGroups

	// Provider info (for logging and provider creation)
	providerType              string
	providerAPIKey            string             // Resolved default-provider API key, carried on the spec (§5.3.1)
	providerRefName           string             // Provider CRD name (for per-provider attribution)
	extraProviders            []ResolvedProvider // Non-default providers (embedding/tts/stt/image/inference)
	model                     string
	baseURL                   string            // Custom base URL for provider (e.g., Ollama endpoint)
	headers                   map[string]string // Custom HTTP headers for every provider request
	inputCostPer1K            float64           // CRD pricing: cost per 1K input tokens
	outputCostPer1K           float64           // CRD pricing: cost per 1K output tokens
	providerRequestTimeout    time.Duration     // Non-streaming HTTP timeout (0 = provider default)
	providerStreamIdleTimeout time.Duration     // SSE stream idle timeout (0 = 30s default)

	// Platform hosting configuration (empty platformType = direct provider access)
	platformType     string // "bedrock", "vertex", or "azure"
	platformRegion   string
	platformProject  string
	platformEndpoint string

	// Auth configuration for platform-hosted providers.
	authType                  string // "workloadIdentity", "accessKey", "serviceAccount", "servicePrincipal"
	authRoleArn               string
	authServiceAccountEmail   string
	authCredentialsSecretName string
	authCredentialsSecretKey  string
	authCredentialsNamespace  string // namespace in which to read the credentials secret

	// Session recording (Pattern C)
	sessionStore session.Store

	// Memory store for cross-session memory (via memory-api HTTP client)
	memoryStore    pkmemory.Store
	workspaceUID   string // Workspace CRD UID for memory scope
	memoryStrategy string // Retrieval strategy: "semantic" or "" (keyword FTS)
	memoryDenyCEL  string // Access deny-filter CEL expression (empty = no filter)
	memoryLimit    int    // Max memories injected per turn; 0 = defaultEpisodicLimit (10)
	// memoryRetrievalEnabled gates ambient RAG auto-injection; memoryToolsEnabled
	// gates the memory__remember/recall tools. Both default true (see NewServer)
	// so a wired memory store keeps its historical both-on behavior unless a
	// CRD sub-toggle explicitly disables one.
	memoryRetrievalEnabled bool
	memoryToolsEnabled     bool

	// Media resolution for mock provider
	mediaResolver *MediaResolver

	// mediaStorage is Omnia's media storage backend, injected into the
	// PromptKit SDK (via sdk.WithMediaStorage in WithMediaStorage below) so
	// storage_ref attachments resolve to a model-fetchable URL or bytes at
	// provider-call time, every turn (#1817).
	mediaStorage media.Storage
}

// ServerOption configures the server.
type ServerOption func(*Server)

// WithLogger sets the logr.Logger for the server's own structured logging.
// To also set the slog.Logger passed to the PromptKit SDK, use WithSlogLogger.
func WithLogger(log logr.Logger) ServerOption {
	return func(s *Server) {
		s.log = log
	}
}

// WithSlogLogger sets the *slog.Logger passed to the PromptKit SDK.
// Create this via logging.SlogFromZap so it writes directly to the Zap core,
// producing output identical to the logr logger set via WithLogger.
func WithSlogLogger(l *slog.Logger) ServerOption {
	return func(s *Server) {
		s.sdkLogger = l
	}
}

// WithPackPath sets the path to the PromptPack file.
func WithPackPath(path string) ServerOption {
	return func(s *Server) {
		s.packPath = path
	}
}

// WithPromptName sets the prompt name to use from the pack.
func WithPromptName(name string) ServerOption {
	return func(s *Server) {
		s.promptName = name
	}
}

// WithAgentIdentity sets the agent name and namespace for eval result enrichment.
func WithAgentIdentity(agentName, namespace string) ServerOption {
	return func(s *Server) {
		s.agentName = agentName
		s.namespace = namespace
	}
}

// WithAgentUID sets the AgentRuntime's Kubernetes UID. This is plumbed into
// the memory scope as agent_id so retrieval queries can merge institutional,
// agent, and user tiers.
func WithAgentUID(uid string) ServerOption {
	return func(s *Server) {
		s.agentUID = uid
	}
}

// ServerAgentUID returns the configured agent UID. Exposed for wiring tests
// under cmd/runtime; production code should not depend on this accessor.
func ServerAgentUID(s *Server) string {
	return s.agentUID
}

// ServerMemoryRetrieval returns the configured retrieval strategy, denyCEL
// expression, and episodic limit. Exposed for wiring tests under cmd/runtime;
// production code should not depend on this accessor.
func ServerMemoryRetrieval(s *Server) (strategy, denyCEL string, limit int) {
	return s.memoryStrategy, s.memoryDenyCEL, s.memoryLimit
}

// WithPromptPackName sets the PromptPack CRD name for tracing.
func WithPromptPackName(name string) ServerOption {
	return func(s *Server) {
		s.promptPackName = name
	}
}

// WithFunctionOutputFormat configures the function-mode response format from
// the AgentRuntime CRD (spec.mode / spec.outputFormat / spec.outputSchema). The
// actual sdk.WithResponseFormat option is built lazily in
// buildConversationOptions, where the agent name (used as the schema name) is
// available. A no-op for non-function modes (see resolveResponseFormat). (#1483)
func WithFunctionOutputFormat(mode, outputFormat string, outputSchema []byte) ServerOption {
	return func(s *Server) {
		s.mode = mode
		s.outputFormat = outputFormat
		s.outputSchemaJSON = outputSchema
	}
}

// ServerOutputFormat returns the configured mode and outputFormat. Exposed for
// wiring tests under cmd/runtime; production code should not depend on it.
func ServerOutputFormat(s *Server) (mode, outputFormat string) {
	return s.mode, s.outputFormat
}

// WithPromptPackVersion sets the PromptPack version for tracing.
func WithPromptPackVersion(version string) ServerOption {
	return func(s *Server) {
		s.promptPackVersion = version
	}
}

// WithStateStore sets the state store for conversation persistence.
func WithStateStore(store statestore.Store) ServerOption {
	return func(s *Server) {
		s.stateStore = store
		s.sdkOptions = append(s.sdkOptions, sdk.WithStateStore(store))
	}
}

// WithMediaStorage sets Omnia's media storage backend and injects it into the
// PromptKit SDK (via sdk.WithMediaStorage) so every provider's MediaLoader
// resolves storage_ref attachments (built via sdk.WithImageStorageRef et al.
// in media_processing.go) to a model-fetchable URL or bytes at provider-call
// time. Because resolution happens fresh on every call (a new MediaLoader per
// provider call — see PromptKit runtime/providers/base_provider.go), a
// storage_ref stays resolvable indefinitely across conversation turns, unlike
// a presigned URL minted once and frozen into conversation history, which
// would eventually expire (#1817).
func WithMediaStorage(store media.Storage) ServerOption {
	return func(s *Server) {
		s.mediaStorage = store
		s.sdkOptions = append(s.sdkOptions, sdk.WithMediaStorage(newOmniaMediaStore(store, nil)))
	}
}

// HasMediaStorage reports whether a media storage backend is configured.
// Exposed for wiring tests under cmd/runtime; production code should not
// depend on this accessor.
func (s *Server) HasMediaStorage() bool {
	return s.mediaStorage != nil
}

// WithSDKOptions adds additional SDK options.
func WithSDKOptions(opts ...sdk.Option) ServerOption {
	return func(s *Server) {
		s.sdkOptions = append(s.sdkOptions, opts...)
	}
}

// WithSkillManifest reads the PromptPack skill manifest at path (typically
// passed via OMNIA_PROMPTPACK_MANIFEST_PATH) and appends one
// sdk.WithSkillSource option per resolved entry, plus the configured
// MaxActive setting. Empty path or missing file is a no-op — skills are
// optional.
//
// Each manifest entry becomes a PromptKit [pkskills.SkillSource] so the
// per-PromptPack MountAs can narrow the virtual path the workflow filter
// sees. ReadResource still hits ContentPath on disk.
//
// Selector configuration is intentionally NOT wired here: the tag and
// embedding selectors require additional inputs (tag list, Provider for
// embeddings) that aren't on the manifest. They can be wired in a follow-up
// once a real user asks.
func WithSkillManifest(path string) ServerOption {
	return func(s *Server) {
		manifest, err := skills.Read(path)
		if err != nil {
			s.log.Error(err, "skill manifest read failed", "manifestPath", path)
			return
		}
		names := make([]string, 0, len(manifest.Skills))
		paths := make([]string, 0, len(manifest.Skills))
		mounts := make([]string, 0, len(manifest.Skills))
		for _, e := range manifest.Skills {
			s.sdkOptions = append(s.sdkOptions, sdk.WithSkillSource(pkskills.SkillSource{
				Dir:     e.ContentPath,
				MountAs: e.MountAs,
			}))
			names = append(names, e.Name)
			paths = append(paths, e.ContentPath)
			mounts = append(mounts, e.MountAs)
		}
		maxActive := 0
		if manifest.Config != nil && manifest.Config.MaxActive > 0 {
			maxActive = int(manifest.Config.MaxActive)
			s.sdkOptions = append(s.sdkOptions,
				sdk.WithMaxActiveSkillsOption(maxActive))
		}
		s.log.Info("skill manifest loaded",
			"manifestPath", path,
			"skillCount", len(manifest.Skills),
			"skillNames", names,
			"skillPaths", paths,
			"skillMounts", mounts,
			"maxActive", maxActive)
	}
}

// WithMockProvider enables mock provider mode for testing.
func WithMockProvider(enabled bool) ServerOption {
	return func(s *Server) {
		s.mockProvider = enabled
	}
}

// WithMockConfigPath sets the path to the mock responses file.
func WithMockConfigPath(path string) ServerOption {
	return func(s *Server) {
		s.mockConfigPath = path
	}
}

// WithModel overrides the model from the pack.
func WithModel(model string) ServerOption {
	return func(s *Server) {
		if model != "" {
			s.sdkOptions = append(s.sdkOptions, sdk.WithModel(model))
		}
	}
}

// WithToolsConfig sets the path to the tools configuration file.
func WithToolsConfig(path string) ServerOption {
	return func(s *Server) {
		s.toolsConfigPath = path
	}
}

// WithTracingProvider sets the tracing provider for the server.
func WithTracingProvider(provider *tracing.Provider) ServerOption {
	return func(s *Server) {
		s.tracingProvider = provider
	}
}

// WithProviderInfo sets the provider type and model for metrics labels.
func WithProviderInfo(providerType, model string) ServerOption {
	return func(s *Server) {
		s.providerType = providerType
		s.model = model
	}
}

// WithProviderAPIKey sets the resolved default-provider API key. Carried on the
// spec's Credential rather than process env (design §5.3.1).
func WithProviderAPIKey(key string) ServerOption {
	return func(s *Server) {
		s.providerAPIKey = key
	}
}

// WithProviderRefName sets the Provider CRD name, denormalized onto
// provider_calls so same-type providers are attributed separately. Empty when
// the runtime is not configured via a providerRef.
func WithProviderRefName(name string) ServerOption {
	return func(s *Server) {
		s.providerRefName = name
	}
}

// WithExtraProviders sets the non-default providers (any role except the
// default llm) resolved from the AgentRuntime's spec.providers[]. Each maps to
// its role's SDK option in buildConversationOptions.
func WithExtraProviders(providers []ResolvedProvider) ServerOption {
	return func(s *Server) {
		s.extraProviders = providers
	}
}

// WithBaseURL sets the base URL for the provider (e.g., for Ollama or custom endpoints).
func WithBaseURL(baseURL string) ServerOption {
	return func(s *Server) {
		s.baseURL = baseURL
	}
}

// WithHeaders sets custom HTTP headers applied to every provider request.
// Used for gateway providers that require attribution or tenant headers.
func WithHeaders(headers map[string]string) ServerOption {
	return func(s *Server) {
		s.headers = headers
	}
}

// PlatformConfig holds the hyperscaler platform hosting configuration.
type PlatformConfig struct {
	Type     string // "bedrock", "vertex", or "azure"
	Region   string
	Project  string
	Endpoint string
}

// AuthConfig holds authentication configuration for platform-hosted providers.
type AuthConfig struct {
	Type                       string // "workloadIdentity", "accessKey", "serviceAccount", "servicePrincipal"
	RoleArn                    string
	ServiceAccountEmail        string
	CredentialsSecretName      string
	CredentialsSecretKey       string
	CredentialsSecretNamespace string
}

// WithPlatform sets the hyperscaler hosting configuration. Empty Type means
// direct provider access (no platform hosting).
func WithPlatform(p PlatformConfig) ServerOption {
	return func(s *Server) {
		s.platformType = p.Type
		s.platformRegion = p.Region
		s.platformProject = p.Project
		s.platformEndpoint = p.Endpoint
	}
}

// WithAuth sets the platform auth configuration. Required when WithPlatform
// is used with a non-empty Type. Ignored otherwise.
func WithAuth(a AuthConfig) ServerOption {
	return func(s *Server) {
		s.authType = a.Type
		s.authRoleArn = a.RoleArn
		s.authServiceAccountEmail = a.ServiceAccountEmail
		s.authCredentialsSecretName = a.CredentialsSecretName
		s.authCredentialsSecretKey = a.CredentialsSecretKey
		s.authCredentialsNamespace = a.CredentialsSecretNamespace
	}
}

// WithProviderRequestTimeout caps the wall-clock duration of non-streaming
// provider HTTP calls. Zero leaves the provider's built-in default in place.
func WithProviderRequestTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.providerRequestTimeout = d
	}
}

// WithProviderStreamIdleTimeout bounds how long an SSE streaming body may
// remain silent before it is aborted. Useful for slow local models whose
// first-token latency can exceed the default 30s. Zero uses the default.
func WithProviderStreamIdleTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.providerStreamIdleTimeout = d
	}
}

// WithPricing sets the provider pricing from the CRD for cost calculation.
// When set, PromptKit uses these rates instead of its built-in pricing tables.
func WithPricing(inputCostPer1K, outputCostPer1K float64) ServerOption {
	return func(s *Server) {
		s.inputCostPer1K = inputCostPer1K
		s.outputCostPer1K = outputCostPer1K
	}
}

// WithEvalCollector sets the unified PromptKit metrics collector for eval metrics.
func WithEvalCollector(c *pkmetrics.Collector) ServerOption {
	return func(s *Server) {
		s.evalCollector = c
	}
}

// WithEvalDefs sets the eval definitions loaded from the prompt pack.
func WithEvalDefs(defs []evals.EvalDef) ServerOption {
	return func(s *Server) {
		s.evalDefs = defs
	}
}

// WithInlineEvalGroups sets the eval group filter for inline execution.
// A nil or empty value lets buildEvalOptions fall back to
// DefaultInlineEvalGroups. Non-empty values run only evals in those
// groups on the inline path; all others are left for the eval-worker.
func WithInlineEvalGroups(groups []string) ServerOption {
	return func(s *Server) {
		s.inlineEvalGroups = groups
	}
}

// WithSessionStore sets the session store for recording events to session-api.
// When set, the runtime bridges PromptKit events to session-api via OmniaEventStore.
func WithSessionStore(store session.Store) ServerOption {
	return func(s *Server) {
		s.sessionStore = store
	}
}

// WithMemoryStore sets the memory store for cross-session memory.
// The store is typically an HTTP client backed by memory-api.
func WithMemoryStore(store pkmemory.Store) ServerOption {
	return func(s *Server) {
		s.memoryStore = store
	}
}

// WithWorkspaceUID sets the workspace UID for memory scope.
func WithWorkspaceUID(uid string) ServerOption {
	return func(s *Server) {
		s.workspaceUID = uid
	}
}

// WithMemoryRetrieval configures the retrieval strategy, access deny-filter,
// and episodic limit (from spec.memory.retrieval). When strategy is "semantic"
// and the memory store supports it, per-turn retrieval uses semantic hybrid
// search with the deny-filter; otherwise keyword FTS. limit 0 falls back to
// defaultEpisodicLimit (10).
func WithMemoryRetrieval(strategy, denyCEL string, limit int) ServerOption {
	return func(s *Server) {
		s.memoryStrategy = strategy
		s.memoryDenyCEL = denyCEL
		s.memoryLimit = limit
	}
}

// WithMemoryModes sets the two independent memory axes from
// spec.memory.retrieval.enabled and spec.memory.tools.enabled. retrievalEnabled
// gates ambient RAG auto-injection; toolsEnabled gates the memory__remember /
// memory__recall tools. Both default true (see NewServer) when not set.
func WithMemoryModes(retrievalEnabled, toolsEnabled bool) ServerOption {
	return func(s *Server) {
		s.memoryRetrievalEnabled = retrievalEnabled
		s.memoryToolsEnabled = toolsEnabled
	}
}

// WithMediaBasePath sets the base path for resolving mock:// URLs.
func WithMediaBasePath(path string) ServerOption {
	return func(s *Server) {
		if path != "" {
			s.mediaResolver = NewMediaResolver(path)
		}
	}
}

// HasMediaResolver reports whether a media resolver has been wired into the
// server via WithMediaBasePath. Used by wiring tests in cmd/runtime to assert
// that cmd/runtime/main.go forwards cfg.MediaBasePath to the server (without
// which mock:// and file:// URL resolution in media chunks silently fails).
func (s *Server) HasMediaResolver() bool {
	return s.mediaResolver != nil
}

// WithContextWindow sets the token budget for conversation context.
// When set, PromptKit automatically truncates older messages when the budget is exceeded.
func WithContextWindow(tokens int) ServerOption {
	return func(s *Server) {
		if tokens > 0 {
			s.sdkOptions = append(s.sdkOptions, sdk.WithTokenBudget(tokens))
		}
	}
}

// WithTruncationStrategy sets the strategy for handling context overflow.
// Valid values: "sliding" (remove oldest), "summarize" (summarize before
// removing), "custom" (the runtime implements truncation itself — no SDK
// truncation is configured). "custom" is intended for custom runtimes
// (spec.framework.type: custom); on this PromptKit runtime it means no
// truncation is applied at all, which cmd/runtime warns about at startup.
func WithTruncationStrategy(strategy string) ServerOption {
	return func(s *Server) {
		// "custom" means the custom runtime handles it - don't set SDK truncation
		if strategy != "" && strategy != string(v1alpha1.TruncationStrategyCustom) {
			s.sdkOptions = append(s.sdkOptions, sdk.WithTruncation(strategy))
		}
	}
}

// NewServer creates a new runtime server.
func NewServer(opts ...ServerOption) *Server {
	s := &Server{
		conversations:  make(map[string]*sdk.Conversation),
		turnIndices:    make(map[string]int),
		unsubscribeFns: make(map[string][]func()),
		healthy:        true,
		// Default both memory axes on; WithMemoryModes overrides from the CRD.
		memoryRetrievalEnabled: true,
		memoryToolsEnabled:     true,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// InitializeTools loads and connects tool backends from the config file.
// This should be called before handling any conversations.
func (s *Server) InitializeTools(ctx context.Context) error {
	if s.toolsConfigPath == "" {
		s.log.Info("no tools config path set, skipping tool initialization")
		return nil
	}

	s.log.Info("initializing tools", "configPath", s.toolsConfigPath)

	executor := tools.NewOmniaExecutor(s.log, s.tracingProvider)

	if err := executor.LoadConfig(s.toolsConfigPath); err != nil {
		return fmt.Errorf("failed to load tools config: %w", err)
	}

	if err := executor.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize tool backends: %w", err)
	}

	s.toolExecutor = executor
	s.toolsInitialized = true
	s.log.Info("tools initialized successfully",
		"toolCount", len(executor.ToolNames()))

	// Surface the ToolRegistry's tools into the pack prompts' allowed-tools
	// lists. PromptKit's ProviderStage only exposes a tool to the LLM when the
	// prompt names it (or it is a system capability); an Omnia ToolRegistry
	// `http`/`grpc` tool the pack prompt never lists would otherwise be
	// filtered out and never dispatched. See surfaceRegistryToolsInPack.
	s.packPath = surfaceRegistryToolsInPack(s.packPath, executor.ToolNames(), s.log)

	return nil
}

// SetToolRegistryInfo populates registry/handler metadata on the tool executor.
// This must be called after InitializeTools and before handling conversations.
func (s *Server) SetToolRegistryInfo(registryName, registryNamespace string, handlers []tools.HandlerEntry) {
	if s.toolExecutor == nil {
		return
	}
	s.toolExecutor.SetRegistryInfo(registryName, registryNamespace, handlers)
}

// ServerToolRegistryInfo exposes the registry name/namespace recorded on the
// tool executor. Test-only accessor — production code should not depend on it.
func ServerToolRegistryInfo(s *Server) (string, string) {
	if s == nil || s.toolExecutor == nil {
		return "", ""
	}
	return s.toolExecutor.RegistryName(), s.toolExecutor.RegistryNamespace()
}

// SetHealthy sets the server health status.
func (s *Server) SetHealthy(healthy bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.healthy = healthy
}

// Health implements the health check RPC.
func (s *Server) Health(_ context.Context, _ *runtimev1.HealthRequest) (*runtimev1.HealthResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	statusMsg := "ready"
	if !s.healthy {
		statusMsg = "not ready"
	}

	return &runtimev1.HealthResponse{
		Healthy:         s.healthy,
		Status:          statusMsg,
		ContractVersion: contract.Version,
		Capabilities:    Capabilities(),
	}, nil
}

// probeConversationExists reports whether the state store holds a conversation
// for sessionID, returning ErrNotFound when it definitively does not.
//
// It prefers MessageReader.MessageCount over Store.Load because a probe must not
// change what it measures. The two bundled stores disagree on what a TTL means:
// the memory store treats it as idle time and refreshes it on Load, while Redis
// runs a fixed window from the last write and does not. Probing with Load would
// therefore silently extend a memory-backed conversation's life on every
// reconnect while leaving a Redis-backed one untouched. MessageCount is a pure
// read on both.
//
// That divergence is upstream — PromptKit#1649. This is a workaround, not a
// fix: any path that reads through Load still inherits it.
//
// The existence verdict is unchanged: MessageCount returns ErrNotFound exactly
// where Load finds nothing, and (0, nil) for a conversation that exists but
// holds no messages — which resumes fine.
//
// Falls back to Load for a store that does not implement MessageReader, matching
// the optional-interface pattern PromptKit's own pipeline stages use.
func (s *Server) probeConversationExists(ctx context.Context, sessionID string) error {
	if reader, ok := s.stateStore.(statestore.MessageReader); ok {
		_, err := reader.MessageCount(ctx, sessionID)
		return err
	}

	state, err := s.stateStore.Load(ctx, sessionID)
	if err == nil && state == nil {
		// Defensive: a custom store may signal a miss this way instead.
		return statestore.ErrNotFound
	}
	return err
}

// HasConversation reports whether a session's working context can still be
// resumed. It mirrors getOrCreateConversation's resolution order exactly — the
// process-local conversation map first, then the state store — so the answer is
// the same one the next Converse call would arrive at, not an approximation.
//
// A store failure is reported as UNAVAILABLE rather than NOT_FOUND. The
// distinction matters: NOT_FOUND is a client-visible expiry, while UNAVAILABLE
// means the context may well be intact and the store was simply unreachable.
func (s *Server) HasConversation(
	ctx context.Context,
	req *runtimev1.HasConversationRequest,
) (*runtimev1.HasConversationResponse, error) {
	sessionID := req.GetSessionId()
	if sessionID == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	// An in-flight Converse stream holds the conversation here; getOrCreateConversation
	// short-circuits on it before ever consulting the store, so it must be checked first.
	s.conversationMu.RLock()
	_, live := s.conversations[sessionID]
	s.conversationMu.RUnlock()
	if live {
		return &runtimev1.HasConversationResponse{
			State: runtimev1.ResumeState_RESUME_STATE_RESUMABLE,
		}, nil
	}

	// sdk.Resume returns ErrNoStateStore when no store is configured, so nothing
	// would ever resume — but that is a runtime misconfiguration, not an expiry.
	if s.stateStore == nil {
		return &runtimev1.HasConversationResponse{
			State:  runtimev1.ResumeState_RESUME_STATE_UNAVAILABLE,
			Detail: "no state store configured",
		}, nil
	}

	// Both bundled stores report a miss as ErrNotFound, so that sentinel — not a
	// nil result — is what distinguishes an expired conversation from a store
	// that could not be reached.
	err := s.probeConversationExists(ctx, sessionID)
	switch {
	case errors.Is(err, statestore.ErrNotFound), errors.Is(err, statestore.ErrInvalidID):
		return &runtimev1.HasConversationResponse{
			State: runtimev1.ResumeState_RESUME_STATE_NOT_FOUND,
		}, nil
	case err != nil:
		return &runtimev1.HasConversationResponse{
			State:  runtimev1.ResumeState_RESUME_STATE_UNAVAILABLE,
			Detail: err.Error(),
		}, nil
	}
	return &runtimev1.HasConversationResponse{
		State: runtimev1.ResumeState_RESUME_STATE_RESUMABLE,
	}, nil
}

// Converse implements the bidirectional streaming conversation RPC.
func (s *Server) Converse(stream runtimev1.RuntimeService_ConverseServer) error {
	ctx := stream.Context()
	var lastSessionID string

	defer func() {
		// Remove conversation when stream ends to prevent unbounded map growth.
		if lastSessionID != "" {
			s.removeConversation(lastSessionID)
		}
	}()

	for {
		// Receive client message
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Internal, "failed to receive message: %v", err)
		}

		lastSessionID = msg.GetSessionId()

		// Duplex audio sessions are handled as a self-contained sub-call:
		// the stream is bridged to sdk.OpenDuplex for the duration of the session.
		if msg.GetDuplexStart() != nil {
			if duplexErr := s.handleDuplexSession(ctx, stream, msg); duplexErr != nil {
				s.log.Error(duplexErr, "duplex session failed", "sessionID", msg.GetSessionId())
				_ = stream.Send(&runtimev1.ServerMessage{Message: &runtimev1.ServerMessage_Error{Error: &runtimev1.Error{
					Code: "DUPLEX_ERROR", Message: "duplex session failed",
				}}})
			}
			return nil
		}

		// Process the message
		if err := s.processMessage(ctx, stream, msg); err != nil {
			s.log.Error(err, "failed to process message",
				"sessionID", msg.GetSessionId())

			// Send a generic error to the client. The detailed error is
			// logged above but must not be forwarded because it may contain
			// sensitive information such as provider API keys.
			_ = stream.Send(&runtimev1.ServerMessage{
				Message: &runtimev1.ServerMessage_Error{
					Error: &runtimev1.Error{
						Code:    "INTERNAL_ERROR",
						Message: "an internal error occurred while processing the message",
					},
				},
			})
		}
	}
}

// removeConversation removes a completed conversation and cleans up associated resources.
// This prevents the conversations and turnIndices maps from growing unboundedly.
func (s *Server) removeConversation(sessionID string) {
	s.conversationMu.Lock()
	defer s.conversationMu.Unlock()

	conv, exists := s.conversations[sessionID]
	if !exists {
		return
	}

	// Unsubscribe event bus listeners
	for _, unsub := range s.unsubscribeFns[sessionID] {
		unsub()
	}
	delete(s.unsubscribeFns, sessionID)

	if err := conv.Close(); err != nil {
		s.log.Error(err, "failed to close conversation", "sessionID", sessionID)
	}
	delete(s.conversations, sessionID)
	delete(s.turnIndices, sessionID)

	s.log.V(1).Info("conversation removed", "sessionID", sessionID)
}

// Close closes all open conversations, the tool manager, and the tracing provider.
func (s *Server) Close() error {
	s.conversationMu.Lock()
	defer s.conversationMu.Unlock()

	for id, conv := range s.conversations {
		for _, unsub := range s.unsubscribeFns[id] {
			unsub()
		}
		if err := conv.Close(); err != nil {
			s.log.Error(err, "failed to close conversation", "sessionID", id)
		}
	}
	s.conversations = make(map[string]*sdk.Conversation)
	s.turnIndices = make(map[string]int)
	s.unsubscribeFns = make(map[string][]func())

	// Close tool executor
	if s.toolExecutor != nil {
		if err := s.toolExecutor.Close(); err != nil {
			s.log.Error(err, "failed to close tool executor")
		}
		s.toolExecutor = nil
		s.toolsInitialized = false
	}

	// Shutdown tracing provider
	if s.tracingProvider != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.tracingProvider.Shutdown(ctx); err != nil {
			s.log.Error(err, "failed to shutdown tracing provider")
		}
		s.tracingProvider = nil
	}

	return nil
}
