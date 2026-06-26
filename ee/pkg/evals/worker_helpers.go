/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"fmt"
	"time"

	sdkmetrics "github.com/AltairaLabs/PromptKit/runtime/metrics"
	"go.opentelemetry.io/otel/trace"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
	redisprovider "github.com/altairalabs/omnia/internal/session/providers/redis"
)

// EvalCollector returns the unified PromptKit metrics Collector for per-eval-name metrics.
// The caller (main.go) uses the Collector's Registry() for merged Prometheus gathering.
func (w *EvalWorker) EvalCollector() *sdkmetrics.Collector {
	return w.getSDKRunner().EvalCollector()
}

// getMessages reads session messages from the Redis hot tier.
func (w *EvalWorker) getMessages(ctx context.Context, sessionID string) ([]session.Message, error) {
	ptrMsgs, err := w.getMessageStore().GetRecentMessages(ctx, sessionID, 0)
	if err != nil {
		return nil, fmt.Errorf("get messages from redis: %w", err)
	}
	messages := make([]session.Message, 0, len(ptrMsgs))
	for _, m := range ptrMsgs {
		if m != nil {
			messages = append(messages, *m)
		}
	}
	return messages, nil
}

// convertToEvalResults converts SDK result items into persistable EvalResults.
func (w *EvalWorker) convertToEvalResults(
	items []api.EvaluateResultItem,
	event api.SessionEvent,
	agentName string,
) []*api.EvalResult {
	results := make([]*api.EvalResult, 0, len(items))
	for _, item := range items {
		result := toEvalResult(item, event, agentName)
		results = append(results, result)
	}
	return results
}

// getSDKRunner returns the SDK runner, initializing a default one if needed.
func (w *EvalWorker) getSDKRunner() *SDKRunner {
	if w.sdkRunner == nil {
		w.sdkRunner = NewSDKRunner()
	}
	return w.sdkRunner
}

// TracerProvider returns the tracer provider, if any. Exported for testing.
func (w *EvalWorker) TracerProvider() trace.TracerProvider {
	if w.sdkRunner != nil {
		return w.sdkRunner.tracerProvider
	}
	return nil
}

// getMessageStore returns the message store, initializing from the redis client if needed.
func (w *EvalWorker) getMessageStore() MessageStore {
	if w.messageStore == nil {
		w.messageStore = redisprovider.NewFromClient(w.redisClient, redisprovider.DefaultOptions())
	}
	return w.messageStore
}

// getResultWriter returns the result writer.
func (w *EvalWorker) getResultWriter() EvalResultWriter {
	return w.resultWriter
}

// getMetrics returns the metrics recorder, initializing a no-op one if needed.
func (w *EvalWorker) getMetrics() WorkerMetricsRecorder {
	if w.metrics == nil {
		w.metrics = &NoOpWorkerMetrics{}
	}
	return w.metrics
}

// getRateLimiter returns the rate limiter, initializing a default one if needed.
func (w *EvalWorker) getRateLimiter() *RateLimiter {
	if w.rateLimiter == nil {
		w.rateLimiter = NewRateLimiter(nil)
	}
	return w.rateLimiter
}

// countAssistantMessages counts the number of assistant messages to derive turn index.
func countAssistantMessages(messages []session.Message) int {
	count := 0
	for _, m := range messages {
		if m.Role == session.RoleAssistant {
			count++
		}
	}
	return count
}

// toEvalResult converts an EvaluateResultItem to an EvalResult for persistence.
func toEvalResult(item api.EvaluateResultItem, event api.SessionEvent, agentName string) *api.EvalResult {
	result := &api.EvalResult{
		SessionID:         event.SessionID,
		MessageID:         event.MessageID,
		AgentName:         agentName,
		Namespace:         event.Namespace,
		PromptPackName:    event.PromptPackName,
		PromptPackVersion: event.PromptPackVersion,
		EvalID:            item.EvalID,
		EvalType:          item.EvalType,
		Trigger:           item.Trigger,
		Passed:            item.Passed,
		Score:             item.Score,
		Details:           item.Details,
		Source:            evalSource,
		CreatedAt:         time.Now(),
	}

	if item.DurationMs > 0 {
		d := item.DurationMs
		result.DurationMs = &d
	}

	return result
}
