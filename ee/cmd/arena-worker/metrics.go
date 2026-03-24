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

// WorkerMetrics holds Prometheus metrics for arena work item execution.
// These complement the queue-level metrics in ee/pkg/arena/queue/metrics.go
// by tracking per-item execution outcomes.
type WorkerMetrics struct {
	// WorkItemsTotal counts completed and failed work items.
	WorkItemsTotal *prometheus.CounterVec

	// WorkItemDuration tracks work item execution duration.
	WorkItemDuration *prometheus.HistogramVec
}

// DefaultWorkItemDurationBuckets covers the range from fast rule-based scenarios
// (100ms) to slow multi-turn LLM conversations (10min).
var DefaultWorkItemDurationBuckets = []float64{
	0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600,
}

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
		}, []string{"job_name", "status"}),

		WorkItemDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "omnia_arena_work_item_duration_seconds",
			Help:    "Arena work item execution duration in seconds",
			Buckets: DefaultWorkItemDurationBuckets,
		}, []string{"job_name"}),
	}
}

// RecordWorkItem records a completed or failed work item.
func (m *WorkerMetrics) RecordWorkItem(jobName, status string, durationSec float64) {
	m.WorkItemsTotal.WithLabelValues(jobName, status).Inc()
	m.WorkItemDuration.WithLabelValues(jobName).Observe(durationSec)
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
