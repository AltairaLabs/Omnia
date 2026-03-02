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
	"context"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/evals"
	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/pkg/metrics"
)

// testEvalRecorder captures RecordEval calls for assertions.
type testEvalRecorder struct {
	calls []metrics.EvalRecordMetrics
}

func (r *testEvalRecorder) RecordEval(m metrics.EvalRecordMetrics) {
	r.calls = append(r.calls, m)
}

func TestPrometheusResultWriter_WriteResults(t *testing.T) {
	recorder := &testEvalRecorder{}
	defs := []evals.EvalDef{
		{ID: "conciseness", Type: "regex", Trigger: "every_turn"},
		{ID: "accuracy", Type: "llm_judge", Trigger: "on_session_complete"},
	}

	w := NewPrometheusResultWriter(recorder, defs, logr.Discard())

	score := 0.85
	results := []evals.EvalResult{
		{
			EvalID:     "conciseness",
			Type:       "regex",
			Passed:     true,
			Score:      &score,
			DurationMs: 5,
		},
		{
			EvalID:     "accuracy",
			Type:       "llm_judge",
			Passed:     false,
			DurationMs: 2500,
		},
	}

	err := w.WriteResults(context.Background(), results)
	if err != nil {
		t.Fatalf("WriteResults returned error: %v", err)
	}

	if len(recorder.calls) != 2 {
		t.Fatalf("Expected 2 RecordEval calls, got %d", len(recorder.calls))
	}

	// Check first result
	c := recorder.calls[0]
	if c.EvalID != "conciseness" {
		t.Errorf("Expected EvalID 'conciseness', got %q", c.EvalID)
	}
	if c.EvalType != "regex" {
		t.Errorf("Expected EvalType 'regex', got %q", c.EvalType)
	}
	if c.Trigger != "every_turn" {
		t.Errorf("Expected Trigger 'every_turn', got %q", c.Trigger)
	}
	if !c.Passed {
		t.Error("Expected Passed=true")
	}
	if c.Score == nil || *c.Score != 0.85 {
		t.Errorf("Expected Score 0.85, got %v", c.Score)
	}
	if c.DurationSec != 0.005 {
		t.Errorf("Expected DurationSec 0.005, got %f", c.DurationSec)
	}

	// Check second result
	c = recorder.calls[1]
	if c.Trigger != "on_session_complete" {
		t.Errorf("Expected Trigger 'on_session_complete', got %q", c.Trigger)
	}
	if c.Passed {
		t.Error("Expected Passed=false")
	}
	if c.DurationSec != 2.5 {
		t.Errorf("Expected DurationSec 2.5, got %f", c.DurationSec)
	}
}

func TestPrometheusResultWriter_UnknownEvalID(t *testing.T) {
	recorder := &testEvalRecorder{}
	// No defs — all eval IDs will be unknown
	w := NewPrometheusResultWriter(recorder, nil, logr.Discard())

	results := []evals.EvalResult{
		{
			EvalID:     "missing_eval",
			Type:       "regex",
			Passed:     true,
			DurationMs: 1,
		},
	}

	err := w.WriteResults(context.Background(), results)
	if err != nil {
		t.Fatalf("WriteResults returned error: %v", err)
	}

	if len(recorder.calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(recorder.calls))
	}

	// Should use "unknown" trigger when def is not found
	if recorder.calls[0].Trigger != "unknown" {
		t.Errorf("Expected Trigger 'unknown', got %q", recorder.calls[0].Trigger)
	}
}

func TestPrometheusResultWriter_SkippedResult(t *testing.T) {
	recorder := &testEvalRecorder{}
	defs := []evals.EvalDef{
		{ID: "sampled_eval", Type: "llm_judge", Trigger: "sample_turns"},
	}

	w := NewPrometheusResultWriter(recorder, defs, logr.Discard())

	results := []evals.EvalResult{
		{
			EvalID:     "sampled_eval",
			Type:       "llm_judge",
			Skipped:    true,
			SkipReason: "sampling",
			DurationMs: 0,
		},
	}

	err := w.WriteResults(context.Background(), results)
	if err != nil {
		t.Fatalf("WriteResults returned error: %v", err)
	}

	if len(recorder.calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(recorder.calls))
	}

	c := recorder.calls[0]
	if !c.Skipped {
		t.Error("Expected Skipped=true")
	}
	if c.HasError {
		t.Error("Expected HasError=false for skipped result")
	}
}

func TestPrometheusResultWriter_ErrorResult(t *testing.T) {
	recorder := &testEvalRecorder{}
	defs := []evals.EvalDef{
		{ID: "broken", Type: "regex", Trigger: "every_turn"},
	}

	w := NewPrometheusResultWriter(recorder, defs, logr.Discard())

	results := []evals.EvalResult{
		{
			EvalID:     "broken",
			Type:       "regex",
			Error:      "invalid regex pattern",
			DurationMs: 1,
		},
	}

	err := w.WriteResults(context.Background(), results)
	if err != nil {
		t.Fatalf("WriteResults returned error: %v", err)
	}

	if len(recorder.calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(recorder.calls))
	}

	c := recorder.calls[0]
	if !c.HasError {
		t.Error("Expected HasError=true")
	}
}

func TestPrometheusResultWriter_EmptyResults(t *testing.T) {
	recorder := &testEvalRecorder{}
	w := NewPrometheusResultWriter(recorder, nil, logr.Discard())

	err := w.WriteResults(context.Background(), nil)
	if err != nil {
		t.Fatalf("WriteResults returned error: %v", err)
	}

	if len(recorder.calls) != 0 {
		t.Errorf("Expected 0 calls, got %d", len(recorder.calls))
	}
}
