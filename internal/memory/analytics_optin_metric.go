/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package memory

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

// Metric name constants for the analytics:aggregate opt-in worker.
const (
	metricAnalyticsOptInRatio   = "omnia_memory_consent_analytics_optin_ratio"
	metricAnalyticsUsersTotal   = "omnia_memory_consent_analytics_users_total"
	metricAnalyticsWorkerErrors = "omnia_memory_consent_analytics_worker_errors_total"
)

// DefaultAnalyticsOptInInterval is the default period between worker
// queries. Exported so operators can override via a future flag without
// changing the default behaviour.
const DefaultAnalyticsOptInInterval = 5 * time.Minute

// ConsentStatsSource is the minimal interface the AnalyticsOptInWorker
// needs from privacy-api. *httpclient.Client satisfies it.
type ConsentStatsSource interface {
	GetConsentStats(ctx context.Context) (*privacy.ConsentStats, error)
}

// AnalyticsOptInMetrics groups the Prometheus collectors for the
// analytics:aggregate consent opt-in worker. Construct via
// NewAnalyticsOptInMetrics, register via RegisterAnalyticsOptInMetrics.
type AnalyticsOptInMetrics struct {
	OptInRatio   prometheus.Gauge
	UsersTotal   *prometheus.GaugeVec
	WorkerErrors *prometheus.CounterVec
}

// NewAnalyticsOptInMetrics constructs a fresh collector set without
// registering it anywhere.
func NewAnalyticsOptInMetrics() *AnalyticsOptInMetrics {
	return &AnalyticsOptInMetrics{
		OptInRatio: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: metricAnalyticsOptInRatio,
			Help: "Fraction of users who have granted the analytics:aggregate consent category (0..1). Global across all workspaces.",
		}),
		UsersTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: metricAnalyticsUsersTotal,
			Help: "Absolute count of users with / without the analytics:aggregate consent category. Labels: granted (\"true\"|\"false\").",
		}, []string{"granted"}),
		WorkerErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: metricAnalyticsWorkerErrors,
			Help: "Errors observed by the analytics opt-in worker. Labels: reason.",
		}, []string{"reason"}),
	}
}

// RegisterAnalyticsOptInMetrics registers the collectors on the given
// registry. Returns the first registration error encountered.
func RegisterAnalyticsOptInMetrics(reg prometheus.Registerer, m *AnalyticsOptInMetrics) error {
	collectors := []prometheus.Collector{m.OptInRatio, m.UsersTotal, m.WorkerErrors}
	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// AnalyticsOptInWorker periodically fetches workspace-wide consent stats
// from privacy-api to compute the fraction of users who have granted
// analytics:aggregate consent, updating AnalyticsOptInMetrics.
// When src is nil (no privacy-api configured) RunOnce is a no-op.
type AnalyticsOptInWorker struct {
	src      ConsentStatsSource
	metrics  *AnalyticsOptInMetrics
	interval time.Duration
	log      logr.Logger
}

// NewAnalyticsOptInWorker constructs a worker with the default interval.
// src may be nil — when nil, RunOnce is a no-op and no metrics are emitted
// (used when no privacy-api URL is configured).
func NewAnalyticsOptInWorker(src ConsentStatsSource, metrics *AnalyticsOptInMetrics, log logr.Logger) *AnalyticsOptInWorker {
	return &AnalyticsOptInWorker{
		src:      src,
		metrics:  metrics,
		interval: DefaultAnalyticsOptInInterval,
		log:      log.WithName("analytics-optin-worker"),
	}
}

// Run ticks until ctx is cancelled. Each tick calls RunOnce.
// Errors from RunOnce are logged but do not stop the worker.
func (w *AnalyticsOptInWorker) Run(ctx context.Context) {
	MarkWorkerRunning(WorkerNameAnalyticsOptIn)
	defer MarkWorkerStopped(WorkerNameAnalyticsOptIn)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Run once immediately so metrics are populated before the first tick.
	if err := w.RunOnce(ctx); err != nil {
		w.log.Error(err, "analytics opt-in worker initial run failed")
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil {
				w.log.Error(err, "analytics opt-in worker tick failed")
			}
		}
	}
}

// RunOnce fetches consent stats and updates metrics. When the source is nil
// (no privacy-api configured), RunOnce is a no-op.
func (w *AnalyticsOptInWorker) RunOnce(ctx context.Context) error {
	if w.src == nil {
		w.log.V(1).Info("analytics opt-in worker skipped", "reason", "no-privacy-url")
		return nil
	}

	stats, err := w.src.GetConsentStats(ctx)
	if err != nil {
		w.metrics.WorkerErrors.WithLabelValues("query").Inc()
		return err
	}

	granted := stats.GrantsByCategory[AnalyticsAggregateCategory]
	total := stats.TotalUsers

	// Leave the ratio gauge unchanged when no users exist — oscillating
	// to 0 on every empty-DB tick creates misleading dashboards on
	// fresh deployments. The absolute-count gauges still update to 0/0.
	w.metrics.UsersTotal.WithLabelValues("true").Set(float64(granted))
	w.metrics.UsersTotal.WithLabelValues("false").Set(float64(total - granted))
	if total > 0 {
		w.metrics.OptInRatio.Set(float64(granted) / float64(total))
	}
	return nil
}
