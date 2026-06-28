/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

// fakeConsentStatsSource is a test double for ConsentStatsSource.
type fakeConsentStatsSource struct {
	stats *privacy.ConsentStats
	err   error
}

func (f *fakeConsentStatsSource) GetConsentStats(_ context.Context) (*privacy.ConsentStats, error) {
	return f.stats, f.err
}

func TestAnalyticsOptInWorker_ComputesRatio(t *testing.T) {
	src := &fakeConsentStatsSource{
		stats: &privacy.ConsentStats{
			TotalUsers: 4,
			GrantsByCategory: map[string]int64{
				"analytics:aggregate": 3,
			},
		},
	}

	metrics := NewAnalyticsOptInMetrics()
	reg := prometheus.NewRegistry()
	require.NoError(t, RegisterAnalyticsOptInMetrics(reg, metrics))

	worker := NewAnalyticsOptInWorker(src, metrics, logr.Discard())
	require.NoError(t, worker.RunOnce(context.Background()))

	if got := testutil.ToFloat64(metrics.OptInRatio); got != 0.75 {
		t.Errorf("ratio = %v, want 0.75", got)
	}
	if got := testutil.ToFloat64(metrics.UsersTotal.WithLabelValues("true")); got != 3 {
		t.Errorf("granted = %v, want 3", got)
	}
	if got := testutil.ToFloat64(metrics.UsersTotal.WithLabelValues("false")); got != 1 {
		t.Errorf("not granted = %v, want 1", got)
	}
}

func TestAnalyticsOptInWorker_ZeroUsers_NoDivideByZero(t *testing.T) {
	src := &fakeConsentStatsSource{
		stats: &privacy.ConsentStats{
			TotalUsers:       0,
			GrantsByCategory: map[string]int64{},
		},
	}

	metrics := NewAnalyticsOptInMetrics()
	reg := prometheus.NewRegistry()
	require.NoError(t, RegisterAnalyticsOptInMetrics(reg, metrics))

	// Pre-seed the gauge so we can confirm it stays put when total=0.
	metrics.OptInRatio.Set(0.7)

	worker := NewAnalyticsOptInWorker(src, metrics, logr.Discard())
	require.NoError(t, worker.RunOnce(context.Background()))

	// With zero users the ratio gauge MUST stay at 0.7 (not reset to 0).
	if got := testutil.ToFloat64(metrics.OptInRatio); got != 0.7 {
		t.Errorf("ratio changed on zero users: got %v, want 0.7 (unchanged)", got)
	}
	if got := testutil.ToFloat64(metrics.UsersTotal.WithLabelValues("true")); got != 0 {
		t.Errorf("granted = %v, want 0", got)
	}
	if got := testutil.ToFloat64(metrics.UsersTotal.WithLabelValues("false")); got != 0 {
		t.Errorf("not granted = %v, want 0", got)
	}
}

func TestAnalyticsOptInWorker_NilSource_NoOp(t *testing.T) {
	metrics := NewAnalyticsOptInMetrics()
	reg := prometheus.NewRegistry()
	require.NoError(t, RegisterAnalyticsOptInMetrics(reg, metrics))

	// Pre-seed the gauge to confirm it is not touched when source is nil.
	metrics.OptInRatio.Set(0.42)

	worker := NewAnalyticsOptInWorker(nil, metrics, logr.Discard())
	require.NoError(t, worker.RunOnce(context.Background()))

	// Nil source must not panic, must not error, must not touch any gauge.
	if got := testutil.ToFloat64(metrics.OptInRatio); got != 0.42 {
		t.Errorf("nil source changed ratio gauge: got %v, want 0.42 (unchanged)", got)
	}
	if got := testutil.ToFloat64(metrics.WorkerErrors.WithLabelValues("query")); got != 0 {
		t.Errorf("nil source incremented error counter: got %v, want 0", got)
	}
}

func TestAnalyticsOptInWorker_SourceError_IncrementsCounter(t *testing.T) {
	src := &fakeConsentStatsSource{
		err: errors.New("privacy-api unavailable"),
	}

	metrics := NewAnalyticsOptInMetrics()
	reg := prometheus.NewRegistry()
	require.NoError(t, RegisterAnalyticsOptInMetrics(reg, metrics))

	// Pre-seed the gauge so we can confirm it stays put on error.
	metrics.OptInRatio.Set(0.42)

	worker := NewAnalyticsOptInWorker(src, metrics, logr.Discard())
	if err := worker.RunOnce(context.Background()); err == nil {
		t.Fatal("RunOnce with erroring source: want error, got nil")
	}

	if got := testutil.ToFloat64(metrics.WorkerErrors.WithLabelValues("query")); got != 1 {
		t.Errorf("WorkerErrors{query} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.OptInRatio); got != 0.42 {
		t.Errorf("ratio changed on error: got %v, want 0.42 (unchanged)", got)
	}
}

func TestAnalyticsOptInWorker_Run_StopsOnCtxCancel(t *testing.T) {
	// Nil source: RunOnce is a no-op, so Run exercises the ticker/cancel
	// path without needing any I/O.
	metrics := NewAnalyticsOptInMetrics()
	reg := prometheus.NewRegistry()
	require.NoError(t, RegisterAnalyticsOptInMetrics(reg, metrics))

	worker := NewAnalyticsOptInWorker(nil, metrics, logr.Discard())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Run so the ticker select exits on the first case

	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Run returned promptly after ctx cancel.
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s of context cancellation")
	}
}
