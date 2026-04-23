/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metric name constants.
const (
	metricRetentionSoftDeleted = "omnia_memory_retention_soft_deleted_total"
	metricRetentionHardDeleted = "omnia_memory_retention_hard_deleted_total"
	metricRetentionRunDuration = "omnia_memory_retention_run_duration_seconds"
	metricRetentionRunErrors   = "omnia_memory_retention_run_errors_total"
)

// retentionMetrics owns the Prometheus metrics emitted by the
// composite retention worker. Held behind an atomic pointer so the
// memory-api binary can install a shared set at startup while tests
// install per-test registries without panicking on duplicate registration.
type retentionMetrics struct {
	softDeleted *prometheus.CounterVec
	hardDeleted *prometheus.CounterVec
	runDuration *prometheus.HistogramVec
	runErrors   *prometheus.CounterVec
}

var defaultRetentionMetrics atomic.Pointer[retentionMetrics]

// RegisterRetentionMetrics registers the composite-worker metrics on
// the given registerer. Safe to call more than once — the last call
// wins. Returns non-nil only on unrecoverable registration errors.
func RegisterRetentionMetrics(reg prometheus.Registerer) error {
	m := &retentionMetrics{
		softDeleted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: metricRetentionSoftDeleted,
			Help: "Number of memory rows flipped to forgotten=true by the retention worker.",
		}, []string{"tier", "branch"}),
		hardDeleted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: metricRetentionHardDeleted,
			Help: "Number of memory rows hard-deleted after the soft-delete grace window expired.",
		}, []string{}),
		runDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    metricRetentionRunDuration,
			Help:    "Wall-clock duration of a single retention worker pass.",
			Buckets: []float64{0.01, 0.1, 0.5, 1, 5, 15, 60, 300},
		}, []string{"outcome"}),
		runErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: metricRetentionRunErrors,
			Help: "Number of retention worker passes that encountered at least one branch error.",
		}, []string{"tier", "branch"}),
	}
	for _, c := range []prometheus.Collector{m.softDeleted, m.hardDeleted, m.runDuration, m.runErrors} {
		if err := reg.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}
	defaultRetentionMetrics.Store(m)
	return nil
}

func (m *retentionMetrics) observeSoftDelete(tier Tier, branch RetentionBranch, n int64) {
	if m == nil || n <= 0 {
		return
	}
	m.softDeleted.WithLabelValues(string(tier), string(branch)).Add(float64(n))
}

func (m *retentionMetrics) observeHardDelete(n int64) {
	if m == nil || n <= 0 {
		return
	}
	m.hardDeleted.WithLabelValues().Add(float64(n))
}

func (m *retentionMetrics) observeRun(dur time.Duration, ok bool) {
	if m == nil {
		return
	}
	outcome := "ok"
	if !ok {
		outcome = "error"
	}
	m.runDuration.WithLabelValues(outcome).Observe(dur.Seconds())
}

func (m *retentionMetrics) observeBranchError(tier Tier, branch RetentionBranch) {
	if m == nil {
		return
	}
	m.runErrors.WithLabelValues(string(tier), string(branch)).Inc()
}
