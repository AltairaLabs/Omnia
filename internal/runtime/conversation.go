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
	"log/slog"
	"time"

	"github.com/AltairaLabs/PromptKit/sdk"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/trace"

	"github.com/altairalabs/omnia/pkg/logctx"
	"github.com/altairalabs/omnia/pkg/policy"
)

// toolCallExecutionTimeout is the pipeline execution timeout when tools are
// configured. The PromptKit default (30s) is too short for multi-round
// tool-calling where each round involves LLM inference + tool execution.
const toolCallExecutionTimeout = 120 * time.Second

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

	log.V(1).Info("conversation creating",
		"sdkOptionsCount", len(opts),
		"hasEvalCollector", s.evalCollector != nil,
		"evalDefCount", len(s.evalDefs))

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

	// Subscribe to event bus logging for observability
	s.subscribeToEventBusLogging(sessionID, conv)

	return conv, nil
}

// buildConversationOptions builds the SDK options for a conversation, including provider setup.
func (s *Server) buildConversationOptions(ctx context.Context, sessionID string) ([]sdk.Option, error) {
	log := logctx.LoggerWithContext(s.log, ctx)

	opts := append([]sdk.Option{
		sdk.WithConversationID(sessionID),
	}, s.sdkOptions...)

	// Pass Omnia's logger to the SDK so all output flows through the same Zap backend.
	// Enrich with trace_id so PromptKit logs correlate with the active trace.
	if s.sdkLogger != nil {
		sdkLog := s.sdkLogger
		if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
			sdkLog = sdkLog.With(slog.String("trace_id", sc.TraceID().String()))
		}
		opts = append(opts, sdk.WithLogger(sdkLog))
	}

	// Add provider based on configuration
	if s.mockProvider {
		log.Info("using mock provider for conversation")
		provider, err := s.createMockProvider()
		if err != nil {
			return nil, err
		}
		opts = append(opts, sdk.WithProvider(provider))
	} else {
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
	}

	// Wire each resolved non-default provider to its role's SDK option.
	opts = append(opts, s.extraProviderOptions(log)...)

	// Function-mode response-format constraint (#1483). resolveResponseFormat
	// returns nil for agent mode and for outputFormat "text", so this is a
	// no-op outside constrained function invocations.
	if rf := resolveResponseFormat(s.mode, s.outputFormat, s.outputSchemaJSON, s.agentName); rf != nil {
		opts = append(opts, sdk.WithResponseFormat(rf))
		log.V(1).Info("response format wired",
			"mode", s.mode,
			"outputFormat", s.outputFormat,
			"formatType", string(rf.Type))
	}

	// Wire eval middleware when collector is configured
	evalOpts := s.buildEvalOptions()
	log.V(1).Info("eval options wired",
		"evalOptionsCount", len(evalOpts),
		"hasEvalCollector", s.evalCollector != nil,
		"evalDefCount", len(s.evalDefs))
	opts = append(opts, evalOpts...)

	// Wire event store for session recording (Pattern C).
	if s.sessionStore != nil {
		eventStore := NewOmniaEventStore(s.sessionStore, s.log)
		eventStore.SetSessionID(sessionID)
		eventStore.SetAgentMeta(AgentMeta{
			AgentName:         s.agentName,
			Namespace:         s.namespace,
			PromptPackName:    s.promptPackName,
			PromptPackVersion: s.promptPackVersion,
			ProviderName:      s.providerRefName,
		})
		if s.toolExecutor != nil {
			eventStore.SetToolMetaFn(s.toolExecutor.GetToolMeta)
		}
		opts = append(opts, sdk.WithEventStore(eventStore))
		log.V(1).Info("event store wired",
			"hasSessionStore", s.sessionStore != nil)
	}

	// Wire memory store for cross-session memory (via memory-api HTTP).
	//
	// Two independent axes from the CRD (spec.memory.retrieval.enabled and
	// spec.memory.tools.enabled):
	//   - retrieval (ambient RAG): attach the CompositeRetriever so PromptKit's
	//     MemoryRetrievalStage auto-injects the user's profile + a per-turn
	//     similarity search into the prompt.
	//   - tools: expose memory__remember / memory__recall to the LLM.
	// PromptKit's memory capability always registers the tools when a store is
	// passed, so tools-off feeds the executor a no-op store (writes discarded,
	// reads empty) while the retriever keeps the real store. See #1517 and
	// AltairaLabs/PromptKit#1427.
	if s.memoryStore != nil && s.workspaceUID != "" && (s.memoryRetrievalEnabled || s.memoryToolsEnabled) {
		scope := map[string]string{
			"workspace_id": s.workspaceUID,
		}
		if uid := policy.UserID(ctx); uid != "" {
			scope["user_id"] = uid // Already pseudonymized by the facade
		}
		if s.agentUID != "" {
			scope["agent_id"] = s.agentUID
		}

		// Pick the executor store (real vs no-op for tools-off) and whether to
		// attach the ambient retriever.
		executorStore, attachRetriever := memoryWiring(s.memoryStore, s.memoryRetrievalEnabled, s.memoryToolsEnabled)

		memOpts := []sdk.MemoryOption{}
		if attachRetriever {
			// Strategy, denyCEL, and limit are threaded from spec.memory.retrieval.
			retriever := NewCompositeRetriever(s.memoryStore, RetrievalConfig{
				Strategy:    s.memoryStrategy,
				DenyCEL:     s.memoryDenyCEL,
				WorkspaceID: s.workspaceUID,
				Limit:       s.memoryLimit,
			}, log)
			memOpts = append(memOpts, sdk.WithMemoryRetriever(retriever))
		}
		opts = append(opts, sdk.WithMemory(executorStore, scope, memOpts...))

		// Teach the LLM Omnia's memory model (tiers, structured dedup,
		// purpose/title/summary) via the tool-calling instructions — only when
		// the tools are actually active; no point describing no-op tools.
		if s.memoryToolsEnabled {
			opts = append(opts, memoryToolOverrides()...)
		}
		log.V(1).Info("memory store wired",
			"session_id", sessionID,
			"trace_id", sessionID,
			"hasUserID", scope["user_id"] != "",
			"hasAgentID", scope["agent_id"] != "",
			"scopeKeys", len(scope),
			"retrievalEnabled", s.memoryRetrievalEnabled,
			"toolsEnabled", s.memoryToolsEnabled,
		)
	}

	// Wire tracing provider into SDK for span propagation
	if s.tracingProvider != nil {
		opts = append(opts, sdk.WithTracerProvider(s.tracingProvider.TracerProvider()))
	}

	// Increase pipeline execution timeout when tools are configured.
	// The default 30s is too short for multi-round tool-calling with
	// slower providers (e.g. Ollama). Each round involves LLM inference
	// + tool execution, so we allow 120s.
	if s.toolsInitialized && s.toolExecutor != nil {
		opts = append(opts, sdk.WithExecutionTimeout(toolCallExecutionTimeout))
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
