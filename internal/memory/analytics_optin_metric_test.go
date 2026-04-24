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
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsOptInWorker_ComputesRatio(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	_, err := store.Pool().Exec(ctx, `
		INSERT INTO user_privacy_preferences (user_id, consent_grants)
		VALUES
			($1, $2),
			($3, $4),
			($5, $6),
			($7, $8)`,
		"u1", []string{"analytics:aggregate"},
		"u2", []string{"analytics:aggregate", "memory:preferences"},
		"u3", []string{"memory:preferences"},
		"u4", []string{},
	)
	require.NoError(t, err)

	metrics := NewAnalyticsOptInMetrics()
	reg := prometheus.NewRegistry()
	require.NoError(t, RegisterAnalyticsOptInMetrics(reg, metrics))

	worker := NewAnalyticsOptInWorker(store.Pool(), metrics, logr.Discard())
	require.NoError(t, worker.RunOnce(ctx))

	if got := testutil.ToFloat64(metrics.OptInRatio); got != 0.5 {
		t.Errorf("ratio = %v, want 0.5", got)
	}
	if got := testutil.ToFloat64(metrics.UsersTotal.WithLabelValues("true")); got != 2 {
		t.Errorf("granted = %v, want 2", got)
	}
	if got := testutil.ToFloat64(metrics.UsersTotal.WithLabelValues("false")); got != 2 {
		t.Errorf("not granted = %v, want 2", got)
	}
}

func TestAnalyticsOptInWorker_QueryError_IncrementsCounter(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	// Close the pool so the query fails, exercising the error path.
	store.Pool().Close()

	metrics := NewAnalyticsOptInMetrics()
	reg := prometheus.NewRegistry()
	require.NoError(t, RegisterAnalyticsOptInMetrics(reg, metrics))

	// Pre-seed the gauge so we can confirm it stays put on error.
	metrics.OptInRatio.Set(0.42)

	worker := NewAnalyticsOptInWorker(store.Pool(), metrics, logr.Discard())
	if err := worker.RunOnce(ctx); err == nil {
		t.Fatal("RunOnce on closed pool: want error, got nil")
	}

	if got := testutil.ToFloat64(metrics.WorkerErrors.WithLabelValues("query")); got != 1 {
		t.Errorf("WorkerErrors{query} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.OptInRatio); got != 0.42 {
		t.Errorf("ratio changed on error: got %v, want 0.42 (unchanged)", got)
	}
}

func TestAnalyticsOptInWorker_Run_StopsOnCtxCancel(t *testing.T) {
	store := newStore(t)
	metrics := NewAnalyticsOptInMetrics()
	reg := prometheus.NewRegistry()
	require.NoError(t, RegisterAnalyticsOptInMetrics(reg, metrics))

	worker := NewAnalyticsOptInWorker(store.Pool(), metrics, logr.Discard())
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

func TestAnalyticsOptInWorker_EmptyTableLeavesGaugeUnchanged(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	metrics := NewAnalyticsOptInMetrics()
	reg := prometheus.NewRegistry()
	require.NoError(t, RegisterAnalyticsOptInMetrics(reg, metrics))

	// Pre-seed the gauge with a known value.
	metrics.OptInRatio.Set(0.7)

	worker := NewAnalyticsOptInWorker(store.Pool(), metrics, logr.Discard())
	require.NoError(t, worker.RunOnce(ctx))

	// Empty user_privacy_preferences → gauge MUST stay at 0.7 (not reset).
	if got := testutil.ToFloat64(metrics.OptInRatio); got != 0.7 {
		t.Errorf("ratio changed on empty table: got %v, want 0.7 (unchanged)", got)
	}
}
