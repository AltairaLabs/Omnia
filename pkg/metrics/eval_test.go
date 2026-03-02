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

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNewEvalMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := EvalMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	}

	m := NewEvalMetricsWithRegisterer(reg, cfg)
	if m == nil {
		t.Fatal("NewEvalMetricsWithRegisterer returned nil")
	}

	if m.EvalsExecuted == nil {
		t.Error("EvalsExecuted is nil")
	}
	if m.EvalScore == nil {
		t.Error("EvalScore is nil")
	}
	if m.EvalDuration == nil {
		t.Error("EvalDuration is nil")
	}
	if m.EvalsPassed == nil {
		t.Error("EvalsPassed is nil")
	}
	if m.EvalsFailed == nil {
		t.Error("EvalsFailed is nil")
	}
}

func TestNewEvalMetrics_CustomBuckets(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := EvalMetricsConfig{
		AgentName:       "test-agent",
		Namespace:       "test-ns",
		DurationBuckets: []float64{0.1, 0.5, 1.0},
	}

	m := NewEvalMetricsWithRegisterer(reg, cfg)
	if m == nil {
		t.Fatal("NewEvalMetricsWithRegisterer returned nil")
	}
	if m.EvalDuration == nil {
		t.Error("EvalDuration is nil")
	}
}

func TestEvalMetrics_RecordEval_Success(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewEvalMetricsWithRegisterer(reg, EvalMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	})

	score := 0.85
	m.RecordEval(EvalRecordMetrics{
		EvalID:      "response_conciseness",
		EvalType:    "regex",
		Trigger:     "every_turn",
		Passed:      true,
		Score:       &score,
		DurationSec: 0.005,
	})

	gathered, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	names := make(map[string]bool)
	for _, mf := range gathered {
		names[mf.GetName()] = true
	}

	expectedNames := []string{
		"omnia_eval_executed_total",
		"omnia_eval_score",
		"omnia_eval_duration_seconds",
		"omnia_eval_passed_total",
	}
	for _, name := range expectedNames {
		if !names[name] {
			t.Errorf("Expected metric %q not found", name)
		}
	}

	// Failed counter should NOT appear since the eval passed
	if names["omnia_eval_failed_total"] {
		t.Error("omnia_eval_failed_total should not be present for a passing eval")
	}
}

func TestEvalMetrics_RecordEval_Failed(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewEvalMetricsWithRegisterer(reg, EvalMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	})

	m.RecordEval(EvalRecordMetrics{
		EvalID:      "response_length",
		EvalType:    "threshold",
		Trigger:     "every_turn",
		Passed:      false,
		DurationSec: 0.003,
	})

	gathered, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	names := make(map[string]bool)
	for _, mf := range gathered {
		names[mf.GetName()] = true
	}

	if !names["omnia_eval_failed_total"] {
		t.Error("Expected omnia_eval_failed_total for a failing eval")
	}
	if !names["omnia_eval_executed_total"] {
		t.Error("Expected omnia_eval_executed_total")
	}
}

func TestEvalMetrics_RecordEval_Skipped(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewEvalMetricsWithRegisterer(reg, EvalMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	})

	m.RecordEval(EvalRecordMetrics{
		EvalID:      "expensive_eval",
		EvalType:    "llm_judge",
		Trigger:     "sample_turns",
		Skipped:     true,
		DurationSec: 0.0,
	})

	gathered, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Should have executed counter with status=skipped but no passed/failed
	names := make(map[string]bool)
	for _, mf := range gathered {
		names[mf.GetName()] = true
	}

	if !names["omnia_eval_executed_total"] {
		t.Error("Expected omnia_eval_executed_total for skipped eval")
	}
	if names["omnia_eval_passed_total"] {
		t.Error("Passed counter should not appear for skipped eval")
	}
	if names["omnia_eval_failed_total"] {
		t.Error("Failed counter should not appear for skipped eval")
	}
}

func TestEvalMetrics_RecordEval_Error(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewEvalMetricsWithRegisterer(reg, EvalMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	})

	m.RecordEval(EvalRecordMetrics{
		EvalID:      "broken_eval",
		EvalType:    "regex",
		Trigger:     "every_turn",
		HasError:    true,
		DurationSec: 0.001,
	})

	gathered, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	names := make(map[string]bool)
	for _, mf := range gathered {
		names[mf.GetName()] = true
	}

	if !names["omnia_eval_executed_total"] {
		t.Error("Expected omnia_eval_executed_total for errored eval")
	}
	// No passed/failed counters for errors
	if names["omnia_eval_passed_total"] {
		t.Error("Passed counter should not appear for errored eval")
	}
	if names["omnia_eval_failed_total"] {
		t.Error("Failed counter should not appear for errored eval")
	}
}

func TestEvalMetrics_RecordEval_NoScore(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewEvalMetricsWithRegisterer(reg, EvalMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	})

	m.RecordEval(EvalRecordMetrics{
		EvalID:      "binary_eval",
		EvalType:    "regex",
		Trigger:     "every_turn",
		Passed:      true,
		Score:       nil,
		DurationSec: 0.002,
	})

	gathered, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	names := make(map[string]bool)
	for _, mf := range gathered {
		names[mf.GetName()] = true
	}

	// Score gauge should NOT appear when score is nil
	if names["omnia_eval_score"] {
		t.Error("omnia_eval_score should not appear when score is nil")
	}
}

func TestNoOpEvalMetrics_RecordEval(t *testing.T) {
	m := &NoOpEvalMetrics{}

	score := 0.5
	// Should not panic
	m.RecordEval(EvalRecordMetrics{
		EvalID:      "test",
		EvalType:    "regex",
		Trigger:     "every_turn",
		Passed:      true,
		Score:       &score,
		DurationSec: 0.1,
	})
}

func TestEvalMetricsRecorder_Interface(t *testing.T) {
	var _ EvalMetricsRecorder = &EvalMetrics{}
	var _ EvalMetricsRecorder = &NoOpEvalMetrics{}
}

func TestDefaultEvalDurationBuckets(t *testing.T) {
	if len(DefaultEvalDurationBuckets) == 0 {
		t.Error("DefaultEvalDurationBuckets is empty")
	}

	for i := 1; i < len(DefaultEvalDurationBuckets); i++ {
		if DefaultEvalDurationBuckets[i] <= DefaultEvalDurationBuckets[i-1] {
			t.Errorf("Buckets not in ascending order: %v", DefaultEvalDurationBuckets)
		}
	}
}
