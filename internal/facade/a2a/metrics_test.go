/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package a2a

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestMetrics creates metrics with a fresh registry to avoid global state conflicts.
func newTestMetrics(t *testing.T) *Metrics {
	t.Helper()
	reg := prometheus.NewRegistry()

	m := &Metrics{
		TasksTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "test_a2a_tasks_total",
			Help: "Total number of A2A tasks created.",
		}, []string{"method", "status"}),

		TaskDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "test_a2a_task_duration_seconds",
			Help:    "Duration of A2A task execution in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method"}),

		ActiveTasks: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "test_a2a_active_tasks",
			Help: "Number of currently active A2A tasks.",
		}),

		DiscoveryRequests: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_a2a_discovery_requests_total",
			Help: "Total number of agent card discovery requests.",
		}),

		RPCRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "test_a2a_rpc_requests_total",
			Help: "Total number of A2A JSON-RPC requests.",
		}, []string{"method", "status"}),

		RPCDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "test_a2a_rpc_duration_seconds",
			Help:    "Duration of A2A JSON-RPC requests in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method"}),
	}

	reg.MustRegister(m.TasksTotal, m.TaskDuration, m.ActiveTasks,
		m.DiscoveryRequests, m.RPCRequests, m.RPCDuration)

	return m
}

func TestMetricsMiddleware_Discovery(t *testing.T) {
	metrics := newTestMetrics(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := NewMetricsMiddleware(inner, metrics)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Check discovery counter incremented.
	var m io_prometheus_client.Metric
	require.NoError(t, metrics.DiscoveryRequests.(prometheus.Metric).Write(&m))
	assert.Equal(t, float64(1), *m.Counter.Value)
}

func TestMetricsMiddleware_RPCRequest(t *testing.T) {
	metrics := newTestMetrics(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := NewMetricsMiddleware(inner, metrics)

	req := httptest.NewRequest(http.MethodPost, "/a2a", nil)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Check RPC counter incremented for method "rpc" with status "200".
	counter, err := metrics.RPCRequests.GetMetricWithLabelValues("rpc", "200")
	require.NoError(t, err)
	var m io_prometheus_client.Metric
	require.NoError(t, counter.Write(&m))
	assert.Equal(t, float64(1), *m.Counter.Value)
}

func TestMetricsMiddleware_ErrorStatus(t *testing.T) {
	metrics := newTestMetrics(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	mw := NewMetricsMiddleware(inner, metrics)

	req := httptest.NewRequest(http.MethodPost, "/a2a", nil)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	counter, err := metrics.RPCRequests.GetMetricWithLabelValues("rpc", "400")
	require.NoError(t, err)
	var m io_prometheus_client.Metric
	require.NoError(t, counter.Write(&m))
	assert.Equal(t, float64(1), *m.Counter.Value)
}

func TestMetricsMiddleware_HealthEndpoint(t *testing.T) {
	metrics := newTestMetrics(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := NewMetricsMiddleware(inner, metrics)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)

	counter, err := metrics.RPCRequests.GetMetricWithLabelValues("healthz", "200")
	require.NoError(t, err)
	var m io_prometheus_client.Metric
	require.NoError(t, counter.Write(&m))
	assert.Equal(t, float64(1), *m.Counter.Value)
}

func TestClassifyMethod(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/.well-known/agent.json", "discovery"},
		{"/a2a", "rpc"},
		{"/healthz", "healthz"},
		{"/readyz", "readyz"},
		{"/other", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			assert.Equal(t, tt.expected, classifyMethod(req))
		})
	}
}

func TestStatusWriter_CapturesStatus(t *testing.T) {
	w := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

	sw.WriteHeader(http.StatusNotFound)
	assert.Equal(t, http.StatusNotFound, sw.status)
	assert.True(t, sw.wroteHeader)
}

func TestStatusWriter_DefaultStatus(t *testing.T) {
	w := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

	// Without calling WriteHeader, default is 200.
	assert.Equal(t, http.StatusOK, sw.status)
	assert.False(t, sw.wroteHeader)
}

func TestStatusWriter_OnlyFirstWrite(t *testing.T) {
	w := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

	sw.WriteHeader(http.StatusNotFound)
	sw.WriteHeader(http.StatusInternalServerError) // second call should be ignored
	assert.Equal(t, http.StatusNotFound, sw.status)
}

func TestNewMetrics(t *testing.T) {
	// NewMetrics uses promauto, so it registers to the default registry.
	// Just verify it doesn't panic and returns non-nil fields.
	m := NewMetrics("test-agent", "test-ns")
	assert.NotNil(t, m.TasksTotal)
	assert.NotNil(t, m.TaskDuration)
	assert.NotNil(t, m.ActiveTasks)
	assert.NotNil(t, m.DiscoveryRequests)
	assert.NotNil(t, m.RPCRequests)
	assert.NotNil(t, m.RPCDuration)
}
