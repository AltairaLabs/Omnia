/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metric label value constants.
const (
	MetricStatusSuccess = "success"
	MetricStatusError   = "error"
	MetricStatusSkipped = "skipped"
)

// DefaultEvalDurationBuckets are histogram buckets for eval execution duration.
// Ranges from fast rule-based evals (1ms) to slow LLM judge evals (30s).
var DefaultEvalDurationBuckets = []float64{
	0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30,
}

// WorkerMetrics holds Prometheus metrics for the eval worker.
type WorkerMetrics struct {
	// EventsReceived counts events consumed from Redis Streams by event type.
	EventsReceived *prometheus.CounterVec

	// EvalsExecuted counts eval executions by type, trigger, and status.
	EvalsExecuted *prometheus.CounterVec

	// EvalDuration tracks eval execution duration in seconds.
	EvalDuration *prometheus.HistogramVec

	// EvalsSampled counts sampling decisions (sampled vs skipped).
	EvalsSampled *prometheus.CounterVec

	// StreamLag tracks the approximate consumer lag (pending messages) per stream.
	StreamLag *prometheus.GaugeVec

	// EventProcessingDuration tracks time to process a single stream message.
	EventProcessingDuration *prometheus.HistogramVec

	// ResultsWritten counts eval results written to session-api.
	ResultsWritten *prometheus.CounterVec
}

// WorkerMetricsConfig configures the eval worker metrics.
type WorkerMetricsConfig struct {
	EvalDurationBuckets []float64
}

// NewWorkerMetrics creates and registers Prometheus metrics for the eval worker
// using the default Prometheus registry.
func NewWorkerMetrics(cfg *WorkerMetricsConfig) *WorkerMetrics {
	return NewWorkerMetricsWithRegisterer(prometheus.DefaultRegisterer, cfg)
}

// NewWorkerMetricsWithRegisterer creates WorkerMetrics registered against the
// given Prometheus registerer. Use prometheus.NewRegistry() in tests.
func NewWorkerMetricsWithRegisterer(reg prometheus.Registerer, cfg *WorkerMetricsConfig) *WorkerMetrics {
	var buckets []float64
	if cfg != nil && cfg.EvalDurationBuckets != nil {
		buckets = cfg.EvalDurationBuckets
	} else {
		buckets = DefaultEvalDurationBuckets
	}

	factory := promauto.With(reg)
	return &WorkerMetrics{
		EventsReceived: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_eval_worker_events_received_total",
			Help: "Total session events consumed from Redis Streams",
		}, []string{"event_type"}),

		EvalsExecuted: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_eval_worker_evals_executed_total",
			Help: "Total eval executions by type, trigger, and status",
		}, []string{"eval_type", "trigger", "status"}),

		EvalDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "omnia_eval_worker_eval_duration_seconds",
			Help:    "Eval execution duration in seconds",
			Buckets: buckets,
		}, []string{"eval_type", "trigger"}),

		EvalsSampled: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_eval_worker_evals_sampled_total",
			Help: "Sampling decisions: sampled (executed) vs skipped",
		}, []string{"eval_type", "decision"}),

		StreamLag: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "omnia_eval_worker_stream_lag",
			Help: "Approximate pending messages per Redis stream",
		}, []string{"stream"}),

		EventProcessingDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "omnia_eval_worker_event_processing_duration_seconds",
			Help:    "Time to process a single stream event end-to-end",
			Buckets: buckets,
		}, []string{"event_type"}),

		ResultsWritten: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_eval_worker_results_written_total",
			Help: "Eval results written to session-api",
		}, []string{"status"}),
	}
}

// Initialize pre-registers metrics so they appear in /metrics at startup.
func (m *WorkerMetrics) Initialize() {
	for _, et := range []string{"message.assistant", "session.completed", "unknown"} {
		m.EventsReceived.WithLabelValues(et).Add(0)
		m.EventProcessingDuration.WithLabelValues(et)
	}

	for _, status := range []string{MetricStatusSuccess, MetricStatusError} {
		m.ResultsWritten.WithLabelValues(status).Add(0)
	}
}

// WorkerMetricsRecorder is the interface for recording eval worker metrics.
type WorkerMetricsRecorder interface {
	RecordEventReceived(eventType string)
	RecordEvalExecuted(evalType, trigger, status string, durationSec float64)
	RecordSamplingDecision(evalType, decision string)
	RecordEventProcessing(eventType string, durationSec float64)
	RecordResultsWritten(count int, success bool)
	SetStreamLag(stream string, lag float64)
}

// Ensure implementations satisfy interfaces.
var (
	_ WorkerMetricsRecorder = (*WorkerMetrics)(nil)
	_ WorkerMetricsRecorder = (*NoOpWorkerMetrics)(nil)
)

// RecordEventReceived increments the events received counter.
func (m *WorkerMetrics) RecordEventReceived(eventType string) {
	m.EventsReceived.WithLabelValues(eventType).Inc()
}

// RecordEvalExecuted records an eval execution with its outcome.
func (m *WorkerMetrics) RecordEvalExecuted(evalType, trigger, status string, durationSec float64) {
	m.EvalsExecuted.WithLabelValues(evalType, trigger, status).Inc()
	m.EvalDuration.WithLabelValues(evalType, trigger).Observe(durationSec)
}

// RecordSamplingDecision records whether an eval was sampled or skipped.
func (m *WorkerMetrics) RecordSamplingDecision(evalType, decision string) {
	m.EvalsSampled.WithLabelValues(evalType, decision).Inc()
}

// RecordEventProcessing records the total time to process a stream event.
func (m *WorkerMetrics) RecordEventProcessing(eventType string, durationSec float64) {
	m.EventProcessingDuration.WithLabelValues(eventType).Observe(durationSec)
}

// RecordResultsWritten records eval results written to session-api.
func (m *WorkerMetrics) RecordResultsWritten(count int, success bool) {
	status := MetricStatusSuccess
	if !success {
		status = MetricStatusError
	}
	m.ResultsWritten.WithLabelValues(status).Add(float64(count))
}

// SetStreamLag sets the current consumer lag for a stream.
func (m *WorkerMetrics) SetStreamLag(stream string, lag float64) {
	m.StreamLag.WithLabelValues(stream).Set(lag)
}

// NoOpWorkerMetrics is a no-op implementation for when metrics are disabled.
type NoOpWorkerMetrics struct{}

// RecordEventReceived is a no-op.
func (n *NoOpWorkerMetrics) RecordEventReceived(_ string) {}

// RecordEvalExecuted is a no-op.
func (n *NoOpWorkerMetrics) RecordEvalExecuted(_, _, _ string, _ float64) {}

// RecordSamplingDecision is a no-op.
func (n *NoOpWorkerMetrics) RecordSamplingDecision(_, _ string) {}

// RecordEventProcessing is a no-op.
func (n *NoOpWorkerMetrics) RecordEventProcessing(_ string, _ float64) {}

// RecordResultsWritten is a no-op.
func (n *NoOpWorkerMetrics) RecordResultsWritten(_ int, _ bool) {}

// SetStreamLag is a no-op.
func (n *NoOpWorkerMetrics) SetStreamLag(_ string, _ float64) {}
