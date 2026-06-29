/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

// stubStatsReader implements privacy.ConsentStatsReader for unit tests.
type stubStatsReader struct {
	stats privacy.ConsentStats
	err   error
}

func (s *stubStatsReader) Stats(_ context.Context) (privacy.ConsentStats, error) {
	return s.stats, s.err
}

// readGauge extracts the current float64 value from a prometheus.Gauge.
func readGauge(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	m := &dto.Metric{}
	require.NoError(t, g.Write(m))
	return m.GetGauge().GetValue()
}

// newTestWorker creates an OptInMetricWorker with a fresh registry.
func newTestWorker(t *testing.T, store privacy.ConsentStatsReader) *OptInMetricWorker {
	t.Helper()
	reg := prometheus.NewRegistry()
	return NewOptInMetricWorker(store, time.Hour, reg, zap.New(zap.UseDevMode(true)))
}

// TestOptInMetricWorker_KnownStats asserts that collect() sets the correct
// gauge values from a known consent-stats snapshot.
func TestOptInMetricWorker_KnownStats(t *testing.T) {
	store := &stubStatsReader{
		stats: privacy.ConsentStats{
			TotalUsers:  10,
			OptedOutAll: 2,
			GrantsByCategory: map[string]int64{
				string(privacy.ConsentAnalyticsAggregate): 4,
				string(privacy.ConsentMemoryIdentity):     7,
			},
		},
	}

	w := newTestWorker(t, store)
	w.collect(t.Context())

	assert.InDelta(t, 10.0, readGauge(t, w.usersTotal), 0.001, "usersTotal gauge")
	assert.InDelta(t, 4.0, readGauge(t, w.optInTotal), 0.001, "optInTotal gauge")
}

// TestOptInMetricWorker_ZeroUsers asserts that with zero users the opt-in
// gauge stays at its zero-value without any divide-by-zero panic.
func TestOptInMetricWorker_ZeroUsers(t *testing.T) {
	store := &stubStatsReader{
		stats: privacy.ConsentStats{
			TotalUsers:       0,
			GrantsByCategory: map[string]int64{},
		},
	}

	w := newTestWorker(t, store)

	// Collect must not panic when TotalUsers == 0.
	assert.NotPanics(t, func() {
		w.collect(t.Context())
	})
	assert.InDelta(t, 0.0, readGauge(t, w.usersTotal), 0.001)
	// opt-in gauge is not updated when users == 0 (guard divide-by-zero path).
	assert.InDelta(t, 0.0, readGauge(t, w.optInTotal), 0.001)
}

// TestOptInMetricWorker_NoAnalyticsGrants asserts correct gauge values when
// users exist but none have analytics:aggregate granted.
func TestOptInMetricWorker_NoAnalyticsGrants(t *testing.T) {
	store := &stubStatsReader{
		stats: privacy.ConsentStats{
			TotalUsers:  5,
			OptedOutAll: 0,
			GrantsByCategory: map[string]int64{
				string(privacy.ConsentMemoryIdentity): 3,
			},
		},
	}

	w := newTestWorker(t, store)
	w.collect(t.Context())

	assert.InDelta(t, 5.0, readGauge(t, w.usersTotal), 0.001)
	assert.InDelta(t, 0.0, readGauge(t, w.optInTotal), 0.001)
}

// TestOptInMetricWorker_StoreError asserts that a stats error is logged and
// gauges are left unchanged (worker does not panic or stop).
func TestOptInMetricWorker_StoreError(t *testing.T) {
	store := &stubStatsReader{err: assert.AnError}

	w := newTestWorker(t, store)

	assert.NotPanics(t, func() {
		w.collect(t.Context())
	})
	// Gauges remain at their initial zero value.
	assert.InDelta(t, 0.0, readGauge(t, w.usersTotal), 0.001)
}

// TestOptInMetricWorker_RunStopsOnContextCancel asserts that Run returns when
// the context is cancelled without blocking indefinitely.
func TestOptInMetricWorker_RunStopsOnContextCancel(t *testing.T) {
	store := &stubStatsReader{
		stats: privacy.ConsentStats{
			TotalUsers:       3,
			GrantsByCategory: map[string]int64{string(privacy.ConsentAnalyticsAggregate): 1},
		},
	}

	reg := prometheus.NewRegistry()
	// Use a very short interval so at least one tick fires before cancellation.
	w := NewOptInMetricWorker(store, time.Millisecond, reg, zap.New(zap.UseDevMode(true)))

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Run returned after context cancellation — correct.
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation within 2s")
	}
}
