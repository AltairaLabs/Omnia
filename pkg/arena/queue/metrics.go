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

package queue

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metric status constants.
const (
	StatusSuccess = "success"
	StatusError   = "error"
)

// Operation name constants.
const (
	OpPush = "push"
	OpPop  = "pop"
	OpAck  = "ack"
	OpNack = "nack"
)

// QueueMetrics holds Prometheus metrics for arena queue operations.
// These metrics track queue items, operations, and job activity.
type QueueMetrics struct {
	// ItemsTotal tracks current items by status (pending/processing/completed/failed).
	ItemsTotal *prometheus.GaugeVec

	// OperationsTotal tracks total operations (push/pop/ack/nack).
	OperationsTotal *prometheus.CounterVec

	// OperationDuration tracks operation latency.
	OperationDuration *prometheus.HistogramVec

	// JobsActive tracks the number of active jobs.
	JobsActive prometheus.Gauge

	// ItemRetries tracks retry attempts by job.
	ItemRetries *prometheus.CounterVec
}

// QueueMetricsConfig configures the queue metrics.
type QueueMetricsConfig struct {
	// Namespace is the namespace for the metrics (optional).
	Namespace string

	// OperationDurationBuckets for operation duration histogram.
	// If nil, defaults to DefaultOperationDurationBuckets.
	OperationDurationBuckets []float64
}

// DefaultOperationDurationBuckets are the default histogram buckets for queue operation durations.
// Queue operations are typically fast (Redis/memory operations).
var DefaultOperationDurationBuckets = []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1}

// NewQueueMetrics creates and registers all Prometheus metrics for queue operations.
func NewQueueMetrics(cfg QueueMetricsConfig) *QueueMetrics {
	var constLabels prometheus.Labels
	if cfg.Namespace != "" {
		constLabels = prometheus.Labels{
			"namespace": cfg.Namespace,
		}
	}

	durationBuckets := cfg.OperationDurationBuckets
	if durationBuckets == nil {
		durationBuckets = DefaultOperationDurationBuckets
	}

	return &QueueMetrics{
		ItemsTotal: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "omnia_arena_queue_items",
			Help:        "Current number of items in the queue by status",
			ConstLabels: constLabels,
		}, []string{"job_id", "status"}),

		OperationsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_arena_queue_operations_total",
			Help:        "Total number of queue operations",
			ConstLabels: constLabels,
		}, []string{"operation", "status"}),

		OperationDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "omnia_arena_queue_operation_duration_seconds",
			Help:        "Queue operation duration in seconds",
			ConstLabels: constLabels,
			Buckets:     durationBuckets,
		}, []string{"operation"}),

		JobsActive: promauto.NewGauge(prometheus.GaugeOpts{
			Name:        "omnia_arena_queue_jobs_active",
			Help:        "Number of currently active jobs",
			ConstLabels: constLabels,
		}),

		ItemRetries: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_arena_queue_retries_total",
			Help:        "Total number of item retry attempts",
			ConstLabels: constLabels,
		}, []string{"job_id"}),
	}
}

// Initialize pre-registers queue metrics.
// This ensures metrics appear in /metrics output immediately at startup.
func (m *QueueMetrics) Initialize() {
	// Initialize jobs active gauge
	m.JobsActive.Set(0)

	// Initialize operation counters for known operations
	for _, op := range []string{OpPush, OpPop, OpAck, OpNack} {
		m.OperationsTotal.WithLabelValues(op, StatusSuccess).Add(0)
		m.OperationsTotal.WithLabelValues(op, StatusError).Add(0)
		m.OperationDuration.WithLabelValues(op)
	}
}

// RecordOperation records metrics for a queue operation.
func (m *QueueMetrics) RecordOperation(operation string, durationSeconds float64, success bool) {
	status := StatusSuccess
	if !success {
		status = StatusError
	}

	m.OperationsTotal.WithLabelValues(operation, status).Inc()
	m.OperationDuration.WithLabelValues(operation).Observe(durationSeconds)
}

// RecordItemStatusChange updates the item count gauges when an item changes status.
func (m *QueueMetrics) RecordItemStatusChange(jobID string, oldStatus, newStatus ItemStatus) {
	if oldStatus != "" {
		m.ItemsTotal.WithLabelValues(jobID, string(oldStatus)).Dec()
	}
	if newStatus != "" {
		m.ItemsTotal.WithLabelValues(jobID, string(newStatus)).Inc()
	}
}

// RecordItemsPushed records when items are pushed to the queue.
func (m *QueueMetrics) RecordItemsPushed(jobID string, count int) {
	m.ItemsTotal.WithLabelValues(jobID, string(ItemStatusPending)).Add(float64(count))
}

// RecordRetry records a retry attempt for an item.
func (m *QueueMetrics) RecordRetry(jobID string) {
	m.ItemRetries.WithLabelValues(jobID).Inc()
}

// IncrementActiveJobs increments the active jobs count.
func (m *QueueMetrics) IncrementActiveJobs() {
	m.JobsActive.Inc()
}

// DecrementActiveJobs decrements the active jobs count.
func (m *QueueMetrics) DecrementActiveJobs() {
	m.JobsActive.Dec()
}

// QueueMetricsRecorder is the interface for recording queue metrics.
// This allows for no-op implementations when metrics are disabled.
type QueueMetricsRecorder interface {
	RecordOperation(operation string, durationSeconds float64, success bool)
	RecordItemStatusChange(jobID string, oldStatus, newStatus ItemStatus)
	RecordItemsPushed(jobID string, count int)
	RecordRetry(jobID string)
	IncrementActiveJobs()
	DecrementActiveJobs()
}

// NoOpQueueMetrics is a no-op implementation for when metrics are disabled.
// All methods intentionally do nothing to allow code to call metrics methods
// without nil checks, while avoiding the overhead of actual metric recording.
type NoOpQueueMetrics struct{}

// RecordOperation is a no-op implementation for disabled metrics.
func (n *NoOpQueueMetrics) RecordOperation(_ string, _ float64, _ bool) {
	// Intentionally empty: metrics are disabled
}

// RecordItemStatusChange is a no-op implementation for disabled metrics.
func (n *NoOpQueueMetrics) RecordItemStatusChange(_ string, _, _ ItemStatus) {
	// Intentionally empty: metrics are disabled
}

// RecordItemsPushed is a no-op implementation for disabled metrics.
func (n *NoOpQueueMetrics) RecordItemsPushed(_ string, _ int) {
	// Intentionally empty: metrics are disabled
}

// RecordRetry is a no-op implementation for disabled metrics.
func (n *NoOpQueueMetrics) RecordRetry(_ string) {
	// Intentionally empty: metrics are disabled
}

// IncrementActiveJobs is a no-op implementation for disabled metrics.
func (n *NoOpQueueMetrics) IncrementActiveJobs() {
	// Intentionally empty: metrics are disabled
}

// DecrementActiveJobs is a no-op implementation for disabled metrics.
func (n *NoOpQueueMetrics) DecrementActiveJobs() {
	// Intentionally empty: metrics are disabled
}

// Ensure implementations satisfy interfaces.
var (
	_ QueueMetricsRecorder = (*QueueMetrics)(nil)
	_ QueueMetricsRecorder = (*NoOpQueueMetrics)(nil)
)
