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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// CompactionMetrics holds Prometheus metrics for compaction CronJob runs.
type CompactionMetrics struct {
	// RunDurationSeconds tracks the total duration of a compaction run.
	RunDurationSeconds prometheus.Histogram
	// SessionsCompactedTotal counts sessions moved from warm to cold.
	SessionsCompactedTotal prometheus.Counter
	// BatchesProcessedTotal counts batches processed.
	BatchesProcessedTotal prometheus.Counter
	// ErrorsTotal counts errors by operation type.
	ErrorsTotal *prometheus.CounterVec
	// LastRunTimestamp records the timestamp of the last compaction run.
	LastRunTimestamp prometheus.Gauge
}

// NewCompactionMetrics creates and registers all Prometheus metrics for compaction.
func NewCompactionMetrics() *CompactionMetrics {
	return &CompactionMetrics{
		RunDurationSeconds: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "omnia_compaction_run_duration_seconds",
			Help:    "Duration of a compaction run in seconds",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12), // 1s to ~1h
		}),
		SessionsCompactedTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "omnia_compaction_sessions_compacted_total",
			Help: "Total number of sessions compacted from warm to cold storage",
		}),
		BatchesProcessedTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "omnia_compaction_batches_processed_total",
			Help: "Total number of batches processed during compaction",
		}),
		ErrorsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_compaction_errors_total",
			Help: "Total number of compaction errors by operation",
		}, []string{"operation"}),
		LastRunTimestamp: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "omnia_compaction_last_run_timestamp",
			Help: "Unix timestamp of the last compaction run",
		}),
	}
}

// RecordDuration observes a compaction run duration.
func (m *CompactionMetrics) RecordDuration(d time.Duration) {
	m.RunDurationSeconds.Observe(d.Seconds())
}

// RecordSessionsCompacted adds n to the sessions compacted counter.
func (m *CompactionMetrics) RecordSessionsCompacted(n int64) {
	m.SessionsCompactedTotal.Add(float64(n))
}

// RecordBatchProcessed increments the batch counter.
func (m *CompactionMetrics) RecordBatchProcessed() {
	m.BatchesProcessedTotal.Inc()
}

// RecordError increments the error counter for the given operation.
func (m *CompactionMetrics) RecordError(operation string) {
	m.ErrorsTotal.WithLabelValues(operation).Inc()
}

// RecordLastRun sets the last run timestamp to now.
func (m *CompactionMetrics) RecordLastRun() {
	m.LastRunTimestamp.SetToCurrentTime()
}

// NewCompactionMetricsWithRegistry creates compaction metrics with a custom
// registry. Use this instead of NewCompactionMetrics when you need an isolated
// registry (e.g. for testing or per-run CronJob binaries).
func NewCompactionMetricsWithRegistry(reg *prometheus.Registry) *CompactionMetrics {
	runDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "omnia_compaction_run_duration_seconds",
		Help:    "Duration of a compaction run in seconds",
		Buckets: prometheus.ExponentialBuckets(1, 2, 12),
	})
	sessionsCompacted := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "omnia_compaction_sessions_compacted_total",
		Help: "Total number of sessions compacted from warm to cold storage",
	})
	batchesProcessed := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "omnia_compaction_batches_processed_total",
		Help: "Total number of batches processed during compaction",
	})
	errorsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "omnia_compaction_errors_total",
		Help: "Total number of compaction errors by operation",
	}, []string{"operation"})
	lastRunTimestamp := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "omnia_compaction_last_run_timestamp",
		Help: "Unix timestamp of the last compaction run",
	})

	reg.MustRegister(runDuration, sessionsCompacted, batchesProcessed, errorsTotal, lastRunTimestamp)

	return &CompactionMetrics{
		RunDurationSeconds:     runDuration,
		SessionsCompactedTotal: sessionsCompacted,
		BatchesProcessedTotal:  batchesProcessed,
		ErrorsTotal:            errorsTotal,
		LastRunTimestamp:       lastRunTimestamp,
	}
}
