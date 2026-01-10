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

func TestNewRuntimeMetrics(t *testing.T) {
	// Use a custom registry to avoid conflicts with global registry
	reg := prometheus.NewRegistry()

	cfg := RuntimeMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	}

	// Create metrics with custom registry
	m := newRuntimeMetricsWithRegistry(cfg, reg)
	if m == nil {
		t.Fatal("NewRuntimeMetrics returned nil")
	}

	// Verify all metrics are initialized
	if m.ToolCallsTotal == nil {
		t.Error("ToolCallsTotal is nil")
	}
	if m.ToolCallDuration == nil {
		t.Error("ToolCallDuration is nil")
	}
	if m.PipelinesActive == nil {
		t.Error("PipelinesActive is nil")
	}
	if m.PipelineDuration == nil {
		t.Error("PipelineDuration is nil")
	}
	if m.StageElementsTotal == nil {
		t.Error("StageElementsTotal is nil")
	}
	if m.StageDuration == nil {
		t.Error("StageDuration is nil")
	}
	if m.ValidationsTotal == nil {
		t.Error("ValidationsTotal is nil")
	}
	if m.ValidationDuration == nil {
		t.Error("ValidationDuration is nil")
	}
}

func TestNewRuntimeMetrics_Promauto(t *testing.T) {
	// Test actual NewRuntimeMetrics with promauto - uses unique names to avoid conflicts
	cfg := RuntimeMetricsConfig{
		AgentName: "promauto-runtime-agent",
		Namespace: "promauto-runtime-ns",
	}

	m := NewRuntimeMetrics(cfg)
	if m == nil {
		t.Fatal("NewRuntimeMetrics returned nil")
	}

	// Verify all metrics are initialized
	if m.ToolCallsTotal == nil {
		t.Error("ToolCallsTotal is nil")
	}
	if m.ToolCallDuration == nil {
		t.Error("ToolCallDuration is nil")
	}
	if m.PipelinesActive == nil {
		t.Error("PipelinesActive is nil")
	}
	if m.PipelineDuration == nil {
		t.Error("PipelineDuration is nil")
	}
	if m.StageElementsTotal == nil {
		t.Error("StageElementsTotal is nil")
	}
	if m.StageDuration == nil {
		t.Error("StageDuration is nil")
	}
	if m.ValidationsTotal == nil {
		t.Error("ValidationsTotal is nil")
	}
	if m.ValidationDuration == nil {
		t.Error("ValidationDuration is nil")
	}
}

func TestNewRuntimeMetrics_CustomBuckets(t *testing.T) {
	reg := prometheus.NewRegistry()
	customToolBuckets := []float64{0.1, 0.5, 1.0}
	customPipelineBuckets := []float64{1, 5, 10}
	customStageBuckets := []float64{0.01, 0.1, 1.0}
	customValidationBuckets := []float64{0.001, 0.01, 0.1}

	cfg := RuntimeMetricsConfig{
		AgentName:                 "test-agent",
		Namespace:                 "test-ns",
		ToolDurationBuckets:       customToolBuckets,
		PipelineDurationBuckets:   customPipelineBuckets,
		StageDurationBuckets:      customStageBuckets,
		ValidationDurationBuckets: customValidationBuckets,
	}

	m := newRuntimeMetricsWithRegistry(cfg, reg)
	if m == nil {
		t.Fatal("NewRuntimeMetrics returned nil")
	}

	// Verify histograms are initialized
	if m.ToolCallDuration == nil {
		t.Error("ToolCallDuration is nil")
	}
	if m.PipelineDuration == nil {
		t.Error("PipelineDuration is nil")
	}
	if m.StageDuration == nil {
		t.Error("StageDuration is nil")
	}
	if m.ValidationDuration == nil {
		t.Error("ValidationDuration is nil")
	}
}

func TestRuntimeMetrics_RecordToolCall_Success(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := RuntimeMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	}

	m := newRuntimeMetricsWithRegistry(cfg, reg)

	tc := ToolCallMetrics{
		ToolName:        "web_search",
		DurationSeconds: 0.5,
		Success:         true,
	}

	// Should not panic
	m.RecordToolCall(tc)

	// Verify metrics were recorded by gathering them
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Check that we have metrics
	if len(metrics) == 0 {
		t.Error("No metrics gathered")
	}

	// Verify specific metric families exist
	metricNames := make(map[string]bool)
	for _, mf := range metrics {
		metricNames[mf.GetName()] = true
	}

	expectedNames := []string{
		"omnia_runtime_tool_calls_total",
		"omnia_runtime_tool_call_duration_seconds",
	}

	for _, name := range expectedNames {
		if !metricNames[name] {
			t.Errorf("Expected metric %q not found", name)
		}
	}
}

func TestRuntimeMetrics_RecordToolCall_Error(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := RuntimeMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	}

	m := newRuntimeMetricsWithRegistry(cfg, reg)

	tc := ToolCallMetrics{
		ToolName:        "web_search",
		DurationSeconds: 0.1,
		Success:         false, // Error case
	}

	// Should not panic
	m.RecordToolCall(tc)

	// Verify metrics were recorded
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(metrics) == 0 {
		t.Error("No metrics gathered")
	}
}

func TestRuntimeMetrics_RecordPipeline(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := RuntimeMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	}

	m := newRuntimeMetricsWithRegistry(cfg, reg)

	// Start a pipeline
	m.RecordPipelineStart()

	// End the pipeline
	pm := PipelineMetrics{
		DurationSeconds: 5.0,
		Success:         true,
	}
	m.RecordPipelineEnd(pm)

	// Verify metrics were recorded by gathering them
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Check that we have metrics
	if len(metrics) == 0 {
		t.Error("No metrics gathered")
	}

	// Verify specific metric families exist
	metricNames := make(map[string]bool)
	for _, mf := range metrics {
		metricNames[mf.GetName()] = true
	}

	expectedNames := []string{
		"omnia_runtime_pipelines_active",
		"omnia_runtime_pipeline_duration_seconds",
	}

	for _, name := range expectedNames {
		if !metricNames[name] {
			t.Errorf("Expected metric %q not found", name)
		}
	}
}

func TestRuntimeMetrics_RecordPipeline_Error(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := RuntimeMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	}

	m := newRuntimeMetricsWithRegistry(cfg, reg)

	// Start a pipeline
	m.RecordPipelineStart()

	// End the pipeline with error
	pm := PipelineMetrics{
		DurationSeconds: 1.0,
		Success:         false, // Error case
	}
	m.RecordPipelineEnd(pm)

	// Verify metrics were recorded
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(metrics) == 0 {
		t.Error("No metrics gathered")
	}
}

func TestRuntimeMetrics_RecordStage(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := RuntimeMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	}

	m := newRuntimeMetricsWithRegistry(cfg, reg)

	sm := StageMetrics{
		StageName:       "generate",
		StageType:       "transform",
		DurationSeconds: 0.5,
		Success:         true,
	}

	// Should not panic
	m.RecordStage(sm)

	// Verify metrics were recorded
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	metricNames := make(map[string]bool)
	for _, mf := range metrics {
		metricNames[mf.GetName()] = true
	}

	expectedNames := []string{
		"omnia_runtime_stage_elements_total",
		"omnia_runtime_stage_duration_seconds",
	}

	for _, name := range expectedNames {
		if !metricNames[name] {
			t.Errorf("Expected metric %q not found", name)
		}
	}
}

func TestRuntimeMetrics_RecordStage_Error(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := RuntimeMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	}

	m := newRuntimeMetricsWithRegistry(cfg, reg)

	sm := StageMetrics{
		StageName:       "generate",
		StageType:       "transform",
		DurationSeconds: 0.1,
		Success:         false,
	}

	m.RecordStage(sm)

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(metrics) == 0 {
		t.Error("No metrics gathered")
	}
}

func TestRuntimeMetrics_RecordValidation(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := RuntimeMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	}

	m := newRuntimeMetricsWithRegistry(cfg, reg)

	vm := ValidationMetrics{
		ValidatorName:   "json_schema",
		ValidatorType:   "output",
		DurationSeconds: 0.01,
		Success:         true,
	}

	// Should not panic
	m.RecordValidation(vm)

	// Verify metrics were recorded
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	metricNames := make(map[string]bool)
	for _, mf := range metrics {
		metricNames[mf.GetName()] = true
	}

	expectedNames := []string{
		"omnia_runtime_validations_total",
		"omnia_runtime_validation_duration_seconds",
	}

	for _, name := range expectedNames {
		if !metricNames[name] {
			t.Errorf("Expected metric %q not found", name)
		}
	}
}

func TestRuntimeMetrics_RecordValidation_Error(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := RuntimeMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	}

	m := newRuntimeMetricsWithRegistry(cfg, reg)

	vm := ValidationMetrics{
		ValidatorName:   "json_schema",
		ValidatorType:   "output",
		DurationSeconds: 0.005,
		Success:         false,
	}

	m.RecordValidation(vm)

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(metrics) == 0 {
		t.Error("No metrics gathered")
	}
}

func TestNoOpRuntimeMetrics(t *testing.T) {
	m := &NoOpRuntimeMetrics{}

	// All these should not panic - no-op implementation
	m.RecordToolCall(ToolCallMetrics{
		ToolName:        "test-tool",
		DurationSeconds: 0.5,
		Success:         true,
	})

	m.RecordPipelineStart()

	m.RecordPipelineEnd(PipelineMetrics{
		DurationSeconds: 1.0,
		Success:         true,
	})

	m.RecordStage(StageMetrics{
		StageName:       "test-stage",
		StageType:       "transform",
		DurationSeconds: 0.1,
		Success:         true,
	})

	m.RecordValidation(ValidationMetrics{
		ValidatorName:   "test-validator",
		ValidatorType:   "output",
		DurationSeconds: 0.01,
		Success:         true,
	})
}

func TestRuntimeMetricsRecorder_Interface(t *testing.T) {
	// Verify both implementations satisfy the interface
	var _ RuntimeMetricsRecorder = &RuntimeMetrics{}
	var _ RuntimeMetricsRecorder = &NoOpRuntimeMetrics{}
}

func TestDefaultToolDurationBuckets(t *testing.T) {
	// Verify default buckets are reasonable
	if len(DefaultToolDurationBuckets) == 0 {
		t.Error("DefaultToolDurationBuckets is empty")
	}

	// Verify buckets are in ascending order
	for i := 1; i < len(DefaultToolDurationBuckets); i++ {
		if DefaultToolDurationBuckets[i] <= DefaultToolDurationBuckets[i-1] {
			t.Errorf("Buckets not in ascending order: %v", DefaultToolDurationBuckets)
		}
	}
}

func TestDefaultStageDurationBuckets(t *testing.T) {
	if len(DefaultStageDurationBuckets) == 0 {
		t.Error("DefaultStageDurationBuckets is empty")
	}

	for i := 1; i < len(DefaultStageDurationBuckets); i++ {
		if DefaultStageDurationBuckets[i] <= DefaultStageDurationBuckets[i-1] {
			t.Errorf("Buckets not in ascending order: %v", DefaultStageDurationBuckets)
		}
	}
}

func TestDefaultValidationDurationBuckets(t *testing.T) {
	if len(DefaultValidationDurationBuckets) == 0 {
		t.Error("DefaultValidationDurationBuckets is empty")
	}

	for i := 1; i < len(DefaultValidationDurationBuckets); i++ {
		if DefaultValidationDurationBuckets[i] <= DefaultValidationDurationBuckets[i-1] {
			t.Errorf("Buckets not in ascending order: %v", DefaultValidationDurationBuckets)
		}
	}
}

// newRuntimeMetricsWithRegistry creates Runtime metrics with a custom registry for testing.
// This avoids conflicts with the global prometheus registry during tests.
func newRuntimeMetricsWithRegistry(cfg RuntimeMetricsConfig, reg *prometheus.Registry) *RuntimeMetrics {
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

	// Tool metrics
	toolCallsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "omnia_runtime_tool_calls_total",
		Help:        "Total number of tool calls",
		ConstLabels: labels,
	}, []string{"tool", "status"})

	toolCallDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "omnia_runtime_tool_call_duration_seconds",
		Help:        "Tool call duration in seconds",
		ConstLabels: labels,
		Buckets:     toolBuckets,
	}, []string{"tool"})

	// Pipeline metrics
	pipelinesActive := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name:        "omnia_runtime_pipelines_active",
		Help:        "Number of currently active pipelines",
		ConstLabels: labels,
	}, []string{})

	pipelineDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "omnia_runtime_pipeline_duration_seconds",
		Help:        "Pipeline execution duration in seconds",
		ConstLabels: labels,
		Buckets:     pipelineBuckets,
	}, []string{"status"})

	// Stage metrics
	stageElementsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "omnia_runtime_stage_elements_total",
		Help:        "Total number of stage executions",
		ConstLabels: labels,
	}, []string{"stage", "status"})

	stageDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "omnia_runtime_stage_duration_seconds",
		Help:        "Stage execution duration in seconds",
		ConstLabels: labels,
		Buckets:     stageBuckets,
	}, []string{"stage", "stage_type"})

	// Validation metrics
	validationsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "omnia_runtime_validations_total",
		Help:        "Total number of validations",
		ConstLabels: labels,
	}, []string{"validator", "validator_type", "status"})

	validationDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "omnia_runtime_validation_duration_seconds",
		Help:        "Validation duration in seconds",
		ConstLabels: labels,
		Buckets:     validationBuckets,
	}, []string{"validator", "validator_type"})

	// Register with provided registry
	reg.MustRegister(toolCallsTotal, toolCallDuration, pipelinesActive, pipelineDuration)
	reg.MustRegister(stageElementsTotal, stageDuration, validationsTotal, validationDuration)

	return &RuntimeMetrics{
		ToolCallsTotal:     toolCallsTotal,
		ToolCallDuration:   toolCallDuration,
		PipelinesActive:    pipelinesActive,
		PipelineDuration:   pipelineDuration,
		StageElementsTotal: stageElementsTotal,
		StageDuration:      stageDuration,
		ValidationsTotal:   validationsTotal,
		ValidationDuration: validationDuration,
	}
}
