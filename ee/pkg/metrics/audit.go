/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// AuditMetrics holds Prometheus metrics for session audit logging.
type AuditMetrics struct {
	// EventsTotal counts audit events by event_type.
	EventsTotal *prometheus.CounterVec
	// WriteErrors counts write failures by event_type.
	WriteErrors *prometheus.CounterVec
	// WriteDuration tracks write latency by event_type.
	WriteDuration *prometheus.HistogramVec
	// BufferDrops counts events dropped due to full buffer by event_type.
	BufferDrops *prometheus.CounterVec
	// QueriesTotal counts audit log queries.
	QueriesTotal prometheus.Counter
	// QueryDuration tracks audit query latency.
	QueryDuration prometheus.Histogram
}

// NewAuditMetrics creates and registers all Prometheus metrics for audit logging.
func NewAuditMetrics() *AuditMetrics {
	return &AuditMetrics{
		EventsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_audit_events_total",
			Help: "Total number of audit events logged",
		}, []string{"event_type"}),

		WriteErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_audit_write_errors_total",
			Help: "Total number of audit write errors",
		}, []string{"event_type"}),

		WriteDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "omnia_audit_write_duration_seconds",
			Help:    "Duration of audit log writes",
			Buckets: prometheus.DefBuckets,
		}, []string{"event_type"}),

		BufferDrops: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_audit_buffer_drops_total",
			Help: "Total number of audit events dropped due to full buffer",
		}, []string{"event_type"}),

		QueriesTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "omnia_audit_queries_total",
			Help: "Total number of audit log queries",
		}),

		QueryDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "omnia_audit_query_duration_seconds",
			Help:    "Duration of audit log queries",
			Buckets: prometheus.DefBuckets,
		}),
	}
}

// NewAuditMetricsWithRegistry creates audit metrics with a custom registry for testing.
func NewAuditMetricsWithRegistry(reg *prometheus.Registry) *AuditMetrics {
	eventsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "omnia_audit_events_total",
		Help: "Total number of audit events logged",
	}, []string{"event_type"})

	writeErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "omnia_audit_write_errors_total",
		Help: "Total number of audit write errors",
	}, []string{"event_type"})

	writeDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "omnia_audit_write_duration_seconds",
		Help:    "Duration of audit log writes",
		Buckets: prometheus.DefBuckets,
	}, []string{"event_type"})

	bufferDrops := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "omnia_audit_buffer_drops_total",
		Help: "Total number of audit events dropped due to full buffer",
	}, []string{"event_type"})

	queriesTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "omnia_audit_queries_total",
		Help: "Total number of audit log queries",
	})

	queryDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "omnia_audit_query_duration_seconds",
		Help:    "Duration of audit log queries",
		Buckets: prometheus.DefBuckets,
	})

	reg.MustRegister(
		eventsTotal, writeErrors, writeDuration,
		bufferDrops, queriesTotal, queryDuration,
	)

	return &AuditMetrics{
		EventsTotal:   eventsTotal,
		WriteErrors:   writeErrors,
		WriteDuration: writeDuration,
		BufferDrops:   bufferDrops,
		QueriesTotal:  queriesTotal,
		QueryDuration: queryDuration,
	}
}
