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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// EvalMetrics holds Prometheus metrics for eval executions in the runtime.
// Labels are low-cardinality only (eval_id, eval_type, trigger, status).
// High-cardinality dimensions (session_id, turn) belong in OTel traces.
type EvalMetrics struct {
	// EvalsExecuted counts eval executions by eval_id, eval_type, trigger, and status.
	EvalsExecuted *prometheus.CounterVec

	// EvalScore tracks the latest eval score by eval_id, eval_type, and trigger.
	EvalScore *prometheus.GaugeVec

	// EvalDuration tracks eval execution duration in seconds.
	EvalDuration *prometheus.HistogramVec

	// EvalsPassed counts passed eval executions.
	EvalsPassed *prometheus.CounterVec

	// EvalsFailed counts failed eval executions.
	EvalsFailed *prometheus.CounterVec
}

// EvalMetricsConfig configures the eval metrics.
type EvalMetricsConfig struct {
	AgentName       string
	Namespace       string
	DurationBuckets []float64
}

// DefaultEvalDurationBuckets are histogram buckets for eval execution duration.
// Ranges from fast rule-based evals (1ms) to slow LLM judge evals (30s).
var DefaultEvalDurationBuckets = []float64{
	0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30,
}

// NewEvalMetrics creates and registers eval Prometheus metrics using the default registry.
func NewEvalMetrics(cfg EvalMetricsConfig) *EvalMetrics {
	return NewEvalMetricsWithRegisterer(prometheus.DefaultRegisterer, cfg)
}

// NewEvalMetricsWithRegisterer creates eval metrics registered against the given
// Prometheus registerer. Use prometheus.NewRegistry() in tests for isolation.
func NewEvalMetricsWithRegisterer(reg prometheus.Registerer, cfg EvalMetricsConfig) *EvalMetrics {
	labels := prometheus.Labels{
		"agent":     cfg.AgentName,
		"namespace": cfg.Namespace,
	}

	buckets := cfg.DurationBuckets
	if buckets == nil {
		buckets = DefaultEvalDurationBuckets
	}

	factory := promauto.With(reg)
	return &EvalMetrics{
		EvalsExecuted: factory.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_eval_executed_total",
			Help:        "Total eval executions by eval_id, eval_type, trigger, and status",
			ConstLabels: labels,
		}, []string{"eval_id", "eval_type", "trigger", "status"}),

		EvalScore: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "omnia_eval_score",
			Help:        "Latest eval score by eval_id, eval_type, and trigger",
			ConstLabels: labels,
		}, []string{"eval_id", "eval_type", "trigger"}),

		EvalDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "omnia_eval_duration_seconds",
			Help:        "Eval execution duration in seconds",
			ConstLabels: labels,
			Buckets:     buckets,
		}, []string{"eval_id", "eval_type", "trigger"}),

		EvalsPassed: factory.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_eval_passed_total",
			Help:        "Total passed eval executions",
			ConstLabels: labels,
		}, []string{"eval_id", "eval_type", "trigger"}),

		EvalsFailed: factory.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_eval_failed_total",
			Help:        "Total failed eval executions",
			ConstLabels: labels,
		}, []string{"eval_id", "eval_type", "trigger"}),
	}
}

// EvalRecordMetrics contains the data for recording a single eval execution.
type EvalRecordMetrics struct {
	EvalID      string
	EvalType    string
	Trigger     string
	Passed      bool
	Score       *float64
	DurationSec float64
	Skipped     bool
	HasError    bool
}

// RecordEval records metrics for a single eval execution.
func (m *EvalMetrics) RecordEval(r EvalRecordMetrics) {
	status := StatusSuccess
	if r.Skipped {
		status = "skipped"
	} else if r.HasError {
		status = StatusError
	}

	m.EvalsExecuted.WithLabelValues(r.EvalID, r.EvalType, r.Trigger, status).Inc()
	m.EvalDuration.WithLabelValues(r.EvalID, r.EvalType, r.Trigger).Observe(r.DurationSec)

	if r.Score != nil {
		m.EvalScore.WithLabelValues(r.EvalID, r.EvalType, r.Trigger).Set(*r.Score)
	}

	if !r.Skipped && !r.HasError {
		if r.Passed {
			m.EvalsPassed.WithLabelValues(r.EvalID, r.EvalType, r.Trigger).Inc()
		} else {
			m.EvalsFailed.WithLabelValues(r.EvalID, r.EvalType, r.Trigger).Inc()
		}
	}
}

// EvalMetricsRecorder is the interface for recording eval metrics.
// This allows for no-op implementations when metrics are disabled.
type EvalMetricsRecorder interface {
	RecordEval(r EvalRecordMetrics)
}

// Ensure implementations satisfy the interface.
var (
	_ EvalMetricsRecorder = (*EvalMetrics)(nil)
	_ EvalMetricsRecorder = (*NoOpEvalMetrics)(nil)
)

// NoOpEvalMetrics is a no-op implementation for when eval metrics are disabled.
type NoOpEvalMetrics struct{}

// RecordEval is a no-op implementation that intentionally does nothing.
func (n *NoOpEvalMetrics) RecordEval(_ EvalRecordMetrics) {}
