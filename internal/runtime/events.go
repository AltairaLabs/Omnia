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

	"github.com/altairalabs/omnia/pkg/metrics"
)

// subscribeToEventBusMetrics subscribes to PromptKit event bus events to capture metrics.
// This allows us to observe fine-grained metrics emitted during conversation execution.
func (s *Server) subscribeToEventBusMetrics(sessionID string, conv *sdk.Conversation) {
	eventBus := conv.EventBus()
	if eventBus == nil {
		s.log.V(1).Info("event bus unavailable",
			"sessionID", sessionID)
		return
	}

	s.log.V(1).Info("event bus subscribed",
		"sessionID", sessionID,
		"hasMetrics", s.metrics != nil,
		"hasRuntimeMetrics", s.runtimeMetrics != nil)

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
