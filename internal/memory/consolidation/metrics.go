/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package consolidation

import "github.com/prometheus/client_golang/prometheus"

// Metric label names. Centralised so all collectors stay consistent.
const (
	labelWorkspace  = "workspace"
	labelFunction   = "function"
	labelStatus     = "status"
	labelAction     = "action"
	labelOutcome    = "outcome"
	labelTargetTier = "target_tier"
)

// Metrics holds the Prometheus collectors for the consolidation
// subsystem. All metric names are under omnia_memory_consolidation_*.
//
// Per CLAUDE.md observability boundaries, these are operational
// signals (worker liveness, per-pass timing) — Prometheus is the
// source of truth, not session-api.
type Metrics struct {
	PassesTotal                 *prometheus.CounterVec
	PassDurationSeconds         *prometheus.HistogramVec
	ActionsTotal                *prometheus.CounterVec
	FunctionCallDurationSeconds *prometheus.HistogramVec
}

// NewMetrics constructs a Metrics with collectors not yet registered.
// Caller is responsible for registration (memory-api wires this into
// its existing Prometheus registry).
func NewMetrics() *Metrics {
	return &Metrics{
		PassesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_memory_consolidation_passes_total",
			Help: "Total consolidation passes per workspace, function, status.",
		}, []string{labelWorkspace, labelFunction, labelStatus}),
		PassDurationSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "omnia_memory_consolidation_pass_duration_seconds",
			Help:    "Duration of one consolidation pass (per axis).",
			Buckets: prometheus.DefBuckets,
		}, []string{labelWorkspace, labelFunction}),
		ActionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_memory_consolidation_actions_total",
			Help: "Total actions emitted per workspace, function, action kind, outcome, target tier.",
		}, []string{labelWorkspace, labelFunction, labelAction, labelOutcome, labelTargetTier}),
		FunctionCallDurationSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "omnia_memory_consolidation_function_call_duration_seconds",
			Help:    "Duration of the HTTP call to a consolidation function.",
			Buckets: prometheus.DefBuckets,
		}, []string{labelWorkspace, labelFunction}),
	}
}

// MustRegister registers all collectors with the provided registry.
// Panics on duplicate registration (consistent with Prometheus
// conventions for one-shot wiring at startup).
func (m *Metrics) MustRegister(reg prometheus.Registerer) {
	reg.MustRegister(
		m.PassesTotal,
		m.PassDurationSeconds,
		m.ActionsTotal,
		m.FunctionCallDurationSeconds,
	)
}
