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

	"github.com/altairalabs/omnia/internal/runtime/skills"
	"github.com/altairalabs/omnia/internal/runtime/tools"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/tracing"
)

// Server implements the RuntimeService gRPC server.
// It wraps the PromptKit SDK to handle LLM conversations.
type Server struct {
	runtimev1.UnimplementedRuntimeServiceServer

	log               logr.Logger
	sdkLogger         *slog.Logger
	packPath          string
	agentName         string
	namespace         string
	promptPackName    string
	promptPackVersion string
	promptName        string
	stateStore        statestore.Store
	mockProvider      bool
	mockConfigPath    string
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
	evalCollector *pkmetrics.Collector
	evalDefs      []evals.EvalDef

	// Provider info (for logging and provider creation)
	providerType              string
	model                     string
	baseURL                   string        // Custom base URL for provider (e.g., Ollama endpoint)
	inputCostPer1K            float64       // CRD pricing: cost per 1K input tokens
	outputCostPer1K           float64       // CRD pricing: cost per 1K output tokens
	providerRequestTimeout    time.Duration // Non-streaming HTTP timeout (0 = provider default)
	providerStreamIdleTimeout time.Duration // SSE stream idle timeout (0 = 30s default)

	// Session recording (Pattern C)
	sessionStore session.Store

	// Memory store for cross-session memory (via memory-api HTTP client)
	memoryStore  pkmemory.Store
	workspaceUID string // Workspace CRD UID for memory scope

	// Media resolution for mock provider
	mediaResolver *MediaResolver
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

// WithPromptPackName sets the PromptPack CRD name for tracing.
func WithPromptPackName(name string) ServerOption {
	return func(s *Server) {
		s.promptPackName = name
	}
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

// WithSDKOptions adds additional SDK options.
func WithSDKOptions(opts ...sdk.Option) ServerOption {
	return func(s *Server) {
		s.sdkOptions = append(s.sdkOptions, opts...)
	}
}

// WithSkillManifest reads the PromptPack skill manifest at path (typically
// passed via OMNIA_PROMPTPACK_MANIFEST_PATH) and appends one
// sdk.WithSkillsDir option per resolved entry, plus the configured
// MaxActive setting. Empty path or missing file is a no-op — skills are
// optional.
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
		for _, e := range manifest.Skills {
			s.sdkOptions = append(s.sdkOptions, sdk.WithSkillsDir(e.ContentPath))
			names = append(names, e.Name)
			paths = append(paths, e.ContentPath)
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

// WithBaseURL sets the base URL for the provider (e.g., for Ollama or custom endpoints).
func WithBaseURL(baseURL string) ServerOption {
	return func(s *Server) {
		s.baseURL = baseURL
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
// Valid values: "sliding" (remove oldest), "summarize" (summarize before removing),
// "custom" (delegate to custom runtime implementation - no SDK truncation).
func WithTruncationStrategy(strategy string) ServerOption {
	return func(s *Server) {
		// "custom" means the custom runtime handles it - don't set SDK truncation
		if strategy != "" && strategy != "custom" {
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
		Healthy: s.healthy,
		Status:  statusMsg,
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
