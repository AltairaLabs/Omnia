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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the runtime.
// These metrics track LLM usage for cost analysis and monitoring.
type Metrics struct {
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

// NewMetrics creates and registers all Prometheus metrics for the runtime.
func NewMetrics(agentName, namespace string) *Metrics {
	labels := prometheus.Labels{
		"agent":     agentName,
		"namespace": namespace,
	}

	return &Metrics{
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
			Buckets:     []float64{0.5, 1, 2, 5, 10, 30, 60, 120, 300}, // LLM calls can be slow
		}, []string{"provider", "model"}),
	}
}

// RecordRequest records metrics for an LLM request.
func (m *Metrics) RecordRequest(provider, model string, inputTokens, outputTokens, cacheHits int, costUSD float64, durationSeconds float64, success bool) {
	status := "success"
	if !success {
		status = "error"
	}

	m.InputTokensTotal.WithLabelValues(provider, model).Add(float64(inputTokens))
	m.OutputTokensTotal.WithLabelValues(provider, model).Add(float64(outputTokens))
	m.CacheHitsTotal.WithLabelValues(provider, model).Add(float64(cacheHits))
	m.RequestsTotal.WithLabelValues(provider, model, status).Inc()
	m.CostUSDTotal.WithLabelValues(provider, model).Add(costUSD)
	m.RequestDuration.WithLabelValues(provider, model).Observe(durationSeconds)
}

// NoOpMetrics is a no-op implementation for when metrics are disabled.
type NoOpMetrics struct{}

func (n *NoOpMetrics) RecordRequest(provider, model string, inputTokens, outputTokens, cacheHits int, costUSD float64, durationSeconds float64, success bool) {
}
