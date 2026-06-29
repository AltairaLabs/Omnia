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

// OptInMetricWorker periodically reads consent stats from the privacy store
// and updates Prometheus gauges for the analytics opt-in ratio. It relocates
// the metrics previously owned by the memory-api AnalyticsOptInWorker (CE2
// deleted that worker; CE3 re-homes the metrics here where the source-of-truth
// consent data lives).
type OptInMetricWorker struct {
	store      privacy.ConsentStatsReader
	interval   time.Duration
	log        logr.Logger
	optInTotal prometheus.Gauge
	usersTotal prometheus.Gauge
}

// NewOptInMetricWorker creates an OptInMetricWorker and registers its gauges
// with reg. MustRegister panics on duplicate registration, consistent with
// one-shot binary startup wiring.
func NewOptInMetricWorker(
	store privacy.ConsentStatsReader,
	interval time.Duration,
	reg prometheus.Registerer,
	log logr.Logger,
) *OptInMetricWorker {
	optIn := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "omnia_memory_analytics_optin_total",
		Help: "Number of users with analytics:aggregate consent granted.",
	})
	users := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "omnia_memory_users_total",
		Help: "Total number of users with a privacy-preferences record.",
	})
	reg.MustRegister(optIn, users)

	return &OptInMetricWorker{
		store:      store,
		interval:   interval,
		log:        log.WithName("optin-metric"),
		optInTotal: optIn,
		usersTotal: users,
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
// logged but do not stop the worker.
func (w *OptInMetricWorker) collect(ctx context.Context) {
	stats, err := w.store.Stats(ctx)
	if err != nil {
		w.log.Error(err, "consent stats query failed")
		return
	}

	w.usersTotal.Set(float64(stats.TotalUsers))

	// Guard divide-by-zero: only set opt-in count when there are users.
	if stats.TotalUsers > 0 {
		n := stats.GrantsByCategory[string(privacy.ConsentAnalyticsAggregate)]
		w.optInTotal.Set(float64(n))
	}
}
