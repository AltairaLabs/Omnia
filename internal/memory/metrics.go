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

package memory

import (
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metric name constants. Kept in one place so dashboards and tests agree.
const (
	metricAccessUpdates        = "omnia_memory_accessed_updates_total"
	metricAccessUpdateErrors   = "omnia_memory_accessed_update_errors_total"
	metricAccessUpdateDuration = "omnia_memory_accessed_update_duration_seconds"
)

// accessMetrics owns the Prometheus metrics emitted by the read-path
// accessed_at touch. Held behind an atomic pointer so the memory-api
// binary can install a shared set at startup while tests can keep
// per-test registries without panicking on duplicate registration.
type accessMetrics struct {
	updates        prometheus.Counter
	updateErrors   prometheus.Counter
	updateDuration prometheus.Histogram
}

// defaultAccessMetrics is the process-wide instance. Nil until installed
// via RegisterAccessMetrics. The touch path tolerates nil — metrics are
// optional, updates still happen.
var defaultAccessMetrics atomic.Pointer[accessMetrics]

// RegisterAccessMetrics registers the read-path accessed-at metrics on
// the given registerer (pass prometheus.DefaultRegisterer in production).
// Returns an error only when metric registration collides; callers can
// safely treat that as fatal at startup.
//
// Calling more than once replaces the active metrics pointer — useful in
// tests that want a fresh registry per case.
func RegisterAccessMetrics(reg prometheus.Registerer) error {
	m := &accessMetrics{
		updates: prometheus.NewCounter(prometheus.CounterOpts{
			Name: metricAccessUpdates,
			Help: "Number of successful accessed_at / access_count updates triggered by memory retrieval.",
		}),
		updateErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: metricAccessUpdateErrors,
			Help: "Number of accessed_at update attempts that returned an error (DB unavailable, timeout, etc.).",
		}),
		updateDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    metricAccessUpdateDuration,
			Help:    "Wall-clock duration of the async accessed_at UPDATE, in seconds.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}),
	}
	for _, c := range []prometheus.Collector{m.updates, m.updateErrors, m.updateDuration} {
		if err := reg.Register(c); err != nil {
			// Allow re-registration when the exact same collector was
			// already registered (tests re-use the default registry).
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}
	defaultAccessMetrics.Store(m)
	return nil
}

// recordAccessUpdate ticks the counter + histogram for one touch attempt.
// No-op when the metrics haven't been registered.
func (m *accessMetrics) recordAccessUpdate(dur time.Duration, err error) {
	if m == nil {
		return
	}
	m.updateDuration.Observe(dur.Seconds())
	if err != nil {
		m.updateErrors.Inc()
		return
	}
	m.updates.Inc()
}
