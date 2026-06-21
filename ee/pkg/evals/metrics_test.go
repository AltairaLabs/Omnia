/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"testing"

	runtimeevals "github.com/AltairaLabs/PromptKit/runtime/evals"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	candidateVariant = "candidate"
	llmJudgeEvalType = "llm_judge"
)

func TestNewWorkerMetrics_DefaultBuckets(t *testing.T) {
	// Reset registry to avoid registration conflicts between tests.
	reg := prometheus.NewRegistry()
	m := newWorkerMetricsWithRegistry(reg, nil)
	require.NotNil(t, m)
	assert.NotNil(t, m.EventsReceived)
	assert.NotNil(t, m.EvalsExecuted)
	assert.NotNil(t, m.EvalDuration)
	assert.NotNil(t, m.EvalsSampled)
	assert.NotNil(t, m.StreamLag)
	assert.NotNil(t, m.EventProcessingDuration)
	assert.NotNil(t, m.ResultsWritten)
	assert.NotNil(t, m.EvalScore)
}

func TestWorkerMetrics_RecordEvalScore(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newWorkerMetricsWithRegistry(reg, nil)

	labels := EvalLabels{Agent: "rag-hero", Namespace: "omnia-demo", PromptPackName: "p", Variant: candidateVariant}
	m.RecordEvalScore("faithfulness", labels, 0.9)
	m.RecordEvalScore("faithfulness", labels, 0.7)

	families, err := reg.Gather()
	require.NoError(t, err)

	var found bool
	for _, fam := range families {
		if fam.GetName() != "omnia_eval_score" {
			continue
		}
		found = true
		require.Len(t, fam.GetMetric(), 1)
		h := fam.GetMetric()[0].GetHistogram()
		assert.Equal(t, uint64(2), h.GetSampleCount(), "_count must reflect both observations")
		assert.InDelta(t, 1.6, h.GetSampleSum(), 1e-9, "_sum must be the score total")

		labelMap := map[string]string{}
		for _, l := range fam.GetMetric()[0].GetLabel() {
			labelMap[l.GetName()] = l.GetValue()
		}
		assert.Equal(t, "faithfulness", labelMap[labelKeyEvalID])
		assert.Equal(t, candidateVariant, labelMap[labelKeyVariant])
		assert.Equal(t, "rag-hero", labelMap[labelKeyAgent])
	}
	assert.True(t, found, "omnia_eval_score histogram family not found")
}

// NoOpWorkerMetrics must satisfy the recorder interface without panicking.
func TestNoOpWorkerMetrics_RecordEvalScore(t *testing.T) {
	var m WorkerMetricsRecorder = &NoOpWorkerMetrics{}
	assert.NotPanics(t, func() {
		m.RecordEvalScore("faithfulness", EvalLabels{Variant: candidateVariant}, 0.5)
	})
}

// recordEvalMetrics must observe the score histogram only for successful scored
// evals — skipped, errored, and scoreless (boolean) results must not contribute.
func TestRecordEvalMetrics_ScoreFiltering(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newWorkerMetricsWithRegistry(reg, nil)
	runner := NewSDKRunner(WithMetrics(m))

	score := func(v float64) *float64 { return &v }
	results := []runtimeevals.EvalResult{
		{EvalID: "faithfulness", Type: llmJudgeEvalType, Score: score(0.8)}, // counted
		{EvalID: "skipped-one", Type: llmJudgeEvalType, Score: score(1.0), Skipped: true},
		{EvalID: "errored-one", Type: llmJudgeEvalType, Score: score(0.1), Error: "judge failed"},
		{EvalID: "contains-one", Type: containsEvalType, Value: true}, // boolean, no score
	}

	runner.recordEvalMetrics(results, runtimeevals.TriggerOnSessionComplete,
		EvalLabels{Agent: "a", Namespace: "n", PromptPackName: "p", Variant: candidateVariant})

	families, err := reg.Gather()
	require.NoError(t, err)

	var totalSamples uint64
	for _, fam := range families {
		if fam.GetName() != "omnia_eval_score" {
			continue
		}
		for _, metric := range fam.GetMetric() {
			totalSamples += metric.GetHistogram().GetSampleCount()
		}
	}
	assert.Equal(t, uint64(1), totalSamples, "only the successful scored eval should be observed")
}

func TestNewWorkerMetrics_CustomBuckets(t *testing.T) {
	reg := prometheus.NewRegistry()
	buckets := []float64{0.1, 0.5, 1.0}
	m := newWorkerMetricsWithRegistry(reg, &WorkerMetricsConfig{
		EvalDurationBuckets: buckets,
	})
	require.NotNil(t, m)
}

func TestWorkerMetrics_Initialize(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newWorkerMetricsWithRegistry(reg, nil)
	m.Initialize()

	// Verify pre-registered event types.
	families, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, fam := range families {
		if fam.GetName() == "omnia_eval_worker_events_received_total" {
			found = true
			assert.GreaterOrEqual(t, len(fam.GetMetric()), 3)
		}
	}
	assert.True(t, found, "events_received_total metric family not found")
}

func TestWorkerMetrics_RecordEventReceived(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newWorkerMetricsWithRegistry(reg, nil)

	m.RecordEventReceived("message.assistant")
	m.RecordEventReceived("message.assistant")
	m.RecordEventReceived("session.completed")

	families, err := reg.Gather()
	require.NoError(t, err)

	for _, fam := range families {
		if fam.GetName() == "omnia_eval_worker_events_received_total" {
			for _, metric := range fam.GetMetric() {
				for _, label := range metric.GetLabel() {
					if label.GetValue() == "message.assistant" {
						assert.Equal(t, float64(2), metric.GetCounter().GetValue())
					}
				}
			}
		}
	}
}

func TestWorkerMetrics_RecordEvalExecuted(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newWorkerMetricsWithRegistry(reg, nil)

	m.RecordEvalExecuted("llm_judge", "per_turn", MetricStatusSuccess, 1.5)
	m.RecordEvalExecuted("rule", "per_turn", MetricStatusError, 0.01)

	families, err := reg.Gather()
	require.NoError(t, err)

	counterFound := false
	histFound := false
	for _, fam := range families {
		if fam.GetName() == "omnia_eval_worker_evals_executed_total" {
			counterFound = true
			assert.Equal(t, 2, len(fam.GetMetric()))
		}
		if fam.GetName() == "omnia_eval_worker_eval_duration_seconds" {
			histFound = true
		}
	}
	assert.True(t, counterFound)
	assert.True(t, histFound)
}

func TestWorkerMetrics_RecordSamplingDecision(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newWorkerMetricsWithRegistry(reg, nil)

	m.RecordSamplingDecision("llm_judge", MetricStatusSampled)
	m.RecordSamplingDecision("llm_judge", MetricStatusSkipped)
	m.RecordSamplingDecision("rule", MetricStatusSampled)

	families, err := reg.Gather()
	require.NoError(t, err)

	for _, fam := range families {
		if fam.GetName() == "omnia_eval_worker_evals_sampled_total" {
			assert.Equal(t, 3, len(fam.GetMetric()))
		}
	}
}

func TestWorkerMetrics_RecordResultsWritten(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newWorkerMetricsWithRegistry(reg, nil)

	m.RecordResultsWritten(5, true)
	m.RecordResultsWritten(2, false)

	families, err := reg.Gather()
	require.NoError(t, err)

	for _, fam := range families {
		if fam.GetName() == "omnia_eval_worker_results_written_total" {
			for _, metric := range fam.GetMetric() {
				for _, label := range metric.GetLabel() {
					if label.GetValue() == MetricStatusSuccess {
						assert.Equal(t, float64(5), metric.GetCounter().GetValue())
					}
					if label.GetValue() == MetricStatusError {
						assert.Equal(t, float64(2), metric.GetCounter().GetValue())
					}
				}
			}
		}
	}
}

func TestWorkerMetrics_SetStreamLag(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newWorkerMetricsWithRegistry(reg, nil)

	m.SetStreamLag("omnia:eval-events:ns1", 42)

	families, err := reg.Gather()
	require.NoError(t, err)

	for _, fam := range families {
		if fam.GetName() == "omnia_eval_worker_stream_lag" {
			assert.Equal(t, 1, len(fam.GetMetric()))
			assert.Equal(t, float64(42), fam.GetMetric()[0].GetGauge().GetValue())
		}
	}
}

func TestWorkerMetrics_RecordEventProcessing(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newWorkerMetricsWithRegistry(reg, nil)

	m.RecordEventProcessing("message.assistant", 0.5)

	families, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, fam := range families {
		if fam.GetName() == "omnia_eval_worker_event_processing_duration_seconds" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestNoOpWorkerMetrics(t *testing.T) {
	var m WorkerMetricsRecorder = &NoOpWorkerMetrics{}

	// Verify no panics.
	m.RecordEventReceived("test")
	m.RecordEvalExecuted("type", "trigger", "status", 1.0)
	m.RecordSamplingDecision("type", MetricStatusSampled)
	m.RecordEventProcessing("test", 0.5)
	m.RecordResultsWritten(1, true)
	m.SetStreamLag("stream", 10)
}

// newWorkerMetricsWithRegistry delegates to the exported constructor.
func newWorkerMetricsWithRegistry(reg prometheus.Registerer, cfg *WorkerMetricsConfig) *WorkerMetrics {
	return NewWorkerMetricsWithRegisterer(reg, cfg)
}
