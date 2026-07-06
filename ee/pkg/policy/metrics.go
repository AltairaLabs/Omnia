/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Decision outcomes reported by the DecisionsTotal counter's "outcome" label.
const (
	OutcomeAllowed   = "allowed"
	OutcomeDenied    = "denied"
	OutcomeWouldDeny = "would_deny"
)

// Metrics holds the Prometheus metrics emitted by the policy-broker sidecar.
// ConstLabels {agent, namespace} are baked in at registration so every series
// this process emits carries the same agent identity used by the facade and
// runtime containers (internal/agent.Metrics), letting a single Grafana panel
// join across all three sidecars on those labels.
type Metrics struct {
	// DecisionsTotal counts every ToolPolicy decision the broker makes, by
	// outcome (allowed|denied|would_deny), the calling tool_registry, and the
	// policy/rule that produced the outcome ("" when no rule matched).
	DecisionsTotal *prometheus.CounterVec

	// DecisionDuration is the latency of a single broker decision (request
	// decode through evaluator return), in seconds.
	DecisionDuration prometheus.Histogram

	// ActivePolicies is the number of compiled ToolPolicies currently loaded
	// by the broker's evaluator.
	ActivePolicies prometheus.Gauge
}

// Prometheus label names for the DecisionsTotal counter.
const (
	labelOutcome      = "outcome"
	labelToolRegistry = "tool_registry"
	labelPolicy       = "policy"
)

// NewBrokerMetrics creates and registers the policy-broker's Prometheus
// metrics against the default registry (promhttp.Handler() default), mirroring
// internal/agent.NewMetrics's ConstLabels pattern so the broker's series line
// up with the facade/runtime series on {agent, namespace}.
func NewBrokerMetrics(agentName, namespace string) *Metrics {
	labels := prometheus.Labels{
		"agent":     agentName,
		"namespace": namespace,
	}

	return &Metrics{
		DecisionsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_toolpolicy_decisions_total",
			Help:        "Total number of ToolPolicy decisions made by the policy broker",
			ConstLabels: labels,
		}, []string{labelOutcome, labelToolRegistry, labelPolicy}),

		DecisionDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:        "omnia_toolpolicy_decision_duration_seconds",
			Help:        "Policy broker decision latency in seconds",
			ConstLabels: labels,
			Buckets:     []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5},
		}),

		ActivePolicies: promauto.NewGauge(prometheus.GaugeOpts{
			Name:        "omnia_toolpolicy_active_policies",
			Help:        "Number of compiled ToolPolicies currently loaded by the broker",
			ConstLabels: labels,
		}),
	}
}

// RecordDecision records the outcome, tool registry, and matched ToolPolicy
// name for a single broker decision, plus its latency. The `policy` label is
// the ToolPolicy that produced the decision (`decision.Policy`, the CRD name)
// — empty on a clean allow where no policy denied the call. The specific rule
// that fired (`decision.DeniedBy`) stays in the structured decision logs
// rather than as a metric label, keeping cardinality to policies × registries.
func (m *Metrics) RecordDecision(decision Decision, toolRegistry string, durationSeconds float64) {
	m.DecisionsTotal.WithLabelValues(decisionOutcome(decision), toolRegistry, decision.Policy).Inc()
	m.DecisionDuration.Observe(durationSeconds)
}

// SetActivePolicies sets the current compiled-policy count.
func (m *Metrics) SetActivePolicies(count int) {
	m.ActivePolicies.Set(float64(count))
}

// decisionOutcome classifies a Decision into the outcome label values the
// DecisionsTotal counter uses: "denied" when a rule actually blocked the
// call, "would_deny" when an audit-mode policy would have blocked it, and
// "allowed" otherwise.
func decisionOutcome(decision Decision) string {
	if !decision.Allowed {
		return OutcomeDenied
	}
	if decision.WouldDeny {
		return OutcomeWouldDeny
	}
	return OutcomeAllowed
}
