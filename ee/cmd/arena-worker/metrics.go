/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const defaultMetricsAddr = ":9090"

// Label key constants used across arena worker metrics.
const (
	labelJobName   = "job_name"
	labelStatus    = "status"
	labelScenario  = "scenario"
	labelProvider  = "provider"
	labelErrorType = "error_type"
	labelDirection = "direction"
)

// WorkerMetrics holds Prometheus metrics for arena work item execution.
// These complement the queue-level metrics in ee/pkg/arena/queue/metrics.go
// by tracking per-item execution outcomes.
type WorkerMetrics struct {
	// WorkItemsTotal counts completed and failed work items.
	WorkItemsTotal *prometheus.CounterVec

	// WorkItemDuration tracks work item execution duration.
	WorkItemDuration *prometheus.HistogramVec

	// TurnLatency tracks end-to-end turn latency per scenario and provider.
	TurnLatency *prometheus.HistogramVec

	// TTFTDuration tracks time-to-first-token per scenario and provider.
	TTFTDuration *prometheus.HistogramVec

	// ActiveVUs tracks the current number of active virtual users.
	ActiveVUs prometheus.Gauge

	// TrialsTotal counts individual trials by status (pass/fail).
	TrialsTotal *prometheus.CounterVec

	// ErrorsTotal counts errors by type.
	ErrorsTotal *prometheus.CounterVec

	// TokensTotal counts input and output tokens.
	TokensTotal *prometheus.CounterVec
}

// DefaultWorkItemDurationBuckets covers the range from fast rule-based scenarios
// (100ms) to slow multi-turn LLM conversations (10min).
var DefaultWorkItemDurationBuckets = []float64{
	0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600,
}

// DefaultTurnLatencyBuckets covers the typical range for a single LLM turn.
var DefaultTurnLatencyBuckets = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60}

// DefaultTTFTBuckets covers the typical range for time-to-first-token.
var DefaultTTFTBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

// NewWorkerMetrics creates and registers arena worker metrics
// using the default Prometheus registry.
func NewWorkerMetrics() *WorkerMetrics {
	return newWorkerMetricsWithRegisterer(prometheus.DefaultRegisterer)
}

// newWorkerMetricsWithRegisterer creates WorkerMetrics registered against the
// given registerer. Use prometheus.NewRegistry() in tests to avoid duplicate registration.
func newWorkerMetricsWithRegisterer(reg prometheus.Registerer) *WorkerMetrics {
	factory := promauto.With(reg)
	return &WorkerMetrics{
		WorkItemsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_arena_work_items_total",
			Help: "Total arena work items processed by status (completed/failed)",
		}, []string{labelJobName, labelStatus}),

		WorkItemDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "omnia_arena_work_item_duration_seconds",
			Help:    "Arena work item execution duration in seconds",
			Buckets: DefaultWorkItemDurationBuckets,
		}, []string{labelJobName}),

		TurnLatency: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "omnia_arena_turn_latency_seconds",
			Help:    "Arena LLM turn latency in seconds",
			Buckets: DefaultTurnLatencyBuckets,
		}, []string{labelJobName, labelScenario, labelProvider}),

		TTFTDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "omnia_arena_ttft_seconds",
			Help:    "Arena time-to-first-token in seconds",
			Buckets: DefaultTTFTBuckets,
		}, []string{labelJobName, labelScenario, labelProvider}),

		ActiveVUs: factory.NewGauge(prometheus.GaugeOpts{
			Name: "omnia_arena_active_vus",
			Help: "Current number of active arena virtual users",
		}),

		TrialsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_arena_trials_total",
			Help: "Total arena trials by status",
		}, []string{labelJobName, labelScenario, labelProvider, labelStatus}),

		ErrorsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_arena_errors_total",
			Help: "Total arena errors by type",
		}, []string{labelJobName, labelProvider, labelErrorType}),

		TokensTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_arena_tokens_total",
			Help: "Total arena tokens by direction (input/output)",
		}, []string{labelJobName, labelProvider, labelDirection}),
	}
}

// RecordWorkItem records a completed or failed work item.
func (m *WorkerMetrics) RecordWorkItem(jobName, status string, durationSec float64) {
	m.WorkItemsTotal.WithLabelValues(jobName, status).Inc()
	m.WorkItemDuration.WithLabelValues(jobName).Observe(durationSec)
}

// RecordTurnLatency records the end-to-end latency for a single LLM turn.
func (m *WorkerMetrics) RecordTurnLatency(jobName, scenario, provider string, seconds float64) {
	m.TurnLatency.WithLabelValues(jobName, scenario, provider).Observe(seconds)
}

// RecordTTFT records time-to-first-token for a single LLM turn.
func (m *WorkerMetrics) RecordTTFT(jobName, scenario, provider string, seconds float64) {
	m.TTFTDuration.WithLabelValues(jobName, scenario, provider).Observe(seconds)
}

// RecordTrial records a trial outcome (pass/fail).
func (m *WorkerMetrics) RecordTrial(jobName, scenario, provider, status string) {
	m.TrialsTotal.WithLabelValues(jobName, scenario, provider, status).Inc()
}

// RecordError records an error by type.
func (m *WorkerMetrics) RecordError(jobName, provider, errorType string) {
	m.ErrorsTotal.WithLabelValues(jobName, provider, errorType).Inc()
}

// RecordTokens records token counts by direction (input/output).
func (m *WorkerMetrics) RecordTokens(jobName, provider, direction string, count float64) {
	m.TokensTotal.WithLabelValues(jobName, provider, direction).Add(count)
}

// SetActiveVUs sets the current number of active virtual users.
func (m *WorkerMetrics) SetActiveVUs(count float64) {
	m.ActiveVUs.Set(count)
}

// newMetricsMux creates the HTTP handler mux with /metrics, /healthz, and /readyz endpoints.
func newMetricsMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// startMetricsServer starts the Prometheus metrics and health probe HTTP server.
func startMetricsServer(addr string, log logr.Logger) {
	server := &http.Server{
		Addr:              addr,
		Handler:           newMetricsMux(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Info("starting metrics/health server", "addr", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error(err, "metrics server failed")
	}
}
