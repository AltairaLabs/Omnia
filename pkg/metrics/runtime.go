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

// RuntimeMetrics holds Prometheus metrics for runtime operations.
// These metrics track tool calls, pipeline executions, stages, and validations.
type RuntimeMetrics struct {
	// Tool metrics
	// ToolCallsTotal is the total number of tool calls.
	ToolCallsTotal *prometheus.CounterVec
	// ToolCallDuration is the histogram of tool call durations.
	ToolCallDuration *prometheus.HistogramVec

	// Pipeline metrics
	// PipelinesActive is the number of currently active pipelines.
	PipelinesActive *prometheus.GaugeVec
	// PipelineDuration is the histogram of pipeline durations.
	PipelineDuration *prometheus.HistogramVec

	// Stage metrics
	// StageElementsTotal is the total number of stage executions.
	StageElementsTotal *prometheus.CounterVec
	// StageDuration is the histogram of stage durations.
	StageDuration *prometheus.HistogramVec

	// Validation metrics
	// ValidationsTotal is the total number of validations.
	ValidationsTotal *prometheus.CounterVec
	// ValidationDuration is the histogram of validation durations.
	ValidationDuration *prometheus.HistogramVec
}

// RuntimeMetricsConfig configures the runtime metrics.
type RuntimeMetricsConfig struct {
	AgentName string
	Namespace string
	// ToolDurationBuckets for tool call duration histogram.
	// If nil, defaults to standard tool buckets.
	ToolDurationBuckets []float64
	// PipelineDurationBuckets for pipeline duration histogram.
	// If nil, defaults to standard LLM buckets.
	PipelineDurationBuckets []float64
	// StageDurationBuckets for stage duration histogram.
	// If nil, defaults to standard stage buckets.
	StageDurationBuckets []float64
	// ValidationDurationBuckets for validation duration histogram.
	// If nil, defaults to standard validation buckets.
	ValidationDurationBuckets []float64
}

// DefaultToolDurationBuckets are the default histogram buckets for tool call durations.
// Tool calls are typically faster than LLM calls, but can vary widely.
var DefaultToolDurationBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

// DefaultStageDurationBuckets are the default histogram buckets for stage durations.
// Stages can range from very fast transforms to slower LLM generations.
var DefaultStageDurationBuckets = []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30}

// DefaultValidationDurationBuckets are the default histogram buckets for validation durations.
// Validations are typically fast.
var DefaultValidationDurationBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1}

// NewRuntimeMetrics creates and registers all Prometheus metrics for runtime operations.
func NewRuntimeMetrics(cfg RuntimeMetricsConfig) *RuntimeMetrics {
	labels := prometheus.Labels{
		"agent":     cfg.AgentName,
		"namespace": cfg.Namespace,
	}

	toolBuckets := cfg.ToolDurationBuckets
	if toolBuckets == nil {
		toolBuckets = DefaultToolDurationBuckets
	}

	pipelineBuckets := cfg.PipelineDurationBuckets
	if pipelineBuckets == nil {
		pipelineBuckets = DefaultLLMDurationBuckets
	}

	stageBuckets := cfg.StageDurationBuckets
	if stageBuckets == nil {
		stageBuckets = DefaultStageDurationBuckets
	}

	validationBuckets := cfg.ValidationDurationBuckets
	if validationBuckets == nil {
		validationBuckets = DefaultValidationDurationBuckets
	}

	return &RuntimeMetrics{
		// Tool metrics
		ToolCallsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_runtime_tool_calls_total",
			Help:        "Total number of tool calls",
			ConstLabels: labels,
		}, []string{"tool", "status"}),

		ToolCallDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "omnia_runtime_tool_call_duration_seconds",
			Help:        "Tool call duration in seconds",
			ConstLabels: labels,
			Buckets:     toolBuckets,
		}, []string{"tool"}),

		// Pipeline metrics
		PipelinesActive: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "omnia_runtime_pipelines_active",
			Help:        "Number of currently active pipelines",
			ConstLabels: labels,
		}, []string{}),

		PipelineDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "omnia_runtime_pipeline_duration_seconds",
			Help:        "Pipeline execution duration in seconds",
			ConstLabels: labels,
			Buckets:     pipelineBuckets,
		}, []string{"status"}),

		// Stage metrics
		StageElementsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_runtime_stage_elements_total",
			Help:        "Total number of stage executions",
			ConstLabels: labels,
		}, []string{"stage", "status"}),

		StageDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "omnia_runtime_stage_duration_seconds",
			Help:        "Stage execution duration in seconds",
			ConstLabels: labels,
			Buckets:     stageBuckets,
		}, []string{"stage", "stage_type"}),

		// Validation metrics
		ValidationsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_runtime_validations_total",
			Help:        "Total number of validations",
			ConstLabels: labels,
		}, []string{"validator", "validator_type", "status"}),

		ValidationDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "omnia_runtime_validation_duration_seconds",
			Help:        "Validation duration in seconds",
			ConstLabels: labels,
			Buckets:     validationBuckets,
		}, []string{"validator", "validator_type"}),
	}
}

// ToolCallMetrics contains the metrics for a single tool call.
type ToolCallMetrics struct {
	ToolName        string
	DurationSeconds float64
	Success         bool
}

// RecordToolCall records metrics for a tool call.
func (m *RuntimeMetrics) RecordToolCall(tc ToolCallMetrics) {
	status := StatusSuccess
	if !tc.Success {
		status = StatusError
	}

	m.ToolCallsTotal.WithLabelValues(tc.ToolName, status).Inc()
	m.ToolCallDuration.WithLabelValues(tc.ToolName).Observe(tc.DurationSeconds)
}

// PipelineMetrics contains the metrics for a pipeline execution.
type PipelineMetrics struct {
	DurationSeconds float64
	Success         bool
}

// RecordPipelineStart records the start of a pipeline execution.
func (m *RuntimeMetrics) RecordPipelineStart() {
	m.PipelinesActive.WithLabelValues().Inc()
}

// RecordPipelineEnd records the end of a pipeline execution.
func (m *RuntimeMetrics) RecordPipelineEnd(pm PipelineMetrics) {
	status := StatusSuccess
	if !pm.Success {
		status = StatusError
	}

	m.PipelinesActive.WithLabelValues().Dec()
	m.PipelineDuration.WithLabelValues(status).Observe(pm.DurationSeconds)
}

// StageMetrics contains the metrics for a stage execution.
type StageMetrics struct {
	StageName       string
	StageType       string
	DurationSeconds float64
	Success         bool
}

// RecordStage records metrics for a stage execution.
func (m *RuntimeMetrics) RecordStage(sm StageMetrics) {
	status := StatusSuccess
	if !sm.Success {
		status = StatusError
	}

	m.StageElementsTotal.WithLabelValues(sm.StageName, status).Inc()
	m.StageDuration.WithLabelValues(sm.StageName, sm.StageType).Observe(sm.DurationSeconds)
}

// ValidationMetrics contains the metrics for a validation execution.
type ValidationMetrics struct {
	ValidatorName   string
	ValidatorType   string
	DurationSeconds float64
	Success         bool
}

// RecordValidation records metrics for a validation execution.
func (m *RuntimeMetrics) RecordValidation(vm ValidationMetrics) {
	status := StatusSuccess
	if !vm.Success {
		status = StatusError
	}

	m.ValidationsTotal.WithLabelValues(vm.ValidatorName, vm.ValidatorType, status).Inc()
	m.ValidationDuration.WithLabelValues(vm.ValidatorName, vm.ValidatorType).Observe(vm.DurationSeconds)
}

// RuntimeMetricsRecorder is the interface for recording runtime metrics.
// This allows for no-op implementations when metrics are disabled.
type RuntimeMetricsRecorder interface {
	RecordToolCall(tc ToolCallMetrics)
	RecordPipelineStart()
	RecordPipelineEnd(pm PipelineMetrics)
	RecordStage(sm StageMetrics)
	RecordValidation(vm ValidationMetrics)
}

// NoOpRuntimeMetrics is a no-op implementation for when metrics are disabled.
type NoOpRuntimeMetrics struct{}

// RecordToolCall is a no-op implementation for disabled metrics.
func (n *NoOpRuntimeMetrics) RecordToolCall(_ ToolCallMetrics) {
	// Intentionally empty - metrics are disabled
}

// RecordPipelineStart is a no-op implementation for disabled metrics.
func (n *NoOpRuntimeMetrics) RecordPipelineStart() {
	// Intentionally empty - metrics are disabled
}

// RecordPipelineEnd is a no-op implementation for disabled metrics.
func (n *NoOpRuntimeMetrics) RecordPipelineEnd(_ PipelineMetrics) {
	// Intentionally empty - metrics are disabled
}

// RecordStage is a no-op implementation for disabled metrics.
func (n *NoOpRuntimeMetrics) RecordStage(_ StageMetrics) {
	// Intentionally empty - metrics are disabled
}

// RecordValidation is a no-op implementation for disabled metrics.
func (n *NoOpRuntimeMetrics) RecordValidation(_ ValidationMetrics) {
	// Intentionally empty - metrics are disabled
}
