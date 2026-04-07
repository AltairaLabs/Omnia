/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import "github.com/prometheus/client_golang/prometheus"

const (
	metricRolloutActive          = "omnia_rollout_active"
	metricRolloutStepTransitions = "omnia_rollout_step_transitions_total"
	metricRolloutPromotions      = "omnia_rollout_promotions_total"
	metricRolloutRollbacks       = "omnia_rollout_rollbacks_total"
	metricRolloutStepDuration    = "omnia_rollout_step_duration_seconds"
)

// Label constants for rollout metrics.
const (
	labelNamespace    = "namespace"
	labelAgentRuntime = "agentruntime"
	labelStepType     = "step_type"
	labelReason       = "reason"
)

// RolloutMetrics holds Prometheus metrics for rollout observability.
type RolloutMetrics struct {
	Active          *prometheus.GaugeVec
	StepTransitions *prometheus.CounterVec
	Promotions      *prometheus.CounterVec
	Rollbacks       *prometheus.CounterVec
	StepDuration    *prometheus.HistogramVec
}

// DefaultRolloutDurationBuckets are histogram buckets for rollout step durations.
var DefaultRolloutDurationBuckets = []float64{10, 30, 60, 120, 300, 600, 1800, 3600}

// NewRolloutMetrics creates and registers Prometheus metrics for rollout observability.
func NewRolloutMetrics(reg prometheus.Registerer) *RolloutMetrics {
	m := &RolloutMetrics{
		Active: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: metricRolloutActive,
			Help: "Whether a rollout is currently active for this AgentRuntime",
		}, []string{labelNamespace, labelAgentRuntime}),

		StepTransitions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: metricRolloutStepTransitions,
			Help: "Total rollout step transitions",
		}, []string{labelNamespace, labelAgentRuntime, labelStepType}),

		Promotions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: metricRolloutPromotions,
			Help: "Total rollout promotions",
		}, []string{labelNamespace, labelAgentRuntime}),

		Rollbacks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: metricRolloutRollbacks,
			Help: "Total rollout rollbacks",
		}, []string{labelNamespace, labelAgentRuntime, labelReason}),

		StepDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    metricRolloutStepDuration,
			Help:    "Duration of rollout steps in seconds",
			Buckets: DefaultRolloutDurationBuckets,
		}, []string{labelNamespace, labelAgentRuntime, labelStepType}),
	}

	reg.MustRegister(m.Active, m.StepTransitions, m.Promotions, m.Rollbacks, m.StepDuration)
	return m
}
