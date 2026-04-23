/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegisterAccessMetrics_InstallsCollectors confirms the counters and
// histogram end up on the given registerer so Prometheus can scrape them.
func TestRegisterAccessMetrics_InstallsCollectors(t *testing.T) {
	reg := prometheus.NewRegistry()
	require.NoError(t, RegisterAccessMetrics(reg))

	mfs, err := reg.Gather()
	require.NoError(t, err)
	names := map[string]bool{}
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	assert.True(t, names[metricAccessUpdates], "successes counter must be registered")
	assert.True(t, names[metricAccessUpdateErrors], "errors counter must be registered")
	assert.True(t, names[metricAccessUpdateDuration], "duration histogram must be registered")
}

// TestRecordAccessUpdate_CountsSuccessesAndErrors proves the metrics
// record-helper increments the right counters based on the error arg.
func TestRecordAccessUpdate_CountsSuccessesAndErrors(t *testing.T) {
	reg := prometheus.NewRegistry()
	require.NoError(t, RegisterAccessMetrics(reg))

	m := defaultAccessMetrics.Load()
	require.NotNil(t, m)

	m.recordAccessUpdate(10*time.Millisecond, nil)
	m.recordAccessUpdate(20*time.Millisecond, nil)
	m.recordAccessUpdate(30*time.Millisecond, errors.New("pool closed"))

	assert.InDelta(t, 2.0, testutil.ToFloat64(m.updates), 0.0001,
		"two successful updates should count as 2")
	assert.InDelta(t, 1.0, testutil.ToFloat64(m.updateErrors), 0.0001,
		"one failed update should count as 1")
}

// TestRecordAccessUpdate_NilMetricsIsNoop confirms the touch path can be
// safely invoked before RegisterAccessMetrics has run (memory-api binary
// installs them at boot but tests sometimes skip that).
func TestRecordAccessUpdate_NilMetricsIsNoop(t *testing.T) {
	var m *accessMetrics
	assert.NotPanics(t, func() {
		m.recordAccessUpdate(time.Millisecond, nil)
	})
}

// TestRegisterAccessMetrics_IdempotentOnDuplicateRegister ensures the
// helper doesn't explode when called twice against the same registry —
// process-wide init plus per-test reset is a common pattern.
func TestRegisterAccessMetrics_IdempotentOnDuplicateRegister(t *testing.T) {
	reg := prometheus.NewRegistry()
	require.NoError(t, RegisterAccessMetrics(reg))
	// Second call on the same reg — register returns AlreadyRegisteredError
	// which we're expected to swallow; ensure no error bubbles out.
	assert.NoError(t, RegisterAccessMetrics(reg))
}
