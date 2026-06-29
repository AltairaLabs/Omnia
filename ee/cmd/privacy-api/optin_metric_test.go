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

// readGaugeVec extracts the current float64 value from a prometheus.GaugeVec
// for the given label values.
func readGaugeVec(t *testing.T, gv *prometheus.GaugeVec, labelValues ...string) float64 {
	t.Helper()
	g, err := gv.GetMetricWithLabelValues(labelValues...)
	require.NoError(t, err)
	m := &dto.Metric{}
	require.NoError(t, g.Write(m))
	return m.GetGauge().GetValue()
}

// readCounterVec extracts the current float64 value from a prometheus.CounterVec
// for the given label values.
func readCounterVec(t *testing.T, cv *prometheus.CounterVec, labelValues ...string) float64 {
	t.Helper()
	c, err := cv.GetMetricWithLabelValues(labelValues...)
	require.NoError(t, err)
	m := &dto.Metric{}
	require.NoError(t, c.Write(m))
	return m.GetCounter().GetValue()
}

// newTestWorker creates an OptInMetricWorker with a fresh registry.
func newTestWorker(t *testing.T, store privacy.ConsentStatsReader) *OptInMetricWorker {
	t.Helper()
	reg := prometheus.NewRegistry()
	return NewOptInMetricWorker(store, time.Hour, reg, zap.New(zap.UseDevMode(true)))
}

// TestOptInMetricWorker_KnownStats asserts that collect() sets the correct
// gauge values from a known consent-stats snapshot.
// Specifically: 4 granted out of 10 total → ratio = 0.4; granted vec = 4; not-granted = 6.
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

	assert.InDelta(t, 0.4, readGauge(t, w.optInRatio), 0.001, "optInRatio gauge (4/10=0.4)")
	assert.InDelta(t, 4.0, readGaugeVec(t, w.usersTotal, "true"), 0.001, "usersTotal{granted=true}")
	assert.InDelta(t, 6.0, readGaugeVec(t, w.usersTotal, "false"), 0.001, "usersTotal{granted=false}")
}

// TestOptInMetricWorker_ZeroUsers asserts that with zero users the opt-in
// ratio gauge stays at its zero-value without any divide-by-zero panic, and the
// user count gauges are set to zero.
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
	// Ratio gauge is not updated when users == 0 (guard divide-by-zero path).
	assert.InDelta(t, 0.0, readGauge(t, w.optInRatio), 0.001, "optInRatio must stay 0 when no users")
	assert.InDelta(t, 0.0, readGaugeVec(t, w.usersTotal, "true"), 0.001, "usersTotal{granted=true} == 0")
	assert.InDelta(t, 0.0, readGaugeVec(t, w.usersTotal, "false"), 0.001, "usersTotal{granted=false} == 0")
}

// TestOptInMetricWorker_NoAnalyticsGrants asserts correct gauge values when
// users exist but none have analytics:aggregate granted (ratio == 0.0).
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

	assert.InDelta(t, 0.0, readGauge(t, w.optInRatio), 0.001, "optInRatio == 0 when no analytics grants")
	assert.InDelta(t, 0.0, readGaugeVec(t, w.usersTotal, "true"), 0.001, "usersTotal{granted=true} == 0")
	assert.InDelta(t, 5.0, readGaugeVec(t, w.usersTotal, "false"), 0.001, "usersTotal{granted=false} == 5")
}

// TestOptInMetricWorker_StoreError asserts that a stats error is logged,
// the worker-errors counter is incremented, and gauges are left unchanged.
func TestOptInMetricWorker_StoreError(t *testing.T) {
	store := &stubStatsReader{err: assert.AnError}

	w := newTestWorker(t, store)

	assert.NotPanics(t, func() {
		w.collect(t.Context())
	})
	// Gauges remain at their initial zero value.
	assert.InDelta(t, 0.0, readGauge(t, w.optInRatio), 0.001, "optInRatio unchanged on error")
	// Worker-errors counter must be incremented with reason=query.
	assert.InDelta(t, 1.0, readCounterVec(t, w.workerErrors, "query"), 0.001,
		"workerErrors{reason=query} must be incremented on Stats() error")
}

// TestOptInMetricWorker_RunCollectsAtStartup asserts that Run populates the
// gauges before the first interval tick fires (immediate-collect behaviour).
// The test uses a very long interval (1 hour) so the ticker can never fire
// within the test window; if the gauge is populated the only explanation is
// the pre-loop immediate collect.
func TestOptInMetricWorker_RunCollectsAtStartup(t *testing.T) {
	store := &stubStatsReader{
		stats: privacy.ConsentStats{
			TotalUsers:       6,
			GrantsByCategory: map[string]int64{string(privacy.ConsentAnalyticsAggregate): 3},
		},
	}

	reg := prometheus.NewRegistry()
	// Use a very long interval — ticker must never fire during this test.
	w := NewOptInMetricWorker(store, time.Hour, reg, zap.New(zap.UseDevMode(true)))

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	// Give Run a brief window to execute the immediate collect, then cancel.
	// 200 ms is orders of magnitude less than the 1-hour interval.
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	// Gauge must be populated from the pre-loop collect, not from a tick.
	assert.InDelta(t, 0.5, readGauge(t, w.optInRatio), 0.001,
		"optInRatio must be populated at startup (3/6=0.5)")
	assert.InDelta(t, 3.0, readGaugeVec(t, w.usersTotal, "true"), 0.001,
		"usersTotal{granted=true} must be populated at startup")
	assert.InDelta(t, 3.0, readGaugeVec(t, w.usersTotal, "false"), 0.001,
		"usersTotal{granted=false} must be populated at startup")
}

// TestOptInMetricWorker_RunRespectsAlreadyCancelledContext asserts that Run
// returns immediately without calling collect when the context is already
// cancelled before Run is invoked.
func TestOptInMetricWorker_RunRespectsAlreadyCancelledContext(t *testing.T) {
	store := &stubStatsReader{
		stats: privacy.ConsentStats{
			TotalUsers:       4,
			GrantsByCategory: map[string]int64{string(privacy.ConsentAnalyticsAggregate): 2},
		},
	}

	w := newTestWorker(t, store)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel before Run is called

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Run returned immediately — correct.
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return promptly for a pre-cancelled context")
	}

	// Gauges must remain at zero — no collect should have occurred.
	assert.InDelta(t, 0.0, readGauge(t, w.optInRatio), 0.001,
		"optInRatio must remain 0 when context was already cancelled")
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
