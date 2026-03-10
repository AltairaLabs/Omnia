/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"encoding/json"
	"testing"

	runtimeevals "github.com/AltairaLabs/PromptKit/runtime/evals"
	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/altairalabs/omnia/internal/session"
)

func TestConvertToSDKDefs(t *testing.T) {
	defs := []EvalDef{
		{ID: "e1", Type: "contains", Trigger: "per_turn", Params: map[string]any{"value": "hello"}},
		{ID: "e2", Type: "llm_judge", Trigger: "on_session_complete", Params: map[string]any{"criteria": "helpful"}},
		{ID: "e3", Type: "custom", Trigger: "every_turn"},
	}

	sdkDefs := convertToSDKDefs(defs)

	require.Len(t, sdkDefs, 3)

	assert.Equal(t, "e1", sdkDefs[0].ID)
	assert.Equal(t, "contains", sdkDefs[0].Type)
	assert.Equal(t, runtimeevals.TriggerEveryTurn, sdkDefs[0].Trigger)
	assert.Equal(t, map[string]any{"value": "hello"}, sdkDefs[0].Params)

	assert.Equal(t, "e2", sdkDefs[1].ID)
	assert.Equal(t, runtimeevals.TriggerOnSessionComplete, sdkDefs[1].Trigger)

	assert.Equal(t, runtimeevals.EvalTrigger("every_turn"), sdkDefs[2].Trigger)
}

func TestMapTrigger(t *testing.T) {
	tests := []struct {
		input    string
		expected runtimeevals.EvalTrigger
	}{
		{"per_turn", runtimeevals.TriggerEveryTurn},
		{"on_session_complete", runtimeevals.TriggerOnSessionComplete},
		{"every_turn", runtimeevals.EvalTrigger("every_turn")},
		{"sample_turns", runtimeevals.EvalTrigger("sample_turns")},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, mapTrigger(tt.input))
		})
	}
}

func TestConvertSDKResults(t *testing.T) {
	score := 0.85
	results := []runtimeevals.EvalResult{
		{EvalID: "e1", Type: "contains", Passed: true, Score: &score, DurationMs: 5},
		{EvalID: "e2", Type: "regex", Passed: false, DurationMs: 3},
		{EvalID: "e3", Type: "llm_judge", Skipped: true, SkipReason: "sampling"},
		{EvalID: "e4", Type: "contains", Passed: true, Error: "handler panic", DurationMs: 1},
	}

	items := convertSDKResults(results)

	require.Len(t, items, 3, "skipped results should be filtered out")

	assert.Equal(t, "e1", items[0].EvalID)
	assert.True(t, items[0].Passed)
	assert.Equal(t, &score, items[0].Score)
	assert.Equal(t, 5, items[0].DurationMs)
	assert.Equal(t, evalSource, items[0].Source)

	assert.Equal(t, "e2", items[1].EvalID)
	assert.False(t, items[1].Passed)

	assert.Equal(t, "e4", items[2].EvalID)
	assert.False(t, items[2].Passed, "error should force passed=false")
}

func TestNewSDKRunner(t *testing.T) {
	runner := NewSDKRunner()
	require.NotNil(t, runner)
	assert.Nil(t, runner.tracerProvider, "no tracer provider by default")
}

func TestNewSDKRunner_WithTracerProvider(t *testing.T) {
	tp := noop.NewTracerProvider()
	runner := NewSDKRunner(WithTracerProvider(tp))
	require.NotNil(t, runner)
	assert.Equal(t, tp, runner.tracerProvider)
}

func TestSDKRunner_RunTurnEvals_ContainsHandler(t *testing.T) {
	runner := NewSDKRunner()

	defs := []EvalDef{
		{
			ID:      "e1",
			Type:    "contains",
			Trigger: "per_turn",
			Params:  map[string]any{"patterns": []any{"hello"}},
		},
	}
	messages := []session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "say hello"},
		{ID: "m2", Role: session.RoleAssistant, Content: "hello world"},
	}

	items := runner.RunTurnEvals(context.Background(), defs, messages, "sess-1", 1, nil)
	require.Len(t, items, 1)
	assert.Equal(t, "e1", items[0].EvalID)
	assert.True(t, items[0].Passed)
}

func TestSDKRunner_RunTurnEvals_ContainsFails(t *testing.T) {
	runner := NewSDKRunner()

	defs := []EvalDef{
		{
			ID:      "e1",
			Type:    "contains",
			Trigger: "per_turn",
			Params:  map[string]any{"patterns": []any{"goodbye"}},
		},
	}
	messages := []session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "say hello"},
		{ID: "m2", Role: session.RoleAssistant, Content: "hello world"},
	}

	items := runner.RunTurnEvals(context.Background(), defs, messages, "sess-1", 1, nil)
	require.Len(t, items, 1)
	assert.False(t, items[0].Passed)
}

func TestSDKRunner_RunSessionEvals(t *testing.T) {
	runner := NewSDKRunner()

	defs := []EvalDef{
		{
			ID:      "e1",
			Type:    "contains",
			Trigger: "on_session_complete",
			Params:  map[string]any{"patterns": []any{"hello"}},
		},
	}
	messages := []session.Message{
		{ID: "m1", Role: session.RoleAssistant, Content: "hello"},
	}

	items := runner.RunSessionEvals(context.Background(), defs, messages, "sess-1", 1, nil)
	require.Len(t, items, 1)
	assert.True(t, items[0].Passed)
}

func TestToAnyMap(t *testing.T) {
	specs := map[string]providers.ProviderSpec{
		"openai":    {Model: "gpt-4"},
		"anthropic": {Model: "claude-3"},
	}
	result := toAnyMap(specs)
	assert.Len(t, result, 2)
	assert.Equal(t, providers.ProviderSpec{Model: "gpt-4"}, result["openai"])
	assert.Equal(t, providers.ProviderSpec{Model: "claude-3"}, result["anthropic"])
}

func TestToAnyMap_Empty(t *testing.T) {
	result := toAnyMap(map[string]providers.ProviderSpec{})
	assert.Empty(t, result)
}

func TestToAnyMap_Nil(t *testing.T) {
	result := toAnyMap(nil)
	assert.Empty(t, result)
}

func TestBuildDetailsJSON_AllFields(t *testing.T) {
	r := runtimeevals.EvalResult{
		Explanation: "The response was too informal",
		Error:       "timeout reached",
		Message:     "eval failed",
		Details:     map[string]any{"key": "value"},
		Violations: []runtimeevals.EvalViolation{
			{TurnIndex: 1, Description: "Used slang"},
		},
	}
	raw := buildDetailsJSON(r)
	require.NotNil(t, raw)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(raw, &parsed))
	assert.Equal(t, "The response was too informal", parsed["explanation"])
	assert.Equal(t, "timeout reached", parsed["error"])
	assert.Equal(t, "eval failed", parsed["message"])
	assert.NotNil(t, parsed["details"])
	assert.NotNil(t, parsed["violations"])
}

func TestBuildDetailsJSON_Empty(t *testing.T) {
	r := runtimeevals.EvalResult{EvalID: "e1", Passed: true}
	raw := buildDetailsJSON(r)
	assert.Nil(t, raw, "no diagnostic fields should return nil")
}

func TestBuildDetailsJSON_OnlyExplanation(t *testing.T) {
	r := runtimeevals.EvalResult{Explanation: "Good tone"}
	raw := buildDetailsJSON(r)
	require.NotNil(t, raw)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(raw, &parsed))
	assert.Equal(t, "Good tone", parsed["explanation"])
	assert.Nil(t, parsed["error"])
}

func TestConvertSDKResults_CarriesDetails(t *testing.T) {
	results := []runtimeevals.EvalResult{
		{
			EvalID:      "e1",
			Type:        "llm_judge",
			Passed:      false,
			Explanation: "Too informal",
			Error:       "threshold exceeded",
		},
	}
	items := convertSDKResults(results)
	require.Len(t, items, 1)
	require.NotNil(t, items[0].Details)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(items[0].Details, &parsed))
	assert.Equal(t, "Too informal", parsed["explanation"])
	assert.Equal(t, "threshold exceeded", parsed["error"])
}

func TestSDKRunner_RunTurnEvals_SkipsMismatchedTrigger(t *testing.T) {
	runner := NewSDKRunner()

	defs := []EvalDef{
		{
			ID:      "e1",
			Type:    "contains",
			Trigger: "on_session_complete",
			Params:  map[string]any{"patterns": []any{"hello"}},
		},
	}
	messages := []session.Message{
		{ID: "m1", Role: session.RoleAssistant, Content: "hello"},
	}

	// RunTurnEvals should skip on_session_complete triggers.
	items := runner.RunTurnEvals(context.Background(), defs, messages, "sess-1", 1, nil)
	assert.Empty(t, items)
}
