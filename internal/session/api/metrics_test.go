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
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPMetrics_DefaultBuckets(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newHTTPMetricsWithRegistry(reg, nil)
	require.NotNil(t, m)
	assert.NotNil(t, m.RequestDuration)
	assert.NotNil(t, m.RequestsTotal)
	assert.NotNil(t, m.EventsPublished)
	assert.NotNil(t, m.EventPublishDuration)
}

func TestNewHTTPMetrics_CustomBuckets(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newHTTPMetricsWithRegistry(reg, &HTTPMetricsConfig{
		DurationBuckets: []float64{0.1, 1.0, 10.0},
	})
	require.NotNil(t, m)
}

func TestHTTPMetrics_Initialize(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newHTTPMetricsWithRegistry(reg, nil)
	m.Initialize()

	families, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, fam := range families {
		if fam.GetName() == metricEventsPublished {
			found = true
			assert.Equal(t, 2, len(fam.GetMetric()))
		}
	}
	assert.True(t, found)
}

func TestMetricsMiddleware_RecordsRequestMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newHTTPMetricsWithRegistry(reg, nil)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	handler := MetricsMiddleware(m, inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
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
	m := newHTTPMetricsWithRegistry(reg, nil)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	handler := MetricsMiddleware(m, inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/abc", nil)
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

func TestHTTPMetrics_EventsPublished(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newHTTPMetricsWithRegistry(reg, nil)

	m.EventsPublished.WithLabelValues("success").Inc()
	m.EventsPublished.WithLabelValues("success").Inc()
	m.EventsPublished.WithLabelValues("error").Inc()

	families, err := reg.Gather()
	require.NoError(t, err)

	for _, fam := range families {
		if fam.GetName() == metricEventsPublished {
			for _, metric := range fam.GetMetric() {
				for _, label := range metric.GetLabel() {
					if label.GetValue() == "success" {
						assert.Equal(t, float64(2), metric.GetCounter().GetValue())
					}
					if label.GetValue() == "error" {
						assert.Equal(t, float64(1), metric.GetCounter().GetValue())
					}
				}
			}
		}
	}
}

func TestMetricsMiddleware_MultipleRequests(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newHTTPMetricsWithRegistry(reg, nil)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := MetricsMiddleware(m, inner)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", nil)
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

func TestNewHTTPMetrics_Production(t *testing.T) {
	// Test the production constructor (uses default registry).
	// Skip if metrics already registered from other tests in same process.
	m := NewHTTPMetrics(nil)
	require.NotNil(t, m)
	assert.NotNil(t, m.RequestDuration)
	assert.NotNil(t, m.RequestsTotal)
	assert.NotNil(t, m.EventsPublished)
	assert.NotNil(t, m.EventPublishDuration)
}

func TestNormalizeRoute_FallbackToPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/abc", nil)
	// Clear the pattern to test the fallback path.
	req.Pattern = ""
	route := normalizeRoute(req)
	assert.Equal(t, "/api/v1/sessions/abc", route)
}

func TestNormalizeRoute_UsesPattern(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/abc", nil)
	req.Pattern = "GET /api/v1/sessions/{sessionID}"
	route := normalizeRoute(req)
	assert.Equal(t, "GET /api/v1/sessions/{sessionID}", route)
}

// newHTTPMetricsWithRegistry creates HTTPMetrics against a custom registry for testing.
func newHTTPMetricsWithRegistry(reg prometheus.Registerer, cfg *HTTPMetricsConfig) *HTTPMetrics {
	var buckets []float64
	if cfg != nil && cfg.DurationBuckets != nil {
		buckets = cfg.DurationBuckets
	} else {
		buckets = DefaultHTTPDurationBuckets
	}

	requestDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    metricRequestDuration,
		Help:    "HTTP request duration in seconds",
		Buckets: buckets,
	}, []string{"method", "route", "status_code"})
	reg.MustRegister(requestDuration)

	requestsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricRequestsTotal,
		Help: "Total HTTP requests by method, route, and status code",
	}, []string{"method", "route", "status_code"})
	reg.MustRegister(requestsTotal)

	eventsPublished := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricEventsPublished,
		Help: "Redis stream event publish attempts by status",
	}, []string{"status"})
	reg.MustRegister(eventsPublished)

	eventPublishDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    metricEventPublishDuration,
		Help:    "Time to publish an event to Redis Streams",
		Buckets: []float64{0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	})
	reg.MustRegister(eventPublishDuration)

	return &HTTPMetrics{
		RequestDuration:      requestDuration,
		RequestsTotal:        requestsTotal,
		EventsPublished:      eventsPublished,
		EventPublishDuration: eventPublishDuration,
	}
}
