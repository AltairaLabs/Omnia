/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package facade

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/altairalabs/omnia/internal/session"
)

// EventBusEvent represents an event from PromptKit's EventBus.
// This mirrors PromptKit's event types and will be replaced by the actual
// PromptKit types when the SDK is integrated.
type EventBusEvent struct {
	Type      string          `json:"type"` // "provider.call", "tool.execute", "validation", "recording.message"
	SessionID string          `json:"sessionId"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// Supported event types from PromptKit's EventBus.
const (
	EventTypeProviderCall     = "provider.call"
	EventTypeToolExecute      = "tool.execute"
	EventTypeValidation       = "validation"
	EventTypeRecordingMessage = "recording.message"
)

// EventBridgeSessionClient defines the subset of session.Store that the
// EventBridge needs. This allows decoupling from the full Store interface
// and simplifies testing.
type EventBridgeSessionClient interface {
	AppendMessage(ctx context.Context, sessionID string, msg session.Message) error
	UpdateSessionStats(ctx context.Context, sessionID string, update session.SessionStatsUpdate) error
}

// EventBridge bridges PromptKit EventBus events to Omnia's session store.
// For PromptKit agents, this provides richer session recordings than the
// facade's recordingResponseWriter alone.
type EventBridge struct {
	sessionClient EventBridgeSessionClient
	agentName     string
	namespace     string
	logger        *slog.Logger

	mu      sync.RWMutex
	enabled bool
}

// NewEventBridge creates a bridge that forwards EventBus events to session-api.
func NewEventBridge(sessionClient EventBridgeSessionClient, agentName, namespace string, logger *slog.Logger) *EventBridge {
	return &EventBridge{
		sessionClient: sessionClient,
		agentName:     agentName,
		namespace:     namespace,
		logger:        logger.With("component", "event-bridge"),
		enabled:       false,
	}
}

// SetEnabled enables or disables the bridge.
func (b *EventBridge) SetEnabled(enabled bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.enabled = enabled
}

// IsEnabled returns whether the bridge is currently enabled.
func (b *EventBridge) IsEnabled() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.enabled
}

// HandleEvent processes an EventBus event and forwards enriched data
// to the session store. Returns nil immediately if the bridge is disabled.
func (b *EventBridge) HandleEvent(ctx context.Context, event EventBusEvent) error {
	if !b.IsEnabled() {
		return nil
	}

	if event.SessionID == "" {
		return fmt.Errorf("event missing session ID")
	}

	msg, statsUpdate := b.buildMessageAndStats(event)

	if err := b.sessionClient.AppendMessage(ctx, event.SessionID, msg); err != nil {
		b.logger.ErrorContext(ctx, "failed to append event message",
			"error", err,
			"eventType", event.Type,
			"sessionID", event.SessionID,
		)
		return fmt.Errorf("failed to append message: %w", err)
	}

	if err := b.sessionClient.UpdateSessionStats(ctx, event.SessionID, statsUpdate); err != nil {
		b.logger.ErrorContext(ctx, "failed to update session stats",
			"error", err,
			"eventType", event.Type,
			"sessionID", event.SessionID,
		)
		return fmt.Errorf("failed to update session stats: %w", err)
	}

	return nil
}

// buildMessageAndStats converts an EventBusEvent into a session message
// and a stats update.
func (b *EventBridge) buildMessageAndStats(event EventBusEvent) (session.Message, session.SessionStatsUpdate) {
	metadata := map[string]string{
		"type":       "event_bridge",
		"event_type": event.Type,
		"agent":      b.agentName,
		"namespace":  b.namespace,
	}

	content := string(event.Data)
	if content == "" {
		content = "{}"
	}

	ts := event.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	msg := session.Message{
		ID:        uuid.New().String(),
		Role:      session.RoleSystem,
		Content:   content,
		Timestamp: ts,
		Metadata:  metadata,
	}

	statsUpdate := session.SessionStatsUpdate{
		AddMessages: 1,
	}

	return msg, statsUpdate
}
