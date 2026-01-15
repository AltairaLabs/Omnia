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
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/providers/mock"
	// Register all providers via blank imports
	// TODO: PromptKit should provide a "providers/all" package for convenience
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/claude"
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/gemini"
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/ollama"
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/openai"
	"github.com/AltairaLabs/PromptKit/runtime/statestore"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/PromptKit/sdk"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"

	"github.com/altairalabs/omnia/internal/runtime/tools"
	"github.com/altairalabs/omnia/internal/runtime/tracing"
	"github.com/altairalabs/omnia/pkg/logctx"
	"github.com/altairalabs/omnia/pkg/metrics"
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

func (s *Server) processMessage(ctx context.Context, stream runtimev1.RuntimeService_ConverseServer, msg *runtimev1.ClientMessage) error {
	sessionID := msg.GetSessionId()
	content := msg.GetContent()
	metadata := msg.GetMetadata()

	// Enrich context with session ID and start tracing span
	ctx = logctx.WithSessionID(ctx, sessionID)
	log := logctx.LoggerWithContext(s.log, ctx)
	ctx = s.startTracingSpan(ctx, sessionID)

	// Extract mock scenario and prepare message content
	scenario := s.extractScenario(metadata, content, log)
	log.V(1).Info("processing message", "contentLength", len(content), "scenario", scenario)

	// Get or create conversation for this session
	conv, err := s.getOrCreateConversation(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get conversation: %w", err)
	}

	// Prepare message content with scenario if needed
	messageContent := s.prepareMessageContent(content, scenario, log)

	// Build send options for multimodal content (images, audio, etc.)
	sendOpts := buildSendOptions(msg.GetParts(), log)

	// Stream response and collect results
	finalResponse, accumulatedContent, err := s.streamResponse(ctx, stream, conv, messageContent, sendOpts)
	if err != nil {
		return err
	}

	// Build and send the done message
	return s.sendDoneMessage(ctx, stream, log, finalResponse, accumulatedContent, content)
}

// startTracingSpan starts a conversation span if tracing is enabled, returning the enriched context.
func (s *Server) startTracingSpan(ctx context.Context, sessionID string) context.Context {
	if s.tracingProvider != nil {
		var span trace.Span
		ctx, span = s.tracingProvider.StartConversationSpan(ctx, sessionID)
		defer span.End()
	}
	return ctx
}

// extractScenario extracts the mock scenario from metadata/content if mock provider is enabled.
func (s *Server) extractScenario(metadata map[string]string, content string, log logr.Logger) string {
	if !s.mockProvider {
		return ScenarioDefault
	}
	scenario := extractMockScenario(metadata, content)
	log.V(1).Info("mock scenario detected", "scenario", scenario)
	return scenario
}

// prepareMessageContent prepends scenario context to the message if using mock provider.
func (s *Server) prepareMessageContent(content string, scenario string, log logr.Logger) string {
	if s.mockProvider && scenario != ScenarioDefault {
		log.V(2).Info("enriched message with scenario", "scenario", scenario)
		return fmt.Sprintf("[scenario:%s] %s", scenario, content)
	}
	return content
}

// streamResponse streams the LLM response and sends chunks to the client.
func (s *Server) streamResponse(ctx context.Context, stream runtimev1.RuntimeService_ConverseServer, conv *sdk.Conversation, content string, opts []sdk.SendOption) (*sdk.Response, string, error) {
	streamCh := conv.Stream(ctx, content, opts...)
	var finalResponse *sdk.Response
	var accumulatedContent strings.Builder

	for chunk := range streamCh {
		if chunk.Error != nil {
			return nil, "", fmt.Errorf("failed to send message: provider stream failed: %w", chunk.Error)
		}

		switch chunk.Type {
		case sdk.ChunkText:
			if chunk.Text != "" {
				accumulatedContent.WriteString(chunk.Text)
				if err := stream.Send(&runtimev1.ServerMessage{
					Message: &runtimev1.ServerMessage_Chunk{
						Chunk: &runtimev1.Chunk{Content: chunk.Text},
					},
				}); err != nil {
					return nil, "", fmt.Errorf("failed to send chunk: %w", err)
				}
			}
		case sdk.ChunkDone:
			finalResponse = chunk.Message
		}
	}

	return finalResponse, accumulatedContent.String(), nil
}

// sendDoneMessage builds usage info and sends the done message to the client.
func (s *Server) sendDoneMessage(ctx context.Context, stream runtimev1.RuntimeService_ConverseServer, log logr.Logger, finalResponse *sdk.Response, accumulatedContent string, originalContent string) error {
	responseText, usage := s.buildUsageInfo(ctx, finalResponse, accumulatedContent, originalContent)

	// Build multimodal parts if response contains media
	var parts []*runtimev1.ContentPart
	if finalResponse != nil && finalResponse.HasMedia() && s.mediaResolver != nil {
		var err error
		parts, err = s.resolveResponseParts(ctx, finalResponse.Parts())
		if err != nil {
			log.Error(err, "failed to resolve media parts, falling back to text-only")
		}
	}

	// Send done message
	if err := stream.Send(&runtimev1.ServerMessage{
		Message: &runtimev1.ServerMessage_Done{
			Done: &runtimev1.Done{
				FinalContent: responseText,
				Usage:        usage,
				Parts:        parts,
			},
		},
	}); err != nil {
		return fmt.Errorf("failed to send done: %w", err)
	}

	return nil
}

// buildUsageInfo extracts usage info from the final response and records tracing metrics.
func (s *Server) buildUsageInfo(ctx context.Context, finalResponse *sdk.Response, accumulatedContent string, originalContent string) (string, *runtimev1.Usage) {
	if finalResponse == nil {
		return accumulatedContent, nil
	}

	responseText := finalResponse.Text()
	if finalResponse.TokensUsed() == 0 {
		return responseText, nil
	}

	inputTokens := finalResponse.InputTokens()
	outputTokens := finalResponse.OutputTokens()
	costUSD := finalResponse.Cost()

	usage := &runtimev1.Usage{
		InputTokens:  int32(inputTokens),
		OutputTokens: int32(outputTokens),
		CostUsd:      float32(costUSD),
	}

	// Add LLM metrics to the conversation span
	if s.tracingProvider != nil {
		span := trace.SpanFromContext(ctx)
		tracing.AddLLMMetrics(span, inputTokens, outputTokens, costUSD)
		tracing.AddConversationMetrics(span, len(originalContent), len(responseText))
		tracing.SetSuccess(span)
	}

	return responseText, usage
}

// resolveResponseParts converts PromptKit ContentParts to gRPC ContentParts,
// resolving any file:// or mock:// URLs to base64 data.
func (s *Server) resolveResponseParts(ctx context.Context, parts []types.ContentPart) ([]*runtimev1.ContentPart, error) {
	log := logctx.LoggerWithContext(s.log, ctx)
	result := make([]*runtimev1.ContentPart, 0, len(parts))

	for _, part := range parts {
		grpcPart := &runtimev1.ContentPart{
			Type: part.Type,
		}

		switch part.Type {
		case types.ContentTypeText:
			if part.Text != nil {
				grpcPart.Text = *part.Text
			}

		case types.ContentTypeImage, types.ContentTypeAudio, types.ContentTypeVideo:
			if part.Media == nil {
				continue
			}

			mediaContent, err := s.resolveMediaContent(ctx, part.Media)
			if err != nil {
				log.Error(err, "failed to resolve media content", "type", part.Type)
				continue
			}
			grpcPart.Media = mediaContent
		}

		result = append(result, grpcPart)
	}

	return result, nil
}

// resolveMediaContent resolves a PromptKit MediaContent to a gRPC MediaContent,
// converting file:// and mock:// URLs to base64 data.
func (s *Server) resolveMediaContent(ctx context.Context, media *types.MediaContent) (*runtimev1.MediaContent, error) {
	log := logctx.LoggerWithContext(s.log, ctx)

	// If we already have base64 data, use it directly
	if media.Data != nil && *media.Data != "" {
		return &runtimev1.MediaContent{
			Data:     *media.Data,
			MimeType: media.MIMEType,
		}, nil
	}

	// If we have a URL, try to resolve it
	if media.URL != nil && *media.URL != "" {
		url := *media.URL

		// Check if URL needs resolution (file:// or mock://)
		if IsResolvableURL(url) {
			if s.mediaResolver == nil {
				return nil, fmt.Errorf("media resolver not configured, cannot resolve URL: %s", url)
			}

			base64Data, mimeType, isPassthrough, err := s.mediaResolver.ResolveURL(url)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve media URL %s: %w", url, err)
			}

			if isPassthrough {
				// HTTP/HTTPS URL - pass through unchanged
				return &runtimev1.MediaContent{
					Url:      url,
					MimeType: mimeType,
				}, nil
			}

			log.V(1).Info("resolved media URL", "url", url, "mimeType", mimeType, "dataSize", len(base64Data))
			return &runtimev1.MediaContent{
				Data:     base64Data,
				MimeType: mimeType,
			}, nil
		}

		// HTTP/HTTPS URL - pass through unchanged
		return &runtimev1.MediaContent{
			Url:      url,
			MimeType: media.MIMEType,
		}, nil
	}

	// If we have a file path, read it
	if media.FilePath != nil && *media.FilePath != "" {
		base64Data, mimeType, _, err := s.mediaResolver.ResolveURL("file://" + *media.FilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read media file %s: %w", *media.FilePath, err)
		}

		return &runtimev1.MediaContent{
			Data:     base64Data,
			MimeType: mimeType,
		}, nil
	}

	return nil, fmt.Errorf("media content has no data source")
}

// getOrCreateConversation gets an existing conversation or creates a new one.
func (s *Server) getOrCreateConversation(ctx context.Context, sessionID string) (*sdk.Conversation, error) {
	// Try to get existing conversation with read lock
	s.conversationMu.RLock()
	conv, exists := s.conversations[sessionID]
	s.conversationMu.RUnlock()

	if exists {
		return conv, nil
	}

	// Create new conversation with write lock
	s.conversationMu.Lock()
	defer s.conversationMu.Unlock()

	// Double-check after acquiring write lock
	if conv, exists = s.conversations[sessionID]; exists {
		return conv, nil
	}

	// Create and initialize the conversation
	conv, err := s.createConversation(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	s.conversations[sessionID] = conv
	return conv, nil
}

// createConversation creates and initializes a new conversation with the given session ID.
func (s *Server) createConversation(ctx context.Context, sessionID string) (*sdk.Conversation, error) {
	log := logctx.LoggerWithContext(s.log, ctx)

	// Build SDK options with provider
	opts, err := s.buildConversationOptions(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Try to resume existing conversation first, or create new
	conv, err := s.resumeOrOpenConversation(sessionID, opts, log)
	if err != nil {
		return nil, err
	}

	// Register tools with the conversation if available
	if s.toolsInitialized && s.toolExecutor != nil {
		if err := s.registerToolsWithConversation(ctx, conv); err != nil {
			log.Error(err, "failed to register tools with conversation")
			// Continue without tools - don't fail the conversation
		}
	}

	// Subscribe to event bus metrics for observability
	s.subscribeToEventBusMetrics(sessionID, conv)

	return conv, nil
}

// buildConversationOptions builds the SDK options for a conversation, including provider setup.
func (s *Server) buildConversationOptions(ctx context.Context, sessionID string) ([]sdk.Option, error) {
	log := logctx.LoggerWithContext(s.log, ctx)

	opts := append([]sdk.Option{
		sdk.WithConversationID(sessionID),
	}, s.sdkOptions...)

	// Add provider based on configuration
	if s.mockProvider {
		log.Info("using mock provider for conversation")
		provider, err := s.createMockProvider()
		if err != nil {
			return nil, err
		}
		return append(opts, sdk.WithProvider(provider)), nil
	}

	// Try to create an explicit provider from config
	provider, err := s.createProviderFromConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create provider from config: %w", err)
	}
	if provider != nil {
		log.Info("using explicit provider from config", "type", s.providerType)
		opts = append(opts, sdk.WithProvider(provider))
	}
	// If provider is nil, PromptKit will auto-detect from environment

	return opts, nil
}

// resumeOrOpenConversation tries to resume an existing conversation, or opens a new one.
func (s *Server) resumeOrOpenConversation(sessionID string, opts []sdk.Option, log logr.Logger) (*sdk.Conversation, error) {
	conv, err := sdk.Resume(sessionID, s.packPath, s.promptName, opts...)
	if err != nil {
		log.V(1).Info("creating new conversation")
		conv, err = sdk.Open(s.packPath, s.promptName, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to open pack: %w", err)
		}
	} else {
		log.V(1).Info("resumed existing conversation")
	}
	return conv, nil
}

// createMockProvider creates a mock provider based on configuration.
func (s *Server) createMockProvider() (*mock.Provider, error) {
	if s.mockConfigPath != "" {
		// Use file-based mock repository
		repo, err := mock.NewFileMockRepository(s.mockConfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load mock config: %w", err)
		}
		return mock.NewProviderWithRepository("mock", "mock-model", false, repo), nil
	}
	// Use in-memory mock provider with default responses
	return mock.NewProvider("mock", "mock-model", false), nil
}

// createProviderFromConfig creates a PromptKit provider based on runtime configuration.
// This is used for explicit provider types (ollama, claude, openai, gemini).
// Returns nil, nil if provider type is empty (no provider configured).
func (s *Server) createProviderFromConfig() (providers.Provider, error) {
	// Skip if no explicit provider type
	if s.providerType == "" {
		return nil, nil
	}

	// Create provider from spec
	spec := providers.ProviderSpec{
		ID:      s.providerType,
		Type:    s.providerType,
		Model:   s.model,
		BaseURL: s.baseURL,
	}

	s.log.Info("creating explicit provider from config",
		"type", s.providerType,
		"model", s.model,
		"baseURL", s.baseURL)

	provider, err := providers.CreateProviderFromSpec(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider from spec: %w", err)
	}

	return provider, nil
}

// Mock scenario metadata key.
const (
	MetadataKeyMockScenario = "mock_scenario"
	MetadataKeyContentType  = "content_type"
)

// Default scenario identifiers.
const (
	ScenarioDefault       = "default"
	ScenarioImageAnalysis = "image-analysis"
	ScenarioAudioAnalysis = "audio-analysis"
	ScenarioDocumentQA    = "document-qa"
)

// extractMockScenario determines the mock scenario to use based on message metadata
// and content analysis. Priority:
// 1. Explicit mock_scenario in metadata
// 2. Auto-detection based on content_type metadata
// 3. Auto-detection based on content patterns
// 4. Default scenario
func extractMockScenario(metadata map[string]string, content string) string {
	// Check for explicit scenario in metadata
	if scenario, ok := metadata[MetadataKeyMockScenario]; ok && scenario != "" {
		return scenario
	}

	// Auto-detect based on content_type metadata
	if contentType, ok := metadata[MetadataKeyContentType]; ok {
		if scenario := detectScenarioFromContentType(contentType); scenario != "" {
			return scenario
		}
	}

	// Auto-detect based on content patterns
	if scenario := detectScenarioFromContent(content); scenario != "" {
		return scenario
	}

	return ScenarioDefault
}

// detectScenarioFromContentType maps content types to scenarios.
func detectScenarioFromContentType(contentType string) string {
	switch {
	case isImageContentType(contentType):
		return ScenarioImageAnalysis
	case isAudioContentType(contentType):
		return ScenarioAudioAnalysis
	case isDocumentContentType(contentType):
		return ScenarioDocumentQA
	default:
		return ""
	}
}

// detectScenarioFromContent analyzes content to detect scenarios.
// This is a fallback when metadata doesn't specify the content type.
func detectScenarioFromContent(content string) string {
	// Check for common patterns indicating multi-modal content
	// These patterns might appear in content when referencing uploaded media
	patterns := map[string]string{
		"[image:":    ScenarioImageAnalysis,
		"[audio:":    ScenarioAudioAnalysis,
		"[document:": ScenarioDocumentQA,
		"[pdf:":      ScenarioDocumentQA,
	}

	for pattern, scenario := range patterns {
		if containsPattern(content, pattern) {
			return scenario
		}
	}

	return ""
}

// isImageContentType checks if a content type represents an image.
func isImageContentType(contentType string) bool {
	imageTypes := []string{"image/", "png", "jpg", "jpeg", "gif", "webp", "svg"}
	for _, t := range imageTypes {
		if containsPattern(contentType, t) {
			return true
		}
	}
	return false
}

// isAudioContentType checks if a content type represents audio.
func isAudioContentType(contentType string) bool {
	audioTypes := []string{"audio/", "mp3", "wav", "ogg", "m4a", "flac"}
	for _, t := range audioTypes {
		if containsPattern(contentType, t) {
			return true
		}
	}
	return false
}

// isDocumentContentType checks if a content type represents a document.
func isDocumentContentType(contentType string) bool {
	docTypes := []string{"application/pdf", "pdf", "document", "text/"}
	for _, t := range docTypes {
		if containsPattern(contentType, t) {
			return true
		}
	}
	return false
}

// containsPattern performs case-insensitive substring check.
func containsPattern(s, pattern string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(pattern))
}

// registerToolsWithConversation registers all available tools with a conversation.
func (s *Server) registerToolsWithConversation(ctx context.Context, conv *sdk.Conversation) error {
	log := logctx.LoggerWithContext(s.log, ctx)

	// Get all tool descriptors from the executor
	descriptors, err := s.toolExecutor.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("failed to list tools: %w", err)
	}

	// Register each tool with the conversation using a context-aware handler
	for _, desc := range descriptors {
		toolName := desc.Name
		log.V(1).Info("registering tool with conversation", "tool", toolName)

		// Create a closure that captures the executor, descriptor, and context info
		conv.OnToolCtx(toolName, func(toolCtx context.Context, args map[string]any) (any, error) {
			// Enrich with tool name
			toolCtx = logctx.WithTool(toolCtx, toolName)
			return s.executeToolForConversation(toolCtx, toolName, args)
		})
	}

	log.Info("registered tools with conversation", "count", len(descriptors))
	return nil
}

// executeToolForConversation executes a tool call for a conversation.
func (s *Server) executeToolForConversation(ctx context.Context, toolName string, args map[string]any) (any, error) {
	log := logctx.LoggerWithContext(s.log, ctx)

	// Start tool span if tracing is enabled
	var span trace.Span
	if s.tracingProvider != nil {
		ctx, span = s.tracingProvider.StartToolSpan(ctx, toolName)
		defer span.End()
	}

	log.V(1).Info("executing tool for conversation")

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

// subscribeToEventBusMetrics subscribes to PromptKit event bus events to capture metrics.
// This allows us to observe fine-grained metrics emitted during conversation execution.
func (s *Server) subscribeToEventBusMetrics(sessionID string, conv *sdk.Conversation) {
	eventBus := conv.EventBus()
	if eventBus == nil {
		return
	}

	// Subscribe to provider call completed events to record Prometheus metrics
	eventBus.Subscribe(events.EventProviderCallCompleted, func(e *events.Event) {
		data, ok := e.Data.(*events.ProviderCallCompletedData)
		if !ok {
			return
		}

		// Record metrics to Prometheus
		if s.metrics != nil {
			s.metrics.RecordRequest(metrics.LLMRequestMetrics{
				Provider:        data.Provider,
				Model:           data.Model,
				InputTokens:     data.InputTokens,
				OutputTokens:    data.OutputTokens,
				CacheHits:       data.CachedTokens,
				CostUSD:         data.Cost,
				DurationSeconds: data.Duration.Seconds(),
				Success:         true,
			})
		}

		s.log.V(1).Info("event: provider call completed",
			"sessionID", sessionID,
			"provider", data.Provider,
			"model", data.Model,
			"inputTokens", data.InputTokens,
			"outputTokens", data.OutputTokens,
			"cachedTokens", data.CachedTokens,
			"cost", data.Cost,
			"finishReason", data.FinishReason,
			"durationMs", data.Duration.Milliseconds(),
		)
	})

	// Subscribe to provider call failed events to record failures
	eventBus.Subscribe(events.EventProviderCallFailed, func(e *events.Event) {
		data, ok := e.Data.(*events.ProviderCallFailedData)
		if !ok {
			return
		}

		// Record failed request metric
		if s.metrics != nil {
			s.metrics.RecordRequest(metrics.LLMRequestMetrics{
				Provider:        data.Provider,
				Model:           data.Model,
				DurationSeconds: data.Duration.Seconds(),
				Success:         false,
			})
		}

		s.log.V(1).Info("event: provider call failed",
			"sessionID", sessionID,
			"provider", data.Provider,
			"model", data.Model,
			"error", data.Error,
			"durationMs", data.Duration.Milliseconds(),
		)
	})

	// Subscribe to pipeline started events
	eventBus.Subscribe(events.EventPipelineStarted, func(e *events.Event) {
		// Record pipeline start for active pipeline gauge
		if s.runtimeMetrics != nil {
			s.runtimeMetrics.RecordPipelineStart()
		}

		s.log.V(1).Info("event: pipeline started",
			"sessionID", sessionID,
		)
	})

	// Subscribe to pipeline completed events for overall visibility
	eventBus.Subscribe(events.EventPipelineCompleted, func(e *events.Event) {
		data, ok := e.Data.(*events.PipelineCompletedData)
		if !ok {
			return
		}

		// Record pipeline completion metrics
		if s.runtimeMetrics != nil {
			s.runtimeMetrics.RecordPipelineEnd(metrics.PipelineMetrics{
				DurationSeconds: data.Duration.Seconds(),
				Success:         true,
			})
		}

		s.log.V(0).Info("event: pipeline completed",
			"sessionID", sessionID,
			"provider", s.providerType,
			"model", s.model,
			"totalInputTokens", data.InputTokens,
			"totalOutputTokens", data.OutputTokens,
			"totalCost", data.TotalCost,
			"messageCount", data.MessageCount,
			"durationMs", data.Duration.Milliseconds(),
		)
	})

	// Subscribe to pipeline failed events
	eventBus.Subscribe(events.EventPipelineFailed, func(e *events.Event) {
		data, ok := e.Data.(*events.PipelineFailedData)
		if !ok {
			return
		}

		// Record pipeline failure metrics
		if s.runtimeMetrics != nil {
			s.runtimeMetrics.RecordPipelineEnd(metrics.PipelineMetrics{
				DurationSeconds: data.Duration.Seconds(),
				Success:         false,
			})
		}

		s.log.V(0).Info("event: pipeline failed",
			"sessionID", sessionID,
			"error", data.Error,
			"durationMs", data.Duration.Milliseconds(),
		)
	})

	// Subscribe to stage completed events
	eventBus.Subscribe(events.EventStageCompleted, func(e *events.Event) {
		data, ok := e.Data.(*events.StageCompletedData)
		if !ok {
			return
		}

		// Record stage metrics
		if s.runtimeMetrics != nil {
			s.runtimeMetrics.RecordStage(metrics.StageMetrics{
				StageName:       data.Name,
				StageType:       data.StageType,
				DurationSeconds: data.Duration.Seconds(),
				Success:         true,
			})
		}

		s.log.V(1).Info("event: stage completed",
			"sessionID", sessionID,
			"stage", data.Name,
			"stageType", data.StageType,
			"index", data.Index,
			"durationMs", data.Duration.Milliseconds(),
		)
	})

	// Subscribe to stage failed events
	eventBus.Subscribe(events.EventStageFailed, func(e *events.Event) {
		data, ok := e.Data.(*events.StageFailedData)
		if !ok {
			return
		}

		// Record stage failure metrics
		if s.runtimeMetrics != nil {
			s.runtimeMetrics.RecordStage(metrics.StageMetrics{
				StageName:       data.Name,
				StageType:       data.StageType,
				DurationSeconds: data.Duration.Seconds(),
				Success:         false,
			})
		}

		s.log.V(1).Info("event: stage failed",
			"sessionID", sessionID,
			"stage", data.Name,
			"stageType", data.StageType,
			"index", data.Index,
			"error", data.Error,
			"durationMs", data.Duration.Milliseconds(),
		)
	})

	// Subscribe to tool call completed events (tool metrics)
	eventBus.Subscribe(events.EventToolCallCompleted, func(e *events.Event) {
		data, ok := e.Data.(*events.ToolCallCompletedData)
		if !ok {
			return
		}

		// Record tool call metrics
		if s.runtimeMetrics != nil {
			s.runtimeMetrics.RecordToolCall(metrics.ToolCallMetrics{
				ToolName:        data.ToolName,
				DurationSeconds: data.Duration.Seconds(),
				Success:         data.Status == "success",
			})
		}

		s.log.V(1).Info("event: tool call completed",
			"sessionID", sessionID,
			"toolName", data.ToolName,
			"callID", data.CallID,
			"status", data.Status,
			"durationMs", data.Duration.Milliseconds(),
		)
	})

	// Subscribe to tool call failed events
	eventBus.Subscribe(events.EventToolCallFailed, func(e *events.Event) {
		data, ok := e.Data.(*events.ToolCallFailedData)
		if !ok {
			return
		}

		// Record tool call failure metrics
		if s.runtimeMetrics != nil {
			s.runtimeMetrics.RecordToolCall(metrics.ToolCallMetrics{
				ToolName:        data.ToolName,
				DurationSeconds: data.Duration.Seconds(),
				Success:         false,
			})
		}

		s.log.V(1).Info("event: tool call failed",
			"sessionID", sessionID,
			"toolName", data.ToolName,
			"callID", data.CallID,
			"error", data.Error,
			"durationMs", data.Duration.Milliseconds(),
		)
	})

	// Subscribe to validation passed events
	eventBus.Subscribe(events.EventValidationPassed, func(e *events.Event) {
		data, ok := e.Data.(*events.ValidationPassedData)
		if !ok {
			return
		}

		// Record validation metrics
		if s.runtimeMetrics != nil {
			s.runtimeMetrics.RecordValidation(metrics.ValidationMetrics{
				ValidatorName:   data.ValidatorName,
				ValidatorType:   data.ValidatorType,
				DurationSeconds: data.Duration.Seconds(),
				Success:         true,
			})
		}

		s.log.V(1).Info("event: validation passed",
			"sessionID", sessionID,
			"validator", data.ValidatorName,
			"validatorType", data.ValidatorType,
			"durationMs", data.Duration.Milliseconds(),
		)
	})

	// Subscribe to validation failed events
	eventBus.Subscribe(events.EventValidationFailed, func(e *events.Event) {
		data, ok := e.Data.(*events.ValidationFailedData)
		if !ok {
			return
		}

		// Record validation failure metrics
		if s.runtimeMetrics != nil {
			s.runtimeMetrics.RecordValidation(metrics.ValidationMetrics{
				ValidatorName:   data.ValidatorName,
				ValidatorType:   data.ValidatorType,
				DurationSeconds: data.Duration.Seconds(),
				Success:         false,
			})
		}

		s.log.V(1).Info("event: validation failed",
			"sessionID", sessionID,
			"validator", data.ValidatorName,
			"validatorType", data.ValidatorType,
			"error", data.Error,
			"durationMs", data.Duration.Milliseconds(),
		)
	})
}

// buildSendOptions converts gRPC content parts to SDK send options.
// This enables multimodal messages (images, audio, files) to be sent to the LLM.
func buildSendOptions(parts []*runtimev1.ContentPart, log logr.Logger) []sdk.SendOption {
	if len(parts) == 0 {
		return nil
	}

	var opts []sdk.SendOption
	for _, part := range parts {
		if part.Media == nil {
			continue
		}

		opt := processMediaPart(part.Media, log)
		if opt != nil {
			opts = append(opts, opt)
		}
	}

	return opts
}

// processMediaPart converts a single media part to an SDK send option based on its type.
func processMediaPart(media *runtimev1.MediaContent, log logr.Logger) sdk.SendOption {
	switch {
	case isImageContentType(media.MimeType):
		return processImageMedia(media, log)
	case isAudioContentType(media.MimeType):
		return processAudioMedia(media, log)
	default:
		return processFileMedia(media, log)
	}
}

// processImageMedia handles image content (base64 data or URL).
func processImageMedia(media *runtimev1.MediaContent, log logr.Logger) sdk.SendOption {
	if media.Data != "" {
		data, err := decodeMediaData(media.Data)
		if err != nil {
			log.Error(err, "failed to decode image data")
			return nil
		}
		log.V(1).Info("adding image from data", "mimeType", media.MimeType, "size", len(data))
		return sdk.WithImageData(data, media.MimeType)
	}
	if media.Url != "" {
		log.V(1).Info("adding image from URL", "url", media.Url)
		return sdk.WithImageURL(media.Url)
	}
	return nil
}

// processAudioMedia handles audio content (base64 data or URL).
// For base64 data, uses sdk.WithAudioData to pass bytes directly without temp files.
func processAudioMedia(media *runtimev1.MediaContent, log logr.Logger) sdk.SendOption {
	if media.Data != "" {
		data, err := decodeMediaData(media.Data)
		if err != nil {
			log.Error(err, "failed to decode audio data")
			return nil
		}
		log.V(1).Info("adding audio from data", "mimeType", media.MimeType, "size", len(data))
		return sdk.WithAudioData(data, media.MimeType)
	}
	if media.Url != "" {
		log.V(1).Info("adding audio from URL", "url", media.Url)
		return sdk.WithAudioFile(media.Url)
	}
	return nil
}

// processFileMedia handles generic file content.
func processFileMedia(media *runtimev1.MediaContent, log logr.Logger) sdk.SendOption {
	if media.Data != "" {
		data, err := decodeMediaData(media.Data)
		if err != nil {
			log.Error(err, "failed to decode file data")
			return nil
		}
		log.V(1).Info("adding file from data", "mimeType", media.MimeType, "size", len(data))
		return sdk.WithFile(media.MimeType, data)
	}
	return nil
}

// decodeMediaData decodes base64-encoded media data.
// It handles both standard and URL-safe base64 encoding.
func decodeMediaData(data string) ([]byte, error) {
	// Try standard base64 first
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err == nil {
		return decoded, nil
	}

	// Try URL-safe base64
	decoded, err = base64.URLEncoding.DecodeString(data)
	if err == nil {
		return decoded, nil
	}

	// Try raw (no padding) base64
	decoded, err = base64.RawStdEncoding.DecodeString(data)
	if err == nil {
		return decoded, nil
	}

	return nil, fmt.Errorf("failed to decode base64 data: %w", err)
}
