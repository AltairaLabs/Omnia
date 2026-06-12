/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"sync"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/google/uuid"

	"github.com/altairalabs/omnia/internal/session"
)

// sourceAgent is the provider-call Source for the agent's own LLM calls. The
// facade records those on the agent session, so the arena worker captures only
// the OTHER sources (selfplay user simulation, judge) — they run inside the
// worker's engine, behind no facade, and would otherwise go unrecorded.
const sourceAgent = "agent"

// selfPlayCollector buffers provider-call events whose Source is not "agent"
// (self-play and judge calls) so they can be attached to the facade-recorded
// session after a fleet run. It does not create a session of its own. The event
// bus dispatches asynchronously, so callers MUST drain/close the bus (or rely on
// the engine having finished) before reading collected().
type selfPlayCollector struct {
	mu    sync.Mutex
	calls []session.ProviderCall
}

func newSelfPlayCollector() *selfPlayCollector { return &selfPlayCollector{} }

// OnEvent is a bus subscriber. It records completed/failed provider calls that
// did not originate from the agent itself (the facade already records those).
func (c *selfPlayCollector) OnEvent(e *events.Event) {
	switch e.Type {
	case events.EventProviderCallCompleted:
		data, ok := e.Data.(*events.ProviderCallCompletedData)
		if !ok || isAgentSource(data.Source) {
			return
		}
		c.append(session.ProviderCall{
			ID:            uuid.NewString(),
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
			Labels:        data.Labels,
			Source:        data.Source,
			CreatedAt:     e.Timestamp,
		})
	case events.EventProviderCallFailed:
		data, ok := e.Data.(*events.ProviderCallFailedData)
		if !ok || isAgentSource(data.Source) {
			return
		}
		errMsg := ""
		if data.Error != nil {
			errMsg = data.Error.Error()
		}
		c.append(session.ProviderCall{
			ID:           uuid.NewString(),
			Provider:     data.Provider,
			Model:        data.Model,
			Status:       session.ProviderCallStatusFailed,
			DurationMs:   data.Duration.Milliseconds(),
			ErrorMessage: errMsg,
			Labels:       data.Labels,
			Source:       data.Source,
			CreatedAt:    e.Timestamp,
		})
	}
}

// isAgentSource reports whether a provider-call Source belongs to the agent
// (recorded by the facade). Empty Source defaults to "agent" for back-compat.
func isAgentSource(s string) bool { return s == "" || s == sourceAgent }

func (c *selfPlayCollector) append(pc session.ProviderCall) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, pc)
}

// collected returns the accumulated calls. Call only after the engine run is done.
func (c *selfPlayCollector) collected() []session.ProviderCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// selfPlaySummary returns the (model, provider) of the first self-play call, used
// to surface the user-simulator model on the session at a glance. Returns empty
// strings when there are no self-play calls.
func selfPlaySummary(calls []session.ProviderCall) (model, provider string) {
	for _, pc := range calls {
		if pc.Source == sourceSelfPlay {
			return pc.Model, pc.Provider
		}
	}
	return "", ""
}

// sourceSelfPlay is the provider-call Source for self-play user-simulation calls.
const sourceSelfPlay = "selfplay"
