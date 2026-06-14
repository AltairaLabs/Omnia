/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package projectionworker

import "github.com/prometheus/client_golang/prometheus"

// Metric label names. workspace is the Workspace CR UID (data key);
// policy is the MemoryPolicy CR name (config identifier).
const (
	labelWorkspace = "workspace"
	labelPolicy    = "policy"
	labelStatus    = "status"
)

// Metrics holds Prometheus collectors for the projection worker
// (omnia_memory_projection_*). Operational signals; Prometheus is the source.
type Metrics struct {
	RendersTotal  *prometheus.CounterVec
	RenderSeconds *prometheus.HistogramVec
}

// NewMetrics constructs a Metrics with collectors not yet registered.
func NewMetrics() *Metrics {
	return &Metrics{
		RendersTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_memory_projection_renders_total",
			Help: "Projection pre-renders per workspace UID, policy, status (ok|error).",
		}, []string{labelWorkspace, labelPolicy, labelStatus}),
		RenderSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "omnia_memory_projection_render_seconds",
			Help:    "Duration of one workspace projection render.",
			Buckets: prometheus.DefBuckets,
		}, []string{labelWorkspace, labelPolicy}),
	}
}

// MustRegister registers all collectors with reg.
func (m *Metrics) MustRegister(reg prometheus.Registerer) {
	reg.MustRegister(m.RendersTotal, m.RenderSeconds)
}
