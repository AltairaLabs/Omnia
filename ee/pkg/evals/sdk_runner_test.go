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

// spyMetrics records metric calls for testing.
type spyMetrics struct {
	NoOpWorkerMetrics
	evalExecuted     []evalExecutedCall
	samplingDecision []samplingDecisionCall
	streamLag        []streamLagCall
	eventsReceived   []string
	resultsWritten   []resultsWrittenCall
	eventProcessing  []eventProcessingCall
}

type evalExecutedCall struct {
	evalType, trigger, status string
	durationSec               float64
}

type samplingDecisionCall struct {
	evalType, decision string
}

type streamLagCall struct {
	stream string
	lag    float64
}

type resultsWrittenCall struct {
	count   int
	success bool
}

type eventProcessingCall struct {
	eventType   string
	durationSec float64
}

func (s *spyMetrics) RecordEventReceived(eventType string) {
	s.eventsReceived = append(s.eventsReceived, eventType)
}

func (s *spyMetrics) RecordEvalExecuted(evalType, trigger, status string, durationSec float64) {
	s.evalExecuted = append(s.evalExecuted, evalExecutedCall{evalType, trigger, status, durationSec})
}

func (s *spyMetrics) RecordSamplingDecision(evalType, decision string) {
	s.samplingDecision = append(s.samplingDecision, samplingDecisionCall{evalType, decision})
}

func (s *spyMetrics) RecordEventProcessing(eventType string, durationSec float64) {
	s.eventProcessing = append(s.eventProcessing, eventProcessingCall{eventType, durationSec})
}

func (s *spyMetrics) RecordResultsWritten(count int, success bool) {
	s.resultsWritten = append(s.resultsWritten, resultsWrittenCall{count, success})
}

func (s *spyMetrics) SetStreamLag(stream string, lag float64) {
	s.streamLag = append(s.streamLag, streamLagCall{stream, lag})
}

// testPackData builds a minimal pack.json with the given eval definitions.
func testPackData(defs []runtimeevals.EvalDef) []byte {
	pack := map[string]any{
		"id":      "test-pack",
		"version": "v1",
		"evals":   defs,
		"prompts": map[string]any{
			"default": map[string]any{
				"id":              "default",
				"name":            "Default",
				"version":         "1.0.0",
				"system_template": "You are a helpful assistant.",
			},
		},
	}
	data, _ := json.Marshal(pack)
	return data
}

func TestConvertSDKResults(t *testing.T) {
	score := 0.85
	results := []runtimeevals.EvalResult{
		{EvalID: "e1", Type: "contains", Value: true, Score: &score, DurationMs: 5},
		{EvalID: "e2", Type: "regex", Value: false, DurationMs: 3},
		{EvalID: "e3", Type: "llm_judge", Skipped: true, SkipReason: "sampling"},
		{EvalID: "e4", Type: "contains", Value: true, Error: "handler panic", DurationMs: 1},
	}

	items := convertSDKResults(results, runtimeevals.TriggerEveryTurn)

	require.Len(t, items, 3, "skipped results should be filtered out")

	assert.Equal(t, "e1", items[0].EvalID)
	assert.True(t, items[0].Passed)
	assert.Equal(t, &score, items[0].Score)
	assert.Equal(t, 5, items[0].DurationMs)
	assert.Equal(t, evalSource, items[0].Source)
	assert.Equal(t, "every_turn", items[0].Trigger)

	assert.Equal(t, "e2", items[1].EvalID)
	assert.False(t, items[1].Passed)
	assert.Equal(t, "every_turn", items[1].Trigger)

	assert.Equal(t, "e4", items[2].EvalID)
	assert.False(t, items[2].Passed, "error should force passed=false")
	assert.Equal(t, "every_turn", items[2].Trigger)
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

	packData := testPackData([]runtimeevals.EvalDef{
		{
			ID:      "e1",
			Type:    "contains",
			Trigger: runtimeevals.TriggerEveryTurn,
			Params:  map[string]any{"patterns": []any{"hello"}},
		},
	})
	messages := []session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "say hello"},
		{ID: "m2", Role: session.RoleAssistant, Content: "hello world"},
	}

	items := runner.RunTurnEvals(context.Background(), packData, messages, "sess-1", 1, nil, EvalLabels{})
	require.Len(t, items, 1)
	assert.Equal(t, "e1", items[0].EvalID)
	assert.True(t, items[0].Passed)
}

func TestSDKRunner_RunTurnEvals_ContainsFails(t *testing.T) {
	runner := NewSDKRunner()

	packData := testPackData([]runtimeevals.EvalDef{
		{
			ID:      "e1",
			Type:    "contains",
			Trigger: runtimeevals.TriggerEveryTurn,
			Params:  map[string]any{"patterns": []any{"goodbye"}},
		},
	})
	messages := []session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "say hello"},
		{ID: "m2", Role: session.RoleAssistant, Content: "hello world"},
	}

	items := runner.RunTurnEvals(context.Background(), packData, messages, "sess-1", 1, nil, EvalLabels{})
	require.Len(t, items, 1)
	assert.False(t, items[0].Passed)
}

func TestSDKRunner_RunSessionEvals(t *testing.T) {
	runner := NewSDKRunner()

	packData := testPackData([]runtimeevals.EvalDef{
		{
			ID:      "e1",
			Type:    "contains",
			Trigger: runtimeevals.TriggerOnSessionComplete,
			Params:  map[string]any{"patterns": []any{"hello"}},
		},
	})
	messages := []session.Message{
		{ID: "m1", Role: session.RoleAssistant, Content: "hello"},
	}

	items := runner.RunSessionEvals(context.Background(), packData, messages, "sess-1", 1, nil, EvalLabels{})
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
	r := runtimeevals.EvalResult{EvalID: "e1", Value: true}
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
			Value:       false,
			Explanation: "Too informal",
			Error:       "threshold exceeded",
		},
	}
	items := convertSDKResults(results, runtimeevals.TriggerOnSessionComplete)
	require.Len(t, items, 1)
	require.NotNil(t, items[0].Details)
	assert.Equal(t, "on_session_complete", items[0].Trigger)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(items[0].Details, &parsed))
	assert.Equal(t, "Too informal", parsed["explanation"])
	assert.Equal(t, "threshold exceeded", parsed["error"])
}

func TestSDKRunner_RunTurnEvals_SkipsMismatchedTrigger(t *testing.T) {
	runner := NewSDKRunner()

	packData := testPackData([]runtimeevals.EvalDef{
		{
			ID:      "e1",
			Type:    "contains",
			Trigger: runtimeevals.TriggerOnSessionComplete,
			Params:  map[string]any{"patterns": []any{"hello"}},
		},
	})
	messages := []session.Message{
		{ID: "m1", Role: session.RoleAssistant, Content: "hello"},
	}

	// RunTurnEvals should skip on_session_complete triggers.
	items := runner.RunTurnEvals(context.Background(), packData, messages, "sess-1", 1, nil, EvalLabels{})
	assert.Empty(t, items)
}

func TestRecordEvalMetrics_MixedResults(t *testing.T) {
	spy := &spyMetrics{}
	runner := &SDKRunner{metrics: spy}

	results := []runtimeevals.EvalResult{
		{EvalID: "e1", Type: "contains", DurationMs: 5},
		{EvalID: "e2", Type: "llm_judge", Error: "timeout", DurationMs: 3000},
		{EvalID: "e3", Type: "regex", Skipped: true, SkipReason: "sampling"},
	}

	runner.recordEvalMetrics(results, runtimeevals.TriggerEveryTurn)

	// Two executed, one skipped.
	require.Len(t, spy.evalExecuted, 2)
	assert.Equal(t, evalExecutedCall{"contains", "every_turn", MetricStatusSuccess, 0.005}, spy.evalExecuted[0])
	assert.Equal(t, evalExecutedCall{"llm_judge", "every_turn", MetricStatusError, 3.0}, spy.evalExecuted[1])

	// Three sampling decisions total.
	require.Len(t, spy.samplingDecision, 3)
	assert.Equal(t, samplingDecisionCall{"contains", MetricStatusSampled}, spy.samplingDecision[0])
	assert.Equal(t, samplingDecisionCall{"llm_judge", MetricStatusSampled}, spy.samplingDecision[1])
	assert.Equal(t, samplingDecisionCall{"regex", MetricStatusSkipped}, spy.samplingDecision[2])
}

func TestRecordEvalMetrics_NilMetrics(t *testing.T) {
	runner := &SDKRunner{} // no metrics set
	results := []runtimeevals.EvalResult{
		{EvalID: "e1", Type: "contains"},
	}
	// Should not panic.
	runner.recordEvalMetrics(results, runtimeevals.TriggerEveryTurn)
}

func TestRecordEvalMetrics_EmptyResults(t *testing.T) {
	spy := &spyMetrics{}
	runner := &SDKRunner{metrics: spy}
	runner.recordEvalMetrics(nil, runtimeevals.TriggerEveryTurn)
	assert.Empty(t, spy.evalExecuted)
	assert.Empty(t, spy.samplingDecision)
}

func TestWithMetrics_Option(t *testing.T) {
	spy := &spyMetrics{}
	runner := NewSDKRunner(WithMetrics(spy))
	assert.Equal(t, spy, runner.metrics)
}

func TestSDKRunner_RunTurnEvals_RecordsMetrics(t *testing.T) {
	spy := &spyMetrics{}
	runner := NewSDKRunner(WithMetrics(spy))

	packData := testPackData([]runtimeevals.EvalDef{
		{
			ID:      "e1",
			Type:    "contains",
			Trigger: runtimeevals.TriggerEveryTurn,
			Params:  map[string]any{"patterns": []any{"hello"}},
		},
	})
	messages := []session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "say hello"},
		{ID: "m2", Role: session.RoleAssistant, Content: "hello world"},
	}

	items := runner.RunTurnEvals(context.Background(), packData, messages, "sess-1", 1, nil, EvalLabels{})
	require.Len(t, items, 1)

	// Verify metrics were recorded.
	require.Len(t, spy.evalExecuted, 1)
	assert.Equal(t, "contains", spy.evalExecuted[0].evalType)
	assert.Equal(t, "every_turn", spy.evalExecuted[0].trigger)
	assert.Equal(t, MetricStatusSuccess, spy.evalExecuted[0].status)

	require.Len(t, spy.samplingDecision, 1)
	assert.Equal(t, samplingDecisionCall{"contains", MetricStatusSampled}, spy.samplingDecision[0])
}
