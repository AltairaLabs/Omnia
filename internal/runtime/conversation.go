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

	"github.com/AltairaLabs/PromptKit/sdk"
	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/pkg/logctx"
)

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

	// Wire eval middleware when collector is configured
	opts = append(opts, s.buildEvalOptions()...)

	// Wire tracing provider into SDK for span propagation
	if s.tracingProvider != nil {
		opts = append(opts, sdk.WithTracerProvider(s.tracingProvider.TracerProvider()))
	}

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
