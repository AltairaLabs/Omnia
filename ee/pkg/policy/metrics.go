/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metric label constants.
const (
	labelType     = "type"
	labelPolicy   = "policy"
	labelRule     = "rule"
	labelDecision = "decision"
	labelAgent    = "agent"
	labelTool     = "tool"
)

// Decision label values.
const (
	decisionAllow = "allow"
	decisionDeny  = "deny"
)

// PolicyDecisionsTotal counts the total number of policy decisions.
var PolicyDecisionsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "omnia_policy_decisions_total",
		Help: "Total number of policy decisions",
	},
	[]string{labelType, labelPolicy, labelRule, labelDecision},
)

// PolicyEvaluationDuration tracks duration of policy evaluation.
var PolicyEvaluationDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "omnia_policy_evaluation_duration_seconds",
		Help:    "Duration of policy evaluation",
		Buckets: prometheus.DefBuckets,
	},
	[]string{labelType},
)

// PolicyDenialsTotal counts the total number of policy denials.
var PolicyDenialsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "omnia_policy_denials_total",
		Help: "Total number of policy denials",
	},
	[]string{labelPolicy, labelRule, labelAgent, labelTool},
)

// RegisterMetrics registers all policy metrics with the given registerer.
func RegisterMetrics(reg prometheus.Registerer) {
	reg.MustRegister(PolicyDecisionsTotal)
	reg.MustRegister(PolicyEvaluationDuration)
	reg.MustRegister(PolicyDenialsTotal)
}
