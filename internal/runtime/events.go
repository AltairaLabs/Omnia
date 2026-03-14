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
	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/AltairaLabs/PromptKit/sdk"
)

// subscribeToEventBusLogging subscribes to PromptKit event bus events to capture structured logs.
// Pipeline and provider metrics are handled by the PromptKit Collector via EventBus subscription.
// Unsubscribe functions are stored in s.unsubscribeFns[sessionID] and called when the
// conversation is removed, preventing leaked subscriptions.
func (s *Server) subscribeToEventBusLogging(sessionID string, conv *sdk.Conversation) {
	eventBus := conv.EventBus()
	if eventBus == nil {
		s.log.V(1).Info("event bus unavailable",
			"sessionID", sessionID)
		return
	}

	s.log.V(1).Info("event bus subscribed",
		"sessionID", sessionID)

	var unsubs []func()

	// Subscribe to provider call completed events for logging
	unsubs = append(unsubs, eventBus.Subscribe(events.EventProviderCallCompleted, func(e *events.Event) {
		data, ok := asPtr[events.ProviderCallCompletedData](e.Data)
		if !ok {
			return
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
	}))

	// Subscribe to provider call failed events for logging
	unsubs = append(unsubs, eventBus.Subscribe(events.EventProviderCallFailed, func(e *events.Event) {
		data, ok := asPtr[events.ProviderCallFailedData](e.Data)
		if !ok {
			return
		}

		s.log.V(1).Info("event: provider call failed",
			"sessionID", sessionID,
			"provider", data.Provider,
			"model", data.Model,
			"error", data.Error,
			"durationMs", data.Duration.Milliseconds(),
		)
	}))

	// Subscribe to pipeline started events for logging
	unsubs = append(unsubs, eventBus.Subscribe(events.EventPipelineStarted, func(_ *events.Event) {
		s.log.V(1).Info("event: pipeline started",
			"sessionID", sessionID,
		)
	}))

	// Subscribe to pipeline completed events for logging
	unsubs = append(unsubs, eventBus.Subscribe(events.EventPipelineCompleted, func(e *events.Event) {
		data, ok := asPtr[events.PipelineCompletedData](e.Data)
		if !ok {
			return
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
	}))

	// Subscribe to pipeline failed events for logging
	unsubs = append(unsubs, eventBus.Subscribe(events.EventPipelineFailed, func(e *events.Event) {
		data, ok := asPtr[events.PipelineFailedData](e.Data)
		if !ok {
			return
		}

		s.log.V(0).Info("event: pipeline failed",
			"sessionID", sessionID,
			"error", data.Error,
			"durationMs", data.Duration.Milliseconds(),
		)
	}))

	// Subscribe to stage completed events for logging
	unsubs = append(unsubs, eventBus.Subscribe(events.EventStageCompleted, func(e *events.Event) {
		data, ok := asPtr[events.StageCompletedData](e.Data)
		if !ok {
			return
		}

		s.log.V(1).Info("event: stage completed",
			"sessionID", sessionID,
			"stage", data.Name,
			"stageType", data.StageType,
			"index", data.Index,
			"durationMs", data.Duration.Milliseconds(),
		)
	}))

	// Subscribe to stage failed events for logging
	unsubs = append(unsubs, eventBus.Subscribe(events.EventStageFailed, func(e *events.Event) {
		data, ok := asPtr[events.StageFailedData](e.Data)
		if !ok {
			return
		}

		s.log.V(1).Info("event: stage failed",
			"sessionID", sessionID,
			"stage", data.Name,
			"stageType", data.StageType,
			"index", data.Index,
			"error", data.Error,
			"durationMs", data.Duration.Milliseconds(),
		)
	}))

	// Subscribe to tool call completed events for logging
	unsubs = append(unsubs, eventBus.Subscribe(events.EventToolCallCompleted, func(e *events.Event) {
		data, ok := asPtr[events.ToolCallCompletedData](e.Data)
		if !ok {
			return
		}

		s.log.V(1).Info("event: tool call completed",
			"sessionID", sessionID,
			"toolName", data.ToolName,
			"callID", data.CallID,
			"status", data.Status,
			"durationMs", data.Duration.Milliseconds(),
		)
	}))

	// Subscribe to tool call failed events for logging
	unsubs = append(unsubs, eventBus.Subscribe(events.EventToolCallFailed, func(e *events.Event) {
		data, ok := asPtr[events.ToolCallFailedData](e.Data)
		if !ok {
			return
		}

		s.log.V(1).Info("event: tool call failed",
			"sessionID", sessionID,
			"toolName", data.ToolName,
			"callID", data.CallID,
			"error", data.Error,
			"durationMs", data.Duration.Milliseconds(),
		)
	}))

	// Subscribe to validation passed events for logging
	unsubs = append(unsubs, eventBus.Subscribe(events.EventValidationPassed, func(e *events.Event) {
		data, ok := asPtr[events.ValidationPassedData](e.Data)
		if !ok {
			return
		}

		s.log.V(1).Info("event: validation passed",
			"sessionID", sessionID,
			"validator", data.ValidatorName,
			"validatorType", data.ValidatorType,
			"durationMs", data.Duration.Milliseconds(),
		)
	}))

	// Subscribe to validation failed events for logging
	unsubs = append(unsubs, eventBus.Subscribe(events.EventValidationFailed, func(e *events.Event) {
		data, ok := asPtr[events.ValidationFailedData](e.Data)
		if !ok {
			return
		}

		s.log.V(1).Info("event: validation failed",
			"sessionID", sessionID,
			"validator", data.ValidatorName,
			"validatorType", data.ValidatorType,
			"error", data.Error,
			"durationMs", data.Duration.Milliseconds(),
		)
	}))

	// Store unsubscribe functions for cleanup when the conversation ends.
	// NOTE: caller (getOrCreateConversation) already holds conversationMu write lock.
	s.unsubscribeFns[sessionID] = unsubs
}
