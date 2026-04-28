/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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
	"regexp"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// uuidRegex matches UUID-like path segments (8-4-4-4-12 hex pattern).
var uuidRegex = regexp.MustCompile(
	`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`,
)

// Metric name constants.
const (
	metricRequestDuration   = "omnia_memory_api_request_duration_seconds"
	metricRequestsTotal     = "omnia_memory_api_requests_total"
	metricMemoriesSaved     = "omnia_memory_api_memories_saved_total"
	metricMemoriesRetrieved = "omnia_memory_api_memories_retrieved_total"
	metricRetrievalDuration = "omnia_memory_api_retrieval_duration_seconds"
	metricSafeGoDrops       = "omnia_memory_api_side_effect_drops_total"
)

// safeGoDropsTotal counts side effects (audit log, event publish,
// async embedding) dropped because the safeGo semaphore was at
// capacity. Per-label so operators can see which side effect
// degrades first under burst.
var safeGoDropsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: metricSafeGoDrops,
	Help: "Side effects dropped by safeGo because the in-flight semaphore was full",
}, []string{"label"})

// recordSafeGoDrop bumps the drop counter for a specific label.
func recordSafeGoDrop(label string) {
	safeGoDropsTotal.WithLabelValues(label).Inc()
}

// DefaultHTTPDurationBuckets are histogram buckets for HTTP request durations.
var DefaultHTTPDurationBuckets = []float64{
	0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
}

// HTTPMetrics holds Prometheus metrics for the memory-api HTTP layer.
type HTTPMetrics struct {
	RequestDuration   *prometheus.HistogramVec
	RequestsTotal     *prometheus.CounterVec
	MemoriesSaved     prometheus.Counter
	MemoriesRetrieved *prometheus.CounterVec
	RetrievalDuration *prometheus.HistogramVec
}

// NewHTTPMetrics creates and registers Prometheus metrics for memory-api
// using the default global registerer (promauto).
func NewHTTPMetrics(reg prometheus.Registerer) *HTTPMetrics {
	_ = reg // promauto uses the default registry; reg kept for API symmetry.
	return &HTTPMetrics{
		RequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    metricRequestDuration,
			Help:    "HTTP request duration in seconds",
			Buckets: DefaultHTTPDurationBuckets,
		}, []string{"method", "route", "status_code"}),

		RequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: metricRequestsTotal,
			Help: "Total HTTP requests by method, route, and status code",
		}, []string{"method", "route", "status_code"}),

		MemoriesSaved: promauto.NewCounter(prometheus.CounterOpts{
			Name: metricMemoriesSaved,
			Help: "Total memories saved",
		}),

		MemoriesRetrieved: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: metricMemoriesRetrieved,
			Help: "Total memories retrieved by strategy",
		}, []string{"strategy"}),

		RetrievalDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    metricRetrievalDuration,
			Help:    "Memory retrieval duration in seconds by strategy",
			Buckets: DefaultHTTPDurationBuckets,
		}, []string{"strategy"}),
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

// Flush implements http.Flusher by delegating to the underlying ResponseWriter
// when it supports flushing.
func (s *statusCapture) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// MetricsMiddleware wraps an http.Handler to record request metrics.
func (m *HTTPMetrics) MetricsMiddleware(next http.Handler) http.Handler {
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
func normalizeRoute(r *http.Request) string {
	if pat := r.Pattern; pat != "" {
		return pat
	}
	return uuidRegex.ReplaceAllString(r.URL.Path, ":id")
}
