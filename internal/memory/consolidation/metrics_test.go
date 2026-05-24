/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package consolidation

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetrics_PassesTotalIncrements(t *testing.T) {
	m := NewMetrics()
	m.PassesTotal.WithLabelValues(testWorkspaceID, "safe-default", "ok").Inc()
	got := testutil.ToFloat64(m.PassesTotal.WithLabelValues(testWorkspaceID, "safe-default", "ok"))
	if got != 1 {
		t.Errorf("PassesTotal = %v, want 1", got)
	}
}

func TestMetrics_ActionsTotalLabels(t *testing.T) {
	m := NewMetrics()
	m.ActionsTotal.WithLabelValues(testWorkspaceID, "safe-default", "rescope", "applied", "agent-scoped").Inc()
	got := testutil.ToFloat64(m.ActionsTotal.WithLabelValues(testWorkspaceID, "safe-default", "rescope", "applied", "agent-scoped"))
	if got != 1 {
		t.Errorf("ActionsTotal = %v, want 1", got)
	}
}

func TestMetrics_PassDurationObserves(t *testing.T) {
	m := NewMetrics()
	m.PassDurationSeconds.WithLabelValues(testWorkspaceID, "safe-default").Observe(0.5)
	// Just verify registration works — histogram introspection requires Collect.
	reg := prometheus.NewRegistry()
	m.MustRegister(reg)
}

func TestMetrics_FunctionCallDurationObserves(t *testing.T) {
	m := NewMetrics()
	m.FunctionCallDurationSeconds.WithLabelValues(testWorkspaceID, "safe-default").Observe(1.2)
	reg := prometheus.NewRegistry()
	m.MustRegister(reg)
}
