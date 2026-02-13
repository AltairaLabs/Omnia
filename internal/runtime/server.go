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
	"sync"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/AltairaLabs/PromptKit/runtime/statestore"
	// Register all providers via blank imports
	// TODO: PromptKit should provide a "providers/all" package for convenience
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/claude"
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/gemini"
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/ollama"
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/openai"
	"github.com/AltairaLabs/PromptKit/sdk"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"

	"github.com/altairalabs/omnia/internal/runtime/tools"
	"github.com/altairalabs/omnia/internal/tracing"
)

// Server implements the RuntimeService gRPC server.
// It wraps the PromptKit SDK to handle LLM conversations.
type Server struct {
	runtimev1.UnimplementedRuntimeServiceServer

	log            logr.Logger
	packPath       string
	promptName     string
	stateStore     statestore.Store
	mockProvider   bool
	mockConfigPath string
	sdkOptions     []sdk.Option
	conversations  map[string]*sdk.Conversation
	conversationMu sync.RWMutex
	healthy        bool
	mu             sync.RWMutex

	// Tool management
	toolManager      *tools.Manager
	toolExecutor     *tools.ManagerExecutor
	toolsConfigPath  string
	toolsInitialized bool

	// Tracing
	tracingProvider *tracing.Provider

	// Metrics
	metrics        *Metrics
	runtimeMetrics *RuntimeMetrics
	providerType   string
	model          string
	baseURL        string // Custom base URL for provider (e.g., Ollama endpoint)

	// Media resolution for mock provider
	mediaResolver *MediaResolver
}

// ServerOption configures the server.
type ServerOption func(*Server)

// WithLogger sets the logger for the server.
func WithLogger(log logr.Logger) ServerOption {
	return func(s *Server) {
		s.log = log
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

// WithMetrics sets the Prometheus metrics collector for the server.
func WithMetrics(metrics *Metrics) ServerOption {
	return func(s *Server) {
		s.metrics = metrics
	}
}

// WithRuntimeMetrics sets the runtime Prometheus metrics collector for the server.
// This tracks tool calls and pipeline executions.
func WithRuntimeMetrics(metrics *RuntimeMetrics) ServerOption {
	return func(s *Server) {
		s.runtimeMetrics = metrics
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

// WithMediaBasePath sets the base path for resolving mock:// URLs.
func WithMediaBasePath(path string) ServerOption {
	return func(s *Server) {
		if path != "" {
			s.mediaResolver = NewMediaResolver(path)
		}
	}
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
		conversations: make(map[string]*sdk.Conversation),
		healthy:       true,
	}

	for _, opt := range opts {
		opt(s)
	}

	// Initialize metrics with known label values so they appear in /metrics immediately.
	// CounterVec and HistogramVec only show up in Prometheus output after being observed.
	if s.metrics != nil && s.providerType != "" && s.model != "" {
		s.metrics.Initialize(s.providerType, s.model)
	}
	if s.runtimeMetrics != nil {
		s.runtimeMetrics.Initialize()
	}

	return s
}

// InitializeTools loads and connects tool adapters from the config file.
// This should be called before handling any conversations.
func (s *Server) InitializeTools(ctx context.Context) error {
	if s.toolsConfigPath == "" {
		s.log.Info("no tools config path set, skipping tool initialization")
		return nil
	}

	s.log.Info("initializing tools", "configPath", s.toolsConfigPath)

	// Create tool manager
	s.toolManager = tools.NewManager(s.log.WithName("tools"))

	// Load configuration
	if err := s.toolManager.LoadFromConfig(s.toolsConfigPath); err != nil {
		return fmt.Errorf("failed to load tools config: %w", err)
	}

	// Connect all adapters
	if err := s.toolManager.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect tool adapters: %w", err)
	}

	// Create the executor for PromptKit integration
	s.toolExecutor = tools.NewManagerExecutor(s.toolManager, s.log)

	s.toolsInitialized = true
	s.log.Info("tools initialized successfully",
		"toolCount", len(s.toolManager.ListTools()))

	return nil
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

	for {
		// Receive client message
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Internal, "failed to receive message: %v", err)
		}

		// Process the message
		if err := s.processMessage(ctx, stream, msg); err != nil {
			s.log.Error(err, "failed to process message",
				"sessionID", msg.GetSessionId())

			// Send error to client
			_ = stream.Send(&runtimev1.ServerMessage{
				Message: &runtimev1.ServerMessage_Error{
					Error: &runtimev1.Error{
						Code:    "INTERNAL_ERROR",
						Message: err.Error(),
					},
				},
			})
		}
	}
}

// Close closes all open conversations, the tool manager, and the tracing provider.
func (s *Server) Close() error {
	s.conversationMu.Lock()
	defer s.conversationMu.Unlock()

	for id, conv := range s.conversations {
		if err := conv.Close(); err != nil {
			s.log.Error(err, "failed to close conversation", "sessionID", id)
		}
	}
	s.conversations = make(map[string]*sdk.Conversation)

	// Close tool manager
	if s.toolManager != nil {
		if err := s.toolManager.Close(); err != nil {
			s.log.Error(err, "failed to close tool manager")
		}
		s.toolManager = nil
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
