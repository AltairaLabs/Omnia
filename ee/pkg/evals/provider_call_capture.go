/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"log/slog"
	"sync"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/google/uuid"

	"github.com/altairalabs/omnia/internal/session"
)

// providerCallCollector accumulates ProviderCallCompleted/Failed events emitted
// during a single sdk.Evaluate() run. Every provider type the eval pipeline
// touches — judge LLM calls, RAG-eval embeddings, etc. — surfaces as one of
// these events; the collector turns each into a session.ProviderCall stamped
// with the evaluated session's namespace/agent so the usage is attributed
// without a JOIN. The recorded source (e.g. "judge") keeps the call out of the
// session-totals rollup.
//
// Bus dispatch is asynchronous, so callers MUST Close the bus before reading
// calls() to ensure every event has been processed.
type providerCallCollector struct {
	sessionID string
	namespace string
	agentName string

	mu    sync.Mutex
	calls []*session.ProviderCall
}

func newProviderCallCollector(sessionID, namespace, agentName string) *providerCallCollector {
	return &providerCallCollector{
		sessionID: sessionID,
		namespace: namespace,
		agentName: agentName,
	}
}

// onCompleted records a completed provider call (with token usage).
func (c *providerCallCollector) onCompleted(e *events.Event) {
	data, ok := e.Data.(*events.ProviderCallCompletedData)
	if !ok {
		return
	}
	c.append(&session.ProviderCall{
		ID:            uuid.NewString(),
		SessionID:     c.sessionID,
		Namespace:     c.namespace,
		AgentName:     c.agentName,
		Provider:      data.Provider,
		Model:         data.Model,
		Status:        session.ProviderCallStatusCompleted,
		InputTokens:   int64(data.InputTokens),
		OutputTokens:  int64(data.OutputTokens),
		CachedTokens:  int64(data.CachedTokens),
		CostUSD:       data.Cost,
		DurationMs:    data.Duration.Milliseconds(),
		FinishReason:  data.FinishReason,
		ToolCallCount: int32(data.ToolCallCount),
		Source:        data.Source,
		CreatedAt:     e.Timestamp,
	})
}

// onFailed records a failed provider call (no token usage).
func (c *providerCallCollector) onFailed(e *events.Event) {
	data, ok := e.Data.(*events.ProviderCallFailedData)
	if !ok {
		return
	}
	errMsg := ""
	if data.Error != nil {
		errMsg = data.Error.Error()
	}
	c.append(&session.ProviderCall{
		ID:           uuid.NewString(),
		SessionID:    c.sessionID,
		Namespace:    c.namespace,
		AgentName:    c.agentName,
		Provider:     data.Provider,
		Model:        data.Model,
		Status:       session.ProviderCallStatusFailed,
		DurationMs:   data.Duration.Milliseconds(),
		ErrorMessage: errMsg,
		Source:       data.Source,
		CreatedAt:    e.Timestamp,
	})
}

func (c *providerCallCollector) append(pc *session.ProviderCall) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, pc)
}

// collected returns the accumulated calls. Call only after the bus is drained.
func (c *providerCallCollector) collected() []*session.ProviderCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// flushProviderCalls persists collected provider calls best-effort: a failed
// write is logged but never aborts evaluation (usage tracking must not break
// the eval result path).
func flushProviderCalls(
	ctx context.Context,
	writer ProviderCallWriter,
	logger *slog.Logger,
	calls []*session.ProviderCall,
) {
	for _, pc := range calls {
		if err := writer.RecordProviderCall(ctx, pc.SessionID, pc); err != nil && logger != nil {
			logger.Warn("failed to record eval provider call",
				"sessionID", pc.SessionID,
				"provider", pc.Provider,
				"source", pc.Source,
				"error", err,
			)
		}
	}
}
