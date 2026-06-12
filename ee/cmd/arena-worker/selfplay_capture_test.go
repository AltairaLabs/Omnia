/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"errors"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
)

const (
	testProvOllama  = "ollama"
	testProvOpenAI  = "openai"
	testModelLlama  = "llama3.2:3b"
	testModelGPT4o  = "gpt-4o"
	testModelClaude = "claude"
)

func TestSelfPlayCollector(t *testing.T) {
	c := newSelfPlayCollector()

	// Agent calls are recorded by the facade — the collector ignores them.
	c.OnEvent(&events.Event{Type: events.EventProviderCallCompleted, Data: &events.ProviderCallCompletedData{
		Provider: testProvOpenAI, Model: testModelGPT4o, Source: "agent", Cost: 0.03,
	}})
	// Empty source defaults to agent.
	c.OnEvent(&events.Event{Type: events.EventProviderCallCompleted, Data: &events.ProviderCallCompletedData{
		Provider: testProvOpenAI, Model: testModelGPT4o, Source: "",
	}})
	// Self-play and judge calls are captured.
	c.OnEvent(&events.Event{Type: events.EventProviderCallCompleted, Data: &events.ProviderCallCompletedData{
		Provider: testProvOllama, Model: testModelLlama, Source: sourceSelfPlay,
		InputTokens: 322, OutputTokens: 42, Cost: 0,
	}})
	c.OnEvent(&events.Event{Type: events.EventProviderCallCompleted, Data: &events.ProviderCallCompletedData{
		Provider: "anthropic", Model: testModelClaude, Source: "judge", Cost: 0.01,
	}})
	// A failed self-play call is captured with the error.
	c.OnEvent(&events.Event{Type: events.EventProviderCallFailed, Data: &events.ProviderCallFailedData{
		Provider: testProvOllama, Model: testModelLlama, Source: sourceSelfPlay, Error: errors.New("boom"),
	}})

	calls := c.collected()
	require.Len(t, calls, 3, "agent + empty-source calls excluded")

	bySource := map[string]session.ProviderCall{}
	for _, pc := range calls {
		bySource[pc.Source+":"+string(pc.Status)] = pc
	}
	sp := bySource[sourceSelfPlay+":"+string(session.ProviderCallStatusCompleted)]
	assert.Equal(t, testModelLlama, sp.Model)
	assert.Equal(t, int64(322), sp.InputTokens)
	assert.Equal(t, "judge", bySource["judge:"+string(session.ProviderCallStatusCompleted)].Source)
	assert.Equal(t, "boom", bySource[sourceSelfPlay+":"+string(session.ProviderCallStatusFailed)].ErrorMessage)

	// selfPlaySummary surfaces the self-play model/provider.
	model, provider := selfPlaySummary(calls)
	assert.Equal(t, testModelLlama, model)
	assert.Equal(t, testProvOllama, provider)
}

func TestCollectedSelfPlayCalls_NilCollector(t *testing.T) {
	assert.Nil(t, collectedSelfPlayCalls(nil))
}
