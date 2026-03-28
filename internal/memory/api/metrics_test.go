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
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newHTTPMetricsWithRegistry creates HTTPMetrics against a custom registry for testing.
func newHTTPMetricsWithRegistry(reg prometheus.Registerer) *HTTPMetrics {
	requestDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    metricRequestDuration,
		Help:    "HTTP request duration in seconds",
		Buckets: DefaultHTTPDurationBuckets,
	}, []string{"method", "route", "status_code"})
	reg.MustRegister(requestDuration)

	requestsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricRequestsTotal,
		Help: "Total HTTP requests by method, route, and status code",
	}, []string{"method", "route", "status_code"})
	reg.MustRegister(requestsTotal)

	memoriesSaved := prometheus.NewCounter(prometheus.CounterOpts{
		Name: metricMemoriesSaved,
		Help: "Total memories saved",
	})
	reg.MustRegister(memoriesSaved)

	memoriesRetrieved := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricMemoriesRetrieved,
		Help: "Total memories retrieved by strategy",
	}, []string{"strategy"})
	reg.MustRegister(memoriesRetrieved)

	retrievalDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    metricRetrievalDuration,
		Help:    "Memory retrieval duration in seconds by strategy",
		Buckets: DefaultHTTPDurationBuckets,
	}, []string{"strategy"})
	reg.MustRegister(retrievalDuration)

	return &HTTPMetrics{
		RequestDuration:   requestDuration,
		RequestsTotal:     requestsTotal,
		MemoriesSaved:     memoriesSaved,
		MemoriesRetrieved: memoriesRetrieved,
		RetrievalDuration: retrievalDuration,
	}
}

func TestNewHTTPMetrics_RegistersAllMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newHTTPMetricsWithRegistry(reg)
	require.NotNil(t, m)
	assert.NotNil(t, m.RequestDuration)
	assert.NotNil(t, m.RequestsTotal)
	assert.NotNil(t, m.MemoriesSaved)
	assert.NotNil(t, m.MemoriesRetrieved)
	assert.NotNil(t, m.RetrievalDuration)
}

func TestMetricsMiddleware_RecordsRequestMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newHTTPMetricsWithRegistry(reg)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	handler := m.MetricsMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	families, err := reg.Gather()
	require.NoError(t, err)

	requestsFound := false
	durationFound := false
	for _, fam := range families {
		if fam.GetName() == metricRequestsTotal {
			requestsFound = true
			require.Equal(t, 1, len(fam.GetMetric()))
			assert.Equal(t, float64(1), fam.GetMetric()[0].GetCounter().GetValue())
		}
		if fam.GetName() == metricRequestDuration {
			durationFound = true
		}
	}
	assert.True(t, requestsFound, "requests_total not found")
	assert.True(t, durationFound, "request_duration_seconds not found")
}

func TestMetricsMiddleware_RecordsStatusCode(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newHTTPMetricsWithRegistry(reg)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	handler := m.MetricsMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/abc", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)

	families, err := reg.Gather()
	require.NoError(t, err)

	for _, fam := range families {
		if fam.GetName() == metricRequestsTotal {
			for _, metric := range fam.GetMetric() {
				for _, label := range metric.GetLabel() {
					if label.GetName() == "status_code" {
						assert.Equal(t, "404", label.GetValue())
					}
				}
			}
		}
	}
}

func TestMetricsMiddleware_MultipleRequests(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newHTTPMetricsWithRegistry(reg)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := m.MetricsMiddleware(inner)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/memories", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	families, err := reg.Gather()
	require.NoError(t, err)

	for _, fam := range families {
		if fam.GetName() == metricRequestsTotal {
			require.Equal(t, 1, len(fam.GetMetric()))
			assert.Equal(t, float64(3), fam.GetMetric()[0].GetCounter().GetValue())
		}
	}
}

func TestStatusCapture_DefaultCode(t *testing.T) {
	rr := httptest.NewRecorder()
	sc := &statusCapture{ResponseWriter: rr, code: http.StatusOK}
	_, _ = sc.Write([]byte("ok"))
	assert.Equal(t, http.StatusOK, sc.code)
}

func TestStatusCapture_ExplicitCode(t *testing.T) {
	rr := httptest.NewRecorder()
	sc := &statusCapture{ResponseWriter: rr, code: http.StatusOK}
	sc.WriteHeader(http.StatusCreated)
	assert.Equal(t, http.StatusCreated, sc.code)
}

func TestStatusCapture_Flush_Supported(t *testing.T) {
	rr := httptest.NewRecorder()
	sc := &statusCapture{ResponseWriter: rr, code: http.StatusOK}
	sc.Flush()
	assert.True(t, rr.Flushed)
}

func TestStatusCapture_Flush_NotSupported(t *testing.T) {
	w := &nonFlushWriter{}
	sc := &statusCapture{ResponseWriter: w, code: http.StatusOK}
	// Should not panic.
	sc.Flush()
}

func TestNormalizeRoute_UsesPattern(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories/abc", nil)
	req.Pattern = "GET /api/v1/memories/{id}"
	route := normalizeRoute(req)
	assert.Equal(t, "GET /api/v1/memories/{id}", route)
}

func TestNormalizeRoute_FallbackSanitizesUUIDs(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/memories/550e8400-e29b-41d4-a716-446655440000", nil)
	req.Pattern = ""
	route := normalizeRoute(req)
	assert.Equal(t, "/api/v1/memories/:id", route)
}

func TestNormalizeRoute_FallbackNoUUID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/memories", nil)
	req.Pattern = ""
	route := normalizeRoute(req)
	assert.Equal(t, "/api/v1/memories", route)
}

func TestMemoriesSavedCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newHTTPMetricsWithRegistry(reg)

	m.MemoriesSaved.Inc()
	m.MemoriesSaved.Inc()

	families, err := reg.Gather()
	require.NoError(t, err)

	for _, fam := range families {
		if fam.GetName() == metricMemoriesSaved {
			assert.Equal(t, float64(2), fam.GetMetric()[0].GetCounter().GetValue())
		}
	}
}

func TestMemoriesRetrievedCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newHTTPMetricsWithRegistry(reg)

	m.MemoriesRetrieved.WithLabelValues("keyword").Inc()
	m.MemoriesRetrieved.WithLabelValues("semantic").Inc()
	m.MemoriesRetrieved.WithLabelValues("keyword").Inc()

	families, err := reg.Gather()
	require.NoError(t, err)

	for _, fam := range families {
		if fam.GetName() == metricMemoriesRetrieved {
			for _, metric := range fam.GetMetric() {
				for _, label := range metric.GetLabel() {
					if label.GetValue() == "keyword" {
						assert.Equal(t, float64(2), metric.GetCounter().GetValue())
					}
					if label.GetValue() == "semantic" {
						assert.Equal(t, float64(1), metric.GetCounter().GetValue())
					}
				}
			}
		}
	}
}

func TestNewHTTPMetrics_Production(t *testing.T) {
	m := NewHTTPMetrics(prometheus.DefaultRegisterer)
	require.NotNil(t, m)
	assert.NotNil(t, m.RequestDuration)
	assert.NotNil(t, m.RequestsTotal)
	assert.NotNil(t, m.MemoriesSaved)
}

// nonFlushWriter is an http.ResponseWriter that does not implement http.Flusher.
type nonFlushWriter struct{}

func (n *nonFlushWriter) Header() http.Header         { return http.Header{} }
func (n *nonFlushWriter) Write(b []byte) (int, error) { return len(b), nil }
func (n *nonFlushWriter) WriteHeader(_ int)           {}
