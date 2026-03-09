/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"testing"

	runtimeevals "github.com/AltairaLabs/PromptKit/runtime/evals"
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
