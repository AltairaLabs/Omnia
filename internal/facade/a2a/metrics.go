/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package a2a

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds Prometheus metrics for the A2A facade.
type Metrics struct {
	// TasksTotal counts total tasks created, labeled by agent, method, and status.
	TasksTotal *prometheus.CounterVec

	// TaskDuration tracks task execution duration in seconds.
	TaskDuration *prometheus.HistogramVec

	// ActiveTasks tracks the number of currently active (non-terminal) tasks.
	ActiveTasks prometheus.Gauge

	// DiscoveryRequests counts agent card discovery requests.
	DiscoveryRequests prometheus.Counter

	// RPCRequests counts all JSON-RPC requests by method and HTTP status.
	RPCRequests *prometheus.CounterVec

	// RPCDuration tracks JSON-RPC request duration by method.
	RPCDuration *prometheus.HistogramVec
}

// NewMetrics creates and registers A2A Prometheus metrics.
func NewMetrics(agentName, namespace string) *Metrics {
	labels := prometheus.Labels{
		"agent":     agentName,
		"namespace": namespace,
	}

	return &Metrics{
		TasksTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_a2a_tasks_total",
			Help:        "Total number of A2A tasks created.",
			ConstLabels: labels,
		}, []string{"method", "status"}),

		TaskDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "omnia_a2a_task_duration_seconds",
			Help:        "Duration of A2A task execution in seconds.",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}, []string{"method"}),

		ActiveTasks: promauto.NewGauge(prometheus.GaugeOpts{
			Name:        "omnia_a2a_active_tasks",
			Help:        "Number of currently active A2A tasks.",
			ConstLabels: labels,
		}),

		DiscoveryRequests: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "omnia_a2a_discovery_requests_total",
			Help:        "Total number of agent card discovery requests.",
			ConstLabels: labels,
		}),

		RPCRequests: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_a2a_rpc_requests_total",
			Help:        "Total number of A2A JSON-RPC requests.",
			ConstLabels: labels,
		}, []string{"method", "status"}),

		RPCDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "omnia_a2a_rpc_duration_seconds",
			Help:        "Duration of A2A JSON-RPC requests in seconds.",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}, []string{"method"}),
	}
}

// MetricsMiddleware wraps an HTTP handler with Prometheus metrics collection.
// It records request count, duration, and status code for each A2A endpoint.
type MetricsMiddleware struct {
	next    http.Handler
	metrics *Metrics
}

// NewMetricsMiddleware creates middleware that instruments an A2A handler with metrics.
func NewMetricsMiddleware(handler http.Handler, metrics *Metrics) *MetricsMiddleware {
	return &MetricsMiddleware{
		next:    handler,
		metrics: metrics,
	}
}

// ServeHTTP instruments the request and delegates to the next handler.
func (m *MetricsMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Track discovery requests separately.
	if r.URL.Path == "/.well-known/agent.json" {
		m.metrics.DiscoveryRequests.Inc()
	}

	// Wrap response writer to capture status code.
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	m.next.ServeHTTP(sw, r)

	duration := time.Since(start).Seconds()
	method := classifyMethod(r)
	status := strconv.Itoa(sw.status)

	m.metrics.RPCRequests.WithLabelValues(method, status).Inc()
	m.metrics.RPCDuration.WithLabelValues(method).Observe(duration)
}

// classifyMethod determines the A2A method from the request path.
func classifyMethod(r *http.Request) string {
	switch r.URL.Path {
	case "/.well-known/agent.json":
		return "discovery"
	case "/a2a":
		return "rpc"
	case "/healthz":
		return "healthz"
	case "/readyz":
		return "readyz"
	default:
		return "unknown"
	}
}

// statusWriter captures the HTTP response status code.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// WriteHeader captures the status code.
func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}
