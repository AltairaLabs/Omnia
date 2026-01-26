/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package queue

import (
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// testMetricsJobID is a constant job ID used across metrics tests.
const testMetricsJobID = "test-job"

// metricsOnce ensures NewQueueMetrics is only called once per test run
var metricsOnce = sync.OnceValue(func() *QueueMetrics {
	return NewQueueMetrics(QueueMetricsConfig{
		Namespace: "test-namespace",
	})
})

func TestNewQueueMetricsCreation(t *testing.T) {
	// Use sync.Once to ensure we only register metrics once
	metrics := metricsOnce()

	// Verify all metrics are created and non-nil
	if metrics == nil {
		t.Fatal("NewQueueMetrics returned nil")
	}
	if metrics.ItemsTotal == nil {
		t.Error("ItemsTotal is nil")
	}
	if metrics.OperationsTotal == nil {
		t.Error("OperationsTotal is nil")
	}
	if metrics.OperationDuration == nil {
		t.Error("OperationDuration is nil")
	}
	if metrics.JobsActive == nil {
		t.Error("JobsActive is nil")
	}
	if metrics.ItemRetries == nil {
		t.Error("ItemRetries is nil")
	}
}

func TestNewQueueMetricsWithCustomBuckets(t *testing.T) {
	// We can't call NewQueueMetrics again due to promauto global registration,
	// but we can verify the config handling by checking the default buckets
	if DefaultOperationDurationBuckets == nil {
		t.Error("DefaultOperationDurationBuckets is nil")
	}
	if len(DefaultOperationDurationBuckets) == 0 {
		t.Error("DefaultOperationDurationBuckets is empty")
	}
}

func TestQueueMetricsStruct(t *testing.T) {
	// Create metrics manually for testing (avoiding promauto global registration)
	metrics := &QueueMetrics{
		ItemsTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "test_struct_omnia_arena_queue_items",
			Help: "Current number of items in the queue by status",
		}, []string{"job_id", "status"}),

		OperationsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "test_struct_omnia_arena_queue_operations_total",
			Help: "Total number of queue operations",
		}, []string{"operation", "status"}),

		OperationDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "test_struct_omnia_arena_queue_operation_duration_seconds",
			Help:    "Queue operation duration in seconds",
			Buckets: DefaultOperationDurationBuckets,
		}, []string{"operation"}),

		JobsActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_struct_omnia_arena_queue_jobs_active",
			Help: "Number of currently active jobs",
		}),

		ItemRetries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "test_struct_omnia_arena_queue_retries_total",
			Help: "Total number of item retry attempts",
		}, []string{"job_id"}),
	}

	// Verify all fields are initialized
	if metrics.ItemsTotal == nil {
		t.Error("ItemsTotal is nil")
	}
	if metrics.OperationsTotal == nil {
		t.Error("OperationsTotal is nil")
	}
	if metrics.OperationDuration == nil {
		t.Error("OperationDuration is nil")
	}
	if metrics.JobsActive == nil {
		t.Error("JobsActive is nil")
	}
	if metrics.ItemRetries == nil {
		t.Error("ItemRetries is nil")
	}
}

func TestQueueMetricsConfigWithNamespace(t *testing.T) {
	cfg := QueueMetricsConfig{
		Namespace: "test-ns",
	}

	if cfg.Namespace != "test-ns" {
		t.Errorf("Namespace = %s, want test-ns", cfg.Namespace)
	}
}

func TestQueueMetricsConfigWithBuckets(t *testing.T) {
	customBuckets := []float64{0.01, 0.1, 1.0}
	cfg := QueueMetricsConfig{
		OperationDurationBuckets: customBuckets,
	}

	if len(cfg.OperationDurationBuckets) != 3 {
		t.Errorf("OperationDurationBuckets length = %d, want 3", len(cfg.OperationDurationBuckets))
	}
}

func TestQueueMetricsRecordOperation(t *testing.T) {
	metrics := createTestMetrics(t)

	// Record successful push
	metrics.RecordOperation(OpPush, 0.001, true)

	// Verify counter
	val := testutil.ToFloat64(metrics.OperationsTotal.WithLabelValues(OpPush, StatusSuccess))
	if val != 1 {
		t.Errorf("OperationsTotal for push/success = %f, want 1", val)
	}

	// Record failed pop
	metrics.RecordOperation(OpPop, 0.002, false)

	val = testutil.ToFloat64(metrics.OperationsTotal.WithLabelValues(OpPop, StatusError))
	if val != 1 {
		t.Errorf("OperationsTotal for pop/error = %f, want 1", val)
	}
}

func TestQueueMetricsRecordItemStatusChange(t *testing.T) {
	metrics := createTestMetrics(t)

	// Add an item as pending
	metrics.RecordItemStatusChange(testMetricsJobID, "", ItemStatusPending)

	val := testutil.ToFloat64(metrics.ItemsTotal.WithLabelValues(testMetricsJobID, string(ItemStatusPending)))
	if val != 1 {
		t.Errorf("ItemsTotal pending = %f, want 1", val)
	}

	// Move from pending to processing
	metrics.RecordItemStatusChange(testMetricsJobID, ItemStatusPending, ItemStatusProcessing)

	val = testutil.ToFloat64(metrics.ItemsTotal.WithLabelValues(testMetricsJobID, string(ItemStatusPending)))
	if val != 0 {
		t.Errorf("ItemsTotal pending after change = %f, want 0", val)
	}

	val = testutil.ToFloat64(metrics.ItemsTotal.WithLabelValues(testMetricsJobID, string(ItemStatusProcessing)))
	if val != 1 {
		t.Errorf("ItemsTotal processing = %f, want 1", val)
	}
}

func TestQueueMetricsRecordItemsPushed(t *testing.T) {
	metrics := createTestMetrics(t)

	metrics.RecordItemsPushed(testMetricsJobID, 5)

	val := testutil.ToFloat64(metrics.ItemsTotal.WithLabelValues(testMetricsJobID, string(ItemStatusPending)))
	if val != 5 {
		t.Errorf("ItemsTotal pending = %f, want 5", val)
	}

	// Push more items
	metrics.RecordItemsPushed(testMetricsJobID, 3)

	val = testutil.ToFloat64(metrics.ItemsTotal.WithLabelValues(testMetricsJobID, string(ItemStatusPending)))
	if val != 8 {
		t.Errorf("ItemsTotal pending = %f, want 8", val)
	}
}

func TestQueueMetricsRecordRetry(t *testing.T) {
	metrics := createTestMetrics(t)

	metrics.RecordRetry(testMetricsJobID)
	metrics.RecordRetry(testMetricsJobID)

	val := testutil.ToFloat64(metrics.ItemRetries.WithLabelValues(testMetricsJobID))
	if val != 2 {
		t.Errorf("ItemRetries = %f, want 2", val)
	}
}

func TestQueueMetricsActiveJobs(t *testing.T) {
	metrics := createTestMetrics(t)

	// Initial value
	val := testutil.ToFloat64(metrics.JobsActive)
	if val != 0 {
		t.Errorf("JobsActive initial = %f, want 0", val)
	}

	metrics.IncrementActiveJobs()
	metrics.IncrementActiveJobs()

	val = testutil.ToFloat64(metrics.JobsActive)
	if val != 2 {
		t.Errorf("JobsActive after increment = %f, want 2", val)
	}

	metrics.DecrementActiveJobs()

	val = testutil.ToFloat64(metrics.JobsActive)
	if val != 1 {
		t.Errorf("JobsActive after decrement = %f, want 1", val)
	}
}

func TestQueueMetricsInitialize(t *testing.T) {
	metrics := createTestMetrics(t)

	metrics.Initialize()

	// Verify JobsActive was initialized to 0
	val := testutil.ToFloat64(metrics.JobsActive)
	if val != 0 {
		t.Errorf("JobsActive after Initialize = %f, want 0", val)
	}

	// Verify operation counters were pre-registered
	for _, op := range []string{OpPush, OpPop, OpAck, OpNack} {
		// These should exist after initialization (value 0)
		successVal := testutil.ToFloat64(metrics.OperationsTotal.WithLabelValues(op, StatusSuccess))
		if successVal != 0 {
			t.Errorf("OperationsTotal[%s,success] = %f, want 0", op, successVal)
		}

		errorVal := testutil.ToFloat64(metrics.OperationsTotal.WithLabelValues(op, StatusError))
		if errorVal != 0 {
			t.Errorf("OperationsTotal[%s,error] = %f, want 0", op, errorVal)
		}
	}
}

func TestDefaultOperationDurationBuckets(t *testing.T) {
	if len(DefaultOperationDurationBuckets) == 0 {
		t.Error("DefaultOperationDurationBuckets is empty")
	}

	// Verify buckets are in ascending order
	for i := 1; i < len(DefaultOperationDurationBuckets); i++ {
		if DefaultOperationDurationBuckets[i] <= DefaultOperationDurationBuckets[i-1] {
			t.Errorf("Buckets not in ascending order at index %d", i)
		}
	}
}

func TestQueueMetricsConfigDefaults(t *testing.T) {
	cfg := QueueMetricsConfig{}

	if cfg.Namespace != "" {
		t.Errorf("Default Namespace = %s, want empty", cfg.Namespace)
	}

	if cfg.OperationDurationBuckets != nil {
		t.Error("Default OperationDurationBuckets should be nil")
	}
}

func TestStatusConstants(t *testing.T) {
	if StatusSuccess != "success" {
		t.Errorf("StatusSuccess = %s, want success", StatusSuccess)
	}
	if StatusError != "error" {
		t.Errorf("StatusError = %s, want error", StatusError)
	}
}

func TestOperationConstants(t *testing.T) {
	if OpPush != "push" {
		t.Errorf("OpPush = %s, want push", OpPush)
	}
	if OpPop != "pop" {
		t.Errorf("OpPop = %s, want pop", OpPop)
	}
	if OpAck != "ack" {
		t.Errorf("OpAck = %s, want ack", OpAck)
	}
	if OpNack != "nack" {
		t.Errorf("OpNack = %s, want nack", OpNack)
	}
}

func TestNoOpQueueMetricsAllMethods(t *testing.T) {
	m := &NoOpQueueMetrics{}

	// Verify all methods can be called without panicking
	m.RecordOperation(OpPush, 0.001, true)
	m.RecordOperation(OpPop, 0.002, false)
	m.RecordItemStatusChange(testMetricsJobID, ItemStatusPending, ItemStatusProcessing)
	m.RecordItemStatusChange(testMetricsJobID, "", ItemStatusPending)
	m.RecordItemsPushed(testMetricsJobID, 10)
	m.RecordRetry(testMetricsJobID)
	m.IncrementActiveJobs()
	m.DecrementActiveJobs()

	// Verify NoOpQueueMetrics implements QueueMetricsRecorder (compile-time check)
	var _ QueueMetricsRecorder = m
}

func TestQueueMetricsImplementsRecorder(t *testing.T) {
	metrics := createTestMetrics(t)

	// Verify QueueMetrics implements QueueMetricsRecorder (compile-time check)
	var _ QueueMetricsRecorder = metrics
}

// createTestMetrics creates QueueMetrics for testing without using promauto.
func createTestMetrics(t *testing.T) *QueueMetrics {
	t.Helper()

	return &QueueMetrics{
		ItemsTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "test_omnia_arena_queue_items",
			Help: "Current number of items in the queue by status",
		}, []string{"job_id", "status"}),

		OperationsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "test_omnia_arena_queue_operations_total",
			Help: "Total number of queue operations",
		}, []string{"operation", "status"}),

		OperationDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "test_omnia_arena_queue_operation_duration_seconds",
			Help:    "Queue operation duration in seconds",
			Buckets: DefaultOperationDurationBuckets,
		}, []string{"operation"}),

		JobsActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_omnia_arena_queue_jobs_active",
			Help: "Number of currently active jobs",
		}),

		ItemRetries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "test_omnia_arena_queue_retries_total",
			Help: "Total number of item retry attempts",
		}, []string{"job_id"}),
	}
}
