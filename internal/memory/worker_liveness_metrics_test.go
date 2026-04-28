/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	dto "github.com/prometheus/client_model/go"
)

// readGaugeVec returns the value of the workerRunning gauge for the
// given name label. Pulls the metric directly from the prometheus
// registry rather than re-registering, so tests don't fight on
// duplicate-collector errors.
func readWorkerRunning(name string) float64 {
	g, err := workerRunning.GetMetricWithLabelValues(name)
	if err != nil {
		return -1 // signal a label that hasn't been emitted at all
	}
	var m dto.Metric
	if err := g.Write(&m); err != nil {
		return -1
	}
	return m.GetGauge().GetValue()
}

// TestMarkWorker_FlipsGauge is the unit-level contract test. The
// regression-prevention assertions live in the per-worker tests
// below — they assert the worker actually CALLS the helper inside
// its Run loop, which is the wiring failure mode (issue #1038).
func TestMarkWorker_FlipsGauge(t *testing.T) {
	const name = "test_marker"
	MarkWorkerRunning(name)
	if got := readWorkerRunning(name); got != 1 {
		t.Errorf("after MarkWorkerRunning: gauge=%v want=1", got)
	}
	MarkWorkerStopped(name)
	if got := readWorkerRunning(name); got != 0 {
		t.Errorf("after MarkWorkerStopped: gauge=%v want=0", got)
	}
}

// TestAnalyticsOptInWorker_FlipsLivenessGauge wires the regression
// test to the actual worker. Run must Set the gauge to 1 before
// any tick fires, and back to 0 when ctx cancels. Issue #1038's
// failure mode was workers that "started" but never called the
// liveness signal — this test fails loudly if anyone removes the
// MarkWorkerRunning call from the Run loop.
func TestAnalyticsOptInWorker_FlipsLivenessGauge(t *testing.T) {
	store := newStore(t)
	metrics := NewAnalyticsOptInMetrics()
	worker := NewAnalyticsOptInWorker(store.Pool(), metrics, logr.Discard())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	// Wait briefly for the worker to enter its loop. Polling rather
	// than sleeping so a fast machine doesn't race past Set(1).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if readWorkerRunning(WorkerNameAnalyticsOptIn) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := readWorkerRunning(WorkerNameAnalyticsOptIn); got != 1 {
		t.Errorf("worker running but gauge=%v, want=1 — Run() is missing MarkWorkerRunning(%q)",
			got, WorkerNameAnalyticsOptIn)
	}

	cancel()
	<-done

	if got := readWorkerRunning(WorkerNameAnalyticsOptIn); got != 0 {
		t.Errorf("worker stopped but gauge=%v, want=0 — Run() is missing deferred MarkWorkerStopped(%q)",
			got, WorkerNameAnalyticsOptIn)
	}
}
