/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

// Metric name constants for the analytics:aggregate opt-in worker.
// These names match the originals from internal/memory/analytics_optin_metric.go
// (deleted in CE2); CE3 re-homes the metrics here where the source-of-truth
// consent data lives.
const (
	metricAnalyticsOptInRatio   = "omnia_memory_consent_analytics_optin_ratio"
	metricAnalyticsUsersTotal   = "omnia_memory_consent_analytics_users_total"
	metricAnalyticsWorkerErrors = "omnia_memory_consent_analytics_worker_errors_total"
)

// OptInMetricWorker periodically reads consent stats from the privacy store
// and updates Prometheus gauges for the analytics opt-in ratio. It relocates
// the metrics previously owned by the memory-api AnalyticsOptInWorker (CE2
// deleted that worker; CE3 re-homes the metrics here where the source-of-truth
// consent data lives).
type OptInMetricWorker struct {
	store        privacy.ConsentStatsReader
	interval     time.Duration
	log          logr.Logger
	optInRatio   prometheus.Gauge
	usersTotal   *prometheus.GaugeVec
	workerErrors *prometheus.CounterVec
}

// NewOptInMetricWorker creates an OptInMetricWorker and registers its collectors
// with reg. MustRegister panics on duplicate registration, consistent with
// one-shot binary startup wiring.
func NewOptInMetricWorker(
	store privacy.ConsentStatsReader,
	interval time.Duration,
	reg prometheus.Registerer,
	log logr.Logger,
) *OptInMetricWorker {
	optInRatio := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: metricAnalyticsOptInRatio,
		Help: `Fraction of users who have granted the analytics:aggregate consent category (0..1). Global across all workspaces.`,
	})
	users := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: metricAnalyticsUsersTotal,
		Help: `Absolute count of users with / without the analytics:aggregate consent category. Labels: granted ("true"|"false").`,
	}, []string{"granted"})
	workerErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricAnalyticsWorkerErrors,
		Help: "Errors observed by the analytics opt-in worker. Labels: reason.",
	}, []string{"reason"})
	reg.MustRegister(optInRatio, users, workerErrors)

	return &OptInMetricWorker{
		store:        store,
		interval:     interval,
		log:          log.WithName("optin-metric"),
		optInRatio:   optInRatio,
		usersTotal:   users,
		workerErrors: workerErrors,
	}
}

// Run drives the metric-collection loop until ctx is cancelled. It collects
// immediately on the first tick so the gauges are populated at startup.
func (w *OptInMetricWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.collect(ctx)
		}
	}
}

// collect reads the current consent stats and updates the gauges. Errors are
// logged and counted but do not stop the worker.
func (w *OptInMetricWorker) collect(ctx context.Context) {
	stats, err := w.store.Stats(ctx)
	if err != nil {
		w.log.Error(err, "consent stats query failed")
		w.workerErrors.WithLabelValues("query").Inc()
		return
	}

	granted := stats.GrantsByCategory[string(privacy.ConsentAnalyticsAggregate)]
	total := stats.TotalUsers

	w.usersTotal.WithLabelValues("true").Set(float64(granted))
	w.usersTotal.WithLabelValues("false").Set(float64(total - granted))

	// Leave the ratio gauge unchanged when no users exist — oscillating to 0 on
	// every empty-DB tick creates misleading dashboards on fresh deployments.
	if total > 0 {
		w.optInRatio.Set(float64(granted) / float64(total))
	}
}
