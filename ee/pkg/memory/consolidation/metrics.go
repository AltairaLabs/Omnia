/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package consolidation

import "github.com/prometheus/client_golang/prometheus"

// Metric label names. Centralised so all collectors stay consistent.
// workspace is the Workspace CR UID (data key); policy is the
// MemoryPolicy CR name (config identifier). Both labels are emitted on
// every consolidation metric — workspace tells operators "whose data is
// this?", policy tells them "which rulebook drove this run?".
const (
	labelWorkspace  = "workspace"
	labelPolicy     = "policy"
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
			Help: "Total consolidation passes per workspace UID, policy name, function, status.",
		}, []string{labelWorkspace, labelPolicy, labelFunction, labelStatus}),
		PassDurationSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "omnia_memory_consolidation_pass_duration_seconds",
			Help:    "Duration of one consolidation pass (per axis).",
			Buckets: prometheus.DefBuckets,
		}, []string{labelWorkspace, labelPolicy, labelFunction}),
		ActionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_memory_consolidation_actions_total",
			Help: "Total actions emitted per workspace UID, policy name, function, action kind, outcome, target tier.",
		}, []string{labelWorkspace, labelPolicy, labelFunction, labelAction, labelOutcome, labelTargetTier}),
		FunctionCallDurationSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "omnia_memory_consolidation_function_call_duration_seconds",
			Help:    "Duration of the HTTP call to a consolidation function.",
			Buckets: prometheus.DefBuckets,
		}, []string{labelWorkspace, labelPolicy, labelFunction}),
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
