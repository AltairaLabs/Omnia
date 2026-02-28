/*
Copyright 2026.

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

package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metric name constants.
const (
	metricRequestDuration      = "omnia_session_api_request_duration_seconds"
	metricRequestsTotal        = "omnia_session_api_requests_total"
	metricEventsPublished      = "omnia_session_api_events_published_total"
	metricEventPublishDuration = "omnia_session_api_event_publish_duration_seconds"
)

// DefaultHTTPDurationBuckets are histogram buckets for HTTP request durations.
var DefaultHTTPDurationBuckets = []float64{
	0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
}

// HTTPMetrics holds Prometheus metrics for the session-api HTTP layer.
type HTTPMetrics struct {
	// RequestDuration tracks HTTP request duration in seconds by method, route, and status code.
	RequestDuration *prometheus.HistogramVec

	// RequestsTotal counts HTTP requests by method, route, and status code.
	RequestsTotal *prometheus.CounterVec

	// EventsPublished counts Redis stream event publish attempts.
	EventsPublished *prometheus.CounterVec

	// EventPublishDuration tracks time to publish an event to Redis Streams.
	EventPublishDuration prometheus.Histogram
}

// HTTPMetricsConfig configures the session-api HTTP metrics.
type HTTPMetricsConfig struct {
	DurationBuckets []float64
}

// NewHTTPMetrics creates and registers Prometheus metrics for session-api.
func NewHTTPMetrics(cfg *HTTPMetricsConfig) *HTTPMetrics {
	var buckets []float64
	if cfg != nil && cfg.DurationBuckets != nil {
		buckets = cfg.DurationBuckets
	} else {
		buckets = DefaultHTTPDurationBuckets
	}

	return &HTTPMetrics{
		RequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    metricRequestDuration,
			Help:    "HTTP request duration in seconds",
			Buckets: buckets,
		}, []string{"method", "route", "status_code"}),

		RequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: metricRequestsTotal,
			Help: "Total HTTP requests by method, route, and status code",
		}, []string{"method", "route", "status_code"}),

		EventsPublished: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: metricEventsPublished,
			Help: "Redis stream event publish attempts by status",
		}, []string{"status"}),

		EventPublishDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    metricEventPublishDuration,
			Help:    "Time to publish an event to Redis Streams",
			Buckets: []float64{0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		}),
	}
}

// Initialize pre-registers metrics so they appear in /metrics at startup.
func (m *HTTPMetrics) Initialize() {
	for _, status := range []string{"success", "error"} {
		m.EventsPublished.WithLabelValues(status).Add(0)
	}
}

// statusCapture wraps http.ResponseWriter to capture the status code.
type statusCapture struct {
	http.ResponseWriter
	code int
}

func (s *statusCapture) WriteHeader(code int) {
	s.code = code
	s.ResponseWriter.WriteHeader(code)
}

// MetricsMiddleware returns HTTP middleware that records request metrics.
func MetricsMiddleware(m *HTTPMetrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sc := &statusCapture{ResponseWriter: w, code: http.StatusOK}

		next.ServeHTTP(sc, r)

		duration := time.Since(start).Seconds()
		route := normalizeRoute(r)
		status := strconv.Itoa(sc.code)

		m.RequestDuration.WithLabelValues(r.Method, route, status).Observe(duration)
		m.RequestsTotal.WithLabelValues(r.Method, route, status).Inc()
	})
}

// normalizeRoute extracts a low-cardinality route label from the request.
// It replaces dynamic path segments (session IDs) with placeholders.
func normalizeRoute(r *http.Request) string {
	// Use the registered pattern from Go 1.22+ ServeMux if available.
	if pat := r.Pattern; pat != "" {
		return pat
	}
	return r.URL.Path
}
