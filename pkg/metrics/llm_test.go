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

func TestNewLLMMetrics(t *testing.T) {
	// Use a custom registry to avoid conflicts with global registry
	reg := prometheus.NewRegistry()

	cfg := LLMMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	}

	// Create metrics with custom registry
	m := newLLMMetricsWithRegistry(cfg, reg)
	if m == nil {
		t.Fatal("NewLLMMetrics returned nil")
	}

	// Verify all metrics are initialized
	if m.InputTokensTotal == nil {
		t.Error("InputTokensTotal is nil")
	}
	if m.OutputTokensTotal == nil {
		t.Error("OutputTokensTotal is nil")
	}
	if m.CacheHitsTotal == nil {
		t.Error("CacheHitsTotal is nil")
	}
	if m.RequestsTotal == nil {
		t.Error("RequestsTotal is nil")
	}
	if m.CostUSDTotal == nil {
		t.Error("CostUSDTotal is nil")
	}
	if m.RequestDuration == nil {
		t.Error("RequestDuration is nil")
	}
}

func TestNewLLMMetrics_Promauto(t *testing.T) {
	// Test actual NewLLMMetrics with promauto - uses unique names to avoid conflicts
	// Note: promauto registers to global registry, so we use unique agent/namespace
	cfg := LLMMetricsConfig{
		AgentName: "promauto-test-agent",
		Namespace: "promauto-test-ns",
	}

	m := NewLLMMetrics(cfg)
	if m == nil {
		t.Fatal("NewLLMMetrics returned nil")
	}

	// Verify all metrics are initialized
	if m.InputTokensTotal == nil {
		t.Error("InputTokensTotal is nil")
	}
	if m.OutputTokensTotal == nil {
		t.Error("OutputTokensTotal is nil")
	}
	if m.CacheHitsTotal == nil {
		t.Error("CacheHitsTotal is nil")
	}
	if m.RequestsTotal == nil {
		t.Error("RequestsTotal is nil")
	}
	if m.CostUSDTotal == nil {
		t.Error("CostUSDTotal is nil")
	}
	if m.RequestDuration == nil {
		t.Error("RequestDuration is nil")
	}

	// Exercise RecordRequest on the promauto-created metrics
	m.RecordRequest(LLMRequestMetrics{
		Provider:        "test-provider",
		Model:           "test-model",
		InputTokens:     10,
		OutputTokens:    5,
		CacheHits:       1,
		CostUSD:         0.001,
		DurationSeconds: 0.1,
		Success:         true,
	})
}

func TestNewLLMMetrics_PromautoCustomBuckets(t *testing.T) {
	// Test actual NewLLMMetrics with custom buckets
	cfg := LLMMetricsConfig{
		AgentName:       "promauto-bucket-agent",
		Namespace:       "promauto-bucket-ns",
		DurationBuckets: []float64{0.1, 0.5, 1.0},
	}

	m := NewLLMMetrics(cfg)
	if m == nil {
		t.Fatal("NewLLMMetrics returned nil")
	}

	if m.RequestDuration == nil {
		t.Error("RequestDuration is nil")
	}
}

func TestNewLLMMetrics_CustomBuckets(t *testing.T) {
	reg := prometheus.NewRegistry()
	customBuckets := []float64{1, 2, 3}

	cfg := LLMMetricsConfig{
		AgentName:       "test-agent",
		Namespace:       "test-ns",
		DurationBuckets: customBuckets,
	}

	m := newLLMMetricsWithRegistry(cfg, reg)
	if m == nil {
		t.Fatal("NewLLMMetrics returned nil")
	}

	// RequestDuration should be initialized with custom buckets
	if m.RequestDuration == nil {
		t.Error("RequestDuration is nil")
	}
}

func TestLLMMetrics_RecordRequest_Success(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := LLMMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	}

	m := newLLMMetricsWithRegistry(cfg, reg)

	req := LLMRequestMetrics{
		Provider:        "anthropic",
		Model:           "claude-sonnet-4",
		InputTokens:     100,
		OutputTokens:    50,
		CacheHits:       10,
		CostUSD:         0.005,
		DurationSeconds: 1.5,
		Success:         true,
	}

	// Should not panic
	m.RecordRequest(req)

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
		"omnia_llm_input_tokens_total",
		"omnia_llm_output_tokens_total",
		"omnia_llm_cache_hits_total",
		"omnia_llm_requests_total",
		"omnia_llm_cost_usd_total",
		"omnia_llm_request_duration_seconds",
	}

	for _, name := range expectedNames {
		if !metricNames[name] {
			t.Errorf("Expected metric %q not found", name)
		}
	}
}

func TestLLMMetrics_RecordRequest_Error(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := LLMMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	}

	m := newLLMMetricsWithRegistry(cfg, reg)

	req := LLMRequestMetrics{
		Provider:        "anthropic",
		Model:           "claude-sonnet-4",
		InputTokens:     100,
		OutputTokens:    0,
		CacheHits:       0,
		CostUSD:         0.001,
		DurationSeconds: 0.5,
		Success:         false, // Error case
	}

	// Should not panic
	m.RecordRequest(req)

	// Verify metrics were recorded
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(metrics) == 0 {
		t.Error("No metrics gathered")
	}
}

func TestNoOpLLMMetrics_RecordRequest(t *testing.T) {
	m := &NoOpLLMMetrics{}

	req := LLMRequestMetrics{
		Provider:        "anthropic",
		Model:           "claude-sonnet-4",
		InputTokens:     100,
		OutputTokens:    50,
		CacheHits:       10,
		CostUSD:         0.005,
		DurationSeconds: 1.5,
		Success:         true,
	}

	// Should not panic - no-op implementation
	m.RecordRequest(req)
}

func TestLLMMetricsRecorder_Interface(t *testing.T) {
	// Verify both implementations satisfy the interface
	var _ LLMMetricsRecorder = &LLMMetrics{}
	var _ LLMMetricsRecorder = &NoOpLLMMetrics{}
}

func TestDefaultLLMDurationBuckets(t *testing.T) {
	// Verify default buckets are reasonable
	if len(DefaultLLMDurationBuckets) == 0 {
		t.Error("DefaultLLMDurationBuckets is empty")
	}

	// Verify buckets are in ascending order
	for i := 1; i < len(DefaultLLMDurationBuckets); i++ {
		if DefaultLLMDurationBuckets[i] <= DefaultLLMDurationBuckets[i-1] {
			t.Errorf("Buckets not in ascending order: %v", DefaultLLMDurationBuckets)
		}
	}

	// Verify we have reasonable range for LLM calls (up to 5 minutes)
	maxBucket := DefaultLLMDurationBuckets[len(DefaultLLMDurationBuckets)-1]
	if maxBucket < 60 {
		t.Errorf("Max bucket %v is too small for LLM calls", maxBucket)
	}
}

func TestLLMMetrics_Initialize(t *testing.T) {
	reg := prometheus.NewRegistry()
	cfg := LLMMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-ns",
	}

	m := newLLMMetricsWithRegistry(cfg, reg)

	// Initialize metrics with provider and model
	m.Initialize("anthropic", "claude-sonnet-4")

	// Verify metrics were pre-registered by gathering them
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	// Check that we have metrics registered
	if len(metrics) == 0 {
		t.Error("No metrics gathered after Initialize")
	}

	// Verify specific metric families exist with the initialized labels
	metricNames := make(map[string]bool)
	for _, mf := range metrics {
		metricNames[mf.GetName()] = true
	}

	expectedNames := []string{
		"omnia_llm_input_tokens_total",
		"omnia_llm_output_tokens_total",
		"omnia_llm_cache_hits_total",
		"omnia_llm_requests_total",
		"omnia_llm_cost_usd_total",
	}

	for _, name := range expectedNames {
		if !metricNames[name] {
			t.Errorf("Expected metric %q not found after Initialize", name)
		}
	}
}

// newLLMMetricsWithRegistry creates LLM metrics with a custom registry for testing.
// This avoids conflicts with the global prometheus registry during tests.
func newLLMMetricsWithRegistry(cfg LLMMetricsConfig, reg *prometheus.Registry) *LLMMetrics {
	labels := prometheus.Labels{
		"agent":     cfg.AgentName,
		"namespace": cfg.Namespace,
	}

	buckets := cfg.DurationBuckets
	if buckets == nil {
		buckets = DefaultLLMDurationBuckets
	}

	inputTokens := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "omnia_llm_input_tokens_total",
		Help:        "Total number of input tokens sent to LLMs",
		ConstLabels: labels,
	}, []string{"provider", "model"})

	outputTokens := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "omnia_llm_output_tokens_total",
		Help:        "Total number of output tokens received from LLMs",
		ConstLabels: labels,
	}, []string{"provider", "model"})

	cacheHits := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "omnia_llm_cache_hits_total",
		Help:        "Total number of prompt cache hits",
		ConstLabels: labels,
	}, []string{"provider", "model"})

	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "omnia_llm_requests_total",
		Help:        "Total number of LLM requests",
		ConstLabels: labels,
	}, []string{"provider", "model", "status"})

	cost := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "omnia_llm_cost_usd_total",
		Help:        "Total estimated cost in USD",
		ConstLabels: labels,
	}, []string{"provider", "model"})

	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "omnia_llm_request_duration_seconds",
		Help:        "LLM request duration in seconds",
		ConstLabels: labels,
		Buckets:     buckets,
	}, []string{"provider", "model"})

	// Register with provided registry
	reg.MustRegister(inputTokens, outputTokens, cacheHits, requests, cost, duration)

	return &LLMMetrics{
		InputTokensTotal:  inputTokens,
		OutputTokensTotal: outputTokens,
		CacheHitsTotal:    cacheHits,
		RequestsTotal:     requests,
		CostUSDTotal:      cost,
		RequestDuration:   duration,
	}
}
