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
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/AltairaLabs/PromptKit/runtime/providers/mock"
	"github.com/AltairaLabs/PromptKit/runtime/statestore"
	"github.com/AltairaLabs/PromptKit/sdk"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"

	"github.com/altairalabs/omnia/internal/runtime/tools"
	"github.com/altairalabs/omnia/internal/runtime/tracing"
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

// NewServer creates a new runtime server.
func NewServer(opts ...ServerOption) *Server {
	s := &Server{
		conversations: make(map[string]*sdk.Conversation),
		healthy:       true,
	}

	for _, opt := range opts {
		opt(s)
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

func (s *Server) processMessage(ctx context.Context, stream runtimev1.RuntimeService_ConverseServer, msg *runtimev1.ClientMessage) error {
	sessionID := msg.GetSessionId()
	content := msg.GetContent()

	// Start conversation span if tracing is enabled
	if s.tracingProvider != nil {
		var span trace.Span
		ctx, span = s.tracingProvider.StartConversationSpan(ctx, sessionID)
		defer span.End()
	}

	s.log.V(1).Info("processing message",
		"sessionID", sessionID,
		"contentLength", len(content))

	// Get or create conversation for this session
	conv, err := s.getOrCreateConversation(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get conversation: %w", err)
	}

	// Send the message using PromptKit SDK
	resp, err := conv.Send(ctx, content)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	// Stream the response back to the client
	// For now, send the full response as a single chunk
	// Note: Streaming will be implemented when SDK supports it
	responseText := resp.Text()

	// Send chunk
	if err := stream.Send(&runtimev1.ServerMessage{
		Message: &runtimev1.ServerMessage_Chunk{
			Chunk: &runtimev1.Chunk{Content: responseText},
		},
	}); err != nil {
		return fmt.Errorf("failed to send chunk: %w", err)
	}

	// Build usage info
	var usage *runtimev1.Usage
	inputTokens := resp.InputTokens()
	outputTokens := resp.OutputTokens()
	costUSD := resp.Cost()

	if resp.TokensUsed() > 0 {
		usage = &runtimev1.Usage{
			InputTokens:  int32(inputTokens),
			OutputTokens: int32(outputTokens),
			CostUsd:      float32(costUSD),
		}

		// Add LLM metrics to the conversation span
		if s.tracingProvider != nil {
			span := trace.SpanFromContext(ctx)
			tracing.AddLLMMetrics(span, inputTokens, outputTokens, costUSD)
			tracing.AddConversationMetrics(span, len(content), len(responseText))
			tracing.SetSuccess(span)
		}
	}

	// Send done message
	if err := stream.Send(&runtimev1.ServerMessage{
		Message: &runtimev1.ServerMessage_Done{
			Done: &runtimev1.Done{
				FinalContent: responseText,
				Usage:        usage,
			},
		},
	}); err != nil {
		return fmt.Errorf("failed to send done: %w", err)
	}

	return nil
}

// getOrCreateConversation gets an existing conversation or creates a new one.
func (s *Server) getOrCreateConversation(sessionID string) (*sdk.Conversation, error) {
	s.conversationMu.RLock()
	conv, exists := s.conversations[sessionID]
	s.conversationMu.RUnlock()

	if exists {
		return conv, nil
	}

	// Create new conversation
	s.conversationMu.Lock()
	defer s.conversationMu.Unlock()

	// Double-check after acquiring write lock
	if conv, exists = s.conversations[sessionID]; exists {
		return conv, nil
	}

	// Build options with conversation ID
	opts := append([]sdk.Option{
		sdk.WithConversationID(sessionID),
	}, s.sdkOptions...)

	// Add mock provider if enabled
	if s.mockProvider {
		s.log.Info("using mock provider for conversation", "sessionID", sessionID)
		var provider *mock.Provider
		if s.mockConfigPath != "" {
			// Use file-based mock repository
			repo, err := mock.NewFileMockRepository(s.mockConfigPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load mock config: %w", err)
			}
			provider = mock.NewProviderWithRepository("mock", "mock-model", false, repo)
		} else {
			// Use in-memory mock provider with default responses
			provider = mock.NewProvider("mock", "mock-model", false)
		}
		opts = append(opts, sdk.WithProvider(provider))
	}

	// Try to resume existing conversation first
	conv, err := sdk.Resume(sessionID, s.packPath, s.promptName, opts...)
	if err != nil {
		// If resume fails (conversation not found), create new
		s.log.V(1).Info("creating new conversation", "sessionID", sessionID)
		conv, err = sdk.Open(s.packPath, s.promptName, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to open pack: %w", err)
		}
	} else {
		s.log.V(1).Info("resumed existing conversation", "sessionID", sessionID)
	}

	// Register tools with the conversation if available
	if s.toolsInitialized && s.toolExecutor != nil {
		if err := s.registerToolsWithConversation(conv); err != nil {
			s.log.Error(err, "failed to register tools with conversation", "sessionID", sessionID)
			// Continue without tools - don't fail the conversation
		}
	}

	s.conversations[sessionID] = conv
	return conv, nil
}

// registerToolsWithConversation registers all available tools with a conversation.
func (s *Server) registerToolsWithConversation(conv *sdk.Conversation) error {
	ctx := context.Background()

	// Get all tool descriptors from the executor
	descriptors, err := s.toolExecutor.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("failed to list tools: %w", err)
	}

	// Register each tool with the conversation using a context-aware handler
	for _, desc := range descriptors {
		toolName := desc.Name
		s.log.V(1).Info("registering tool with conversation", "tool", toolName)

		// Create a closure that captures the executor and descriptor
		conv.OnToolCtx(toolName, func(ctx context.Context, args map[string]any) (any, error) {
			return s.executeToolForConversation(ctx, toolName, args)
		})
	}

	s.log.Info("registered tools with conversation", "count", len(descriptors))
	return nil
}

// executeToolForConversation executes a tool call for a conversation.
func (s *Server) executeToolForConversation(ctx context.Context, toolName string, args map[string]any) (any, error) {
	// Start tool span if tracing is enabled
	var span trace.Span
	if s.tracingProvider != nil {
		ctx, span = s.tracingProvider.StartToolSpan(ctx, toolName)
		defer span.End()
	}

	s.log.V(1).Info("executing tool for conversation", "tool", toolName)

	// Call the tool through the manager
	result, err := s.toolManager.Call(ctx, toolName, args)
	if err != nil {
		if span != nil {
			tracing.RecordError(span, err)
		}
		return nil, fmt.Errorf("tool execution failed: %w", err)
	}

	// Add tool result metrics to span
	if span != nil {
		resultSize := 0
		if result.Content != nil {
			resultSize = len(fmt.Sprintf("%v", result.Content))
		}
		tracing.AddToolResult(span, result.IsError, resultSize)
		if result.IsError {
			tracing.RecordError(span, fmt.Errorf("tool error: %v", result.Content))
		} else {
			tracing.SetSuccess(span)
		}
	}

	if result.IsError {
		return nil, fmt.Errorf("tool error: %v", result.Content)
	}

	return result.Content, nil
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
