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

	"github.com/go-logr/logr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/AltairaLabs/PromptKit/runtime/providers/mock"
	"github.com/AltairaLabs/PromptKit/runtime/statestore"
	"github.com/AltairaLabs/PromptKit/sdk"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
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
	// TODO: Implement streaming when SDK supports it
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
	if resp.TokensUsed() > 0 {
		usage = &runtimev1.Usage{
			InputTokens:  int32(resp.InputTokens()),
			OutputTokens: int32(resp.OutputTokens()),
			CostUsd:      float32(resp.Cost()),
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

	s.conversations[sessionID] = conv
	return conv, nil
}

// Close closes all open conversations.
func (s *Server) Close() error {
	s.conversationMu.Lock()
	defer s.conversationMu.Unlock()

	for id, conv := range s.conversations {
		if err := conv.Close(); err != nil {
			s.log.Error(err, "failed to close conversation", "sessionID", id)
		}
	}
	s.conversations = make(map[string]*sdk.Conversation)
	return nil
}
