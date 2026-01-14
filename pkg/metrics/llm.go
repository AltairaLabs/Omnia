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

// Package metrics provides shared Prometheus metrics for Omnia components.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Status label constants for metrics.
const (
	StatusSuccess = "success"
	StatusError   = "error"
)

// LLMMetrics holds all Prometheus metrics for LLM interactions.
// These metrics track LLM usage for cost analysis and monitoring.
// This is shared between the runtime and demo handler.
type LLMMetrics struct {
	// InputTokensTotal is the total number of input tokens sent to LLMs.
	InputTokensTotal *prometheus.CounterVec

	// OutputTokensTotal is the total number of output tokens received from LLMs.
	OutputTokensTotal *prometheus.CounterVec

	// CacheHitsTotal is the total number of cache hits (prompt caching).
	CacheHitsTotal *prometheus.CounterVec

	// RequestsTotal is the total number of LLM requests.
	RequestsTotal *prometheus.CounterVec

	// CostUSDTotal is the total estimated cost in USD.
	CostUSDTotal *prometheus.CounterVec

	// RequestDuration is the histogram of LLM request durations.
	RequestDuration *prometheus.HistogramVec
}

// LLMMetricsConfig configures the LLM metrics.
type LLMMetricsConfig struct {
	AgentName string
	Namespace string

	// PromptPack CRD reference (for Grafana queries)
	PromptPackName      string
	PromptPackNamespace string

	// Provider CRD reference (for Grafana queries, only when using providerRef)
	ProviderRefName      string
	ProviderRefNamespace string

	// Buckets for the request duration histogram.
	// If nil, defaults to standard LLM buckets.
	DurationBuckets []float64
}

// DefaultLLMDurationBuckets are the default histogram buckets for LLM request durations.
// LLM calls can be slow, so buckets extend up to 5 minutes.
var DefaultLLMDurationBuckets = []float64{0.5, 1, 2, 5, 10, 30, 60, 120, 300}

// NewLLMMetrics creates and registers all Prometheus metrics for LLM interactions.
func NewLLMMetrics(cfg LLMMetricsConfig) *LLMMetrics {
	labels := prometheus.Labels{
		"agent":                  cfg.AgentName,
		"namespace":              cfg.Namespace,
		"promptpack_name":        cfg.PromptPackName,
		"promptpack_namespace":   cfg.PromptPackNamespace,
		"provider_ref_name":      cfg.ProviderRefName,
		"provider_ref_namespace": cfg.ProviderRefNamespace,
	}

	buckets := cfg.DurationBuckets
	if buckets == nil {
		buckets = DefaultLLMDurationBuckets
	}

	return &LLMMetrics{
		InputTokensTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_llm_input_tokens_total",
			Help:        "Total number of input tokens sent to LLMs",
			ConstLabels: labels,
		}, []string{"provider", "model"}),

		OutputTokensTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_llm_output_tokens_total",
			Help:        "Total number of output tokens received from LLMs",
			ConstLabels: labels,
		}, []string{"provider", "model"}),

		CacheHitsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_llm_cache_hits_total",
			Help:        "Total number of prompt cache hits",
			ConstLabels: labels,
		}, []string{"provider", "model"}),

		RequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_llm_requests_total",
			Help:        "Total number of LLM requests",
			ConstLabels: labels,
		}, []string{"provider", "model", "status"}),

		CostUSDTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_llm_cost_usd_total",
			Help:        "Total estimated cost in USD",
			ConstLabels: labels,
		}, []string{"provider", "model"}),

		RequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "omnia_llm_request_duration_seconds",
			Help:        "LLM request duration in seconds",
			ConstLabels: labels,
			Buckets:     buckets,
		}, []string{"provider", "model"}),
	}
}

// Initialize pre-registers metrics with the given label values.
// This ensures metrics appear in /metrics output immediately at startup,
// even before any LLM requests are made. For CounterVec/HistogramVec,
// Prometheus only shows metrics after they've been observed with specific label values.
func (m *LLMMetrics) Initialize(provider, model string) {
	// Initialize counters with zero (Add(0) registers the label combination)
	m.InputTokensTotal.WithLabelValues(provider, model).Add(0)
	m.OutputTokensTotal.WithLabelValues(provider, model).Add(0)
	m.CacheHitsTotal.WithLabelValues(provider, model).Add(0)
	m.RequestsTotal.WithLabelValues(provider, model, StatusSuccess).Add(0)
	m.RequestsTotal.WithLabelValues(provider, model, StatusError).Add(0)
	m.CostUSDTotal.WithLabelValues(provider, model).Add(0)
	// Histograms are initialized by observing zero (won't affect distribution)
	// Note: We don't observe 0 for histograms as it would add a data point
	// The histogram will appear once first real observation is made
}

// LLMRequestMetrics contains the metrics for a single LLM request.
type LLMRequestMetrics struct {
	Provider        string
	Model           string
	InputTokens     int
	OutputTokens    int
	CacheHits       int
	CostUSD         float64
	DurationSeconds float64
	Success         bool
}

// RecordRequest records metrics for an LLM request.
func (m *LLMMetrics) RecordRequest(req LLMRequestMetrics) {
	status := StatusSuccess
	if !req.Success {
		status = StatusError
	}

	m.InputTokensTotal.WithLabelValues(req.Provider, req.Model).Add(float64(req.InputTokens))
	m.OutputTokensTotal.WithLabelValues(req.Provider, req.Model).Add(float64(req.OutputTokens))
	m.CacheHitsTotal.WithLabelValues(req.Provider, req.Model).Add(float64(req.CacheHits))
	m.RequestsTotal.WithLabelValues(req.Provider, req.Model, status).Inc()
	m.CostUSDTotal.WithLabelValues(req.Provider, req.Model).Add(req.CostUSD)
	m.RequestDuration.WithLabelValues(req.Provider, req.Model).Observe(req.DurationSeconds)
}

// LLMMetricsRecorder is the interface for recording LLM metrics.
// This allows for no-op implementations when metrics are disabled.
type LLMMetricsRecorder interface {
	RecordRequest(req LLMRequestMetrics)
}

// NoOpLLMMetrics is a no-op implementation for when metrics are disabled.
type NoOpLLMMetrics struct{}

// RecordRequest is a no-op implementation that intentionally does nothing.
// This is used when metrics collection is disabled.
func (n *NoOpLLMMetrics) RecordRequest(_ LLMRequestMetrics) {
	// Intentionally empty: no-op implementation for when metrics are disabled
}
