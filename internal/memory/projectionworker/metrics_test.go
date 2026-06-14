/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package projectionworker

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetrics_RegisterAndObserve(t *testing.T) {
	m := NewMetrics()
	reg := prometheus.NewRegistry()
	m.MustRegister(reg)
	m.RendersTotal.WithLabelValues("ws", "policy", "ok").Inc()
	m.RenderSeconds.WithLabelValues("ws", "policy").Observe(0.5)
	got, err := reg.Gather()
	if err != nil || len(got) == 0 {
		t.Fatalf("gather: %v len=%d", err, len(got))
	}
}
