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

	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// Provider defines the interface for LLM providers.
type Provider interface {
	// Chat sends a message to the LLM and streams the response.
	Chat(ctx context.Context, messages []Message, streamCh chan<- StreamEvent) error
}

// Message represents a chat message for the provider.
type Message struct {
	Role    string
	Content string
}

// StreamEvent represents an event from the LLM stream.
type StreamEvent struct {
	// Type is the event type.
	Type StreamEventType
	// Content is the text content (for Chunk events).
	Content string
	// ToolCall contains tool call information (for ToolCall events).
	ToolCall *ToolCall
	// ToolResult contains tool result information (for ToolResult events).
	ToolResult *ToolResult
	// Error contains error information (for Error events).
	Error error
	// Usage contains token usage (for Done events).
	Usage *Usage
}

// StreamEventType defines the type of stream event.
type StreamEventType int

const (
	// EventChunk indicates a text chunk.
	EventChunk StreamEventType = iota
	// EventToolCall indicates a tool call request.
	EventToolCall
	// EventToolResult indicates a tool call result.
	EventToolResult
	// EventDone indicates the stream is complete.
	EventDone
	// EventError indicates an error occurred.
	EventError
)

// ToolCall represents a tool call from the LLM.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ToolResult represents the result of a tool call.
type ToolResult struct {
	ID      string
	Result  string
	IsError bool
}

// Usage represents token usage information.
type Usage struct {
	InputTokens  int32
	OutputTokens int32
	CostUSD      float32
}

// SessionStore defines the interface for session management.
type SessionStore interface {
	// GetHistory retrieves the conversation history for a session.
	GetHistory(ctx context.Context, sessionID string) ([]Message, error)
	// AppendMessage adds a message to the session history.
	AppendMessage(ctx context.Context, sessionID string, msg Message) error
	// CreateSession creates a new session if it doesn't exist.
	CreateSession(ctx context.Context, sessionID, agentName, namespace string) error
}

// PackLoader defines the interface for loading PromptPacks.
type PackLoader interface {
	// LoadSystemPrompt loads the system prompt from the pack.
	LoadSystemPrompt() (string, error)
}

// Server implements the RuntimeService gRPC server.
type Server struct {
	runtimev1.UnimplementedRuntimeServiceServer

	log       logr.Logger
	provider  Provider
	sessions  SessionStore
	pack      PackLoader
	agentName string
	namespace string
	mu        sync.RWMutex
	healthy   bool
}

// ServerOption configures the server.
type ServerOption func(*Server)

// WithLogger sets the logger for the server.
func WithLogger(log logr.Logger) ServerOption {
	return func(s *Server) {
		s.log = log
	}
}

// WithProvider sets the LLM provider.
func WithProvider(p Provider) ServerOption {
	return func(s *Server) {
		s.provider = p
	}
}

// WithSessionStore sets the session store.
func WithSessionStore(store SessionStore) ServerOption {
	return func(s *Server) {
		s.sessions = store
	}
}

// WithPackLoader sets the pack loader.
func WithPackLoader(loader PackLoader) ServerOption {
	return func(s *Server) {
		s.pack = loader
	}
}

// WithAgentInfo sets the agent name and namespace.
func WithAgentInfo(name, namespace string) ServerOption {
	return func(s *Server) {
		s.agentName = name
		s.namespace = namespace
	}
}

// NewServer creates a new runtime server.
func NewServer(opts ...ServerOption) *Server {
	s := &Server{
		healthy: true,
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

	// Ensure session exists
	if s.sessions != nil {
		if err := s.sessions.CreateSession(ctx, sessionID, s.agentName, s.namespace); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}

	// Build message history
	var messages []Message

	// Add system prompt from pack
	if s.pack != nil {
		systemPrompt, err := s.pack.LoadSystemPrompt()
		if err != nil {
			return fmt.Errorf("failed to load system prompt: %w", err)
		}
		if systemPrompt != "" {
			messages = append(messages, Message{Role: "system", Content: systemPrompt})
		}
	}

	// Add conversation history
	if s.sessions != nil {
		history, err := s.sessions.GetHistory(ctx, sessionID)
		if err != nil {
			s.log.V(1).Info("no history for session", "sessionID", sessionID, "error", err)
		} else {
			messages = append(messages, history...)
		}
	}

	// Add the new user message
	userMsg := Message{Role: "user", Content: content}
	messages = append(messages, userMsg)

	// Save user message to session
	if s.sessions != nil {
		if err := s.sessions.AppendMessage(ctx, sessionID, userMsg); err != nil {
			s.log.Error(err, "failed to save user message", "sessionID", sessionID)
		}
	}

	// Call the LLM provider
	if s.provider == nil {
		return status.Errorf(codes.FailedPrecondition, "no provider configured")
	}

	// Create channel for stream events
	eventCh := make(chan StreamEvent, 100)

	// Start provider in goroutine
	var providerErr error
	go func() {
		providerErr = s.provider.Chat(ctx, messages, eventCh)
		close(eventCh)
	}()

	// Collect final content for session storage
	var finalContent string

	// Stream events to client
	for event := range eventCh {
		var sendErr error

		switch event.Type {
		case EventChunk:
			finalContent += event.Content
			sendErr = stream.Send(&runtimev1.ServerMessage{
				Message: &runtimev1.ServerMessage_Chunk{
					Chunk: &runtimev1.Chunk{Content: event.Content},
				},
			})

		case EventToolCall:
			if event.ToolCall != nil {
				sendErr = stream.Send(&runtimev1.ServerMessage{
					Message: &runtimev1.ServerMessage_ToolCall{
						ToolCall: &runtimev1.ToolCall{
							Id:            event.ToolCall.ID,
							Name:          event.ToolCall.Name,
							ArgumentsJson: event.ToolCall.Arguments,
						},
					},
				})
			}

		case EventToolResult:
			if event.ToolResult != nil {
				sendErr = stream.Send(&runtimev1.ServerMessage{
					Message: &runtimev1.ServerMessage_ToolResult{
						ToolResult: &runtimev1.ToolResult{
							Id:         event.ToolResult.ID,
							ResultJson: event.ToolResult.Result,
							IsError:    event.ToolResult.IsError,
						},
					},
				})
			}

		case EventDone:
			done := &runtimev1.Done{FinalContent: finalContent}
			if event.Usage != nil {
				done.Usage = &runtimev1.Usage{
					InputTokens:  event.Usage.InputTokens,
					OutputTokens: event.Usage.OutputTokens,
					CostUsd:      event.Usage.CostUSD,
				}
			}
			sendErr = stream.Send(&runtimev1.ServerMessage{
				Message: &runtimev1.ServerMessage_Done{Done: done},
			})

		case EventError:
			sendErr = stream.Send(&runtimev1.ServerMessage{
				Message: &runtimev1.ServerMessage_Error{
					Error: &runtimev1.Error{
						Code:    "PROVIDER_ERROR",
						Message: event.Error.Error(),
					},
				},
			})
		}

		if sendErr != nil {
			return fmt.Errorf("failed to send message: %w", sendErr)
		}
	}

	// Save assistant response to session
	if s.sessions != nil && finalContent != "" {
		assistantMsg := Message{Role: "assistant", Content: finalContent}
		if err := s.sessions.AppendMessage(ctx, sessionID, assistantMsg); err != nil {
			s.log.Error(err, "failed to save assistant message", "sessionID", sessionID)
		}
	}

	if providerErr != nil {
		return fmt.Errorf("provider error: %w", providerErr)
	}

	return nil
}

// WaitForReady waits for the server to be ready.
func (s *Server) WaitForReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		s.mu.RLock()
		healthy := s.healthy
		s.mu.RUnlock()

		if healthy {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			// retry
		}
	}

	return fmt.Errorf("server not ready after %v", timeout)
}
