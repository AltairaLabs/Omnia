/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/PromptKit/tools/arena/engine"
	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/pkg/logctx"
	"github.com/go-logr/logr"
)

// PromptKitHandler implements facade.MessageHandler using a local PromptKit engine.
// It supports dynamic reload of the configuration without dropping the WebSocket connection.
type PromptKitHandler struct {
	mu               sync.RWMutex
	config           *config.Config
	providerRegistry *providers.Registry
	log              logr.Logger

	// Session state for conversations
	sessions map[string]*SessionState

	// K8s provider loading (optional, for dynamic namespace-based provider resolution)
	k8sLoader *K8sProviderLoader
	// Cache of provider registries per namespace
	nsRegistries map[string]*providers.Registry
}

// SessionState holds conversation state for a session.
type SessionState struct {
	Messages   []types.Message
	ProviderID string // Selected provider for this session
	mu         sync.Mutex
}

// NewPromptKitHandler creates a new handler with the given configuration.
func NewPromptKitHandler(cfg *config.Config, log logr.Logger) (*PromptKitHandler, error) {
	h := &PromptKitHandler{
		config:       cfg,
		log:          log.WithName("promptkit-handler"),
		sessions:     make(map[string]*SessionState),
		nsRegistries: make(map[string]*providers.Registry),
	}

	// Try to initialize K8s provider loader (will fail if not in cluster, which is ok)
	k8sLoader, err := NewK8sProviderLoader(log)
	if err != nil {
		log.Info("K8s provider loader not available (may be running outside cluster)", "error", err.Error())
	} else {
		h.k8sLoader = k8sLoader
		log.Info("K8s provider loader initialized for dynamic provider resolution")
	}

	// Build the components (only if we have a static config)
	if cfg != nil && len(cfg.LoadedProviders) > 0 {
		if err := h.buildComponents(); err != nil {
			return nil, fmt.Errorf("failed to build components: %w", err)
		}
	}

	return h, nil
}

// Name returns the handler name for metrics labeling.
func (h *PromptKitHandler) Name() string {
	return "promptkit"
}

// HandleMessage processes a client message and streams responses via the ResponseWriter.
func (h *PromptKitHandler) HandleMessage(
	ctx context.Context,
	sessionID string,
	msg *facade.ClientMessage,
	writer facade.ResponseWriter,
) error {
	// Extract namespace from context for K8s provider resolution
	namespace := logctx.Namespace(ctx)

	// Get registry and config (potentially namespace-specific)
	registry, cfg, err := h.getRegistryAndConfig(ctx, namespace)
	if err != nil {
		return writer.WriteError("PROVIDER_LOAD_ERROR", err.Error())
	}

	if registry == nil {
		return writer.WriteError("ENGINE_NOT_READY", "PromptKit engine is not initialized. No providers available.")
	}

	// Get or create session state
	session := h.getOrCreateSession(sessionID)

	// Check for special commands in metadata
	if msg.Metadata != nil {
		if _, isReload := msg.Metadata["reload"]; isReload {
			return h.handleReload(ctx, msg, writer)
		}
		if _, isReset := msg.Metadata["reset"]; isReset {
			h.ResetSession(sessionID)
			return writer.WriteDone("Session reset")
		}
		if providerID, ok := msg.Metadata["provider"]; ok {
			session.mu.Lock()
			session.ProviderID = providerID
			session.mu.Unlock()
		}
	}

	// Build user message
	userMsg := types.NewUserMessage(msg.Content)

	// Handle multimodal content
	if len(msg.Parts) > 0 {
		userMsg = h.convertToPKMessage("user", msg.Parts)
	}

	// Add user message to history
	session.mu.Lock()
	session.Messages = append(session.Messages, userMsg)
	messages := make([]types.Message, len(session.Messages))
	copy(messages, session.Messages)
	providerID := session.ProviderID
	session.mu.Unlock()

	// Determine which provider to use
	if providerID == "" {
		// Use first available provider
		for id := range cfg.LoadedProviders {
			providerID = id
			break
		}
	}

	if providerID == "" {
		return writer.WriteError("NO_PROVIDER", "No provider configured")
	}

	// Get provider from registry
	provider, ok := registry.Get(providerID)
	if !ok {
		return writer.WriteError("PROVIDER_ERROR", fmt.Sprintf("Provider not found: %s", providerID))
	}

	// Build prediction request
	req := providers.PredictionRequest{
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   4096,
	}

	// Apply provider defaults if available
	if p, ok := cfg.LoadedProviders[providerID]; ok {
		req.Temperature = p.Defaults.Temperature
		if p.Defaults.MaxTokens > 0 {
			req.MaxTokens = p.Defaults.MaxTokens
		}
	}

	// Execute with streaming
	response, err := h.executeStreaming(ctx, provider, req, writer)
	if err != nil {
		h.log.Error(err, "prediction failed", "sessionID", sessionID)
		return writer.WriteError("EXECUTION_ERROR", err.Error())
	}

	// Add assistant response to history
	session.mu.Lock()
	session.Messages = append(session.Messages, types.NewAssistantMessage(response))
	session.mu.Unlock()

	return nil
}

// executeStreaming runs a streaming prediction and forwards chunks to the writer.
func (h *PromptKitHandler) executeStreaming(
	ctx context.Context,
	provider providers.Provider,
	req providers.PredictionRequest,
	writer facade.ResponseWriter,
) (string, error) {
	if !provider.SupportsStreaming() {
		// Fall back to non-streaming
		resp, err := provider.Predict(ctx, req)
		if err != nil {
			return "", err
		}
		if err := writer.WriteDone(resp.Content); err != nil {
			return "", err
		}
		return resp.Content, nil
	}

	// Stream the response
	stream, err := provider.PredictStream(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to start stream: %w", err)
	}

	var fullContent string
	for chunk := range stream {
		if chunk.Error != nil {
			return "", chunk.Error
		}

		// Send delta as chunk
		if chunk.Delta != "" {
			if err := writer.WriteChunk(chunk.Delta); err != nil {
				h.log.Error(err, "failed to write chunk")
			}
		}

		// Handle tool calls
		if len(chunk.ToolCalls) > 0 {
			for _, tc := range chunk.ToolCalls {
				args := make(map[string]interface{})
				if len(tc.Args) > 0 {
					_ = json.Unmarshal(tc.Args, &args)
				}
				if err := writer.WriteToolCall(&facade.ToolCallInfo{
					ID:        tc.ID,
					Name:      tc.Name,
					Arguments: args,
				}); err != nil {
					h.log.Error(err, "failed to write tool call")
				}
			}
		}

		fullContent = chunk.Content

		// Check for completion
		if chunk.FinishReason != nil {
			break
		}
	}

	// Signal completion
	if err := writer.WriteDone(fullContent); err != nil {
		return "", fmt.Errorf("failed to write done: %w", err)
	}

	return fullContent, nil
}

// convertToPKMessage converts facade content parts to a PromptKit message.
func (h *PromptKitHandler) convertToPKMessage(role string, parts []facade.ContentPart) types.Message {
	msg := types.Message{Role: role}

	for _, part := range parts {
		switch part.Type {
		case facade.ContentPartTypeText:
			msg.AddTextPart(part.Text)
		case facade.ContentPartTypeImage:
			if part.Media != nil {
				if part.Media.URL != "" {
					msg.AddImagePartFromURL(part.Media.URL, nil)
				} else if part.Media.Data != "" {
					// Create image part from base64 data
					imagePart := types.NewImagePartFromData(part.Media.Data, part.Media.MimeType, nil)
					msg.AddPart(imagePart)
				}
			}
		case facade.ContentPartTypeAudio:
			if part.Media != nil && part.Media.Data != "" {
				audioPart := types.NewAudioPartFromData(part.Media.Data, part.Media.MimeType)
				msg.AddPart(audioPart)
			}
		case facade.ContentPartTypeVideo:
			if part.Media != nil && part.Media.Data != "" {
				videoPart := types.NewVideoPartFromData(part.Media.Data, part.Media.MimeType)
				msg.AddPart(videoPart)
			}
		}
	}

	return msg
}

// handleReload reloads the engine configuration.
func (h *PromptKitHandler) handleReload(
	_ context.Context,
	msg *facade.ClientMessage,
	writer facade.ResponseWriter,
) error {
	// Parse new configuration from message content
	var newConfig config.Config
	if err := json.Unmarshal([]byte(msg.Content), &newConfig); err != nil {
		return writer.WriteError("INVALID_CONFIG", fmt.Sprintf("failed to parse config: %v", err))
	}

	h.mu.Lock()
	h.config = &newConfig
	h.mu.Unlock()

	// Rebuild components
	if err := h.buildComponents(); err != nil {
		return writer.WriteError("RELOAD_ERROR", fmt.Sprintf("failed to rebuild components: %v", err))
	}

	h.log.Info("configuration reloaded successfully")
	return writer.WriteDone("Configuration reloaded successfully")
}

// Reload updates the configuration and rebuilds components.
// This is called externally (e.g., from file watcher).
func (h *PromptKitHandler) Reload(cfg *config.Config) error {
	h.mu.Lock()
	h.config = cfg
	h.mu.Unlock()

	return h.buildComponents()
}

// ReloadFromPath loads configuration from a file path and reloads.
func (h *PromptKitHandler) ReloadFromPath(configPath string) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	return h.Reload(cfg)
}

// getRegistryAndConfig returns the provider registry and config.
// If K8s provider loading is enabled, it loads providers from the pod's namespace.
// Otherwise, it falls back to the static configuration.
func (h *PromptKitHandler) getRegistryAndConfig(
	ctx context.Context,
	_ string,
) (*providers.Registry, *config.Config, error) {
	// If K8s loading is enabled, use dynamic loading from the pod's namespace
	if h.k8sLoader != nil {
		return h.getOrLoadK8sRegistry(ctx)
	}

	// Fall back to static config
	h.mu.RLock()
	registry := h.providerRegistry
	cfg := h.config
	h.mu.RUnlock()

	return registry, cfg, nil
}

// getOrLoadK8sRegistry returns a cached registry or loads providers from K8s.
// The dev console only accesses providers in its own namespace for security.
func (h *PromptKitHandler) getOrLoadK8sRegistry(ctx context.Context) (*providers.Registry, *config.Config, error) {
	namespace := h.k8sLoader.Namespace()

	h.mu.RLock()
	if registry, ok := h.nsRegistries[namespace]; ok {
		h.mu.RUnlock()
		return registry, h.config, nil
	}
	h.mu.RUnlock()

	// Load providers from K8s (only from this pod's namespace)
	h.log.Info("loading providers from K8s", "namespace", namespace)
	loadedProviders, err := h.k8sLoader.LoadProviders(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load providers: %w", err)
	}

	if len(loadedProviders) == 0 {
		h.log.Info("no providers found in namespace, falling back to static config", "namespace", namespace)
		h.mu.RLock()
		registry := h.providerRegistry
		cfg := h.config
		h.mu.RUnlock()
		return registry, cfg, nil
	}

	// Build config from loaded providers
	cfg := BuildConfigFromProviders(loadedProviders)

	// Build registry from providers
	registry, _, _, _, _, err := engine.BuildEngineComponents(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build provider registry: %w", err)
	}

	// Cache the registry
	h.mu.Lock()
	h.nsRegistries[namespace] = registry
	h.mu.Unlock()

	h.log.Info("loaded providers from K8s", "namespace", namespace, "count", len(loadedProviders))
	return registry, cfg, nil
}

// InvalidateProviderCache invalidates the cached provider registry.
// Call this when providers in the namespace change.
func (h *PromptKitHandler) InvalidateProviderCache() {
	if h.k8sLoader == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	namespace := h.k8sLoader.Namespace()
	if registry, ok := h.nsRegistries[namespace]; ok {
		_ = registry.Close()
		delete(h.nsRegistries, namespace)
	}
}

// buildComponents creates the PromptKit components from configuration.
func (h *PromptKitHandler) buildComponents() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	cfg := h.config
	if cfg == nil {
		return fmt.Errorf("no configuration provided")
	}

	// Build engine components using the same pattern as arena-worker
	providerRegistry, _, _, _, _, err := engine.BuildEngineComponents(cfg)
	if err != nil {
		return fmt.Errorf("failed to build engine components: %w", err)
	}

	h.providerRegistry = providerRegistry
	h.log.Info("components built successfully")
	return nil
}

// getOrCreateSession gets or creates session state for the given session ID.
func (h *PromptKitHandler) getOrCreateSession(sessionID string) *SessionState {
	h.mu.Lock()
	defer h.mu.Unlock()

	if session, ok := h.sessions[sessionID]; ok {
		return session
	}

	session := &SessionState{
		Messages: make([]types.Message, 0),
	}
	h.sessions[sessionID] = session
	return session
}

// ResetSession clears the conversation history for a session.
func (h *PromptKitHandler) ResetSession(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if session, ok := h.sessions[sessionID]; ok {
		session.mu.Lock()
		session.Messages = make([]types.Message, 0)
		session.mu.Unlock()
	}
}

// GetSessionHistory returns the message history for a session.
func (h *PromptKitHandler) GetSessionHistory(sessionID string) []types.Message {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if session, ok := h.sessions[sessionID]; ok {
		session.mu.Lock()
		defer session.mu.Unlock()
		messages := make([]types.Message, len(session.Messages))
		copy(messages, session.Messages)
		return messages
	}
	return nil
}

// ListProviders returns the list of available provider IDs.
func (h *PromptKitHandler) ListProviders() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.providerRegistry == nil {
		return nil
	}
	return h.providerRegistry.List()
}

// Close shuts down the handler and releases resources.
func (h *PromptKitHandler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	var lastErr error

	// Close all namespace registries
	for ns, registry := range h.nsRegistries {
		if err := registry.Close(); err != nil {
			h.log.Error(err, "failed to close namespace registry", "namespace", ns)
			lastErr = err
		}
	}
	h.nsRegistries = make(map[string]*providers.Registry)

	// Close main registry
	if h.providerRegistry != nil {
		if err := h.providerRegistry.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Interface assertion
var _ facade.MessageHandler = (*PromptKitHandler)(nil)
